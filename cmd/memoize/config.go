package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/andrew-grechkin/exec-utils/internal/cli"
	"github.com/andrew-grechkin/exec-utils/internal/duration"
)

type Config struct {
	Command []string

	Buffer    bool
	Clear     bool
	CacheOnly bool
	Empty     bool
	Slide     bool
	Verbose   bool
	Workdir   bool
	File      string
	Label     string
	TTLRaw    string

	Dir      string
	Filename string
	TTL      time.Duration
	Pwd      string
}

// Populate Config fields from environment and computes cache-key hashes.
// Must run after flag parsing, before any cache operations.
func (c *Config) resolve() error {
	c.Dir = cli.ResolveStateDir("", "MEMOIZE_DIR", defaultDirName)
	c.Filename = os.Getenv("MEMOIZE_FILENAME")

	ttlStr := c.TTLRaw
	if ttlStr == "" {
		ttlStr = os.Getenv("MEMOIZE_TTL")
	}
	if ttlStr == "" {
		c.TTL = defaultTTL
	} else {
		d, err := duration.Parse(ttlStr)
		if err != nil {
			return fmt.Errorf("cannot parse TTL: %s", ttlStr)
		}
		c.TTL = d
	}

	pwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}
	c.Pwd = pwd

	if c.Filename == "" {
		var b strings.Builder
		if c.Workdir {
			b.WriteString(pwd)
			b.WriteByte('\n')
		}
		b.WriteString(strings.Join(c.Command, " "))
		b.WriteByte('\n')
		c.Filename = cli.SHA256Hex(b.String())
	}

	return nil
}

// Return the directory holding this invocation's cache files:
// $MEMOIZE_DIR, or $MEMOIZE_DIR/<sha256(label)> when -l is set.
func (c *Config) cacheDir() string {
	if c.Label == "" {
		return c.Dir
	}
	return filepath.Join(c.Dir, cli.SHA256Hex(c.Label))
}

// Return the path where the cached stdout is stored. When -f is
// set the label is ignored and the caller-provided path is used verbatim
func (c *Config) cacheFile() string {
	if c.File != "" {
		return c.File
	}
	return filepath.Join(c.cacheDir(), c.Filename+dataSuffix)
}

// Return the path where the debug reproducer script is written,
// or "" when -f is set (in that case we don't manage a companion .sh)
func (c *Config) scriptFile() string {
	if c.File != "" {
		return ""
	}
	return filepath.Join(c.cacheDir(), c.Filename+scriptSuffix)
}

// Report whether the cache at path is a valid hit for this config:
// exists, is non-empty (or -e was passed), and its mtime is newer than
// time.Now() - TTL.
func (c *Config) isFresh(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	if info.Size() == 0 && !c.Empty {
		return false
	}

	return time.Since(info.ModTime()) < c.TTL
}

func (c *Config) logf(stderr io.Writer, format string, args ...any) {
	if !c.Verbose {
		return
	}

	fmt.Fprintf(stderr, format+"\n", args...)
}
