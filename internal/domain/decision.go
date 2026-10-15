package domain

import "time"

// Outcome is the control engine's verdict on one action (book §12).
type Outcome string

const (
	// OutcomeAllow: the action satisfies an explicit authorization.
	OutcomeAllow Outcome = "ALLOW"
	// OutcomeDeny: the action is explicitly not permitted.
	OutcomeDeny Outcome = "DENY"
	// OutcomeRequireApproval: execution waits for a trusted user decision.
	OutcomeRequireApproval Outcome = "REQUIRE_APPROVAL"
	// OutcomeUnknown: evidence is insufficient to conclude "allowed". On an
	// enforced path this must never be treated as ALLOW.
	OutcomeUnknown Outcome = "UNKNOWN"
)

// KnownOutcome reports whether o is one of the four defined outcomes.
func KnownOutcome(o Outcome) bool {
	switch o {
	case OutcomeAllow, OutcomeDeny, OutcomeRequireApproval, OutcomeUnknown:
		return true
	}
	return false
}

// Restrictiveness orders outcomes when rules with different effects apply to
// one action: the most restrictive effect governs. UNKNOWN outranks ALLOW
// because "not proven allowed" must not be weakened by a permissive rule.
func (o Outcome) Restrictiveness() int {
	switch o {
	case OutcomeDeny:
		return 3
	case OutcomeRequireApproval:
		return 2
	case OutcomeUnknown:
		return 1
	default:
		return 0
	}
}

// CombineOutcomes folds outcomes into the single governing one.
func CombineOutcomes(outcomes ...Outcome) Outcome {
	worst := OutcomeAllow
	for _, o := range outcomes {
		if o.Restrictiveness() > worst.Restrictiveness() {
			worst = o
		}
	}
	return worst
}

// Decision is a complete, replayable verdict: what the engine concluded, why,
// on what basis of evidence, and for which exact action. Storing decisions in
// this shape is what lets the UI explain any outcome and the final report
// audit it, without re-running policy.
type Decision struct {
	Outcome      Outcome
	Reasons      []Reason
	ActionDigest string

	// The requester context, stored with every decision for audit.
	MissionID MissionID
	TaskID    TaskID
	AgentID   AgentID

	// Mode is the autonomy mode in force when the decision was made, and
	// Coverage records whether the deciding path can actually enforce anything
	// (ENFORCED) or only observe. A decision on an OBSERVED-only path must not
	// be presented as protection (S10).
	Mode     AutonomyMode
	Coverage CoverageState

	// ApprovalID is set when the outcome is OutcomeRequireApproval and a
	// pending approval was recorded for the user.
	ApprovalID ApprovalID

	DecidedAt time.Time
}

// EnforcementOutcome maps a decision to what a pre-execution gate must do.
// Only an explicit ALLOW lets the action proceed; UNKNOWN collapses to deny —
// this is the mechanical form of "对强制控制路径，UNKNOWN 不得自动转为 ALLOW".
func (d Decision) EnforcementOutcome() Outcome {
	if d.Outcome == OutcomeUnknown {
		return OutcomeDeny
	}
	return d.Outcome
}
