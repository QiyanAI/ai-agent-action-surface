package control

// Temporary adversarial-verification tests. These use ONLY the real engine code
// and the pluggable Store interface; the wrapper below parks a goroutine at the
// exact point the scheduler can preempt it between its store read and store
// write (the window the production store widens with a WAL append+fsync).

import (
	"sync"
	"testing"
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
)

type zzGate struct {
	Store
	onCreateAuth func(domain.Authorization)
	onUpdate     func(domain.ApprovalRequest)
}

func (g *zzGate) CreateAuthorization(a domain.Authorization) error {
	if g.onCreateAuth != nil {
		g.onCreateAuth(a)
	}
	return g.Store.CreateAuthorization(a)
}

func (g *zzGate) UpdateApproval(a domain.ApprovalRequest) error {
	if g.onUpdate != nil {
		g.onUpdate(a)
	}
	return g.Store.UpdateApproval(a)
}

// (a) Cancel vs approve: Decide reads P=pending, and while it mints the
// authorization (WAL append+fsync on the production store) CancelApproval
// reads P=pending, writes P=cancelled and reports success; Decide's
// whole-record update then lands and P ends approved with a live grant.
func TestVerifyRace_CancelLosesToConcurrentApprove(t *testing.T) {
	f := newFixture(t, domain.ModeCompanion)
	now := f.now

	action := domain.Action{Kind: domain.ActionNetwork, Tool: "WebFetch", URL: "https://example.com/exfil"}
	scope := f.mission.Policy.Scope

	ap := domain.ApprovalRequest{
		ID:           domain.NewApprovalID(),
		Action:       action,
		ActionDigest: action.CanonicalDigest(),
		MissionID:    f.mission.ID,
		TaskID:       f.task.ID,
		AgentID:      f.builder.ID,
		ScopeDigest:  scope.ScopeDigest(),
		State:        domain.ApprovalPending,
		RequestedAt:  now,
		ExpiresAt:    now.Add(domain.ApprovalTTL),
	}
	if err := f.store.CreateApproval(ap); err != nil {
		t.Fatalf("seed approval: %v", err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	g := &zzGate{Store: f.store}
	g.onCreateAuth = func(domain.Authorization) {
		once.Do(func() { close(entered) })
		<-release
	}
	eng := New(g)

	type result struct {
		ap  domain.ApprovalRequest
		err error
	}
	decideDone := make(chan result, 1)
	go func() {
		a, err := eng.Decide(ap.ID, true, domain.Actor{Source: domain.SourceLocalUser, Label: "operator"}, now.Add(time.Minute))
		decideDone <- result{a, err}
	}()

	// Decide has read P=pending and is parked inside CreateAuthorization.
	<-entered

	// The orchestrator cancels the approval (e.g. the task-cancel path).
	if err := eng.CancelApproval(ap.ID, domain.Actor{Source: domain.SourceLocalUser, Label: "operator"}, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("CancelApproval returned error: %v", err)
	}
	// At this instant the store really does record "cancelled".
	if got, _, _ := f.store.GetApproval(ap.ID); got.State != domain.ApprovalCancelled {
		t.Fatalf("expected cancelled in store at this point, got %s", got.State)
	}

	close(release)
	res := <-decideDone
	if res.err != nil {
		t.Fatalf("Decide(approve) failed: %v", res.err)
	}

	got, _, err := f.store.GetApproval(ap.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != domain.ApprovalApproved {
		t.Fatalf("claim refuted: final state is %s", got.State)
	}
	if got.Authorization == nil {
		t.Fatal("claim refuted: no authorization on the record")
	}

	// The grant is live: a retry for the cancelled action is allowed.
	retry := f.req(action)
	retry.Now = now.Add(3 * time.Minute)
	d, err := eng.Evaluate(retry)
	if err != nil {
		t.Fatalf("retry Evaluate: %v", err)
	}
	if d.Outcome != domain.OutcomeAllow {
		t.Fatalf("claim refuted: retry got %s, want ALLOW via the cancelled approval's grant", d.Outcome)
	}
	t.Logf("reproduced: cancel returned nil, store ended %s, retry ALLOWed for the cancelled action (approval %s)", got.State, ap.ID)
}

// (b) Deny erasure: the retry's requireApproval has already read P=pending and
// decided it is stale; while it is between that read and its UpdateApproval,
// the user denies P one second before the TTL. The deny lands, then the stale
// expiry write lands on top of it: the recorded denial disappears.
func TestVerifyRace_DenyErasedByExpiryRefresh(t *testing.T) {
	f := newFixture(t, domain.ModeCompanion)
	now := f.now

	action := domain.Action{Kind: domain.ActionFileDelete, Tool: "Bash", Path: "C:/proj/gone.txt"}
	scope := f.mission.Policy.Scope

	ap := domain.ApprovalRequest{
		ID:           domain.NewApprovalID(),
		Action:       action,
		ActionDigest: action.CanonicalDigest(),
		MissionID:    f.mission.ID,
		TaskID:       f.task.ID,
		AgentID:      f.builder.ID,
		ScopeDigest:  scope.ScopeDigest(),
		State:        domain.ApprovalPending,
		RequestedAt:  now,
		ExpiresAt:    now.Add(domain.ApprovalTTL),
	}
	if err := f.store.CreateApproval(ap); err != nil {
		t.Fatalf("seed approval: %v", err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	g := &zzGate{Store: f.store}
	g.onUpdate = func(a domain.ApprovalRequest) {
		if a.State == domain.ApprovalExpired {
			once.Do(func() { close(entered) })
			<-release
		}
	}
	eng := New(g)

	// The agent retries one second after the TTL: it reads P as pending and
	// un-decidable, and heads for the expiry write.
	retryNow := now.Add(domain.ApprovalTTL + time.Second)
	req := f.req(action)
	req.Now = retryNow

	type evalResult struct {
		dec domain.Decision
		err error
	}
	retryDone := make(chan evalResult, 1)
	go func() {
		d, err := eng.Evaluate(req)
		retryDone <- evalResult{d, err}
	}()

	// The retry has read P=pending and is parked just before its expiry write.
	<-entered

	// The user clicks Deny while P is still inside its TTL.
	denied, err := eng.Decide(ap.ID, false, domain.Actor{Source: domain.SourceLocalUser, Label: "operator"}, now.Add(domain.ApprovalTTL-time.Second))
	if err != nil {
		t.Fatalf("Decide(deny) failed: %v", err)
	}
	if denied.State != domain.ApprovalDenied {
		t.Fatalf("Decide(deny) returned %s", denied.State)
	}
	if stored, _, _ := f.store.GetApproval(ap.ID); stored.State != domain.ApprovalDenied {
		t.Fatalf("expected the deny recorded at this point, got %s", stored.State)
	}

	close(release)
	res := <-retryDone
	if res.err != nil {
		t.Fatalf("retry Evaluate error: %v", res.err)
	}
	if res.dec.Outcome != domain.OutcomeRequireApproval {
		t.Fatalf("retry got %s, want REQUIRE_APPROVAL", res.dec.Outcome)
	}

	stored, _, err := f.store.GetApproval(ap.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != domain.ApprovalExpired {
		t.Fatalf("claim refuted: deny survived, state=%s", stored.State)
	}

	// The next retry re-prompts: the deny is not remembered as final.
	req2 := f.req(action)
	req2.Now = retryNow.Add(time.Second)
	d2, err := eng.Evaluate(req2)
	if err != nil {
		t.Fatalf("second retry Evaluate: %v", err)
	}
	if d2.Outcome != domain.OutcomeRequireApproval {
		t.Fatalf("claim refuted: second retry got %s, want a fresh REQUIRE_APPROVAL", d2.Outcome)
	}
	if d2.ApprovalID != res.dec.ApprovalID {
		t.Fatalf("fresh prompt is %s, want %s", d2.ApprovalID, res.dec.ApprovalID)
	}
	t.Logf("reproduced: deny on %s was erased (state %s) and the next retry re-prompted with %s", ap.ID, stored.State, d2.ApprovalID)
}
