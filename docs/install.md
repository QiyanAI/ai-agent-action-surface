# Install and run Qiyan (Public Beta)

This page is for people who want to **run** Qiyan, not build it. Four steps,
no account, no Go toolchain, no JSON editing. (Developers who want to build
from source: two commands, see the end.)

> **EARLY ACCESS / BETA / NOT PRODUCTION-CERTIFIED.** v0.3.0-beta.1 is an
> unsigned binary. It is early access — **not** a production security
> certification — and carries no support or compatibility promise.
>
> **Where to get it.** When the Release is listed on
> [Releases](https://github.com/QiyanAI/ai-agent-action-surface/releases), the
> five platform files are there with `SHA256SUMS`, `LICENSE.txt` and the
> EARLY ACCESS/BETA label, and build provenance is verifiable with
> `gh attestation verify`. If it is not listed there yet, invited testers
> receive current candidates through the dispatch-only **Candidate Build**
> workflow (Actions → Candidate Build), and anyone can build from source with
> Go 1.27+ (`go build -o qiyan ./cmd/qiyan`).

## The four steps

1. **Download** the file for your system (windows/amd64, linux/amd64,
   linux/arm64, darwin/amd64, darwin/arm64) from the Releases page — or take
   the fallback above if it is not listed there yet. There is **no archive**:
   the file you download *is* the program, so there is nothing to unzip.
2. **Save it into a folder you will keep** — for example `C:\Tools\qiyan` or
   `~/qiyan`. Do this *before* starting it or connecting any project. The
   service records the binary's absolute path, and **moving or deleting the
   binary later makes the gate fail closed and blocks the tools of every
   project you connected** (see "If you move the binary" below). If you want a
   shorter name than the one it ships with, rename it **now** — before you
   connect anything — and then substitute your name for the shipped one in the
   commands below. Verify **before** renaming: `SHA256SUMS` lists the shipped
   names, so a renamed file no longer matches it.
3. **Start it.**

   On **Linux and macOS** the downloaded file does not carry execute permission
   yet, so allow it to run once, then start it:

   ```sh
   chmod +x ./qiyan_0.3.0-beta.1_<os>_<arch>
   ./qiyan_0.3.0-beta.1_<os>_<arch> serve
   ```

   On **Windows**, run the downloaded file from the folder you saved it in —
   its name ends in `.exe`:

   ```powershell
   .\qiyan_0.3.0-beta.1_windows_amd64.exe serve
   ```

   The banner prints the console address, a one-click **sign-in link**, the
   state directory and the token file paths.
4. **Open the console** — the printed address (or the sign-in link, which signs
   you in directly) — and click **Try the demo**: the 60-second walkthrough
   needs no account and no Claude Code. Then connect a real project if you
   want the three-agent fleet (`/docs` in the repository, or the console's
   project picker).

No Claude Code yet? The demo and the **Safety Lab** scans work anyway. The
three-agent mission needs Claude Code installed; the connect wizard probes it
on your machine before claiming anything.

## Verify what you downloaded

**Download `SHA256SUMS` too**, from the same Release page, and put it in the
same folder as your binary — then run the check from that folder, because the
commands below need both files side by side.

`SHA256SUMS` lists all five platform files. You normally downloaded only one of
them, so check the one you have. Note carefully what the flag below means:
skipping a file you do **not** have is not the same as accepting a file that is
**wrong**. A file that is present is always checked, and a mismatch always
fails.

**Linux** (GNU coreutils):

```sh
sha256sum -c --ignore-missing SHA256SUMS
# your file's line must print OK; a mismatch prints FAILED and exits non-zero
```

**macOS** ships `shasum`, not `sha256sum`:

```sh
shasum -a 256 -c --ignore-missing SHA256SUMS
# your file's line must print OK
```

To check one file on its own instead:

```sh
shasum -a 256 qiyan_0.3.0-beta.1_darwin_arm64
# the printed digest must be identical to the matching line in SHA256SUMS
```

**Windows** (PowerShell), where neither tool is built in:

```powershell
Get-FileHash .\qiyan_0.3.0-beta.1_windows_amd64.exe -Algorithm SHA256
# compare case-insensitively: the Hash must equal the matching line in SHA256SUMS
# (PowerShell prints upper-case hex; SHA256SUMS is lower-case)
```

Then confirm **where the file was built from**. This one needs the GitHub CLI
(`gh`), which stock Windows does not include; on Windows the file name keeps its
`.exe` suffix, so include it:

```sh
# Linux / macOS
gh attestation verify qiyan_0.3.0-beta.1_<os>_<arch> --repo QiyanAI/ai-agent-action-surface

# Windows
gh attestation verify qiyan_0.3.0-beta.1_windows_amd64.exe --repo QiyanAI/ai-agent-action-surface
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

> **EARLY ACCESS / BETA / NOT PRODUCTION-CERTIFIED（早期体验 / Beta /
> 未经生产认证）。** v0.3.0-beta.1 是未签名的二进制，属于早期体验——**不是**
> 生产级安全认证——不附带支持或兼容性承诺。当该版本出现在
> [Releases](https://github.com/QiyanAI/ai-agent-action-surface/releases)
> 页面上时，五个平台文件连同 `SHA256SUMS`、`LICENSE.txt` 与
> EARLY ACCESS/BETA 标记都在那里。若该页面尚未列出，受邀测试者可通过
> Candidate Build 工作流获取当前候选包；任何人也可从源码构建（Go 1.27+）。

同样的四个步骤：

1. **下载**你系统的文件（windows/amd64、linux/amd64、linux/arm64、
   darwin/amd64、darwin/arm64）（若 Releases 页面尚未列出，见上方替代方式）。
   Release 提供的**不是压缩包**——下载下来的文件**就是**程序本体，
   不需要解压。
2. **把它保存到你会长期保留的文件夹**（例如 `C:\Tools\qiyan` 或 `~/qiyan`）。
   请在启动或连接任何项目**之前**放好——服务会记录二进制的绝对路径，之后移动
   或删除它会让门禁按失败即拒绝工作，阻止已连接项目里的所有工具（见下方
   "移动了二进制？"）。想换一个更短的文件名，请现在改好，并在下面的命令里改用
   你选的名字。**请先校验再改名**：`SHA256SUMS` 记录的是原始文件名，改名之后
   就不再匹配。
3. **启动它。** **Linux / macOS：**刚下载的文件还没有执行权限，先赋权再运行：

   ```sh
   chmod +x ./qiyan_0.3.0-beta.1_<os>_<arch>
   ./qiyan_0.3.0-beta.1_<os>_<arch> serve
   ```

   **Windows：**文件名为 `qiyan_0.3.0-beta.1_windows_amd64.exe`，在你保存它的
   文件夹里运行：

   ```powershell
   .\qiyan_0.3.0-beta.1_windows_amd64.exe serve
   ```

   横幅会打印控制台地址、一键登录链接、状态目录与 token 路径。
4. **打开控制台**——横幅打印的地址（或一键登录链接，可直接登录）——点击
   **Try the demo**：60 秒演示无需账号、无需 Claude Code。之后想要三角色小队，
   再连接一个真实项目（仓库中的 `/docs`，或控制台的项目选择器）。

还没装 Claude Code？没关系：**演示**与**安全实验室**的扫描照样能用。**三角色
小队需要先安装 Claude Code**；连接向导会在你机器上先实测，确认之后才给出结论。

**校验：** 请一并从**同一个 Release 页面**下载 `SHA256SUMS`，与二进制放在
**同一个文件夹**，并在那个文件夹里执行校验——下面的命令需要两个文件在一起。

`SHA256SUMS` 记录的是五个平台的文件，而你通常只下载了自己系统的那一个。
请特别留意下面这个参数的含义：**跳过你手上没有的文件**，不等于**接受一个校验
不符的文件**——只要文件存在就一定会被校验，不一致一定会失败。

- **Linux**（GNU coreutils）：`sha256sum -c --ignore-missing SHA256SUMS`，
  属于你这个文件的那一行必须显示 `OK`；不一致会显示 `FAILED` 并以非零码退出。
- **macOS** 自带的是 `shasum` 而不是 `sha256sum`：
  `shasum -a 256 -c --ignore-missing SHA256SUMS`，同样那一行必须显示 `OK`。
  想只校验单个文件，也可以运行
  `shasum -a 256 qiyan_0.3.0-beta.1_darwin_arm64`——输出的摘要必须与
  `SHA256SUMS` 中对应行一致。
- **Windows**（PowerShell，默认没有上述两个工具）：
  `Get-FileHash .\qiyan_0.3.0-beta.1_windows_amd64.exe -Algorithm SHA256`——
  得到的 Hash 值必须与 `SHA256SUMS` 中对应行一致。注意 PowerShell 输出的是
  **大写**十六进制，而 `SHA256SUMS` 是小写，比较时忽略大小写即可。

再确认文件**从哪里构建而来**。这一步需要 GitHub CLI（`gh`），Windows 默认
不含；在 Windows 上文件名要带上 `.exe` 后缀：

```sh
# Linux / macOS
gh attestation verify qiyan_0.3.0-beta.1_<os>_<arch> --repo QiyanAI/ai-agent-action-surface

# Windows
gh attestation verify qiyan_0.3.0-beta.1_windows_amd64.exe --repo QiyanAI/ai-agent-action-surface
```

它证明文件由哪个提交、经哪个工作流构建而来，但不能证明软件没有缺陷，也不是
法律签名。

二进制未签名，Windows SmartScreen / macOS Gatekeeper 会提示风险：仅在你完成
校验后对本文件选择"仍要运行"，**不要**因此关闭系统对其他软件的保护。

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
