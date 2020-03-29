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
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/templexxx/fnc"
)

type output struct {
	fp      string
	f       *os.File
	maxSize int64

	backups    *Backups
	localTime  bool
	maxBackups int
}

func newOutput(fp string, maxSize int64, backups *Backups, localTime bool, maxBackups int) *output {
	return &output{
		fp:      fp,
		f:       nil,
		maxSize: maxSize,

		backups:    backups,
		localTime:  localTime,
		maxBackups: maxBackups,
	}
}

// open opens a new log file.
// If log file existed, move it to backups.
func (o *output) open() (err error) {

	fp := o.fp
	if fnc.Exist(fp) { // file exist may happen in rotation process.
		backupFP, t := makeBackupFP(fp, o.localTime, time.Now())
		err = os.Rename(fp, backupFP)
		if err != nil {
			return fmt.Errorf("failed to rename log file, output: %s backup: %s", fp, backupFP)
		}

		heap.Push(o.backups, Backup{t, backupFP})
		if o.backups.Len() > o.maxBackups {
			v := heap.Pop(o.backups)
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
	f, err := fnc.OpenFile(fp, flag, 0644)
	if err != nil {
		return fmt.Errorf("failed to create log file: %s", err.Error())
	}
	fnc.PreAllocate(f, o.maxSize)

	o.f = f
	return
}
