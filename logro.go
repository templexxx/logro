/*
 * Copyright (c) 2019. Temple3x (temple3x@gmail.com)
 * Copyright (c) 2014 Nate Finch
 *
 * Use of this source code is governed by the MIT License
 * that can be found in the LICENSE file.
 */

package logro

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"

	"github.com/templexxx/fnc"
)

// Rotation is implement io.WriteCloser interface with func Sync() (err error).
type Rotation struct {
	cfg *Config

	isRunning int64

	output *output

	buf *buffer
	// Notify file write, value is write bytes size,
	// 0 means write all bytes in buffer.
	fileWriteJobs chan int64
	flushJobs     chan flushJob

	ctx        context.Context
	loopCtx    context.Context
	loopCancel func()
	loopWg     sync.WaitGroup
}

// New creates a Rotation.
func New(cfg *Config) (r *Rotation, err error) {

	r, err = prepare(cfg)
	if err != nil {
		return
	}

	r.run()

	return
}

func prepare(cfg *Config) (r *Rotation, err error) {

	if cfg.OutputPath == "" {
		return nil, errors.New("empty log file path")
	}

	cfg.parse()

	r = &Rotation{cfg: cfg}
	bs, err := listBackups(cfg.OutputPath, cfg.MaxBackups)
	if err != nil {
		return
	}
	out := newOutput(cfg.OutputPath, cfg.MaxSize, bs, cfg.LocalTime, cfg.MaxBackups)
	err = out.open()
	if err != nil {
		return
	}
	r.output = out

	r.buf = newBuffer(r.cfg.BufSize)
	r.fileWriteJobs = make(chan int64, 256)
	r.flushJobs = make(chan flushJob, 256)

	return
}

func (r *Rotation) run() {
	r.startLoop()
	atomic.StoreInt64(&r.isRunning, 1)
}

func (r *Rotation) startLoop() {
	r.loopCtx, r.loopCancel = context.WithCancel(context.Background())
	r.loopWg.Add(2)
	go r.fileWriteLoop()
	go r.flushLoop()
}

// Write data to log file.
func (r *Rotation) Write(p []byte) (written int, err error) {

	if r.isClosed() {
		return
	}

	written = len(p)
	err = r.buf.write(p)
	if err != nil {
		r.fileWriteJobs <- 0
		return
	}

	r.fileWriteJobs <- int64(written)

	return
}

func (r *Rotation) Sync() (err error) {

	if r.isClosed() {
		return
	}

	r.fileWriteJobs <- 0
	return
}

// TODO check goroutine leak
func (r *Rotation) Close() (err error) {

	if !atomic.CompareAndSwapInt64(&r.isRunning, 1, 0) {
		return
	}

	r.stopLoop()

	close(r.fileWriteJobs)
	close(r.flushJobs)

	if r.output.f != nil {
		return r.output.f.Close()
	}

	return
}

func (r *Rotation) fileWriteLoop() {

	defer r.loopWg.Done()

	ctx, cancel := context.WithCancel(r.loopCtx)
	defer cancel()

	p := make([]byte, r.cfg.BufSize)
	n := int64(0)
	written := int64(0)

	for {
		select {
		case job := <-r.fileWriteJobs:
			if job != 0 {
				n += job
				if n >= r.cfg.FileWriteSize {
					p0 := p[:n]
					err := r.buf.read(p0)
					if err != nil {
						n = r.buf.readAll(p)
						p0 = p[:n]
					}
					r.fileWrite(r.output, written, p0)
					written += n
					n = 0
				}
			} else {
				n = r.buf.readAll(p)
				p0 := p[:n]
				r.fileWrite(r.output, written, p0)
				written += n
				n = 0
			}
		case <-ctx.Done():
			return
		}
	}
}

func (r *Rotation) fileWrite(out *output, written int64, p []byte) {
	fw, err := out.f.WriteAt(p, written)
	if err == nil {
		f := out.f
		isOld := false

		if written >= r.cfg.MaxSize {
			isOld = true
			err := out.open()
			if err != nil {
				r.Close()
			}
		}

		r.flushJobs <- flushJob{
			f:     f,
			size:  int64(fw),
			isOld: isOld,
		}
	}
}

type flushJob struct {
	f     *os.File
	size  int64
	isOld bool
}

func (r *Rotation) flushLoop() {

	defer r.loopWg.Done()

	ctx, cancel := context.WithCancel(r.loopCtx)
	defer cancel()

	n := int64(0)
	flushed := int64(0)

	for {
		select {
		case job := <-r.flushJobs:
			if !job.isOld {
				n += job.size
				if n >= r.cfg.FlushSize {
					fnc.FlushHint(job.f, flushed, n)
					flushed += n
					n = 0
				}
			} else {
				// Warning:
				// 1. May drop too much cache,
				// because log ship may still need the cache(a bit slower than writing).
				// 2. May still has some cache,
				// because these dirty page cache haven't been flushed to disk or file size is bigger than MaxSize.
				fnc.DropCache(job.f, 0, r.cfg.MaxSize)
				job.f.Close()
			}

		case <-ctx.Done():
			return
		}
	}
}

func (r *Rotation) stopLoop() {
	r.loopCancel()
	r.loopWg.Wait()
}

func (r *Rotation) isClosed() bool {
	return atomic.LoadInt64(&r.isRunning) == 0
}
