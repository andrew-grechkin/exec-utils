# Contributing

## Development Setup

To get started with development, you'll need Go installed on your system.
Optionally, you can install [`just`](https://github.com/casey/just) for simplified command execution.

### Building

Build every binary under `cmd/` (test them compile successfully):

```bash
just build
```

Global install (into `$GOBIN`):

```bash
just install
```

Ensure that directory is on your `PATH` to run the utilities from anywhere.

### Updating Dependencies

To update the project's Go dependencies and clean up `go.mod` and `go.sum`, use:

```bash
just update
```

## Code Style and Linting

```bash
just fix
```

```bash
just lint
```

## Testing

Run the test suite with:

```bash
just test
```

Integration fixtures only (rebuilds binaries, then drives them):

```bash
just test-int
```

Integration tests are directory-driven. Each subdirectory of
`test/fixtures/<util>/` is one test case, described by plain-text files:

| filename                     | purpose                                              |
|------------------------------|------------------------------------------------------|
| `args`                       | argv passed to the util, one token per line          |
| `stdin`                      | raw stdin bytes                                      |
| `run`                        | executable script (invoked instead of the util itself, with `$BIN` set to the binary path) -- for tests that don't fit the "invoke once with args" shape |
| `expected-stdout`            | exact-match assertion on stdout                      |
| `expected-stdout-contains`   | substring assertion on stdout                        |
| `expected-stderr-contains`   | substring assertion on stderr                        |
| `expected-exit`              | integer exit code (default 0)                        |
| `description`                | free-form doc, ignored by the runner                 |

All files are optional. Missing `args` means no args; missing `expected-*` means no assertion on that stream.
