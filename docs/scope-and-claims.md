# Scope and claim boundary

This document states exactly what this repository does and does not do today. It
exists so that no reader, and no automated system, can mistake an intention for
an implemented capability. If the code and this document ever disagree, the code
is wrong and this document is the specification.

## What is implemented and verified with real runs

| Capability | Detail | Evidence |
| --- | --- | --- |
| Local control service | `qiyan serve`: owns all state, serves the versioned `/v1` API and the embedded console on loopback only | API tests + live runs |
| Action control engine | One authority for every interface: ALLOW / DENY / REQUIRE_APPROVAL / UNKNOWN, with identity binding, scope containment, budgets, delegation rules, single-use approvals | Engine tests incl. negatives; adversarial-review fixes with regression tests |
| Durable local state | Append-only log + atomic snapshots; corruption or sequence gaps **block** the store, and a blocked store denies everything | Store tests incl. corruption, gap, compaction, claim races |
| PreToolUse hook | `qiyan hook pretooluse`: real pre-execution blocking; every failure mode exits 2 (deny); no decision is ever invented | Hook tests; R02-00 matrix of 40+ live nested runs |
| Claude Code connector | `qiyan connect claude`: detects version, **runs a live three-scenario probe on this machine**, installs project-scoped settings with `onFailure: "block"`, keeps a one-time backup, and undoes exactly its own entries | Connector tests + live verification (install → blocked/allow runs → undo) |
| Three-role fleet | `qiyan run`: Planner → Builder → Reviewer as three real sessions with distinct identities, real hand-offs, approval pauses and resumes | Unit tests + live three-session run producing real files |
| Agent Safety Lab | Scans a Qiyan manifest, a Claude Code project, or an MCP JSON config; every finding carries a source and an honest boundary; suggestions preview, apply (deny rules), **verify with a real probe**, and undo | Lab tests + live scan/apply/verify |
| Reference Agent Host | `cmd/qiyanhost`: a separate binary that uses only the public HTTP API; obeys decisions; demonstrates allow, deny, approval and a rogue bypass the system does not claim to control | Live F03 run |
| Demo & training | A synthetic 60-second demo and two interactive training scenarios; the package cannot even reach the state store (separation test) | Scenario tests + live simulation |
| Console | Embedded bilingual HTML/JS/CSS: home, mission control, connect wizard, safety lab, demo, developer entry; CSP with no inline script; all rendered data via textContent | Serving tests + live smoke |

## What is verified about Claude Code versions specifically

Measured on this machine with real nested runs (see the R02-00 record):

| Version | Deny blocks? | Hook failure fails closed? | Supported for |
| --- | --- | --- | --- |
| ≥ 2.1.295 (probed: 2.1.295, 2.1.300) | yes | yes (`onFailure: "block"`) | full protection; the connector's live probe re-verifies per machine |
| 2.1.292, 2.1.68 | yes | **no — fails open** | explicit denials only; the connector refuses to claim fail-closed and says why |

A version number is never the claim. `qiyan connect claude` runs the probe on
the actual machine and records the measured tier.

## What is not implemented, or explicitly not claimed

- **No operating-system sandbox.** Path checks are lexical and scope-based;
  symlink races and direct process behavior outside Qiyan's integration points
  are outside what Qiyan can prove, and are reported that way.
- **No promise to control commands Qiyan cannot interpret.** Unrecognized
  commands, unrecognized tools, path-form programs and shell metacharacters go
  to approval (or denial) — never to a silent allow.
- **Command containment is curated, not general.** Only commands whose
  recognized behavior is proven free of side effects run without asking — a
  small set of print-only tools (ls, cat, grep, …) with their path arguments
  checked against the authorized roots, plus compiler-class subcommands that
  do not execute project code (go build/vet/list) and content-free git
  readers (status, rev-parse, …). Anything that can run code or change
  configuration goes to your approval: this includes `go test` (it executes
  the project's own test binaries), `go env -w/-u` (persistent configuration),
  `date -s` / positional `date` and `hostname NAME` (system-state setters),
  git content transformers (`log`, `diff`, `show`, `blame` — repository
  configuration can make them execute helper programs), and flag forms that
  run external helpers (`-vettool`, `-toolexec`, `-exec`).
- **MCP servers are never started or contacted.** The lab reads their
  configuration only.
- **No network force-blocking.** Outbound tool actions are gated (approval),
  and the coverage table says what is enforced where.
- **No other runtimes.** Only Claude Code is an officially supported
  integration; Cursor, ChatGPT, Codex, n8n and others are not claimed.
- **No SaaS, no accounts, no telemetry.** Everything is local.
- **No auto-fix of arbitrary vulnerabilities.** The lab's auto-apply covers
  exactly the deny-rule addition it previews and verifies.
- **One process owns the state.** `qiyan serve` is the single writer; the hook
  and CLI are clients. If the service cannot be reached, protected actions are
  denied (fail closed), never allowed.

## What is not released

No version of this project has been published as a product release. The release
drill remains a provenance-attested test artifact only.

## Evidence index

Real-run records from the R02 build live alongside the repository (session
transcripts, probe logs, E2E logs). Within the repository, the acceptance
report at `docs/R02-ACCEPTANCE-REPORT.md` lists what was run where, and the
fleet/lab/connector tests re-run the core of it on any machine.
