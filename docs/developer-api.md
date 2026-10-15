# Qiyan Control API — developer guide

**Protocol version: v1 (contract revision 3, 2026-10).** The API is local,
HTTP/JSON, and loopback-only. It is the same interface the Qiyan console, the
CLI, the Claude Code hook, and the bundled reference host use — there is no
private path. This document is the contract; where code and document disagree,
the code is wrong.

Compatibility policy for v1: **additive only.** Field names, outcome strings,
error classes and URL paths are stable; new fields and new reason codes may
appear (tolerate them), existing ones never change meaning. Requests are
decoded **strictly** — any unknown JSON field is rejected with `400
INVALID_REQUEST`, so a typo can never be silently dropped. Bodies are capped
at 1 MiB. There is no protocol-negotiation handshake; the version is the `/v1`
path segment, and this document names the revision.

## Start here: the third-party runbook

The full prerequisite chain before `POST /v1/authorize` can succeed. Every
step is operator-level (a human with the operator token from the state dir):

```sh
qiyan serve                              # 1. control service (prints addr + token paths)
qiyan agents add --name MyHost --role builder
                                         # 2. agent identity; prints the agent token ONCE
qiyan mission create --goal "host demo" --root D:/scratch \
    --tool note.read --tool report.append --tool tmp.cleanup
                                         # 3. mission + the EXACT tool names your host will use
qiyan task create --mission mi_… --role builder --agent ag_… --title "host work"
                                         # 4. a task whose agent IS your agent identity
qiyan session bind --task ta_… --agent ag_… --session myhost-1
                                         # 5. binding starts the task (READY → RUNNING)

export AGENT_TOKEN=…                     # the token from step 2
go run ./cmd/qiyanhost --addr 127.0.0.1:8760 --token $AGENT_TOKEN \
    --mission mi_… --task ta_… --workdir D:/scratch
                                         # 6. ask → obey → report (the reference host)
```

Credentials live as 0600 files in the state dir (file permissions are the
local-user trust anchor): `operator.token` (local-user operations), `hook.token`
(only `POST /v1/hook/decide`). Agent tokens are shown exactly once at
registration and stored only as SHA-256. The **UI session cookie** (console
only) additionally requires the `X-Qiyan-CSRF` header on non-GET requests.

### Trusted identity requirements (why step 4 above matters)

`POST /v1/authorize` derives **all** identity server-side; nothing in your
request body is trusted:

