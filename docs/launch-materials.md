# R04 launch materials (Public Beta v0.3.0-beta.1)

**Status: for the maintainer to publish; nothing here posts itself.** The §6
notes are the drafting source for the GitHub Release body. Posting to any other
channel happens only when the maintainer — a human — does it, under that
community's own rules. These are drafts for that use.

Everything below follows the claim boundary in
[scope-and-claims.md](scope-and-claims.md); if any phrasing here ever
contradicts the code or that boundary, the boundary wins and this file is
wrong.

---

## 1. One-line description (any channel, EN)

> **Qiyan — Your AI Team. Your Rules.** A local, open platform for running AI
> agents under pre-execution control: connect Claude Code, watch a small agent
> team work, and stop sensitive actions *before* they run — in-scope ones wait
> for your approval, and anything outside the mission's scope is denied
> outright.

## 2. 一句话描述（中文，任意渠道）

> **Qiyan —— 你的 AI 团队，你的规则。** 本地优先的开放平台：用执行前控制运行
> AI Agents——接入 Claude Code，看小队干活，敏感行动在**执行之前**就被拦下：
> 任务范围内的敏感操作等待你的批准，超出任务范围的操作被直接拒绝。

## 3. Short blurb (~60 words, GitHub Release / community posts)

Qiyan connects to your Claude Code projects and puts a real gate in front of
what agents do. A Planner, a Builder and a Reviewer run as three real sessions;
every shell command, deletion and credential use waits for your approval, and
anything outside the mission's scope is denied outright, with the reason in
plain words. Decisions are recorded with evidence, what is *not* protected
is stated plainly, and an open local API lets other tools ask the same
question before they act. Local-first, no account, no telemetry. Apache-2.0.

### 简短介绍（中文）

Qiyan 接入你的 Claude Code 项目，把真正的门禁放在 Agent 行动之前。策划员、
开发员、检查员是三个真实会话；每一条 Shell 命令、删除与凭据使用都会停下等待
你的批准，而越界路径与范围外操作被直接拒绝、并给出通俗原因。每个决策都带证据
记录，**没有**被保护的部分如实写明；开放、
本地的 API 让其他软件也能在行动前问同一个问题。本地优先、无需账号、无遥测。
Apache-2.0。

## 4. Medium post (~150 words, tech communities / dev forums)

**Your AI team. Your rules.**

AI agents are becoming coworkers — but most tooling asks you to trust first and
audit later. Qiyan is a local-first counterpart: connect your existing Claude
Code setup, hand a goal to a three-agent crew (planner → builder → reviewer),
and watch a console where every sensitive action **stops before it runs**:
shell commands always wait for your approval, and an out-of-scope write is
**denied outright** — with a reason in plain words, and with no approval that
could release it. Each decision is recorded with machine-stable evidence,
and the console shows coverage per path: `ENFORCED`, `OBSERVED`, `UNCOVERED`,
`UNKNOWN` — with the not-yet-verified paths shown honestly as such. There is no OS-sandbox
claim, no cloud, no account and no telemetry. Developers can put the same
decision point in front of their own agent host through the open
[Open Control Contract](developer-api.md): *may I do this?* → allow, deny, or
wait for one human approval.

