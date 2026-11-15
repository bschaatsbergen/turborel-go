// Wire tests use fixed byte sequences and malformed peer input.
package turborel

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

// Verifies: [DESIGN] D2: framing and Query bytes match the reference protocol exactly.
func TestQueryFrame(t *testing.T) {
	e := &encoder{limit: defaultMaxBody}
	e.str("AB")
	e.flag(true)
	var frame bytes.Buffer
	if err := writeFrame(&frame, msgQuery, e); err != nil {
		t.Fatal(err)
	}
	want := []byte{'Q', 7, 0, 0, 0, 2, 0, 0, 0, 'A', 'B', 1}
	if !bytes.Equal(frame.Bytes(), want) {
		t.Fatalf("frame: %x", frame.Bytes())
	}
	kind, d, err := readFrame(&frame, defaultMaxBody)
	if err != nil || kind != msgQuery {
		t.Fatalf("header: %c %v", kind, err)
	}
	if d.str() != "AB" || !d.flag() || d.finish() != nil {
		t.Fatal("query fields")
	}
}

// Verifies: [DESIGN] D2: truncated headers, oversized bodies, invalid flags and UTF-8 are rejected.
func TestMalformedFrames(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{name: "header", data: []byte{'Q'}},
		{name: "body", data: []byte{'Q', 2, 0, 0, 0, 1}},
		{name: "oversized", data: []byte{'Q', 255, 255, 255, 255}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := readFrame(bytes.NewReader(test.data), 64); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	d := &decoder{data: []byte{2}}
	d.flag()
	if d.finish() == nil {
		t.Fatal("invalid flag accepted")
	}
	d = &decoder{data: []byte{1, 0, 0, 0, 255}}
	d.str()
	if d.finish() == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	d = &decoder{data: []byte{0}}
	if d.finish() == nil {
		t.Fatal("trailing bytes accepted")
	}
}

type shortWriter struct{}

// Write simulates a writer that makes no progress without reporting an error.
func (shortWriter) Write([]byte) (int, error) { return 0, nil }

// Verifies: [DESIGN] D2: zero-progress writes and oversized requests fail without spinning or partial frames.
func TestWriteLimits(t *testing.T) {
	if err := writeAll(shortWriter{}, []byte{1}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write: %v", err)
	}
	e := &encoder{limit: 2}
	e.str("long")
	var frame bytes.Buffer
	if err := writeFrame(&frame, msgQuery, e); err == nil || frame.Len() != 0 {
		t.Fatal("oversized frame written")
	}
}

// Verifies: [DESIGN] D2: character metadata includes its length-unit byte before the next column.
func TestCharacterMetadata(t *testing.T) {
	d := &decoder{data: []byte{tagVarying, 40, 0, 0, 0, 0, tagBool}}
	first, second := readType(d, 0), readType(d, 0)
	if err := d.finish(); err != nil {
		t.Fatal(err)
	}
	if first.length != 40 || second.tag != tagBool {
		t.Fatalf("types: %v %v", first, second)
	}
}

// Verifies: [DESIGN] D2: arbitrary frames and values cannot panic or allocate from unvalidated counts.
func FuzzDecode(f *testing.F) {
	f.Add([]byte{msgRows, 4, 0, 0, 0, 0, 0, 0, 0})
	f.Add([]byte{msgDescription, 1, 0, 0, 0, tagBool})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4096 {
			return
		}
		_, d, err := readFrame(bytes.NewReader(data), 4096)
		if err == nil {
			readColumns(d)
			_ = d.finish() // Malformed inputs are expected; the fuzz property is bounded decoding.
		}
		readValue(&decoder{data: data}, 0)
		readType(&decoder{data: data}, 0)
	})
}
