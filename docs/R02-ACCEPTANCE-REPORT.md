# R02 — Acceptance report

**Status: IMPLEMENTATION COMPLETE — VERIFIED (with two explicitly NOT
VALIDATED experience gates; see §9). Final acceptance belongs to the user.**

**Verified code state:** `b4876cf0babed4d9e596b1e78f15a14259e14069`; amended by
the R02-SEC-01 fix at `0d894507175fcf97f8b416baa490801525650f22` and converged
at `f7ffba4c491e6ca94d8cfa05531ea9aa96234f18` (all on `r02/mission-control`;
tree hashes byte-identical to the locally tested trees — verified by tree-hash
comparison after each landing). This report is added in a report-only commit
on top of that state; no code changed by this commit.

**Baseline:** `main` @ `e6d2842042370cd268d81581a751125125c7887c`, exactly the
SHA the R02 V2.0 task book names. R01 was neither reopened nor modified.

**How this was produced:** every claim below comes from a real run — local
test suites, live end-to-end executions on this machine (Windows 11), and the
three-OS CI on the pull request. Evidence artifacts live alongside the
session; the repository tests re-derive the core of them on any machine.

---

## 1. GitHub and code (§25.1)

- 13 commits landed from `e6d2842` → `b4876cf0` (the git transport to
  github.com was intermittently unreachable during the build; commits were
  landed through the GitHub REST API from the same local objects, and the
  remote tree hash was then compared byte-for-byte — identical).
- Modules implemented: `internal/domain` (core model), `internal/control`
  (single decision engine), `internal/store` (durable state), `internal/api`
  (versioned Control API + embedded console), `internal/connector` (Claude
  Code integration), `internal/mission` (three-role fleet), `internal/lab`
  (Safety Lab), `internal/playground` (synthetic demo/training), `cmd/qiyan`
  (CLI: serve / hook / connect / run + operator commands), `cmd/qiyanhost`
  (independent reference host).
- Build: standard-library only (`go.sum` remains empty; CI's dependency gate
  enforced). `gofmt -l` clean, `go vet ./...` clean, `go test ./...` green
  locally and on the CI matrix (Linux, Windows, macOS), plus the CI security
  job (govulncheck + no-third-party-dependencies). CI run `38726057926` on PR
  #7: **all four jobs green** (ubuntu-latest, windows-latest, macos-latest,
  security checks) on the verified tree; this report commit re-runs the same
  matrix.
- **R01 did not regress** (F05): `qiyan help`, `qiyan version`, unknown-command
  exit-2 semantics, the 3-OS CI, the release drill and the stdlib-only gate
  are all preserved and asserted by tests (`cmd/qiyan/main_test.go`); the bare
  version constant moved from `0.0.0-r01` to `0.2.0-r02` and build identity is
  still injected at build time.
- Read-only GitHub footprint: no branch-protection changes, no org settings,
  no releases, no merges.

## 2. Real multi-agent collaboration (§25.2)

- Three roles as three real sessions with distinct identities, bound to
  tasks through the store; hand-offs are real (planner output feeds the
  builder prompt). Verified in a live run: planner → builder → reviewer, all
  tasks `COMPLETED`, real files created in the working directory, 16 real
  control decisions recorded, including three genuine denials by the engine
  and one **live approval pause → operator approve → resume → execution**.
- Failure/cancel/unknown coverage: unit tests exercise illegal transitions,
  cancelled tasks stopping scheduling, cancelled approvals never approvable,
  timeout expiry, and the run registry refusing double runs; the task
  lifecycle table (`domain.CanTransition`) forbids over-claims like
  UNKNOWN→RUNNING.

## 3. Execution control (§25.3)

The R02-00 probe campaign (40+ real nested Claude Code runs, four versions)
pinned the actual semantics; the engine implements them; the hook transport
was re-verified against a live service (`cmd/qiyan/hook_test.go`) and in the
connector E2E.