Try it (no account, 60-second demo): [Releases
download](https://github.com/QiyanAI/ai-agent-action-surface/releases) ·
[repository](https://github.com/QiyanAI/ai-agent-action-surface) ·
[install guide](install.md). It is an early-access Beta — stated plainly, not
a production security certification.

### 中等长度（中文

**你的 AI 团队，你的规则。**

AI Agent 正在成为同事——但多数工具让你先信任、后审计。Qiyan 是本地优先的
另一条路：接入你现有的 Claude Code，把目标交给三角色小队（策划 → 开发 →
检查），在控制台里看每一个敏感行动**在执行前停下**：Shell 命令一律等待你的
批准；越界写入被**直接拒绝**——给出通俗原因，且不存在任何可以放行它的批准。
每个决策都以机器稳定的证据记录，控制台
按路径显示覆盖状态：`ENFORCED`、`OBSERVED`、`UNCOVERED`、`UNKNOWN`——诚实的
未验证同样在内。没有操作系统沙箱的宣称，不上云、无账号、无遥测。开发者可以
通过开放的[开放控制契约](developer-api.zh-CN.md)把同一个决策点放进自己的
Agent Host：*我可以做这件事吗？* → 允许、拒绝、或等待一次人工批准。

立即体验（无需账号，60 秒演示）：[Releases
下载](https://github.com/QiyanAI/ai-agent-action-surface/releases) ·
[仓库](https://github.com/QiyanAI/ai-agent-action-surface) ·
[安装指南](install.md)。这是早期体验 Beta——如实说明，不是生产级安全认证。

## 5. Three core use cases (the demos)

The three reproducible cases behind every version of this copy — with exact
commands and what you will see — live in [demos.md](demos.md):

- **Demo A — Your AI Team.** One goal; three real sessions; a sensitive action
  waits for your decision. *AI actually works for you — and you stay in
  control.*
- **Demo B — Before It Acts.** An out-of-bounds action meets the gate and does
  not run, with the reason in plain words. *Safety is not a scan report — it is
  control before the action.*
- **Demo C — Build With Qiyan.** The independent reference host integrates via
  the public API only: allow / deny / approval / stop-when-service-is-down.
  *Other developers can use Qiyan as their open control layer.*

中文对照：

- **Demo A — 你的 AI 团队。** 一个目标；三个真实会话；敏感行动停下等你决定。
  *AI 真的替你干活——而控制权始终在你。*
- **Demo B — 在它行动之前。** 越界行动撞上门禁、没有执行，原因用通俗语言给出。
  *安全不是扫描报告，而是行动之前真实发生的控制。*
- **Demo C — 用 Qiyan 构建。** 独立参考 Host 仅用公开 API 完成接入：允许 /
  拒绝 / 审批 / 控制服务失联即停止。*其他开发者可以把 Qiyan 当作自己的开放
  控制层。*

演示的可复现步骤（真实运行、真实记录）见 [demos.md](demos.md)。

## 6. GitHub Release notes (source for the Release body; the maintainer attaches them)

> **Qiyan v0.3.0-beta.1 — Early Access / Beta / NOT PRODUCTION-CERTIFIED**
>
> The first public Beta of Qiyan, an open Agent Mission & Control platform.
> Download the file for your platform below (windows/amd64, linux/amd64,
> linux/arm64, darwin/amd64, darwin/arm64), verify it with `SHA256SUMS` and
> `gh attestation verify <file> --repo QiyanAI/ai-agent-action-surface`, and
> run `qiyan serve` → open the console → **Try the demo** (no account needed,
> 60 seconds, nothing real runs).
>
> **What it does:** connect Claude Code projects, run a three-role agent crew
> under pre-execution control, stop every sensitive action before it runs —
> in-scope actions wait for your approval; anything outside the mission's
> scope is denied outright — keep every decision with evidence, and integrate
> other tools via the open Control Contract.
>
> **What it is not:** not a production security certification; not an OS
> sandbox; not a claim over paths that bypass the Control API; only Claude
> Code is a supported runtime (fail-closed on 2.1.295+, measured per machine).
> Binaries are unsigned — SmartScreen/Gatekeeper will warn; do not disable
> those warnings for other software. Shell commands always wait for your
> approval. Full boundary: docs/scope-and-claims.md.
>
> **Build identity:** commit `<SHA>` · tag `v0.3.0-beta.1` · assets: the five
> platform binaries + `SHA256SUMS` + `LICENSE.txt` + the EARLY ACCESS/BETA
> label (`CANDIDATE-LABEL.txt`). Provenance: verify any binary with
> `gh attestation verify <file> --repo QiyanAI/ai-agent-action-surface` — the
> attestation binds it to this tag's commit.
>
> **Platform scope, precisely:** all five targets are built from this tag;
> windows/amd64 was executed on Windows 11 for the release checks; the other
> targets are built-and-attested but not executed; there is no officially
> supported platform list yet. Runtime support: only Claude Code, fail-closed
> on 2.1.295+ (probed per machine).
>
> **First run:** put the file in a folder you will keep *before* connecting
> any project — moving it later fails the gate closed. Install, verify and
> uninstall: <https://github.com/QiyanAI/ai-agent-action-surface/blob/main/docs/install.md>.
> Known limitations: that page's "Known limitations" section. Security
> reports: <https://github.com/QiyanAI/ai-agent-action-surface/blob/main/SECURITY.md>
> (private channel, no public issues).
>
> Apache-2.0. No account, no telemetry, nothing sent to any Qiyan service.
> When you connect Claude Code, model traffic goes to Anthropic under its own
> terms. Full boundary: <https://github.com/QiyanAI/ai-agent-action-surface/blob/main/docs/scope-and-claims.md>.

## 7. Boundary block — include (verbatim or linked) in every channel

Required by R04-09 in any copy used publicly; keep this block together:

- Qiyan's own control service runs locally (loopback); it uploads nothing to
  any Qiyan service. When you connect Claude Code, its model calls go to
  Anthropic under Anthropic's own network and data-handling terms.
- Qiyan is **not an operating-system sandbox**.
- Only **Claude Code** is a supported runtime today.
- **Shell commands always wait for your approval.**
- Paths not covered by the integration are **not claimed as protected** — the
  coverage table says what is actually enforced.
- **Beta is not a production security certification.**

中文版：

- Qiyan 自身的控制服务在本机（环回）运行，不向任何 Qiyan 服务上传数据；连接
  Claude Code 时，其模型调用发往 Anthropic、适用 Anthropic 自身的网络与数据
  处理条款。
- Qiyan **不是操作系统沙箱**。
- 当前正式支持的运行时只有 **Claude Code**。
- **Shell 命令一律等待你的批准。**
- 集成未覆盖的路径**不宣称受到保护**——覆盖表如实说明哪些真正被强制执行。
- **Beta 不是生产级安全认证。**

## 8. Release-day checklist (maintainer actions — or the agent, only under a named authorization)

Order matters: the tag comes **before** the build, because the beta flavor
refuses to run outside a `refs/tags/v<version>` checkout — that machine check
is what binds the artifact's version identity to a real tag (G3). A tag
publishes nothing by itself.

1. Merge the R04 PR (normal maintainer flow; CI must be green — G1).
2. Create tag `v0.3.0-beta.1` on the merge commit (the exact SHA main now is).
   A lightweight tag is fine; it is not a Release.
3. Dispatch **Candidate Build** **at the tag**
   (`--ref v0.3.0-beta.1`) with `flavor: beta`,
   `version_label: 0.3.0-beta.1`; wait for the run; download the artifact.
   The label file inside records the tag, the commit and the flavor.
4. Verify: `sha256sum -c SHA256SUMS` (or Windows `Get-FileHash` per file),
   `gh attestation verify` (each binary), run the Windows binary once
   (`serve` + console + demo) — record the outputs for the readiness record.
5. Create the Release as a **draft prerelease**, with the five binaries +
   `SHA256SUMS` + `LICENSE.txt` + `CANDIDATE-LABEL.txt` (8 files) from a
   directory that holds exactly those files, and the notes with the commit/tag
   and platform-scope lines filled from the run. `--draft` is deliberate: the
   tag, the eight assets and the checksums are re-checked from the draft
   before anything is public. `--verify-tag` refuses to create a second tag,
   and `--latest=false` keeps a prerelease out of the "Latest" slot:

   ```sh
   gh release create v0.3.0-beta.1 release-assets/* \
     --repo QiyanAI/ai-agent-action-surface \
     --verify-tag --draft --prerelease --latest=false \
     --title "Qiyan v0.3.0-beta.1 — Public Beta" \
     --notes-file release-notes.md
   ```
6. **Publish the draft** once step 5's checks pass:

   ```sh
   gh release edit v0.3.0-beta.1 \
     --repo QiyanAI/ai-agent-action-surface \
     --draft=false --prerelease
   ```

R03 test-build artifacts (the `r03-candidate-*` sets) expire in the artifact
store and are **never** part of the Release; only the freshly built,
tag-bound `beta-candidate-*` set goes up.

### No release-day doc swap any more — superseded

An earlier revision of this checklist had the maintainer open a follow-up PR
*after* publication to swap the README's release-state paragraph (and its
Chinese twin) from "awaiting the maintainer's authorization" to "download
from Releases". That is no longer needed and must not be done: the wording in
`README.md`, `README.zh-CN.md` and `docs/install.md` now carries both states
itself — "**when the Release is listed on [Releases] …**" for the published
case, "**if it is not listed there yet …**" for the window before it appears —
in English and Chinese alike. Swapping that text now would break it.

## 9. Contribution and license entry (every channel)

Repository: <https://github.com/QiyanAI/ai-agent-action-surface> ·
Contributing: [CONTRIBUTING.md](../CONTRIBUTING.md) (issues first; guided
forms; `good first issue` / `help wanted` labels mark the small work) ·
License: **Apache-2.0** · Security: [SECURITY.md](../SECURITY.md) (private
reporting — never a public issue) ·
开发者入口：Issues 引导表单（含 Agent Runtime Adapter Request）与
[CONTRIBUTING.md](../CONTRIBUTING.md)。

## 10. Channel notes

- **GitHub Release:** §6 notes + §7 boundary; assets + checksums + provenance
  attached (R04-04 set).
- **Tech communities / dev forums:** §3 or §4 text; link the repository and
  `docs/install.md`; include §7 (or link it if the platform allows).
- **AI-Agent communities:** lead with Demo A/B/C; the contract section of §4;
  keep the Beta wording.
- **What is forbidden:** buying placement, mass DMs/emails, fake accounts or
  reviews, and any phrasing from the banned list in
  `.github/claims-banned.txt`. Posting is a maintainer action; this file never
  posts itself.
