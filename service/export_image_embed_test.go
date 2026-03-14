package service

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sealchat/utils"
)

func TestInlinePayloadEmbedsContentHTMLImagesAsDataURL(t *testing.T) {
	initTestDB(t)

	cfg := utils.ReadConfig()
	oldUploadDir := cfg.Storage.Local.UploadDir
	uploadDir := t.TempDir()
	cfg.Storage.Local.UploadDir = uploadDir
	t.Cleanup(func() {
		cfg.Storage.Local.UploadDir = oldUploadDir
	})

	const attachmentID = "inline_payload_html_image"
	imageData, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO7Z0ioAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode png fixture failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uploadDir, attachmentID), imageData, 0644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}

	payload := &ExportPayload{
		Messages: []ExportMessage{
			{
				ID:          "msg-inline-html",
				Content:     "普通文本，不含图片标签",
				ContentHTML: `<p><img src="id:inline_payload_html_image" alt="图片" /></p>`,
			},
		},
	}

	newInlineImageEmbedder().inlinePayload(payload)

	got := payload.Messages[0].ContentHTML
	if !strings.Contains(got, `<img src="data:image/png;base64,`) {
		t.Fatalf("expected content_html image to be embedded as data url, got %q", got)
	}
	if strings.Contains(got, `src="id:inline_payload_html_image"`) {
		t.Fatalf("expected attachment id src to be replaced, got %q", got)
	}
}
