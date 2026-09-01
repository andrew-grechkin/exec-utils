package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

// Return the value of key if set and non-empty, otherwise fallback. Convenience for env-driven flag defaults where an
// explicit empty-string env-var should not override the built-in default.
func EnvDefault(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// Return the hex-encoded sha256 of s. Used by memoize and ratelimit for keying cache/state files by label or by the
// wrapped command's args.
func SHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
