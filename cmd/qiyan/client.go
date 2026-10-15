package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// Operator CLI: thin clients of the local control API. They hold no logic of
// their own — every approval, mission and agent registration goes through the
// same versioned endpoints a third-party tool would call, which keeps the
// product honest about its open layer.

func joinState(dir, name string) string { return filepath.Join(dir, name) }

type apiClient struct {
	addr  string
	token string
	http  *http.Client
}

func loadClient(dir string) (*apiClient, error) {
	info, err := readServerInfo(dir)
	if err != nil {
		return nil, err
	}
	tok, err := readSecret(joinState(dir, "operator.token"))
	if err != nil {
		return nil, fmt.Errorf("operator token not found in %s (start `%s serve` first)", dir, programName)
	}
	return &apiClient{addr: info.Addr, token: tok, http: &http.Client{Timeout: 10 * time.Second}}, nil
}

func (c *apiClient) do(method, path string, body any) (int, map[string]any, error) {
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
		return 0, nil, fmt.Errorf("cannot reach the Qiyan service: %v", err)
	}
	defer resp.Body.Close()
	var out map[string]any
	rawOut, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = json.Unmarshal(rawOut, &out)
	return resp.StatusCode, out, nil
}

func apiError(body map[string]any) string {
	if e, ok := body["error"].(map[string]any); ok {
		if msg, ok := e["message"].(string); ok {
			return msg
		}
	}
	return "request failed"
}

func runStatus(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("state-dir", stateDirDefault(), "state directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	c, err := loadClient(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
		return 1
	}
	code, body, err := c.do("GET", "/v1/health", nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
		return 1
	}
	if code != 200 {
		fmt.Fprintf(stderr, "%s: health check failed: %s\n", programName, apiError(body))
		return 1
	}
	fmt.Fprintf(stdout, "status:      %v\nversion:     %v\n", body["status"], body["version"])
	code, body, err = c.do("GET", "/v1/approvals?state=pending", nil)
	if err == nil && code == 200 {
		if list, ok := body["approvals"].([]any); ok {
			fmt.Fprintf(stdout, "approvals:   %d waiting for you\n", len(list))
		}
	}
	return 0
}

func runAgents(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s: usage: %s agents add|list ...\n", programName, programName)
		return 2
	}
	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("agents add", flag.ContinueOnError)
		fs.SetOutput(stderr)
		name := fs.String("name", "", "agent display name (required)")
		role := fs.String("role", "", "agent role: planner|builder|reviewer|external|reference (required)")
		runtimeName := fs.String("runtime", "", "runtime label, e.g. claude-code@2.1.300")
		dir := fs.String("state-dir", stateDirDefault(), "state directory")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if *name == "" || *role == "" {
			fmt.Fprintf(stderr, "%s: agents add requires --name and --role\n", programName)
			return 2
		}
		c, err := loadClient(*dir)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", programName, err)
			return 1
		}
		code, body, err := c.do("POST", "/v1/agents", map[string]any{"name": *name, "role": *role, "runtime": *runtimeName})
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", programName, err)
			return 1
		}
		if code != 201 {
			fmt.Fprintf(stderr, "%s: %s\n", programName, apiError(body))
			return 1
		}
		agent, _ := body["agent"].(map[string]any)
		token, _ := body["token"].(string)
		fmt.Fprintf(stdout, "agent:  %v (%v)\nrole:   %v\n", agent["id"], agent["name"], agent["role"])
		fmt.Fprintf(stdout, "token:  %s\n", token)
		fmt.Fprintln(stdout, "This token is shown once and stored only as a hash. Give it to the agent's host.")
		return 0
	case "list":
		fs := flag.NewFlagSet("agents list", flag.ContinueOnError)
		fs.SetOutput(stderr)
		dir := fs.String("state-dir", stateDirDefault(), "state directory")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		c, err := loadClient(*dir)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", programName, err)
			return 1
		}
		code, body, err := c.do("GET", "/v1/agents", nil)
		if err != nil || code != 200 {
			fmt.Fprintf(stderr, "%s: %s\n", programName, apiError(body))
			return 1
		}
		list, _ := body["agents"].([]any)
		for _, a := range list {
			m, _ := a.(map[string]any)
			fmt.Fprintf(stdout, "%v  %-10v %-12v %v\n", m["id"], m["name"], m["role"], m["runtime"])
		}
		return 0
	default:
		fmt.Fprintf(stderr, "%s: unknown agents subcommand %q\n", programName, args[0])
		return 2
	}
}

