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
