package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/utils"
)

var (
	ErrDice3DConfigInvalid  = errors.New("3D 骰子世界配置无效")
	ErrDice3DProfileInvalid = errors.New("3D 骰子个人配置无效")
)

const (
	legacyDefaultDice3DBotPattern = `(?i)\[(?P<count>\d+)d(?P<sides>\d+)=(?P<values>\d+(?:\+\d+)*)\]`
	defaultDice3DBotPattern       = `(?i)\[(?P<count>\d*)d(?P<sides>\d+)=(?P<values>\d+(?:\+\d+)*)\]`
)

func DefaultDice3DWorldConfig() protocol.Dice3DWorldConfig {
	return protocol.Dice3DWorldConfig{
		Version:         1,
		PlatformStyleID: "",
		Enabled:         true,
		SurfaceMode:     "auto",
		CustomSurface:   protocol.Dice3DCustomSurface{X: 0.1, Y: 0.1, Width: 0.8, Height: 0.8},
		DefaultSkin: protocol.Dice3DSkin{
			FaceBackground: "#f5f6fa",
			FaceForeground: "#111827",
			EdgeColor:      "#d1d5db",
			Roughness:      0.72,
			Metalness:      0.05,
			Scale:          1,
		},
		Motion: protocol.Dice3DMotionConfig{
			Speed:       1,
			ThrowForce:  1,
			WallBounce:  0.48,
			EntryEdge:   "random",
			LingerMS:    8000,
			MaxDice:     60,
			Interactive: true,
		},
		Audio: protocol.Dice3DAudioConfig{Enabled: true, Volume: 0.65},
		BotRules: []protocol.Dice3DBotRule{{
			ID:                    "seal-standard",
			Name:                  "海豹骰标准",
			Enabled:               true,
			Pattern:               defaultDice3DBotPattern,
			CountGroup:            "count",
			SidesGroup:            "sides",
			ValuesGroup:           "values",
			ValueSeparatorPattern: `\+`,
		}},
	}
}

