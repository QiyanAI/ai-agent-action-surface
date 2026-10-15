// Command qiyanhost is the independent reference Agent Host.
//
// It exists to prove one thing: a third-party agent can obtain Qiyan's full
// control capability — authorization decisions, human approvals, result
// reporting, evidence — through the public versioned Control API alone. It
// therefore imports NOTHING from this repository's internal packages; it is a
// plain HTTP client, exactly like any external integration would be.
//
// Its discipline is the point: every action it performs is sent to
// /v1/authorize first, and it obeys the answer. ALLOW runs the action,
// DENY skips it, REQUIRE_APPROVAL waits for the user's decision and then
// re-asks. The `--rogue` flag deliberately performs one action WITHOUT
// asking — so verification can show that the system does not mark it as
// protected: there is no decision for it, and coverage claims nothing.
//
// All actions are harmless and local (files in the host's own workdir; the
// "external send" writes a local outbox line and says so). Nothing here
// deletes user data, sends real network traffic, or touches anything outside
// its scratch directory.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const usage = `qiyanhost - independent reference Agent Host (public Control API only)

Usage:
  qiyanhost --addr HOST:PORT --token TOKEN --task TASK --mission MISSION --workdir DIR [--rogue] [--json]

Every planned action is submitted to the Qiyan control API before it runs and
the answer is obeyed. --rogue performs one extra action without asking, to
demonstrate that unprotected actions are not claimed as protected.

The host's actions use these tool names, which the operator should grant in
the mission scope (qiyan mission create ... --tool note.read --tool
report.append --tool tmp.cleanup --tool mail.send):
  note.read      read a file          (file.read)
  report.append  append to a file     (file.modify)
  tmp.cleanup    remove a scratch file (file.delete)
  mail.send      external notification (network.access; simulated locally, no real network)
