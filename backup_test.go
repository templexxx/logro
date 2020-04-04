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
	"strconv"
	"testing"
	"time"
)

// Test Backups(Heap)
func TestBackups_Heap(t *testing.T) {
	s := make(Backups, 0, 3)
	b := &s

	if heap.Pop(b) != nil || b.Len() != 0 {
		t.Fatal("should be empty")
	}

	for i := 4; i >= 0; i-- {
		heap.Push(b, Backup{ts: int64(i), fp: strconv.Itoa(i)})
	}

	if b.Len() != 5 {
		t.Fatal("len mismatch")
	}

	i := 0
	for {
		val := heap.Pop(b) // Pop min.
		if val == nil {
			break
		}
		v := val.(Backup)
		if v.ts != int64(i) || v.fp != strconv.Itoa(i) {
			t.Fatal("value mismatch", v.ts, i)
		}
		i++
	}
}

func TestListBackups(t *testing.T) {
	testListBackupsPathError(t, 2)

	for i := 0; i < 3*2; i++ {
		testListBackups(t, i, 2)

	}
}

func testListBackupsPathError(t *testing.T, maxBackups int) {

	dir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(dir)
	fn := "logro-test.log"
	output := filepath.Join(dir, fn)

	b, err := listBackups(output, maxBackups)
	if err == nil || b != nil {
		t.Fatal("should raise path error")
	}
}

func makeBackups(output string, n int) (TSs []int64, err error) {

	TSs = make([]int64, n)
	now := time.Now()
	for i := 0; i < n; i++ {
		fn, ts := makeBackupFP(output, false, now.Add(time.Second*time.Duration(int64(i))))
		TSs[i] = ts
		_, err = os.Create(fn)
		if err != nil {
			return
		}
	}

	// Create some illegal backup log file/dir.
	// listBackups should ignore them.
	os.Mkdir(filepath.Join(filepath.Dir(output), "dir"), 0755)
	os.Create(filepath.Join(filepath.Dir(output), "c.log"))
	os.Create(filepath.Join(filepath.Dir(output), "a-c"))
	os.Create(filepath.Join(filepath.Dir(output), "a-c.log"))
	return
}

func testListBackups(t *testing.T, i, maxBackups int) {

	dir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	fn := "logro-test.log"
	output := filepath.Join(dir, fn)

	TSs, err := makeBackups(output, i)
	if err != nil {
		t.Fatal(err)
	}

	b, err := listBackups(output, maxBackups)
	if err != nil {
		t.Fatal(err)
	}

	cnt := maxBackups
	if len(TSs) < maxBackups {
		cnt = len(TSs)
	}

	if b.Len() != cnt {
		t.Fatal("mismatch backups len")
	}

	TSs = TSs[len(TSs)-cnt:]
	for _, ts := range TSs {

		val := heap.Pop(b)
		if val == nil {
			break
		}

		if val.(Backup).ts != ts {
			t.Fatal("mismatch backup ts")
		}
	}
}

func TestGetPrefixAndExt(t *testing.T) {
	output := "a/b.log"
	prefix, ext := getPrefixAndExt(output)
	if prefix != "b-" {
		t.Fatal("prefix mismatch")
	}

	if ext != ".log" {
		t.Fatal("ext mismatch")
	}
}

func TestMakeBackupFP(t *testing.T) {
	now := time.Now()
	fnBase := "logro-test"
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