func runApprovals(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "list" {
		fmt.Fprintf(stderr, "%s: usage: %s approvals list\n", programName, programName)
		return 2
	}
	fs := flag.NewFlagSet("approvals list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("state-dir", stateDirDefault(), "state directory")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	c, err := loadClient(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
		return 1
	}
	code, body, err := c.do("GET", "/v1/approvals?state=pending", nil)
	if err != nil || code != 200 {
		fmt.Fprintf(stderr, "%s: %s\n", programName, apiError(body))
		return 1
	}
	list, _ := body["approvals"].([]any)
	if len(list) == 0 {
		fmt.Fprintln(stdout, "no approvals are waiting.")
		return 0
	}
	for _, a := range list {
		m, _ := a.(map[string]any)
		action, _ := m["action"].(map[string]any)
		kind, target := "", ""
		if action != nil {
			kind, _ = action["kind"].(string)
			target, _ = action["path"].(string)
			if target == "" {
				target, _ = action["command"].(string)
			}
			if target == "" {
				target, _ = action["url"].(string)
			}
		}
		fmt.Fprintf(stdout, "%v  %-16v %-10v %s\n", m["id"], m["agentName"], kind, truncateForDisplay(target, 60))
	}
	fmt.Fprintf(stdout, "\napprove with: %s approve <id>    deny with: %s deny <id>\n", programName, programName)
	return 0
}

func runApprovalDecision(args []string, approve bool, stdout, stderr io.Writer) int {
	verb := "deny"
	if approve {
		verb = "approve"
	}
	// The ID is positional but users write it in either order with flags
	// (`approve ID --state-dir D` or `approve --state-dir D ID`); the flag
	// package alone stops at the first positional, so split explicitly.
	var id string
	var flagArgs []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--state-dir" && i+1 < len(args):
			flagArgs = append(flagArgs, a, args[i+1])
			i++
		case strings.HasPrefix(a, "--state-dir="):
			flagArgs = append(flagArgs, a)
		case id == "":
			id = a
		default:
			fmt.Fprintf(stderr, "%s: usage: %s %s <approval-id> [--state-dir DIR]\n", programName, programName, verb)
			return 2
		}
	}
	fs := flag.NewFlagSet(verb, flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("state-dir", stateDirDefault(), "state directory")
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s: usage: %s %s <approval-id> [--state-dir DIR]\n", programName, programName, verb)
		return 2
	}
	c, err := loadClient(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
		return 1
	}
	code, body, err := c.do("POST", "/v1/approvals/"+id+"/decision", map[string]any{"approve": approve})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
		return 1
	}
	if code != 200 {
		fmt.Fprintf(stderr, "%s: %s\n", programName, apiError(body))
		return 1
	}
	if approve {
		fmt.Fprintf(stdout, "approved %s — the agent's retry of that exact action may proceed once.\n", id)
	} else {
		fmt.Fprintf(stdout, "denied %s — that action will not run for this task.\n", id)
	}
	return 0
}

