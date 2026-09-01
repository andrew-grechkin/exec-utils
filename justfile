#!/usr/bin/env -S just --one --justfile

set export

export GOBIN := `echo "${GOBIN:-${GOPATH:-$HOME/go}/bin}"`

alias fmt := fix

# Default recipe
[private]
@default: test

# Build all binaries (no binaries created in pattern mode, so it's just validation)
@build: fix
    go build ./cmd/...

# Run complexity lint
cc:
    #!/usr/bin/env -S bash -Eeuo pipefail
    [[ -x "$GOBIN/gocyclo" ]] || go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
    "$GOBIN/gocyclo" -over 15 .

# Remove build artifacts (everything in .gitignore)
@clean:
    git clean -Xdf

# Detect flaky tests by retrying test suite multiple times and stopping if one test fails
@detect-flaky-tests tries='50': install
    "$GOBIN/retry" -v -n '{{tries}}' -c 0 -d 0s -j 0.3 -- just test

# Show implementation documentation
[no-cd]
@doc:
    go doc -all -u .

# Format and modernize Go source code
@fix:
    go fmt ./...
    go fix ./...

# Install all binaries to $GOBIN
@install:
    go install ./cmd/...

# Run Go linter
@lint: cc
    go vet ./...

# Run all tests
test: lint test-unit test-int

