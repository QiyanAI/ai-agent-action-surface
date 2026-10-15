package mission

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/control"
	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
	"github.com/QiyanAI/ai-agent-action-surface/internal/store"
)

// Event is one step of the run's visible story, surfaced to the CLI and the
// console as it happens. Events are for humans; the machine record lives in
// the decision log, and every event that matters carries the IDs to reach it.
type Event struct {
	At         time.Time         `json:"at"`
	Kind       string            `json:"kind"` // role-start | role-end | approval-pending | approval-decided | task-state | error | mission-end
	Role       string            `json:"role,omitempty"`
	TaskID     domain.TaskID     `json:"taskId,omitempty"`
	Text       string            `json:"text,omitempty"`
	ApprovalID domain.ApprovalID `json:"approvalId,omitempty"`
	Outcome    string            `json:"outcome,omitempty"`
}

// RunStatus is the observable state of a mission run.
type RunStatus struct {
	MissionID domain.MissionID `json:"missionId"`
	State     string           `json:"state"` // running | completed | failed | cancelled
	StartedAt time.Time        `json:"startedAt"`
	EndedAt   *time.Time       `json:"endedAt,omitempty"`
	Summary   string           `json:"summary,omitempty"`
	Events    []Event          `json:"events"`
	Artifacts []string         `json:"artifacts,omitempty"`
}

// Orchestrator drives one mission: three roles, three sessions, real
// hand-offs. It decides nothing about safety — every action still passes the
// engine — it only schedules and reacts.
type Orchestrator struct {
	Store  *store.Store
	Engine *control.Engine

	// NewSession builds the runner for one role. Injected so tests use a
	// scripted fake; the server wires the real Claude Code runner.
	NewSession func(role domain.AgentRole, agent domain.Agent, task domain.Task, sessionID string) SessionRunner

	// ApprovalWait bounds how long a run pauses at a pending approval before
	// failing the task. PollEvery is the store poll interval.
	ApprovalWait time.Duration
	PollEvery    time.Duration
	// MaxResumes bounds resume turns per task.
	MaxResumes int
	// OnEvent receives every event as it happens (must not block). The run
	// also records its own trail, returned in the final status, so callers
	// that never attached a listener still get the full story.
	OnEvent func(Event)
	Now     func() time.Time

	evMu  sync.Mutex
	evLog []Event
}

func (o *Orchestrator) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

func (o *Orchestrator) emit(ev Event) {
	ev.At = o.now()
	o.evMu.Lock()
	o.evLog = append(o.evLog, ev)
	if len(o.evLog) > 1000 {
		o.evLog = o.evLog[len(o.evLog)-1000:]
	}
	o.evMu.Unlock()
	if o.OnEvent != nil {
		o.OnEvent(ev)
	}
}

func (o *Orchestrator) eventSnapshot() []Event {
	o.evMu.Lock()
	defer o.evMu.Unlock()
	return append([]Event(nil), o.evLog...)
}

