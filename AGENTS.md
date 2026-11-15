# Guide for coding agents

How to work on turborel-go as a coding agent, with any tool. If this file
disagrees with [RULES.md](RULES.md) or [CONTRIBUTING.md](CONTRIBUTING.md),
follow those.

turborel-go is a Go database/sql driver for Turborel. It speaks the native
wire protocol directly, with no third-party runtime dependencies.

## Before you change anything

Read RULES.md §3 (code), §4 (tests), §7 (process) and the §1 decisions for
your area. Read CONTRIBUTING.md for the process and checks for changes
that touch data. Inspect the pinned server's codec before changing wire
behavior; the server is the compatibility reference.

## Build and check

Every change must pass `go run ./cmd/verify -coverage` before handoff.
Set TURBORELD to the server binary built as CONTRIBUTING.md describes.

```
go run ./cmd/verify -coverage    # every check, including live server tests
go run ./cmd/verify              # the same, without coverage collection
go test ./...                   # tests that need no server
go test -tags=integration ./...  # tests against TURBORELD
golangci-lint run ./...          # static analysis
govulncheck ./...                # known vulnerabilities in reachable code
```

Automation is Go in `cmd/verify/`, never shell scripts (D6). Workflow
commands invoke tools directly. CI runs the checks on Linux, macOS and
Windows; say which platforms were actually tested locally.

## Where things are

| Path | What |
|---|---|
| `driver.go`, `config.go` | database/sql entry points and connection options |
| `transport.go`, `auth.go`, `scram.go` | connections, TLS, authentication, cancellation |
| `wire.go`, `metadata.go` | framing and descriptions |
| `value*.go`, `decfloat.go` | exact values and conversions |
| `conn.go`, `query.go`, `statement.go`, `rows.go`, `transaction.go` | sessions and driver interfaces |
| `*_test.go` | deterministic unit tests and build-tagged live server tests |
| `cmd/verify/` | the project's checks |
| `README.md`, `doc.go`, `examples/` | usage, Go package documentation and runnable examples |

## Rules that are broken most often

- No unsafe, production panics or dependency without a decision.
- No ErrBadConn after a request might have reached the server.
- No SQL interpolation for parameters; preserve exact numeric values.
- Bound every wire allocation, recursion and cleanup operation.
- Keep functions to 50 code lines and 5 parameters, files to 600 lines.
- Document public items and start files with their purpose.
- Comments say why; no commented-out code or TODO comments.
- Every test has a `// Verifies: [DESIGN] D…` or `[TOOL] R-…` paragraph.
- Tests use loopback only, fixed fixtures and explicit synchronization.
- A fixed defect gets a regression test. Coverage floors only rise.

## Documents

Update RULES.md for decisions, CONFORMANCE.md for behavior, TODO.md for
open work, and README.md and Go documentation for anything users see.
Run examples and record their actual output. A driver test cites a design decision; do not invent SQL
standard citations or claim the driver's tests prove server conformance.

## Commits

- Commit only when the maintainer asks; never push without approval.
- Use CONTRIBUTING.md's form, one logical change per commit.
- Add no AI or agent attribution, Co-Authored-By or Signed-off-by.
- Stage explicit paths: `git add -A -- <path> ...`. Never stage the whole tree.
- Never stage LICENSE, scratch files or a server checkout.
