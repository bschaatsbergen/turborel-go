// Value tests cover signed boundaries, precision and datetime encodings.
package turborel

import (
	"bytes"
	"math/big"
	"testing"
	"time"
)

// Verifies: [DESIGN] D2: exact parameters retain all digits, scales and signed 128-bit boundary values.
func TestExactValues(t *testing.T) {
	for _, text := range []string{"0", "-1", "0.00000000000000000000000000000000000001", "12345678901234567890.12",
		"170141183460469231731687303715884105727", "-170141183460469231731687303715884105728"} {
		t.Run(text, func(t *testing.T) {
			e := &encoder{limit: defaultMaxBody}
			putDecimal(e, text)
			if e.err != nil {
				t.Fatal(e.err)
			}
			d := &decoder{data: e.data}
			got := readValue(d, 0)
			if d.finish() != nil || got != text {
				t.Fatalf("value: %v %v", got, d.err)
			}
		})
	}
	for _, text := range []string{"", ".", "1.", "1e2", "1.2.3", "--1", "-+1", "+-1", "170141183460469231731687303715884105728"} {
		t.Run("invalid "+text, func(t *testing.T) {
			if _, _, err := parseDecimal(text); err == nil {
				t.Fatal("invalid decimal accepted")
			}
		})
	}
}

// Verifies: [DESIGN] D2: signed coefficients use two's-complement little-endian bytes.
func TestNegativeCoefficientBytes(t *testing.T) {
	e := &encoder{limit: defaultMaxBody}
	put128(e, big.NewInt(-1))
	if !bytes.Equal(e.data, bytes.Repeat([]byte{255}, 16)) {
		t.Fatalf("coefficient: %x", e.data)
	}
}

// Verifies: [DESIGN] D2: timestamp parameters preserve UTC instants and offsets without Unix-epoch confusion.
func TestDatetimeValues(t *testing.T) {
	for _, offset := range []int{-12 * 3600, 0, 13 * 3600} {
		t.Run(time.FixedZone("", offset).String(), func(t *testing.T) {
			value := time.Date(2024, 2, 29, 12, 34, 56, 123456000, time.FixedZone("", offset))
			e := &encoder{limit: defaultMaxBody}
			putDatetime(e, value, dataType{tag: tagZonedTimestamp})
			if e.err != nil {
				t.Fatal(e.err)
			}
			d := &decoder{data: e.data}
			got, ok := readValue(d, 0).(time.Time)
			if !ok || d.finish() != nil || !got.Equal(value) {
				t.Fatalf("datetime: %v %v", got, d.err)
			}
		})
	}
}

// Verifies: [DESIGN] D2: every supported basic parameter produces a valid wire value and nil remains SQL NULL.
func TestBasicValues(t *testing.T) {
	cases := []struct {
		name  string
		value any
	}{
		{name: "null"}, {name: "integer", value: int32(42)}, {name: "float", value: 1.25},
		{name: "boolean", value: true}, {name: "text", value: "é'"}, {name: "binary", value: []byte{0, 255}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			e := &encoder{limit: defaultMaxBody}
			putValue(e, test.value, dataType{}, 0)
			if e.err != nil {
				t.Fatal(e.err)
			}
			d := &decoder{data: e.data}
			got := readValue(d, 0)
			if err := d.finish(); err != nil {
				t.Fatal(err)
			}
			if test.value == nil && got != nil {
				t.Fatalf("null: %v", got)
			}
		})
	}
}

// Verifies: [DESIGN] D2: composite output preserves nested structure, exact text and NULL values.
func TestNestedCompositeValues(t *testing.T) {
	element := dataType{tag: tagExact, precision: 38}
	inner := dataType{tag: tagArray, length: 3, element: &element}
	outer := dataType{tag: tagArray, length: 2, element: &inner}
	e := &encoder{limit: defaultMaxBody}
	putValue(e, Array{Array{Decimal("123.45"), nil}, Array{}}, outer, 0)
	if e.err != nil {
		t.Fatal(e.err)
	}
	d := &decoder{data: e.data}
	if got := readValue(d, 0); got != `[["123.45",null],[]]` || d.finish() != nil {
		t.Fatalf("nested value: %v %v", got, d.err)
	}
}

// Verifies: [DESIGN] D2: malformed parameters fail before a request is sent.
func TestInvalidValues(t *testing.T) {
	cases := []struct {
		value any
		typ   dataType
	}{
		{value: Array{1}, typ: dataType{tag: tagArray}},
		{value: Row{1}, typ: dataType{tag: tagRow}},
		{value: Interval{Units: "bad"}},
		{value: Interval{Units: "1", Qualifier: [4]uint32{6}}},
		{value: time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC), typ: dataType{tag: tagDate}},
		{value: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), typ: dataType{tag: tagChar}},
		{value: make(chan int)},
	}
	for _, test := range cases {
		e := &encoder{limit: defaultMaxBody}
		putValue(e, test.value, test.typ, 0)
		if e.err == nil {
			t.Fatalf("accepted %T", test.value)
		}
	}
}

// Verifies: [DESIGN] D2: decimal floating point preserves large exponents and refuses values outside the server's range.
func TestDecFloatValues(t *testing.T) {
	for _, text := range []string{"1.25e100", "-1e-6143", "9e6144", "0e2147483647"} {
		e := &encoder{limit: defaultMaxBody}
		putValue(e, DecFloat(text), dataType{tag: tagDecFloat}, 0)
		if e.err != nil {
			t.Fatal(e.err)
		}
		d := &decoder{data: e.data}
		var value DecFloat
		if err := value.Scan(readValue(d, 0)); err != nil {
			t.Fatal(err)
		}
		if err := d.finish(); err != nil {
			t.Fatal(err)
		}
	}
	for _, text := range []string{"NaN", "Inf", "1e6145", "1e-6144", "1e", "1e1e1"} {
		if _, _, err := parseDecFloat(text); err == nil {
			t.Fatalf("accepted %s", text)
		}
	}
}

// Verifies: [DESIGN] D2: row fields preserve their declared order and nested values.
func TestRowValues(t *testing.T) {
	typ := dataType{tag: tagRow, fields: []column{{name: "N", typ: dataType{tag: tagExact}}, {name: "B", typ: dataType{tag: tagBool}}}}
	e := &encoder{limit: defaultMaxBody}
	putValue(e, Row{42, true}, typ, 0)
	if e.err != nil {
		t.Fatal(e.err)
	}
	d := &decoder{data: e.data}
	if got := readValue(d, 0); got != `["42",true]` || d.finish() != nil {
		t.Fatalf("row: %v %v", got, d.err)
	}
}

// Verifies: [DESIGN] D2: valid local datetime boundaries can have a UTC instant in the neighboring year.
func TestDatetimeCalendarBoundaries(t *testing.T) {
	values := []time.Time{
		time.Date(1, 1, 1, 0, 0, 0, 0, time.FixedZone("", 13*3600)),
		time.Date(9999, 12, 31, 23, 59, 59, 999999000, time.FixedZone("", -12*3600)),
	}
	for _, value := range values {
		e := &encoder{limit: defaultMaxBody}
		putDatetime(e, value, dataType{tag: tagZonedTimestamp})
		if e.err != nil {
			t.Fatal(e.err)
		}
		d := &decoder{data: e.data}
		got, ok := readValue(d, 0).(time.Time)
		if !ok || d.finish() != nil || !got.Equal(value) {
			t.Fatalf("boundary: %v %v", got, d.err)
		}
	}
}
