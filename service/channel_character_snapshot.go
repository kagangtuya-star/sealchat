package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/protocol"
)

const (
	defaultCharacterOverlayTemplate = `{"version":1,"preferredColumns":2,"items":[]}`
	maxCharacterSnapshotBytes       = 1024 * 1024
	maxCharacterOverlayBytes        = 64 * 1024
)

var characterSnapshotColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{3}([0-9a-fA-F]{3})?([0-9a-fA-F]{2})?$`)

type CharacterSnapshotWriteResult struct {
	Item    *protocol.CharacterSnapshotItem `json:"item"`
	Changed bool                            `json:"changed"`
}

type CharacterSnapshotSettingsUpdateInput struct {
	BadgeTemplate              string
	TheaterOverlayTemplateJSON string
}

type CharacterSnapshotPreferenceUpdateInput struct {
	BadgeTemplateMode          string
	BadgeTemplate              string
	TheaterOverlayTemplateMode string
	TheaterOverlayTemplateJSON string
}

func CharacterSnapshotList(channelID, actorID string) ([]*protocol.CharacterSnapshotItem, error) {
	channelID = strings.TrimSpace(channelID)
	if !CanReadChannelByUserId(actorID, channelID) {
		return nil, errors.New("无权查看频道人物卡快照")
	}
	var rows []*model.ChannelCharacterSnapshotModel
	if err := model.GetDB().Where("channel_id = ? AND is_active = ?", channelID, true).
		Order("updated_at desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	identityIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if row != nil && row.IdentityID != "" {
			identityIDs = append(identityIDs, row.IdentityID)
		}
	}
	var identities []model.ChannelIdentityModel
	if len(identityIDs) > 0 {
		if err := model.GetDB().Select("id", "channel_id", "user_id").Where("id IN ?", identityIDs).Find(&identities).Error; err != nil {
			return nil, err
		}
	}
	validIdentityOwners := make(map[string]string, len(identities))
	for i := range identities {
		if identities[i].ChannelID == channelID {
			validIdentityOwners[identities[i].ID] = identities[i].UserID
		}
	}
	settings, preferences, err := loadCharacterSnapshotTemplates(channelID)
	if err != nil {
		return nil, err
	}
	items := make([]*protocol.CharacterSnapshotItem, 0, len(rows))
	invalidIdentityIDs := make([]string, 0)
	for _, row := range rows {
		if row == nil || validIdentityOwners[row.IdentityID] != row.UserID {
			if row != nil && row.IdentityID != "" {
				invalidIdentityIDs = append(invalidIdentityIDs, row.IdentityID)
			}
			continue
		}
		item, err := characterSnapshotModelToProtocol(row)
		if err == nil && item != nil {
			applyEffectiveCharacterSnapshotTemplates(item, settings, preferences[item.UserID])
			items = append(items, item)
		}
	}
	if len(invalidIdentityIDs) > 0 {
		_ = model.GetDB().Model(&model.ChannelCharacterSnapshotModel{}).
			Where("channel_id = ? AND identity_id IN ?", channelID, invalidIdentityIDs).
			Update("is_active", false).Error
	}
	return items, nil
}

func CharacterSnapshotProbeList(channelID string) ([]*protocol.CharacterSnapshotProbeItem, error) {
	var rows []model.ChannelCharacterSnapshotModel
	if err := model.GetDB().Select("user_id", "identity_id", "content_hash", "server_revision").
		Where("channel_id = ? AND is_active = ?", strings.TrimSpace(channelID), true).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	items := make([]*protocol.CharacterSnapshotProbeItem, 0, len(rows))
	for i := range rows {
		items = append(items, &protocol.CharacterSnapshotProbeItem{
			UserID:         rows[i].UserID,
			IdentityID:     rows[i].IdentityID,
			ContentHash:    rows[i].ContentHash,
			ServerRevision: rows[i].ServerRevision,
		})
	}
	return items, nil
}

func CharacterSnapshotUpsert(channelID, identityID, actorID, sourceType, sourceCardID string, sourceUpdatedAt int64, data protocol.CharacterSnapshotData) (*CharacterSnapshotWriteResult, error) {
	channelID = strings.TrimSpace(channelID)
	identityID = strings.TrimSpace(identityID)
	actorID = strings.TrimSpace(actorID)
	if channelID == "" || identityID == "" || actorID == "" {
		return nil, errors.New("缺少频道、身份或用户ID")
	}
	if !CanReadChannelByUserId(actorID, channelID) {
		return nil, errors.New("无权同步频道人物卡快照")
	}
	identity, err := model.ChannelIdentityGetByID(identityID)
	if err != nil {
		return nil, errors.New("频道身份不存在")
	}
	if identity.ChannelID != channelID || identity.UserID != actorID {
		return nil, errors.New("不能同步他人的频道身份")
	}
	data.Identity.ID = identityID
	data.Identity.UserID = actorID
	data.Identity.DisplayName = strings.TrimSpace(data.Identity.DisplayName)
	if data.Identity.DisplayName == "" {
		data.Identity.DisplayName = strings.TrimSpace(identity.DisplayName)
	}
	if utf8.RuneCountInString(data.Identity.DisplayName) > 512 {
		return nil, errors.New("频道身份显示名过长")
	}
	data.Identity.Color = normalizeCharacterSnapshotColor(data.Identity.Color, identity.Color)
	data.Identity.AvatarAttachmentID = strings.TrimSpace(data.Identity.AvatarAttachmentID)
	if data.Identity.AvatarAttachmentID == "" {
		data.Identity.AvatarAttachmentID = strings.TrimSpace(identity.AvatarAttachmentID)
	}
	if utf8.RuneCountInString(data.Identity.AvatarAttachmentID) > 256 {
		return nil, errors.New("频道身份头像引用过长")
	}
	if data.Identity.AvatarDecorations == nil {
		data.Identity.AvatarDecorations = identity.AvatarDecorations
	}
	if data.Card != nil {
		data.Card.Name = strings.TrimSpace(data.Card.Name)
		data.Card.SheetType = strings.TrimSpace(data.Card.SheetType)
		data.Card.AvatarAttachmentID = strings.TrimSpace(data.Card.AvatarAttachmentID)
		if utf8.RuneCountInString(data.Card.Name) > 128 || utf8.RuneCountInString(data.Card.SheetType) > 64 {
			return nil, errors.New("人物卡名称或类型过长")
		}
		if utf8.RuneCountInString(data.Card.AvatarAttachmentID) > 256 {
			return nil, errors.New("人物卡头像引用过长")
		}
		if len(data.Card.TemplateText) > 512*1024 {
			return nil, errors.New("人物卡模板快照过大")
		}
		if data.Card.Attrs == nil {
			data.Card.Attrs = map[string]any{}
		}
	}
	if data.BadgeAttrs == nil {
		data.BadgeAttrs = map[string]any{}
	}
	payloadJSON, err := json.Marshal(data)
	if err != nil {
		return nil, errors.New("人物卡快照无法序列化")
	}
	if len(payloadJSON) > maxCharacterSnapshotBytes {
		return nil, errors.New("人物卡快照不可超过1MB")
	}
	hashBytes := sha256.Sum256(payloadJSON)
	contentHash := hex.EncodeToString(hashBytes[:])
	sourceType = strings.TrimSpace(sourceType)
	if sourceType == "" {
		sourceType = "client"
	}
	if sourceType != "client" && sourceType != "sealdice" && sourceType != "sealchat" {
		return nil, errors.New("人物卡快照来源类型无效")
	}
	if sourceUpdatedAt < 0 {
		sourceUpdatedAt = 0
	}
	sourceCardID = strings.TrimSpace(sourceCardID)
	if utf8.RuneCountInString(sourceCardID) > 100 {
		return nil, errors.New("人物卡来源ID过长")
	}
	now := time.Now().UnixMilli()
	var saved *model.ChannelCharacterSnapshotModel
	changed := false
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		var row model.ChannelCharacterSnapshotModel
		findErr := tx.Where("channel_id = ? AND identity_id = ?", channelID, identityID).First(&row).Error
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		if findErr == nil && row.IsActive && row.ContentHash == contentHash {
			deactivated := tx.Model(&model.ChannelCharacterSnapshotModel{}).
				Where("channel_id = ? AND user_id = ? AND identity_id <> ? AND is_active = ?", channelID, actorID, identityID, true).
				Updates(map[string]any{"is_active": false, "server_revision": now})
			if deactivated.Error != nil {
				return deactivated.Error
			}
			updates := map[string]any{"last_seen_at": now, "source_type": sourceType, "source_card_id": sourceCardID}
			if sourceUpdatedAt > row.SourceUpdatedAt {
				updates["source_updated_at"] = sourceUpdatedAt
			}
			if err := tx.Model(&row).Updates(updates).Error; err != nil {
				return err
			}
			row.LastSeenAt = now
			row.SourceType = sourceType
			row.SourceCardID = sourceCardID
			if sourceUpdatedAt > row.SourceUpdatedAt {
				row.SourceUpdatedAt = sourceUpdatedAt
			}
			saved = &row
			changed = deactivated.RowsAffected > 0
			return nil
		}
		if err := tx.Model(&model.ChannelCharacterSnapshotModel{}).
			Where("channel_id = ? AND user_id = ? AND identity_id <> ? AND is_active = ?", channelID, actorID, identityID, true).
			Updates(map[string]any{"is_active": false, "server_revision": now}).Error; err != nil {
			return err
		}
		revision := nextCharacterSnapshotRevision(row.ServerRevision)
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			row = model.ChannelCharacterSnapshotModel{
				ChannelID:       channelID,
				IdentityID:      identityID,
				UserID:          actorID,
				IsActive:        true,
				SourceType:      sourceType,
				SourceCardID:    sourceCardID,
				PayloadJSON:     string(payloadJSON),
				ContentHash:     contentHash,
				ServerRevision:  revision,
				SourceUpdatedAt: sourceUpdatedAt,
				LastSeenAt:      now,
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&row).Updates(map[string]any{
				"user_id": actorID, "is_active": true, "source_type": sourceType,
				"source_card_id": sourceCardID, "payload_json": string(payloadJSON),
				"content_hash": contentHash, "server_revision": revision,
				"source_updated_at": sourceUpdatedAt, "last_seen_at": now,
			}).Error; err != nil {
				return err
			}
			row.UserID = actorID
			row.IsActive = true
			row.SourceType = sourceType
			row.SourceCardID = sourceCardID
			row.PayloadJSON = string(payloadJSON)
			row.ContentHash = contentHash
			row.ServerRevision = revision
			row.SourceUpdatedAt = sourceUpdatedAt
			row.LastSeenAt = now
		}
		saved = &row
		changed = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	item, err := characterSnapshotModelToProtocol(saved)
	if err != nil {
		return nil, err
	}
	settings, preferences, templateErr := loadCharacterSnapshotTemplates(channelID)
	if templateErr == nil {
		applyEffectiveCharacterSnapshotTemplates(item, settings, preferences[actorID])
	}
	return &CharacterSnapshotWriteResult{Item: item, Changed: changed}, nil
}

func CharacterSnapshotClear(channelID, identityID, actorID string) (*CharacterSnapshotWriteResult, error) {
	channelID = strings.TrimSpace(channelID)
	identityID = strings.TrimSpace(identityID)
	actorID = strings.TrimSpace(actorID)
	if channelID == "" || identityID == "" || actorID == "" || !CanReadChannelByUserId(actorID, channelID) {
		return nil, errors.New("无权清除频道身份快照")
	}
	var row model.ChannelCharacterSnapshotModel
	if err := model.GetDB().Where("channel_id = ? AND identity_id = ? AND user_id = ?", channelID, identityID, actorID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &CharacterSnapshotWriteResult{Changed: false}, nil
		}
		return nil, err
	}
	if !row.IsActive {
		return &CharacterSnapshotWriteResult{Changed: false}, nil
	}
	row.IsActive = false
	row.ServerRevision = nextCharacterSnapshotRevision(row.ServerRevision)
	row.LastSeenAt = time.Now().UnixMilli()
	if err := model.GetDB().Model(&row).Updates(map[string]any{
		"is_active": false, "server_revision": row.ServerRevision, "last_seen_at": row.LastSeenAt,
	}).Error; err != nil {
		return nil, err
	}
	item, _ := characterSnapshotModelToProtocol(&row)
	settings, preferences, templateErr := loadCharacterSnapshotTemplates(channelID)
	if templateErr == nil {
		applyEffectiveCharacterSnapshotTemplates(item, settings, preferences[actorID])
	}
	return &CharacterSnapshotWriteResult{Item: item, Changed: true}, nil
}

func CharacterSnapshotSettingsGet(channelID, actorID string) (*protocol.CharacterSnapshotSettingsPayload, error) {
	if !CanReadChannelByUserId(actorID, channelID) {
		return nil, errors.New("无权查看频道人物卡模板")
	}
	var row model.ChannelCharacterSnapshotSettingsModel
	err := model.GetDB().Where("channel_id = ?", strings.TrimSpace(channelID)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &protocol.CharacterSnapshotSettingsPayload{
			ChannelID: strings.TrimSpace(channelID), TheaterOverlayTemplateJSON: defaultCharacterOverlayTemplate, SchemaVersion: 1,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return characterSnapshotSettingsToProtocol(&row), nil
}

func CharacterSnapshotSettingsUpdate(channelID, actorID string, update CharacterSnapshotSettingsUpdateInput) (*protocol.CharacterSnapshotSettingsPayload, error) {
	channelID = strings.TrimSpace(channelID)
	if err := ensureCharacterSnapshotSettingsPermission(channelID, actorID); err != nil {
		return nil, err
	}
	badgeTemplate := strings.TrimSpace(update.BadgeTemplate)
	if utf8.RuneCountInString(badgeTemplate) > 512 {
		return nil, errors.New("徽章模板长度需在512个字符以内")
	}
	overlayJSON, err := validateCharacterOverlayTemplate(update.TheaterOverlayTemplateJSON)
	if err != nil {
		return nil, err
	}
	var saved model.ChannelCharacterSnapshotSettingsModel
	var revision int64
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		findErr := tx.Where("channel_id = ?", channelID).First(&saved).Error
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		revision = nextCharacterSnapshotRevision(saved.ServerRevision)
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			saved = model.ChannelCharacterSnapshotSettingsModel{ChannelID: channelID, BadgeTemplate: badgeTemplate, TheaterOverlayTemplateJSON: overlayJSON, SchemaVersion: 1, ServerRevision: revision, UpdatedBy: actorID}
			return tx.Create(&saved).Error
		}
		return tx.Model(&saved).Updates(map[string]any{"badge_template": badgeTemplate, "theater_overlay_template_json": overlayJSON, "schema_version": 1, "server_revision": revision, "updated_by": actorID}).Error
	})
	if err != nil {
		return nil, err
	}
	saved.BadgeTemplate = badgeTemplate
	saved.TheaterOverlayTemplateJSON = overlayJSON
	saved.SchemaVersion = 1
	saved.ServerRevision = revision
	saved.UpdatedBy = actorID
	return characterSnapshotSettingsToProtocol(&saved), nil
}

func CharacterSnapshotPreferenceGet(channelID, actorID string) (*protocol.CharacterSnapshotPreferencePayload, error) {
	if !CanReadChannelByUserId(actorID, channelID) {
		return nil, errors.New("无权查看人物卡模板偏好")
	}
	var row model.ChannelCharacterSnapshotPreferenceModel
	err := model.GetDB().Where("channel_id = ? AND user_id = ?", strings.TrimSpace(channelID), actorID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return defaultCharacterSnapshotPreference(channelID, actorID), nil
	}
	if err != nil {
		return nil, err
	}
	return characterSnapshotPreferenceToProtocol(&row), nil
}

func CharacterSnapshotPreferenceUpdate(channelID, actorID string, update CharacterSnapshotPreferenceUpdateInput) (*protocol.CharacterSnapshotPreferencePayload, error) {
	channelID = strings.TrimSpace(channelID)
	if !CanReadChannelByUserId(actorID, channelID) {
		return nil, errors.New("无权更新人物卡模板偏好")
	}
	badgeMode, err := validateCharacterSnapshotMode(update.BadgeTemplateMode)
	if err != nil {
		return nil, err
	}
	overlayMode, err := validateCharacterSnapshotMode(update.TheaterOverlayTemplateMode)
	if err != nil {
		return nil, err
	}
	badgeTemplate := strings.TrimSpace(update.BadgeTemplate)
	if utf8.RuneCountInString(badgeTemplate) > 512 {
		return nil, errors.New("徽章模板长度需在512个字符以内")
	}
	overlayJSON, err := validateCharacterOverlayTemplate(update.TheaterOverlayTemplateJSON)
	if err != nil {
		return nil, err
	}
	var saved model.ChannelCharacterSnapshotPreferenceModel
	var revision int64
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		findErr := tx.Where("channel_id = ? AND user_id = ?", channelID, actorID).First(&saved).Error
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		revision = nextCharacterSnapshotRevision(saved.ServerRevision)
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			saved = model.ChannelCharacterSnapshotPreferenceModel{ChannelID: channelID, UserID: actorID, BadgeTemplateMode: badgeMode, BadgeTemplate: badgeTemplate, TheaterOverlayTemplateMode: overlayMode, TheaterOverlayTemplateJSON: overlayJSON, SchemaVersion: 1, ServerRevision: revision}
			return tx.Create(&saved).Error
		}
		return tx.Model(&saved).Updates(map[string]any{"badge_template_mode": badgeMode, "badge_template": badgeTemplate, "theater_overlay_template_mode": overlayMode, "theater_overlay_template_json": overlayJSON, "schema_version": 1, "server_revision": revision}).Error
	})
	if err != nil {
		return nil, err
	}
	saved.BadgeTemplateMode = badgeMode
	saved.BadgeTemplate = badgeTemplate
	saved.TheaterOverlayTemplateMode = overlayMode
	saved.TheaterOverlayTemplateJSON = overlayJSON
	saved.SchemaVersion = 1
	saved.ServerRevision = revision
	return characterSnapshotPreferenceToProtocol(&saved), nil
}

func characterSnapshotModelToProtocol(row *model.ChannelCharacterSnapshotModel) (*protocol.CharacterSnapshotItem, error) {
	if row == nil {
		return nil, nil
	}
	var data protocol.CharacterSnapshotData
	if err := json.Unmarshal([]byte(row.PayloadJSON), &data); err != nil {
		return nil, err
	}
	return &protocol.CharacterSnapshotItem{ChannelID: row.ChannelID, IdentityID: row.IdentityID, UserID: row.UserID, SourceType: row.SourceType, SourceCardID: row.SourceCardID, Data: data, ContentHash: row.ContentHash, ServerRevision: row.ServerRevision, SourceUpdatedAt: row.SourceUpdatedAt, LastSeenAt: row.LastSeenAt}, nil
}

func characterSnapshotSettingsToProtocol(row *model.ChannelCharacterSnapshotSettingsModel) *protocol.CharacterSnapshotSettingsPayload {
	return &protocol.CharacterSnapshotSettingsPayload{ChannelID: row.ChannelID, BadgeTemplate: row.BadgeTemplate, TheaterOverlayTemplateJSON: row.TheaterOverlayTemplateJSON, SchemaVersion: row.SchemaVersion, ServerRevision: row.ServerRevision, UpdatedBy: row.UpdatedBy}
}

func characterSnapshotPreferenceToProtocol(row *model.ChannelCharacterSnapshotPreferenceModel) *protocol.CharacterSnapshotPreferencePayload {
	return &protocol.CharacterSnapshotPreferencePayload{ChannelID: row.ChannelID, UserID: row.UserID, BadgeTemplateMode: row.BadgeTemplateMode, BadgeTemplate: row.BadgeTemplate, TheaterOverlayTemplateMode: row.TheaterOverlayTemplateMode, TheaterOverlayTemplateJSON: row.TheaterOverlayTemplateJSON, SchemaVersion: row.SchemaVersion, ServerRevision: row.ServerRevision}
}

func loadCharacterSnapshotTemplates(channelID string) (*model.ChannelCharacterSnapshotSettingsModel, map[string]*model.ChannelCharacterSnapshotPreferenceModel, error) {
	settings := &model.ChannelCharacterSnapshotSettingsModel{ChannelID: channelID, TheaterOverlayTemplateJSON: defaultCharacterOverlayTemplate, SchemaVersion: 1}
	if err := model.GetDB().Where("channel_id = ?", channelID).First(settings).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	var rows []*model.ChannelCharacterSnapshotPreferenceModel
	if err := model.GetDB().Where("channel_id = ?", channelID).Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	preferences := make(map[string]*model.ChannelCharacterSnapshotPreferenceModel, len(rows))
	for _, row := range rows {
		if row != nil && row.UserID != "" {
			preferences[row.UserID] = row
		}
	}
	return settings, preferences, nil
}

func applyEffectiveCharacterSnapshotTemplates(item *protocol.CharacterSnapshotItem, settings *model.ChannelCharacterSnapshotSettingsModel, preference *model.ChannelCharacterSnapshotPreferenceModel) {
	if item == nil {
		return
	}
	item.BadgeTemplate = settings.BadgeTemplate
	item.TheaterOverlayTemplateJSON = settings.TheaterOverlayTemplateJSON
	if item.TheaterOverlayTemplateJSON == "" {
		item.TheaterOverlayTemplateJSON = defaultCharacterOverlayTemplate
	}
	if preference == nil {
		return
	}
	switch preference.BadgeTemplateMode {
	case "off":
		item.BadgeTemplate = ""
	case "custom":
		item.BadgeTemplate = preference.BadgeTemplate
	}
	switch preference.TheaterOverlayTemplateMode {
	case "off":
		item.TheaterOverlayTemplateJSON = ""
	case "custom":
		item.TheaterOverlayTemplateJSON = preference.TheaterOverlayTemplateJSON
	}
}

func defaultCharacterSnapshotPreference(channelID, userID string) *protocol.CharacterSnapshotPreferencePayload {
	return &protocol.CharacterSnapshotPreferencePayload{ChannelID: strings.TrimSpace(channelID), UserID: strings.TrimSpace(userID), BadgeTemplateMode: "inherit", TheaterOverlayTemplateMode: "inherit", TheaterOverlayTemplateJSON: defaultCharacterOverlayTemplate, SchemaVersion: 1}
}

func ensureCharacterSnapshotSettingsPermission(channelID, actorID string) error {
	channel, err := model.ChannelGet(channelID)
	if err != nil || channel == nil || channel.ID == "" {
		return errors.New("频道不存在")
	}
	if pm.CanWithSystemRole(actorID, pm.PermModAdmin) || pm.CanWithChannelRole(actorID, channelID, pm.PermFuncChannelManageInfo) || (channel.WorldID != "" && IsWorldAdmin(channel.WorldID, actorID)) {
		return nil
	}
	return errors.New("无权更新频道人物卡模板")
}

func validateCharacterSnapshotMode(mode string) (string, error) {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "inherit"
	}
	if mode != "inherit" && mode != "custom" && mode != "off" {
		return "", errors.New("模板模式必须为 inherit、custom 或 off")
	}
	return mode, nil
}

func validateCharacterOverlayTemplate(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = defaultCharacterOverlayTemplate
	}
	if len(raw) > maxCharacterOverlayBytes {
		return "", errors.New("小剧场数据浮层模板不可超过64KB")
	}
	var template struct {
		Version          int `json:"version"`
		PreferredColumns int `json:"preferredColumns"`
		Items            []struct {
			ID        string         `json:"id"`
			Name      string         `json:"name"`
			Current   map[string]any `json:"current"`
			Max       map[string]any `json:"max,omitempty"`
			Min       map[string]any `json:"min,omitempty"`
			BarColor  string         `json:"barColor,omitempty"`
			TextColor string         `json:"textColor,omitempty"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &template); err != nil {
		return "", errors.New("小剧场数据浮层模板不是有效JSON")
	}
	if template.Version != 1 {
		return "", errors.New("小剧场数据浮层模板版本必须为1")
	}
	if template.Items == nil {
		return "", errors.New("小剧场数据浮层模板 items 必须为数组")
	}
	if template.PreferredColumns == 0 {
		template.PreferredColumns = 2
	}
	if template.PreferredColumns < 1 || template.PreferredColumns > 4 || len(template.Items) > 64 {
		return "", errors.New("小剧场数据浮层模板列数或数据项数量无效")
	}
	seen := map[string]struct{}{}
	for _, item := range template.Items {
		id := strings.TrimSpace(item.ID)
		name := strings.TrimSpace(item.Name)
		if id == "" || name == "" || utf8.RuneCountInString(id) > 64 || utf8.RuneCountInString(name) > 64 {
			return "", errors.New("小剧场数据项必须有有效ID与名称")
		}
		if _, exists := seen[id]; exists {
			return "", fmt.Errorf("小剧场数据项ID重复: %s", id)
		}
		seen[id] = struct{}{}
		if err := validateCharacterNumericSource(item.Current, true); err != nil {
			return "", fmt.Errorf("数据项 %s 当前值无效: %w", name, err)
		}
		if err := validateCharacterNumericSource(item.Max, false); err != nil {
			return "", fmt.Errorf("数据项 %s 最大值无效: %w", name, err)
		}
		if err := validateCharacterNumericSource(item.Min, false); err != nil {
			return "", fmt.Errorf("数据项 %s 最小值无效: %w", name, err)
		}
		if item.BarColor != "" && !characterSnapshotColorPattern.MatchString(item.BarColor) {
			return "", fmt.Errorf("数据项 %s 数据条颜色无效", name)
		}
		if item.TextColor != "" && !characterSnapshotColorPattern.MatchString(item.TextColor) {
			return "", fmt.Errorf("数据项 %s 文本颜色无效", name)
		}
	}
	canonical, err := json.Marshal(template)
	if err != nil {
		return "", err
	}
	return string(canonical), nil
}

func validateCharacterNumericSource(source map[string]any, required bool) error {
	if len(source) == 0 {
		if required {
			return errors.New("不能为空")
		}
		return nil
	}
	path, hasPath := source["path"].(string)
	value, hasValue := source["value"].(float64)
	if hasPath == hasValue {
		return errors.New("必须且只能指定 path 或 value")
	}
	if hasPath && (strings.TrimSpace(path) == "" || utf8.RuneCountInString(path) > 128) {
		return errors.New("path 无效")
	}
	if hasValue && (math.IsNaN(value) || math.IsInf(value, 0)) {
		return errors.New("value 无效")
	}
	return nil
}

func normalizeCharacterSnapshotColor(value, fallback string) string {
	value = strings.TrimSpace(value)
	if characterSnapshotColorPattern.MatchString(value) {
		return value
	}
	fallback = strings.TrimSpace(fallback)
	if characterSnapshotColorPattern.MatchString(fallback) {
		return fallback
	}
	return ""
}

func nextCharacterSnapshotRevision(previous int64) int64 {
	now := time.Now().UnixMilli()
	if now <= previous {
		return previous + 1
	}
	return now
}
