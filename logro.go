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

	output      *output
	fileWritten int64

	buf      *bufIO
	dirty    int64
	syncJobs chan syncJob

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

	cfg.adjust()

	r = &Rotation{cfg: cfg}
	bs, err := listBackups(cfg.OutputPath, cfg.MaxBackups)
	if err != nil {
		return
	}
	out := newOutput(cfg.OutputPath, bs, cfg.LocalTime, cfg.MaxBackups)
	err = out.open()
	if err != nil {
		return
	}
	r.output = out

	r.buf = newBufIO(out.getFile(), int(r.cfg.BufSize))
	// TODO should bigger?
	r.syncJobs = make(chan syncJob, 256)

	return
}

func (r *Rotation) run() {
	r.startLoop()
	atomic.StoreInt64(&r.isRunning, 1)
}

func (r *Rotation) startLoop() {
	r.loopCtx, r.loopCancel = context.WithCancel(context.Background())
	r.loopWg.Add(1)
	go r.syncLoop()
}

// Write writes data to buffer then notify file write.
func (r *Rotation) Write(p []byte) (written int, err error) {

	if r.isClosed() {
		return
	}

	written, fw, err := r.buf.write(p)
	if err != nil {
		return
	}
	atomic.AddInt64(&r.fileWritten, int64(fw))
	atomic.AddInt64(&r.dirty, int64(fw))

	f := r.output.getFile()
	// TODO should I compress fw?
	dirty := atomic.LoadInt64(&r.dirty)
	if dirty >= r.cfg.PerSyncSize {
		r.syncJobs <- syncJob{
			f:     f,
			size:  dirty, // TODO may dirty too many
			isOld: false,
		}
		atomic.CompareAndSwapInt64(&r.dirty, dirty, 0)
	}

	if atomic.LoadInt64(&r.fileWritten) >= r.cfg.MaxSize {
		err = r.output.open()
		if err != nil {
			atomic.StoreInt64(&r.fileWritten, 0)
			return
		}
		r.buf.reset(r.output.getFile())
		r.syncJobs <- syncJob{
			f:     f,
			size:  0,
			isOld: true,
		}
		atomic.StoreInt64(&r.fileWritten, 0)
	}
	// TODO think about openNew later

	return
}

// Sync syncs all dirty data.
func (r *Rotation) Sync() (err error) {

	if r.isClosed() {
		return
	}

	fw, err := r.buf.flushSafe()
	if err != nil {
		return
	}

	r.syncJobs <- syncJob{
		f:     r.output.getFile(),
		size:  int64(fw),
		isOld: false,
	}
	return
}

// Close closes logro and release all resources.
func (r *Rotation) Close() (err error) {

	r.Sync() // Sync before close.

	if !atomic.CompareAndSwapInt64(&r.isRunning, 1, 0) {
		return
	}

	r.stopLoop()

	close(r.syncJobs)

	if r.output._f != nil {
		return r.output._f.Close()
	}

	return
}

func (r *Rotation) stopLoop() {
	r.loopCancel()
	r.loopWg.Wait()
}

func (r *Rotation) isClosed() bool {
	return atomic.LoadInt64(&r.isRunning) == 0
}

type syncJob struct {
	f     *os.File
	size  int64
	isOld bool
}

func (r *Rotation) syncLoop() {

	defer r.loopWg.Done()

	ctx, cancel := context.WithCancel(r.loopCtx)
	defer cancel()

	n := int64(0)
	flushed := int64(0)

	for {
		select {
		case job := <-r.syncJobs:
			if !job.isOld {
				n += job.size
				if n >= r.cfg.PerSyncSize {
					fnc.FlushHint(job.f, flushed, n)
					flushed += n
					n = 0
				}
			} else {
				fnc.FlushHint(job.f, flushed, n)
				// Warning:
				// 1. May drop too much cache,
				// because log shipper may still need the cache(a bit slower than writing).
				// 2. May still has some cache,
				// because these dirty page cache haven't been flushed to disk or file size is bigger than MaxSize.
				fnc.DropCache(job.f, 0, r.cfg.MaxSize)
				job.f.Close()
				flushed = 0
				n = 0
			}

		case <-ctx.Done():
			return
		}
	}
}
