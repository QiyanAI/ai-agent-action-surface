package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/connector"
	"github.com/QiyanAI/ai-agent-action-surface/internal/control"
	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
	"github.com/QiyanAI/ai-agent-action-surface/internal/mission"
	"github.com/QiyanAI/ai-agent-action-surface/internal/store"
)

const uiCookieName = "qiyan_ui"

// maxBodyBytes caps every request body. The API is local and low-volume; the
// cap is a blast-radius limit, not a tuning knob.
const maxBodyBytes = 1 << 20 // 1 MiB

// Server is the local control API. It is served only on loopback by the
// caller (cmd/qiyan), and independently re-checks the Host header so a DNS
// rebinding attempt that reaches the socket still fails here.
type Server struct {
	Store   *store.Store
	Engine  *control.Engine
	Version string

	// OperatorToken authorizes local-user operations (approvals, missions).
	// HookToken authorizes only hook decision traffic. Both are minted at
	// start and delivered to their holders through 0600 files in the state
	// directory — file permissions are the local-user trust anchor.
	OperatorToken string
	HookToken     string

	// StateDir is where the installed integration points its hook. The
	// server owns it so connect plans always name the same directory the
	// running service writes to.
	StateDir string

	// probe runs the real enforcement verification; overridable in tests.
	probe ProbeFunc

	// runs tracks live and finished mission runs (three-role fleet).
	runs *mission.RunRegistry

	sessions *sessionStore
	started  time.Time
	mux      *http.ServeMux
}

// ProbeFunc matches connector.ProbeReal; tests substitute a stub because a
// real probe launches live runtime sessions.
type ProbeFunc func(ctx context.Context, claudeBinary, qiyanBinary, workRoot string) (connector.ProbeResult, error)

// NewServer wires a server around a store and engine. stateDir names the
// directory installed hooks point at (the directory the store already uses).
func NewServer(st *store.Store, engine *control.Engine, version, operatorToken, hookToken, stateDir string) *Server {
	s := &Server{
		Store: st, Engine: engine, Version: version,
		OperatorToken: operatorToken, HookToken: hookToken,
		StateDir: stateDir,
		probe:    connector.ProbeReal,
		runs:     mission.NewRunRegistry(),
		sessions: newSessionStore(),
		started:  time.Now(),
		mux:      http.NewServeMux(),
	}
	s.routes()
	return s
}

// SetProbe replaces the probe implementation (tests only).
func (s *Server) SetProbe(p ProbeFunc) { s.probe = p }

// Handler returns the fully wrapped HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.middleware(s.mux)
}

