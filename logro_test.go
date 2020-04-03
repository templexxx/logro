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
	"sync"
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

// TODO fix test
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
		time.Sleep(time.Microsecond) // Avoid too fast write.
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

// TODO fix test
// Written > MaxSize, should open a new log file. ( Write concurrent)
func TestRotation_WriteMaxSizeConcurrent(t *testing.T) {
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
		MaxSize:       bufSize,
		BufSize:       bufSize,
		FileWriteSize: bufSize / 2,
		FlushSize:     bufSize,
	}
	r, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	pLen := cfg.MaxSize + cfg.FileWriteSize
	p := make([]byte, pLen)
	rand.Read(p)

	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			defer wg.Done()
			for i, v := range p[int64(i)*pLen/2 : int64(i)*pLen/2+pLen/2] {
				time.Sleep(time.Microsecond) // Avoid too fast write.
				_, err := r.Write([]byte{v})
				if err != nil {
					t.Fatal(err, i)
				}
			}

		}(i)
	}

	wg.Wait()

	time.Sleep(time.Second)

	if !isMatchFileSize(cfg.FileWriteSize, fp) {
		t.Fatal("log file size mismatch")
	}
}

func isMatchFileSize(size int64, output string) bool {
	f, err := os.OpenFile(output, os.O_RDONLY, 0600)
	if err != nil {
		return false
	}
	defer f.Close()

	fs, err := f.Stat()
	if err != nil {
		return false
	}

	return size == fs.Size()
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
