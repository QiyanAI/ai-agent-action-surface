# Qiyan — 开放式 AI Agent 任务指挥与安全控制平台

**你的 AI 团队，你的规则。**（Your AI Team. Your Rules.）

Qiyan 是一个本地优先的开放平台：用可验证的执行前控制运行 AI Agents——接入你
已有的 AI 软件，把任务交给一支小的 Agent 小队，观察每一步行动，并决定它们
可以做什么。安全在**执行之前**生效，而不是事后描述。

一切都在本地运行：无需账号、不上云、无遥测。中英文均为完整的一等支持。

![控制台首页：服务状态、四个入口、无需账号的演示](docs/images/console-home.png)

## 它能帮你做什么

- **让一支 AI 小队干活。** 给出一个目标；策划员、开发员、检查员在本机的三个
  真实会话中协作——每一个敏感操作都会停下来等你的决定。
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
4. 敏感操作**停下来问你**——批准一次，或拒绝；被拒绝的操作在该任务内保持拒绝。
5. 每个决策都**带证据记录下来**，控制台如实展示哪些被强制执行——以及哪些没有。

![一次真实运行停在真实审批上，原因用通俗语言说明](docs/images/mission-approval.png)

## 立即试玩

**无需账号。** 控制台内置 60 秒合成演示：三个 Agent 协作、一次操作被阻止、
一次由你亲手做出的决定。合成内容处处标注 `DEMO`，永远不会被当作真实观测记录。

### 不装 Go 工具链也能运行（早期体验）

> **EARLY ACCESS / TEST BUILD / NOT PRODUCTION-CERTIFIED（早期体验 / 测试构建 /
> 未经生产认证）。** 候选程序是未签名的、无 CGO 的测试二进制，覆盖
> linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64，
> 由仅手动触发的 **Candidate Build** 工作流（Actions → Candidate Build）
> 构建，作为该次运行的工件下载——绝不创建 GitHub Release，不附带支持或
> 兼容性承诺。请向维护者索取运行链接（只有具备写权限的用户可以触发）。
> 每个工件集都包含 `CANDIDATE-LABEL.txt`（EARLY ACCESS 标记随文件同行）、
> `SHA256SUMS`，以及把每个二进制绑定到确切提交的构建来源证明
> （`gh attestation verify <file> --repo QiyanAI/ai-agent-action-surface`）。
> Windows SmartScreen 与 macOS Gatekeeper 会提示风险；没有签名。**请让二进制
> 留在你放置它的位置**——已连接的项目指向它的绝对路径，移动或删除它会让门禁
> 按失败即拒绝工作，从而阻止该项目中的工具调用。关于该渠道的完整说明：
> [docs/scope-and-claims.zh-CN.md](docs/scope-and-claims.zh-CN.md)。

然后运行你解压出的文件（工件保留完整文件名；也可以重命名为 `qiyan`）：

```sh
./qiyan_0.3.0-r03-candidate_<os>_<arch> serve
# 打印控制台地址、状态目录与 token 文件路径
```

打开打印出的控制台地址，粘贴启动横幅中 token 文件里的 operator token，
从演示开始——或直接按下面的步骤连接一个真实项目。

### 从源码运行（Go 1.27+）

```sh
go build -o qiyan ./cmd/qiyan
./qiyan serve
```

全部仅使用标准库，没有任何第三方依赖需要下载。

### 运行环境

CI 在 **ubuntu-latest、windows-latest、macos-latest** 上构建与测试，候选构建
覆盖上述五个平台目标；R02/R03 的实机验证在 **Windows 11** 上完成。目前
**没有"支持的平台列表"**——其他环境请视为未经验证，而不是"不支持"。

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
- 没有 SaaS、没有账号、没有遥测。任何内容都不会离开你的电脑。

完整且精确的声明边界（做了什么、**没做什么**）：
[docs/scope-and-claims.zh-CN.md](docs/scope-and-claims.zh-CN.md) ·
[English](docs/scope-and-claims.md)。

## 面向开发者：开放控制层

控制台的**开发者入口**完整演示了整条流程——并为它配了分步引导。接口是一套
本地、版本化的 HTTP/JSON API：行动前 `POST /v1/authorize`、轮询审批、用
`POST /v1/events` 回传结果。仓库自带一个完全独立的参考 Host：

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
| 执行前 PreToolUse 门禁 | 在真实 Claude Code 上验证（拒绝可阻断；≥ 2.1.295 上故障也阻断） |
| 真实三角色小队（策划 → 开发 → 检查） | 已实机验证：产出真实文件，伴随真实控制决策 |
| 安全实验室（含已验证的修复） | 已实机验证：扫描 → 预览 → 应用 → 真实探针 → ENFORCED 覆盖记录 → 撤销 |
| 参考 Agent Host（仅用公开 API） | 已实机验证：允许 / 拒绝 / 审批 / 绕过路径演示 |
| 演示与训练场 | 合成、可互动，且与真实事件在数据层分离 |
| R03 验收证据 | [docs/R03-ACCEPTANCE-REPORT.md](docs/R03-ACCEPTANCE-REPORT.md) |

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

## 安全、许可与贡献

Apache-2.0，仅使用标准库。漏洞报告：[SECURITY.md](SECURITY.md)；
贡献规则：[CONTRIBUTING.md](CONTRIBUTING.md)。
