package cli

import (
	"fmt"
	"strings"
	"syscall"
)

// Human-readable signal names accepted on -s. Includes the common POSIX signals a user script might want to escalate
// through. SIG prefix stripped case-insensitively at parse time, so "TERM", "term", "SIGTERM", "sigterm" all resolve.
var signalNames = map[string]syscall.Signal{
	"HUP":  syscall.SIGHUP,
	"INT":  syscall.SIGINT,
	"QUIT": syscall.SIGQUIT,
	"ABRT": syscall.SIGABRT,
	"KILL": syscall.SIGKILL,
	"ALRM": syscall.SIGALRM,
	"TERM": syscall.SIGTERM,
	"USR1": syscall.SIGUSR1,
	"USR2": syscall.SIGUSR2,
}

// Parse a single -s value (e.g. "TERM", "SIGTERM", "sigterm") into a syscall.Signal.
func ParseSignal(name string) (syscall.Signal, error) {
	n := strings.ToUpper(strings.TrimSpace(name))
	n = strings.TrimPrefix(n, "SIG")
	if sig, ok := signalNames[n]; ok {
		return sig, nil
	}
	return 0, fmt.Errorf("--signal: %q is not a known signal name (expected TERM, INT, HUP, QUIT, ABRT, ALRM, KILL, USR1, USR2, with or without SIG prefix)", name)
}

// Parse the accumulated -s values (already split on commas by pflag's StringSlice) into an ordered slice of signals.
// Empty input yields the default escalation: TERM then KILL. If the user provided a list without KILL anywhere in it,
// KILL is appended as the final unignorable step.
func ParseSignals(ss []string) ([]syscall.Signal, error) {
	if len(ss) == 0 {
		return []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL}, nil
	}
	out := make([]syscall.Signal, 0, len(ss)+1)
	hasKill := false
	for _, s := range ss {
		sig, err := ParseSignal(s)
		if err != nil {
			return nil, err
		}
		out = append(out, sig)
		if sig == syscall.SIGKILL {
			hasKill = true
		}
	}
	if !hasKill {
		out = append(out, syscall.SIGKILL)
	}
	return out, nil
}
