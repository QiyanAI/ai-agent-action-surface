// Package store implements Qiyan's durable local state: a single-writer,
// append-only event log with periodic snapshots holding agents, missions,
// tasks, session bindings, approvals, authorizations, decisions and coverage.
//
// One writer by construction: the `qiyan serve` process owns the state
// directory. The PreToolUse hook is a control-API client, not a second writer —
// so the store needs no cross-process locking, and the hook's failure mode is
// simply "cannot reach control → deny", which is the semantics the book
// requires anyway (book §09).
//
// Durability model:
//   - every mutation appends one JSON line to wal.jsonl, fsynced before the
//     mutation is reported successful;
//   - snapshots.json is rewritten atomically (temp file, fsync, rename) and
//     carries the last applied sequence number;
//   - on load, the snapshot is applied, then wal lines with higher sequence
//     numbers are replayed. A line that cannot be parsed stops the replay and
//     the store opens in a BLOCKED state: it serves reads as errors and
//     refuses writes, because resuming from state we cannot vouch for is
//     exactly the "missing authorization must not be restored as approved"
//     failure the book names (book §18.1).
package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
)

// Store is the durable state. It implements control.Store and adds the
// registries the control API serves (agents with token hashes, missions and
// tasks, session bindings, coverage records).
type Store struct {
	dir string

	mu      sync.Mutex
	closed  bool
	blocked error // non-nil once the on-disk state proved unreadable
	lastSeq uint64
	walFile *os.File

	agents        map[domain.AgentID]agentRecord
	tokens        map[string]domain.AgentID // sha256(token) -> agent
	missions      map[domain.MissionID]domain.Mission
	tasks         map[domain.TaskID]domain.Task
	sessions      map[string]SessionBinding // runtime session id -> agent/task binding
	approvals     map[domain.ApprovalID]domain.ApprovalRequest
	approvalSeq   []domain.ApprovalID
	auths         map[domain.ApprovalID]domain.Authorization
	decisions     []DecisionRecord
	results       []ResultEvent
	coverage      map[string]domain.Coverage
	installations map[string]Installation
}

// Installation records one runtime integration Qiyan has installed: where the
// settings live, how to undo it, and which verified version/tier it was
// installed under.
type Installation struct {
	ProjectDir    string    `json:"projectDir"`
	SettingsPath  string    `json:"settingsPath"`
	BackupPath    string    `json:"backupPath,omitempty"`
	HookCommand   string    `json:"hookCommand"`
	ClaudeBinary  string    `json:"claudeBinary"`
	Version       string    `json:"version"`
	Tier          string    `json:"tier"`
	ProbeEvidence []string  `json:"probeEvidence,omitempty"`
	InstalledAt   time.Time `json:"installedAt"`
}

// ResultEvent is one execution result reported back by an agent: what the
// runtime actually did after a decision (book §14, "操作结果回传").
type ResultEvent struct {
	TaskID       domain.TaskID  `json:"taskId"`
	AgentID      domain.AgentID `json:"agentId"`
	ActionDigest string         `json:"actionDigest"`
	Result       string         `json:"result"` // executed | failed | blocked | cancelled
	Note         string         `json:"note"`
	At           time.Time      `json:"at"`
}

// DecisionRecord is one audited engine decision.
type DecisionRecord struct {
	Decision domain.Decision `json:"decision"`
	Action   domain.Action   `json:"action"`
	At       time.Time       `json:"at"`
}

// agentRecord pairs a registered agent with its credential hash.
type agentRecord struct {
	Agent     domain.Agent
	TokenHash string // sha256 hex of the bearer token; the token itself is never stored
}

// walEvent is one mutation. Payload types are keyed by Type; replay applies
// them in sequence order and is idempotent because every apply sets state
// rather than incrementing.
type walEvent struct {
	Seq     uint64          `json:"seq"`
	Type    string          `json:"type"`
	At      time.Time       `json:"at"`
	Payload json.RawMessage `json:"payload"`
}

