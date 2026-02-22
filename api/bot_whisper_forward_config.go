package api

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	botWhisperForwardRuleTypeLegacyHiddenDice = "legacy_hidden_dice"
	botWhisperForwardRuleTypeKeyword          = "keyword"
	botWhisperForwardRuleTypeRegex            = "regex"
	botWhisperForwardRuleTypeAll              = "all"

	botWhisperForwardRuleLogicAny = "any"
	botWhisperForwardRuleLogicAll = "all"

	botWhisperForwardMaxRules       = 32
	botWhisperForwardMaxKeywordSize = 256
	botWhisperForwardMaxPatternSize = 1024
)

var botWhisperForwardRuleIDSanitizer = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

type BotWhisperForwardRule struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Enabled bool   `json:"enabled"`
	Keyword string `json:"keyword,omitempty"`
	Pattern string `json:"pattern,omitempty"`
	Flags   string `json:"flags,omitempty"`
}

type BotWhisperForwardConfig struct {
	Enabled                    bool                    `json:"enabled"`
	AsWhisper                  bool                    `json:"asWhisper"`
	AppendAtTargetsWhenWhisper bool                    `json:"appendAtTargetsWhenWhisper"`
	RuleLogic                  string                  `json:"ruleLogic"`
	Rules                      []BotWhisperForwardRule `json:"rules"`
}

func defaultBotWhisperForwardConfig() BotWhisperForwardConfig {
	return BotWhisperForwardConfig{
		Enabled:                    true,
		AsWhisper:                  true,
		AppendAtTargetsWhenWhisper: false,
		RuleLogic:                  botWhisperForwardRuleLogicAny,
		Rules: []BotWhisperForwardRule{
			{
				ID:      "legacy-hidden-dice",
				Type:    botWhisperForwardRuleTypeLegacyHiddenDice,
				Enabled: true,
			},
		},
	}
}

func parseBotWhisperForwardConfig(raw string) BotWhisperForwardConfig {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultBotWhisperForwardConfig()
	}
	var cfg BotWhisperForwardConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return defaultBotWhisperForwardConfig()
	}
	normalized, err := normalizeBotWhisperForwardConfig(cfg, false)
	if err != nil {
		return defaultBotWhisperForwardConfig()
	}
	return normalized
}

func validateAndNormalizeBotWhisperForwardConfig(raw string) (BotWhisperForwardConfig, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		cfg := defaultBotWhisperForwardConfig()
		data, _ := json.Marshal(cfg)
		return cfg, string(data), nil
	}
	var cfg BotWhisperForwardConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return BotWhisperForwardConfig{}, "", fmt.Errorf("配置格式错误: %w", err)
	}
	normalized, err := normalizeBotWhisperForwardConfig(cfg, true)
	if err != nil {
		return BotWhisperForwardConfig{}, "", err
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return BotWhisperForwardConfig{}, "", fmt.Errorf("配置序列化失败: %w", err)
	}
	return normalized, string(data), nil
}

