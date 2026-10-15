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

// verifyLeg is the outcome of one controlled probe session.
type verifyLeg struct {
	ran      bool // the command's effect was observed (or the transcript said DONE)
	refused  bool // the runtime refused the command (transcript markers)
	complete bool // the session produced an interpretable result at all
	text     string
	why      string // when complete is false
}

// runVerifyLeg runs one headless session asking for the probe command, under
// an explicit settings file, with the command pre-allowed so the only
// differentiator between the two legs of a comparison is the deny rule.
func runVerifyLeg(ctx context.Context, binary, workDir, settingsPath, allowTool, prompt string) verifyLeg {
	runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, binary,
		"-p", "--output-format", "json",
		"--permission-mode", "default",
		// User-level baseline only, so nothing else in the project masks the
		// result; the probe command itself is pre-allowed — the comparison
		// between the two legs is then attributable to the rule alone.
		"--setting-sources", "user",
		"--allowedTools", allowTool,
		"--settings", settingsPath,
		prompt)
	cmd.Dir = workDir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()
	text := out.String()
	ok, why := sessionCompleted(runErr, text)
	return verifyLeg{
		complete: ok,
		why:      why,
		text:     text,
		refused:  strings.Contains(text, "BLOCKED") || strings.Contains(text, `"permission_denials":[{`),
	}
}

// verdictFor decides verified/refused from the two legs. Verified requires
// ALL of: without the rule the command really ran; with the rule the runtime
// produced an explicit refusal; and the command's effect was absent. Anything
// else is indeterminate and must not be recorded as protection.
func verdictFor(rule string, control, guarded verifyLeg, controlRan, guardedRan, guardedRefused bool) (bool, string) {
	if !control.complete {
		return false, "the no-rule control run was indeterminate (" + control.why + "); the rule's effect could not be isolated"
	}
	if !guarded.complete {
		return false, "the with-rule run was indeterminate (" + guarded.why + "); the rule's effect was not confirmed"
	}
	if !controlRan {
		return false, "without the rule the command did not run in this environment, so the rule's effect could not be isolated"
	}
	if guardedRan {
		return false, "the command ran even with the rule present — the rule did not refuse it on this machine"
	}
	if !guardedRefused {
		return false, "the with-rule run produced no explicit refusal of the command; the rule's effect was not confirmed"
	}
	return true, ""
}

// VerifyDenyRule proves a deny rule by controlled comparison on this machine:
// the same harmless command is attempted once with the rule absent (it must
// run) and once with the rule present (it must be refused). A single run can
// never prove a rule — a headless session may refuse the command for reasons
// that have nothing to do with the rule. claudeBin may be empty (PATH).
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
	controlDir, err := os.MkdirTemp("", "qiyan-verify-control-")
	if err != nil {
		return VerifyResult{}, err
	}
	defer os.RemoveAll(controlDir)
	guardedDir, err := os.MkdirTemp("", "qiyan-verify-guarded-")
	if err != nil {
		return VerifyResult{}, err
	}
	defer os.RemoveAll(guardedDir)
	for _, dir := range []string{controlDir, guardedDir} {
		if rule == "Bash(rm:*)" {
			if err := os.WriteFile(filepath.Join(dir, "scratch.txt"), []byte("scratch\n"), 0o600); err != nil {
				return VerifyResult{}, err
			}
		}
	}
	controlSettings := filepath.Join(controlDir, "qiyan-control-settings.json")
	if err := os.WriteFile(controlSettings, []byte("{\"permissions\":{\"deny\":[]}}\n"), 0o600); err != nil {
		return VerifyResult{}, err
	}

	program := probe.command
	if i := strings.IndexByte(program, ' '); i >= 0 {
		program = program[:i]
	}
	allowTool := "Bash(" + program + ":*)"
	prompt := fmt.Sprintf("Use the Bash tool to run exactly this command once: %s — then reply with one word: DONE if the command succeeded, or BLOCKED if it was blocked.", probe.command)

	control := runVerifyLeg(ctx, det.Binary, controlDir, controlSettings, allowTool, prompt)
	guarded := runVerifyLeg(ctx, det.Binary, guardedDir, settingsPathFor(projectDir), allowTool, prompt)

	var controlRan, guardedRan, guardedRefused bool
	switch rule {
	case "Bash(curl:*)":
		// No filesystem effect to observe: the transcript markers are the
		// signal, and the refusal marker must be explicit.
		controlRan = control.complete && strings.Contains(control.text, "DONE") && !control.refused
		guardedRan = guarded.complete && !guarded.refused && strings.Contains(guarded.text, "DONE")
	default:
		controlRan = control.complete && !probe.check(controlDir)
		guardedRan = guarded.complete && !probe.check(guardedDir)
	}
	guardedRefused = guarded.complete && guarded.refused && !guardedRan
	verified, why := verdictFor(rule, control, guarded, controlRan, guardedRan, guardedRefused)
	evidence := fmt.Sprintf("controlled comparison with %s: without the rule %q %s; with the rule %q %s",
		det.Version, probe.command,
		map[bool]string{true: "ran", false: "did not run"}[controlRan],
		probe.command,
		map[bool]string{true: "also ran", false: "was refused"}[guardedRan])
	if !verified {
		return VerifyResult{Rule: rule, Verified: false, Evidence: evidence, Note: why}, nil
	}
	return VerifyResult{Rule: rule, Verified: true, Evidence: evidence, Note: probe.harmless}, nil
}

// sessionCompleted reports whether the verification session produced a
// result that may be interpreted. A failed run, an empty result, or a result
// the runtime itself marked as an error (is_error:true — e.g. auth or model
// failures) is indeterminate, not evidence that the rule blocked anything.
func sessionCompleted(runErr error, text string) (bool, string) {
	if runErr != nil {
		return false, "the verification session failed: " + runErr.Error()
	}
	if strings.TrimSpace(text) == "" {
		return false, "the verification session produced no result"
	}
	if strings.Contains(text, `"is_error":true`) {
		return false, "the verification session reported an error result"
	}
	return true, ""
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
