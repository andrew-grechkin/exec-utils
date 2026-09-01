package cli

import (
	"path/filepath"
	"testing"
)

func TestResolveStateDir(t *testing.T) {
	t.Run("explicit wins over everything", func(t *testing.T) {
		t.Setenv("XDG_RUNTIME_DIR", "/xdg")
		t.Setenv("FOO_DIR", "/env")
		if got := ResolveStateDir("/explicit", "FOO_DIR", "util"); got != "/explicit" {
			t.Errorf("got %q, want /explicit", got)
		}
	})
	t.Run("env wins over XDG when explicit empty", func(t *testing.T) {
		t.Setenv("XDG_RUNTIME_DIR", "/xdg")
		t.Setenv("FOO_DIR", "/env")
		if got := ResolveStateDir("", "FOO_DIR", "util"); got != "/env" {
			t.Errorf("got %q, want /env", got)
		}
	})
	t.Run("XDG when env unset", func(t *testing.T) {
		t.Setenv("XDG_RUNTIME_DIR", "/xdg")
		if got := ResolveStateDir("", "MISSING_ENV_VAR", "util"); got != filepath.Join("/xdg", "util") {
			t.Errorf("got %q, want /xdg/util", got)
		}
	})
	t.Run("tmp fallback when nothing set", func(t *testing.T) {
		t.Setenv("XDG_RUNTIME_DIR", "")
		if got := ResolveStateDir("", "MISSING_ENV_VAR", "util"); got != filepath.Join("/tmp", "util") {
			t.Errorf("got %q, want /tmp/util", got)
		}
	})
	t.Run("empty envKey skips env lookup", func(t *testing.T) {
		t.Setenv("XDG_RUNTIME_DIR", "/xdg")
		if got := ResolveStateDir("", "", "util"); got != filepath.Join("/xdg", "util") {
			t.Errorf("got %q, want /xdg/util", got)
		}
	})
}
