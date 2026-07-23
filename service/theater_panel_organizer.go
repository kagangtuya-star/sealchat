package service

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
)

const (
	TheaterPanelDomainAudio  = "audio"
	TheaterPanelDomainEffect = "effect"
)

type TheaterPanelOrganizerSnapshot struct {
	Folders []model.TheaterPanelFolderModel `json:"folders"`
	Items   []model.TheaterPanelItemModel   `json:"items"`
}

func normalizeTheaterPanelDomain(domain string) (string, error) {
	domain = strings.TrimSpace(domain)
	if domain != TheaterPanelDomainAudio && domain != TheaterPanelDomainEffect {
		return "", theaterPayloadError("domain 无效")
	}
	return domain, nil
}

func requireTheaterPanelWrite(actorID, worldID, channelID, domain string) error {
	if domain == TheaterPanelDomainAudio {
		if !CanManageTheaterResources(actorID, worldID, channelID) {
			return newTheaterError(TheaterErrorPermissionDenied, "没有 Theater 素材管理权限", 403, nil)
		}
		return nil
	}
	_, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionObjectEdit)
	return err
}

func GetTheaterPanelOrganizer(_ context.Context, actorID, worldID, channelID string) (*TheaterPanelOrganizerSnapshot, error) {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionView); err != nil {
		return nil, err
	}
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return nil, err
	}
	var folders []model.TheaterPanelFolderModel
	if err := model.GetDB().Where("room_id = ?", room.ID).Order("domain ASC, sort_order ASC, id ASC").Find(&folders).Error; err != nil {
		return nil, err
	}
	var items []model.TheaterPanelItemModel
	if err := model.GetDB().Where("room_id = ?", room.ID).Order("domain ASC, folder_id ASC, sort_order ASC, id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	var collapsedIDs []string
	if err := model.GetDB().Model(&model.TheaterPanelFolderStateModel{}).
		Where("room_id = ? AND user_id = ? AND collapsed = ?", room.ID, actorID, true).
		Pluck("folder_id", &collapsedIDs).Error; err != nil {
		return nil, err
	}
	collapsed := make(map[string]struct{}, len(collapsedIDs))
	for _, id := range collapsedIDs {
		collapsed[id] = struct{}{}
	}
	for index := range folders {
		_, folders[index].Collapsed = collapsed[folders[index].ID]
	}
	return &TheaterPanelOrganizerSnapshot{Folders: folders, Items: items}, nil
}

func CreateTheaterPanelFolder(_ context.Context, actorID, worldID, channelID, domain, name string) (*model.TheaterPanelFolderModel, error) {
	domain, err := normalizeTheaterPanelDomain(domain)
	if err != nil {
		return nil, err
	}
	if err := requireTheaterPanelWrite(actorID, worldID, channelID, domain); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return nil, err
	}
	if name == "" {
		name, err = nextTheaterPanelFolderName(room.ID, domain)
		if err != nil {
			return nil, err
		}
	}
	if len([]rune(name)) > 128 {
		return nil, theaterPayloadError("文件夹名称长度必须为 1-128")
	}
	var duplicateCount int64
	if err := model.GetDB().Model(&model.TheaterPanelFolderModel{}).Where("room_id = ? AND domain = ? AND name = ?", room.ID, domain, name).Count(&duplicateCount).Error; err != nil {
		return nil, err
	}
	if duplicateCount > 0 {
		return nil, theaterPayloadError("同名文件夹已存在")
	}
	var maximum int64
	if err := model.GetDB().Model(&model.TheaterPanelFolderModel{}).Where("room_id = ? AND domain = ?", room.ID, domain).Select("COALESCE(MAX(sort_order), -1)").Scan(&maximum).Error; err != nil {
		return nil, err
	}
	folder := &model.TheaterPanelFolderModel{RoomID: room.ID, Domain: domain, Name: name, SortOrder: maximum + 1, CreatedBy: actorID, UpdatedBy: actorID}
	folder.Init()
	if err := model.GetDB().Create(folder).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, theaterPayloadError("同名文件夹已存在")
		}
		return nil, err
	}
	return folder, nil
}

func nextTheaterPanelFolderName(roomID, domain string) (string, error) {
	var names []string
	if err := model.GetDB().Model(&model.TheaterPanelFolderModel{}).Where("room_id = ? AND domain = ?", roomID, domain).Pluck("name", &names).Error; err != nil {
		return "", err
	}
	used := make(map[string]struct{}, len(names))
	for _, name := range names {
		used[name] = struct{}{}
	}
	for index := 1; ; index++ {
		name := "新建文件夹"
		if index > 1 {
			name += " " + strconv.Itoa(index)
		}
		if _, exists := used[name]; !exists {
			return name, nil
		}
	}
}

func UpdateTheaterPanelFolder(_ context.Context, actorID, worldID, channelID, folderID, name string) (*model.TheaterPanelFolderModel, error) {
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return nil, err
	}
	folder, err := loadTheaterPanelFolder(room.ID, folderID)
	if err != nil {
		return nil, err
	}
	if err := requireTheaterPanelWrite(actorID, worldID, channelID, folder.Domain); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 128 {
		return nil, theaterPayloadError("文件夹名称长度必须为 1-128")
	}
	var duplicateCount int64
	if err := model.GetDB().Model(&model.TheaterPanelFolderModel{}).Where("room_id = ? AND domain = ? AND name = ? AND id <> ?", room.ID, folder.Domain, name, folder.ID).Count(&duplicateCount).Error; err != nil {
		return nil, err
	}
	if duplicateCount > 0 {
		return nil, theaterPayloadError("同名文件夹已存在")
	}
	if err := model.GetDB().Model(folder).Updates(map[string]any{"name": name, "updated_by": actorID}).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, theaterPayloadError("同名文件夹已存在")
		}
		return nil, err
	}
	folder.Name = name
	folder.UpdatedBy = actorID
	return folder, nil
}

