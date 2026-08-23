package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/protocol"
)

const (
	ChannelIdentityVariantMatchModePrefix  = "prefix"
	ChannelIdentityVariantMatchModeKeyword = "keyword"
	ChannelIdentityVariantMatchModeRegex   = "regex"
)

type ChannelIdentityVariantInput struct {
	ChannelID                  string
	IdentityID                 string
	SelectorEmoji              string
	Keyword                    string
	MatchMode                  string
	MatchConfig                string
	Note                       string
	AvatarAttachmentID         string
	DisplayName                string
	Color                      string
	Appearance                 map[string]any
	Enabled                    bool
	TheaterPresentation        *protocol.TheaterPresentationPatch
	TheaterPresentationSet     bool
	SkipTheaterAssetValidation bool
	ExpectedRevision           int64
}

type ResolvedIdentityAppearance struct {
	IdentityID          string
	VariantID           string
	DisplayName         string
	Color               string
	AvatarAttachmentID  string
	AvatarDecorations   protocol.AvatarDecorationList
	TheaterPresentation *protocol.TheaterPresentation
}

func normalizeChannelIdentityVariantKeyword(keyword string) string {
	return strings.TrimSpace(keyword)
}

func normalizeChannelIdentityVariantMatch(input *ChannelIdentityVariantInput) {
	input.MatchMode = strings.ToLower(strings.TrimSpace(input.MatchMode))
	input.MatchConfig = strings.TrimSpace(input.MatchConfig)
	if input.MatchMode == "" {
		input.MatchMode = ChannelIdentityVariantMatchModePrefix
	}
	switch input.MatchMode {
	case ChannelIdentityVariantMatchModePrefix:
		if input.MatchConfig == "" {
			input.MatchConfig = "="
		}
	case ChannelIdentityVariantMatchModeKeyword:
		input.MatchConfig = strings.ToLower(input.MatchConfig)
		if input.MatchConfig == "" {
			input.MatchConfig = "any"
		}
	case ChannelIdentityVariantMatchModeRegex:
		input.MatchConfig = strings.ToLower(input.MatchConfig)
		if input.MatchConfig == "" {
			input.MatchConfig = "sensitive"
		}
	}
}

func normalizeChannelIdentityVariantEmoji(value string) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) > 64 {
		return string([]rune(value)[:64])
	}
	return value
}

func normalizeChannelIdentityVariantNote(note string) string {
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > 255 {
		return string([]rune(note)[:255])
	}
	return note
}

func normalizeChannelIdentityVariantAppearance(input *ChannelIdentityVariantInput) (map[string]any, error) {
	result := map[string]any{}
	for key, value := range input.Appearance {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" || trimmedKey == "theaterPresentation" {
			continue
		}
		result[trimmedKey] = value
	}

	displayName := strings.TrimSpace(input.DisplayName)
	if displayName != "" {
		if utf8.RuneCountInString(displayName) > 32 {
			return nil, errors.New("差分显示名长度需在32个字符以内")
		}
		result["displayName"] = displayName
	}
	input.DisplayName = displayName

	if input.Color != "" {
		color := model.ChannelIdentityNormalizeColor(input.Color)
		if color == "" {
			return nil, errors.New("差分颜色格式不正确")
		}
		input.Color = color
		result["color"] = color
	}

	input.AvatarAttachmentID = strings.TrimSpace(input.AvatarAttachmentID)
	if input.AvatarAttachmentID != "" {
		result["avatarAttachmentId"] = input.AvatarAttachmentID
	}
	if input.TheaterPresentation != nil {
		if err := protocol.ValidateTheaterPresentationPatch(*input.TheaterPresentation); err != nil {
			return nil, fmt.Errorf("演出外观差分无效: %w", err)
		}
		result["theaterPresentation"] = input.TheaterPresentation
	}
	return result, nil
}

