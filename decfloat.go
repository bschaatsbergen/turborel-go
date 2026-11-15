// Decimal floating point keeps decimal digits and exponents out of binary floats.

package turborel

import (
	"math/big"
	"strconv"
	"strings"
)

const minAdjustedExponent = -6143
const maxAdjustedExponent = 6144

// DecFloat is a finite decimal floating-point parameter, for example "1.25e100".
// Its coefficient has at most 38 digits. Results can be scanned into DecFloat.
type DecFloat string

// Scan accepts decimal text and validates its coefficient and exponent.
func (f *DecFloat) Scan(src any) error {
	if f == nil {
		return invalid("nil DecFloat destination")
	}
	var text string
	switch v := src.(type) {
	case string:
		text = v
	case []byte:
		text = string(v)
	default:
		return invalid("DecFloat requires a non-null decimal string")
	}
	if _, _, err := parseDecFloat(text); err != nil {
		return err
	}
	*f = DecFloat(text)
	return nil
}

func parseDecFloat(text string) (*big.Int, int32, error) {
	if len(text) > 2*maxDecimalScale+16 {
		return nil, 0, invalid("invalid decimal float length")
	}
	coefficient, power, hasPower := strings.Cut(strings.ToLower(text), "e")
	var exponent int64
	if hasPower {
		var err error
		exponent, err = strconv.ParseInt(power, 10, 32)
		if err != nil {
			return nil, 0, invalid("invalid decimal float exponent")
		}
	}
	n, scale, err := parseDecimal(coefficient)
	if err != nil {
		return nil, 0, err
	}
	exponent -= int64(scale)
	if n.Sign() == 0 {
		return n, 0, nil
	}
	digits := strings.TrimPrefix(n.String(), "-")
	if len(digits) > maxDecimalScale {
		return nil, 0, invalid("decimal float coefficient exceeds 38 digits")
	}
	adjusted := exponent + int64(len(digits)) - 1
	if adjusted < minAdjustedExponent || adjusted > maxAdjustedExponent {
		return nil, 0, invalid("decimal float exponent is outside the server's range")
	}
	return n, int32(exponent), nil
}

func putDecFloat(e *encoder, text string) {
	n, exponent, err := parseDecFloat(text)
	if err != nil {
		e.err = err
		return
	}
	e.u8(tagDecFloat)
	put128(e, n)
	e.u32(uint32(exponent))
}
