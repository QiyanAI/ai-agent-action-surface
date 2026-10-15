# Qiyan — 开放式 AI Agent 任务指挥与安全控制平台

**你的 AI 团队，你的规则。**（Your AI Team. Your Rules.）

Qiyan 是一个本地优先的开放平台：用可验证的执行前控制运行 AI Agents——接入你
已有的 AI 软件，把任务交给一支小的 Agent 小队，观察每一步行动，并决定它们
可以做什么。安全在**执行之前**生效，而不是事后描述。

一切都在本地运行：无需账号、不上云、无遥测。中英文均为完整的一等支持。

## 一分钟试玩（无需账号）

```sh
go build -o qiyan ./cmd/qiyan
./qiyan serve
```

打开控制台打印的地址，选择 **立即试玩**——一段合成演示（清晰标注 DEMO）：
三个 Agent 协作、一次操作在执行前被阻止、以及一次由你亲手做出的决定。

## 把真实项目纳入控制

```sh
# 1. 在项目中接入 Claude Code（先在你本机运行真实探针）
./qiyan connect claude --project D:/work/my-project

# 2. 创建任务并运行三角色小队
./qiyan mission create --goal "写一个 hello.txt 和一份简短报告" \
    --root D:/work/my-project
./qiyan run --mission mi_...

# 需要你决定的事项会出现在控制台（也可用 `qiyan approvals list` 查看）
```

控制台包含：**任务指挥**（小队卡片、实时时间线、行动地图、内嵌审批）、
**安全实验室**（带证据地检查 Agent 声明能力、预览改善、应用并以真实探针验证
结果）、**连接向导**、**演示**，以及面向开放 Control API 的**开发者入口**。

## 今日已验证的事实

| 领域 | 状态 |
| --- | --- |
| 本地控制服务、决策引擎、持久化状态 | 已实现、有测试、并经实机运行验证 |
| 执行前 PreToolUse 门禁 | 在真实 Claude Code 上验证（拒绝可阻断；≥ 2.1.295 上故障也阻断） |
| 真实三角色小队（策划 → 开发 → 检查） | 已实机验证：产出真实文件，伴随真实控制决策 |
| 安全实验室（含已验证的修复） | 已实机验证：扫描 → 应用 → 真实探针 → ENFORCED 覆盖记录 → 撤销 |
| 参考 Agent Host（仅用公开 API） | 已实机验证：允许 / 拒绝 / 审批 / 绕过路径演示 |
| 演示与训练场 | 合成、可互动，且与真实事件在数据层分离 |

以上每一项的边界都写在[《范围与声明边界》](docs/scope-and-claims.zh-CN.md)中——
**未**宣称的内容（无操作系统沙箱、不控制无法解释的任意 Shell、不支持其他运行时、
不强制阻断网络）同样精确列明。

## 诚实的限制（简述）

- 正式支持的运行时只有 **Claude Code**；失败即拒绝的保护需要 **2.1.295** 及以上
  （连接器会检测并如实说明；以真实探针为准，绝不只看版本号）。
- 路径检查是词法与范围级别的——它们是真实、经验证的门禁，但不是操作系统沙箱。
- Qiyan 无法解释的命令一律进入你的审批，绝不静默放行。

## 仓库结构

```
cmd/qiyan        命令行：serve / hook / connect / run / lab / demo 客户端
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

开发者 API 文档：[docs/developer-api.md](docs/developer-api.md)（英文）。

## 安全与许可

Apache-2.0，仅使用标准库。详见 [SECURITY.md](SECURITY.md) 与
[docs/scope-and-claims.zh-CN.md](docs/scope-and-claims.zh-CN.md)。
