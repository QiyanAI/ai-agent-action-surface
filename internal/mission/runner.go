// Package mission orchestrates Qiyan's three-role fleet — Planner, Builder,
// Reviewer — over real agent runtimes. Each role is a separate, identifiable
// session (book §11.1); every tool call they make re-enters the Action
// Control Engine through the installed hook. The orchestrator never decides
// anything itself: it schedules sessions, drives hand-offs, and reacts to the
// engine's answers (including pausing at approvals).
package mission

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// SessionResult is the outcome of one non-interactive runtime turn.
type SessionResult struct {
	FinalText string
	IsError   bool
	// Blocked reports that the runtime stopped because a tool call was
	// denied or is waiting for approval (observed via the decision trail,
	// not guessed from prose).
	Blocked bool
	// ExitCode of the runtime process; 0 on success.
	ExitCode int
}

// SessionRunner runs one role's session. The real implementation shells out
// to Claude Code; tests substitute a scripted fake.
type SessionRunner interface {
	// Start launches the first turn with the given prompt.
	Start(ctx context.Context, prompt string) (SessionResult, error)
	// Resume continues the same session after a pause (approval decided).
	Resume(ctx context.Context, prompt string) (SessionResult, error)
}

// ClaudeSession runs a real Claude Code session with Qiyan's hook settings.
type ClaudeSession struct {
	Binary    string
	SessionID string
	Cwd       string
	Settings  string // path to the generated mission settings file
	ExtraEnv  []string
}

// claudeResult is the shape of `claude -p --output-format json` output.
type claudeResult struct {
	IsError    bool   `json:"is_error"`
	Result     string `json:"result"`
	Subtype    string `json:"subtype"`
	StopReason string `json:"stop_reason"`
}

func (s *ClaudeSession) run(ctx context.Context, prompt string, first bool) (SessionResult, error) {
	args := []string{"-p", "--output-format", "json",
		"--permission-mode", "default",
		// Mission sessions run with exactly Qiyan's generated settings and
		// the user-level baseline — never the project layer, so a project
		// that also has the installed integration cannot double-fire hooks
		// (two decisions for one call would block the second).
		"--setting-sources", "user",
		"--settings", s.Settings,
	}
	if first {
		args = append(args, "--session-id", s.SessionID)
	} else {
		args = append(args, "--resume", s.SessionID)
	}
	args = append(args, prompt)

	cmd := exec.CommandContext(ctx, s.Binary, args...)
	cmd.Dir = s.Cwd
	if len(s.ExtraEnv) > 0 {
		cmd.Env = append(cmd.Environ(), s.ExtraEnv...)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	start := time.Now()
	err := cmd.Run()
	_ = start

	// Parse the JSON result line (warnings may precede it).
	res := SessionResult{}
	var parsed bool
	for _, line := range reverseLines(out.String()) {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var r claudeResult
		if json.Unmarshal([]byte(line), &r) == nil {
			res.FinalText = r.Result
			res.IsError = r.IsError
			res.Blocked = r.StopReason == "tool_deferred"
			parsed = true
			break
		}
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		} else if ctx.Err() != nil {
			return res, ctx.Err()
		} else {
			return res, err
		}
	}
	if !parsed && res.ExitCode != 0 {
		return res, fmt.Errorf("runtime exited %d with no parseable result", res.ExitCode)
	}
	return res, nil
}

// Start implements SessionRunner.
func (s *ClaudeSession) Start(ctx context.Context, prompt string) (SessionResult, error) {
	return s.run(ctx, prompt, true)
}

// Resume implements SessionRunner.
func (s *ClaudeSession) Resume(ctx context.Context, prompt string) (SessionResult, error) {
	return s.run(ctx, prompt, false)
}

func reverseLines(s string) []string {
	lines := strings.Split(s, "\n")
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return lines
}
