package lab

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestScanManifestFindingsAndLimits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent-manifest.json")
	writeFile(t, path, `{
  "manifestVersion": "1",
  "name": "MailBot",
  "version": "0.1.0",
  "runtime": "custom",
  "tools": [
    {"name": "read_notes", "kind": "file.read", "description": "reads notes", "targets": ["notes/"]},
    {"name": "exec_shell", "kind": "process.exec", "description": "runs commands"},
    {"name": "send_mail", "kind": "network.access", "description": "emails people"},
    {"name": "use_key", "kind": "credential.use", "description": "reads the API key"}
  ]
}`)
	res, err := Scan(InputManifest, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) < 4 {
		t.Fatalf("expected per-tool findings, got %d", len(res.Findings))
	}
	for _, f := range res.Findings {
		if f.Level != domain.LevelDeclared {
			t.Errorf("manifest findings are declarations, got level %s", f.Level)
		}
		if f.SourceRef == "" || f.Boundary == "" {
			t.Errorf("every finding needs a source and an honest boundary: %+v", f)
		}
	}
	// Credential + network combination is flagged as a review item — without
	// claiming anything was actually sent.
	foundCombo := false
	for _, f := range res.Findings {
		if strings.Contains(f.Detail, "credential access and network access") {
			foundCombo = true
			if strings.Contains(f.Detail, "sent") || strings.Contains(f.Detail, "exfiltrat") {
				t.Fatal("the scan must not claim exfiltration it cannot see")
			}
		}
	}
	if !foundCombo {
		t.Fatal("credential+network combination should be surfaced")
	}
}

func TestScanMCPNeverLeaksEnvValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")
	writeFile(t, path, `{
  "mcpServers": {
    "mailer": {
      "command": "npx",
      "args": ["-y", "@some/mail-server"],
      "env": {"MAIL_API_KEY": "super-secret-value-123", "REGION": "eu"}
    },
    "remote": {"url": "https://mcp.example.com/v1?token=abc"}
  }
}`)
	res, err := Scan(InputMCPJSON, path)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(res)
	if strings.Contains(string(raw), "super-secret-value-123") {
		t.Fatal("environment variable VALUES must never appear in scan output")
	}
	if strings.Contains(string(raw), "token=abc") {
		t.Fatal("URL query strings (which may carry credentials) must be redacted")
	}
	if !strings.Contains(string(raw), "MAIL_API_KEY") {
		t.Fatal("variable NAMES are the useful, safe part and should appear")
	}
	sawExec, sawCred := false, false
	for _, f := range res.Findings {
		if f.Capability == domain.CapExecuteProcess {
			sawExec = true
		}
		if f.Capability == domain.CapUseCredentials {
			sawCred = true
		}
		if f.Level != domain.LevelStaticallyConnected {
			t.Errorf("MCP wiring is static evidence, got %s", f.Level)
		}
	}
	if !sawExec || !sawCred {
		t.Fatalf("expected exec and credential findings, got %+v", res.Findings)
	}
}

