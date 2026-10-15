package store

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
)

func testAgent() domain.Agent {
	return domain.Agent{ID: domain.NewAgentID(), Name: "Builder", Role: domain.RoleBuilder}
}

func testScope() domain.Scope {
	return domain.Scope{
		Roots:        []string{"C:/proj"},
		AllowedKinds: []domain.ActionKind{domain.ActionFileModify},
		AllowedTools: []string{"Edit"},
		MaxSteps:     10, MaxRuntime: time.Hour,
	}
}

func putFixture(t *testing.T, s *Store) (domain.Agent, domain.Mission, domain.Task) {
	t.Helper()
	ag := testAgent()
	if err := s.RegisterAgent(ag, "hash-"+string(ag.ID)); err != nil {
		t.Fatal(err)
	}
	m := domain.Mission{ID: domain.NewMissionID(), Goal: "g", State: domain.MissionRunning,
		Policy: domain.Policy{Mode: domain.ModeCompanion, Scope: testScope()}}
	if err := s.PutMission(m); err != nil {
		t.Fatal(err)
	}
	task := domain.Task{ID: domain.NewTaskID(), MissionID: m.ID, Role: domain.RoleBuilder,
		State: domain.TaskRunning, AgentID: ag.ID, StartedAt: time.Now()}
	if err := s.PutTask(task); err != nil {
		t.Fatal(err)
	}
	return ag, m, task
}

