# The three demos — reproducible, real, no mock-ups

Three cases show what Qiyan does. All three run on a normal machine with a
downloadable build; none uses the synthetic Playground as evidence — the
synthetic 60-second demo is the in-console walkthrough and is labelled `DEMO`
there and here. Every figure comes from a real run recorded for this document;
the console marks real runs "Real run on this computer".

Preparation (all three demos):

```sh
# 1. a build you keep in place (Downloads is not a home; move it first)
qiyan serve --addr 127.0.0.1:8811        # note console URL + sign-in link
# 2. connect a scratch project with Claude Code 2.1.295+ (for A and B)
qiyan connect claude --project D:/work/demo-project
#    → the wizard runs the live probe and reports the measured tier
```

**Honesty notes, up front:** Demo A and B need Claude Code installed;
Demo C and the 60-second demo need it not at all. The measured protection tier
(and the Claude Code version it was measured against) is stated with each
figure below — the console shows the same facts. Where a screenshot shows the
synthetic demo, it is captioned as synthetic.

## Demo A — Your AI Team

*One goal. Three real sessions. Every sensitive action waits for you.*

```sh
qiyan mission create --goal "Read the notes in notes/ and write one summary report in REPORT.md" \
    --root D:/work/demo-project --mode controlled_auto
qiyan run --mission mi_…
# the console's mission page follows the run live
```

What happens, in order: the Planner plans; the Builder edits files; the
Reviewer **checks** — and when the Builder hits something sensitive (a shell
command, a write, a deletion), the run **pauses** and the console shows the
approval card with the reason in plain words. You click Approve or Deny —
once. The decision is consumed exactly once; a denied action stays denied for
that task; when the Reviewer tries a write as a read-only role, Qiyan **denies
it outright** (`write_by_readonly`) and the run continues. The mission ends
with `REPORT.md` on disk and a decision trail where every decision is stamped
with its coverage.

Reproduce + observe:

- the three sessions: watch the console's run area as the roles take turns;
- the gate: `qiyan approvals list` while the run is paused (or the console);
- the evidence: `GET /v1/decisions?task=ta_…` (or the console's action map) —
  each row carries outcome, structured reasons and `coverage`;
- the artifact: `REPORT.md` in your scratch project.

![A real run paused on a real approval, the reason in plain words](images/mission-approval.png)

## Demo B — Before It Acts

*The block happens before the action — and Qiyan says why.*

```sh
# same connected project; ask the crew for something that reaches outside scope
qiyan mission create --goal "Clean up temp files in the parent folder" \
    --root D:/work/demo-project --mode controlled_auto
```

The Builder attempts a path outside the mission's roots (or a shell command —
every shell command is approval-gated by design). The gate answers **before
anything runs**: an out-of-scope path gets `path_outside_scope`; a request that
reaches the environment gets `sensitive_command` and waits; a read-only role
attempting a write gets `write_by_readonly` and is **denied outright**. The
console's action map shows the row as *Stopped by Qiyan*; the approval card —
when the gate asks instead of denying — carries the full reason text.

Reproduce + observe:

- `qiyan approvals list` — the pending item names the exact tool and target;
- `GET /v1/decisions?task=ta_…` (or the console's action map / Developers
  entry) — the DENY rows with `write_by_readonly` / `path_outside_scope` and
  their parameters;
- the fail-closed line: stop `qiyan serve` and watch a hooked session refuse
  to run tools (exit 2) rather than act without a decision.

The honest limit, said here too: the console renders the plain-language reason
text on the approval card and in the decision trail; map rows show the outcome
(*Stopped by Qiyan*). Coverage is per path: `ENFORCED` only where a real check
proved it — elsewhere the console says `OBSERVED` / `UNCOVERED` / `UNKNOWN`.

## Demo C — Build With Qiyan

*Any host can ask before it acts — over the public API only.*

The [developer guide](developer-api.md) opens with the complete runbook.
The repository ships the reference host (`cmd/qiyanhost`, standard library
only, **no private imports**) and that is the whole integration:

```sh
qiyan serve                                  # 127.0.0.1:8760
qiyan agents add --name DemoHost --role builder     # prints ag_… + token once
qiyan mission create --goal "host demo" --root D:/work/demo-scratch \
    --tool note.read --tool report.append --tool tmp.cleanup --tool mail.send \
    --kind network.access
qiyan task create --mission mi_… --role builder --agent ag_… --title "host work"
qiyan session bind --task ta_… --agent ag_… --session demo-1
go run ./cmd/qiyanhost --addr 127.0.0.1:8760 --token $AGENT_TOKEN \
    --mission mi_… --task ta_… --workdir D:/work/demo-scratch
```

What you see, step by step: an out-of-scope **read** is `DENY`-ed
(`path_outside_scope`) and **not executed**; an in-scope read and an in-scope
append are `ALLOW`-ed and executed; removing the scratch file becomes
`REQUIRE_APPROVAL` (`delete_operation`) and the external notification another
(`external_network`), each waiting for you in the console — and after each
approval the identical retry executes **exactly once**. Stop `qiyan serve`
mid-run and the host stops too: it treats an unreachable control service as
*denied* (the contract states this obligation explicitly; without a decision
in time it reports "still waiting → not executed"). `--rogue` adds one write
with **no** control call, so you can see the coverage table never marks that
path protected.

**Ecosystem honesty:** this demonstrates *our* reference host working. Real
third-party adoption is a separate fact and stays **NOT VALIDATED** until an
outside developer completes the R04-02 script in
[the field-test kit](R04-field-test-kit.md).

## Captions and status used with these figures

- Real runs: "Real run on this computer — v0.3.0-beta.1, measured tier:
  enforced (Claude Code 2.1.300 or other probed version)". Substitute the
  actual measured version/tier; never generalize one machine's probe into a
  support claim.
- Synthetic: "The built-in 60-second demo — synthetic content, labelled DEMO
  in the console; nothing in it is a real run."

_These walks were executed on 2026-10-16 with the R04 rehearsal build
(`qiyan 0.3.0-r04-test`, three-role mission + reference host on Windows 11,
Claude Code 2.1.301 measured `enforced`); the raw transcripts and decision
exports go with the R04 acceptance record, and the release-day re-shoot swaps
in the shipped `v0.3.0-beta.1` binary and its figures._
