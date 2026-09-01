package cli

import "testing"

func TestResolveLockMode(t *testing.T) {
	cases := []struct {
		name             string
		wait, skip, fail bool
		want             LockMode
		wantErr          bool
	}{
		{"none defaults to wait", false, false, false, LockWait, false},
		{"only wait", true, false, false, LockWait, false},
		{"only skip", false, true, false, LockSkip, false},
		{"only fail", false, false, true, LockFail, false},
		{"wait+skip conflicts", true, true, false, 0, true},
		{"skip+fail conflicts", false, true, true, 0, true},
		{"all three conflicts", true, true, true, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveLockMode(tc.wait, tc.skip, tc.fail)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
