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
	"unsafe"

	"github.com/templexxx/go-diodes"

	"github.com/templexxx/fnc"
)

// Rotation is implement io.WriteCloser interface with func Sync() (err error).
type Rotation struct {
	cfg *Config

	isRunning int64

	backups *Backups

	f   *os.File
	buf *diodes.ManyToOne

	syncJob    chan struct{}
	flushJobs  chan flushJob
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

	r.buf = diodes.NewManyToOne(cfg.BufItem, nil)
	r.syncJob = make(chan struct{}, 1)
	r.flushJobs = make(chan flushJob, 16)

	return
}

// open opens a new log file.
// If log file existed, move it to backups.
func (r *Rotation) open() (err error) {

	fp := r.cfg.OutputPath

	if r.f != nil { // File exist may happen in rotation process.
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
	r.loopWg.Add(2)
	go r.writeLoop()
	go r.syncLoop()
}

// Write writes data to buffer then notify file write.
func (r *Rotation) Write(p []byte) (written int, err error) {

	if r.isClosed() {
		return
	}

	r.buf.Set(unsafe.Pointer(&p))

	return len(p), nil
}

// Sync syncs all dirty data.
func (r *Rotation) Sync() (err error) {

	if r.isClosed() {
		return
	}

	r.syncJob <- struct{}{}

	return
}

// Close closes logro and release all resources.
func (r *Rotation) Close() (err error) {

	if !atomic.CompareAndSwapInt64(&r.isRunning, 1, 0) {
		return
	}

	r.stopLoop()

	close(r.flushJobs)

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

type flushJob struct {
	f     *os.File
	size  int64
	isOld bool
}

func (r *Rotation) writeLoop() {

	defer r.loopWg.Done()

	ctx, cancel := context.WithCancel(r.loopCtx)
	defer cancel()

	bufw := newBufIO(r.f, int(r.cfg.PerWriteSize))
	dirty := 0
	written := 0
	for {
		select {
		case <-ctx.Done():
			return

		case <-r.syncJob:
			for i := 0; i < r.cfg.BufItem; i++ { // There is a limit, avoiding blocking.
				p, ok := r.buf.TryNext()
				if !ok {
					break
				}
				_, fw, _ := bufw.write(*(*[]byte)(p))
				dirty += fw
				written += fw
			}
			fw, _ := bufw.flush()
			dirty += fw
			written += fw

		default:
			p, ok := r.buf.TryNext()
			if !ok {
				time.Sleep(2 * time.Millisecond)
				continue
			}
			_, fw, _ := bufw.write(*(*[]byte)(p))
			dirty += fw
			written += fw

			if int64(dirty) >= r.cfg.PerSyncSize {
				r.flushJobs <- flushJob{r.f, int64(dirty), false}
				dirty = 0
			}

			if int64(written) >= r.cfg.MaxSize {
				written = 0 // Avoiding keeping renew file if we can't create new file.
				oldF := r.f
				err := r.open()
				if err == nil {
					r.flushJobs <- flushJob{oldF, 0, true}
					bufw.reset(r.f)
				}
			}
		}
	}
}

func (r *Rotation) syncLoop() {

	defer r.loopWg.Done()

	ctx, cancel := context.WithCancel(r.loopCtx)
	defer cancel()

	n := int64(0)
	offset := int64(0)

	for {
		select {
		case job := <-r.flushJobs:
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
