package domain

// ReasonCode identifies *why* a decision came out the way it did, in a form the
// UI can render in any language without re-deriving the security logic. The
// plain-language strings ("开发员想修改项目目录之外的文件，已被阻止") live in the
// localization layer, keyed by these codes; the control engine must never
// produce user-facing prose itself, and the two language renderings of one code
// must state the same fact (UX-13).
type ReasonCode string

// The canonical reason codes. Grouped by the question they answer in the UI:
// what happened, why it needs attention, what Qiyan did, what the user can do.
const (
	// Scope reasons.
	ReasonPathOutsideScope     ReasonCode = "path_outside_scope"    // target resolves outside the authorized roots
	ReasonToolNotInScope       ReasonCode = "tool_not_in_scope"     // tool/action kind not authorized for this task
	ReasonWriteByReadOnly      ReasonCode = "write_by_readonly"     // a read-only role attempted a mutating action
	ReasonDelegationEscalation ReasonCode = "delegation_escalation" // delegation would grant more than the delegator holds
	ReasonBudgetExhausted      ReasonCode = "budget_exhausted"

	// Sensitivity reasons — these turn an in-scope action into REQUIRE_APPROVAL.
	ReasonDeleteOperation     ReasonCode = "delete_operation"
	ReasonCredentialReference ReasonCode = "credential_reference"
	ReasonExternalNetwork     ReasonCode = "external_network"
	ReasonExternalStateChange ReasonCode = "external_state_change"
	ReasonUnrecognizedCommand ReasonCode = "unrecognized_command" // cannot be reliably interpreted → never silently allowed
	ReasonSensitiveCommand    ReasonCode = "sensitive_command"    // recognized but high-impact (e.g. process control, package install)

	// Outcome mechanics.
	ReasonUserApproved          ReasonCode = "user_approved"
	ReasonUserDenied            ReasonCode = "user_denied"
	ReasonApprovalExpired       ReasonCode = "approval_expired"
	ReasonApprovalCancelled     ReasonCode = "approval_cancelled"
	ReasonAuthorizationReplayed ReasonCode = "authorization_replayed"
	ReasonActionChanged         ReasonCode = "action_changed" // approved target differs from the retried target
	ReasonTaskCancelled         ReasonCode = "task_cancelled"
	ReasonControlUnavailable    ReasonCode = "control_unavailable"
	ReasonIdentityUnknown       ReasonCode = "identity_unknown"
	ReasonModeCompanion         ReasonCode = "mode_companion" // companion mode: sensitive actions wait for the user
	ReasonModeControlledAuto    ReasonCode = "mode_controlled_auto"
	ReasonModeBoundedAutonomy   ReasonCode = "mode_bounded_autonomy"

	// ReasonNoExternalEffect marks a session-local tool pass-through: allowed,
	// recorded, and explicitly NOT a protected action (S10 honesty).
	ReasonNoExternalEffect ReasonCode = "no_external_effect"

	// Coverage honesty.
	ReasonPathNotCovered ReasonCode = "path_not_covered" // action lies on an execution path Qiyan cannot enforce
)

// Reason is a code plus interpolation parameters for the UI. Params carries
// structured values (paths, tool names, roles) so localization controls
// wording, not facts.
type Reason struct {
	Code   ReasonCode
	Params map[string]string
}

// NewReason builds a Reason with key/value parameters; callers pass pairs.
func NewReason(code ReasonCode, kv ...string) Reason {
	r := Reason{Code: code}
	if len(kv) > 0 {
		r.Params = make(map[string]string, len(kv)/2)
		for i := 0; i+1 < len(kv); i += 2 {
			r.Params[kv[i]] = kv[i+1]
		}
	}
	return r
}
