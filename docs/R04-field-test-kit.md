# R04 field-test kit — first-use and first-integration tests

This kit is for the **real people** who run R04's user tests (R04-01 /
R04-02). It imports the R03 kit's protocols unchanged — its U02 and U13
tables, the host checklist and the results rules — and adds the R04 steps.
`docs/R03-field-test-kit.md` remains as the R03 record.

Nothing in this document simulates a user: the results only count if a human
who did not build Qiyan does the steps and the notes record what actually
happened, including failures and confusion. **Claude Code must not fill in
these forms, must not complete steps for a tester, and must not be counted as
a tester.** An executor rehearsal of these protocols is labelled as such and
never entered in the results tables.

## Before the test (host checklist)

1. Put the build the tester will use at a **stable path** (the binary records
   its own absolute path; moving it later fails the gate closed). Candidate:
   any `test`-flavor set from the Candidate Build workflow; once the Public
   Beta ships, the v0.3.0-beta.1 download from Releases.
2. Verify it before handing it over: `sha256sum -c SHA256SUMS` and
   `gh attestation verify <file> --repo QiyanAI/ai-agent-action-surface`.
3. For U13 step 0 the tester starts the service themselves; give them **only
   the binary and the README's "Get the build" section** — no verbal help.
4. Have the console's **Try the demo** page ready as the 60-second entry.
5. Keep the stopwatch on your phone; write times down as they happen.

## Consent (read aloud, keep the answer)

> "This is an early-access test of Qiyan, and it only starts with your
> informed consent. Three things about privacy so you can decide:
>
> **One** — Qiyan has no cloud service: nothing you do is uploaded to any
> Qiyan service, and no account is needed. **Two** — the built-in 60-second
> demo is local and synthetic; it calls no external AI model at all.
> **Three** — if we connect Claude Code and run a real task, its model calls
> go to Anthropic under Anthropic's own network and data-handling terms;
> that is the one channel where data reaches a third party, and it is Claude
> Code's channel, not Qiyan's.
>
> I will also write down what you do, what you say and how long steps take —
> quoted in your own words, with your name replaced by a letter if you
> prefer. The notes go into the project's test record; they are not sold or
> used for anything else, and you can ask me to delete them at any time.
> OK to start?"

Record: consent yes/no, anonymized yes/no.

---

## Test U02 — first 60 seconds (newcomer, no code knowledge needed)

Vehicle: the console home + the synthetic demo. No account, nothing real
runs, and no external AI model is called.

| # | Ask / watch | Record |
| --- | --- | --- |
| 1 | "You have 60 seconds. Tell me what this product does." | their words, verbatim |
| 2 | "What would you click next?" | what they click, and how long it takes to decide |
| 3 | In the demo: at the blocked action, "what just happened?" | their words |
| 4 | In the demo: at the approval, let them approve or deny | their choice + why |
| 5 | Stopwatch: time from start until they can explain the core value | seconds (target: ≤ 60; record the real number even if it is 180) |

**R04 addition — demo completion (R04-01: "能否完成免费试玩"):** stay silent and
watch the demo to its end. Record: demo completed? · if abandoned, at which
step · wall time. Do not prompt; abandonment is a finding, not a failure.

Also record: the exact sentence (in their language) that shows understanding
or its absence; anything they misread; anything that looked broken.

## Test U13 — two independent first users (one ideally with little coding experience)

Vehicle: the real product on the machine. For each tester, watch them (do not
help) and record completion / time / confusion / their own words per step.

