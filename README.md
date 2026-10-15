# Qiyan — Open Agent Mission & Control

**Your AI team. Your rules.**

Qiyan is a local, open platform for running AI agents under verifiable
pre-execution control: connect your existing AI software, hand a mission to a
small crew of agents, watch every action, and decide what they may do. Safety
is enforced *before* something runs — not described afterwards.

Everything is local-first: no account, no cloud, no telemetry. English and
中文 are both first-class.

## Try it in one minute (no account needed)

```sh
go build -o qiyan ./cmd/qiyan
./qiyan serve
```

Open the printed console URL and choose **Try the demo** — a synthetic
walkthrough (clearly labelled DEMO) showing a three-agent crew, one action
blocked before it happens, and one decision you make yourself.

## Put a real project under control

```sh
# 1. Connect Claude Code in a project (runs a live probe on your machine first)
./qiyan connect claude --project D:/work/my-project

# 2. Create a mission and run the three-role fleet
./qiyan mission create --goal "Write hello.txt and a short report" \
    --root D:/work/my-project
./qiyan run --mission mi_...

# Decisions that need you appear in the console (and in `qiyan approvals list`)
```

The console gives you: **Mission Control** (fleet cards, live timeline, action
map, inline approvals), the **Safety Lab** (inspect an agent's declared
abilities with evidence, preview improvements, apply them, and verify the
result with a real probe), the **Connect wizard**, the **demo**, and a
**Developer entry** for the open Control API.

## What is verified today

| Area | State |
| --- | --- |
| Local control service, engine, durable state | implemented, tested, verified in live runs |
| PreToolUse gate with real pre-execution blocking | verified on real Claude Code runs (deny blocks; failure modes block with `onFailure: "block"` on versions ≥ 2.1.295) |
| Three-role real fleet (Planner → Builder → Reviewer) | verified: live run producing real files with real control decisions |
| Safety Lab including verified fixes | verified: scan → apply → real probe → ENFORCED coverage record → undo |
| Reference Agent Host (public API only) | verified: allow / deny / approval / rogue-bypass demonstration |
| Demo & training | synthetic, interactive, data-layer-separated from real events |

Every claim above is bounded in
[Docs: scope and claims](docs/scope-and-claims.md) — what is **not** claimed
(no OS sandbox, no arbitrary-shell semantic control, no other runtimes, no
network force-blocking) is written down there with equal precision.

## Honest limits (short list)

- Only **Claude Code** is a supported runtime, and only from **2.1.295** for
  fail-closed protection (the connector detects and says so; the probe is the
  judge, never the version number).
- Qiyan's path checks are lexical and scope-based — they raise real, verified
  gates, but they are not an operating-system sandbox.
- Commands Qiyan cannot interpret go to your approval; they are never silently
  allowed.

## Repository layout

```
cmd/qiyan        the CLI: serve / hook / connect / run / lab / demo clients
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

Developer documentation for the API: [docs/developer-api.md](docs/developer-api.md)
· 中文说明见 [README.zh-CN.md](README.zh-CN.md)。

## Security and license

Apache-2.0. Standard library only. See [SECURITY.md](SECURITY.md) and
[docs/scope-and-claims.md](docs/scope-and-claims.md).
