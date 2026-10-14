# qiyan — Evidence-Backed AI Agent Action Surface & Blast Radius Map

> **Early engineering scaffold — nothing here works yet.**
>
> This repository is at its first step. It holds a Go module, a command-line
> entry point that implements only `help` and `version`, and the CI, security and
> release plumbing needed to develop safely in the open. It does not scan,
> analyse, observe or map anything today. The exact boundary of what is and is
> not implemented is written down in [docs/scope-and-claims.md](docs/scope-and-claims.md).

## Intended direction

The long-term goal is an *evidence-backed action surface* for AI agents: a tool
that makes the actions an agent can take — and the blast radius of those
actions — visible and auditable, with every claim traceable to evidence.

That direction is a **goal, not a description of current behaviour.** No part of
it is implemented in this repository, and this README does not claim otherwise.

## Current status

| Area | State |
| --- | --- |
| CLI | `qiyan help` and `qiyan version` only |
| Scan / analysis engine | not implemented |
| Evidence model | not implemented |
| Blast-radius mapping | not implemented |
| Published release | **none** — no version of this project has been released |
| Public API / compatibility promise | none — nothing is stable yet |

## Build and run

Requires Go 1.27 or later.

```sh
git clone https://github.com/QiyanAI/ai-agent-action-surface.git
cd ai-agent-action-surface
go build ./cmd/qiyan
./qiyan help
./qiyan version
```

Check the working tree:

```sh
go vet ./...
go test ./...
```

The build is standard-library only. The module has no third-party dependencies,
so it compiles with the Go toolchain alone and no module downloads.

## Security

Please report suspected vulnerabilities privately rather than in a public issue.
See [SECURITY.md](SECURITY.md) for the reporting channel and what to expect.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Contributions are welcome, but note that
the project is a single-maintainer effort in its engineering-foundation phase;
changes are expected to arrive by pull request and to pass CI.

## Licence

Apache License 2.0 — see [LICENSE](LICENSE).

Copyright 2026 QiyanAI.

---

[中文说明](README.zh-CN.md)