// Run executes the mission to a terminal state. It returns the final status;
// a non-nil error means the run itself could not proceed (the status explains
// what happened to the mission).
func (o *Orchestrator) Run(ctx context.Context, missionID domain.MissionID) (RunStatus, error) {
	if o.PollEvery <= 0 {
		o.PollEvery = time.Second
	}
	if o.ApprovalWait <= 0 {
		o.ApprovalWait = 10 * time.Minute
	}
	if o.MaxResumes <= 0 {
		o.MaxResumes = 5
	}
	status := RunStatus{MissionID: missionID, State: "running", StartedAt: o.now()}
	end := func(state, summary string) (RunStatus, error) {
		status.State = state
		status.Summary = summary
		t := o.now()
		status.EndedAt = &t
		o.emit(Event{Kind: "mission-end", Text: summary, Outcome: state})
		status.Events = o.eventSnapshot()
		return status, nil
	}

	if o.Store == nil {
		return status, fmt.Errorf("mission: orchestrator has no store")
	}

	m, found, err := o.Store.GetMission(missionID)
	if err != nil {
		return status, err
	}
	if !found {
		return status, fmt.Errorf("mission %s not found", missionID)
	}
	if len(m.Policy.Scope.Roots) == 0 {
		return status, fmt.Errorf("mission has no authorized working directory")
	}
	workdir := m.Policy.Scope.Roots[0]
	if fi, err := os.Stat(workdir); err != nil || !fi.IsDir() {
		return status, fmt.Errorf("mission working directory %s is not a directory", workdir)
	}

	roster, tasks, err := o.ensureFleet(m)
	if err != nil {
		return status, err
	}

	// The three roles run in order; each task is bound to one session and
	// one agent, and the hook resolves identity from that binding alone.
	planner := roster[domain.RolePlanner]
	builder := roster[domain.RoleBuilder]
	reviewer := roster[domain.RoleReviewer]

	// Planner.
	plannerTask := tasks[domain.RolePlanner]
	o.emit(Event{Kind: "role-start", Role: string(domain.RolePlanner), TaskID: plannerTask.ID,
		Text: "planning the mission"})
	planText, err := o.runRole(ctx, m, planner, plannerTask, plannerPrompt(m, workdir), &status)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			o.setStateTask(plannerTask, domain.TaskCancelled)
			o.setMissionState(m, domain.MissionCancelled)
			return end("cancelled", "run cancelled by the user during planning")
		}
		o.setStateTask(plannerTask, domain.TaskFailed)
		o.setMissionState(m, domain.MissionFailed)
		return end("failed", "planner session failed: "+err.Error())
	}
	o.setStateTask(plannerTask, domain.TaskCompleted)
	o.emit(Event{Kind: "role-end", Role: string(domain.RolePlanner), TaskID: plannerTask.ID,
		Text: "plan ready"})

	// Builder.
	builderTask := tasks[domain.RoleBuilder]
	o.emit(Event{Kind: "role-start", Role: string(domain.RoleBuilder), TaskID: builderTask.ID,
		Text: "building per the plan"})
	builderText, err := o.runRole(ctx, m, builder, builderTask, builderPrompt(m, workdir, planText), &status)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			o.setStateTask(builderTask, domain.TaskCancelled)
			o.setMissionState(m, domain.MissionCancelled)
			return end("cancelled", "run cancelled by the user while building; actions already allowed remain in the audit trail")
		}
		o.setStateTask(builderTask, domain.TaskFailed)
		o.setMissionState(m, domain.MissionFailed)
		return end("failed", "builder session failed: "+err.Error())
	}
	o.recordBuilderArtifacts(builderTask)
	status.Artifacts = o.artifactPaths(builderTask)
	o.setStateTask(builderTask, domain.TaskCompleted)
	o.emit(Event{Kind: "role-end", Role: string(domain.RoleBuilder), TaskID: builderTask.ID,
		Text: "build complete"})

	// Reviewer.
	reviewerTask := tasks[domain.RoleReviewer]
	o.emit(Event{Kind: "role-start", Role: string(domain.RoleReviewer), TaskID: reviewerTask.ID,
		Text: "reviewing the result"})
	reviewText, err := o.runRole(ctx, m, reviewer, reviewerTask, reviewerPrompt(m, workdir, builderText), &status)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			o.setStateTask(reviewerTask, domain.TaskCancelled)
			o.setMissionState(m, domain.MissionCancelled)
			return end("cancelled", "run cancelled by the user during review")
		}
		o.setStateTask(reviewerTask, domain.TaskFailed)
		o.setMissionState(m, domain.MissionFailed)
		return end("failed", "reviewer session failed: "+err.Error())
	}
	_ = reviewText
	o.setStateTask(reviewerTask, domain.TaskCompleted)
	o.emit(Event{Kind: "role-end", Role: string(domain.RoleReviewer), TaskID: reviewerTask.ID,
		Text: "review complete"})

	o.setMissionState(m, domain.MissionCompleted)
	return end("completed", "mission completed: planner → builder → reviewer")
}

