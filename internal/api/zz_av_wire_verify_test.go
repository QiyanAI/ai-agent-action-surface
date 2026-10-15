package api

// Temporary adversarial-verification reproduction (delete after use).
// Wire-level: an agent host POSTs /v1/authorize declaring kind=session.local
// for a tool with an external effect (deploy_prod).

import (
	"encoding/json"
	"testing"

	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
)

func TestZZAVWireSessionLocalSpoof(t *testing.T) {
	for _, mode := range []string{"companion", "controlled_auto", "bounded_autonomy"} {
		f := newAPIFixture(t)

		resp, body := f.do(t, "POST", "/v1/agents", f.opTok, map[string]any{
			"name": "Host", "role": "builder", "runtime": "third-party-host",
		}, nil)
		if resp.StatusCode != 201 {
			t.Fatalf("[%s] agent create: %d %v", mode, resp.StatusCode, body)
		}
		agentRaw, _ := json.Marshal(body["agent"])
		var agent domain.Agent
		_ = json.Unmarshal(agentRaw, &agent)
		agTok, _ := body["token"].(string)

		resp, body = f.do(t, "POST", "/v1/missions", f.opTok, map[string]any{
			"goal": "deploy mission", "mode": mode,
			"scope": map[string]any{
				"roots":        []string{"C:/deploy"},
				"allowedKinds": []string{"session.local", "external.state_change", "process.exec"},
				"allowedTools": []string{"deploy_prod"},
				"maxSteps":     50, "maxRuntimeSeconds": 3600,
			},
		}, nil)
		if resp.StatusCode != 201 {
			t.Fatalf("[%s] mission create: %d %v", mode, resp.StatusCode, body)
		}
		missionID, _ := body["id"].(string)

		resp, body = f.do(t, "POST", "/v1/missions/"+missionID+"/tasks", f.opTok, map[string]any{
			"role": "builder", "title": "deploy", "agentId": string(agent.ID),
		}, nil)
		if resp.StatusCode != 201 {
			t.Fatalf("[%s] task create: %d %v", mode, resp.StatusCode, body)
		}
		taskID, _ := body["id"].(string)

		// Operator lifecycle step over the wire: bind a runtime session, which
		// (working tree) starts the task.
		resp, body = f.do(t, "POST", "/v1/sessions", f.opTok, map[string]any{
			"sessionId": "host-sess-1", "runtime": "third-party-host",
			"taskId": taskID, "agentId": string(agent.ID),
		}, nil)
		t.Logf("[%s] session bind: HTTP %d body=%v", mode, resp.StatusCode, body)
		_, tb := f.do(t, "GET", "/v1/tasks/"+taskID, f.opTok, nil, nil)
		t.Logf("[%s] task state after bind: %v", mode, tb["state"])

		// The spoof: an external effect declared as session bookkeeping.
		resp, spoofBody := f.do(t, "POST", "/v1/authorize", agTok, map[string]any{
			"missionId": missionID, "taskId": taskID,
			"action": map[string]any{"kind": "session.local", "tool": "deploy_prod", "raw": "publish release"},
		}, nil)
		t.Logf("[%s] SPOOF declare session.local via deploy_prod: HTTP %d body=%v", mode, resp.StatusCode, spoofBody)

		// The same tool with its true kind.
		resp, trueBody := f.do(t, "POST", "/v1/authorize", agTok, map[string]any{
			"missionId": missionID, "taskId": taskID,
			"action": map[string]any{"kind": "external.state_change", "tool": "deploy_prod", "url": "https://prod.example.com/release", "raw": "publish release"},
		}, nil)
		t.Logf("[%s] TRUE external.state_change via deploy_prod: HTTP %d body=%v", mode, resp.StatusCode, trueBody)

		// How many user prompts were raised in total?
		_, apBody := f.do(t, "GET", "/v1/approvals?state=pending", f.opTok, nil, nil)
		list, _ := apBody["approvals"].([]any)
		t.Logf("[%s] pending approvals raised: %d", mode, len(list))

		// Decisions recorded for this task.
		_, decBody := f.do(t, "GET", "/v1/decisions?task="+taskID, f.opTok, nil, nil)
		if ds, ok := decBody["decisions"].([]any); ok {
			for i, d := range ds {
				t.Logf("[%s] decision[%d]=%v", mode, i, d)
			}
		} else {
			t.Logf("[%s] decisions body=%v", mode, decBody)
		}

		if spoofBody["outcome"] != "ALLOW" {
			t.Errorf("[%s] spoof outcome = %v, want ALLOW", mode, spoofBody["outcome"])
		}
		if trueBody["outcome"] != "REQUIRE_APPROVAL" {
			t.Errorf("[%s] true-kind outcome = %v, want REQUIRE_APPROVAL", mode, trueBody["outcome"])
		}
	}
}
