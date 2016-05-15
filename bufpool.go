package rdx

import (
	"bytes"
	"strconv"
	"sync"
)

func memclr(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

type padbuf struct {
	*bytes.Buffer
	tmp [64]byte
}

var buffers = sync.Pool{
	New: func() interface{} {
		const mincap = 80
		return &padbuf{Buffer: bytes.NewBuffer(make([]byte, 0, mincap))}
	},
}

func tempbuffer(cap int) *padbuf {
	b := buffers.Get().(*padbuf)
	b.Grow(cap)
	return b
}

func putbuffer(b *padbuf) {
	// This could become a problem if enormous payloads are always being sent, but should only occur when sending
	// huge strings or arrays.
	const maxcap = 4096 * 8
	if b.Cap() > maxcap {
		return
	}
	b.Reset()
	memclr(b.tmp[:])
	buffers.Put(b)
}

func putint(buf *padbuf, n int64) {
	b := strconv.AppendInt(buf.tmp[:0], n, 10)
	b = append(b, "\r\n"...)
	buf.Write(b)
}

func putfloat(buf *padbuf, f float64) {
	buf.Write(strconv.AppendFloat(buf.tmp[:0], f, 'f', -1, 64))
}

func intlen(i int64) (n int) {
	if i < 0 {
		n++
		i = -i
	}
	switch {
	case i < 10:
		return n + 1
	case i < 100:
		return n + 2
	case i < 1000:
		return n + 3
	case i < 10000:
		return n + 4
	case i < 100000:
		return n + 5
	case i < 1000000:
		return n + 6
	case i < 10000000:
		return n + 7
	case i < 100000000:
		return n + 8
	case i < 1000000000:
		return n + 9
	case i < 10000000000:
		return n + 10
	case i < 100000000000:
		return n + 11
	case i < 1000000000000:
		return n + 12
	case i < 10000000000000:
		return n + 13
	case i < 100000000000000:
		return n + 14
	case i < 1000000000000000:
		return n + 15
	case i < 10000000000000000:
		return n + 16
	case i < 100000000000000000:
		return n + 17
	case i < 1000000000000000000:
		return n + 18
	default:
		return n + 19
	}
}
