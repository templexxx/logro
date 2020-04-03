// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
/*
* Copyright (c) 2019. Temple3x (temple3x@gmail.com)
*
* Use of this source code is governed by the MIT License
* that can be found in the LICENSE file.
 */

package logro

import (
	"io"
	"sync"
)

// bufIO implements buffering for an io.Writer object.
// If an error occurs writing to a Writer, no more data will be
// accepted and all subsequent writes, and flush, will return the error.
// After all data has been written, the client should call the
// flush method to guarantee all data has been forwarded to
// the underlying io.Writer.
type bufIO struct {
	mu sync.Mutex

	err error
	buf []byte
	n   int
	w   io.Writer
}

func newBufIO(w io.Writer, size int) *bufIO {

	return &bufIO{
		buf: make([]byte, size),
		w:   w,
	}
}

// reset resets io.Writer in bufIO, remains all buffered bytes.
func (b *bufIO) reset(w io.Writer) {
	b.mu.Lock()
	b.w = w
	b.mu.Unlock()
}

// write writes the contents of p into the buffer.
// It returns the numbers of bytes written and written to io.Writer.
// it also returns an error explaining
// why the write is short (caused by underlying io.Writer).
func (b *bufIO) write(p []byte) (nn int, fw int, err error) {

	b.mu.Lock()
	defer b.mu.Unlock()

	for len(p) > b.avail() && b.err == nil {
		var n int
		if b.buffered() == 0 {
			// Large write, empty buffer.
			// Write directly from p to avoid copy.
			n, b.err = b.w.Write(p)
			fw += n
		} else {
			n = copy(b.buf[b.n:], p) // Try to fill buffer.
			b.n += n
			var fn int
			fn, b.err = b.flush()
			fw += fn
		}
		nn += n
		p = p[n:]
	}
	if b.err != nil {
		return nn, fw, b.err
	}
	n := copy(b.buf[b.n:], p)
	b.n += n
	nn += n
	return nn, fw, nil
}

// flush writes any buffered data to the underlying io.Writer.
// Returns flushed and any error.
func (b *bufIO) flush() (flushed int, err error) {
	if b.err != nil {
		return 0, b.err
	}
	if b.n == 0 {
		return 0, nil
	}
	n, err := b.w.Write(b.buf[0:b.n])
	if n < b.n && err == nil {
		err = io.ErrShortWrite
	}
	if err != nil {
		if n > 0 && n < b.n {
			copy(b.buf[0:b.n-n], b.buf[n:b.n])
		}
		b.n -= n
		b.err = err
		return n, err
	}
	b.n = 0
	return n, nil
}

// avail returns how many bytes are unused in the buffer.
func (b *bufIO) avail() int { return len(b.buf) - b.n }

// buffered returns the number of bytes that have been written into the current buffer.
func (b *bufIO) buffered() int { return b.n }
