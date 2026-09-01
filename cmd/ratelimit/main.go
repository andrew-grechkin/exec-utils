// ratelimit bounds how often a command may run. See help.txt for the full flag reference.
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
	fs := pflag.NewFlagSet("ratelimit", pflag.ContinueOnError)
	fs.SetInterspersed(false)
	fs.SetOutput(io.Discard)

	var (
		cfg         Config
		showHelp    bool
		showVersion bool
		wait        bool
		skip        bool
		fail        bool
	)
	fs.StringVarP(&cfg.IntervalRaw, "interval", "i", "", "")
	fs.BoolVarP(&wait, "wait", "w", false, "")
	fs.BoolVarP(&skip, "skip", "s", false, "")
	fs.BoolVarP(&fail, "fail", "f", false, "")
	fs.StringVarP(&cfg.Label, "label", "l", "", "")
	fs.StringVarP(&cfg.DirOverride, "dir", "d", "", "")
	fs.BoolVarP(&cfg.Verbose, "verbose", "v", false, "")
	fs.BoolVarP(&showHelp, "help", "h", false, "")
	fs.BoolVar(&showVersion, "version", false, "")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "ratelimit: %v\n", err)
		return cli.ExitWrapperError
	}

	if showHelp {
		_, _ = stdout.Write(helpText)
		return cli.ExitOK
	}
	if showVersion {
		if err := cli.PrintVersion(stdout); err != nil {
			fmt.Fprintf(stderr, "ratelimit: %v\n", err)
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
		fmt.Fprintf(stderr, "ratelimit: %v\n", err)
		return cli.ExitWrapperError
	}
	cfg.Mode = mode

	if err := cfg.resolve(); err != nil {
		fmt.Fprintf(stderr, "ratelimit: %v\n", err)
		return cli.ExitWrapperError
	}
	return doRatelimit(&cfg, stdin, stdout, stderr)
}
