package domain

import "time"

// EvidenceLevel states how strongly a claim about an agent's abilities is
// backed (book §13.1). The levels are ordered by strength, and no claim may
// be presented above the level its evidence actually supports — a static
// declaration is never "a confirmed exploitable risk", and only ENFORCED
// means a pre-execution gate really stops the action.
type EvidenceLevel string

const (
	// LevelDeclared: an explicit declaration (manifest, config) says so.
	LevelDeclared EvidenceLevel = "DECLARED"
	// LevelStaticallyConnected: static evidence links subjects (tool wiring,
	// config references), but nothing ran.
	LevelStaticallyConnected EvidenceLevel = "STATICALLY_CONNECTED"
	// LevelObserved: a supported integration point observed the event happen.
	LevelObserved EvidenceLevel = "OBSERVED"
	// LevelEnforced: a pre-execution control actually gated the action —
	// proven by a real allow/deny outcome on a real attempt.
	LevelEnforced EvidenceLevel = "ENFORCED"
	// LevelUnknown: unverified, undetermined, or outside the supported range.
	LevelUnknown EvidenceLevel = "UNKNOWN"
)

// EvidenceSubject names what an evidence record is about.
type EvidenceSubject struct {
	Kind string // "agent" | "tool" | "action" | "path" | "config" | "policy"
	Ref  string // stable reference: agent ID, tool name, config path, event ID
}

// Evidence is one traceable claim about a subject, with its source. Every
// relation shown in the Action Graph must resolve to evidence records like
// these — nodes and edges without a source are not drawn.
type Evidence struct {
	ID      EvidenceID
	Level   EvidenceLevel
	Subject EvidenceSubject
	Cap     Capability

	// SourceRef is where this came from: "file:line" for static findings, an
	// event ID for observed/enforced ones, a session ID for runtime evidence.
	SourceRef string

	// Boundary says what this record does NOT cover. Empty is not allowed for
	// enforced claims: "protection here" always has an edge.
	Boundary string

	ObservedAt time.Time
}

// CoverageState is what Qiyan can actually do on one execution path — the
// three-way distinction of book §09, kept honest: only ENFORCED may be called
// protected.
type CoverageState string

const (
	// CoverageEnforced: Qiyan receives a pre-execution decision point and its
	// deny is proven to stop the action on this path.
	CoverageEnforced CoverageState = "ENFORCED"
	// CoverageObserved: Qiyan sees the event but cannot stop it in time.
	CoverageObserved CoverageState = "OBSERVED"
	// CoverageUncovered: Qiyan cannot prove it even sees the action.
	CoverageUncovered CoverageState = "UNCOVERED"
	// CoverageUnknown: the path's status could not be determined.
	CoverageUnknown CoverageState = "UNKNOWN"
)

// Coverage records the enforcement status of one execution path (e.g.
// "claude-code/2.1.300/PreToolUse/Bash"). Coverage records are produced by
// real verification runs, not by assumption: an integration point that was
// never exercised stays UNKNOWN.
type Coverage struct {
	ID        CoverageID
	Path      string
	State     CoverageState
	Basis     string // what proved it: run ID, test name, or "not verified"
	CheckedAt time.Time
}