func DeleteTheaterPanelFolder(_ context.Context, actorID, worldID, channelID, folderID string) error {
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return err
	}
	folder, err := loadTheaterPanelFolder(room.ID, folderID)
	if err != nil {
		return err
	}
	if err := requireTheaterPanelWrite(actorID, worldID, channelID, folder.Domain); err != nil {
		return err
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.TheaterPanelItemModel{}).Where("room_id = ? AND folder_id = ?", room.ID, folder.ID).Update("folder_id", "").Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("room_id = ? AND folder_id = ?", room.ID, folder.ID).Delete(&model.TheaterPanelFolderStateModel{}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Where("room_id = ? AND id = ?", room.ID, folder.ID).Delete(&model.TheaterPanelFolderModel{}).Error
	})
}

func SetTheaterPanelFolderCollapsed(_ context.Context, actorID, worldID, channelID, folderID string, collapsed bool) error {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionView); err != nil {
		return err
	}
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return err
	}
	if _, err := loadTheaterPanelFolder(room.ID, folderID); err != nil {
		return err
	}
	if !collapsed {
		return model.GetDB().Unscoped().Where("room_id = ? AND user_id = ? AND folder_id = ?", room.ID, actorID, folderID).Delete(&model.TheaterPanelFolderStateModel{}).Error
	}
	state := model.TheaterPanelFolderStateModel{RoomID: room.ID, UserID: actorID, FolderID: folderID, Collapsed: true}
	state.Init()
	return model.GetDB().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "room_id"}, {Name: "user_id"}, {Name: "folder_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"collapsed", "updated_at"}),
	}).Create(&state).Error
}

func ReorderTheaterPanelFolders(_ context.Context, actorID, worldID, channelID, domain string, folderIDs []string) error {
	domain, err := normalizeTheaterPanelDomain(domain)
	if err != nil {
		return err
	}
	if err := requireTheaterPanelWrite(actorID, worldID, channelID, domain); err != nil {
		return err
	}
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return err
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		for index, id := range uniqueTheaterPanelIDs(folderIDs) {
			result := tx.Model(&model.TheaterPanelFolderModel{}).Where("room_id = ? AND domain = ? AND id = ?", room.ID, domain, id).Update("sort_order", index)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return newTheaterError(TheaterErrorNotFound, "文件夹不存在", 404, nil)
			}
		}
		return nil
	})
}

func ReorderTheaterPanelItems(_ context.Context, actorID, worldID, channelID, domain, folderID string, targetIDs []string) error {
	domain, err := normalizeTheaterPanelDomain(domain)
	if err != nil {
		return err
	}
	if err := requireTheaterPanelWrite(actorID, worldID, channelID, domain); err != nil {
		return err
	}
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return err
	}
	folderID = strings.TrimSpace(folderID)
	if folderID != "" {
		folder, loadErr := loadTheaterPanelFolder(room.ID, folderID)
		if loadErr != nil {
			return loadErr
		}
		if folder.Domain != domain {
			return theaterPayloadError("文件夹类型不匹配")
		}
	}
	ids := uniqueTheaterPanelIDs(targetIDs)
	if err := validateTheaterPanelTargets(room.ID, worldID, channelID, domain, ids); err != nil {
		return err
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		for index, targetID := range ids {
			item := model.TheaterPanelItemModel{RoomID: room.ID, Domain: domain, TargetID: targetID, FolderID: folderID, SortOrder: int64(index)}
			item.Init()
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "room_id"}, {Name: "domain"}, {Name: "target_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"folder_id", "sort_order", "updated_at"}),
			}).Create(&item).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func loadTheaterPanelFolder(roomID, folderID string) (*model.TheaterPanelFolderModel, error) {
	folderID = strings.TrimSpace(folderID)
	if err := validateTheaterID(folderID, "folderId"); err != nil {
		return nil, err
	}
	var folder model.TheaterPanelFolderModel
	if err := model.GetDB().Where("room_id = ? AND id = ?", roomID, folderID).First(&folder).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, newTheaterError(TheaterErrorNotFound, "文件夹不存在", 404, nil)
		}
		return nil, err
	}
	return &folder, nil
}

func uniqueTheaterPanelIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func validateTheaterPanelTargets(roomID, worldID, channelID, domain string, targetIDs []string) error {
	if len(targetIDs) == 0 {
		return nil
	}
	if domain == TheaterPanelDomainEffect {
		var count int64
		if err := model.GetDB().Model(&model.TheaterObjectModel{}).Where("room_id = ? AND kind = ? AND id IN ?", roomID, "effect", targetIDs).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(targetIDs)) {
			return newTheaterError(TheaterErrorNotFound, "部分特效不存在", 404, nil)
		}
		return nil
	}
	var assets []model.AudioAsset
	if err := model.GetDB().Where("id IN ?", targetIDs).Find(&assets).Error; err != nil {
		return err
	}
	if len(assets) != len(targetIDs) {
		return newTheaterError(TheaterErrorResourceNotFound, "部分音频素材不存在", 404, nil)
	}
	channelTag := theaterChannelAudioTag(channelID)
	for index := range assets {
		asset := &assets[index]
		if asset.Scope != model.AudioScopeWorld || asset.WorldID == nil || strings.TrimSpace(*asset.WorldID) != strings.TrimSpace(worldID) || !hasAudioTag(asset, theaterFeatureAudioTag) || !hasAudioTag(asset, channelTag) {
			return newTheaterError(TheaterErrorResourceNotFound, "部分音频素材不属于当前 Theater", 404, nil)
		}
	}
	return nil
}
