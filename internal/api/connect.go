package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/connector"
	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
	"github.com/QiyanAI/ai-agent-action-surface/internal/store"
)

// Connect endpoints: the Claude Code integration, driven from the CLI today
// and from the console wizard later. Preview never touches the filesystem;
// apply runs the real enforcement probe first and refuses to install an
// integration it cannot vouch for unless the operator explicitly accepts the
// weaker tier.

type connectReq struct {
	ProjectDir string `json:"projectDir"`
	ClaudeBin  string `json:"claudeBin,omitempty"`
}

type connectApplyReq struct {
	connectReq
	AllowUnprobed bool `json:"allowUnprobed,omitempty"`
}

type planPayload struct {
	SettingsPath string   `json:"settingsPath"`
	BackupPath   string   `json:"backupPath,omitempty"`
	NoChanges    bool     `json:"noChanges"`
	Changes      []string `json:"changes"`
}

func planToPayload(p connector.Plan) planPayload {
	return planPayload{
		SettingsPath: p.SettingsPath, BackupPath: p.BackupPath,
		NoChanges: p.NoChanges, Changes: p.Changes,
	}
}

func (s *Server) validateProject(w http.ResponseWriter, dir string) bool {
	if dir == "" || !filepath.IsAbs(dir) {
		writeError(w, http.StatusBadRequest, domain.ErrInvalidRequest, "projectDir must be an absolute path")
		return false
	}
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		writeError(w, http.StatusBadRequest, domain.ErrInvalidRequest, "projectDir is not an existing directory")
		return false
	}
	return true
}

func (s *Server) handleConnectPreview(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	var req connectReq
	if err := readJSON(r, &req); err != nil {
		fail(w, err)
		return
	}
	if !s.validateProject(w, req.ProjectDir) {
		return
	}
	det, err := connector.Detect(req.ClaudeBin)
	if err != nil {
		writeError(w, http.StatusBadRequest, domain.ErrInvalidRequest, "%v", err)
		return
	}
	exe, err := os.Executable()
	if err != nil {
		fail(w, domain.NewError(domain.ErrInternal, "cannot locate the qiyan binary: %v", err))
		return
	}
	plan, err := connector.PlanInstall(req.ProjectDir, exe, s.StateDir)
	if err != nil {
		writeError(w, http.StatusConflict, domain.ErrConflict, "%v", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"detected":         det,
		"supportFloor":     connector.SupportFloor(),
		"plan":             planToPayload(plan),
		"probeRunsOnApply": !plan.NoChanges,
		"probeNotice":      "applying runs three tiny real sessions through the installed Claude Code (a small usage of your existing account); the exact behavior on this machine is what decides the support tier",
	})
}

func (s *Server) handleConnectApply(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	var req connectApplyReq
	if err := readJSON(r, &req); err != nil {
		fail(w, err)
		return
	}
	if !s.validateProject(w, req.ProjectDir) {
		return
	}
	det, err := connector.Detect(req.ClaudeBin)
	if err != nil {
		writeError(w, http.StatusBadRequest, domain.ErrInvalidRequest, "%v", err)
		return
	}
	exe, err := os.Executable()
	if err != nil {
		fail(w, domain.NewError(domain.ErrInternal, "cannot locate the qiyan binary: %v", err))
		return
	}
	plan, err := connector.PlanInstall(req.ProjectDir, exe, s.StateDir)
	if err != nil {
		writeError(w, http.StatusConflict, domain.ErrConflict, "%v", err)
		return
	}
	if plan.NoChanges {
		writeJSON(w, http.StatusOK, map[string]any{"plan": planToPayload(plan), "detected": det, "noChanges": true})
		return
	}

	// Real probe: what this machine actually does, not what the version says.
	// The probe outlives the server's ordinary WriteTimeout by design.
	extendWriteDeadline(w, 6*time.Minute)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	probe, err := s.probe(ctx, det.Binary, exe, os.TempDir())
	if err != nil {
		fail(w, domain.NewError(domain.ErrControlUnavailable, "the enforcement probe could not run: %v", err))
		return
	}
	tier := probe.Tier()
	if tier != connector.TierEnforced && !req.AllowUnprobed {
		// The error envelope stays the contract; the structured probe rides
		// along so consoles can explain the outcome without parsing prose.
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": map[string]string{
				"class":   string(domain.ErrConflict),
				"message": fmt.Sprintf("this Claude Code (%s) did not pass the fail-closed probe on this machine (tier: %s); re-run with allowUnprobed to install with the weaker, honestly-labelled protection", det.Version, tier),
			},
			"tier":  string(tier),
			"probe": probe,
		})
		return
	}

	if err := connector.Apply(plan); err != nil {
		fail(w, domain.NewError(domain.ErrInternal, "could not write settings: %v", err))
		return
	}

	inst := store.Installation{
		ProjectDir: req.ProjectDir, SettingsPath: plan.SettingsPath, BackupPath: plan.BackupPath,
		HookCommand:  connector.HookCommand(exe, s.StateDir),
		ClaudeBinary: det.Binary, Version: det.Version, Tier: string(tier),
		ProbeEvidence: probe.Evidence, InstalledAt: time.Now().UTC(),
	}
	if err := s.Store.PutInstallation(inst); err != nil {
		fail(w, err)
		return
	}
	if err := s.recordProbeCoverage(probe, req.ProjectDir); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"installed": true, "installation": inst, "probe": probe, "plan": planToPayload(plan),
	})
}

