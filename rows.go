// Rows retain at most one protocol frame and finish before their connection is reused.

package turborel

import (
	"database/sql/driver"
	"io"
	"reflect"
	"time"
)

type rows struct {
	connection *conn
	operation  *operation
	statement  *stmt
	columns    []column
	batch      [][]driver.Value
	at         int
	ended      bool
	closed     bool
	result     Result
	err        error
}

// Columns returns the names supplied by the server.
func (r *rows) Columns() []string {
	names := make([]string, 0, len(r.columns))
	for _, c := range r.columns {
		names = append(names, c.name)
	}
	return names
}

// Next copies one row and receives another bounded batch when necessary.
func (r *rows) Next(destination []driver.Value) error {
	if r.closed {
		return io.EOF
	}
	if len(destination) != len(r.columns) {
		return invalid("row destination has the wrong width")
	}
	for r.at >= len(r.batch) && !r.ended {
		if err := r.advance(); err != nil {
			return err
		}
	}
	if r.at < len(r.batch) {
		copy(destination, r.batch[r.at])
		r.at++
		return nil
	}
	if r.err != nil {
		return r.err
	}
	if err := r.release(); err != nil {
		return err
	}
	return io.EOF
}

func (r *rows) advance() error {
	c := r.connection
	kind, d, err := c.receive()
	if err != nil {
		return r.finish(err)
	}
	switch kind {
	case msgRows:
		r.batch, r.at = readRows(d, len(r.columns)), 0
		if err := d.finish(); err != nil {
			return r.finish(c.broken(err))
		}
		return nil
	case msgComplete:
		r.result, err = c.completion(d)
		return r.finish(err)
	default:
		return r.finish(c.broken(invalid("unexpected message in query response")))
	}
}

func (r *rows) finish(err error) error {
	r.ended = true
	r.connection.busy = false
	r.err = r.operation.finish(err)
	return r.err
}

func (r *rows) release() error {
	if r.statement == nil {
		return nil
	}
	statement := r.statement
	r.statement = nil
	return statement.Close()
}

// Close drains the response within a bounded cleanup period.
func (r *rows) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	if !r.ended {
		deadline := time.Now().Add(cleanupTimeout)
		if original := r.operation.deadline; !original.IsZero() && original.Before(deadline) {
			deadline = original
		}
		if err := r.connection.stream.SetReadDeadline(deadline); err != nil {
			_ = r.finish(r.connection.broken(err)) // finish saves the error returned below.
		}
		for !r.ended {
			_ = r.advance() // advance records its failure and ends the response.
		}
	}
	r.batch = nil
	cleanup := r.release()
	if r.err != nil {
		return r.err
	}
	return cleanup
}

// ColumnTypeDatabaseTypeName reports the native SQL type.
func (r *rows) ColumnTypeDatabaseTypeName(index int) string {
	if index < 0 || index >= len(r.columns) {
		return ""
	}
	return r.columns[index].typ.databaseName()
}

// ColumnTypeScanType reports the lossless Go representation.
func (r *rows) ColumnTypeScanType(index int) reflect.Type {
	if index < 0 || index >= len(r.columns) {
		return reflect.TypeFor[any]()
	}
	return r.columns[index].typ.scanType()
}

// ColumnTypeNullable reports the column constraint from its description.
func (r *rows) ColumnTypeNullable(index int) (bool, bool) {
	if index < 0 || index >= len(r.columns) {
		return false, false
	}
	return !r.columns[index].notNull, true
}

// ColumnTypeLength reports declared character units or binary bytes.
func (r *rows) ColumnTypeLength(index int) (int64, bool) {
	if index < 0 || index >= len(r.columns) {
		return 0, false
	}
	typ := r.columns[index].typ
	switch typ.tag {
	case tagChar, tagVarying, tagBinary, tagVarBinary:
		return int64(typ.length), true
	default:
		return 0, false
	}
}

// ColumnTypePrecisionScale reports exact numeric precision and scale.
func (r *rows) ColumnTypePrecisionScale(index int) (int64, int64, bool) {
	if index < 0 || index >= len(r.columns) {
		return 0, 0, false
	}
	typ := r.columns[index].typ
	if typ.tag != tagExact {
		return 0, 0, false
	}
	return int64(typ.precision), int64(typ.scale), true
}

var (
	_ driver.Rows                           = (*rows)(nil)
	_ driver.RowsColumnTypeDatabaseTypeName = (*rows)(nil)
	_ driver.RowsColumnTypeScanType         = (*rows)(nil)
	_ driver.RowsColumnTypeNullable         = (*rows)(nil)
	_ driver.RowsColumnTypeLength           = (*rows)(nil)
	_ driver.RowsColumnTypePrecisionScale   = (*rows)(nil)
)
