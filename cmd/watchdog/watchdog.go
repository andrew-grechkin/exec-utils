package main

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/andrew-grechkin/exec-utils/internal/cli"
)

const (
	defaultIdle  = 1 * time.Minute
	defaultGrace = 10 * time.Second
	pumpBufSize  = 64 << 10
)

type Config struct {
	Command       []string
	Idle          time.Duration
	IncludeStderr bool
	Signals       []syscall.Signal
	Grace         time.Duration
	Verbose       bool
}

// Run COMMAND under an idle-timer. Pipes stdout (and stderr with -e) through a pumping goroutine that stamps
// lastActivity on every read. A supervisor goroutine wakes periodically to compare lastActivity against the idle
// budget; on stall it walks the signal list with --grace between each. The whole thing runs in its own process
// group so the escalation reaches shell wrappers and their children too.
func doWatchdog(cfg *Config, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
	cmd.Stdin = stdin
	cmd.SysProcAttr = cli.NewChildProcAttr()

	stdoutR, err := cmd.StdoutPipe()
	if err != nil {
		return cli.ExitWrapperError, fmt.Errorf("stdout pipe: %w", err)
	}
	var stderrR io.ReadCloser
	if cfg.IncludeStderr {
		stderrR, err = cmd.StderrPipe()
		if err != nil {
			return cli.ExitWrapperError, fmt.Errorf("stderr pipe: %w", err)
		}
	} else {
		cmd.Stderr = stderr
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(stderr, "watchdog: %v\n", err)
		return cli.ExitCodeFor(err), nil
	}
	pgid := cmd.Process.Pid
	stopFwd := cli.StartSignalForwarder(pgid, nil)
	defer stopFwd()

	var lastActivity atomic.Int64
	lastActivity.Store(time.Now().UnixNano())

	var pumps sync.WaitGroup
	pumps.Go(func() {
		pumpAndTrack(stdoutR, stdout, &lastActivity)
	})
	if cfg.IncludeStderr {
		pumps.Go(func() {
			pumpAndTrack(stderrR, stderr, &lastActivity)
		})
	}

	// The supervisor exits when the child exits, signaled via done. Escalation runs INSIDE the supervisor so we do
	// not need a second goroutine.
	done := make(chan struct{})
	go supervise(pgid, cfg, &lastActivity, done, stderr)

	pumps.Wait()
	waitErr := cmd.Wait()
	close(done)

	return cli.ExitCodeFor(waitErr), nil
}

// Read r line-by-line (bufio.Scanner default is line-oriented), stamp lastActivity on every read, and forward the
// bytes to w. Reading in lines rather than raw chunks means the activity timestamp updates once per line rather than
// once per burst, which is usually what "no output" means to a human watching a log.
func pumpAndTrack(r io.Reader, w io.Writer, last *atomic.Int64) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, pumpBufSize), pumpBufSize)
	for scanner.Scan() {
		last.Store(time.Now().UnixNano())
		line := scanner.Bytes()
		_, _ = w.Write(line)
		_, _ = w.Write([]byte{'\n'})
	}
}

// Watch the idle budget until the child exits (done closes). When idle is exceeded, walk the signal escalation with
// grace between each. If the child dies mid-escalation, done fires and we stop early.
func supervise(pgid int, cfg *Config, last *atomic.Int64, done <-chan struct{}, stderr io.Writer) {
	// Poll roughly 10 times per idle window; users don't need sub-millisecond precision on a stall detector.
	tick := max(cfg.Idle/10, 100*time.Millisecond)
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			elapsed := time.Since(time.Unix(0, last.Load()))
			if elapsed < cfg.Idle {
				continue
			}
			logf(cfg, stderr, "idle %s exceeded (limit %s); escalating", elapsed.Round(time.Millisecond), cfg.Idle)
			cli.Escalate(pgid, cfg.Signals, cfg.Grace, done, func(f string, a ...any) { logf(cfg, stderr, f, a...) })
			return
		}
	}
}

func logf(cfg *Config, stderr io.Writer, format string, args ...any) {
	if !cfg.Verbose {
		return
	}
	fmt.Fprintf(stderr, "watchdog: "+format+"\n", args...)
}
