// Parameter values are encoded separately from SQL, using Prepare's types.

package turborel

import (
	"database/sql/driver"
	"math"
	"math/big"
	"strconv"
	"time"
)

func putValue(e *encoder, value any, typ dataType, depth int) {
	if depth > maxDepth {
		e.err = invalid("parameter nesting exceeds limit")
		return
	}
	if value == nil {
		e.u8(tagNull)
		return
	}
	switch v := value.(type) {
	case DecFloat:
		putDecFloat(e, string(v))
	case Decimal:
		putDecimal(e, string(v))
	case Array:
		putParts(e, []any(v), typ, depth, tagArray)
	case Row:
		putParts(e, []any(v), typ, depth, tagRow)
	case Extension:
		e.u8(tagExtension)
		e.str(v.Name)
		e.str(v.Text)
		e.bytes(v.Bytes)
	case Interval:
		putInterval(e, v)
	case time.Time:
		putDatetime(e, v, typ)
	default:
		putBasic(e, value)
	}
}

func putBasic(e *encoder, value any) {
	v, err := driver.DefaultParameterConverter.ConvertValue(value)
	if err != nil {
		e.err = invalid("unsupported parameter type")
		return
	}
	switch v := v.(type) {
	case nil:
		e.u8(tagNull)
	case int64:
		putDecimal(e, strconv.FormatInt(v, 10))
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			e.err = invalid("non-finite float parameter")
			return
		}
		e.u8(tagApprox)
		e.u64(math.Float64bits(v))
	case bool:
		e.u8(tagBool)
		e.flag(v)
	case string:
		e.u8(tagChar)
		e.str(v)
	case []byte:
		e.u8(tagBinary)
		e.bytes(v)
	default:
		e.err = invalid("unsupported converted parameter type")
	}
}

func putDecimal(e *encoder, s string) {
	n, scale, err := parseDecimal(s)
	if err != nil {
		e.err = err
		return
	}
	e.u8(tagExact)
	put128(e, n)
	e.u32(scale)
}

func putParts(e *encoder, parts []any, typ dataType, depth int, tag byte) {
	if typ.tag != tag {
		e.err = invalid("composite parameter does not match its type")
		return
	}
	if len(parts) > defaultMaxBody || tag == tagRow && len(parts) != len(typ.fields) {
		e.err = invalid("composite parameter has invalid cardinality")
		return
	}
	if tag == tagArray && (typ.element == nil || uint64(len(parts)) > uint64(typ.length)) {
		e.err = invalid("array parameter exceeds its cardinality")
		return
	}
	e.u8(tag)
	e.u32(uint32(len(parts)))
	for i, part := range parts {
		var t dataType
		if tag == tagArray {
			t = *typ.element
		} else {
			t = typ.fields[i].typ
		}
		putValue(e, part, t, depth+1)
	}
}

func putInterval(e *encoder, v Interval) {
	n, ok := new(big.Int).SetString(v.Units, 10)
	if !ok || !fits128(n) {
		e.err = invalid("invalid interval units")
		return
	}
	if v.Qualifier[0] > 5 || v.Qualifier[1] > 5 {
		e.err = invalid("invalid interval qualifier")
		return
	}
	e.u8(tagInterval)
	for _, field := range v.Qualifier {
		e.u32(field)
	}
	put128(e, n)
}

func putDatetime(e *encoder, v time.Time, typ dataType) {
	if v.Year() < 1 || v.Year() > 9999 {
		e.err = invalid("datetime year is outside 1..9999")
		return
	}
	_, offset := v.Zone()
	if offset%secondsPerMinute != 0 || offset < -779*secondsPerMinute || offset > 780*secondsPerMinute {
		e.err = invalid("datetime offset is outside the server's range")
		return
	}
	tag := typ.tag
	zoned := tag == tagZonedTime || tag == tagZonedTimestamp
	instant := v.UnixMicro() + unixEpochMicros
	switch tag {
	case tagDate:
		instant = time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, time.UTC).UnixMicro() + unixEpochMicros
		offset = 0
	case tagTime, tagZonedTime:
		tag = tagTime
		instant = (int64(v.Hour()*3600+v.Minute()*60+v.Second()-offset)*microsPerSecond + int64(v.Nanosecond()/1000)) % microsPerDay
		if instant < 0 {
			instant += microsPerDay
		}
	case tagTimestamp, tagZonedTimestamp:
		tag = tagTimestamp
	default:
		e.err = invalid("time.Time requires a datetime parameter")
		return
	}
	e.u8(tag)
	e.flag(zoned)
	e.u8(maxTimePrecision)
	e.u32(uint32(uint16(int16(offset / secondsPerMinute))))
	e.u64(uint64(instant))
}
