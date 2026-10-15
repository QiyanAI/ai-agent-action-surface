# Qiyan Control API — 开发者指南

**协议版本：v1（契约修订版 3，2026-10）。** 接口是本地、HTTP/JSON、仅限回环地址的。
控制台、CLI、Claude Code Hook 与随仓库分发的参考 Host 使用的是同一套接口——
不存在私有通道。本文档即契约；代码与文档不一致时，以代码为准、文档为错。

v1 的兼容性策略：**只做加法。** 字段名、结果字符串、错误类与 URL 路径保持稳定；
可能出现新字段与新原因码（请容忍它们），既有内容永不改变含义。请求为**严格解码**
——任何未知 JSON 字段都会被拒绝并返回 `400 INVALID_REQUEST`，拼写错误绝不可能
被静默丢弃。请求体上限 1 MiB。没有协议协商握手；版本就是路径中的 `/v1`，
本文档标明修订版本。

## 从这里开始：第三方接入完整流程

在 `POST /v1/authorize` 能成功之前的全部前置环节。每一步都由操作者（持有状态
目录中 operator token 的人）完成：

```sh
qiyan serve                              # 1. 控制服务（打印地址与 token 文件路径）
qiyan agents add --name MyHost --role builder
                                         # 2. Agent 身份；agent token 只显示一次
qiyan mission create --goal "host demo" --root D:/scratch \
    --tool note.read --tool report.append --tool tmp.cleanup
                                         # 3. 任务 + 你的 Host 将使用的确切工具名
qiyan task create --mission mi_… --role builder --agent ag_… --title "host work"
                                         # 4. 任务被指派给你的 Agent 身份
qiyan session bind --task ta_… --agent ag_… --session myhost-1
                                         # 5. 绑定会话即启动任务（READY → RUNNING）

export AGENT_TOKEN=…                     # 第 2 步得到的 token
go run ./cmd/qiyanhost --addr 127.0.0.1:8760 --token $AGENT_TOKEN \
    --mission mi_… --task ta_… --workdir D:/scratch
                                         # 6. 询问 → 遵守 → 回传（参考 Host）
```

凭据以 0600 文件存放在状态目录（文件权限即本地用户信任锚）：`operator.token`
（本地用户操作）、`hook.token`（仅 `POST /v1/hook/decide` 通道）。Agent token 在
注册时只显示一次，之后仅存 SHA-256。**UI 会话 Cookie**（仅限控制台）在非 GET
请求上还必须携带 `X-Qiyan-CSRF` 头。

### 可信身份要求（上面第 4 步为什么重要）

`POST /v1/authorize` 的**全部**身份都由服务端推导；请求体中的任何内容都不被信任：

- Agent 来自 Bearer token；
- 任务来自你指定的已存任务（它的状态、它被指派的 Agent、它所属任务的授权范围）；
- **请求体中的 `missionId` 仅供参考**：服务端以已存任务所属的任务为准，忽略冲突值；
- 请求者必须是该任务自己的被指派 Agent，且在任务名册中；任务必须处于存活状态。
  否则：`DENY`，原因 `identity_unknown`；未知 token 则返回 `UNKNOWN_IDENTITY` 错误。
- 工具与 kind 必须位于任务授权范围内（第 3 步的 `--tool` 授权）；其余一律
  `tool_not_in_scope` / `unrecognized_command`。

## 唯一重要的问题：“我可以做这件事吗？”

```
POST /v1/authorize          Authorization: Bearer <agent token>
{
  "missionId": "mi_…",           // 仅供参考；以已存任务为准
  "taskId":    "ta_…",
  "action": {
    "kind": "file.modify",       // 必填；取值见下表
    "tool": "report.append",     // 必填、非空；必须在任务的工具清单中
    "path": "D:/work/report.md", // file.* 类：绝对路径、正斜杠、位于授权根目录内
    "command": "go build ./…",   // process.exec
    "cwd": "D:/work",            // process.exec：运行目录
    "url": "https://…",          // network.access
    "credentialRef": "DEPLOY_TOKEN", // credential.use：只给名称，绝不给值
    "contentDigest": "…",        // 可选：本次写入内容的 SHA-256
    "delegationTo": "ag_…",      // 仅 agent.delegation（见下）
    "delegationTask": "ta_…",
    "delegationKind": "file.modify",
    "raw": { }                   // 可选、不透明；供你自己的审计展示
  }
}
```

