package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/andrew-grechkin/exec-utils/internal/cli"
)

const defaultDirName = "once"

// Config bundles flag-driven inputs and the resolved paths. Split fields for readability.
type Config struct {
	Command []string

	// Flags
	Label       string
	DirOverride string
	Mode        cli.LockMode
	Verbose     bool

	// Resolved
	Dir     string
	KeyFile string
}

// Read env vars and compute the lockfile path. Call after flag parsing, before doOnce.
func (c *Config) resolve() error {
	c.Dir = cli.ResolveStateDir(c.DirOverride, "ONCE_DIR", defaultDirName)

	key := c.Label
	if key == "" {
		pwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getwd: %w", err)
		}
		// Namespace an anonymous invocation by pwd + argv so two shells in different directories don't collide, and
		// two invocations with different argv don't either.
		key = pwd + "\n" + strings.Join(c.Command, " ")
	}
	c.KeyFile = filepath.Join(c.Dir, cli.SHA256Hex(key))
	return nil
}

// Hold an exclusive flock on the label's keyfile for COMMAND's whole runtime. Mode picks the contention behavior:
// LockWait blocks, LockSkip exits 0, LockFail exits ExitWrapperError. The fd is CLOEXEC by default in Go, so the
// child never inherits the lock -- releasing on the parent's close is the correct handoff. Return the child's
// propagated exit code.
func doOnce(cfg *Config, stdin io.Reader, stdout, stderr io.Writer) int {
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		fmt.Fprintf(stderr, "once: mkdir %s: %v\n", cfg.Dir, err)
		return cli.ExitWrapperError
	}

	f, err := os.OpenFile(cfg.KeyFile, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		fmt.Fprintf(stderr, "once: open %s: %v\n", cfg.KeyFile, err)
		return cli.ExitWrapperError
	}
	defer f.Close()

	fd := int(f.Fd())
	switch cfg.Mode {
	case cli.LockWait:
		cfg.logf(stderr, "waiting on lock %s", cfg.KeyFile)
		if err := syscall.Flock(fd, syscall.LOCK_EX); err != nil {
			fmt.Fprintf(stderr, "once: flock %s: %v\n", cfg.KeyFile, err)
			return cli.ExitWrapperError
		}
	case cli.LockSkip, cli.LockFail:
		err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err != nil {
			if errors.Is(err, syscall.EWOULDBLOCK) {
				if cfg.Mode == cli.LockSkip {
					cfg.logf(stderr, "lock held elsewhere, skipping")
					return cli.ExitOK
				}
				fmt.Fprintln(stderr, "once: lock held elsewhere")
				return cli.ExitWrapperError
			}
			fmt.Fprintf(stderr, "once: flock %s: %v\n", cfg.KeyFile, err)
			return cli.ExitWrapperError
		}
	}
	cfg.logf(stderr, "lock acquired")

	cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = cli.NewChildProcAttr()

	runErr := cli.RunChild(cmd)
	if cli.IsWrapperError(runErr) {
		fmt.Fprintf(stderr, "once: %v\n", runErr)
	}
	return cli.ExitCodeFor(runErr)
}

func (c *Config) logf(stderr io.Writer, format string, args ...any) {
	if !c.Verbose {
		return
	}
	fmt.Fprintf(stderr, "once: "+format+"\n", args...)
}
