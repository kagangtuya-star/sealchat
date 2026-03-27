package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/knadh/koanf/v2"
)

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

func TestReadConfigLogUploadEndpointOverride(t *testing.T) {
	oldK := k
	oldCurrentConfig := currentConfig
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := []byte("logUpload:\n  endpoint: https://example.com/custom-shader\n")
	if err := os.WriteFile(configPath, configContent, 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	k = koanf.New(".")
	currentConfig = nil
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(oldCwd)
		k = oldK
		currentConfig = oldCurrentConfig
	})

	cfg := ReadConfig()
	if cfg.LogUpload.Endpoint != "https://example.com/custom-shader" {
		t.Fatalf("unexpected log upload endpoint: %s", cfg.LogUpload.Endpoint)
	}
	if len(cfg.LogUpload.Endpoints) != 1 || cfg.LogUpload.Endpoints[0] != "https://example.com/custom-shader" {
		t.Fatalf("unexpected normalized log upload endpoints: %#v", cfg.LogUpload.Endpoints)
	}
	if !cfg.LogUpload.Enabled {
		t.Fatalf("expected log upload to remain enabled by default")
	}
	if cfg.LogUpload.TimeoutSeconds != 15 {
		t.Fatalf("unexpected default log upload timeout: %d", cfg.LogUpload.TimeoutSeconds)
	}
}

func TestReadConfigLogUploadEndpointsFallbackOrder(t *testing.T) {
	oldK := k
	oldCurrentConfig := currentConfig
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := []byte(
		"logUpload:\n" +
			"  endpoint: https://primary.example.com/dice/api/log\n" +
			"  endpoints:\n" +
			"    - https://backup-a.example.com/dice/api/log\n" +
			"    - https://primary.example.com/dice/api/log\n" +
			"    - \"  \"\n" +
			"    - https://backup-b.example.com/dice/api/log\n",
	)
	if err := os.WriteFile(configPath, configContent, 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	k = koanf.New(".")
	currentConfig = nil
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(oldCwd)
		k = oldK
		currentConfig = oldCurrentConfig
	})

	cfg := ReadConfig()
	expected := []string{
		"https://primary.example.com/dice/api/log",
		"https://backup-a.example.com/dice/api/log",
		"https://backup-b.example.com/dice/api/log",
	}
	if cfg.LogUpload.Endpoint != expected[0] {
		t.Fatalf("unexpected primary log upload endpoint: %s", cfg.LogUpload.Endpoint)
	}
	if len(cfg.LogUpload.Endpoints) != len(expected) {
		t.Fatalf("unexpected endpoint count: %#v", cfg.LogUpload.Endpoints)
	}
	for idx, want := range expected {
		if cfg.LogUpload.Endpoints[idx] != want {
			t.Fatalf("unexpected endpoint at %d: got %s want %s", idx, cfg.LogUpload.Endpoints[idx], want)
		}
	}
}

func TestResolveBotCommandPrefixesDefaults(t *testing.T) {
	got := ResolveBotCommandPrefixes(nil)
	expected := []string{".", "。", "．", "｡", "/"}
	if len(got) != len(expected) {
		t.Fatalf("unexpected prefix count: %#v", got)
	}
	for idx, want := range expected {
		if got[idx] != want {
			t.Fatalf("unexpected prefix at %d: got %q want %q", idx, got[idx], want)
		}
	}
}

func TestResolveBotCommandPrefixesCustom(t *testing.T) {
	got := ResolveBotCommandPrefixes([]string{"/", "!", " / "})
	expected := []string{"/", "!"}
	if len(got) != len(expected) {
		t.Fatalf("unexpected prefix count: %#v", got)
	}
	for idx, want := range expected {
		if got[idx] != want {
			t.Fatalf("unexpected prefix at %d: got %q want %q", idx, got[idx], want)
		}
	}
}
