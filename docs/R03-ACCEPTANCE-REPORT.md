# R03 acceptance report — Developer Experience, Adoption & Ecosystem Proof

**Status: R03 = IMPLEMENTATION COMPLETE — PENDING ACCEPTANCE.** This report is
the executor's record. Acceptance belongs to the maintainer (ChatGPT is the
independent technical reviewer; product and release decisions are the owner's).

- Task book: R03 V1.0 (FINAL DESIGN), baseline `main` = `dfdc85a5…` — verified
  byte-identical before work began (tree `d41ae9c7…`).
- Work branch: `r03/dx-adoption` (single R03 branch; one final acceptance PR).
- **Verified code state (final): commit
  `04419a6f732838b1d04925441402c807a2a5b0f3`** (remote). Because the replay
  lander reconstructs commits from diffs, remote-vs-local equality was proven
  by comparing every blob: zero content-different files, zero files only on
  one side except this report itself. An earlier verified state
  (`93b59f8d…`) carries the same evidence trail for the pre-review rounds.
- CI on the verified state: runs `38764495173` (earlier state) and
  `38770922625` (final code state) — exact results in §CI results; this
  report's own commit is re-verified by a report-only CI run.
- Evidence outside the repository (session transcripts, probe logs, screenshots
  beyond the three committed ones, demo transcripts, candidate checksums,
  the adversarial-review findings and the reviewer's retained reproduction
  harness): `D:\Qiyan Ai\r03-preflight\` on the verification machine.

## What was delivered (workstreams)

| Area | Delivered |
| --- | --- |
| R03-01 unified project selection | `GET /v1/fs/list` + `POST /v1/fs/mkdir` (directories only, symlinks not traversed, the state dir refused, 1000-entry cap) + a shared `picker.js` used by Mission/Lab/Connect: connected projects (server truth), recents (browser-local), folder browsing, one-folder test-workspace creation, manual path entry preserved |
| R03-02 simplified connect | auto-detection surface (`GET /v1/detect/claude`), executable path moved into **Advanced**, project picker, plain-language plan/evidence with raw details one disclosure away, **the reachable weaker-tier path** (409 → explicit acknowledged install), measured-tier display, post-connect CTA into mission creation, translate-not-soften vocabulary |
| R03-03 install experience | `Candidate Build` workflow (dispatch-only, artifacts only, no Release, no new permission types), version identity `0.3.0-r03`, README install section with EARLY ACCESS / TEST BUILD labels, SHA-256 + attestation instructions, keep-the-binary-in-place warning, honest platform statement |
| R03-04 guided tours | `tour.js` + Help entry: Mission (7 steps), Lab (6 steps), Developer (5 steps); real-control highlights; steps tied to real state where possible (mission-created, run-started, scan-done…); first-use prompt; back/skip/exit; both languages; never performs approvals |
| R03-05 sample tasks | example goal chips on Mission Control, scope/cost/account notice before starting a real run; harmless-block experience via the Lab's real probe in a created sandbox; empty-state guidance (“create a safe test workspace”) in the picker |
| R03-06 Mission Control convergence | outcome-first summary card (state, duration, files produced from **real recorded artifacts**, decision counts); humanized states (Running / Waiting for you / Stopped by you / Cancelled…); stop-the-fleet button; start errors surfaced; run-record loss stated instead of silence; action map distinguishes Allowed / Stopped by Qiyan / Denied by you / Waiting; coverage line for the folder |
| R03-07 plain-language safety | one wording source per reason on the CLI side, parity guard plan, translated probe evidence, four-question structure in the Lab (what it can do / why care / what Qiyan did / your options), `share.js` hardcoded-English fix |
| R03-08 demo cases | three reproducible cases executed live on this machine (§Demo evidence) |
| R03-09/10 GitHub & README | repo description + topics updated through the normal GitHub API (no protection/permission changes), README restructured to the book's reading order (EN primary, zh parity) with three committed screenshots, stale R01/R02 text corrected (main.go package comment, SECURITY.md scope, CI comment) |
| R03-11 open contract | `docs/developer-api.md` (+zh) rewritten as contract revision 3: minimum action fields per kind, decision envelope, trusted-identity prerequisites, approval/authorization binding (TTLs, single-use, replay), evidence↔outcome correspondence (incl. its absence of read-back and digest non-verification), unprotected-path expression (coverage states, per-decision coverage, rogue statement), protocol version + additive-only policy + strict-decoding note, full runbook, reason-code table, HTTP mapping |
| R03-12…14 human tests | **instruments prepared** (`docs/R03-field-test-kit.md`: U02 60-second protocol, U13 five-step observational protocol, third-party host checklist). **No independent humans have run them yet → NOT VALIDATED** (see §Human tests) |
| R03-15 competitive evidence | §Competitive differentiation — public facts re-verified live 2026-10-15, including where competitors are stronger |
| R03-16 candidate distribution | candidates built + checksummed + identity-verified (§Candidates), workflow ships in this PR |
| R03-17 adoption data | §Adoption evidence — only real, verifiable numbers |
| R03-18/19/20 | §Acceptance gates / security / verdict |

## Defects found and fixed during R03 (all on this branch)

Every one of these was found by running the real product, not by reading code.

1. **Baseline flake (crash):** `TestRunRegistryRejectsDoubleRun` fed a storeless
   orchestrator to a background goroutine → nil-pointer panic killed the test
   process nondeterministically (`-count=40` reproduced 100%). Fixed with a
   nil-store guard returning an error + a deterministic blocking-runner test.
   (`b5ede7b`)
2. **Cancel reported as failure.** A user-cancelled run ended `failed`
   (“context canceled”) and the mission went FAILED. Now: state `cancelled`
   with an honest summary, mission CANCELLED, task CANCELLED — verified live.
3. **Artifacts never reached the wire.** `Task.Artifacts` was recorded but the
   payload had no field, so “Files produced” could never render. Added
   `artifacts` to the task payload (additive) and populated the run record —
   verified live (REPORT.md).
4. **Lab verification could report a false “verified.”** The probe ignored the
   command error and the transcript error result: a session that never ran
   (e.g. nested-session refusal) left the scratch file intact → “verified”.
   Now three gates (run error, empty result, `"is_error":true`), and — from
   the adversarial review, finding 9 — a **controlled comparison**: the same
   command is attempted once without the rule (it must run) and once with it
   (it must be refused with an explicit marker). A single headless session can
   refuse a command for reasons that have nothing to do with the rule, so only
   the comparison may set `verified:true` (live-verified after the fix:
   “without the rule `rm scratch.txt` ran; with the rule … was refused”).
5. **Transport cut long probes at 10 s.** The serve `WriteTimeout` is 10 s; the
   enforcement probe legitimately runs 15–90 s of real sessions. The probe
   endpoint now extends its own write deadline via `http.ResponseController`.
   This was load-bearing in this very build: a real enforced-tier connect
   probe took **14.1 s** — without the fix the response would have been cut.
6. **Coverage rows were global, not per project** — connecting a second
   project overwrote the first project's protection evidence, and the
   uninstall row was missing a separator. Rows now carry the project scope.
7. **CSP killed inline styles.** The console CSP is `style-src 'self'`, so
   every markup `style="display:none"` was inactive in real browsers — the
   mission run area and (new) stop button were visible from first load. All
   visibility/layout moved to classes (`.hidden`, `.flex1`, …); a
   first-load screenshot of the broken state is kept as evidence. (`d333c67`)
8. **Deep links died at sign-in.** `ensureSession` stripped the whole query
   string after `?k=` bootstrap, destroying `?mission=`/`?project=`/`?tour=`.
   Now only the one-time key is removed. (`2bf9f78`)
9. **Picker read wrong wire names** (`ProjectDir`/`path` vs actual
   `projectDir`/`path`) → connected rows and browse entries rendered empty.
   (`1697941`, `2bf9f78`)
10. **Drive roots.** `path.Clean` reduced `D:/` to `D:` (a per-drive working
    directory on Windows). Drive roots keep their slash end-to-end. (`1697941`)
11. **CI caught a 8.3-short-name test bug** on the GitHub Windows runner
    (`RUNNER~1` vs resolved long path) — assertion now compares the physically
    resolved form. (`cf7c8f3`)

A further **27 confirmed findings** from the adversarial review round were all
closed in commit `256cf9c` (remote `04419a6f`) — none of the 27 was refuted.
See §Adversarial verification round for the itemized list.

## Demo evidence (three cases, executed live on the verification machine)

Environment for A/B: control service on loopback; Claude Code **2.1.300**
scratch install (the machine's PATH Claude Code is 2.1.68 → honestly
“blocking-only”; see §Acceptance gates item 3).

**Case A — real three-agent mission (Enforced tier).** Mission
`mi_aaaadikaf3fgq2sadua4s43hq6nzs`, goal “read the notes and write one summary
report”. Real sessions: planner planned; builder attempted `ls` → approval
raised (`sensitive_command`); the operator approved once; the retried call
executed exactly once (`user_approved`), the grant was consumed; builder wrote
`REPORT.md` (872 B, committed to evidence); reviewer verified; mission
COMPLETED. **All 16 recorded decisions carry coverage ENFORCED** — including
two hard DENYs of the reviewer's shell attempts (`write_by_readonly`).
Transcript: `demo-run-transcript.txt`; trail: `decisions-summary.txt`; UI:
shot `demo-mission-completed.png`.

**Case B — deny, stop, and the weak-tier honesty paths.** Companion-mode
mission `mi_aaaadikahxowxtt2bpovr5r7xfa4s`: the builder's `README.md` write
paused; the operator **denied**; the builder continued without it and said so;
the run completed. A third mission `mi_aaaadikah5urnqrw2y5ixsdflqj6k` was
**cancelled mid-review** → run `cancelled`, mission CANCELLED, reviewer task
CANCELLED — screenshots `demo2-approval-pending.png`, `demo3-cancelled.png`.
Connect wizard with the machine's 2.1.68: the live probe ran, did not reach
enforced, and the console offered the explicit, acknowledged weaker install
instead of a dead end. Connect with 2.1.300: probe 3/3 → **tier enforced**,
install recorded, four project-scoped ENFORCED coverage rows, `connect status`
shows `tier=enforced`, undo path unchanged.

**Case C — third-party host (public API only).** `cmd/qiyanhost` (zero private
imports) against the live service: out-of-scope read **DENY** (not executed);
in-scope read **ALLOW**; append **ALLOW**; scratch delete **REQUIRE_APPROVAL**
→ approved once → executed; `mail.send` **DENY** (`tool_not_in_scope` — the
operator had not granted the `network.access` kind; a correct scope refusal).
Rogue run: one write **without asking** — and it appears in **no** decision
record (14 decisions total across the two runs; the rogue write is none of
them). Transcripts `host-run-transcript.txt`, `host-rogue-transcript.txt`.

**UI verification battery.** A CDP-driven interactive smoke
(`uismoke.mjs`, outside the repo) drove the real console: tour highlight and
exit; picker connected-row selection; folder browsing; mission creation;
lab scan/plain-language column/preview; connect preview + probe + honest weak
choice; zh rendering — **16/16 checks passed**.

## Acceptance gates

### R03-18 — P0 product-experience acceptance

| # | Gate | Status | Evidence |
| --- | --- | --- | --- |
| 1 | Connected projects reusable across main entries | PASS | picker live: connected row with measured tier; smoke check |
| 2 | No repeated path typing across pages | PASS | one picker on Mission/Lab/Connect; identical field contract |
| 3 | Claude Code auto-detected or accurately guided | PASS | `detect/claude` live: found 2.1.68 → “Partial — measured”, floor 2.1.295; missing case renders install guidance |
| 4 | Step-by-step tutorials in all three entries | PASS | tour.js, 7/6/5 steps; smoke verified highlight + step text + exit |
| 5 | No-account try remains | PASS | demo page unchanged, labelled DEMO |
| 6 | Real three-agent mission completes | PASS | Case A (16 ENFORCED decisions, real artifact) |
| 7 | Approval/deny/security behavior not regressed | PASS | engine/lab/hook/store suites green; live deny + approval + one-shot consumption |
| 8 | Safety Lab evidence and boundaries stay true | PASS | live scan→preview (no write, verified byte-identical settings)→apply→verify (2 rules verified, ENFORCED rows); false-positive gates added |
| 9 | Third-party Control API compatible | PASS | additive-only; case C ran against the live API; contract revision documented |
| 10 | EN/ZH semantic parity | PASS* | all new UI strings keyed in both tables (273/273); lab/connect server prose shown as raw evidence with a labeled frame; *full translation of server-authored evidence strings remains a documented limitation |
| 11 | A candidate path without building Go | **PARTIAL** | candidates exist and run (built + checksummed + identity-verified); the dispatch workflow ships in this PR — GitHub only dispatches workflows present on the default branch, so the artifact channel activates on merge. Exact wording in README; recorded as a limitation, not silently claimed |
| 12 | Real first-use human tests recorded | **NOT VALIDATED** | no independent humans ran yet; kit prepared |

### R03-19 — P0 security & engineering acceptance

Preserved, with live or suite evidence: identity/permission boundaries (operator/hook/agent tokens, UI session+CSRF; picker endpoints require auth and refuse the state dir); mandatory shell approval (no name allowlist — Case A/C refusals); pre-execution deny (Case A/B/C); high-impact authorization (delete approved once, exactly once); approval replay protection (single-use claim; test suite); cross-agent delegation constraints (engine suite); local API auth & CSRF (suite + smoke); file/project scope control (Case C scope DENY); no secret leakage (scan redaction tests; no tokens in artifacts); no private task upload; no telemetry by default; three-platform Go CI + security job (§CI); main branch protection untouched (verified read-only: `enforce_admins`, 4 required checks, no force-push); no release created (0 releases before/after; the drill and candidate workflows create none in this PR).

The picker/wizard additions were explicitly checked against the rule “simpler UI must not grant more authority”: selection only fills a path field; every write path (connect apply, lab apply, mkdir, mission create) still validates and confirms as before.

### CI

| Run | Head | Result (as recorded by GitHub) |
| --- | --- | --- |
| `38750826815` | baseline main `dfdc85a5` | success (4/4) |
| `38755487262` | `7d5a4083` (early) | ubuntu/windows/security success; macos cancelled while queued |
| `38760550594` | `c4a05c10` | **found the RUNNER~1 assertion bug** (fixed in `cf7c8f3`); ubuntu/security success; windows failed on the bug; macos cancelled while queued |
| `38764495173` | `93b59f8d` | attempt 1: ubuntu/windows/security success, macos cancelled while queued (conclusion: failure) — **attempt 2 (rerun of the failed job): success**; run now concludes success (4/4 on the same head) |
| `38770922625` | **`04419a6f` — the final verified code state** | **success — all four jobs green: ubuntu-latest, windows-latest, macos-latest, security checks** |

macOS jobs on this runner pool were repeatedly cancelled **while still
queued** during the first dispatch wave (the API exposes no cancellation
reason; nothing on the branch superseded the runs by the recorded timestamps,
so no cause is asserted here). The reruns and the final run completed
normally, and the final verified code state carries a complete 4/4 record.
The report's own commit is re-verified by a report-only CI run on its head.

## Adversarial verification round

After the implementation rounds, an independent multi-agent adversarial
review ran over the full R03 diff (5 dimensions — picker/fs security, UI
safety, honesty-bearing logic, wire-contract compatibility, workflows/public
claims — 37 reviewer agents; every claimed finding then faced a dedicated
verifier prompted to refute it). **Result: 27 confirmed findings, 0
refuted; all 27 are fixed in `256cf9c` (remote `04419a6f`).** The reviewer's
retained reproduction harness (an out-of-repo copy of the tree) was re-run
against the fixed code and now shows the refusals it previously bypassed.

| # | Severity | Finding (condensed) | Fix |
| --- | --- | --- | --- |
| 1 | med | State-dir refusal compared the configured spelling only; alias spellings (8.3 short names / junctions / symlinked ancestors) could list and mkdir inside the state dir (repro p. retained) | physically-resolved forms + os.SameFile ancestor walk (Go's EvalSymlinks does **not** resolve NTFS junctions — string forms alone cannot close this); alias regression test |
| 2 | low | Case-variant state-dir spelling passed on case-insensitive filesystems off Windows | comparison folds case on Windows **and darwin** (Linux stays case-sensitive) |
| 3 | low | UNC spelling `\\server\share` collapsed to a root-relative local path | fsNormalize/fsAbsolute preserve UNC; unit test |
| 4 | low | mkdir accepted trailing-dot/space names (stored under a different real name) and device names (`CON`, `NUL`…) | name validation (dot/space suffix, reserved stems) + test |
| 5 | med | Outcome card looked coverage up under a dead fallback root → a reloaded mission falsely claimed “never measured” | root comes from the stored mission scope |
| 6 | med | A terminal mission kept re-rendering every 2 s, destroying the share-card preview and polling forever | summary rendered once; poller stops on terminal state |
| 7/19 | low | Picker “Up” shortened a drive root to `D:`, and the selected value was the local spelling, not the server's canonical path | drive-root slash restored; the server's canonical `path` is adopted before selection |
| 8 | low | Help button dead on pages without tour.js; a tour could not start before sign-in | tour.js loaded on all pages; last-instant page detection in `Tour.run` |
| 9 | **high** | Lab “verified:true” was not attributable: headless default mode refuses the probe command with or without the rule (control run reproduced: `BLOCKED`, no rule present) | reworked to a **controlled comparison** (rule absent → must run; rule present → must refuse with an explicit marker); live-verified after the fix |
| 10 | **high** | A run whose sessions all ended `is_error:true` still reported “completed” | `is_error` fails the role; regression test |
| 11 | med | Cancel during an approval wait left the approval PENDING and approvable for a dead run | `cancelPending` on every cancel path; regression test |
| 12 | low | Version-implied tier rendered with the “measured on this machine” label | neutral “(unprobed)” labels |
| 13 | med | `POST /v1/missions/{id}/run` discarded body-decode errors (doc promises strict decoding) | strict optional-body decode + test (note: Go matches JSON keys case-insensitively — a case-variant is honored, not dropped; different-letter typos are rejected) |
| 14 | med | Doc claimed a distinct replay refusal; the engine actually re-gates a spent grant | contract corrected; `authorization_replayed` moved to the *never emitted* list (now four codes) |
| 15 | med | Doc labeled `GET /v1/coverage` “no auth” | corrected (operator/session; only health is unauthenticated) |
| 16 | low | Documented 403 class list misstated loopback/CSRF refusals | corrected with the two deliberate exceptions |
| 17 | low | Doc's `"raw": {}` example is rejected by strict decoding (the field is a string) | example corrected, 200-char truncation stated |
| 18 | low | Runbook claimed READY→RUNNING for CLI-created tasks (actually CREATED→RUNNING) | corrected (EN+ZH) |
| 20 | **high** | `candidate-build.yml` substituted the dispatch input into shell text (command execution inside a job holding id-token/attestations); the drill's tag input had the same class | inputs pass through `env:` + whitelist validation in both workflows; artifact set gains `CANDIDATE-LABEL.txt` |
| 21 | **high** | This report initially claimed an all-green 3-OS run that the GitHub record contradicted (macOS was cancelled; run concluded failure) | corrected to the recorded facts; the final verified state now carries a genuine 4/4 run (`38770922625`), and the earlier head was completed green by rerun |
| 22 | low | Drill comment contradicted reality after the candidate workflow landed | comment updated |
| 23 | low | “labelled EARLY ACCESS” was claimed but no step applied a label | the label file now travels with the artifacts (and the doc says exactly that) |
| 24 | low | README early-access path was not actionable | where candidates come from + the artifact's real file name in the commands |
| 25 | low | README caption promised a weaker-tier screenshot that showed only the strict tier | screenshot replaced with the real dual-tier view (Strict 2.1.300 + Partial 2.1.68) |
| 26 | low | README said approvals bind “exactly that action” while content binding needs `contentDigest` | wording aligned with the contract (EN+ZH) |
| 27 | low | zh claims doc dropped “CGO-free” from the EN paragraph | parity restored |

Findings log with evidence and verdicts:
`r03-preflight/review-findings.md` (outside the repo); the retained
reproduction harness at `%TEMP%\qiyan-r03-repro` now passes against the fixed
tree.

## R03-20 — viral qualification (evidence-supported judgment)

**A. Is it easy to spread?** A short demo exists and is honest (synthetic
labelled); the three product cases are reproducible. **READY WITH LIMITATIONS
(user evidence pending).** The one-liner claim (“your AI team, your rules”)
is supported by a real product — but no independent user has been timed or
observed yet (U02/U13 NOT VALIDATED), so “strangers understand it in 60
seconds” is **NOT VALIDATED**.

**B. Independent value without enterprise SaaS?** Strong capability evidence
(pre-execution gate, measured tiers, one-shot approvals, coverage honesty) in
a fully local, account-free package. The competitive review also shows this
category has real business-model fragility (Vibe Kanban's shutdown post is
about free-user economics) — supporting the no-SaaS thesis while warning
against assuming it. **READY WITH DOCUMENTED LIMITATIONS.**

**C. Ecosystem attraction?** The contract revision, the fully independent
reference host, and the rogue-honesty pattern are genuinely reusable. But
**no third-party developer has attempted the integration yet (R03-14 NOT
VALIDATED)** — the book forbids substituting the maintainers' own host for
that evidence.

**Overall verdict (executor's recommendation):**
**READY WITH DOCUMENTED LIMITATIONS** — every product-capability gate passes
with live evidence; the adoption-side gates (U02, U13, developer recruitment,
third-party integration) are **NOT VALIDATED** because they require people the
executor cannot legitimately supply. The book's rule stands: no fake passes.

## Competitive differentiation (R03-15) — verified public facts, 2026-10-15

Method: three competitor classes researched and then adversarially
fact-checked against primary sources (repos, PyPI, official docs) on
2026-10-15; corrections applied; unverifiable claims marked. Full working
material: `r03-preflight/competitive-digest.md` (outside the repo).

Condensed, honestly:

| Axis | Qiyan today | Where the class is stronger (verified) |
| --- | --- | --- |
| First-use effort | no account, no toolchain (candidate) or `go build`; guided tours; measured tier in 3 real probes | Snyk Agent Scan/Cisco scanner: pip/uvx one-liners with wide auto-discovery (14 clients / 8 clients), deep static analysis; Vibe Kanban `npx` |
| Agent teamwork | real 3-role fleet on the user's machine with per-action gating | CrewAI/OpenHands/LangGraph: broader models, hosted governance (SSO/RBAC), richer HITL workflows |
| Real pre-execution control | per-tool-call gate with fail-closed floor measurement and honest tiers | OS-level sandboxes are deeper isolation: Anthropic sandbox-runtime, Codex sandbox, Microsandbox microVMs (kernel-level); Devin CLI fails closed if its sandbox is unavailable |
| Human authorization | one-action-one-execution approvals bound to digests, verified live | OpenHands ConfirmRisky / analyzer-gated confirms at comparable depth; Pipelock HITL + signed receipts; Claude Code's own permission modes are the baseline primitive Qiyan builds on |
| Evidence traceability | append-only decision trail with ENFORCED/OBSERVED coverage, honest gaps | Pipelock: signed, hash-linked, offline-verifiable receipts (stronger); LangSmith trace platform |
| Third-party integration | open local /v1 contract + independent reference host + rogue-honesty | MCP ecosystem gateways (Lasso), Cisco SDK/REST, LangGraph as an embeddable framework |
| Local-first & privacy | everything local; no account/telemetry | mcp-audit fully offline; both Anthropic/OpenAI sandboxes local OS primitives |
| Uncovered-path statement | coverage states, per-decision coverage, rogue demo, scope-and-claims | mcp-audit's GAPS.md and Pipelock's scorecard are in the same honesty class; OpenHands' “What this does not do” is exemplary |

**Not claimed:** Qiyan is not novel in category — the general concept
(“local mission control with a PreToolUse approval gate”) exists in small
projects, and the gate primitive itself is documented Claude Code
functionality. Qiyan's differentiators are the combination and its honesty
discipline, not first-of-kind. No competitor's weakness is asserted beyond
what the cited public sources state.

## Candidates (R03-16)

Built at the **final verified content** (the tree of remote
`04419a6f…`), 5 targets, `CGO_ENABLED=0 -trimpath`, each set accompanied by
`CANDIDATE-LABEL.txt` (EARLY ACCESS / TEST BUILD / NOT PRODUCTION-CERTIFIED):

| File | SHA-256 |
| --- | --- |
| `qiyan_0.3.0-r03-candidate_linux_amd64` | `23af73212d8a4361edb5a94bd3db9bb8f00794d3ac63d9635cb9a9774e276803` |
| `qiyan_0.3.0-r03-candidate_linux_arm64` | `84e39a86f4777ecf574ba544db03c0a3f78537bc531d72cdf7f3f8780ac8c7e6` |
| `qiyan_0.3.0-r03-candidate_darwin_amd64` | `0fdde8cbf9f2cc5fe0139dae733931ae8e061315c6e4820f353e1c7b0c886e1c` |
| `qiyan_0.3.0-r03-candidate_darwin_arm64` | `1c8e72922db7e4e5acc68d29784f7e7e0783ee799ad9e3f6cbb57023f50a37a6` |
| `qiyan_0.3.0-r03-candidate_windows_amd64.exe` | `6dbe6a2611f34c5f8fadeb0f946c9f331ac92e311580701fb230f0ba001e2b69` |

Identity verified by running the Windows candidate: `qiyan
0.3.0-r03-candidate (commit 04419a6f…, built …)`; it was then **run
standalone** (serve booted, `/v1/health` ok, console served). An earlier
candidate set was rebuilt and replaced after the adversarial-review fixes —
the shipped candidates contain the fixed code, not the reviewed-away
defects. (The local artifact set carries the same `CANDIDATE-LABEL.txt` content the
workflow writes; the workflow additionally signs a provenance attestation.)
These are TEST BUILDS (labels, unsigned, no support promise; full honesty in
`docs/scope-and-claims.md`). GitHub-side artifact publication requires the
workflow to exist on the default branch (GitHub's dispatch rule) — **that
channel activates when this PR merges; until then the files above exist only
on the verification machine.** This is a platform constraint, stated rather
than papered over.

## Adoption evidence (R03-17)

Real and verifiable only; nothing fabricated:

| Metric | Value (2026-10-15T15:53Z, GitHub API) |
| --- | --- |
| Stars / forks / watchers / open issues | 0 / 0 / 0 / 0 |
| Contributors | 1 (the maintainer account) |
| Releases | 0 (none created in R03 — checked before and after) |
| Independent testers (U02/U13), integrators | 0 yet → NOT VALIDATED |
| Downloads | no published distribution channel yet (see §Candidates) |

## Human tests (R03-12/13/14) — NOT VALIDATED

No independent humans have run the protocols. The instruments are ready
(`docs/R03-field-test-kit.md`) and the demo page serves the U02 vehicle. Per
the task book, fabricated or self-run results are not an option; these three
items remain **NOT VALIDATED** until the owner organizes real testers.

## Honest gaps and limitations carried forward

1. U02, U13, R03-13 (5–10 developers), R03-14 (third-party integration):
   **NOT VALIDATED** (need people).
2. Candidate artifact channel: live only after merge (GitHub dispatch rule;
   §Candidates). Local candidates are verified; the workflow itself could not
   be dispatched from a branch.
3. macOS CI jobs were repeatedly cancelled **while queued** during the first
   dispatch wave (no cause is exposed by the API; none is asserted); the
   reruns and the final run completed normally, and the final verified code
   state has a full 4/4 green record (run `38770922625`).
4. Lab/connect server-authored prose (finding details, plan change lines)
   renders as English evidence in zh mode inside a labeled frame; UI chrome,
   reasons, statuses and doc pairs are fully bilingual (273/273 keys). Full
   translation of evidence prose is a documented remaining item.
5. The engine emits 22 of 25 declared reason codes; the declared-only three
   are labeled as such in the contract doc.
6. `POST /v1/events` still has no read-back and does not verify the digest —
   documented in the contract and listed here as a real limitation.
7. The mission deep-link fix and several UX fixes changed console assets
   embedded in the binary — verified by screenshots and the smoke battery,
   not by an automated UI test suite in CI (none exists; the smoke rig is
   external by design).

## What was NOT done (boundary)

No merge, no release, no GitHub Release/draft, no branch-protection or
permission changes, no new repositories, no rebuild of R01/R02, no weakening
of approvals/identity/fail-closed rules, no telemetry, no upper-bound claims.
Repo metadata (description/topics) was updated through the normal API — the
one R03-09 act outside code, recorded here explicitly.

**R03 = IMPLEMENTATION COMPLETE — PENDING ACCEPTANCE.**
