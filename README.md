# Qiyan — Open Agent Mission & Control Platform

**Your AI Team. Your Rules.**

Qiyan is a local, open platform for running AI agents under verifiable
pre-execution control: connect your existing AI software, hand a mission to a
small crew of agents, watch every action, and decide what they may do. Safety
is enforced *before* something runs — not described afterwards. And other
software can put the same control in front of its own actions through Qiyan's
open, local Control API.

Every capability below links to a reproducible run record
([R03 acceptance evidence](docs/R03-ACCEPTANCE-REPORT.md)), and what is *not*
covered is stated plainly ([scope and claim boundary](docs/scope-and-claims.md)).

Everything is local-first: no account, no cloud, no telemetry — Qiyan sends
nothing to any Qiyan service. English and 中文 are both first-class.

**Try it now:** the **v0.3.0-beta.1 Public Beta** builds — five platforms, with
checksums and build provenance — are prepared and awaiting the maintainer's
release authorization; when they ship they appear on
[Releases](https://github.com/QiyanAI/ai-agent-action-surface/releases). No
account is needed, and a 60-second demo ships inside. Until the Beta is live:
build from source in two commands, or ask the maintainer for an early-access
candidate — either way you are one step from the demo. Full steps:
[Try it](#try-it).

![The console home: service status, four entries, and the no-account demo](docs/images/console-home.png)

## What it can do for you

- **Let a small AI team do the work.** Give one goal; a Planner, a Builder and
  a Reviewer work in three real sessions on your machine — and every sensitive
  action pauses for your decision.
- **Check whether an AI is risky.** The Safety Lab reads an agent's declared
  abilities and project wiring, attaches evidence to every finding in plain
  words, and its suggestions can be applied and *verified on your machine*.
- **Put safety control into your own software.** Any agent host can ask
  Qiyan's open, local Control API before acting: `ALLOW` runs, `DENY` stops,
  `REQUIRE_APPROVAL` waits for exactly one execution of exactly that tool call
  (supply `contentDigest` to bind the written bytes too — see the contract).

## Understand it in 30 seconds

1. **Connect** your Claude Code project (a wizard; one project file changes,
   backed up first, undoable).
2. A live probe **measures** whether dangerous actions can really be stopped on
   this machine — the version number is never trusted.
3. **Give the fleet a mission**; the Planner plans, the Builder works, the
   Reviewer checks.
4. Anything sensitive **stops and asks you** — approve once, or deny; a denied
   action stays denied for that task.
5. Every decision is **recorded with evidence**, and the console shows what is
   actually enforced — and what is not.

![A real run pausing on a real approval, with the reason in plain words](docs/images/mission-approval.png)

## Try it

**No account needed.** A 60-second synthetic demo ships inside the console:
three agents working, one action blocked, one decision you make yourself.
Synthetic content is labelled `DEMO` everywhere and can never be recorded as a
real observation.

### Get the build (no Go toolchain)

> **EARLY ACCESS / BETA / NOT PRODUCTION-CERTIFIED.** v0.3.0-beta.1 is an
> unsigned, CGO-free binary for linux/amd64, linux/arm64, darwin/amd64,
> darwin/arm64 and windows/amd64. It is early access — **not** a production
> security certification — and carries no support or compatibility promise.
> It is prepared and awaiting the maintainer's release authorization; when it
> ships, the files attach to
> [Releases](https://github.com/QiyanAI/ai-agent-action-surface/releases)
> together with `SHA256SUMS`, `LICENSE.txt` and a build-provenance attestation
> binding each binary to its exact commit. Until then, invited testers receive
> current candidates through the dispatch-only **Candidate Build** workflow
> (Actions → Candidate Build), and anyone can build from source below.
>
> Windows SmartScreen and macOS Gatekeeper will warn on first run; there is no
> signing — and do not disable those warnings for other software. **Keep the
> binary where you put it** (a folder you will keep, before connecting any
> project): connected projects point at its absolute path, and moving or
> deleting it makes the gate fail closed and block that project's tools.
> Step-by-step install, verify and uninstall instructions — including what is
> built vs actually tested vs officially supported:
> [docs/install.md](docs/install.md). Known limitations and the security
> report channel travel with the release and in
> [docs/scope-and-claims.md](docs/scope-and-claims.md).

Download for your platform, then verify and run:

```sh
sha256sum -c SHA256SUMS        # every line must say OK
gh attestation verify qiyan_0.3.0-beta.1_<os>_<arch> --repo QiyanAI/ai-agent-action-surface
./qiyan_0.3.0-beta.1_<os>_<arch> serve
# prints the console address, a one-click sign-in link, the state dir and token paths
```

Open the printed console address (or the sign-in link) and start with the
60-second demo — no account, no Claude Code needed — or connect a real
project as below.

### Run it from source (Go 1.27+)

```sh
go build -o qiyan ./cmd/qiyan
./qiyan serve
```

Everything is standard library only; there are no third-party dependencies to
download.

### Where it runs: built vs tested vs supported

Three different things, stated separately — built is not tested, and tested is
not supported:

| State | What it covers |
| --- | --- |
| **Built** | linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 — the five targets above, each bound to its exact commit by the candidate build's gate and attestation |
| **Actually tested** | windows/amd64: a real candidate ran (service boot, `/v1/health`, console served, 2026-10-15); CI additionally compiles and runs `qiyan version`/`help` on ubuntu-, windows- and macos-latest; all deep feature verification in R02/R03 ran on one **Windows 11** machine. The linux/arm64 and darwin/amd64 candidate files have **never been executed**; linux/amd64 and darwin/arm64 were smoke-run only through CI's runner-native builds |
| **Officially supported** | **No platform list exists yet.** Runtime support: only Claude Code, fail-closed on 2.1.295+ (measured per machine by the connector's live probe — a version number is never trusted) |

Treat anything not in the "actually tested" row as unverified, not unsupported.
CI runs on **ubuntu-latest, windows-latest and macos-latest**.

## Connect your own AI project

```sh
qiyan connect claude --project D:/work/my-project
```

The wizard detects Claude Code automatically (or takes an explicit binary),
shows the exact one file it will change, runs the live enforcement probe, and
only then installs. The console version has a visual project picker, plain
language, and the same rules: nothing global, one backup, undo removes exactly
what Qiyan added.

![Measured protection tier after a real probe — with the weaker tier stated as plainly as the stronger one](docs/images/connect-measured.png)

Then give the fleet a mission (console, or CLI):

```sh
qiyan mission create --goal "Read the notes and write one summary report" \
    --root D:/work/my-project
qiyan run --mission mi_...
# decisions that need you appear in the console and in `qiyan approvals list`
```

## Safety boundaries (the short, honest list)

- Only **Claude Code** is a supported runtime. Fail-closed protection needs
  **2.1.295+**; the connector probes the actual machine and refuses to claim
  what it cannot verify.
- **Shell commands always wait for your approval.** There is no name-based
  auto-allow at all, by design (see scope-and-claims for why). Commands Qiyan
  cannot interpret are never silently allowed.
- Path checks are **lexical and scope-based** — real, verified gates, but
  **not an operating-system sandbox**. If your host can bypass the Control
  API, Qiyan does not claim to control that path; the coverage table says what
  is actually enforced.
- No SaaS, no accounts, no telemetry — Qiyan's own control service runs on
  loopback and uploads nothing to any Qiyan service. When you connect Claude
  Code, its model calls go to Anthropic under Anthropic's own network and
  data-handling terms; Qiyan neither intermediates nor controls that traffic.

The full, precise list of what is and is not claimed:
[docs/scope-and-claims.md](docs/scope-and-claims.md) ·
[中文](docs/scope-and-claims.zh-CN.md).

## For developers: the Qiyan Open Control Contract

The contract spells one loop in five stages —
**Action → Risk & Evidence → Authorization → Enforcement → Outcome** — so any
agent host can ask before it acts and prove after it acted. The console's
**Developers** entry walks the whole flow — and there is a guided tour for it.
The interface is a local, versioned HTTP/JSON API: `POST /v1/authorize` before
acting, poll approvals, report results with `POST /v1/events`. The repository
ships a fully independent reference host:

```sh
go run ./cmd/qiyanhost --addr 127.0.0.1:8760 --token $AGENT_TOKEN \
    --mission mi_... --task ta_... --workdir D:/scratch
```

Contract documentation (endpoints, decision semantics, binding rules, error
classes, coverage, compatibility policy):
[docs/developer-api.md](docs/developer-api.md) ·
[中文 docs/developer-api.zh-CN.md](docs/developer-api.zh-CN.md).

## What is verified today

| Area | State |
| --- | --- |
| Local control service, engine, durable state | implemented, tested, verified in live runs |
| PreToolUse gate with real pre-execution blocking | verified on real Claude Code runs (deny blocks; failure modes block with `onFailure: "block"` on versions ≥ 2.1.295) |
| Three-role real fleet (Planner → Builder → Reviewer) | verified: live run producing real files with real control decisions |
| Safety Lab including verified fixes | verified: scan → preview → apply → real probe → ENFORCED coverage record → undo |
| Reference Agent Host (public API only) | verified: allow / deny / approval / rogue-bypass demonstration |
| Demo & training | synthetic, interactive, data-layer-separated from real events |
| R03 acceptance evidence | [docs/R03-ACCEPTANCE-REPORT.md](docs/R03-ACCEPTANCE-REPORT.md) |

### The Action Graph

Underneath every layer is one structure: **Agent → Tool → Action Capability**,
with every edge backed by evidence, not by assertion. An action kind carries
capabilities (`read_files`, `write_files`, `execute_process`, …); a tool name
maps to the capabilities it can exercise; and the resulting record is only as
strong as its `EvidenceLevel` — `DECLARED` (a manifest says so),
`STATICALLY_CONNECTED` (config wiring), `OBSERVED` (an integration saw it), or
`ENFORCED` (a real pre-execution gate proved it stopped). An evidence record
never appears above its level, and every enforced claim carries a stated
`Boundary` — what it does *not* cover. That is also why the console shows
coverage as `ENFORCED / OBSERVED / UNCOVERED / UNKNOWN` per execution path,
and why there is deliberately no `protected: true` shortcut anywhere in the
API: an unexercised path stays `UNKNOWN`, and unknown is not a promise.
(`internal/domain/evidence.go`, `capability.go`.)

## Repository layout

```
cmd/qiyan        the CLI: serve / hook / status / agents / approvals / mission /
                 task / session / connect / run
cmd/qiyanhost    the independent reference Agent Host (public API only)
internal/domain  identities, states, actions, policy, evidence, coverage
internal/control the single action-decision engine
internal/store   durable local state (single writer: serve)
internal/api     the versioned local Control API + embedded console
internal/connector  Claude Code detection, live probe, settings install/undo
internal/mission    the three-role fleet orchestrator
internal/lab        the Agent Safety Lab
internal/playground synthetic demo & training content (inert by construction)
```

## Feedback

Bug reports, feature requests, adapter requests and documentation problems all
go through
[issues](https://github.com/QiyanAI/ai-agent-action-surface/issues/new/choose) —
there are guided forms for each. This is a single-maintainer project: responses
are best-effort, and no service level is promised. Suspected vulnerabilities do
**not** go in issues — use the private channel in [SECURITY.md](SECURITY.md).

## Security, license and contributing

Apache-2.0. Standard library only. Vulnerability reporting:
[SECURITY.md](SECURITY.md) · contribution rules: [CONTRIBUTING.md](CONTRIBUTING.md).
