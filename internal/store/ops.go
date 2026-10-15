package store

import (
	"sort"
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
)

// This file implements control.Store on the durable store, plus the registry
// operations the control API needs. Every mutation funnels through
// appendLocked, so the wal is the single ordering of truth and a replay
// reconstructs exactly the same state.

// SessionBinding ties a runtime session to the task and agent it was launched
// for. The PreToolUse hook resolves tool calls to identities through this
// table — a session that was never bound cannot act.
type SessionBinding struct {
	SessionID string           `json:"sessionId"`
	Runtime   string           `json:"runtime"`
	MissionID domain.MissionID `json:"missionId"`
	TaskID    domain.TaskID    `json:"taskId"`
	AgentID   domain.AgentID   `json:"agentId"`
	BoundAt   time.Time        `json:"boundAt"`
}

// --- control.Store implementation ---

// CreateApproval implements control.Store.
func (s *Store) CreateApproval(a domain.ApprovalRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return storeBlockedErr(s.blocked)
	}
	if _, exists := s.approvals[a.ID]; exists {
		return domain.NewError(domain.ErrConflict, "approval %s already exists", a.ID)
	}
	return s.appendLocked(evApprovalPut, a, time.Now().UTC())
}

// GetApproval implements control.Store.
func (s *Store) GetApproval(id domain.ApprovalID) (domain.ApprovalRequest, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return domain.ApprovalRequest{}, false, storeBlockedErr(s.blocked)
	}
	a, ok := s.approvals[id]
	return a, ok, nil
}

// LatestApprovalForAction implements control.Store.
func (s *Store) LatestApprovalForAction(task domain.TaskID, agent domain.AgentID, digest string) (domain.ApprovalRequest, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return domain.ApprovalRequest{}, false, storeBlockedErr(s.blocked)
	}
	for i := len(s.approvalSeq) - 1; i >= 0; i-- {
		a := s.approvals[s.approvalSeq[i]]
		if a.TaskID == task && a.AgentID == agent && a.ActionDigest == digest {
			return a, true, nil
		}
	}
	return domain.ApprovalRequest{}, false, nil
}

// UpdateApproval implements control.Store.
func (s *Store) UpdateApproval(a domain.ApprovalRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return storeBlockedErr(s.blocked)
	}
	if _, exists := s.approvals[a.ID]; !exists {
		return domain.NewError(domain.ErrApprovalNotFound, "approval %s not found", a.ID)
	}
	return s.appendLocked(evApprovalPut, a, time.Now().UTC())
}

// CreateAuthorization implements control.Store.
func (s *Store) CreateAuthorization(a domain.Authorization) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return storeBlockedErr(s.blocked)
	}
	if _, exists := s.auths[a.ID]; exists {
		return domain.NewError(domain.ErrConflict, "authorization %s already exists", a.ID)
	}
	return s.appendLocked(evAuthCreate, a, time.Now().UTC())
}

// ClaimAuthorizationForAction implements control.Store. The claim is a single
// wal append: once the event is on disk the grant is spent, and no replay can
// re-claim it. The returned snapshot is pre-claim so Authorization.Check can
// still validate every binding against live values.
func (s *Store) ClaimAuthorizationForAction(task domain.TaskID, agent domain.AgentID, actionDigest, scopeDigest string, now time.Time) (domain.Authorization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return domain.Authorization{}, storeBlockedErr(s.blocked)
	}
	for _, stored := range s.auths {
		if stored.TaskID != task || stored.AgentID != agent ||
			stored.ActionDigest != actionDigest || stored.ScopeDigest != scopeDigest {
			continue
		}
		if stored.ConsumedAt != nil || now.After(stored.ExpiresAt) {
			continue
		}
		snapshot := stored
		claim := struct {
			AuthID domain.ApprovalID `json:"authId"`
			At     time.Time         `json:"at"`
		}{AuthID: stored.ID, At: now.UTC()}
		if err := s.appendLocked(evAuthClaim, claim, now); err != nil {
			return domain.Authorization{}, err
		}
		return snapshot, nil
	}
	return domain.Authorization{}, domain.NewError(domain.ErrApprovalNotFound, "no unconsumed authorization matches this action")
}

// AppendDecision implements control.Store.
func (s *Store) AppendDecision(d domain.Decision, action domain.Action) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return storeBlockedErr(s.blocked)
	}
	return s.appendLocked(evDecisionAppend, DecisionRecord{Decision: d, Action: action, At: time.Now().UTC()}, time.Now().UTC())
}

// IncrementSteps implements control.Store. A task that cannot be found is an
// error: the engine treats this failure as deny, so an unregistered task can
// never quietly consume "no budget".
func (s *Store) IncrementSteps(task domain.TaskID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return storeBlockedErr(s.blocked)
	}
	t, ok := s.tasks[task]
	if !ok {
		return domain.NewError(domain.ErrNotFound, "task %s is not registered", task)
	}
	t.StepsUsed++
	t.UpdatedAt = time.Now().UTC()
	return s.appendLocked(evTaskPut, t, time.Now().UTC())
}

// --- registries ---

// RegisterAgent records an agent and the hash of its bearer token. The token
// itself is never stored; only its SHA-256.
func (s *Store) RegisterAgent(a domain.Agent, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return storeBlockedErr(s.blocked)
	}
	if _, exists := s.agents[a.ID]; exists {
		return domain.NewError(domain.ErrConflict, "agent %s already exists", a.ID)
	}
	rec := agentRecord{Agent: a, TokenHash: tokenHash}
	return s.appendLocked(evAgentRegister, rec, time.Now().UTC())
}

