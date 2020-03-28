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

// Config of logro.
type Config struct {
	// OutputPath is the log file path.
	OutputPath string `json:"output_path" toml:"output_path"`
	// MaxSize is the maximum size of a log file before it gets rotated.
	// Unit: MB.
	// Default: 128.
	MaxSize int64 `json:"max_size_mb" toml:"max_size_mb"`
	// MaxBackups is the maximum number of backup log files to retain.
	MaxBackups int `json:"max_backups" toml:"max_backups"`
	// LocalTime is the timestamp in backup log file. Default is to use UTC time.
	// If true, use local time.
	LocalTime bool `json:"local_time" toml:"local_time"`

	// BufSize is logro's buffer size.
	// Unit: MB.
	// Default: 64.
	BufSize int64 `json:"buf_size_mb" toml:"buf_size_mb"`
	// FileWriteSize writes data to file every FileWriteSize.
	// Unit: KB.
	// Default: 256.
	FileWriteSize int64 `json:"file_write_size_kb" toml:"file_write_size_kb"`
	// FlushSize flushes data to storage media(hint) every FlushSize.
	// Unit: KB.
	// Default: 1024.
	FlushSize int64 `json:"flush_size_kb" toml:"flush_size_kb"`

	// Develop mode. Default is false.
	// It' used for testing, if it's true, the page cache control unit could not be aligned to page cache size.
	Developed bool `json:"developed" toml:"developed"`
}

// Rotation is implement io.WriteCloser interface with func Sync() (err error).
type Rotation struct {
	sync.Mutex

	cfg *Config

	file  *os.File // output *os.File.
	fsize int64    // output file size.
	// dirty page cache info.
	// ps: may already been flushed by kernel.
	dirtyOffset int64
	dirtySize   int64
	// user-space buffer for log writing.
	buf *buffer

	fileWriteJobs chan int64
	flushJobs     chan flushJob

	// jobs of sync page cache, use chan for avoiding stall.
	syncJobs chan flushJob
	Backups  *Backups // all backups information.

	fileUnWritten int64

	output *output
}

// Use variables for tests easier.
var (
	kb int64 = 1024
	mb int64 = 1024 * 1024
)

// Default configs.
var (
	defaultBufSize       = 64 * mb
	defaultFileWriteSize = 256 * kb
	defaultFlushSize     = mb

	// We don't need to keep too many backups,
	// in practice, log shipper will collect the logs.
	defaultMaxSize    = 128 * mb
	defaultMaxBackups = 8
)

// New creates a Rotation.
func New(cfg *Config) (r *Rotation, err error) {

	if cfg.OutputPath == "" {
		return nil, errors.New("empty log file path")
	}
	cfg.parse()

	r = &Rotation{cfg: cfg}
	err = r.prepareOutput()
	if err != nil {
		return
	}

	r.buf = newBuffer(r.cfg.BufSize)
	r.fileWriteJobs = make(chan int64, 256)
	r.flushJobs = make(chan flushJob, 256)

	go r.flushLoop() // sync log content async.

	return
}

func (c *Config) parse() {

	if c.MaxSize <= 0 {
		c.MaxSize = defaultMaxSize
	} else {
		c.MaxSize = c.MaxSize * mb
	}
	if c.MaxBackups <= 0 {
		c.MaxBackups = defaultMaxBackups
	}

	if c.BufSize <= 0 {
		c.BufSize = defaultBufSize
	} else {
		c.BufSize = c.BufSize * mb
	}
	if c.FileWriteSize <= 0 {
		c.FileWriteSize = defaultFileWriteSize
	} else {
		c.FileWriteSize = c.FileWriteSize * kb
	}
	if c.FlushSize <= 0 {
		c.FlushSize = defaultFlushSize
	} else {
		c.FlushSize = c.FlushSize * kb
	}

	if c.BufSize <= 2*c.FileWriteSize || c.FlushSize <= 2*c.FileWriteSize {
		c.BufSize = defaultBufSize
		c.FileWriteSize = defaultFileWriteSize
		c.FlushSize = defaultFlushSize
	}

	if !c.Developed {
		c.MaxSize = alignToPage(c.MaxSize)
		c.FlushSize = alignToPage(c.FlushSize)
	}
}

const pageSize = 1 << 12 // 4KB.

func alignToPage(n int64) int64 {
	return (n + pageSize - 1) &^ (pageSize - 1)
}

func (r *Rotation) prepareOutput() error {
	backups := make(Backups, 0, r.cfg.MaxBackups*2)
	r.Backups = &backups
	r.Backups.list(r.cfg.OutputPath, r.cfg.MaxBackups)

	err := r.openExistOrNew()
	if err != nil {
		return err
	}
	return nil
}

// Open log file when start up.
// TODO check size after open
// TODO allocate after open new
func (r *Rotation) openExistOrNew() (err error) {

	fp := r.cfg.OutputPath
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
	fp := r.cfg.OutputPath
	if fnc.Exist(fp) { // file exist may happen in rotation process.
		backupFP, t := makeBackupFP(fp, r.cfg.LocalTime, time.Now())

		err = os.Rename(fp, backupFP)
		if err != nil {
			return fmt.Errorf("failed to rename log file, output: %s backup: %s", fp, backupFP)
		}

		r.sync(true)

		heap.Push(r.Backups, Backup{t, backupFP})
		if r.Backups.Len() > r.cfg.MaxBackups {
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

	written = len(p)
	err = r.buf.write(p)
	if err != nil {
		r.fileWriteJobs <- 0
		return
	}

	r.fileWriteJobs <- int64(written)

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
		r.syncJobs <- flushJob{r.file, r.dirtyOffset, r.dirtySize, isBackup}
	}

	r.dirtyOffset += r.dirtySize
	r.dirtySize = 0
}

type flushJob struct {
	f        *os.File
	offset   int64
	size     int64
	isBackup bool
}

func (r *Rotation) flushLoop() {

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
			fnc.DropCache(f, 0, r.cfg.MaxSize)
			f.Close()
		}
	}
}

type output struct {
	f      *os.File
	size   int64
	offset int64
}

func (r *Rotation) fileWriteLoop() {

	p := make([]byte, r.cfg.BufSize)
	n := int64(0)
	for job := range r.fileWriteJobs {
		if job != 0 {
			n += job
			if n >= r.cfg.FileWriteSize {
				p0 := p[:n]
				err := r.buf.read(p0)
				if err != nil {
					read := r.buf.readAll(p)
					p0 = p[:read]
				}
				r.fileWrite(r.output, p0)
				n = 0
			}
		} else {
			read := r.buf.readAll(p)
			p0 := p[:read]
			r.fileWrite(r.output, p0)
			n = 0
		}
	}
}

func (r *Rotation) fileWrite(out *output, p []byte) {
	fw, err := out.f.WriteAt(p, out.offset)
	if err == nil {
		r.flushJobs <- flushJob{
			f:        out.f,
			offset:   out.offset,
			size:     int64(fw),
			isBackup: false,
		}
	}
	out.offset += int64(fw)
	out.size += int64(fw)

	if out.size > r.cfg.MaxSize {
		r.openNew()
		// TODO drop out.f cache
	}
}

func (r *Rotation) Close() (err error) {
	// TODO exit goroutine flush all
	return r.file.Close()
}
