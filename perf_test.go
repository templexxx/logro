/*
* Copyright (c) 2019. Temple3x (temple3x@gmail.com)
*
* Use of this source code is governed by the MIT License
* that can be found in the LICENSE file.
 */

package logro

import (
	"flag"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type perfResult struct {
	cost   time.Duration
	submit int64
	fail   int64
}

var enablePerfTest bool

func init() {
	flag.BoolVar(&enablePerfTest, "perf", false, "enable write perf tests")
}

func TestWritePerf(t *testing.T) {
	if !enablePerfTest {
		t.Skip("skip write perf tests, enable it by adding '-perf=true'")
	}
	t.Run("Logro", wrapTestWritePerf(testWritePerf, 128*mb, 256, 64, 16))
}

func wrapTestWritePerf(f func(*testing.T,
	int64, int64, int64, int64),
	total, blockSize, bufSize, perSyncSize int64) func(t *testing.T) {

	return func(t *testing.T) {
		f(t, total, blockSize, bufSize, perSyncSize)
	}
}

func testWritePerf(t *testing.T, total, blockSize, bufSize, perSyncSize int64) {
	dir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	fn := "logro-perf-test.log"
	fp := filepath.Join(dir, fn)

	cfg := new(Config)
	cfg.OutputPath = fp
	cfg.BufSize = bufSize
	cfg.PerSyncSize = perSyncSize

	r, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	thread := runtime.NumCPU()
	var size = total / int64(thread)
	var write func([]byte) (int64, error)
	write = func(p []byte) (int64, error) {
		n, err := r.Write(p)
		return int64(n), err
	}
	result := runPerfJob(write, blockSize, size, thread)
	sec := result.cost.Seconds()
	bw := float64(size*int64(thread)) / sec
	iops := float64(result.submit) / sec
	lat := time.Duration(result.cost.Nanoseconds() / result.submit)
	fmt.Printf("config: %#v\n", r.cfg)
	fmt.Printf("submit: %d, complete: %d, bufsize: %s, blocksize: %s, "+
		"bandwidth: %s/s, io: %s, avg_iops: %.2f, avg_latency: %s, cost: %s thead: %d\n",
		result.submit, result.submit-result.fail, byteToStr(float64(cfg.BufSize)), byteToStr(float64(blockSize)),
		byteToStr(bw), byteToStr(float64(size*int64(thread))), iops, lat, result.cost.String(), thread)

}

func runPerfJob(write func(p []byte) (int64, error), blockSize int64, size int64, thread int) perfResult {

	var failCnt int64
	p := make([]byte, blockSize)
	rand.Read(p)
	eachThreadWrites := size / blockSize

	wg := new(sync.WaitGroup)
	wg.Add(thread)

	start := time.Now()

	for i := 0; i < thread; i++ {
		go func() {
			defer wg.Done()

			for j := 0; j < int(eachThreadWrites); j++ {
				_, err := write(p)
				if err != nil {
					atomic.AddInt64(&failCnt, 1)
				}
			}
		}()
	}
	wg.Wait()

	cost := time.Now().Sub(start)

	return perfResult{
		cost:   cost,
		submit: eachThreadWrites * int64(thread),
		fail:   failCnt,
	}
}

func byteToStr(n float64) string {
	if n >= float64(mb*1024) {
		return fmt.Sprintf("%.2fGB", n/float64(mb*1024))
	}

	if n >= float64(mb) {
		return fmt.Sprintf("%.2fMB", n/float64(mb))
	}

	if n >= float64(kb) {
		return fmt.Sprintf("%.2fKB", n/float64(kb))
	}

	return fmt.Sprintf("%.2fB", n)
}
