// Reply values are decoded with bounded nesting and no precision loss.

package turborel

import (
	"database/sql/driver"
	"math"
	"strconv"
	"time"
)

const (
	microsPerSecond   int64 = 1_000_000
	microsPerDay      int64 = 86_400 * microsPerSecond
	unixEpochMicros   int64 = 62_135_596_800 * microsPerSecond
	secondsPerMinute        = 60
	maxTimePrecision        = 6
	lastCalendarMicro int64 = 315537897599999999
)

func readValue(d *decoder, depth int) driver.Value {
	v := readTree(d, depth)
	switch v.(type) {
	case []any, Interval:
		result, err := jsonValue(v)
		if err != nil {
			d.err = err
		}
		return result
	default:
		return v
	}
}

func readTree(d *decoder, depth int) any {
	if depth > maxDepth {
		d.err = invalid("value nesting exceeds limit")
		return nil
	}
	switch tag := d.u8(); tag {
	case tagNull:
		return nil
	case tagChar:
		return d.str()
	case tagBinary:
		return append([]byte{}, d.bytes()...)
	case tagBool:
		return d.flag()
	case tagApprox:
		return math.Float64frombits(d.u64())
	case tagExact:
		n, scale := read128(d), d.u32()
		text, err := decimalText(n, scale)
		if err != nil {
			d.err = err
		}
		return text
	case tagDecFloat:
		n, exponent := read128(d), int32(d.u32())
		return n.String() + "e" + strconv.FormatInt(int64(exponent), 10)
	case tagDate, tagTime, tagTimestamp:
		return readDatetime(d)
	case tagInterval:
		return readInterval(d)
	case tagExtension:
		name, text := d.str(), d.str()
		_ = name // The column describes the type; database/sql receives its display text.
		d.bytes()
		return text
	case tagArray, tagRow:
		values := []any{}
		for range d.count() {
			values = append(values, readTree(d, depth+1))
		}
		return values
	default:
		d.err = invalid("unknown protocol value tag")
		return nil
	}
}

func readInterval(d *decoder) Interval {
	var qualifier [4]uint32
	for i := range qualifier {
		qualifier[i] = d.u32()
	}
	return Interval{Units: read128(d).String(), Qualifier: qualifier}
}

func readDatetime(d *decoder) driver.Value {
	d.flag()
	precision, displacement, instant := d.u8(), d.u32(), int64(d.u64())
	if precision > maxTimePrecision || displacement > math.MaxUint16 {
		d.err = invalid("invalid datetime metadata")
		return nil
	}
	zone := int16(uint16(displacement))
	if zone < -779 || zone > 780 || instant < -microsPerDay || instant > lastCalendarMicro+microsPerDay {
		d.err = invalid("datetime is outside the server's range")
		return nil
	}
	location := time.FixedZone("", int(zone)*secondsPerMinute)
	return time.UnixMicro(instant - unixEpochMicros).In(location)
}

func readRows(d *decoder, columns int) [][]driver.Value {
	rows := [][]driver.Value{}
	for range d.count() {
		n := d.count()
		if n != columns {
			d.err = invalid("row width differs from its description")
			return nil
		}
		row := make([]driver.Value, 0, n)
		for range n {
			row = append(row, readValue(d, 0))
		}
		rows = append(rows, row)
	}
	return rows
}
