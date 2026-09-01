// Serialize STDOUT across parallel workers. Run the given command (letting many spew instances execute concurrently),
// capture the child's STDOUT in memory, then acquire an exclusive flock on the resolved target of STDOUT only to emit
// the captured bytes. The lock is held only during the write not during the child's runtime.
package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/pflag"

	"github.com/andrew-grechkin/exec-utils/internal/cli"
)

//go:embed help.txt
var helpText []byte

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := pflag.NewFlagSet("spew", pflag.ContinueOnError)
	fs.SetInterspersed(false)
	fs.SetOutput(io.Discard)

	var showHelp, showVersion bool
	fs.BoolVarP(&showHelp, "help", "h", false, "")
	fs.BoolVarP(&showVersion, "version", "v", false, "")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "spew: %v\n", err)
		return cli.ExitWrapperError
	}

	if showHelp {
		_, _ = stdout.Write(helpText)
		return cli.ExitOK
	}
	if showVersion {
		if err := cli.PrintVersion(stdout); err != nil {
			fmt.Fprintf(stderr, "spew: %v\n", err)
			return cli.ExitWrapperError
		}
		return cli.ExitOK
	}

	cmdArgs := fs.Args()
	if len(cmdArgs) == 0 {
		_, _ = stdout.Write(helpText)
		return cli.ExitOK
	}

	code, err := spew(cmdArgs, stdin, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "spew: %v\n", err)
	}
	return code
}

// Run the given command UNLOCKED while capturing the child's STDOUT to an in-memory buffer, then acquires the flock
// only to emit the captured bytes. STDERR streams through to the caller unchanged (spew's atomicity contract is about
// stdout only). The command's exit code is returned; a non-nil err indicates a spew-internal failure (buffering,
// locking, launching or emitting) rather than the child's own failure
func spew(args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	var outBuf bytes.Buffer

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = stdin
	cmd.Stdout = &outBuf
	cmd.Stderr = stderr
	cmd.SysProcAttr = cli.NewChildProcAttr()

	runErr := cli.RunChild(cmd)
	exitCode := cli.ExitCodeFor(runErr)
	if cli.IsWrapperError(runErr) {
		fmt.Fprintf(stderr, "spew: %v\n", runErr)
	}

	// Emit whatever was captured, even on child failure. Locking regardless keeps parallel siblings' output atomic even
	// in the failure case
	release, err := acquireLock()
	if err != nil {
		return cli.ExitWrapperError, err
	}
	defer release()

	if _, err := stdout.Write(outBuf.Bytes()); err != nil {
		return cli.ExitWrapperError, fmt.Errorf("emit: %w", err)
	}

	return exitCode, nil
}

// Open the lock target and take an exclusive flock on it. Closing the returned release func drops the lock
func acquireLock() (release func(), err error) {
	target := resolveLockTarget()

	f, err := os.Open(target)
	if err != nil {
		return nil, fmt.Errorf("open lock target %q: %w", target, err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("flock %q: %w", target, err)
	}

	return func() { _ = f.Close() }, nil
}

// The canonical filesystem path stdout points at (a tty, a regular file), falling back to the running executable when
// STDOUT is a pipe, socket or anonymous fd that has no filesystem entry to lock on
func resolveLockTarget() string {
	if target, err := os.Readlink("/proc/self/fd/1"); err == nil {
		if _, err := os.Stat(target); err == nil {
			return target
		}
	}

	if exe, err := os.Executable(); err == nil {
		return exe
	}

	return os.Args[0]
}
