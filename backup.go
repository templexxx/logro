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
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Backup holds backup log file' path & create time.
type Backup struct {
	ts int64
	fp string
}

// Backups implements heap interface.
type Backups []Backup

func (b *Backups) Less(i, j int) bool {
	return (*b)[i].ts < ((*b)[j].ts)
}

func (b *Backups) Swap(i, j int) {
	if i >= 0 && j >= 0 {
		(*b)[i], (*b)[j] = (*b)[j], (*b)[i]
	}
}

func (b *Backups) Len() int {
	return len(*b)
}

func (b *Backups) Pop() (v interface{}) {
	if b.Len()-1 >= 0 {
		*b, v = (*b)[:b.Len()-1], (*b)[b.Len()-1]
	}
	return
}

func (b *Backups) Push(v interface{}) {
	*b = append(*b, v.(Backup))
}

// List all backup log files (in init process),
// and remove them if there are too many backups.
func (b *Backups) list(outputPath string, max int) {

	dir := filepath.Dir(outputPath)
	ns, err := ioutil.ReadDir(dir)
	if err != nil {
		return // Path error, ignore
	}

	prefix, ext := getPrefixAndExt(outputPath)

	for _, f := range ns {
		if f.IsDir() {
			continue
		}
		if ts := parseTime(f.Name(), prefix, ext); ts != 0 {
			heap.Push(b, Backup{ts, filepath.Join(dir, f.Name())})
			continue
		}
	}

	for b.Len() > max {
		v := heap.Pop(b)
		os.Remove(v.(Backup).fp)
	}
}

// getPrefixAndExt returns the filename part and extension part from the rotation's filename.
func getPrefixAndExt(outputPath string) (prefix, ext string) {
	name := filepath.Base(outputPath)
	ext = filepath.Ext(name)
	prefix = name[:len(name)-len(ext)] + "-"
	return prefix, ext
}

const backupTimeFmt = "2006-01-02T15:04:05.000Z0700"

// parseTime extracts the formatted time from the filename by stripping off
// the filename's prefix and extension. This prevents someone's filename from
// confusing time.parse.
func parseTime(fp, prefix, ext string) int64 {
	filename := filepath.Base(fp)
	if !strings.HasPrefix(filename, prefix) {
		return 0
	}
	if !strings.HasSuffix(filename, ext) {
		return 0
	}
	tsStr := filename[len(prefix) : len(filename)-len(ext)]
	t, err := time.Parse(backupTimeFmt, tsStr)
	if err != nil {
		return 0
	}
	return t.Unix()
}

func makeBackupFP(name string, local bool, t time.Time) (string, int64) {
	dir := filepath.Dir(name)
	filename := filepath.Base(name)
	ext := filepath.Ext(filename)
	prefix := filename[:len(filename)-len(ext)]
	if !local {
		t = t.UTC()
	}

	timestamp := t.Format(backupTimeFmt)
	return filepath.Join(dir, fmt.Sprintf("%s-%s%s", prefix, timestamp, ext)), t.Unix()
}
