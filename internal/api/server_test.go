package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/control"
	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
	"github.com/QiyanAI/ai-agent-action-surface/internal/store"
)

type apiFixture struct {
	srv     *httptest.Server
	api     *Server
	store   *store.Store
	opTok   string
	hookTok string
	agent   domain.Agent
	agTok   string
	mission domain.Mission
	task    domain.Task
}

func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	engine := control.New(st)
	f := &apiFixture{store: st, opTok: NewToken(), hookTok: NewToken()}
	f.api = NewServer(st, engine, "test", f.opTok, f.hookTok, t.TempDir())
	f.srv = httptest.NewServer(f.api.Handler())
	t.Cleanup(func() { f.srv.Close(); st.Close() })
	return f
}

func (f *apiFixture) do(t *testing.T, method, path, token string, body any, extra map[string]string) (*http.Response, map[string]any) {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rd = bytes.NewReader(raw)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, f.srv.URL+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "127.0.0.1"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	return resp, out
}

// setup registers a builder agent, a mission and a bound task.
func (f *apiFixture) setup(t *testing.T, mode string) {
	t.Helper()
	resp, body := f.do(t, "POST", "/v1/agents", f.opTok, map[string]any{
		"name": "Builder", "role": "builder", "runtime": "test",
	}, nil)
	if resp.StatusCode != 201 {
		t.Fatalf("agent create: %d %v", resp.StatusCode, body)
	}
	agentRaw, _ := json.Marshal(body["agent"])
	_ = json.Unmarshal(agentRaw, &f.agent)
	f.agTok, _ = body["token"].(string)

	resp, body = f.do(t, "POST", "/v1/missions", f.opTok, map[string]any{
		"goal": "test mission", "mode": mode,
		"scope": map[string]any{
			"roots":        []string{"C:/proj"},
			"allowedKinds": []string{"file.read", "file.modify", "process.exec", "session.local"},
			"allowedTools": []string{"Read", "Edit", "Bash", "Write", "Task"},
			"maxSteps":     50, "maxRuntimeSeconds": 3600,
		},
	}, nil)
	if resp.StatusCode != 201 {
		t.Fatalf("mission create: %d %v", resp.StatusCode, body)
	}
	missionRaw, _ := json.Marshal(body)
	_ = json.Unmarshal(missionRaw, &f.mission)

	resp, body = f.do(t, "POST", "/v1/missions/"+string(f.mission.ID)+"/tasks", f.opTok, map[string]any{
		"role": "builder", "title": "build it", "agentId": string(f.agent.ID),
	}, nil)
	if resp.StatusCode != 201 {
		t.Fatalf("task create: %d %v", resp.StatusCode, body)
	}
	taskRaw, _ := json.Marshal(body)
	_ = json.Unmarshal(taskRaw, &f.task)

	// Move the task to RUNNING and bind a session.
	f.task.State = domain.TaskRunning
	f.task.StartedAt = time.Now()
	if err := f.store.PutTask(f.task); err != nil {
		t.Fatal(err)
	}
	resp, body = f.do(t, "POST", "/v1/sessions", f.opTok, map[string]any{
		"sessionId": "sess-1", "runtime": "claude-code@2.1.300",
		"taskId": string(f.task.ID), "agentId": string(f.agent.ID),
	}, nil)
	if resp.StatusCode != 201 {
		t.Fatalf("session bind: %d %v", resp.StatusCode, body)
	}
}

func (f *apiFixture) hookDecide(t *testing.T, tool string, input map[string]any) (*http.Response, map[string]any) {
	t.Helper()
	return f.do(t, "POST", "/v1/hook/decide", f.hookTok, map[string]any{
		"sessionId": "sess-1", "toolName": tool, "toolInput": input, "cwd": "C:/proj",
	}, nil)
}

