package utils

import (
	"strings"
	"unicode/utf8"
)

// “更多跑团功能”入口布局：只决定入口的显示与顺序，不承载权限或可用性。
// action key 由前端 ChatActionRibbon 定义；这里不维护已知 key 列表，
// 以便旧服务端保存的配置能原样保留新版前端引入的入口。
const (
	ActionRibbonLayoutVersion       = 1
	MaxActionRibbonLayoutPresets    = 50
	MaxActionRibbonLayoutPresetName = 32
	MaxActionRibbonLayoutKeys       = 200
	MaxActionRibbonLayoutKeyLen     = 64
	MaxActionRibbonLayoutPresetID   = 64

	WorldActionRibbonLayoutModeInherit = "inherit"
	WorldActionRibbonLayoutModePreset  = "preset"
	WorldActionRibbonLayoutModeCustom  = "custom"
)

type ActionRibbonLayoutPreset struct {
	ID        string   `json:"id" yaml:"id"`
	Name      string   `json:"name" yaml:"name"`
	Order     []string `json:"order" yaml:"order"`
	Hidden    []string `json:"hidden" yaml:"hidden"`
	UpdatedAt int64    `json:"updatedAt" yaml:"updatedAt"`
}

// ActionRibbonLayoutConfig 平台级布局方案集合；DefaultPresetID 为空表示使用系统内置顺序。
type ActionRibbonLayoutConfig struct {
	Version         int                        `json:"version" yaml:"version"`
	DefaultPresetID string                     `json:"defaultPresetId" yaml:"defaultPresetId"`
	Presets         []ActionRibbonLayoutPreset `json:"presets" yaml:"presets"`
}

// WorldActionRibbonLayout 世界级布局：引用平台方案并以差异覆写。
// Mode=custom 时 PresetID 为基础方案（空表示平台默认），Order 为空表示沿用基础方案顺序，
// Hide/Show 是相对基础方案的显隐差异。
type WorldActionRibbonLayout struct {
	Version  int      `json:"version"`
	Mode     string   `json:"mode"`
	PresetID string   `json:"presetId,omitempty"`
	Order    []string `json:"order,omitempty"`
	Hide     []string `json:"hide,omitempty"`
	Show     []string `json:"show,omitempty"`
}

func isActionRibbonKeyRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == ':' || r == '.'
}

func normalizeActionRibbonKey(raw string) string {
	key := strings.TrimSpace(raw)
	if key == "" || len(key) > MaxActionRibbonLayoutKeyLen {
		return ""
	}
	for _, r := range key {
		if !isActionRibbonKeyRune(r) {
			return ""
		}
	}
	return key
}

// NormalizeActionRibbonKeys 去空、去重并丢弃非法 key，保持原有顺序。
func NormalizeActionRibbonKeys(keys []string) []string {
	out := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, raw := range keys {
		key := normalizeActionRibbonKey(raw)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
		if len(out) >= MaxActionRibbonLayoutKeys {
			break
		}
	}
	return out
}

func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

func NormalizeActionRibbonLayoutConfig(cfg ActionRibbonLayoutConfig) ActionRibbonLayoutConfig {
	result := ActionRibbonLayoutConfig{
		Version: ActionRibbonLayoutVersion,
		Presets: make([]ActionRibbonLayoutPreset, 0, len(cfg.Presets)),
	}
	seen := make(map[string]struct{}, len(cfg.Presets))
	for _, item := range cfg.Presets {
		id := strings.TrimSpace(item.ID)
		if id == "" || len(id) > MaxActionRibbonLayoutPresetID {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		name := truncateRunes(strings.TrimSpace(item.Name), MaxActionRibbonLayoutPresetName)
		if name == "" {
			name = "未命名方案"
		}
		seen[id] = struct{}{}
		result.Presets = append(result.Presets, ActionRibbonLayoutPreset{
			ID:        id,
			Name:      name,
			Order:     NormalizeActionRibbonKeys(item.Order),
			Hidden:    NormalizeActionRibbonKeys(item.Hidden),
			UpdatedAt: item.UpdatedAt,
		})
		if len(result.Presets) >= MaxActionRibbonLayoutPresets {
			break
		}
	}
	defaultID := strings.TrimSpace(cfg.DefaultPresetID)
	if _, ok := seen[defaultID]; ok {
		result.DefaultPresetID = defaultID
	}
	return result
}

// NormalizeWorldActionRibbonLayout 规范化世界布局；无法识别的模式回退为继承平台。
// 这里不校验 PresetID 是否仍存在：平台方案被删除后由解析端安全回退到平台默认，
// 管理员无需逐个修复引用该方案的世界。
func NormalizeWorldActionRibbonLayout(cfg WorldActionRibbonLayout) WorldActionRibbonLayout {
	result := WorldActionRibbonLayout{Version: ActionRibbonLayoutVersion, Mode: WorldActionRibbonLayoutModeInherit}
	presetID := strings.TrimSpace(cfg.PresetID)
	if len(presetID) > MaxActionRibbonLayoutPresetID {
		presetID = ""
	}
	switch strings.TrimSpace(cfg.Mode) {
	case WorldActionRibbonLayoutModePreset:
		if presetID == "" {
			return result
		}
		result.Mode = WorldActionRibbonLayoutModePreset
		result.PresetID = presetID
	case WorldActionRibbonLayoutModeCustom:
		result.Mode = WorldActionRibbonLayoutModeCustom
		result.PresetID = presetID
		result.Order = NormalizeActionRibbonKeys(cfg.Order)
		hide := NormalizeActionRibbonKeys(cfg.Hide)
		hidden := make(map[string]struct{}, len(hide))
		for _, key := range hide {
			hidden[key] = struct{}{}
		}
		show := make([]string, 0, len(cfg.Show))
		for _, key := range NormalizeActionRibbonKeys(cfg.Show) {
			if _, conflict := hidden[key]; !conflict {
				show = append(show, key)
			}
		}
		result.Hide = hide
		result.Show = show
	}
	return result
}
