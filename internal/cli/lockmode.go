package cli

import "fmt"

// LockMode picks what a lock-holding wrapper does when the lock/state is already claimed. Shared between ratelimit
// (mtime-driven cooldown) and once (flock contention).
type LockMode int

const (
	LockWait LockMode = iota // Default: block until we can proceed.
	LockSkip                 // Exit 0 without running.
	LockFail                 // Exit ExitWrapperError without running.
)

// Pick exactly one mode from the three mutually-exclusive booleans. Return an error if more than one is set; default
// to LockWait when none is set (the "just do the safe thing" default that every mode-carrying util in the fleet
// uses).
func ResolveLockMode(wait, skip, fail bool) (LockMode, error) {
	set := 0
	for _, b := range []bool{wait, skip, fail} {
		if b {
			set++
		}
	}
	if set > 1 {
		return 0, fmt.Errorf("--wait / --skip / --fail are mutually exclusive; pick one")
	}
	switch {
	case skip:
		return LockSkip, nil
	case fail:
		return LockFail, nil
	default:
		return LockWait, nil
	}
}
