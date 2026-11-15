// A prepared statement belongs to one session and survives its transactions.

package turborel

import (
	"context"
	"database/sql/driver"
	"math"
)

type stmt struct {
	connection *conn
	number     uint32
	parameters []dataType
	columns    []column
	closed     bool
}

// Prepare implements the legacy driver interface.
func (c *conn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}

// PrepareContext obtains parameter and column metadata from the server.
func (c *conn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	return c.prepare(ctx, query)
}

func (c *conn) prepare(ctx context.Context, query string) (*stmt, error) {
	op, err := c.start(ctx)
	if err != nil {
		return nil, err
	}
	if c.next == math.MaxUint32 {
		return nil, op.finish(invalid("statement identifiers exhausted"))
	}
	s := &stmt{connection: c, number: c.next, parameters: []dataType{}}
	c.next++
	e := c.encode()
	e.u32(s.number)
	e.str(query)
	if err := c.send(msgPrepare, e); err != nil {
		return nil, op.finish(err)
	}
	kind, d, err := c.receive()
	if err != nil {
		return nil, op.finish(err)
	}
	if kind != msgPrepared {
		return nil, op.finish(c.broken(invalid("expected a prepared statement")))
	}
	if d.u32() != s.number {
		return nil, op.finish(c.broken(invalid("unexpected statement identifier")))
	}
	for range d.count() {
		d.flag()
		s.parameters = append(s.parameters, readType(d, 0))
	}
	s.columns = readColumns(d)
	if err := d.finish(); err != nil {
		return nil, op.finish(c.broken(err))
	}
	return s, op.finish(nil)
}

// NumInput reports the parameter count returned by Prepare.
func (s *stmt) NumInput() int { return len(s.parameters) }

// Close releases the server statement with a bounded cleanup deadline.
func (s *stmt) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	c := s.connection
	if c.bad.Load() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	op, err := c.start(ctx)
	if err != nil {
		c.discard()
		return err
	}
	e := c.encode()
	e.u32(s.number)
	err = c.send(msgDeallocate, e)
	if err == nil {
		_, err = c.expectComplete()
	}
	err = op.finish(err)
	if err != nil {
		c.discard()
	}
	return err
}

// Query implements the legacy driver interface.
func (s *stmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.QueryContext(context.Background(), namedValues(args))
}

// Exec implements the legacy driver interface.
func (s *stmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.ExecContext(context.Background(), namedValues(args))
}

// QueryContext binds values separately from SQL and streams the response.
func (s *stmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return s.query(ctx, args, false)
}

// ExecContext binds values and consumes the response.
func (s *stmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	r, err := s.query(ctx, args, false)
	if err != nil {
		return nil, err
	}
	return consume(r)
}

func (s *stmt) query(ctx context.Context, args []driver.NamedValue, owned bool) (*rows, error) {
	if s.closed {
		return nil, invalid("statement is closed")
	}
	if len(args) != len(s.parameters) {
		return nil, invalid("parameter count differs from prepared statement")
	}
	c := s.connection
	e := c.encode()
	e.u32(s.number)
	e.u32(uint32(len(args)))
	for i, arg := range args {
		if arg.Name != "" {
			return nil, unsupported("named parameters")
		}
		putValue(e, arg.Value, s.parameters[i], 0)
	}
	e.u32(0)
	e.flag(!c.inTx)
	if e.err != nil {
		return nil, e.err
	}
	op, err := c.start(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.send(msgExecute, e); err != nil {
		return nil, op.finish(err)
	}
	r := &rows{connection: c, operation: op, columns: s.columns}
	if owned {
		r.statement = s
	}
	c.busy = true
	if err := r.advance(); err != nil {
		return nil, err
	}
	return r, nil
}

var (
	_ driver.Stmt             = (*stmt)(nil)
	_ driver.StmtExecContext  = (*stmt)(nil)
	_ driver.StmtQueryContext = (*stmt)(nil)
)
