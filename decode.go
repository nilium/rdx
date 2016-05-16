package rdx

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
)

var (
	ErrMissingPrefix = errors.New("rdx: missing type prefix")
	ErrMissingCRLF   = errors.New("rdx: missing CRLF sequence")
	ErrIntRange      = errors.New("rdx: integer out of range of int64")
	ErrBadLength     = errors.New("rdx: invalid length")
	ErrInvalidInt    = errors.New("rdx: malformed integer / length")
	ErrEmptyInt      = errors.New("rdx: empty integer / length")
	ErrInvalidLength = errors.New("rdx: invalid length")
)

type InvalidPrefixError byte

func (c InvalidPrefixError) Error() string {
	return fmt.Sprintf("rdx: invalid message prefix %q", rune(c))
}

// A bytesReader is any reader that supports reading up to and including the delim byte. It must
// function exactly as defined by the (*bufio.Reader).ReadBytes function.
type bytesReader interface {
	io.Reader
	io.ByteReader

	ReadBytes(delim byte) (line []byte, err error)
}

type Reader struct {
	r bytesReader
}

func NewReader(r io.Reader) *Reader {
	var br bytesReader
	if ir, ok := r.(bytesReader); ok {
		br = ir
	} else {
		br = bufio.NewReader(r)
	}

	return &Reader{r: br}
}

func parseInt(b []byte) (n int64, err error) {
	var (
		sc  int64 = 1
		off       = 0
	)

	if b[0] == '-' {
		off++
		sc = -1
	}

	for _, oct := range b[off:] {
		if oct < '0' || oct > '9' {
			return 0, ErrInvalidInt
		}
		p := n
		n = n*10 + int64(oct-'0')*sc
		if (sc == 1 && p > n) || (sc == -1 && p < n) {
			return 0, ErrIntRange
		}
	}

	return n, nil
}

var crlf = []byte{'\r', '\n'}

func (r *Reader) readInt(head []byte) (Int, error) {
	length := len(head)
	if length == 3 {
		return 0, ErrEmptyInt
	}

	n, err := parseInt(head[1 : length-2])
	return Int(n), err
}

func (r *Reader) readBulkString(head []byte) (Msg, error) {
	length, err := r.readInt(head)
	if err != nil {
		if err == ErrInvalidInt {
			err = ErrInvalidLength
		}
		return nil, err
	}

	if length == -1 {
		return Nil, nil
	} else if length < 0 {
		return nil, ErrInvalidLength
	}

	buf := make([]byte, length+2)
	r.r.Read(buf)
	if !bytes.HasSuffix(buf, crlf) {
		return nil, ErrMissingCRLF
	}

	if length == 0 {
		return String(nil), nil
	}

	sep := len(buf) - 2
	return String(buf[:sep:sep]), nil
}

func (r *Reader) readSimpleString(head []byte) (String, error) {
	n := len(head) - 2
	return String(head[1:n:n]), nil
}

func (r *Reader) readArray(head []byte) (Msg, error) {
	length, err := r.readInt(head)
	if err != nil {
		if err == ErrInvalidInt {
			err = ErrInvalidLength
		}
		return nil, err
	}

	if length == -1 {
		return Nil, nil
	} else if length < 0 {
		return nil, ErrInvalidLength
	} else if length == 0 {
		return Array(nil), nil
	}

	ary := make([]Msg, length)
	for i := range ary {
		ary[i], err = r.Read()
		if err != nil {
			return nil, err
		}
	}

	return Array(ary), nil
}

func (r *Reader) readError(head []byte) (Error, error) {
	n := len(head) - 2
	return Error(string(head[1:n])), nil
}

func (r *Reader) Read() (Msg, error) {
	head, err := r.r.ReadBytes('\n')
	if err != nil {
		return nil, err
	} else if !bytes.HasSuffix(head, crlf) {
		return nil, ErrMissingCRLF
	} else if len(head) == 2 {
		return nil, ErrMissingPrefix
	}

	switch head[0] {
	case '-':
		return r.readError(head)
	case '+':
		return r.readSimpleString(head)
	case ':':
		val, err := r.readInt(head)
		if err != nil {
			// Special case: readInt returns Int, a value type, so cannot return nil.
			// Make it nil here.
			return nil, err
		}
		return val, nil
	case '$':
		return r.readBulkString(head)
	case '*':
		return r.readArray(head)
	default:
		return nil, InvalidPrefixError(head[0])
	}
}
