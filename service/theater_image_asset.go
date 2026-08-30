package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"sealchat/model"
)

type TheaterImageAssetPublic struct {
	ID         string                          `json:"id"`
	Name       string                          `json:"name"`
	ResourceID string                          `json:"resourceId"`
	Preset     *model.TheaterImageObjectPreset `json:"preset,omitempty"`
	CreatedAt  time.Time                       `json:"createdAt"`
	UpdatedAt  time.Time                       `json:"updatedAt"`
	Resource   TheaterResourcePublic           `json:"resource"`
}

type TheaterImageAssetPatch struct {
	Name       *string
	PresetSet  bool
	PresetJSON []byte
}

func theaterImageAssetPatchUpdates(actorID string, patch TheaterImageAssetPatch) (map[string]any, error) {
	if patch.Name == nil && !patch.PresetSet {
		return nil, theaterPayloadError("没有可更新的图片素材字段")
	}
	updates := map[string]any{"updated_by": actorID}
	if patch.Name != nil {
		if strings.TrimSpace(*patch.Name) == "" {
			return nil, theaterPayloadError("素材名称长度必须为 1-255")
		}
		name, err := theaterImageAssetName(*patch.Name, "")
		if err != nil {
			return nil, err
		}
		updates["name"] = name
	}
	if patch.PresetSet {
		_, presetJSON, err := normalizeTheaterImageObjectPresetJSON(patch.PresetJSON)
		if err != nil {
			return nil, err
		}
		updates["preset_json"] = presetJSON
	}
	return updates, nil
}

func theaterImageAssetPreset(asset model.TheaterImageAssetModel) *model.TheaterImageObjectPreset {
	if strings.TrimSpace(asset.PresetJSON) == "" {
		return nil
	}
	preset, _, err := normalizeTheaterImageObjectPresetJSON([]byte(asset.PresetJSON))
	if err != nil {
		return nil
	}
	return preset
}

func theaterImageAssetName(requestedName, fallback string) (string, error) {
	name := strings.TrimSpace(requestedName)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(strings.TrimSpace(fallback)), filepath.Ext(fallback))
	}
	name = strings.ReplaceAll(strings.ReplaceAll(name, "\r", " "), "\n", " ")
	if name == "" {
		name = "未命名图片"
	}
	if utf8.RuneCountInString(name) > 255 {
		return "", theaterPayloadError("素材名称长度必须为 1-255")
	}
	return name, nil
}

func theaterImageAssetPublic(conn *gorm.DB, asset model.TheaterImageAssetModel) (*TheaterImageAssetPublic, error) {
	var resource model.TheaterResourceModel
	if err := conn.Where("room_id = ? AND id = ?", asset.RoomID, asset.ResourceID).First(&resource).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, newTheaterError(TheaterErrorResourceNotFound, "图片资源不存在", 404, nil)
		}
		return nil, err
	}
	publicResource, err := theaterResourcePublicFromModel(conn, resource)
	if err != nil {
		return nil, err
	}
	return &TheaterImageAssetPublic{
		ID: asset.ID, Name: asset.Name, ResourceID: asset.ResourceID,
		Preset: theaterImageAssetPreset(asset), CreatedAt: asset.CreatedAt, UpdatedAt: asset.UpdatedAt, Resource: publicResource,
	}, nil
}

func ListTheaterImageAssets(_ context.Context, actorID, worldID, channelID string) ([]TheaterImageAssetPublic, error) {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionView); err != nil {
		return nil, err
	}
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return nil, err
	}
	var assets []model.TheaterImageAssetModel
	if err := model.GetDB().Where("room_id = ?", room.ID).Order("created_at ASC, id ASC").Find(&assets).Error; err != nil {
		return nil, err
	}
	items := make([]TheaterImageAssetPublic, 0, len(assets))
	for _, asset := range assets {
		item, publicErr := theaterImageAssetPublic(model.GetDB(), asset)
		if publicErr != nil {
			return nil, publicErr
		}
		items = append(items, *item)
	}
	return items, nil
}

