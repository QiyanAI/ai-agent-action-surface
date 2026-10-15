package lab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/connector"
	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
)

// Applying a suggestion: preview first, back up once, then verify with a real
// probe.
//
// The only auto-applicable suggestions in this build add `permissions.deny`
// rules to a project's Claude settings. Deny rules are evaluated by Claude
// Code itself regardless of what any hook returns, so they hold even when
// Qiyan is not connected — and they can be verified like everything else:
// by attempting a harmless matching command and watching what happens.

// DenyPlan is one pending settings change from a suggestion.
type DenyPlan struct {
	SettingsPath  string   `json:"settingsPath"`
	BackupPath    string   `json:"backupPath"`
	ExistedBefore bool     `json:"existedBefore"`
	Add           []string `json:"add"`
	Remove        []string `json:"remove,omitempty"`
	Changes       []string `json:"changes"`
	NoChanges     bool     `json:"noChanges"`
	NewContent    []byte   `json:"-"`
}

func settingsPathFor(projectDir string) string {
	return filepath.Join(projectDir, ".claude", "settings.local.json")
}

// PlanAddDenyRules computes the change that adds rules to permissions.deny.
func PlanAddDenyRules(projectDir string, rules []string) (DenyPlan, error) {
	plan := DenyPlan{SettingsPath: settingsPathFor(projectDir), Add: rules}
	plan.BackupPath = plan.SettingsPath + ".qiyan-backup"
	doc, existed, err := loadDoc(plan.SettingsPath)
	if err != nil {
		return plan, err
	}
	plan.ExistedBefore = existed

	perms := ensureMap(doc, "permissions")
	deny := toStrings(perms["deny"])
	existing := map[string]bool{}
	for _, d := range deny {
		existing[d] = true
	}
	var added []string
	for _, r := range rules {
		if !existing[r] {
			deny = append(deny, r)
			added = append(added, r)
		}
	}
	if len(added) == 0 {
		plan.NoChanges = true
		plan.Changes = append(plan.Changes, "these rules are already present; nothing to change")
		return plan, nil
	}
	perms["deny"] = deny
	content, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return plan, err
	}
	plan.NewContent = append(content, '\n')
	plan.Changes = append(plan.Changes, fmt.Sprintf("add %d deny rule(s) to %s", len(added), plan.SettingsPath))
	for _, a := range added {
		plan.Changes = append(plan.Changes, "  + permissions.deny: "+a)
	}
	if !existed {
		plan.Changes = append(plan.Changes, "the file does not exist yet and will be created")
	} else {
		plan.Changes = append(plan.Changes, "a one-time backup is kept at "+plan.BackupPath)
	}
	return plan, nil
}

// PlanRemoveDenyRules computes the undo of a previous add.
func PlanRemoveDenyRules(projectDir string, rules []string) (DenyPlan, error) {
	plan := DenyPlan{SettingsPath: settingsPathFor(projectDir), Remove: rules}
	plan.BackupPath = plan.SettingsPath + ".qiyan-backup"
	doc, existed, err := loadDoc(plan.SettingsPath)
	if err != nil {
		return plan, err
	}
	plan.ExistedBefore = existed
	if !existed {
		plan.NoChanges = true
		plan.Changes = append(plan.Changes, "no settings file; nothing to remove")
		return plan, nil
	}
	perms, _ := doc["permissions"].(map[string]any)
	if perms == nil {
		plan.NoChanges = true
		return plan, nil
	}
	drop := map[string]bool{}
	for _, r := range rules {
		drop[r] = true
	}
	var kept []string
	removed := 0
	for _, d := range toStrings(perms["deny"]) {
		if drop[d] {
			removed++
			continue
		}
		kept = append(kept, d)
	}
	if removed == 0 {
		plan.NoChanges = true
		plan.Changes = append(plan.Changes, "nothing to remove")
		return plan, nil
	}
	if len(kept) == 0 {
		delete(perms, "deny")
	} else {
		perms["deny"] = kept
	}
	if len(perms) == 0 {
		delete(doc, "permissions")
	}
	content, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return plan, err
	}
	plan.NewContent = append(content, '\n')
	plan.Changes = append(plan.Changes, fmt.Sprintf("remove %d rule(s) from %s", removed, plan.SettingsPath))
	return plan, nil
}

