package control

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
)

// --- fixtures ---

type fixture struct {
	engine   *Engine
	store    *InMemoryStore
	mission  domain.Mission
	task     domain.Task
	builder  domain.Agent
	planner  domain.Agent
	reviewer domain.Agent
	now      time.Time
}

func newFixture(t *testing.T, mode domain.AutonomyMode) *fixture {
	t.Helper()
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	store := NewInMemoryStore()
	e := New(store)

	builder := domain.Agent{ID: domain.NewAgentID(), Name: "Builder", Role: domain.RoleBuilder}
	planner := domain.Agent{ID: domain.NewAgentID(), Name: "Planner", Role: domain.RolePlanner, ReadOnly: true}
	reviewer := domain.Agent{ID: domain.NewAgentID(), Name: "Reviewer", Role: domain.RoleReviewer, ReadOnly: true}

	scope := domain.Scope{
		Roots: []string{"C:/proj"},
		AllowedKinds: []domain.ActionKind{
			domain.ActionFileRead, domain.ActionFileCreate, domain.ActionFileModify, domain.ActionFileDelete,
			domain.ActionProcessExec, domain.ActionNetwork, domain.ActionCredentialUse,
			domain.ActionExternalState, domain.ActionDelegation,
		},
		AllowedTools: []string{"Read", "Write", "Edit", "Bash", "WebFetch", "Task", "note.append"},
		MaxSteps:     50,
		MaxRuntime:   time.Hour,
	}
	mission := domain.Mission{
		ID:     domain.NewMissionID(),
		Goal:   "test mission",
		Policy: domain.Policy{Mode: mode, Scope: scope},
		State:  domain.MissionRunning,
	}
	task := domain.Task{
		ID:        domain.NewTaskID(),
		MissionID: mission.ID,
		Role:      domain.RoleBuilder,
		State:     domain.TaskRunning,
		AgentID:   builder.ID,
		StartedAt: now,
	}
	return &fixture{
		engine: e, store: store, mission: mission, task: task,
		builder: builder, planner: planner, reviewer: reviewer, now: now,
	}
}

func (f *fixture) roster() []domain.Agent {
	return []domain.Agent{f.builder, f.planner, f.reviewer}
}

func (f *fixture) req(a domain.Action) Request {
	return Request{
		Mission: f.mission, Task: f.task, Agent: f.builder, Action: a,
		Agents: f.roster(), Now: f.now,
	}
}

func (f *fixture) eval(t *testing.T, a domain.Action) domain.Decision {
	t.Helper()
	d, err := f.engine.Evaluate(f.req(a))
	if err != nil {
		t.Fatalf("Evaluate(%v) unexpected error: %v", a.Kind, err)
	}
	return d
}

func write(path string) domain.Action {
	return domain.Action{Kind: domain.ActionFileModify, Tool: "Edit", Path: path}
}

func read(path string) domain.Action {
	return domain.Action{Kind: domain.ActionFileRead, Tool: "Read", Path: path}
}

func bash(cmd string) domain.Action {
	// Cwd set to the task's working directory, as the hook path does; this is
	// what the Windows cwd-shadowing check evaluates.
	return domain.Action{Kind: domain.ActionProcessExec, Tool: "Bash", Command: cmd, CommandProgram: ExtractProgram(cmd), Cwd: "C:/proj"}
}

// --- positive cases ---

func TestReadInsideScopeAllowed(t *testing.T) {
	f := newFixture(t, domain.ModeCompanion)
	if got := f.eval(t, read("C:/proj/a.txt")).Outcome; got != domain.OutcomeAllow {
		t.Fatalf("in-scope read should allow, got %s", got)
	}
}

func TestWriteAllowedInControlledAuto(t *testing.T) {
	f := newFixture(t, domain.ModeControlledAuto)
	if got := f.eval(t, write("C:/proj/src/a.go")).Outcome; got != domain.OutcomeAllow {
		t.Fatalf("in-scope write should allow in controlled-auto, got %s", got)
	}
}