func CreateTheaterImageAsset(_ context.Context, actorID, worldID, channelID, resourceID, requestedName string) (*TheaterImageAssetPublic, error) {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionResourceUpload); err != nil {
		return nil, err
	}
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return nil, err
	}
	resourceID = strings.TrimSpace(resourceID)
	if err := validateTheaterID(resourceID, "resourceId"); err != nil {
		return nil, err
	}
	var resource model.TheaterResourceModel
	if err := model.GetDB().Where("room_id = ? AND id = ?", room.ID, resourceID).First(&resource).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, newTheaterError(TheaterErrorResourceNotFound, "图片资源不存在", 404, nil)
		}
		return nil, err
	}
	if resource.Status != "ready" {
		return nil, newTheaterError(TheaterErrorResourceNotReady, "图片资源尚未 ready", 409, nil)
	}
	if resource.Kind != "static_image" && resource.Kind != "animated_image" {
		return nil, theaterPayloadError("resourceId 必须指向图片资源")
	}
	name, err := theaterImageAssetName(requestedName, resource.OriginalFilename)
	if err != nil {
		return nil, err
	}
	asset := model.TheaterImageAssetModel{RoomID: room.ID, Name: name, ResourceID: resource.ID, CreatedBy: actorID, UpdatedBy: actorID}
	asset.Init()
	if err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&asset).Error; err != nil {
			return err
		}
		var maximum int64
		if err := tx.Model(&model.TheaterPanelItemModel{}).
			Where("room_id = ? AND domain = ? AND folder_id = ''", room.ID, TheaterPanelDomainImage).
			Select("COALESCE(MAX(sort_order), -1)").Scan(&maximum).Error; err != nil {
			return err
		}
		item := model.TheaterPanelItemModel{RoomID: room.ID, Domain: TheaterPanelDomainImage, TargetID: asset.ID, SortOrder: maximum + 1}
		item.Init()
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		return recalculateTheaterResourceReferences(tx, room.ID)
	}); err != nil {
		return nil, err
	}
	return theaterImageAssetPublic(model.GetDB(), asset)
}

func UpdateTheaterImageAsset(_ context.Context, actorID, worldID, channelID, assetID string, patch TheaterImageAssetPatch) (*TheaterImageAssetPublic, error) {
	if patch.Name != nil && !CanManageTheaterResources(actorID, worldID, channelID) {
		return nil, newTheaterError(TheaterErrorPermissionDenied, "没有 Theater 素材管理权限", 403, nil)
	}
	if patch.PresetSet {
		if _, _, permissionErr := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionObjectEdit); permissionErr != nil {
			return nil, permissionErr
		}
	}
	updates, err := theaterImageAssetPatchUpdates(actorID, patch)
	if err != nil {
		return nil, err
	}
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return nil, err
	}
	asset, err := loadTheaterImageAsset(room.ID, assetID)
	if err != nil {
		return nil, err
	}
	if err := model.GetDB().Model(asset).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := model.GetDB().Where("room_id = ? AND id = ?", room.ID, asset.ID).First(asset).Error; err != nil {
		return nil, err
	}
	return theaterImageAssetPublic(model.GetDB(), *asset)
}

func DeleteTheaterImageAsset(_ context.Context, actorID, worldID, channelID, assetID string) error {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionResourceDelete); err != nil {
		return err
	}
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return err
	}
	asset, err := loadTheaterImageAsset(room.ID, assetID)
	if err != nil {
		return err
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("room_id = ? AND domain = ? AND target_id = ?", room.ID, TheaterPanelDomainImage, asset.ID).Delete(&model.TheaterPanelItemModel{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("room_id = ? AND id = ?", room.ID, asset.ID).Delete(&model.TheaterImageAssetModel{}).Error; err != nil {
			return err
		}
		return recalculateTheaterResourceReferences(tx, room.ID)
	})
}

func loadTheaterImageAsset(roomID, assetID string) (*model.TheaterImageAssetModel, error) {
	assetID = strings.TrimSpace(assetID)
	if err := validateTheaterID(assetID, "assetId"); err != nil {
		return nil, err
	}
	var asset model.TheaterImageAssetModel
	if err := model.GetDB().Where("room_id = ? AND id = ?", roomID, assetID).First(&asset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, newTheaterError(TheaterErrorResourceNotFound, "图片素材不存在", 404, nil)
		}
		return nil, err
	}
	return &asset, nil
}
