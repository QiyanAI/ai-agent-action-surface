// Command qiyan is the command-line entry point for the QiyanAI
// ai-agent-action-surface project.
//
// This is an early engineering scaffold. It deliberately implements only help
// and version output: no scanning, analysis, evidence collection or agent-action
// capability exists yet, and the program makes no claim to any. See
// docs/scope-and-claims.md for the boundary this scaffold is held to.
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
	version = "0.0.0-r01"
	commit  = "unknown"
	built   = "unknown"
)

const usageText = programName + ` - Evidence-Backed AI Agent Action Surface & Blast Radius Map

Early engineering scaffold. Only help and version output exist today.

Usage:
  qiyan help       Show this help text
  qiyan version    Show version and build identity

Flags:
  -h, --help       Show this help text
  -v, --version    Show version and build identity
`

// main delegates to run so that every behaviour is reachable from a test.
func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes one invocation and returns the process exit code.
//
// Exit codes: 0 on success, 2 for an unrecognised command.
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
	default:
		fmt.Fprintf(stderr, "%s: unknown command %q\n\n", programName, args[0])
		fmt.Fprint(stderr, usageText)
		return 2
	}
}
