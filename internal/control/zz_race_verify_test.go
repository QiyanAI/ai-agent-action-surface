package control

// Temporary verification test: deterministically force the legal interleavings
// described in the claim. The Engine and InMemoryStore are the real ones; the
// wrappers only DELAY specific store calls to pin a schedule, never change
// their behavior. This file is deleted after the verification run.

import (
	"sync"
	"testing"
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
)

// gateA forces: Decide reads P and mints the authorization -> CancelApproval
// runs to completion (P=cancelled, returns nil) -> Decide's whole-record
// UpdateApproval lands last.
type gateA struct {
	*InMemoryStore
	authCreated chan struct{}
	cancelDone  chan struct{}
}

func (g *gateA) CreateAuthorization(a domain.Authorization) error {
	if err := g.InMemoryStore.CreateAuthorization(a); err != nil {
		return err
	}
	close(g.authCreated)
	<-g.cancelDone
	return nil
}

func TestVerifyRaceCancelVsApprove(t *testing.T) {
	f := newFixture(t, domain.ModeCompanion)
	action := write("C:/proj/a.go")
	d := f.eval(t, action)
	if d.Outcome != domain.OutcomeRequireApproval {
		t.Fatalf("setup: want REQUIRE_APPROVAL, got %s", d.Outcome)
	}

	g := &gateA{
		InMemoryStore: f.store,
		authCreated:   make(chan struct{}),
		cancelDone:    make(chan struct{}),
	}
	e := New(g)

	type result struct {
		ap  domain.ApprovalRequest
		err error
	}
	decideRes := make(chan result, 1)
	go func() {
		ap, err := e.Decide(d.ApprovalID, true, domain.Actor{Source: domain.SourceLocalUser, Label: "owner"}, f.now)
		decideRes <- result{ap, err}
	}()

	<-g.authCreated // Decide has read P (pending) and minted the grant
	started := time.Now()
	cancelErr := e.CancelApproval(d.ApprovalID, domain.Actor{Source: domain.SourceLocalUser, Label: "operator"}, f.now)
	t.Logf("CancelApproval ran in %v, err=%v", time.Since(started), cancelErr)
	close(g.cancelDone) // release Decide's UpdateApproval

	res := <-decideRes
	if res.err != nil {
		t.Fatalf("Decide error: %v", res.err)
	}
	t.Logf("Decide returned state=%s auth=%v; cancelErr=%v", res.ap.State, res.ap.Authorization != nil, cancelErr)

	stored, found, _ := f.store.GetApproval(d.ApprovalID)
	if !found {
		t.Fatal("P vanished")
	}
	t.Logf("FINAL stored P state = %s", stored.State)

	// Live grant for an action whose cancel succeeded?
	now := f.now
	snap, err := f.store.ClaimAuthorizationForAction(f.task.ID, f.builder.ID,
		action.CanonicalDigest(), f.mission.Policy.Scope.ScopeDigest(), now)
	t.Logf("live unconsumed grant claimable = %v (err=%v, consumedAt=%v)", err == nil, err, snap.ConsumedAt)

	if cancelErr != nil {
		t.Fatalf("premise broken: CancelApproval errored: %v", cancelErr)
	}
	if stored.State != domain.ApprovalApproved {
		t.Fatalf("claim refuted for scenario (a): final state %s (expected approved under the claim)", stored.State)
	}
	if err != nil {
		t.Fatalf("claim refuted for scenario (a): no live grant (err=%v)", err)
	}
	t.Log("SCENARIO (a) REPRODUCED: cancel returned nil yet P ended approved with a live grant")
}

// gateB forces: retry reads P (pending, past TTL) -> user's Deny write lands ->
// retry's stale whole-record expiry write lands on top -> retry creates P2.
type gateB struct {
	*InMemoryStore
	readOnce    sync.Once
	denyOnce    sync.Once
	retryRead   chan struct{}
	denyWritten chan struct{}
}

