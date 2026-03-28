package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/protocol"
)

var (
	channelIdentityVariantKeywordPattern = regexp.MustCompile(`^[\p{L}\p{N}_-]{1,64}$`)
)

type ChannelIdentityVariantInput struct {
	ChannelID          string
	IdentityID         string
	SelectorEmoji      string
	Keyword            string
	Note               string
	AvatarAttachmentID string
	DisplayName        string
	Color              string
	Appearance         map[string]any
	Enabled            bool
}

type ResolvedIdentityAppearance struct {
	IdentityID         string
	VariantID          string
	DisplayName        string
	Color              string
	AvatarAttachmentID string
	AvatarDecoration   *protocol.AvatarDecoration
}

func normalizeChannelIdentityVariantKeyword(keyword string) string {
	return strings.TrimSpace(keyword)
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
		if trimmedKey == "" {
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
	return result, nil
}

func validateChannelIdentityVariantInput(input *ChannelIdentityVariantInput) error {
	input.ChannelID = strings.TrimSpace(input.ChannelID)
	input.IdentityID = strings.TrimSpace(input.IdentityID)
	input.Keyword = normalizeChannelIdentityVariantKeyword(input.Keyword)
	input.SelectorEmoji = normalizeChannelIdentityVariantEmoji(input.SelectorEmoji)
	input.Note = normalizeChannelIdentityVariantNote(input.Note)

	if input.ChannelID == "" {
		return errors.New("缺少频道ID")
	}
	if input.IdentityID == "" {
		return errors.New("缺少身份ID")
	}
	if input.Keyword == "" {
		return errors.New("差分快捷关键词不能为空")
	}
	if !channelIdentityVariantKeywordPattern.MatchString(input.Keyword) {
		return errors.New("差分快捷关键词仅支持字母、数字、下划线和短横线，长度不超过64")
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

func ensureIdentityVariantAttachmentOwnership(userID string, attachmentID string) error {
	if attachmentID == "" {
		return nil
	}
	_, err := ResolveAttachmentOwnership(userID, attachmentID)
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

func ensureChannelIdentityVariantKeywordUnique(userID string, channelID string, identityID string, keyword string, excludeID string) error {
	items, err := model.ChannelIdentityVariantListByIdentityID(channelID, userID, identityID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item == nil || item.ID == excludeID {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(item.Keyword), keyword) {
			return fmt.Errorf("差分快捷关键词 %s 已存在", keyword)
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
	if input == nil {
		return nil, errors.New("参数不能为空")
	}
	if err := validateChannelIdentityVariantInput(input); err != nil {
		return nil, err
	}
	identity, err := ensureChannelIdentityVariantOwnership(userID, input.ChannelID, input.IdentityID)
	if err != nil {
		return nil, err
	}
	if err := ensureIdentityVariantAttachmentOwnership(userID, input.AvatarAttachmentID); err != nil {
		return nil, err
	}
	if err := ensureChannelIdentityVariantKeywordUnique(userID, input.ChannelID, identity.ID, input.Keyword, ""); err != nil {
		return nil, err
	}
	appearance, err := normalizeChannelIdentityVariantAppearance(input)
	if err != nil {
		return nil, err
	}
	appearanceJSON, err := serializeChannelIdentityVariantAppearanceJSON(appearance)
	if err != nil {
		return nil, err
	}
	sortMax, err := model.ChannelIdentityVariantMaxSort(input.ChannelID, userID, identity.ID)
	if err != nil {
		return nil, err
	}
	item := &model.ChannelIdentityVariantModel{
		IdentityID:         identity.ID,
		ChannelID:          input.ChannelID,
		UserID:             userID,
		SelectorEmoji:      input.SelectorEmoji,
		Keyword:            input.Keyword,
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
	if err := model.ChannelIdentityVariantUpsert(item); err != nil {
		return nil, err
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
	if input == nil {
		return nil, errors.New("参数不能为空")
	}
	if err := validateChannelIdentityVariantInput(input); err != nil {
		return nil, err
	}
	item, err := ChannelIdentityVariantGetForUser(userID, input.ChannelID, variantID)
	if err != nil {
		return nil, err
	}
	if _, err := ensureChannelIdentityVariantOwnership(userID, input.ChannelID, input.IdentityID); err != nil {
		return nil, err
	}
	if item.IdentityID != input.IdentityID {
		return nil, errors.New("不能将差分移动到其他身份")
	}
	if err := ensureIdentityVariantAttachmentOwnership(userID, input.AvatarAttachmentID); err != nil {
		return nil, err
	}
	if err := ensureChannelIdentityVariantKeywordUnique(userID, input.ChannelID, input.IdentityID, input.Keyword, item.ID); err != nil {
		return nil, err
	}
	appearance, err := normalizeChannelIdentityVariantAppearance(input)
	if err != nil {
		return nil, err
	}
	appearanceJSON, err := serializeChannelIdentityVariantAppearanceJSON(appearance)
	if err != nil {
		return nil, err
	}
	values := map[string]any{
		"selector_emoji":       input.SelectorEmoji,
		"keyword":              input.Keyword,
		"note":                 input.Note,
		"avatar_attachment_id": input.AvatarAttachmentID,
		"display_name":         input.DisplayName,
		"color":                input.Color,
		"appearance_json":      appearanceJSON,
		"enabled":              input.Enabled,
	}
	if err := model.ChannelIdentityVariantUpdate(item.ID, values); err != nil {
		return nil, err
	}
	return model.ChannelIdentityVariantGetByID(item.ID)
}

func ChannelIdentityVariantDelete(userID string, channelID string, variantID string) error {
	item, err := ChannelIdentityVariantGetForUser(userID, channelID, variantID)
	if err != nil {
		return err
	}
	return model.ChannelIdentityVariantDelete(item.ID)
}

func ChannelIdentityVariantReorder(userID string, channelID string, identityID string, ids []string) error {
	channelID = strings.TrimSpace(channelID)
	identityID = strings.TrimSpace(identityID)
	if channelID == "" || identityID == "" {
		return errors.New("缺少频道ID或身份ID")
	}
	if _, err := ensureChannelIdentityVariantOwnership(userID, channelID, identityID); err != nil {
		return err
	}
	items, err := model.ChannelIdentityVariantListByIdentityID(channelID, userID, identityID)
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
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		for _, id := range ids {
			trimmed := strings.TrimSpace(id)
			if trimmed == "" {
				continue
			}
			if err := tx.Model(&model.ChannelIdentityVariantModel{}).
				Where("id = ? AND channel_id = ? AND user_id = ? AND identity_id = ?", trimmed, channelID, userID, identityID).
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
		IdentityID:         identity.ID,
		DisplayName:        identity.DisplayName,
		Color:              identity.Color,
		AvatarAttachmentID: identity.AvatarAttachmentID,
		AvatarDecoration:   identity.AvatarDecoration,
	}
	if variant == nil {
		return result
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
