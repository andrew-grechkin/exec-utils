package main

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/andrew-grechkin/exec-utils/internal/cli"
)

const (
	defaultDelay    = 2 * time.Second
	defaultMaxDelay = 30 * time.Second
	maxGrace        = 5 * time.Second
)

type Config struct {
	Command []string

	Max          int              // 0 = unlimited
	Delay        time.Duration    // base delay
	Backoff      BackoffSpec      // strategy
	MaxDelay     time.Duration    // cap on Backoff.Delay(...)
	Jitter       float64          // 0..1 fraction of noise applied to each delay
	Timeout      time.Duration    // per-attempt timeout (0 = none)
	TotalTimeout time.Duration    // total wall-clock cap (0 = none)
	RetryCodes   map[int]struct{} // empty = retry on any non-zero
	Signals      []syscall.Signal // escalation list for per-attempt timeout kill (TERM,KILL by default)
	Verbose      bool
}

// Turn the pflag StringSlice values into a set of exit codes. pflag has already split on commas and accumulated
// repeats, so each element is one code string. An empty or nil slice yields an empty map, interpreted as "retry on
// any non-zero" downstream. Every element must be a bare integer -- no leading/trailing whitespace, no empty entries
// from stray commas; the error message names --code and quotes the offending token so the exact character causing
// the problem is visible.
func ParseCodes(ss []string) (map[int]struct{}, error) {
	out := map[int]struct{}{}
	for _, s := range ss {
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("--code: %q is not a valid exit code (expected a bare integer; no whitespace, no empty entries from stray commas)", s)
		}
		out[n] = struct{}{}
	}
	return out, nil
}

// Report whether the given exit code should trigger another attempt.
// Empty set: retry on any non-zero (default; stop on success). Non-empty set: retry only when code matches -- include 0
// to invert the loop and detect the first failure across otherwise-passing runs (flake detection).
func (c *Config) shouldRetry(code int) bool {
	if len(c.RetryCodes) == 0 {
		return code != 0
	}
	_, ok := c.RetryCodes[code]
	return ok
}

func applyJitter(d time.Duration, jitter float64, rng *rand.Rand) time.Duration {
	if jitter <= 0 {
		return d
	}

	noise := (rng.Float64()*2 - 1) * jitter
	out := max(time.Duration(float64(d)*(1+noise)), 0)

	return out
}

// Prepare stdin (buffer when replayable, pass through when it's a tty), then re-run the command, respecting timeouts
// and exit-code filter and sleeping with the chosen backoff between attempts.
func doRetry(cfg *Config, stdin io.Reader, stdout, stderr io.Writer) int {
	stdinFn, err := prepareStdin(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "retry: read stdin: %v\n", err)
		return cli.ExitWrapperError
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	start := time.Now()
	// Set by runOnce's signal forwarder whenever an external SIGINT/SIGTERM/SIGHUP arrives during an attempt. On a
	// user-driven kill (Ctrl-C) the whole subtree is being torn down; the loop must stop iterating rather than
	// spawning a fresh attempt into the shutdown.
	var interrupted atomic.Bool

	var lastCode int
	for attempt := 1; cfg.Max == 0 || attempt <= cfg.Max; attempt++ {
		if attempt > 1 && cfg.timedOut(start) {
			cfg.logf(stderr, "total timeout %s exceeded before attempt %d; giving up", cfg.TotalTimeout, attempt)
			return cli.ExitTimeout
		}

		code := runOnce(cfg, stdinFn(), stdout, stderr, &interrupted)
		lastCode = code

		if interrupted.Load() {
			cfg.logf(stderr, "interrupted during attempt %d; propagating exit %d", attempt, code)
			return code
		}

		if !cfg.shouldRetry(code) {
			// Either default-mode success, or terminal signal in a custom
			// retry set (including "-c 0" flake mode seeing a failure).
			cfg.logf(stderr, "attempt %d exit %d; done", attempt, code)
			return code
		}

		if cfg.Max > 0 && attempt >= cfg.Max {
			cfg.logf(stderr, "reached max %d attempts (last exit %d)", cfg.Max, code)
			return code
		}

		d, ok := cfg.nextDelay(attempt, start, rng)
		if !ok {
			cfg.logf(stderr, "total timeout %s exceeded; giving up", cfg.TotalTimeout)
			return cli.ExitTimeout
		}

		cfg.logf(stderr, "attempt %d exit %d; sleeping %s", attempt, code, d)
		time.Sleep(d)
	}

	return lastCode
}

