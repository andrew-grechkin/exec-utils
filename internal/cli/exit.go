package cli

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// Exit codes shared across exec-utils commands. The convention follows
// coreutils' timeout(1): 124 for the wrapper's timeout signal, 125 for
// the wrapper's own errors (bad flags, internal I/O), 126 for a command
// that was found but couldn't be invoked, 127 for a command not found.
// This leaves 0-123 free for the wrapped command's own exit codes, so
// callers can distinguish "the child failed with 2" from "we rejected
// the flags".
const (
	ExitOK           = 0
	ExitTimeout      = 124
	ExitWrapperError = 125
	ExitCannotInvoke = 126
	ExitNotFound     = 127
)

// Convert an *exec.Cmd Run/Start/Wait error into the appropriate exit code. Precedence:
//   - nil error -> ExitOK
//   - *exec.ExitError, signaled -> 128 + signum (shell convention: 128+SIGTERM=143, 128+SIGKILL=137)
//   - *exec.ExitError -> child's own exit code
//   - exec.ErrNotFound anywhere in the chain -> ExitNotFound (127)
//   - os.ErrPermission anywhere in the chain -> ExitCannotInvoke (126)
//   - any other pre-child failure -> ExitCannotInvoke (126)
func ExitCodeFor(err error) int {
	if err == nil {
		return ExitOK
	}
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return ee.ExitCode()
	}
	if errors.Is(err, exec.ErrNotFound) {
		return ExitNotFound
	}
	if errors.Is(err, os.ErrPermission) {
		return ExitCannotInvoke
	}
	return ExitCannotInvoke
}

// Report whether err represents a wrapper-side failure (couldn't fork/exec, binary not found, permission denied,
// pre-child I/O) rather than the child's own outcome (normal exit or signal death). Distinguishes on the error type,
// not the exit code, so future wrapper failure modes flow through without touching call sites. Callers use this to
// decide whether to print an extra "<util>: <err>" diagnostic: on wrapper errors the message carries the only signal
// to the user; on child outcomes the exit code (or the child's own STDERR, or the escalation log) already speaks.
func IsWrapperError(err error) bool {
	if err == nil {
		return false
	}
	_, fromChild := errors.AsType[*exec.ExitError](err)
	return !fromChild
}
