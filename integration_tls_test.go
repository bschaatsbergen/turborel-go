//go:build integration

// TLS fixtures use fixed Ed25519 seeds and validity dates for reproducible identities.
package turborel_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	turborel "github.com/bschaatsbergen/turborel-go"
)

type testPKI struct {
	root        *x509.CertPool
	certificate tls.Certificate
	options     []string
}

func makePKI(t *testing.T) testPKI {
	t.Helper()
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, ed25519.SeedSize))
	ca := certificateTemplate("test authority", 1)
	ca.IsCA, ca.BasicConstraintsValid = true, true
	ca.KeyUsage = x509.KeyUsageCertSign
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	server := certificateTemplate("localhost", 2)
	server.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	server.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	serverPEM, serverKey := signCertificate(t, server, ca, key)
	client := certificateTemplate("ANN", 3)
	client.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	clientPEM, clientKey := signCertificate(t, client, ca, key)
	certificate, err := tls.X509KeyPair(clientPEM, clientKey)
	if err != nil {
		t.Fatal(err)
	}
	rootPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	root := x509.NewCertPool()
	if !root.AppendCertsFromPEM(rootPEM) {
		t.Fatal("CA certificate")
	}
	dir := t.TempDir()
	for name, data := range map[string][]byte{"ca.pem": rootPEM, "server.pem": serverPEM, "server.key": serverKey} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	options := []string{
		"--owner", "ANN", "--set", "SERVER.TLS=REQUIRED",
		"--set", "SERVER.PASSWORD_ITERATIONS=10000", "--set", "SERVER.LOGIN_DELAY_MS=0",
		"--set", "SERVER.TLS_CERTIFICATE=" + filepath.Join(dir, "server.pem"),
		"--set", "SERVER.TLS_KEY=" + filepath.Join(dir, "server.key"),
		"--set", "SERVER.TLS_CLIENT_CA=" + filepath.Join(dir, "ca.pem"),
	}
	return testPKI{root: root, certificate: certificate, options: options}
}

func certificateTemplate(name string, serial int64) *x509.Certificate {
	return &x509.Certificate{
		SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name},
		NotBefore: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:  time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC),
		KeyUsage:  x509.KeyUsageDigitalSignature,
	}
}

func signCertificate(t *testing.T, leaf, ca *x509.Certificate, caKey ed25519.PrivateKey) ([]byte, []byte) {
	t.Helper()
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{byte(leaf.SerialNumber.Int64())}, ed25519.SeedSize))
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, key.Public(), caKey)
	if err != nil {
		t.Fatal(err)
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})
}

// Verifies: [DESIGN] D1, D5: password, SCRAM-PLUS and certificates authenticate over verified TLS.
func TestServerTLSAuthentication(t *testing.T) {
	pki := makePKI(t)
	address := startServer(t, pki.options)
	for _, auth := range []string{"password", "scram", "certificate"} {
		t.Run(auth, func(t *testing.T) {
			config := turborel.Config{Auth: auth, Password: "integration-password", TLS: &tls.Config{
				RootCAs: pki.root, Certificates: []tls.Certificate{pki.certificate}, MinVersion: tls.VersionTLS13,
			}}
			db := openServer(t, address, config)
			var value int
			if err := db.QueryRowContext(t.Context(), "VALUES (42)").Scan(&value); err != nil || value != 42 {
				t.Fatalf("query: %d %v", value, err)
			}
		})
	}
	connector, err := turborel.OpenConnector(turborel.Config{Address: address, User: "ANN", Password: "wrong", TLS: &tls.Config{
		RootCAs: pki.root, Certificates: []tls.Certificate{pki.certificate}, MinVersion: tls.VersionTLS13,
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = connector.Connect(t.Context())
	var failure *turborel.Error
	if !errors.As(err, &failure) || failure.SQLState != "28000" {
		t.Fatalf("wrong password: %v", err)
	}
}

// Verifies: [DESIGN] D5: a certificate signed by an untrusted authority is refused before credentials are sent.
func TestServerUntrustedCertificate(t *testing.T) {
	pki := makePKI(t)
	address := startServer(t, pki.options)
	connector, err := turborel.OpenConnector(turborel.Config{Address: address, User: "ANN", Password: "integration-password"})
	if err != nil {
		t.Fatal(err)
	}
	if c, err := connector.Connect(t.Context()); err == nil {
		checkClose(t, c)
		t.Fatal("untrusted TLS accepted")
	}
}
