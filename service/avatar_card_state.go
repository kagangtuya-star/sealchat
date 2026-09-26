package service

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
	"sealchat/protocol"
)

const maxWorldCharacterAttrsBytes = 64 * 1024

type AvatarBotMutationContext struct {
	TargetUserID string
	SourceCardID string
}

// AvatarBotMutationAuthorize checks the durable identity, snapshot and source policy.
// Live connection and BOT card checks belong to the WebSocket API boundary.
func AvatarBotMutationAuthorize(channelID, identityID, actorID, expectedCardID string, botCapable bool) (*AvatarBotMutationContext, error) {
	channelID, identityID, actorID = strings.TrimSpace(channelID), strings.TrimSpace(identityID), strings.TrimSpace(actorID)
	if channelID == "" || identityID == "" || actorID == "" {
		return nil, errors.New("NOT_EDITABLE")
	}
	identity, err := model.ChannelIdentityGetByID(identityID)
	if err != nil || identity == nil || identity.ChannelID != channelID || identity.IsHidden {
		return nil, errors.New("NOT_EDITABLE")
	}
	if identity.UserID == actorID {
		if !CanReadChannelByUserId(actorID, channelID) {
			return nil, errors.New("PERMISSION_DENIED")
		}
	} else {
		actor, actorErr := ResolveChannelIdentityActor(channelID, actorID, identity.UserID)
		if actorErr != nil || actor == nil || actor.OperatorRank < 3 {
			return nil, errors.New("PERMISSION_DENIED")
		}
		if _, actorErr = ValidateChannelIdentityActorIdentity(actor, channelID, identityID); actorErr != nil {
			return nil, errors.New("PERMISSION_DENIED")
		}
	}
	settings, err := AvatarCardSettingsGetByChannel(channelID)
	if err != nil {
		return nil, err
	}
	if settings.SourceMode == "world" || !botCapable {
		return nil, errors.New("NOT_EDITABLE")
	}
	var snapshot model.ChannelCharacterSnapshotModel
	if err = model.GetDB().Where("channel_id = ? AND identity_id = ? AND is_active = ?", channelID, identityID, true).Take(&snapshot).Error; err != nil || strings.TrimSpace(snapshot.SourceCardID) == "" || snapshot.UserID != identity.UserID {
		return nil, errors.New("NOT_EDITABLE")
	}
	if expectedCardID != "" && snapshot.SourceCardID != expectedCardID {
		return nil, errors.New("CARD_CHANGED")
	}
	return &AvatarBotMutationContext{TargetUserID: identity.UserID, SourceCardID: snapshot.SourceCardID}, nil
}