func TestSafeCommandAllowedInAutonomousModes(t *testing.T) {
	// Curated commands run without asking in the two autonomous modes; in
	// companion mode even a read-only program waits for the user.
	for _, mode := range []domain.AutonomyMode{domain.ModeControlledAuto, domain.ModeBoundedAutonomy} {
		f := newFixture(t, mode)
		for _, cmd := range []string{"go build ./cmd/qiyan", "go vet ./...", "go list ./...", "git status", "ls -la", "cat notes.md"} {
			if got := f.eval(t, bash(cmd)).Outcome; got != domain.OutcomeAllow {
				t.Errorf("mode %s: %q should be allowed, got %s", mode, cmd, got)
			}
		}
	}
}

func TestCompanionGatesEvenReadOnlyPrograms(t *testing.T) {
	f := newFixture(t, domain.ModeCompanion)
	for _, cmd := range []string{"go test ./...", "git status", "ls -la"} {
		if got := f.eval(t, bash(cmd)).Outcome; got != domain.OutcomeRequireApproval {
			t.Errorf("companion mode: %q must wait for the user, got %s", cmd, got)
		}
	}
}

// --- negative / gating cases ---

func TestWriteOutsideScopeDenied(t *testing.T) {
	f := newFixture(t, domain.ModeBoundedAutonomy)
	d := f.eval(t, write("C:/elsewhere/evil.txt"))
	if d.Outcome != domain.OutcomeDeny {
		t.Fatalf("out-of-scope write must deny, got %s", d.Outcome)
	}
	if d.Reasons[0].Code != domain.ReasonPathOutsideScope {
		t.Fatalf("wrong reason: %v", d.Reasons[0].Code)
	}
}

func TestUnnormalizedPathDeniedNotGuessed(t *testing.T) {
	f := newFixture(t, domain.ModeBoundedAutonomy)
	for _, p := range []string{`C:\proj\a.txt`, "proj/a.txt", "../proj/a.txt", ""} {
		if got := f.eval(t, write(p)).Outcome; got != domain.OutcomeDeny {
			t.Errorf("path %q must deny (never guess), got %s", p, got)
		}
	}
}

func TestDeleteWaitsForUserInEveryMode(t *testing.T) {
	for _, mode := range []domain.AutonomyMode{domain.ModeCompanion, domain.ModeControlledAuto, domain.ModeBoundedAutonomy} {
		f := newFixture(t, mode)
		a := domain.Action{Kind: domain.ActionFileDelete, Tool: "Bash", Path: "C:/proj/old.txt"}
		if got := f.eval(t, a).Outcome; got != domain.OutcomeRequireApproval {
			t.Errorf("mode %s: delete must require approval, got %s", mode, got)
		}
	}
}

func TestSensitiveAndUnrecognizedCommandsRequireApproval(t *testing.T) {
	f := newFixture(t, domain.ModeControlledAuto)
	cmds := []string{
		"rm -rf build",         // sensitive program
		"git push origin main", // non-curated subcommand
		"npm install",          // package manager
		"cat a.txt > b.txt",    // redirection metachar
		"rm x && rm y",         // compound
		"frobnicate --all",     // unknown program
		"python3 script.py",    // interpreter
	}
	for _, c := range cmds {
		if got := f.eval(t, bash(c)).Outcome; got != domain.OutcomeRequireApproval {
			t.Errorf("%q must require approval, got %s", c, got)
		}
	}
}

func TestNetworkCredentialExternalStateRequireApproval(t *testing.T) {
	f := newFixture(t, domain.ModeBoundedAutonomy)
	actions := []domain.Action{
		{Kind: domain.ActionNetwork, Tool: "WebFetch", URL: "https://example.com"},
		{Kind: domain.ActionCredentialUse, Tool: "note.append", CredentialRef: "DEPLOY_TOKEN"},
		{Kind: domain.ActionExternalState, Tool: "note.append", URL: "https://api.example.com/orders"},
	}
	for _, a := range actions {
		if got := f.eval(t, a).Outcome; got != domain.OutcomeRequireApproval {
			t.Errorf("%v must require approval, got %s", a.Kind, got)
		}
	}
}

func TestIdentityMismatchDenied(t *testing.T) {
	f := newFixture(t, domain.ModeControlledAuto)
	req := f.req(read("C:/proj/a.txt"))
	req.Agent = f.reviewer // task is assigned to the builder
	d, err := f.engine.Evaluate(req)
	if err != nil || d.Outcome != domain.OutcomeDeny {
		t.Fatalf("impersonation must deny, got %s err=%v", d.Outcome, err)
	}
}