| Gate | Result | Evidence |
| --- | --- | --- |
| S01 pre-execution block | PASS | probe matrix (exit-2 and JSON-deny both block, overriding an allow-list); live connector run: out-of-scope read **DENY** with the reason shown to the agent |
| S02 normal actions unaffected | PASS | same runs: in-scope read/write executed; `allow-runs` probe scenario |
| S03 human approval | PASS | live: delete and network actions waited for approval; approved retry executed exactly the authorized action; re-approval required for repeats |
| S04 failure = deny | PASS on ≥2.1.295 (probed 2.1.295/2.1.300: crash, timeout, missing-script all blocked); **documented FAIL-OPEN below 2.1.295** (probed 2.1.292/2.1.68) — the connector refuses to install without the live probe passing unless the operator explicitly accepts the labelled weaker tier; the hook itself exits 2 on every internal failure (tests: `TestHookFailsClosedWithoutService`) |
| S05 identity & delegation | PASS | impersonation tests; delegation rules (transitions, kind-in-scope, read-only receivers, no transitive delegation); roster check cannot be skipped |
| S06 replay & swap | PASS | single-use binding claims (concurrent-claim test: exactly one winner); target/scope/content swaps invalidate grants (tests) |
| S07 cancellation & unknowns | PASS | cancel stops scheduling + cancels pending approvals (tests); store corruption/gaps block the store and everything it gates |
| S08 privacy & credentials | PASS | MCP env **values** never appear in scan output (test), URLs redacted; tokens stored only as hashes; no secrets in logs; no telemetry |
| S09 untrusted input | PASS | all identity from sessions/tokens, never request bodies; malicious task text cannot change the gate (engine tests); UI renders data via textContent under a no-inline-script CSP |
| S10 coverage honesty | PASS | engine stamps only what the transport declared (hook=ENFORCED, API hosts=OBSERVED, otherwise UNKNOWN — tested); coverage rows come from real probes; the console's coverage table shows actual state |
| S11 local service security | PASS | loopback-only + Host/Origin checks, CSRF double-submit for cookie sessions, strict JSON, body limits, scoped credentials (tests) |
| S12 harmless testing | PASS | every live test operated on scratch files/temp dirs; “external send” writes a local outbox line and says so; verification probes are harmless even when enforcement fails |

## 4. Agent Safety Lab (§25.4)

- Supported inputs: Qiyan manifest, Claude Code project config, MCP JSON —
  with explicit refusal for anything else (tested). No scanned code is ever
  executed; no MCP server is ever contacted.
- Every finding carries subject, evidence level (DECLARED /
  STATICALLY_CONNECTED, with ENFORCED reserved for real control results),
  source reference and a boundary note (tested).
- Negative cases and UNKNOWN ranges are tested (bad JSON, missing files,
  empty directories, unsupported kinds).
- Improvement preview → apply → **live verification** → undo, demonstrated in
  a real run: two deny rules applied to a real project's settings, both
  verified by real probes against Claude Code 2.1.300, coverage rows recorded
  as ENFORCED, undo removed exactly those rules.

## 5. Open control layer (§25.5)

- The versioned API contract is documented (`docs/developer-api.md`, and a
  Chinese edition) and demonstrated by `cmd/qiyanhost`, which imports **no**
  internal packages.
- Live F03 run: normal actions allowed; out-of-scope denied; sensitive
  awaited approval and then ran; results reported; and with `--rogue` the host
  performed one action without any control call — the decision trail contains
  **no record for it**, and no coverage row claims it: the system does not
  mark unprotected actions as protected.

## 6. UI and experience (§25.6)

All pages are served from the binary (no external CDN), CSP-hardened, and
bilingual (EN/ZH) with shared reason-code wording, so the two languages state
the same facts. Shipped: home with the three entries + demo; connect wizard
with preview and undo; mission control with fleet cards, live timeline, action
map (drawn only from recorded decisions), inline approvals with plain-language
reasons and the “one action, once” explainer; Safety Lab; developer entry;
demo and training.

- 60-second demo: implemented as five synthetic beats (normal action,
  blocked action, user decision) with a visible live timer; the *scripted*
  path was measured in a headless run; the *human* number (§U02) is NOT
  VALIDATED (see §9).
- Two interactive training scenarios: A (protect project files; widen the
  allowed range and watch the same request pass) and B (decide a sensitive
  request; approve / deny / time out / cancel outcomes all described).
- Fix previews: the lab's apply path always previews the exact file change,
  backs up once, and shows the real verification result.
- Share card: generated locally as an SVG from counts and fixed labels only —
  no paths, prompts, transcripts or names beyond the fixed crew labels; shown
  as a preview and saved only on click; nothing is uploaded.
- Bilingual check: every user-visible string on the console is keyed and
  rendered in both languages; reason codes share one wording table.

## 7. Privacy and product claims (§25.7)

No uploads, no telemetry, no external calls from the product itself; secrets
never rendered or logged (values excluded by construction and by test); all
tests harmless; no mocked result is presented as a real run (the demo and
training are labelled DEMO/TRAINING and are separated at the data layer — the
playground package cannot even import the state store, enforced by test);
`README`, `README.zh-CN`, `docs/scope-and-claims(.zh-CN)` and
`docs/developer-api(.zh-CN)` were rewritten to match the shipped code.

## 8. The two end-to-end chains (§25.8)

1. **Real chain (verified live):** `qiyan serve` → `qiyan connect claude
   --project …` (live probe) → `qiyan mission create` → `qiyan run` with real
   Claude Code sessions → a live approval decided by the operator → results
   and decisions in the console. Recorded with real outputs.
