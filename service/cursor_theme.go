package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"sealchat/model"
	"sealchat/utils"
)

func ResizeAnimatedWebPCursor(data []byte, width, height, quality int, toolchain MediaToolchain) ([]byte, error) {
	if !toolchain.FFmpegAvailable() {
		return nil, fmt.Errorf("动态 WebP 调整尺寸需要 FFmpeg")
	}
	canvasWidth, canvasHeight, _, frames, err := parseAnimatedWebP(data)
	if err != nil {
		return nil, err
	}
	tempDir, err := os.MkdirTemp("", "sealchat-cursor-animation-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tempDir)
	sourcePath := filepath.Join(tempDir, "source.webp")
	if err := os.WriteFile(sourcePath, data, 0o600); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	concatPath, err := decodeAnimatedWebPFrames(ctx, sourcePath, tempDir, theaterMediaMetadata{
		Width:      canvasWidth,
		Height:     canvasHeight,
		FrameCount: len(frames),
		HasAlpha:   true,
	})
	if err != nil {
		return nil, err
	}
	outputPath := filepath.Join(tempDir, "resized.webp")
	filter := fmt.Sprintf("scale=%d:%d:flags=lanczos,format=rgba", width, height)
	commandOutput, err := (execMediaCommandRunner{}).Run(ctx, toolchain.FFmpegPath,
		"-y", "-f", "concat", "-safe", "0", "-i", concatPath, "-map", "0:v:0", "-an",
		"-vf", filter, "-vsync", "vfr", "-c:v", "libwebp_anim", "-lossless", "1",
		"-quality", strconv.Itoa(max(1, min(quality, 100))), "-loop", "0", outputPath,
	)
	if err != nil {
		message := strings.TrimSpace(string(commandOutput))
		if len(message) > 500 {
			message = message[len(message)-500:]
		}
		return nil, fmt.Errorf("动态 WebP 调整失败: %s", message)
	}
	return os.ReadFile(outputPath)
}

func ValidateCursorThemeAttachments(cfg utils.CursorThemeConfig, worldID, actorID string) error {
	for slot, asset := range cfg.Slots {
		if asset.Mode != "custom" {
			continue
		}
		attachmentID := strings.TrimPrefix(strings.TrimSpace(asset.AttachmentID), "id:")
		attachment, err := ResolveAttachment(attachmentID)
		if err != nil {
			return err
		}
		if attachment == nil {
			return fmt.Errorf("鼠标样式 %s 的附件不存在", slot)
		}
		if strings.ToLower(strings.TrimSpace(attachment.MimeType)) != "image/webp" {
			return fmt.Errorf("鼠标样式 %s 必须使用 WebP 图片", slot)
		}
		if worldID != "" && attachment.UserID != actorID && !(attachment.RootIDType == "world_cursor" && attachment.RootID == worldID) {
			return fmt.Errorf("鼠标样式 %s 的附件不属于当前世界管理员", slot)
		}
	}
	return nil
}

func validateWorldCursorThemeAttachments(worldID, actorID string, cfg utils.CursorThemeConfig) error {
	return ValidateCursorThemeAttachments(cfg, worldID, actorID)
}

func ConfirmCursorThemeAttachments(cfg utils.CursorThemeConfig) {
	ids := make([]string, 0, len(cfg.Slots))
	for _, asset := range cfg.Slots {
		if asset.Mode == "custom" && strings.TrimSpace(asset.AttachmentID) != "" {
			ids = append(ids, strings.TrimPrefix(strings.TrimSpace(asset.AttachmentID), "id:"))
		}
	}
	if len(ids) > 0 {
		model.GetDB().Model(&model.AttachmentModel{}).Where("id IN ?", ids).Update("is_temp", false)
	}
}