// Event type names.
const (
	evAgentRegister  = "agent.register"
	evMissionCreate  = "mission.create"
	evMissionState   = "mission.state"
	evTaskPut        = "task.put"
	evSessionBind    = "session.bind"
	evApprovalPut    = "approval.put"
	evAuthCreate     = "authorization.create"
	evAuthClaim      = "authorization.claim"
	evDecisionAppend = "decision.append"
	evResultAppend   = "result.append"
	evCoveragePut    = "coverage.put"
	evInstallPut     = "installation.put"
)

// snapshot is the compacted state.
type snapshot struct {
	LastSeq       uint64                                       `json:"lastSeq"`
	Agents        map[domain.AgentID]agentRecord               `json:"agents"`
	Missions      map[domain.MissionID]domain.Mission          `json:"missions"`
	Tasks         map[domain.TaskID]domain.Task                `json:"tasks"`
	Sessions      map[string]SessionBinding                    `json:"sessions"`
	Approvals     map[domain.ApprovalID]domain.ApprovalRequest `json:"approvals"`
	ApprovalSeq   []domain.ApprovalID                          `json:"approvalSeq"`
	Auths         map[domain.ApprovalID]domain.Authorization   `json:"authorizations"`
	Decisions     []DecisionRecord                             `json:"decisions"`
	Results       []ResultEvent                                `json:"results"`
	Coverage      map[string]domain.Coverage                   `json:"coverage"`
	Installations map[string]Installation                      `json:"installations"`
}

// Open loads (or creates) the store rooted at dir. It only reports an error
// for environment problems (unwritable directory); state corruption is
// reported through BlockedReason and by every subsequent operation failing
// closed, because a hook must always be able to say "denied — control
// unavailable" rather than crash into fail-open.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("store: state directory: %w", err)
	}
	s := &Store{
		dir:           dir,
		agents:        map[domain.AgentID]agentRecord{},
		tokens:        map[string]domain.AgentID{},
		missions:      map[domain.MissionID]domain.Mission{},
		tasks:         map[domain.TaskID]domain.Task{},
		sessions:      map[string]SessionBinding{},
		approvals:     map[domain.ApprovalID]domain.ApprovalRequest{},
		auths:         map[domain.ApprovalID]domain.Authorization{},
		coverage:      map[string]domain.Coverage{},
		installations: map[string]Installation{},
	}
	if err := s.load(); err != nil {
		s.blocked = err
	}
	return s, nil
}

// BlockedReason returns non-nil when the store is serving in blocked mode.
func (s *Store) BlockedReason() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.blocked
}

func (s *Store) load() error {
	snapPath := filepath.Join(s.dir, "snapshots.json")
	raw, err := os.ReadFile(snapPath)
	if err == nil {
		var snap snapshot
		if err := json.Unmarshal(raw, &snap); err != nil {
			return fmt.Errorf("store: snapshot unreadable: %w", err)
		}
		s.applySnapshot(snap)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("store: snapshot: %w", err)
	}

	walPath := filepath.Join(s.dir, "wal.jsonl")
	f, err := os.Open(walPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("store: wal: %w", err)
		}
	} else {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		lineNo := 0
		for sc.Scan() {
			lineNo++
			line := sc.Bytes()
			if len(line) == 0 {
				continue
			}
			var ev walEvent
			if err := json.Unmarshal(line, &ev); err != nil {
				f.Close()
				return fmt.Errorf("store: wal line %d unreadable: %w", lineNo, err)
			}
			if ev.Seq <= s.lastSeq {
				continue // already in the snapshot; replay is idempotent
			}
			if ev.Seq != s.lastSeq+1 {
				f.Close()
				return fmt.Errorf("store: wal sequence gap at line %d (have %d, saw %d)", lineNo, s.lastSeq, ev.Seq)
			}
			if err := s.apply(ev); err != nil {
				f.Close()
				return fmt.Errorf("store: wal line %d: %w", lineNo, err)
			}
		}
		if err := sc.Err(); err != nil {
			f.Close()
			return fmt.Errorf("store: wal read: %w", err)
		}
		f.Close()
	}

	walFile, err := os.OpenFile(walPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("store: wal open for append: %w", err)
	}
	s.walFile = walFile
	return nil
}

// Close releases the wal handle. Idempotent.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.walFile == nil {
		s.closed = true
		return nil
	}
	s.closed = true
	return s.walFile.Close()
}

