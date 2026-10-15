// Package control implements Qiyan's single Action Control Engine. Every
// interface — the Claude Code hook, the local UI, the Control API and the
// reference host — obtains decisions here and nowhere else, so the three
// entry points can never drift into different safety logics (book §07, §12).
package control

import (
	"sync"
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
)

// Store is what the engine persists through. It is deliberately small: the
// engine's guarantees (single-use authorizations, approval memory, audit
// trail) depend on these operations being atomic.
//
// The interface is defined here, on the consumer side, so tests can use the
// in-memory implementation below and the file-backed store (R02-03) can be
// added without touching the engine.
type Store interface {
	// CreateApproval records a new pending approval.
	CreateApproval(domain.ApprovalRequest) error
	// GetApproval returns one approval by ID.
	GetApproval(domain.ApprovalID) (domain.ApprovalRequest, bool, error)
	// LatestApprovalForAction finds the most recent approval for this exact
	// (task, agent, action digest) so the engine can dedupe pending requests
	// and remember a user's deny.
	LatestApprovalForAction(task domain.TaskID, agent domain.AgentID, actionDigest string) (domain.ApprovalRequest, bool, error)
	// UpdateApproval persists a state change (approved/denied/expired/cancelled).
	UpdateApproval(domain.ApprovalRequest) error

	// CreateAuthorization stores a grant produced by an approval.
	CreateAuthorization(domain.Authorization) error
	// ClaimAuthorization atomically returns the authorization for nonce and
	// marks it consumed. It fails with ErrAuthorizationReplayed if already
	// spent and ErrApprovalNotFound if unknown — never a silent zero value.
	ClaimAuthorization(nonce string, now time.Time) (domain.Authorization, error)

	// AppendDecision records every decision (all outcomes) for audit.
	AppendDecision(d domain.Decision, action domain.Action) error
	// IncrementSteps bumps the task's controlled-action counter.
	IncrementSteps(task domain.TaskID) error
}

// InMemoryStore is a Store for tests and for the engine's own unit runs. It is
// NOT the production store: nothing survives a restart, by design, so that
// nobody mistakes it for the durable one.
type InMemoryStore struct {
	mu          sync.Mutex
	approvals   map[domain.ApprovalID]domain.ApprovalRequest
	approvalSeq []domain.ApprovalID // insertion order, for LatestApprovalForAction
	auths       map[string]domain.Authorization
	decisions   []storedDecision
	steps       map[domain.TaskID]int
}

type storedDecision struct {
	Decision domain.Decision
	Action   domain.Action
}

// NewInMemoryStore returns an empty store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		approvals: map[domain.ApprovalID]domain.ApprovalRequest{},
		auths:     map[string]domain.Authorization{},
		steps:     map[domain.TaskID]int{},
	}
}

// CreateApproval implements Store.
func (s *InMemoryStore) CreateApproval(a domain.ApprovalRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.approvals[a.ID]; exists {
		return domain.NewError(domain.ErrConflict, "approval %s already exists", a.ID)
	}
	s.approvals[a.ID] = a
	s.approvalSeq = append(s.approvalSeq, a.ID)
	return nil
}

// GetApproval implements Store.
func (s *InMemoryStore) GetApproval(id domain.ApprovalID) (domain.ApprovalRequest, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.approvals[id]
	return a, ok, nil
}

// LatestApprovalForAction implements Store.
func (s *InMemoryStore) LatestApprovalForAction(task domain.TaskID, agent domain.AgentID, digest string) (domain.ApprovalRequest, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.approvalSeq) - 1; i >= 0; i-- {
		a := s.approvals[s.approvalSeq[i]]
		if a.TaskID == task && a.AgentID == agent && a.ActionDigest == digest {
			return a, true, nil
		}
	}
	return domain.ApprovalRequest{}, false, nil
}

// UpdateApproval implements Store.
func (s *InMemoryStore) UpdateApproval(a domain.ApprovalRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.approvals[a.ID]; !exists {
		return domain.NewError(domain.ErrApprovalNotFound, "approval %s not found", a.ID)
	}
	s.approvals[a.ID] = a
	return nil
}

// CreateAuthorization implements Store.
func (s *InMemoryStore) CreateAuthorization(a domain.Authorization) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.auths[a.Nonce]; exists {
		return domain.NewError(domain.ErrConflict, "authorization nonce already exists")
	}
	s.auths[a.Nonce] = a
	return nil
}

// ClaimAuthorization implements Store.
//
// The returned value is the pre-claim snapshot (ConsumedAt nil) so the caller
// can still run Authorization.Check against it; single-use is already
// guaranteed by this atomic transition. Failures return zero value plus an
// error — never a zero-value authorization that looks valid.
func (s *InMemoryStore) ClaimAuthorization(nonce string, now time.Time) (domain.Authorization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.auths[nonce]
	if !ok {
		return domain.Authorization{}, domain.NewError(domain.ErrApprovalNotFound, "no authorization for this token")
	}
	if stored.ConsumedAt != nil {
		return domain.Authorization{}, domain.NewError(domain.ErrAuthorizationReplayed, "authorization already used at %s", stored.ConsumedAt.Format(time.RFC3339))
	}
	claimed := stored
	t := now
	stored.ConsumedAt = &t
	s.auths[nonce] = stored
	return claimed, nil
}

// AppendDecision implements Store.
func (s *InMemoryStore) AppendDecision(d domain.Decision, action domain.Action) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.decisions = append(s.decisions, storedDecision{Decision: d, Action: action})
	return nil
}

// IncrementSteps implements Store.
func (s *InMemoryStore) IncrementSteps(task domain.TaskID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steps[task]++
	return nil
}

// Decisions returns a copy of the recorded decisions (test helper).
func (s *InMemoryStore) Decisions() []storedDecision {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]storedDecision(nil), s.decisions...)
}
