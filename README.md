<p align="center">
  <a href="https://turborel.com/">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="artwork/turborel-logo-white.svg">
      <img src="artwork/turborel-logo.svg" alt="turborel" width="360">
    </picture>
  </a>
</p>

> turborel-go is in alpha. The Go API and Turborel's native protocol may
> change between builds. See [VERSIONING.md](VERSIONING.md).

A Go `database/sql` driver for [Turborel](https://turborel.com/), written
with the standard library. It connects directly to `turboreld` over TCP
or a Unix socket. This is an independent client for the Turborel project.

## What it has

- Queries, prepared statements and positional parameters, sent separately
  from SQL text.
- Transactions, isolation levels, connection pooling through `database/sql`,
  and streamed result batches.
- Verified TLS, SCRAM-SHA-256 with channel binding, password and client
  certificate authentication. Local trust is available when the server
  explicitly enables it.
- Context cancellation using the server's cancellation key, bounded cleanup,
  and no automatic retry of a write whose outcome is unknown.
- Exact decimal values, decimal floating point, binary values, datetimes,
  arrays, rows and intervals. SQLCODE and SQLSTATE remain available as
  typed errors.

## Quick start

Requires Go 1.27 or later; use the current patch release.

```sh
go get github.com/bschaatsbergen/turborel-go
```

```go
connector, err := turborel.OpenConnector(turborel.Config{
    Address:  "localhost:7422",
    User:     "ANN",
    Password: os.Getenv("TURBOREL_PASSWORD"),
})
if err != nil {
    return err
}
db := sql.OpenDB(connector)
defer db.Close()

var answer int
if err := db.QueryRowContext(ctx, "VALUES (CAST(? AS INTEGER))", 42).Scan(&answer); err != nil {
    return err
}
fmt.Println(answer)
```

```text
42
```

Import `github.com/bschaatsbergen/turborel-go` as `turborel` alongside
`database/sql`. TCP uses TLS and SCRAM by default. Supply `Config.TLS` for
a private certificate authority or a client certificate.

For URL configuration, import the driver and use
`sql.Open("turborel", "turborel://ANN:password@localhost:7422")`. Keep
credentials out of source code and construct URLs with `net/url` when
passwords contain reserved characters. `sql.Open` does not connect;
`PingContext` checks connectivity.

The [runnable example](examples/query/main.go) reads `TURBOREL_DSN`.
API details live in the [Go package documentation](doc.go) and the comments on
exported types. Read them locally with `go doc` or `go doc Config`.
For SQL syntax and server configuration, see [Turborel](https://turborel.com/).

## Connecting

[Config](config.go) describes the connection options and their defaults.
`database/sql` manages the pool; use `SetMaxOpenConns`, `SetMaxIdleConns`
and `SetConnMaxLifetime` to configure it.

URL options are `auth`, `socket`, `tls` (`require` or `disable`),
`connect_timeout`, `read_timeout`, and `allow_insecure_scram`. Durations
use Go syntax such as `5s`. Unknown or duplicate options fail. A listener
serves one database, so the URL has no database path.

Authentication can be `scram` (the default), `password`, `certificate`, or
`trust`. Password authentication requires TLS or a Unix socket. Certificate
authentication needs a client certificate accepted by the server. TLS
certificate verification cannot be disabled.

For a local development server explicitly configured to trust local clients:

```text
turborel://ANN@127.0.0.1:7422?auth=trust&tls=disable
```

For Unix sockets, set `Config.Network` to `unix` and `Config.Address` to
the socket path, or set the URL's `socket` option. Windows clients use TCP;
named pipes are not implemented.

## Queries and values

Use positional `?` parameters, as in the quick start. The server must be
able to infer each parameter's type; a `CAST` makes it explicit. Named
parameters are refused. For repeated statements, use `PrepareContext`
and close the statement when finished. Close query rows promptly and check
`Rows.Err` after iteration.

| SQL value | Result value | Parameter |
|---|---|---|
| NULL | nil | nil or sql.Null types |
| CHARACTER, CHARACTER VARYING | string | string |
| BINARY, BINARY VARYING | []byte | []byte |
| Exact number, including INTEGER | string | integer or turborel.Decimal |
| FLOAT | float64 | finite float64 |
| DECFLOAT | decimal exponent string | turborel.DecFloat |
| BOOLEAN | bool | bool |
| DATE, TIME, TIMESTAMP | time.Time | time.Time, guided by prepared type |
| INTERVAL | JSON object | turborel.Interval |
| ARRAY | JSON array | turborel.Array |
| ROW | JSON array in field order | turborel.Row |
| Plug-in type | display string | turborel.Extension |

Scan exact integers into Go integers when they fit, or use `string` or
`turborel.Decimal` to preserve every digit. Nested arrays and rows remain
nested JSON, with exact numbers represented as strings. A nil `[]byte`
is empty binary; use an untyped `nil` for SQL NULL. Datetimes retain
microsecond precision and fixed offsets; TIME values have a synthetic date,
so use their clock fields and offset.

`Rows.ColumnTypes` reports names, SQL types, nullability, declared lengths,
and exact numeric precision and scale. `RowsAffected` and `LastInsertId`
return errors matching `turborel.ErrUnsupported`: the server does not send
these values.

## Transactions and failures

Statements outside `sql.Tx` use autocommit. `BeginTx` supports the default
isolation level, READ UNCOMMITTED, READ COMMITTED, REPEATABLE READ and
SERIALIZABLE. READ UNCOMMITTED requires `ReadOnly`. Commit or roll back
every transaction; avoid mixing transaction-control SQL with `sql.Tx`.

Use `sql.Conn` for work that depends on session settings or temporary
objects. Pool reuse rolls back leftover transactions but preserves this
other session state, so restore settings explicitly. A new database user
may need `CREATE SCHEMA AUTHORIZATION ANN` before creating tables.

Contexts apply through row consumption. Cancellation discards the connection
and asks the server to stop the operation. Closing rows early drains the
response within five seconds or an earlier operation deadline. `ReadTimeout`
bounds the whole operation, including row consumption. Commit, rollback
and statement cleanup have a five-second deadline.

Use `errors.As` to inspect `*turborel.Error` for SQLCODE, SQLSTATE and the
other server diagnostics. Use `errors.Is` for context cancellation and
`turborel.ErrUnsupported`. A connection failure or cancellation can leave a
write's outcome uncertain; the driver never automatically retries such a
write. The application must resolve that uncertainty.

## Checking a change

```sh
TURBORELD=/path/to/turboreld go run ./cmd/verify -coverage
```

This checks source rules, formatting, static analysis, unit tests and live
server tests with race detection, then enforces the coverage floor.
[CONTRIBUTING.md](CONTRIBUTING.md) gives the pinned server revision and
how to build it. `go test ./...` runs the tests that need no server.

## Documents

- [Go package documentation](doc.go): connection, value and transaction behavior.
- [VERSIONING.md](VERSIONING.md): alpha, releases and protocol versions.
- [CONFORMANCE.md](CONFORMANCE.md): implemented driver and protocol behavior.
- [RULES.md](RULES.md): design decisions, code rules and verification.
- [TODO.md](TODO.md): open work.
- [CONTRIBUTING.md](CONTRIBUTING.md) and [SECURITY.md](SECURITY.md): contributing
  and reporting a vulnerability.
- [LICENSE](LICENSE): BSD 2-Clause. The linked Turborel logos are governed
  by the upstream [trademark policy](https://github.com/turborel/turborel/blob/main/TRADEMARK.md).
