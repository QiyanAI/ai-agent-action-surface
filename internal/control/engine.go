package control

import (
	"strconv"
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
)

// Engine is the single action-decision authority. The hook, the UI, the local
// API and the reference host all reach decisions through Evaluate — that is
// what makes their answers identical by construction rather than by review
// (book §07, §12).
//
// Callers must establish the requester's identity at their own transport
// boundary (hook: session binding; API: bearer token) and pass the resolved
// domain.Agent in the Request. The engine never reads identity from the
// action or from anything an agent could have written.
type Engine struct {
	Store Store
	// Clock is swappable for tests; nil means time.Now.
	Clock func() time.Time
}

// New returns an engine persisting through store.
func New(store Store) *Engine {
	return &Engine{Store: store, Clock: time.Now}
}

func (e *Engine) now(req Request) time.Time {
	if !req.Now.IsZero() {
		return req.Now
	}
	return e.Clock()
}

// Request is one decision request. Mission, Task and Agent are the requester
// context the transport layer resolved; Action is already normalized;
// Nonce optionally presents a previously granted single-use authorization.
type Request struct {
	Mission domain.Mission
	Task    domain.Task
	Agent   domain.Agent
	Action  domain.Action
	Nonce   string

	// Agents is the mission roster (the fleet plus any registered external
	// agents). Delegation targets are validated against it.
	Agents []domain.Agent

	Now time.Time
}

