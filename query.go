// Direct statements and parameterized executions share the streamed response.

package turborel

import (
	"context"
	"database/sql/driver"
	"io"
)

// QueryContext starts one response whose rows own the operation context.
func (c *conn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(ctx, query, args)
}

func (c *conn) query(ctx context.Context, query string, args []driver.NamedValue) (*rows, error) {
	if len(args) > 0 {
		statement, err := c.prepare(ctx, query)
		if err != nil {
			return nil, err
		}
		result, err := statement.query(ctx, args, true)
		if err != nil {
			_ = statement.Close()
		} // Preserve the execution error; Close discards on cleanup failure.
		return result, err
	}
	op, err := c.start(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.direct(query, !c.inTx); err != nil {
		return nil, op.finish(err)
	}
	r := &rows{connection: c, operation: op, columns: []column{}}
	kind, d, err := c.receive()
	if err != nil {
		return nil, op.finish(err)
	}
	switch kind {
	case msgDescription:
		r.columns = readColumns(d)
		if err := d.finish(); err != nil {
			return nil, op.finish(c.broken(err))
		}
		c.busy = true
		return r, nil
	case msgComplete:
		r.result, err = c.completion(d)
		r.ended = true
		return r, op.finish(err)
	default:
		return nil, op.finish(c.broken(invalid("expected a description or completion")))
	}
}

// ExecContext consumes the response and returns its completion.
func (c *conn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	r, err := c.query(ctx, query, args)
	if err != nil {
		return nil, err
	}
	return consume(r)
}

func consume(r *rows) (driver.Result, error) {
	destination := make([]driver.Value, len(r.columns))
	for {
		err := r.Next(destination)
		if err == io.EOF {
			return r.result, r.Close()
		}
		if err != nil {
			_ = r.Close() // Next's failure is the useful diagnostic.
			return nil, err
		}
	}
}

func namedValues(args []driver.Value) []driver.NamedValue {
	values := make([]driver.NamedValue, 0, len(args))
	for i, value := range args {
		values = append(values, driver.NamedValue{Ordinal: i + 1, Value: value})
	}
	return values
}