func TestHealthAndHostChecks(t *testing.T) {
	f := newAPIFixture(t)
	resp, body := f.do(t, "GET", "/v1/health", "", nil, nil)
	if resp.StatusCode != 200 || body["status"] != "ok" {
		t.Fatalf("health: %d %v", resp.StatusCode, body)
	}

	// A Host header that does not name loopback is refused before routing.
	req, _ := http.NewRequest("GET", f.srv.URL+"/v1/health", nil)
	req.Host = "evil.example.com"
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 403 {
		t.Fatalf("foreign Host must be refused, got %d", resp2.StatusCode)
	}

	// A cross-site Origin is refused even on reads.
	resp3, _ := f.do(t, "GET", "/v1/health", "", nil, map[string]string{"Origin": "https://evil.example.com"})
	if resp3.StatusCode != 403 {
		t.Fatalf("cross-origin must be refused, got %d", resp3.StatusCode)
	}
}

func TestAuthBoundaries(t *testing.T) {
	f := newAPIFixture(t)
	f.setup(t, "companion")

	if resp, _ := f.do(t, "GET", "/v1/agents", "", nil, nil); resp.StatusCode != 401 {
		t.Fatalf("unauthenticated list must 401, got %d", resp.StatusCode)
	}
	if resp, _ := f.do(t, "GET", "/v1/agents", f.agTok, nil, nil); resp.StatusCode != 401 {
		t.Fatalf("agent token must not read operator endpoints, got %d", resp.StatusCode)
	}
	if resp, _ := f.do(t, "GET", "/v1/agents", f.hookTok, nil, nil); resp.StatusCode != 401 {
		t.Fatalf("hook token must not read operator endpoints, got %d", resp.StatusCode)
	}
	if resp, _ := f.do(t, "POST", "/v1/hook/decide", f.opTok, map[string]any{
		"sessionId": "sess-1", "toolName": "Read", "toolInput": map[string]any{"file_path": "C:/proj/a.txt"}, "cwd": "C:/proj",
	}, nil); resp.StatusCode != 401 {
		t.Fatalf("operator token must not drive the hook channel, got %d", resp.StatusCode)
	}
}

func TestHookDecideAllowGateAndApprovalFlow(t *testing.T) {
	f := newAPIFixture(t)
	f.setup(t, "companion")

	// In-scope read: allowed.
	resp, body := f.hookDecide(t, "Read", map[string]any{"file_path": "C:/proj/a.txt"})
	if resp.StatusCode != 200 || body["outcome"] != "ALLOW" {
		t.Fatalf("in-scope read: %d %v", resp.StatusCode, body)
	}

	// Out-of-scope write: denied by scope, no approval offered.
	resp, body = f.hookDecide(t, "Edit", map[string]any{"file_path": "C:/elsewhere/a.txt"})
	if body["outcome"] != "DENY" {
		t.Fatalf("out-of-scope write must deny: %v", body)
	}

	// In-scope write in companion mode: approval required.
	resp, body = f.hookDecide(t, "Edit", map[string]any{"file_path": "C:/proj/src/a.go"})
	if resp.StatusCode != 200 || body["outcome"] != "REQUIRE_APPROVAL" {
		t.Fatalf("companion write must ask: %d %v", resp.StatusCode, body)
	}
	approvalID, _ := body["approvalId"].(string)
	if approvalID == "" {
		t.Fatalf("missing approvalId: %v", body)
	}

	// Pending approval is visible to the operator.
	resp, body = f.do(t, "GET", "/v1/approvals?state=pending", f.opTok, nil, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("approval list: %d", resp.StatusCode)
	}
	list, _ := body["approvals"].([]any)
	if len(list) != 1 {
		t.Fatalf("expected one pending approval, got %v", body)
	}

	// The agent cannot decide it.
	if resp, _ := f.do(t, "POST", "/v1/approvals/"+approvalID+"/decision", f.agTok, map[string]any{"approve": true}, nil); resp.StatusCode != 401 {
		t.Fatalf("agent must not decide approvals, got %d", resp.StatusCode)
	}

	// The operator approves; the retry succeeds; the next retry asks anew.
	if resp, body = f.do(t, "POST", "/v1/approvals/"+approvalID+"/decision", f.opTok, map[string]any{"approve": true}, nil); resp.StatusCode != 200 {
		t.Fatalf("approval decision failed: %d %v", resp.StatusCode, body)
	}
	resp, body = f.hookDecide(t, "Edit", map[string]any{"file_path": "C:/proj/src/a.go"})
	if body["outcome"] != "ALLOW" {
		t.Fatalf("approved retry must allow: %v", body)
	}
	resp, body = f.hookDecide(t, "Edit", map[string]any{"file_path": "C:/proj/src/a.go"})
	if body["outcome"] != "REQUIRE_APPROVAL" {
		t.Fatalf("consumed grant must re-ask: %v", body)
	}
}