// runRole runs one role to the end of its turn(s), handling approval pauses.
// It returns the role's final text.
func (o *Orchestrator) runRole(ctx context.Context, m domain.Mission, agent domain.Agent, task domain.Task, firstPrompt string, status *RunStatus) (string, error) {
	sid := newSessionUUID()
	runner := o.NewSession(agent.Role, agent, task, sid)

	if err := o.Store.BindSession(store.SessionBinding{
		SessionID: sid, Runtime: "claude-code", MissionID: m.ID, TaskID: task.ID,
		AgentID: agent.ID, BoundAt: o.now(),
	}); err != nil {
		return "", err
	}
	task.State = domain.TaskRunning
	if task.StartedAt.IsZero() {
		task.StartedAt = o.now()
	}
	task.AgentID = agent.ID
	if err := o.Store.PutTask(task); err != nil {
		return "", err
	}

	turnStart := o.now()
	res, err := runner.Start(ctx, firstPrompt)
	if err != nil {
		return "", err
	}
	final := res.FinalText

	for resumes := 0; resumes < o.MaxResumes; resumes++ {
		if ctx.Err() != nil {
			o.cancelPending(task.ID)
			o.setStateTask(task, domain.TaskCancelled)
			return final, ctx.Err()
		}
		// Did this turn raise an approval? It may still be pending, or the
		// user may already have decided — a decision that lands before the
		// orchestrator looks must still lead to the resume, or an approved
		// action would silently never run.
		raised, err := o.latestApprovalForTurn(task.ID, agent.ID, turnStart)
		if err != nil {
			return final, err
		}
		if raised == nil {
			return final, nil
		}

		decision := string(raised.State)
		if raised.State == domain.ApprovalPending {
			o.setStateTask(task, domain.TaskWaitingApproval)
			o.emit(Event{Kind: "approval-pending", Role: string(agent.Role), TaskID: task.ID,
				ApprovalID: raised.ID, Text: fmt.Sprintf("%s is waiting for your decision on %s %s",
					agent.Name, raised.Action.Kind, raised.Action.DisplayTarget())})
			decision, err = o.waitForDecision(ctx, raised.ID)
			if err != nil {
				return final, err
			}
		}
		o.emit(Event{Kind: "approval-decided", TaskID: task.ID, ApprovalID: raised.ID, Outcome: decision})
		o.setStateTask(task, domain.TaskRunning)

		var resumePrompt string
		if decision == string(domain.ApprovalApproved) {
			resumePrompt = "Your previous tool call was approved by the user. Perform that exact action now, then continue the task to completion."
		} else {
			resumePrompt = "Your previous tool call was denied (" + decision + "). Continue the task without that action and explain the limitation in your final summary."
		}
		turnStart = o.now()
		res, err = runner.Resume(ctx, resumePrompt)
		if err != nil {
			return final, err
		}
		if res.FinalText != "" {
			final = res.FinalText
		}
	}
	return final, nil
}

// latestApprovalForTurn returns the newest approval raised for this task and
// agent at or after turnStart, in whatever state it is in — including one the
// user already decided.
func (o *Orchestrator) latestApprovalForTurn(task domain.TaskID, agent domain.AgentID, turnStart time.Time) (*domain.ApprovalRequest, error) {
	all, err := o.Store.ListApprovals("", agent)
	if err != nil {
		return nil, err
	}
	var newest *domain.ApprovalRequest
	for i := range all {
		ap := &all[i]
		if ap.TaskID != task {
			continue
		}
		if ap.RequestedAt.Before(turnStart.Add(-time.Second)) {
			continue // an earlier turn's approval, already handled
		}
		if newest == nil || ap.RequestedAt.After(newest.RequestedAt) {
			newest = ap
		}
	}
	return newest, nil
}

