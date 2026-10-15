package domain

import "time"

// AgentRole names the part an agent plays in a mission. The default fleet is
// Planner → Builder → Reviewer (book §11); roles carry default permission
// profiles, and a mission may narrow them but never widen a read-only role's
// defaults.
type AgentRole string

const (
	RolePlanner   AgentRole = "planner"
	RoleBuilder   AgentRole = "builder"
	RoleReviewer  AgentRole = "reviewer"
	RoleExternal  AgentRole = "external"  // third-party agents that arrive via the Control API
	RoleReference AgentRole = "reference" // the bundled reference host's agent
)

// KnownRole reports whether r is defined.
func KnownRole(r AgentRole) bool {
	switch r {
	case RolePlanner, RoleBuilder, RoleReviewer, RoleExternal, RoleReference:
		return true
	}
	return false
}

// DefaultReadOnly reports whether the role starts read-only. Planner and
// Reviewer do; Builder does not. Widening a role is a per-mission decision that
// must be visible in the mission's scope, never a silent default.
func (r AgentRole) DefaultReadOnly() bool {
	switch r {
	case RolePlanner, RoleReviewer:
		return true
	default:
		return false
	}
}

// ActorSource distinguishes who produced an event or decision: the local user,
// a monitored agent, or an external client of the Control API. The distinction
// is load-bearing — only the local user may approve sensitive actions (book
// §12.3), and audit records must never blur the three (book §18).
type ActorSource string

const (
	SourceLocalUser      ActorSource = "local_user"
	SourceAgent          ActorSource = "agent"
	SourceExternalClient ActorSource = "external_client"
)

// Actor identifies the originator of an event or a decision.
type Actor struct {
	Source  ActorSource
	AgentID AgentID // set when Source is SourceAgent
	Client  string  // free-form client label for external clients ("qiyanhost/0.1")
	Label   string  // local user display label, e.g. the OS user name; never a credential
}

// Agent is a registered actor that can request authorization: one of the
// built-in fleet roles, or an external agent registered through the Control
// API. Runtime records where it actually runs ("claude-code@2.1.300",
// "qiyanhost/0.1") so evidence and coverage can name the exact environment a
// claim was verified in.
type Agent struct {
	ID           AgentID
	Name         string
	Role         AgentRole
	Runtime      string
	ReadOnly     bool
	RegisteredAt time.Time
}

// Mission is one goal the user gave the fleet, with the policy that governs it.
// Tasks belong to a mission; the mission's scope is the outermost boundary for
// every task in it (per-task scopes may only narrow it).
type Mission struct {
	ID        MissionID
	Goal      string // the user's natural-language objective, stored verbatim
	Policy    Policy
	State     MissionState
	CreatedAt time.Time
	UpdatedAt time.Time
	Tasks     []TaskID
}

// MissionState is the mission-level lifecycle. It mirrors the task states but
// carries fewer of them: a mission is done when its tasks are.
type MissionState string

const (
	MissionCreated   MissionState = "CREATED"
	MissionRunning   MissionState = "RUNNING"
	MissionCompleted MissionState = "COMPLETED"
	MissionFailed    MissionState = "FAILED"
	MissionCancelled MissionState = "CANCELLED"
	MissionUnknown   MissionState = "UNKNOWN"
)

// TaskState is the task lifecycle from the book §11.4, verbatim.
type TaskState string

const (
	TaskCreated         TaskState = "CREATED"
	TaskReady           TaskState = "READY"
	TaskRunning         TaskState = "RUNNING"
	TaskWaitingApproval TaskState = "WAITING_APPROVAL"
	TaskCompleted       TaskState = "COMPLETED"
	TaskFailed          TaskState = "FAILED"
	TaskCancelled       TaskState = "CANCELLED"
	TaskUnknown         TaskState = "UNKNOWN"
)

// Task is one unit of work assigned to one role.
type Task struct {
	ID        TaskID
	MissionID MissionID
	Role      AgentRole
	Title     string
	State     TaskState
	AgentID   AgentID // the concrete agent instance executing it, when assigned
	DependsOn []TaskID

	Artifacts []Artifact
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Artifact is a produced result, referenced by path (kept local; nothing is
// uploaded — book §18).
type Artifact struct {
	Path        string
	Description string
}

// taskTransitions is the task state machine. Transitions exist only between
// states that real lifecycle events can produce; anything else is a bug or a
// spoof and is rejected.
//
// UNKNOWN is reachable from any running state (its source of truth was lost),
// and may be *reconciled* to a terminal state when verified evidence arrives —
// never directly to RUNNING, because a task whose events were lost must not
// silently resume as if nothing happened (S07). Terminal states never change.
var taskTransitions = map[TaskState][]TaskState{
	TaskCreated:         {TaskReady, TaskCancelled, TaskUnknown},
	TaskReady:           {TaskRunning, TaskCancelled, TaskUnknown},
	TaskRunning:         {TaskWaitingApproval, TaskCompleted, TaskFailed, TaskCancelled, TaskUnknown},
	TaskWaitingApproval: {TaskRunning, TaskFailed, TaskCancelled, TaskUnknown},
	TaskCompleted:       {},
	TaskFailed:          {},
	TaskCancelled:       {},
	TaskUnknown:         {TaskCompleted, TaskFailed, TaskCancelled},
}

// CanTransition reports whether from → to is a legal task transition.
func CanTransition(from, to TaskState) bool {
	for _, t := range taskTransitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

// TaskTerminal reports whether s admits no further transitions.
func TaskTerminal(s TaskState) bool {
	return len(taskTransitions[s]) == 0
}