// Evaluate decides one action and records the decision. It returns a Decision
// for every path — a policy denial is a normal result, not an error. An
// error return means the engine could not complete; callers must apply
// domain.FailClosedOutcome(err), which never yields ALLOW.
//
// Audit-or-deny: if the decision cannot be recorded in the Store, the engine
// refuses the action — an unrecorded allow would be invisible to the user.
func (e *Engine) Evaluate(req Request) (domain.Decision, error) {
	now := e.now(req)
	scope := req.Mission.Policy.Scope

	d := domain.Decision{
		ActionDigest: req.Action.CanonicalDigest(),
		MissionID:    req.Mission.ID,
		TaskID:       req.Task.ID,
		AgentID:      req.Agent.ID,
		Mode:         req.Mission.Policy.Mode,
		Coverage:     domain.CoverageEnforced,
		DecidedAt:    now,
	}

	finish := func(outcome domain.Outcome, reasons ...domain.Reason) (domain.Decision, error) {
		d.Outcome = outcome
		d.Reasons = append(d.Reasons, reasons...)
		if err := e.Store.AppendDecision(d, req.Action); err != nil {
			// Recording failed: the caller must not receive a usable verdict.
			denied := d
			denied.Outcome = domain.OutcomeDeny
			denied.Reasons = append(denied.Reasons, domain.NewReason(domain.ReasonControlUnavailable, "stage", "audit"))
			return denied, domain.NewError(domain.ErrStoreCorrupted, "decision could not be recorded: %v", err)
		}
		return d, nil
	}

	// --- Structural checks: who, when, on what. These apply to every path,
	// including the approved-retry path. ---

	if req.Mission.State == domain.MissionCancelled {
		return finish(domain.OutcomeDeny, domain.NewReason(domain.ReasonTaskCancelled, "mission", string(req.Mission.ID)))
	}
	switch req.Task.State {
	case domain.TaskRunning, domain.TaskWaitingApproval:
		// May act.
	case domain.TaskCancelled:
		return finish(domain.OutcomeDeny, domain.NewReason(domain.ReasonTaskCancelled, "task", string(req.Task.ID)))
	default:
		return finish(domain.OutcomeUnknown, domain.NewReason(domain.ReasonIdentityUnknown, "task_state", string(req.Task.State)))
	}

	if req.Agent.ID == "" || req.Task.AgentID != req.Agent.ID {
		return finish(domain.OutcomeDeny, domain.NewReason(domain.ReasonIdentityUnknown,
			"task_agent", string(req.Task.AgentID), "requester", string(req.Agent.ID)))
	}
	if len(req.Agents) > 0 && !rosterContains(req.Agents, req.Agent.ID) {
		return finish(domain.OutcomeDeny, domain.NewReason(domain.ReasonIdentityUnknown,
			"requester", string(req.Agent.ID), "reason", "not_in_mission_roster"))
	}

	if req.Task.StepsUsed >= scope.MaxSteps {
		return finish(domain.OutcomeDeny, domain.NewReason(domain.ReasonBudgetExhausted,
			"budget", "steps", "used", strconv.Itoa(req.Task.StepsUsed), "limit", strconv.Itoa(scope.MaxSteps)))
	}
	if !req.Task.StartedAt.IsZero() && now.Sub(req.Task.StartedAt) > scope.MaxRuntime {
		return finish(domain.OutcomeDeny, domain.NewReason(domain.ReasonBudgetExhausted, "budget", "runtime"))
	}

	a := req.Action
	if !domain.KnownActionKind(a.Kind) {
		return finish(domain.OutcomeDeny, domain.NewReason(domain.ReasonUnrecognizedCommand, "kind", string(a.Kind)))
	}
	if req.Agent.ReadOnly && a.Kind.Mutation() && a.Kind != domain.ActionDelegation {
		return finish(domain.OutcomeDeny, domain.NewReason(domain.ReasonWriteByReadOnly,
			"role", string(req.Agent.Role), "kind", string(a.Kind)))
	}
	if a.Tool == "" {
		return finish(domain.OutcomeDeny, domain.NewReason(domain.ReasonUnrecognizedCommand, "tool", "missing"))
	}
	if !scope.AllowsTool(a.Tool) {
		return finish(domain.OutcomeDeny, domain.NewReason(domain.ReasonToolNotInScope, "tool", a.Tool))
	}
	if !scope.AllowsKind(a.Kind) {
		return finish(domain.OutcomeDeny, domain.NewReason(domain.ReasonToolNotInScope, "kind", string(a.Kind)))
	}
	if isFileKind(a.Kind) {
		if !domain.IsAbsoluteNormalized(a.Path) {
			// Guessing at unnormalized paths is exactly what the book forbids.
			return finish(domain.OutcomeDeny, domain.NewReason(domain.ReasonUnrecognizedCommand, "path", a.Path))
		}
		if !domain.PathWithinRoots(scope.Roots, a.Path) {
			return finish(domain.OutcomeDeny, domain.NewReason(domain.ReasonPathOutsideScope, "path", a.Path))
		}
	}

	// --- Approved-retry path: a presented nonce must be genuine, bound and
	// unspent. A failed claim is a hard denial — never a silent downgrade to
	// re-asking. ---

	if req.Nonce != "" {
		auth, err := e.Store.ClaimAuthorization(req.Nonce, now)
		if err != nil {
			reasons := reasonsOf(err)
			return finish(domain.FailClosedOutcome(err), reasons...)
		}
		if err := auth.Check(now, a, req.Mission.ID, req.Task.ID, req.Agent.ID, scope); err != nil {
			reasons := reasonsOf(err)
			return finish(domain.FailClosedOutcome(err), reasons...)
		}
		// The sensitivity that required approval is satisfied; grant.
		if err := e.Store.IncrementSteps(req.Task.ID); err != nil {
			return finish(domain.OutcomeDeny, domain.NewReason(domain.ReasonControlUnavailable, "stage", "budget"))
		}
		return finish(domain.OutcomeAllow,
			domain.NewReason(domain.ReasonUserApproved, "approval", string(auth.ID)))
	}

	// --- Policy gating for everything else. ---

	outcome, reasons := e.gate(req, scope)
	switch outcome {
	case domain.OutcomeAllow:
		if err := e.Store.IncrementSteps(req.Task.ID); err != nil {
			return finish(domain.OutcomeDeny, domain.NewReason(domain.ReasonControlUnavailable, "stage", "budget"))
		}
		return finish(domain.OutcomeAllow, reasons...)

	case domain.OutcomeRequireApproval:
		return e.requireApproval(req, scope, now, reasons, func(id domain.ApprovalID) { d.ApprovalID = id }, finish)

	default:
		return finish(domain.OutcomeDeny, reasons...)
	}
}

