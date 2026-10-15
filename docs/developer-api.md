# Qiyan Control API — developer guide

The API is local, versioned (`/v1`), HTTP/JSON, and loopback-only. It is the
same interface the Qiyan console, the CLI, the Claude Code hook, and the
bundled reference host use — there is no private path.

Start the service:

```sh
qiyan serve                       # prints the address and the state dir
```

Credentials live as 0600 files in the state dir (file permissions are the
local-user trust anchor):

- `operator.token` — local-user operations (missions, approvals, connect).
- `hook.token` — only the `POST /v1/hook/decide` channel.

Agent tokens are issued once by `qiyan agents add --name X --role Y` (or
`POST /v1/agents`); only their SHA-256 is stored.

## The one question that matters: "may I do this?"

```
POST /v1/authorize          Authorization: Bearer <agent token>
{
  "missionId": "mi_...",
  "taskId":    "ta_...",
  "action": {
    "kind": "file.modify",           // file.read | file.create | file.modify |
                                     // file.delete | process.exec | network.access |
                                     // credential.use | external.state_change |
                                     // agent.delegation | session.local | unknown
    "tool": "report.append",         // your tool's name; must be in the mission's allowlist
    "path": "D:/work/report.md",     // file.* kinds: absolute, forward slashes
    "command": "go build ./...",      // process.exec
    "cwd": "D:/work",                // process.exec: the directory it will run in
    "url": "https://…",              // network.access
    "credentialRef": "DEPLOY_TOKEN", // credential.use: the NAME, never a value
    "contentDigest": "…"             // optional: bind the exact content a write would produce
  }
}
```

The answer is one of four outcomes — handle all four:

| Outcome | What your host must do |
| --- | --- |
| `ALLOW` | Perform exactly that action. |
| `DENY` | Do not perform it. The `reasons` explain why (codes + parameters; render them in your UI's language). |
| `REQUIRE_APPROVAL` | Do **not** perform it yet. Wait for the user (poll `GET /v1/approvals/{id}`), then re-submit the *identical* action. The user's approval authorizes one execution of exactly that action — a changed target, tool, or scope invalidates it. Never retry in a loop before the decision. |
| `UNKNOWN` | Treat as denial on any enforced path. It means Qiyan could not conclude "allowed". |

Also useful: `GET /v1/coverage` lists what is actually enforced on this
machine (real verification runs, not documentation). If your host can bypass
this API, Qiyan does not claim to control that path — say so at your
integration point, exactly like the coverage table does.

## Reporting what actually happened

```
POST /v1/events             Authorization: Bearer <agent token>
{ "taskId": "ta_...", "actionDigest": "…", "result": "executed|failed|blocked|cancelled", "note": "…" }
```

`actionDigest` comes from the authorize response. Honest reporting is the
point: `failed` and `blocked` exist so success is never implied.

## Approvals

```
GET  /v1/approvals?state=pending        # operator
GET  /v1/approvals/{id}                 # operator, or the bound agent
POST /v1/approvals/{id}/decision        # operator/local user ONLY
{ "approve": true }
```

Approval semantics, precisely: one approval → at most one execution, of the
exact action, bound to the agent, task, action digest and scope digest; it
expires (`expiresAt`); replay is refused; a denied action stays denied for
that task; agents cannot approve anything (`401`); and there is no
"allow everything" endpoint. Silence is never consent — an unanswered request
expires.

## Missions, tasks, sessions

```
POST /v1/missions                       # operator: goal + mode + explicit scope
                                        #   scope = roots, allowedKinds, allowedTools, budgets
POST /v1/missions/{id}/tasks            # operator
POST /v1/sessions                       # operator: bind a runtime session to a task (starts it)
POST /v1/missions/{id}/run              # operator: run the three-role fleet
GET  /v1/missions/{id}/run              # follow its live event trail
POST /v1/missions/{id}/run/cancel
GET  /v1/decisions?task=…               # the audited trail
```

## The gate itself (for runtime integrations)

```
POST /v1/hook/decide         Authorization: Bearer <hook token>
{ "sessionId": "…", "toolName": "Bash", "toolInput": {…}, "cwd": "…" }
```

Used by the installed Claude Code hook. Identity comes from the session
binding, never from the request body. Exit-code semantics on the runtime side
(verified, not assumed): `ALLOW` → exit 0; everything else → exit 2 with the
explanation on stderr; any failure to obtain a decision → exit 2 (fail
closed).

## Reference host

`cmd/qiyanhost` is a complete, minimal integration (about 300 lines, public
API only) that registers, asks before every action, obeys, waits for
approvals, and reports results — including a `--rogue` mode that skips the
control call so you can see that the system does *not* mark such actions as
protected.

## Error shape

```json
{ "error": { "class": "OUT_OF_SCOPE", "message": "…" } }
```

Classes are stable (`OUT_OF_SCOPE`, `APPROVAL_REQUIRED`, `AUTHORIZATION_BINDING`,
`AUTHORIZATION_REPLAYED`, `UNKNOWN_IDENTITY`, `BUDGET_EXHAUSTED`,
`CONTROL_UNAVAILABLE`, `STORE_CORRUPTED`, …). Map them to your UI; never parse
the message.
