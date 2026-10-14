# Scope and claim boundary

This document states exactly what this repository does and does not do today. It
exists so that no reader, and no automated system, can mistake an intention for
an implemented capability. If the code and this document ever disagree, the code
is wrong and this document is the specification.

## What is implemented

| Capability | Detail |
| --- | --- |
| `qiyan help` | Prints usage text to stdout, exits 0 |
| `qiyan version` | Prints the version and build identity, exits 0 |
| Unknown command | Prints an error to stderr, prints usage, exits 2 |
| Build | Standard-library-only Go module, `go build ./cmd/qiyan` |
| Tests | Unit tests over the CLI's behaviour, `go test ./...` |
| CI | Go build, vet, gofmt and test on Linux, Windows and macOS |
| Release drill | A build-provenance-attested test artifact, produced on demand |

That is the complete list. There is no other behaviour.

## What is not implemented

The following are **goals**, and none of them exist in this repository:

- scanning, probing or discovery of any kind;
- an evidence model, or any collection or storage of evidence;
- blast-radius mapping or graph construction;
- any notion of an "action surface" derived from a real system;
- any agent integration, plugin, adapter or extension point;
- any output format, report, or machine-readable result;
- any network access at all — the program opens no sockets;
- any file-system traversal beyond reading its own arguments.

`qiyan` currently accepts two verbs and prints text. It performs no I/O other
than writing to stdout and stderr.

## Claims this project does not make

- It does **not** claim to find, detect, prevent or assess vulnerabilities.
- It does **not** claim any security, privacy or compliance property, because it
  has none to offer yet.
- It does **not** claim compatibility with any other tool, format or standard.
- It does **not** claim the project is production-ready, stable, or suitable for
  any purpose. Nothing here is stable. No version has been released.
- It does **not** claim its CI being green means its output is trustworthy.
  There is no output to trust yet.

## Supply-chain statements, stated precisely

- **SHA-256 digests** in the release drill are integrity checksums. They are
  **not** a digital signature and provide no authenticity on their own. CI
  itself produces no digests; it does not publish an artifact to digest.
- **GitHub artifact attestations** are a build-provenance record, signed by the
  GitHub Actions identity of the workflow run. They are **not** an enterprise or
  legally binding signature, and they attest to how an artifact was built, not
  to what it is safe to run.
- The build is reproducible in the sense that it uses only the Go standard
  library and no downloaded modules; no stronger reproducibility claim is made.

## If you need something not listed above

It is not here yet. Open an issue describing the need, or see
[CONTRIBUTING.md](../CONTRIBUTING.md).

---

[中文版](scope-and-claims.zh-CN.md)
