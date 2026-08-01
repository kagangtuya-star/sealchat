package utils

import (
	"fmt"
	"strings"

	"sealchat/protocol"
)

const (
	MaxPlatformThemeCount       = 50
	MaxPlatformDice3DStyleCount = 50
	MaxPlatformThemeName        = 32
	MaxThemeColorValueLen       = 64
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
	PlatformThemes               []PlatformThemeConfig       `json:"platformThemes" yaml:"platformThemes"`
	DefaultPlatformThemeID       string                      `json:"defaultPlatformThemeId" yaml:"defaultPlatformThemeId"`
	PlatformDice3DStyles         []PlatformDice3DStyleConfig `json:"platformDice3DStyles" yaml:"platformDice3DStyles"`
	DefaultPlatformDice3DStyleID string                      `json:"defaultPlatformDice3DStyleId" yaml:"defaultPlatformDice3DStyleId"`
}

type PlatformDice3DStyleConfig struct {
	ID        string                     `json:"id" yaml:"id"`
	Name      string                     `json:"name" yaml:"name"`
	Config    protocol.Dice3DWorldConfig `json:"config" yaml:"config"`
	Skin      *protocol.Dice3DSkin       `json:"skin,omitempty" yaml:"skin,omitempty"`
	CreatedAt int64                      `json:"createdAt" yaml:"createdAt"`
	UpdatedAt int64                      `json:"updatedAt" yaml:"updatedAt"`
}

func NormalizeThemeManagementConfig(cfg ThemeManagementConfig) ThemeManagementConfig {
	result := ThemeManagementConfig{
		PlatformThemes:       make([]PlatformThemeConfig, 0, len(cfg.PlatformThemes)),
		PlatformDice3DStyles: make([]PlatformDice3DStyleConfig, 0, len(cfg.PlatformDice3DStyles)),
	}
	seenDiceStyleIDs := make(map[string]struct{}, len(cfg.PlatformDice3DStyles))
	for _, item := range cfg.PlatformDice3DStyles {
		id := strings.TrimSpace(item.ID)
		name := strings.TrimSpace(item.Name)
		if id == "" || name == "" {
			continue
		}
		if _, exists := seenDiceStyleIDs[id]; exists {
			continue
		}
		seenDiceStyleIDs[id] = struct{}{}
		item.ID = id
		item.Name = name
		if item.Config.Version == 0 && item.Skin != nil {
			item.Config = protocol.Dice3DWorldConfig{
				Version: 1, Enabled: true, SurfaceMode: "auto",
				CustomSurface: protocol.Dice3DCustomSurface{X: 0.1, Y: 0.1, Width: 0.8, Height: 0.8},
				DefaultSkin:   *item.Skin,
				Motion:        protocol.Dice3DMotionConfig{Speed: 1, ThrowForce: 1, WallBounce: 0.48, EntryEdge: "random", LingerMS: 8000, MaxDice: 60, Interactive: true},
				Audio:         protocol.Dice3DAudioConfig{Enabled: true, Volume: 0.65},
			}
		}
		item.Skin = nil
		item.Config.PlatformStyleID = id
		if item.Config.DefaultSkin.Textures == nil {
			item.Config.DefaultSkin.Textures = map[string]string{}
		}
		result.PlatformDice3DStyles = append(result.PlatformDice3DStyles, item)
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
	diceStyleDefaultID := strings.TrimSpace(cfg.DefaultPlatformDice3DStyleID)
	if _, exists := seenDiceStyleIDs[diceStyleDefaultID]; exists {
		result.DefaultPlatformDice3DStyleID = diceStyleDefaultID
	}

	return result
}

func ValidateThemeManagementConfig(cfg ThemeManagementConfig) error {
	if len(cfg.PlatformThemes) > MaxPlatformThemeCount {
		return fmt.Errorf("平台主题数量不能超过 %d", MaxPlatformThemeCount)
	}
	if len(cfg.PlatformDice3DStyles) > MaxPlatformDice3DStyleCount {
		return fmt.Errorf("平台 3D 骰子样式数量不能超过 %d", MaxPlatformDice3DStyleCount)
	}
	seenDiceStyleIDs := make(map[string]struct{}, len(cfg.PlatformDice3DStyles))
	seenDiceStyleNames := make(map[string]struct{}, len(cfg.PlatformDice3DStyles))
	for _, item := range cfg.PlatformDice3DStyles {
		id := strings.TrimSpace(item.ID)
		name := strings.TrimSpace(item.Name)
		if id == "" || name == "" {
			return fmt.Errorf("平台 3D 骰子样式 id 和名称不能为空")
		}
		if len([]rune(name)) > MaxPlatformThemeName {
			return fmt.Errorf("平台 3D 骰子样式名称不能超过 %d 个字符", MaxPlatformThemeName)
		}
		if _, exists := seenDiceStyleIDs[id]; exists {
			return fmt.Errorf("平台 3D 骰子样式 id 重复: %s", id)
		}
		seenDiceStyleIDs[id] = struct{}{}
		nameKey := strings.ToLower(name)
		if _, exists := seenDiceStyleNames[nameKey]; exists {
			return fmt.Errorf("平台 3D 骰子样式名称重复: %s", name)
		}
		seenDiceStyleNames[nameKey] = struct{}{}
		if len(item.Config.DefaultSkin.Textures) > 8 {
			return fmt.Errorf("平台 3D 骰子样式纹理数量不能超过 8")
		}
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
	if defaultID != "" {
		if _, exists := seenIDs[defaultID]; !exists {
			return fmt.Errorf("平台默认主题不存在: %s", defaultID)
		}
	}
	diceStyleDefaultID := strings.TrimSpace(cfg.DefaultPlatformDice3DStyleID)
	if diceStyleDefaultID != "" {
		if _, exists := seenDiceStyleIDs[diceStyleDefaultID]; !exists {
			return fmt.Errorf("平台默认 3D 骰子样式不存在: %s", diceStyleDefaultID)
		}
	}
	return nil
}
