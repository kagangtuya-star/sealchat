package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/knadh/koanf/v2"
)

func readWebSocketConfigForTest(t *testing.T, content string) *AppConfig {
	t.Helper()
	oldK := k
	oldCurrentConfig := currentConfig
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	k = koanf.New(".")
	currentConfig = nil
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldCwd)
		k = oldK
		currentConfig = oldCurrentConfig
	})
	return ReadConfig()
}

func TestReadConfigWebSocketQueueDefaultForLegacyConfig(t *testing.T) {
	cfg := readWebSocketConfigForTest(t, "serveAt: :3212\n")
	if got := cfg.WebSocket.OutboundQueueSize; got != DefaultWebSocketOutboundQueueSize {
		t.Fatalf("outbound queue size = %d, want %d", got, DefaultWebSocketOutboundQueueSize)
	}
}

func TestReadConfigWebSocketQueueOverride(t *testing.T) {
	cfg := readWebSocketConfigForTest(t, "websocket:\n  outboundQueueSize: 64\n")
	if got := cfg.WebSocket.OutboundQueueSize; got != 64 {
		t.Fatalf("outbound queue size = %d, want 64", got)
	}
}
