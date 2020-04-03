/*
 * Copyright (c) 2019. Temple3x (temple3x@gmail.com)
 * Copyright (c) 2014 Nate Finch
 *
 * Use of this source code is governed by the MIT License
 * that can be found in the LICENSE file.
 */

package logro

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

	// BufSize is logro's write buffer size.
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

// Use variables for tests easier.
var (
	kb int64 = 1024
	mb int64 = 1024 * 1024
)

// Default configs.
var (
	defaultBufSize       = 128 * mb
	defaultFileWriteSize = 256 * kb
	defaultFlushSize     = mb

	// We don't need to keep too many backups,
	// in practice, log shipper will collect the logs.
	defaultMaxSize    = 128 * mb
	defaultMaxBackups = 8
)

func (c *Config) adjust() {

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

	if c.BufSize*1024 < 2*c.FileWriteSize || c.FlushSize < 2*c.FileWriteSize {
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