func TestAgentOutsideRosterDenied(t *testing.T) {
	f := newFixture(t, domain.ModeControlledAuto)
	stranger := f.builder
	stranger.ID = domain.NewAgentID()
	stranger.Name = "Stranger"
	f.task.AgentID = stranger.ID // sneaks into the task binding
	req := f.req(read("C:/proj/a.txt"))
	req.Task = f.task
	req.Agent = stranger
	d, err := f.engine.Evaluate(req)
	if err != nil || d.Outcome != domain.OutcomeDeny {
		t.Fatalf("agent outside the mission roster must deny, got %s err=%v", d.Outcome, err)
	}
}

func TestReadOnlyRoleCannotWriteEvenWithApproval(t *testing.T) {
	f := newFixture(t, domain.ModeControlledAuto)
	f.task.AgentID = f.planner.ID
	req := f.req(write("C:/proj/a.txt"))
	req.Agent = f.planner
	d, err := f.engine.Evaluate(req)
	if err != nil {
		t.Fatal(err)
	}
	if d.Outcome != domain.OutcomeDeny || d.Reasons[0].Code != domain.ReasonWriteByReadOnly {
		t.Fatalf("read-only write must deny with write_by_readonly, got %s %v", d.Outcome, d.Reasons)
	}
}

func TestBudgetStopsTheTask(t *testing.T) {
	f := newFixture(t, domain.ModeBoundedAutonomy)
	f.task.StepsUsed = f.mission.Policy.Scope.MaxSteps
	req := f.req(read("C:/proj/a.txt"))
	if d, _ := f.engine.Evaluate(req); d.Outcome != domain.OutcomeDeny {
		t.Fatalf("spent step budget must deny, got %s", d.Outcome)
	}

	f2 := newFixture(t, domain.ModeBoundedAutonomy)
	f2.task.StartedAt = f2.now.Add(-2 * time.Hour)
	req2 := f2.req(read("C:/proj/a.txt"))
	if d, _ := f2.engine.Evaluate(req2); d.Outcome != domain.OutcomeDeny {
		t.Fatalf("expired runtime budget must deny, got %s", d.Outcome)
	}
}

func TestCancelledTaskDenied(t *testing.T) {
	f := newFixture(t, domain.ModeBoundedAutonomy)
	f.task.State = domain.TaskCancelled
	req := f.req(read("C:/proj/a.txt"))
	if d, _ := f.engine.Evaluate(req); d.Outcome != domain.OutcomeDeny {
		t.Fatalf("cancelled task must deny, got %s", d.Outcome)
	}
}

func TestToolAndKindScope(t *testing.T) {
	f := newFixture(t, domain.ModeControlledAuto)
	outsideTool := read("C:/proj/a.txt")
	outsideTool.Tool = "Mystery"
	if d := f.eval(t, outsideTool); d.Outcome != domain.OutcomeDeny {
		t.Fatalf("tool outside allowlist must deny, got %s", d.Outcome)
	}

	f2 := newFixture(t, domain.ModeControlledAuto)
	f2.mission.Policy.Scope.AllowedKinds = []domain.ActionKind{domain.ActionFileRead}
	if d := f2.eval(t, write("C:/proj/a.txt")); d.Outcome != domain.OutcomeDeny {
		t.Fatalf("kind outside scope must deny, got %s", d.Outcome)
	}
}

func TestUnknownActionNeverAllowed(t *testing.T) {
	f := newFixture(t, domain.ModeBoundedAutonomy)
	// A mission must opt in to the unknown kind explicitly to get the
	// approval gate for it; without that opt-in the structural check denies.
	f.mission.Policy.Scope.AllowedKinds = append(f.mission.Policy.Scope.AllowedKinds, domain.ActionUnknown)
	a := domain.Action{Kind: domain.ActionUnknown, Tool: "note.append", Raw: "unclassifiable"}
	if got := f.eval(t, a).Outcome; got != domain.OutcomeRequireApproval {
		t.Fatalf("unknown kind in scope must at most require approval, got %s", got)
	}

	f2 := newFixture(t, domain.ModeBoundedAutonomy)
	f2.mission.Policy.Scope.AllowedKinds = []domain.ActionKind{domain.ActionFileRead}
	if got := f2.eval(t, a).Outcome; got != domain.OutcomeDeny {
		t.Fatalf("unknown kind outside scope must deny, got %s", got)
	}
}

