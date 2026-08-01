package utils

import (
	"fmt"
	"strings"
)

const (
	CursorThemeVersion  = 1
	MaxCursorHotspot    = 127
	MaxCursorAttachment = 100
	MinCursorSize       = 16
	DefaultCursorSize   = 32
	MaxCursorSize       = 128
)

var CursorThemeSlots = []string{"default", "pointer", "text", "grab", "grabbing", "not-allowed"}

type CursorAssetConfig struct {
	Mode         string `json:"mode" yaml:"mode"`
	AttachmentID string `json:"attachmentId,omitempty" yaml:"attachmentId,omitempty"`
	HotspotX     int    `json:"hotspotX,omitempty" yaml:"hotspotX,omitempty"`
	HotspotY     int    `json:"hotspotY,omitempty" yaml:"hotspotY,omitempty"`
	Width        int    `json:"width,omitempty" yaml:"width,omitempty"`
	Height       int    `json:"height,omitempty" yaml:"height,omitempty"`
	Size         int    `json:"size,omitempty" yaml:"size,omitempty"`
	Animated     bool   `json:"animated,omitempty" yaml:"animated,omitempty"`
}

type CursorThemeConfig struct {
	Version int                          `json:"version" yaml:"version"`
	Slots   map[string]CursorAssetConfig `json:"slots" yaml:"slots"`
}

func NormalizeCursorThemeConfig(cfg CursorThemeConfig, allowInherit bool) CursorThemeConfig {
	result := CursorThemeConfig{Version: CursorThemeVersion, Slots: make(map[string]CursorAssetConfig)}
	allowed := make(map[string]struct{}, len(CursorThemeSlots))
	for _, slot := range CursorThemeSlots {
		allowed[slot] = struct{}{}
	}
	for slot, asset := range cfg.Slots {
		if _, ok := allowed[slot]; !ok {
			continue
		}
		mode := strings.TrimSpace(asset.Mode)
		if mode == "" {
			mode = "browser"
		}
		if mode == "inherit" && !allowInherit {
			mode = "browser"
		}
		if mode != "inherit" && mode != "browser" && mode != "custom" {
			continue
		}
		asset.Mode = mode
		asset.AttachmentID = strings.TrimPrefix(strings.TrimSpace(asset.AttachmentID), "id:")
		asset.HotspotX = max(0, min(asset.HotspotX, MaxCursorHotspot))
		asset.HotspotY = max(0, min(asset.HotspotY, MaxCursorHotspot))
		if asset.Size == 0 {
			asset.Size = DefaultCursorSize
		}
		asset.Size = max(MinCursorSize, min(asset.Size, MaxCursorSize))
		if mode != "custom" {
			asset.AttachmentID = ""
			asset.Width = 0
			asset.Height = 0
			asset.Size = 0
			asset.Animated = false
		}
		result.Slots[slot] = asset
	}
	return result
}

func ValidateCursorThemeConfig(cfg CursorThemeConfig, allowInherit bool) error {
	if cfg.Version != 0 && cfg.Version != CursorThemeVersion {
		return fmt.Errorf("不支持的鼠标样式版本: %d", cfg.Version)
	}
	if len(cfg.Slots) > len(CursorThemeSlots) {
		return fmt.Errorf("鼠标样式不能超过 %d 种", len(CursorThemeSlots))
	}
	allowed := make(map[string]struct{}, len(CursorThemeSlots))
	for _, slot := range CursorThemeSlots {
		allowed[slot] = struct{}{}
	}
	for slot, asset := range cfg.Slots {
		if _, ok := allowed[slot]; !ok {
			return fmt.Errorf("不支持的鼠标样式类型: %s", slot)
		}
		mode := strings.TrimSpace(asset.Mode)
		if mode == "" {
			mode = "browser"
		}
		if mode == "inherit" && !allowInherit {
			return fmt.Errorf("平台鼠标样式不能使用 inherit")
		}
		if mode != "inherit" && mode != "browser" && mode != "custom" {
			return fmt.Errorf("鼠标样式模式无效: %s", mode)
		}
		attachmentID := strings.TrimPrefix(strings.TrimSpace(asset.AttachmentID), "id:")
		if mode == "custom" && attachmentID == "" {
			return fmt.Errorf("鼠标样式 %s 缺少附件", slot)
		}
		if len(attachmentID) > MaxCursorAttachment {
			return fmt.Errorf("鼠标样式附件 ID 过长")
		}
		if asset.HotspotX < 0 || asset.HotspotX > MaxCursorHotspot || asset.HotspotY < 0 || asset.HotspotY > MaxCursorHotspot {
			return fmt.Errorf("鼠标热点坐标必须在 0-%d 之间", MaxCursorHotspot)
		}
		if mode == "custom" && asset.Size != 0 && (asset.Size < MinCursorSize || asset.Size > MaxCursorSize) {
			return fmt.Errorf("鼠标尺寸必须在 %d-%d 之间", MinCursorSize, MaxCursorSize)
		}
	}
	return nil
}
