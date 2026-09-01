# Ideas: sibling utilities

Ranked candidates for future additions to `exec-utils`. All share the same
grammar as `spew` / `memoize`: `<tool> [OPTIONS] [--] COMMAND [COMMAND ARGS...]`,
GNU-style flags, exit-code passthrough, embedded help, `--version`.

Composition ordering shorthand: `A B -- cmd` means A wraps B wraps cmd.
Innermost tools closest to `cmd`; outermost run last. E.g.
`spew memoize retry -- cmd` retries cmd until success, memoizes the
successful output, and serializes the emission.

These is just drafts, not final decisions, implementations and improvements need to be discussed

---

## quiet

Chronic-style output suppressor: hide COMMAND's output unless it fails.

    quiet [OPTIONS] [--] COMMAND [COMMAND ARGS...]

Options:

    -e, --err-only         On failure, show only stderr (default: both).
    -o, --out-only         On failure, show only stdout.
    -k, --keep             Show output on BOTH success and failure
                           (opposite of default; pairs with -l for tee-log).
    -l, --log FILE         Also tee to FILE, regardless of exit.
    -t, --tail N           On failure, show only the last N lines.

Examples:

    quiet -- flaky-migration                 # silent unless it fails
    retry -n 3 -- quiet -- deploy            # loud on each failure, silent on the eventual success
    quiet -- retry -n 3 -- deploy            # nothing unless the whole loop fails; then dump all attempts

Composition:

    The two orderings pick distinct semantics rather than one being "right":
    - retry OUTSIDE quiet: every failing attempt dumps its output live; the
      moment one attempt succeeds, quiet swallows it and the loop is silent.
    - quiet OUTSIDE retry: quiet buffers everything; if any attempt succeeds
      the whole thing is silent, otherwise on total failure quiet dumps all
      attempts together.

    "Print only the last failure's output" isn't a distinct mode; it's a
    shell trick when the command is idempotent:

        retry -n 3 -- cmd &>/dev/null || quiet -- cmd

    Silent for 3 attempts; if all fail, one more run under quiet surfaces a
    single failure. Costs one extra run on failure and only fits commands
    safe to re-invoke (no POST, no migration).

---

## Composition matrix (canonical stacks)

    parallel workers, clean output:
        xargs -P16 -rn1 bash -c 'spew memoize -t 1h -- work "$1"' _

    parallel workers with per-worker labels:
        xargs -P16 -rn1 bash -c 'spew prefix -w "w$1" -- work "$1"' _

    flaky network call, cache the eventual success:
        memoize retry -n 5 -d 2s -- curl -fsSL https://api/...

    stall-safe deploy with retries and silent success:
        quiet retry -n 3 -- watchdog -i 60s -- deploy

    at-most-once cron job:
        * * * * * once -s -l backup -- ratelimit -s -i 6h -- backup.sh