// waitForDecision polls until the approval leaves pending, or the wait bound
// elapses (which cancels it so the run does not hang on a forgotten prompt).
func (o *Orchestrator) waitForDecision(ctx context.Context, id domain.ApprovalID) (string, error) {
	deadline := o.now().Add(o.ApprovalWait)
	for {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		ap, found, err := o.Store.GetApproval(id)
		if err != nil {
			return "", err
		}
		if !found {
			return "", fmt.Errorf("approval %s disappeared", id)
		}
		if ap.State != domain.ApprovalPending {
			return string(ap.State), nil
		}
		if o.now().After(deadline) {
			_ = o.Engine.CancelApproval(id, domain.Actor{Source: domain.SourceLocalUser, Label: "qiyan-run timeout"}, o.now())
			return string(domain.ApprovalCancelled), nil
		}
		time.Sleep(o.PollEvery)
	}
}

// ensureFleet registers (or reuses) the three role agents and tasks.
func (o *Orchestrator) ensureFleet(m domain.Mission) (map[domain.AgentRole]domain.Agent, map[domain.AgentRole]domain.Task, error) {
	agents := map[domain.AgentRole]domain.Agent{}
	existing, err := o.Store.ListAgents()
	if err != nil {
		return nil, nil, err
	}
	byName := map[string]domain.Agent{}
	for _, a := range existing {
		byName[a.Name] = a
	}
	roles := []struct {
		role domain.AgentRole
		name string
	}{
		{domain.RolePlanner, "Planner"},
		{domain.RoleBuilder, "Builder"},
		{domain.RoleReviewer, "Reviewer"},
	}
	for _, r := range roles {
		if a, ok := byName[r.name]; ok {
			agents[r.role] = a
			continue
		}
		a := domain.Agent{ID: domain.NewAgentID(), Name: r.name, Role: r.role,
			Runtime: "claude-code", ReadOnly: r.role.DefaultReadOnly(), RegisteredAt: o.now()}
		if err := o.Store.RegisterAgent(a, ""); err != nil {
			return nil, nil, err
		}
		agents[r.role] = a
	}

	tasks := map[domain.AgentRole]domain.Task{}
	existingTasks, err := o.Store.TasksForMission(m.ID)
	if err != nil {
		return nil, nil, err
	}
	byRole := map[domain.AgentRole]domain.Task{}
	for _, t := range existingTasks {
		byRole[t.Role] = t
	}
	titles := map[domain.AgentRole]string{
		domain.RolePlanner:  "Plan the mission",
		domain.RoleBuilder:  "Execute the plan",
		domain.RoleReviewer: "Review the result",
	}
	for _, r := range roles {
		if t, ok := byRole[r.role]; ok {
			// Tasks waiting from a previous interrupted run resume life.
			if t.State == domain.TaskCompleted || t.State == domain.TaskFailed || t.State == domain.TaskCancelled {
				return nil, nil, fmt.Errorf("task for role %s is already %s; re-run needs a fresh mission", r.role, t.State)
			}
			tasks[r.role] = t
			continue
		}
		t := domain.Task{
			ID: domain.NewTaskID(), MissionID: m.ID, Role: r.role, Title: titles[r.role],
			State: domain.TaskReady, AgentID: agents[r.role].ID, CreatedAt: o.now(),
		}
		if err := o.Store.PutTask(t); err != nil {
			return nil, nil, err
		}
		tasks[r.role] = t
	}
	return agents, tasks, nil
}

func (o *Orchestrator) setMissionState(m domain.Mission, s domain.MissionState) {
	cur, found, err := o.Store.GetMission(m.ID)
	if err != nil || !found {
		o.emit(Event{Kind: "error", Text: "mission state update failed"})
		return
	}
	cur.State = s
	cur.UpdatedAt = o.now()
	if err := o.Store.PutMission(cur); err != nil {
		o.emit(Event{Kind: "error", Text: err.Error()})
	}
	o.emit(Event{Kind: "task-state", Text: "mission " + string(s), Outcome: string(s)})
}