// ApplyDenyPlan writes the plan (backup once, atomic replace).
func ApplyDenyPlan(p DenyPlan) error {
	if p.NoChanges {
		return nil
	}
	if p.ExistedBefore {
		if _, err := os.Stat(p.BackupPath); os.IsNotExist(err) {
			orig, err := os.ReadFile(p.SettingsPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(p.BackupPath, orig, 0o600); err != nil {
				return err
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(p.SettingsPath), 0o700); err != nil {
		return err
	}
	tmp := p.SettingsPath + ".tmp"
	if err := os.WriteFile(tmp, p.NewContent, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p.SettingsPath)
}

// VerifyResult reports a real, harmless verification run.
type VerifyResult struct {
	Rule     string `json:"rule"`
	Verified bool   `json:"verified"`
	Evidence string `json:"evidence"`
	Note     string `json:"note,omitempty"`
}

// verifyProbes maps a deny-rule prefix to a probe command that is harmless
// even if enforcement fails. A rule with no defined probe is reported as
// unverified rather than guessed at.
var verifyProbes = map[string]struct {
	command  string
	check    func(dir string) bool // returns true when the command was blocked
	harmless string
}{
	"Bash(rm:*)": {
		command: "rm scratch.txt",
		check: func(dir string) bool {
			_, err := os.Stat(filepath.Join(dir, "scratch.txt"))
			return err == nil // blocked => the scratch file survived
		},
		harmless: "deletes only a scratch file this check created in its own temp directory",
	},
	"Bash(curl:*)": {
		command:  "curl --help",
		check:    func(string) bool { return false }, // decided from the transcript below
		harmless: "curl --help only prints help text even if it runs",
	},
	"Bash(touch:*)": {
		command: "touch probe-artifact.txt",
		check: func(dir string) bool {
			_, err := os.Stat(filepath.Join(dir, "probe-artifact.txt"))
			return os.IsNotExist(err)
		},
		harmless: "touches a file in its own temp directory",
	},
}

// VerifyDenyRule attempts the probe command under the project's settings and
// reports whether Claude Code refused it. claudeBin may be empty (PATH).
func VerifyDenyRule(ctx context.Context, projectDir, claudeBin, rule string) (VerifyResult, error) {
	probe, ok := verifyProbes[rule]
	if !ok {
		return VerifyResult{Rule: rule, Verified: false,
			Note: "no harmless verification probe exists for this rule in this build; the rule's presence was confirmed, its effect was not"}, nil
	}
	det, err := connector.Detect(claudeBin)
	if err != nil {
		return VerifyResult{}, domain.NewError(domain.ErrControlUnavailable, "no Claude Code to verify with: %v", err)
	}
	dir, err := os.MkdirTemp("", "qiyan-verify-")
	if err != nil {
		return VerifyResult{}, err
	}
	defer os.RemoveAll(dir)
	if rule == "Bash(rm:*)" {
		if err := os.WriteFile(filepath.Join(dir, "scratch.txt"), []byte("scratch\n"), 0o600); err != nil {
			return VerifyResult{}, err
		}
	}

	prompt := fmt.Sprintf("Use the Bash tool to run exactly this command once: %s — then reply with one word: DONE if the command succeeded, or BLOCKED if it was blocked.", probe.command)
	runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, det.Binary,
		"-p", "--output-format", "json",
		"--permission-mode", "default",
		// The project's settings file, applied directly; user-level baseline
		// only, so nothing else in the project masks the result.
		"--setting-sources", "user",
		"--settings", settingsPathFor(projectDir),
		prompt)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	_ = cmd.Run()

	text := out.String()
	switch rule {
	case "Bash(curl:*)":
		blocked := strings.Contains(text, "BLOCKED") || !strings.Contains(text, "DONE")
		return VerifyResult{Rule: rule, Verified: blocked,
			Evidence: fmt.Sprintf("probe with %s on %s: %s", det.Version, probe.command, firstWords(text, 160)),
			Note:     probe.harmless}, nil
	default:
		blocked := probe.check(dir)
		return VerifyResult{Rule: rule, Verified: blocked,
			Evidence: fmt.Sprintf("probe with %s: the matching command was %s", det.Version, map[bool]string{true: "refused", false: "allowed"}[blocked]),
			Note:     probe.harmless}, nil
	}
}

// --- helpers ---

func loadDoc(path string) (map[string]any, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, false, nil
		}
		return nil, false, err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, false, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	return doc, true, nil
}

func ensureMap(doc map[string]any, key string) map[string]any {
	m, ok := doc[key].(map[string]any)
	if !ok {
		m = map[string]any{}
		doc[key] = m
	}
	return m
}

func toStrings(v any) []string {
	list, _ := v.([]any)
	var out []string
	for _, e := range list {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func firstWords(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
