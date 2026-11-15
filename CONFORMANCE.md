# Driver and protocol conformance

This describes the client, not the server's SQL standard conformance.
Compatibility is with native protocol version 1 at VERSIONING.md's pin.

| Behavior | Implementation | Evidence |
|---|---|---|
| URL and typed configuration | DriverContext, Connector | TestDSN, TestTLSConfiguration |
| Verified TLS | TCP, custom roots and client certificates | TestServerTLSAuthentication, TestServerUntrustedCertificate |
| Authentication | Trust, password, SCRAM-SHA-256, SCRAM-PLUS, certificate | TestServerTLSAuthentication, TestSCRAMProof |
| Queries and positional parameters | QueryerContext, ExecerContext, PrepareContext | TestServerStatements |
| Streaming and early close | Bounded result frames, response drainage | TestServerStreamingAndEarlyClose |
| Transactions | Commit, rollback, access mode and four isolation levels | TestServerTransactions, TestTransactionModes |
| Scalar values | NULL, exact and approximate numbers, text, binary, boolean | TestBasicValues, TestServerScalarTypes |
| Datetimes, DECFLOAT, arrays, intervals | Prepared metadata and lossless encodings | TestServerRichParameters, TestDecFloatValues |
| Nested arrays and rows | Parameters; JSON results | TestNestedCompositeValues, TestRowValues |
| Cancellation | Fresh connection with session cancel key; discard old session | TestNativeCancellation |
| Uncertain writes | No ErrBadConn retry after sending | TestAmbiguousWriteIsNotRetried |
| Frame validation | Length, UTF-8, tag, flag and recursion bounds | TestMalformedFrames, FuzzDecode |
| Server errors | SQLCODE, SQLSTATE, message, rule, position, transaction state | TestServerTransactions, TestServerDiagnostics |

Rows expose column names, SQL type names, declared lengths, nullability,
and exact numeric precision and scale. Exact SQL integer families share
the wire's NUMERIC type; their original spelling is unavailable.

The default query path asks Execute for all rows and receives bounded
batches. It does not expose native Fetch, scrollable cursors or multiple
result sets. These are API choices, not claims that the wire lacks them.

RowsAffected and LastInsertId return ErrUnsupported because version 1
carries neither count nor generated key. Windows named pipes are not
implemented. Extension output is display text; its binary payload is not
exposed through database/sql. Session settings and temporary objects are
not reset when a pooled connection is reused. See README.md, Go package documentation and TODO.md.