func TestScanClaudeProjectSuggestions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude", "settings.local.json"), `{
  "permissions": {"allow": ["Bash(npm test:*)"]}
}`)
	res, err := Scan(InputClaudeJSON, dir)
	if err != nil {
		t.Fatal(err)
	}
	var denySuggestion *Suggestion
	var connectSuggestion *Suggestion
	for i := range res.Suggestions {
		switch res.Suggestions[i].ID {
		case "add-baseline-denies":
			denySuggestion = &res.Suggestions[i]
		case "connect-qiyan":
			connectSuggestion = &res.Suggestions[i]
		}
	}
	if denySuggestion == nil || !denySuggestion.AutoApply {
		t.Fatal("a project without deny rules should get the auto-appliable baseline suggestion")
	}
	if connectSuggestion == nil {
		t.Fatal("a project without the Qiyan hook should get the connect suggestion")
	}

	// With deny rules and the hook present, both suggestions disappear.
	writeFile(t, filepath.Join(dir, ".claude", "settings.local.json"), `{
  "permissions": {"deny": ["Bash(rm:*)"]},
  "hooks": {"PreToolUse": [{"hooks": [{"type": "command", "command": "\"/x/qiyan\" hook pretooluse --state-dir \"/s\""}]}]}
}`)
	res2, err := Scan(InputClaudeJSON, dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range res2.Suggestions {
		if s.ID == "add-baseline-denies" || s.ID == "connect-qiyan" {
			t.Fatalf("suggestion %s should not appear for a project that already has it", s.ID)
		}
	}
}

func TestScanFailuresAreHonest(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "manifest.json")
	writeFile(t, bad, "{not json")
	if _, err := Scan(InputManifest, bad); domain.ClassOf(err) != domain.ErrInvalidRequest {
		t.Fatalf("bad JSON must be a clear invalid-request error, got %v", err)
	}
	if _, err := Scan(InputManifest, filepath.Join(dir, "missing.json")); domain.ClassOf(err) != domain.ErrNotFound {
		t.Fatalf("missing file must be NOT_FOUND, got %v", err)
	}
	if _, err := Scan("python-project", dir); domain.ClassOf(err) != domain.ErrInvalidRequest {
		t.Fatalf("unsupported input kinds must be refused explicitly, got %v", err)
	}
	// An empty directory has nothing to analyse and says so.
	if _, err := Scan(InputClaudeJSON, dir); domain.ClassOf(err) != domain.ErrNotFound {
		t.Fatalf("nothing-to-scan must be NOT_FOUND, got %v", err)
	}
}

func TestDenyPlanApplyAndUndo(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, ".claude", "settings.local.json")
	writeFile(t, settings, `{
  "permissions": {"allow": ["Bash(npm test:*)"]},
  "other": {"keep": true}
}`)

	plan, err := PlanAddDenyRules(dir, []string{"Bash(rm:*)", "Bash(curl:*)"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.NoChanges {
		t.Fatal("expected changes")
	}
	if err := ApplyDenyPlan(plan); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(settings)
	if !strings.Contains(string(raw), "Bash(rm:*)") || !strings.Contains(string(raw), "npm test") || !strings.Contains(string(raw), "\"keep\"") {
		t.Fatalf("apply must add deny rules and preserve everything else:\n%s", raw)
	}
	backup, err := os.ReadFile(settings + ".qiyan-backup")
	if err != nil || strings.Contains(string(backup), "Bash(rm:*)") {
		t.Fatalf("a pre-change backup must exist: %v", err)
	}

	// Idempotence.
	plan2, _ := PlanAddDenyRules(dir, []string{"Bash(rm:*)", "Bash(curl:*)"})
	if !plan2.NoChanges {
		t.Fatal("re-adding present rules must be a no-op")
	}

	unplan, err := PlanRemoveDenyRules(dir, []string{"Bash(rm:*)", "Bash(curl:*)"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyDenyPlan(unplan); err != nil {
		t.Fatal(err)
	}
	raw2, _ := os.ReadFile(settings)
	if strings.Contains(string(raw2), "Bash(rm:*)") {
		t.Fatal("undo must remove the rules")
	}
	if !strings.Contains(string(raw2), "npm test") || !strings.Contains(string(raw2), "\"keep\"") {
		t.Fatalf("undo must leave everything else alone:\n%s", raw2)
	}
}

func TestVerificationSessionMustComplete(t *testing.T) {
	// Regression: the probe used to ignore the command error and judge only
	// the filesystem — a session that never ran marked the rule "verified"
	// because its scratch file survived for the wrong reason.
	if ok, why := sessionCompleted(nil, "result text"); !ok {
		t.Fatalf("a completed session must be interpretable: %s", why)
	}
	if ok, why := sessionCompleted(errors.New("exit status 1"), "Error: cannot launch"); ok {
		t.Fatal("a failed session must not be judged as blocked")
	} else if !strings.Contains(why, "failed") {
		t.Fatalf("the reason must say the session failed: %q", why)
	}
	if ok, why := sessionCompleted(nil, "   \n"); ok {
		t.Fatal("an empty result must not be judged as blocked")
	} else if !strings.Contains(why, "no result") {
		t.Fatalf("the reason must say there was no result: %q", why)
	}
	// A session that reports its own error (is_error:true) proves nothing
	// about the rule, even with exit code 0.
	errText := `{"type":"result","is_error":true,"result":"API Error"}`
	if ok, why := sessionCompleted(nil, errText); ok {
		t.Fatal("an error result must not be judged as blocked")
	} else if !strings.Contains(why, "error result") {
		t.Fatalf("the reason must say the session reported an error: %q", why)
	}
}

func TestDenyRuleVerdictRequiresControlledComparison(t *testing.T) {
	ok := verifyLeg{complete: true, text: "…DONE…"}
	refused := verifyLeg{complete: true, text: "…BLOCKED…", refused: true}
	dead := verifyLeg{complete: false, why: "the verification session failed"}
	chat := verifyLeg{complete: true, text: "I would rather not run that."}

	// The honest positive: without the rule it ran, with it, refused.
	if v, _ := verdictFor("Bash(rm:*)", ok, refused, true, false, true); !v {
		t.Fatal("a controlled comparison with a real refusal must verify")
	}
	// The old defect's shape: the guarded leg was 'refused' only because the
	// environment refuses the command regardless — the control leg shows it.
	if v, why := verdictFor("Bash(rm:*)", refused, refused, false, false, true); v || !strings.Contains(why, "could not be isolated") {
		t.Fatalf("a non-discriminating environment must not verify: %v %q", v, why)
	}
	if v, _ := verdictFor("Bash(rm:*)", dead, refused, false, false, true); v {
		t.Fatal("an indeterminate control leg must not verify")
	}
	if v, _ := verdictFor("Bash(rm:*)", dead, dead, false, false, true); v {
		t.Fatal("two indeterminate legs must not verify")
	}
	if v, why := verdictFor("Bash(rm:*)", ok, chat, true, false, false); v || !strings.Contains(why, "no explicit refusal") {
		t.Fatalf("a chat-only guarded run must not verify: %v %q", v, why)
	}
	if v, why := verdictFor("Bash(rm:*)", ok, ok, true, true, false); v || !strings.Contains(why, "ran even with the rule") {
		t.Fatalf("a rule that did not refuse must be reported: %v %q", v, why)
	}
}
