/*
* Copyright (c) 2019. Temple3x (temple3x@gmail.com)
* Copyright 2019 smallnest
*
* Use of this source code is governed by the MIT License
* that can be found in the LICENSE file.
 */

package logro

import (
	"errors"
	"sync"
)

var (
	ErrNoAvailWrite = errors.New("no avail space to write")
	ErrNoAvailRead  = errors.New("no avail data to read")
)

// buffer is a circular buffer.
// It's used for logro's user-facing write.
type buffer struct {
	mu sync.Mutex

	buf  []byte
	size int64

	isFull bool

	cons int64 // next position to consume
	prod int64 // next position to product
}

func newBuffer(size int64) *buffer {
	return &buffer{
		buf:  make([]byte, size),
		size: size,
	}
}

func (b *buffer) isAvailWrite(n int64) bool {
	return b.getAvailWrite() >= n
}

// write writes len(p) bytes from p to the underlying buf.
//
// If there is no avail space to write return error and do nothing.
func (b *buffer) Write(p []byte) (err error) {
	if len(p) == 0 {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	n := int64(len(p))
	if !b.isAvailWrite(n) {
		return ErrNoAvailWrite
	}

	if b.prod >= b.cons {
		prodToEnd := b.size - b.prod
		if prodToEnd >= n {
			copy(b.buf[b.prod:], p)
			b.prod += n
		} else {
			copy(b.buf[b.prod:], p[:prodToEnd])
			copy(b.buf[0:], p[prodToEnd:])
			b.prod = n - prodToEnd
		}
	} else {
		copy(b.buf[b.prod:], p)
		b.prod += n
	}

	if b.prod == b.size {
		b.prod = 0
	}
	if b.prod == b.cons {
		b.isFull = true
	}

	return nil
}

func (b *buffer) isAvailRead(n int64) bool {

	return b.getAvailRead() >= n
}

// read reads len(p) bytes from the underlying buf to p.
//
// If there is no avail data to read return error and do nothing.
func (b *buffer) read(p []byte) (err error) {
	if len(p) == 0 {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	n := int64(len(p))
	if !b.isAvailRead(n) {
		return ErrNoAvailRead
	}

	if b.cons+n <= b.size {
		copy(p, b.buf[b.cons:b.cons+n])
	} else {
		copy(p, b.buf[b.cons:b.size])
		consToEnd := b.size - b.cons
		copy(p[consToEnd:], b.buf[0:n-consToEnd])
	}

	b.cons = (b.cons + n) % b.size

	b.isFull = false
	return nil
}

// readAll reads all available data to p.
// Returns bytes read.
func (b *buffer) readAll(p []byte) int64 {

	b.mu.Lock()
	defer func() {
		b.reset()
		b.mu.Unlock()
	}()

	if b.prod == b.cons {
		if b.isFull {
			copy(p, b.buf)
			return b.size
		}
		return 0
	}

	if b.prod > b.cons {
		copy(p, b.buf[b.cons:b.prod])
		return b.prod - b.cons
	}

	n := b.size - b.cons + b.prod
	consToEnd := b.size - b.cons
	copy(p, b.buf[b.cons:b.size])
	copy(p[consToEnd:], b.buf[0:b.prod])

	return n
}

// getAvailRead gets the length of available read bytes.
func (b *buffer) getAvailRead() int64 {

	if b.prod == b.cons {
		if b.isFull {
			return b.size
		}
		return 0
	}

	if b.prod > b.cons {
		return b.prod - b.cons
	}

	return b.size - b.cons + b.prod
}

// getAvailWrite gets the length of available bytes to write.
func (b *buffer) getAvailWrite() int64 {

	if b.prod == b.cons {
		if b.isFull {
			return 0
		}
		return b.size
	}

	if b.prod < b.cons {
		return b.cons - b.prod
	}

	return b.size - b.prod + b.cons
}

func (b *buffer) reset() {
	b.cons = 0
	b.prod = 0
	b.isFull = false
}
