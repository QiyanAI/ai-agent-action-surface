// Package zzprobe is a temporary adversarial-verification probe (not part of
// the product). It drives the real control engine over the real file-backed
// store, mirroring api.Server.evaluate: each "request" loads its own Task
// snapshot, then evaluates. It checks whether two evaluations that loaded the
// same pre-increment snapshot can both be allowed and push StepsUsed past
// MaxSteps.
package zzprobe

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/QiyanAI/ai-agent-action-surface/internal/api"
	"github.com/QiyanAI/ai-agent-action-surface/internal/control"
	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
	"github.com/QiyanAI/ai-agent-action-surface/internal/store"
)

const maxSteps = 50

func setup(t *testing.T) (*store.Store, control.Engine, domain.Agent, domain.Mission, domain.Task) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ag := domain.Agent{ID: domain.NewAgentID(), Name: "Builder", Role: domain.RoleBuilder}
	if err := st.RegisterAgent(ag, "hash"); err != nil {
		t.Fatal(err)
	}
	m := domain.Mission{
		ID: domain.NewMissionID(), Goal: "g", State: domain.MissionRunning,
		Policy: domain.Policy{Mode: domain.ModeBoundedAutonomy, Scope: domain.Scope{
			Roots:        []string{"C:/proj"},
			AllowedKinds: []domain.ActionKind{domain.ActionFileRead, domain.ActionFileModify},
			AllowedTools: []string{"Read", "Edit"},
			MaxSteps:     maxSteps,
			MaxRuntime:   time.Hour,
		}},
	}
	if err := st.PutMission(m); err != nil {
		t.Fatal(err)
	}
	// The production store's single source of truth: the stored task.
	task := domain.Task{
		ID: domain.NewTaskID(), MissionID: m.ID, Role: domain.RoleBuilder,
		State: domain.TaskRunning, AgentID: ag.ID,
		StepsUsed: maxSteps - 1, // one step of budget left
		StartedAt: time.Now(),
	}
	if err := st.PutTask(task); err != nil {
		t.Fatal(err)
	}
	return st, *control.New(st), ag, m, task
}

func req(m domain.Mission, ag domain.Agent, tk domain.Task, a domain.Action) control.Request {
	return control.Request{
		Mission: m, Task: tk, Agent: ag, Action: a,
		Agents: []domain.Agent{ag}, Now: time.Now(),
	}
}

// TestZZProbeSnapshotOvershoot: deterministic reproduction of the interleaving
// two concurrent handlers produce when both load the task before either
// increments. This mirrors api.Server.evaluate lines 612-641 exactly:
// GetTask -> build Request -> Engine.Evaluate.
func TestZZProbeSnapshotOvershoot(t *testing.T) {
	st, eng, ag, m, task := setup(t)
	defer st.Close()

	// Handler 1 loads the task snapshot.
	snap1, ok, err := st.GetTask(task.ID)
	if err != nil || !ok {
		t.Fatalf("GetTask 1: %v ok=%v", err, ok)
	}
	// Handler 2 loads the task snapshot (before handler 1 has evaluated).
	snap2, ok, err := st.GetTask(task.ID)
	if err != nil || !ok {
		t.Fatalf("GetTask 2: %v ok=%v", err, ok)
	}
	if snap1.StepsUsed != maxSteps-1 || snap2.StepsUsed != maxSteps-1 {
		t.Fatalf("precondition: both snapshots must show %d used, got %d and %d",
			maxSteps-1, snap1.StepsUsed, snap2.StepsUsed)
	}

	// Both handlers now evaluate concurrently (the snapshot is all the engine
	// checks; the store is only touched again by IncrementSteps).
	d1, err1 := eng.Evaluate(req(m, ag, snap1, domain.Action{Kind: domain.ActionFileRead, Tool: "Read", Path: "C:/proj/a.txt"}))
	d2, err2 := eng.Evaluate(req(m, ag, snap2, domain.Action{Kind: domain.ActionFileModify, Tool: "Edit", Path: "C:/proj/b.txt"}))

	final, _, err := st.GetTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("d1=%s (%v), d2=%s (%v), final StepsUsed=%d, MaxSteps=%d",
		d1.Outcome, err1, d2.Outcome, err2, final.StepsUsed, maxSteps)

	if d1.Outcome != domain.OutcomeAllow || d2.Outcome != domain.OutcomeAllow {
		t.Fatalf("NO OVERSHOOT: an evaluation observed live state (d1=%s d2=%s)", d1.Outcome, d2.Outcome)
	}
	if final.StepsUsed <= maxSteps {
		t.Fatalf("NO OVERSHOOT: final StepsUsed=%d <= MaxSteps=%d", final.StepsUsed, maxSteps)
	}
	t.Logf("OVERSHOOT CONFIRMED: both actions ALLOWed, %d steps consumed against a %d-step budget",
		final.StepsUsed, maxSteps)
}

