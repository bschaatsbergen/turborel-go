// SCRAM uses standard-library cryptography and verifies the server's proof.

package turborel

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
)

// Bound password derivation work even when the authenticating peer is hostile.
const maxSCRAMIterations = 2_000_000

type scramClient struct {
	nonce    string
	header   string
	binding  []byte
	bare     string
	expected []byte
	verified bool
}

func mac(key []byte, text string) []byte {
	hash := hmac.New(sha256.New, key)
	hash.Write([]byte(text))
	return hash.Sum(nil)
}

func (s *scramClient) answer(message, password string) ([]byte, error) {
	if s.verified {
		return nil, invalid("SCRAM challenge after server verification")
	}
	if s.expected != nil {
		return s.verify(message)
	}
	fields := strings.Split(message, ",")
	if len(fields) != 3 {
		return nil, invalid("invalid SCRAM server-first message")
	}
	nonce, ok := strings.CutPrefix(fields[0], "r=")
	if !ok || len(nonce) <= len(s.nonce) || !strings.HasPrefix(nonce, s.nonce) {
		return nil, invalid("SCRAM server nonce does not extend the client nonce")
	}
	saltText, ok := strings.CutPrefix(fields[1], "s=")
	if !ok {
		return nil, invalid("SCRAM salt is missing")
	}
	salt, err := base64.StdEncoding.Strict().DecodeString(saltText)
	if err != nil {
		return nil, invalid("invalid SCRAM salt")
	}
	iterationsText, ok := strings.CutPrefix(fields[2], "i=")
	if !ok {
		return nil, invalid("SCRAM iteration count is missing")
	}
	iterations, err := strconv.Atoi(iterationsText)
	if err != nil || iterations < 1 || iterations > maxSCRAMIterations {
		return nil, invalid("SCRAM iteration count is outside its supported range")
	}
	return s.proof(message, password, nonce, salt, iterations)
}

func (s *scramClient) proof(message, password, nonce string, salt []byte, iterations int) ([]byte, error) {
	salted, err := pbkdf2.Key(sha256.New, password, salt, iterations, sha256.Size)
	if err != nil {
		return nil, err
	}
	clientKey := mac(salted, "Client Key")
	storedKey := sha256.Sum256(clientKey)
	channel := append([]byte(s.header), s.binding...)
	final := "c=" + base64.StdEncoding.EncodeToString(channel) + ",r=" + nonce
	auth := s.bare + "," + message + "," + final
	clientSignature := mac(storedKey[:], auth)
	for i := range clientKey {
		clientKey[i] ^= clientSignature[i]
	}
	s.expected = mac(mac(salted, "Server Key"), auth)
	return []byte(final + ",p=" + base64.StdEncoding.EncodeToString(clientKey)), nil
}

func (s *scramClient) verify(message string) ([]byte, error) {
	encoded, ok := strings.CutPrefix(message, "v=")
	if !ok {
		return nil, invalid("SCRAM server proof is missing")
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || !hmac.Equal(signature, s.expected) {
		return nil, invalid("SCRAM server proof is invalid")
	}
	s.verified = true
	return []byte{}, nil
}