// --- approval lifecycle ---

func TestCompanionWriteApprovalHappyPath(t *testing.T) {
	f := newFixture(t, domain.ModeCompanion)
	d := f.eval(t, write("C:/proj/src/a.go"))
	if d.Outcome != domain.OutcomeRequireApproval || d.ApprovalID == "" {
		t.Fatalf("companion write must ask with an approval ID, got %s id=%q", d.Outcome, d.ApprovalID)
	}

	// Agents cannot approve their own actions.
	if _, err := f.engine.Decide(d.ApprovalID, true, domain.Actor{Source: domain.SourceAgent, AgentID: f.builder.ID}, f.now); domain.ClassOf(err) != domain.ErrIdentityConflict {
		t.Fatalf("agent self-approval must be rejected, got %v", err)
	}

	ap, err := f.engine.Decide(d.ApprovalID, true, domain.Actor{Source: domain.SourceLocalUser, Label: "owner"}, f.now)
	if err != nil {
		t.Fatalf("user approval failed: %v", err)
	}
	if ap.Authorization == nil {
		t.Fatal("approved approval must carry an authorization")
	}

	// The approved retry runs the authorized action — the retry itself carries
	// nothing but the same tool call; the grant is claimed by exact binding.
	if got := f.eval(t, write("C:/proj/src/a.go")).Outcome; got != domain.OutcomeAllow {
		t.Fatalf("approved retry must allow, got %s", got)
	}

	// The grant is single-use: the next identical attempt is gated again,
	// with a fresh approval to decide on.
	d3 := f.eval(t, write("C:/proj/src/a.go"))
	if d3.Outcome != domain.OutcomeRequireApproval {
		t.Fatalf("a consumed grant must not authorize again, got %s", d3.Outcome)
	}
	if d3.ApprovalID == d.ApprovalID {
		t.Fatal("the fresh request must be a new approval, not the spent one")
	}
}

func TestApprovalCannotBeSwappedToAnotherTarget(t *testing.T) {
	f := newFixture(t, domain.ModeCompanion)
	d := f.eval(t, write("C:/proj/src/a.go"))
	if _, err := f.engine.Decide(d.ApprovalID, true, domain.Actor{Source: domain.SourceLocalUser}, f.now); err != nil {
		t.Fatal(err)
	}
	// A different target is a different action: the grant for a.go must not
	// slide over to OTHER.go (S06). It gets gated like any new action.
	if got := f.eval(t, write("C:/proj/src/OTHER.go")).Outcome; got == domain.OutcomeAllow {
		t.Fatal("approval must not transfer to a changed action (S06)")
	}
	// And the untouched grant still authorizes exactly its own action once.
	if got := f.eval(t, write("C:/proj/src/a.go")).Outcome; got != domain.OutcomeAllow {
		t.Fatalf("the original grant should still authorize its exact action, got %s", got)
	}
}

func TestApprovalInvalidatedByScopeChange(t *testing.T) {
	f := newFixture(t, domain.ModeCompanion)
	d := f.eval(t, write("C:/proj/a.go"))
	if _, err := f.engine.Decide(d.ApprovalID, true, domain.Actor{Source: domain.SourceLocalUser}, f.now); err != nil {
		t.Fatal(err)
	}
	f.mission.Policy.Scope.Roots = []string{"C:/proj", "C:/extra"} // widened afterwards
	if got := f.eval(t, write("C:/proj/a.go")).Outcome; got == domain.OutcomeAllow {
		t.Fatal("scope change must invalidate the grant, it must not silently apply")
	}
}

func TestExpiredAuthorizationDenied(t *testing.T) {
	f := newFixture(t, domain.ModeCompanion)
	d := f.eval(t, write("C:/proj/a.go"))
	if _, err := f.engine.Decide(d.ApprovalID, true, domain.Actor{Source: domain.SourceLocalUser}, f.now); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(domain.AuthorizationTTL + time.Minute)
	if got := f.eval(t, write("C:/proj/a.go")).Outcome; got == domain.OutcomeAllow {
		t.Fatal("expired grant must not authorize")
	}
}

