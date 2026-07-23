package service

import (
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
)

func setTheaterRoomsStatus(tx *gorm.DB, channelIDs []string, status string) error {
	if tx == nil || len(channelIDs) == 0 {
		return nil
	}
	return tx.Model(&model.TheaterRoomModel{}).Where("channel_id IN ?", channelIDs).Updates(map[string]any{"status": status, "updated_at": time.Now()}).Error
}

func archiveTheaterRoomsByWorld(tx *gorm.DB, worldID string) error {
	if tx == nil || strings.TrimSpace(worldID) == "" {
		return nil
	}
	return tx.Model(&model.TheaterRoomModel{}).Where("world_id = ?", worldID).Updates(map[string]any{"status": "archived", "updated_at": time.Now()}).Error
}

func cleanupTheaterChannels(tx *gorm.DB, channelIDs []string) error {
	if tx == nil || len(channelIDs) == 0 {
		return nil
	}
	var rooms []model.TheaterRoomModel
	if err := tx.Where("channel_id IN ?", channelIDs).Find(&rooms).Error; err != nil {
		return err
	}
	roomIDs := make([]string, 0, len(rooms))
	for _, room := range rooms {
		roomIDs = append(roomIDs, room.ID)
	}
	if len(roomIDs) == 0 {
		return nil
	}
	if err := tx.Unscoped().Where("room_id IN ?", roomIDs).Delete(&model.TheaterPanelFolderStateModel{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("room_id IN ?", roomIDs).Delete(&model.TheaterPanelItemModel{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("room_id IN ?", roomIDs).Delete(&model.TheaterPanelFolderModel{}).Error; err != nil {
		return err
	}
	now := time.Now()
	if err := tx.Model(&model.TheaterResourceModel{}).Where("room_id IN ?", roomIDs).Updates(map[string]any{
		"status":             "deleting",
		"reference_count":    0,
		"deleted_at":         &now,
		"cleanup_reason":     theaterResourceCleanupScopeDelete,
		"cleanup_after":      now.Add(theaterResourceDeleteGrace),
		"cleanup_attempts":   0,
		"cleanup_last_error": "",
	}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("room_id IN ?", roomIDs).Delete(&model.TheaterObjectModel{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("room_id IN ?", roomIDs).Delete(&model.TheaterSceneModel{}).Error; err != nil {
		return err
	}
	var snapshotIDs []string
	if err := tx.Model(&model.TheaterSnapshotModel{}).Where("room_id IN ?", roomIDs).Pluck("id", &snapshotIDs).Error; err != nil {
		return err
	}
	if err := deleteTheaterResourceHoldsForSnapshots(tx, snapshotIDs); err != nil {
		return err
	}
	if err := tx.Unscoped().Where("room_id IN ?", roomIDs).Delete(&model.TheaterSnapshotModel{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("room_id IN ?", roomIDs).Delete(&model.TheaterMutationModel{}).Error; err != nil {
		return err
	}
	return tx.Unscoped().Where("id IN ?", roomIDs).Delete(&model.TheaterRoomModel{}).Error
}

func collectChannelTreeIDs(tx *gorm.DB, roots []string) ([]string, error) {
	seen := map[string]struct{}{}
	frontier := make([]string, 0, len(roots))
	for _, id := range roots {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			frontier = append(frontier, id)
		}
	}
	for len(frontier) > 0 {
		var children []string
		if err := tx.Model(&model.ChannelModel{}).Where("parent_id IN ?", frontier).Pluck("id", &children).Error; err != nil {
			return nil, err
		}
		frontier = frontier[:0]
		for _, id := range children {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			frontier = append(frontier, id)
		}
	}
	result := make([]string, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	return result, nil
}