func normalizeBotWhisperForwardConfig(cfg BotWhisperForwardConfig, strict bool) (BotWhisperForwardConfig, error) {
	if cfg.RuleLogic != botWhisperForwardRuleLogicAll {
		cfg.RuleLogic = botWhisperForwardRuleLogicAny
	}
	if len(cfg.Rules) > botWhisperForwardMaxRules {
		return BotWhisperForwardConfig{}, fmt.Errorf("规则数量不能超过 %d", botWhisperForwardMaxRules)
	}
	normalizedRules := make([]BotWhisperForwardRule, 0, len(cfg.Rules))
	for idx, rule := range cfg.Rules {
		rule.Type = strings.TrimSpace(strings.ToLower(rule.Type))
		if rule.Type == "" {
			if strict {
				return BotWhisperForwardConfig{}, fmt.Errorf("规则 #%d 缺少类型", idx+1)
			}
			continue
		}
		if !isSupportedBotWhisperForwardRuleType(rule.Type) {
			if strict {
				return BotWhisperForwardConfig{}, fmt.Errorf("规则 #%d 类型不支持: %s", idx+1, rule.Type)
			}
			continue
		}
		rule.ID = normalizeBotWhisperForwardRuleID(rule.ID, idx)
		switch rule.Type {
		case botWhisperForwardRuleTypeKeyword:
			rule.Keyword = strings.TrimSpace(rule.Keyword)
			if rule.Keyword == "" {
				if strict {
					return BotWhisperForwardConfig{}, fmt.Errorf("关键字规则 #%d 缺少 keyword", idx+1)
				}
				continue
			}
			if len(rule.Keyword) > botWhisperForwardMaxKeywordSize {
				return BotWhisperForwardConfig{}, fmt.Errorf("关键字规则 #%d 长度超过 %d", idx+1, botWhisperForwardMaxKeywordSize)
			}
			rule.Pattern = ""
			rule.Flags = ""
		case botWhisperForwardRuleTypeRegex:
			rule.Pattern = strings.TrimSpace(rule.Pattern)
			if rule.Pattern == "" {
				if strict {
					return BotWhisperForwardConfig{}, fmt.Errorf("正则规则 #%d 缺少 pattern", idx+1)
				}
				continue
			}
			if len(rule.Pattern) > botWhisperForwardMaxPatternSize {
				return BotWhisperForwardConfig{}, fmt.Errorf("正则规则 #%d 长度超过 %d", idx+1, botWhisperForwardMaxPatternSize)
			}
			flags, err := normalizeBotWhisperForwardRegexFlags(rule.Flags)
			if err != nil {
				return BotWhisperForwardConfig{}, fmt.Errorf("正则规则 #%d flags 非法: %w", idx+1, err)
			}
			rule.Flags = flags
			if strict {
				if _, err := compileBotWhisperForwardRegex(rule.Pattern, rule.Flags); err != nil {
					return BotWhisperForwardConfig{}, fmt.Errorf("正则规则 #%d 无法编译: %w", idx+1, err)
				}
			}
			rule.Keyword = ""
		default:
			rule.Keyword = ""
			rule.Pattern = ""
			rule.Flags = ""
		}
		normalizedRules = append(normalizedRules, rule)
	}
	if len(normalizedRules) == 0 {
		defaultCfg := defaultBotWhisperForwardConfig()
		normalizedRules = defaultCfg.Rules
	}
	cfg.Rules = normalizedRules
	return cfg, nil
}

func isSupportedBotWhisperForwardRuleType(ruleType string) bool {
	switch ruleType {
	case botWhisperForwardRuleTypeLegacyHiddenDice, botWhisperForwardRuleTypeKeyword, botWhisperForwardRuleTypeRegex, botWhisperForwardRuleTypeAll:
		return true
	default:
		return false
	}
}

func normalizeBotWhisperForwardRuleID(id string, idx int) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Sprintf("rule-%d", idx+1)
	}
	id = botWhisperForwardRuleIDSanitizer.ReplaceAllString(id, "-")
	id = strings.Trim(id, "-")
	if id == "" {
		return fmt.Sprintf("rule-%d", idx+1)
	}
	if len(id) > 64 {
		id = id[:64]
	}
	return id
}

func normalizeBotWhisperForwardRegexFlags(flags string) (string, error) {
	flags = strings.TrimSpace(strings.ToLower(flags))
	if flags == "" {
		return "", nil
	}
	seen := map[rune]bool{}
	result := make([]rune, 0, len(flags))
	for _, r := range flags {
		if r != 'i' && r != 'm' {
			return "", fmt.Errorf("仅允许 i/m")
		}
		if seen[r] {
			continue
		}
		seen[r] = true
		result = append(result, r)
	}
	return string(result), nil
}

func compileBotWhisperForwardRegex(pattern, flags string) (*regexp.Regexp, error) {
	prefix := ""
	if strings.Contains(flags, "i") {
		prefix += "(?i)"
	}
	if strings.Contains(flags, "m") {
		prefix += "(?m)"
	}
	return regexp.Compile(prefix + pattern)
}
