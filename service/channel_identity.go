package service

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/protocol"
)

type ChannelIdentityInput struct {
	ChannelID          string
	DisplayName        string
	Color              string
	AvatarAttachmentID string
	AvatarDecorations  protocol.AvatarDecorationList
	IsDefault          bool
	IsTemporary        bool
	ICOOCOnActivate    string
	FolderIDs          []string
}

type ChannelIdentityReplaceResult struct {
	Item          *model.ChannelIdentityModel
	OldIdentityID string
	RemovedID     string
}

const temporaryIdentityActivateModePrefPrefix = "tmpMode:"

func normalizeTemporaryIdentityActivateMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ic":
		return "ic"
	case "ooc":
		return "ooc"
	default:
		return ""
	}
}

func temporaryIdentityActivateModePrefKey(identityID string) string {
	identityID = strings.TrimSpace(identityID)
	if identityID == "" {
		return ""
	}
	return temporaryIdentityActivateModePrefPrefix + identityID
}

func syncTemporaryIdentityActivateMode(userID, identityID, mode string) error {
	return syncTemporaryIdentityActivateModeTx(model.GetDB(), userID, identityID, mode)
}

func syncTemporaryIdentityActivateModeTx(conn *gorm.DB, userID, identityID, mode string) error {
	key := temporaryIdentityActivateModePrefKey(identityID)
	if key == "" {
		return nil
	}
	normalizedMode := normalizeTemporaryIdentityActivateMode(mode)
	if normalizedMode == "" {
		return model.UserPreferenceDeleteTx(conn, userID, key)
	}
	_, err := model.UserPreferenceUpsertTx(conn, userID, key, normalizedMode)
	return err
}

func loadTemporaryIdentityActivateModeMap(userID string) (map[string]string, error) {
	items, err := model.UserPreferenceListByPrefix(userID, temporaryIdentityActivateModePrefPrefix)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(items))
	for _, item := range items {
		key := strings.TrimSpace(item.PrefKey)
		if !strings.HasPrefix(key, temporaryIdentityActivateModePrefPrefix) {
			continue
		}
		identityID := strings.TrimSpace(strings.TrimPrefix(key, temporaryIdentityActivateModePrefPrefix))
		if identityID == "" {
			continue
		}
		mode := normalizeTemporaryIdentityActivateMode(item.PrefValue)
		if mode == "" {
			continue
		}
		result[identityID] = mode
	}
	return result, nil
}

func ApplyTemporaryIdentityActivateModes(userID string, identities []*model.ChannelIdentityModel) error {
	if len(identities) == 0 {
		return nil
	}
	modeMap, err := loadTemporaryIdentityActivateModeMap(userID)
	if err != nil {
		return err
	}
	for _, identity := range identities {
		if identity == nil || !identity.IsTemporary {
			continue
		}
		identity.ICOOCOnActivate = modeMap[identity.ID]
	}
	return nil
}

func validateIdentityInput(input *ChannelIdentityInput) error {
	if strings.TrimSpace(input.DisplayName) == "" {
		return errors.New("频道昵称不能为空")
	}
	if len([]rune(input.DisplayName)) > 32 {
		return errors.New("频道昵称长度需在32个字符以内")
	}
	if input.Color != "" {
		color := model.ChannelIdentityNormalizeColor(input.Color)
		if color == "" {
			return errors.New("昵称颜色格式不正确")
		}
		input.Color = color
	}
	if len(input.FolderIDs) > 20 {
		return errors.New("文件夹数量过多")
	}
	input.ICOOCOnActivate = normalizeTemporaryIdentityActivateMode(input.ICOOCOnActivate)
	return nil
}

func ensureAttachmentOwnership(userID string, attachmentID string) error {
	if attachmentID == "" {
		return nil
	}
	_, err := ResolveAttachmentOwnership(userID, attachmentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("头像附件不存在")
		}
		return err
	}
	return nil
}

