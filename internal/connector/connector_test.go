package connector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseVersionTiers(t *testing.T) {
	cases := []struct {
		out   string
		ver   string
		tier  SupportTier
		isErr bool
	}{
		{"2.1.300 (Claude Code)", "2.1.300", TierEnforced, false},
		{"2.1.295 (Claude Code)", "2.1.295", TierEnforced, false},
		{"2.1.294 (Claude Code)", "2.1.294", TierBlockingOnly, false},
		{"2.1.292 (Claude Code)", "2.1.292", TierBlockingOnly, false},
		{"2.1.68 (Claude Code)", "2.1.68", TierBlockingOnly, false},
		{"3.0.0", "3.0.0", TierEnforced, false},
		{"no version here", "", TierUnsupported, true},
	}
	for _, c := range cases {
		ver, tier, err := ParseVersion(c.out)
		if c.isErr {
			if err == nil {
				t.Errorf("%q: expected an error", c.out)
			}
			continue
		}
		if err != nil || ver != c.ver || tier != c.tier {
			t.Errorf("%q: got (%s, %s, %v), want (%s, %s)", c.out, ver, tier, err, c.ver, c.tier)
		}
	}
}

func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return doc
}

func TestSettingsInstallPreservesUserContentAndUndoes(t *testing.T) {
	project := t.TempDir()
	claudeDir := filepath.Join(project, ".claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.local.json")

	// The user already has content: their own hook group and an unrelated key.
	original := `{
  "permissions": {"allow": ["Bash(npm test:*)"]},
  "hooks": {
    "PreToolUse": [
      {"matcher": "Write", "hooks": [{"type": "command", "command": "echo user-hook"}]}
    ]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanInstall(project, "/opt/qiyan/qiyan", "/home/u/.qiyan")
	if err != nil {
		t.Fatal(err)
	}
	if plan.NoChanges || len(plan.Changes) == 0 {
		t.Fatalf("expected a plan with changes, got %+v", plan)
	}
	if err := Apply(plan); err != nil {
		t.Fatal(err)
	}

	doc := readJSONFile(t, settingsPath)
	if _, ok := doc["permissions"]; !ok {
		t.Fatal("unrelated keys must survive the install")
	}
	hooks := doc["hooks"].(map[string]any)
	groups := hooks["PreToolUse"].([]any)
	if len(groups) != 2 {
		t.Fatalf("expected the user group plus Qiyan's, got %d", len(groups))
	}
	// The backup exists, byte-identical to the original.
	backup, err := os.ReadFile(settingsPath + ".qiyan-backup")
	if err != nil || string(backup) != original {
		t.Fatalf("backup must exist and match the original exactly: %v", err)
	}

	// Idempotence: a second plan changes nothing.
	plan2, err := PlanInstall(project, "/opt/qiyan/qiyan", "/home/u/.qiyan")
	if err != nil {
		t.Fatal(err)
	}
	if !plan2.NoChanges {
		t.Fatal("installing twice must be a no-op")
	}

	// Undo removes exactly Qiyan's entries.
	unplan, err := PlanUninstall(project)
	if err != nil {
		t.Fatal(err)
	}
	if unplan.NoChanges {
		t.Fatal("uninstall should have work to do")
	}
	if err := Apply(unplan); err != nil {
		t.Fatal(err)
	}
	after := readJSONFile(t, settingsPath)
	if _, ok := after["permissions"]; !ok {
		t.Fatal("unrelated keys must survive the uninstall")
	}
	hooksAfter, _ := after["hooks"].(map[string]any)
	groupsAfter, _ := hooksAfter["PreToolUse"].([]any)
	if len(groupsAfter) != 1 {
		t.Fatalf("user's own group must remain, got %d groups", len(groupsAfter))
	}
	if strings.Contains(string(mustRaw(t, settingsPath)), HookMarker) {
		t.Fatal("no Qiyan hook may remain")
	}
}

func TestSettingsInstallOnFreshProjectAndUndoRemovesFile(t *testing.T) {
	project := t.TempDir()
	plan, err := PlanInstall(project, "/opt/qiyan/qiyan", "/state")
	if err != nil {
		t.Fatal(err)
	}
	if plan.ExistedBefore {
		t.Fatal("fresh project should not have a settings file")
	}
	if err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plan.SettingsPath); err != nil {
		t.Fatal("settings file should now exist")
	}
	unplan, err := PlanUninstall(project)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(unplan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(plan.SettingsPath); !os.IsNotExist(err) {
		t.Fatal("an emptied file we managed should be removed on undo")
	}
}

func TestMixedGroupKeepsUserEntries(t *testing.T) {
	project := t.TempDir()
	claudeDir := filepath.Join(project, ".claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.local.json")
	mixed := `{
  "hooks": {
    "PreToolUse": [
      {"hooks": [
        {"type": "command", "command": "qiyan hook pretooluse --state-dir /x"},
        {"type": "command", "command": "echo keep-me"}
      ]}
    ]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(mixed), 0o600); err != nil {
		t.Fatal(err)
	}
	unplan, err := PlanUninstall(project)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(unplan); err != nil {
		t.Fatal(err)
	}
	raw := string(mustRaw(t, settingsPath))
	if strings.Contains(raw, HookMarker) {
		t.Fatal("Qiyan entry must be gone")
	}
	if !strings.Contains(raw, "keep-me") {
		t.Fatal("the user's entry inside the same group must be kept")
	}
}

func mustRaw(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
