// Values preserve exact numbers and describe the protocol's composite values.

package turborel

import (
	"database/sql/driver"
	"encoding/json"
	"math/big"
	"slices"
	"strconv"
	"strings"
)

const (
	integer128Bytes = 16
	integer128Bits  = 128
	maxDecimalScale = 38
)

// Decimal is an exact decimal parameter, written without an exponent.
// Decimal results are returned as strings and can be scanned into Decimal.
type Decimal string

// Scan reads an exact decimal without converting it through floating point.
func (d *Decimal) Scan(src any) error {
	if d == nil {
		return invalid("nil Decimal destination")
	}
	var text string
	switch v := src.(type) {
	case string:
		text = v
	case []byte:
		text = string(v)
	case int64:
		text = strconv.FormatInt(v, 10)
	default:
		return invalid("Decimal requires a non-null decimal string")
	}
	if _, _, err := parseDecimal(text); err != nil {
		return err
	}
	*d = Decimal(text)
	return nil
}

// Extension is an opaque plug-in value. Text is its server-provided rendering;
// Bytes is its original encoding. Parameters can send it back unchanged.
type Extension struct {
	// Name identifies the server's plug-in type.
	Name string
	// Text is the server's human-readable representation.
	Text string
	// Bytes holds the plug-in's original binary representation.
	Bytes []byte
}

// Interval is a signed count of months or microseconds with its wire qualifier:
// start field, end field, leading precision, and fractional precision.
// Field numbers are YEAR=0 through SECOND=5. Results are JSON strings of this form.
type Interval struct {
	// Units is the signed integer count, represented without precision loss.
	Units string
	// Qualifier describes start, end, leading precision and fractional precision.
	Qualifier [4]uint32
}

// Array is an array parameter whose element type comes from Prepare.
type Array []any

// Row is a row parameter whose field types come from Prepare.
type Row []any

func parseDecimal(s string) (*big.Int, uint32, error) {
	if len(s) == 0 || len(s) > 2*maxDecimalScale+3 {
		return nil, 0, invalid("invalid decimal length")
	}
	whole, fraction, dot := strings.Cut(s, ".")
	negative := strings.HasPrefix(whole, "-")
	if strings.HasPrefix(whole, "-") || strings.HasPrefix(whole, "+") {
		whole = whole[1:]
	}
	if whole == "" || dot && fraction == "" {
		return nil, 0, invalid("decimal requires digits")
	}
	digits := whole + fraction
	for _, digit := range digits {
		if digit < '0' || digit > '9' {
			return nil, 0, invalid("invalid decimal digit")
		}
	}
	if len(fraction) > maxDecimalScale {
		return nil, 0, invalid("decimal scale exceeds 38")
	}
	n, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return nil, 0, invalid("invalid decimal")
	}
	if negative {
		n.Neg(n)
	}
	if !fits128(n) {
		return nil, 0, invalid("decimal coefficient exceeds signed 128 bits")
	}
	return n, uint32(len(fraction)), nil
}

func fits128(n *big.Int) bool {
	bound := new(big.Int).Lsh(big.NewInt(1), integer128Bits-1)
	if n.Sign() >= 0 {
		return n.Cmp(bound) < 0
	}
	return n.Cmp(bound.Neg(bound)) >= 0
}

func read128(d *decoder) *big.Int {
	b := slices.Clone(d.take(integer128Bytes))
	if len(b) != integer128Bytes {
		return new(big.Int)
	}
	negative := b[integer128Bytes-1]&0x80 != 0
	slices.Reverse(b)
	n := new(big.Int).SetBytes(b)
	if negative {
		n.Sub(n, new(big.Int).Lsh(big.NewInt(1), integer128Bits))
	}
	return n
}

func put128(e *encoder, n *big.Int) {
	if !fits128(n) {
		e.err = invalid("coefficient exceeds signed 128 bits")
		return
	}
	u := new(big.Int).Set(n)
	if u.Sign() < 0 {
		u.Add(u, new(big.Int).Lsh(big.NewInt(1), integer128Bits))
	}
	b := make([]byte, integer128Bytes)
	u.FillBytes(b)
	slices.Reverse(b)
	e.put(b)
}

func decimalText(n *big.Int, scale uint32) (string, error) {
	if scale > maxDecimalScale {
		return "", invalid("decimal scale exceeds 38")
	}
	negative := n.Sign() < 0
	digits := new(big.Int).Abs(n).String()
	if scale > 0 {
		width := int(scale)
		if len(digits) <= width {
			digits = strings.Repeat("0", width+1-len(digits)) + digits
		}
		at := len(digits) - width
		digits = digits[:at] + "." + digits[at:]
	}
	if negative {
		digits = "-" + digits
	}
	return digits, nil
}

func jsonValue(v any) (driver.Value, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, invalid("composite value cannot be represented as JSON")
	}
	return string(b), nil
}