func ChannelIdentityCreate(userID string, input *ChannelIdentityInput) (*model.ChannelIdentityModel, error) {
	if err := validateIdentityInput(input); err != nil {
		return nil, err
	}

	member, err := model.MemberGetByUserIDAndChannelIDBase(userID, input.ChannelID, "", false)
	if err != nil {
		return nil, err
	}
	if member == nil {
		return nil, errors.New("仅频道成员可创建频道身份")
	}

	if err := ensureAttachmentOwnership(userID, input.AvatarAttachmentID); err != nil {
		return nil, err
	}
	avatarDecorations, err := NormalizeAvatarDecorations(userID, input.AvatarDecorations)
	if err != nil {
		return nil, err
	}

	sortMax, err := model.ChannelIdentityMaxSort(input.ChannelID, userID)
	if err != nil {
		return nil, err
	}

	item := &model.ChannelIdentityModel{
		ChannelID:          input.ChannelID,
		UserID:             userID,
		DisplayName:        strings.TrimSpace(input.DisplayName),
		Color:              input.Color,
		AvatarAttachmentID: input.AvatarAttachmentID,
		AvatarDecorations:  avatarDecorations,
		SortOrder:          sortMax + 1,
		IsDefault:          input.IsDefault,
		IsTemporary:        input.IsTemporary,
	}
	if item.IsDefault {
		if err := model.ChannelIdentityEnsureSingleDefault(item.ChannelID, item.UserID, ""); err != nil {
			return nil, err
		}
	}
	if err := model.ChannelIdentityUpsert(item); err != nil {
		return nil, err
	}

	folderIDs := input.FolderIDs
	if folderIDs == nil {
		folderIDs = []string{}
	}
	membership, err := ChannelIdentityFolderAssign(userID, input.ChannelID, []string{item.ID}, folderIDs, "replace")
	if err != nil {
		return nil, err
	}
	item.FolderIDs = membership[item.ID]
	item.ICOOCOnActivate = input.ICOOCOnActivate
	if item.IsTemporary {
		if err := syncTemporaryIdentityActivateMode(userID, item.ID, input.ICOOCOnActivate); err != nil {
			return nil, err
		}
	}

	// 如果当前无默认身份，则自动设置为默认
	if !item.IsDefault {
		if _, err := model.ChannelIdentityFindDefault(item.ChannelID, item.UserID); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				item.IsDefault = true
				if errUpdate := model.ChannelIdentityUpdate(item.ID, map[string]any{"is_default": true}); errUpdate == nil {
					item.IsDefault = true
				}
			} else {
				return nil, err
			}
		}
	}

	return item, nil
}

func ChannelIdentityUpdate(userID string, identityID string, input *ChannelIdentityInput) (*model.ChannelIdentityModel, error) {
	if err := validateIdentityInput(input); err != nil {
		return nil, err
	}
	identity, err := model.ChannelIdentityValidateOwnership(identityID, userID, input.ChannelID)
	if err != nil {
		return nil, err
	}
	if err := ensureAttachmentOwnership(userID, input.AvatarAttachmentID); err != nil {
		return nil, err
	}
	avatarDecorations, err := NormalizeAvatarDecorations(userID, input.AvatarDecorations)
	if err != nil {
		return nil, err
	}

	values := map[string]any{
		"display_name":         strings.TrimSpace(input.DisplayName),
		"color":                input.Color,
		"avatar_attachment_id": input.AvatarAttachmentID,
		"avatar_decoration":    avatarDecorations,
		"is_default":           input.IsDefault,
	}

	if err := model.ChannelIdentityUpdate(identity.ID, values); err != nil {
		return nil, err
	}

	if input.IsDefault {
		if err := model.ChannelIdentityEnsureSingleDefault(identity.ChannelID, identity.UserID, identity.ID); err != nil {
			return nil, err
		}
	} else if identity.IsDefault && !input.IsDefault {
		if err := ChannelIdentityEnsureDefault(identity.ChannelID, identity.UserID); err != nil {
			return nil, err
		}
	}

	updated, err := model.ChannelIdentityGetByID(identity.ID)
	if err != nil {
		return nil, err
	}
	updated.ICOOCOnActivate = ""
	if input.FolderIDs != nil {
		membership, err := ChannelIdentityFolderAssign(userID, input.ChannelID, []string{identity.ID}, input.FolderIDs, "replace")
		if err != nil {
			return nil, err
		}
		updated.FolderIDs = membership[identity.ID]
	} else {
		membership, err := loadIdentityFolderMembership([]string{identity.ID})
		if err == nil {
			updated.FolderIDs = membership[identity.ID]
		}
	}
	if updated.IsTemporary {
		updated.ICOOCOnActivate = input.ICOOCOnActivate
		if err := syncTemporaryIdentityActivateMode(userID, updated.ID, input.ICOOCOnActivate); err != nil {
			return nil, err
		}
	} else {
		if err := syncTemporaryIdentityActivateMode(userID, updated.ID, ""); err != nil {
			return nil, err
		}
	}
	return updated, nil
}