func NormalizeDice3DWorldConfig(value protocol.Dice3DWorldConfig) (protocol.Dice3DWorldConfig, error) {
	defaults := DefaultDice3DWorldConfig()
	if value.Version == 0 {
		return defaults, nil
	}
	value.Version = 1
	value.PlatformStyleID = strings.TrimSpace(value.PlatformStyleID)
	if len(value.PlatformStyleID) > 100 {
		return value, fmt.Errorf("%w: platformStyleId", ErrDice3DConfigInvalid)
	}
	if value.SurfaceMode == "" {
		value.SurfaceMode = defaults.SurfaceMode
	}
	switch value.SurfaceMode {
	case "auto", "chat", "theater", "fullscreen", "custom":
	default:
		return value, fmt.Errorf("%w: surfaceMode", ErrDice3DConfigInvalid)
	}
	value.DefaultSkin = normalizeDice3DSkin(value.DefaultSkin, defaults.DefaultSkin)
	value.CustomSurface.X = dice3DClampFloat(value.CustomSurface.X, 0, 1, defaults.CustomSurface.X)
	value.CustomSurface.Y = dice3DClampFloat(value.CustomSurface.Y, 0, 1, defaults.CustomSurface.Y)
	value.CustomSurface.Width = dice3DClampFloat(value.CustomSurface.Width, 0.1, 1, defaults.CustomSurface.Width)
	value.CustomSurface.Height = dice3DClampFloat(value.CustomSurface.Height, 0.1, 1, defaults.CustomSurface.Height)
	if value.CustomSurface.X+value.CustomSurface.Width > 1 {
		value.CustomSurface.X = 1 - value.CustomSurface.Width
	}
	if value.CustomSurface.Y+value.CustomSurface.Height > 1 {
		value.CustomSurface.Y = 1 - value.CustomSurface.Height
	}
	value.Motion.Speed = dice3DClampFloat(value.Motion.Speed, 0.25, 3, defaults.Motion.Speed)
	value.Motion.ThrowForce = dice3DClampFloat(value.Motion.ThrowForce, 0.25, 3, defaults.Motion.ThrowForce)
	value.Motion.WallBounce = dice3DClampFloat(value.Motion.WallBounce, 0, 0.95, defaults.Motion.WallBounce)
	if value.Motion.EntryEdge == "" {
		value.Motion.EntryEdge = defaults.Motion.EntryEdge
	}
	switch value.Motion.EntryEdge {
	case "random", "top", "right", "bottom", "left":
	default:
		return value, fmt.Errorf("%w: motion.entryEdge", ErrDice3DConfigInvalid)
	}
	value.Motion.LingerMS = dice3DClampInt(value.Motion.LingerMS, 500, 30000, defaults.Motion.LingerMS)
	value.Motion.MaxDice = dice3DClampInt(value.Motion.MaxDice, 1, 100, defaults.Motion.MaxDice)
	value.Audio.Volume = dice3DClampFloat(value.Audio.Volume, 0, 1, defaults.Audio.Volume)
	value.Audio.SoundAssetID = strings.TrimSpace(value.Audio.SoundAssetID)
	if len(value.Audio.SoundAssetID) > 200 {
		return value, fmt.Errorf("%w: 音效附件 ID 过长", ErrDice3DConfigInvalid)
	}
	if len(value.BotRules) == 0 {
		value.BotRules = defaults.BotRules
	}
	if len(value.BotRules) > 50 {
		return value, fmt.Errorf("%w: BOT 规则过多", ErrDice3DConfigInvalid)
	}
	for index := range value.BotRules {
		rule := &value.BotRules[index]
		if rule.Pattern == legacyDefaultDice3DBotPattern {
			rule.Pattern = defaultDice3DBotPattern
		}
		if strings.TrimSpace(rule.ID) == "" {
			rule.ID = fmt.Sprintf("rule-%d", index+1)
		}
		if strings.TrimSpace(rule.Pattern) == "" || len(rule.Pattern) > 2000 {
			return value, fmt.Errorf("%w: BOT 规则正则为空或过长", ErrDice3DConfigInvalid)
		}
		compiledRule, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return value, fmt.Errorf("%w: BOT 规则正则错误: %v", ErrDice3DConfigInvalid, err)
		}
		if rule.CountGroup == "" {
			rule.CountGroup = "count"
		}
		if rule.SidesGroup == "" {
			rule.SidesGroup = "sides"
		}
		if rule.ValuesGroup == "" {
			rule.ValuesGroup = "values"
		}
		if compiledRule.SubexpIndex(rule.CountGroup) < 0 || compiledRule.SubexpIndex(rule.SidesGroup) < 0 || compiledRule.SubexpIndex(rule.ValuesGroup) < 0 {
			return value, fmt.Errorf("%w: BOT 规则缺少指定捕获组", ErrDice3DConfigInvalid)
		}
		if rule.ValueSeparatorPattern == "" {
			rule.ValueSeparatorPattern = `\+`
		}
		if _, err := regexp.Compile(rule.ValueSeparatorPattern); err != nil {
			return value, fmt.Errorf("%w: 点数分隔正则错误: %v", ErrDice3DConfigInvalid, err)
		}
	}
	sort.SliceStable(value.BotRules, func(i, j int) bool { return value.BotRules[i].Priority > value.BotRules[j].Priority })
	return value, nil
}

func NormalizeDice3DMemberProfile(value protocol.Dice3DMemberProfile) (protocol.Dice3DMemberProfile, error) {
	value.Version = 1
	value.Skin = normalizeDice3DSkin(value.Skin, DefaultDice3DWorldConfig().DefaultSkin)
	if value.DockCorner == "" {
		value.DockCorner = "bottom-right"
	}
	switch value.DockCorner {
	case "top-left", "top-right", "bottom-left", "bottom-right", "free":
	default:
		return value, fmt.Errorf("%w: dockCorner", ErrDice3DProfileInvalid)
	}
	value.DockX = dice3DClampFloat(value.DockX, 0, 1, 0.9)
	value.DockY = dice3DClampFloat(value.DockY, 0, 1, 0.82)
	if len(value.DockStacks) == 0 {
		value.DockStacks = []protocol.Dice3DDockStack{{ID: "default-2d6", Label: "2d6", Expression: "2d6", Color: "#f5f6fa"}}
	}
	if len(value.DockStacks) > 8 {
		value.DockStacks = value.DockStacks[:8]
	}
	for index := range value.DockStacks {
		stack := &value.DockStacks[index]
		stack.ID = strings.TrimSpace(stack.ID)
		if stack.ID == "" {
			stack.ID = fmt.Sprintf("stack-%d", index+1)
		}
		stack.Label = strings.TrimSpace(stack.Label)
		stack.Expression = strings.TrimSpace(stack.Expression)
		if stack.Expression == "" || len(stack.Expression) > 100 {
			return value, fmt.Errorf("%w: 骰子堆表达式为空或过长", ErrDice3DProfileInvalid)
		}
		if stack.Label == "" {
			stack.Label = stack.Expression
		}
		if stack.Color == "" || !validHexColor(stack.Color) {
			stack.Color = value.Skin.FaceBackground
		}
	}
	if value.Audio != nil {
		value.Audio.Volume = dice3DClampFloat(value.Audio.Volume, 0, 1, 0.65)
		value.Audio.SoundAssetID = strings.TrimSpace(value.Audio.SoundAssetID)
		if len(value.Audio.SoundAssetID) > 200 {
			return value, fmt.Errorf("%w: 音效附件 ID 过长", ErrDice3DProfileInvalid)
		}
	}
	return value, nil
}