// Report whether the total wall-clock cap has been reached. When TotalTimeout is 0 (unset), always false.
func (c *Config) timedOut(start time.Time) bool {
	return c.TotalTimeout > 0 && time.Since(start) >= c.TotalTimeout
}

// Compute the sleep between attempt `n` and `n+1`: base * strategy, capped by MaxDelay, perturbed by jitter, and
// finally clipped to the remaining TotalTimeout budget.
// Return (delay, true) when a sleep should happen, or (0, false) when the total budget is exhausted and the caller
// should stop retrying.
func (c *Config) nextDelay(attempt int, start time.Time, rng *rand.Rand) (time.Duration, bool) {
	d := c.Backoff.Delay(attempt, c.Delay)
	if c.MaxDelay > 0 && d > c.MaxDelay {
		d = c.MaxDelay
	}

	d = applyJitter(d, c.Jitter, rng)
	if c.TotalTimeout > 0 {
		remaining := c.TotalTimeout - time.Since(start)
		if remaining <= 0 {
			return 0, false
		}
		if d > remaining {
			d = remaining
		}
	}

	return d, true
}

// Execute the wrapped command once. Return the child's own exit code, or ExitCannotInvoke/ExitNotFound (126/127) when
// the process couldn't be launched. When --timeout fires, walk the --signal escalation on the child's process group
// with graceFor(--timeout) between each step, giving well-behaved children a chance to clean up before SIGKILL.
// interrupted is flipped to true by the shared signal forwarder if a terminal-driven kill (INT/TERM/HUP) arrives
// during this attempt, so doRetry can stop iterating rather than spawn a fresh attempt into a shutdown.
func runOnce(cfg *Config, stdin io.Reader, stdout, stderr io.Writer, interrupted *atomic.Bool) int {
	cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.SysProcAttr = cli.NewChildProcAttr()

	if err := cmd.Start(); err != nil {
		if cli.IsWrapperError(err) {
			fmt.Fprintf(stderr, "retry: %v\n", err)
		}
		return cli.ExitCodeFor(err)
	}

	stopFwd := cli.StartSignalForwarder(cmd.Process.Pid, interrupted)
	defer stopFwd()

	done := make(chan struct{})
	var wg sync.WaitGroup
	if cfg.Timeout > 0 {
		pgid := cmd.Process.Pid
		grace := graceFor(cfg.Timeout)
		wg.Go(func() {
			select {
			case <-done:
				return
			case <-time.After(cfg.Timeout):
				cfg.logf(stderr, "per-attempt timeout %s exceeded; escalating", cfg.Timeout)
				cli.Escalate(pgid, cfg.Signals, grace, done, func(f string, a ...any) {
					cfg.logf(stderr, f, a...)
				})
			}
		})
	}

	waitErr := cmd.Wait()
	close(done)
	wg.Wait()

	if cli.IsWrapperError(waitErr) {
		fmt.Fprintf(stderr, "retry: %v\n", waitErr)
	}
	return cli.ExitCodeFor(waitErr)
}

// Pick the graceful-shutdown window between the first signal and SIGKILL for a per-attempt timeout of t. Half the
// timeout, capped at maxGrace: a short bound stays short (users of -t 100ms expect fast bounds), a long bound gets
// meaningful cleanup time.
func graceFor(t time.Duration) time.Duration {
	g := min(t/2, maxGrace)
	return g
}

// Return a factory that produces the STDIN for each attempt. For a tty STDIN pass the underlying *os.File through
// unbuffered (otherwise io.ReadAll would block waiting for EOF); for anything else (pipe, file, tests) buffer once and
// hand out a fresh bytes.Reader per attempt so the wrapped command sees the same bytes on every try.
func prepareStdin(stdin io.Reader) (func() io.Reader, error) {
	if f, ok := stdin.(*os.File); ok {
		if info, err := f.Stat(); err == nil && (info.Mode()&os.ModeCharDevice) != 0 {
			return func() io.Reader { return f }, nil
		}
	}

	buf, err := io.ReadAll(stdin)
	if err != nil {
		return nil, err
	}

	return func() io.Reader { return bytes.NewReader(buf) }, nil
}

func (c *Config) logf(stderr io.Writer, format string, args ...any) {
	if !c.Verbose {
		return
	}
	fmt.Fprintf(stderr, "retry: "+format+"\n", args...)
}
