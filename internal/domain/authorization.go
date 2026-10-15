package domain

import "time"

// Authorization is a single-use, tightly bound grant produced by a user
// approval. It is the only thing that lets a REQUIRE_APPROVAL action run, and
// it is bound to exactly one agent, mission, task, action and scope, with an
// expiry (book §12.1). Nothing about it is reusable: not for another agent,
// not for another task, not for a bigger action, not even for the same action
// twice (a denied-then-retried action needs a fresh approval).
type Authorization struct {
	// ID equals the ApprovalID that produced it — the audit link between
	// "user said yes" and "this execution was authorized".
	ID ApprovalID

	// Nonce is the opaque, single-use token value the retrying action presents.
	// It authorizes only the OneAction described by ActionDigest.
	Nonce string

	MissionID MissionID
	TaskID    TaskID
	AgentID   AgentID

	// ActionDigest binds the exact action (kind and target). A retry with a
	// changed target fails binding (S06).
	ActionDigest string

	// ScopeDigest binds the scope in force when the grant was made: widening
	// or narrowing the mission scope afterwards invalidates the grant.
	ScopeDigest string

	GrantedAt time.Time
	ExpiresAt time.Time
	GrantedBy Actor

	// ConsumedAt is set the first time the grant is spent; the state store
	// claims it atomically (compare-and-set), so concurrent retries cannot
	// both win.
	ConsumedAt *time.Time
}

// AuthorizationTTL is how long a user approval remains usable. Short by
// design: approvals are for the decision in front of the user, not a standing
// permission.
const AuthorizationTTL = 10 * time.Minute

// Check verifies the grant against the operation about to run. It returns nil
// only when every binding holds, the grant is unexpired and unconsumed.
//
// Check is pure validation; consuming the grant (marking ConsumedAt) is the
// store's atomic operation. Callers must consume *before* releasing execution,
// not after, or two racing retries could both pass Check.
func (a Authorization) Check(now time.Time, action Action, mission MissionID, task TaskID, agent AgentID, scope Scope) error {
	if a.MissionID != mission || a.TaskID != task || a.AgentID != agent {
		return NewError(ErrAuthorizationBinding,
			"authorization %s was granted to agent %s for task %s, not to this requester", a.ID, a.AgentID, a.TaskID).
			WithReasons(NewReason(ReasonIdentityUnknown, "agent", string(agent)))
	}
	if got := action.CanonicalDigest(); got != a.ActionDigest {
		return NewError(ErrAuthorizationBinding,
			"authorized action digest %s does not match attempted action digest %s", a.ActionDigest, got).
			WithReasons(NewReason(ReasonActionChanged))
	}
	if got := scope.ScopeDigest(); got != a.ScopeDigest {
		return NewError(ErrAuthorizationBinding, "mission scope changed since the approval was granted").
			WithReasons(NewReason(ReasonActionChanged, "what", "scope"))
	}
	if now.After(a.ExpiresAt) {
		return NewError(ErrApprovalExpired, "authorization %s expired at %s", a.ID, a.ExpiresAt.Format(time.RFC3339)).
			WithReasons(NewReason(ReasonApprovalExpired))
	}
	if a.ConsumedAt != nil {
		return NewError(ErrAuthorizationReplayed, "authorization %s was already used at %s", a.ID, a.ConsumedAt.Format(time.RFC3339)).
			WithReasons(NewReason(ReasonAuthorizationReplayed))
	}
	return nil
}

// ApprovalState is the lifecycle of a pending approval.
type ApprovalState string

const (
	ApprovalPending   ApprovalState = "pending"
	ApprovalApproved  ApprovalState = "approved"
	ApprovalDenied    ApprovalState = "denied"
	ApprovalExpired   ApprovalState = "expired"
	ApprovalCancelled ApprovalState = "cancelled"
)

// ApprovalRequest is what the user sees and decides on. It carries the exact
// action (so the UI can show the affected operation, object and range — UX-07)
// and structured reasons the UI localizes; it never carries a blanket "allow
// everything" affordance — one approval, one action.
type ApprovalRequest struct {
	ID           ApprovalID
	Action       Action
	ActionDigest string

	MissionID MissionID
	TaskID    TaskID
	AgentID   AgentID
	AgentName string
	Role      AgentRole
	Mode      AutonomyMode

	// ScopeDigest is the mission scope in force when the approval was
	// requested; the authorization produced on approval binds it, so a scope
	// change between asking and approving invalidates the grant.
	ScopeDigest string

	// Reasons explains, in structured form, why approval is required — the
	// same codes the decision carries, so the two can never drift.
	Reasons []Reason

	State       ApprovalState
	RequestedAt time.Time
	ExpiresAt   time.Time
	DecidedAt   *time.Time
	DecidedBy   Actor

	// Authorization is set when, and only when, State is ApprovalApproved.
	Authorization *Authorization
}

// ApprovalTTL is how long a pending approval waits before it expires. An
// expired approval may not be approved afterwards — the user must see a fresh
// request rather than act on a stale one.
const ApprovalTTL = 15 * time.Minute

// Decidable reports whether the approval can still take a user decision.
func (r ApprovalRequest) Decidable(now time.Time) bool {
	return r.State == ApprovalPending && now.Before(r.ExpiresAt)
}
