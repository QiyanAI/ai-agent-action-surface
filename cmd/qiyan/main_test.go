package main

import (
	"bytes"
	"strings"
	"testing"
)

// invoke runs the CLI with the given arguments and returns stdout, stderr and
// the exit code, so each behaviour can be asserted independently.
func invoke(args ...string) (stdout, stderr string, code int) {
	var out, errBuf bytes.Buffer
	code = run(args, &out, &errBuf)
	return out.String(), errBuf.String(), code
}

func TestNoArgumentsPrintsUsageToStdout(t *testing.T) {
	stdout, stderr, code := invoke()

	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("stdout does not contain usage text: %q", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestHelpFormsAreEquivalent(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		stdout, _, code := invoke(arg)
		if code != 0 {
			t.Errorf("%q: exit code = %d, want 0", arg, code)
		}
		if !strings.Contains(stdout, "Usage:") {
			t.Errorf("%q: stdout does not contain usage text: %q", arg, stdout)
		}
	}
}

func TestVersionFormsReportBuildIdentity(t *testing.T) {
	for _, arg := range []string{"version", "-v", "--version"} {
		stdout, _, code := invoke(arg)
		if code != 0 {
			t.Errorf("%q: exit code = %d, want 0", arg, code)
		}
		if !strings.Contains(stdout, programName+" "+version) {
			t.Errorf("%q: stdout %q does not report %q", arg, stdout, programName+" "+version)
		}
		if !strings.Contains(stdout, "commit ") {
			t.Errorf("%q: stdout %q does not report a commit identity", arg, stdout)
		}
	}
}

func TestUnknownCommandFailsWithExitCode2(t *testing.T) {
	stdout, stderr, code := invoke("definitely-not-a-command")

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("stderr does not report the unknown command: %q", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty on error", stdout)
	}
}

// The help text is the CLI's only contract today, so it must actually name the
// verbs that exist. Without this, a verb could be implemented and remain
// undiscoverable.
func TestHelpNamesEveryImplementedVerb(t *testing.T) {
	stdout, _, code := invoke("help")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	for _, verb := range []string{"help", "version", "serve", "hook", "status", "agents", "approvals", "approve", "deny", "mission"} {
		if !strings.Contains(stdout, verb) {
			t.Errorf("help output does not name the %q verb:\n%s", verb, stdout)
		}
	}
}

// Bare invocation and `help` are the same entry point from a user's point of
// view; if they ever diverge, one of them is wrong.
func TestBareInvocationMatchesHelp(t *testing.T) {
	bareOut, _, bareCode := invoke()
	helpOut, _, helpCode := invoke("help")

	if bareCode != helpCode {
		t.Fatalf("exit codes differ: bare = %d, help = %d", bareCode, helpCode)
	}
	if bareOut != helpOut {
		t.Errorf("bare invocation and `help` disagree:\nbare: %q\nhelp: %q", bareOut, helpOut)
	}
}