func ChannelIdentityReplaceTemporary(userID string, identityID string, input *ChannelIdentityInput) (*ChannelIdentityReplaceResult, error) {
	if err := validateIdentityInput(input); err != nil {
		return nil, err
	}
	identity, err := model.ChannelIdentityValidateOwnership(identityID, userID, input.ChannelID)
	if err != nil {
		return nil, err
	}
	if !identity.IsTemporary {
		return nil, errors.New("仅临时身份支持替换式编辑")
	}
	if err := ensureAttachmentOwnership(userID, input.AvatarAttachmentID); err != nil {
		return nil, err
	}
	avatarDecorations, err := NormalizeAvatarDecorations(userID, input.AvatarDecorations)
	if err != nil {
		return nil, err
	}

	folderIDs := sanitizeFolderIDs(input.FolderIDs)
	if input.FolderIDs == nil {
		membership, err := loadIdentityFolderMembership([]string{identity.ID})
		if err != nil {
			return nil, err
		}
		folderIDs = sanitizeFolderIDs(membership[identity.ID])
	}
	if _, err := ChannelIdentityFoldersValidateOwnership(input.ChannelID, userID, folderIDs); err != nil {
		return nil, err
	}

	result := &ChannelIdentityReplaceResult{
		OldIdentityID: identity.ID,
		RemovedID:     identity.ID,
	}
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		item := &model.ChannelIdentityModel{
			ChannelID:          identity.ChannelID,
			UserID:             identity.UserID,
			DisplayName:        strings.TrimSpace(input.DisplayName),
			Color:              input.Color,
			AvatarAttachmentID: input.AvatarAttachmentID,
			AvatarDecorations:  avatarDecorations,
			IsDefault:          input.IsDefault,
			IsTemporary:        true,
			SortOrder:          identity.SortOrder,
		}
		if err := tx.Create(item).Error; err != nil {
			return err
		}
		if item.IsDefault {
			if err := tx.Model(&model.ChannelIdentityModel{}).
				Where("channel_id = ? AND user_id = ? AND id <> ?", item.ChannelID, item.UserID, item.ID).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		if len(folderIDs) > 0 {
			records := make([]*model.ChannelIdentityFolderMemberModel, 0, len(folderIDs))
			for idx, folderID := range folderIDs {
				records = append(records, &model.ChannelIdentityFolderMemberModel{
					ChannelID:  item.ChannelID,
					UserID:     item.UserID,
					FolderID:   folderID,
					IdentityID: item.ID,
					SortOrder:  idx,
				})
			}
			if err := tx.Create(&records).Error; err != nil {
				return err
			}
		}

		var config model.ChannelIdentityModeConfigModel
		if err := tx.Where("user_id = ? AND channel_id = ?", identity.UserID, identity.ChannelID).
			Limit(1).
			Find(&config).Error; err != nil {
			return err
		}
		configExists := config.ID != ""
		if configExists {
			nextICIdentityID := config.ICIdentityID
			nextOOCIdentityID := config.OOCIdentityID
			changed := false
			if nextICIdentityID == identity.ID {
				nextICIdentityID = item.ID
				changed = true
			}
			if nextOOCIdentityID == identity.ID {
				nextOOCIdentityID = item.ID
				changed = true
			}
			if changed {
				if _, err := model.ChannelIdentityModeConfigUpsertTx(tx, identity.UserID, identity.ChannelID, nextICIdentityID, nextOOCIdentityID); err != nil {
					return err
				}
			}
		}

		if err := syncTemporaryIdentityActivateModeTx(tx, identity.UserID, item.ID, input.ICOOCOnActivate); err != nil {
			return err
		}
		if err := syncTemporaryIdentityActivateModeTx(tx, identity.UserID, identity.ID, ""); err != nil {
			return err
		}

		if err := tx.Where("identity_id = ?", identity.ID).Delete(&model.ChannelIdentityVariantModel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("identity_id = ?", identity.ID).Delete(&model.ChannelIdentityFolderMemberModel{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", identity.ID).Delete(&model.ChannelIdentityModel{}).Error; err != nil {
			return err
		}
		if !item.IsDefault {
			var hasDefault int64
			if err := tx.Model(&model.ChannelIdentityModel{}).
				Where("channel_id = ? AND user_id = ? AND is_default = ?", item.ChannelID, item.UserID, true).
				Count(&hasDefault).Error; err != nil {
				return err
			}
			if hasDefault == 0 {
				var fallback model.ChannelIdentityModel
				if err := tx.Where("channel_id = ? AND user_id = ?", item.ChannelID, item.UserID).
					Order("sort_order ASC, created_at ASC").
					Limit(1).
					Find(&fallback).Error; err != nil {
					return err
				}
				if fallback.ID != "" {
					if err := tx.Model(&model.ChannelIdentityModel{}).
						Where("id = ?", fallback.ID).
						Update("is_default", true).Error; err != nil {
						return err
					}
					if fallback.ID == item.ID {
						item.IsDefault = true
					}
				}
			}
		}
		item.FolderIDs = append([]string{}, folderIDs...)
		item.ICOOCOnActivate = input.ICOOCOnActivate
		result.Item = item
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func ChannelIdentityDelete(userID string, channelID string, identityID string) error {
	identity, err := model.ChannelIdentityValidateOwnership(identityID, userID, channelID)
	if err != nil {
		return err
	}
	if err := model.ChannelIdentityVariantDeleteByIdentityIDs([]string{identity.ID}); err != nil {
		return err
	}
	if err := model.ChannelIdentityDelete(identity.ID); err != nil {
		return err
	}
	if err := model.ChannelIdentityModeConfigClearIdentityReferences(userID, channelID, identity.ID); err != nil {
		return err
	}
	if err := syncTemporaryIdentityActivateMode(userID, identity.ID, ""); err != nil {
		return err
	}
	_ = model.ChannelIdentityFolderMemberDeleteByIdentityIDs([]string{identity.ID})

	if identity.IsDefault {
		// 重新指定一个默认身份
		items, err := model.ChannelIdentityList(channelID, userID)
		if err != nil {
			return err
		}
		if len(items) > 0 {
			if err := model.ChannelIdentityUpdate(items[0].ID, map[string]any{"is_default": true}); err != nil {
				return err
			}
		}
	}
	return nil
}

func ChannelIdentityResolve(userID string, channelID string, identityID string) (*model.ChannelIdentityModel, error) {
	if identityID == "" {
		identity, err := model.ChannelIdentityFindDefault(channelID, userID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return identity, nil
	}
	identity, err := model.ChannelIdentityValidateOwnership(identityID, userID, channelID)
	if err != nil {
		return nil, err
	}
	return identity, nil
}

func ChannelIdentityEnsureDefault(channelID string, userID string) error {
	_, err := model.ChannelIdentityFindDefault(channelID, userID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	items, err := model.ChannelIdentityList(channelID, userID)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	return model.ChannelIdentityUpdate(items[0].ID, map[string]any{"is_default": true})
}

func ChannelIdentitySerialize(item *model.ChannelIdentityModel) map[string]any {
	if item == nil {
		return nil
	}
	return map[string]any{
		"id":                 item.ID,
		"channelId":          item.ChannelID,
		"userId":             item.UserID,
		"displayName":        item.DisplayName,
		"color":              item.Color,
		"avatarAttachmentId": item.AvatarAttachmentID,
		"avatarDecorations":  item.AvatarDecorations,
		"isDefault":          item.IsDefault,
		"isTemporary":        item.IsTemporary,
		"icOocOnActivate":    item.ICOOCOnActivate,
		"sortOrder":          item.SortOrder,
		"folderIds":          item.FolderIDs,
	}
}

func ChannelIdentityValidateMessageIdentity(userID string, channelID string, identityID string) (*model.ChannelIdentityModel, error) {
	identity, err := ChannelIdentityResolve(userID, channelID, identityID)
	if err != nil {
		return nil, err
	}
	if identity == nil {
		return nil, nil
	}

	member, err := model.MemberGetByUserIDAndChannelIDBase(userID, channelID, "", false)
	if err != nil {
		return nil, err
	}
	if member == nil {
		return nil, fmt.Errorf("用户不在频道内")
	}
	return identity, nil
}

// EnsureHiddenDefaultIdentity 确保用户在频道内有一个隐形默认身份
// 如果不存在则根据用户主页信息自动创建
func EnsureHiddenDefaultIdentity(userID string, channelID string) (*model.ChannelIdentityModel, error) {
	userID = strings.TrimSpace(userID)
	channelID = strings.TrimSpace(channelID)
	if userID == "" || channelID == "" {
		return nil, nil
	}

	// 检查是否已存在隐形身份
	identity, err := model.ChannelIdentityFindHidden(channelID, userID)
	if err == nil && identity != nil {
		return identity, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// 获取用户信息
	user := model.UserGet(userID)
	if user == nil {
		return nil, fmt.Errorf("用户不存在")
	}

	// 创建隐形默认身份
	displayName := strings.TrimSpace(user.Nickname)
	if displayName == "" {
		displayName = strings.TrimSpace(user.Username)
	}
	if displayName == "" {
		displayName = "未知用户"
	}

	item := &model.ChannelIdentityModel{
		ChannelID:          channelID,
		UserID:             userID,
		DisplayName:        displayName,
		Color:              model.ChannelIdentityNormalizeColor(user.NickColor),
		AvatarAttachmentID: user.Avatar,
		IsDefault:          false,
		IsHidden:           true,
		SortOrder:          0,
	}

	if err := model.ChannelIdentityUpsert(item); err != nil {
		return nil, err
	}

	return item, nil
}
