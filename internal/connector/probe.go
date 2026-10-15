package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The real enforcement probe.
//
// This is the R02-00 verification campaign distilled into a function the
// product runs itself, because the book's rule stands: a version number is
// not evidence. The probe installs throwaway settings in a temp directory,
// runs the actual runtime against them with a harmless `touch` target, and
// records what really happened:
//
//	deny-blocked      a deny decision stopped the call before execution
//	failure-blocks    a crashing hook (onFailure:"block") stopped it too —
//	                  this is what separates ENFORCED from blocking-only
//	allow-runs        a normal call was still allowed through (S02)
//
// The hook used is `qiyan hook probe`, a self-contained entry in this same
// binary, so the probe needs nothing installed on the machine but Qiyan
// itself and the runtime.
type ProbeResult struct {
	Version       string   `json:"version"`
	DenyBlocked   bool     `json:"denyBlocked"`
	FailureBlocks bool     `json:"failureBlocks"`
	AllowRuns     bool     `json:"allowRuns"`
	Evidence      []string `json:"evidence"`
}

// Tier returns the strongest support tier this live run proved — regardless
// of what the version number suggested.
func (p ProbeResult) Tier() SupportTier {
	switch {
	case p.DenyBlocked && p.FailureBlocks && p.AllowRuns:
		return TierEnforced
	case p.DenyBlocked:
		return TierBlockingOnly
	default:
		return TierUnsupported
	}
}

// ProbeReal runs the three live scenarios against claudeBinary. qiyanBinary
// is the path to this product's own executable (os.Executable()).
func ProbeReal(ctx context.Context, claudeBinary, qiyanBinary, workRoot string) (ProbeResult, error) {
	result := ProbeResult{}
	det, err := Detect(claudeBinary)
	if err != nil {
		return result, err
	}
	result.Version = det.Version
	claudeBinary = det.Binary

	scenarios := []struct {
		name        string
		mode        string
		expectBlock bool
		record      func(bool)
	}{
		{"deny-blocked", "deny2", true, func(blocked bool) { result.DenyBlocked = blocked }},
		{"failure-blocks", "crash", true, func(blocked bool) { result.FailureBlocks = blocked }},
		{"allow-runs", "allow", false, func(ran bool) { result.AllowRuns = ran }},
	}

	for _, sc := range scenarios {
		blocked, ran, evidence, err := runScenario(ctx, claudeBinary, qiyanBinary, workRoot, sc.name, sc.mode)
		if err != nil {
			result.Evidence = append(result.Evidence, fmt.Sprintf("%s: run failed: %v", sc.name, err))
			sc.record(false)
			continue
		}
		result.Evidence = append(result.Evidence, fmt.Sprintf("%s: %s", sc.name, evidence))
		if sc.expectBlock {
			sc.record(blocked)
		} else {
			sc.record(ran)
		}
	}
	return result, nil
}

// runScenario performs one nested run and reports (blocked, ran, evidence).
func runScenario(ctx context.Context, claudeBinary, qiyanBinary, workRoot, name, mode string) (bool, bool, string, error) {
	dir, err := os.MkdirTemp(workRoot, "qiyan-probe-"+name+"-")
	if err != nil {
		return false, false, "", err
	}
	defer os.RemoveAll(dir)

	settings := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Bash",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": fmt.Sprintf("\"%s\" hook probe %s", qiyanBinary, mode),
							"timeout": 5,
							// Set on every scenario: if a version ignores it,
							// the crash scenario failing to block is exactly
							// the signal the probe is looking for.
							"onFailure": "block",
						},
					},
				},
			},
		},
	}
	settingsPath := filepath.Join(dir, "probe-settings.json")
	raw, _ := json.MarshalIndent(settings, "", "  ")
	if err := os.WriteFile(settingsPath, raw, 0o600); err != nil {
		return false, false, "", err
	}

	artifact := filepath.Join(dir, "artifact.txt")
	prompt := fmt.Sprintf("Use the Bash tool to run exactly this command once: touch \"%s\" — then reply with one word: DONE if the command succeeded, or BLOCKED if it was blocked.", artifact)

	runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, claudeBinary,
		"-p", "--output-format", "json",
		"--permission-mode", "default",
		"--allowedTools", "Bash(touch:*)",
		"--settings", settingsPath,
		prompt,
	)
	cmd.Dir = dir
	start := time.Now()
	out, runErr := cmd.CombinedOutput()
	elapsed := time.Since(start).Round(time.Millisecond)

	_, statErr := os.Stat(artifact)
	ran := statErr == nil
	blocked := !ran

	evidence := fmt.Sprintf("ran=%v in %s", ran, elapsed)
	if runErr != nil {
		// The nested runtime exiting non-zero is not itself a failure of the
		// probe; report the tail of its output for the record.
		tail := strings.TrimSpace(string(out))
		if len(tail) > 200 {
			tail = tail[len(tail)-200:]
		}
		evidence += fmt.Sprintf(" (runtime exit: %v, output tail: %q)", runErr, tail)
	}
	return blocked, ran, evidence, nil
}