func ResolveDice3DWorldConfig(worldID string) (protocol.Dice3DWorldConfig, error) {
	var world model.WorldModel
	if err := model.GetDB().Where("id = ? AND status = ?", worldID, "active").Limit(1).Find(&world).Error; err != nil {
		return protocol.Dice3DWorldConfig{}, err
	}
	if world.ID == "" {
		return protocol.Dice3DWorldConfig{}, ErrWorldNotFound
	}
	worldConfig := world.GetDice3DConfig()
	if worldConfig.Version == 0 {
		if platformConfig, ok := resolvePlatformDice3DConfig(""); ok {
			return platformConfig, nil
		}
	}
	return NormalizeDice3DWorldConfig(worldConfig)
}

func resolvePlatformDice3DConfig(styleID string) (protocol.Dice3DWorldConfig, bool) {
	config := utils.GetConfig()
	if config == nil {
		return protocol.Dice3DWorldConfig{}, false
	}
	management := config.ThemeManagement
	targetID := strings.TrimSpace(styleID)
	if targetID == "" {
		targetID = strings.TrimSpace(management.DefaultPlatformDice3DStyleID)
	}
	if targetID == "" {
		return protocol.Dice3DWorldConfig{}, false
	}
	for _, item := range management.PlatformDice3DStyles {
		if item.ID != targetID {
			continue
		}
		normalized, err := NormalizeDice3DWorldConfig(item.Config)
		if err != nil {
			return protocol.Dice3DWorldConfig{}, false
		}
		normalized.PlatformStyleID = item.ID
		return normalized, true
	}
	return protocol.Dice3DWorldConfig{}, false
}

func NormalizePlatformDice3DStyles(management utils.ThemeManagementConfig) (utils.ThemeManagementConfig, error) {
	for index := range management.PlatformDice3DStyles {
		item := &management.PlatformDice3DStyles[index]
		normalized, err := NormalizeDice3DWorldConfig(item.Config)
		if err != nil {
			return management, fmt.Errorf("平台 3D 骰子样式 %q 无效: %w", item.Name, err)
		}
		normalized.PlatformStyleID = item.ID
		item.Config = normalized
	}
	return management, nil
}

func SaveDice3DWorldConfig(worldID, actorID string, value protocol.Dice3DWorldConfig) (protocol.Dice3DWorldConfig, error) {
	if !IsWorldAdmin(worldID, actorID) {
		return value, ErrWorldPermission
	}
	normalized, err := NormalizeDice3DWorldConfig(value)
	if err != nil {
		return value, err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return value, err
	}
	result := model.GetDB().Model(&model.WorldModel{}).Where("id = ? AND status = ?", worldID, "active").Update("dice_3d_config_json", string(raw))
	if result.Error != nil {
		return value, result.Error
	}
	if result.RowsAffected == 0 {
		return value, ErrWorldNotFound
	}
	return normalized, nil
}

func ResolveDice3DMemberProfile(worldID, userID string) (protocol.Dice3DMemberProfile, int64, error) {
	if !IsWorldMember(worldID, userID) {
		return protocol.Dice3DMemberProfile{}, 0, ErrWorldPermission
	}
	item, err := model.WorldMemberDice3DProfileGet(worldID, userID)
	if err != nil || item == nil {
		defaults, normalizeErr := NormalizeDice3DMemberProfile(protocol.Dice3DMemberProfile{})
		if err != nil {
			return defaults, 0, err
		}
		return defaults, 0, normalizeErr
	}
	normalized, err := NormalizeDice3DMemberProfile(item.GetProfile())
	return normalized, item.Revision, err
}

func SaveDice3DMemberProfile(worldID, userID string, value protocol.Dice3DMemberProfile) (protocol.Dice3DMemberProfile, int64, error) {
	if !IsWorldMember(worldID, userID) {
		return value, 0, ErrWorldPermission
	}
	normalized, err := NormalizeDice3DMemberProfile(value)
	if err != nil {
		return value, 0, err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return value, 0, err
	}
	item, err := model.WorldMemberDice3DProfileUpsert(worldID, userID, string(raw))
	if err != nil {
		return value, 0, err
	}
	return normalized, item.Revision, nil
}

