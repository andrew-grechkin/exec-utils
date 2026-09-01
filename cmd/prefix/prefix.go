package main

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andrew-grechkin/exec-utils/internal/cli"
)

const (
	tokenTimestamp   = 'T'
	tokenEpoch       = 't'
	tokenPID         = 'p'
	tokenSeq         = 'n'
	tokenPercent     = '%'
	timestampLayout  = "2006-01-02T15:04:05"
	scannerMaxBuffer = 1 << 20 // 1 MiB per line
)

type Config struct {
	Command    []string
	Prefix     string
	AlsoStderr bool
	Stream     bool
	Delimiter  string
}

// Return the caller's -p value or fall back to a bare timestamp when nothing was supplied. `prefix -- cmd` is the
// common case and shouldn't require any flag to produce useful output.
func resolveTemplate(cfg *Config) string {
	if cfg.Prefix != "" {
		return cfg.Prefix
	}
	return "%" + string(tokenTimestamp)
}

type prefixer struct {
	template   string
	pid        int
	streamMark string
	sep        string
	counter    atomic.Int64
}

// Materialize one prefix (including trailing separator). Called exactly once per line, so incrementing the counter and
// reading the clock here is correct even under concurrent stdout/stderr goroutines.
func (p *prefixer) format() string {
	seq := p.counter.Add(1)
	now := time.Now()

	var b strings.Builder
	b.WriteString(p.streamMark)
	for i := 0; i < len(p.template); i++ {
		c := p.template[i]
		if c != '%' || i+1 == len(p.template) {
			b.WriteByte(c)
			continue
		}
		i++
		switch p.template[i] {
		case tokenTimestamp:
			b.WriteString(now.Format(timestampLayout))
		case tokenEpoch:
			b.WriteString(strconv.FormatInt(now.Unix(), 10))
		case tokenPID:
			b.WriteString(strconv.Itoa(p.pid))
		case tokenSeq:
			b.WriteString(strconv.FormatInt(seq, 10))
		case tokenPercent:
			b.WriteByte('%')
		default:
			// Unknown token: pass through verbatim.
			b.WriteByte('%')
			b.WriteByte(p.template[i])
		}
	}
	b.WriteString(p.sep)

	return b.String()
}

// Read line-by-line, prefix each and writes to w with a trailing newline. Block until input hits EOF. Writes are
// serialized via mu so a stdout goroutine and stderr goroutine never interleave partial writes into shared destinations
// (rare but possible when the caller has merged 1 and 2, e.g. `prefix ... 2>&1`).
func pumpLines(r io.Reader, p *prefixer, w io.Writer, mu *sync.Mutex) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64<<10), scannerMaxBuffer)
	for scanner.Scan() {
		line := scanner.Bytes()
		mu.Lock()
		_, err := fmt.Fprintf(w, "%s%s\n", p.format(), line)
		mu.Unlock()
		if err != nil {
			return err
		}
	}
	return scanner.Err()
}

// Launch the wrapped command, pump its STDOUT (and STDERR when requested) through per-stream prefixers and return the
// child's exit code.
func doPrefix(cfg *Config, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	template := resolveTemplate(cfg)
	if cfg.Stream {
		cfg.AlsoStderr = true
	}

	cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
	cmd.Stdin = stdin
	cmd.SysProcAttr = cli.NewChildProcAttr()

	stdoutR, err := cmd.StdoutPipe()
	if err != nil {
		return cli.ExitWrapperError, fmt.Errorf("stdout pipe: %w", err)
	}

	var stderrR io.ReadCloser
	if cfg.AlsoStderr {
		stderrR, err = cmd.StderrPipe()
		if err != nil {
			return cli.ExitWrapperError, fmt.Errorf("stderr pipe: %w", err)
		}
	} else {
		cmd.Stderr = stderr
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(stderr, "prefix: %v\n", err)
		return cli.ExitCodeFor(err), nil
	}
	pid := cmd.Process.Pid
	stopFwd := cli.StartSignalForwarder(pid, nil)
	defer stopFwd()

	outPrefixer := &prefixer{template: template, pid: pid, sep: cfg.Delimiter}
	if cfg.Stream {
		outPrefixer.streamMark = "STDOUT| "
	}

	var errPrefixer *prefixer
	if cfg.AlsoStderr {
		errPrefixer = &prefixer{template: template, pid: pid, sep: cfg.Delimiter}
		if cfg.Stream {
			errPrefixer.streamMark = "STDERR| "
		}
	}

	// mu guards writes so a stdout pump and stderr pump never interleave partial writes when the caller has merged fd 1
	// and fd 2. If stdout and stderr are distinct destinations the mutex is uncontended.
	var mu sync.Mutex

	var wg sync.WaitGroup
	var stdoutErr, stderrErr error
	wg.Go(func() {
		stdoutErr = pumpLines(stdoutR, outPrefixer, stdout, &mu)
	})
	if cfg.AlsoStderr {
		wg.Go(func() {
			stderrErr = pumpLines(stderrR, errPrefixer, stderr, &mu)
		})
	}
	wg.Wait()

	waitErr := cmd.Wait()
	exitCode := cli.ExitCodeFor(waitErr)
	if stdoutErr != nil {
		fmt.Fprintf(stderr, "prefix: stdout pump: %v\n", stdoutErr)
	}
	if stderrErr != nil {
		fmt.Fprintf(stderr, "prefix: stderr pump: %v\n", stderrErr)
	}

	return exitCode, nil
}
