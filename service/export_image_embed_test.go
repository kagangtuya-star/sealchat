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
	if !strings.Contains(got, `<img src="scasset:`) {
		t.Fatalf("expected content_html image to be replaced with shared asset ref, got %q", got)
	}
	if strings.Contains(got, `src="id:inline_payload_html_image"`) {
		t.Fatalf("expected attachment id src to be replaced, got %q", got)
	}
	if len(payload.InlineAssets) != 1 {
		t.Fatalf("expected one inline asset entry, got %d", len(payload.InlineAssets))
	}
}

func TestInlinePayloadDedupesRepeatedInlineAssets(t *testing.T) {
	initTestDB(t)

	cfg := utils.ReadConfig()
	oldUploadDir := cfg.Storage.Local.UploadDir
	uploadDir := t.TempDir()
	cfg.Storage.Local.UploadDir = uploadDir
	t.Cleanup(func() {
		cfg.Storage.Local.UploadDir = oldUploadDir
	})

	const attachmentID = "inline_payload_dedupe_image"
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
				ID:           "msg-inline-1",
				SenderAvatar: "id:inline_payload_dedupe_image",
				ContentHTML:  `<p><img src="id:inline_payload_dedupe_image" alt="图片1" /></p>`,
			},
			{
				ID:           "msg-inline-2",
				SenderAvatar: "id:inline_payload_dedupe_image",
				ContentHTML:  `<p><img src="id:inline_payload_dedupe_image" alt="图片2" /></p>`,
			},
		},
	}

	newInlineImageEmbedder().inlinePayload(payload)

	if len(payload.InlineAssets) != 1 {
		t.Fatalf("expected exactly 1 deduped inline asset, got %d (%v)", len(payload.InlineAssets), payload.InlineAssets)
	}

	if !strings.HasPrefix(payload.Messages[0].SenderAvatar, inlineAssetRefPrefix) {
		t.Fatalf("expected sender avatar to use shared asset ref, got %q", payload.Messages[0].SenderAvatar)
	}
	if payload.Messages[0].SenderAvatar != payload.Messages[1].SenderAvatar {
		t.Fatalf("expected repeated avatar refs to match, got %q vs %q", payload.Messages[0].SenderAvatar, payload.Messages[1].SenderAvatar)
	}
	if !strings.Contains(payload.Messages[0].ContentHTML, `src="`+payload.Messages[0].SenderAvatar+`"`) {
		t.Fatalf("expected first content html to use shared asset ref, got %q", payload.Messages[0].ContentHTML)
	}
	if !strings.Contains(payload.Messages[1].ContentHTML, `src="`+payload.Messages[1].SenderAvatar+`"`) {
		t.Fatalf("expected second content html to use shared asset ref, got %q", payload.Messages[1].ContentHTML)
	}
}
