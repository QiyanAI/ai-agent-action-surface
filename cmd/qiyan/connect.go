package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

// qiyan connect — the Claude Code integration wizard (CLI form).
//
// The wizard shows the discovered runtime, what would change and where, and
// waits for confirmation before anything is written. On apply it first runs
// the real enforcement probe: three tiny live sessions that establish what
// this machine's Claude Code actually does with Qiyan's hook. A version that
// cannot fail closed is installed only if the operator explicitly accepts the
// weaker, labelled tier — never silently.

func runConnect(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s: usage: %s connect claude --project DIR [--claude-bin PATH] [--yes] [--allow-unprobed]\n", programName, programName)
		fmt.Fprintf(stderr, "       %s connect status | %s connect undo --project DIR [--yes]\n", programName, programName)
		return 2
	}
	switch args[0] {
	case "claude":
		return runConnectClaude(args[1:], stdout, stderr, stdin)
	case "status":
		return runConnectStatus(args[1:], stdout, stderr)
	case "undo":
		return runConnectUndo(args[1:], stdout, stderr, stdin)
	default:
		fmt.Fprintf(stderr, "%s: unknown connect subcommand %q\n", programName, args[0])
		return 2
	}
}

func runConnectClaude(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	fs := flag.NewFlagSet("connect claude", flag.ContinueOnError)
	fs.SetOutput(stderr)
	project := fs.String("project", "", "project directory to protect (required)")
	claudeBin := fs.String("claude-bin", "", "path to the Claude Code binary (default: found on PATH)")
	yes := fs.Bool("yes", false, "apply without asking")
	allowUnprobed := fs.Bool("allow-unprobed", false, "install even if the fail-closed probe does not pass (records the weaker tier honestly)")
	dir := fs.String("state-dir", stateDirDefault(), "state directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *project == "" {
		fmt.Fprintf(stderr, "%s: connect claude requires --project\n", programName)
		return 2
	}
	c, err := loadClient(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
		return 1
	}
	req := map[string]any{"projectDir": *project}
	if *claudeBin != "" {
		req["claudeBin"] = *claudeBin
	}

	code, body, err := c.do("POST", "/v1/connect/preview", req)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
		return 1
	}
	if code != 200 {
		fmt.Fprintf(stderr, "%s: %s\n", programName, apiError(body))
		return 1
	}
	det, _ := body["detected"].(map[string]any)
	plan, _ := body["plan"].(map[string]any)
	noChanges, _ := plan["noChanges"].(bool)

	fmt.Fprintf(stdout, "Claude Code:      %v (version %v)\n", det["Binary"], det["Version"])
	fmt.Fprintf(stdout, "version implies:  %v (the probe on this machine decides for real)\n", det["TierFromVersion"])
	fmt.Fprintf(stdout, "support floor:    %v for fail-closed protection\n", body["supportFloor"])
	if noChanges {
		fmt.Fprintln(stdout, "already connected — nothing to change.")
		return 0
	}
	fmt.Fprintf(stdout, "settings file:    %v\n", plan["settingsPath"])
	fmt.Fprintln(stdout, "planned changes:")
	if changes, ok := plan["Changes"].([]any); ok {
		for _, ch := range changes {
			fmt.Fprintf(stdout, "  - %v\n", ch)
		}
	} else if changes, ok := plan["changes"].([]any); ok {
		for _, ch := range changes {
			fmt.Fprintf(stdout, "  - %v\n", ch)
		}
	}
	if notice, ok := body["probeNotice"].(string); ok {
		fmt.Fprintf(stdout, "\nnote: %s\n", notice)
	}

	if !*yes {
		fmt.Fprint(stdout, "\napply these changes? [y/N] ")
		reader := bufio.NewReader(stdin)
		line, _ := reader.ReadString('\n')
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y") {
			fmt.Fprintln(stdout, "not applied.")
			return 0
		}
	}

	applyReq := map[string]any{"projectDir": *project, "allowUnprobed": *allowUnprobed}
	if *claudeBin != "" {
		applyReq["claudeBin"] = *claudeBin
	}
	fmt.Fprintln(stdout, "\nrunning the enforcement probe (three tiny sessions; this can take a minute)...")
	// The apply runs live probe sessions; give the request room.
	c.http.Timeout = 10 * time.Minute
	code, body, err = c.do("POST", "/v1/connect/apply", applyReq)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
		return 1
	}
	if code != 200 {
		fmt.Fprintf(stderr, "%s: %s\n", programName, apiError(body))
		return 1
	}
	if noChanges, _ := body["noChanges"].(bool); noChanges {
		fmt.Fprintln(stdout, "already connected — nothing to apply.")
		return 0
	}
	inst, _ := body["installation"].(map[string]any)
	probe, _ := body["probe"].(map[string]any)
	fmt.Fprintf(stdout, "installed. tier: %v\n", inst["tier"])
	if evidence, ok := probe["evidence"].([]any); ok {
		for _, ev := range evidence {
			fmt.Fprintf(stdout, "  probe: %v\n", ev)
		}
	}
	fmt.Fprintf(stdout, "\nUndo any time with: %s connect undo --project %q\n", programName, *project)
	fmt.Fprintln(stdout, "Protection applies to future Claude Code sessions in that project; existing sessions are not retroactively covered.")
	return 0
}

func runConnectStatus(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("connect status", flag.ContinueOnError)
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
	code, body, err := c.do("GET", "/v1/connect", nil)
	if err != nil || code != 200 {
		fmt.Fprintf(stderr, "%s: %s\n", programName, apiError(body))
		return 1
	}
	list, _ := body["installations"].([]any)
	if len(list) == 0 {
		fmt.Fprintln(stdout, "no integrations installed.")
		return 0
	}
	for _, it := range list {
		m, _ := it.(map[string]any)
		fmt.Fprintf(stdout, "%-40v claude-code %-8v tier=%-14v %v\n", m["projectDir"], m["version"], m["tier"], m["settingsPath"])
	}
	return 0
}

func runConnectUndo(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	fs := flag.NewFlagSet("connect undo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	project := fs.String("project", "", "project directory to disconnect (required)")
	yes := fs.Bool("yes", false, "apply without asking")
	dir := fs.String("state-dir", stateDirDefault(), "state directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *project == "" {
		fmt.Fprintf(stderr, "%s: connect undo requires --project\n", programName)
		return 2
	}
	if !*yes {
		fmt.Fprintf(stdout, "remove Qiyan's hook entries from %s/.claude/settings.local.json? [y/N] ", *project)
		reader := bufio.NewReader(stdin)
		line, _ := reader.ReadString('\n')
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y") {
			fmt.Fprintln(stdout, "not removed.")
			return 0
		}
	}
	c, err := loadClient(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
		return 1
	}
	code, body, err := c.do("DELETE", "/v1/connect?project="+*project, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
		return 1
	}
	if code != 200 {
		fmt.Fprintf(stderr, "%s: %s\n", programName, apiError(body))
		return 1
	}
	removed, _ := body["removed"].(bool)
	if removed {
		fmt.Fprintln(stdout, "disconnected. Your other settings were left untouched.")
	} else {
		fmt.Fprintln(stdout, "nothing to remove.")
	}
	return 0
}
