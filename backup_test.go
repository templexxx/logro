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
	"testing"
	"time"
)

// Test Backups(Heap)
func TestBackups_Heap(t *testing.T) {
	s := make(Backups, 0, 3)
	b := &s

	if heap.Pop(b) != nil {
		t.Fatal("should be empty")
	}

	heap.Push(b, Backup{1, "fp1"})
	if b.Len() != 1 {
		t.Fatal("should has 1")
	}

	v := heap.Pop(b).(Backup)
	if v.ts != 1 || v.fp != "fp1" {
		t.Fatal("value mismatch")
	}
}

func TestBackups_List(t *testing.T) {
	s := make(Backups, 0, 3)
	b := &s
	dir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(dir)
	fn := "a.log"
	output := filepath.Join(dir, fn)

	b.list(output, 1)
	if b.Len() != 0 {
		t.Fatal("should be empty")
	}

	bf, _ := makeBackupFP(fn, false, time.Now())
	_, err = os.Create(filepath.Join(dir, bf))
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(bf)
	b.list(output, 1)
	if b.Len() != 1 {
		t.Fatal("mismatch backups len")
	}
	heap.Pop(b)
	time.Sleep(time.Millisecond)
	bf2, ts2 := makeBackupFP(fn, false, time.Now())
	_, err = os.Create(filepath.Join(dir, bf2))
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(bf2)
	b.list(output, 1)
	if b.Len() != 1 {
		t.Fatal("mismatch backups len", b.Len())
	}

	if heap.Pop(b).(Backup).ts != ts2 {
		t.Fatal("mismatch backups ts")
	}
}

func TestGetPrefixAndExt(t *testing.T) {
	output := "a/b.log"
	prefix, ext := getPrefixAndExt(output)
	if prefix != "b-" || ext != ".log" {
		t.Fatal("prefix/log mismatch")
	}
}

func TestBackupFP(t *testing.T) {
	now := time.Now()
	fnBase := "a"
	fnExt := ".log"
	fn := fnBase + fnExt

	// Test Make.
	utc := fmt.Sprintf("%s-%s%s", fnBase, now.UTC().Format(backupTimeFmt), fnExt)
	actFP, actTS := makeBackupFP(fn, false, now)
	if actFP != utc || actTS != now.Unix() {
		t.Fatal("make: mismatch UTC time")
	}

	local := fmt.Sprintf("%s-%s%s", fnBase, now.Format(backupTimeFmt), fnExt)
	actFP, actTS = makeBackupFP(fn, true, now)
	if actFP != local || actTS != now.Unix() {
		t.Fatal("make: mismatch Local time")
	}

	// Test Parse Time
	prefix, ext := getPrefixAndExt(fn)

	actTS = parseTime(utc, prefix, ext)
	if actTS != now.Unix() {
		t.Fatal("parse: mismatch UTC time")
	}

	actTS = parseTime(local, prefix, ext)
	if actTS != now.Unix() {
		t.Fatal("parse: mismatch Local time")
	}
}
