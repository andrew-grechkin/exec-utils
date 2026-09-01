// Execute a command and cache its stdout
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

func run(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := pflag.NewFlagSet("memoize", pflag.ContinueOnError)
	fs.SetInterspersed(false)
	fs.SetOutput(io.Discard)

	var (
		cfg         Config
		showHelp    bool
		showVersion bool
	)

	fs.BoolVarP(&cfg.Buffer, "buffer", "b", false, "")
	fs.BoolVarP(&cfg.Clear, "clear", "C", false, "")
	fs.BoolVarP(&cfg.CacheOnly, "cache-only", "c", false, "")
	fs.BoolVarP(&cfg.Empty, "empty", "e", false, "")
	fs.BoolVarP(&cfg.Slide, "slide", "s", false, "")
	fs.BoolVarP(&cfg.Verbose, "verbose", "v", false, "")
	fs.BoolVarP(&cfg.Workdir, "workdir", "w", false, "")
	fs.BoolVarP(&showHelp, "help", "h", false, "")
	fs.BoolVar(&showVersion, "version", false, "")
	fs.StringVarP(&cfg.File, "file", "f", "", "")
	fs.StringVarP(&cfg.Label, "label", "l", "", "")
	fs.StringVarP(&cfg.TTLRaw, "ttl", "t", "", "")

	if err := fs.Parse(argv); err != nil {
		fmt.Fprintf(stderr, "memoize: %v\n", err)
		return cli.ExitWrapperError
	}
	cfg.Command = fs.Args()

	if showHelp {
		_, _ = stdout.Write(helpText)
		return cli.ExitOK
	}

	if showVersion {
		if err := cli.PrintVersion(stdout); err != nil {
			fmt.Fprintf(stderr, "memoize: %v\n", err)
			return cli.ExitWrapperError
		}
		return cli.ExitOK
	}

	if len(cfg.Command) == 0 && !cfg.Clear {
		_, _ = stdout.Write(helpText)
		return cli.ExitOK
	}

	if err := cfg.resolve(); err != nil {
		fmt.Fprintf(stderr, "memoize: %v\n", err)
		return cli.ExitWrapperError
	}

	if cfg.Clear {
		return clearCache(&cfg, stderr)
	}

	return execute(&cfg, stdin, stdout, stderr)
}