func BuildDiceVisualPayload(messageID, worldID, channelID, actorUserID, content string, rolls []*model.MessageDiceRollModel, isBot bool, createdAt time.Time) (*protocol.DiceVisualPayload, error) {
	config, err := ResolveDice3DWorldConfig(worldID)
	if err != nil || !config.Enabled {
		return nil, err
	}
	groups := make([]protocol.DiceVisualGroup, 0)
	if len(rolls) > 0 {
		for _, roll := range rolls {
			if roll == nil || roll.IsError {
				continue
			}
			parsed := parseDiceGroups(defaultDice3DBotPattern, `\+`, roll.ResultDetail)
			parsed = append(parsed, parseDiceScriptPoolGroups(roll.ResultDetail)...)
			parsed = append(parsed, parseDiceScriptSingleGroups(roll.ResultDetail)...)
			groups = append(groups, parsed...)
		}
	}
	if isBot && len(groups) == 0 {
		for _, rule := range config.BotRules {
			if !rule.Enabled || !matchesDice3DRuleScope(rule, channelID, actorUserID) {
				continue
			}
			parsed := parseDiceGroupsFromRule(rule, content)
			groups = append(groups, parsed...)
			if len(parsed) > 0 {
				break
			}
		}
	}
	if len(groups) == 0 {
		return nil, nil
	}
	skin := config.DefaultSkin
	audio := config.Audio
	if profileItem, profileErr := model.WorldMemberDice3DProfileGet(worldID, actorUserID); profileErr == nil && profileItem != nil {
		profile, normalizeErr := NormalizeDice3DMemberProfile(profileItem.GetProfile())
		if normalizeErr == nil {
			if profile.UseOverride {
				skin = profile.Skin
			}
			if profile.Audio != nil {
				audio = *profile.Audio
			}
		}
	}
	createdMS := createdAt.UnixMilli()
	if createdMS <= 0 {
		createdMS = time.Now().UnixMilli()
	}
	return &protocol.DiceVisualPayload{
		Version: 1, RollID: messageID, MessageID: messageID, ChannelID: channelID,
		ActorUserID: actorUserID, Seed: dice3DSeed(messageID), Groups: groups,
		Appearance: skin, Motion: config.Motion, Audio: audio, SurfaceMode: config.SurfaceMode,
		CustomSurface: config.CustomSurface, CreatedAt: createdMS,
	}, nil
}

func parseDiceGroupsFromRule(rule protocol.Dice3DBotRule, content string) []protocol.DiceVisualGroup {
	re, err := regexp.Compile(rule.Pattern)
	if err != nil {
		return nil
	}
	separator, err := regexp.Compile(rule.ValueSeparatorPattern)
	if err != nil {
		return nil
	}
	return parseDiceGroupsWithRegex(re, separator, content, rule.CountGroup, rule.SidesGroup, rule.ValuesGroup)
}

func parseDiceGroups(pattern, separatorPattern, content string) []protocol.DiceVisualGroup {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	separator, err := regexp.Compile(separatorPattern)
	if err != nil {
		return nil
	}
	return parseDiceGroupsWithRegex(re, separator, content, "count", "sides", "values")
}

func parseDiceScriptSingleGroups(content string) []protocol.DiceVisualGroup {
	re := regexp.MustCompile(`(?i)(\d+)\[(?:1)?d(\d+)\]`)
	groups := make([]protocol.DiceVisualGroup, 0)
	for _, match := range re.FindAllStringSubmatch(content, -1) {
		if len(match) != 3 {
			continue
		}
		value, valueErr := strconv.Atoi(match[1])
		sides, sidesErr := strconv.Atoi(match[2])
		if valueErr != nil || sidesErr != nil || !supportedDice3DSides(sides) || value < 1 || value > sides {
			continue
		}
		groups = append(groups, protocol.DiceVisualGroup{Type: fmt.Sprintf("d%d", sides), Results: []int{value}})
	}
	return groups
}

