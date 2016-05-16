package rdx_test

import (
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"

	"go.spiff.io/rdx"
)

type dectest struct {
	msg    string
	typ    rdx.Type
	result rdx.Msg
	err    error
}

func (d *dectest) eval(t *testing.T, nth int) {
	try := func(br io.Reader) {
		r := rdx.NewReader(br)
		decmsg, err := r.Read()

		if (d.err != nil) != (err != nil) || (d.err != nil && err != nil && d.err != err) {
			t.Errorf("[%d ; %T] Written err = %v; want %v", nth, d.result, err, d.err)
		}

		if !reflect.DeepEqual(d.result, decmsg) {
			t.Errorf("[%d ; %T] Read %#+ v; want %#+ v", nth, d.result, decmsg, d.result)
		}

		if d.result != nil && !rdx.IsA(decmsg, d.typ) {
			var typ rdx.Type
			if decmsg != nil {
				typ = decmsg.Type()
			}
			t.Errorf("[%d ; %T] Read msg type=%x; want %x", nth, d.result, typ, d.typ)
		}
	}

	// Triggers the bytesReader type assertion in NewReader
	try(bytes.NewBufferString(d.msg))
	// Does not trigger the assertion (strings.Reader doesn't implement ReadBytes).
	try(strings.NewReader(d.msg))
}

func TestReader_Read(t *testing.T) {
	table := []dectest{
		// Nil
		{msg: "$-1\r\n", typ: rdx.TNil, result: rdx.Nil},
		{msg: "*-1\r\n", typ: rdx.TNil, result: rdx.Nil},

		// Bad prefix
		{msg: "%-1\r\n", err: rdx.InvalidPrefixError('%')},
		{msg: "\r\n", err: rdx.ErrMissingPrefix},

		// Bad suffix
		{msg: "\n", err: rdx.ErrMissingCRLF},
		{msg: "$-1\n", err: rdx.ErrMissingCRLF},

		// Integers
		{msg: ":1000000000000000000000000\r\n", err: rdx.ErrIntRange},
		{msg: ":9223372036854775808\r\n", err: rdx.ErrIntRange},
		{msg: ":-9223372036854775809\r\n", err: rdx.ErrIntRange},
		{msg: ":0xff\r\n", err: rdx.ErrInvalidInt},
		{msg: ":\r\n", err: rdx.ErrEmptyInt},
		{msg: ":9223372036854775807\r\n", typ: rdx.TInt, result: rdx.Int(1<<63 - 1)},
		{msg: ":-9223372036854775808\r\n", typ: rdx.TInt, result: rdx.Int(-(1 << 63))},
		{msg: ":10000000000000000\r\n", typ: rdx.TInt, result: rdx.Int(10000000000000000)},
		{msg: ":-10000000000000000\r\n", typ: rdx.TInt, result: rdx.Int(-10000000000000000)},
		{msg: ":0\r\n", typ: rdx.TInt, result: rdx.Int(0)},

		// Bulk strings
		{msg: "$-3\r\n\r\n", err: rdx.ErrInvalidLength},
		{msg: "$1000000000000000000000000\r\n\r\n", err: rdx.ErrIntRange},
		{msg: "$f\r\n\r\n", err: rdx.ErrInvalidLength},
		{msg: "$0\r\n", err: rdx.ErrMissingCRLF},
		{msg: "$0\r\n\n", err: rdx.ErrMissingCRLF},
		{msg: "$0\r\n\r", err: rdx.ErrMissingCRLF},
		{msg: "$0\r\n\r\n", typ: rdx.TBulkString, result: rdx.String(nil)},
		{msg: "$3\r\nfoo\r\n", typ: rdx.TBulkString, result: rdx.String("foo")},
		{msg: "$22\r\nこんにちは 世界\r\n", typ: rdx.TBulkString, result: rdx.String("こんにちは 世界")},

		// Simple strings
		// These aren't checked for TSimpleString, as the reader will never return
		// a SimpleString. So, it checks for TString, as this includes both SimpleString and
		// BulkString.
		{msg: "+こんにちは 世界\r\n", typ: rdx.TString, result: rdx.String("こんにちは 世界")},
		{msg: "+\r\n", typ: rdx.TString, result: rdx.String("")},
		{msg: "+\n\r\n", typ: rdx.TString, err: rdx.ErrMissingCRLF},

		// Errors
		{msg: "-\r\n", typ: rdx.TError, result: rdx.Error("")},
		{msg: "-KIND error string\r\n", typ: rdx.TError, result: rdx.Error("KIND error string")},
		{msg: "-\n\r\n", err: rdx.ErrMissingCRLF},

		// Arrays
		{msg: "*-2\r\n", err: rdx.ErrInvalidLength},
		{msg: "*f\r\n\r\n", err: rdx.ErrInvalidLength},
		{msg: "*1000000000000000000000000\r\n", err: rdx.ErrIntRange},
		// Ensure nil on error, and since we have a predictable error here, check for it.
		{msg: "*4\r\n:123\r\n", err: io.EOF},
		{msg: "*0\r\n", typ: rdx.TArray, result: rdx.Array(nil)},
		{msg: "*1\r\n:123\r\n", typ: rdx.TArray, result: rdx.Array([]rdx.Msg{rdx.Int(123)})},
		{msg: "*1\r\n:123\r\n$-1\r\n+foo\r\n-bar\r\n",
			typ:    rdx.TArray,
			result: rdx.Array([]rdx.Msg{rdx.Int(123)}),
		},
		{msg: "*4\r\n:123\r\n$-1\r\n+foo\r\n-bar\r\n",
			typ: rdx.TArray,
			result: rdx.Array([]rdx.Msg{
				rdx.Int(123),
				rdx.Nil,
				rdx.String("foo"),
				rdx.Error("bar"),
			})},
	}

	for i, d := range table {
		d.eval(t, i)
	}
}
