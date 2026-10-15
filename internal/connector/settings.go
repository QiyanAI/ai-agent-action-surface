package connector

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Project-scoped settings installation.
//
// Rules this file exists to honour (book §10, UX-03):
//   - The install targets the project's own .claude/settings.local.json — a
//     per-project, not-shared file. Qiyan never edits the user's global
//     ~/.claude/settings.json behind their back.
//   - The exact file and exact content are shown before anything is written;
//     applying requires the caller's confirmation.
//   - The original file is backed up once, before the first modification.
//   - Undo removes exactly what Qiyan added and nothing else.

// HookMarker identifies Qiyan's hook entry inside a settings file.
const HookMarker = "hook pretooluse"

// Plan describes one pending settings change, fully materialised: the CLI or
// UI shows Changes and the target path, the user confirms, Apply writes it.
type Plan struct {
	ProjectDir    string
	SettingsPath  string
	BackupPath    string
	ExistedBefore bool
	NoChanges     bool
	Changes       []string
	NewContent    []byte
	// Uninstall marks a removal plan.
	Uninstall bool
}

// HookCommand builds the hook command line Qiyan installs. Binaries and state
// directories are quoted because both may contain spaces.
func HookCommand(qiyanBinary, stateDir string) string {
	return fmt.Sprintf("\"%s\" %s --state-dir \"%s\"", qiyanBinary, HookMarker, stateDir)
}

// settingsFile is one .claude settings file, kept as a generic document so
// unknown user content survives a round trip.
type settingsFile struct {
	doc map[string]any
}

func loadSettings(path string) (*settingsFile, bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &settingsFile{doc: map[string]any{}}, false, nil
		}
		return nil, false, err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, false, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	return &settingsFile{doc: doc}, true, nil
}

func (f *settingsFile) hooksList() []any {
	hooks, ok := f.doc["hooks"].(map[string]any)
	if !ok {
		return nil
	}
	list, _ := hooks["PreToolUse"].([]any)
	return list
}

func (f *settingsFile) setHooksList(list []any) {
	if len(list) == 0 {
		if hooks, ok := f.doc["hooks"].(map[string]any); ok {
			delete(hooks, "PreToolUse")
			if len(hooks) == 0 {
				delete(f.doc, "hooks")
			}
		}
		return
	}
	hooks, ok := f.doc["hooks"].(map[string]any)
	if !ok {
		hooks = map[string]any{}
		f.doc["hooks"] = hooks
	}
	hooks["PreToolUse"] = list
}

// groupContainsQiyan reports whether a matcher group holds a Qiyan hook.
func groupContainsQiyan(group any) bool {
	g, ok := group.(map[string]any)
	if !ok {
		return false
	}
	entries, _ := g["hooks"].([]any)
	for _, e := range entries {
		if entryIsQiyan(e) {
			return true
		}
	}
	return false
}

func entryIsQiyan(entry any) bool {
	e, ok := entry.(map[string]any)
	if !ok {
		return false
	}
	cmd, _ := e["command"].(string)
	return strings.Contains(cmd, HookMarker) && strings.Contains(cmd, "qiyan")
}

func (f *settingsFile) hasQiyanHook() bool {
	for _, g := range f.hooksList() {
		if groupContainsQiyan(g) {
			return true
		}
	}
	return false
}

// qiyanGroup builds the hook group Qiyan installs: no matcher (every tool),
// a window of 10 seconds, and onFailure "block" so a dead or hung hook means
// the action does not run — on versions where that field has effect, which
// the probe has already established.
func qiyanGroup(hookCommand string) map[string]any {
	return map[string]any{
		"hooks": []any{
			map[string]any{
				"type":      "command",
				"command":   hookCommand,
				"timeout":   10,
				"onFailure": "block",
			},
		},
	}
}

