//go:build integration

// Integration tests exercise database/sql against the actual Rust server.
package turborel_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	turborel "github.com/bschaatsbergen/turborel-go"
)

// Verifies: [DESIGN] D2, D3: direct SQL, bound values, metadata, and autocommit interoperate with the server.
func TestServerStatements(t *testing.T) {
	db := openFreshServer(t)
	ctx := t.Context()
	execSQL(t, db, "CREATE TABLE STAFF (ID INTEGER NOT NULL, NAME CHARACTER VARYING(40), SALARY NUMERIC(18,2))")
	statement, err := db.PrepareContext(ctx, "INSERT INTO STAFF VALUES (?, ?, ?)")
	if err != nil {
		t.Fatal(err)
	}
	defer checkClose(t, statement)
	result, err := statement.ExecContext(ctx, 1, "Ada's name", turborel.Decimal("1234567890123456.78"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := result.RowsAffected(); !errors.Is(err, turborel.ErrUnsupported) {
		t.Fatalf("RowsAffected: %v", err)
	}
	var name string
	var amount turborel.Decimal
	err = db.QueryRowContext(ctx, "SELECT NAME, SALARY FROM STAFF WHERE ID = ?", 1).Scan(&name, &amount)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Ada's name" || amount != "1234567890123456.78" {
		t.Fatalf("values: %q %s", name, amount)
	}
	rows, err := db.QueryContext(ctx, "SELECT ID, NAME, SALARY FROM STAFF")
	if err != nil {
		t.Fatal(err)
	}
	defer checkClose(t, rows)
	columns, err := rows.ColumnTypes()
	if err != nil {
		t.Fatal(err)
	}
	if len(columns) != 3 {
		t.Fatalf("columns: %v", columns)
	}
	if nullable, known := columns[0].Nullable(); !known || nullable {
		t.Fatal("ID nullability")
	}
	if size, known := columns[1].Length(); !known || size != 40 {
		t.Fatal("NAME length")
	}
	if p, s, known := columns[2].DecimalSize(); !known || p != 18 || s != 2 {
		t.Fatal("SALARY precision")
	}
}

// Verifies: [DESIGN] D3: commits persist, rollbacks undo, and errors preserve SQL diagnostics.
func TestServerTransactions(t *testing.T) {
	db := openFreshServer(t)
	db.SetMaxOpenConns(1)
	execSQL(t, db, "CREATE TABLE T (ID INTEGER PRIMARY KEY)")
	for _, commit := range []bool{false, true} {
		tx, err := db.BeginTx(t.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(t.Context(), "INSERT INTO T VALUES (?)", 7); err != nil {
			t.Fatal(err)
		}
		if commit {
			err = tx.Commit()
		} else {
			err = tx.Rollback()
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM T").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("committed count: %d", count)
	}
	_, err := db.ExecContext(t.Context(), "INSERT INTO T VALUES (?)", 7)
	var failure *turborel.Error
	if !errors.As(err, &failure) || failure.SQLState != "23000" {
		t.Fatalf("duplicate: %v", err)
	}
	if failure.TransactionActive {
		t.Fatal("autocommit failure retained transaction")
	}
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// Verifies: [DESIGN] D3, D4: rows stream across frames and early Close leaves the pool synchronized.
func TestServerStreamingAndEarlyClose(t *testing.T) {
	db := openFreshServer(t)
	db.SetMaxOpenConns(1)
	execSQL(t, db, "CREATE TABLE T (ID INTEGER)")
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 50 {
		if _, err := tx.ExecContext(t.Context(), "INSERT INTO T VALUES (?)", i); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"SELECT ID FROM T ORDER BY ID", "SELECT ID FROM T WHERE ID >= ? ORDER BY ID"} {
		args := []any{}
		if query != "SELECT ID FROM T ORDER BY ID" {
			args = append(args, 0)
		}
		rows, err := db.QueryContext(t.Context(), query, args...)
		if err != nil {
			t.Fatal(err)
		}
		if !rows.Next() {
			t.Fatalf("first row: %v", rows.Err())
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		var count int
		if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM T").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 50 {
			t.Fatalf("count after Close: %d", count)
		}
	}
}

// Verifies: [DESIGN] D2: SQL nulls, bytes, booleans and datetimes survive bound-value round trips.
func TestServerScalarTypes(t *testing.T) {
	db := openFreshServer(t)
	execSQL(t, db, "CREATE TABLE T (N INTEGER, B BOOLEAN, X BINARY VARYING(4), D DATE, TS TIMESTAMP(6) WITH TIME ZONE)")
	instant := time.Date(2024, time.February, 29, 12, 34, 56, 123456000, time.FixedZone("", 3600))
	_, err := db.ExecContext(t.Context(), "INSERT INTO T VALUES (?, ?, ?, ?, ?)", nil, true, []byte{0, 255}, instant, instant)
	if err != nil {
		t.Fatal(err)
	}
	var null sql.NullInt64
	var boolean bool
	var binary []byte
	var date, timestamp time.Time
	err = db.QueryRowContext(t.Context(), "SELECT N, B, X, D, TS FROM T").Scan(&null, &boolean, &binary, &date, &timestamp)
	if err != nil {
		t.Fatal(err)
	}
	if null.Valid || !boolean || !bytes.Equal(binary, []byte{0, 255}) {
		t.Fatalf("values: %v %v %v", null, boolean, binary)
	}
	if date.Format(time.DateOnly) != "2024-02-29" || !timestamp.Equal(instant) {
		t.Fatalf("datetimes: %v %v", date, timestamp)
	}
}

// Verifies: [DESIGN] D4: an already canceled operation does not use or poison a pooled connection.
func TestServerCanceledContext(t *testing.T) {
	db := openFreshServer(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := db.ExecContext(ctx, "SELECT 1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// Verifies: [DESIGN] D2: datetime, decimal float, interval and array parameters use the server's prepared types.
func TestServerRichParameters(t *testing.T) {
	db := openFreshServer(t)
	execSQL(t, db, "CREATE TABLE RICH (D DATE, T TIME(6) WITH TIME ZONE, TS TIMESTAMP(6), F DECFLOAT(34), A INTEGER ARRAY[3], I INTERVAL DAY(3) TO SECOND(6))")
	instant := time.Date(2024, 2, 29, 12, 34, 56, 123456000, time.FixedZone("", 3600))
	interval := turborel.Interval{Units: "1234567", Qualifier: [4]uint32{2, 5, 3, 6}}
	_, err := db.ExecContext(t.Context(), "INSERT INTO RICH VALUES (?, ?, ?, ?, ?, ?)", instant, instant, instant, turborel.DecFloat("1.25e100"), turborel.Array{1, nil, 3}, interval)
	if err != nil {
		t.Fatal(err)
	}
	var date, clock, stamp time.Time
	var floating turborel.DecFloat
	var array, span string
	if err := db.QueryRowContext(t.Context(), "SELECT D, T, TS, F, A, I FROM RICH").Scan(&date, &clock, &stamp, &floating, &array, &span); err != nil {
		t.Fatal(err)
	}
	if date.Format("2006-01-02") != "2024-02-29" || clock.Hour() != 12 || !stamp.Equal(instant) {
		t.Fatalf("datetimes: %v %v %v", date, clock, stamp)
	}
	if floating != "125e98" || array != `["1",null,"3"]` || span != `{"Units":"1234567","Qualifier":[2,5,3,6]}` {
		t.Fatalf("values: %s %s %s", floating, array, span)
	}
}
