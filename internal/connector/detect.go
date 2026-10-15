// Package connector implements the officially supported Claude Code
// integration: version detection, the real enforcement probe, project-scoped
// settings installation with preview and undo, and the coverage records that
// back every "protected" claim.
//
// The discipline of this package comes straight from the R02-00 verification:
// version numbers are a hint, never evidence. A version claims fail-closed
// support only after a live probe has actually blocked a call on this
// machine, and every coverage record names the run that proved it.
package connector

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SupportTiers classify what a runtime version can actually do, ordered from
// strongest to weakest. The tiers exist because the earlier probe campaign
// showed adjacent versions behaving differently: 2.1.292 (a released
// "stable") fails open on hook failure while 2.1.295 blocks.
type SupportTier string

const (
	// TierEnforced: pre-execution deny works AND hook failure/timeout means
	// block (requires onFailure:"block" support, verified live).
	TierEnforced SupportTier = "enforced"
	// TierBlockingOnly: deny works, but a failed or timed-out hook fails
	// OPEN — usable for explicit denials, never for fail-closed claims.
	TierBlockingOnly SupportTier = "blocking-only"
	// TierUnsupported: hooks could not be confirmed at all.
	TierUnsupported SupportTier = "unsupported"
)

// Detected describes a discovered Claude Code installation.
type Detected struct {
	// Binary is the resolved executable path.
	Binary string
	// Version is the parsed dotted version, e.g. "2.1.300".
	Version string
	// TierFromVersion is what the version number alone implies. It is NOT the
	// support claim — ProbeReal decides that.
	TierFromVersion SupportTier
}

// failClosedFloor is the first version documented (and independently probed
// in R02-00) to support onFailure:"block". Below it, and only below it, the
// version alone bottoms out at TierBlockingOnly.
var failClosedFloor = [3]int{2, 1, 295}

var versionRE = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// Detect resolves the Claude Code binary and version. binary may be empty, in
// which case PATH is searched.
func Detect(binary string) (Detected, error) {
	if binary == "" {
		path, err := exec.LookPath("claude")
		if err != nil {
			return Detected{}, fmt.Errorf("claude not found on PATH: %w", err)
		}
		binary = path
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary, "--version").CombinedOutput()
	if err != nil {
		return Detected{}, fmt.Errorf("running %s --version: %w (output: %s)", binary, err, strings.TrimSpace(string(out)))
	}
	version, tier, err := ParseVersion(string(out))
	if err != nil {
		return Detected{}, err
	}
	return Detected{Binary: binary, Version: version, TierFromVersion: tier}, nil
}

// ParseVersion extracts the version from `--version` output and derives the
// version-only tier. The output looks like "2.1.300 (Claude Code)".
func ParseVersion(output string) (string, SupportTier, error) {
	m := versionRE.FindStringSubmatch(output)
	if m == nil {
		return "", TierUnsupported, fmt.Errorf("no version number in %q", strings.TrimSpace(output))
	}
	nums := [3]int{}
	for i := 0; i < 3; i++ {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return "", TierUnsupported, fmt.Errorf("bad version component in %q", m[0])
		}
		nums[i] = n
	}
	version := fmt.Sprintf("%d.%d.%d", nums[0], nums[1], nums[2])
	if compareVersion(nums, failClosedFloor) >= 0 {
		return version, TierEnforced, nil
	}
	return version, TierBlockingOnly, nil
}

func compareVersion(a, b [3]int) int {
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// SupportFloor returns the oldest version whose fail-closed behavior is
// documented by the vendor, as a display string. Actual support still
// requires ProbeReal on the machine in question.
func SupportFloor() string {
	return fmt.Sprintf("%d.%d.%d", failClosedFloor[0], failClosedFloor[1], failClosedFloor[2])
}