| Step | What to observe | Record |
| --- | --- | --- |
| 0. Boot and enter (R04 addition) | Given only the binary + README section: can they launch it, find the console address line, open the console, locate the operator token and sign in (the printed sign-in link counts)? | done? time? words? what did they do with the token file? |
| 1. Project selection | Can they find and choose a folder without typing a path? Do they read the picker's offers? | done? time? words? |
| 2. Claude Code connection | Do they understand what will be changed BEFORE applying? Do they understand the measured protection line afterwards? | done? words? |
| 3. Create a mission | Do they understand the goal box and the working folder? | done? time? |
| 4. Read a safety prompt | When the first approval appears: do they understand what the AI wants and why it is paused? | words? |
| 4b. A real hard refusal (R04 addition) | During the run (or via the Safety Lab's harmless-block probe), let the first **DENY** happen unprompted — e.g. the reviewer's shell attempts in an enforced-tier run. Ask: what just happened? Is this "stopped by Qiyan" or "waiting for you"? | words? did they distinguish DENY from REQUIRE_APPROVAL? |
| 5. Agent check (Safety Lab) | Can they produce a finding and open its evidence? | done? words? |
| 6. Find help (R04 addition) | Before offering anything: "if you wanted instructions, where would you look?" Record whether they find the Help button / start a guided tour / notice the first-use prompt. | done? words? |

Also record, for each tester: which of the four questions ("what does it want
to do / why care / what did Qiyan do / what are my options") they could answer
unprompted.

**Do not complete any step for them.** If a tester is stuck for more than 3
minutes, note where and move on; a stuck step is a finding, not a failure of
the test.

## Test R04-02 — third-party developer integration (one person outside the project)

Vehicle: `docs/developer-api.md` **only** — the developer may not read the
source code or ask the Qiyan authors for help beyond re-sharing the document.
Pinned identity for the test: developer-api.md, contract revision 3 (its
header states the revision; if the header differs, record which one they had).

The seven items the task book requires (R04-02), as an ordered script with the
evidence to collect:

| # | Task for the developer | Expected result | Evidence to collect |
| --- | --- | --- | --- |
| 1 | Register a test agent (their host identity) | agent registered; token handled per doc | their command + output (redact the token) |
| 2 | Create a controlled mission with a tool allowance | mission + task exist; scope stated | their command + the mission/task IDs |
| 3 | Get one action **allowed**, and execute it | ALLOW runs; executed exactly once | their host's transcript of the call + execution |
| 4 | Get one action **denied** (outside scope) | DENY stops it; the action does not run | transcript showing the DENY and the skipped execution |
| 5 | Hit one **human approval**: wait, get it approved, retry the same call | executes exactly once; the retry does not double-execute | transcript + the approval record |
| 6 | Stop the control service, attempt an action | the action does not run (fail closed); ask them to state what the document says should happen and whether their host did that | observed behavior + their reading of developer-api.md |
| 7 | State which of their host's paths are NOT covered by Qiyan | an honest list in their words | their text, verbatim |

Debrief: (a) can they explain what `ALLOW` / `DENY` / `REQUIRE_APPROVAL`
oblige their host to do? (b) where did they first need help? (c) would they
continue integrating — why/why not?

If no independent developer can be recruited yet, the result is recorded
**NOT VALIDATED** and the invitation below stays open — never replaced by the
Qiyan authors running their own reference host (the task book forbids it).

### Invitation text (EN)

> **Invitation: try Qiyan's Open Control Contract as an outside developer.**
> Qiyan is an early-access, local-first control layer: before an agent host
> acts, it asks Qiyan and obeys ALLOW / DENY / REQUIRE_APPROVAL. I'm looking
> for one developer with no part in the project to integrate a small host
> against the public contract — register an agent, run one allowed action, one
> denied action, one approval, and see what happens when the service is down.
> About 1–2 hours, using only `docs/developer-api.md` (revision 3). Qiyan
> itself has no cloud service — the whole test runs on your machine — and if
> you connect Claude Code for a real task, its model calls go to Anthropic
> under Anthropic's own terms. You get a checksummed, provenance-attested
> binary and keep all the notes. Results are reported as [your own words], and stay "NOT
> VALIDATED" until a real outsider (you) completes them. Interested?

### 邀请文本（中文）

> **邀请：以外部开发者身份试用 Qiyan 开放控制契约。**
> Qiyan 是一个早期体验、本地优先的控制层：Agent Host 在行动之前先询问
> Qiyan，并遵守 ALLOW / DENY / REQUIRE_APPROVAL 的裁决。现寻找一位与项目
> 无关的开发者，依据公开契约做一个小接入——注册一个测试 Agent、完成一次
> 允许操作、一次禁止操作、一次人工审批，并观察控制服务停止时会发生什么。
> 约 1–2 小时，只使用 `docs/developer-api.md`（revision 3）。Qiyan 自身没有
> 云服务——整场测试都在你的机器上运行——如果你连接 Claude Code 执行真实任务，
> 其模型调用会发往 Anthropic、适用其自身条款。你会拿到带校验和与来源证明的
> 二进制，记录全归你。结果以
> 你的原话如实记录，在真实的"外部人"完成之前保持 NOT VALIDATED。有兴趣吗？

Handoff package: the chosen candidate set (binary + `SHA256SUMS` + label) plus
the pinned doc identity above; the developer keeps the notes and may publish
them under their own name.

---

## Results record (fill in; this table is the G4/G6 evidence)

| Tester (A/B/…) | Test | Consent | Completed? | Time | Key verbatim | P0 blocker? (blocks a core flow — yes/no, what) |
| --- | --- | --- | --- | --- | --- | --- |
|  | U02 |  |  |  |  |  |
|  | U13 (step-by-step above) |  |  |  |  |  |
|  | R04-02 (developer) |  |  |  |  |  |

Rules: copy the filled tables (names replaced per consent) into the acceptance
record. The 60-second number is a design target, not a promise — an honest 95
seconds beats a fabricated 45. Any item nobody could run is reported as
**NOT VALIDATED** with the reason. A P0 blocker (something blocking
start→console, demo, picker, connect, a blocked action, or help) goes into
the one consolidated fix round; taste-level suggestions are recorded for
later, not chased.

## 中文摘要（给组织者与中文测试者）

**测试前：** 把二进制放在固定路径并先校验（`sha256sum -c SHA256SUMS`、
`gh attestation verify`）；U13 第 0 步只给测试者二进制和 README 的"获取构建"
一节，不得口头帮忙；演示页作为 60 秒入口；秒表随时记录。

**同意（必须事先取得知情同意）：** 按上文"Consent"朗读并记录同意与否、是否
匿名。其中文版本：**一**——Qiyan 没有云服务，你做的任何事都不会上传到任何
Qiyan 服务，也无需账号；**二**——内置的 60 秒演示是本地合成的，完全不调用
外部 AI 模型；**三**——如果连接 Claude Code 执行真实任务，其模型调用会发往
Anthropic、适用 Anthropic 自身的网络与数据处理条款，这是数据到达第三方的
唯一渠道，且属于 Claude Code 的渠道、不是 Qiyan 的。测试记录以你的原话引用
（可选择匿名），只用于本项目测试记录，不作他用，你可随时要求删除。

**U02（60 秒）：** 五个问题逐条记录原话；新增完成度记录（是否完成试玩、
在第几步放弃、用时）。合成演示不调用任何外部 AI 模型。

**U13（两名独立用户，含一名少代码经验者）：** 第 0 步自己启动并进入控制台
（记录用时、对 token 文件做了什么）；第 1–3 步项目选择/连接/建任务；第 4 步
审批提示；第 4b 步真实的硬拒绝（区分 DENY 与 REQUIRE_APPROVAL）；第 5 步安全
实验室；第 6 步自己找帮助（记录是否找到 Help/引导）。卡住超过 3 分钟就记录并
继续——卡住本身就是发现。

**R04-02（外部开发者）：** 七项脚本逐条执行并收证据（允许一次、禁止一次、
一次审批且重试不重复执行、停服后动作不运行、说出未受控路径）；若无外部开发者，
结果保持 **NOT VALIDATED**，邀请函持续有效，绝不用自测顶替。

**结果：** 填最后一节的记录表；一句话不编——诚实的 95 秒胜过造出来的 45 秒；
没人能跑的条目如实写 NOT VALIDATED；P0 阻断问题集中修一轮。
