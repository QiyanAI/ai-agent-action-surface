// Command qiyan is the command-line entry point for the QiyanAI
// ai-agent-action-surface project: the local control service (`serve`), the
// Claude Code PreToolUse hook, the connector, the three-role mission fleet
// and the operator clients (status, agents, approvals, missions, connect).
//
// What it does not do is stated next to what it does; see
// docs/scope-and-claims.md for the exact boundary.
package main

import (
	"fmt"
	"io"
	"os"
)

const programName = "qiyan"

// Build identity. These defaults describe an untagged local `go build`.
// The release drill overwrites them with
//
//	-ldflags "-X main.version=... -X main.commit=... -X main.built=..."
//
// so that a produced binary reports the exact commit it was built from.
//
// The commit value is injected at build time and is never written back into the
// repository: no tracked file ever contains the SHA of the commit that contains
// it, which would be a self-referential hash.
var (
	version = "0.3.0-r03"
	commit  = "unknown"
	built   = "unknown"
)

const usageText = programName + ` - Open Agent Mission & Control Platform

Run local AI agents under verifiable pre-execution control.

Usage:
  qiyan help                          Show this help text
  qiyan version                       Show version and build identity
  qiyan serve                         Start the local control service (API + console)
  qiyan hook pretooluse               PreToolUse hook entry (called by the agent runtime)
  qiyan status                        Show control service status and pending approvals
  qiyan agents add --name N --role R  Register an agent and print its token
  qiyan agents list                   List registered agents
  qiyan approvals list                List pending approvals
  qiyan approve <id>                  Approve one pending action
  qiyan deny <id>                     Deny one pending action
  qiyan mission create --goal G       Create a mission (flags: --root, --mode, --steps, --runtime-min)
  qiyan task create --mission M --role R   Create a task under a mission
  qiyan session bind --task T --agent A --session S   Bind a runtime session to a task
  qiyan connect claude --project DIR  Connect Claude Code under Qiyan control (with live probe)
  qiyan connect status                Show installed integrations
  qiyan connect undo --project DIR    Disconnect Claude Code and undo Qiyan's settings
  qiyan run --mission M               Run the mission's three-role fleet and follow it

Flags:
  -h, --help       Show this help text
  -v, --version    Show version and build identity
  --state-dir DIR  State directory (default: user config dir / QiyanAI / qiyan)

Exit codes: 0 success, 1 error, 2 usage error or unrecognised command.
For the hook entry point: exit 0 allows the action, exit 2 blocks it —
either because Qiyan denied it, or because a decision could not be obtained
(fail-closed).`

// main delegates to run so that every behaviour is reachable from a test.
func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes one invocation and returns the process exit code.
//
// Exit codes: 0 on success, 1 on runtime errors, 2 for an unrecognised command.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stdout, usageText)
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usageText)
		return 0
	case "version", "-v", "--version":
		fmt.Fprintf(stdout, "%s %s (commit %s, built %s)\n", programName, version, commit, built)
		return 0
	case "serve":
		return runServe(args[1:], stdout, stderr)
	case "hook":
		return runHook(args[1:], os.Stdin, stdout, stderr)
	case "status":
		return runStatus(args[1:], stdout, stderr)
	case "agents":
		return runAgents(args[1:], stdout, stderr)
	case "approvals":
		return runApprovals(args[1:], stdout, stderr)
	case "approve":
		return runApprovalDecision(args[1:], true, stdout, stderr)
	case "deny":
		return runApprovalDecision(args[1:], false, stdout, stderr)
	case "mission":
		return runMission(args[1:], stdout, stderr)
	case "task":
		return runTask(args[1:], stdout, stderr)
	case "session":
		return runSession(args[1:], stdout, stderr)
	case "connect":
		return runConnect(args[1:], stdout, stderr, os.Stdin)
	case "run":
		return runMissionRun(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s: unknown command %q\n\n", programName, args[0])
		fmt.Fprint(stderr, usageText)
		return 2
	}
}