- the agent comes from the bearer token;
- the task comes from the stored task you name (its state, its assigned agent,
  its mission's scope);
- **the body's `missionId` is advisory**: the server uses the mission the
  stored task belongs to and ignores a conflicting value;
- the requester must be the task's own assigned agent and be on the mission
  roster; the task must be in a live state. Otherwise: `DENY` with reason
  `identity_unknown`, or an `UNKNOWN_IDENTITY` error for an unknown token.
- tool and kind must be inside the mission scope (the `--tool` grants from
  step 3); everything else is `tool_not_in_scope` / `unrecognized_command`.

## The one question that matters: "may I do this?"

```
POST /v1/authorize          Authorization: Bearer <agent token>
{
  "missionId": "mi_…",           // advisory; the stored task decides
  "taskId":    "ta_…",
  "action": {
    "kind": "file.modify",       // required; one of the kinds below
    "tool": "report.append",     // required, non-empty; must be in the mission's tool list
    "path": "D:/work/report.md", // file.* kinds: absolute, forward slashes, inside scope roots
    "command": "go build ./…",   // process.exec
    "cwd": "D:/work",            // process.exec: where it will run
    "url": "https://…",          // network.access
    "credentialRef": "DEPLOY_TOKEN", // credential.use: the NAME, never a value
    "contentDigest": "…",        // optional: SHA-256 of the exact content a write would produce
    "delegationTo": "ag_…",      // agent.delegation only (see below)
    "delegationTask": "ta_…",
    "delegationKind": "file.modify",
    "raw": { }                   // optional, opaque; for your own audit display
  }
}
```

**Minimum fields per kind** — everything else is optional:

| kind | required | notes |
| --- | --- | --- |
| `file.read` / `file.create` / `file.modify` / `file.delete` | `kind`, `tool`, `path` | path must be absolute and normalized (`forward/slashes`, no `..`); containment is checked against the mission scope |
| `process.exec` | `kind`, `tool`, `command` | `cwd` strongly recommended (the engine reasons about it); **every** shell-class action requires human approval by design — there is no name allowlist |
| `network.access` | `kind`, `tool`, `url` | approval-gated |
| `credential.use` | `kind`, `tool`, `credentialRef` | names only, never values; approval-gated |
| `external.state_change` | `kind`, `tool` | approval-gated |
| `agent.delegation` | `kind`, `tool`, `delegationTo`, `delegationTask`, `delegationKind` | restricted to the planner→builder, builder→reviewer, reviewer→builder triangle; no transitive delegation; a read-only receiver may not be handed a mutating kind |
| `session.local` | `kind`, `tool` | bookkeeping tools only; anything else needs approval |

`contentDigest` semantics, precisely: **without it, an approval covers the
target but not the content** — the retried call may write different bytes to
the same path. Provide it whenever your host can compute it and you want the
approval to bind content too.

### The decision (response envelope)

HTTP `200` with the decision — **including for `DENY`/`UNKNOWN`**: the
transport succeeded; the answer is the body:

```json
{
  "outcome": "REQUIRE_APPROVAL",      // ALLOW | DENY | REQUIRE_APPROVAL | UNKNOWN
  "reasons": [ { "code": "sensitive_command", "params": { "program": "go" } } ],
  "actionDigest": "…",                 // identity of the exact action — keep it
  "approvalId": "ap_…",                // present when REQUIRE_APPROVAL
  "mode": "controlled_auto",
  "coverage": "OBSERVED"               // ENFORCED only on the hook path; see below
}
```

| Outcome | What your host must do |
| --- | --- |
| `ALLOW` | Perform exactly that action. |
| `DENY` | Do not perform it. `reasons` explain why — render them in your UI's language from the codes; never parse prose. |
| `REQUIRE_APPROVAL` | Do **not** perform it yet. Wait for the human: poll `GET /v1/approvals/{id}` (the bound agent may read its own), then re-submit the *identical* action. Re-submitting while still pending is safe and returns the same approval (deduplication by task + agent + action digest) — that is the one retry loop that is correct. Once decided, re-submit exactly once. |
| `UNKNOWN` | Treat as denial on any enforced path. It means Qiyan could not conclude "allowed" — it is never a transient "try again later". |

An **approved retry still passes every structural check** (task state, scope,
budgets, path containment). The approval is not a bypass; it resolves exactly
one gate.

## Approvals and authorization binding

```
GET  /v1/approvals?state=pending        # operator
GET  /v1/approvals/{id}                 # operator, or the bound agent (own only)
POST /v1/approvals/{id}/decision        # operator / local user ONLY — agents get 401
{ "approve": true }
```

Binding conditions, precisely:

- one approval → **at most one execution**, of the **exact action**, bound to
  mission + task + agent + action digest + scope digest;
- **approval TTL: 15 minutes**; you may read `expiresAt` on the approval;
- approving mints a single-use **authorization** (TTL 10 minutes) that is
  **claimed atomically on the retried call** — a concurrent double-retry
  cannot execute twice; replay after the claim is refused
  (`AUTHORIZATION_REPLAYED`/`authorization_replayed`);
- a **denied action stays denied** for that task (reason `user_denied`); a
  **cancelled** approval can never be approved later (`approval_cancelled`);
- an **expired** approval must be re-requested (a fresh gate evaluation);
- a scope change invalidates any outstanding grant;
- **silence is never consent**, and there is no "allow everything" endpoint.

## Reporting what actually happened (evidence ↔ outcome)

```
POST /v1/events             Authorization: Bearer <agent token>
{ "taskId": "ta_…", "actionDigest": "…", "result": "executed|failed|blocked|cancelled", "note": "…" }
```

`actionDigest` comes from the authorize response. The server enforces: the
enum, the agent owns the task, and `note` is truncated to 400 characters.

Honest limits, stated here because a contract is where they belong:

- **there is no read-back endpoint** for reported results yet — you cannot
  `GET` what you reported; keep your own copy;
- the server **does not verify** that `actionDigest` matches a recorded
  decision — a stale or mismatched digest is accepted. Treat the pairing of
  `actionDigest` with your decision record as your own responsibility;
- decisions themselves are fully readable: `GET /v1/decisions?task=…` returns,
  per decision, the outcome, the structured reasons, the action payload, mode
  and coverage — that is the audited trail.

## Expressing what is NOT protected

The contract distinguishes observed from enforced, everywhere:

- **per-decision `coverage`**: decisions taken through the Claude Code hook
  are stamped `ENFORCED` (the tool call provably stopped/ran at the gate);
  decisions taken through `POST /v1/authorize` are `OBSERVED` — **compliance
  is your host's code**, Qiyan observes the question, it cannot enforce your
  obedience;
- **`GET /v1/coverage`** (operator/session): rows produced by real
  verification runs on this machine, each with a state —
  `ENFORCED` (a real check proved the control held), `OBSERVED` (a weaker,
  observed-only fact — e.g. a `blocking-only` probe tier), `UNCOVERED`
  (measured as not protected — e.g. an uninstalled integration), `UNKNOWN`
  (not measured);
- there is deliberately **no "protected: true" shortcut**: never derive
  protection from installation records, version numbers, or documentation;
- if your host can act without calling this API (the `--rogue` demonstration),
  that path is **outside Qiyan's enforcement** and the product claims nothing
  about it. Say the same at your integration point.

## Reason codes (stable identifiers)

Reasons are `{ code, params }` — machine-stable, render at the edge. The
vocabulary today (25 declared; the three marked *never emitted* describe
states the engine does not currently produce — treat any appearance as a code
to render generically):

| Code | Meaning |
| --- | --- |
| `path_outside_scope` | target path is outside the mission's roots |
| `tool_not_in_scope` / `unrecognized_command` | tool/kind not in the mission's allowlist, or not interpretable |
| `write_by_readonly` | a read-only role attempted a mutation |
| `delete_operation` | deletions always require approval |
| `sensitive_command` | every shell-class command requires approval (`{program}`) |
| `credential_reference` / `external_network` / `external_state_change` | approval-gated classes |
| `mode_companion` | accompany mode: changes wait for confirmation |
| `budget_exhausted` (`{what}`: steps/runtime) | mission budget reached |
| `task_cancelled` | the task was cancelled |
| `identity_unknown` (`{what}`) | identity/state could not be confirmed |
| `delegation_escalation` (`{what}`) | delegation would widen authority |
| `user_approved` (`{approval}`) / `user_denied` | the human's decision on the retried action |
| `approval_expired` / `approval_cancelled` / `action_changed` | grant no longer valid for this attempt |
| `authorization_replayed` | the single-use grant was already spent |
| `control_unavailable` (`{stage}`) | audit-or-deny: a decision that cannot be recorded is a denial |
| `no_external_effect` | bookkeeping-only session action |
| `mode_controlled_auto`, `mode_bounded_autonomy`, `path_not_covered` *never emitted* | declared vocabulary, not produced today |

## The gate itself (for runtime integrations)

```
POST /v1/hook/decide         Authorization: Bearer <hook token>
{ "sessionId": "…", "toolName": "Bash", "toolInput": {…}, "cwd": "…" }
```

Used by the installed Claude Code hook. Identity comes from the session
binding, never from the request body; an unbound session is refused. Exit-code
semantics on the runtime side (verified, not assumed): `ALLOW` → exit 0;
everything else → exit 2 with the explanation on stderr; any failure to obtain
a decision → exit 2 (fail closed).

## Missions, tasks, sessions

```
POST /v1/missions                       # operator: goal + mode + explicit scope
                                        #   scope = roots, allowedKinds, allowedTools, budgets
GET  /v1/missions / GET /v1/missions/{id}
POST /v1/missions/{id}/tasks            # operator
POST /v1/sessions                       # operator: bind a runtime session to a task (starts it)
POST /v1/missions/{id}/run              # operator: run the three-role fleet
GET  /v1/missions/{id}/run              # live event trail (in-memory; lost on restart — the
                                        #   audited decision trail survives in GET /v1/decisions)
POST /v1/missions/{id}/run/cancel       # stops the run; state becomes "cancelled"
GET  /v1/decisions?task=…               # the audited trail
GET  /v1/coverage, GET /v1/health       # enforcement facts / liveness (no auth, no data)
```

## Reference host

`cmd/qiyanhost` is a complete, minimal integration (about 300 lines,
**standard library only, zero private imports**) that asks `/v1/authorize`
before every action, obeys `ALLOW`/`DENY`, waits out `REQUIRE_APPROVAL` by
polling its own approval and re-submitting (safe due to dedup), reports every
result to `/v1/events` — and with `--rogue` performs one write with **no**
control call, so you can see that uncovered paths are never marked as
protected. The token is operator-issued (`qiyan agents add`); the host never
registers itself.

## Error shape, classes and HTTP mapping

```json
{ "error": { "class": "OUT_OF_SCOPE", "message": "…" } }
```

Classes are stable; new classes may be added, existing strings never change.
Map them in your UI; never parse `message`. HTTP status: `400`
`INVALID_REQUEST` (incl. unknown fields, bad JSON, failed validation) ·
`401` `UNKNOWN_IDENTITY` (missing/invalid credential) · `403`
`IDENTITY_CONFLICT`, `AUTHORIZATION_BINDING`, `AUTHORIZATION_REPLAYED`,
`OUT_OF_SCOPE` (loopback-host/CSRF refusals also use this class) · `404`
`NOT_FOUND`, `APPROVAL_NOT_FOUND` · `409` `CONFLICT`, `APPROVAL_EXPIRED`,
`APPROVAL_CANCELLED` · `503` `STORE_CORRUPTED`, `CONTROL_UNAVAILABLE` · `500`
`INTERNAL` (plus `BUDGET_EXHAUSTED`, `TASK_CANCELLED`, `APPROVAL_REQUIRED`
surfaced through decisions where applicable).

## What this contract does not cover

The console-local endpoints for project selection (`/v1/fs/*`), the connect
wizard (`/v1/connect*`), the Safety Lab (`/v1/lab/*`) and the synthetic
playground (`/v1/playground/scenarios`) exist for the local console and are
**not** part of the third-party integration surface: they may change without a
contract revision. The supported integration surface is exactly what this
document describes.
