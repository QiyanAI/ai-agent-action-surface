package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ActionKind names one category of externally visible action an agent can
// attempt. The set mirrors the book's core action classes (book §12).
type ActionKind string

const (
	ActionFileRead      ActionKind = "file.read"
	ActionFileCreate    ActionKind = "file.create"
	ActionFileModify    ActionKind = "file.modify"
	ActionFileDelete    ActionKind = "file.delete"
	ActionProcessExec   ActionKind = "process.exec"
	ActionNetwork       ActionKind = "network.access"
	ActionCredentialUse ActionKind = "credential.use"
	ActionExternalState ActionKind = "external.state_change"
	ActionDelegation    ActionKind = "agent.delegation"
	// ActionSessionLocal names tools that only touch the agent's own session
	// bookkeeping (todo lists, skill loading, task tracking). They have no
	// effect outside the agent, so gating them would be noise — but they still
	// flow through the engine and are recorded, so the decision authority
	// stays single. They are not "protected": the coverage list says so.
	ActionSessionLocal ActionKind = "session.local"
	ActionUnknown      ActionKind = "unknown"
)

// KnownActionKind reports whether k is one of the defined kinds.
func KnownActionKind(k ActionKind) bool {
	switch k {
	case ActionFileRead, ActionFileCreate, ActionFileModify, ActionFileDelete,
		ActionProcessExec, ActionNetwork, ActionCredentialUse, ActionExternalState,
		ActionDelegation, ActionSessionLocal, ActionUnknown:
		return true
	}
	return false
}

// Mutation reports whether the kind changes local or external state. Read-only
// roles use this to reject mutating actions before any policy runs.
func (k ActionKind) Mutation() bool {
	switch k {
	case ActionFileRead, ActionSessionLocal:
		return false
	default:
		return true
	}
}

// Action is one attempt by an agent to affect something outside itself. It is
// the unit the control engine decides on and the unit an approval authorizes.
//
// Only the semantic fields participate in the canonical digest: Raw is carried
// for evidence and display but must never influence the digest, so cosmetic
// differences in how a runtime spelled the request do not invalidate a
// grant — while any change of kind or target does (S06, "审批评审后更换操作
// 目标不能继续使用旧授权").
type Action struct {
	Kind ActionKind

	// Tool is the enforcement path's tool name (e.g. "Edit", "Bash",
	// "note.append"). It is part of the action's identity: an approval for
	// "change this file via Edit" must not authorize the same file change via
	// a shell command — different tool, different risk, so the digest binds it
	// and the scope's tool allowlist checks it.
	Tool string

	// Path is the target of file.* kinds. Callers must pass it already
	// normalized (absolute, forward slashes, cleaned) via NormalizePath.
	Path string

	// Command is the full command line for process.exec.
	Command string

	// CommandProgram is the resolved program name for process.exec (first
	// token), lowercased, e.g. "git", "npm", with any directory part stripped.
	CommandProgram string

	// URL is the target of network.access, scheme+host+path, query stripped of
	// values is NOT done here — the engine decides how much of a URL is
	// meaningful for policy; the digest binds whatever was evaluated.
	URL string

	// CredentialRef names a credential *by reference* (env var name, keychain
	// item), never its value. Values must not reach this struct (S08).
	CredentialRef string

	// Delegation target and material for agent.delegation.
	DelegationTo   AgentID
	DelegationTask TaskID
	DelegationKind ActionKind // the kind of work being delegated

	// Raw is the runtime's original request fragment (tool input), kept for
	// evidence. Excluded from the digest by design.
	Raw string
}

// CanonicalDigest returns the hex SHA-256 of the action's canonical encoding.
// Two actions with equal semantic fields produce equal digests regardless of
// field order in the original request; any change to kind or target changes it.
func (a Action) CanonicalDigest() string {
	h := sha256.Sum256([]byte(a.canonical()))
	return hex.EncodeToString(h[:])
}

// canonical builds a length-prefixed, order-fixed encoding. Length prefixes
// make the encoding unambiguous ("ab"+"c" must not collide with "a"+"bc").
func (a Action) canonical() string {
	var b strings.Builder
	field := func(name, value string) {
		b.WriteString(name)
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(len(value)))
		b.WriteByte(':')
		b.WriteString(value)
		b.WriteByte(';')
	}
	field("kind", string(a.Kind))
	field("tool", a.Tool)
	field("path", a.Path)
	field("command", a.Command)
	field("program", a.CommandProgram)
	field("url", a.URL)
	field("cred", a.CredentialRef)
	field("dto", string(a.DelegationTo))
	field("dtask", string(a.DelegationTask))
	field("dkind", string(a.DelegationKind))
	return b.String()
}

// DisplayTarget returns a short human-readable target used in logs and as a
// parameter for localized UI strings. It returns data, not sentences.
func (a Action) DisplayTarget() string {
	switch a.Kind {
	case ActionFileRead, ActionFileCreate, ActionFileModify, ActionFileDelete:
		return a.Path
	case ActionProcessExec:
		return a.Command
	case ActionNetwork:
		return a.URL
	case ActionCredentialUse:
		return a.CredentialRef
	case ActionExternalState:
		return a.URL
	case ActionDelegation:
		return fmt.Sprintf("%s→%s", a.DelegationKind, a.DelegationTo)
	default:
		return a.Raw
	}
}

// DelegationsOf returns the mission/task pair a delegation action refers to.
// Kept tiny but named, so call sites read as intent rather than field access.
func (a Action) DelegationsOf() (AgentID, TaskID) { return a.DelegationTo, a.DelegationTask }

// SortedStringsCopy returns a sorted copy of s; a helper for deterministic
// digests of string sets (scope roots, tool lists) used by authorization
// binding. The input is not modified.
func SortedStringsCopy(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}