func variantTheaterPresentationPatch(item *model.ChannelIdentityVariantModel) (*protocol.TheaterPresentationPatch, bool) {
	if item == nil || strings.TrimSpace(item.AppearanceJSON) == "" {
		return nil, false
	}
	var document struct {
		TheaterPresentation json.RawMessage `json:"theaterPresentation"`
	}
	if json.Unmarshal([]byte(item.AppearanceJSON), &document) != nil || len(document.TheaterPresentation) == 0 {
		return nil, false
	}
	if strings.TrimSpace(string(document.TheaterPresentation)) == "null" {
		return nil, true
	}
	var patch protocol.TheaterPresentationPatch
	if json.Unmarshal(document.TheaterPresentation, &patch) != nil {
		return nil, false
	}
	return &patch, true
}

func validateChannelIdentityVariantInput(input *ChannelIdentityVariantInput) error {
	input.ChannelID = strings.TrimSpace(input.ChannelID)
	input.IdentityID = strings.TrimSpace(input.IdentityID)
	input.Keyword = normalizeChannelIdentityVariantKeyword(input.Keyword)
	normalizeChannelIdentityVariantMatch(input)
	input.SelectorEmoji = normalizeChannelIdentityVariantEmoji(input.SelectorEmoji)
	input.Note = normalizeChannelIdentityVariantNote(input.Note)

	if input.ChannelID == "" {
		return errors.New("缺少频道ID")
	}
	if input.IdentityID == "" {
		return errors.New("缺少身份ID")
	}
	if input.Keyword == "" {
		return errors.New("差分匹配内容不能为空")
	}
	if utf8.RuneCountInString(input.Keyword) > 255 {
		return errors.New("差分匹配内容长度不能超过255个字符")
	}
	switch input.MatchMode {
	case ChannelIdentityVariantMatchModePrefix:
		if utf8.RuneCountInString(input.MatchConfig) > 8 || strings.IndexFunc(input.MatchConfig, unicode.IsSpace) >= 0 {
			return errors.New("前缀符号不能包含空白且长度不能超过8个字符")
		}
	case ChannelIdentityVariantMatchModeKeyword:
		if input.MatchConfig != "any" && input.MatchConfig != "all" {
			return errors.New("无效的关键词匹配类型")
		}
		separator, forbidden := "|", "&"
		if input.MatchConfig == "all" {
			separator, forbidden = "&", "|"
		}
		if strings.Contains(input.Keyword, forbidden) {
			return errors.New("关键词匹配内容不能混用 | 和 &")
		}
		for _, part := range strings.Split(input.Keyword, separator) {
			if strings.TrimSpace(part) == "" {
				return errors.New("关键词匹配内容包含空关键词")
			}
		}
	case ChannelIdentityVariantMatchModeRegex:
		if input.MatchConfig != "sensitive" && input.MatchConfig != "insensitive" {
			return errors.New("无效的正则表达式匹配方式")
		}
		pattern := input.Keyword
		if input.MatchConfig == "insensitive" {
			pattern = "(?i:" + pattern + ")"
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return errors.New("正则表达式无效")
		}
	default:
		return errors.New("无效的差分匹配方式")
	}
	if input.SelectorEmoji == "" {
		return errors.New("差分选择表情不能为空")
	}
	if utf8.RuneCountInString(input.SelectorEmoji) > 64 {
		return errors.New("差分选择表情过长")
	}
	if utf8.RuneCountInString(input.Note) > 255 {
		return errors.New("差分备注过长")
	}
	return nil
}

func ensureIdentityVariantAttachmentAccessible(ownerUserID string, operatorUserID string, channelID string, attachmentID string) error {
	if attachmentID == "" {
		return nil
	}
	_, err := ResolveAttachmentAccessible(ownerUserID, operatorUserID, channelID, attachmentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("差分头像附件不存在")
		}
		return err
	}
	return nil
}

func ensureChannelIdentityVariantOwnership(userID string, channelID string, identityID string) (*model.ChannelIdentityModel, error) {
	identity, err := model.ChannelIdentityValidateOwnership(identityID, userID, channelID)
	if err != nil {
		return nil, err
	}
	return identity, nil
}