func TestPersistAndReload(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ag, m, task := putFixture(t, s)

	action := domain.Action{Kind: domain.ActionFileModify, Tool: "Edit", Path: "C:/proj/a.go"}
	ap := domain.ApprovalRequest{
		ID: domain.NewApprovalID(), Action: action, ActionDigest: action.CanonicalDigest(),
		MissionID: m.ID, TaskID: task.ID, AgentID: ag.ID,
		State: domain.ApprovalPending, RequestedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := s.CreateApproval(ap); err != nil {
		t.Fatal(err)
	}
	auth := domain.Authorization{
		ID: ap.ID, MissionID: m.ID, TaskID: task.ID, AgentID: ag.ID,
		ActionDigest: action.CanonicalDigest(), ScopeDigest: m.Policy.Scope.ScopeDigest(),
		GrantedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
		GrantedBy: domain.Actor{Source: domain.SourceLocalUser},
	}
	if err := s.CreateAuthorization(auth); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeStep(task.ID, 1_000_000); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendDecision(domain.Decision{Outcome: domain.OutcomeAllow, TaskID: task.ID}, action); err != nil {
		t.Fatal(err)
	}
	claimed, err := s.ClaimAuthorizationForAction(task.ID, ag.ID, action.CanonicalDigest(), m.Policy.Scope.ScopeDigest(), time.Now())
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if claimed.ConsumedAt != nil {
		t.Fatal("returned snapshot must be pre-claim")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen: everything must be exactly as left.
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if err := s2.BlockedReason(); err != nil {
		t.Fatalf("store unexpectedly blocked: %v", err)
	}
	gotTask, ok, _ := s2.GetTask(task.ID)
	if !ok || gotTask.StepsUsed != 1 {
		t.Fatalf("task steps not persisted: %+v ok=%v", gotTask, ok)
	}
	gotAg, ok, _ := s2.AgentByTokenHash("hash-" + string(ag.ID))
	if !ok || gotAg.ID != ag.ID {
		t.Fatal("agent token hash not persisted")
	}
	gotAp, ok, _ := s2.GetApproval(ap.ID)
	if !ok || gotAp.State != domain.ApprovalPending {
		t.Fatal("approval not persisted")
	}
	decisions, _ := s2.DecisionsForTask(task.ID)
	if len(decisions) != 1 || decisions[0].Decision.Outcome != domain.OutcomeAllow {
		t.Fatal("decision not persisted")
	}
	// The grant was claimed before the restart; a post-restart claim must fail.
	if _, err := s2.ClaimAuthorizationForAction(task.ID, ag.ID, action.CanonicalDigest(), m.Policy.Scope.ScopeDigest(), time.Now()); domain.ClassOf(err) != domain.ErrApprovalNotFound {
		t.Fatalf("claimed-before-restart grant must stay consumed, got %v", err)
	}
}

func TestCorruptWalBlocksTheStore(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	putFixture(t, s)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	wal := filepath.Join(dir, "wal.jsonl")
	f, err := os.OpenFile(wal, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{this is not json\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if s2.BlockedReason() == nil {
		t.Fatal("corrupt wal must block the store")
	}
	// Every operation must fail closed with a corrupt-store class.
	if _, _, err := s2.GetTask(domain.NewTaskID()); domain.ClassOf(err) != domain.ErrStoreCorrupted {
		t.Fatalf("blocked store must refuse reads, got %v", err)
	}
	ag := testAgent()
	if err := s2.RegisterAgent(ag, "h"); domain.ClassOf(err) != domain.ErrStoreCorrupted {
		t.Fatalf("blocked store must refuse writes, got %v", err)
	}
}

func TestSequenceGapBlocksTheStore(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	putFixture(t, s)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	wal := filepath.Join(dir, "wal.jsonl")
	f, err := os.OpenFile(wal, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"seq":9999,"type":"coverage.put","at":"2026-10-15T00:00:00Z","payload":{}}` + "\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if s2.BlockedReason() == nil {
		t.Fatal("a sequence gap must block the store")
	}
}

func TestCompactionPreservesState(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ag, m, task := putFixture(t, s)
	if err := s.ConsumeStep(task.ID, 1_000_000); err != nil {
		t.Fatal(err)
	}
	if err := s.Compact(); err != nil {
		t.Fatal(err)
	}
	// Mutations after compaction must replay cleanly from the wal.
	if err := s.ConsumeStep(task.ID, 1_000_000); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if err := s2.BlockedReason(); err != nil {
		t.Fatalf("compacted store blocked: %v", err)
	}
	gotTask, ok, _ := s2.GetTask(task.ID)
	if !ok || gotTask.StepsUsed != 2 {
		t.Fatalf("steps across compaction wrong: %+v", gotTask)
	}
	if _, ok, _ := s2.GetMission(m.ID); !ok {
		t.Fatal("mission lost in compaction")
	}
	if gotAg, ok, _ := s2.AgentByID(ag.ID); !ok || gotAg.Name != ag.Name {
		t.Fatal("agent lost in compaction")
	}
}

func TestClaimIsAtomicAndSingleUse(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ag, m, task := putFixture(t, s)
	action := domain.Action{Kind: domain.ActionFileModify, Tool: "Edit", Path: "C:/proj/a.go"}
	auth := domain.Authorization{
		ID: domain.NewApprovalID(), TaskID: task.ID, AgentID: ag.ID,
		ActionDigest: action.CanonicalDigest(), ScopeDigest: m.Policy.Scope.ScopeDigest(),
		GrantedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := s.CreateAuthorization(auth); err != nil {
		t.Fatal(err)
	}

	const workers = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.ClaimAuthorizationForAction(task.ID, ag.ID, action.CanonicalDigest(), m.Policy.Scope.ScopeDigest(), time.Now()); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if successes != 1 {
		t.Fatalf("exactly one concurrent claim may succeed, got %d", successes)
	}
	// And a later claim finds nothing.
	if _, err := s.ClaimAuthorizationForAction(task.ID, ag.ID, action.CanonicalDigest(), m.Policy.Scope.ScopeDigest(), time.Now()); domain.ClassOf(err) != domain.ErrApprovalNotFound {
		t.Fatalf("consumed grant must not be claimable again, got %v", err)
	}
}

func TestConsumeStepRequiresRegisteredTask(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.ConsumeStep(domain.NewTaskID(), 10); domain.ClassOf(err) != domain.ErrNotFound {
		t.Fatalf("unknown task must not silently consume budget, got %v", err)
	}
}

func TestConsumeStepBudgetAndTransitionApprovalCAS(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ag, m, task := putFixture(t, s)

	// Budget: exactly maxSteps consumptions succeed.
	for i := 0; i < 3; i++ {
		if err := s.ConsumeStep(task.ID, 3); err != nil {
			t.Fatalf("consume %d: %v", i, err)
		}
	}
	if err := s.ConsumeStep(task.ID, 3); domain.ClassOf(err) != domain.ErrBudgetExhausted {
		t.Fatalf("budget must be exhausted, got %v", err)
	}

	// Concurrent ConsumeStep: only the remaining budget may succeed.
	task2 := domain.Task{ID: domain.NewTaskID(), MissionID: m.ID, Role: domain.RoleBuilder,
		State: domain.TaskRunning, AgentID: ag.ID}
	if err := s.PutTask(task2); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	okCount := 0
	var mu sync.Mutex
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.ConsumeStep(task2.ID, 5); err == nil {
				mu.Lock()
				okCount++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if okCount != 5 {
		t.Fatalf("exactly 5 concurrent consumptions may pass a budget of 5, got %d", okCount)
	}

	// TransitionApproval CAS: one pending approval, one winner.
	ap := domain.ApprovalRequest{ID: domain.NewApprovalID(), TaskID: task.ID, AgentID: ag.ID,
		State: domain.ApprovalPending, RequestedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	if err := s.CreateApproval(ap); err != nil {
		t.Fatal(err)
	}
	_, won1, err := s.TransitionApproval(ap.ID, domain.ApprovalPending, domain.ApprovalApproved, time.Now(), domain.Actor{Source: domain.SourceLocalUser})
	if err != nil || !won1 {
		t.Fatalf("first transition must win: %v %v", won1, err)
	}
	_, won2, err := s.TransitionApproval(ap.ID, domain.ApprovalPending, domain.ApprovalDenied, time.Now(), domain.Actor{Source: domain.SourceLocalUser})
	if err != nil || won2 {
		t.Fatalf("second transition must lose: %v %v", won2, err)
	}
}