// PlanInstall computes the change to <projectDir>/.claude/settings.local.json
// that adds the Qiyan hook. It performs no writes.
func PlanInstall(projectDir, qiyanBinary, stateDir string) (Plan, error) {
	settingsPath := filepath.Join(projectDir, ".claude", "settings.local.json")
	plan := Plan{
		ProjectDir:   projectDir,
		SettingsPath: settingsPath,
		BackupPath:   settingsPath + ".qiyan-backup",
		Uninstall:    false,
	}
	f, existed, err := loadSettings(settingsPath)
	if err != nil {
		return plan, err
	}
	plan.ExistedBefore = existed

	cmd := HookCommand(qiyanBinary, stateDir)
	if f.hasQiyanHook() {
		plan.NoChanges = true
		plan.Changes = append(plan.Changes, "the Qiyan PreToolUse hook is already installed; nothing to change")
		plan.NewContent = nil
		return plan, nil
	}

	f.setHooksList(append(f.hooksList(), qiyanGroup(cmd)))
	content, err := marshalSettings(f.doc)
	if err != nil {
		return plan, err
	}
	plan.NewContent = content
	action := "add"
	if existed {
		action = "add to the existing file"
	}
	plan.Changes = append(plan.Changes,
		fmt.Sprintf("%s: .claude/settings.local.json (%s)", action, settingsPath),
		fmt.Sprintf("hook command: %s", cmd),
		"applies to every tool (no matcher); timeout 10s; onFailure=block",
	)
	plan.Changes = append(plan.Changes, backupNote(existed, plan.BackupPath)...)
	return plan, nil
}

// PlanUninstall computes the removal of Qiyan's hook entries.
func PlanUninstall(projectDir string) (Plan, error) {
	settingsPath := filepath.Join(projectDir, ".claude", "settings.local.json")
	plan := Plan{
		ProjectDir:   projectDir,
		SettingsPath: settingsPath,
		BackupPath:   settingsPath + ".qiyan-backup",
		Uninstall:    true,
	}
	f, existed, err := loadSettings(settingsPath)
	if err != nil {
		return plan, err
	}
	plan.ExistedBefore = existed
	if !existed || !f.hasQiyanHook() {
		plan.NoChanges = true
		plan.Changes = append(plan.Changes, "no Qiyan hook is installed here; nothing to remove")
		return plan, nil
	}

	var kept []any
	removed := 0
	for _, g := range f.hooksList() {
		if !groupContainsQiyan(g) {
			kept = append(kept, g)
			continue
		}
		// A group could mix Qiyan and user entries; keep the others.
		gm, _ := g.(map[string]any)
		entries, _ := gm["hooks"].([]any)
		var others []any
		for _, e := range entries {
			if entryIsQiyan(e) {
				removed++
				continue
			}
			others = append(others, e)
		}
		if len(others) > 0 {
			gm["hooks"] = others
			kept = append(kept, gm)
		}
	}
	f.setHooksList(kept)
	content, err := marshalSettings(f.doc)
	if err != nil {
		return plan, err
	}
	plan.NewContent = content
	plan.Changes = append(plan.Changes,
		fmt.Sprintf("remove %d Qiyan hook entr%s from %s", removed, plural(removed, "y", "ies"), settingsPath),
		"everything else in the file is preserved",
	)
	if len(f.doc) == 0 {
		plan.Changes = append(plan.Changes, "the file becomes empty and will be removed")
	}
	return plan, nil
}

// Apply writes a plan. It backs the original file up once (before the first
// modification ever made), then replaces the file atomically.
func Apply(plan Plan) error {
	if plan.NoChanges {
		return nil
	}
	if plan.ExistedBefore {
		if _, err := os.Stat(plan.BackupPath); os.IsNotExist(err) {
			orig, err := os.ReadFile(plan.SettingsPath)
			if err != nil {
				return fmt.Errorf("backing up %s: %w", plan.SettingsPath, err)
			}
			if err := os.WriteFile(plan.BackupPath, orig, 0o600); err != nil {
				return fmt.Errorf("writing backup %s: %w", plan.BackupPath, err)
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(plan.SettingsPath), 0o700); err != nil {
		return err
	}
	// An uninstall that emptied the document removes the file we managed;
	// backups stay.
	if plan.Uninstall && isEffectivelyEmpty(plan.NewContent) {
		if err := os.Remove(plan.SettingsPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	tmp := plan.SettingsPath + ".tmp"
	if err := os.WriteFile(tmp, plan.NewContent, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, plan.SettingsPath)
}

func isEffectivelyEmpty(content []byte) bool {
	var doc map[string]any
	if err := json.Unmarshal(content, &doc); err != nil {
		return false
	}
	return len(doc) == 0
}

func marshalSettings(doc map[string]any) ([]byte, error) {
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func backupNote(existed bool, backupPath string) []string {
	if existed {
		return []string{fmt.Sprintf("a one-time backup of the original file is kept at %s", backupPath)}
	}
	return []string{"the file does not exist yet and will be created"}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