// appendLocked writes and fsyncs one event. The caller holds s.mu.
func (s *Store) appendLocked(evType string, payload any, at time.Time) error {
	if s.blocked != nil {
		return domain.NewError(domain.ErrStoreCorrupted, "state store is blocked: %v", s.blocked)
	}
	if s.walFile == nil {
		return domain.NewError(domain.ErrStoreCorrupted, "state store is not writable")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return domain.NewError(domain.ErrInternal, "event payload: %v", err)
	}
	ev := walEvent{Seq: s.lastSeq + 1, Type: evType, At: at.UTC(), Payload: raw}
	line, err := json.Marshal(ev)
	if err != nil {
		return domain.NewError(domain.ErrInternal, "event encode: %v", err)
	}
	line = append(line, '\n')
	if _, err := s.walFile.Write(line); err != nil {
		// A failed write may have written a partial line; the next append
		// would corrupt the log. Stop accepting writes entirely.
		s.blocked = fmt.Errorf("wal write failed: %w", err)
		return domain.NewError(domain.ErrStoreCorrupted, "state store write failed: %v", err)
	}
	if err := s.walFile.Sync(); err != nil {
		s.blocked = fmt.Errorf("wal fsync failed: %w", err)
		return domain.NewError(domain.ErrStoreCorrupted, "state store sync failed: %v", err)
	}
	if err := s.apply(ev); err != nil {
		// The event is on disk; in-memory divergence means we cannot trust
		// ourselves. Block.
		s.blocked = fmt.Errorf("apply after write failed: %w", err)
		return domain.NewError(domain.ErrStoreCorrupted, "state store apply failed: %v", err)
	}
	return nil
}

// apply mutates state from one event; idempotent for replay.
func (s *Store) apply(ev walEvent) error {
	switch ev.Type {
	case evAgentRegister:
		var rec agentRecord
		if err := json.Unmarshal(ev.Payload, &rec); err != nil {
			return err
		}
		s.agents[rec.Agent.ID] = rec
		if rec.TokenHash != "" {
			s.tokens[rec.TokenHash] = rec.Agent.ID
		}
	case evMissionCreate, evMissionState:
		var m domain.Mission
		if err := json.Unmarshal(ev.Payload, &m); err != nil {
			return err
		}
		s.missions[m.ID] = m
	case evTaskPut:
		var t domain.Task
		if err := json.Unmarshal(ev.Payload, &t); err != nil {
			return err
		}
		s.tasks[t.ID] = t
	case evSessionBind:
		var b SessionBinding
		if err := json.Unmarshal(ev.Payload, &b); err != nil {
			return err
		}
		s.sessions[b.SessionID] = b
	case evApprovalPut:
		var a domain.ApprovalRequest
		if err := json.Unmarshal(ev.Payload, &a); err != nil {
			return err
		}
		if _, exists := s.approvals[a.ID]; !exists {
			s.approvalSeq = append(s.approvalSeq, a.ID)
		}
		s.approvals[a.ID] = a
	case evAuthCreate:
		var a domain.Authorization
		if err := json.Unmarshal(ev.Payload, &a); err != nil {
			return err
		}
		s.auths[a.ID] = a
	case evAuthClaim:
		var rec struct {
			AuthID domain.ApprovalID `json:"authId"`
			At     time.Time         `json:"at"`
		}
		if err := json.Unmarshal(ev.Payload, &rec); err != nil {
			return err
		}
		if a, ok := s.auths[rec.AuthID]; ok && a.ConsumedAt == nil {
			t := rec.At
			a.ConsumedAt = &t
			s.auths[rec.AuthID] = a
		}
	case evDecisionAppend:
		var rec DecisionRecord
		if err := json.Unmarshal(ev.Payload, &rec); err != nil {
			return err
		}
		s.decisions = append(s.decisions, rec)
	case evResultAppend:
		var rec ResultEvent
		if err := json.Unmarshal(ev.Payload, &rec); err != nil {
			return err
		}
		s.results = append(s.results, rec)
	case evCoveragePut:
		var c domain.Coverage
		if err := json.Unmarshal(ev.Payload, &c); err != nil {
			return err
		}
		s.coverage[c.Path] = c
	case evInstallPut:
		// One event type carries both upserts and tombstones; replay applies
		// whichever it finds, so removal survives a restart too.
		var rec struct {
			Installation
			Deleted bool `json:"deleted"`
		}
		if err := json.Unmarshal(ev.Payload, &rec); err != nil {
			return err
		}
		if rec.ProjectDir == "" {
			return nil
		}
		if rec.Deleted {
			delete(s.installations, rec.ProjectDir)
		} else {
			s.installations[rec.ProjectDir] = rec.Installation
		}
	default:
		return fmt.Errorf("unknown event type %q", ev.Type)
	}
	s.lastSeq = ev.Seq
	return nil
}

