# exec-utils

[![Go Reference](https://pkg.go.dev/badge/github.com/andrew-grechkin/exec-utils.svg)](https://pkg.go.dev/github.com/andrew-grechkin/exec-utils)

A set of small command-wrapper utilities that fill the gaps left by:

- [coreutils](https://www.gnu.org/software/coreutils/)
- [util-linux](https://github.com/util-linux/util-linux)
- [moreutils](https://joeyh.name/code/moreutils/)

Those three ship the atoms of a Unix shell (`flock`, `timeout`, `xargs -P`, `stdbuf`, `ts`, `chronic`) but stop short of
a handful of ergonomic wrappers that show up in every non-trivial script: output-serialized parallel workers, retry
loops with policy, output caching, per-line labeling, side-effect rate limiting. Every tool here follows the same
grammar, exit-code convention, and composition rules so they stack against each other and against the classics.

Run `<util> --help` for the full flag reference; this README stays short and points at each tool's embedded help.

## UTILITIES

- [`spew`](cmd/spew/help.txt) - Serialize STDOUT across parallel workers. Buffers each child's STDOUT and emits
  atomically under `flock` so writers to the same destination never interleave
- [`memoize`](cmd/memoize/help.txt) - Execute a command and cache its STDOUT under a TTL. Subsequent invocations with
  the same arguments return the cached output without re-running the command
- [`retry`](cmd/retry/help.txt) - Re-run a failing command until it succeeds or a limit is reached. Fixed, linear,
  exponential, or Fibonacci backoff; per-attempt and total-wall-clock timeouts; STDIN buffered and replayed to each
  attempt; `-c 0` inverts the loop into flake detection (retry-while-succeeding)
- [`prefix`](cmd/prefix/help.txt) - Prepend a per-line label to a command's STDOUT (optionally STDERR). Defaults to a
  timestamp; templates support timestamp, PID, sequence-counter tokens; stream-distinguishing markers; streams
  line-by-line without buffering
- [`ratelimit`](cmd/ratelimit/help.txt) - Bound how often a command may run. State kept as an mtime-only file per label;
  three modes for the cooldown case: `-w` wait (default), `-s` skip, `-f` fail. Complements `memoize`: memoize caches
  output, ratelimit gates side-effect commands
- [`watchdog`](cmd/watchdog/help.txt) - Kill a command if it stalls (no output for a period). Complements `timeout(1)`:
  timeout caps total wall-clock, watchdog fires on lack of progress. Signal escalation (`-s TERM,INT -s KILL` etc.)
  with `--grace` between each; whole-process-group signaling; `-e` counts STDERR activity too
- [`once`](cmd/once/help.txt) - Serialize a command across all callers sharing the same label. Ergonomic wrapper around
  `flock(2)`: `-w` wait (default), `-s` skip, `-f` fail. Distinct from `spew`: `once` gates COMMAND EXECUTION, `spew`
  gates STDOUT WRITES

## DESIGN

Everything in this repo obeys three rules so the tools compose without surprises:

1. **Same grammar.** `<tool> [OPTIONS] [--] COMMAND [COMMAND ARGS...]`. GNU-style flags (long+short, `-abc` combining, `--`
   terminator). Flag parsing stops at the first positional, so a wrapped command's flags pass through untouched
   (`spew jq -S` runs `jq -S`)
2. **Uniform exit codes** (coreutils [`timeout(1)`](https://www.gnu.org/software/coreutils/manual/html_node/timeout-invocation.html)
   convention): `<child exit>` propagated, `124` wrapper timeout, `125` wrapper error, `126` cannot invoke, `127` not found.
   `0-123` stays reserved for the wrapped command
3. **Composable with itself and each other.** Any tool can wrap any other. Four-way stack that actually runs:

   ```bash
   spew memoize -t 5m retry -n 3 prefix -- curl -fsSL https://httpbin.org/uuid
   ```

   Every tool buffers or streams appropriately so nesting stays parallel where you expect and serial where you expect

## INSTALLATION

Install one utility:

```bash
go install github.com/andrew-grechkin/exec-utils/cmd/spew@latest
go install github.com/andrew-grechkin/exec-utils/cmd/memoize@latest
go install github.com/andrew-grechkin/exec-utils/cmd/retry@latest
go install github.com/andrew-grechkin/exec-utils/cmd/prefix@latest
go install github.com/andrew-grechkin/exec-utils/cmd/ratelimit@latest
go install github.com/andrew-grechkin/exec-utils/cmd/watchdog@latest
go install github.com/andrew-grechkin/exec-utils/cmd/once@latest
```

Install every utility in this repo:

```bash
go install github.com/andrew-grechkin/exec-utils/cmd/...@latest
```

### Using [`mise`](https://mise.jdx.dev)

```bash
mise use go:github.com/andrew-grechkin/exec-utils/cmd/spew@latest
mise use go:github.com/andrew-grechkin/exec-utils/cmd/memoize@latest
mise use go:github.com/andrew-grechkin/exec-utils/cmd/retry@latest
mise use go:github.com/andrew-grechkin/exec-utils/cmd/prefix@latest
mise use go:github.com/andrew-grechkin/exec-utils/cmd/ratelimit@latest
mise use go:github.com/andrew-grechkin/exec-utils/cmd/watchdog@latest
mise use go:github.com/andrew-grechkin/exec-utils/cmd/once@latest
```

## RECIPES

Single-tool examples live in each utility's `--help`. Below are copy-paste-runnable patterns that showcase what happens
when the tools stack. Everything uses standard Linux tools plus `httpbin.org` where a network endpoint is needed.

### Parallel HTTP fetch with retry, cache, and clean output

8 URLs to fetch. Each request may be flaky (retry with 2x exponential backoff + jitter). Successful responses cached for
5 minutes. Output atomic per worker, no interleave. Second run of the same block is instant from cache.

```bash
seq 1 8 | xargs -P0 -rn1 bash -c '
    spew memoize -t 5m -- \
        retry -n 3 -b 2 -j 0.2 -- \
            curl -fsSL "https://httpbin.org/anything?w=$0"
'
```

Inside-out: `curl` fetches → `retry` retries transient failures → `memoize` caches the success for 5 min → `spew` emits
each response as one atomic block.

### Flake-hunt a command until it fails

Loop while the command keeps exiting 0, stop on the first non-zero exit and return that code. Timestamps every line so
you can see whether a run was slow. Synthetic 1-in-5 failure rate for demo:

```bash
retry -n 100 -c 0 -d 100ms -- prefix -- bash -c '
    n=$((RANDOM % 5))
    echo "roll: $n"
    test $n -ne 0
'
```

`-c 0` inverts retry's loop: retry _while_ exit=0. Real use: swap the `bash -c` block for `go test -run FlakyThing` to
reproduce a real flake.

### At-most-once-per-hour side-effect with retry inside

Wrap a POST that must fire at most once per hour per fingerprint, but survive transient failures with 3 retries inside
the hour window. Add this line to cron and it's minute-triggered but hour-throttled:

```bash
ratelimit -s -i 1h -l demo-alert -- \
    retry -n 3 -d 500ms -b 2 -- \
        curl -fsSL -o /dev/null -w '%{http_code}\n' -XPOST -d '{"event":"disk-full"}' https://httpbin.org/post
```

`ratelimit -s` silently skips if a call landed in the last hour, so a `* * * * *` cron entry is cheap.

### Parallel workers with attributable, timestamped, atomic output

4 workers each doing an HTTP call. Every line prefixed with `[w<id>] <ISO timestamp>`; each worker's block arrives
contiguous (spew wraps prefix wraps curl):

```bash
seq 1 4 | xargs -P0 -rn1 bash -c '
    spew prefix -p "[w$0] %T" -- \
        curl -sSL "https://httpbin.org/anything?w=$0"
'
```

### Prime a cache, consume it from many callers

The producer command populates the cache once; consumers using the same args get instant results and cannot fall back to
recomputing if the cache is stale (`-c` mode fails loud instead):

```bash
# Prime (runs the find):
memoize -t 24h -l headers -- find /usr/include -maxdepth 3 -name '*.h' | wc -l

# Later consumer with identical args: served from cache, find NEVER runs:
memoize -c -t 24h -l headers -- find /usr/include -maxdepth 3 -name '*.h' | wc -l
```

If the cache is missing or stale, the `-c` consumer exits 125 with `memoize: cache is not found` instead of silently
recomputing.

### Poll a service until it's up, capped by wall-clock

Attempt every 500ms for up to 3 seconds. On success exit 0; on wall-clock exhaustion exit 124 (ExitTimeout). Demo target
is an unopened local port so the timeout path is easy to observe:

```bash
retry -T 3s -d 500ms -- curl -fsSL -m 1 http://localhost:9999
```

Real use: replace `localhost:9999` with a real health endpoint.

### One-line fix for interleaved parallel output

Four workers, each producing lines. Without spew the output can interleave; with spew each worker's lines stay
contiguous:

```bash
# Without spew (may interleave under load):
seq 1 4 | xargs -P0 -rn1 bash -c 'seq 1 50 | sed "s/^/w$0-/"'

# With spew (blocks stay atomic):
seq 1 4 | xargs -P0 -rn1 bash -c 'seq 1 50 | spew sed "s/^/w$0-/"'
```

Compared to per-worker temp files + concatenation, or hand-rolled `flock` around a subshell, or piping to `ts | uniq`,
spew is the direct primitive.

### Rate-limited fan-out (shared cooldown across parallel workers)

10 workers, all sharing a single label. `ratelimit -w` queues them so consecutive calls stay at least 200ms apart even
though `xargs -P0` starts them simultaneously. Useful for a courteous batch against a rate-limited API:

```bash
seq 1 10 | xargs -P0 -rn1 bash -c '
    ratelimit -w -i 200ms -l api-batch -- \
        curl -fsSL "https://httpbin.org/uuid"
'
```

Peer workers block on the same flock; each releases after touching mtime, so the effective throttle is exactly one call
per 200ms across the fleet.

### Per-attempt timeout vs total-wall-clock timeout

`retry -t DUR` kills an individual attempt if it exceeds DUR (like a stall detector inside one retry cycle);
`retry -T DUR` caps the whole loop. They compose:

```bash
# httpbin sleeps 2s per call. -t 500ms kills each attempt fast; -n 3 stops after 3 kills.
retry -n 3 -t 500ms -d 100ms -- curl -fsSL -m 5 https://httpbin.org/delay/2
```

Wall time is ~1.7s (3 attempts x 500ms + 2 x 100ms sleep) not 6s. When `-t` fires, retry walks the `-s` escalation on
the child's process group (default `TERM,KILL`, with `min(-t/2, 5s)` between steps) so a well-behaved child can clean up
before SIGKILL. Signals share names and semantics with `watchdog -s`; a bare `retry -t DUR -- cmd` needs no signal flag.
Final exit follows the shell convention `128 + signum`: 143 when the child cleaned up on SIGTERM, 137 when it ignored
TERM and had to be killed.

### Stall-safe polling with escalation

Poll a hung endpoint but never wait longer than 500ms per attempt for output. TERM first (child gets to clean up), KILL
after 200ms grace if it ignored TERM. Whole process group is signaled so shell wrappers die cleanly too.

```bash
watchdog -v -i 1s -g 2s -- bash -c 'trap "" TERM; sleep 5'
```

`bash -c 'trap "" TERM'` deliberately ignores SIGTERM so you can watch the escalation reach SIGKILL in the verbose log.
Wall time is ~700ms (500ms idle + 200ms grace) not 5s.

### Merged build-log streams with STDOUT/STDERR distinction

Three parallel "compilers" each emitting STDOUT and STDERR. `spew prefix -s` labels each stream (`STDOUT|` / `STDERR|`)
AND keeps every worker's block atomic:

```bash
seq 1 3 | xargs -P0 -rn1 bash -c '
    spew prefix -p "[b$0]" -s -- bash -c "echo compile ok; echo warn: unused var >&2"
' 2>&1 | sort
```

Sort by the stream tag to separate signal (STDOUT, real output) from noise (STDERR, warnings) after the fact, without
needing to redirect at the source.

### Cron entry that never overlaps itself

A minute-triggered cron job that must not overlap: `once -s` returns exit 0 immediately if a prior run is still
holding the lock, so cron doesn't stack invocations while a slow run finishes.

```bash
* * * * * once -s -l backup -- rsync -a src/ dst/
```

Compose with `ratelimit` when the job should also honor a minimum interval independent of overlap:

```bash
once -s -l job -- ratelimit -s -i 6h -l job -- ./expensive-job.sh
```

`once` gates execution (a second caller returns 0 if a first is running); `ratelimit` gates cadence (a second caller
returns 0 if the last run was within the interval). Both together: at most one running, at most one per 6h.

## CONVENTIONS

Invariants every utility in this repo shares.
New utilities should follow them so the fleet stays legible.

- **Verbose.** `-v` / `--verbose` logs to STDERR, one line per event, prefixed with `<util>: `
- **Help.** `//go:embed help.txt` at build; `-h` / `--help` and no-args both write it to STDOUT
- **Version.** `--version` writes `debug.ReadBuildInfo().Main` as pretty JSON via `cli.PrintVersion` (tip:
  `<util> --version | jq -r .Version`)
- **State dirs.** `cli.ResolveStateDir(explicit, envKey, defaultName)`. Precedence: `--dir` flag value, then
  `$<UTIL>_DIR`, then `$XDG_RUNTIME_DIR/<util>`, then `/tmp/<util>`
- **Labels map to keyfiles.** `filepath.Join(dir, cli.SHA256Hex(label))`. Anonymous invocations key on
  `pwd + "\n" + argv` so two shells in different directories never collide
- **Child process attrs and signals.** Every wrapper spawns its child with `cli.NewChildProcAttr()` (which sets
  `Setpgid: true`) AND either uses `cli.RunChild(cmd)` or installs `cli.StartSignalForwarder(cmd.Process.Pid, ...)`
  between `cmd.Start()` and `cmd.Wait()`. This pair is not optional; using one without the other is a bug. See
  DESIGN DECISIONS below for the full rationale
- **Signal parsing.** `-s` / `--signal` uses `cli.ParseSignals`; default is `TERM,KILL`; `KILL` is auto-appended if
  the user's list omits it. Names case-insensitive, `SIG` prefix optional
- **Error classification.** `cli.IsWrapperError(err)` decides whether the wrapper prints its own diagnostic.
  Wrapper-side failures (couldn't exec, permission denied) do; child outcomes (own exit code, signal death) don't
  (they're already visible via the exit code and the child's own STDERR)
- **Integration fixtures.** Directory-per-fixture under `test/fixtures/<util>/<case>/`. Recognised files: `args`,
  `stdin`, `run`, `expected-stdout`, `expected-stdout-contains`, `expected-stderr-contains`, `expected-exit`,
  `description`. All optional. See `CONTRIBUTING.md`

## DESIGN DECISIONS

Rationale for the non-obvious choices. When extending a utility or adding a new one, understand why these were picked
before deviating.

- **`setpgid` on the child, wrapper outside, wrapper forwards terminal signals.** This is the load-bearing design
    choice for how every wrapper signals its subtree.

    **Goal.** When the wrapper's own timer fires (retry per-attempt timeout, watchdog stall), I want to kill the
    wrapped command AND anything it spawned in the background: a `bash -c 'helper &; wait'`, a `coproc`, a
    background pipeline. Without a pgroup, `kill(childPid, sig)` only reaches the direct child; any grandchild in
    the background is reparented to init and keeps running until it exits on its own. For example,
    `retry -t 100ms -- bash -c 'coproc { sleep 30; }; wait'` would kill bash but leave the coproc and its `sleep 30`
    running orphaned for the rest of the 30 seconds. The clean POSIX mechanism for reaching a whole subtree is
    `kill(-pgid, sig)` on a process group.

    **Why the child leads its own pgroup, not the wrapper.** `cli.NewChildProcAttr()` sets `Setpgid: true` with no
    Pgid, which creates a fresh process group where `child.pgid == child.pid`; everything the child forks inherits
    that pgid. The wrapper is deliberately NOT a member of that pgroup. That is what makes
    `kill(-childpgid, SIGKILL)` safe: the wrapper survives to propagate the child's exit code. The alternative
    configuration ("wrapper is pgroup leader, children join") was rejected because SIGKILL is uncatchable, so a
    wrapper inside the pgroup it's signalling would die with the children.

    **The forwarder pays the cost of that isolation.** Because the child is in a separate pgroup, terminal signals
    delivered to the wrapper's foreground pgroup (SIGINT from Ctrl-C, SIGHUP on ssh disconnect, SIGTERM from an
    external kill) no longer reach the child. `cli.StartSignalForwarder` installs a `signal.Notify` handler for
    SIGINT / SIGTERM / SIGHUP that relays each signal into the child's pgroup via `kill(-childpgid, sig)`. Retry
    additionally passes an `*atomic.Bool` "interrupted" flag so its loop stops iterating instead of spawning a
    fresh attempt into a shutdown.

    **Composition.** `spew -- retry -- bash -c 'cmd &'`: spew's child (retry) is in a new pgroup led by retry;
    retry's child (bash) is in another new pgroup led by bash; bash's `cmd &` inherits bash's pgroup. Every wrapper
    aggregates exactly its own subtree; wrappers do not accidentally signal each other. Terminal Ctrl-C reaches
    spew via foreground pgroup, spew forwards to retry, retry forwards to bash, bash's pgroup dies together
- **`ratelimit` and `once` share plumbing but not semantics.** Both use `cli.LockMode` (wait / skip / fail), both use
    `cli.ResolveStateDir`, both hash labels the same way. They differ in the question they ask: `ratelimit` gates on the
    state file's mtime relative to `-i` (cadence); `once` gates on live `flock` contention (concurrent overlap).
    Composable: `once -s -- ratelimit -s -- ...` means "at most one running AND at most one per interval"
- **`spew` locks on STDOUT's underlying file (via `/proc/self/fd/1`), not a dedicated lockfile.** Every writer to the
    same file (or terminal) converges on the same lock target automatically, no coordination needed. Fallback is the
    running binary path when STDOUT is a pipe / socket with no filesystem entry
- **Signal-killed exit is `128 + signum` (shell convention), not Go's `-1` sentinel.** Set in `cli.ExitCodeFor` via
    `syscall.WaitStatus.Signaled()`. This is what bash reports and what every shell script author expects
- **`memoize` caches STDOUT, not exit codes.** Non-zero exit skips the cache write entirely (no negative caching); empty
    output is treated as a miss unless `-e` is passed (guards against caching "silently failed" runs). Sliding TTL
    (`-s`) bumps mtime on each hit, so a heavily-read cache never expires
- **Retry's flake mode is `-c 0`, not a dedicated flag.** `-c CODE` means "retry only on these codes"; passing 0 inverts
    the loop (retry WHILE succeeding, stop on first failure). Same primitive, dual-use. See `just detect-flaky-tests` in
    the justfile for the canonical usage

## GOTCHAS

Things that will trip a contributor writing their first patch.

- **Fixture timing bounds must survive load.** The integration harness runs every fixture concurrently under `xargs
  -P0`. A timing-tight fixture that passes in isolation can fail catastrophically under the parallel harness. Rule of
  thumb: wall-clock upper bounds of 3s or more, kill-escalation grace of 500ms or more, TTL windows at least 4x the
  sleep between reads. Prefer readiness synchronization (child writes `ready` to stdout, wrapper's pump resets its
  clock) over "wait N ms"
- **The trap-race pattern.** `bash -c 'trap "" TERM; sleep N'` does NOT guarantee the trap is installed before the outer
  wrapper's timer fires; bash's own startup under load can exceed 100 to 200ms. Use `bash -c 'trap "" TERM; echo ready;
  sleep N'` so the wrapper's stdout-watching timer only starts counting silence after bash confirms readiness. This
  burned two escalation fixtures; the pattern is now the standing convention
- **`memoize` on a side-effect-only command doesn't do what it looks like.** `memoize -- migrate` caches migrate's
  STDOUT, not its exit code. If migrate's stdout is empty, it's a cache miss every time. If migrate exits non-zero,
  nothing is cached. Use `ratelimit` for "run at most once per interval" or `once` for "one at a time"; `memoize` is for
  "reuse the output of an expensive read-only computation"
- **`-c` in retry accumulates AND splits on commas.** Both `-c 0,2` and `-c 0 -c 2` yield the same set. Same for `-s` in
  retry / watchdog. This is pflag `StringSliceVar` behaviour and users may be surprised expecting one or the other
  exclusively
- **`spew` buffers the entire child STDOUT in memory before emitting.** For giant outputs (multi-GB log dumps) it will
  hold that in RAM until the child exits. Use `prefix` when you want line-atomic streaming instead of block-atomic
  buffering

## AUTHOR

- Andrew Grechkin

## LICENSE

This project is licensed under the MIT License
See the `LICENSE` file for details.
