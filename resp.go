package rdx

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type Type uint

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

// Msg is any type that can be encoded as a resp message.
type Msg interface {
	Type() Type
	String() string

	io.WriterTo
}

// ErrMsg is any type that can be encoded as a resp message that represents an error.
type ErrMsg interface {
	Msg
	error
}

// ToError converts the Msg, m, to an ErrMsg if it is an ErrMsg. Otherwise, it returns nil.
func ToError(m Msg) ErrMsg {
	err, _ := m.(ErrMsg)
	return err
}

type nilmsg int
type Int int64
type String []byte
type Array []Msg
type Error string

// Encode-specific types -- when read over the wire, these will still be treated as their
// simplified types.

// BulkString is a convenience type for encoding a string as a bulk string. This can be used
// to skip converting a string to a byte slice when writing a message.
type BulkString string

// SimpleString explicitly encodes a string as a basic string instead of a bulk string. When
// read over the wire, all SimpleStrings are received as String to avoid type preferences on
// strings. If the SimpleString contains the sequence "\r\n", it is automatically promoted
// to a BulkString to avoid producing an error.
type SimpleString string

// Float64 encodes a float64 as a bulk string. This is a convenience type for skipping
// float-to-string conversion.
type Float64 float64

func ensure(msg Msg) Msg {
	if msg == nil {
		return Nil
	}
	return msg
}

// Nil is a Msg representing a nil value.
const Nil nilmsg = 0

var (
	ErrInvalidError     = errors.New(`rdx: error contains forbidden character`)
	ErrInvalidSimpleStr = errors.New(`rdx: simple string contains forbidden character`)
)

var _ Msg = Error("")

func (e Error) Error() string  { return string(e) }
func (e Error) Type() Type     { return TError }
func (e Error) String() string { return string(e) }
func (e Error) estlen() int    { return 3 + len(e) }

func (e Error) writeTo(buf *bytes.Buffer) (n int64) {
	buf.WriteByte('-')
	buf.WriteString(string(e))
	buf.WriteString("\r\n")
	return int64(len(e) + 3)
}

func (e Error) WriteTo(w io.Writer) (n int64, err error) {
	s := string(e)
	if strings.ContainsAny(s, "\r\n") {
		return 0, ErrInvalidError
	}

	if buf, ok := w.(*bytes.Buffer); ok {
		return e.writeTo(buf), nil
	}

	buf := tempbuffer(e.estlen())
	e.writeTo(buf)

	n, err = buf.WriteTo(w)
	putbuffer(buf)

	return n, err
}

var _ Msg = String(nil)

func (String) Type() Type       { return TBulkString }
func (s String) String() string { return string(s) }
func (s String) estlen() int {
	sz := len(s)
	sz += intlen(int64(sz))
	return 5 + sz
}

func (s String) writeTo(buf *bytes.Buffer) (n int64) {
	n = int64(len(s))
	n += putint(buf, '$', n) + 2
	buf.WriteString(string(s))
	buf.WriteString("\r\n")
	return n
}

func (s String) WriteTo(w io.Writer) (n int64, err error) {
	if buf, ok := w.(*bytes.Buffer); ok {
		return s.writeTo(buf), nil
	}

	buf := tempbuffer(s.estlen())
	s.writeTo(buf)

	n, err = buf.WriteTo(w)
	putbuffer(buf)

	return n, err
}

var _ Msg = Nil

var nilmsgBytes = [...]byte{'$', '-', '1', '\r', '\n'}

func (nilmsg) Type() Type     { return TNil }
func (nilmsg) String() string { return "<nil>" }
func (nilmsg) estlen() int    { return len(nilmsgBytes) }

func (nilmsg) WriteTo(w io.Writer) (n int64, err error) {
	b := nilmsgBytes // copy
	in, err := w.Write(b[:])
	return int64(in), err
}

var _ Msg = Int(0)

func (Int) Type() Type       { return TInt }
func (i Int) String() string { return strconv.FormatInt(int64(i), 10) }
func (i Int) estlen() int    { return 3 + intlen(int64(i)) }

func (i Int) WriteTo(w io.Writer) (n int64, err error) {
	i64 := int64(i)
	buf := tempbuffer(i.estlen())
	putint(buf, ':', i64)

	n, err = buf.WriteTo(w)
	putbuffer(buf)

	return n, err
}

type estlen interface {
	estlen() int
}

var _ Msg = Array(nil)

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

func (a Array) writeTo(buf *bytes.Buffer) (err error) {
	putint(buf, '*', int64(len(a)))
	for _, m := range a {
		switch m := ensure(m).(type) {
		case Array:
			err = m.writeTo(buf)
		default:
			_, err = m.WriteTo(buf)
		}

		if err != nil {
			return err
		}
	}
	return nil
}

func (a Array) WriteTo(w io.Writer) (n int64, err error) {
	buf := tempbuffer(a.estlen())
	defer putbuffer(buf)
	if err = a.writeTo(buf); err != nil {
		return 0, err
	}

	return buf.WriteTo(w)
}

var _ Msg = BulkString("")

func (BulkString) Type() Type       { return TBulkString }
func (s BulkString) String() string { return string(s) }

func (s BulkString) estlen() int {
	sz := len(s)
	sz += intlen(int64(sz))
	return 5 + sz
}

func (s BulkString) writeTo(buf *bytes.Buffer) (n int64, err error) {
	n = int64(len(s))
	n += putint(buf, '$', n) + 2
	buf.WriteString(string(s))
	buf.WriteString("\r\n")
	return n, nil
}

func (s BulkString) WriteTo(w io.Writer) (n int64, err error) {
	if buf, ok := w.(*bytes.Buffer); ok {
		return s.writeTo(buf)
	}

	buf := tempbuffer(s.estlen())
	s.writeTo(buf)

	n, err = buf.WriteTo(w)
	putbuffer(buf)

	return n, err
}

var _ Msg = SimpleString("")

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

func (s SimpleString) writeTo(buf *bytes.Buffer) (n int64, err error) {
	buf.WriteByte('+')
	buf.WriteString(string(s))
	buf.WriteString("\r\n")
	return int64(len(s) + 3), nil
}

func (s SimpleString) WriteTo(w io.Writer) (n int64, err error) {
	if strings.ContainsAny(string(s), "\r\n") {
		return BulkString(s).WriteTo(w)
	} else if buf, ok := w.(*bytes.Buffer); ok {
		return s.writeTo(buf)
	}

	buf := tempbuffer(s.estlen())
	s.writeTo(buf)
	n, err = buf.WriteTo(w)
	putbuffer(buf)

	return n, err
}

var _ Msg = Float64(0)

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
	return ensure(msg).Type()&typ != 0 && typ != 0
}

func Write(w io.Writer, msg Msg) (n int, err error) {
	in, err := ensure(msg).WriteTo(w)
	return int(in), err
}
