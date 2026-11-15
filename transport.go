// Transport deadlines and cancellation bound every network operation.

package turborel

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"
)

func dial(ctx context.Context, c Config) (net.Conn, error) {
	dialer := net.Dialer{Timeout: c.ConnectTimeout}
	stream, err := dialer.DialContext(ctx, c.Network, c.Address)
	if err != nil {
		return nil, fmt.Errorf("turborel: connect: %w", err)
	}
	if c.TLS == nil {
		return stream, nil
	}
	config := c.TLS.Clone()
	if config.ServerName == "" {
		host, _, err := net.SplitHostPort(c.Address)
		if err != nil {
			_ = stream.Close()
			return nil, invalid("TLS requires a server name")
		}
		config.ServerName = host
	}
	secure := tls.Client(stream, config)
	if err := secure.HandshakeContext(ctx); err != nil {
		_ = stream.Close()
		return nil, fmt.Errorf("turborel: TLS handshake: %w", err)
	}
	return secure, nil
}

type operation struct {
	connection *conn
	stop       func() bool
	done       chan struct{}
	// A result's lifetime extends through Rows.Next and Close. This context is
	// owned by that single operation, never retained by the pooled connection.
	ctx      context.Context
	deadline time.Time
	once     sync.Once
	err      error
}

func (c *conn) start(ctx context.Context) (*operation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := c.available(); err != nil {
		return nil, err
	}
	deadline, _ := ctx.Deadline()
	if c.config.ReadTimeout > 0 {
		readUntil := time.Now().Add(c.config.ReadTimeout)
		if deadline.IsZero() || readUntil.Before(deadline) {
			deadline = readUntil
		}
	}
	if err := c.stream.SetDeadline(deadline); err != nil {
		return nil, c.broken(err)
	}
	op := &operation{connection: c, ctx: ctx, deadline: deadline, done: make(chan struct{})}
	op.stop = context.AfterFunc(ctx, func() {
		defer close(op.done)
		c.bad.Store(true)
		// Unblock the caller first; a bounded cancel connection asks the server
		// to stop work while the original session still owns its cancel key.
		_ = c.stream.SetDeadline(time.Now()) // Close below also unblocks failed deadline updates.
		c.cancelRunning()
		_ = c.stream.Close() // The connection is already marked unusable.
	})
	return op, nil
}

func (op *operation) finish(err error) error {
	op.once.Do(func() {
		if !op.stop() {
			<-op.done
		}
		if canceled := op.ctx.Err(); canceled != nil {
			err = canceled
		}
		if !op.connection.bad.Load() {
			if reset := op.connection.stream.SetDeadline(time.Time{}); reset != nil && err == nil {
				err = op.connection.broken(reset)
			}
		}
		op.err = err
	})
	return op.err
}

func (c *conn) cancelRunning() {
	key := c.key.Load()
	if key == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cancelTimeout)
	defer cancel()
	stream, err := dial(ctx, c.config)
	if err != nil {
		return
	} // The original connection is discarded even if cancellation cannot be delivered.
	defer func() { _ = stream.Close() }() // The cancellation connection is never reused.
	deadline, _ := ctx.Deadline()
	if err := stream.SetDeadline(deadline); err != nil {
		return
	}
	e := c.encode()
	e.put(key[:])
	_ = writeFrame(stream, msgCancel, e) // Cancellation is best effort; the caller receives its context error.
}