// gate applies the mode-and-kind policy to a structurally valid request.
func (e *Engine) gate(req Request, scope domain.Scope) (domain.Outcome, []domain.Reason) {
	a := req.Action
	mode := req.Mission.Policy.Mode

	switch a.Kind {
	case domain.ActionFileRead:
		return domain.OutcomeAllow, nil

	case domain.ActionFileCreate, domain.ActionFileModify:
		if mode == domain.ModeCompanion {
			return domain.OutcomeRequireApproval, []domain.Reason{domain.NewReason(domain.ReasonModeCompanion, "kind", string(a.Kind))}
		}
		return domain.OutcomeAllow, nil

	case domain.ActionFileDelete:
		// Deleting is irreversible in the general case; it waits for the user
		// in every mode (book §UX-08: 越权、不可逆……仍受强制门禁约束).
		return domain.OutcomeRequireApproval, []domain.Reason{domain.NewReason(domain.ReasonDeleteOperation, "path", a.Path)}

	case domain.ActionProcessExec:
		class, program := ClassifyCommand(a.Command)
		switch class {
		case CmdSafe:
			return domain.OutcomeAllow, nil
		case CmdSensitive:
			return domain.OutcomeRequireApproval, CommandReasons(class, program)
		default:
			return domain.OutcomeRequireApproval, CommandReasons(class, program)
		}

	case domain.ActionNetwork:
		return domain.OutcomeRequireApproval, []domain.Reason{domain.NewReason(domain.ReasonExternalNetwork, "target", a.URL)}

	case domain.ActionCredentialUse:
		return domain.OutcomeRequireApproval, []domain.Reason{domain.NewReason(domain.ReasonCredentialReference, "credential", a.CredentialRef)}

	case domain.ActionExternalState:
		return domain.OutcomeRequireApproval, []domain.Reason{domain.NewReason(domain.ReasonExternalStateChange, "target", a.URL)}

	case domain.ActionDelegation:
		return e.gateDelegation(req, scope)

	default: // ActionUnknown and anything unforeseen: the user decides, explicitly.
		return domain.OutcomeRequireApproval, []domain.Reason{domain.NewReason(domain.ReasonUnrecognizedCommand, "kind", string(a.Kind))}
	}
}

// delegateTransitions lists the role handoffs the fleet supports. Delegation
// outside this shape is refused rather than guessed at.
var delegateTransitions = map[domain.AgentRole]map[domain.AgentRole]bool{
	domain.RolePlanner:  {domain.RoleBuilder: true},
	domain.RoleBuilder:  {domain.RoleReviewer: true},
	domain.RoleReviewer: {domain.RoleBuilder: true},
}

// gateDelegation enforces book §11.3: delegation must not widen anything. The
// receiver's authority is never inherited from the delegator — it comes from
// the receiver's own task — and a delegation may not smuggle in an action
// kind, or a mutating workload aimed at a read-only role, that the mission
// envelope does not itself cover.
func (e *Engine) gateDelegation(req Request, scope domain.Scope) (domain.Outcome, []domain.Reason) {
	a := req.Action
	to, _ := a.DelegationsOf()

	if a.DelegationKind == domain.ActionDelegation {
		return domain.OutcomeDeny, []domain.Reason{domain.NewReason(domain.ReasonDelegationEscalation, "reason", "transitive_delegation")}
	}
	if !domain.KnownActionKind(a.DelegationKind) || a.DelegationKind == domain.ActionUnknown {
		return domain.OutcomeDeny, []domain.Reason{domain.NewReason(domain.ReasonUnrecognizedCommand, "delegated_kind", string(a.DelegationKind))}
	}
	receiver := findAgent(req.Agents, to)
	if receiver == nil {
		return domain.OutcomeDeny, []domain.Reason{domain.NewReason(domain.ReasonIdentityUnknown, "delegation_target", string(to))}
	}
	if !delegateTransitions[req.Agent.Role][receiver.Role] {
		return domain.OutcomeDeny, []domain.Reason{domain.NewReason(domain.ReasonDelegationEscalation,
			"from", string(req.Agent.Role), "to", string(receiver.Role))}
	}
	if !scope.AllowsKind(a.DelegationKind) {
		return domain.OutcomeDeny, []domain.Reason{domain.NewReason(domain.ReasonDelegationEscalation,
			"reason", "delegated_kind_outside_mission_scope", "delegated_kind", string(a.DelegationKind))}
	}
	if receiver.ReadOnly && a.DelegationKind.Mutation() {
		return domain.OutcomeDeny, []domain.Reason{domain.NewReason(domain.ReasonDelegationEscalation,
			"reason", "mutating_work_to_readonly_receiver", "receiver", string(receiver.ID))}
	}
	return domain.OutcomeAllow, nil
}

