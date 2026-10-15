# R02 — Acceptance report

**Status: IMPLEMENTATION COMPLETE — VERIFIED (with two explicitly NOT
VALIDATED experience gates; see §9). Final acceptance belongs to the user.**

**Verified code state:** `b4876cf0babed4d9e596b1e78f15a14259e14069` on
`r02/mission-control` (tree `6ad5ad2756…`, byte-identical to the locally
tested tree — verified by tree-hash comparison after landing). This report is
added in a report-only commit on top of that state; no code changed.

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
