package cli

import (
	"os"
	"os/exec"
	"os/signal"
	"sync/atomic"
	"syscall"
)

// ChildSignals is the set every wrapper forwards from itself to the child's process group. INT and TERM handle
// Ctrl-C and orderly kill; HUP handles terminal close (ssh disconnect, tmux pane close, cron finishing). The set is
// uniform across the fleet so a user's Ctrl-C at the top of a wrapper stack reaches every descendant no matter which
// wrapper is on top.
var ChildSignals = []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP}

// StartSignalForwarder installs a signal.Notify handler that relays any ChildSignals delivered to this process to
// the child's process group via kill(-pgid, sig). Returns a stop function to be deferred after cmd.Wait completes.
// pgid MUST equal the child's process-group leader, which is child.Pid when the child was spawned with
// NewChildProcAttr (Setpgid: true creates a new pgroup with pgid == child.Pid).
//
// interrupted (optional, nil-safe) is set to true the first time a signal is forwarded. Callers with retry loops
// use it to stop iterating instead of spawning a fresh attempt into a user-initiated kill.
//
// Why this exists in one sentence: NewChildProcAttr isolates the child into a new pgroup so the wrapper can
// aggregate-signal grandchildren without hitting itself; that isolation also blocks terminal signals from reaching
// the child, so this forwarder plugs the gap.
//
// See README's "DESIGN DECISIONS" section for the pgroup / forwarding trade-off in full.
func StartSignalForwarder(pgid int, interrupted *atomic.Bool) func() {
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, ChildSignals...)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case sig, ok := <-sigs:
				if !ok {
					return
				}
				if interrupted != nil {
					interrupted.Store(true)
				}
				_ = syscall.Kill(-pgid, sig.(syscall.Signal))
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(sigs)
		close(done)
	}
}

// RunChild is a drop-in for cmd.Run(): Start, forward ChildSignals to the child's pgroup for the lifetime of the
// child, then Wait. cmd MUST have Setpgid set (see NewChildProcAttr). A Start failure returns the start error and
// no forwarding is installed. Simple wrappers (memoize, ratelimit, once, spew) use this; wrappers that need custom
// Start/Wait sequencing (prefix, watchdog, retry) call StartSignalForwarder directly.
func RunChild(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	stop := StartSignalForwarder(cmd.Process.Pid, nil)
	defer stop()
	return cmd.Wait()
}
