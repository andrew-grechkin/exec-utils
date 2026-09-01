package cli

import "syscall"

// NewChildProcAttr returns the SysProcAttr every exec-utils wrapper assigns to its child *exec.Cmd before Start.
// `Setpgid: true` with no Pgid puts the child in a FRESH process group where pgid == child.Pid. The child is the
// leader of that group; anything the child spawns (helpers via `bash -c '... &'`, `coproc`, background pipelines)
// inherits the same pgid, so the wrapper has a single kill(-pgid) target that covers the entire descendant subtree.
//
// The wrapper itself is NOT in the child's pgroup. That is deliberate: it lets the wrapper safely do
// `kill(-childpgid, SIGKILL)` for escalation without killing itself (SIGKILL is uncatchable, so
// "wrapper is in the pgroup and catches KILL via signal.Notify" is NOT a workaround, only signal-forwarding to a
// group we're outside of is).
//
// The consequence of that isolation is that terminal signals (Ctrl-C, SIGHUP on ssh disconnect) delivered to the
// wrapper's foreground pgroup no longer reach the child. That is compensated by StartSignalForwarder, which every
// wrapper installs.
func NewChildProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