// requireApproval dedupes and remembers: one pending request per exact action,
// a user's deny is final for that action within the task, and everything else
// produces a fresh, expiring request.
func (e *Engine) requireApproval(req Request, scope domain.Scope, now time.Time, reasons []domain.Reason,
	setApproval func(domain.ApprovalID),
	finish func(domain.Outcome, ...domain.Reason) (domain.Decision, error)) (domain.Decision, error) {

	digest := req.Action.CanonicalDigest()
	existing, found, err := e.Store.LatestApprovalForAction(req.Task.ID, req.Agent.ID, digest)
	if err != nil {
		return finish(domain.OutcomeDeny, domain.NewReason(domain.ReasonControlUnavailable, "stage", "approval_lookup"))
	}
	if found {
		switch existing.State {
		case domain.ApprovalPending:
			if existing.Decidable(now) {
				setApproval(existing.ID)
				return finish(domain.OutcomeRequireApproval, reasons...)
			}
			// Expire the stale request, then issue a fresh one below.
			existing.State = domain.ApprovalExpired
			t := now
			existing.DecidedAt = &t
			if err := e.Store.UpdateApproval(existing); err != nil {
				return finish(domain.OutcomeDeny, domain.NewReason(domain.ReasonControlUnavailable, "stage", "approval_expire"))
			}
		case domain.ApprovalDenied:
			return finish(domain.OutcomeDeny, domain.NewReason(domain.ReasonUserDenied, "approval", string(existing.ID)))
		case domain.ApprovalCancelled:
			return finish(domain.OutcomeDeny, domain.NewReason(domain.ReasonApprovalCancelled, "approval", string(existing.ID)))
		}
		// Approved (authorization spent or not presented), expired: ask anew.
	}

	ap := domain.ApprovalRequest{
		ID:           domain.NewApprovalID(),
		Action:       req.Action,
		ActionDigest: digest,
		MissionID:    req.Mission.ID,
		TaskID:       req.Task.ID,
		AgentID:      req.Agent.ID,
		AgentName:    req.Agent.Name,
		Role:         req.Agent.Role,
		Mode:         req.Mission.Policy.Mode,
		ScopeDigest:  scope.ScopeDigest(),
		Reasons:      reasons,
		State:        domain.ApprovalPending,
		RequestedAt:  now,
		ExpiresAt:    now.Add(domain.ApprovalTTL),
	}
	if err := e.Store.CreateApproval(ap); err != nil {
		return finish(domain.OutcomeDeny, domain.NewReason(domain.ReasonControlUnavailable, "stage", "approval_create"))
	}
	setApproval(ap.ID)
	return finish(domain.OutcomeRequireApproval, reasons...)
}