func TestDeniedApprovalMeansDeniedRetries(t *testing.T) {
	f := newFixture(t, domain.ModeCompanion)
	d := f.eval(t, write("C:/proj/a.go"))
	if _, err := f.engine.Decide(d.ApprovalID, false, domain.Actor{Source: domain.SourceLocalUser}, f.now); err != nil {
		t.Fatal(err)
	}
	d2 := f.eval(t, write("C:/proj/a.go"))
	if d2.Outcome != domain.OutcomeDeny {
		t.Fatalf("retry after user denial must deny, got %s", d2.Outcome)
	}
	if d2.Reasons[0].Code != domain.ReasonUserDenied {
		t.Fatalf("reason should be user_denied, got %v", d2.Reasons[0].Code)
	}
}

func TestPendingApprovalIsDedupedAndRefreshed(t *testing.T) {
	f := newFixture(t, domain.ModeCompanion)
	d1 := f.eval(t, write("C:/proj/a.go"))
	d2 := f.eval(t, write("C:/proj/a.go"))
	if d1.ApprovalID != d2.ApprovalID {
		t.Fatalf("a second evaluation of the same pending action must reuse the approval")
	}
	// After expiry the next ask is a fresh request.
	f.now = f.now.Add(domain.ApprovalTTL + time.Minute)
	d3 := f.eval(t, write("C:/proj/a.go"))
	if d3.ApprovalID == d1.ApprovalID {
		t.Fatalf("expired approval must not be reused")
	}
}

func TestExpiredApprovalCannotBeApprovedLater(t *testing.T) {
	f := newFixture(t, domain.ModeCompanion)
	d := f.eval(t, write("C:/proj/a.go"))
	if _, err := f.engine.Decide(d.ApprovalID, true, domain.Actor{Source: domain.SourceLocalUser}, f.now.Add(domain.ApprovalTTL+time.Minute)); domain.ClassOf(err) != domain.ErrConflict {
		t.Fatalf("late approval must be refused, got %v", err)
	}
}

// --- delegation ---

func delegation(from domain.Agent, to domain.AgentID, kind domain.ActionKind) domain.Action {
	return domain.Action{Kind: domain.ActionDelegation, Tool: "Task", DelegationTo: to, DelegationKind: kind}
}

func TestDelegationPipelineAllowsAndEscalationDenies(t *testing.T) {
	f := newFixture(t, domain.ModeControlledAuto)

	// Planner → Builder handing off a write: allowed even though planner is
	// read-only (delegation is coordination, and the builder's own authority
	// is checked at the builder's own evaluation).
	f.task.AgentID = f.planner.ID
	req := f.req(delegation(f.planner, f.builder.ID, domain.ActionFileModify))
	req.Agent = f.planner
	if d, err := f.engine.Evaluate(req); err != nil || d.Outcome != domain.OutcomeAllow {
		t.Fatalf("planner→builder handoff should allow, got %s err=%v", d.Outcome, err)
	}

	// Reviewer → Builder feedback loop: allowed.
	f.task.AgentID = f.reviewer.ID
	req = f.req(delegation(f.reviewer, f.builder.ID, domain.ActionFileModify))
	req.Agent = f.reviewer
	if d, err := f.engine.Evaluate(req); err != nil || d.Outcome != domain.OutcomeAllow {
		t.Fatalf("reviewer→builder feedback should allow, got %s err=%v", d.Outcome, err)
	}

	// Builder → Planner: not a supported transition.
	f.task.AgentID = f.builder.ID
	if d := f.eval(t, delegation(f.builder, f.planner.ID, domain.ActionFileRead)); d.Outcome != domain.OutcomeDeny {
		t.Fatalf("builder→planner must deny, got %s", d.Outcome)
	}

	// Mutating work aimed at a read-only receiver: escalation.
	f.task.AgentID = f.planner.ID
	req = f.req(delegation(f.planner, f.reviewer.ID, domain.ActionFileModify))
	req.Agent = f.planner
	if d, _ := f.engine.Evaluate(req); d.Outcome != domain.OutcomeDeny {
		t.Fatalf("mutating delegation to a read-only receiver must deny, got %s", d.Outcome)
	}

	// Delegating a kind the mission scope does not cover.
	scopeless := f.mission
	scopeless.Policy.Scope.AllowedKinds = []domain.ActionKind{domain.ActionFileRead}
	req = f.req(delegation(f.planner, f.builder.ID, domain.ActionFileModify))
	req.Agent = f.planner
	req.Mission = scopeless
	if d, _ := f.engine.Evaluate(req); d.Outcome != domain.OutcomeDeny {
		t.Fatalf("delegating outside the mission envelope must deny, got %s", d.Outcome)
	}

	// Delegating the right to delegate, and delegating to strangers: denied.
	req = f.req(delegation(f.planner, f.builder.ID, domain.ActionDelegation))
	req.Agent = f.planner
	if d, _ := f.engine.Evaluate(req); d.Outcome != domain.OutcomeDeny {
		t.Fatalf("transitive delegation must deny, got %s", d.Outcome)
	}
	req = f.req(delegation(f.planner, domain.NewAgentID(), domain.ActionFileModify))
	req.Agent = f.planner
	if d, _ := f.engine.Evaluate(req); d.Outcome != domain.OutcomeDeny {
		t.Fatalf("delegation to an unknown agent must deny, got %s", d.Outcome)
	}
}