func TestUnboundSessionCannotDecide(t *testing.T) {
	f := newAPIFixture(t)
	f.setup(t, "companion")
	resp, _ := f.do(t, "POST", "/v1/hook/decide", f.hookTok, map[string]any{
		"sessionId": "sess-unknown", "toolName": "Read",
		"toolInput": map[string]any{"file_path": "C:/proj/a.txt"}, "cwd": "C:/proj",
	}, nil)
	if resp.StatusCode != 403 {
		t.Fatalf("unbound session must be refused, got %d", resp.StatusCode)
	}
}

func TestAgentAuthorizePath(t *testing.T) {
	f := newAPIFixture(t)
	f.setup(t, "controlled_auto")

	// The bound agent gets a decision for its own task.
	resp, body := f.do(t, "POST", "/v1/authorize", f.agTok, map[string]any{
		"missionId": string(f.mission.ID), "taskId": string(f.task.ID),
		"action": map[string]any{"kind": "file.modify", "tool": "note.append", "path": "C:/proj/n.txt"},
	}, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("authorize: %d %v", resp.StatusCode, body)
	}
	// note.append is not in the tool allowlist -> deny.
	if body["outcome"] != "DENY" {
		t.Fatalf("tool outside allowlist must deny: %v", body)
	}

	resp, body = f.do(t, "POST", "/v1/authorize", f.agTok, map[string]any{
		"missionId": string(f.mission.ID), "taskId": string(f.task.ID),
		"action": map[string]any{"kind": "file.read", "tool": "Read", "path": "C:/proj/n.txt"},
	}, nil)
	if body["outcome"] != "ALLOW" {
		t.Fatalf("in-scope read must allow: %v", body)
	}

	// A second registered agent cannot act as this task's agent.
	resp, body2 := f.do(t, "POST", "/v1/agents", f.opTok, map[string]any{"name": "Other", "role": "builder"}, nil)
	if resp.StatusCode != 201 {
		t.Fatal("second agent registration failed")
	}
	otherTok, _ := body2["token"].(string)
	resp, body = f.do(t, "POST", "/v1/authorize", otherTok, map[string]any{
		"missionId": string(f.mission.ID), "taskId": string(f.task.ID),
		"action": map[string]any{"kind": "file.read", "tool": "Read", "path": "C:/proj/n.txt"},
	}, nil)
	if body["outcome"] != "DENY" {
		t.Fatalf("impersonation must deny: %v", body)
	}
}

func TestResultEventsAndOwnership(t *testing.T) {
	f := newAPIFixture(t)
	f.setup(t, "controlled_auto")

	resp, _ := f.do(t, "POST", "/v1/events", f.agTok, map[string]any{
		"taskId": string(f.task.ID), "actionDigest": "abc", "result": "executed", "note": "done",
	}, nil)
	if resp.StatusCode != 201 {
		t.Fatalf("result event: %d", resp.StatusCode)
	}
	if resp, _ := f.do(t, "POST", "/v1/events", f.agTok, map[string]any{
		"taskId": string(f.task.ID), "actionDigest": "abc", "result": "exploded",
	}, nil); resp.StatusCode != 400 {
		t.Fatalf("invalid result must 400, got %d", resp.StatusCode)
	}
	events, err := f.store.ResultsForTask(f.task.ID)
	if err != nil || len(events) != 1 || events[0].Result != "executed" {
		t.Fatalf("result not stored: %v %v", events, err)
	}
}