**各 kind 的最小字段**——其余均为可选：

| kind | 必填 | 说明 |
| --- | --- | --- |
| `file.read` / `file.create` / `file.modify` / `file.delete` | `kind`、`tool`、`path` | 路径必须绝对且已规范化（正斜杠、无 `..`）；包含关系按任务范围检查 |
| `process.exec` | `kind`、`tool`、`command` | 强烈建议给 `cwd`（引擎会据它推理）；**所有 Shell 类操作按设计一律需要人工批准**——不存在命令名白名单 |
| `network.access` | `kind`、`tool`、`url` | 走审批门禁 |
| `credential.use` | `kind`、`tool`、`credentialRef` | 只给名称、绝不给值；走审批门禁 |
| `external.state_change` | `kind`、`tool` | 走审批门禁 |
| `agent.delegation` | `kind`、`tool`、`delegationTo`、`delegationTask`、`delegationKind` | 仅限策划员→开发员、开发员→检查员、检查员→开发员三角；不允许传递式委派；只读接收方不得被委派修改类操作 |
| `session.local` | `kind`、`tool` | 仅限会话记录类工具；其余需要批准 |

`contentDigest` 语义，精确表述：**不带它时，批准覆盖的是目标路径而非内容**——
重试调用可以在同一路径写入不同的字节。当你的 Host 能计算它、且你希望批准连内容
一起绑定，请提供它。

### 决策（响应信封）

HTTP `200` + 决策——**`DENY`/`UNKNOWN` 也是 200**：传输成功了，答案在响应体里：

```json
{
  "outcome": "REQUIRE_APPROVAL",      // ALLOW | DENY | REQUIRE_APPROVAL | UNKNOWN
  "reasons": [ { "code": "sensitive_command", "params": { "program": "go" } } ],
  "actionDigest": "…",                 // 该确切操作的标识——请保存
  "approvalId": "ap_…",                // REQUIRE_APPROVAL 时存在
  "mode": "controlled_auto",
  "coverage": "OBSERVED"               // 仅 Hook 路径为 ENFORCED；见下文
}
```

| 结果 | 你的 Host 必须怎么做 |
| --- | --- |
| `ALLOW` | 执行那一个确切操作。 |
| `DENY` | 不要执行。`reasons` 说明原因——用你 UI 的语言按原因码渲染；绝不解析散文。 |
| `REQUIRE_APPROVAL` | **先不要执行。** 等待人类：轮询 `GET /v1/approvals/{id}`（被指派的 Agent 可读自己的），然后原样重新提交*完全相同*的操作。仍在等待时重复提交是安全的，会返回同一个批准（按 任务+Agent+操作摘要 去重）——这是唯一正确的重试循环。决定之后，原样重试恰好一次。 |
| `UNKNOWN` | 在任何强制路径上一律视为拒绝。它意味着 Qiyan 无法得出“允许”——绝不是需要“稍后重试”的临时状态。 |

**获得批准后的重试仍要经过全部结构检查**（任务状态、范围、预算、路径包含）。
批准不是旁路；它只解决一道闸门。

## 审批与授权绑定

```
GET  /v1/approvals?state=pending        # 操作者
GET  /v1/approvals/{id}                 # 操作者，或被指派的 Agent（仅自己的）
POST /v1/approvals/{id}/decision        # 仅操作者/本地用户——Agent 一律 401
{ "approve": true }
```

绑定条件，精确表述：

- 一次批准 → **至多一次执行**，且是**那个确切操作**，绑定于 任务 + 任务单 + Agent +
  操作摘要 + 范围摘要；
