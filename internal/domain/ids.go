// Package domain holds Qiyan's core domain model: stable identities, states,
// actions, capabilities, policy shapes, authorization binding, evidence levels
// and coverage states.
//
// The package is pure: no I/O, no network, no clock reads except where a caller
// passes a time in. Everything the control engine (internal/control) decides
// with is defined here, and everything the API, connector, hook and reference
// host exchange is expressed through these types. Keeping the model in one
// place is what prevents the UI, the connector and third-party hosts from each
// growing their own, subtly different safety logic.
package domain

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/binary"
	"strings"
	"time"
)

// Identifier types. Each is a distinct string type so that a task ID cannot be
// passed where an agent ID is expected.
type (
	AgentID       string
	MissionID     string
	TaskID        string
	ActionID      string
	ApprovalID    string
	EvidenceID    string
	DelegationID  string
	CoverageID    string
	SessionID     string // an external runtime's session identifier (e.g. Claude Code)
	CorrelationID string // groups the events of one in-flight decision
)

// idEncoding is lowercase base32 with no padding: readable, path-safe, and
// case-insensitive so it survives Windows filesystems and URLs unchanged.
var idEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// newSortableID returns prefix followed by a lexicographically sortable,
// collision-resistant identifier.
//
// Layout: 8 bytes of Unix milliseconds (big-endian) then 10 random bytes.
// IDs therefore sort in creation order, which lets the event store use them as
// stable, replay-detectable ordinals, while the random tail keeps them
// unguessable — approvals and authorizations must not be enumerable, and
// crypto/rand is the only randomness source this package uses.
func newSortableID(prefix string) string {
	var raw [18]byte
	ms := time.Now().UnixMilli()
	binary.BigEndian.PutUint64(raw[:8], uint64(ms))
	if _, err := rand.Read(raw[8:]); err != nil {
		// crypto/rand failing is unrecoverable for a security tool: an ID is
		// about to name an authorization. Panicking is deliberate — callers
		// must not continue with predictable identifiers.
		panic("domain: crypto/rand unavailable: " + err.Error())
	}
	return prefix + "_" + strings.ToLower(idEncoding.EncodeToString(raw[:]))
}

// NewAgentID and friends mint identifiers for each domain entity.
func NewAgentID() AgentID             { return AgentID(newSortableID("ag")) }
func NewMissionID() MissionID         { return MissionID(newSortableID("mi")) }
func NewTaskID() TaskID               { return TaskID(newSortableID("ta")) }
func NewActionID() ActionID           { return ActionID(newSortableID("ac")) }
func NewApprovalID() ApprovalID       { return ApprovalID(newSortableID("ap")) }
func NewEvidenceID() EvidenceID       { return EvidenceID(newSortableID("ev")) }
func NewDelegationID() DelegationID   { return DelegationID(newSortableID("dl")) }
func NewCoverageID() CoverageID       { return CoverageID(newSortableID("cv")) }
func NewCorrelationID() CorrelationID { return CorrelationID(newSortableID("co")) }
