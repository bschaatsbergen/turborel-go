// A connection owns one session, one response at a time, and its transaction.

package turborel

import (
	"bufio"
	"context"
	"database/sql/driver"
	"fmt"
	"net"
	"sync/atomic"
)

type conn struct {
	stream net.Conn
	reader *bufio.Reader
	config Config
	bad    atomic.Bool
	key    atomic.Pointer[[cancelKeyBytes]byte]
	next   uint32
	active bool
	inTx   bool
	busy   bool
	server string
}

func newConn(stream net.Conn, config Config) *conn {
	return &conn{stream: stream, reader: bufio.NewReader(stream), config: config}
}

func (c *conn) encode() *encoder { return &encoder{limit: int(c.config.MaxBody)} }

func (c *conn) available() error {
	if c.bad.Load() {
		return driver.ErrBadConn
	}
	if c.busy {
		return invalid("close the current rows before another operation")
	}
	return nil
}

func (c *conn) discard() {
	if c.bad.CompareAndSwap(false, true) {
		_ = c.stream.Close() // Closing a failed session cannot restore it.
	}
}

func (c *conn) broken(err error) error {
	c.discard()
	// Never return ErrBadConn here: the server may already have applied a write.
	return fmt.Errorf("turborel: connection lost: %w", err)
}

func (c *conn) send(kind byte, e *encoder) error {
	if e.err != nil {
		return e.err
	}
	if err := writeFrame(c.stream, kind, e); err != nil {
		return c.broken(err)
	}
	return nil
}

func (c *conn) receive() (byte, *decoder, error) {
	kind, d, err := readFrame(c.reader, c.config.MaxBody)
	if err != nil {
		return 0, nil, c.broken(err)
	}
	if kind != msgError {
		return kind, d, nil
	}
	failure := readFailure(d)
	if err := d.finish(); err != nil {
		return 0, nil, c.broken(err)
	}
	c.active = failure.TransactionActive
	if failure.endsConnection() {
		c.discard()
	}
	return kind, nil, failure
}

func readFailure(d *decoder) *Error {
	e := &Error{SQLCode: int32(d.u32()), SQLState: d.str(), Message: d.str()}
	if d.flag() {
		e.Rule = d.str()
	}
	if d.flag() {
		position := d.u64()
		e.Position = &position
	}
	e.TransactionActive = d.flag()
	if len(e.SQLState) != 5 {
		d.err = invalid("SQLSTATE must contain five characters")
	}
	return e
}

func (c *conn) completion(d *decoder) (Result, error) {
	result := Result{SQLCode: int32(d.u32()), SQLState: d.str()}
	c.active = d.flag()
	if len(result.SQLState) != 5 {
		d.err = invalid("SQLSTATE must contain five characters")
	}
	if err := d.finish(); err != nil {
		return result, c.broken(err)
	}
	return result, nil
}

func (c *conn) expectComplete() (Result, error) {
	kind, d, err := c.receive()
	if err != nil {
		return Result{}, err
	}
	if kind != msgComplete {
		return Result{}, c.broken(invalid("expected a completion"))
	}
	return c.completion(d)
}

func (c *conn) direct(query string, ends bool) error {
	e := c.encode()
	e.str(query)
	e.flag(ends)
	return c.send(msgQuery, e)
}

// Close drops the socket so an unfinished transaction rolls back.
func (c *conn) Close() error {
	// A socket close rolls back; Terminate can commit under SESSION.COMMIT_AT_END.
	c.discard()
	return nil
}

// IsValid reports whether the pool may reuse the connection.
func (c *conn) IsValid() bool { return !c.bad.Load() && !c.busy && !c.inTx }

// ResetSession rolls back a transaction left active outside database/sql.
func (c *conn) ResetSession(ctx context.Context) error {
	if err := c.available(); err != nil {
		return err
	}
	if c.inTx {
		c.discard()
		return driver.ErrBadConn
	}
	if !c.active {
		return ctx.Err()
	}
	_, err := c.ExecContext(ctx, "ROLLBACK", nil)
	if err != nil {
		c.discard()
		return driver.ErrBadConn
	}
	return nil
}

// Ping checks the connection with a statement that opens no persistent objects.
func (c *conn) Ping(ctx context.Context) error {
	_, err := c.ExecContext(ctx, "VALUES (1)", nil)
	return err
}

// CheckNamedValue admits exact and composite parameters before default conversion.
func (c *conn) CheckNamedValue(v *driver.NamedValue) error {
	if v.Name != "" {
		return unsupported("named parameters; use positional ? placeholders")
	}
	switch v.Value.(type) {
	case Decimal, DecFloat, Array, Row, Interval, Extension:
		return nil
	default:
		return driver.ErrSkip
	}
}