- **批准 TTL：15 分钟**（响应里有 `expiresAt` 可读）；
- 批准会铸造一次性**授权**（TTL 10 分钟），在重试调用时**原子领取**——并发的双重
  重试不可能执行两次；领取之后再重放会被拒绝（`AUTHORIZATION_REPLAYED`/
  `authorization_replayed`）；
- **被拒绝的操作在该任务内保持拒绝**（原因 `user_denied`）；**已取消**的批准永不
  可能再被批准（`approval_cancelled`）；
- **过期**的批准必须重新发起（经过一次全新的闸门评估）；
- 范围变化会使任何未使用的授权失效；
- **沉默不是同意**，也不存在“全部允许”端点。

## 如实回传发生了什么（证据 ↔ 结果）

```
POST /v1/events             Authorization: Bearer <agent token>
{ "taskId": "ta_…", "actionDigest": "…", "result": "executed|failed|blocked|cancelled", "note": "…" }
```

`actionDigest` 来自 authorize 响应。服务端强制：枚举合法、Agent 拥有该任务、
`note` 截断至 400 字符。

诚实的局限，写在这里因为它们属于契约：

- 目前**没有回读端点**——你无法 `GET` 回自己上报的内容；请自行留存副本；
- 服务端**不校验** `actionDigest` 是否对应某条已记录的决策——过期或不匹配的摘要
  会被接受。把 `actionDigest` 与你自己的决策记录配对，是你自己的责任；
- 决策本身完全可读：`GET /v1/decisions?task=…` 按决策返回 结果、结构化原因、
  操作负载、模式与覆盖——这就是可审计的轨迹。

## 表达“哪些没有被保护”

契约处处区分“观察到”与“被强制执行”：

- **每条决策的 `coverage`**：经过 Claude Code Hook 的决策标记为 `ENFORCED`
  （工具调用可证明在闸门处被拦下/放行）；经过 `POST /v1/authorize` 的决策标记为
  `OBSERVED`——**是否遵守由你的 Host 代码决定**，Qiyan 只观察到这次询问，无法
  强制你的服从；
- **`GET /v1/coverage`**（操作者/会话）：本机真实验证运行产生的行，每行带状态——
  `ENFORCED`（真实检查证明控制有效）、`OBSERVED`（较弱、仅为观察到的事实——例如
  `blocking-only` 探针档位）、`UNCOVERED`（实测未受保护——例如已卸载的接入）、
  `UNKNOWN`（未测量）；
- 这里有意**没有 `protected: true` 快捷值**：永远不要从安装记录、版本号或文档
  推导保护结论；
- 如果你的 Host 能不经过本 API 直接行动（`--rogue` 演示），那条路径**在 Qiyan 的
  强制执行之外**，产品对它不做任何宣称。请在你的接入点同样如实说明。

## 原因码（稳定标识）

原因是 `{ code, params }`——机器稳定，在边缘渲染。当前词表（声明 25 个；标注
*未产生* 的三个描述引擎目前不会产生的状态——若出现请按通用代码渲染）：

| 原因码 | 含义 |
| --- | --- |
| `path_outside_scope` | 目标路径在任务授权根目录之外 |
| `tool_not_in_scope` / `unrecognized_command` | 工具/kind 不在任务清单中，或无法解释 |
| `write_by_readonly` | 只读角色尝试修改 |
| `delete_operation` | 删除一律需要批准 |
| `sensitive_command` | 所有 Shell 类命令一律需要批准（`{program}`） |
| `credential_reference` / `external_network` / `external_state_change` | 审批门禁类别 |
| `mode_companion` | 陪同模式：修改等待确认 |
| `budget_exhausted`（`{what}`：steps/runtime） | 任务预算耗尽 |
| `task_cancelled` | 任务已被取消 |
| `identity_unknown`（`{what}`） | 身份/状态无法确认 |
| `delegation_escalation`（`{what}`） | 委派会扩大权限 |
| `user_approved`（`{approval}`）/ `user_denied` | 人类对重试操作的决定 |
| `approval_expired` / `approval_cancelled` / `action_changed` | 授权对本次尝试不再有效 |
| `authorization_replayed` | 一次性授权已被用掉 |
| `control_unavailable`（`{stage}`） | 审计即拒绝：无法记录的决策就是拒绝 |
| `no_external_effect` | 仅会话内部记录的操作 |
| `mode_controlled_auto`、`mode_bounded_autonomy`、`path_not_covered` *未产生* | 已声明词表，目前不会产生 |

