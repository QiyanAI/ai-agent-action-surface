# Security Policy

## Reporting a vulnerability

Report suspected vulnerabilities through GitHub's private vulnerability
reporting for this repository:

**<https://github.com/QiyanAI/ai-agent-action-surface/security/advisories/new>**

You can also reach it from the repository's **Security** tab, under
**Report a vulnerability**.

That channel is private between you and the maintainers. **Please do not open a
public issue for a suspected vulnerability.**

If you are unsure whether something is a vulnerability, report it privately
anyway. Reports that turn out not to be vulnerabilities are still welcome.

## 安全漏洞报告（中文）

通过本仓库的 GitHub 私密漏洞报告渠道提交：

**<https://github.com/QiyanAI/ai-agent-action-surface/security/advisories/new>**

也可以从仓库的 **Security** 选项卡进入 **Report a vulnerability**。

该渠道仅在您与维护者之间可见。**请不要为疑似漏洞开公开 Issue。**
不确定是否算漏洞时，仍请私下报告；最终不是漏洞的报告同样欢迎。

我们会（通常几天内）确认收到报告，并评估是否构成漏洞及其严重程度；修复并
披露后会在公告中致谢——除非您要求匿名。请不要测试您不拥有、或未获授权测试的
基础设施；报告中不要包含真实凭证、个人数据或第三方数据。

这是一个单维护者项目，没有付费安全团队：确认与修复是尽力而为的，
**没有漏洞赏金**。

## What is in scope

The attack surface today includes, specifically:

- the local control service and its API (`qiyan serve`, `internal/api`),
  including the console's session/CSRF model, the hook decision endpoint and
  the local folder-listing endpoints used by the project picker;
- the PreToolUse hook (`qiyan hook pretooluse`) and the decision engine
  (`internal/control`) — a bypass of an enforced decision is a vulnerability;
- the Claude Code connector (`internal/connector`), including the settings
  install/undo path and the probe;
- the Safety Lab's scan and apply paths (`internal/lab`) — file writes and any
  way to make it execute something it should not;
- the CLI's argument handling in `cmd/qiyan`;
- the GitHub Actions workflows in `.github/workflows/`, including the release
  drill, the candidate build and their build-provenance attestations;
- the repository's own supply chain — the Go module and the pinned GitHub
  Actions used by CI.

Findings in any of those are in scope. Findings about capabilities this project
does not yet have are not — see
[docs/scope-and-claims.md](docs/scope-and-claims.md) for the boundary.

## What to expect

- **Acknowledgement** of your report, normally within a few days.
- An assessment of whether it is a vulnerability and how severe it is.
- Credit in the advisory once a fix is available and disclosed, unless you ask
  to remain anonymous.

This is a single-maintainer project without a paid security team. Response
times are best-effort, and there is **no bug bounty** — reports are handled on
goodwill in both directions.

## Please do not

- Open a public issue, pull request or discussion describing an unpatched
  vulnerability.
- Test against infrastructure you do not own or have permission to test.
- Include real credentials, personal data or third-party data in a report.

## Supported versions

No stable, production-certified release exists, and none of this is a support
commitment. The project has one public version: **v0.3.0-beta.1**, a Public
Beta listed on
[Releases](https://github.com/QiyanAI/ai-agent-action-surface/releases) once
it is published.

Early-access builds are signalled `EARLY ACCESS / BETA /
NOT PRODUCTION-CERTIFIED` (that Beta, and any later early-access build
published there) and `EARLY ACCESS / TEST BUILD / NOT PRODUCTION-CERTIFIED`
(test candidates — assembled for invited testers, never published as a product
version). None of them is production certified, none carries a support or
compatibility promise, and there is no supported-version patch policy yet.
Reports are nonetheless welcome — the feedback is most useful at this stage.
