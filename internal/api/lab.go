package api

import (
	"context"
	"net/http"
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
	"github.com/QiyanAI/ai-agent-action-surface/internal/lab"
)

// Safety Lab endpoints. Scans never execute the scanned project's code and
// never connect anywhere; applying a suggestion always previews first, and
// the result is verified with a real, harmless probe whose outcome is
// recorded as coverage.

type labScanReq struct {
	Kind string `json:"kind,omitempty"` // manifest | claude-project | mcp-json
	Path string `json:"path"`
}

func (s *Server) handleLabScan(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	var req labScanReq
	if err := readJSON(r, &req); err != nil {
		fail(w, err)
		return
	}
	if req.Path == "" {
		writeError(w, http.StatusBadRequest, domain.ErrInvalidRequest, "path is required")
		return
	}
	result, err := lab.Scan(lab.InputKind(req.Kind), req.Path)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type labApplyReq struct {
	ProjectDir   string `json:"projectDir"`
	SuggestionID string `json:"suggestionId"`
	ClaudeBin    string `json:"claudeBin,omitempty"`
	Preview      bool   `json:"preview,omitempty"`
}

type labApplyResp struct {
	Applied       []string           `json:"appliedRules"`
	Changes       []string           `json:"changes"`
	SettingsPath  string             `json:"settingsPath"`
	BackupPath    string             `json:"backupPath"`
	Verifications []lab.VerifyResult `json:"verifications"`
	// Preview marks a dry-run response: the plan, and nothing written.
	Preview bool `json:"preview,omitempty"`
}

func (s *Server) handleLabApply(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	var req labApplyReq
	if err := readJSON(r, &req); err != nil {
		fail(w, err)
		return
	}
	if !s.validateProject(w, req.ProjectDir) {
		return
	}
	// Re-derive the suggestion from a fresh scan: the client sends an ID, not
	// arbitrary rules, so the server only ever applies what the scan itself
	// proposed.
	scan, err := lab.Scan(lab.InputClaudeJSON, req.ProjectDir)
	if err != nil {
		fail(w, err)
		return
	}
	var suggestion *lab.Suggestion
	for i := range scan.Suggestions {
		if scan.Suggestions[i].ID == req.SuggestionID {
			suggestion = &scan.Suggestions[i]
			break
		}
	}
	if suggestion == nil {
		writeError(w, http.StatusNotFound, domain.ErrNotFound, "no suggestion %q for this project", req.SuggestionID)
		return
	}
	if !suggestion.AutoApply || suggestion.Kind != "add-deny-rule" || len(suggestion.AddDeny) == 0 {
		writeError(w, http.StatusConflict, domain.ErrConflict,
			"this suggestion is advisory only; apply it by hand following its description")
		return
	}

	plan, err := lab.PlanAddDenyRules(req.ProjectDir, suggestion.AddDeny)
	if err != nil {
		fail(w, err)
		return
	}
	if plan.NoChanges {
		if req.Preview {
			writeJSON(w, http.StatusOK, labApplyResp{Changes: plan.Changes, SettingsPath: plan.SettingsPath, Preview: true})
			return
		}
		writeJSON(w, http.StatusOK, labApplyResp{Changes: plan.Changes, SettingsPath: plan.SettingsPath})
		return
	}
	if req.Preview {
		// A real preview: the exact diff that would be written, nothing else.
		writeJSON(w, http.StatusOK, labApplyResp{Changes: plan.Changes, SettingsPath: plan.SettingsPath, BackupPath: plan.BackupPath, Preview: true})
		return
	}
	if err := lab.ApplyDenyPlan(plan); err != nil {
		fail(w, domain.NewError(domain.ErrInternal, "could not write settings: %v", err))
		return
	}

	// Verify each rule with a real, harmless probe and record the outcome.
	// The probes run real Claude Code sessions and outlive the ordinary
	// WriteTimeout by design.
	resp := labApplyResp{Applied: suggestion.AddDeny, Changes: plan.Changes,
		SettingsPath: plan.SettingsPath, BackupPath: plan.BackupPath}
	extendWriteDeadline(w, 5*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	projectScope := domain.NormalizePath(req.ProjectDir, "")
	for _, rule := range suggestion.AddDeny {
		vr, err := lab.VerifyDenyRule(ctx, req.ProjectDir, req.ClaudeBin, rule)
		if err != nil {
			vr = lab.VerifyResult{Rule: rule, Verified: false, Note: err.Error()}
		}
		resp.Verifications = append(resp.Verifications, vr)
		state := domain.CoverageUncovered
		if vr.Verified {
			state = domain.CoverageEnforced
		}
		basis := vr.Evidence
		if basis == "" {
			basis = vr.Note
		}
		if err := s.Store.PutCoverage(domain.Coverage{
			ID: domain.NewCoverageID(), Path: "claude-code/permission-deny/" + rule + " (" + projectScope + ")",
			State: state, Basis: basis, CheckedAt: time.Now().UTC(),
		}); err != nil {
			fail(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

type labUndoReq struct {
	ProjectDir string   `json:"projectDir"`
	Rules      []string `json:"rules"`
}

func (s *Server) handleLabUndo(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	var req labUndoReq
	if err := readJSON(r, &req); err != nil {
		fail(w, err)
		return
	}
	if !s.validateProject(w, req.ProjectDir) || len(req.Rules) == 0 {
		writeError(w, http.StatusBadRequest, domain.ErrInvalidRequest, "projectDir and rules are required")
		return
	}
	plan, err := lab.PlanRemoveDenyRules(req.ProjectDir, req.Rules)
	if err != nil {
		fail(w, err)
		return
	}
	if !plan.NoChanges {
		if err := lab.ApplyDenyPlan(plan); err != nil {
			fail(w, domain.NewError(domain.ErrInternal, "could not write settings: %v", err))
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"changes": plan.Changes, "noChanges": plan.NoChanges})
}
