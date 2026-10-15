# Qiyan — 开放式 AI Agent 任务指挥与安全控制平台

**你的 AI 团队，你的规则。**（Your AI Team. Your Rules.）

Qiyan 是一个本地优先的开放平台：用可验证的执行前控制运行 AI Agents——接入你
已有的 AI 软件，把任务交给一支小的 Agent 小队，观察每一步行动，并决定它们
可以做什么。安全在**执行之前**生效，而不是事后描述。其他软件也可以通过 Qiyan
开放、本地的 Control API，把同样的控制放在自己的行动之前。

下文每一项能力都链接到可复现的运行记录
（[R03 验收证据](docs/R03-ACCEPTANCE-REPORT.md)），**没有**覆盖的范围同样写明
（[范围与声明边界](docs/scope-and-claims.zh-CN.md)）。

一切都在本地运行：无需账号、不上云、无遥测——Qiyan 不向任何 Qiyan 服务发送
数据。中英文均为完整的一等支持。

**立刻体验：** **v0.3.0-beta.1 Public Beta**（五个平台，均从带 tag 的提交构建，
附校验和与构建来源证明）已装配完成，等待维护者的发布授权；获授权后文件将
出现在 [Releases](https://github.com/QiyanAI/ai-agent-action-surface/releases)。
无需账号，内置 60 秒演示。Beta 发布前：两条命令从源码构建，或向维护者索取
早期体验候选包——任何一条路都离演示只有一步。完整步骤：
[立即试玩](#立即试玩)。

![控制台首页：服务状态、四个入口、无需账号的演示](docs/images/console-home.png)

## 它能帮你做什么

- **让一支 AI 小队干活。** 给出一个目标；策划员、开发员、检查员在本机的三个
  真实会话中协作——敏感操作会在门禁处停下：范围内的等待你的决定，超出任务
  范围的被直接拒绝。
- **检查一个 AI 有没有风险。** 安全实验室读取 Agent 声明的能力与项目接线，
  每条结论都用通俗语言配证据，改善建议可以应用并**在你本机实测验证**。
- **给你的软件接入安全控制。** 任何 Agent Host 都可以在行动前询问 Qiyan 开放
  的本地 Control API：`ALLOW` 执行，`DENY` 停止，`REQUIRE_APPROVAL` 等待——
  一次批准，只授权那一个确切的工具调用执行一次（提供 `contentDigest` 即可把
  写入内容一并绑定，见契约文档）。

## 30 秒看懂

1. **连接**你的 Claude Code 项目（向导操作；只改一个项目文件，先留备份，可撤销）。
2. 真实探针会**实测**这台机器上危险操作能否真的被拦下——绝不只信版本号。
3. **给小队一个任务**：策划员制定计划、开发员执行、检查员复核。
4. 敏感操作**在门禁处停下**——范围内的等待你的决定（批准一次，或拒绝；被拒绝
   的操作在该任务内保持拒绝），超出任务范围的被直接拒绝，不会被任何批准放行。
5. 每个决策都**带证据记录下来**，控制台如实展示哪些被强制执行——以及哪些没有。

![一次真实运行停在真实审批上，原因用通俗语言说明](docs/images/mission-approval.png)

## 立即试玩

**无需账号。** 控制台内置 60 秒合成演示：三个 Agent 协作、一次操作被阻止、
一次由你亲手做出的决定。合成内容处处标注 `DEMO`，永远不会被当作真实观测记录。

### 获取构建（不装 Go 工具链）

> **EARLY ACCESS / BETA / NOT PRODUCTION-CERTIFIED（早期体验 / Beta /
> 未经生产认证）。** v0.3.0-beta.1 是未签名的、无 CGO 的二进制，覆盖
> linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64。
> 它属于早期体验——**不是**生产级安全认证——不附带支持或兼容性承诺。发布
> 已装配完成、等待维护者的授权；获授权后，各文件连同 `SHA256SUMS`、
> `LICENSE.txt` 与 EARLY ACCESS/BETA 标记——五个二进制加这三个文件——一起
> 出现在 [Releases](https://github.com/QiyanAI/ai-agent-action-surface/releases)；
> 每个二进制的构建来源可用 `gh attestation verify` 验证（证明存放在仓库的
> attestation 存储中，不是随附的签名文件）。Beta 发布前：受邀测试者通过仅
> 手动触发的 **Candidate Build** 工作流（Actions → Candidate Build）获取当前
> 候选包；任何人也可以按下方两条命令从源码构建。
>
> Windows SmartScreen 与 macOS Gatekeeper 会提示风险——没有签名，也请不要因此
> 关闭其他软件的系统安全警告。**请把二进制放在你会长期保留的文件夹里**
> （在连接任何项目之前）：已连接的项目指向它的绝对路径，移动或删除它会让
> 门禁按失败即拒绝工作，从而阻止该项目中的工具调用。分步的安装、校验与
> 卸载说明——包括"已构建 / 已实测 / 正式支持"三者的区别：
> [docs/install.md](docs/install.md)。已知限制与安全报告入口随发布随附，
> 也见 [docs/scope-and-claims.zh-CN.md](docs/scope-and-claims.zh-CN.md)。

为你的平台下载，然后校验并运行：

```sh
sha256sum -c SHA256SUMS        # 每一行都必须显示 OK（Git Bash/WSL；原生 Windows 用下方 Get-FileHash）
gh attestation verify qiyan_0.3.0-beta.1_<os>_<arch> --repo QiyanAI/ai-agent-action-surface
./qiyan_0.3.0-beta.1_<os>_<arch> serve
# 打印控制台地址、登录链接、状态目录与 token 文件路径
```

原生 Windows（PowerShell）没有内置 `sha256sum` 与 `gh`，改用：

```powershell
Get-FileHash .\qiyan_0.3.0-beta.1_windows_amd64.exe -Algorithm SHA256
# 将得到的哈希与 SHA256SUMS 中对应行比对
```

打开打印出的控制台地址（或登录链接），从 60 秒演示开始——无需账号、无需
Claude Code——或直接按下面的步骤连接一个真实项目。

### 从源码运行（Go 1.27+）

```sh
go build -o qiyan ./cmd/qiyan
./qiyan serve
```

全部仅使用标准库，没有任何第三方依赖需要下载。

### 运行环境：已构建 vs 已实测 vs 正式支持

三件不同的事，分开写明——已构建≠已测试，已测试≠正式支持：

| 状态 | 覆盖范围 |
| --- | --- |
| **已构建** | linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64——上述五个目标，每个都由候选构建的完整性门禁与来源证明绑定到确切提交 |
| **已实测** | windows/amd64：真实候选包实际运行过（服务启动、`/v1/health`、控制台可访问，2026-10-15）；CI 另在 ubuntu-、windows-、macos-latest 上编译并运行 `qiyan version`/`help`；R02/R03 的全部深度功能验证都在同一台 **Windows 11** 机器上完成。linux/arm64 与 darwin/amd64 的候选文件**从未被执行过**；linux/amd64 与 darwin/arm64 仅经 CI 运行器原生构建冒烟 |
| **正式支持** | **目前没有"支持的平台列表"。** 运行时支持：仅 Claude Code，2.1.295+ 上失败即拒绝（由连接器逐机实测，绝不只信版本号） |

"已实测"行之外的环境，请视为未经验证，而不是"不支持"。CI 运行于
**ubuntu-latest、windows-latest、macos-latest**。

## 连接你自己的 AI 项目

```sh
qiyan connect claude --project D:/work/my-project
```

向导会自动检测 Claude Code（也可指定二进制），展示将要改动的确切文件，运行
真实执行探针，然后才安装。控制台版本带有可视化项目选择器、通俗语言说明，
规则完全一致：不改全局、只留一份备份、撤销只移除 Qiyan 添加的内容。

![真实探针之后的保护等级实测——较弱档位与较强档位同样如实地写明](docs/images/connect-measured.png)

然后给小队一个任务（控制台或命令行均可）：

```sh
qiyan mission create --goal "阅读笔记并生成一份汇总报告" \
    --root D:/work/my-project
qiyan run --mission mi_...
# 需要你决定的事项会出现在控制台，也可用 `qiyan approvals list` 查看
```

## 安全边界（诚实清单·简述）

- 正式支持的运行时只有 **Claude Code**；失败即拒绝的保护需要 **2.1.295+**。
  连接器在当前机器上实测，无法验证的内容绝不含糊宣称。
- **Shell 命令一律等待你的批准。** 按设计，这里完全没有基于命令名的自动放行
  （原因见《范围与声明边界》）。Qiyan 无法解释的命令绝不静默放行。
- 路径检查是**词法与范围级别**的——真实、经验证的门禁，但**不是操作系统沙箱**。
  如果你的 Host 能绕过 Control API，Qiyan 不会宣称控制了那条路径；覆盖表如实
  说明哪些部分真正被强制执行。
- 没有 SaaS、没有账号、没有遥测——Qiyan 自身的控制服务只在环回地址运行，
  不向任何 Qiyan 服务上传数据。连接 Claude Code 时，其模型调用会发往
  Anthropic、适用 Anthropic 自身的网络与数据处理条款；该流量不经由 Qiyan
  中转，也不受 Qiyan 控制。

完整且精确的声明边界（做了什么、**没做什么**）：
[docs/scope-and-claims.zh-CN.md](docs/scope-and-claims.zh-CN.md) ·
[English](docs/scope-and-claims.md)。

## 面向开发者：Qiyan 开放控制契约

契约把一个循环写成五个阶段——
**Action → Risk & Evidence → Authorization → Enforcement → Outcome**——
任何 Agent Host 都能在行动前先问、行动后留下证明。控制台的**开发者入口**
完整演示了整条流程——并为它配了分步引导。接口是一套本地、版本化的
HTTP/JSON API：行动前 `POST /v1/authorize`、轮询审批、用 `POST /v1/events`
回传结果。仓库自带一个完全独立的参考 Host：

```sh
go run ./cmd/qiyanhost --addr 127.0.0.1:8760 --token $AGENT_TOKEN \
    --mission mi_... --task ta_... --workdir D:/scratch
```

接口契约文档（端点、决策语义、绑定规则、错误类、覆盖、兼容性策略）：
[docs/developer-api.zh-CN.md](docs/developer-api.zh-CN.md) ·
[English docs/developer-api.md](docs/developer-api.md)。

## 今日已验证的事实

| 领域 | 状态 |
| --- | --- |
| 本地控制服务、决策引擎、持久化状态 | 已实现、有测试、并经实机运行验证 |
| 执行前 PreToolUse 门禁 | 在真实 Claude Code 上验证（拒绝可阻断；≥ 2.1.295 上故障也阻断，`onFailure: "block"`） |
| 真实三角色小队（策划 → 开发 → 检查） | 已实机验证：产出真实文件，伴随真实控制决策 |
| 安全实验室（含已验证的修复） | 已实机验证：扫描 → 预览 → 应用 → 真实探针 → ENFORCED 覆盖记录 → 撤销 |
| 参考 Agent Host（仅用公开 API） | 已实机验证：允许 / 拒绝 / 审批 / 绕过路径演示 |
| 演示与训练场 | 合成、可互动，且与真实事件在数据层分离 |
| R03 验收证据 | [docs/R03-ACCEPTANCE-REPORT.md](docs/R03-ACCEPTANCE-REPORT.md) |

### 行动图（Action Graph）

所有层之下是同一个结构：**Agent → Tool → Action Capability（行动能力）**，
每条边都由证据支撑、而不是断言。行动类别携带能力（`read_files`、
`write_files`、`execute_process` 等）；工具名映射到它可能行使的能力；最终
记录的可信度不高于其 `EvidenceLevel`——`DECLARED`（声明如此）、
`STATICALLY_CONNECTED`（静态接线）、`OBSERVED`（接入点观测到）、或
`ENFORCED`（真实执行前门禁证明了它被拦下）。证据记录绝不高于其等级呈现，
每条"已强制执行"的结论都带明确的 `Boundary`——它**不**覆盖什么。这也是
控制台按执行路径显示 `ENFORCED / OBSERVED / UNCOVERED / UNKNOWN` 的原因，
也是 API 中刻意没有 `protected: true` 捷径的原因：从未被实跑过的路径保持
`UNKNOWN`，而未经验证不是承诺。（`internal/domain/evidence.go`、
`capability.go`。）

## 仓库结构

```
cmd/qiyan        命令行：serve / hook / status / agents / approvals / mission /
                 task / session / connect / run
cmd/qiyanhost    独立的参考 Agent Host（仅使用公开 API）
internal/domain  身份、状态、行动、策略、证据、覆盖
internal/control 唯一的行动决策引擎
internal/store   持久化本地状态（唯一写入者：serve）
internal/api     版本化本地 Control API + 内嵌控制台
internal/connector  Claude Code 检测、真实探针、配置安装/撤销
internal/mission    三角色小队编排
internal/lab        Agent 安全实验室
internal/playground 合成演示与训练内容（构造上即为惰性）
```

## 反馈

缺陷报告、功能请求、适配请求与文档问题都通过
[Issues](https://github.com/QiyanAI/ai-agent-action-surface/issues/new/choose)
提交——每种都有引导表单。本项目由一位维护者维护：响应是尽力而为的，不承诺
任何服务等级。疑似安全漏洞**不要**开 Issue——请使用
[SECURITY.md](SECURITY.md) 中的私密渠道。

## 安全、许可与贡献

Apache-2.0，仅使用标准库。漏洞报告：[SECURITY.md](SECURITY.md)；
贡献规则：[CONTRIBUTING.md](CONTRIBUTING.md)。
