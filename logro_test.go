///*
// * Copyright (c) 2019. Temple3x (temple3x@gmail.com)
// * Copyright (c) 2014 Nate Finch
// *
// * Use of this source code is governed by the MIT License
// * that can be found in the LICENSE file.
// */

package logro

import (
	"bytes"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestNew(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	fp := filepath.Join(dir, "a.log")

	l, err := New(&Config{
		OutputPath: fp,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
}

func TestRotation_Write(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	fp := filepath.Join(dir, "a.log")

	kb = 1
	mb = 1
	bufSize := int64(64)
	r, err := New(&Config{
		OutputPath:    fp,
		Developed:     true,
		MaxSize:       bufSize * 2,
		BufSize:       bufSize,
		FileWriteSize: bufSize / 2,
		FlushSize:     bufSize,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	p := make([]byte, bufSize)
	rand.Read(p)

	for _, v := range p {
		written, err := r.Write([]byte{v})
		if err != nil {
			t.Fatal(err)
		}

		if written != 1 {
			t.Fatal("written mismatch")
		}
	}
}

func TestRotation_Sync(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	fp := filepath.Join(dir, "a.log")

	kb = 1
	mb = 1
	bufSize := int64(64)
	r, err := New(&Config{
		OutputPath:    fp,
		Developed:     true,
		MaxSize:       bufSize * 2,
		BufSize:       bufSize,
		FileWriteSize: bufSize / 2,
		FlushSize:     bufSize,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	p := make([]byte, bufSize)
	rand.Read(p)

	for _, v := range p {
		written, err := r.Write([]byte{v})
		if err != nil {
			t.Fatal(err)
		}

		if written != 1 {
			t.Fatal("written mismatch")
		}
	}

	r.Sync()
	time.Sleep(time.Second)

	if !isMatchFileContent(p, fp) {
		t.Fatal("log file content mismatch")
	}
}

// check no goroutine leak
func TestRotation_Close(t *testing.T) {
	defer goleak.VerifyNone(t)

	dir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	fp := filepath.Join(dir, "a.log")

	kb = 1
	mb = 1
	bufSize := int64(64)
	r, err := New(&Config{
		OutputPath:    fp,
		Developed:     true,
		MaxSize:       bufSize * 2,
		BufSize:       bufSize,
		FileWriteSize: bufSize / 2,
		FlushSize:     bufSize,
	})
	if err != nil {
		r.Close()
		t.Fatal(err)
	}

	p := make([]byte, bufSize)
	rand.Read(p)

	go func() {
		for _, v := range p {
			r.Write([]byte{v})
		}
	}()

	time.Sleep(10 * time.Millisecond)
	r.Close()
}

func TestRotation_Sync_Background(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	fp := filepath.Join(dir, "a.log")

	kb = 1
	mb = 1
	bufSize := int64(64)
	r, err := New(&Config{
		OutputPath:    fp,
		Developed:     true,
		MaxSize:       bufSize * 2,
		BufSize:       bufSize,
		FileWriteSize: bufSize / 2,
		FlushSize:     bufSize,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	p := make([]byte, bufSize)
	rand.Read(p)

	for _, v := range p {
		written, err := r.Write([]byte{v})
		if err != nil {
			t.Fatal(err)
		}

		if written != 1 {
			t.Fatal("written mismatch")
		}
	}

	time.Sleep(time.Second)

	if !isMatchFileContent(p, fp) {
		t.Fatal("log file content mismatch")
	}
}

// Written > MaxSize, should open a new log file.
func TestRotation_WriteMaxSize(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	fp := filepath.Join(dir, "a.log")

	kb = 1
	mb = 1
	bufSize := int64(64)
	cfg := &Config{
		OutputPath:    fp,
		Developed:     true,
		MaxSize:       bufSize * 2,
		BufSize:       bufSize,
		FileWriteSize: bufSize / 2,
		FlushSize:     bufSize,
	}
	r, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	p := make([]byte, cfg.MaxSize+cfg.FileWriteSize)
	rand.Read(p)

	for i, v := range p {
		time.Sleep(time.Millisecond) // Avoid too fast write.
		written, err := r.Write([]byte{v})
		if err != nil {
			t.Fatal(err, i)
		}

		if written != 1 {
			t.Fatal("written mismatch")
		}
	}

	time.Sleep(time.Second)

	if !isMatchFileContent(p[cfg.MaxSize:], fp) {
		t.Fatal("log file content mismatch")
	}
}

func isMatchFileContent(p []byte, output string) bool {
	f, err := os.OpenFile(output, os.O_RDONLY, 0600)
	if err != nil {
		return false
	}
	defer f.Close()

	act := make([]byte, len(p))
	_, err = f.Read(act)
	if err != nil {
		return false
	}

	return bytes.Equal(p, act)
}

// TODO Concurrent Test
//
//// Written >= MaxSize in Concurrency
//func TestRotation_WriteMaxSizeConcurrency(t *testing.T) {
//	f, err := ioutil.TempFile(os.TempDir(), "")
//	if err != nil {
//		t.Fatal(err)
//	}
//	defer os.Remove(f.Name())
//
//	mb = 1
//	bytesPerSync := int64(7)
//	maxSize := bytesPerSync * 2
//	r, err := New(&Config{
//		OutputPath: f.Name(),
//		Developed:  true,
//		FlushSize:  bytesPerSync,
//		MaxSize:    maxSize,
//	})
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	n := maxSize
//	wg := new(sync.WaitGroup)
//	wg.Add(int(n))
//	for i := int64(0); i < n; i++ {
//		go func() {
//			defer wg.Done()
//			r.Write([]byte{'1'})
//		}()
//	}
//	wg.Wait()
//	if r.Backups.Len() != 1 {
//		t.Fatal("backups mismatch", r.Backups.Len())
//	}
//	if r.fsize != 0 {
//		t.Fatal("fsize mismatch")
//	}
//	stat, err := r.file.Stat()
//	if err != nil {
//		t.Fatal(err)
//	}
//	if stat.Size() != 0 {
//		t.Fatal("true fsize mismatch")
//	}
//}
//
//func BenchmarkWrite(b *testing.B) {
//
//	path, err := ioutil.TempDir(os.TempDir(), "logro-test")
//	if err != nil {
//		b.Fatal(err)
//	}
//	fn := filepath.Join(path, "write")
//
//	f, err := os.OpenFile(fn, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
//	if err != nil {
//		b.Fatal(err)
//	}
//	f.Close()
//	defer os.RemoveAll(path)
//
//	p := make([]byte, 256)
//	rand.Read(p)
//
//	err = fnc.Flush(f, 0, 256)
//	b.Fatal(err) // TODO how about linux will panic?
//
//	buf := bufio.NewWriterSize(f, 32*1024)
//
//	b.SetBytes(256)
//	for i := 0; i < b.N; i++ {
//		buf.Write(p)
//	}
//
//}

// TODO bench test, output like jfse, and compare lumjack, and old version. input should > maxSize
