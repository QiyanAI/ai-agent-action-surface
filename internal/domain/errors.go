package domain

import "fmt"

// ErrorClass is a stable, machine-readable error classification. Classes are
// part of the wire contract (API error bodies, hook exit behavior); the human
// message is not. New classes may be added; existing strings must not change.
type ErrorClass string

// The stable error classes. Each describes *why* a decision or request failed,
// not what to display: user-facing text is produced from ReasonCodes so that
// English and Chinese renderings stay semantically identical (UX-13).
const (
	// ErrOutOfScope: the action exceeds the task's authorized scope.
	ErrOutOfScope ErrorClass = "OUT_OF_SCOPE"
	// ErrUnrecognizedAction: the action cannot be reliably interpreted, and a
	// protected path must never guess. Callers map this to deny or approval —
	// never to allow.
	ErrUnrecognizedAction ErrorClass = "UNRECOGNIZED_ACTION"
	// ErrApprovalRequired: execution may proceed only after a user decision.
	ErrApprovalRequired ErrorClass = "APPROVAL_REQUIRED"
	// ErrApprovalNotFound: the referenced approval does not exist.
	ErrApprovalNotFound ErrorClass = "APPROVAL_NOT_FOUND"
	// ErrApprovalExpired: the approval or authorization expired before use.
	ErrApprovalExpired ErrorClass = "APPROVAL_EXPIRED"
	// ErrApprovalCancelled: the user cancelled the pending approval.
	ErrApprovalCancelled ErrorClass = "APPROVAL_CANCELLED"
	// ErrAuthorizationBinding: the authorization does not match this agent,
	// mission, task, action or scope — including a swapped operation target.
	ErrAuthorizationBinding ErrorClass = "AUTHORIZATION_BINDING"
	// ErrAuthorizationReplayed: the single-use authorization was already spent.
	ErrAuthorizationReplayed ErrorClass = "AUTHORIZATION_REPLAYED"
	// ErrUnknownIdentity: the caller's or agent's identity cannot be confirmed.
	ErrUnknownIdentity ErrorClass = "UNKNOWN_IDENTITY"
	// ErrIdentityConflict: the identity is confirmed but contradicts the mission
	// plan (e.g. a read-only role asking for a write).
	ErrIdentityConflict ErrorClass = "IDENTITY_CONFLICT"
	// ErrControlUnavailable: the deciding component cannot decide right now.
	// Protected paths must treat this as deny.
	ErrControlUnavailable ErrorClass = "CONTROL_UNAVAILABLE"
	// ErrStoreCorrupted: local state is unreadable or fails validation.
	ErrStoreCorrupted ErrorClass = "STORE_CORRUPTED"
	// ErrBudgetExhausted: the task's turn/time/step budget is spent.
	ErrBudgetExhausted ErrorClass = "BUDGET_EXHAUSTED"
	// ErrTaskCancelled: the task was cancelled; no new controlled actions.
	ErrTaskCancelled ErrorClass = "TASK_CANCELLED"
	// ErrInvalidRequest / ErrNotFound / ErrConflict / ErrInternal: transport-
	// and bookkeeping-level classes for the API layer.
	ErrInvalidRequest ErrorClass = "INVALID_REQUEST"
	ErrNotFound       ErrorClass = "NOT_FOUND"
	ErrConflict       ErrorClass = "CONFLICT"
	ErrInternal       ErrorClass = "INTERNAL"
)

// DomainError is the single error type this package produces. It carries the
// stable class plus structured reasons; it deliberately does not carry
// user-facing prose (that is derived at the edge, per locale).
type DomainError struct {
	Class   ErrorClass
	Message string // developer-facing; safe to log, never shown raw to end users
	Reasons []Reason
}

func (e *DomainError) Error() string {
	if e.Message == "" {
		return string(e.Class)
	}
	return string(e.Class) + ": " + e.Message
}

// NewError builds a DomainError with a formatted developer message.
func NewError(class ErrorClass, format string, args ...any) *DomainError {
	return &DomainError{Class: class, Message: fmt.Sprintf(format, args...)}
}

// WithReasons attaches structured reason codes for user-facing rendering.
func (e *DomainError) WithReasons(reasons ...Reason) *DomainError {
	e.Reasons = append(e.Reasons, reasons...)
	return e
}

// ClassOf extracts the stable class from any error this package or the control
// engine produces; unknown errors are classified INTERNAL. The control engine
// maps classes to outcomes, so this function is the one place that decides how
// an unexpected failure is treated — and UNKNOWN never becomes ALLOW.
func ClassOf(err error) ErrorClass {
	if err == nil {
		return ""
	}
	if de, ok := err.(*DomainError); ok {
		return de.Class
	}
	return ErrInternal
}

// FailClosedOutcome maps an error to the outcome a protected path must take.
// Every branch returns DENY or UNKNOWN; this is the mechanical guarantee behind
// "检查程序异常、控制服务失联、超时、无效响应或身份无法确认，必须……拒绝或停止"
// (book §09).
func FailClosedOutcome(err error) Outcome {
	switch ClassOf(err) {
	case "":
		return OutcomeAllow
	case ErrApprovalRequired:
		return OutcomeRequireApproval
	default:
		return OutcomeDeny
	}
}