func parseDiceScriptPoolGroups(content string) []protocol.DiceVisualGroup {
	re := regexp.MustCompile(`(?i)\[(\d+)d(\d+)[^=\]]*=\{([^}]+)\}\]`)
	groups := make([]protocol.DiceVisualGroup, 0)
	for _, match := range re.FindAllStringSubmatch(content, -1) {
		if len(match) != 4 {
			continue
		}
		count, countErr := strconv.Atoi(match[1])
		sides, sidesErr := strconv.Atoi(match[2])
		if countErr != nil || sidesErr != nil || count < 1 || count > 100 || !supportedDice3DSides(sides) {
			continue
		}
		parts := strings.Split(match[3], "|")
		if len(parts) != count {
			continue
		}
		values := make([]int, 0, count)
		valid := true
		for _, part := range parts {
			value, valueErr := strconv.Atoi(strings.TrimSpace(part))
			if valueErr != nil || value < 1 || value > sides {
				valid = false
				break
			}
			values = append(values, value)
		}
		if valid {
			groups = append(groups, protocol.DiceVisualGroup{Type: fmt.Sprintf("d%d", sides), Results: values})
		}
	}
	return groups
}

func parseDiceGroupsWithRegex(re, separator *regexp.Regexp, content, countName, sidesName, valuesName string) []protocol.DiceVisualGroup {
	countIndex := re.SubexpIndex(countName)
	sidesIndex := re.SubexpIndex(sidesName)
	valuesIndex := re.SubexpIndex(valuesName)
	if countIndex < 0 || sidesIndex < 0 || valuesIndex < 0 {
		return nil
	}
	groups := make([]protocol.DiceVisualGroup, 0)
	for _, match := range re.FindAllStringSubmatch(content, -1) {
		if countIndex >= len(match) || sidesIndex >= len(match) || valuesIndex >= len(match) {
			continue
		}
		countText := strings.TrimSpace(match[countIndex])
		if countText == "" {
			countText = "1"
		}
		count, countErr := strconv.Atoi(countText)
		sides, sidesErr := strconv.Atoi(match[sidesIndex])
		if countErr != nil || sidesErr != nil || count < 1 || count > 100 || !supportedDice3DSides(sides) {
			continue
		}
		parts := separator.Split(strings.TrimSpace(match[valuesIndex]), -1)
		if len(parts) != count {
			continue
		}
		values := make([]int, 0, count)
		valid := true
		for _, part := range parts {
			value, valueErr := strconv.Atoi(strings.TrimSpace(part))
			if valueErr != nil || value < 1 || value > sides {
				valid = false
				break
			}
			values = append(values, value)
		}
		if valid {
			groups = append(groups, protocol.DiceVisualGroup{Type: fmt.Sprintf("d%d", sides), Results: values})
		}
	}
	return groups
}

func matchesDice3DRuleScope(rule protocol.Dice3DBotRule, channelID, botUserID string) bool {
	return (len(rule.ChannelIDs) == 0 || containsString(rule.ChannelIDs, channelID)) &&
		(len(rule.BotUserIDs) == 0 || containsString(rule.BotUserIDs, botUserID))
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == target {
			return true
		}
	}
	return false
}

func supportedDice3DSides(sides int) bool {
	switch sides {
	case 2, 4, 6, 8, 10, 12, 20, 100:
		return true
	default:
		return false
	}
}

func normalizeDice3DSkin(value, defaults protocol.Dice3DSkin) protocol.Dice3DSkin {
	if !validHexColor(value.FaceBackground) {
		value.FaceBackground = defaults.FaceBackground
	}
	if !validHexColor(value.FaceForeground) {
		value.FaceForeground = defaults.FaceForeground
	}
	if !validHexColor(value.EdgeColor) {
		value.EdgeColor = defaults.EdgeColor
	}
	value.Roughness = dice3DClampFloat(value.Roughness, 0, 1, defaults.Roughness)
	value.Metalness = dice3DClampFloat(value.Metalness, 0, 1, defaults.Metalness)
	value.Scale = dice3DClampFloat(value.Scale, 0.5, 2, defaults.Scale)
	textures := make(map[string]string)
	for _, diceType := range []string{"d2", "d4", "d6", "d8", "d10", "d12", "d20", "d100"} {
		source := strings.TrimSpace(value.Textures[diceType])
		if source != "" && len(source) <= 1000 {
			textures[diceType] = source
		}
	}
	value.Textures = textures
	return value
}

func validHexColor(value string) bool {
	matched, _ := regexp.MatchString(`^#[0-9a-fA-F]{6}$`, strings.TrimSpace(value))
	return matched
}

func dice3DClampFloat(value, minimum, maximum, fallback float64) float64 {
	if value < minimum || value > maximum {
		return fallback
	}
	return value
}

func dice3DClampInt(value, minimum, maximum, fallback int) int {
	if value < minimum || value > maximum {
		return fallback
	}
	return value
}

func dice3DSeed(value string) int64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(value))
	return int64(hash.Sum64() & 0x7fffffffffffffff)
}
