// Execute a command and re-runs it on failure until it succeeds or a limit is reached
package main

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/pflag"

	"github.com/andrew-grechkin/exec-utils/internal/cli"
	"github.com/andrew-grechkin/exec-utils/internal/duration"
)

//go:embed help.txt
var helpText []byte

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := pflag.NewFlagSet("retry", pflag.ContinueOnError)
	fs.SetInterspersed(false)
	fs.SetOutput(io.Discard)

	var (
		showHelp    bool
		showVersion bool
		maxAttempts int
		delayStr    string
		backoffStr  string
		maxDelayStr string
		jitter      float64
		timeoutStr  string
		totalStr    string
		codeStrs    []string
		signalStrs  []string
		verbose     bool
	)

	maxDefault, _ := parseIntEnv("RETRY_MAX", 0)
	delayDefault := cli.EnvDefault("RETRY_DELAY", "2s")
	backoffDefault := cli.EnvDefault("RETRY_BACKOFF", "fixed")

	fs.IntVarP(&maxAttempts, "max", "n", maxDefault, "")
	fs.StringVarP(&delayStr, "delay", "d", delayDefault, "")
	fs.StringVarP(&backoffStr, "backoff", "b", backoffDefault, "")
	fs.StringVarP(&maxDelayStr, "max-delay", "m", "30s", "")
	fs.Float64VarP(&jitter, "jitter", "j", 0, "")
	fs.StringVarP(&timeoutStr, "timeout", "t", "", "")
	fs.StringVarP(&totalStr, "total-timeout", "T", "", "")
	// StringSliceVarP accumulates across occurrences AND splits on commas within each occurrence, so users can write
	// -c 0,2 or -c 0 -c 2 or -c 0,1 -c 2 -- all end up in one slice.
	fs.StringSliceVarP(&codeStrs, "code", "c", nil, "")
	// Same accumulation semantics as watchdog's -s: "-s TERM,INT -s KILL" all end up in one ordered list. Default is
	// TERM,KILL; KILL is auto-appended when the user's list omits it (it is the only unignorable signal).
	fs.StringSliceVarP(&signalStrs, "signal", "s", nil, "")
	fs.BoolVarP(&verbose, "verbose", "v", false, "")
	fs.BoolVarP(&showHelp, "help", "h", false, "")
	fs.BoolVar(&showVersion, "version", false, "")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "retry: %v\n", err)
		return cli.ExitWrapperError
	}

	if showHelp {
		_, _ = stdout.Write(helpText)
		return cli.ExitOK
	}

	if showVersion {
		if err := cli.PrintVersion(stdout); err != nil {
			fmt.Fprintf(stderr, "retry: %v\n", err)
			return cli.ExitWrapperError
		}
		return cli.ExitOK
	}

	rest := fs.Args()
	if len(rest) == 0 {
		_, _ = stdout.Write(helpText)
		return cli.ExitOK
	}

	cfg, err := buildConfig(rest, maxAttempts, delayStr, backoffStr, maxDelayStr, jitter, timeoutStr, totalStr, codeStrs, signalStrs, verbose)
	if err != nil {
		fmt.Fprintf(stderr, "retry: %v\n", err)
		return cli.ExitWrapperError
	}

	return doRetry(cfg, stdin, stdout, stderr)
}

// Validate and resolve all string-form flags to their typed equivalents, so doRetry receives a clean Config.
func buildConfig(rest []string, maxAttempts int, delayStr, backoffStr, maxDelayStr string, jitter float64, timeoutStr, totalStr string, codeStrs, signalStrs []string, verbose bool) (*Config, error) {
	if maxAttempts < 0 {
		return nil, fmt.Errorf("--max must be >= 0")
	}

	if jitter < 0 || jitter > 1 {
		return nil, fmt.Errorf("--jitter must be in [0, 1], got %v", jitter)
	}

	delay, err := duration.Parse(delayStr)
	if err != nil {
		return nil, fmt.Errorf("--delay: %w", err)
	}

	maxDelay, err := duration.Parse(maxDelayStr)
	if err != nil {
		return nil, fmt.Errorf("--max-delay: %w", err)
	}

	spec, err := ParseBackoff(backoffStr)
	if err != nil {
		return nil, err
	}

	codes, err := ParseCodes(codeStrs)
	if err != nil {
		return nil, err
	}

	sigs, err := cli.ParseSignals(signalStrs)
	if err != nil {
		return nil, err
	}

	var timeout time.Duration
	if timeoutStr != "" {
		timeout, err = duration.Parse(timeoutStr)
		if err != nil {
			return nil, fmt.Errorf("--timeout: %w", err)
		}
	}
	var total time.Duration
	if totalStr != "" {
		total, err = duration.Parse(totalStr)
		if err != nil {
			return nil, fmt.Errorf("--total-timeout: %w", err)
		}
	}

	return &Config{
		Command:      rest,
		Max:          maxAttempts,
		Delay:        delay,
		Backoff:      spec,
		MaxDelay:     maxDelay,
		Jitter:       jitter,
		Timeout:      timeout,
		TotalTimeout: total,
		RetryCodes:   codes,
		Signals:      sigs,
		Verbose:      verbose,
	}, nil
}

func parseIntEnv(key string, fallback int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback, nil
	}

	var n int
	_, err := fmt.Sscanf(v, "%d", &n)
	if err != nil {
		return fallback, err
	}

	return n, nil
}