2. **No-external-AI chain (verified live):** console → demo → the synthetic
   crew, block, and decision — with zero decisions recorded in the store
   (checked), proving it plays without touching the real control path.

## 9. NOT VALIDATED (kept honest, per the book)

- **U02 — human 60-second timing.** The scripted path length and the
  in-page timer are implemented and measured; the human comprehension number
  requires a person and is NOT VALIDATED in this delivery.
- **U13 — independent first-time users.** No users unaffiliated with the
  build were available to this session; the walkthrough is NOT VALIDATED.
  The console is usable and bilingual, but developer self-testing is not
  claimed as evidence for first-time ease.

No gate is claimed beyond its evidence. No capability is marked VERIFIED
without a real run. Nothing is BLOCKED at the capability level (the
intermittent github.com transport was worked around with verified-identical
objects; it is an environmental note, not a capability block).

## 9a. R02-SEC-01 addendum — command auto-allow boundary (independent acceptance finding)

An independent acceptance pass found that the auto-allow table admitted
commands with side effects it could not rule out. Fixed in commit `0d8945…`;
the fix is strictly a classification-boundary change.

**Audit of same-class side-effect paths in the auto-allow table** (each one
either demoted or screened; nothing else touched):

| Form | Why it was wrongly auto-allowed | Now |
| --- | --- | --- |
| `go test` | runs the project's own test binaries (arbitrary project code) | approval (sensitive) |
| `go env`, `go env -w`, `go env -u` | `-w`/`-u` persist configuration | approval (sensitive) |
| `go vet -vettool=…` | runs an external analysis binary | demoted by the flag screen |
| `date`, `date -s …` | GNU `-s` and BSD positional forms set the system clock | approval (unrecognized) |
| `hostname NAME` | positional form sets the host name | approval (unrecognized) |
| `git log/diff/show/blame` | textconv filters, `diff.external` and `GIT_EXTERNAL_DIFF` can make them execute configured helper programs | approval (sensitive) |
| kept as safe | proven side-effect-free forms | `go build/vet/list/version`, `git status/rev-parse/ls-files/describe/shortlog`, print-only tools (ls, cat, grep, …) |

**Negative evidence (live, through the real hook channel):** the acceptance
script drove the actual `/v1/hook/decide` endpoint used by Claude Code with a
bound session — 12 side-effecting forms (`go test ./...`, `go test`,
`go env -w GOFLAGS=-mod=mod`, `go env -u GOPATH`, `go env`,
`go vet -vettool=/tmp/tool ./...`, `date -s 12:00`, `hostname newname`,
`git log`, `git diff`, `git show HEAD`, `git blame main.go`) **all returned
REQUIRE_APPROVAL, each with a real pending approval recorded**; 9 positively
verified read-only forms (`go build ./...`, `go vet ./...`, `go list ./...`,
`go version`, `git status`, `git rev-parse HEAD`, `git shortlog -sn`,
`ls -la`, `uname -a`) still ran with no friction. Regression tests
(`internal/control/engine_test.go`) assert both directions and run on the
three-OS CI.

**Docs updated for consistency:** `docs/scope-and-claims.md` /
`.zh-CN.md` now state the proven-side-effect-free rule and name the demoted
forms; the `developer-api` examples no longer use a now-gated command.

**Unchanged true gaps:** U02 (human 60-second timing) and U13 (independent
first-time users) remain **NOT VALIDATED** — the fix changes no UX claim and
no architecture. This addendum affects only the command classification
boundary described above.

## 9b. R02-SEC-01 second review — principle-based convergence

The second independent review reproduced a same-class gap: `git status` can
execute an external hook via `core.fsmonitor` (Git documents this behavior),
so `git -c core.fsmonitor=… status` slipped through the `-c` value-flag skip
into the auto-allow table. Reproduced harmlessly in a temp repository.

Instead of another per-form blacklist entry, the boundary converged to the
principle itself (commit `f7ffba4c`):

- **Auto-allow is exactly one class: print-only programs** (ls, cat, head,
  tail, wc, grep, echo, pwd, uname, whoami, which, where, stat, file, diff,
  true/false/test/type, cd) whose recognized usage reads its own arguments
  and files inside the authorized roots — nothing consults configuration
  that names programs, nothing spawns helpers, nothing writes.
- **Every toolchain and repo tool now requires approval, whatever the
  subcommand**: `git` (repo-local config is writable by the gated agent
  itself — `core.fsmonitor` runs on `git status`; textconv / diff.external /
  GIT_EXTERNAL_DIFF on content commands; `-c`/`--config-env` inject keys
  inline), `go` (`GOFLAGS`/`GOENV` consulted; `GOTOOLCHAIN` can fetch and
  execute a different toolchain; `go test` runs project test binaries;
  `go env -w/-u` persists configuration), `npm`/`node` (config machinery
  Qiyan cannot verify). The subcommand-scanning machinery was deleted with
  it — there is no name-based vouching left to bypass.