# Run integration tests
test-int: install
    #!/usr/bin/env -S bash -Eeuo pipefail
    # Discovering fixtures under test/fixtures/<util>/<case>/
    #
    # Each fixture is a directory of plain-text files. Recognized filenames:
    #   args                        argv (one per line) passed to the util
    #   stdin                       raw stdin bytes for the util
    #   run                         executable script; when present, runs INSTEAD
    #                               of the util directly. The runner sets $BIN to
    #                               the util's binary path. Use this for tests
    #                               that don't fit the "invoke with args" shape
    #                               (concurrency, timing, multi-process).
    #   expected-stdout             exact-match assertion on stdout
    #   expected-stdout-contains    substring assertion on stdout
    #   expected-stderr-contains    substring assertion on stderr
    #   expected-exit               integer exit code (default 0)
    #   description                 free-form doc, ignored by runner
    shopt -s nullglob

    export LANG="C"

    # Two-level parallel: utils run concurrently under spew (each util's whole banner + fixture block is emitted
    # atomically), and within each util the fixtures ALSO run concurrently under spew (each fixture's pass/fail lines
    # are emitted atomically inside the util's block).
    # Wall time = max(slowest single fixture) across the whole tree.

    printf '%s\n' test/fixtures/*/ \
        | xargs -rn1 -P0 "$GOBIN/spew" just _run-util-fixtures \
        | just _test-int-stats

# Run unit tests
@test-unit: install
    go test -v ./...

# Update Go dependencies
@update:
    go get -u ./...
    go mod tidy

# Upgrade Golang
upgrade: && update
    #!/usr/bin/env -S bash -Eeuo pipefail
    go get go@latest

# Scan dependencies for known CVEs
vulncheck:
    #!/usr/bin/env -S bash -Eeuo pipefail
    [[ -x "$GOBIN/govulncheck" ]] || go install golang.org/x/vuln/cmd/govulncheck@latest
    "$GOBIN/govulncheck" ./...

# Watch tests
[positional-arguments, no-exit-message]
watch recipe='test' *args:
    #!/usr/bin/env -S bash -Eeuo pipefail
    shift 1
    [[ -x "$(command -v inotifywait)" ]] || { echo "inotifywait not found; install inotify-tools" >&2; exit 1; }

    while true; do
        just "$recipe" "$@" || true
        echo "{{YELLOW}}> watching for changes (ctrl-c to stop){{NORMAL}}" >&2
        inotifywait -r -q -e modify,create,delete,move --exclude '(^|/)\.git(/|$)' .
    done

# Run one util's fixtures in parallel; produce its whole block on STDOUT
[private, no-exit-message]
_run-util-fixtures util_dir:
    #!/usr/bin/env -S bash -Eeuo pipefail
    shopt -s nullglob
    util=$(basename "$util_dir")
    bin="$GOBIN/$util"
    if [[ ! -x "$bin" ]]; then
        printf '\n> %s: no binary at %s (SKIP)\n' "$util" "$bin"
        exit 0
    fi

    fixtures=("$util_dir"*/)
    (( ${#fixtures[@]} == 0 )) && exit 0

    printf '\n> %s fixtures\n' "$util"
    printf '%s\n' "${fixtures[@]}" \
        | xargs -rn1 -P0 "$GOBIN/spew" just _run-fixture "$bin"

# Print the summary and exits non-zero if any fail
[private, no-exit-message]
_test-int-stats:
    #!/usr/bin/env -S perl -p
    BEGIN { $| = 1 }
    $pass++ if m/PASS\b/;
    $fail++ if m/FAIL\b/;
    END {
        printf "\n%d passed, %d failed\n", $pass || 0, $fail || 0;
        exit($fail ? 1 : 0);
    }

# Run a single fixture and print a colored pass/fail line
[private, no-exit-message]
_run-fixture bin fdir:
    #!/usr/bin/env -S bash -Eeuo pipefail
    util=$(basename "$(dirname "$fdir")")
    name="$util/$(basename "$fdir")"

    actual_stdout=$(mktemp)
    actual_stderr=$(mktemp)
    # shellcheck disable=SC2064
    trap "rm -f '$actual_stdout' '$actual_stderr'" EXIT

    if [[ -x "$fdir/run" ]]; then
        set +e
        BIN="$bin" "$fdir/run" >"$actual_stdout" 2>"$actual_stderr"
        actual_exit=$?
        set -e
    else
        args=()
        if [[ -f "$fdir/args" ]]; then
            while IFS= read -r line || [[ -n "$line" ]]; do
                [[ -z "$line" ]] && continue
                args+=("$line")
            done < "$fdir/args"
        fi
        stdin_src=/dev/null
        [[ -f "$fdir/stdin" ]] && stdin_src="$fdir/stdin"
        set +e
        "$bin" "${args[@]}" <"$stdin_src" >"$actual_stdout" 2>"$actual_stderr"
        actual_exit=$?
        set -e
    fi

    want_exit=0
    [[ -f "$fdir/expected-exit" ]] && want_exit=$(<"$fdir/expected-exit") && want_exit=${want_exit//$'\n'/}

    errs=""
    if [[ "$actual_exit" != "$want_exit" ]]; then
        errs+="exit: got $actual_exit, want $want_exit"$'\n'
        errs+="stderr: $(cat "$actual_stderr")"$'\n'
    fi

    if [[ -f "$fdir/expected-stdout" ]]; then
        if ! diff_out=$(diff -u "$fdir/expected-stdout" "$actual_stdout" 2>&1); then
            errs+="stdout mismatch:"$'\n'"$diff_out"$'\n'
        fi
    fi

    if [[ -f "$fdir/expected-stdout-contains" ]]; then
        want=$(<"$fdir/expected-stdout-contains"); want=${want%$'\n'}
        if ! grep -qF -- "$want" "$actual_stdout"; then
            errs+="stdout missing substring: $want"$'\n'
            errs+="stdout was: $(cat "$actual_stdout")"$'\n'
        fi
    fi

    if [[ -f "$fdir/expected-stderr-contains" ]]; then
        want=$(<"$fdir/expected-stderr-contains"); want=${want%$'\n'}
        if ! grep -qF -- "$want" "$actual_stderr"; then
            errs+="stderr missing substring: $want"$'\n'
            errs+="stderr was: $(cat "$actual_stderr")"$'\n'
        fi
    fi

    if [[ -z "$errs" ]]; then
        printf '  \033[32mPASS\033[0m: %s\n' "$name"
    else
        printf '  \033[31mFAIL\033[0m: %s\n' "$name"
        printf '%s' "$errs" | sed 's/^/      /'
        exit 1
    fi