// TestZZProbeParallelOvershoot: the claimed scenario arithmetic, 8 handlers,
// two-phase. Phase 1: all 8 handlers load their own task snapshot (this is
// the interleaving the code permits — nothing between load and increment
// excludes it). Phase 2: all 8 evaluate concurrently, exactly as claimed:
// "N handlers load StepsUsed=49, all pass the check, all are ALLOWed, all
// increment". Run under -race too: the store is mutex-clean; this is a
// logical TOCTOU, not a data race.
func TestZZProbeParallelOvershoot(t *testing.T) {
	st, eng, ag, m, task := setup(t)
	defer st.Close()

	const workers = 8
	snaps := make([]domain.Task, workers)
	loadErr := make([]error, workers)
	var loaded sync.WaitGroup
	var evaluated sync.WaitGroup
	results := make([]domain.Outcome, workers)
	evalErr := make([]error, workers)

	for i := 0; i < workers; i++ {
		loaded.Add(1)
		evaluated.Add(1)
		go func(i int) {
			defer evaluated.Done()
			snap, ok, err := st.GetTask(task.ID)
			if err != nil || !ok {
				loadErr[i] = err
			}
			snaps[i] = snap
			loaded.Done() // phase 1 done for this handler
			loaded.Wait() // wait until every handler has loaded
			a := domain.Action{Kind: domain.ActionFileModify, Tool: "Edit",
				Path: "C:/proj/f" + string(rune('a'+i)) + ".txt"}
			d, err := eng.Evaluate(req(m, ag, snaps[i], a))
			results[i], evalErr[i] = d.Outcome, err
		}(i)
	}
	evaluated.Wait()

	final, _, err := st.GetTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	allowed := 0
	for i := range results {
		if loadErr[i] != nil || evalErr[i] != nil {
			t.Fatalf("worker %d: load=%v eval=%v", i, loadErr[i], evalErr[i])
		}
		if results[i] == domain.OutcomeAllow {
			allowed++
		}
	}
	t.Logf("two-phase parallel: %d/%d ALLOWed, final StepsUsed=%d (budget %d, started at %d)",
		allowed, workers, final.StepsUsed, maxSteps, maxSteps-1)
	if allowed != workers {
		t.Fatalf("NO OVERSHOOT: only %d/%d allowed", allowed, workers)
	}
	if final.StepsUsed <= maxSteps {
		t.Fatalf("NO OVERSHOOT: final StepsUsed=%d <= %d", final.StepsUsed, maxSteps)
	}
	t.Logf("OVERSHOOT CONFIRMED: %d controlled actions executed against a %d-step budget (used=%d)",
		allowed, maxSteps, final.StepsUsed)
}

// TestZZProbeHTTP fires the real handler stack: httptest server, hook token,
// bound session, 8 simultaneous POST /v1/hook/decide -- one per parallel tool
// call, exactly the claimed vector. It logs the observed overshoot; the
// deterministic tests above carry the proof if scheduling refuses to
// interleave on a given run.
func TestZZProbeHTTP(t *testing.T) {
	st, eng, ag, _, task := setup(t)
	defer st.Close()

	srv := api.NewServer(st, eng, "probe", "optok-probe", "hooktok-probe")
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	post := func(path, token string, body any) (int, []byte) {
		raw, _ := json.Marshal(body)
		req, err := http.NewRequest("POST", ts.URL+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		out, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, out
	}

	// Bind one runtime session to the task (operator credential).
	code, out := post("/v1/sessions", "optok-probe", map[string]any{
		"sessionId": "sess-probe", "runtime": "claude-code",
		"taskId": string(task.ID), "agentId": string(ag.ID),
	})
	if code != http.StatusCreated {
		t.Fatalf("session bind: %d %s", code, out)
	}

	// 8 parallel hook decisions for the same session/task, fired together.
	const workers = 8
	release := make(chan struct{})
	codes := make([]int, workers)
	bodies := make([][]byte, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-release
			codes[i], bodies[i] = post("/v1/hook/decide", "hooktok-probe", map[string]any{
				"sessionId": "sess-probe",
				"toolName":  "Edit",
				"toolInput": map[string]any{"file_path": fmt.Sprintf("C:/proj/par%d.txt", i)},
				"cwd":       "C:/proj",
			})
		}(i)
	}
	close(release)
	wg.Wait()

	final, _, err := st.GetTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	allowed := 0
	for i := 0; i < workers; i++ {
		if codes[i] != http.StatusOK {
			t.Errorf("worker %d: HTTP %d %s", i, codes[i], bodies[i])
			continue
		}
		var dec struct {
			Outcome string `json:"outcome"`
		}
		if err := json.Unmarshal(bodies[i], &dec); err != nil {
			t.Errorf("worker %d: bad body %s", i, bodies[i])
			continue
		}
		if dec.Outcome == "ALLOW" {
			allowed++
		}
	}
	t.Logf("HTTP parallel: %d/%d ALLOWed, final StepsUsed=%d (budget %d, started at %d)",
		allowed, workers, final.StepsUsed, maxSteps, maxSteps-1)
	if final.StepsUsed > maxSteps {
		t.Logf("OVERSHOOT CONFIRMED VIA HTTP: %d reviews vs %d budget", final.StepsUsed, maxSteps)
	}
}
