package control

import "strings"

// Shell command handling.
//
// Qiyan does not classify shell commands at all any more. The boundary is the
// user's approval, for every shell command, without exception:
//
//   - Successive independent reviews walked through every version of a
//     name-based auto-allow list and kept finding same-class counterexamples:
//     `go test` runs project code; `go env -w/-u` persists configuration;
//     `git status` executes a configured core.fsmonitor hook; `sort -o`,
//     `uniq in out`, `date -s`, `hostname NAME` have write/state modes; and
//     `file --compile -m …` writes a compiled magic file. Any curated name —
//     even one whose *recognized* usage prints — carries modes and
//     configuration lookups that pure string inspection cannot rule out.
//   - Without a real isolation guarantee for generic shell execution (which
//     this project explicitly does not claim — no OS sandbox, no path
//     enforcement inside arbitrary processes), the only honest default is
//     REQUIRE_APPROVAL for the whole class. There is deliberately NO name
//     whitelist and NO argument blacklist to bypass: the mechanism itself
//     was removed, not patched.
//
// What this does NOT change:
//
//   - Native semantic tools with defined meanings — Read, Glob, Grep (tool
//     layer, mapped to file-read actions) — keep their own rules and keep
//     auto-executing inside the authorized scope, with out-of-scope access
//     still hard-denied. Session bookkeeping tools still pass through.
//   - Every structural gate (identity, tool/kind scope, budgets, delegation,
//     cancellation) applies to shell actions exactly as before; an exec
//     action whose kind or tool is outside the mission scope is still DENIED,
//     not merely asked about.

// ExtractProgram returns the base program name of a command line, for the
// action's digest field and for the approval message's {program} parameter.
// It is presentation/identity only: it decides nothing.
func ExtractProgram(cmd string) string {
	fields := strings.Fields(strings.TrimSpace(cmd))
	if len(fields) == 0 {
		return ""
	}
	p := strings.ToLower(strings.Trim(fields[0], `"'"`))
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		p = p[i+1:]
	}
	return p
}
