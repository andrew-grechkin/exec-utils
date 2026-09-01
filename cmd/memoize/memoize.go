package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/andrew-grechkin/exec-utils/internal/cli"
)

const (
	defaultTTL      = 24 * time.Hour
	defaultDirName  = "memoize"
	dataSuffix      = ".stdout"
	scriptSuffix    = ".sh"
	temporarySuffix = ".memoize"
)

// Clear the cache (or with -l, just that label's subdir).
// Command args are ignored per the bash script's behavior (with a warning)
func clearCache(cfg *Config, stderr io.Writer) int {
	if len(cfg.Command) > 0 {
		fmt.Fprintln(stderr, "warning: command ignored during clear cache operation")
	}

	target := cfg.Dir
	if cfg.Label != "" {
		target = cfg.cacheDir()
	}

	if err := os.RemoveAll(target); err != nil {
		fmt.Fprintf(stderr, "memoize: clear %s: %v\n", target, err)
		return cli.ExitWrapperError
	}

	return cli.ExitOK
}

// Serve from cache when fresh, else run the wrapped command and cache its stdout. Return the exit code to propagate
// to the caller.
func execute(cfg *Config, stdin io.Reader, stdout, stderr io.Writer) int {
	cacheFile := cfg.cacheFile()

	if cfg.isFresh(cacheFile) {
		return serveFromCache(cfg, cacheFile, stdout, stderr)
	}

	if cfg.CacheOnly {
		fmt.Fprintln(stderr, "memoize: cache is not found")
		return cli.ExitWrapperError
	}

	return executeAndCache(cfg, cacheFile, stdin, stdout, stderr)
}

// Copy the cached bytes to STDOUT. With -s, bump the cache
// mtime so the sliding TTL restart from this read
func serveFromCache(cfg *Config, path string, stdout, stderr io.Writer) int {
	info, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(stderr, "memoize: %v\n", err)
		return cli.ExitWrapperError
	}
	cfg.logf(stderr, "fresh cache is found with size: %d", info.Size())

	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "memoize: %v\n", err)
		return cli.ExitWrapperError
	}
	defer f.Close()

	if cfg.Slide {
		now := time.Now()
		_ = os.Chtimes(path, now, now)
	}

	if _, err := io.Copy(stdout, f); err != nil {
		fmt.Fprintf(stderr, "memoize: %v\n", err)
		return cli.ExitWrapperError
	}

	return cli.ExitOK
}

// Run the wrapped command. Default mode tees stdout to both
// the caller's stdout and a temp file. --buffer routes stdout to the temp
// only, then emits it after success. In either case the temp is atomically
// renamed to the cache path if the command exits 0.
func executeAndCache(cfg *Config, cacheFile string, stdin io.Reader, stdout, stderr io.Writer) int {
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
		fmt.Fprintf(stderr, "memoize: mkdir: %v\n", err)
		return cli.ExitWrapperError
	}

	tmpPath := cacheFile + temporarySuffix
	tmp, err := os.Create(tmpPath)
	if err != nil {
		fmt.Fprintf(stderr, "memoize: create temp: %v\n", err)
		return cli.ExitWrapperError
	}
	defer os.Remove(tmpPath)

	var cmdStdout io.Writer = tmp
	if !cfg.Buffer {
		cmdStdout = io.MultiWriter(stdout, tmp)
	}

	cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
	cmd.Stdin = stdin
	cmd.Stdout = cmdStdout
	cmd.Stderr = stderr
	cmd.Env = filteredEnv(os.Environ())
	cmd.SysProcAttr = cli.NewChildProcAttr()

	runErr := cli.RunChild(cmd)
	_ = tmp.Close()

	exitCode := cli.ExitCodeFor(runErr)
	if exitCode != cli.ExitOK {
		if cli.IsWrapperError(runErr) {
			fmt.Fprintf(stderr, "memoize: %v\n", runErr)
		}
		return exitCode
	}

	if err := os.Rename(tmpPath, cacheFile); err != nil {
		fmt.Fprintf(stderr, "memoize: commit cache: %v\n", err)
		return cli.ExitWrapperError
	}
	if sf := cfg.scriptFile(); sf != "" {
		if err := writeScriptFile(sf, cfg.Pwd, cfg.Command); err != nil {
			cfg.logf(stderr, "note: could not write script file %s: %v", sf, err)
		}
	}

	if cfg.Buffer {
		return emitFile(cacheFile, stdout, stderr)
	}

	return cli.ExitOK
}

func emitFile(path string, stdout, stderr io.Writer) int {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "memoize: read cache: %v\n", err)
		return cli.ExitWrapperError
	}
	defer f.Close()

	if _, err := io.Copy(stdout, f); err != nil {
		fmt.Fprintf(stderr, "memoize: emit cache: %v\n", err)
		return cli.ExitWrapperError
	}

	return cli.ExitOK
}

func writeScriptFile(path, pwd string, cmd []string) error {
	var b strings.Builder
	b.WriteString("cd ")
	b.WriteString(shquote(pwd))
	b.WriteString(" && ")
	for i, a := range cmd {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(shquote(a))
	}
	b.WriteByte('\n')

	tmp := path + temporarySuffix
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}

	return os.Rename(tmp, path)
}

func filteredEnv(env []string) []string {
	out := env[:0:0]
	for _, e := range env {
		if strings.HasPrefix(e, "MEMOIZE_") {
			continue
		}
		out = append(out, e)
	}
	return out
}

func shquote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexFunc(s, needsShellQuoting) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func needsShellQuoting(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z',
		r >= 'A' && r <= 'Z',
		r >= '0' && r <= '9':
		return false
	}

	switch r {
	case '/', '.', '_', '-', '=', ':', ',', '+', '%', '@':
		return false
	}

	return true
}
