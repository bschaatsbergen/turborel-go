// Package turborel connects database/sql to Turborel's native protocol.
//
// Importing the package registers the driver "turborel". This alpha client
// targets the server commit recorded in CONTRIBUTING.md.
//
// # Connections
//
// [OpenConnector] validates a [Config] without contacting the server. Pass the
// connector to database/sql.OpenDB and use PingContext to check connectivity.
// TCP defaults to verified TLS, at least TLS 1.2, and SCRAM authentication.
// Configure TLS roots or client certificates through Config.TLS. OpenConnector
// clones tls.Config; its referenced certificate pools and slices must not be
// mutated afterward. Certificate verification cannot be disabled.
//
// [ParseDSN] accepts turborel://USER:PASSWORD@HOST:PORT with the options auth,
// socket, tls, connect_timeout, read_timeout and allow_insecure_scram. Unknown
// or duplicate options fail. A listener serves one database, so URL paths are
// unsupported. Use net/url to encode credentials and keep them out of logs.
//
// SCRAM verifies the server proof and uses channel binding over TLS. TLS 1.3
// uses tls-exporter. TLS 1.2 uses tls-server-end-point and refuses certificate
// signature algorithms without an upstream-compatible hash. Password auth
// requires TLS or a Unix socket. Certificate auth requires a client certificate
// accepted by the server. Trust requires explicit server configuration.
// Windows clients use TCP; named pipes are not implemented.
//
// # Queries and values
//
// QueryContext, QueryRowContext and ExecContext accept positional ? parameters.
// Named parameters are refused. Parameters are prepared and bound on the server;
// a CAST can make an otherwise ambiguous parameter type explicit. PrepareContext
// reuses a statement for repeated executions. Close statements when finished,
// close rows promptly, and check Rows.Err after iteration. A connection permits
// one active response. Rows stream one protocol frame at a time.
//
// Exact numbers, including SQL integers, are returned as strings. Scan them into
// Go integers when they fit, or into string or [Decimal] to preserve every digit.
// Decimal supports signed 128-bit coefficients and scales up to 38. [DecFloat]
// preserves up to 38 coefficient digits and the server's decimal exponent range.
// Neither conversion passes through binary floating point.
//
// [Array] and [Row] parameters use the types returned by Prepare. Their results
// are JSON arrays, with row fields in declared order. Nested composites remain
// nested; exact numbers are strings and binary children use base64. [Interval]
// results are JSON objects containing Units and Qualifier. [Extension] parameters
// carry their type name, display text and bytes; results expose display text.
// A nil []byte parameter is empty binary; use nil for SQL NULL.
//
// Datetimes use time.Time, microsecond precision and fixed offsets. Input
// nanoseconds below a microsecond are truncated. DATE input uses its local
// calendar date. TIME results have a synthetic date around 0001-01-01; use their
// clock fields and offset. Unzoned types follow the server's session-zone rules.
//
// Rows.ColumnTypes reports names, SQL type names, nullability, declared lengths,
// and exact numeric precision and scale. Character lengths use their declared
// character or octet units. Exact-number type spellings share NUMERIC on the wire.
// RowsAffected and LastInsertId return errors matching [ErrUnsupported]: protocol
// version 1 does not carry affected-row counts or generated keys.
//
// # Transactions and sessions
//
// database/sql owns the pool. Configure it with sql.DB.SetMaxOpenConns,
// SetMaxIdleConns and SetConnMaxLifetime. The driver uses autocommit outside sql.Tx.
// BeginTx
// supports the default isolation level, READ UNCOMMITTED, READ COMMITTED,
// REPEATABLE READ and SERIALIZABLE. READ UNCOMMITTED requires ReadOnly; unsupported
// isolation levels return ErrUnsupported. Commit or roll back every transaction
// and avoid mixing manual transaction-control SQL with sql.Tx.
//
// Use sql.Conn for work that depends on session settings or temporary objects.
// ResetSession rolls back leftover transactions, but does not restore settings
// or drop temporary objects. Restore session settings explicitly. Socket closure
// rolls back unfinished work; the driver avoids Terminate, which can commit under
// the server's SESSION.COMMIT_AT_END setting.
//
// # Cancellation and errors
//
// Operation contexts apply through row consumption. Cancellation unblocks I/O,
// sends the session's cancel key on a separate connection using the same transport,
// then discards the original connection. Cancel delivery is best effort and
// bounded to one second. An expired context does not prove a write was rolled back;
// cancellation can race with a successful commit.
//
// Closing rows early drains unread batches within five seconds or an earlier
// operation deadline. A failure discards the connection. Config.ReadTimeout bounds
// the whole operation, not each read. Commit, rollback and statement cleanup use
// five-second deadlines because their database/sql interfaces carry no context.
//
// Use errors.As to inspect [Error] and its SQLCode, SQLState, Message, Rule,
// optional Position and TransactionActive fields. Context errors remain detectable
// with errors.Is, and local transport errors wrap their causes. A failure after a
// write might have reached the server is never returned as driver.ErrBadConn, so
// database/sql cannot silently repeat it. Applications must resolve uncertain
// outcomes using their own operation identifiers or other domain-specific checks.
package turborel
