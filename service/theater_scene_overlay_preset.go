package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
)

const (
	theaterSceneOverlayPresetMaxCount = 200
	theaterSceneOverlayPresetMaxItems = 32
	theaterSceneOverlayPresetMaxBytes = 128 << 10
)

type TheaterSceneOverlayMediaRef struct {
	ResourceID string `json:"resourceId"`
	Variant    string `json:"variant,omitempty"`
	MimeType   string `json:"mimeType,omitempty"`
	Animated   *bool  `json:"animated,omitempty"`
	LoopCount  *int   `json:"loopCount,omitempty"`
}

type TheaterSceneOverlayPresetItem struct {
	EffectID  string                       `json:"effectId"`
	Name      string                       `json:"name,omitempty"`
	Enabled   *bool                        `json:"enabled,omitempty"`
	Opacity   *float64                     `json:"opacity,omitempty"`
	BlendMode string                       `json:"blendMode,omitempty"`
	Layer     string                       `json:"layer,omitempty"`
	Media     *TheaterSceneOverlayMediaRef `json:"media,omitempty"`
	Params    map[string]any               `json:"params,omitempty"`
}

type TheaterSceneOverlayPresetInput struct {
	Name        string                          `json:"name"`
	Description string                          `json:"description"`
	Tags        []string                        `json:"tags"`
	Overlays    []TheaterSceneOverlayPresetItem `json:"overlays"`
}

type TheaterSceneOverlayPreset struct {
	ID          string                          `json:"id"`
	Name        string                          `json:"name"`
	Description string                          `json:"description"`
	Tags        []string                        `json:"tags"`
	Overlays    []TheaterSceneOverlayPresetItem `json:"overlays"`
	Revision    int64                           `json:"revision"`
	CreatedBy   string                          `json:"createdBy,omitempty"`
	UpdatedBy   string                          `json:"updatedBy,omitempty"`
	CreatedAt   time.Time                       `json:"createdAt"`
	UpdatedAt   time.Time                       `json:"updatedAt"`
}

type TheaterSceneOverlayPresetPatch struct {
	Name        *string
	Description *string
	Tags        *[]string
	Overlays    *[]TheaterSceneOverlayPresetItem
	Revision    int64
}

func ListTheaterSceneOverlayPresets(_ context.Context, actorID, worldID, channelID string) ([]TheaterSceneOverlayPreset, error) {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionView); err != nil {
		return nil, err
	}
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return nil, err
	}
	var rows []model.TheaterSceneOverlayPresetModel
	if err := model.GetDB().Where("room_id = ?", room.ID).Order("updated_at DESC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]TheaterSceneOverlayPreset, 0, len(rows))
	for i := range rows {
		preset, err := theaterSceneOverlayPresetFromModel(&rows[i])
		if err != nil {
			return nil, fmt.Errorf("decode theater scene overlay preset %s: %w", rows[i].ID, err)
		}
		result = append(result, *preset)
	}
	return result, nil
}

func CreateTheaterSceneOverlayPreset(_ context.Context, actorID, worldID, channelID string, input TheaterSceneOverlayPresetInput) (*TheaterSceneOverlayPreset, error) {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionObjectEdit); err != nil {
		return nil, err
	}
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return nil, err
	}
	normalized, tagsJSON, overlaysJSON, err := normalizeTheaterSceneOverlayPresetInput(input)
	if err != nil {
		return nil, err
	}
	if err := validateTheaterSceneOverlayPresetResources(room.ID, normalized.Overlays); err != nil {
		return nil, err
	}
	row := &model.TheaterSceneOverlayPresetModel{
		RoomID: room.ID, Name: normalized.Name, Description: normalized.Description,
		TagsJSON: tagsJSON, OverlaysJSON: overlaysJSON, Revision: 1, CreatedBy: actorID, UpdatedBy: actorID,
	}
	if err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		var lockedRoom model.TheaterRoomModel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", room.ID).First(&lockedRoom).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&model.TheaterSceneOverlayPresetModel{}).Where("room_id = ?", room.ID).Count(&count).Error; err != nil {
			return err
		}
		if count >= theaterSceneOverlayPresetMaxCount {
			return newTheaterError(TheaterErrorLimitExceeded, "场景预设数量已达上限", 409, nil)
		}
		if err := tx.Create(row).Error; err != nil {
			if isTheaterSceneOverlayPresetDuplicateError(err) {
				return theaterPayloadError("同名场景预设已存在")
			}
			return err
		}
		return recalculateTheaterResourceReferences(tx, room.ID)
	}); err != nil {
		return nil, err
	}
	return theaterSceneOverlayPresetFromModel(row)
}

