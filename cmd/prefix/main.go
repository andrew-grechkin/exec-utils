// Prepend a per-line label to a wrapped command's STDOUT (and optionally stderr)
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
	fs := pflag.NewFlagSet("prefix", pflag.ContinueOnError)
	fs.SetInterspersed(false)
	fs.SetOutput(io.Discard)

	var (
		cfg         Config
		showHelp    bool
		showVersion bool
	)
	fs.StringVarP(&cfg.Prefix, "prefix", "p", "", "")
	fs.BoolVarP(&cfg.AlsoStderr, "also-stderr", "e", false, "")
	fs.BoolVarP(&cfg.Stream, "stream", "s", false, "")
	fs.StringVarP(&cfg.Delimiter, "delimiter", "d", "\t", "")
	fs.BoolVarP(&showHelp, "help", "h", false, "")
	fs.BoolVar(&showVersion, "version", false, "")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(stderr, "prefix: %v\n", err)
		return cli.ExitWrapperError
	}

	if showHelp {
		_, _ = stdout.Write(helpText)
		return cli.ExitOK
	}

	if showVersion {
		if err := cli.PrintVersion(stdout); err != nil {
			fmt.Fprintf(stderr, "prefix: %v\n", err)
			return cli.ExitWrapperError
		}
		return cli.ExitOK
	}

	cfg.Command = fs.Args()
	if len(cfg.Command) == 0 {
		_, _ = stdout.Write(helpText)
		return cli.ExitOK
	}

	code, err := doPrefix(&cfg, stdin, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "prefix: %v\n", err)
	}

	return code
}