// AgentByTokenHash resolves a presented bearer token's hash to its agent.
func (s *Store) AgentByTokenHash(hash string) (domain.Agent, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return domain.Agent{}, false, storeBlockedErr(s.blocked)
	}
	id, ok := s.tokens[hash]
	if !ok {
		return domain.Agent{}, false, nil
	}
	rec, ok := s.agents[id]
	return rec.Agent, ok, nil
}

// AgentByID returns a registered agent.
func (s *Store) AgentByID(id domain.AgentID) (domain.Agent, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return domain.Agent{}, false, storeBlockedErr(s.blocked)
	}
	rec, ok := s.agents[id]
	return rec.Agent, ok, nil
}

// ListAgents returns all registered agents, sorted by ID.
func (s *Store) ListAgents() ([]domain.Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return nil, storeBlockedErr(s.blocked)
	}
	out := make([]domain.Agent, 0, len(s.agents))
	for _, rec := range s.agents {
		out = append(out, rec.Agent)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// PutMission creates or updates a mission.
func (s *Store) PutMission(m domain.Mission) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return storeBlockedErr(s.blocked)
	}
	evType := evMissionCreate
	if _, exists := s.missions[m.ID]; exists {
		evType = evMissionState
	}
	m.UpdatedAt = time.Now().UTC()
	return s.appendLocked(evType, m, time.Now().UTC())
}

// GetMission returns a mission.
func (s *Store) GetMission(id domain.MissionID) (domain.Mission, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return domain.Mission{}, false, storeBlockedErr(s.blocked)
	}
	m, ok := s.missions[id]
	return m, ok, nil
}

// ListMissions returns all missions, newest first by ID ordering.
func (s *Store) ListMissions() ([]domain.Mission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return nil, storeBlockedErr(s.blocked)
	}
	out := make([]domain.Mission, 0, len(s.missions))
	for _, m := range s.missions {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

// PutTask creates or updates a task.
func (s *Store) PutTask(t domain.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return storeBlockedErr(s.blocked)
	}
	if _, exists := s.tasks[t.ID]; !exists {
		if _, ok := s.missions[t.MissionID]; !ok {
			return domain.NewError(domain.ErrNotFound, "mission %s is not registered", t.MissionID)
		}
	}
	t.UpdatedAt = time.Now().UTC()
	return s.appendLocked(evTaskPut, t, time.Now().UTC())
}

// GetTask returns a task.
func (s *Store) GetTask(id domain.TaskID) (domain.Task, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return domain.Task{}, false, storeBlockedErr(s.blocked)
	}
	t, ok := s.tasks[id]
	return t, ok, nil
}

// TasksForMission lists a mission's tasks in creation order.
func (s *Store) TasksForMission(m domain.MissionID) ([]domain.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return nil, storeBlockedErr(s.blocked)
	}
	var out []domain.Task
	for _, t := range s.tasks {
		if t.MissionID == m {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// BindSession records the session→(mission, task, agent) mapping the hook
// resolves against.
func (s *Store) BindSession(b SessionBinding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return storeBlockedErr(s.blocked)
	}
	if _, ok := s.tasks[b.TaskID]; !ok {
		return domain.NewError(domain.ErrNotFound, "task %s is not registered", b.TaskID)
	}
	if _, ok := s.agents[b.AgentID]; !ok {
		return domain.NewError(domain.ErrNotFound, "agent %s is not registered", b.AgentID)
	}
	return s.appendLocked(evSessionBind, b, time.Now().UTC())
}

// LookupSession resolves a runtime session ID.
func (s *Store) LookupSession(sessionID string) (SessionBinding, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return SessionBinding{}, false, storeBlockedErr(s.blocked)
	}
	b, ok := s.sessions[sessionID]
	return b, ok, nil
}

// ListApprovals returns approvals, newest first. state filters by
// ApprovalState when non-empty.
func (s *Store) ListApprovals(state domain.ApprovalState, agent domain.AgentID) ([]domain.ApprovalRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return nil, storeBlockedErr(s.blocked)
	}
	var out []domain.ApprovalRequest
	for i := len(s.approvalSeq) - 1; i >= 0; i-- {
		a := s.approvals[s.approvalSeq[i]]
		if state != "" && a.State != state {
			continue
		}
		if agent != "" && a.AgentID != agent {
			continue
		}
		out = append(out, a)
	}
	return out, nil
}

// PutCoverage records the verified state of one execution path.
func (s *Store) PutCoverage(c domain.Coverage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return storeBlockedErr(s.blocked)
	}
	return s.appendLocked(evCoveragePut, c, time.Now().UTC())
}

// ListCoverage returns coverage records, sorted by path.
func (s *Store) ListCoverage() ([]domain.Coverage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return nil, storeBlockedErr(s.blocked)
	}
	out := make([]domain.Coverage, 0, len(s.coverage))
	for _, c := range s.coverage {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// DecisionsForTask returns the audited decisions of one task, oldest first.
func (s *Store) DecisionsForTask(task domain.TaskID) ([]DecisionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return nil, storeBlockedErr(s.blocked)
	}
	var out []DecisionRecord
	for _, d := range s.decisions {
		if d.Decision.TaskID == task {
			out = append(out, d)
		}
	}
	return out, nil
}

// RecentDecisions returns the newest n decisions across all tasks.
func (s *Store) RecentDecisions(n int) ([]DecisionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return nil, storeBlockedErr(s.blocked)
	}
	if n <= 0 || n > len(s.decisions) {
		n = len(s.decisions)
	}
	out := append([]DecisionRecord(nil), s.decisions[len(s.decisions)-n:]...)
	return out, nil
}

func storeBlockedErr(reason error) error {
	return domain.NewError(domain.ErrStoreCorrupted, "state store is blocked: %v", reason)
}