func UpdateTheaterSceneOverlayPreset(_ context.Context, actorID, worldID, channelID, presetID string, patch TheaterSceneOverlayPresetPatch) (*TheaterSceneOverlayPreset, error) {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionObjectEdit); err != nil {
		return nil, err
	}
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return nil, err
	}
	if err := validateTheaterID(presetID, "presetId"); err != nil {
		return nil, err
	}
	var row model.TheaterSceneOverlayPresetModel
	if err := model.GetDB().Where("room_id = ? AND id = ?", room.ID, presetID).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, newTheaterError(TheaterErrorNotFound, "场景预设不存在", 404, nil)
		}
		return nil, err
	}
	input, err := theaterSceneOverlayPresetInputFromModel(&row)
	if err != nil {
		return nil, err
	}
	if patch.Name != nil {
		input.Name = *patch.Name
	}
	if patch.Description != nil {
		input.Description = *patch.Description
	}
	if patch.Tags != nil {
		input.Tags = *patch.Tags
	}
	if patch.Overlays != nil {
		input.Overlays = *patch.Overlays
	}
	normalized, tagsJSON, overlaysJSON, err := normalizeTheaterSceneOverlayPresetInput(input)
	if err != nil {
		return nil, err
	}
	if err := validateTheaterSceneOverlayPresetResources(room.ID, normalized.Overlays); err != nil {
		return nil, err
	}
	updates := map[string]any{
		"name": normalized.Name, "description": normalized.Description,
		"tags_json": tagsJSON, "overlays_json": overlaysJSON,
		"updated_by": actorID, "revision": row.Revision + 1,
	}
	var conflict bool
	var updatedRow model.TheaterSceneOverlayPresetModel
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.TheaterSceneOverlayPresetModel{}).
			Where("room_id = ? AND id = ? AND revision = ?", room.ID, presetID, patch.Revision).
			Updates(updates)
		if result.Error != nil {
			if isTheaterSceneOverlayPresetDuplicateError(result.Error) {
				return theaterPayloadError("同名场景预设已存在")
			}
			return result.Error
		}
		if result.RowsAffected != 1 {
			conflict = true
			return nil
		}
		if patch.Overlays != nil {
			if err := recalculateTheaterResourceReferences(tx, room.ID); err != nil {
				return err
			}
		}
		return tx.Where("room_id = ? AND id = ?", room.ID, presetID).First(&updatedRow).Error
	})
	if err != nil {
		return nil, err
	}
	if conflict {
		return nil, newTheaterError(TheaterErrorRevisionConflict, "场景预设已被其他用户修改", 409, map[string]any{"revision": row.Revision})
	}
	return theaterSceneOverlayPresetFromModel(&updatedRow)
}

func DeleteTheaterSceneOverlayPreset(_ context.Context, actorID, worldID, channelID, presetID string) error {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionObjectEdit); err != nil {
		return err
	}
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return err
	}
	if err := validateTheaterID(presetID, "presetId"); err != nil {
		return err
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		result := tx.Unscoped().Where("room_id = ? AND id = ?", room.ID, presetID).Delete(&model.TheaterSceneOverlayPresetModel{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return newTheaterError(TheaterErrorNotFound, "场景预设不存在", 404, nil)
		}
		return recalculateTheaterResourceReferences(tx, room.ID)
	})
}

func normalizeTheaterSceneOverlayPresetInput(input TheaterSceneOverlayPresetInput) (TheaterSceneOverlayPresetInput, string, string, error) {
	input.Name = strings.TrimSpace(input.Name)
	if err := validateTheaterPresetName(input.Name); err != nil {
		return input, "", "", err
	}
	input.Description = strings.TrimSpace(input.Description)
	if len([]rune(input.Description)) > 512 {
		return input, "", "", theaterPayloadError("description 长度无效")
	}
	if len(input.Tags) > 16 {
		return input, "", "", theaterPayloadError("tags 数量超限")
	}
	seenTags := map[string]struct{}{}
	for i, tag := range input.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || len([]rune(tag)) > 32 {
			return input, "", "", theaterPayloadError("tags[" + strconv.Itoa(i) + "] 无效")
		}
		if _, ok := seenTags[tag]; ok {
			return input, "", "", theaterPayloadError("tags 不能重复")
		}
		seenTags[tag] = struct{}{}
		input.Tags[i] = tag
	}
	if len(input.Overlays) == 0 || len(input.Overlays) > theaterSceneOverlayPresetMaxItems {
		return input, "", "", theaterPayloadError("overlays 数量无效")
	}
	for i := range input.Overlays {
		if err := normalizeTheaterSceneOverlayPresetItem(&input.Overlays[i], i); err != nil {
			return input, "", "", err
		}
	}
	tagsRaw, err := json.Marshal(input.Tags)
	if err != nil {
		return input, "", "", err
	}
	overlaysRaw, err := json.Marshal(input.Overlays)
	if err != nil {
		return input, "", "", err
	}
	if len(overlaysRaw) > theaterSceneOverlayPresetMaxBytes {
		return input, "", "", theaterPayloadError("overlays 数据过大")
	}
	return input, string(tagsRaw), string(overlaysRaw), nil
}

func validateTheaterPresetName(value string) error {
	if len([]rune(value)) < 1 || len([]rune(value)) > 128 {
		return theaterPayloadError("name 长度无效")
	}
	return nil
}

func isTheaterSceneOverlayPresetDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	message := err.Error()
	return strings.Contains(message, "UNIQUE constraint failed") ||
		strings.Contains(message, "Error 1062") ||
		strings.Contains(message, "Duplicate entry") ||
		strings.Contains(message, "SQLSTATE 23505") ||
		strings.Contains(message, "duplicate key value")
}

func normalizeTheaterSceneOverlayPresetItem(item *TheaterSceneOverlayPresetItem, index int) error {
	item.EffectID = strings.TrimSpace(item.EffectID)
	if item.EffectID == "" || len([]rune(item.EffectID)) > 128 {
		return theaterPayloadError("overlays[" + strconv.Itoa(index) + "].effectId 无效")
	}
	item.Name = strings.TrimSpace(item.Name)
	if len([]rune(item.Name)) > 128 {
		return theaterPayloadError("overlays.name 长度无效")
	}
	if item.Opacity != nil && (math.IsNaN(*item.Opacity) || math.IsInf(*item.Opacity, 0) || *item.Opacity < 0 || *item.Opacity > 1) {
		return theaterPayloadError("overlays.opacity 无效")
	}
	if item.BlendMode != "" {
		allowed := map[string]bool{"normal": true, "multiply": true, "screen": true, "overlay": true, "darken": true, "lighten": true, "color-dodge": true, "color-burn": true, "hard-light": true, "soft-light": true}
		if !allowed[item.BlendMode] {
			return theaterPayloadError("overlays.blendMode 无效")
		}
	}
	if item.Layer != "" && item.Layer != "belowCharacters" && item.Layer != "aboveCharacters" {
		return theaterPayloadError("overlays.layer 无效")
	}
	if item.Params != nil {
		if len(item.Params) > 64 {
			return theaterPayloadError("overlays.params 数量超限")
		}
		for key, value := range item.Params {
			if strings.TrimSpace(key) == "" || len([]rune(key)) > 64 {
				return theaterPayloadError("overlays.params 键无效")
			}
			switch current := value.(type) {
			case nil, string, bool:
			case float64:
				if math.IsNaN(current) || math.IsInf(current, 0) {
					return theaterPayloadError("overlays.params 数字无效")
				}
			default:
				return theaterPayloadError("overlays.params 仅支持基础类型")
			}
		}
	}
	if item.Media != nil {
		item.Media.ResourceID = strings.TrimSpace(item.Media.ResourceID)
		if err := validateTheaterID(item.Media.ResourceID, "overlays.media.resourceId"); err != nil {
			return err
		}
		item.Media.Variant = strings.TrimSpace(item.Media.Variant)
		if item.Media.Variant == "" {
			item.Media.Variant = "original"
		}
		if len([]rune(item.Media.Variant)) > 64 || len([]rune(item.Media.MimeType)) > 128 {
			return theaterPayloadError("overlays.media 无效")
		}
		if item.Media.LoopCount != nil && (*item.Media.LoopCount < 1 || *item.Media.LoopCount > 65535) {
			return theaterPayloadError("overlays.media.loopCount 无效")
		}
	}
	return nil
}

func validateTheaterSceneOverlayPresetResources(roomID string, overlays []TheaterSceneOverlayPresetItem) error {
	for _, item := range overlays {
		if item.Media == nil {
			continue
		}
		var count int64
		if err := model.GetDB().Model(&model.TheaterResourceModel{}).Where("room_id = ? AND id = ? AND status = ?", roomID, item.Media.ResourceID, "ready").Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return newTheaterError(TheaterErrorResourceNotReady, "场景预设引用的媒体尚未就绪", 409, map[string]any{"resourceId": item.Media.ResourceID})
		}
	}
	return nil
}

func theaterSceneOverlayPresetFromModel(row *model.TheaterSceneOverlayPresetModel) (*TheaterSceneOverlayPreset, error) {
	input, err := theaterSceneOverlayPresetInputFromModel(row)
	if err != nil {
		return nil, err
	}
	return &TheaterSceneOverlayPreset{ID: row.ID, Name: input.Name, Description: input.Description, Tags: input.Tags, Overlays: input.Overlays, Revision: row.Revision, CreatedBy: row.CreatedBy, UpdatedBy: row.UpdatedBy, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}

func theaterSceneOverlayPresetInputFromModel(row *model.TheaterSceneOverlayPresetModel) (TheaterSceneOverlayPresetInput, error) {
	var input TheaterSceneOverlayPresetInput
	input.Name, input.Description = row.Name, row.Description
	if strings.TrimSpace(row.TagsJSON) != "" && json.Unmarshal([]byte(row.TagsJSON), &input.Tags) != nil {
		return input, theaterPayloadError("场景预设 tags 数据损坏")
	}
	if strings.TrimSpace(row.OverlaysJSON) == "" || json.Unmarshal([]byte(row.OverlaysJSON), &input.Overlays) != nil {
		return input, theaterPayloadError("场景预设 overlays 数据损坏")
	}
	return input, nil
}
