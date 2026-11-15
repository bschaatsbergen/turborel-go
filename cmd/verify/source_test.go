// Verifier tests ensure source rules reject defects instead of merely reporting success.
package main

import (
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Verifies: [TOOL] R-SIZE, R-DOC, R-SAFE: representative source violations fail the source gate.
func TestSourceChecks(t *testing.T) {
	cases := []struct{ name, source, rule string }{
		{"valid", "// A fixture.\npackage fixture\n// Work completes.\nfunc Work() {}\n", ""},
		{"unsafe", "// A fixture.\npackage fixture\nimport \"unsafe\"\n", "R-SAFE-1"},
		{"docs", "// A fixture.\npackage fixture\nfunc Work() {}\n", "R-DOC-1"},
		{"trace", "// A fixture.\npackage fixture\n// TestWork checks work.\nfunc TestWork() {}\n", "R-TRACE-1"},
		{"parameters", "// A fixture.\npackage fixture\nfunc work(a,b,c,d,e,f int) {}\n", "R-SIZE-3"},
		{"function", "// A fixture.\npackage fixture\nfunc work() {\n" + strings.Repeat("work()\n", 51) + "}\n", "R-SIZE-1"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			source, err := format.Source([]byte(test.source))
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "fixture.go")
			if err := os.WriteFile(path, source, 0600); err != nil {
				t.Fatal(err)
			}
			err = checkFile(path)
			if test.rule == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.rule) {
				t.Fatalf("expected %s, got %v", test.rule, err)
			}
		})
	}
}

// Verifies: [TOOL] R-COV-2: empty, malformed and below-floor coverage fails; executed statements meet the floor.
func TestCoverageFloor(t *testing.T) {
	cases := []struct {
		profile string
		valid   bool
	}{
		{"mode: atomic\nexample/driver.go:1.1,2.2 10 1\n", true},
		{"mode: atomic\nexample/driver.go:1.1,2.2 10 0\n", false},
		{"mode: atomic\n", false},
		{"mode: atomic\ninvalid\n", false},
		{"mode: atomic\nexample/driver.go:1.1,2.2 x 1\n", false},
	}
	for _, test := range cases {
		path := filepath.Join(t.TempDir(), "coverage.out")
		if err := os.WriteFile(path, []byte(test.profile), 0600); err != nil {
			t.Fatal(err)
		}
		if err := checkCoverage(path); (err == nil) != test.valid {
			t.Fatalf("profile %q: %v", test.profile, err)
		}
	}
}