## 闸门本身（面向运行时集成）

```
POST /v1/hook/decide         Authorization: Bearer <hook token>
{ "sessionId": "…", "toolName": "Bash", "toolInput": {…}, "cwd": "…" }
```

供已安装的 Claude Code Hook 使用。身份来自会话绑定、绝不来自请求体；未绑定的
会话会被拒绝。运行时侧的退出码语义（经验证、非假设）：`ALLOW` → exit 0；其余
一律 → exit 2 并在 stderr 说明；任何无法取得决策的情况 → exit 2（失败即拒绝）。

## 任务、子任务与会话

```
POST /v1/missions                       # 操作者：目标 + 模式 + 显式范围
                                        #   scope = roots、allowedKinds、allowedTools、预算
GET  /v1/missions / GET /v1/missions/{id}
POST /v1/missions/{id}/tasks            # 操作者
POST /v1/sessions                       # 操作者：把运行时会话绑定到任务（即启动该任务）
POST /v1/missions/{id}/run              # 操作者：运行三角色小队
GET  /v1/missions/{id}/run              # 实时事件轨迹（内存态；重启即丢失——
                                        #   可审计的决策轨迹保存在 GET /v1/decisions）
POST /v1/missions/{id}/run/cancel       # 停止运行；状态变为 "cancelled"
GET  /v1/decisions?task=…               # 可审计的轨迹
GET  /v1/coverage, GET /v1/health       # 强制执行事实 / 存活（无鉴权、无数据）
```

## 参考 Host

`cmd/qiyanhost` 是一个完整而最小的集成（约 300 行，**仅标准库、零内部导入**）：
在每个操作前询问 `/v1/authorize`，遵守 `ALLOW`/`DENY`，遇到 `REQUIRE_APPROVAL`
时轮询自己的批准并原样重试（因去重而是安全的），把每个结果回传到 `/v1/events`
——并以 `--rogue` 模式做一次**不**经过控制调用的写入，向你展示未覆盖路径永远
不会被标记为受保护。token 由操作者签发（`qiyan agents add`）；Host 从不自行注册。

## 错误形状、错误类与 HTTP 映射

```json
{ "error": { "class": "OUT_OF_SCOPE", "message": "…" } }
```

错误类稳定；可以新增，既有字符串永不改变。在你的 UI 中按其映射；绝不解析
`message`。HTTP 状态：`400` `INVALID_REQUEST`（含未知字段、坏 JSON、校验失败）·
`401` `UNKNOWN_IDENTITY`（凭据缺失/无效）· `403` `IDENTITY_CONFLICT`、
`AUTHORIZATION_BINDING`、`AUTHORIZATION_REPLAYED`、`OUT_OF_SCOPE`（回环主机/CSRF
拒绝同样使用该类）· `404` `NOT_FOUND`、`APPROVAL_NOT_FOUND` · `409` `CONFLICT`、
`APPROVAL_EXPIRED`、`APPROVAL_CANCELLED` · `503` `STORE_CORRUPTED`、
`CONTROL_UNAVAILABLE` · `500` `INTERNAL`（另有 `BUDGET_EXHAUSTED`、
`TASK_CANCELLED`、`APPROVAL_REQUIRED` 视情况通过决策结果呈现）。

## 本契约不覆盖的内容

面向本地控制台的项目选择端点（`/v1/fs/*`）、连接向导（`/v1/connect*`）、安全
实验室（`/v1/lab/*`）与合成演习场（`/v1/playground/scenarios`）是本地控制台
自用的接口，**不属于**第三方集成面：它们可以不经过契约修订而变化。受支持的
集成面，恰为本文档描述的范围。
