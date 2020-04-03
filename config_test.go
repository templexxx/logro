/*
 * Copyright (c) 2019. Temple3x (temple3x@gmail.com)
 * Copyright (c) 2014 Nate Finch
 *
 * Use of this source code is governed by the MIT License
 * that can be found in the LICENSE file.
 */

package logro

import (
	"testing"
)

func TestConfigParse(t *testing.T) {
	cfg := new(Config)
	cfg.adjust()

	if cfg.MaxSize != defaultMaxSize {
		t.Fatal("mismatch")
	}

	if cfg.MaxBackups != defaultMaxBackups {
		t.Fatal("mismatch")
	}

	if cfg.BufSize != defaultBufSize {
		t.Fatal("mismatch")
	}

	if cfg.FileWriteSize != defaultFileWriteSize {
		t.Fatal("mismatch")
	}

	if cfg.FlushSize != defaultFlushSize {
		t.Fatal("mismatch")
	}
}

func TestConfigDevelop(t *testing.T) {
	bufSize := int64(64)
	kb = 1
	mb = 1
	cfg := &Config{
		Developed:     true,
		MaxSize:       bufSize * 2,
		BufSize:       bufSize,
		FileWriteSize: bufSize / 2,
		FlushSize:     bufSize,
	}
	cfg.adjust()
	if cfg.BufSize != bufSize {
		t.Fatal("bufSize mismatch")
	}
	if cfg.FileWriteSize != bufSize/2 {
		t.Fatal("fileWriteSize mismatch")
	}
	if cfg.FlushSize != bufSize {
		t.Fatal("flushSzie mismatch")
	}
}

func TestAlignToPage(t *testing.T) {
	for i := 1; i <= pageSize; i++ {
		if alignToPage(int64(i)) != pageSize {
			t.Fatal("align mismatch")
		}
	}
	for i := pageSize + 1; i < pageSize*2; i++ {
		if alignToPage(int64(i)) != pageSize*2 {
			t.Fatal("align mismatch")
		}
	}
}
