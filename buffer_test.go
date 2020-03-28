package logro

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestBuffer_ReadAfterWrite(t *testing.T) {
	size := int64(8)
	buf := newBuffer(size)
	for i := 0; i < int(size); i++ {
		a := buf.getAvailWrite()
		if a != size-int64(i) {
			t.Fatal("avail mismatch")
		}

		err := buf.write([]byte{uint8(i)})
		if err != nil {
			t.Fatal(err)
		}
	}

	if buf.isFull != true {
		t.Fatal("buf should be full")
	}

	if buf.prod != 0 {
		t.Fatal("buf prod should be 0")
	}

	if buf.cons != 0 {
		t.Fatal("buf cons should be 0")
	}

	err := buf.write([]byte{uint8(255)})
	if err != ErrNoAvailWrite {
		t.Fatal("write should failed")
	}

	p := make([]byte, 1)
	for i := 0; i < int(size); i++ {
		a := buf.getAvailRead()
		if a != size-int64(i) {
			t.Fatal("avail mismatch")
		}

		err := buf.read(p)
		if err != nil {
			t.Fatal(err)
		}

		if p[0] != uint8(i) {
			t.Fatal("read data mismatch")
		}
	}

	if buf.isFull != false {
		t.Fatal("buf should not be full")
	}

	if buf.prod != 0 {
		t.Fatal("buf prod should be 0")
	}

	if buf.cons != 0 {
		t.Fatal("buf cons should be 0")
	}

	err = buf.read(p)
	if err != ErrNoAvailRead {
		t.Fatal("read should failed")
	}
}

func TestBuffer_Write(t *testing.T) {
	size := int64(8)
	buf := newBuffer(size)
	p := make([]byte, size)
	for i := 0; i < int(size); i++ {
		p[i] = uint8(i)
	}

	// Case1: b.prod >= b.cons && prodToEnd < n
	err := buf.write(p[:7])
	if err != nil {
		t.Fatal(err)
	}
	err = buf.read(p[:1])
	if err != nil {
		t.Fatal(err)
	}
	err = buf.write(p[:2])
	if err != nil {
		t.Fatal(err)
	}
	if buf.prod != 1 {
		t.Fatal("buf prod mismatch")
	}
	if buf.cons != 1 {
		t.Fatal("buf cons mismatch")
	}

	// Case2: b.prod < b.cons
	err = buf.read(p[:1]) // Now cons is 2.
	if err != nil {
		t.Fatal(err)
	}

	err = buf.write(p[:1])
	if err != nil {
		t.Fatal(err)
	}
	if buf.prod != 2 {
		t.Fatal("buf prod mismatch")
	}
	if buf.cons != 2 {
		t.Fatal("buf cons mismatch")
	}

	err = buf.write(nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBuffer_Read(t *testing.T) {
	size := int64(8)
	buf := newBuffer(size)
	p := make([]byte, size)
	for i := 0; i < int(size); i++ {
		p[i] = uint8(i)
	}

	// Case1: b.cons + n  >= b.size
	err := buf.write(p)
	if err != nil {
		t.Fatal(err)
	}
	err = buf.read(p[:3])
	if err != nil {
		t.Fatal(err)
	}
	err = buf.write(p[:1])
	if err != nil {
		t.Fatal(err)
	}
	if buf.prod != 1 {
		t.Fatal("buf prod mismatch")
	}
	if buf.cons != 3 {
		t.Fatal("buf cons mismatch")
	}
	err = buf.read(p[:8-3+1])
	if err != nil {
		t.Fatal(err)
	}
	if buf.prod != 1 {
		t.Fatal("buf prod mismatch")
	}
	if buf.cons != 1 {
		t.Fatal("buf cons mismatch")
	}

	err = buf.read(nil)
	if err != nil {
		t.Fatal(err)
	}
}

// In logro, actually there is only one thread consume buf.
// Test it for ensuring it's ok.
func TestConcurrentWrite(t *testing.T) {
	size := int64(8)
	buf := newBuffer(size)

	cnt := int64(0)

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				err := buf.write([]byte{'1'})
				if err != nil {
					break
				}
				atomic.AddInt64(&cnt, 1)
			}

		}()
	}

	wg.Wait()
	if cnt != size {
		t.Fatal("write size mismatch")
	}
}

func TestConcurrentRead(t *testing.T) {
	size := int64(8)
	buf := newBuffer(size)
	p := make([]byte, size)
	for i := 0; i < int(size); i++ {
		p[i] = uint8(i)
	}
	err := buf.write(p)
	if err != nil {
		t.Fatal(err)
	}

	cnt := int64(0)

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p0 := make([]byte, 1)
			for {
				err := buf.read(p0)
				if err != nil {
					break
				}
				atomic.AddInt64(&cnt, 1)
			}

		}()
	}

	wg.Wait()
	if cnt != size {
		t.Fatal("read size mismatch")
	}
}

// Multi write one read, logro model.
func TestConcurrentWriteRead(t *testing.T) {
	size := int64(8)
	buf := newBuffer(size)

	writeCnt := int64(0)

	readCnt := int64(0)
	readChan := make(chan int, 2)
	go func() {
		p0 := make([]byte, size)
		for flag := range readChan {
			if flag == 0 {
				err := buf.read(p0[:2])
				if err == nil {
					atomic.AddInt64(&readCnt, 2)
				}
			} else {
				n := buf.readAll(p0)
				atomic.AddInt64(&readCnt, n)
			}
		}
	}()

	var wg sync.WaitGroup

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 128; j++ {
				err := buf.write([]byte{'1'})
				if err != nil {
					readChan <- 2
					break
				} else {
					atomic.AddInt64(&writeCnt, 1)
					readChan <- 1
				}
			}

		}()
	}

	wg.Wait()
	close(readChan)
	p1 := make([]byte, size)
	n := buf.readAll(p1)
	atomic.AddInt64(&readCnt, n)
	if atomic.LoadInt64(&writeCnt) != atomic.LoadInt64(&readCnt) {
		t.Fatal("write read size mismatch")
	}
}
