package api

import (
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
	"github.com/QiyanAI/ai-agent-action-surface/internal/store"
)

// Wire payloads. The API never marshals internal domain structs directly:
// response shapes are a versioned contract (book §14), so they get their own
// types with stable, explicitly-tagged fields. When the domain model evolves,
// these decide what actually changes for clients.

// AgentPayload is one registered agent. Tokens never appear here.
type AgentPayload struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Role         string    `json:"role"`
	Runtime      string    `json:"runtime,omitempty"`
	ReadOnly     bool      `json:"readOnly"`
	RegisteredAt time.Time `json:"registeredAt"`
}

func agentPayload(a domain.Agent) AgentPayload {
	return AgentPayload{
		ID: string(a.ID), Name: a.Name, Role: string(a.Role),
		Runtime: a.Runtime, ReadOnly: a.ReadOnly, RegisteredAt: a.RegisteredAt,
	}
}

// ScopePayload / PolicyPayload describe a mission's envelope.
type ScopePayload struct {
	Roots          []string `json:"roots"`
	AllowedKinds   []string `json:"allowedKinds"`
	AllowedTools   []string `json:"allowedTools"`
	MaxSteps       int      `json:"maxSteps"`
	MaxRuntimeSecs int      `json:"maxRuntimeSeconds"`
}

type PolicyPayload struct {
	Mode  string       `json:"mode"`
	Scope ScopePayload `json:"scope"`
}

func policyPayload(p domain.Policy) PolicyPayload {
	sp := ScopePayload{
		Roots:    append([]string(nil), p.Scope.Roots...),
		MaxSteps: p.Scope.MaxSteps, MaxRuntimeSecs: int(p.Scope.MaxRuntime.Seconds()),
	}
	for _, k := range p.Scope.AllowedKinds {
		sp.AllowedKinds = append(sp.AllowedKinds, string(k))
	}
	sp.AllowedTools = append([]string(nil), p.Scope.AllowedTools...)
	return PolicyPayload{Mode: string(p.Mode), Scope: sp}
}

// MissionPayload is one mission.
type MissionPayload struct {
	ID        string        `json:"id"`
	Goal      string        `json:"goal"`
	Policy    PolicyPayload `json:"policy"`
	State     string        `json:"state"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

func missionPayload(m domain.Mission) MissionPayload {
	return MissionPayload{
		ID: string(m.ID), Goal: m.Goal, Policy: policyPayload(m.Policy),
		State: string(m.State), CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// TaskPayload is one task.
type TaskPayload struct {
	ID        string    `json:"id"`
	MissionID string    `json:"missionId"`
	Role      string    `json:"role"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	AgentID   string    `json:"agentId,omitempty"`
	StepsUsed int       `json:"stepsUsed"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func taskPayload(t domain.Task) TaskPayload {
	return TaskPayload{
		ID: string(t.ID), MissionID: string(t.MissionID), Role: string(t.Role),
		Title: t.Title, State: string(t.State), AgentID: string(t.AgentID),
		StepsUsed: t.StepsUsed, CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
	}
}

// ApprovalPayload is what the UI and CLI render a pending decision from. It
// carries the exact action, the structured reasons, and — once approved —
// the authorization identity. It never carries a blanket "allow all".
type ApprovalPayload struct {
	ID           string          `json:"id"`
	Action       ActionPayload   `json:"action"`
	ActionDigest string          `json:"actionDigest"`
	MissionID    string          `json:"missionId"`
	TaskID       string          `json:"taskId"`
	AgentID      string          `json:"agentId"`
	AgentName    string          `json:"agentName"`
	Role         string          `json:"role"`
	Mode         string          `json:"mode"`
	Reasons      []ReasonPayload `json:"reasons"`
	State        string          `json:"state"`
	RequestedAt  time.Time       `json:"requestedAt"`
	ExpiresAt    time.Time       `json:"expiresAt"`
	DecidedAt    *time.Time      `json:"decidedAt,omitempty"`
	DecidedBy    string          `json:"decidedBy,omitempty"`
	// AuthorizationID is set only when State is "approved".
	AuthorizationID string `json:"authorizationId,omitempty"`
}

func approvalPayload(a domain.ApprovalRequest) ApprovalPayload {
	p := ApprovalPayload{
		ID: string(a.ID), Action: ActionToPayload(a.Action), ActionDigest: a.ActionDigest,
		MissionID: string(a.MissionID), TaskID: string(a.TaskID),
		AgentID: string(a.AgentID), AgentName: a.AgentName, Role: string(a.Role),
		Mode: string(a.Mode), State: string(a.State),
		RequestedAt: a.RequestedAt, ExpiresAt: a.ExpiresAt, DecidedAt: a.DecidedAt,
		DecidedBy: string(a.DecidedBy.Source),
	}
	for _, r := range a.Reasons {
		p.Reasons = append(p.Reasons, ReasonPayload{Code: string(r.Code), Params: r.Params})
	}
	if a.Authorization != nil {
		p.AuthorizationID = string(a.Authorization.ID)
	}
	return p
}

// CoveragePayload is one execution path's enforcement status.
type CoveragePayload struct {
	Path      string    `json:"path"`
	State     string    `json:"state"`
	Basis     string    `json:"basis"`
	CheckedAt time.Time `json:"checkedAt"`
}

func coveragePayload(c domain.Coverage) CoveragePayload {
	return CoveragePayload{Path: c.Path, State: string(c.State), Basis: c.Basis, CheckedAt: c.CheckedAt}
}

// DecisionRecordPayload is one audited decision.
type DecisionRecordPayload struct {
	Outcome      string          `json:"outcome"`
	Reasons      []ReasonPayload `json:"reasons"`
	ActionDigest string          `json:"actionDigest"`
	Action       ActionPayload   `json:"action"`
	MissionID    string          `json:"missionId,omitempty"`
	TaskID       string          `json:"taskId,omitempty"`
	AgentID      string          `json:"agentId,omitempty"`
	Mode         string          `json:"mode,omitempty"`
	Coverage     string          `json:"coverage,omitempty"`
	ApprovalID   string          `json:"approvalId,omitempty"`
	DecidedAt    time.Time       `json:"decidedAt"`
}

func decisionRecordPayload(r store.DecisionRecord) DecisionRecordPayload {
	d := r.Decision
	p := DecisionRecordPayload{
		Outcome: string(d.Outcome), ActionDigest: d.ActionDigest,
		Action:    ActionToPayload(r.Action),
		MissionID: string(d.MissionID), TaskID: string(d.TaskID), AgentID: string(d.AgentID),
		Mode: string(d.Mode), Coverage: string(d.Coverage),
		ApprovalID: string(d.ApprovalID), DecidedAt: d.DecidedAt,
	}
	for _, reason := range d.Reasons {
		p.Reasons = append(p.Reasons, ReasonPayload{Code: string(reason.Code), Params: reason.Params})
	}
	return p
}
