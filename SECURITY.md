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

No version of this project has been released as a product. R03 early-access
candidates are test builds only (`EARLY ACCESS / TEST BUILD /
NOT PRODUCTION-CERTIFIED`): there is nothing to patch on a supported-version
basis yet. Reports are nonetheless welcome — the feedback is most useful at
this stage.
