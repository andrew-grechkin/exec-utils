package cli

import (
	"syscall"
	"time"
)

// Deliver signals in order to the process group -pgid, waiting grace between each. The caller must have set Setpgid on
// the child so the whole group receives the signal, including any shell wrappers and their descendants. Return early
// when done fires (the child has exited), since further signals would be delivered to nothing. logf may be nil for
// silent operation; when non-nil it is called for each successful send and for kill(2) errors.
func Escalate(pgid int, sigs []syscall.Signal, grace time.Duration, done <-chan struct{}, logf func(format string, args ...any)) {
	for i, sig := range sigs {
		if err := syscall.Kill(-pgid, sig); err != nil {
			if logf != nil {
				logf("kill(-%d, %v): %v", pgid, sig, err)
			}
		} else if logf != nil {
			logf("sent %v to process group", sig)
		}

		// After the last signal (guaranteed to be KILL by ParseSignals) there is no next step to wait for.
		if i == len(sigs)-1 {
			return
		}

		select {
		case <-done:
			return
		case <-time.After(grace):
		}
	}
}
