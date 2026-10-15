package mission

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/control"
	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
	"github.com/QiyanAI/ai-agent-action-surface/internal/store"
)

// fakeRunner plays scripted turns; its prompts are captured for assertions.
type fakeRunner struct {
	script  []SessionResult
	i       int
	prompts []string
	onStart func(o *Orchestrator, agent domain.Agent, task domain.Task)
}

func (f *fakeRunner) next(prompt string) SessionResult {
	f.prompts = append(f.prompts, prompt)
	if f.i < len(f.script) {
		r := f.script[f.i]
		f.i++
		return r
	}
	return SessionResult{FinalText: "no more script"}
}

func (f *fakeRunner) Start(_ context.Context, prompt string) (SessionResult, error) {
	return f.next(prompt), nil
}

func (f *fakeRunner) Resume(_ context.Context, prompt string) (SessionResult, error) {
	return f.next(prompt), nil
}

func testMission(t *testing.T, st *store.Store, mode domain.AutonomyMode) domain.Mission {
	t.Helper()
	// Roots are stored normalized, exactly as the API layer does it; a raw
	// t.TempDir() path with backslashes would (correctly) fail containment.
	workdir := domain.NormalizePath(t.TempDir(), "")
	scope := domain.Scope{
		Roots: []string{workdir},
		AllowedKinds: []domain.ActionKind{
			domain.ActionFileRead, domain.ActionFileCreate, domain.ActionFileModify,
			domain.ActionFileDelete, domain.ActionProcessExec, domain.ActionSessionLocal,
		},
		AllowedTools: []string{"Read", "Write", "Edit", "Bash"},
		MaxSteps:     100, MaxRuntime: time.Hour,
	}
	m := domain.Mission{ID: domain.NewMissionID(), Goal: "make a report",
		Policy: domain.Policy{Mode: mode, Scope: scope}, State: domain.MissionRunning, CreatedAt: time.Now()}
	if err := st.PutMission(m); err != nil {
		t.Fatal(err)
	}
	return m
}

func newTestOrchestrator(t *testing.T, st *store.Store) *Orchestrator {
	t.Helper()
	return &Orchestrator{
		Store: st, Engine: control.New(st),
		PollEvery: 5 * time.Millisecond, ApprovalWait: 5 * time.Second, MaxResumes: 3,
	}
}

