# Qiyan Control API —— 开发者指引

API 是本地的、版本化的（`/v1`）、HTTP/JSON，仅监听环回地址。控制台、CLI、
Claude Code Hook 与内置参考 Host 使用的都是这一套接口——不存在私有通道。

启动服务：

```sh
qiyan serve                       # 打印监听地址与状态目录
```

凭据以 0600 文件保存在状态目录（文件权限即本地用户的信任锚点）：

- `operator.token` —— 本地用户操作（任务、审批、连接）。
- `hook.token` —— 仅用于 `POST /v1/hook/decide` 通道。

Agent Token 由 `qiyan agents add --name X --role Y`（或 `POST /v1/agents`）
一次性签发；仅存储其 SHA-256。

## 唯一重要的问题：“我可以做这件事吗？”

```
POST /v1/authorize          Authorization: Bearer <agent token>
{
  "missionId": "mi_...",
  "taskId":    "ta_...",
  "action": {
    "kind": "file.modify",           // file.read | file.create | file.modify |
                                     // file.delete | process.exec | network.access |
                                     // credential.use | external.state_change |
                                     // agent.delegation | session.local | unknown
    "tool": "report.append",         // 你工具的名字；必须在任务的允许清单里
    "path": "D:/work/report.md",     // file.*：绝对路径、正斜杠
    "command": "go build ./...",      // process.exec
    "cwd": "D:/work",                // process.exec：命令将运行的工作目录
    "url": "https://…",              // network.access
    "credentialRef": "DEPLOY_TOKEN", // credential.use：只写名称，绝不写值
    "contentDigest": "…"             // 可选：绑定这次写入的确切内容
  }
}
```

返回四种结果之一——四种都必须处理：

| 结果 | 你的 Host 必须做什么 |
| --- | --- |
| `ALLOW` | 执行这一确切操作。 |
| `DENY` | 不要执行。`reasons` 说明原因（代码 + 参数，按你界面的语言渲染）。 |
| `REQUIRE_APPROVAL` | **先不要执行**。等待用户（轮询 `GET /v1/approvals/{id}`），然后重新提交**完全相同**的操作。用户的批准只授权这一确切操作执行一次——目标、工具或范围改变即使批准失效。决定之前不要循环重试。 |
| `UNKNOWN` | 在任何强制路径上按拒绝处理。它意味着 Qiyan 无法得出“被允许”的结论。 |

另有用处：`GET /v1/coverage` 列出本机实际被强制执行的路径（来自真实验证运行，
而非文档）。如果你的 Host 能绕过此 API，Qiyan 不会宣称控制了那条路径——请在
你的集成点如实说明，与覆盖表保持一致。

## 上报真实发生的结果

```
POST /v1/events             Authorization: Bearer <agent token>
{ "taskId": "ta_...", "actionDigest": "…", "result": "executed|failed|blocked|cancelled", "note": "…" }
```

`actionDigest` 来自 authorize 的响应。如实上报是重点：`failed` 与 `blocked`
的存在就是为了绝不让成功被默认暗示。

## 审批

```
GET  /v1/approvals?state=pending        # operator
GET  /v1/approvals/{id}                 # operator，或绑定的 Agent 本人
POST /v1/approvals/{id}/decision        # 仅 operator/本地用户
{ "approve": true }
```

审批语义（精确定义）：一次批准 → 至多一次执行，且只能执行那一确切操作，绑定
到 Agent、任务、操作摘要与范围摘要；会过期（`expiresAt`）；重放会被拒绝；
被拒绝的操作在本任务内保持拒绝；Agent 不能批准任何东西（返回 `401`）；
不存在“全部允许”端点。沉默从来不是同意——无人回应的请求会过期。

## 任务、子任务、会话

```
POST /v1/missions                       # operator：目标 + 模式 + 明确范围
                                        #   范围 = 目录、允许的操作类别、允许的工具、预算
POST /v1/missions/{id}/tasks            # operator
POST /v1/sessions                       # operator：把运行时会话绑定到子任务（并启动它）
POST /v1/missions/{id}/run              # operator：运行三角色小队
GET  /v1/missions/{id}/run              # 跟随其实时事件流
POST /v1/missions/{id}/run/cancel
GET  /v1/decisions?task=…               # 审计轨迹
```

## 门禁本身（供运行时集成方）

```
POST /v1/hook/decide         Authorization: Bearer <hook token>
{ "sessionId": "…", "toolName": "Bash", "toolInput": {…}, "cwd": "…" }
```

供已安装的 Claude Code Hook 使用。身份来自会话绑定，绝不采信请求体。运行时
侧的退出码语义（经实测，而非假设）：`ALLOW` → 退出 0；其余一律退出 2 并在
stderr 说明原因；无法获得决策的任何情况 → 退出 2（失败即拒绝）。

## 参考 Host

`cmd/qiyanhost` 是一份完整、最小的集成示例（约 300 行，仅使用公开 API）：
注册、每个操作前先询问、遵守决策、等待审批、如实回传——并提供 `--rogue`
模式跳过控制调用，让你亲眼看到系统**不会**把这类操作标记为受保护。

## 错误结构

```json
{ "error": { "class": "OUT_OF_SCOPE", "message": "…" } }
```

Class 是稳定契约（`OUT_OF_SCOPE`、`APPROVAL_REQUIRED`、`AUTHORIZATION_BINDING`、
`AUTHORIZATION_REPLAYED`、`UNKNOWN_IDENTITY`、`BUDGET_EXHAUSTED`、
`CONTROL_UNAVAILABLE`、`STORE_CORRUPTED` 等）。请据此编程，不要解析 message 文本。
