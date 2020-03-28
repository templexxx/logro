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
	"container/heap"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/templexxx/fnc"
)

// Config of Logro.
type Config struct {
	// Configs of Log Rotation.
	OutputPath string `json:"output_path" toml:"output_path"` // Log file path.
	// Maximum size of a log file before it gets rotated.
	// Unit is MB.
	MaxSize int64 `json:"max_size" toml:"max_size"`
	// Maximum number of backup log files to retain.
	MaxBackups int `json:"max_backups" toml:"max_backups"`
	// Timestamp in backup log file. Default is to use UTC time.
	LocalTime bool `json:"local_time" toml:"local_time"`

	// After write BytesPerSync bytes flush data to storage media(hint).
	BytesPerSync int64 `json:"bytes_per_sync" toml:"bytes_per_sync"`

	// Develop mode. Default is false.
	// It' used for testing, if it's true, the page cache control unit could not be aligned to page cache size.
	Developed bool `json:"developed" toml:"developed"`
}

// Rotation is implement io.WriteCloser interface with func Sync() (err error).
type Rotation struct {
	sync.Mutex

	conf *Config

	file  *os.File // output *os.File.
	fsize int64    // output file size.
	// dirty page cache info.
	// ps: may already been flushed by kernel.
	dirtyOffset int64
	dirtySize   int64
	// user-space buffer for log writing.
	buf *bufio.Writer
	// jobs of sync page cache, use chan for avoiding stall.
	syncJobs chan syncJob
	Backups  *Backups // all backups information.
}

var (
	// Use variables for tests easier.
	kb int64 = 1024
	mb       = 1024 * kb

	// >32KB couldn't improve performance significantly.
	defaultBufSize = 32 * kb // 32KB

	defaultBytesPerSync = mb // 1MB

	// We don't need to keep too many backups,
	// in practice, log shipper will collect the logs already.
	defaultMaxSize    = 128 * mb
	defaultMaxBackups = 8
)

// New create a Rotation.
func New(conf *Config) (r *Rotation, err error) {

	r = &Rotation{conf: conf}
	err = r.init()
	if err != nil {
		return
	}

	go r.doSyncJob() // sync log content async.

	return
}

func (r *Rotation) init() (err error) {

	err = r.parseConf()
	if err != nil {
		return
	}

	backups := make(Backups, 0, r.conf.MaxBackups*2)
	r.Backups = &backups
	r.Backups.list(r.conf.OutputPath, r.conf.MaxBackups)

	err = r.openExistOrNew()
	if err != nil {
		return
	}

	r.buf = bufio.NewWriterSize(r.file, int(defaultBufSize))

	r.syncJobs = make(chan syncJob, 8) // 8 is enough in most cases.
	return
}

func (r *Rotation) parseConf() (err error) {

	conf := r.conf
	if conf.OutputPath == "" {
		return errors.New("empty log file path")
	}

	if conf.MaxBackups <= 0 {
		conf.MaxBackups = defaultMaxBackups
	}

	if conf.MaxSize <= 0 {
		conf.MaxSize = defaultMaxSize
	} else {
		conf.MaxSize = conf.MaxSize * mb
	}
	if conf.BytesPerSync <= 0 {
		conf.BytesPerSync = defaultBytesPerSync
	}
	if !conf.Developed {
		conf.MaxSize = alignToPage(conf.MaxSize)
		conf.BytesPerSync = alignToPage(conf.BytesPerSync)
	}

	return
}

const pageSize = 1 << 12 // 4KB.

func alignToPage(n int64) int64 {
	return (n + pageSize - 1) &^ (pageSize - 1)
}

// Open log file when start up.
func (r *Rotation) openExistOrNew() (err error) {

	fp := r.conf.OutputPath
	if !fnc.Exist(fp) {
		return r.openNew()
	}

	f, err := r.openFile(fp, os.O_WRONLY)
	if err != nil {
		return
	}
	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return
	}

	r.file = f
	r.fsize = stat.Size()

	// maybe not correct, but it's ok.
	r.dirtyOffset = stat.Size()
	r.dirtySize = 0

	return
}

// Open a new log file in two conditions:
// 1. Start up with no existed log file.
// 2. Need rename in rotation process.
func (r *Rotation) openNew() (err error) {
	fp := r.conf.OutputPath
	if fnc.Exist(fp) { // file exist may happen in rotation process.
		backupFP, t := makeBackupFP(fp, r.conf.LocalTime, time.Now())

		err = os.Rename(fp, backupFP)
		if err != nil {
			return fmt.Errorf("failed to rename log file, output: %s backup: %s", fp, backupFP)
		}

		r.sync(true)

		heap.Push(r.Backups, Backup{t, backupFP})
		if r.Backups.Len() > r.conf.MaxBackups {
			v := heap.Pop(r.Backups)
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
	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	f, err := r.openFile(fp, flag)
	if err != nil {
		return
	}

	r.file = f
	r.buf = bufio.NewWriterSize(f, int(defaultBufSize))
	r.fsize = 0
	r.dirtyOffset = 0
	r.dirtySize = 0

	return
}

func (r *Rotation) openFile(fp string, flag int) (f *os.File, err error) {

	// Logro use append-only mode to write data,
	// although it will change file size in every write
	// (in Logro cases, we don't sync frequently,so it's okay),
	// but it avoid Read-Modify-Write because append means newly allocated pages are always cached.
	flag |= os.O_APPEND

	f, err = fnc.OpenFile(fp, flag, 0644)
	if err != nil {
		return f, fmt.Errorf("failed to open log file:%s", err.Error())
	}

	return
}

// Write data to log file.
func (r *Rotation) Write(p []byte) (written int, err error) {
	r.Lock()
	defer r.Unlock()

	if r.file == nil {
		err = errors.New("failed to open log file")
		return
	}

	written, err = r.buf.Write(p)
	if err != nil {
		return
	}

	r.fsize += int64(written)
	r.dirtySize += int64(written)

	if r.dirtySize >= r.conf.BytesPerSync {
		r.sync(false)
	}

	if r.fsize >= r.conf.MaxSize {
		if err = r.openNew(); err != nil {
			return
		}
	}
	return
}

// Sync buf & dirty_page_cache to the storage media.
func (r *Rotation) Sync() (err error) {
	r.Lock()
	defer r.Unlock()

	r.sync(false)
	return
}

// sync creates sync job.
// isBackup means the r.file is backup file,
// we need close file & clean page cache.
func (r *Rotation) sync(isBackup bool) {

	if r.buf != nil {
		r.buf.Flush()
	}

	if r.file != nil {
		r.syncJobs <- syncJob{r.file, r.dirtyOffset, r.dirtySize, isBackup}
	}

	r.dirtyOffset += r.dirtySize
	r.dirtySize = 0
}

type syncJob struct {
	f        *os.File
	offset   int64
	size     int64
	isBackup bool
}

func (r *Rotation) doSyncJob() {

	for job := range r.syncJobs {
		f, offset, size := job.f, job.offset, job.size
		if size == 0 {
			continue
		}
		fnc.FlushHint(f, offset, size)
		if job.isBackup {
			// Warning:
			// 1. May drop too much cache,
			// because log ship may still need the cache(a bit slower than writing).
			// 2. May still has same cache,
			// because these page cache haven't been flushed to disk.
			fnc.DropCache(f, 0, r.conf.MaxSize)
			f.Close()
		}
	}
}

func (r *Rotation) Close() (err error) {
	// TODO exit goroutine flush all
	return r.file.Close()
}