// Decide records the user's verdict on a pending approval. Approving mints the
// single-use Authorization bound to the exact action and scope; denying ends
// the matter for this action.
//
// Only the local user may decide — an agent cannot approve its own
// high-impact action (book §12.3), and this check lives here so every
// interface inherits it.
func (e *Engine) Decide(approvalID domain.ApprovalID, approve bool, by domain.Actor, now time.Time) (domain.ApprovalRequest, error) {
	if by.Source != domain.SourceLocalUser {
		return domain.ApprovalRequest{}, domain.NewError(domain.ErrIdentityConflict,
			"approvals are decided by the local user, not by %s", by.Source)
	}
	ap, found, err := e.Store.GetApproval(approvalID)
	if err != nil {
		return domain.ApprovalRequest{}, domain.NewError(domain.ErrStoreCorrupted, "approval lookup failed: %v", err)
	}
	if !found {
		return domain.ApprovalRequest{}, domain.NewError(domain.ErrApprovalNotFound, "approval %s not found", approvalID)
	}
	if !ap.Decidable(now) {
		return domain.ApprovalRequest{}, domain.NewError(domain.ErrConflict,
			"approval %s is %s and can no longer be decided", approvalID, ap.State)
	}

	t := now
	ap.DecidedAt = &t
	ap.DecidedBy = by

	if !approve {
		ap.State = domain.ApprovalDenied
		if err := e.Store.UpdateApproval(ap); err != nil {
			return domain.ApprovalRequest{}, domain.NewError(domain.ErrStoreCorrupted, "approval update failed: %v", err)
		}
		return ap, nil
	}

	auth := domain.Authorization{
		ID:           ap.ID,
		Nonce:        domain.NewNonce(),
		MissionID:    ap.MissionID,
		TaskID:       ap.TaskID,
		AgentID:      ap.AgentID,
		ActionDigest: ap.ActionDigest,
		ScopeDigest:  ap.ScopeDigest,
		GrantedAt:    now,
		ExpiresAt:    now.Add(domain.AuthorizationTTL),
		GrantedBy:    by,
	}
	if err := e.Store.CreateAuthorization(auth); err != nil {
		return domain.ApprovalRequest{}, domain.NewError(domain.ErrStoreCorrupted, "authorization could not be stored: %v", err)
	}
	ap.State = domain.ApprovalApproved
	ap.Authorization = &auth
	if err := e.Store.UpdateApproval(ap); err != nil {
		return domain.ApprovalRequest{}, domain.NewError(domain.ErrStoreCorrupted, "approval update failed: %v", err)
	}
	return ap, nil
}

// CancelApproval lets the user or the orchestrator withdraw a pending request
// (for example when the task is cancelled). A cancelled approval cannot be
// approved later.
func (e *Engine) CancelApproval(approvalID domain.ApprovalID, by domain.Actor, now time.Time) error {
	ap, found, err := e.Store.GetApproval(approvalID)
	if err != nil {
		return domain.NewError(domain.ErrStoreCorrupted, "approval lookup failed: %v", err)
	}
	if !found {
		return domain.NewError(domain.ErrApprovalNotFound, "approval %s not found", approvalID)
	}
	if ap.State != domain.ApprovalPending {
		return nil // already decided; nothing to cancel
	}
	ap.State = domain.ApprovalCancelled
	ap.DecidedBy = by
	t := now
	ap.DecidedAt = &t
	if err := e.Store.UpdateApproval(ap); err != nil {
		return domain.NewError(domain.ErrStoreCorrupted, "approval update failed: %v", err)
	}
	return nil
}

// --- small helpers ---

func isFileKind(k domain.ActionKind) bool {
	switch k {
	case domain.ActionFileRead, domain.ActionFileCreate, domain.ActionFileModify, domain.ActionFileDelete:
		return true
	}
	return false
}

func rosterContains(agents []domain.Agent, id domain.AgentID) bool {
	for _, a := range agents {
		if a.ID == id {
			return true
		}
	}
	return false
}

func findAgent(agents []domain.Agent, id domain.AgentID) *domain.Agent {
	for i := range agents {
		if agents[i].ID == id {
			return &agents[i]
		}
	}
	return nil
}

// reasonsOf extracts the structured reasons from a domain error so decisions
// carry them even when the refusal came from the assertion layer. Errors that
// left detail behind get their canonical reason code back here.
func reasonsOf(err error) []domain.Reason {
	if de, ok := err.(*domain.DomainError); ok && len(de.Reasons) > 0 {
		return de.Reasons
	}
	switch domain.ClassOf(err) {
	case domain.ErrAuthorizationReplayed:
		return []domain.Reason{domain.NewReason(domain.ReasonAuthorizationReplayed)}
	case domain.ErrApprovalNotFound:
		return []domain.Reason{domain.NewReason(domain.ReasonApprovalExpired, "reason", "authorization_not_recognized")}
	case domain.ErrApprovalExpired:
		return []domain.Reason{domain.NewReason(domain.ReasonApprovalExpired)}
	case domain.ErrAuthorizationBinding:
		return []domain.Reason{domain.NewReason(domain.ReasonActionChanged)}
	}
	return []domain.Reason{domain.NewReason(domain.ReasonControlUnavailable, "class", string(domain.ClassOf(err)))}
}
