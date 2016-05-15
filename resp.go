package rdx

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type Type int

var (
	msgNilStr = []byte("$-1\r\n\r\n")
)

const (
	TNil Type = 1 << iota
	TError
	TArray
	TInt
	TSimpleString
	TBulkString
	TString = TSimpleString | TBulkString
)

type (
	Msg interface {
		Type() Type
		String() string

		io.WriterTo

		Err() error
	}

	niltype struct{}
	Int     int64
	String  []byte
	Array   []Msg
	Error   string

	// Encode-specific types -- when read over the wire, these will still be treated as their simplified types.

	// BulkString is a convenience type for encoding a string as a bulk string. This can be used to skip converting
	// a string to a byte slice when writing a message.
	BulkString string

	// SimpleString explicitly encodes a string as a basic string instead of a bulk string. When read over the wire,
	// all SimpleStrings are received as String to avoid type preferences on strings. If the SimpleString contains the
	// sequence "\r\n", it is automatically promoted to a BulkString to avoid producing an error.
	SimpleString string

	// Float64 encodes a float64 as a bulk string. This is a convenience type for skipping float-to-string
	// conversion.
	Float64 float64
)

func ensure(msg Msg) Msg {
	if msg == nil {
		return Nil
	}
	return msg
}

var Nil niltype

var (
	ErrInvalidError     = errors.New(`rdx: error contains forbidden character`)
	ErrInvalidSimpleStr = errors.New(`rdx: simple string contains forbidden character`)
)

var _ Msg = Error("")

func (e Error) Err() error     { return e }
func (e Error) Error() string  { return string(e) }
func (e Error) Type() Type     { return TError }
func (e Error) String() string { return string(e) }
func (e Error) estlen() int    { return 3 + len(e) }

func (e Error) WriteTo(w io.Writer) (n int64, err error) {
	s := string(e)
	if strings.ContainsAny(s, "\r\n") {
		return 0, ErrInvalidError
	}

	buf := tempbuffer(e.estlen())
	buf.WriteByte('-')
	buf.WriteString(s)
	buf.WriteString("\r\n")

	n, err = buf.WriteTo(w)
	putbuffer(buf)
	return n, err
}

var _ Msg = String(nil)

func (String) Err() error       { return nil }
func (String) Type() Type       { return TBulkString }
func (s String) String() string { return string(s) }
func (s String) estlen() int {
	sz := len(s)
	sz += intlen(int64(sz))
	return 5 + sz
}

func (s String) WriteTo(w io.Writer) (n int64, err error) {
	oct := []byte(s)
	sz := len(oct)
	szlen := intlen(int64(sz))

	buf := tempbuffer(5 + sz + szlen)
	buf.WriteByte('$')
	putint(buf, int64(sz))
	buf.Write(oct)
	buf.WriteString("\r\n")

	n, err = buf.WriteTo(w)
	putbuffer(buf)

	return n, err
}

var _ Msg = niltype{}

func (niltype) Err() error     { return nil }
func (niltype) Type() Type     { return TNil }
func (niltype) String() string { return "<nil>" }
func (niltype) estlen() int    { return 5 }

func (niltype) WriteTo(w io.Writer) (n int64, err error) {
	b := [...]byte{'$', '-', '1', '\r', '\n'}
	in, err := w.Write(b[:])
	return int64(in), err
}

var _ Msg = Int(0)

func (Int) Err() error       { return nil }
func (Int) Type() Type       { return TInt }
func (i Int) String() string { return strconv.FormatInt(int64(i), 10) }
func (i Int) estlen() int    { return 3 + intlen(int64(i)) }

func (i Int) WriteTo(w io.Writer) (n int64, err error) {
	i64 := int64(i)
	buf := tempbuffer(i.estlen())
	buf.WriteByte(':')
	putint(buf, i64)

	n, err = buf.WriteTo(w)
	putbuffer(buf)

	return n, err
}

type estlen interface {
	estlen() int
}

var _ Msg = Array(nil)

func (Array) Err() error { return nil }
func (Array) Type() Type { return TArray }

func (a Array) String() string { return fmt.Sprint([]Msg(a)) }

func (a Array) estlen() int {
	sz := 3 + intlen(int64(len(a)))

	for _, m := range a {
		m = ensure(m)
		if em, ok := m.(estlen); ok {
			sz += em.estlen()
		}
	}

	return sz
}

func (a Array) WriteTo(w io.Writer) (n int64, err error) {
	buf := tempbuffer(a.estlen())

	buf.WriteByte('*')
	putint(buf, int64(len(a)))

	for _, m := range a {
		_, err = ensure(m).WriteTo(buf)
		if err != nil {
			return 0, err
		}
	}

	n, err = buf.WriteTo(w)
	putbuffer(buf)

	return n, err
}

var _ Msg = BulkString("")

func (BulkString) Err() error       { return nil }
func (BulkString) Type() Type       { return TBulkString }
func (s BulkString) String() string { return string(s) }

func (s BulkString) estlen() int {
	sz := len(s)
	sz += intlen(int64(sz))
	return 5 + sz
}

func (s BulkString) WriteTo(w io.Writer) (n int64, err error) {
	sz := len(s)
	szlen := intlen(int64(sz))

	buf := tempbuffer(5 + sz + szlen)
	buf.WriteByte('$')
	putint(buf, int64(sz))
	buf.WriteString(string(s))
	buf.WriteString("\r\n")

	n, err = buf.WriteTo(w)
	putbuffer(buf)

	return n, err
}

var _ Msg = SimpleString("")

func (SimpleString) Err() error       { return nil }
func (SimpleString) Type() Type       { return TSimpleString }
func (s SimpleString) String() string { return string(s) }

func (s SimpleString) estlen() int {
	if strings.ContainsAny(string(s), "\r\n") {
		return BulkString(s).estlen()
	}
	sz := len(s)
	sz += intlen(int64(sz))
	return 5 + sz
}

func (s SimpleString) WriteTo(w io.Writer) (n int64, err error) {
	if strings.ContainsAny(string(s), "\r\n") {
		return BulkString(s).WriteTo(w)
	}

	buf := tempbuffer(s.estlen())
	buf.WriteByte('+')
	buf.WriteString(string(s))
	buf.WriteString("\r\n")

	n, err = buf.WriteTo(w)
	putbuffer(buf)

	return n, err
}

var _ Msg = Float64(0)

func (Float64) Err() error       { return nil }
func (Float64) Type() Type       { return TSimpleString }
func (f Float64) String() string { return strconv.FormatFloat(float64(f), 'f', -1, 64) }
func (Float64) estlen() int      { return 23 }

func (f Float64) WriteTo(w io.Writer) (n int64, err error) {
	var tmp = [32]byte{'+'}
	b := strconv.AppendFloat(tmp[1:], float64(f), 'f', -1, 64)
	b = append(b, "\r\n"...)

	in, err := w.Write(b)
	return int64(in), err
}

func ToFloat(msg Msg) (float64, error) {
	return strconv.ParseFloat(ensure(msg).String(), 64)
}

func IsA(msg Msg, typ Type) bool {
	return ensure(msg).Type()&typ != 0
}

func Write(w io.Writer, msg Msg) (n int, err error) {
	in, err := ensure(msg).WriteTo(w)
	return int(in), err
}