// setStateTask applies a lifecycle transition to the task's *current* stored
// state — the caller's copy may be several writes stale.
func (o *Orchestrator) setStateTask(t domain.Task, s domain.TaskState) {
	cur, found, err := o.Store.GetTask(t.ID)
	if err != nil || !found {
		o.emit(Event{Kind: "error", TaskID: t.ID, Text: "task state update failed"})
		return
	}
	if cur.State == s {
		return
	}
	if !domain.CanTransition(cur.State, s) {
		// A state that the lifecycle forbids is a bug, not a message.
		o.emit(Event{Kind: "error", TaskID: t.ID, Text: fmt.Sprintf("illegal task transition %s → %s", cur.State, s)})
		return
	}
	cur.State = s
	cur.UpdatedAt = o.now()
	if err := o.Store.PutTask(cur); err != nil {
		o.emit(Event{Kind: "error", TaskID: t.ID, Text: err.Error()})
	}
	o.emit(Event{Kind: "task-state", TaskID: t.ID, Outcome: string(s)})
}

func (o *Orchestrator) cancelPending(task domain.TaskID) {
	pendings, err := o.Store.ListApprovals(domain.ApprovalPending, "")
	if err != nil {
		return
	}
	for _, ap := range pendings {
		if ap.TaskID == task {
			_ = o.Engine.CancelApproval(ap.ID, domain.Actor{Source: domain.SourceLocalUser, Label: "run cancelled"}, o.now())
		}
	}
}

// recordBuilderArtifacts derives the artifacts from the decisions that were
// actually ALLOWED for the builder task — evidence, not prose.
func (o *Orchestrator) recordBuilderArtifacts(task domain.Task) {
	recs, err := o.Store.DecisionsForTask(task.ID)
	if err != nil {
		return
	}
	seen := map[string]bool{}
	var arts []domain.Artifact
	for _, r := range recs {
		if r.Decision.Outcome != domain.OutcomeAllow {
			continue
		}
		switch r.Action.Kind {
		case domain.ActionFileCreate, domain.ActionFileModify:
			if r.Action.Path != "" && !seen[r.Action.Path] {
				seen[r.Action.Path] = true
				arts = append(arts, domain.Artifact{Path: r.Action.Path, Description: string(r.Action.Kind)})
			}
		}
	}
	t, found, err := o.Store.GetTask(task.ID)
	if err != nil || !found {
		return
	}
	t.Artifacts = arts
	t.UpdatedAt = o.now()
	_ = o.Store.PutTask(t)
}

// artifactPaths re-reads the task's recorded artifacts for the run status.
func (o *Orchestrator) artifactPaths(task domain.Task) []string {
	t, found, err := o.Store.GetTask(task.ID)
	if err != nil || !found {
		return nil
	}
	var paths []string
	for _, a := range t.Artifacts {
		paths = append(paths, a.Path)
	}
	return paths
}

// --- prompts ---

func plannerPrompt(m domain.Mission, workdir string) string {
	return fmt.Sprintf(`You are the Planner in a Qiyan mission. You are read-only: you cannot modify anything.
Goal: %s
Working directory: %s
Write a concise plan: first one short paragraph of approach, then a JSON array of 3 to 7 concrete steps for the Builder. Each step must name the files it will create or change. End your reply with the JSON array inside a fenced %s block.`,
		m.Goal, workdir, "```json")
}

