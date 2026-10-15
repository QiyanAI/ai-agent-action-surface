# R03 field-test kit — first-use tests (U02 / independent first users / third-party hosts)

This kit is for the **real people** who run R03's user tests. Nothing in this
document simulates a user: the results only count if a human who did not build
Qiyan does the steps and the notes record what actually happened, including
failures and confusion. Claude Code must not fill in these forms.

Three tests are described; run the ones you were invited for. Record every
answer verbatim — a confusing sentence the tester actually said is more
valuable than a summary of it.

## Before the test (host checklist)

1. Start the service the tester will use: `qiyan serve` (or a candidate
   binary), and note the console address it prints.
2. Hand the tester the operator token (from the printed token file path).
   Do not touch their mouse or keyboard during the test.
3. Have the demo page ready as the 60-second entry: `http://<addr>/ui/` →
   **Try the demo**.
4. Keep the stopwatch on your phone; write down times as they happen.

---

## Test U02 — first 60 seconds (newcomer, no code knowledge needed)

Vehicle: the console home + the synthetic demo. No account, nothing real runs.

| # | Ask / watch | Record |
| --- | --- | --- |
| 1 | "You have 60 seconds. Tell me what this product does." | their words, verbatim |
| 2 | "What would you click next?" | what they click, and how long it takes to decide |
| 3 | In the demo: at the blocked action, "what just happened?" | their words |
| 4 | In the demo: at the approval, let them approve or deny | their choice + why |
| 5 | Stopwatch: time from start until they can explain the core value | seconds (target: ≤ 60; record the real number even if it is 180) |

Also record: the exact sentence (in their language) they said that shows
understanding or its absence; anything they misread; anything that looked
broken.

## Test U13 — two independent first users (one ideally with little coding experience)

Vehicle: the real product on this machine. For each tester, watch them (do not
help) do the five core operations and record completion / time / confusion /
their own words per step:

| Step | What to observe | Record |
| --- | --- | --- |
| 1. Project selection | Can they find and choose a folder without typing a path? Do they read the picker's offers? | done? time? words? |
| 2. Claude Code connection | Do they understand what will be changed BEFORE applying? Do they understand the measured protection line afterwards? | done? words? |
| 3. Create a mission | Do they understand the goal box and the working folder? | done? time? |
| 4. Read a safety prompt | When the first approval appears: do they understand what the AI wants and why it is paused? | words? |
| 5. Agent check (Safety Lab) | Can they produce a finding and open its evidence? | done? words? |

Also record, for each tester: which of the four questions ("what does it want
to do / why care / what did Qiyan do / what are my options") they could answer
unprompted.

**Do not complete any step for them.** If a tester is stuck for more than 3
minutes, note where and move on; a stuck step is a finding, not a failure of
the test.

## Test C — third-party host integration (one independent developer)

Vehicle: `docs/developer-api.md` ONLY — the developer may not read the source
code or ask the Qiyan authors for help beyond re-sharing the document.

| Step | Record |
| --- | --- |
| 1 | Can they explain, after reading, what `ALLOW` / `DENY` / `REQUIRE_APPROVAL` oblige their host to do? | yes/no + their phrasing |
| 2 | Did they complete the runbook (agent → mission with tools → task → session → authorize) **without asking a question**? | where did they first need help? |
| 3 | Did their host correctly wait for and execute an approved action exactly once? | observed? |
| 4 | When the control service is stopped, does their host treat it as "deny" (no action runs)? | observed? |
| 5 | Can they state which paths Qiyan does NOT control for their host? | their words |
| 6 | Would they continue integrating? Why / why not? | verbatim |

If no independent developer can be recruited, the result is recorded as
**NOT VALIDATED** — never replaced by the Qiyan authors running their own
reference host.

---

## Where results go

Copy the filled tables (with real names replaced by "Tester A/B" if the
testers prefer) into the acceptance record. Per the task book: the 60-second
number is a design target, not a promise — an honest 95 seconds beats a
fabricated 45. Any item nobody could run is reported as **NOT VALIDATED** with
the reason.
