package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/QiyanAI/ai-agent-action-surface/internal/connector"
	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
	"github.com/QiyanAI/ai-agent-action-surface/internal/mission"
)

// Mission run endpoints: the three-role fleet is scheduled by the service
// process (the single writer of state), and every role session talks back
// through the same hook and engine as everything else.

type missionRunReq struct {
	ClaudeBin string `json:"claudeBin,omitempty"`
}

// missionSettingsPath is the generated settings file every mission session
// runs with. It holds exactly one hook: Qiyan's, with onFailure=block.
func (s *Server) missionSettingsPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	path := filepath.Join(s.StateDir, "mission-settings.json")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	doc := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"hooks": []any{
						map[string]any{
							"type":      "command",
							"command":   connector.HookCommand(exe, s.StateDir),
							"timeout":   10,
							"onFailure": "block",
						},
					},
				},
			},
		},
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (s *Server) handleMissionRun(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
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
	var req missionRunReq
	if r.Body != nil {
		_ = readJSON(r, &req) // body is optional for this endpoint
	}
	claudeBin := req.ClaudeBin
	if claudeBin == "" {
		det, err := connector.Detect("")
		if err != nil {
			writeError(w, http.StatusBadRequest, domain.ErrInvalidRequest,
				"no Claude Code found; pass claudeBin: %v", err)
			return
		}
		claudeBin = det.Binary
	}
	if _, err := connector.Detect(claudeBin); err != nil {
		writeError(w, http.StatusBadRequest, domain.ErrInvalidRequest, "claudeBin unusable: %v", err)
		return
	}
	settingsPath, err := s.missionSettingsPath()
	if err != nil {
		fail(w, domain.NewError(domain.ErrInternal, "cannot write mission settings: %v", err))
		return
	}

	workdir := m.Policy.Scope.Roots[0]

	o := &mission.Orchestrator{
		Store:  s.Store,
		Engine: s.Engine,
		NewSession: func(role domain.AgentRole, agent domain.Agent, task domain.Task, sessionID string) mission.SessionRunner {
			return &mission.ClaudeSession{
				Binary: claudeBin, SessionID: sessionID, Cwd: workdir, Settings: settingsPath,
			}
		},
	}
	started, err := s.runs.Start(id, o)
	if err != nil {
		fail(w, err)
		return
	}
	if !started {
		writeError(w, http.StatusConflict, domain.ErrConflict, "a run for this mission is already in progress")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started", "missionId": string(id)})
}

func (s *Server) handleMissionRunStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	id := domain.MissionID(r.PathValue("id"))
	status, ok := s.runs.Status(id)
	if !ok {
		writeError(w, http.StatusNotFound, domain.ErrNotFound, "no run recorded for mission %s", id)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleMissionRunCancel(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	id := domain.MissionID(r.PathValue("id"))
	if !s.runs.Cancel(id) {
		writeError(w, http.StatusConflict, domain.ErrConflict, "no live run for mission %s", id)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelling"})
}
