package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/andrew-grechkin/exec-utils/internal/cli"
	"github.com/andrew-grechkin/exec-utils/internal/duration"
)

const (
	defaultInterval = time.Second
	defaultDirName  = "ratelimit"
)

// Config bundles flag-driven configuration and the resolved paths. Fields split into user inputs and resolve() outputs.
type Config struct {
	Command []string

	// Flags
	IntervalRaw string
	Mode        cli.LockMode
	Label       string
	DirOverride string
	Verbose     bool

	// Resolved
	Interval time.Duration
	Dir      string
	KeyFile  string
}

// Read env vars, parse interval, compute state-file path. Call after flag parsing, before doRatelimit.
func (c *Config) resolve() error {
	c.Dir = cli.ResolveStateDir(c.DirOverride, "RATELIMIT_DIR", defaultDirName)

	rawInterval := c.IntervalRaw
	if rawInterval == "" {
		rawInterval = os.Getenv("RATELIMIT_INTERVAL")
	}
	if rawInterval == "" {
		c.Interval = defaultInterval
	} else {
		d, err := duration.Parse(rawInterval)
		if err != nil {
			return fmt.Errorf("--interval: %q is not a valid duration", rawInterval)
		}
		c.Interval = d
	}

	key := c.Label
	if key == "" {
		key = strings.Join(c.Command, " ")
	}
	c.KeyFile = filepath.Join(c.Dir, cli.SHA256Hex(key))
	return nil
}

// Do the rate-limit dance: open+lock the state file, check its mtime, dispatch on Mode, run COMMAND when allowed.
// Return the exit code to propagate.
func doRatelimit(cfg *Config, stdin io.Reader, stdout, stderr io.Writer) int {
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		fmt.Fprintf(stderr, "ratelimit: mkdir %s: %v\n", cfg.Dir, err)
		return cli.ExitWrapperError
	}

	f, err := os.OpenFile(cfg.KeyFile, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		fmt.Fprintf(stderr, "ratelimit: open %s: %v\n", cfg.KeyFile, err)
		return cli.ExitWrapperError
	}
	defer f.Close()

	// Hold flock through the whole decision, including any wait sleep, so concurrent callers queue behind us instead of
	// all firing at once when the cooldown clears.
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		fmt.Fprintf(stderr, "ratelimit: flock %s: %v\n", cfg.KeyFile, err)
		return cli.ExitWrapperError
	}

	info, err := f.Stat()
	if err != nil {
		fmt.Fprintf(stderr, "ratelimit: stat %s: %v\n", cfg.KeyFile, err)
		return cli.ExitWrapperError
	}

	// A brand-new file (just created above) reports its birth mtime but the file's size is 0 and we've never recorded a
	// run. Treat the "never run before" case as "cooldown expired" so the first invocation runs immediately.
	firstRun := info.Size() == 0
	remaining := cfg.Interval - time.Since(info.ModTime())

	if !firstRun && remaining > 0 {
		switch cfg.Mode {
		case cli.LockSkip:
			cfg.logf(stderr, "cooldown %s remaining, skipping", remaining.Round(time.Millisecond))
			return cli.ExitOK
		case cli.LockFail:
			fmt.Fprintf(stderr, "ratelimit: cooldown %s remaining\n", remaining.Round(time.Millisecond))
			return cli.ExitWrapperError
		case cli.LockWait:
			cfg.logf(stderr, "cooldown %s remaining, waiting", remaining.Round(time.Millisecond))
			time.Sleep(remaining)
		}
	}

	// Touch mtime BEFORE running so concurrent callers who acquire the lock next see the new timestamp. Any content
	// (empty file, 1 byte, whatever) works; we just need mtime and non-zero size to distinguish "we've run" from
	// "brand new". Write a single byte for the size marker.
	if _, err := f.WriteAt([]byte{'.'}, 0); err != nil {
		fmt.Fprintf(stderr, "ratelimit: write %s: %v\n", cfg.KeyFile, err)
		return cli.ExitWrapperError
	}
	now := time.Now()
	if err := os.Chtimes(cfg.KeyFile, now, now); err != nil {
		fmt.Fprintf(stderr, "ratelimit: chtimes %s: %v\n", cfg.KeyFile, err)
		return cli.ExitWrapperError
	}

	// Release lock BEFORE running so long-running commands don't hold up other callers. Reads of mtime by later
	// callers will see the timestamp we just set, which is the correct cooldown anchor.
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = cli.NewChildProcAttr()

	runErr := cli.RunChild(cmd)
	if cli.IsWrapperError(runErr) {
		fmt.Fprintf(stderr, "ratelimit: %v\n", runErr)
	}
	return cli.ExitCodeFor(runErr)
}

func (c *Config) logf(stderr io.Writer, format string, args ...any) {
	if !c.Verbose {
		return
	}
	fmt.Fprintf(stderr, "ratelimit: "+format+"\n", args...)
}
