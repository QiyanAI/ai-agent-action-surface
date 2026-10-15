package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"
)

// AutonomyMode is the user-facing control posture (UX-08). Each mode maps to
// concrete policy conditions in the engine — mode is never just a UI label,
// and a more autonomous mode never disables a protection: it changes which
// in-scope actions run without asking, never whether out-of-scope ones are
// stopped.
type AutonomyMode string

const (
	// ModeCompanion (陪同模式): sensitive actions wait for the user.
	ModeCompanion AutonomyMode = "companion"
	// ModeControlledAuto (受控自动): authorized routine actions run; sensitive
	// actions still wait.
	ModeControlledAuto AutonomyMode = "controlled_auto"
	// ModeBoundedAutonomy (有界自治): agents act continuously inside an explicit
	// task, budget, time and tool envelope; over-reach, irreversible or
	// privilege-escalating actions still stop at the gate.
	ModeBoundedAutonomy AutonomyMode = "bounded_autonomy"
)

// KnownMode reports whether m is defined.
func KnownMode(m AutonomyMode) bool {
	switch m {
	case ModeCompanion, ModeControlledAuto, ModeBoundedAutonomy:
		return true
	}
	return false
}

// Scope is the explicit, checkable envelope for one mission (or one task's
// narrowing of it). Everything here is data the engine evaluates against —
// created by a user decision or a default the user confirmed, never inferred
// from an agent's own claims.
type Scope struct {
	// Roots are authorized directories in normalized form (see NormalizePath).
	Roots []string

	// AllowedKinds lists action kinds that may run in scope. An empty list
	// means none — including nothing read — which is a valid, locked-down
	// mission.
	AllowedKinds []ActionKind

	// AllowedTools lists runtime tool names (e.g. "Read", "Edit", "Bash") the
	// mission may use. Empty means none.
	AllowedTools []string

	// Budget: explicit, verifiable bounds (book §18.2). Both required — an
	// "unlimited" default is exactly the kind of hidden grant the book forbids.
	MaxSteps   int           // controlled actions per task
	MaxRuntime time.Duration // wall-clock budget per task
}

// Policy couples a mode with its scope. The engine evaluates (Policy, Action).
type Policy struct {
	Mode  AutonomyMode
	Scope Scope
}

// Validate checks a policy's internal consistency. A policy that fails
// validation must be rejected at creation, not "repaired" at decision time.
func (p Policy) Validate() error {
	if !KnownMode(p.Mode) {
		return NewError(ErrInvalidRequest, "unknown autonomy mode %q", string(p.Mode))
	}
	for _, r := range p.Scope.Roots {
		if r == "" {
			return NewError(ErrInvalidRequest, "scope root must not be empty")
		}
		if !isAbsSlash(r) {
			return NewError(ErrInvalidRequest, "scope root %q must be absolute and normalized", r)
		}
	}
	for _, k := range p.Scope.AllowedKinds {
		if !KnownActionKind(k) {
			return NewError(ErrInvalidRequest, "unknown action kind %q in scope", string(k))
		}
	}
	if p.Scope.MaxSteps < 1 {
		return NewError(ErrInvalidRequest, "scope must set a positive step budget")
	}
	if p.Scope.MaxRuntime <= 0 {
		return NewError(ErrInvalidRequest, "scope must set a positive runtime budget")
	}
	return nil
}

// AllowsKind reports whether the scope lists k. Absence is denial; callers
// must not infer permission from silence.
func (s Scope) AllowsKind(k ActionKind) bool {
	for _, allowed := range s.AllowedKinds {
		if allowed == k {
			return true
		}
	}
	return false
}

// AllowsTool reports whether the scope lists the runtime tool name.
func (s Scope) AllowsTool(tool string) bool {
	for _, t := range s.AllowedTools {
		if strings.EqualFold(t, tool) {
			return true
		}
	}
	return false
}

// ScopeDigest returns a stable digest of the scope's policy-relevant content.
// Authorizations record the digest of the scope they were granted under, so a
// grant cannot silently survive a scope change (S06's sibling guarantee).
func (s Scope) ScopeDigest() string {
	var b strings.Builder
	for _, r := range SortedStringsCopy(s.Roots) {
		b.WriteString("root:")
		b.WriteString(r)
		b.WriteByte(';')
	}
	kinds := make([]string, 0, len(s.AllowedKinds))
	for _, k := range s.AllowedKinds {
		kinds = append(kinds, string(k))
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		b.WriteString("kind:")
		b.WriteString(k)
		b.WriteByte(';')
	}
	tools := make([]string, 0, len(s.AllowedTools))
	for _, t := range s.AllowedTools {
		tools = append(tools, strings.ToLower(t))
	}
	sort.Strings(tools)
	for _, t := range tools {
		b.WriteString("tool:")
		b.WriteString(t)
		b.WriteByte(';')
	}
	b.WriteString("steps:")
	b.WriteString(strconv.Itoa(s.MaxSteps))
	b.WriteString(";runtime:")
	b.WriteString(s.MaxRuntime.String())
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}