func ensureChannelIdentityVariantKeywordUnique(userID string, channelID string, identityID string, keyword string, matchMode string, matchConfig string, excludeID string) error {
	items, err := model.ChannelIdentityVariantListByIdentityID(channelID, userID, identityID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item == nil || item.ID == excludeID {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(item.Keyword), keyword) && item.MatchMode == matchMode && item.MatchConfig == matchConfig {
			return fmt.Errorf("差分匹配规则 %s 已存在", keyword)
		}
	}
	return nil
}

func serializeChannelIdentityVariantAppearanceJSON(data map[string]any) (string, error) {
	if len(data) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func ChannelIdentityVariantListByUser(channelID string, userID string) ([]*model.ChannelIdentityVariantModel, error) {
	channelID = strings.TrimSpace(channelID)
	userID = strings.TrimSpace(userID)
	if channelID == "" || userID == "" {
		return []*model.ChannelIdentityVariantModel{}, nil
	}
	return model.ChannelIdentityVariantList(channelID, userID)
}

func ChannelIdentityVariantCreate(userID string, input *ChannelIdentityVariantInput) (*model.ChannelIdentityVariantModel, error) {
	return ChannelIdentityVariantCreateWithAccess(userID, userID, input)
}

func ChannelIdentityVariantCreateWithAccess(ownerUserID string, operatorUserID string, input *ChannelIdentityVariantInput) (*model.ChannelIdentityVariantModel, error) {
	if input == nil {
		return nil, errors.New("参数不能为空")
	}
	if err := validateChannelIdentityVariantInput(input); err != nil {
		return nil, err
	}
	identity, err := ensureChannelIdentityVariantOwnership(ownerUserID, input.ChannelID, input.IdentityID)
	if err != nil {
		return nil, err
	}
	if input.TheaterPresentation != nil && !input.SkipTheaterAssetValidation && identity.SharedIdentityID == "" {
		if err := ValidateTheaterPresentationPatchAppearanceAssets(model.GetDB(), input.ChannelID, ownerUserID, identity.ID, *input.TheaterPresentation); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(operatorUserID) == "" {
		operatorUserID = ownerUserID
	}
	if err := ensureIdentityVariantAttachmentAccessible(ownerUserID, operatorUserID, input.ChannelID, input.AvatarAttachmentID); err != nil {
		return nil, err
	}
	if err := ensureChannelIdentityVariantKeywordUnique(ownerUserID, input.ChannelID, identity.ID, input.Keyword, input.MatchMode, input.MatchConfig, ""); err != nil {
		return nil, err
	}
	appearance, err := normalizeChannelIdentityVariantAppearance(input)
	if err != nil {
		return nil, err
	}
	if input.TheaterPresentationSet && input.TheaterPresentation == nil {
		delete(appearance, "theaterPresentation")
	}
	appearanceJSON, err := serializeChannelIdentityVariantAppearanceJSON(appearance)
	if err != nil {
		return nil, err
	}
	sortMax, err := model.ChannelIdentityVariantMaxSort(input.ChannelID, ownerUserID, identity.ID)
	if err != nil {
		return nil, err
	}
	item := &model.ChannelIdentityVariantModel{
		IdentityID:         identity.ID,
		ChannelID:          input.ChannelID,
		UserID:             ownerUserID,
		SelectorEmoji:      input.SelectorEmoji,
		Keyword:            input.Keyword,
		MatchMode:          input.MatchMode,
		MatchConfig:        input.MatchConfig,
		Note:               input.Note,
		AvatarAttachmentID: input.AvatarAttachmentID,
		DisplayName:        input.DisplayName,
		Color:              input.Color,
		AppearanceJSON:     appearanceJSON,
		SortOrder:          sortMax + 1,
		Enabled:            input.Enabled,
	}
	if !input.Enabled {
		item.Enabled = false
	} else {
		item.Enabled = true
	}
	if identity.SharedIdentityID != "" {
		if err := createSharedChannelIdentityVariant(identity, item); err != nil {
			return nil, err
		}
	} else if err := model.ChannelIdentityVariantUpsert(item); err != nil {
		return nil, err
	}
	if err := reconcileTheaterAppearanceAssetOrphans(context.Background(), theaterPresentationPatchAssetIDs(input.TheaterPresentation)); err != nil {
		log.Printf("频道角色差分创建后资源清理失败[identity=%s]: %v", identity.ID, err)
	}
	return item, nil
}

func ChannelIdentityVariantGetForUser(userID string, channelID string, variantID string) (*model.ChannelIdentityVariantModel, error) {
	item, err := model.ChannelIdentityVariantGetByID(strings.TrimSpace(variantID))
	if err != nil {
		return nil, err
	}
	if item.UserID != strings.TrimSpace(userID) || item.ChannelID != strings.TrimSpace(channelID) {
		return nil, errors.New("差分不属于该用户或频道")
	}
	return item, nil
}

func ChannelIdentityVariantUpdate(userID string, variantID string, input *ChannelIdentityVariantInput) (*model.ChannelIdentityVariantModel, error) {
	return ChannelIdentityVariantUpdateWithAccess(userID, userID, variantID, input)
}

func ChannelIdentityVariantUpdateWithAccess(ownerUserID string, operatorUserID string, variantID string, input *ChannelIdentityVariantInput) (*model.ChannelIdentityVariantModel, error) {
	if input == nil {
		return nil, errors.New("参数不能为空")
	}
	if err := validateChannelIdentityVariantInput(input); err != nil {
		return nil, err
	}
	item, err := ChannelIdentityVariantGetForUser(ownerUserID, input.ChannelID, variantID)
	if err != nil {
		return nil, err
	}
	affectedAssetIDs := []string{}
	if patch, exists := variantTheaterPresentationPatch(item); exists {
		affectedAssetIDs = append(affectedAssetIDs, theaterPresentationPatchAssetIDs(patch)...)
	}
	if item.SharedVariantID != "" {
		copies, copiesErr := model.SharedChannelIdentityVariantCopies(item.SharedVariantID)
		if copiesErr != nil {
			return nil, copiesErr
		}
		for _, copy := range copies {
			if patch, exists := variantTheaterPresentationPatch(copy); exists {
				affectedAssetIDs = append(affectedAssetIDs, theaterPresentationPatchAssetIDs(patch)...)
			}
		}
	}
	identity, err := ensureChannelIdentityVariantOwnership(ownerUserID, input.ChannelID, input.IdentityID)
	if err != nil {
		return nil, err
	}
	if item.IdentityID != input.IdentityID {
		return nil, errors.New("不能将差分移动到其他身份")
	}
	if strings.TrimSpace(operatorUserID) == "" {
		operatorUserID = ownerUserID
	}
	if err := ensureIdentityVariantAttachmentAccessible(ownerUserID, operatorUserID, input.ChannelID, input.AvatarAttachmentID); err != nil {
		return nil, err
	}
	if input.TheaterPresentation != nil && !input.SkipTheaterAssetValidation && item.SharedVariantID == "" {
		if err := ValidateTheaterPresentationPatchAppearanceAssetsForVariant(model.GetDB(), input.ChannelID, ownerUserID, identity.ID, item.ID, *input.TheaterPresentation); err != nil {
			return nil, err
		}
	}
	if err := ensureChannelIdentityVariantKeywordUnique(ownerUserID, input.ChannelID, input.IdentityID, input.Keyword, input.MatchMode, input.MatchConfig, item.ID); err != nil {
		return nil, err
	}
	appearance, err := normalizeChannelIdentityVariantAppearance(input)
	if err != nil {
		return nil, err
	}
	if !input.TheaterPresentationSet && input.TheaterPresentation == nil {
		if patch, exists := variantTheaterPresentationPatch(item); exists {
			appearance["theaterPresentation"] = patch
		}
	}
	appearanceJSON, err := serializeChannelIdentityVariantAppearanceJSON(appearance)
	if err != nil {
		return nil, err
	}
	values := map[string]any{
		"selector_emoji": input.SelectorEmoji, "keyword": input.Keyword, "match_mode": input.MatchMode, "match_config": input.MatchConfig, "note": input.Note,
		"avatar_attachment_id": input.AvatarAttachmentID, "display_name": input.DisplayName, "color": input.Color,
		"appearance_json": appearanceJSON, "enabled": input.Enabled,
	}
	var updated *model.ChannelIdentityVariantModel
	if item.SharedVariantID != "" && identity.SharedIdentityID != "" {
		source := *item
		source.SelectorEmoji = input.SelectorEmoji
		source.Keyword = input.Keyword
		source.MatchMode = input.MatchMode
		source.MatchConfig = input.MatchConfig
		source.Note = input.Note
		source.AvatarAttachmentID = input.AvatarAttachmentID
		source.DisplayName = input.DisplayName
		source.Color = input.Color
		source.AppearanceJSON = appearanceJSON
		source.Enabled = input.Enabled
		updated, err = syncSharedChannelIdentityVariant(identity, &source, input.ExpectedRevision)
		if err != nil {
			return nil, err
		}
	} else {
		if err := model.ChannelIdentityVariantUpdate(item.ID, values); err != nil {
			return nil, err
		}
		updated, err = model.ChannelIdentityVariantGetByID(item.ID)
		if err != nil {
			return nil, err
		}
	}
	if patch, exists := variantTheaterPresentationPatch(updated); exists {
		affectedAssetIDs = append(affectedAssetIDs, theaterPresentationPatchAssetIDs(patch)...)
	}
	if updated.SharedVariantID != "" {
		copies, copiesErr := model.SharedChannelIdentityVariantCopies(updated.SharedVariantID)
		if copiesErr != nil {
			return nil, copiesErr
		}
		for _, copy := range copies {
			if patch, exists := variantTheaterPresentationPatch(copy); exists {
				affectedAssetIDs = append(affectedAssetIDs, theaterPresentationPatchAssetIDs(patch)...)
			}
		}
	}
	if err := reconcileTheaterAppearanceAssetOrphans(context.Background(), affectedAssetIDs); err != nil {
		log.Printf("频道角色差分更新后资源清理失败[variant=%s]: %v", updated.ID, err)
	}
	return updated, nil
}

func ChannelIdentityVariantDelete(userID string, channelID string, variantID string) error {
	return ChannelIdentityVariantDeleteWithAccess(userID, userID, channelID, variantID)
}

func ChannelIdentityVariantDeleteWithAccess(ownerUserID string, operatorUserID string, channelID string, variantID string) error {
	item, err := ChannelIdentityVariantGetForUser(ownerUserID, channelID, variantID)
	if err != nil {
		return err
	}
	affectedAssetIDs := []string{}
	if patch, exists := variantTheaterPresentationPatch(item); exists {
		affectedAssetIDs = append(affectedAssetIDs, theaterPresentationPatchAssetIDs(patch)...)
	}
	if item.SharedVariantID != "" {
		copies, copiesErr := model.SharedChannelIdentityVariantCopies(item.SharedVariantID)
		if copiesErr != nil {
			return copiesErr
		}
		for _, copy := range copies {
			if patch, exists := variantTheaterPresentationPatch(copy); exists {
				affectedAssetIDs = append(affectedAssetIDs, theaterPresentationPatchAssetIDs(patch)...)
			}
		}
		err = deleteSharedChannelIdentityVariant(item.SharedVariantID)
	} else {
		err = model.ChannelIdentityVariantDelete(item.ID)
	}
	if err != nil {
		return err
	}
	return reconcileTheaterAppearanceAssetOrphans(context.Background(), affectedAssetIDs)
}

func ChannelIdentityVariantReorder(userID string, channelID string, identityID string, ids []string) error {
	return ChannelIdentityVariantReorderWithAccess(userID, userID, channelID, identityID, ids)
}

func ChannelIdentityVariantReorderWithAccess(ownerUserID string, operatorUserID string, channelID string, identityID string, ids []string) error {
	channelID = strings.TrimSpace(channelID)
	identityID = strings.TrimSpace(identityID)
	if channelID == "" || identityID == "" {
		return errors.New("缺少频道ID或身份ID")
	}
	identity, err := ensureChannelIdentityVariantOwnership(ownerUserID, channelID, identityID)
	if err != nil {
		return err
	}
	items, err := model.ChannelIdentityVariantListByIdentityID(channelID, ownerUserID, identityID)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	indexMap := make(map[string]int, len(ids))
	for index, id := range ids {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		indexMap[trimmed] = index
	}
	nextSort := 1
	if identity.SharedIdentityID != "" {
		byID := make(map[string]*model.ChannelIdentityVariantModel, len(items))
		for _, item := range items {
			byID[item.ID] = item
		}
		ordered := make([]*model.ChannelIdentityVariantModel, 0, len(items))
		for _, id := range ids {
			if item := byID[strings.TrimSpace(id)]; item != nil {
				ordered = append(ordered, item)
			}
		}
		for _, item := range items {
			if _, ok := indexMap[item.ID]; !ok {
				ordered = append(ordered, item)
			}
		}
		return reorderSharedChannelIdentityVariants(ordered)
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		for _, id := range ids {
			trimmed := strings.TrimSpace(id)
			if trimmed == "" {
				continue
			}
			if err := tx.Model(&model.ChannelIdentityVariantModel{}).
				Where("id = ? AND channel_id = ? AND user_id = ? AND identity_id = ?", trimmed, channelID, ownerUserID, identityID).
				Update("sort_order", nextSort).Error; err != nil {
				return err
			}
			nextSort++
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			if _, ok := indexMap[item.ID]; ok {
				continue
			}
			if err := tx.Model(&model.ChannelIdentityVariantModel{}).
				Where("id = ?", item.ID).
				Update("sort_order", nextSort).Error; err != nil {
				return err
			}
			nextSort++
		}
		return nil
	})
}

func ResolveChannelIdentityAppearance(identity *model.ChannelIdentityModel, variant *model.ChannelIdentityVariantModel) *ResolvedIdentityAppearance {
	if identity == nil {
		return nil
	}
	result := &ResolvedIdentityAppearance{
		IdentityID:          identity.ID,
		DisplayName:         identity.DisplayName,
		Color:               identity.Color,
		AvatarAttachmentID:  identity.AvatarAttachmentID,
		AvatarDecorations:   identity.AvatarDecorations,
		TheaterPresentation: cloneTheaterPresentation(identity.TheaterPresentation),
	}
	if variant == nil {
		return result
	}
	if patch, exists := variantTheaterPresentationPatch(variant); exists {
		base := protocol.DefaultTheaterPresentation()
		if identity.TheaterPresentation != nil {
			base = *identity.TheaterPresentation
		}
		resolved := protocol.ResolveTheaterPresentation(base, patch)
		result.TheaterPresentation = &resolved
	}
	result.VariantID = variant.ID
	if value := strings.TrimSpace(variant.DisplayName); value != "" {
		result.DisplayName = value
	}
	if value := strings.TrimSpace(variant.Color); value != "" {
		result.Color = value
	}
	if value := strings.TrimSpace(variant.AvatarAttachmentID); value != "" {
		result.AvatarAttachmentID = value
	}
	return result
}

func matchChannelIdentityVariantPrefix(content string, symbol string, keyword string) (string, bool) {
	activation := symbol + keyword
	if len(content) < len(activation) || !strings.EqualFold(content[:len(activation)], activation) {
		return content, false
	}
	rest := content[len(activation):]
	if rest != "" {
		next, _ := utf8.DecodeRuneInString(rest)
		if !unicode.IsSpace(next) {
			return content, false
		}
	}
	return strings.TrimLeftFunc(rest, unicode.IsSpace), true
}

// ChannelIdentityVariantMatchMessage 按 BOT 消息内容匹配差分；前缀规则会移除匹配头。
func ChannelIdentityVariantMatchMessage(identity *model.ChannelIdentityModel, content string) (*model.ChannelIdentityVariantModel, string, error) {
	if identity == nil || strings.TrimSpace(content) == "" {
		return nil, content, nil
	}
	items, err := model.ChannelIdentityVariantListByIdentityID(identity.ChannelID, identity.UserID, identity.ID)
	if err != nil {
		return nil, content, err
	}
	plainContent := NormalizeMessageContentToPlainText(content)
	if plainContent == "" {
		plainContent = content
	}
	lowerContent := strings.ToLower(plainContent)
	for _, targetMode := range []string{
		ChannelIdentityVariantMatchModePrefix,
		ChannelIdentityVariantMatchModeKeyword,
		ChannelIdentityVariantMatchModeRegex,
	} {
		for _, item := range items {
			if item == nil || !item.Enabled {
				continue
			}
			mode := strings.ToLower(strings.TrimSpace(item.MatchMode))
			if mode == "" {
				mode = ChannelIdentityVariantMatchModePrefix
			}
			if mode != targetMode {
				continue
			}
			keyword := strings.TrimSpace(item.Keyword)
			if keyword == "" {
				continue
			}
			switch mode {
			case ChannelIdentityVariantMatchModePrefix:
				symbol := strings.TrimSpace(item.MatchConfig)
				if symbol == "" {
					symbol = "="
				}
				if rest, matched := matchChannelIdentityVariantPrefix(content, symbol, keyword); matched {
					return item, rest, nil
				}
				if content != plainContent {
					if _, matched := matchChannelIdentityVariantPrefix(plainContent, symbol, keyword); matched {
						return item, content, nil
					}
				}
			case ChannelIdentityVariantMatchModeKeyword:
				matchAll := strings.EqualFold(strings.TrimSpace(item.MatchConfig), "all")
				separator := "|"
				if matchAll {
					separator = "&"
				}
				parts := strings.Split(keyword, separator)
				matched := matchAll
				hasKeyword := false
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part == "" {
						continue
					}
					hasKeyword = true
					partMatched := strings.Contains(lowerContent, strings.ToLower(part))
					if matchAll && !partMatched {
						matched = false
						break
					}
					if !matchAll && partMatched {
						matched = true
						break
					}
				}
				if hasKeyword && matched {
					return item, content, nil
				}
			case ChannelIdentityVariantMatchModeRegex:
				pattern := keyword
				if strings.EqualFold(strings.TrimSpace(item.MatchConfig), "insensitive") {
					pattern = "(?i:" + pattern + ")"
				}
				compiled, compileErr := regexp.Compile(pattern)
				if compileErr == nil && compiled.MatchString(plainContent) {
					return item, content, nil
				}
			}
		}
	}
	return nil, content, nil
}

func ChannelIdentityVariantValidateMessageVariant(userID string, channelID string, identity *model.ChannelIdentityModel, variantID string) (*model.ChannelIdentityVariantModel, error) {
	variantID = strings.TrimSpace(variantID)
	if variantID == "" {
		return nil, nil
	}
	if identity == nil {
		return nil, errors.New("未选择身份时不能指定差分")
	}
	item, err := ChannelIdentityVariantGetForUser(userID, channelID, variantID)
	if err != nil {
		return nil, err
	}
	if item.IdentityID != identity.ID {
		return nil, errors.New("差分不属于当前身份")
	}
	if !item.Enabled {
		return nil, errors.New("差分已被禁用")
	}
	return item, nil
}
