package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"time"
)

// The PreToolUse hook entry point.
//
// Protocol with the agent runtime (verified against real Claude Code runs in
// R02-00): stdin carries the hook JSON; exit 0 means "let it proceed"; exit 2
// blocks the call and the stderr text becomes the reason Claude sees. Any
// failure to obtain a real decision — service down, timeout, malformed
// response, unbound session — exits 2 with an explicit message. The decision
// itself always comes from the control engine; this process only transports
// it. With `onFailure: "block"` configured on the runtime side, even an
// inability to start this process blocks the action.

type hookInput struct {
	HookEventName string          `json:"hook_event_name"`
	SessionID     string          `json:"session_id"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
	Cwd           string          `json:"cwd"`
}

// hookAllowOutput is the runtime's structured permission decision.
type hookAllowOutput struct {
	HookSpecificOutput struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason"`
	} `json:"hookSpecificOutput"`
}

type decisionResponse struct {
	Outcome string `json:"outcome"`
	Reasons []struct {
		Code   string            `json:"code"`
		Params map[string]string `json:"params"`
	} `json:"reasons"`
	ApprovalID string `json:"approvalId"`
	Mode       string `json:"mode"`
}

func runHook(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "pretooluse" {
		fmt.Fprintf(stderr, "%s: usage: %s hook pretooluse [--state-dir DIR] [--timeout SECONDS]\n", programName, programName)
		return 2
	}
	fs := flag.NewFlagSet("hook pretooluse", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("state-dir", stateDirDefault(), "state directory")
	timeout := fs.Float64("timeout", 3.0, "how long to wait for a decision, in seconds")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	raw, err := io.ReadAll(io.LimitReader(stdin, 4<<20))
	if err != nil {
		return blockNoDecision(stderr, "the hook input could not be read")
	}
	var in hookInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return blockNoDecision(stderr, "the hook input was not valid JSON")
	}
	if in.HookEventName != "" && in.HookEventName != "PreToolUse" {
		// Not our event: stay out of the way entirely (no decision).
		return 0
	}

	info, err := readServerInfo(*dir)
	if err != nil {
		return blockNoDecision(stderr, err.Error())
	}
	token, err := readSecret(joinState(*dir, "hook.token"))
	if err != nil {
		return blockNoDecision(stderr, "the hook token is missing; start `qiyan serve` first")
	}

	body, _ := json.Marshal(map[string]any{
		"sessionId": in.SessionID,
		"toolName":  in.ToolName,
		"toolInput": json.RawMessage(in.ToolInput),
		"cwd":       in.Cwd,
	})
	client := &http.Client{Timeout: time.Duration(*timeout * float64(time.Second))}
	req, err := http.NewRequest("POST", "http://"+info.Addr+"/v1/hook/decide", bytes.NewReader(body))
	if err != nil {
		return blockNoDecision(stderr, "could not build the decision request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return blockNoDecision(stderr, "the Qiyan control service did not answer (is `qiyan serve` running?)")
	}
	defer resp.Body.Close()
	respRaw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode != http.StatusOK {
		// A structured refusal (unbound session, bad input) still means block.
		msg := "the control service refused the request"
		var e struct {
			Error struct {
				Class   string `json:"class"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(respRaw, &e) == nil && e.Error.Message != "" {
			msg = e.Error.Message
		}
		return blockWithMessage(stderr, msg)
	}

	var dec decisionResponse
	if err := json.Unmarshal(respRaw, &dec); err != nil {
		return blockNoDecision(stderr, "the decision response could not be parsed")
	}

	switch dec.Outcome {
	case "ALLOW":
		var out hookAllowOutput
		out.HookSpecificOutput.HookEventName = "PreToolUse"
		out.HookSpecificOutput.PermissionDecision = "allow"
		out.HookSpecificOutput.PermissionDecisionReason = "Qiyan: within the task's authorized scope"
		rawOut, _ := json.Marshal(out)
		fmt.Fprintln(stdout, string(rawOut))
		return 0
	case "REQUIRE_APPROVAL":
		msg := renderReasons(dec.Reasons)
		text := fmt.Sprintf("Qiyan is waiting for your approval before this action runs.\n批准前该操作不会执行。\n%s\nApproval: %s (approve with: %s approve %s)", msg, dec.ApprovalID, programName, dec.ApprovalID)
		return blockWithMessage(stderr, text)
	default: // DENY, UNKNOWN, anything unexpected
		msg := renderReasons(dec.Reasons)
		text := fmt.Sprintf("Qiyan blocked this action.\n该操作已被 Qiyan 阻止。\n%s", msg)
		return blockWithMessage(stderr, text)
	}
}

// blockWithMessage writes the user-facing reason and exits 2.
func blockWithMessage(stderr io.Writer, msg string) int {
	fmt.Fprintln(stderr, "Qiyan: "+msg)
	return 2
}

// blockNoDecision is the fail-closed voice: whenever a real decision could
// not be obtained, the action does not run and the reason says why.
func blockNoDecision(stderr io.Writer, why string) int {
	fmt.Fprintf(stderr, "Qiyan could not obtain a control decision (%s), so the action was blocked.\n", why)
	fmt.Fprintln(stderr, "Qiyan 无法获得控制决策，已按失败即拒绝（fail-closed）阻止该操作。")
	return 2
}
