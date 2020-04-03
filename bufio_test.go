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
	"bytes"
	"fmt"
	"io"
	"runtime"
	"sync"
	"testing"
)

const minReadBufferSize = 16

var bufsizes = []int{
	0, minReadBufferSize, 23, 32, 46, 64, 93, 128, 1024, 4096,
}

func TestBufIOWrite(t *testing.T) {
	var data [8192]byte

	for i := 0; i < len(data); i++ {
		data[i] = byte(' ' + i%('~'-' '))
	}
	w := new(bytes.Buffer)
	for i := 0; i < len(bufsizes); i++ {
		for j := 0; j < len(bufsizes); j++ {
			nwrite := bufsizes[i]
			bs := bufsizes[j]

			// Write nwrite bytes using buffer size bs.
			// Check that the right amount makes it out
			// and that the data is correct.

			w.Reset()
			buf := newBufIO(w, bs)
			context := fmt.Sprintf("nwrite=%d bufsize=%d", nwrite, bs)
			n, _, e1 := buf.write(data[0:nwrite])
			if e1 != nil || n != nwrite {
				t.Errorf("%s: buf.Write %d = %d, %v", context, nwrite, n, e1)
				continue
			}
			if _, e := buf.flush(); e != nil {
				t.Errorf("%s: buf.Flush = %v", context, e)
			}

			written := w.Bytes()
			if len(written) != nwrite {
				t.Errorf("%s: %d bytes written", context, len(written))
			}
			for l := 0; l < len(written); l++ {
				if written[l] != data[l] {
					t.Errorf("wrong bytes written")
					t.Errorf("want=%q", data[0:len(written)])
					t.Errorf("have=%q", written)
				}
			}
		}
	}
}

// Check that write errors are returned properly.
type errorWriterTest struct {
	n, m   int
	err    error
	expect error
}

func (w errorWriterTest) Write(p []byte) (int, error) {
	return len(p) * w.n / w.m, w.err
}

var errorWriterTests = []errorWriterTest{
	{0, 1, nil, io.ErrShortWrite},
	{1, 2, nil, io.ErrShortWrite},
	{1, 1, nil, nil},
	{0, 1, io.ErrClosedPipe, io.ErrClosedPipe},
	{1, 2, io.ErrClosedPipe, io.ErrClosedPipe},
	{1, 1, io.ErrClosedPipe, io.ErrClosedPipe},
}

func TestBufIOWriteError(t *testing.T) {
	for _, w := range errorWriterTests {
		buf := newBufIO(w, 4096)
		_, _, e := buf.write([]byte("hello world"))
		if e != nil {
			t.Errorf("Write hello to %v: %v", w, e)
			continue
		}
		// Two flushes, to verify the error is sticky.
		for i := 0; i < 2; i++ {
			_, e = buf.flush()
			if e != w.expect {
				t.Errorf("Flush %d/2 %v: got %v, wanted %v", i+1, w, e, w.expect)
			}
		}
	}
}

func TestBufIOFlush(t *testing.T) {
	w := new(bytes.Buffer)
	size := 32
	buf := newBufIO(w, size)

	for i := 0; i <= size; i++ {
		nn, fw, err := buf.write([]byte{'1'})
		if err != nil {
			t.Fatal(err)
		}
		if nn != 1 {
			t.Fatal("written should be 1")
		}
		if i == size {
			if fw != size {
				t.Fatal("flushed mismatch")
			}
		} else {
			if fw != 0 {
				t.Fatal("flushed should be 0")
			}
		}
	}
}

func TestBufIOConcurrentWrite(t *testing.T) {

	w := new(bytes.Buffer)
	size := 32
	buf := newBufIO(w, size)

	thread := runtime.NumCPU()

	var wg sync.WaitGroup
	for i := 0; i < thread; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 256; j++ {
				n, _, err := buf.write([]byte{uint8(j)})
				if err != nil {
					t.Fatal(err)
				}
				if n != 1 {
					t.Fatal("written should be 1")
				}
			}

		}()
	}

	wg.Wait()
	_, err := buf.flush()
	if err != nil {
		t.Fatal(err)
	}

	if w.Len() != thread*256 {
		t.Fatal("wrong size flushed")
	}
}
