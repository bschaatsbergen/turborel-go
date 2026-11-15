//go:build integration

// Each integration fixture owns an isolated server and drains its diagnostics.
package turborel_test

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	turborel "github.com/bschaatsbergen/turborel-go"
)

func startServer(t *testing.T, extra []string) string {
	t.Helper()
	binary := os.Getenv("TURBORELD")
	if binary == "" {
		t.Fatal("integration tests require TURBORELD pointing to the pinned server binary")
	}
	args := []string{"--memory", "--set", "SERVER.PORT=0", "--set", "SERVER.TRUST_LOCAL=ON", "--set", "SERVER.BATCH_BYTES=256"}
	args = append(args, extra...)
	command := exec.Command(binary, args...)
	command.Stdin = strings.NewReader("integration-password\n")
	stderr, err := command.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	command.Stdout = io.Discard
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	ready := make(chan string, 1)
	finished := make(chan struct{})
	go serverOutput(stderr, ready, finished)
	t.Cleanup(func() {
		_ = command.Process.Kill() // The process may already have exited; Wait still reaps it.
		_ = command.Wait()
		<-finished
	})
	select {
	case address := <-ready:
		if strings.HasPrefix(address, "ERROR:") {
			t.Fatal(address)
		}
		return address
	case <-time.After(30 * time.Second):
		t.Fatal("server did not start")
	}
	return ""
}

func serverOutput(stderr io.Reader, ready chan<- string, finished chan<- struct{}) {
	defer close(finished)
	scanner := bufio.NewScanner(stderr)
	sent := false
	last := "server exited before listening"
	for scanner.Scan() {
		last = scanner.Text()
		var event struct {
			Event     string
			Addresses []string
		}
		if json.Unmarshal(scanner.Bytes(), &event) == nil && event.Event == "listening" && len(event.Addresses) > 0 {
			if !sent {
				ready <- event.Addresses[0]
				sent = true
			}
		}
	}
	if !sent {
		ready <- "ERROR: " + last
	}
}

func openServer(t *testing.T, address string, config turborel.Config) *sql.DB {
	t.Helper()
	config.Address = address
	if config.User == "" {
		config.User = "ANN"
	}
	connector, err := turborel.OpenConnector(config)
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(connector)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	return db
}

func execSQL(t *testing.T, db *sql.DB, query string) {
	t.Helper()
	if _, err := db.ExecContext(t.Context(), query); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

func openFreshServer(t *testing.T) *sql.DB {
	t.Helper()
	db := openServer(t, startServer(t, nil), turborel.Config{Auth: "trust", Plaintext: true})
	execSQL(t, db, "CREATE SCHEMA AUTHORIZATION ANN")
	return db
}

func checkClose(t *testing.T, closer io.Closer) {
	t.Helper()
	if err := closer.Close(); err != nil {
		t.Error(err)
	}
}