func builderPrompt(m domain.Mission, workdir, plan string) string {
	return fmt.Sprintf(`You are the Builder in a Qiyan mission. Execute the plan below by actually creating and modifying the files it names, using your tools. Stay inside the working directory. Sensitive actions may pause for the user's approval; if a call is blocked pending approval, finish your reply with the single line: AWAITING APPROVAL.
Goal: %s
Working directory: %s
Planner's plan:
%s
When the plan is done, reply with a short completion summary listing each file you created or changed.`,
		m.Goal, workdir, plan)
}

func reviewerPrompt(m domain.Mission, workdir, builderSummary string) string {
	return fmt.Sprintf(`You are the Reviewer in a Qiyan mission. You are read-only. Verify the Builder's claimed result by reading the files it names in the working directory.
Goal: %s
Working directory: %s
Builder's summary:
%s
Reply with a short review, then end with a JSON object {"verdict": "OK" or "ISSUES", "issues": [...]} inside a fenced %s block.`,
		m.Goal, workdir, builderSummary, "```json")
}

var fenceRE = regexp.MustCompile("(?s)```(?:json)?\\s*(\\[.*?\\]|\\{.*?\\})\\s*```")

// extractFencedJSON returns the last fenced JSON block's content, or "".
func extractFencedJSON(text string) string {
	matches := fenceRE.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return ""
	}
	return strings.TrimSpace(matches[len(matches)-1][1])
}

// PlanSteps parses the planner's steps from its reply; nil when absent.
func PlanSteps(reply string) []string {
	block := extractFencedJSON(reply)
	if block == "" || !strings.HasPrefix(block, "[") {
		return nil
	}
	var steps []string
	if err := json.Unmarshal([]byte(block), &steps); err != nil {
		return nil
	}
	return steps
}

// newSessionUUID returns an RFC 4122 version-4 ID for the runtime's
// --session-id flag.
func newSessionUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("mission: crypto/rand unavailable: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// RunRegistry tracks in-flight and finished runs per mission (server-side).
type RunRegistry struct {
	mu   sync.Mutex
	runs map[domain.MissionID]*runEntry
}

type runEntry struct {
	cancel context.CancelFunc
	status RunStatus
	done   bool
}

// NewRunRegistry returns an empty registry.
func NewRunRegistry() *RunRegistry {
	return &RunRegistry{runs: map[domain.MissionID]*runEntry{}}
}

// Start begins a run unless one is already live for the mission.
func (r *RunRegistry) Start(missionID domain.MissionID, o *Orchestrator) (bool, error) {
	r.mu.Lock()
	if e, ok := r.runs[missionID]; ok && !e.done {
		r.mu.Unlock()
		return false, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	entry := &runEntry{cancel: cancel, status: RunStatus{MissionID: missionID, State: "running", StartedAt: time.Now()}}
	r.runs[missionID] = entry
	r.mu.Unlock()

	o.OnEvent = func(ev Event) {
		r.mu.Lock()
		defer r.mu.Unlock()
		entry.status.Events = append(entry.status.Events, ev)
		if len(entry.status.Events) > 1000 {
			entry.status.Events = entry.status.Events[len(entry.status.Events)-1000:]
		}
	}
	go func() {
		status, err := o.Run(ctx, missionID)
		r.mu.Lock()
		defer r.mu.Unlock()
		entry.done = true
		entry.status = status
		entry.status.Events = append(entry.status.Events, Event{At: time.Now(), Kind: "run-end", Text: status.Summary})
		if err != nil {
			entry.status.State = "failed"
			entry.status.Summary = err.Error()
		}
	}()
	return true, nil
}

// Status returns the latest run snapshot.
func (r *RunRegistry) Status(missionID domain.MissionID) (RunStatus, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.runs[missionID]
	if !ok {
		return RunStatus{}, false
	}
	cp := e.status
	cp.Events = append([]Event(nil), e.status.Events...)
	return cp, true
}

// Cancel stops a live run.
func (r *RunRegistry) Cancel(missionID domain.MissionID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.runs[missionID]
	if !ok || e.done {
		return false
	}
	e.cancel()
	return true
}
