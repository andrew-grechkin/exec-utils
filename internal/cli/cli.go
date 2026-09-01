// Package cli provides shared CLI-frontend helpers for exec-utils commands: exit-code constants (see exit.go) plus a
// build-info version printer used from every util's -v/--version path.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime/debug"
)

// Write the module build info as pretty-printed JSON to out, or "{}" when no build info is available (e.g. `go run`
// without VCS metadata). Called by each util's --version handler; keeps the output shape uniform across the fleet.
func PrintVersion(out io.Writer) error {
	if info, ok := debug.ReadBuildInfo(); ok {
		b, err := json.MarshalIndent(info.Main, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal build info: %w", err)
		}
		_, err = fmt.Fprintln(out, string(b))
		return err
	}
	_, err := fmt.Fprintln(out, "{}")
	return err
}
