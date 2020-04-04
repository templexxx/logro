/*
 * Copyright (c) 2019. Temple3x (temple3x@gmail.com)
 * Copyright (c) 2014 Nate Finch
 *
 * Use of this source code is governed by the MIT License
 * that can be found in the LICENSE file.
 */

package logro

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/templexxx/fnc"
)

// Rotation is implement io.WriteCloser interface with func Sync() (err error).
type Rotation struct {
	mu sync.Mutex

	cfg *Config

	isRunning int64

	backups *Backups

	f           *os.File
	fileWritten int64
	dirty       int64
	buf         *bufIO

	syncJobs   chan syncJob
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
	r.backups = bs

	err = r.open()
	if err != nil {
		return
	}

	r.buf = newBufIO(r.f, int(r.cfg.BufSize))
	r.syncJobs = make(chan syncJob, 16)

	return
}

// open opens a new log file.
// If log file existed, move it to backups.
func (r *Rotation) open() (err error) {

	fp := r.cfg.OutputPath
	if fnc.Exist(fp) { // File exist may happen in rotation process.
		backupFP, t := makeBackupFP(fp, r.cfg.LocalTime, time.Now())
		err = os.Rename(fp, backupFP)
		if err != nil {
			return fmt.Errorf("failed to rename log file, output: %s backup: %s", fp, backupFP)
		}

		heap.Push(r.backups, Backup{t, backupFP})
		if r.backups.Len() > r.cfg.MaxBackups {
			v := heap.Pop(r.backups)
			os.Remove(v.(Backup).fp)
		}
	}

	// Create a new log file.
	dir := filepath.Dir(fp)
	err = os.MkdirAll(dir, 0755) // ensure we have created the right dir.
	if err != nil {
		return fmt.Errorf("failed to make dirs for log file: %s", err.Error())
	}
	// Truncate here to clean up file content if someone else creates
	// the file between exist checking and create file.
	// Can't use os.O_EXCL here, because it may break rotation process.
	//
	// Most of log shippers monitor file size, and APPEND only can avoid Read-Modify-Write.
	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC | os.O_APPEND
	f, err := fnc.OpenFile(fp, flag, 0644)
	if err != nil {
		return fmt.Errorf("failed to create log file: %s", err.Error())
	}

	r.f = f
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

	r.mu.Lock()
	defer r.mu.Unlock()

	written, fw, err := r.buf.write(p)
	if err != nil {
		return
	}
	r.fileWritten += int64(fw)
	r.dirty += int64(fw)

	if r.dirty >= r.cfg.PerSyncSize {
		r.syncJobs <- syncJob{
			f:     r.f,
			size:  r.dirty,
			isOld: false,
		}
		r.dirty = 0
	}

	if r.fileWritten >= r.cfg.MaxSize {
		r.fileWritten = 0 // Even open failed, could avoid keeping recreating.
		r.syncJobs <- syncJob{
			f:     r.f,
			size:  0,
			isOld: true,
		}
		err = r.open()
		if err != nil {
			return
		}
		r.buf.reset(r.f)
	}

	return
}

// Sync syncs all dirty data.
func (r *Rotation) Sync() (err error) {

	if r.isClosed() {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	fw, err := r.buf.flush()
	if err != nil {
		return
	}

	r.syncJobs <- syncJob{
		f:     r.f,
		size:  int64(fw),
		isOld: false,
	}
	return
}

// Close closes logro and release all resources.
func (r *Rotation) Close() (err error) {

	if !atomic.CompareAndSwapInt64(&r.isRunning, 1, 0) {
		return
	}

	r.stopLoop()

	close(r.syncJobs)

	r.mu.Lock()
	defer r.mu.Unlock()

	r.fileWritten = 0
	r.dirty = 0
	r.buf = nil

	if r.f != nil { // Just in case.
		return r.f.Close()
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
	offset := int64(0)

	for {
		select {
		case job := <-r.syncJobs:
			if !job.isOld {
				n += job.size
				if n >= r.cfg.PerSyncSize {
					fnc.FlushHint(job.f, offset, n)
					offset += n
					n = 0
				}
			} else {
				fnc.FlushHint(job.f, 0, r.cfg.MaxSize)
				fnc.DropCache(job.f, 0, r.cfg.MaxSize)
				job.f.Close()

				// Will have a new file in the next round.
				offset = 0
				n = 0
			}

		case <-ctx.Done():
			return
		}
	}
}