func runMission(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "create" {
		fmt.Fprintf(stderr, "%s: usage: %s mission create --goal G [--root DIR ...] [--mode companion|controlled_auto|bounded_autonomy]\n", programName, programName)
		return 2
	}
	fs := flag.NewFlagSet("mission create", flag.ContinueOnError)
	fs.SetOutput(stderr)
	goal := fs.String("goal", "", "the mission's objective (required)")
	mode := fs.String("mode", "controlled_auto", "autonomy mode")
	steps := fs.Int("steps", 50, "step budget")
	runtimeMin := fs.Int("runtime-min", 60, "runtime budget in minutes")
	dir := fs.String("state-dir", stateDirDefault(), "state directory")
	var roots multiFlag
	fs.Var(&roots, "root", "authorized directory (repeatable; required)")
	var extraTools multiFlag
	fs.Var(&extraTools, "tool", "extra tool name to grant (repeatable)")
	var extraKinds multiFlag
	fs.Var(&extraKinds, "kind", "extra action kind to grant (repeatable)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *goal == "" || len(roots) == 0 {
		fmt.Fprintf(stderr, "%s: mission create requires --goal and at least one --root\n", programName)
		return 2
	}
	c, err := loadClient(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
		return 1
	}
	kinds := []string{"file.read", "file.create", "file.modify", "file.delete", "process.exec", "session.local"}
	kinds = append(kinds, extraKinds...)
	tools := []string{"Read", "Write", "Edit", "Bash", "Glob", "Grep",
		"TodoWrite", "TodoRead", "Skill", "TaskCreate", "TaskGet", "TaskList", "TaskUpdate"}
	tools = append(tools, extraTools...)
	code, body, err := c.do("POST", "/v1/missions", map[string]any{
		"goal": *goal, "mode": *mode,
		"scope": map[string]any{
			"roots": roots, "allowedKinds": kinds, "allowedTools": tools,
			"maxSteps": *steps, "maxRuntimeSeconds": *runtimeMin * 60,
		},
	})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
		return 1
	}
	if code != 201 {
		fmt.Fprintf(stderr, "%s: %s\n", programName, apiError(body))
		return 1
	}
	policy, _ := body["policy"].(map[string]any)
	fmt.Fprintf(stdout, "mission: %v\nmode:    %v\nroots:   %s\n", body["id"], policy["mode"], strings.Join(roots, ", "))
	return 0
}

// runTask creates a task under a mission. The task starts in READY and moves
// to RUNNING when a runtime session is bound to it.
func runTask(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "create" {
		fmt.Fprintf(stderr, "%s: usage: %s task create --mission ID --role R [--agent ID] [--title T]\n", programName, programName)
		return 2
	}
	fs := flag.NewFlagSet("task create", flag.ContinueOnError)
	fs.SetOutput(stderr)
	mission := fs.String("mission", "", "mission ID (required)")
	role := fs.String("role", "", "task role: planner|builder|reviewer (required)")
	agentID := fs.String("agent", "", "agent ID to assign")
	title := fs.String("title", "", "task title")
	dir := fs.String("state-dir", stateDirDefault(), "state directory")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *mission == "" || *role == "" {
		fmt.Fprintf(stderr, "%s: task create requires --mission and --role\n", programName)
		return 2
	}
	c, err := loadClient(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
		return 1
	}
	code, body, err := c.do("POST", "/v1/missions/"+*mission+"/tasks", map[string]any{
		"role": *role, "title": *title, "agentId": *agentID,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
		return 1
	}
	if code != 201 {
		fmt.Fprintf(stderr, "%s: %s\n", programName, apiError(body))
		return 1
	}
	fmt.Fprintf(stdout, "task: %v (state %v)\n", body["id"], body["state"])
	return 0
}

// runSession binds a runtime session to a task, which also starts the task.
func runSession(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "bind" {
		fmt.Fprintf(stderr, "%s: usage: %s session bind --task T --agent A --session S [--runtime R]\n", programName, programName)
		return 2
	}
	fs := flag.NewFlagSet("session bind", flag.ContinueOnError)
	fs.SetOutput(stderr)
	task := fs.String("task", "", "task ID (required)")
	agentID := fs.String("agent", "", "agent ID (required)")
	session := fs.String("session", "", "runtime session ID (required)")
	runtimeName := fs.String("runtime", "", "runtime label")
	dir := fs.String("state-dir", stateDirDefault(), "state directory")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *task == "" || *agentID == "" || *session == "" {
		fmt.Fprintf(stderr, "%s: session bind requires --task, --agent and --session\n", programName)
		return 2
	}
	c, err := loadClient(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
		return 1
	}
	code, body, err := c.do("POST", "/v1/sessions", map[string]any{
		"sessionId": *session, "runtime": *runtimeName, "taskId": *task, "agentId": *agentID,
	})
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
		return 1
	}
	if code != 201 {
		fmt.Fprintf(stderr, "%s: %s\n", programName, apiError(body))
		return 1
	}
	fmt.Fprintf(stdout, "session %s bound to task %s (task now RUNNING)\n", *session, *task)
	return 0
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func truncateForDisplay(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