func AvatarBotMutationTemplatePath(channelID, statID, slot string) (string, error) {
	if statID == "" || (slot != "current" && slot != "max") {
		return "", errors.New("NOT_EDITABLE")
	}
	settings, err := AvatarCardSettingsGetByChannel(channelID)
	if err != nil {
		return "", err
	}
	if len(settings.BotTemplateJSON) > maxCharacterOverlayBytes {
		return "", errors.New("NOT_EDITABLE")
	}
	var template struct {
		Version int `json:"version"`
		Items   []struct {
			ID      string                     `json:"id"`
			Current map[string]json.RawMessage `json:"current"`
			Max     map[string]json.RawMessage `json:"max"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(settings.BotTemplateJSON), &template); err != nil || template.Version != 1 || template.Items == nil {
		return "", errors.New("NOT_EDITABLE")
	}
	for _, item := range template.Items {
		if item.ID != statID {
			continue
		}
		source := item.Current
		if slot == "max" {
			source = item.Max
		}
		if len(source) != 1 {
			return "", errors.New("NOT_EDITABLE")
		}
		var path string
		if err := json.Unmarshal(source["path"], &path); err != nil || strings.TrimSpace(path) == "" || len(path) > 512 {
			return "", errors.New("NOT_EDITABLE")
		}
		return path, nil
	}
	return "", errors.New("NOT_EDITABLE")
}

type AvatarCardSettingsUpdateInput struct {
	SourceMode     *string
	TemplateSource string
	TemplateJSON   *string
}

func AvatarCardSettingsGet(channelID, actorID string) (*protocol.AvatarCardSettingsPayload, error) {
	channelID = strings.TrimSpace(channelID)
	if !CanReadChannelByUserId(actorID, channelID) {
		return nil, errors.New("无权查看头像点击卡片设置")
	}
	return AvatarCardSettingsGetByChannel(channelID)
}

// AvatarCardSettingsGetByChannel reads a channel after the caller has checked read access.
func AvatarCardSettingsGetByChannel(channelID string) (*protocol.AvatarCardSettingsPayload, error) {
	channelID = strings.TrimSpace(channelID)
	var row model.ChannelAvatarCardSettingsModel
	err := model.GetDB().Where("channel_id = ?", channelID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &protocol.AvatarCardSettingsPayload{ChannelID: channelID, SchemaVersion: 1}, nil
	}
	if err != nil {
		return nil, err
	}
	return avatarCardSettingsPayload(&row), nil
}

func AvatarCardSettingsUpdate(channelID, actorID string, input AvatarCardSettingsUpdateInput) (*protocol.AvatarCardSettingsPayload, error) {
	channelID = strings.TrimSpace(channelID)
	if err := ensureCharacterSnapshotSettingsPermission(channelID, actorID); err != nil {
		return nil, err
	}
	mode := ""
	if input.SourceMode != nil {
		mode = strings.TrimSpace(*input.SourceMode)
		if mode != "bot" && mode != "world" {
			return nil, errors.New("头像卡片数据来源必须为 bot 或 world")
		}
	}
	templateSource := strings.TrimSpace(input.TemplateSource)
	if input.TemplateJSON != nil && templateSource != "bot" && templateSource != "world" {
		return nil, errors.New("头像卡片模板来源必须为 bot 或 world")
	}
	if input.TemplateJSON == nil && templateSource != "" {
		return nil, errors.New("缺少头像卡片模板")
	}
	if input.SourceMode == nil && input.TemplateJSON == nil {
		return nil, errors.New("没有可更新的头像卡片设置")
	}
	var templateJSON string
	if input.TemplateJSON != nil {
		var err error
		templateJSON, err = validateCharacterOverlayTemplate(*input.TemplateJSON)
		if err != nil {
			return nil, err
		}
	}
	var saved model.ChannelAvatarCardSettingsModel
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		findErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("channel_id = ?", channelID).Take(&saved).Error
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		revision := nextCharacterSnapshotRevision(saved.ServerRevision)
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			saved = model.ChannelAvatarCardSettingsModel{ChannelID: channelID, SourceMode: mode, SchemaVersion: 1, ServerRevision: revision, UpdatedBy: actorID}
			if templateSource == "bot" {
				saved.BotTemplateJSON = templateJSON
			}
			if templateSource == "world" {
				saved.WorldTemplateJSON = templateJSON
			}
			return tx.Create(&saved).Error
		}
		updates := map[string]any{"schema_version": 1, "server_revision": revision, "updated_by": actorID}
		if input.SourceMode != nil {
			updates["source_mode"] = mode
		}
		if templateSource == "bot" {
			updates["bot_template_json"] = templateJSON
		}
		if templateSource == "world" {
			updates["world_template_json"] = templateJSON
		}
		if err := tx.Model(&saved).Updates(updates).Error; err != nil {
			return err
		}
		if input.SourceMode != nil {
			saved.SourceMode = mode
		}
		saved.SchemaVersion, saved.ServerRevision, saved.UpdatedBy = 1, revision, actorID
		if templateSource == "bot" {
			saved.BotTemplateJSON = templateJSON
		}
		if templateSource == "world" {
			saved.WorldTemplateJSON = templateJSON
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return avatarCardSettingsPayload(&saved), nil
}

func avatarCardSettingsPayload(row *model.ChannelAvatarCardSettingsModel) *protocol.AvatarCardSettingsPayload {
	return &protocol.AvatarCardSettingsPayload{ChannelID: row.ChannelID, SourceMode: row.SourceMode,
		BotTemplateJSON: row.BotTemplateJSON, WorldTemplateJSON: row.WorldTemplateJSON,
		SchemaVersion: row.SchemaVersion, ServerRevision: row.ServerRevision, UpdatedBy: row.UpdatedBy}
}

func WorldCharacterSubjectKey(identity *model.ChannelIdentityModel) string {
	if identity == nil {
		return ""
	}
	if sharedID := strings.TrimSpace(identity.SharedIdentityID); sharedID != "" {
		return "shared:" + sharedID
	}
	return "identity:" + strings.TrimSpace(identity.ID)
}

func worldCharacterChannel(channelID, actorID string) (*model.ChannelModel, error) {
	channelID = strings.TrimSpace(channelID)
	if !CanReadChannelByUserId(actorID, channelID) {
		return nil, errors.New("无权读取频道世界角色状态")
	}
	return worldCharacterChannelByChannel(channelID)
}

func worldCharacterChannelByChannel(channelID string) (*model.ChannelModel, error) {
	channelID = strings.TrimSpace(channelID)
	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return nil, err
	}
	if channel == nil || channel.ID == "" || strings.TrimSpace(channel.WorldID) == "" {
		return nil, errors.New("频道或所属世界不存在")
	}
	return channel, nil
}

func WorldCharacterStateList(channelID, actorID string) (*protocol.WorldCharacterStateListPayload, error) {
	channelID = strings.TrimSpace(channelID)
	if !CanReadChannelByUserId(actorID, channelID) {
		return nil, errors.New("无权读取频道世界角色状态")
	}
	return WorldCharacterStateListByChannel(channelID)
}

// WorldCharacterStateListByChannel reads a channel after the caller has checked read access.
func WorldCharacterStateListByChannel(channelID string) (*protocol.WorldCharacterStateListPayload, error) {
	channel, err := worldCharacterChannelByChannel(channelID)
	if err != nil {
		return nil, err
	}
	identities, err := model.ChannelIdentityListAll(channel.ID)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(identities))
	for _, identity := range identities {
		keys = append(keys, WorldCharacterSubjectKey(identity))
	}
	rowsByKey := make(map[string]model.WorldCharacterStateModel, len(keys))
	if len(keys) > 0 {
		var rows []model.WorldCharacterStateModel
		if err := model.GetDB().Where("world_id = ? AND subject_key IN ?", channel.WorldID, keys).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			rowsByKey[row.SubjectKey] = row
		}
	}
	items := make([]*protocol.WorldCharacterStatePayload, 0, len(identities))
	for _, identity := range identities {
		key := WorldCharacterSubjectKey(identity)
		row, exists := rowsByKey[key]
		attrs := map[string]any{}
		if exists {
			attrs, err = parseWorldCharacterAttrs(row.AttrsJSON)
			if err != nil {
				return nil, err
			}
		}
		items = append(items, &protocol.WorldCharacterStatePayload{WorldID: channel.WorldID, IdentityID: identity.ID,
			UserID: identity.UserID, SharedIdentityID: identity.SharedIdentityID, SubjectKey: key, Attrs: attrs, Revision: row.Revision})
	}
	return &protocol.WorldCharacterStateListPayload{ChannelID: channel.ID, WorldID: channel.WorldID, Items: items}, nil
}

func WorldCharacterStatePatch(channelID, identityID, actorID, path, op string, value float64) (*protocol.WorldCharacterStatePayload, error) {
	channel, err := worldCharacterChannel(channelID, actorID)
	if err != nil {
		return nil, err
	}
	identityID, path, op = strings.TrimSpace(identityID), strings.TrimSpace(path), strings.TrimSpace(op)
	if !validWorldCharacterPath(path) || (op != "set" && op != "add") || math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, errors.New("世界角色状态修改参数无效")
	}
	identity, err := model.ChannelIdentityGetByID(identityID)
	if err != nil || identity.ChannelID != channel.ID {
		return nil, errors.New("频道身份不存在")
	}
	if identity.IsHidden {
		return nil, errors.New("频道身份不存在")
	}
	if identity.UserID != actorID {
		actor, err := ResolveChannelIdentityActor(channel.ID, actorID, identity.UserID)
		if err != nil {
			return nil, err
		}
		if _, err := ValidateChannelIdentityActorIdentity(actor, channel.ID, identityID); err != nil {
			return nil, err
		}
	}
	key := WorldCharacterSubjectKey(identity)
	var attrs map[string]any
	var revision int64
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		var row model.WorldCharacterStateModel
		findErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("world_id = ? AND subject_key = ?", channel.WorldID, key).Take(&row).Error
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return findErr
		}
		attrs = map[string]any{}
		if findErr == nil {
			var parseErr error
			attrs, parseErr = parseWorldCharacterAttrs(row.AttrsJSON)
			if parseErr != nil {
				return parseErr
			}
		}
		if op == "add" {
			current, exists := attrs[path]
			if exists {
				numeric, ok := current.(float64)
				if !ok {
					return errors.New("当前属性不是数值，不能增加")
				}
				value += numeric
			}
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return errors.New("世界角色状态数值无效")
		}
		attrs[path] = value
		encoded, err := json.Marshal(attrs)
		if err != nil || len(encoded) > maxWorldCharacterAttrsBytes {
			return errors.New("世界角色状态不可超过64KB")
		}
		revision = nextCharacterSnapshotRevision(row.Revision)
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			return tx.Create(&model.WorldCharacterStateModel{WorldID: channel.WorldID, SubjectKey: key, AttrsJSON: string(encoded), Revision: revision, UpdatedBy: actorID}).Error
		}
		return tx.Model(&row).Updates(map[string]any{"attrs_json": string(encoded), "revision": revision, "updated_by": actorID}).Error
	})
	if err != nil {
		return nil, err
	}
	return &protocol.WorldCharacterStatePayload{WorldID: channel.WorldID, IdentityID: identity.ID,
		UserID: identity.UserID, SharedIdentityID: identity.SharedIdentityID, SubjectKey: key, Attrs: attrs, Revision: revision}, nil
}

func migrateWorldCharacterStateSubjectTx(tx *gorm.DB, channelID string, oldSubjectKey string, newSubjectKey string) error {
	channelID = strings.TrimSpace(channelID)
	oldSubjectKey = strings.TrimSpace(oldSubjectKey)
	newSubjectKey = strings.TrimSpace(newSubjectKey)
	if tx == nil || channelID == "" || oldSubjectKey == "" || newSubjectKey == "" {
		return errors.New("迁移世界角色状态参数无效")
	}
	if oldSubjectKey == newSubjectKey {
		return nil
	}

	var channel model.ChannelModel
	if err := tx.Select("id", "world_id").Where("id = ?", channelID).Take(&channel).Error; err != nil {
		return err
	}
	worldID := strings.TrimSpace(channel.WorldID)
	if worldID == "" {
		return nil
	}

	var row model.WorldCharacterStateModel
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("world_id = ? AND subject_key = ?", worldID, oldSubjectKey).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return tx.Model(&row).UpdateColumn("subject_key", newSubjectKey).Error
}

func deleteWorldCharacterStateSubjectByChannel(channelID string, subjectKey string) error {
	channelID = strings.TrimSpace(channelID)
	subjectKey = strings.TrimSpace(subjectKey)
	if channelID == "" || subjectKey == "" {
		return nil
	}
	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return err
	}
	worldID := strings.TrimSpace(channel.WorldID)
	if worldID == "" {
		return nil
	}
	return model.GetDB().Where("world_id = ? AND subject_key = ?", worldID, subjectKey).
		Delete(&model.WorldCharacterStateModel{}).Error
}

func parseWorldCharacterAttrs(raw string) (map[string]any, error) {
	if len(raw) > maxWorldCharacterAttrsBytes {
		return nil, errors.New("世界角色状态不可超过64KB")
	}
	var attrs map[string]any
	if err := json.Unmarshal([]byte(raw), &attrs); err != nil || attrs == nil {
		return nil, errors.New("世界角色状态必须是JSON对象")
	}
	return attrs, nil
}

func validWorldCharacterPath(path string) bool {
	if path == "" || utf8.RuneCountInString(path) > 128 {
		return false
	}
	for _, r := range path {
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '_' && r != '-' {
			return false
		}
	}
	return true
}
