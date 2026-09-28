package utils

import (
	"os"
	"path/filepath"
	"strings"
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

func TestWriteConfigPersistsSpeech(t *testing.T) {
	oldK := k
	oldCurrentConfig := currentConfig
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("serveAt: :3212\n"), 0o644); err != nil {
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

	price := 0.00014
	cfg := ReadConfig()
	cfg.AI.Enabled = true
	cfg.AI.Speech = &SpeechConfig{
		Enabled:         true,
		DefaultProvider: "aliyun-test",
		DefaultVoice:    "voice-test",
		Format:          "wav",
		Providers: []SpeechProviderConfig{
			{
				ID:                "aliyun-test",
				Enabled:           true,
				CredentialScope:   "scope-test",
				APIKey:            "secret-test",
				Workspace:         "workspace-test",
				Region:            "cn-beijing",
				SynthesisEndpoint: "https://synthesis.example.com/api",
				VoiceEndpoint:     "https://voice.example.com/api",
				Model:             "qwen-audio-3.0-tts-plus",
				CharacterPrice:    &price,
			},
		},
	}

	WriteConfig(cfg)

	if cfg.AI.Speech == nil || len(cfg.AI.Speech.Providers) != 1 || cfg.AI.Speech.Providers[0].APIKey != "secret-test" {
		t.Fatalf("WriteConfig cleared caller speech config: %#v", cfg.AI.Speech)
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config failed: %v", err)
	}
	if !strings.Contains(string(raw), "speech:") || !strings.Contains(string(raw), "secret-test") || strings.Contains(string(raw), "hasApiKey") {
		t.Fatalf("unexpected persisted speech config:\n%s", raw)
	}

	k = koanf.New(".")
	currentConfig = nil
	got := ReadConfig()
	speech := got.AI.Speech
	if speech == nil {
		t.Fatal("ai.speech was not persisted")
	}
	if !speech.Enabled || speech.DefaultProvider != "aliyun-test" || speech.DefaultVoice != "voice-test" || speech.Format != "wav" {
		t.Fatalf("unexpected speech config: %#v", speech)
	}
	if len(speech.Providers) != 1 {
		t.Fatalf("unexpected provider count: %#v", speech.Providers)
	}
	p := speech.Providers[0]
	if p.ID != "aliyun-test" || !p.Enabled || p.APIKey != "secret-test" || p.CredentialScope != "scope-test" ||
		p.Workspace != "workspace-test" || p.Region != "cn-beijing" ||
		p.SynthesisEndpoint != "https://synthesis.example.com/api" || p.VoiceEndpoint != "https://voice.example.com/api" ||
		p.Model != "qwen-audio-3.0-tts-plus" || p.CharacterPrice == nil || *p.CharacterPrice != price {
		t.Fatalf("unexpected speech provider: %#v", p)
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

func TestNormalizeAIConfigRequestTimeoutDefaults(t *testing.T) {
	for _, value := range []int{0, -1} {
		cfg := NormalizeAIConfig(AIConfig{RequestTimeoutSeconds: value})
		if cfg.RequestTimeoutSeconds != 60 {
			t.Fatalf("RequestTimeoutSeconds(%d) = %d, want 60", value, cfg.RequestTimeoutSeconds)
		}
		if err := ValidateAIConfig(AIConfig{RequestTimeoutSeconds: value}); err != nil {
			t.Fatalf("ValidateAIConfig(%d) error = %v, want nil after normalization", value, err)
		}
	}
}

func TestNormalizeCertificateConfigRenewBeforeDays(t *testing.T) {
	for _, tt := range []struct {
		name   string
		issuer CertificateIssuer
		input  int
		want   int
	}{
		{name: "short-lived zero defaults to 3", issuer: CertificateIssuerLetsEncryptShortLived, input: 0, want: 3},
		{name: "short-lived old default migrates", issuer: CertificateIssuerLetsEncryptShortLived, input: 14, want: 3},
		{name: "short-lived 7 resets to default", issuer: CertificateIssuerLetsEncryptShortLived, input: 7, want: 3},
		{name: "short-lived 30 resets to default", issuer: CertificateIssuerLetsEncryptShortLived, input: 30, want: 3},
		{name: "short-lived 3 remains", issuer: CertificateIssuerLetsEncryptShortLived, input: 3, want: 3},
		{name: "short-lived 4 remains", issuer: CertificateIssuerLetsEncryptShortLived, input: 4, want: 4},
		{name: "short-lived 5 remains", issuer: CertificateIssuerLetsEncryptShortLived, input: 5, want: 5},
		{name: "short-lived 6 remains", issuer: CertificateIssuerLetsEncryptShortLived, input: 6, want: 6},
		{name: "ZeroSSL 14 remains", issuer: CertificateIssuerZeroSSL90Days, input: 14, want: 14},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NormalizeCertificateConfig(CertificateConfig{Issuer: tt.issuer, RenewBeforeDays: tt.input})
			if cfg.RenewBeforeDays != tt.want {
				t.Fatalf("renewBeforeDays = %d, want %d", cfg.RenewBeforeDays, tt.want)
			}
		})
	}
}

func TestValidateCertificateConfigRejectsZeroSSLTLSALPN(t *testing.T) {
	cfg := NormalizeCertificateConfig(CertificateConfig{
		Enabled:          true,
		SubjectIP:        "8.8.8.8",
		Issuer:           CertificateIssuerZeroSSL90Days,
		Challenge:        CertificateChallengeTLSALPN01,
		Email:            "admin@example.com",
		ZeroSSLEABKeyID:  "key-id",
		ZeroSSLEABMACKey: "mac-key",
	})
	if err := ValidateCertificateConfig(cfg); err == nil {
		t.Fatal("expected ZeroSSL TLS-ALPN-01 validation to fail")
	}
}

func TestValidateCertificateConfigHTTPSListenAddress(t *testing.T) {
	for _, addr := range []string{":443", ":8443", "0.0.0.0:443", "[::]:443"} {
		cfg := NormalizeCertificateConfig(CertificateConfig{HTTPSServeAt: addr})
		if err := ValidateCertificateConfig(cfg); err != nil {
			t.Errorf("expected %q to be valid: %v", addr, err)
		}
	}
	for _, addr := range []string{"443", ":0", ":65536", "127.0.0.1:not-a-port"} {
		cfg := NormalizeCertificateConfig(CertificateConfig{HTTPSServeAt: addr})
		if err := ValidateCertificateConfig(cfg); err == nil {
			t.Errorf("expected %q to be invalid", addr)
		}
	}
}
