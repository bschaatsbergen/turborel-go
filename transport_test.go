// Transport tests synchronize failures with received requests instead of sleeps.
package turborel

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func pipeConnection(t *testing.T) (*conn, net.Conn) {
	t.Helper()
	client, peer := net.Pipe()
	t.Cleanup(func() { _ = client.Close(); _ = peer.Close() })
	if err := peer.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	return newConn(client, Config{MaxBody: defaultMaxBody}), peer
}

// Verifies: [DESIGN] D4: cancellation sends the session key on a separate connection and discards the original session.
func TestNativeCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	c, peer := pipeConnection(t)
	c.config = Config{Network: "tcp", Address: listener.Addr().String(), Plaintext: true, MaxBody: defaultMaxBody}
	key := [cancelKeyBytes]byte{1, 2, 3, 4}
	c.key.Store(&key)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := c.ExecContext(ctx, "VALUES (1)", nil); result <- err }()
	kind, _, err := readFrame(peer, defaultMaxBody)
	if err != nil || kind != msgQuery {
		t.Fatalf("query: %c %v", kind, err)
	}
	cancel()
	if err := listener.(*net.TCPListener).SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	request, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = request.Close() }()
	if err := request.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	kind, d, err := readFrame(request, defaultMaxBody)
	if err != nil {
		t.Fatal(err)
	}
	if kind != msgCancel || string(d.take(cancelKeyBytes)) != string(key[:]) || d.finish() != nil {
		t.Fatal("wrong cancel frame")
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("result: %v", err)
	}
	if c.IsValid() {
		t.Fatal("canceled connection reused")
	}
}

type singleConnector struct {
	connection *conn
	opens      atomic.Int32
}

// Connect supplies a single connection and records accidental retries.
func (s *singleConnector) Connect(context.Context) (driver.Conn, error) {
	if s.opens.Add(1) != 1 {
		return nil, errors.New("unexpected retry")
	}
	return s.connection, nil
}

// Driver associates this test connector with its database/sql driver.
func (*singleConnector) Driver() driver.Driver { return &Driver{} }

// Verifies: [DESIGN] D4: a lost reply to a write never triggers database/sql's ErrBadConn retry.
func TestAmbiguousWriteIsNotRetried(t *testing.T) {
	c, peer := pipeConnection(t)
	connector := &singleConnector{connection: c}
	db := sql.OpenDB(connector)
	defer func() { _ = db.Close() }()
	received := make(chan error, 1)
	go func() {
		kind, _, err := readFrame(peer, defaultMaxBody)
		if err == nil && kind != msgQuery {
			err = errors.New("expected query")
		}
		_ = peer.Close()
		received <- err
	}()
	_, err := db.ExecContext(t.Context(), "INSERT INTO T VALUES (1)")
	if err == nil || errors.Is(err, driver.ErrBadConn) {
		t.Fatalf("ambiguous result: %v", err)
	}
	if err := <-received; err != nil {
		t.Fatal(err)
	}
	if connector.opens.Load() != 1 {
		t.Fatal("write was retried")
	}
}

type deadlineRecorder struct {
	net.Conn
	observed chan time.Time
}

// SetReadDeadline records the cleanup deadline before the peer is released.
func (c *deadlineRecorder) SetReadDeadline(deadline time.Time) error {
	c.observed <- deadline
	return c.Conn.SetReadDeadline(deadline)
}

// Verifies: [DESIGN] D4: closing rows never extends an earlier operation deadline.
func TestClosePreservesDeadline(t *testing.T) {
	c, peer := pipeConnection(t)
	recorded := make(chan time.Time, 1)
	c.stream = &deadlineRecorder{Conn: c.stream, observed: recorded}
	op, err := c.start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	original := time.Unix(100, 0)
	op.deadline = original
	r := &rows{connection: c, operation: op}
	finished := make(chan error, 1)
	go func() { finished <- r.Close() }()
	if got := <-recorded; !got.Equal(original) {
		t.Errorf("deadline: %v, want %v", got, original)
	}
	_ = peer.Close() // Release the peer even if a regression extended the deadline.
	if err := <-finished; err == nil {
		t.Fatal("closed peer reported success")
	}
	if c.IsValid() {
		t.Fatal("failed drain returned a reusable connection")
	}
}
