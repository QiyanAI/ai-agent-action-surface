// Package api implements Qiyan's local, versioned control API. Everything —
// the PreToolUse hook, the local UI, CLI helpers and third-party agent hosts —
// reaches the control engine through this one interface, so the open control
// layer is not a special path bolted onto the product: it is how the product
// itself works.
package api

import (
	"encoding/json"

	"github.com/QiyanAI/ai-agent-action-surface/internal/control"
	"github.com/QiyanAI/ai-agent-action-surface/internal/domain"
)

// BuildClaudeCodeAction converts one Claude Code tool call (the tool_name and
// tool_input fields the PreToolUse hook receives) into a domain.Action.
//
// cwd must be the hook input's cwd: it is the resolution base for relative
// paths, which Claude Code has already expanded for file tools — the engine
// still re-validates that every path arrives absolute and normalized, and
// denies anything else rather than guessing.
//
// The mapping is deliberately explicit. A tool that is not in the table
// becomes ActionUnknown, which the engine will never allow silently — new
// Claude Code tools require a decision here, not a default.
func BuildClaudeCodeAction(toolName string, input json.RawMessage, cwd string) (domain.Action, error) {
	str := func(field string) string {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(input, &m); err != nil {
			return ""
		}
		var s string
		if raw, ok := m[field]; ok {
			_ = json.Unmarshal(raw, &s)
		}
		return s
	}

	fileAction := func(kind domain.ActionKind, field string) domain.Action {
		return domain.Action{Kind: kind, Tool: toolName, Path: domain.NormalizePath(str(field), cwd)}
	}

	switch toolName {
	case "Read":
		return fileAction(domain.ActionFileRead, "file_path"), nil
	case "Glob", "Grep":
		// These tools read from a directory (a pattern search is still a read
		// of everything under it). Target the directory given, or the session
		// cwd when only a pattern was passed.
		p := str("path")
		if p == "" {
			p = cwd
		}
		return domain.Action{Kind: domain.ActionFileRead, Tool: toolName, Path: domain.NormalizePath(p, cwd)}, nil
	case "Write", "Edit", "NotebookEdit":
		return fileAction(domain.ActionFileModify, "file_path"), nil
	case "Bash":
		cmd := str("command")
		return domain.Action{
			Kind: domain.ActionProcessExec, Tool: toolName, Command: cmd,
			CommandProgram: control.ExtractProgram(cmd),
			Cwd:            domain.NormalizePath(cwd, ""),
			Raw:            cmd,
		}, nil
	case "BashOutput":
		return domain.Action{Kind: domain.ActionFileRead, Tool: toolName, Path: domain.NormalizePath(cwd, cwd)}, nil
	case "KillShell":
		cmd := "kill-shell " + str("shell_id")
		return domain.Action{Kind: domain.ActionProcessExec, Tool: toolName, Command: cmd, CommandProgram: "kill-shell"}, nil
	case "WebFetch":
		return domain.Action{Kind: domain.ActionNetwork, Tool: toolName, URL: str("url")}, nil
	case "WebSearch":
		return domain.Action{Kind: domain.ActionNetwork, Tool: toolName, URL: "search://" + str("query")}, nil
	case "TodoWrite", "TodoRead", "Skill", "TaskCreate", "TaskGet", "TaskList", "TaskUpdate", "TaskOutput", "TaskStop":
		return domain.Action{Kind: domain.ActionSessionLocal, Tool: toolName}, nil
	case "Task", "Agent":
		// Spawning a subagent: the spawn itself is classified unknown (we
		// cannot see what it will do), and the subagent's own tool calls fire
		// their own PreToolUse events and are gated individually.
		return domain.Action{Kind: domain.ActionUnknown, Tool: toolName, Raw: truncate(string(input), 200)}, nil
	default:
		return domain.Action{Kind: domain.ActionUnknown, Tool: toolName, Raw: truncate(string(input), 200)}, nil
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// ActionPayload is the wire form of an action for API clients (the reference
// host and third-party integrations). Field names are stable.
type ActionPayload struct {
	Kind           string `json:"kind"`
	Tool           string `json:"tool"`
	Path           string `json:"path,omitempty"`
	Command        string `json:"command,omitempty"`
	Cwd            string `json:"cwd,omitempty"`
	URL            string `json:"url,omitempty"`
	CredentialRef  string `json:"credentialRef,omitempty"`
	DelegationTo   string `json:"delegationTo,omitempty"`
	DelegationTask string `json:"delegationTask,omitempty"`
	DelegationKind string `json:"delegationKind,omitempty"`
	Raw            string `json:"raw,omitempty"`
}

// ToAction converts the payload. Paths are normalized; relative paths remain
// relative and are later denied by the engine's absolute-path requirement —
// hosts must send paths their own runtime would actually touch.
func (p ActionPayload) ToAction() domain.Action {
	a := domain.Action{
		Kind:          domain.ActionKind(p.Kind),
		Tool:          p.Tool,
		Command:       p.Command,
		URL:           p.URL,
		CredentialRef: p.CredentialRef,
		Raw:           truncate(p.Raw, 200),
	}
	if p.Path != "" {
		a.Path = domain.NormalizePath(p.Path, "")
	}
	if a.Kind == domain.ActionProcessExec && a.Command != "" {
		a.CommandProgram = control.ExtractProgram(a.Command)
	}
	if p.Cwd != "" {
		a.Cwd = domain.NormalizePath(p.Cwd, "")
	}
	if p.DelegationTo != "" {
		a.DelegationTo = domain.AgentID(p.DelegationTo)
	}
	if p.DelegationTask != "" {
		a.DelegationTask = domain.TaskID(p.DelegationTask)
	}
	if p.DelegationKind != "" {
		a.DelegationKind = domain.ActionKind(p.DelegationKind)
	}
	return a
}

// ActionToPayload is the inverse for responses and evidence views.
func ActionToPayload(a domain.Action) ActionPayload {
	return ActionPayload{
		Kind:           string(a.Kind),
		Tool:           a.Tool,
		Path:           a.Path,
		Command:        a.Command,
		Cwd:            a.Cwd,
		URL:            a.URL,
		CredentialRef:  a.CredentialRef,
		DelegationTo:   string(a.DelegationTo),
		DelegationTask: string(a.DelegationTask),
		DelegationKind: string(a.DelegationKind),
	}
}

// DecisionPayload is the stable response body for a decision.
type DecisionPayload struct {
	Outcome      string          `json:"outcome"`
	Reasons      []ReasonPayload `json:"reasons"`
	ActionDigest string          `json:"actionDigest"`
	ApprovalID   string          `json:"approvalId,omitempty"`
	Mode         string          `json:"mode,omitempty"`
	Coverage     string          `json:"coverage,omitempty"`
}

// ReasonPayload carries a machine reason plus its interpolation parameters;
// localized wording is rendered by the caller, never stored.
type ReasonPayload struct {
	Code   string            `json:"code"`
	Params map[string]string `json:"params,omitempty"`
}

// DecisionToPayload converts a domain decision to its wire form.
func DecisionToPayload(d domain.Decision) DecisionPayload {
	p := DecisionPayload{
		Outcome:      string(d.Outcome),
		ActionDigest: d.ActionDigest,
		ApprovalID:   string(d.ApprovalID),
		Mode:         string(d.Mode),
		Coverage:     string(d.Coverage),
	}
	p.Reasons = make([]ReasonPayload, 0, len(d.Reasons))
	for _, r := range d.Reasons {
		p.Reasons = append(p.Reasons, ReasonPayload{Code: string(r.Code), Params: r.Params})
	}
	return p
}

// SessionLocalTools lists the tool names that map to ActionSessionLocal;
// used by tests and by coverage reporting.
func SessionLocalTools() []string {
	return []string{"TodoWrite", "TodoRead", "Skill", "TaskCreate", "TaskGet", "TaskList", "TaskUpdate", "TaskOutput", "TaskStop"}
}
