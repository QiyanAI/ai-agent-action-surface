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
	// ClaimAuthorizationForAction atomically finds the unconsumed, unexpired
	// grant whose binding (task, agent, action digest, scope digest) matches
	// exactly, marks it consumed and returns the pre-claim snapshot for
	// Authorization.Check. ErrApprovalNotFound when nothing matches — the
	// caller then proceeds to normal gating, never to a silent allow.
	ClaimAuthorizationForAction(task domain.TaskID, agent domain.AgentID, actionDigest, scopeDigest string, now time.Time) (domain.Authorization, error)

	// AppendDecision records every decision (all outcomes) for audit.
	AppendDecision(d domain.Decision, action domain.Action) error

	// ConsumeStep atomically checks and consumes one step of the task's
	// budget: it fails with ErrBudgetExhausted when the task has reached
	// maxSteps, and with ErrNotFound when the task is unknown — the latter
	// only for stores backed by a task registry, which must fail closed.
	// Checking and consuming in one operation is what makes the budget hold
	// under concurrent evaluations.
	ConsumeStep(task domain.TaskID, maxSteps int) error

	// TransitionApproval atomically moves an approval from one state to
	// another and reports whether this caller performed the transition.
	// Decisions and cancellations race through here so a pending approval
	// can be decided exactly once.
	TransitionApproval(id domain.ApprovalID, from, to domain.ApprovalState, at time.Time, by domain.Actor) (domain.ApprovalRequest, bool, error)
}

// InMemoryStore is a Store for tests and for the engine's own unit runs. It is
// NOT the production store: nothing survives a restart, by design, so that
// nobody mistakes it for the durable one.
type InMemoryStore struct {
	mu          sync.Mutex
	approvals   map[domain.ApprovalID]domain.ApprovalRequest
	approvalSeq []domain.ApprovalID // insertion order, for LatestApprovalForAction
	auths       map[domain.ApprovalID]domain.Authorization
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
		auths:     map[domain.ApprovalID]domain.Authorization{},
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
	if _, exists := s.auths[a.ID]; exists {
		return domain.NewError(domain.ErrConflict, "authorization %s already exists", a.ID)
	}
	s.auths[a.ID] = a
	return nil
}

// ClaimAuthorizationForAction implements Store.
//
// The returned value is the pre-claim snapshot (ConsumedAt nil) so the caller
// can still run Authorization.Check against it; single-use is already
// guaranteed by this atomic transition. Failures return a zero value plus an
// error — never a zero-value authorization that looks valid.
func (s *InMemoryStore) ClaimAuthorizationForAction(task domain.TaskID, agent domain.AgentID, actionDigest, scopeDigest string, now time.Time) (domain.Authorization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, stored := range s.auths {
		if stored.TaskID != task || stored.AgentID != agent ||
			stored.ActionDigest != actionDigest || stored.ScopeDigest != scopeDigest {
			continue
		}
		if stored.ConsumedAt != nil || now.After(stored.ExpiresAt) {
			continue
		}
		snapshot := stored // pre-claim, ConsumedAt nil, for Check
		t := now
		stored.ConsumedAt = &t
		s.auths[id] = stored
		return snapshot, nil
	}
	return domain.Authorization{}, domain.NewError(domain.ErrApprovalNotFound, "no unconsumed authorization matches this action")
}

// AppendDecision implements Store.
func (s *InMemoryStore) AppendDecision(d domain.Decision, action domain.Action) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.decisions = append(s.decisions, storedDecision{Decision: d, Action: action})
	return nil
}

// ConsumeStep implements Store. The in-memory double tracks counts directly
// and does not require prior registration; registry-backed stores are the
// ones that must fail closed on unknown tasks.
func (s *InMemoryStore) ConsumeStep(task domain.TaskID, maxSteps int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.steps[task] >= maxSteps {
		return domain.NewError(domain.ErrBudgetExhausted, "step budget spent")
	}
	s.steps[task]++
	return nil
}

// TransitionApproval implements Store with compare-and-swap semantics.
func (s *InMemoryStore) TransitionApproval(id domain.ApprovalID, from, to domain.ApprovalState, at time.Time, by domain.Actor) (domain.ApprovalRequest, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.approvals[id]
	if !ok {
		return domain.ApprovalRequest{}, false, domain.NewError(domain.ErrApprovalNotFound, "approval %s not found", id)
	}
	if a.State != from {
		return a, false, nil
	}
	a.State = to
	t := at
	if to != domain.ApprovalPending {
		a.DecidedAt = &t
		a.DecidedBy = by
	}
	s.approvals[id] = a
	return a, true, nil
}

// Decisions returns a copy of the recorded decisions (test helper).
func (s *InMemoryStore) Decisions() []storedDecision {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]storedDecision(nil), s.decisions...)
}
