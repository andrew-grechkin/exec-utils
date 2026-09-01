package main

import (
	"strings"
	"testing"
	"time"
)

func TestHelpTextEmbedded(t *testing.T) {
	if len(helpText) == 0 {
		t.Fatal("helpText is empty; help.txt was not embedded")
	}
	if !strings.Contains(string(helpText), "Usage: retry") {
		t.Errorf("helpText missing Usage line")
	}
}

func TestBackoffSequences(t *testing.T) {
	base := 100 * time.Millisecond
	cases := []struct {
		name string
		spec BackoffSpec
		want []time.Duration
	}{
		{"fixed", BackoffSpec{Kind: BackoffFixed}, []time.Duration{100, 100, 100, 100, 100}},
		{"linear", BackoffSpec{Kind: BackoffLinear}, []time.Duration{100, 200, 300, 400, 500}},
		{"exp x2", BackoffSpec{Kind: BackoffExponential, Factor: 2}, []time.Duration{100, 200, 400, 800, 1600}},
		{"fib", BackoffSpec{Kind: BackoffFibonacci}, []time.Duration{100, 100, 200, 300, 500}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for i, want := range tc.want {
				got := tc.spec.Delay(i+1, base)
				if got != want*time.Millisecond {
					t.Errorf("attempt %d: got %v, want %v", i+1, got, want*time.Millisecond)
				}
			}
		})
	}
}
