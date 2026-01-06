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

func TestNormalizeServeAtIPv6(t *testing.T) {
	got, changed := NormalizeServeAt("::1")
	if !changed {
		t.Fatalf("expected IPv6 serveAt to be normalized")
	}
	if got != "[::1]:3212" {
		t.Fatalf("unexpected normalized serveAt: %s", got)
	}
}

func TestNormalizeDomainIPv6(t *testing.T) {
	got, changed := NormalizeDomain("2001:db8::1:3212")
	if !changed {
		t.Fatalf("expected IPv6 domain to be normalized")
	}
	if got != "[2001:db8::1]:3212" {
		t.Fatalf("unexpected normalized domain: %s", got)
	}
}
