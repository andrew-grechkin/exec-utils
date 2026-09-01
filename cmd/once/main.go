// Package main serialises a command across all callers sharing the same label. Ergonomic wrapper around flock(2).
// See help.txt.
package main

import (
	_ "embed"
	"fmt"
	"io"
	"os"

	"github.com/spf13/pflag"

	"github.com/andrew-grechkin/exec-utils/internal/cli"
)

//go:embed help.txt
var helpText []byte

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := pflag.NewFlagSet("once", pflag.ContinueOnError)
	fs.SetInterspersed(false)
	fs.SetOutput(io.Discard)

	var (
		cfg              Config
		showHelp         bool
		showVersion      bool
		wait, skip, fail bool
	)
	fs.StringVarP(&cfg.Label, "label", "l", "", "")
	fs.StringVarP(&cfg.DirOverride, "dir", "d", "", "")
	fs.BoolVarP(&wait, "wait", "w", false, "")
	fs.BoolVarP(&skip, "skip", "s", false, "")
	fs.BoolVarP(&fail, "fail", "f", false, "")
	fs.BoolVarP(&cfg.Verbose, "verbose", "v", false, "")
	fs.BoolVarP(&showHelp, "help", "h", false, "")
	fs.BoolVar(&showVersion, "version", false, "")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "once: %v\n", err)
		return cli.ExitWrapperError
	}

	if showHelp {
		_, _ = stdout.Write(helpText)
		return cli.ExitOK
	}
	if showVersion {
		if err := cli.PrintVersion(stdout); err != nil {
			fmt.Fprintf(stderr, "once: %v\n", err)
			return cli.ExitWrapperError
		}
		return cli.ExitOK
	}

	cfg.Command = fs.Args()
	if len(cfg.Command) == 0 {
		_, _ = stdout.Write(helpText)
		return cli.ExitOK
	}

	mode, err := cli.ResolveLockMode(wait, skip, fail)
	if err != nil {
		fmt.Fprintf(stderr, "once: %v\n", err)
		return cli.ExitWrapperError
	}
	cfg.Mode = mode

	if err := cfg.resolve(); err != nil {
		fmt.Fprintf(stderr, "once: %v\n", err)
		return cli.ExitWrapperError
	}
	return doOnce(&cfg, stdin, stdout, stderr)
}