// --- fail-closed plumbing ---

type brokenAuditStore struct{ *InMemoryStore }

func (b brokenAuditStore) AppendDecision(domain.Decision, domain.Action) error {
	return domain.NewError(domain.ErrStoreCorrupted, "disk on fire")
}

func TestAuditFailureDeniesTheAction(t *testing.T) {
	f := newFixture(t, domain.ModeControlledAuto)
	e := New(brokenAuditStore{f.store})
	d, err := e.Evaluate(f.req(read("C:/proj/a.txt")))
	if err == nil {
		t.Fatal("unauditable decision must surface an error")
	}
	if d.Outcome != domain.OutcomeDeny {
		t.Fatalf("unauditable decision must deny, got %s", d.Outcome)
	}
	if domain.FailClosedOutcome(err) != domain.OutcomeDeny {
		t.Fatal("callers mapping the error must also deny")
	}
}

// --- command classifier unit table ---

func TestClassifyCommandTable(t *testing.T) {
	cases := []struct {
		cmd  string
		want CommandClass
	}{
		{"ls -la", CmdSafe},
		{"go build ./cmd/qiyan", CmdSafe},
		{"go vet ./...", CmdSafe},
		{"go list ./...", CmdSafe},
		{"go version", CmdSafe},
		{"npm --version", CmdSafe},
		// R02-SEC-01: project-code execution and configuration persistence
		// are never auto-allowed.
		{"go test ./...", CmdSensitive},
		{"go test", CmdSensitive},
		{"go env", CmdSensitive},
		{"go env -w GOFLAGS=-mod=mod", CmdSensitive},
		{"go env -u GOPATH", CmdSensitive},
		{"go vet -vettool=/tmp/evil ./...", CmdSensitive},
		// System-state setters without a flag to screen (positional forms).
		{"date", CmdUnrecognized},
		{"date -s 12:00", CmdUnrecognized},
		{"hostname", CmdUnrecognized},
		{"hostname newname", CmdUnrecognized},
		{"uname -a", CmdSafe},
		// Output-redirecting flags demote even curated programs.
		{"go build -o bin/x ./cmd/x", CmdSensitive},
		{"go test -coverprofile=cover.out ./...", CmdSensitive},
		{"sort -o out.txt in.txt", CmdSensitive},
		{"uniq in.txt out.txt", CmdSensitive},
		// A value-taking flag must not be read as the subcommand:
		// `git -C log push` is `git push` from directory "log".
		{"git -C log push", CmdSensitive},
		{"git status", CmdSafe},
		{"git rev-parse HEAD", CmdSafe},
		{"git shortlog -sn", CmdSafe},
		// Git content transformers can execute configured helpers
		// (textconv, diff.external, GIT_EXTERNAL_DIFF): approval only.
		{"git log", CmdSensitive},
		{"git diff --stat", CmdSensitive},
		{"git show HEAD", CmdSensitive},
		{"git blame main.go", CmdSensitive},
		{"git push", CmdSensitive},
		{"go run main.go", CmdSensitive},
		{"rm -rf x", CmdSensitive},
		{"cat a | grep b", CmdUnrecognized},
		{"echo $(whoami)", CmdUnrecognized},
		{"myscript.sh", CmdUnrecognized},
		{"", CmdUnrecognized},
		{"ls *.go", CmdUnrecognized}, // glob = shell expansion
		// Path forms are never vouched for by base name: a planted
		// ./cat.bat must not inherit cat's "safe" classification.
		{"./cat.bat", CmdUnrecognized},
		{"./ls", CmdUnrecognized},
		{`C:\Windows\System32\cmd.exe /c dir`, CmdUnrecognized},
		{"/usr/bin/ls", CmdUnrecognized},
		{"../tools/go", CmdUnrecognized},
	}
	for _, c := range cases {
		got, prog := ClassifyCommand(c.cmd, "C:/nonexistent-cwd-for-tests")
		if got != c.want {
			t.Errorf("ClassifyCommand(%q) = %s (prog %q), want %s", c.cmd, got, prog, c.want)
		}
	}
}

