/*
 * Copyright (c) 2019. Temple3x (temple3x@gmail.com)
 * Copyright (c) 2014 Nate Finch
 *
 * Use of this source code is governed by the MIT License
 * that can be found in the LICENSE file.
 */

package logro

import (
	"bufio"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestNewCreate(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(dir)
	fp := filepath.Join(dir, "a.log")
	defer os.Remove(fp)

	_, err = New(&Config{
		OutputPath: fp,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewExist(t *testing.T) {
	f, err := ioutil.TempFile(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	_, err = New(&Config{
		OutputPath: f.Name(),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAlignToPage(t *testing.T) {
	for i := 1; i <= pageSize; i++ {
		if alignToPage(int64(i)) != pageSize {
			t.Fatal("align mismatch")
		}
	}
	for i := pageSize + 1; i < pageSize*2; i++ {
		if alignToPage(int64(i)) != pageSize*2 {
			t.Fatal("align mismatch")
		}
	}
}

func TestRotation_Write(t *testing.T) {
	f, err := ioutil.TempFile(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	r, err := New(&Config{
		OutputPath: f.Name(),
		Developed:  true,
	})
	if err != nil {
		t.Fatal(err)
	}

	bufSize := 8
	r.buf = bufio.NewWriterSize(r.file, bufSize)
	for i := 0; i < bufSize; i++ {
		written, err := r.Write([]byte{'1'})
		if err != nil {
			t.Fatal(err)
		}
		if written != 1 {
			t.Fatal("written mismatch")
		}
		if r.dirtySize != int64(i+1) {
			t.Fatal("dirty size mismatch")
		}
		if r.dirtyOffset != 0 {
			t.Fatal("dirty offset mismatch")
		}
		if r.fsize != int64(i+1) {
			t.Fatal("fsize mismatch")
		}
	}

	stat, err := r.file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() != 0 {
		t.Fatal("true fsize mismatch")
	}
}

func TestRotation_Sync(t *testing.T) {
	f, err := ioutil.TempFile(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	r, err := New(&Config{
		OutputPath: f.Name(),
	})
	if err != nil {
		t.Fatal(err)
	}

	n := 8
	for i := 0; i < n; i++ {
		written, err := r.Write([]byte{'1'})
		if err != nil {
			t.Fatal(err)
		}
		if written != 1 {
			t.Fatal("written mismatch")
		}
		if r.dirtySize != int64(i+1) {
			t.Fatal("dirty size mismatch")
		}
		if r.dirtyOffset != 0 {
			t.Fatal("dirty offset mismatch")
		}
		if r.fsize != int64(i+1) {
			t.Fatal("fsize mismatch")
		}
	}
	r.Sync()
	stat, err := r.file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() != int64(n) {
		t.Fatal("true fsize mismatch")
	}
}

// dirty_size >= BytesPerSync
func TestRotation_AutoSync(t *testing.T) {
	f, err := ioutil.TempFile(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	mb = 1
	bytesPerSync := int64(7)
	r, err := New(&Config{
		OutputPath:   f.Name(),
		Developed:    true,
		BytesPerSync: bytesPerSync,
	})
	if err != nil {
		t.Fatal(err)
	}

	n := bytesPerSync
	for i := int64(0); i < n; i++ {
		written, err := r.Write([]byte{'1'})
		if err != nil {
			t.Fatal(err)
		}
		if written != 1 {
			t.Fatal("written mismatch")
		}
		if r.fsize != i+1 {
			t.Fatal("fsize mismatch")
		}

		if i < n-1 {
			if r.dirtySize != i+1 {
				t.Fatal("dirty size mismatch", r.dirtySize, i)
			}
			if r.dirtyOffset != 0 {
				t.Fatal("dirty offset mismatch")
			}
		} else {
			if r.dirtySize != 0 {
				t.Fatal("dirty size mismatch")
			}
			if r.dirtyOffset != bytesPerSync {
				t.Fatal("dirty offset mismatch")
			}
		}
	}

	stat, err := r.file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() != n {
		t.Fatal("true fsize mismatch")
	}
}

// Written >= MaxSize
func TestRotation_WriteMaxSize(t *testing.T) {
	f, err := ioutil.TempFile(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	mb = 1
	bytesPerSync := int64(7)
	maxSize := bytesPerSync * 2
	r, err := New(&Config{
		OutputPath:   f.Name(),
		Developed:    true,
		BytesPerSync: bytesPerSync,
		MaxSize:      maxSize,
	})
	if err != nil {
		t.Fatal(err)
	}

	n := maxSize
	for i := int64(0); i < n; i++ {
		_, err := r.Write([]byte{'1'})
		if err != nil {
			t.Fatal(err)
		}
	}

	if r.Backups.Len() != 1 {
		t.Fatal("backups mismatch", r.Backups.Len())
	}
	if r.fsize != 0 {
		t.Fatal("fsize mismatch")
	}
	stat, err := r.file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() != 0 {
		t.Fatal("true fsize mismatch")
	}

	// Write one more.
	_, err = r.Write([]byte{'1'})
	if err != nil {
		t.Fatal(err)
	}
	if r.fsize != 1 {
		t.Fatal("fsize mismatch")
	}
	stat, err = r.file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() != 0 {
		t.Fatal("true fsize mismatch")
	}
	r.Sync()
	stat, err = r.file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() != 1 {
		t.Fatal("true fsize mismatch")
	}
}

// Written >= MaxSize in Concurrency
func TestRotation_WriteMaxSizeConcurrency(t *testing.T) {
	f, err := ioutil.TempFile(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())

	mb = 1
	bytesPerSync := int64(7)
	maxSize := bytesPerSync * 2
	r, err := New(&Config{
		OutputPath:   f.Name(),
		Developed:    true,
		BytesPerSync: bytesPerSync,
		MaxSize:      maxSize,
	})
	if err != nil {
		t.Fatal(err)
	}

	n := maxSize
	wg := new(sync.WaitGroup)
	wg.Add(int(n))
	for i := int64(0); i < n; i++ {
		go func() {
			defer wg.Done()
			r.Write([]byte{'1'})
		}()
	}
	wg.Wait()
	if r.Backups.Len() != 1 {
		t.Fatal("backups mismatch", r.Backups.Len())
	}
	if r.fsize != 0 {
		t.Fatal("fsize mismatch")
	}
	stat, err := r.file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() != 0 {
		t.Fatal("true fsize mismatch")
	}
}
