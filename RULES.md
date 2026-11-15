# turborel-go rules

How turborel-go is designed, written and verified. These rules follow
Turborel's conventions, adapted to Go and a client rather than an engine.
The decision numbers below belong to this repository.

## 1. Decisions

| # | Decision |
|---|---|
| D1 | One database/sql driver over Turborel's native protocol version 1 |
| D2 | Preserve exact values; prepare and bind parameters on the server |
| D3 | One response per connection, with database/sql owning the pool |
| D4 | Context cancellation discards the connection; ambiguous writes are never retried |
| D5 | Verified TLS and SCRAM by default; explicit trust for local development |
| D6 | Standard-library implementation and Go verification tools |
| D7 | Pin an upstream revision and test against the actual server |
| D8 | Usage in the README, API details in Go documentation |

### D1. Native protocol

Use the registered name `turborel`. A listener serves one database, so a
DSN does not contain a database path. The protocol is length-prefixed,
little-endian and independent of PostgreSQL or MySQL. `wire.go` bounds
frames to 16 MiB or a smaller configured maximum. The server's codec at
the revision in VERSIONING.md is the reference, including fields omitted
from prose summaries.

### D2. Values and parameters

Exact numbers become strings, never float64. Decimal and DecFloat provide
explicit parameter and scan types. Array, Row, Interval and Extension
provide parameter values; composites return JSON text and extensions
return their display text. Prepared parameter metadata directs datetime
and composite encoding. Never substitute values into SQL.

Protocol version 1 does not carry affected-row counts or generated keys.
Those accessors return ErrUnsupported, never a fabricated count. Datetimes
retain microseconds and fixed offsets; sub-microsecond input is truncated.

### D3. Sessions and transactions

The pool serializes each connection. A response retains its operation
context until completion. Execute requests all rows (`rows=0`); the server
still emits bounded batches. This avoids paginated execution retaining an
autocommit transaction until a separate cursor close. Early Rows.Close
drains within five seconds or discards the connection. Socket close rolls
back, whereas the protocol's Terminate can commit under a session setting.
ResetSession rolls back leftover transactions; it does not reset session
settings or temporary objects. Use sql.Conn when session state matters.

### D4. Cancellation and uncertain outcomes

Context cancellation unblocks socket I/O, sends Cancel on a separate
connection using the same transport, then closes the original session.
Cancellation delivery is best effort and bounded to one second. SQL
errors preserve their fields. ErrBadConn is returned only before a new
request can have been sent, or while resetting a session for reuse.
A failure after sending a write is returned to its caller without retry.

### D5. Authentication

TCP defaults to verified TLS, at least TLS 1.2. SCRAM uses standard-library
cryptography, verifies the server proof and binds to the TLS channel.
TLS 1.3 uses tls-exporter; supported TLS 1.2 certificates use
tls-server-end-point. Password authentication requires TLS or a Unix
socket. Plaintext SCRAM needs an explicit opt-in. InsecureSkipVerify is
refused. Config formatting and DSN parse errors never print passwords.
SCRAM derivation is capped at 2,000,000 iterations to bound hostile peers.

### D6. Tools and dependencies

Runtime and tests use the Go standard library. Automation is Go in
cmd/verify, not shell scripts. Go idioms such as early-return error checks,
bounded indexing and checked narrowing conversions replace Rust-specific
lint requirements. gofmt is authoritative. External development tools are
golangci-lint 2.14.0 and govulncheck 1.9.0; they are not module dependencies.
The Go toolchain is pinned in go.mod and CI. No dependency is added without
updating this decision and explaining its need.

### D7. Compatibility tests

Unit tests cover framing, malformed data, authentication proofs, exact
values and transport failures. Integration tests start a fresh loopback
server for each fixture and exercise database/sql end to end. The server
revision is pinned in VERSIONING.md and CI; changing it requires rerunning
the suite and reviewing wire changes.

### D8. Client documentation

The README covers installation and everyday usage. Package and exported-item
comments describe API behavior alongside the implementation and render as Go
documentation. This keeps the reference together for a single-package client.
Runnable examples live in examples/ and are checked with the code.

## 2. Project layout

One public package at the module root; the verifier is in cmd/verify.
Tests live beside the implementation. Usage lives in README.md, package
documentation in doc.go, and runnable examples in examples/. Keep protocol
details private.

## 3. Code rules

- R-SAFE-1: no unsafe or unchecked assumptions about peer-controlled bytes.
- R-SAFE-4: no runtime dependency without a recorded decision.
- R-ERR-1: no production panic, recover or process exit in the library.
  Commands may exit after reporting their error.
- R-ERR-2: preserve server SQLCODE/SQLSTATE; wrap local causes with %w.
  Local validation errors begin with `[DESIGN]` after the package prefix.
- R-NUM-1: bound lengths and arithmetic before allocating or narrowing.
  Signed-to-unsigned wire conversions preserve explicitly defined bit patterns.
- R-CF-1: keep conditions to at most four terms; avoid hidden side effects.
- R-SIZE-1: functions have at most 50 nonblank, noncomment code lines,
  excluding standalone braces.
- R-SIZE-2: files have at most 600 lines. R-SIZE-3: functions have at most
  five parameters, excluding the receiver.
- R-CONST: name shared protocol limits and message tags.
- R-STYLE-1: gofmt. R-STYLE-7: the library does not print or log.
- R-DOC-1: public items have Go documentation, including error behavior.
- R-DOC-2: comments explain why. R-DOC-5: no TODO comments, commented-out
  code or code citations of the open-work document.
- R-DOC-6: files begin with a purpose comment (after a build tag if needed).
  Separate file-purpose comments from the package clause; doc.go owns the
  package documentation.
- R-DOC-8: user-visible changes update README.md and Go documentation
  where their claims change.

## 4. Test rules

- R-TEST-1: test at the lowest level that shows the behavior.
- R-TEST-2: cover normal, boundary and malformed-input cases.
- R-TEST-5: fixed inputs, explicit synchronization and loopback networking
  only. Deadlines contain hangs; no sleep determines a test's outcome.
  TLS uses fixed keys; authentication generates nonces in production code.
- R-TEST-7: every fixed defect gets a regression test.
- R-TEST-10: shared fixtures live in a common test file.
- R-TRACE-1: each Test and Fuzz function documents `Verifies: [DESIGN] D…`
  or `Verifies: [TOOL] R-…`. Examples compile; examples with Output run.

## 5. Coverage

R-COV-2: the driver statement coverage floor is 70%, checked by
cmd/verify/coverage.go, and may only rise. Tool coverage is reported
separately and excluded from that driver floor. This is statement coverage;
no MC/DC or certification claim is made.

## 6. Deviations

Record a rule deviation here, with scope and reason. Command programs may
print and exit to report their outcome (R-ERR-1, R-STYLE-7). Generated wire
bit patterns use signed conversions only after their width is established
(R-NUM-1). No other deviations are recorded.

## 7. Process and tools

R-PROC-1: every change passes `go run ./cmd/verify -coverage`, with
TURBORELD configured, before handoff. The verifier checks formatting,
source rules, vet, linters, race tests and coverage. CI additionally runs
govulncheck, checks module tidiness and runs on Linux, macOS and Windows.

Update RULES.md, CONFORMANCE.md, TODO.md, README.md and Go documentation
when their claims change.
Do not claim checks that were not run. Commit only on request, never push
without approval, and add no agent attribution. See CONTRIBUTING.md.
