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

	"github.com/templexxx/fnc"
)

// TODO bench IO and IOPS

// TODO bench test, output like jfse, and compare lumjack, and old version. input should > maxSize

const (
	KB = 1024
	MB = 1024 * KB
	GB = 1024 * MB
)

type jobResult struct {
	cost   time.Duration
	submit int64
	fail   int64
}

var runPerf bool

func init() {
	flag.BoolVar(&runPerf, "perf", false, "enable tests of write perf")
}

func TestWritePerf(t *testing.T) {
	//if !runPerf {
	//	t.Skip("skip write perf tests, enable it by adding '-perf=true'")
	//}
	// TODO why just run it faster?
	t.Run("Logro", testLogroWritePerf)
	t.Run("NoBuf", testNoBufWritePerf)
	// TODO add bigger than buffer logro write
	// TODO logro different filewritesize flushsize
	// TODO add direct write
	// TODO add write with a buf
	// TODO compare lumjeck
	// TODO may should care latency more
	// TODO how fast the write will not return error?
}

func testNoBufWritePerf(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	fn := "a.log"
	fp := filepath.Join(dir, fn)

	f, err := fnc.OpenFile(fp, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var total = 128 * mb
	var blockSize int64 = 512 * KB
	thread := runtime.NumCPU()
	var size = total / int64(thread)

	var write func([]byte) (int64, error)
	write = func(p []byte) (int64, error) {
		n, err := f.Write(p)
		return int64(n), err
	}
	result := runJob(write, blockSize, size, thread)
	sec := result.cost.Seconds()
	bw := float64(size*int64(thread)) / sec
	iops := float64(result.submit) / sec
	lat := time.Duration(result.cost.Nanoseconds() / result.submit)
	// TODO add a printPerf func two type of print: one for buffer, one for logro
	fmt.Printf("submit: %d, complete: %d, bufsize: %s, blocksize: %s, "+
		"bandwidth: %s/s, io: %s, avg_iops: %.2f, avg_latency: %s, cost: %s thead: %d\n",
		result.submit, result.submit-result.fail, byteToStr(float64(0*KB)), byteToStr(float64(blockSize)),
		byteToStr(bw), byteToStr(float64(size*int64(thread))), iops, lat, result.cost.String(), thread)
}

func testLogroWritePerf(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	fn := "a.log"
	fp := filepath.Join(dir, fn)

	cfg := new(Config)
	cfg.OutputPath = fp
	bufSize := 256
	cfg.BufSize = int64(bufSize)

	l, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("cfg: %#v\n", cfg)

	defer l.Close()

	var total = 128 * mb
	var blockSize int64 = 256
	thread := runtime.NumCPU()
	var size = total / int64(thread)
	var write func([]byte) (int64, error)
	write = func(p []byte) (int64, error) {
		n, err := l.Write(p)
		return int64(n), err
	}
	result := runJob(write, blockSize, size, thread)
	sec := result.cost.Seconds()
	bw := float64(size*int64(thread)) / sec
	iops := float64(result.submit) / sec
	lat := time.Duration(result.cost.Nanoseconds() / result.submit)
	// TODO add a printPerf func two type of print: one for buffer, one for logro
	fmt.Printf("submit: %d, complete: %d, bufsize: %s, blocksize: %s, "+
		"bandwidth: %s/s, io: %s, avg_iops: %.2f, avg_latency: %s, cost: %s thead: %d\n",
		result.submit, result.submit-result.fail, byteToStr(float64(cfg.BufSize)), byteToStr(float64(blockSize)),
		byteToStr(bw), byteToStr(float64(size*int64(thread))), iops, lat, result.cost.String(), thread)

}

func runJob(write func(p []byte) (int64, error), blockSize int64, size int64, thread int) jobResult {

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

	return jobResult{
		cost:   cost,
		submit: eachThreadWrites * int64(thread),
		fail:   failCnt,
	}
}

func byteToStr(n float64) string {
	if n >= GB {
		return fmt.Sprintf("%.2fGB", n/GB)
	}

	if n >= MB {
		return fmt.Sprintf("%.2fMB", n/MB)
	}

	if n >= KB {
		return fmt.Sprintf("%.2fKB", n/KB)
	}

	return fmt.Sprintf("%.2fB", n)
}
