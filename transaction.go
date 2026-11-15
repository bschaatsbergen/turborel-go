// Transactions translate database/sql modes into the session's next transaction.

package turborel

import (
	"context"
	"database/sql"
	"database/sql/driver"
)

type transaction struct {
	connection *conn
	done       bool
}

// Begin implements the legacy driver interface.
func (c *conn) Begin() (driver.Tx, error) { return c.BeginTx(context.Background(), driver.TxOptions{}) }

// BeginTx sets the access mode and isolation of the next transaction.
func (c *conn) BeginTx(ctx context.Context, options driver.TxOptions) (driver.Tx, error) {
	if c.inTx {
		return nil, invalid("a transaction is already active")
	}
	mode, err := transactionMode(options)
	if err != nil {
		return nil, err
	}
	op, err := c.start(ctx)
	if err != nil {
		return nil, err
	}
	err = c.direct("SET TRANSACTION "+mode, false)
	if err == nil {
		_, err = c.expectComplete()
	}
	if err = op.finish(err); err != nil {
		return nil, err
	}
	c.inTx = true
	return &transaction{connection: c}, nil
}

func transactionMode(options driver.TxOptions) (string, error) {
	access := "READ WRITE"
	if options.ReadOnly {
		access = "READ ONLY"
	}
	var isolation string
	switch sql.IsolationLevel(options.Isolation) {
	case sql.LevelDefault:
	case sql.LevelReadUncommitted:
		if !options.ReadOnly {
			return "", unsupported("read-write READ UNCOMMITTED transaction")
		}
		isolation = "READ UNCOMMITTED"
	case sql.LevelReadCommitted:
		isolation = "READ COMMITTED"
	case sql.LevelRepeatableRead:
		isolation = "REPEATABLE READ"
	case sql.LevelSerializable:
		isolation = "SERIALIZABLE"
	default:
		return "", unsupported("transaction isolation level")
	}
	if isolation != "" {
		access += ", ISOLATION LEVEL " + isolation
	}
	return access, nil
}

// Commit commits this transaction within a bounded cleanup period.
func (t *transaction) Commit() error { return t.end("COMMIT") }

// Rollback rolls back this transaction within a bounded cleanup period.
func (t *transaction) Rollback() error { return t.end("ROLLBACK") }

func (t *transaction) end(sql string) error {
	if t.done {
		return invalid("transaction has already ended")
	}
	t.done = true
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	c := t.connection
	_, err := c.ExecContext(ctx, sql, nil)
	c.inTx = false
	if err != nil {
		c.discard()
	}
	return err
}
