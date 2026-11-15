// Configuration tests constrain privacy defaults and credential redaction.
package turborel

import (
	"crypto/tls"
	"fmt"
	"strings"
	"testing"
)

// Verifies: [DESIGN] D1, D5: DSNs are explicit, validated and private by default.
func TestDSN(t *testing.T) {
	tests := []struct {
		name, dsn string
		valid     bool
	}{
		{name: "secure default", dsn: "turborel://ANN:secret@localhost", valid: true},
		{name: "local trust", dsn: "turborel://ANN@localhost?tls=disable&auth=trust", valid: true},
		{name: "unix", dsn: "turborel://ANN:secret@?socket=/tmp/turborel.sock&auth=password", valid: true},
		{name: "plaintext password", dsn: "turborel://ANN:secret@localhost?tls=disable&auth=password"},
		{name: "plaintext scram", dsn: "turborel://ANN:secret@localhost?tls=disable"},
		{name: "explicit plaintext scram", dsn: "turborel://ANN:secret@localhost?tls=disable&allow_insecure_scram=true", valid: true},
		{name: "empty socket", dsn: "turborel://ANN@?socket=&auth=trust"},
		{name: "missing user", dsn: "turborel://localhost"},
		{name: "database path", dsn: "turborel://ANN@localhost/db"},
		{name: "unknown option", dsn: "turborel://ANN@localhost?secret=secret"},
		{name: "duplicate option", dsn: "turborel://ANN@localhost?auth=trust&auth=scram"},
		{name: "bad timeout", dsn: "turborel://ANN@localhost?connect_timeout=secret"},
		{name: "negative timeout", dsn: "turborel://ANN@localhost?read_timeout=-1s"},
		{name: "bad escape", dsn: "turborel://ANN:secret%xx@localhost"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config, err := ParseDSN(test.dsn)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v: %v", test.valid, err)
			}
			if strings.Contains(fmt.Sprintf("%v %#v", config, err), "secret") {
				t.Fatal("credentials exposed")
			}
		})
	}
}

// Verifies: [DESIGN] D5: TLS configurations are cloned and certificate checks cannot be disabled.
func TestTLSConfiguration(t *testing.T) {
	config := &tls.Config{ServerName: "original"}
	connector, err := OpenConnector(Config{User: "ANN", TLS: config})
	if err != nil {
		t.Fatal(err)
	}
	config.ServerName = "changed"
	if connector.config.TLS.ServerName != "original" {
		t.Fatal("TLS configuration aliases the caller")
	}
	_, err = OpenConnector(Config{User: "ANN", TLS: &tls.Config{InsecureSkipVerify: true}})
	if err == nil {
		t.Fatal("verification disabled")
	}
}
