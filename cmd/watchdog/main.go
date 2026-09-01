// watchdog runs a command under an idle-timer and kills it if it stops producing output. See help.txt.
package main

import (
	_ "embed"
	"fmt"
	"io"
	"os"

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
	fs := pflag.NewFlagSet("watchdog", pflag.ContinueOnError)
	fs.SetInterspersed(false)
	fs.SetOutput(io.Discard)

	var (
		cfg           Config
		showHelp      bool
		showVersion   bool
		idleStr       string
		graceStr      string
		signalStrs    []string
		includeStderr bool
		verbose       bool
	)
	fs.StringVarP(&idleStr, "idle", "i", "1m", "")
	fs.BoolVarP(&includeStderr, "stderr", "e", false, "")
	// StringSliceVarP accumulates across occurrences AND splits on commas within each occurrence, so users can write
	// -s TERM,INT or -s TERM -s INT or -s TERM,INT -s KILL -- all end up in one ordered list.
	fs.StringSliceVarP(&signalStrs, "signal", "s", nil, "")
	fs.StringVarP(&graceStr, "grace", "g", "10s", "")
	fs.BoolVarP(&verbose, "verbose", "v", false, "")
	fs.BoolVarP(&showHelp, "help", "h", false, "")
	fs.BoolVar(&showVersion, "version", false, "")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "watchdog: %v\n", err)
		return cli.ExitWrapperError
	}

	if showHelp {
		_, _ = stdout.Write(helpText)
		return cli.ExitOK
	}
	if showVersion {
		if err := cli.PrintVersion(stdout); err != nil {
			fmt.Fprintf(stderr, "watchdog: %v\n", err)
			return cli.ExitWrapperError
		}
		return cli.ExitOK
	}

	cfg.Command = fs.Args()
	if len(cfg.Command) == 0 {
		_, _ = stdout.Write(helpText)
		return cli.ExitOK
	}

	idle, err := duration.Parse(idleStr)
	if err != nil {
		fmt.Fprintf(stderr, "watchdog: --idle: %v\n", err)
		return cli.ExitWrapperError
	}
	grace, err := duration.Parse(graceStr)
	if err != nil {
		fmt.Fprintf(stderr, "watchdog: --grace: %v\n", err)
		return cli.ExitWrapperError
	}
	sigs, err := cli.ParseSignals(signalStrs)
	if err != nil {
		fmt.Fprintf(stderr, "watchdog: %v\n", err)
		return cli.ExitWrapperError
	}
	cfg.Idle = idle
	cfg.Grace = grace
	cfg.Signals = sigs
	cfg.IncludeStderr = includeStderr
	cfg.Verbose = verbose

	code, err := doWatchdog(&cfg, stdin, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "watchdog: %v\n", err)
	}
	return code
}
