package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/api"
	"github.com/QiyanAI/ai-agent-action-surface/internal/control"
	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
	"github.com/QiyanAI/ai-agent-action-surface/internal/store"
)

// These tests exercise the hook entry point exactly as the agent runtime will
// call it: real stdin JSON in, real exit codes out, against a live API
// server. They are the automated form of the R02-00 probe discipline — exit 0
// only on a real ALLOW, exit 2 for everything else including failures.

type hookFixture struct {
	dir     string
	server  *httptest.Server
	store   *store.Store
	agent   domain.Agent
	task    domain.Task
	mission domain.Mission
}

func newHookFixture(t *testing.T, mode string) *hookFixture {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	engine := control.New(st)
	operatorTok := api.NewToken()
	hookTok := api.NewToken()
	srv := api.NewServer(st, engine, "test", operatorTok, hookTok, dir)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() { ts.Close(); st.Close() })

	// server.json + hook.token as `qiyan serve` would write them.
	addr := strings.TrimPrefix(ts.URL, "http://")
	infoRaw, _ := json.Marshal(serverInfo{Addr: addr})
	if err := os.WriteFile(filepath.Join(dir, "server.json"), infoRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeSecret(filepath.Join(dir, "hook.token"), hookTok); err != nil {
		t.Fatal(err)
	}

	f := &hookFixture{dir: dir, server: ts, store: st}
	f.agent = domain.Agent{ID: domain.NewAgentID(), Name: "Builder", Role: domain.RoleBuilder, RegisteredAt: time.Now()}
	if err := st.RegisterAgent(f.agent, "unused-hash"); err != nil {
		t.Fatal(err)
	}
	scope := domain.Scope{
		Roots:        []string{"C:/proj"},
		AllowedKinds: []domain.ActionKind{domain.ActionFileRead, domain.ActionFileModify, domain.ActionProcessExec, domain.ActionSessionLocal},
		AllowedTools: []string{"Read", "Edit", "Bash", "TodoWrite"},
		MaxSteps:     50, MaxRuntime: time.Hour,
	}
	f.mission = domain.Mission{ID: domain.NewMissionID(), Goal: "g",
		Policy: domain.Policy{Mode: domain.AutonomyMode(mode), Scope: scope}, State: domain.MissionRunning}
	if err := st.PutMission(f.mission); err != nil {
		t.Fatal(err)
	}
	f.task = domain.Task{ID: domain.NewTaskID(), MissionID: f.mission.ID, Role: domain.RoleBuilder,
		State: domain.TaskRunning, AgentID: f.agent.ID, StartedAt: time.Now()}
	if err := st.PutTask(f.task); err != nil {
		t.Fatal(err)
	}
	if err := st.BindSession(store.SessionBinding{
		SessionID: "sess-test", Runtime: "claude-code@2.1.300",
		MissionID: f.mission.ID, TaskID: f.task.ID, AgentID: f.agent.ID, BoundAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	return f
}

// hookStdin builds the hook's stdin payload.
func (f *hookFixture) hookStdin(t *testing.T, tool string, input map[string]any) *bytes.Reader {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"session_id":      "sess-test",
		"hook_event_name": "PreToolUse",
		"tool_name":       tool,
		"tool_input":      input,
		"cwd":             "C:/proj",
	})
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(raw)
}

func (f *hookFixture) run(t *testing.T, tool string, input map[string]any) (string, string, int) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code := runHook([]string{"pretooluse", "--state-dir", f.dir}, f.hookStdin(t, tool, input), &out, &errBuf)
	return out.String(), errBuf.String(), code
}

func TestHookAllowsInScopeRead(t *testing.T) {
	f := newHookFixture(t, "companion")
	out, errOut, code := f.run(t, "Read", map[string]any{"file_path": "C:/proj/a.txt"})
	if code != 0 {
		t.Fatalf("in-scope read must exit 0, got %d (stderr: %s)", code, errOut)
	}
	if !strings.Contains(out, `"permissionDecision":"allow"`) {
		t.Fatalf("stdout must carry the allow decision: %q", out)
	}
}

