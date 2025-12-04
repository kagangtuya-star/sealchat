package service

import "testing"

func TestNormalizeDomainToURLIPv6(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "ipv6 with port",
			input: "[2001:db8::1]:3212",
			want:  "https://[2001:db8::1]:3212",
		},
		{
			name:  "ipv6 loopback without port",
			input: "::1",
			want:  "http://[::1]",
		},
		{
			name:  "ipv4 loopback",
			input: "127.0.0.1:8080",
			want:  "http://127.0.0.1:8080",
		},
		{
			name:  "ipv6 link-local",
			input: "fe80::1",
			want:  "http://[fe80::1]",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeDomainToURL(tt.input); got != tt.want {
				t.Fatalf("normalizeDomainToURL(%q) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}