func (g *gateB) LatestApprovalForAction(task domain.TaskID, agent domain.AgentID, digest string) (domain.ApprovalRequest, bool, error) {
	ap, found, err := g.InMemoryStore.LatestApprovalForAction(task, agent, digest)
	g.readOnce.Do(func() { close(g.retryRead) })
	return ap, found, err
}

func (g *gateB) UpdateApproval(a domain.ApprovalRequest) error {
	if a.State == domain.ApprovalExpired {
		<-g.denyWritten // hold the stale expiry write until the deny has landed
	}
	if err := g.InMemoryStore.UpdateApproval(a); err != nil {
		return err
	}
	if a.State == domain.ApprovalDenied {
		g.denyOnce.Do(func() { close(g.denyWritten) })
	}
	return nil
}

func TestVerifyRaceDenyErasure(t *testing.T) {
	f := newFixture(t, domain.ModeCompanion)
	action := write("C:/proj/a.go")
	d := f.eval(t, action)
	if d.Outcome != domain.OutcomeRequireApproval {
		t.Fatalf("setup: want REQUIRE_APPROVAL, got %s", d.Outcome)
	}
	p, found, _ := f.store.GetApproval(d.ApprovalID)
	if !found {
		t.Fatal("P not found")
	}
	nowDeny := p.ExpiresAt.Add(-time.Second) // user clicks Deny just before TTL
	nowRetry := p.ExpiresAt.Add(time.Second) // retry lands just after TTL

	g := &gateB{
		InMemoryStore: f.store,
		retryRead:     make(chan struct{}),
		denyWritten:   make(chan struct{}),
	}
	e := New(g)

	type retryResult struct {
		dec domain.Decision
		err error
	}
	retryRes := make(chan retryResult, 1)
	go func() {
		req := f.req(action)
		req.Now = nowRetry
		dec, err := e.Evaluate(req)
		retryRes <- retryResult{dec, err}
	}()

	<-g.retryRead // retry has read P as pending (past TTL) -> will take the expiry path
	ap, decideErr := e.Decide(d.ApprovalID, false, domain.Actor{Source: domain.SourceLocalUser, Label: "owner"}, nowDeny)
	rr := <-retryRes
	if rr.err != nil {
		t.Fatalf("retry evaluate error: %v", rr.err)
	}

	storedP, _, _ := f.store.GetApproval(d.ApprovalID)
	latest, ltFound, _ := f.store.LatestApprovalForAction(f.task.ID, f.builder.ID, action.CanonicalDigest())
	t.Logf("Decide err=%v returned state=%s", decideErr, ap.State)
	t.Logf("retry decision=%s freshApproval=%s", rr.dec.Outcome, rr.dec.ApprovalID)
	t.Logf("FINAL stored P state = %s; latest-for-action = %s (id=%s)", storedP.State, latest.State, latest.ID)

	// Next sequential retry, well after everything:
	req3 := f.req(action)
	req3.Now = nowRetry.Add(time.Second)
	d3, err3 := e.Evaluate(req3)
	if err3 != nil {
		t.Fatalf("third evaluate error: %v", err3)
	}
	t.Logf("third retry decision=%s approval=%s", d3.Outcome, d3.ApprovalID)

	if decideErr != nil {
		t.Fatalf("premise broken: user's Deny errored: %v", decideErr)
	}
	if storedP.State != domain.ApprovalExpired {
		t.Fatalf("claim refuted for scenario (b): final state %s (expected expired under the claim)", storedP.State)
	}
	if !ltFound || latest.State != domain.ApprovalPending {
		t.Fatalf("claim refuted for scenario (b): latest-for-action is %s", latest.State)
	}
	if d3.Outcome != domain.OutcomeRequireApproval {
		t.Fatalf("claim refuted for scenario (b): third retry gave %s (expected re-prompt)", d3.Outcome)
	}
	t.Log("SCENARIO (b) REPRODUCED: deny accepted (err=nil) yet P ends expired, P2 pending, retry re-prompts")
}
