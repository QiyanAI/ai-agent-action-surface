# qiyan — 基于证据的 AI Agent 行动面与爆炸半径图

> **早期工程脚手架 —— 目前尚不具备任何实际功能。**
>
> 本仓库处于起步阶段。它只包含一个 Go 模块、一个仅实现 `help` 和 `version`
> 的命令行入口，以及在公开环境中安全开发所需的 CI、安全与发布基础设施。
> 它目前不会扫描、分析、观测或测绘任何对象。
> 已实现与未实现的准确边界记录在
> [docs/scope-and-claims.zh-CN.md](docs/scope-and-claims.zh-CN.md)。

## 长期方向

长期目标是构建面向 AI Agent 的**基于证据的行动面**：让 Agent 能够执行的动作、
以及这些动作的爆炸半径变得可见、可审计，并且每一条结论都可追溯到证据。

该方向是**目标，而非对当前行为的描述**。本仓库未实现其中任何部分，
本说明也不作任何相反的宣称。

## 当前状态

| 领域 | 状态 |
| --- | --- |
| 命令行 | 仅 `qiyan help` 与 `qiyan version` |
| 扫描 / 分析引擎 | 未实现 |
| 证据模型 | 未实现 |
| 爆炸半径测绘 | 未实现 |
| 已发布版本 | **无** —— 本项目尚无任何版本发布 |
| 公开 API / 兼容性承诺 | 无 —— 一切尚未稳定 |

## 构建与运行

需要 Go 1.27 或更高版本。

```sh
git clone https://github.com/QiyanAI/ai-agent-action-surface.git
cd ai-agent-action-surface
go build ./cmd/qiyan
./qiyan help
./qiyan version
```

检查工作区：

```sh
go vet ./...
go test ./...
```

构建仅依赖标准库。本模块没有任何第三方依赖，
因此只需 Go 工具链即可编译，无需下载任何模块。

## 安全

如发现疑似漏洞，请通过私下渠道报告，而不要提交公开 Issue。
报告渠道与处理预期见 [SECURITY.md](SECURITY.md)。

## 贡献

见 [CONTRIBUTING.md](CONTRIBUTING.md)。欢迎贡献，但请注意本项目当前处于
工程基础阶段的单维护者模式：变更需经 Pull Request 合入，并通过 CI 检查。

## 许可证

Apache License 2.0 —— 见 [LICENSE](LICENSE)。

Copyright 2026 QiyanAI。

---

[English README](README.md)
