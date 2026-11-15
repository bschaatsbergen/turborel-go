# Contributing to turborel-go

A client can lose data by retrying an uncertain write or by rounding a
value before sending it. Correct results and transaction boundaries come
first. The process follows Turborel's, adapted to Go.

## Before you start

Read RULES.md §3 (code), §4 (tests), §7 (process), and the decisions for
your area. Check CONFORMANCE.md and TODO.md. Discuss substantial features,
new dependencies and wire changes before implementing them. Maintainer
instructions in the active task establish the scope of an authorized change.

## Setting up

Install Go 1.27.4, Rust through rustup, golangci-lint 2.14.0 and
govulncheck 1.9.0. The latter two are development tools only.

```sh
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
git clone https://github.com/turborel/turborel.git .upstream
git -C .upstream checkout 3f25a033dad94814fbd4e7d871e48eb81b2734cc
```

Inside `.upstream`, run:

```sh
rustup toolchain install
cargo build --locked -p turboreld
```

Back in this repository, point TURBORELD at the absolute path to that
binary. On Unix:

```sh
export TURBORELD="$PWD/.upstream/target/debug/turboreld"
go run ./cmd/verify -coverage
govulncheck ./...
```

On Windows, use the absolute path to `turboreld.exe`. Integration fixtures
start their own in-memory servers on unused loopback ports. They need no
persistent database, external credentials or public network service.

`go test ./...` runs unit tests without a server. The verifier's
`-integration=false` option is useful while editing, but does not replace
the full handoff check. CI runs the full check on Linux, macOS and Windows.

## Making a change

Keep one logical change per commit and make every commit pass verification.
Add normal, boundary and malformed-input tests at the lowest useful level.
Each test documents what it verifies. A fixed defect gets a regression test.
Update RULES.md for decisions, CONFORMANCE.md for implemented behavior,
TODO.md for open work, and README.md and Go documentation for anything
users see.

## Changes that touch data

For values, show that NULLs, signed boundaries and decimal digits survive.
For transactions, test commit, rollback, failed statements and early row
closure. For connection failures, show that a write cannot be retried after
it might have reached the server. For framing, test truncation and excessive
lengths without panic or unbounded allocation. A wire change says what
older servers see and updates the pinned compatibility fixture when needed.

## Commit messages and pull requests

Use the Linux-style form:

```text
area: summary in the imperative, at most 75 characters

The problem and its impact, followed by the change and why it fixes it.
Mention compatibility consequences, tests and measured performance claims.
```

Base changes on main. Keep one logical change per commit, with explicit
paths staged. Never stage LICENSE, local servers or scratch files. Commit
only when asked and never push without approval. Add no agent attribution,
Co-Authored-By, Generated with, or Signed-off-by lines.

## Repository checks

Enable branch protection requiring the `verify` jobs and the `security`
job, and keep workflow permissions read-only. Enable private vulnerability
reporting. The workflows require no repository secrets or deployment
environments. Releases remain a maintainer action.