// TestWindowsCwdShadowingRegression locks in the fix for the confirmed R02
// review finding: on Windows, `cat` resolves from the working directory first,
// so a planted ./cat.bat must demote the command from "safe" to unrecognized.
// On other platforms there is no cwd-first resolution and the check is not
// applied, so the test asserts the platform-correct behaviour.
func TestWindowsCwdShadowingRegression(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cat.bat"), []byte("@echo off\nrm -rf /\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _ := ClassifyCommand("cat notes.txt", dir)
	if runtime.GOOS == "windows" {
		if got != CmdUnrecognized {
			t.Fatalf("planted cat.bat must demote `cat` to unrecognized on Windows, got %s", got)
		}
	} else if got != CmdSafe {
		t.Fatalf("non-Windows has no cwd-first resolution; `cat` stays safe, got %s", got)
	}

	// An empty cwd on Windows can never be verified as safe.
	got, _ = ClassifyCommand("cat notes.txt", "")
	if runtime.GOOS == "windows" && got != CmdUnrecognized {
		t.Fatalf("bare command without a declared cwd must not classify as safe on Windows, got %s", got)
	}
}

// Regression: an absent roster must not silently remove the identity check.
func TestRosterRequiredAlways(t *testing.T) {
	f := newFixture(t, domain.ModeControlledAuto)
	req := f.req(read("C:/proj/a.txt"))
	req.Agents = nil
	if d, _ := f.engine.Evaluate(req); d.Outcome == domain.OutcomeAllow {
		t.Fatalf("missing roster must deny, got %s", d.Outcome)
	}
}

// Regression: kind=session.local is transport-declared; it must pass only
// for tools that are actually known session bookkeeping.
func TestSessionLocalRequiresBookkeepingTool(t *testing.T) {
	f := newFixture(t, domain.ModeControlledAuto)
	f.mission.Policy.Scope.AllowedTools = append(f.mission.Policy.Scope.AllowedTools, "TodoWrite", "mystery.tool")
	f.mission.Policy.Scope.AllowedKinds = append(f.mission.Policy.Scope.AllowedKinds, domain.ActionSessionLocal)

	real := domain.Action{Kind: domain.ActionSessionLocal, Tool: "TodoWrite"}
	if got := f.eval(t, real).Outcome; got != domain.OutcomeAllow {
		t.Fatalf("known bookkeeping tool must be allowed, got %s", got)
	}
	fake := domain.Action{Kind: domain.ActionSessionLocal, Tool: "mystery.tool"}
	if got := f.eval(t, fake).Outcome; got != domain.OutcomeRequireApproval {
		t.Fatalf("a declared session.local on an unknown tool must not skip the gate, got %s", got)
	}
}

// Regression: the engine must stamp the coverage the transport declared, and
// UNKNOWN when nothing was declared — never a hardcoded ENFORCED.
func TestCoverageStampedFromRequest(t *testing.T) {
	f := newFixture(t, domain.ModeControlledAuto)

	req := f.req(read("C:/proj/a.txt"))
	req.Coverage = domain.CoverageEnforced
	if d, _ := f.engine.Evaluate(req); d.Coverage != domain.CoverageEnforced {
		t.Fatalf("declared coverage must be stamped, got %s", d.Coverage)
	}
	req2 := f.req(read("C:/proj/a.txt"))
	if d, _ := f.engine.Evaluate(req2); d.Coverage != domain.CoverageUnknown {
		t.Fatalf("undeclared coverage must be UNKNOWN, got %s", d.Coverage)
	}
}

