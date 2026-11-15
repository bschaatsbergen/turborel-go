// SCRAM tests use the reference implementation's published example exchange.
package turborel

import (
	"strings"
	"testing"
)

func exampleSCRAM() *scramClient {
	return &scramClient{nonce: "rOprNGfwEbeRWgbNEkqO", header: "n,,", bare: "n=user,r=rOprNGfwEbeRWgbNEkqO"}
}

const exampleFirst = "r=rOprNGfwEbeRWgbNEkqO%hvYDpWUa2RaTCAfuxFIlj)hNlF$k0,s=W22ZaJ0SNY7soEsUEjb6gQ==,i=4096"

// Verifies: [DESIGN] D5: client and server proofs match upstream protocol/src/scram/tests.rs byte for byte.
func TestSCRAMProof(t *testing.T) {
	s := exampleSCRAM()
	answer, err := s.answer(exampleFirst, "pencil")
	if err != nil {
		t.Fatal(err)
	}
	want := "c=biws,r=rOprNGfwEbeRWgbNEkqO%hvYDpWUa2RaTCAfuxFIlj)hNlF$k0,p=dHzbZapWIk4jUhN+Ute9ytag9zjfMHgsqmmiz7AndVQ="
	if string(answer) != want {
		t.Fatalf("proof: %s", answer)
	}
	if _, err := s.answer("v=6rriTRBi23WpRR/wtup+mMhUZUn/dB5nLTJRsjl95G4=", "pencil"); err != nil {
		t.Fatal(err)
	}
	if !s.verified {
		t.Fatal("not verified")
	}
	if _, err := s.answer(exampleFirst, "pencil"); err == nil {
		t.Fatal("replay accepted")
	}
}

// Verifies: [DESIGN] D5: malformed challenges, excessive work and false server proofs fail closed.
func TestSCRAMRefusals(t *testing.T) {
	for _, message := range []string{
		"", "r=other,s=AA==,i=1", "r=rOprNGfwEbeRWgbNEkqO,s=AA==,i=1",
		strings.Replace(exampleFirst, "i=4096", "i=0", 1),
		strings.Replace(exampleFirst, "i=4096", "i=2000001", 1),
		exampleFirst + ",x=extension",
	} {
		t.Run(message, func(t *testing.T) {
			if _, err := exampleSCRAM().answer(message, "pencil"); err == nil {
				t.Fatal("challenge accepted")
			}
		})
	}
	s := exampleSCRAM()
	if _, err := s.answer(exampleFirst, "pencil"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.answer("v=AAAA", "pencil"); err == nil {
		t.Fatal("false proof accepted")
	}
}
