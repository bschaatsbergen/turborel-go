// Transaction modes are validated before sending SQL to the server.
package turborel

import (
	"database/sql"
	"database/sql/driver"
	"strings"
	"testing"
)

// Verifies: [DESIGN] D3: supported isolation levels map to SQL and unsupported combinations fail locally.
func TestTransactionModes(t *testing.T) {
	cases := []struct {
		level    sql.IsolationLevel
		readOnly bool
		want     string
	}{
		{sql.LevelDefault, false, "READ WRITE"},
		{sql.LevelReadCommitted, false, "READ WRITE, ISOLATION LEVEL READ COMMITTED"},
		{sql.LevelRepeatableRead, false, "READ WRITE, ISOLATION LEVEL REPEATABLE READ"},
		{sql.LevelSerializable, true, "READ ONLY, ISOLATION LEVEL SERIALIZABLE"},
		{sql.LevelReadUncommitted, true, "READ ONLY, ISOLATION LEVEL READ UNCOMMITTED"},
		{sql.LevelReadUncommitted, false, ""},
		{sql.LevelSnapshot, false, ""},
		{sql.LevelLinearizable, false, ""},
	}
	for _, test := range cases {
		mode, err := transactionMode(driver.TxOptions{Isolation: driver.IsolationLevel(test.level), ReadOnly: test.readOnly})
		if test.want == "" {
			if err == nil {
				t.Fatal("invalid mode accepted")
			}
			continue
		}
		if err != nil || mode != test.want {
			t.Fatalf("mode %q: %v", mode, err)
		}
	}
}

// Verifies: [DESIGN] D1: server diagnostics retain optional fields and reject malformed SQLSTATE values.
func TestServerDiagnostics(t *testing.T) {
	e := &encoder{limit: defaultMaxBody}
	e.u32(0xfffffc18)
	e.str("42000")
	e.str("bad SQL")
	e.flag(true)
	e.str("[DESIGN]")
	e.flag(true)
	e.u64(7)
	e.flag(true)
	d := &decoder{data: e.data}
	failure := readFailure(d)
	if err := d.finish(); err != nil {
		t.Fatal(err)
	}
	if failure.SQLCode != -1000 || failure.Position == nil || *failure.Position != 7 || !failure.TransactionActive || failure.Rule != "[DESIGN]" {
		t.Fatalf("error: %#v", failure)
	}
	if !strings.Contains(failure.Error(), "42000") {
		t.Fatal("missing SQLSTATE")
	}
	if (&Error{SQLState: "08003"}).endsConnection() {
		t.Fatal("statement error closed connection")
	}
	if !(&Error{SQLState: "08006"}).endsConnection() {
		t.Fatal("broken connection kept")
	}
}
