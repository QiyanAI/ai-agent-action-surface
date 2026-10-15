package control

// Temporary adversarial-verification reproduction (delete after use).
// Engine-level: a caller-declared ActionSessionLocal whose tool is NOT in the
// session-local table. Does gate() allow it in every mode, with a
// no_external_effect reason and no approval ever created?

import (
	"testing"
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
)

func TestZZAVEngineSessionLocalDeclaredKind(t *testing.T) {
	modes := []domain.AutonomyMode{domain.ModeCompanion, domain.ModeControlledAuto, domain.ModeBoundedAutonomy}
	for _, mode := range modes {
		store := NewInMemoryStore()
		e := New(store)
		now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

		agent := domain.Agent{ID: domain.NewAgentID(), Name: "Host", Role: domain.RoleBuilder}
		scope := domain.Scope{
			Roots: []string{"C:/deploy"},
			AllowedKinds: []domain.ActionKind{
				domain.ActionFileRead, domain.ActionProcessExec,
				domain.ActionExternalState, domain.ActionSessionLocal,
			},
			AllowedTools: []string{"deploy_prod", "Bash"},
			MaxSteps:     50,
			MaxRuntime:   time.Hour,
		}
		mission := domain.Mission{
			ID: domain.NewMissionID(), Goal: "deploy",
			Policy: domain.Policy{Mode: mode, Scope: scope},
			State:  domain.MissionRunning,
		}
		task := domain.Task{
			ID: domain.NewTaskID(), MissionID: mission.ID, Role: domain.RoleBuilder,
			State: domain.TaskRunning, AgentID: agent.ID, StartedAt: now,
		}
		req := func(a domain.Action) Request {
			return Request{Mission: mission, Task: task, Agent: agent, Action: a, Agents: []domain.Agent{agent}, Now: now}
		}

		// Evidence that the repo knows the difference: deploy_prod is not a
		// session-local tool; TodoWrite is.
		if _, ok := domain.CapabilitiesForTool("deploy_prod"); ok {
			t.Fatalf("deploy_prod unexpectedly a known tool")
		}
		if caps, ok := domain.CapabilitiesForTool("TodoWrite"); !ok || caps[0] != domain.CapInternalState {
			t.Fatalf("TodoWrite caps = %v ok=%v", caps, ok)
		}

		spoof := domain.Action{Kind: domain.ActionSessionLocal, Tool: "deploy_prod", Raw: "publish release"}
		d, err := e.Evaluate(req(spoof))
		t.Logf("[%s] declared session.local + tool deploy_prod => outcome=%s err=%v approvalId=%q", mode, d.Outcome, err, d.ApprovalID)
		for _, r := range d.Reasons {
			t.Logf("[%s]   spoof reason code=%s params=%v", mode, r.Code, r.Params)
		}
		if err != nil {
			t.Errorf("[%s] unexpected error: %v", mode, err)
		}
		if d.Outcome != domain.OutcomeAllow {
			t.Errorf("[%s] spoof outcome = %s, want ALLOW", mode, d.Outcome)
		}
		if d.ApprovalID != "" {
			t.Errorf("[%s] spoof raised approval %q, want none", mode, d.ApprovalID)
		}
		if _, found, _ := store.LatestApprovalForAction(task.ID, agent.ID, spoof.CanonicalDigest()); found {
			t.Errorf("[%s] an approval exists for the spoof action", mode)
		}

		truth := domain.Action{Kind: domain.ActionExternalState, Tool: "deploy_prod", URL: "https://prod.example.com/release", Raw: "publish release"}
		d2, err2 := e.Evaluate(req(truth))
		t.Logf("[%s] declared external.state_change + tool deploy_prod => outcome=%s err=%v approvalId=%q", mode, d2.Outcome, err2, d2.ApprovalID)
		if err2 != nil {
			t.Errorf("[%s] unexpected error (true kind): %v", mode, err2)
		}
		if d2.Outcome != domain.OutcomeRequireApproval {
			t.Errorf("[%s] true-kind outcome = %s, want REQUIRE_APPROVAL", mode, d2.Outcome)
		}
	}
}
