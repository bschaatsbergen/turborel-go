<p align="center">
  <a href="https://turborel.com/">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="artwork/turborel-logo-white.svg">
      <img src="artwork/turborel-logo.svg" alt="turborel" width="360">
    </picture>
  </a>
</p>

An independent Go [`database/sql`](https://pkg.go.dev/database/sql) driver for
[Turborel](https://turborel.com/). Use it to connect your Go application to
a Turborel server, run SQL queries, and manage transactions. It uses only
the Go standard library.

> This client is in alpha. Its API and Turborel's protocol may change
> between builds.

## Install

Requires Go 1.27 or later; use the current patch release.

```sh
go get github.com/bschaatsbergen/turborel-go
```

## Usage

Set `TURBOREL_DSN` to your server's connection URL, for example
`turborel://ANN:password@localhost:7422`, and query it with `database/sql`.

This example assumes a `users` table with `id` and `name` columns and a
row for ID `42`.

```go
import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/bschaatsbergen/turborel-go"
)

func query(ctx context.Context) error {
	db, err := sql.Open("turborel", os.Getenv("TURBOREL_DSN"))
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	var name string
	err = db.QueryRowContext(ctx,
		"SELECT name FROM users WHERE id = ?", 42,
	).Scan(&name)
	if err != nil {
		return err
	}
	fmt.Println(name)
	return nil
}
```

TCP connections use TLS encryption and password authentication by default.
See [Config and ParseDSN](config.go) for
connection options, or [examples/query](examples/query/main.go) for a
complete program.

## Queries and values

Use `QueryRowContext` for one row, `QueryContext` for several rows, and
`ExecContext` for statements that return no rows. Use `?` for parameters;
named parameters are not supported. For repeated queries, prepare a
statement with `PrepareContext` and close it when finished.

Query results use the following Go values. `Scan` converts them into your
destination type when supported.

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

Use `string` or `turborel.Decimal` when you need to preserve every decimal
digit. For nested values, datetime precision and other conversions, see
the [Go package documentation](doc.go).

`RowsAffected` and `LastInsertId` return errors matching
`turborel.ErrUnsupported` because the server does not send row counts or
generated keys.

## Transactions and errors

Start a transaction with `db.BeginTx`, run its statements through the
returned `sql.Tx`, then call `Commit` or `Rollback`. Outside a transaction,
each successful statement commits automatically.

Close query rows when finished and check `Rows.Err` after iteration.
Pass a context to cancel a query or limit how long it can run. If you
change session settings or use temporary tables,
use `sql.Conn` to keep that work on the same connection.

Use `errors.As` to inspect `*turborel.Error` for the server's SQLCODE,
SQLSTATE and message. If the connection is lost during a write, the write
may already have succeeded. The driver does not retry it automatically;
your application must determine its outcome before trying again.

## Contributing

[CONTRIBUTING.md](CONTRIBUTING.md) explains how to build the tested server
revision and run the checks. Once `TURBORELD` points to that server binary:

```sh
go run ./cmd/verify -coverage
```

This runs formatting and source checks, static analysis, tests with race
detection, and the coverage check. `go test ./...` runs the tests that
need no server.

## Reference

- [Go package documentation](doc.go): connection options, values, transactions
  and cancellation. From this repository, `go doc` displays the package guide
  and `go doc Config` displays the connection settings.
- [Turborel](https://turborel.com/): SQL syntax and server configuration.
- [CONFORMANCE.md](CONFORMANCE.md): supported driver and protocol behavior.
- [RULES.md](RULES.md): design decisions and contribution rules.
- [TODO.md](TODO.md): open work.
- [SECURITY.md](SECURITY.md): reporting a vulnerability.
- [LICENSE](LICENSE): BSD 2-Clause. The linked Turborel logos are governed
  by the upstream [trademark policy](https://github.com/turborel/turborel/blob/main/TRADEMARK.md).
