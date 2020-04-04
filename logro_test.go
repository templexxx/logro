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
	"go.uber.org/goleak"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var testConfig = &Config{
	MaxSize:     32,
	BufSize:     4,
	PerSyncSize: 16,
	Developed:   true,
}

type testEnv struct {
	dir string
	fp  string
	r   *Rotation
}

func makeTestEnv() (e *testEnv, err error) {
	dir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		return nil, err
	}
	fp := filepath.Join(dir, "logro-test.log")
	testConfig.OutputPath = fp
	r, err := New(testConfig)
	if err != nil {
		return
	}
	return &testEnv{
		dir: dir,
		fp:  fp,
		r:   r,
	}, nil
}

func (e *testEnv) clear() {
	e.r.Close()
	os.RemoveAll(e.dir)
}

type testRotation struct {
	*testing.T
	r *Rotation
}

func runTests(t *testing.T, tests ...func(tr *testRotation)) {

	for _, test := range tests {
		e, err := makeTestEnv()
		if err != nil {
			t.Fatal(err)
		}

		tr := &testRotation{
			T: t,
			r: e.r,
		}

		test(tr)

		e.clear()
	}
}

func TestNew(t *testing.T) {

	fn := func(tr *testRotation) {
		// TODO try empty output
		// Just new, do nothing.
	}
	runTests(t, fn)
}

func TestRotation_Write(t *testing.T) {

	fn := func(tr *testRotation) {
		r := tr.r
		for i := 0; i < int(r.cfg.MaxSize+1); i++ {
			n, err := r.Write([]byte{'1'})
			if err != nil {
				tr.Fatal(err)
			}
			if n != 1 {
				tr.Fatal("written mismatch")
			}
		}
	}
	runTests(t, fn)
}

func TestRotation_Sync(t *testing.T) {
	fn := func(tr *testRotation) {
		r := tr.r

		p := make([]byte, r.cfg.MaxSize)
		rand.Read(p)
		for i := 0; i < int(r.cfg.MaxSize); i++ {
			n, err := r.Write([]byte{p[i]})
			if err != nil {
				tr.Fatal(err)
			}
			if n != 1 {
				tr.Fatal("written mismatch")
			}
		}
		r.Sync()
		if !isMatchFileContent(p, r.output.fp) {
			tr.Fatal("log file content mismatch")
		}
	}
	runTests(t, fn)
}

// check no goroutine leak
func TestRotation_Close(t *testing.T) {

	defer goleak.VerifyNone(t)
	fn := func(tr *testRotation) {
		r := tr.r
		for i := 0; i < int(r.cfg.MaxSize+1); i++ {
			n, err := r.Write([]byte{'1'})
			if err != nil {
				tr.Fatal(err)
			}
			if n != 1 {
				tr.Fatal("written mismatch")
			}
		}
	}
	runTests(t, fn)
}

// Written > MaxSize, should open a new log file.
func TestRotation_WriteMaxSize(t *testing.T) {

	fn := func(tr *testRotation) {
		r := tr.r
		p := make([]byte, r.cfg.MaxSize+1)
		rand.Read(p)

		for i := 0; i < len(p); i++ {
			n, err := r.Write([]byte{p[i]})
			if err != nil {
				tr.Fatal(err)
			}
			if n != 1 {
				tr.Fatal("written mismatch")
			}
		}

		oldFP := r.output.backups.Pop().(Backup).fp

		if !isMatchFileSize(r.cfg.MaxSize, oldFP) {
			tr.Fatal("log file size mismatch")
		}

		if !isMatchFileContent(p[:r.cfg.MaxSize], oldFP) {
			tr.Fatal("log file content mismatch")
		}
	}
	runTests(t, fn)
}

// Written > MaxSize, should open a new log file. ( Write concurrent)
func TestRotation_WriteMaxSizeConcurrent(t *testing.T) {

	fn := func(tr *testRotation) {
		r := tr.r
		pLen := r.cfg.MaxSize + 2
		p := make([]byte, pLen)
		rand.Read(p)

		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				for i, v := range p[int64(i)*pLen/2 : int64(i)*pLen/2+pLen/2] {
					n, err := r.Write([]byte{v})
					if err != nil {
						tr.Fatal(err, i)
					}
					if n != 1 {
						tr.Fatal("written mismatch")
					}
				}

			}(i)
		}
		wg.Wait()

		oldFP := r.output.backups.Pop().(Backup).fp

		if !isMatchFileSize(r.cfg.MaxSize, oldFP) {
			tr.Fatal("log file size mismatch")
		}
	}
	runTests(t, fn)
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
