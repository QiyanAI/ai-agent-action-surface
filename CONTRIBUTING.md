# Contributing

Thanks for your interest. This project is in its engineering-foundation phase
and is maintained by one person, so please read this before opening a large
change.

## Before you start

For anything larger than a small fix, **open an issue first** and describe what
you intend to change. The project's scope and its non-goals are deliberately
narrow right now (see [docs/scope-and-claims.md](docs/scope-and-claims.md)); a
change that expands the product surface will be declined even if it is well
built.

Issues are the coordination point, and some carry labels that say what they
are for: `good first issue` marks small, well-scoped tasks kept open for new
contributors; `help wanted` marks work the maintainer would like a hand with;
`documentation` marks docs-only work. If you want to start small, look there —
and say in the issue that you are taking it.

## The ground rules

1. **Pull requests only.** `main` is protected: changes arrive through a pull
   request and must pass CI. Direct pushes to `main` are rejected.
2. **Keep it standard-library only.** The module has no third-party
   dependencies and is intended to keep none for now. A change that adds one
   needs a discussion first.
3. **Keep it small.** One concern per pull request.
4. **No secrets, ever.** Do not commit credentials, tokens, keys, personal data
   or third-party data — not even in tests or examples. Use synthetic values.
5. **Keep the docs honest.** If a change alters what the project can do, update
   `README.md`, `README.zh-CN.md` and the claim boundary together. Do not
   describe a capability that is not implemented.

## Local checks

Run these before you push:

```sh
gofmt -l .        # must print nothing
go vet ./...
go test ./...
go build ./cmd/qiyan
```

CI runs equivalents of all four and then goes further: it builds with
`-trimpath`, runs the built binary as a smoke test, scans for known
vulnerabilities with a pinned `govulncheck`, verifies the module cache with
`go mod verify`, and fails if the module acquires any third-party dependency.

## Commit messages

Single-line subject, imperative mood, scoped by area:

```
feat(cli): add version output
fix(ci): pin checkout action to a commit SHA
docs(readme): clarify the claim boundary
```

Keep the body empty. Do not add attribution trailers — the project keeps its
history to the subject line alone.

## Licence

By contributing, you agree that your contribution is licensed under the Apache
License 2.0, as set out in [LICENSE](LICENSE).

## Security

Do not report vulnerabilities through issues or pull requests. See
[SECURITY.md](SECURITY.md) for the private channel.

## 贡献须知（中文）

- 只接受 **Pull Request**：`main` 受保护，直推会被拒绝，且必须通过 CI。
- **仅限标准库**：本项目没有第三方依赖，也不打算引入；如需新增依赖，先开 Issue 讨论。
- **保持改动小**：一个 PR 只做一件事。
- **绝不提交秘密**：凭证、令牌、密钥、个人数据、第三方数据都不要提交（测试与示例也不行），一律使用合成数据。
- **文档必须诚实**：改动影响产品能力时，同步更新 `README.md`、`README.zh-CN.md` 与声明边界；不得描述未实现的能力。
- **提交信息**：单行主题、祈使式（如 `fix(cli): …`），正文留空，不加任何署名尾注。
- **许可**：提交贡献即表示你同意其按 [Apache License 2.0](LICENSE) 授权。
- **新手标签**：`good first issue`、`help wanted`、`documentation` 标记适合新人
  入门的小任务——想从小处开始时从这里挑，并在 Issue 中说明你要认领。
- 漏洞请勿通过 Issue 或 PR 报告——请走 [SECURITY.md](SECURITY.md) 中的私密渠道。