`

type client struct {
	addr  string
	token string
	http  *http.Client
}

func (c *client) do(method, path string, body any) (int, map[string]any, error) {
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, "http://"+c.addr+path, rd)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out, nil
}

type step struct {
	name   string
	action map[string]any
	run    func(workdir string) error
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("qiyanhost", flag.ContinueOnError)
	fs.SetOutput(stderr)
	addr := fs.String("addr", "", "Qiyan control API address (host:port, required)")
	token := fs.String("token", "", "agent bearer token (required)")
	task := fs.String("task", "", "task ID to act under (required)")
	mission := fs.String("mission", "", "mission ID (required)")
	workdir := fs.String("workdir", "", "scratch directory for the host's files (required)")
	rogue := fs.Bool("rogue", false, "perform one action without asking the control API")
	jsonOut := fs.Bool("json", false, "emit one JSON object per step")
	approvalWait := fs.Duration("approval-wait", 90*time.Second, "how long to wait for a pending approval")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *addr == "" || *token == "" || *task == "" || *mission == "" || *workdir == "" {
		fmt.Fprint(stderr, usage)
		return 2
	}
	if err := os.MkdirAll(*workdir, 0o700); err != nil {
		fmt.Fprintf(stderr, "qiyanhost: workdir: %v\n", err)
		return 1
	}

	logf := func(format string, a ...any) {
		if !*jsonOut {
			fmt.Fprintf(stdout, format+"\n", a...)
		}
	}
	emit := func(kind string, kv ...any) {
		if *jsonOut {
			m := map[string]any{"event": kind}
			for i := 0; i+1 < len(kv); i += 2 {
				m[fmt.Sprint(kv[i])] = kv[i+1]
			}
			raw, _ := json.Marshal(m)
			fmt.Fprintln(stdout, string(raw))
		}
	}

	notePath := filepath.Join(*workdir, "note.txt")
	reportPath := filepath.Join(*workdir, "report.md")
	tmpPath := filepath.Join(*workdir, "tmp-old.txt")
	outboxPath := filepath.Join(*workdir, "outbox.log")

	// Bootstrap: the host's own scratch files, created before any controlled
	// action. This is host local setup, not an agent action — and it stays in
	// the host's own directory.
	logf("bootstrap: preparing scratch files in %s (host-local setup, not a controlled action)", *workdir)
	if err := os.WriteFile(notePath, []byte("hello from the reference host\n"), 0o600); err != nil {
		fmt.Fprintf(stderr, "qiyanhost: bootstrap: %v\n", err)
		return 1
	}
	if err := os.WriteFile(tmpPath, []byte("scratch to be deleted\n"), 0o600); err != nil {
		fmt.Fprintf(stderr, "qiyanhost: bootstrap: %v\n", err)
		return 1
	}

	steps := []step{
		{
			// Deliberately outside the mission's authorized folders: the
			// control API must refuse it outright.
			name: "read outside the allowed folders",
			action: map[string]any{
				"kind": "file.read", "tool": "note.read", "path": "C:/outside-the-scope.txt",
			},
			run: func(string) error { return os.ErrPermission },
		},
		{
			name: "read the note",
			action: map[string]any{
				"kind": "file.read", "tool": "note.read", "path": notePath,
			},
			run: func(string) error {
				_, err := os.ReadFile(notePath)
				return err
			},
		},
		{
			name: "append the report line",
			action: map[string]any{
				"kind": "file.modify", "tool": "report.append", "path": reportPath,
			},
			run: func(string) error {
				f, err := os.OpenFile(reportPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
				if err != nil {
					return err
				}
				defer f.Close()
				_, err = f.WriteString("reference host report line\n")
				return err
			},
		},
		{
			name: "remove the scratch file",
			action: map[string]any{
				"kind": "file.delete", "tool": "tmp.cleanup", "path": tmpPath,
			},
			run: func(string) error { return os.Remove(tmpPath) },
		},
		{
			name: "send the external notification",
			action: map[string]any{
				"kind": "network.access", "tool": "mail.send", "url": "https://example.invalid/notify",
			},
			// The reference host never performs real network calls. Its
			// "external send" is an honestly-labeled local outbox line.
			run: func(string) error {
				f, err := os.OpenFile(outboxPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
				if err != nil {
					return err
				}
				defer f.Close()
				_, err = f.WriteString("simulated external send — no network was used\n")
				return err
			},
		},
	}

	c := &client{addr: *addr, token: *token, http: &http.Client{Timeout: 30 * time.Second}}

	for i, st := range steps {
		digest, outcome, approvalID, err := authorize(c, *mission, *task, st.action, *approvalWait, logf)
		if err != nil {
			fmt.Fprintf(stderr, "qiyanhost: step %d (%s): %v\n", i+1, st.name, err)
			emit("error", "step", i+1, "detail", err.Error())
			return 1
		}

		var result string
		switch outcome {
		case "ALLOW":
			if err := st.run(*workdir); err != nil {
				result = "failed"
				logf("step %d: %-32s ALLOW -> executed (but failed: %v)", i+1, st.name, err)
			} else {
				result = "executed"
				logf("step %d: %-32s ALLOW -> executed", i+1, st.name)
			}
		case "DENY":
			result = "blocked"
			logf("step %d: %-32s DENY  -> not executed", i+1, st.name)
		case "REQUIRE_APPROVAL":
			result = "blocked"
			logf("step %d: %-32s still waiting (approval %s) -> not executed", i+1, st.name, approvalID)
		default:
			result = "blocked"
			logf("step %d: %-32s %s -> not executed", i+1, st.name, outcome)
		}
		emit("step", "n", i+1, "name", st.name, "outcome", outcome, "result", result, "actionDigest", digest)

		if _, _, err := c.do("POST", "/v1/events", map[string]any{
			"taskId": *task, "actionDigest": digest, "result": result, "note": st.name,
		}); err != nil {
			fmt.Fprintf(stderr, "qiyanhost: result report failed: %v\n", err)
			return 1
		}
	}

	if *rogue {
		// The rogue action: performed WITHOUT asking. Verification will show
		// no decision exists for it — the system never claims it was covered.
		f, err := os.OpenFile(reportPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = f.WriteString("ROGUE UNCONTROLLED LINE (no control call was made)\n")
			f.Close()
		}
		logf("rogue: wrote one line to the report WITHOUT asking the control API")
		emit("rogue", "note", "one action performed without any control call")
	}

	logf("done.")
	emit("done")
	return 0
}

// authorize submits one action and returns (digest, final outcome, approval id).
// REQUIRE_APPROVAL is polled until the user decides or the wait elapses; an
// approved action is re-submitted and this time runs (or is denied afresh).
func authorize(c *client, mission, task string, action map[string]any, wait time.Duration, logf func(string, ...any)) (string, string, string, error) {
	deadline := time.Now().Add(wait)
	for {
		code, body, err := c.do("POST", "/v1/authorize", map[string]any{
			"missionId": mission, "taskId": task, "action": action,
		})
		if err != nil {
			return "", "", "", fmt.Errorf("authorize request failed: %w", err)
		}
		if code != 200 {
			return "", "", "", fmt.Errorf("authorize returned %d: %v", code, body)
		}
		outcome, _ := body["outcome"].(string)
		digest, _ := body["actionDigest"].(string)
		approvalID, _ := body["approvalId"].(string)

		if outcome != "REQUIRE_APPROVAL" {
			return digest, outcome, approvalID, nil
		}
		if approvalID == "" || time.Now().After(deadline) {
			return digest, outcome, approvalID, nil
		}
		logf("waiting for your approval on %s ...", approvalID)
		time.Sleep(1 * time.Second)
		// Watch the approval state; when it is no longer pending, re-ask the
		// authorize endpoint — the binding-based claim decides from there.
		code, body, err = c.do("GET", "/v1/approvals/"+approvalID, nil)
		if err != nil || code != 200 {
			continue
		}
		state, _ := body["state"].(string)
		if state == "pending" {
			continue
		}
		logf("approval %s is now %s", approvalID, state)
	}
}
