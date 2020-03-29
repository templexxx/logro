/*
 * Copyright (c) 2019. Temple3x (temple3x@gmail.com)
 * Copyright (c) 2014 Nate Finch
 *
 * Use of this source code is governed by the MIT License
 * that can be found in the LICENSE file.
 */

package logro

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func TestOutput_open(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	fn := "a.log"
	fp := filepath.Join(dir, fn)

	_, err = makeBackups(fp, 3)
	if err != nil {
		t.Fatal(err)
	}

	b, err := listBackups(fp, 2)
	if err != nil {
		t.Fatal(err)
	}

	o := newOutput(fp, 4096, b, false, 2)

	// open no exist.
	err = o.open()
	if err != nil {
		t.Fatal(err)
	}
	f0 := o.f
	defer f0.Close()

	if f0.Name() != fp {
		t.Fatal("file name mismatch")
	}

	// open exist.
	err = o.open()
	if err != nil {
		t.Fatal(err)
	}
	f1 := o.f
	defer f1.Close()

	if f1.Name() != fp {
		t.Fatal("file name mismatch")
	}
}