func (s *Server) routes() {
	// Health (no auth; no data leaves it).
	s.mux.HandleFunc("GET /v1/health", s.handleHealth)

	// Agent management (operator).
	s.mux.HandleFunc("POST /v1/agents", s.handleAgentCreate)
	s.mux.HandleFunc("GET /v1/agents", s.handleAgentList)

	// Missions and tasks (operator; agents may read their own).
	s.mux.HandleFunc("POST /v1/missions", s.handleMissionCreate)
	s.mux.HandleFunc("GET /v1/missions", s.handleMissionList)
	s.mux.HandleFunc("GET /v1/missions/{id}", s.handleMissionGet)
	s.mux.HandleFunc("POST /v1/missions/{id}/tasks", s.handleTaskCreate)
	s.mux.HandleFunc("GET /v1/tasks/{id}", s.handleTaskGet)
	s.mux.HandleFunc("POST /v1/tasks/{id}/cancel", s.handleTaskCancel)
	s.mux.HandleFunc("POST /v1/sessions", s.handleSessionBind)

	// Decision paths.
	s.mux.HandleFunc("POST /v1/hook/decide", s.handleHookDecide) // hook token
	s.mux.HandleFunc("POST /v1/authorize", s.handleAuthorize)    // agent token

	// Approvals.
	s.mux.HandleFunc("GET /v1/approvals", s.handleApprovalList)
	s.mux.HandleFunc("GET /v1/approvals/{id}", s.handleApprovalGet)
	s.mux.HandleFunc("POST /v1/approvals/{id}/decision", s.handleApprovalDecision)

	// Result reporting and evidence queries.
	s.mux.HandleFunc("POST /v1/events", s.handleResultEvent)
	s.mux.HandleFunc("GET /v1/decisions", s.handleDecisions)
	s.mux.HandleFunc("GET /v1/coverage", s.handleCoverage)

	// Playground (synthetic scenarios).
	s.mux.HandleFunc("GET /v1/playground/scenarios", s.handlePlaygroundScenarios)

	// Safety Lab.
	s.mux.HandleFunc("POST /v1/lab/scan", s.handleLabScan)
	s.mux.HandleFunc("POST /v1/lab/apply", s.handleLabApply)
	s.mux.HandleFunc("POST /v1/lab/undo", s.handleLabUndo)

	// Mission runs (three-role fleet).
	s.mux.HandleFunc("POST /v1/missions/{id}/run", s.handleMissionRun)
	s.mux.HandleFunc("GET /v1/missions/{id}/run", s.handleMissionRunStatus)
	s.mux.HandleFunc("POST /v1/missions/{id}/run/cancel", s.handleMissionRunCancel)

	// Claude Code integration (connect wizard).
	s.mux.HandleFunc("POST /v1/connect/preview", s.handleConnectPreview)
	s.mux.HandleFunc("POST /v1/connect/apply", s.handleConnectApply)
	s.mux.HandleFunc("GET /v1/connect", s.handleConnectList)
	s.mux.HandleFunc("DELETE /v1/connect", s.handleConnectRemove)
	s.mux.HandleFunc("GET /v1/detect/claude", s.handleDetectClaude)

	// Project selection (folder browsing and one-folder creation for the
	// picker; directories only, state dir refused — see fs.go).
	s.mux.HandleFunc("GET /v1/fs/list", s.handleFSList)
	s.mux.HandleFunc("POST /v1/fs/mkdir", s.handleFSMkdir)

	// UI session bootstrap and the embedded console pages.
	s.mux.HandleFunc("POST /v1/ui/session", s.handleUISession)
	s.uiRoutes()
}

// extendWriteDeadline raises the response write deadline for handlers whose
// work legitimately outlives the server's WriteTimeout: the enforcement probe
// and the lab verify run real Claude Code sessions (bounded by their own
// context timeouts, up to ~5 minutes). Without this, the transport would cut
// the response while the probe is still working. Requires net/http's
// ResponseController support (plain loopback HTTP/1.1 always provides it).
func extendWriteDeadline(w http.ResponseWriter, d time.Duration) {
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Now().Add(d))
}

