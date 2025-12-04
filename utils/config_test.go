package utils

import "testing"

func TestDefaultImageBaseURLIPv6(t *testing.T) {
	got := defaultImageBaseURL("[2001:db8::1]:4000")
	if got != "[2001:db8::1]:4000" {
		t.Fatalf("unexpected default image base URL: %s", got)
	}
}

func TestFormatHostPort(t *testing.T) {
	got := FormatHostPort("2001:db8::2", "9000")
	if got != "[2001:db8::2]:9000" {
		t.Fatalf("expected IPv6 host to be bracketed, got %s", got)
	}
	if bare := FormatHostPort("example.com", "1234"); bare != "example.com:1234" {
		t.Fatalf("unexpected host formatting: %s", bare)
	}
}