func TestHookBlocksOutOfScopeWrite(t *testing.T) {
	f := newHookFixture(t, "controlled_auto")
	out, errOut, code := f.run(t, "Edit", map[string]any{"file_path": "C:/elsewhere/evil.txt"})
	if code != 2 {
		t.Fatalf("out-of-scope write must exit 2, got %d (stdout: %s)", code, out)
	}
	if !strings.Contains(errOut, "Qiyan blocked") {
		t.Fatalf("stderr must explain the block: %q", errOut)
	}
	if !strings.Contains(errOut, "Qiyan 阻止") {
		t.Fatalf("stderr must carry the Chinese line too: %q", errOut)
	}
}

func TestHookBlocksPendingApprovalWithGuidance(t *testing.T) {
	f := newHookFixture(t, "companion")
	out, errOut, code := f.run(t, "Edit", map[string]any{"file_path": "C:/proj/src/a.go"})
	if code != 2 {
		t.Fatalf("companion write must exit 2 while awaiting approval, got %d (stdout %s)", code, out)
	}
	// The message names the approval so a person can act on it.
	list, err := f.store.ListApprovals(domain.ApprovalPending, "")
	if err != nil || len(list) != 1 {
		t.Fatalf("pending approval missing: %v %v", list, err)
	}
	if !strings.Contains(errOut, string(list[0].ID)) {
		t.Fatalf("stderr must name the approval id %s: %q", list[0].ID, errOut)
	}
	if !strings.Contains(errOut, "等待") {
		t.Fatalf("awaiting-approval message must be bilingual: %q", errOut)
	}
}

func TestHookFailsClosedWithoutService(t *testing.T) {
	f := newHookFixture(t, "companion")
	// Remove the service info: no decision can be obtained.
	if err := os.Remove(filepath.Join(f.dir, "server.json")); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := f.run(t, "Read", map[string]any{"file_path": "C:/proj/a.txt"})
	if code != 2 {
		t.Fatalf("missing service must exit 2 (fail-closed), got %d", code)
	}
	if out != "" {
		t.Fatalf("no allow output may be produced, got %q", out)
	}
	if !strings.Contains(errOut, "fail-closed") && !strings.Contains(errOut, "失败即拒绝") {
		t.Fatalf("stderr must name the fail-closed rule: %q", errOut)
	}
}

func TestHookIgnoresForeignEvents(t *testing.T) {
	f := newHookFixture(t, "companion")
	raw := bytes.NewReader([]byte(`{"hook_event_name":"PostToolUse","session_id":"sess-test"}`))
	var out, errBuf bytes.Buffer
	code := runHook([]string{"pretooluse", "--state-dir", f.dir}, raw, &out, &errBuf)
	if code != 0 || out.Len() != 0 || errBuf.Len() != 0 {
		t.Fatalf("foreign events must pass through silently, got code=%d out=%q err=%q", code, out.String(), errBuf.String())
	}
}

func TestHookBlocksUnboundSession(t *testing.T) {
	f := newHookFixture(t, "companion")
	raw := bytes.NewReader([]byte(`{"hook_event_name":"PreToolUse","session_id":"sess-intruder","tool_name":"Read","tool_input":{"file_path":"C:/proj/a.txt"},"cwd":"C:/proj"}`))
	var out, errBuf bytes.Buffer
	code := runHook([]string{"pretooluse", "--state-dir", f.dir}, raw, &out, &errBuf)
	if code != 2 {
		t.Fatalf("unbound session must exit 2, got %d", code)
	}
	if !strings.Contains(errBuf.String(), "not bound") {
		t.Fatalf("stderr should explain the unbound session: %q", errBuf.String())
	}
}

func TestHookApprovalRoundTrip(t *testing.T) {
	f := newHookFixture(t, "companion")
	_, errOut, code := f.run(t, "Edit", map[string]any{"file_path": "C:/proj/src/a.go"})
	if code != 2 {
		t.Fatalf("expected pending approval, got %d", code)
	}
	_ = errOut
	list, _ := f.store.ListApprovals(domain.ApprovalPending, "")
	if len(list) != 1 {
		t.Fatalf("expected one approval, got %d", len(list))
	}
	// Approve directly through the engine (the operator path is covered by
	// the API tests).
	if _, err := control.New(f.store).Decide(list[0].ID, true, domain.Actor{Source: domain.SourceLocalUser, Label: "t"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	out, errOut2, code := f.run(t, "Edit", map[string]any{"file_path": "C:/proj/src/a.go"})
	if code != 0 {
		t.Fatalf("approved retry must exit 0, got %d (%s)", code, errOut2)
	}
	if !strings.Contains(out, "allow") {
		t.Fatalf("allow output expected: %q", out)
	}
}