// --- middleware ---

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Host header must name this machine's loopback.
		if !loopbackHost(r.Host) {
			writeError(w, http.StatusForbidden, domain.ErrInvalidRequest, "requests must target loopback")
			return
		}
		// Browsers attach Origin to cross-origin requests; anything that is
		// not the loopback origin is refused outright, and never combined
		// with cookie credentials.
		if origin := r.Header.Get("Origin"); origin != "" && !loopbackOrigin(origin) {
			writeError(w, http.StatusForbidden, domain.ErrInvalidRequest, "cross-origin requests are not accepted")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

// loopbackHost accepts Host values that name this machine only: "localhost",
// "127.0.0.1", "[::1]", each with an optional port.
func loopbackHost(host string) bool {
	if host == "" {
		return false
	}
	h := host
	if i := strings.LastIndex(h, ":"); i >= 0 && !strings.HasSuffix(h, "]") {
		h = h[:i]
	}
	h = strings.ToLower(strings.Trim(h, "[]"))
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

func loopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return loopbackHost(u.Host)
}

// --- helpers ---

type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Class   string `json:"class"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, class domain.ErrorClass, format string, args ...any) {
	writeJSON(w, status, errorBody{Error: errorDetail{Class: string(class), Message: fmt.Sprintf(format, args...)}})
}

// statusFor maps stable error classes to HTTP statuses.
func statusFor(class domain.ErrorClass) int {
	switch class {
	case domain.ErrInvalidRequest:
		return http.StatusBadRequest
	case domain.ErrUnknownIdentity:
		return http.StatusUnauthorized
	case domain.ErrIdentityConflict, domain.ErrAuthorizationBinding, domain.ErrAuthorizationReplayed:
		return http.StatusForbidden
	case domain.ErrNotFound, domain.ErrApprovalNotFound:
		return http.StatusNotFound
	case domain.ErrConflict, domain.ErrApprovalExpired, domain.ErrApprovalCancelled:
		return http.StatusConflict
	case domain.ErrStoreCorrupted, domain.ErrControlUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

// fail writes the error using its class-appropriate status.
func fail(w http.ResponseWriter, err error) {
	class := domain.ClassOf(err)
	writeError(w, statusFor(class), class, "%s", err.Error())
}

// readJSON decodes a body strictly: unknown fields are rejected so a client
// typo cannot silently drop a security-relevant field (e.g. misspelling
// "path"). Empty bodies are allowed only when dst is nil-checkable by caller.
func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return domain.NewError(domain.ErrInvalidRequest, "request body is required")
		}
		return domain.NewError(domain.ErrInvalidRequest, "invalid JSON body: %v", err)
	}
	return nil
}

// requireAuth enforces one of the given credential kinds.
func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request, kinds ...authKind) (authInfo, bool) {
	info := s.resolveAuth(r)
	for _, k := range kinds {
		if info.Kind == k {
			// Cookie-authenticated mutations additionally require the CSRF
			// header; bearer credentials are immune to CSRF by construction.
			if k == authUISession && r.Method != http.MethodGet {
				if info.Session == nil || r.Header.Get("X-Qiyan-CSRF") != info.Session.CSRF {
					writeError(w, http.StatusForbidden, domain.ErrUnknownIdentity, "missing or invalid CSRF token")
					return authInfo{}, false
				}
			}
			return info, true
		}
	}
	writeError(w, http.StatusUnauthorized, domain.ErrUnknownIdentity, "authentication required")
	return authInfo{}, false
}

// --- handlers ---

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	blocked := s.Store.BlockedReason() != nil
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        map[bool]string{true: "degraded", false: "ok"}[blocked],
		"version":       s.Version,
		"storeBlocked":  blocked,
		"uptimeSeconds": int(time.Since(s.started).Seconds()),
	})
}

type agentCreateReq struct {
	Name    string `json:"name"`
	Role    string `json:"role"`
	Runtime string `json:"runtime"`
}

func (s *Server) handleAgentCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	var req agentCreateReq
	if err := readJSON(r, &req); err != nil {
		fail(w, err)
		return
	}
	role := domain.AgentRole(req.Role)
	if req.Name == "" || !domain.KnownRole(role) {
		writeError(w, http.StatusBadRequest, domain.ErrInvalidRequest, "name and a known role are required")
		return
	}
	ag := domain.Agent{
		ID: domain.NewAgentID(), Name: req.Name, Role: role,
		Runtime: req.Runtime, ReadOnly: role.DefaultReadOnly(),
		RegisteredAt: time.Now().UTC(),
	}
	token := NewToken()
	if err := s.Store.RegisterAgent(ag, hashToken(token)); err != nil {
		fail(w, err)
		return
	}
	// The token is shown exactly once; only its hash is stored.
	writeJSON(w, http.StatusCreated, map[string]any{"agent": agentPayload(ag), "token": token})
}

func (s *Server) handleAgentList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	agents, err := s.Store.ListAgents()
	if err != nil {
		fail(w, err)
		return
	}
	out := make([]AgentPayload, 0, len(agents))
	for _, a := range agents {
		out = append(out, agentPayload(a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": out})
}

type scopeReq struct {
	Roots          []string `json:"roots"`
	AllowedKinds   []string `json:"allowedKinds"`
	AllowedTools   []string `json:"allowedTools"`
	MaxSteps       int      `json:"maxSteps"`
	MaxRuntimeSecs int      `json:"maxRuntimeSeconds"`
}

type missionCreateReq struct {
	Goal  string   `json:"goal"`
	Mode  string   `json:"mode"`
	Scope scopeReq `json:"scope"`
}

func (s *Server) handleMissionCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	var req missionCreateReq
	if err := readJSON(r, &req); err != nil {
		fail(w, err)
		return
	}
	scope := domain.Scope{MaxSteps: req.Scope.MaxSteps, MaxRuntime: time.Duration(req.Scope.MaxRuntimeSecs) * time.Second}
	for _, root := range req.Scope.Roots {
		scope.Roots = append(scope.Roots, domain.NormalizePath(root, ""))
	}
	for _, k := range req.Scope.AllowedKinds {
		scope.AllowedKinds = append(scope.AllowedKinds, domain.ActionKind(k))
	}
	scope.AllowedTools = append(scope.AllowedTools, req.Scope.AllowedTools...)
	policy := domain.Policy{Mode: domain.AutonomyMode(req.Mode), Scope: scope}
	if err := policy.Validate(); err != nil {
		fail(w, err)
		return
	}
	m := domain.Mission{
		ID: domain.NewMissionID(), Goal: req.Goal, Policy: policy,
		State: domain.MissionRunning, CreatedAt: time.Now().UTC(),
	}
	if err := s.Store.PutMission(m); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, missionPayload(m))
}

func (s *Server) handleMissionList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	missions, err := s.Store.ListMissions()
	if err != nil {
		fail(w, err)
		return
	}
	out := make([]MissionPayload, 0, len(missions))
	for _, m := range missions {
		out = append(out, missionPayload(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"missions": out})
}

func (s *Server) handleMissionGet(w http.ResponseWriter, r *http.Request) {
	info, ok := s.requireAuth(w, r, authOperator, authUISession, authAgent)
	if !ok {
		return
	}
	id := domain.MissionID(r.PathValue("id"))
	m, found, err := s.Store.GetMission(id)
	if err != nil {
		fail(w, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, domain.ErrNotFound, "mission %s not found", id)
		return
	}
	tasks, err := s.Store.TasksForMission(id)
	if err != nil {
		fail(w, err)
		return
	}
	// An agent sees the mission only if it holds at least one task in it.
	if info.Kind == authAgent {
		holds := false
		for _, t := range tasks {
			if t.AgentID == info.Agent.ID {
				holds = true
				break
			}
		}
		if !holds {
			writeError(w, http.StatusForbidden, domain.ErrIdentityConflict, "this agent holds no task in that mission")
			return
		}
	}
	tp := make([]TaskPayload, 0, len(tasks))
	for _, t := range tasks {
		tp = append(tp, taskPayload(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"mission": missionPayload(m), "tasks": tp})
}

type taskCreateReq struct {
	Role    string `json:"role"`
	Title   string `json:"title"`
	AgentID string `json:"agentId"`
}

func (s *Server) handleTaskCreate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	missionID := domain.MissionID(r.PathValue("id"))
	if _, found, err := s.Store.GetMission(missionID); err != nil || !found {
		writeError(w, http.StatusNotFound, domain.ErrNotFound, "mission %s not found", missionID)
		return
	}
	var req taskCreateReq
	if err := readJSON(r, &req); err != nil {
		fail(w, err)
		return
	}
	role := domain.AgentRole(req.Role)
	if !domain.KnownRole(role) {
		writeError(w, http.StatusBadRequest, domain.ErrInvalidRequest, "unknown role %q", req.Role)
		return
	}
	t := domain.Task{
		ID: domain.NewTaskID(), MissionID: missionID, Role: role,
		Title: req.Title, State: domain.TaskCreated,
		AgentID: domain.AgentID(req.AgentID), CreatedAt: time.Now().UTC(),
	}
	if err := s.Store.PutTask(t); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, taskPayload(t))
}

func (s *Server) handleTaskGet(w http.ResponseWriter, r *http.Request) {
	info, ok := s.requireAuth(w, r, authOperator, authUISession, authAgent)
	if !ok {
		return
	}
	t, found, err := s.Store.GetTask(domain.TaskID(r.PathValue("id")))
	if err != nil {
		fail(w, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, domain.ErrNotFound, "task not found")
		return
	}
	if info.Kind == authAgent && t.AgentID != info.Agent.ID {
		writeError(w, http.StatusForbidden, domain.ErrIdentityConflict, "not this agent's task")
		return
	}
	writeJSON(w, http.StatusOK, taskPayload(t))
}

func (s *Server) handleTaskCancel(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	id := domain.TaskID(r.PathValue("id"))
	t, found, err := s.Store.GetTask(id)
	if err != nil {
		fail(w, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, domain.ErrNotFound, "task not found")
		return
	}
	if !domain.CanTransition(t.State, domain.TaskCancelled) {
		writeError(w, http.StatusConflict, domain.ErrConflict, "task in state %s cannot be cancelled", t.State)
		return
	}
	t.State = domain.TaskCancelled
	if err := s.Store.PutTask(t); err != nil {
		fail(w, err)
		return
	}
	// Withdraw any pending approvals of this task: a cancelled task must not
	// leave approvable requests behind.
	pending, _ := s.Store.ListApprovals(domain.ApprovalPending, "")
	for _, ap := range pending {
		if ap.TaskID == id {
			_ = s.Engine.CancelApproval(ap.ID, domain.Actor{Source: domain.SourceLocalUser, Label: "operator"}, time.Now())
		}
	}
	writeJSON(w, http.StatusOK, taskPayload(t))
}

type sessionBindReq struct {
	SessionID string `json:"sessionId"`
	Runtime   string `json:"runtime"`
	TaskID    string `json:"taskId"`
	AgentID   string `json:"agentId"`
}

func (s *Server) handleSessionBind(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	var req sessionBindReq
	if err := readJSON(r, &req); err != nil {
		fail(w, err)
		return
	}
	t, found, err := s.Store.GetTask(domain.TaskID(req.TaskID))
	if err != nil || !found {
		writeError(w, http.StatusNotFound, domain.ErrNotFound, "task not found")
		return
	}
	// Binding a runtime session starts the task: this is the moment an agent
	// begins acting on it, and the lifecycle says so (CREATED|READY →
	// RUNNING). Terminal tasks cannot be bound again.
	if t.State == domain.TaskCreated || t.State == domain.TaskReady {
		t.State = domain.TaskRunning
		if t.StartedAt.IsZero() {
			t.StartedAt = time.Now().UTC()
		}
		if err := s.Store.PutTask(t); err != nil {
			fail(w, err)
			return
		}
	}
	if t.State != domain.TaskRunning && t.State != domain.TaskWaitingApproval {
		writeError(w, http.StatusConflict, domain.ErrConflict, "task in state %s cannot be bound", t.State)
		return
	}
	b := store.SessionBinding{
		SessionID: req.SessionID, Runtime: req.Runtime,
		MissionID: t.MissionID, TaskID: t.ID, AgentID: domain.AgentID(req.AgentID),
		BoundAt: time.Now().UTC(),
	}
	if err := s.Store.BindSession(b); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

// hookDecideReq is the PreToolUse hook's request. Fields come straight from
// the hook's stdin JSON; identity comes from the session binding, never from
// these fields.
type hookDecideReq struct {
	SessionID string          `json:"sessionId"`
	ToolName  string          `json:"toolName"`
	ToolInput json.RawMessage `json:"toolInput"`
	Cwd       string          `json:"cwd"`
}

func (s *Server) handleHookDecide(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authHook); !ok {
		return
	}
	var req hookDecideReq
	if err := readJSON(r, &req); err != nil {
		fail(w, err)
		return
	}
	b, found, err := s.Store.LookupSession(req.SessionID)
	if err != nil {
		fail(w, err)
		return
	}
	if !found {
		writeError(w, http.StatusForbidden, domain.ErrUnknownIdentity,
			"runtime session %s is not bound to a Qiyan task", req.SessionID)
		return
	}
	action, err := BuildClaudeCodeAction(req.ToolName, req.ToolInput, req.Cwd)
	if err != nil {
		writeError(w, http.StatusBadRequest, domain.ErrUnrecognizedAction, "%v", err)
		return
	}
	// The hook path is the one place a deny is machine-enforced (verified in
	// R02-00: exit 2 stops the call), so its decisions are stamped ENFORCED.
	dec, err := s.evaluate(b, action, domain.CoverageEnforced)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, DecisionToPayload(dec))
}

// authorizeReq is the third-party host's decision request.
type authorizeReq struct {
	MissionID string        `json:"missionId"`
	TaskID    string        `json:"taskId"`
	Action    ActionPayload `json:"action"`
}

func (s *Server) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	info, ok := s.requireAuth(w, r, authAgent)
	if !ok {
		return
	}
	var req authorizeReq
	if err := readJSON(r, &req); err != nil {
		fail(w, err)
		return
	}
	b := store.SessionBinding{
		MissionID: domain.MissionID(req.MissionID),
		TaskID:    domain.TaskID(req.TaskID),
		AgentID:   info.Agent.ID,
	}
	// A host asking before it acts is observed, not enforced: Qiyan decides,
	// but compliance is the host's code. Claiming ENFORCED here would be the
	// false "protected" label the book forbids (S10); the coverage registry
	// records what a specific, tested integration actually guarantees.
	dec, err := s.evaluate(b, req.Action.ToAction(), domain.CoverageObserved)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, DecisionToPayload(dec))
}

// evaluate assembles the engine request from a binding + action. The binding
// supplies identity (mission, task, agent from the store) — the caller's
// request body never does.
func (s *Server) evaluate(b store.SessionBinding, action domain.Action, coverage domain.CoverageState) (domain.Decision, error) {
	task, found, err := s.Store.GetTask(b.TaskID)
	if err != nil {
		return domain.Decision{}, err
	}
	if !found {
		return domain.Decision{}, domain.NewError(domain.ErrNotFound, "task %s not found", b.TaskID)
	}
	m, found, err := s.Store.GetMission(task.MissionID)
	if err != nil {
		return domain.Decision{}, err
	}
	if !found {
		return domain.Decision{}, domain.NewError(domain.ErrNotFound, "mission %s not found", task.MissionID)
	}
	agent, found, err := s.Store.AgentByID(b.AgentID)
	if err != nil {
		return domain.Decision{}, err
	}
	if !found {
		return domain.Decision{}, domain.NewError(domain.ErrUnknownIdentity, "agent %s is not registered", b.AgentID)
	}
	roster, err := s.Store.ListAgents()
	if err != nil {
		return domain.Decision{}, err
	}
	return s.Engine.Evaluate(control.Request{
		Mission: m, Task: task, Agent: agent, Action: action,
		Agents: roster, Coverage: coverage, Now: time.Now(),
	})
}

func (s *Server) handleApprovalList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	state := domain.ApprovalState(r.URL.Query().Get("state"))
	list, err := s.Store.ListApprovals(state, "")
	if err != nil {
		fail(w, err)
		return
	}
	out := make([]ApprovalPayload, 0, len(list))
	for _, a := range list {
		out = append(out, approvalPayload(a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"approvals": out})
}

func (s *Server) handleApprovalGet(w http.ResponseWriter, r *http.Request) {
	info, ok := s.requireAuth(w, r, authOperator, authUISession, authAgent)
	if !ok {
		return
	}
	ap, found, err := s.Store.GetApproval(domain.ApprovalID(r.PathValue("id")))
	if err != nil {
		fail(w, err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, domain.ErrApprovalNotFound, "approval not found")
		return
	}
	if info.Kind == authAgent && ap.AgentID != info.Agent.ID {
		writeError(w, http.StatusForbidden, domain.ErrIdentityConflict, "not this agent's approval")
		return
	}
	writeJSON(w, http.StatusOK, approvalPayload(ap))
}

type approvalDecisionReq struct {
	Approve *bool `json:"approve"`
}

func (s *Server) handleApprovalDecision(w http.ResponseWriter, r *http.Request) {
	// Only the operator (local user) — the UI session path included — may
	// decide. An agent token here is refused wholesale.
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	var req approvalDecisionReq
	if err := readJSON(r, &req); err != nil {
		fail(w, err)
		return
	}
	if req.Approve == nil {
		writeError(w, http.StatusBadRequest, domain.ErrInvalidRequest, "approve must be true or false")
		return
	}
	ap, err := s.Engine.Decide(domain.ApprovalID(r.PathValue("id")), *req.Approve,
		domain.Actor{Source: domain.SourceLocalUser, Label: "operator"}, time.Now())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, approvalPayload(ap))
}

type resultEventReq struct {
	TaskID       string `json:"taskId"`
	ActionDigest string `json:"actionDigest"`
	Result       string `json:"result"` // executed | failed | blocked | cancelled
	Note         string `json:"note"`
}

func (s *Server) handleResultEvent(w http.ResponseWriter, r *http.Request) {
	info, ok := s.requireAuth(w, r, authAgent)
	if !ok {
		return
	}
	var req resultEventReq
	if err := readJSON(r, &req); err != nil {
		fail(w, err)
		return
	}
	switch req.Result {
	case "executed", "failed", "blocked", "cancelled":
	default:
		writeError(w, http.StatusBadRequest, domain.ErrInvalidRequest, "result must be executed, failed, blocked or cancelled")
		return
	}
	t, found, err := s.Store.GetTask(domain.TaskID(req.TaskID))
	if err != nil || !found || t.AgentID != info.Agent.ID {
		writeError(w, http.StatusForbidden, domain.ErrIdentityConflict, "not this agent's task")
		return
	}
	if err := s.Store.AppendResult(store.ResultEvent{
		TaskID: t.ID, AgentID: info.Agent.ID, ActionDigest: req.ActionDigest,
		Result: req.Result, Note: truncate(req.Note, 400), At: time.Now().UTC(),
	}); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "recorded"})
}

func (s *Server) handleDecisions(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	taskID := domain.TaskID(r.URL.Query().Get("task"))
	var (
		recs []store.DecisionRecord
		err  error
	)
	if taskID != "" {
		recs, err = s.Store.DecisionsForTask(taskID)
	} else {
		recs, err = s.Store.RecentDecisions(200)
	}
	if err != nil {
		fail(w, err)
		return
	}
	out := make([]DecisionRecordPayload, 0, len(recs))
	for _, rec := range recs {
		out = append(out, decisionRecordPayload(rec))
	}
	writeJSON(w, http.StatusOK, map[string]any{"decisions": out})
}

func (s *Server) handleCoverage(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	rows, err := s.Store.ListCoverage()
	if err != nil {
		fail(w, err)
		return
	}
	out := make([]CoveragePayload, 0, len(rows))
	for _, c := range rows {
		out = append(out, coveragePayload(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"coverage": out})
}

type uiSessionReq struct {
	Token string `json:"token"`
}

func (s *Server) handleUISession(w http.ResponseWriter, r *http.Request) {
	var req uiSessionReq
	if err := readJSON(r, &req); err != nil {
		fail(w, err)
		return
	}
	if s.OperatorToken == "" || req.Token != s.OperatorToken {
		writeError(w, http.StatusUnauthorized, domain.ErrUnknownIdentity, "invalid bootstrap token")
		return
	}
	sid, sess := s.sessions.create()
	http.SetCookie(w, &http.Cookie{
		Name: uiCookieName, Value: sid, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"csrf": sess.CSRF})
}
