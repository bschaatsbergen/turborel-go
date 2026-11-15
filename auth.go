// Authentication follows the reference client's Hello/Challenge/Answer exchange.

package turborel

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
)

const (
	nonceBytes     = 18
	maxAuthSteps   = 4
	mechanismSCRAM = "scram-sha-256"
	mechanismPLUS  = "scram-sha-256-plus"
	exporterLabel  = "EXPORTER-Channel-Binding"
)

func (c *conn) login() error {
	mechanism, initial, scram, err := c.authentication()
	if err != nil {
		return err
	}
	e := c.encode()
	e.u32(protocolVersion)
	e.str(c.config.User)
	e.str(mechanism)
	e.bytes(initial)
	e.u32(0)
	if err := c.send(msgHello, e); err != nil {
		return err
	}
	for range maxAuthSteps {
		kind, d, err := c.receive()
		if err != nil {
			return err
		}
		switch kind {
		case msgReady:
			if scram != nil && !scram.verified {
				return invalid("server did not prove its SCRAM identity")
			}
			return c.ready(d)
		case msgChallenge:
			challenge := d.bytes()
			if err := d.finish(); err != nil {
				return err
			}
			if scram == nil {
				return invalid("unexpected authentication challenge")
			}
			answer, err := scram.answer(string(challenge), c.config.Password)
			if err != nil {
				return err
			}
			e := c.encode()
			e.bytes(answer)
			if err := c.send(msgAnswer, e); err != nil {
				return err
			}
		default:
			return invalid("unexpected authentication message")
		}
	}
	return invalid("authentication exceeded its step limit")
}

func (c *conn) ready(d *decoder) error {
	d.str()
	var key [cancelKeyBytes]byte
	copy(key[:], d.take(cancelKeyBytes))
	for range d.count() {
		d.str()
	}
	c.server = d.str()
	if err := d.finish(); err != nil {
		return err
	}
	c.key.Store(&key)
	return nil
}

func (c *conn) authentication() (string, []byte, *scramClient, error) {
	switch c.config.Auth {
	case authPassword:
		return authPassword, []byte(c.config.Password), nil, nil
	case authTrust, authCertificate:
		return c.config.Auth, nil, nil, nil
	case authSCRAM:
		mechanism, header, binding, err := channel(c.stream)
		if err != nil {
			return "", nil, nil, err
		}
		var nonce [nonceBytes]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return "", nil, nil, err
		}
		s := &scramClient{nonce: base64.StdEncoding.EncodeToString(nonce[:]), header: header, binding: binding}
		s.bare = "n=,r=" + s.nonce
		return mechanism, []byte(header + s.bare), s, nil
	default:
		return "", nil, nil, unsupported("authentication mechanism")
	}
}

func channel(stream any) (string, string, []byte, error) {
	secure, ok := stream.(*tls.Conn)
	if !ok {
		return mechanismSCRAM, "n,,", nil, nil
	}
	state := secure.ConnectionState()
	if state.Version == tls.VersionTLS13 {
		binding, err := state.ExportKeyingMaterial(exporterLabel, []byte{}, sha256.Size)
		return mechanismPLUS, "p=tls-exporter,,", binding, err
	}
	if len(state.PeerCertificates) == 0 {
		return "", "", nil, invalid("TLS peer supplied no certificate")
	}
	certificate := state.PeerCertificates[0]
	var binding []byte
	switch certificate.SignatureAlgorithm {
	case x509.SHA384WithRSA, x509.ECDSAWithSHA384, x509.SHA384WithRSAPSS:
		hash := sha512.Sum384(certificate.Raw)
		binding = hash[:]
	case x509.SHA512WithRSA, x509.ECDSAWithSHA512, x509.SHA512WithRSAPSS:
		hash := sha512.Sum512(certificate.Raw)
		binding = hash[:]
	case x509.SHA1WithRSA, x509.ECDSAWithSHA1, x509.SHA256WithRSA, x509.ECDSAWithSHA256, x509.SHA256WithRSAPSS:
		hash := sha256.Sum256(certificate.Raw)
		binding = hash[:]
	default:
		return "", "", nil, unsupported("SCRAM channel binding for this TLS 1.2 certificate; use TLS 1.3")
	}
	return mechanismPLUS, "p=tls-server-end-point,,", binding, nil
}
