package main

import (
	"flag"
	"fmt"
	"io"
	"time"
)

// qiyan run — start a mission's three-role fleet and follow its story.
//
// The service process orchestrates (it is the single writer of state); this
// command starts the run and then follows the event stream, printing what
// each role is doing and surfacing approvals the moment they appear. With
// --auto-approve / --auto-deny the user can pre-decide; the decision record
// says so explicitly, because "the user chose in advance" and "the user
// clicked approve" are different facts.

func runMissionRun(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	missionID := fs.String("mission", "", "mission ID to run (required)")
	claudeBin := fs.String("claude-bin", "", "Claude Code binary (default: found on PATH)")
	autoApprove := fs.Bool("auto-approve", false, "approve every approval request for this run without asking")
	autoDeny := fs.Bool("auto-deny", false, "deny every approval request for this run without asking")
	dir := fs.String("state-dir", stateDirDefault(), "state directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *missionID == "" || (*autoApprove && *autoDeny) {
		fmt.Fprintf(stderr, "%s: run requires --mission (and not both --auto-approve and --auto-deny)\n", programName)
		return 2
	}
	c, err := loadClient(*dir)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
		return 1
	}
	body := map[string]any{}
	if *claudeBin != "" {
		body["claudeBin"] = *claudeBin
	}
	code, out, err := c.do("POST", "/v1/missions/"+*missionID+"/run", body)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", programName, err)
		return 1
	}
	if code != 202 {
		fmt.Fprintf(stderr, "%s: %s\n", programName, apiError(out))
		return 1
	}
	fmt.Fprintf(stdout, "mission %s is running.\n", *missionID)

	printed := 0
	decided := map[string]bool{}
	for {
		code, status, err := c.do("GET", "/v1/missions/"+*missionID+"/run", nil)
		if err != nil {
			fmt.Fprintf(stderr, "%s: %v\n", programName, err)
			return 1
		}
		if code != 200 {
			fmt.Fprintf(stderr, "%s: %s\n", programName, apiError(status))
			return 1
		}
		events, _ := status["events"].([]any)
		for ; printed < len(events); printed++ {
			ev, _ := events[printed].(map[string]any)
			if ev == nil {
				continue
			}
			switch ev["kind"] {
			case "role-start", "role-end":
				fmt.Fprintf(stdout, "[%s] %v: %v\n", ev["role"], ev["kind"], ev["text"])
			case "approval-pending":
				id, _ := ev["approvalId"].(string)
				fmt.Fprintf(stdout, "\n*** NEEDS YOUR DECISION ***\n%v\n", ev["text"])
				if *autoApprove || *autoDeny {
					approve := *autoApprove
					if !decided[id] {
						decided[id] = true
						code, res, err := c.do("POST", "/v1/approvals/"+id+"/decision", map[string]any{"approve": approve})
						if err != nil || code != 200 {
							fmt.Fprintf(stderr, "%s: decision failed: %v %v\n", programName, err, apiError(res))
						} else if approve {
							fmt.Fprintln(stdout, "auto-approved (your --auto-approve choice).")
						} else {
							fmt.Fprintln(stdout, "auto-denied (your --auto-deny choice).")
						}
					}
				} else {
					fmt.Fprintf(stdout, "approve with:  %s approve %s\n", programName, id)
					fmt.Fprintf(stdout, "deny with:     %s deny %s\n", programName, id)
				}
			case "approval-decided":
				fmt.Fprintf(stdout, "decision recorded: %v\n\n", ev["outcome"])
			case "error":
				fmt.Fprintf(stdout, "error: %v\n", ev["text"])
			case "mission-end", "run-end":
				if txt, _ := ev["text"].(string); txt != "" {
					fmt.Fprintf(stdout, "%v\n", txt)
				}
			}
		}
		state, _ := status["state"].(string)
		if state != "running" {
			fmt.Fprintf(stdout, "run state: %s\n", state)
			if summary, _ := status["summary"].(string); summary != "" {
				fmt.Fprintf(stdout, "summary:   %s\n", summary)
			}
			return 0
		}
		time.Sleep(time.Second)
	}
}
