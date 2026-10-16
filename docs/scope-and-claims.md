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
- **Shell commands: there is no name-based auto-allow at all — every shell
  command waits for your approval, and this is not a sandbox.** Successive
  independent reviews of any "safe program" list kept producing same-class
  counterexamples — `go test` runs project code, `go env -w/-u` persists
  configuration, `git status` executes a configured `core.fsmonitor` hook,
  `sort -o`/`uniq in out`/`date -s`/`hostname NAME` have write or state
  modes, and `file --compile -m …` writes a compiled magic file — because
  any program's behavior can be redirected by arguments, configuration or
  environment that pure string inspection cannot verify, and repository-local
  configuration is writable by the very agent being gated. So the mechanism
  itself was removed: no name whitelist, no argument blacklist; a shell
  action is never executed by default. **What does not change:** the native
  semantic tools with defined meanings — Read, Glob, Grep and the session
  bookkeeping tools — keep their own rules and keep auto-executing inside the
  authorized scope, and out-of-scope access through them is still a hard
  DENY; shell actions whose kind or tool is outside the mission scope are
  likewise DENIED, not merely asked about. Beyond the classifier the boundary
  is stated honestly: a shell command name resolves through the session's
  PATH and environment, which are the user's own trust domain — Qiyan
  verifies its gate's decisions, not the user's toolchain installation.
- **MCP servers are never started or contacted.** The lab reads their
  configuration only.
- **No network force-blocking.** Outbound tool actions are gated (approval),
  and the coverage table says what is enforced where.
- **No other runtimes.** Only Claude Code is an officially supported
  integration; Cursor, ChatGPT, Codex, n8n and others are not claimed.
- **No SaaS, no accounts, no telemetry.** Everything is local: Qiyan's own
  control service runs on loopback and uploads nothing to any Qiyan service.
  This claim covers Qiyan itself, not the external tools you connect: when you
  connect Claude Code, its model calls go to Anthropic under Anthropic's own
  network and data-handling terms, and Qiyan neither intermediates nor
  controls that traffic.
- **No auto-fix of arbitrary vulnerabilities.** The lab's auto-apply covers
  exactly the deny-rule addition it previews and verifies.
- **One process owns the state.** `qiyan serve` is the single writer; the hook
  and CLI are clients. If the service cannot be reached, protected actions are
  denied (fail closed), never allowed.

## Release status

No stable, production-certified version of this project has been published. The
one public version is an early-access Beta: **v0.3.0-beta.1**, which is listed
on the Releases page as a GitHub prerelease once it is published. A Beta is a
public, downloadable early test build, and explicitly not a stable release. The
release drill remains a provenance-attested test artifact only.

R03 early-access candidates (the `Candidate Build` workflow) are **TEST
BUILDS**: bare, unsigned, CGO-free binaries. Each artifact set carries a
`CANDIDATE-LABEL.txt` stating `EARLY ACCESS / TEST BUILD /
NOT PRODUCTION-CERTIFIED` (the label travels with the files), plus SHA-256
checksums and a build-provenance attestation that binds each binary to the
exact commit it was built from. They create no
GitHub Release, carry no support or compatibility promise, and describe the
platforms they were actually built for — not a supported-platform list. A
downloaded candidate must stay where it is put: the installed Claude Code
integration points at the binary's absolute path, and moving or deleting it
makes the gate fail closed and block that project's tools.

**Beta builds** (v0.3.0-beta.1 and any later early-access build published on
the Releases page) carry the same boundary and one more: a Beta is early
access, **not a production security certification**. Provenance attests where
the file was built from — it says nothing about whether the software is free
of defects, and nothing outside the coverage records is being claimed as
protected.

## Evidence index

Real-run records from the R02 build live alongside the repository (session
transcripts, probe logs, E2E logs). Within the repository, the acceptance
report at `docs/R02-ACCEPTANCE-REPORT.md` lists what was run where, and the
fleet/lab/connector tests re-run the core of it on any machine.
