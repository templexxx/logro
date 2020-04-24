/*
* Copyright (c) 2019. Temple3x (temple3x@gmail.com)
*
* Use of this source code is governed by the MIT License
* that can be found in the LICENSE file.
 */

package logro

import (
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// This bench is only for non-blocking model,
// that means all write are same for Write.
//
// if there is write stall which will impact user-facing write,
// it shouldn't use this bench.
func BenchmarkRotation_Write(b *testing.B) {
	dir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	fn := "logro-perf-write-test.log"
	fp := filepath.Join(dir, fn)

	cfg := new(Config)
	cfg.OutputPath = fp

	r, err := New(cfg)
	if err != nil {
		b.Fatal(err)
	}
	defer r.Close()

	p := make([]byte, 256)
	rand.Read(p)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Write(p)
	}
}

// Same as BenchmarkRotation_Write.
func BenchmarkRotation_WriteParallel(b *testing.B) {
	dir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)

	fn := "logro-perf-write-test.log"
	fp := filepath.Join(dir, fn)

	cfg := new(Config)
	cfg.OutputPath = fp

	r, err := New(cfg)
	if err != nil {
		b.Fatal(err)
	}
	defer r.Close()

	p := make([]byte, 256)
	rand.Read(p)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r.Write(p)
		}
	})
}
