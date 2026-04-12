package utils

import (
	"fmt"
	"strings"
)

const (
	MaxPlatformThemeCount = 50
	MaxPlatformThemeName  = 32
	MaxThemeColorValueLen = 64
)

type ThemeColorMap map[string]string

type PlatformThemeConfig struct {
	ID        string        `json:"id" yaml:"id"`
	Name      string        `json:"name" yaml:"name"`
	Colors    ThemeColorMap `json:"colors" yaml:"colors"`
	CreatedAt int64         `json:"createdAt" yaml:"createdAt"`
	UpdatedAt int64         `json:"updatedAt" yaml:"updatedAt"`
}

type ThemeManagementConfig struct {
	PlatformThemes         []PlatformThemeConfig `json:"platformThemes" yaml:"platformThemes"`
	DefaultPlatformThemeID string                `json:"defaultPlatformThemeId" yaml:"defaultPlatformThemeId"`
}

func NormalizeThemeManagementConfig(cfg ThemeManagementConfig) ThemeManagementConfig {
	result := ThemeManagementConfig{
		PlatformThemes: make([]PlatformThemeConfig, 0, len(cfg.PlatformThemes)),
	}
	seenIDs := make(map[string]struct{}, len(cfg.PlatformThemes))
	for _, item := range cfg.PlatformThemes {
		id := strings.TrimSpace(item.ID)
		name := strings.TrimSpace(item.Name)
		if id == "" || name == "" {
			continue
		}
		if _, exists := seenIDs[id]; exists {
			continue
		}
		seenIDs[id] = struct{}{}
		next := PlatformThemeConfig{
			ID:        id,
			Name:      name,
			Colors:    make(ThemeColorMap),
			CreatedAt: item.CreatedAt,
			UpdatedAt: item.UpdatedAt,
		}
		for key, value := range item.Colors {
			trimmedKey := strings.TrimSpace(key)
			trimmedValue := strings.TrimSpace(value)
			if trimmedKey == "" || trimmedValue == "" {
				continue
			}
			next.Colors[trimmedKey] = trimmedValue
		}
		result.PlatformThemes = append(result.PlatformThemes, next)
	}

	defaultID := strings.TrimSpace(cfg.DefaultPlatformThemeID)
	if defaultID != "" {
		for _, item := range result.PlatformThemes {
			if item.ID == defaultID {
				result.DefaultPlatformThemeID = defaultID
				break
			}
		}
	}

	return result
}

func ValidateThemeManagementConfig(cfg ThemeManagementConfig) error {
	if len(cfg.PlatformThemes) > MaxPlatformThemeCount {
		return fmt.Errorf("平台主题数量不能超过 %d", MaxPlatformThemeCount)
	}
	seenIDs := make(map[string]struct{}, len(cfg.PlatformThemes))
	seenNames := make(map[string]struct{}, len(cfg.PlatformThemes))
	for _, item := range cfg.PlatformThemes {
		id := strings.TrimSpace(item.ID)
		name := strings.TrimSpace(item.Name)
		if id == "" {
			return fmt.Errorf("平台主题 id 不能为空")
		}
		if name == "" {
			return fmt.Errorf("平台主题名称不能为空")
		}
		if len([]rune(name)) > MaxPlatformThemeName {
			return fmt.Errorf("平台主题名称不能超过 %d 个字符", MaxPlatformThemeName)
		}
		if _, exists := seenIDs[id]; exists {
			return fmt.Errorf("平台主题 id 重复: %s", id)
		}
		seenIDs[id] = struct{}{}
		nameKey := strings.ToLower(name)
		if _, exists := seenNames[nameKey]; exists {
			return fmt.Errorf("平台主题名称重复: %s", name)
		}
		seenNames[nameKey] = struct{}{}
		for key, value := range item.Colors {
			if strings.TrimSpace(key) == "" {
				return fmt.Errorf("平台主题颜色键不能为空")
			}
			if len(strings.TrimSpace(value)) > MaxThemeColorValueLen {
				return fmt.Errorf("平台主题颜色值过长: %s", key)
			}
		}
	}

	defaultID := strings.TrimSpace(cfg.DefaultPlatformThemeID)
	if defaultID == "" {
		return nil
	}
	if _, exists := seenIDs[defaultID]; !exists {
		return fmt.Errorf("平台默认主题不存在: %s", defaultID)
	}
	return nil
}
