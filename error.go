// Errors preserve the server's diagnostics and distinguish local failures.

package turborel

import (
	"errors"
	"fmt"
)

// ErrUnsupported means the server protocol does not provide an operation.
var ErrUnsupported = errors.New("turborel: operation is not supported")

// Error is a statement or connection failure reported by the server.
type Error struct {
	// SQLCode is the signed code reported by the server.
	SQLCode int32
	// SQLState is the five-character SQLSTATE.
	SQLState string
	// Message is the server's diagnostic text.
	Message string
	// Rule is the optional violated rule identifier.
	Rule string
	// Position is the optional byte offset into the SQL text, as sent by the server.
	Position *uint64
	// TransactionActive says whether the server still owns an active transaction.
	TransactionActive bool
}

// Error gives the server's diagnostics without discarding their typed fields.
func (e *Error) Error() string {
	return fmt.Sprintf("turborel: SQLCODE %d (SQLSTATE %s): %s", e.SQLCode, e.SQLState, e.Message)
}

func (e *Error) endsConnection() bool {
	if e.SQLState == "08003" {
		return false
	}
	return len(e.SQLState) == 5 && (e.SQLState[:2] == "08" || e.SQLCode == -919 && e.SQLState[:2] == "28")
}

func invalid(message string) error { return fmt.Errorf("turborel: [DESIGN] %s", message) }

func unsupported(operation string) error {
	return fmt.Errorf("turborel: %s: %w", operation, ErrUnsupported)
}

// Result reports a successful execution. Version 1 carries no affected-row
// count or generated key, so both database/sql accessors return ErrUnsupported.
type Result struct {
	// SQLCode is the completion code reported by the server.
	SQLCode int32
	// SQLState is the completion's five-character SQLSTATE.
	SQLState string
}

// LastInsertId returns ErrUnsupported: the protocol carries no generated key.
func (Result) LastInsertId() (int64, error) { return 0, unsupported("LastInsertId") }

// RowsAffected returns ErrUnsupported: the protocol carries no row count.
func (Result) RowsAffected() (int64, error) { return 0, unsupported("RowsAffected") }
