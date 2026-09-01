package cli

import (
	"syscall"
	"testing"
)

func TestParseSignal(t *testing.T) {
	cases := []struct {
		in      string
		want    syscall.Signal
		wantErr bool
	}{
		{"TERM", syscall.SIGTERM, false},
		{"SIGTERM", syscall.SIGTERM, false},
		{"sigterm", syscall.SIGTERM, false},
		{"  int  ", syscall.SIGINT, false},
		{"KILL", syscall.SIGKILL, false},
		{"bogus", 0, true},
	}
	for _, tc := range cases {
		got, err := ParseSignal(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseSignal(%q): err=%v wantErr=%v", tc.in, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("ParseSignal(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// Pin the auto-append rule: an empty list defaults to TERM,KILL; a list without KILL gets one appended; a list with
// KILL anywhere stays as given.
func TestParseSignals(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []syscall.Signal
	}{
		{"empty defaults to TERM,KILL", nil, []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL}},
		{"single TERM appends KILL", []string{"TERM"}, []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL}},
		{"TERM,INT appends KILL", []string{"TERM", "INT"}, []syscall.Signal{syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL}},
		{"list with trailing KILL kept as-is", []string{"TERM", "INT", "KILL"}, []syscall.Signal{syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL}},
		{"KILL alone stays alone", []string{"KILL"}, []syscall.Signal{syscall.SIGKILL}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseSignals(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("index %d: got %v, want %v", i, got[i], tc.want[i])
				}
			}
		})
	}
}
