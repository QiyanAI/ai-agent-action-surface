# Install and run Qiyan (Public Beta)

This page is for people who want to **run** Qiyan, not build it. Four steps,
no account, no Go toolchain, no JSON editing. (Developers who want to build
from source: two commands, see the end.)

> **EARLY ACCESS / BETA / NOT PRODUCTION-CERTIFIED.** v0.3.0-beta.1 is an
> unsigned binary. It is early access — **not** a production security
> certification — and carries no support or compatibility promise.
>
> **Where to get it.** The Beta is prepared and awaiting the maintainer's
> release authorization; when it ships, the five platform files attach to
> [Releases](https://github.com/QiyanAI/ai-agent-action-surface/releases) with
> `SHA256SUMS`, `LICENSE.txt` and a build-provenance attestation. Until then:
> invited testers receive current candidates through the dispatch-only
> **Candidate Build** workflow (Actions → Candidate Build), and anyone can
> build from source with Go 1.27+ (`go build -o qiyan ./cmd/qiyan`).

## The four steps

1. **Download** the file for your system (windows/amd64, linux/amd64,
   linux/arm64, darwin/amd64, darwin/arm64) from the Releases page.
2. **Unzip it into a folder you will keep** — for example `C:\Tools\qiyan` or
   `~/qiyan`. Do this *before* starting it or connecting any project. The
   service records the binary's absolute path, and **moving or deleting the
   binary later makes the gate fail closed and blocks the tools of every
   project you connected** (see "If you move the binary" below).
3. **Start it:**

   ```sh
   ./qiyan_0.3.0-beta.1_<os>_<arch> serve
   ```

   The banner prints the console address, a one-click **sign-in link**, the
   state directory and the token file paths. (Rename the file to `qiyan` if
   you prefer.)
4. **Open the console** — the printed address (or the sign-in link, which signs
   you in directly) — and click **Try the demo**: the 60-second walkthrough
   needs no account and no Claude Code. Then connect a real project if you
   want the three-agent fleet (`/docs` in the repository, or the console's
   project picker).

No Claude Code yet? The demo and the **Safety Lab** scans work anyway. The
three-agent mission needs Claude Code installed; the connect wizard probes it
on your machine before claiming anything.

## Verify what you downloaded

```sh
sha256sum -c SHA256SUMS          # every line must say OK
gh attestation verify qiyan_0.3.0-beta.1_<os>_<arch> --repo QiyanAI/ai-agent-action-surface
```

`gh attestation verify` proves **where the file was built from** — a specific
commit, by a specific workflow. It does not prove the software is free of
defects, and it is not a legal signature.

**Windows SmartScreen / macOS Gatekeeper will warn** on first run because the
binaries are unsigned. That warning is expected here; use "More info → Run
anyway" for this file only if you have verified the checksum above. Do **not**
disable SmartScreen or Gatekeeper for other software.

## Built vs actually tested vs officially supported

Three different things. See the full table in
[README](../README.md#where-it-runs-built-vs-tested-vs-supported); the short
version: all five targets are **built** and attestation-bound; only
**windows/amd64** has actually been executed (on Windows 11, 2026-10-15), plus
CI `version`/`help` smoke runs on ubuntu/windows/macos runners; linux/arm64
and darwin/amd64 have **never been executed**. There is **no officially
supported platform list yet** — no compatibility promise is made.

Runtime support is separate and precise: only **Claude Code**, with fail-closed
protection on **2.1.295+** — and the version number is never trusted: the
connector runs a live three-scenario probe on your machine.

## Known limitations

- **Unsigned.** No code signing on any platform; system warnings are expected.
- **Not an OS sandbox.** Path checks are lexical and scope-based. If a host can
  bypass the Control API, that path is not controlled — the coverage table says
  what is actually enforced, per execution path.
- **Shell commands always wait for your approval** — by design, with no
  name-based auto-allow.
- **Keep the binary in place.** Connected projects point at its absolute path.
- **One writer.** The serve process owns the state directory; don't run two
  services on one state directory.
- **Not production-certified.** Beta builds have no patch policy yet.

Full boundary: [scope-and-claims.md](scope-and-claims.md) ·
[中文](scope-and-claims.zh-CN.md).

## If you move the binary later

The gate fails closed: every tool call in the connected projects is blocked
until repaired. Fix it with either

```sh
./qiyan connect undo --project <project-dir>     # from the NEW location
```

then reconnect from the new location — or move the binary back to the recorded
path.

## Uninstall / undo

- **Disconnect a project:** `qiyan connect undo --project <project-dir>` (the
  console's Disconnect button does the same). It removes exactly what Qiyan
  added; the pre-install backup is kept beside the settings file as
  `.qiyan-backup`.
- **Stop the service:** end the `qiyan serve` process. Nothing else is running
  in the background.
- **Revoke all tokens and erase all local state:** delete the state directory
  (the banner prints its path; typically `%AppData%\QiyanAI\qiyan` on Windows
  or `~/.config/QiyanAI/qiyan` elsewhere). Operator and hook tokens are
  re-minted on every start, so restarting the service rotates them; deleting
  the state directory removes registered agents and every record Qiyan kept.
  There is no separate agent-token revoke command in this Beta — this is the
  honest, complete revocation path.

## Build from source instead (developers)

Go 1.27+, standard library only:

```sh
go build -o qiyan ./cmd/qiyan
./qiyan serve
```

## 安装与运行（中文）

四个步骤：**下载**你系统的文件 → **解压到你会长期保留的文件夹**（例如
`C:\Tools\qiyan`；在启动或连接任何项目**之前**放好——服务会记录二进制的绝对
路径，之后移动或删除它会让门禁按失败即拒绝工作，阻止已连接项目里的所有工具）
→ 运行 `./qiyan_0.3.0-beta.1_<os>_<arch> serve` → 打开横幅打印的控制台地址或
**一键登录链接**，从 60 秒演示开始（无需账号、无需 Claude Code）。

**校验：** `sha256sum -c SHA256SUMS` 每行须显示 OK；
`gh attestation verify <文件> --repo QiyanAI/ai-agent-action-surface` 证明文件
从哪个提交构建而来——但不能证明软件没有缺陷。二进制未签名，Windows
SmartScreen / macOS Gatekeeper 会提示风险：仅在你完成校验后对本文件选择
"仍要运行"，**不要**因此关闭系统对其他软件的保护。

**状态区分：** 五个平台均已**构建**并绑定来源证明；实际**实测**的只有
windows/amd64（Windows 11，2026-10-15）及 CI 上的冒烟运行；linux/arm64 与
darwin/amd64 **从未被执行**。目前**没有"正式支持的平台列表"**。运行时仅正式
支持 **Claude Code**，2.1.295+ 上失败即拒绝，且版本号从不可信——连接器会在你的
机器上真实实测。

**移动了二进制？** 门禁按失败即拒绝工作：从新位置执行
`qiyan connect undo --project <项目目录>` 后重新连接，或把二进制移回原路径。

**卸载：** 逐个项目 `qiyan connect undo`；结束 `qiyan serve` 进程；如需彻底
撤销，删除状态目录（横幅会打印其路径）——operator 与 hook token 每次启动都会
重铸，删除状态目录会一并移除已注册 Agent 与全部本地记录。Beta 暂无单独的
agent token 撤销命令，这就是完整、诚实的撤销路径。

**安全报告入口：** [SECURITY.md](../SECURITY.md)（私密渠道）；反馈与问题：
Issues 引导表单。
