// Framing and bounded field access implement the native protocol, upstream D45.

package turborel

import (
	"encoding/binary"
	"io"
	"unicode/utf8"
)

const (
	protocolVersion      = 1
	headerBytes          = 5
	cancelKeyBytes       = 16
	defaultMaxBody       = 16 * 1024 * 1024
	maxDepth             = 32
	msgHello        byte = 'H'
	msgAnswer       byte = 'A'
	msgQuery        byte = 'Q'
	msgPrepare      byte = 'P'
	msgExecute      byte = 'E'
	msgDeallocate   byte = 'D'
	msgCancel       byte = 'Z'
	msgChallenge    byte = 'C'
	msgReady        byte = 'R'
	msgPrepared     byte = 'P'
	msgDescription  byte = 'D'
	msgRows         byte = 'B'
	msgComplete     byte = 'K'
	msgError        byte = 'E'
)

type decoder struct {
	data []byte
	err  error
}

func (d *decoder) take(n int) []byte {
	if d.err != nil {
		return nil
	}
	if n < 0 || n > len(d.data) {
		d.err = invalid("truncated protocol field")
		return nil
	}
	v := d.data[:n]
	d.data = d.data[n:]
	return v
}

func (d *decoder) u8() byte {
	b := d.take(1)
	if len(b) == 0 {
		return 0
	}
	return b[0]
}

func (d *decoder) u32() uint32 {
	b := d.take(4)
	if len(b) != 4 {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}

func (d *decoder) u64() uint64 {
	b := d.take(8)
	if len(b) != 8 {
		return 0
	}
	return binary.LittleEndian.Uint64(b)
}

func (d *decoder) count() int {
	n := d.u32()
	if uint64(n) > uint64(len(d.data)) {
		d.err = invalid("protocol count exceeds remaining bytes")
		return 0
	}
	return int(n)
}

func (d *decoder) bytes() []byte { return d.take(d.count()) }

func (d *decoder) str() string {
	b := d.bytes()
	if !utf8.Valid(b) {
		d.err = invalid("protocol text is not UTF-8")
	}
	return string(b)
}

func (d *decoder) flag() bool {
	b := d.u8()
	if b > 1 {
		d.err = invalid("protocol flag is neither zero nor one")
	}
	return b == 1
}

func (d *decoder) finish() error {
	if d.err != nil {
		return d.err
	}
	if len(d.data) != 0 {
		return invalid("protocol message has trailing bytes")
	}
	return nil
}

type encoder struct {
	data  []byte
	err   error
	limit int
}

func (e *encoder) put(b []byte) {
	if e.err != nil {
		return
	}
	if len(b) > e.limit-len(e.data) {
		e.err = invalid("request exceeds maximum frame size")
		return
	}
	e.data = append(e.data, b...)
}

func (e *encoder) u8(v byte)    { e.put([]byte{v}) }
func (e *encoder) u32(v uint32) { e.put(binary.LittleEndian.AppendUint32(nil, v)) }
func (e *encoder) u64(v uint64) { e.put(binary.LittleEndian.AppendUint64(nil, v)) }
func (e *encoder) flag(v bool) {
	if v {
		e.u8(1)
		return
	}
	e.u8(0)
}

func (e *encoder) bytes(v []byte) {
	if uint64(len(v)) > uint64(defaultMaxBody) {
		e.err = invalid("field exceeds maximum size")
		return
	}
	e.u32(uint32(len(v)))
	e.put(v)
}

func (e *encoder) str(v string) {
	if !utf8.ValidString(v) {
		e.err = invalid("request text is not UTF-8")
		return
	}
	e.bytes([]byte(v))
}

func readFrame(r io.Reader, limit uint32) (byte, *decoder, error) {
	var header [headerBytes]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	n := binary.LittleEndian.Uint32(header[1:])
	if n > limit {
		return 0, nil, invalid("response exceeds maximum frame size")
	}
	// ReadAll grows with received bytes rather than trusting an unreceived body.
	b, err := io.ReadAll(io.LimitReader(r, int64(n)))
	if err != nil {
		return 0, nil, err
	}
	if uint64(len(b)) != uint64(n) {
		return 0, nil, io.ErrUnexpectedEOF
	}
	return header[0], &decoder{data: b}, nil
}

func writeFrame(w io.Writer, kind byte, e *encoder) error {
	if e.err != nil {
		return e.err
	}
	header := []byte{kind}
	header = binary.LittleEndian.AppendUint32(header, uint32(len(e.data)))
	if err := writeAll(w, header); err != nil {
		return err
	}
	return writeAll(w, e.data)
}

func writeAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		if n <= 0 || n > len(b) {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}