func TestStrictJSONAndSizeLimit(t *testing.T) {
	f := newAPIFixture(t)
	f.setup(t, "companion")

	// Unknown fields are rejected (a typo'd field must not silently vanish).
	raw := bytes.NewBufferString(`{"name":"X","role":"builder","runtim":"typo"}`)
	req, _ := http.NewRequest("POST", f.srv.URL+"/v1/agents", raw)
	req.Host = "127.0.0.1"
	req.Header.Set("Authorization", "Bearer "+f.opTok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("unknown field must 400, got %d", resp.StatusCode)
	}

	// Oversized bodies are cut off by MaxBytesReader.
	big := bytes.Repeat([]byte("a"), maxBodyBytes+1024)
	req2, _ := http.NewRequest("POST", f.srv.URL+"/v1/agents", bytes.NewReader(big))
	req2.Host = "127.0.0.1"
	req2.Header.Set("Authorization", "Bearer "+f.opTok)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 400 && resp2.StatusCode != 413 {
		t.Fatalf("oversized body must be refused, got %d", resp2.StatusCode)
	}
}

func TestUISessionAndCSRF(t *testing.T) {
	f := newAPIFixture(t)

	// Wrong bootstrap token refused.
	if resp, _ := f.do(t, "POST", "/v1/ui/session", "", map[string]any{"token": "nope"}, nil); resp.StatusCode != 401 {
		t.Fatalf("bad bootstrap must 401, got %d", resp.StatusCode)
	}

	// Correct bootstrap sets a cookie and returns a CSRF token.
	req, _ := http.NewRequest("POST", f.srv.URL+"/v1/ui/session", bytes.NewReader(mustJSON(t, map[string]any{"token": f.opTok})))
	req.Host = "127.0.0.1"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var cookies []*http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == uiCookieName {
			cookies = append(cookies, c)
		}
	}
	var body map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&body)
	resp.Body.Close()
	if len(cookies) != 1 || body["csrf"] == "" {
		t.Fatalf("bootstrap did not set cookie/csrf: %v %v", cookies, body)
	}

	// Cookie without CSRF header cannot mutate.
	req2, _ := http.NewRequest("POST", f.srv.URL+"/v1/agents", bytes.NewReader(mustJSON(t, map[string]any{"name": "X", "role": "builder"})))
	req2.Host = "127.0.0.1"
	req2.AddCookie(cookies[0])
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 403 {
		t.Fatalf("cookie POST without CSRF must 403, got %d", resp2.StatusCode)
	}

	// With the CSRF header it works.
	req3, _ := http.NewRequest("POST", f.srv.URL+"/v1/agents", bytes.NewReader(mustJSON(t, map[string]any{"name": "X", "role": "builder"})))
	req3.Host = "127.0.0.1"
	req3.AddCookie(cookies[0])
	req3.Header.Set("X-Qiyan-CSRF", body["csrf"])
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != 201 {
		t.Fatalf("CSRF-authorized POST should succeed, got %d", resp3.StatusCode)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestUIServesEmbeddedConsole(t *testing.T) {
	f := newAPIFixture(t)

	resp, err := http.Get(f.srv.URL + "/ui/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("console index must serve, got %d", resp.StatusCode)
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") {
		t.Fatalf("console must carry a CSP forbidding inline/external script, got %q", csp)
	}
	if !strings.Contains(string(body), "Qiyan") {
		t.Fatal("index content missing")
	}
	if !strings.Contains(string(body), "i18n.js") {
		t.Fatal("i18n bundle must be referenced")
	}

	resp2, err := http.Get(f.srv.URL + "/ui/does-not-exist.js")
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 404 {
		t.Fatalf("missing asset must 404, got %d", resp2.StatusCode)
	}
}
