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
	t.Run("Logro-Buffer", testBufferWritePerf)
	t.Run("Logro", testLogroWritePerf)
	// TODO add bigger than buffer logro write
	// TODO linux perf
	// TODO logro different filewritesize flushsize
	// TODO add direct write
	// TODO add write with a buf
	// TODO compare lumjeck
	// TODO may should care latency more
	// TODO how fast the write will not return error?
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
	cfg.FileWriteSize = 64 * kb // TODO why 64KB so different?
	cfg.FlushSize = 8 * mb

	l, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	var bufSize int64 = 64 * 1024 * 1024
	var blockSize int64 = 256
	thread := runtime.NumCPU()
	var size = bufSize / int64(thread)
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
		result.submit, result.submit-result.fail, byteToStr(float64(bufSize)), byteToStr(float64(blockSize)),
		byteToStr(bw), byteToStr(float64(size*int64(thread))), iops, lat, result.cost.String(), thread)

}

func testBufferWritePerf(t *testing.T) {

	var bufSize int64 = 64 * 1024 * 1024
	var blockSize int64 = 256
	thread := runtime.NumCPU()
	var size = bufSize / int64(thread)

	buf := newBuffer(bufSize)
	var write func([]byte) (int64, error)
	write = func(p []byte) (int64, error) {
		return 0, buf.write(p)
	}
	result := runJob(write, blockSize, size, thread)
	sec := result.cost.Seconds()
	bw := float64(size*int64(thread)) / sec
	iops := float64(result.submit) / sec
	lat := time.Duration(result.cost.Nanoseconds() / result.submit)
	fmt.Printf("submit: %d, complete: %d, bufsize: %s, blocksize: %s, "+
		"bandwidth: %s/s, io: %s, avg_iops: %.2f, avg_latency: %s, cost: %s thead: %d\n",
		result.submit, result.submit-result.fail, byteToStr(float64(bufSize)), byteToStr(float64(blockSize)),
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
