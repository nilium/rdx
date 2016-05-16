package rdx_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"strings"
	"testing"

	"go.spiff.io/rdx"
)

type enctest struct {
	msg    rdx.Msg
	result string
	err    error
}

func (e *enctest) eval(t *testing.T, nth int) {
	var buf bytes.Buffer

	try := func(w io.Writer) {
		n, err := rdx.Write(w, e.msg)
		if n != len(e.result) {
			t.Errorf("[%d ; %T] Written n = %d; want %d", nth, e.msg, n, len(e.result))
		}

		if (e.err != nil) != (err != nil) || (e.err != nil && err != nil && e.err.Error() != err.Error()) {
			t.Errorf("[%d ; %T] Written err = %v; want %v", nth, e.msg, err, e.err)
		}

		if e.result != buf.String() {
			t.Errorf("[%d ; %T] Wrote %q; want %q", nth, e.msg, buf.String(), e.result)
		}
	}

	try(&buf)
	try(ioutil.Discard)
}

func TestWrite_encoding(t *testing.T) {
	table := []enctest{
		{nil, "$-1\r\n", nil},
		{rdx.Nil, "$-1\r\n", nil},

		{rdx.Error("nonempty"), "-nonempty\r\n", nil},
		{rdx.Error("KIND nonempty"), "-KIND nonempty\r\n", nil},
		{rdx.Error(""), "-\r\n", nil},
		{rdx.Error("\r"), "", rdx.ErrInvalidError},
		{rdx.Error("\n"), "", rdx.ErrInvalidError},

		{rdx.Int(12345), ":12345\r\n", nil},
		{rdx.Int(0), ":0\r\n", nil},
		{rdx.Int(-12345), ":-12345\r\n", nil},

		{rdx.String("foo bar baz quux"), "$16\r\nfoo bar baz quux\r\n", nil},
		{rdx.String(""), "$0\r\n\r\n", nil},
		{rdx.String([]byte{}), "$0\r\n\r\n", nil},
		{rdx.String([]byte{1, 2, 3}), "$3\r\n\x01\x02\x03\r\n", nil},

		{rdx.SimpleString(""), "+\r\n", nil},
		{rdx.SimpleString("hello world"), "+hello world\r\n", nil},
		{rdx.SimpleString("\n"), "$1\r\n\n\r\n", nil},
		{rdx.SimpleString("\r"), "$1\r\n\r\r\n", nil},

		{rdx.Array([]rdx.Msg{nil, rdx.Nil}), "*2\r\n$-1\r\n$-1\r\n", nil},
		{rdx.Array([]rdx.Msg{rdx.Error("\r")}), "", rdx.ErrInvalidError},
		{rdx.Array(nil), "*0\r\n", nil},
		{rdx.Array([]rdx.Msg{
			rdx.Int(12345),
			rdx.String("foo bar baz"),
			rdx.Nil,
			rdx.Array(nil),
			rdx.Array([]rdx.Msg{rdx.Int(67890)}),
		}),
			strings.Join([]string{
				"*5",
				":12345",
				"$11\r\nfoo bar baz",
				"$-1",
				"*0",
				"*1",
				":67890",
				"", // sentinel
			}, "\r\n"),
			nil},
	}

	for i, e := range table {
		e.eval(t, i+1)
	}
}