func (s *Store) applySnapshot(snap snapshot) {
	if snap.Agents != nil {
		s.agents = snap.Agents
		for _, rec := range snap.Agents {
			if rec.TokenHash != "" {
				s.tokens[rec.TokenHash] = rec.Agent.ID
			}
		}
	}
	if snap.Missions != nil {
		s.missions = snap.Missions
	}
	if snap.Tasks != nil {
		s.tasks = snap.Tasks
	}
	if snap.Sessions != nil {
		s.sessions = snap.Sessions
	}
	if snap.Approvals != nil {
		s.approvals = snap.Approvals
	}
	s.approvalSeq = snap.ApprovalSeq
	if snap.Auths != nil {
		s.auths = snap.Auths
	}
	s.decisions = snap.Decisions
	s.results = snap.Results
	if snap.Coverage != nil {
		s.coverage = snap.Coverage
	}
	if snap.Installations != nil {
		s.installations = snap.Installations
	}
	s.lastSeq = snap.LastSeq
}

// Compact writes a snapshot atomically and truncates the wal. It is safe to
// crash at any point: a snapshot with a higher lastSeq simply subsumes the
// wal lines that were not truncated.
func (s *Store) Compact() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.blocked != nil {
		return domain.NewError(domain.ErrStoreCorrupted, "state store is blocked: %v", s.blocked)
	}
	snap := snapshot{
		LastSeq:       s.lastSeq,
		Agents:        s.agents,
		Missions:      s.missions,
		Tasks:         s.tasks,
		Sessions:      s.sessions,
		Approvals:     s.approvals,
		ApprovalSeq:   s.approvalSeq,
		Auths:         s.auths,
		Decisions:     s.decisions,
		Results:       s.results,
		Coverage:      s.coverage,
		Installations: s.installations,
	}
	raw, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return domain.NewError(domain.ErrInternal, "snapshot encode: %v", err)
	}
	tmp := filepath.Join(s.dir, "snapshots.json.tmp")
	final := filepath.Join(s.dir, "snapshots.json")
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return domain.NewError(domain.ErrStoreCorrupted, "snapshot create: %v", err)
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return domain.NewError(domain.ErrStoreCorrupted, "snapshot write: %v", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return domain.NewError(domain.ErrStoreCorrupted, "snapshot sync: %v", err)
	}
	if err := f.Close(); err != nil {
		return domain.NewError(domain.ErrStoreCorrupted, "snapshot close: %v", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		return domain.NewError(domain.ErrStoreCorrupted, "snapshot rename: %v", err)
	}
	// Truncate the wal only after the snapshot is in place. On Windows an
	// O_APPEND handle is opened with append-data rights and cannot truncate,
	// so the handle is closed, the file recreated empty, and a fresh append
	// handle opened. A crash anywhere in here is safe: the snapshot carries
	// lastSeq, so any surviving wal lines simply replay as already-applied.
	walPath := filepath.Join(s.dir, "wal.jsonl")
	if s.walFile != nil {
		if err := s.walFile.Close(); err != nil {
			return domain.NewError(domain.ErrStoreCorrupted, "wal close for compaction: %v", err)
		}
		s.walFile = nil
	}
	empty, err := os.OpenFile(walPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return domain.NewError(domain.ErrStoreCorrupted, "wal truncate: %v", err)
	}
	if err := empty.Close(); err != nil {
		return domain.NewError(domain.ErrStoreCorrupted, "wal truncate close: %v", err)
	}
	s.walFile, err = os.OpenFile(walPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return domain.NewError(domain.ErrStoreCorrupted, "wal reopen after compaction: %v", err)
	}
	return nil
}