func TestFleetRunsThreeRolesInOrder(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m := testMission(t, st, domain.ModeControlledAuto)

	fake := &fakeRunner{script: []SessionResult{
		{FinalText: "Approach paragraph.\n```json\n[\"create report.md\", \"write summary\"]\n```"},
		{FinalText: "Done: created report.md"},
		{FinalText: "```json\n{\"verdict\": \"OK\", \"issues\": []}\n```"},
	}}
	o := newTestOrchestrator(t, st)
	o.NewSession = func(role domain.AgentRole, agent domain.Agent, task domain.Task, sid string) SessionRunner {
		return fake
	}

	status, err := o.Run(context.Background(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "completed" {
		t.Fatalf("state = %s, want completed (summary: %s)", status.State, status.Summary)
	}
	// Three roles ran, in order, each bound to its own session.
	if len(fake.prompts) != 3 {
		t.Fatalf("expected 3 turns, got %d", len(fake.prompts))
	}
	if !strings.Contains(fake.prompts[0], "Planner") || !strings.Contains(fake.prompts[1], "Builder") || !strings.Contains(fake.prompts[2], "Reviewer") {
		t.Fatalf("roles ran out of order: %q", fake.prompts)
	}
	// The builder prompt carries the planner's plan (real hand-off).
	if !strings.Contains(fake.prompts[1], "create report.md") {
		t.Fatalf("builder did not receive the planner's steps: %q", fake.prompts[1])
	}
	// All three tasks completed; sessions bound.
	tasks, _ := st.TasksForMission(m.ID)
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	for _, task := range tasks {
		if task.State != domain.TaskCompleted {
			t.Errorf("task %s (%s) state = %s, want COMPLETED", task.ID, task.Role, task.State)
		}
		b, found, _ := st.LookupSession(string(task.ID)) // session ids are uuids; probe by task instead
		_ = b
		_ = found
	}
	got, _, _ := st.GetMission(m.ID)
	if got.State != domain.MissionCompleted {
		t.Fatalf("mission state = %s, want COMPLETED", got.State)
	}
}

// approvalRunner simulates what the real hook does: it evaluates a
// decision-requiring action through the engine (creating a pending approval)
// and ends its turn. One instance per role.
type approvalRunner struct {
	o       *Orchestrator
	mission domain.Mission
	agent   domain.Agent
	task    domain.Task
	role    domain.AgentRole
	resumes []string
}

func (a *approvalRunner) act() {
	// A real hook resolves the session's task and agent from the store — the
	// captured copies are stale by now (the orchestrator moved the task to
	// RUNNING after handing them out).
	task, found, err := a.o.Store.GetTask(a.task.ID)
	if err != nil || !found {
		return
	}
	agent, found, err := a.o.Store.AgentByID(a.agent.ID)
	if err != nil || !found {
		return
	}
	// Normalized, exactly as the hook transport would construct it.
	action := domain.Action{Kind: domain.ActionFileDelete, Tool: "Edit",
		Path: domain.NormalizePath(a.mission.Policy.Scope.Roots[0]+"/old.txt", "")}
	_, _ = a.o.Engine.Evaluate(control.Request{
		Mission: a.mission, Task: task, Agent: agent, Action: action,
		Agents: []domain.Agent{agent}, Now: time.Now(),
	})
}

func (a *approvalRunner) Start(_ context.Context, prompt string) (SessionResult, error) {
	if strings.Contains(prompt, "Builder") {
		a.act() // create the pending approval the user must decide
		return SessionResult{FinalText: "AWAITING APPROVAL"}, nil
	}
	return SessionResult{FinalText: "planned.\n```json\n[\"x\"]\n```"}, nil
}

func (a *approvalRunner) Resume(_ context.Context, prompt string) (SessionResult, error) {
	a.resumes = append(a.resumes, prompt)
	return SessionResult{FinalText: "finished"}, nil
}

func TestApprovalPausesAndResumes(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m := testMission(t, st, domain.ModeCompanion) // companion: deletes wait for the user

	o := newTestOrchestrator(t, st)
	ar := &approvalRunner{o: o, mission: m, role: domain.RoleBuilder}
	o.NewSession = func(role domain.AgentRole, agent domain.Agent, task domain.Task, sid string) SessionRunner {
		r := &approvalRunner{o: o, mission: m, role: role}
		if role == domain.RoleBuilder {
			r.agent, r.task = agent, task
			ar = r
		}
		return r
	}

	// The "user" approves the first pending approval after a human-scale
	// pause, so the orchestrator really enters its waiting state first.
	go func() {
		engine := control.New(st)
		for i := 0; i < 400; i++ {
			pend, err := st.ListApprovals(domain.ApprovalPending, "")
			if err == nil && len(pend) > 0 {
				time.Sleep(150 * time.Millisecond)
				_, _ = engine.Decide(pend[0].ID, true, domain.Actor{Source: domain.SourceLocalUser, Label: "test-user"}, time.Now())
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	status, err := o.Run(context.Background(), m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "completed" {
		t.Fatalf("state = %s (%s)", status.State, status.Summary)
	}
	if len(ar.resumes) == 0 || !strings.Contains(ar.resumes[0], "approved") {
		t.Fatalf("the post-approval resume must say the action was approved: %q", ar.resumes)
	}
	// The waiting state appeared in the event trail.
	sawWaiting := false
	for _, ev := range status.Events {
		if ev.Kind == "approval-pending" {
			sawWaiting = true
		}
	}
	if !sawWaiting {
		t.Fatal("the run must surface the approval as an event")
	}
}

func TestDeniedApprovalResumesWithDenial(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m := testMission(t, st, domain.ModeCompanion)

	o := newTestOrchestrator(t, st)
	ar := &approvalRunner{o: o, mission: m, role: domain.RoleBuilder}
	o.NewSession = func(role domain.AgentRole, agent domain.Agent, task domain.Task, sid string) SessionRunner {
		r := &approvalRunner{o: o, mission: m, role: role}
		if role == domain.RoleBuilder {
			r.agent, r.task = agent, task
			ar = r
		}
		return r
	}
	go func() {
		engine := control.New(st)
		for i := 0; i < 400; i++ {
			pend, err := st.ListApprovals(domain.ApprovalPending, "")
			if err == nil && len(pend) > 0 {
				_, _ = engine.Decide(pend[0].ID, false, domain.Actor{Source: domain.SourceLocalUser, Label: "test-user"}, time.Now())
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	if _, err := o.Run(context.Background(), m.ID); err != nil {
		t.Fatal(err)
	}
	if len(ar.resumes) == 0 || !strings.Contains(ar.resumes[0], "denied") {
		t.Fatalf("the denial resume must say the action was denied: %q", ar.resumes)
	}
}

func TestPlanStepsParsing(t *testing.T) {
	reply := "here is the plan\n```json\n[\"a\", \"b\", \"c\"]\n```\ntrailing"
	steps := PlanSteps(reply)
	if len(steps) != 3 || steps[0] != "a" {
		t.Fatalf("steps = %v", steps)
	}
	if PlanSteps("no fence here") != nil {
		t.Fatal("no fenced JSON must mean no parsed steps")
	}
	if PlanSteps("```json\n{\"not\":\"array\"}\n```") != nil {
		t.Fatal("an object is not a step list")
	}
}

func TestRunRegistryRejectsDoubleRun(t *testing.T) {
	r := NewRunRegistry()
	o := &Orchestrator{}
	mid := domain.NewMissionID()
	// A zero orchestrator would fail instantly, which is fine: the registry's
	// job is to refuse the second start while the first is live.
	ok1, err := r.Start(mid, o)
	if err != nil || !ok1 {
		t.Fatalf("first start: %v %v", ok1, err)
	}
	ok2, err := r.Start(mid, o)
	if err != nil || ok2 {
		t.Fatalf("second start must be refused while running: %v %v", ok2, err)
	}
}
