package cli

import (
	"os"
	"path/filepath"
)

// Resolve a state/cache directory using the standard exec-utils precedence:
//  1. explicit != "" (typically a --dir flag value): use verbatim.
//  2. envKey set and non-empty: use its value.
//  3. $XDG_RUNTIME_DIR/<defaultName> when XDG_RUNTIME_DIR is set.
//  4. /tmp/<defaultName> otherwise.
//
// envKey may be empty to skip step 2 (for utils that don't expose an env override).
func ResolveStateDir(explicit, envKey, defaultName string) string {
	if explicit != "" {
		return explicit
	}
	if envKey != "" {
		if v := os.Getenv(envKey); v != "" {
			return v
		}
	}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		return filepath.Join(xdg, defaultName)
	}
	return filepath.Join("/tmp", defaultName)
}
