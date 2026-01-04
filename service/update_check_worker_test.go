package service

import "testing"

func TestIsLatestNewer(t *testing.T) {
	cases := []struct {
		name    string
		current string
		latest  string
		expect  bool
	}{
		{name: "newer date", current: "20260102-0362e01", latest: "20260103-0000000", expect: true},
		{name: "same version", current: "20260102-0362e01", latest: "20260102-0362e01", expect: false},
		{name: "same date newer sha", current: "20260102-0362e01", latest: "20260102-1362e01", expect: true},
		{name: "invalid current", current: "dev", latest: "20260102-0362e01", expect: true},
		{name: "empty current", current: "", latest: "20260102-0362e01", expect: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsLatestNewer(tc.current, tc.latest)
			if got != tc.expect {
				t.Fatalf("IsLatestNewer(%q,%q)=%v expect %v", tc.current, tc.latest, got, tc.expect)
			}
		})
	}
}
