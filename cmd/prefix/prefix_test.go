package main

import (
	"strings"
	"testing"
)

func TestHelpTextEmbedded(t *testing.T) {
	if len(helpText) == 0 {
		t.Fatal("helpText is empty; help.txt was not embedded")
	}
	if !strings.Contains(string(helpText), "Usage: prefix") {
		t.Errorf("helpText missing Usage line")
	}
}

func TestResolveTemplate(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{"explicit -p wins", Config{Prefix: "custom"}, "custom"},
		{"no -p defaults to timestamp", Config{}, "%T"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveTemplate(&tc.cfg)
			if got != tc.want {
				t.Errorf("resolveTemplate(%+v) = %q, want %q", tc.cfg, got, tc.want)
			}
		})
	}
}

func TestPrefixerTokens(t *testing.T) {
	p := &prefixer{template: "[%p #%n] %%", pid: 42, sep: " "}
	got1 := p.format()
	got2 := p.format()

	if !strings.HasPrefix(got1, "[42 #1] % ") {
		t.Errorf("first call = %q, want prefix %q", got1, "[42 #1] % ")
	}
	if !strings.HasPrefix(got2, "[42 #2] % ") {
		t.Errorf("second call = %q, want prefix %q", got2, "[42 #2] % ")
	}
}

func TestPrefixerUnknownToken(t *testing.T) {
	p := &prefixer{template: "%Q x", pid: 1, sep: ""}
	got := p.format()
	if got != "%Q x" {
		t.Errorf("got %q, want %q", got, "%Q x")
	}
}

func TestPrefixerStreamMark(t *testing.T) {
	p := &prefixer{template: "%p", pid: 7, streamMark: "1| ", sep: ": "}
	got := p.format()
	if got != "1| 7: " {
		t.Errorf("got %q, want %q", got, "1| 7: ")
	}
}