// Regression: swapping the content of an approved edit must not reuse the
// approval (the hook transport binds content; the API path binds it when the
// host supplies contentDigest).
func TestApprovalContentSwapDoesNotReuseGrant(t *testing.T) {
	f := newFixture(t, domain.ModeCompanion)
	a := write("C:/proj/src/a.go")
	a.ContentDigest = domain.ContentDigestOf("old", "approved change", "")
	d := f.eval(t, a)
	if d.Outcome != domain.OutcomeRequireApproval {
		t.Fatalf("companion write must ask, got %s", d.Outcome)
	}
	if _, err := f.engine.Decide(d.ApprovalID, true, domain.Actor{Source: domain.SourceLocalUser}, f.now); err != nil {
		t.Fatal(err)
	}
	swapped := write("C:/proj/src/a.go")
	swapped.ContentDigest = domain.ContentDigestOf("old", "DIFFERENT change", "")
	if got := f.eval(t, swapped).Outcome; got == domain.OutcomeAllow {
		t.Fatal("different content must not reuse the approval")
	}
	// The original content still claims its grant exactly once.
	if got := f.eval(t, a).Outcome; got != domain.OutcomeAllow {
		t.Fatalf("the exact approved content should run once, got %s", got)
	}
}

// Regression (adversarial review, confirmed): program-level safety must not
// let a curated read-only program reach outside the authorized roots through
// its arguments.
func TestSafeCommandArgumentsStayInScope(t *testing.T) {
	f := newFixture(t, domain.ModeControlledAuto)

	outside := f.eval(t, bash("cat C:/outside-note.txt"))
	if outside.Outcome != domain.OutcomeDeny {
		t.Fatalf("reading outside the roots via cat must deny, got %s", outside.Outcome)
	}
	if outside.Reasons[0].Code != domain.ReasonPathOutsideScope {
		t.Fatalf("reason should be path_outside_scope, got %v", outside.Reasons[0].Code)
	}
	if got := f.eval(t, bash("ls -la C:/outside-dir")).Outcome; got != domain.OutcomeDeny {
		t.Fatalf("listing outside must deny, got %s", got)
	}
	if got := f.eval(t, bash("go build ../outside")).Outcome; got != domain.OutcomeDeny {
		t.Fatalf("build target outside must deny, got %s", got)
	}

	// In-scope forms stay allowed: bare names resolve inside the cwd.
	for _, cmd := range []string{"cat note.txt", "cat sub/note.txt", "go build ./...", "git status", "grep TODO main.go"} {
		if got := f.eval(t, bash(cmd)).Outcome; got != domain.OutcomeAllow {
			t.Errorf("%q should stay allowed, got %s", cmd, got)
		}
	}

	// A flag carrying an unresolvable path value cannot be vouched for.
	if got := f.eval(t, bash("grep -f/outside/pats .")).Outcome; got != domain.OutcomeRequireApproval {
		t.Fatalf("unresolvable path flag must require approval, got %s", got)
	}
}

// Regression (R02-SEC-01): commands capable of executing project code or
// persisting configuration must reach the approval gate in the autonomous
// modes, while truly read-only operations keep flowing without friction.
func TestSideEffectCommandsRequireApprovalInControlledAuto(t *testing.T) {
	f := newFixture(t, domain.ModeControlledAuto)

	sideEffecting := []string{
		"go test ./...",
		"go test",
		"go env -w GOFLAGS=-mod=mod",
		"go env -u GOPATH",
		"go env",
		"go vet -vettool=/tmp/tool ./...",
		"date -s 12:00",
		"hostname newname",
		"git log",
		"git diff",
		"git show HEAD",
		"git blame main.go",
	}
	for _, cmd := range sideEffecting {
		d := f.eval(t, bash(cmd))
		if d.Outcome != domain.OutcomeRequireApproval {
			t.Errorf("%q must require explicit approval, got %s", cmd, d.Outcome)
			continue
		}
		if d.ApprovalID == "" {
			t.Errorf("%q asked for approval but recorded none", cmd)
		}
	}

	readOnly := []string{
		"ls -la",
		"cat notes.txt",
		"grep TODO main.go",
		"uname -a",
		"go build ./...",
		"go vet ./...",
		"go list ./...",
		"go version",
		"git status",
		"git rev-parse HEAD",
		"git shortlog -sn",
	}
	for _, cmd := range readOnly {
		if got := f.eval(t, bash(cmd)).Outcome; got != domain.OutcomeAllow {
			t.Errorf("%q is verified read-only and should keep flowing, got %s", cmd, got)
		}
	}
}