- **The product claim was corrected to the real boundary**: the
  "proven side-effect-free" phrasing was replaced in
  `docs/scope-and-claims(.zh-CN)` with the precise guarantee (print-only
  class; everything redirectable by unverifiable configuration waits for the
  user) plus an explicit statement that the boundary is not a sandbox: bare
  names resolve through the session's PATH/environment, which is the user's
  own trust domain.

**Evidence (live, same day):**
- Vector proof (harmless, operator shell, temp repo): a repo configured with
  `core.fsmonitor=<script>` — plain `git status` executed the script (marker
  file created). The vector is real in this environment.
- Gate evidence (real hook channel, bound session): **12/12 forms →
  REQUIRE_APPROVAL, each with a recorded pending approval** — `git status`
  (in the configured repo), `git -c core.fsmonitor=… status`,
  `git --config-env=core.fsmonitor=FSMON status`, `git log`,
  `git rev-parse HEAD`, `go build ./...`, `go test ./...`,
  `go env -w GOFLAGS=-mod=mod`, `npm --version`, `node -v`, `date -s 12:00`,
  `hostname newname`. **6/6 print-only forms → ALLOW** — `ls -la`,
  `cat note.txt`, `head -3 note.txt`, `grep hello note.txt`, `uname -a`,
  `pwd`.
- Regression tests updated to assert the new boundary in both directions
  (classifier table incl. the fsmonitor forms; engine-level approval/allow
  split; containment tests adjusted), running on the three-OS CI.

**U02 (human 60-second timing) and U13 (independent first-time users) remain
NOT VALIDATED** — unchanged by this convergence; no UX claim and no
architecture was altered.

## 9c. R02-SEC-01 final close — the mechanism removed

The final independent review reproduced one more same-class counterexample
(`file --compile -m qiyanmagic` writes a compiled magic file while `file` sat
on the auto-allow list). The owner's closing instruction was to stop
extending name lists altogether, and the code now does exactly that:

- **The name-based shell auto-allow mechanism is deleted, not patched.** The
  classifier, its allow tables, its argument screens and the path-argument
  analyzer are gone; `internal/control` retains only `ExtractProgram`, which
  is presentation-only (it decides nothing). There is no new whitelist and
  no new blacklist to bypass.
- **Every shell command requires approval, in every mode.** The engine's
  exec arm returns REQUIRE_APPROVAL with the exact command shown to the
  user; nothing about shell execution is ever allowed by default.
- **Nothing else moved.** The native semantic tools (Read, Glob, Grep — the
  tool layer, mapped to file-read actions) keep auto-executing inside the
  authorized scope; session bookkeeping keeps passing; every structural gate
  (identity, tool/kind scope, budgets, delegation, cancellation) and every
  hard DENY — including out-of-scope path containment for native reads —
  is untouched. The product claims were reworded to say exactly this
  (`docs/scope-and-claims`, EN/ZH, plus the hook/console approval strings).

**Live evidence (same day, real hook channel, bound session, mode
controlled_auto):**
- **12/12 shell forms → REQUIRE_APPROVAL, each with a recorded approval** —
  `file --compile -m qiyanmagic out.mgc`, `file -m qiyanmagic note.txt`,
  `ls -la`, `cat note.txt`, `grep hello note.txt`, `git status`,
  `git -c core.fsmonitor=… status`, `go test ./...`, `go build ./...`,
  `go env -w GOFLAGS=-mod=mod`, `date -s 12:00`, `hostname newname`.
- **4/4 native tools inside scope → ALLOW** — Read, Glob, Grep, TodoWrite.
- **Preserved hard DENY** — Read of a file outside the authorized roots →
  DENY `path_outside_scope` (and a shell action whose kind is outside the
  mission scope is still DENY, asserted by tests).
- Regression tests rewritten to these three directions (shell-always-approval
  across all modes; native-tools-stay-automatic; scope denials preserved),
  running on the three-OS CI.

**U02 (human 60-second timing) and U13 (independent first-time users) remain
NOT VALIDATED** — unchanged; this close alters no UX claim and no
architecture beyond the removed mechanism itself.

## 10. Re-running the evidence

```sh
go test ./...                          # all suites, any OS
qiyan serve                            # console + API on loopback
qiyan connect claude --project DIR     # live probe + install + undo
qiyan mission create … && qiyan run --mission …   # the real fleet
go run ./cmd/qiyanhost …               # the public-API reference host
```

The R02-00 probe matrix, the connector, fleet, lab and host live-run logs are
preserved alongside the session (paths referenced in the pull request).
