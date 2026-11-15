// Column and parameter descriptions determine lossless driver conversions.

package turborel

import (
	"fmt"
	"reflect"
	"time"
)

const (
	tagNull byte = iota
	tagChar
	tagExact
	tagApprox
	tagExtension
	tagVarying
	tagBinary
	tagVarBinary
	tagDate
	tagTime
	tagZonedTime
	tagTimestamp
	tagZonedTimestamp
	tagInterval
	tagBool
	tagDecFloat
	tagArray
	tagRow
)

type dataType struct {
	tag       byte
	length    uint32
	precision uint32
	scale     uint32
	name      string
	element   *dataType
	fields    []column
	qualifier [4]uint32
}

type column struct {
	name    string
	typ     dataType
	notNull bool
}

func readType(d *decoder, depth int) dataType {
	t := dataType{tag: d.u8()}
	if depth > maxDepth {
		d.err = invalid("type nesting exceeds limit")
		return t
	}
	switch t.tag {
	case tagChar, tagVarying:
		t.length = d.u32()
		if d.u8() > 1 {
			d.err = invalid("unknown character length unit")
		}
	case tagBinary, tagVarBinary:
		t.length = d.u32()
	case tagExact:
		t.precision, t.scale = d.u32(), d.u32()
	case tagApprox, tagDecFloat, tagDate, tagTime, tagZonedTime, tagTimestamp, tagZonedTimestamp:
		t.precision = d.u32()
	case tagBool:
	case tagInterval:
		for i := range t.qualifier {
			t.qualifier[i] = d.u32()
		}
	case tagExtension:
		t.name = d.str()
		for range d.count() {
			d.u32()
		}
	case tagArray:
		t.length = d.u32()
		element := readType(d, depth+1)
		t.element = &element
	case tagRow:
		for range d.count() {
			name := d.str()
			t.fields = append(t.fields, column{name: name, typ: readType(d, depth+1)})
		}
	default:
		d.err = invalid("unknown protocol type tag")
	}
	return t
}

func readColumns(d *decoder) []column {
	cols := []column{}
	for range d.count() {
		name := ""
		if d.flag() {
			name = d.str()
		}
		cols = append(cols, column{name: name, typ: readType(d, 0), notNull: d.flag()})
	}
	return cols
}

func (t dataType) databaseName() string {
	switch t.tag {
	case tagChar:
		return "CHARACTER"
	case tagVarying:
		return "CHARACTER VARYING"
	case tagBinary:
		return "BINARY"
	case tagVarBinary:
		return "BINARY VARYING"
	case tagExact:
		return "NUMERIC"
	case tagApprox:
		return "FLOAT"
	case tagDecFloat:
		return "DECFLOAT"
	case tagBool:
		return "BOOLEAN"
	case tagDate:
		return "DATE"
	case tagTime:
		return "TIME"
	case tagZonedTime:
		return "TIME WITH TIME ZONE"
	case tagTimestamp:
		return "TIMESTAMP"
	case tagZonedTimestamp:
		return "TIMESTAMP WITH TIME ZONE"
	case tagInterval:
		return "INTERVAL"
	case tagExtension:
		return t.name
	case tagArray:
		return "ARRAY"
	case tagRow:
		return "ROW"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", t.tag)
	}
}

func (t dataType) scanType() reflect.Type {
	switch t.tag {
	case tagApprox:
		return reflect.TypeFor[float64]()
	case tagBool:
		return reflect.TypeFor[bool]()
	case tagBinary, tagVarBinary:
		return reflect.TypeFor[[]byte]()
	case tagDate, tagTime, tagZonedTime, tagTimestamp, tagZonedTimestamp:
		return reflect.TypeFor[time.Time]()
	default:
		return reflect.TypeFor[string]()
	}
}