func (s *Server) handleConnectList(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	list, err := s.Store.ListInstallations()
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"installations": list})
}

// handleDetectClaude reports whether Claude Code is available to this
// service and what its version implies; the measured tier still comes only
// from a real probe. A missing binary is a normal 200 answer, not an error,
// because the wizard renders it as guidance.
func (s *Server) handleDetectClaude(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	det, err := connector.Detect(r.URL.Query().Get("bin"))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"found":        false,
			"error":        err.Error(),
			"supportFloor": connector.SupportFloor(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"found":           true,
		"binary":          det.Binary,
		"version":         det.Version,
		"tierFromVersion": det.TierFromVersion,
		"supportFloor":    connector.SupportFloor(),
	})
}

func (s *Server) handleConnectRemove(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r, authOperator, authUISession); !ok {
		return
	}
	dir := r.URL.Query().Get("project")
	if !s.validateProject(w, dir) {
		return
	}
	plan, err := connector.PlanUninstall(dir)
	if err != nil {
		writeError(w, http.StatusConflict, domain.ErrConflict, "%v", err)
		return
	}
	if !plan.NoChanges {
		if err := connector.Apply(plan); err != nil {
			fail(w, domain.NewError(domain.ErrInternal, "could not write settings: %v", err))
			return
		}
		if err := s.Store.RemoveInstallation(dir); err != nil {
			fail(w, err)
			return
		}
	}
	// Coverage follows reality: this path is no longer protected.
	row := domain.Coverage{
		ID: domain.NewCoverageID(), Path: coveragePathPrefix("uninstalled") + " (" + domain.NormalizePath(dir, "") + ")",
		State: domain.CoverageUncovered, Basis: "integration removed at " + time.Now().UTC().Format(time.RFC3339),
		CheckedAt: time.Now().UTC(),
	}
	if err := s.Store.PutCoverage(row); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": !plan.NoChanges, "plan": planToPayload(plan)})
}

// recordProbeCoverage writes coverage rows straight from probe results: the
// per-scenario facts and one summary row that is ENFORCED only when every
// fail-closed scenario passed. Rows are scoped to the project they were
// measured for — connecting a second project must never overwrite the first
// project's evidence.
func (s *Server) recordProbeCoverage(p connector.ProbeResult, projectDir string) error {
	now := time.Now().UTC()
	scope := " (" + domain.NormalizePath(projectDir, "") + ")"
	type row struct {
		suffix string
		ok     bool
		basis  string
	}
	rows := []row{
		{"deny-blocked", p.DenyBlocked, "a real deny stopped a real tool call"},
		{"failure-blocks", p.FailureBlocks, "a crashing hook blocked the call (onFailure=block honored)"},
		{"allow-runs", p.AllowRuns, "a normal in-scope call still ran (no over-blocking)"},
	}
	for _, r := range rows {
		state := domain.CoverageUncovered
		if r.ok {
			state = domain.CoverageEnforced
		}
		c := domain.Coverage{
			ID:        domain.NewCoverageID(),
			Path:      coveragePathPrefix(p.Version) + "#" + r.suffix + scope,
			State:     state,
			Basis:     r.basis + " (probe on this machine, " + now.Format(time.RFC3339) + ")",
			CheckedAt: now,
		}
		if err := s.Store.PutCoverage(c); err != nil {
			return err
		}
	}
	summaryState := domain.CoverageUncovered
	switch p.Tier() {
	case connector.TierEnforced:
		summaryState = domain.CoverageEnforced
	case connector.TierBlockingOnly:
		summaryState = domain.CoverageObserved
	}
	summary := domain.Coverage{
		ID:        domain.NewCoverageID(),
		Path:      coveragePathPrefix(p.Version) + "#summary" + scope,
		State:     summaryState,
		Basis:     "probe: deny=" + boolWord(p.DenyBlocked) + " failure=" + boolWord(p.FailureBlocks) + " allow=" + boolWord(p.AllowRuns) + " at " + now.Format(time.RFC3339),
		CheckedAt: now,
	}
	return s.Store.PutCoverage(summary)
}

func coveragePathPrefix(version string) string {
	return "claude-code/" + version + "/PreToolUse"
}

func boolWord(b bool) string {
	if b {
		return "blocked"
	}
	return "passed-through"
}
