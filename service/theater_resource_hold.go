package service

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
)

func createTheaterResourceHolds(tx *gorm.DB, snapshot *model.TheaterSnapshotModel, expiresAt *time.Time) error {
	if tx == nil || snapshot == nil || snapshot.ID == "" {
		return nil
	}
	counts := map[string]int64{}
	countResourceIDsInJSON(snapshot.SnapshotJSON, counts)
	for resourceID := range counts {
		var exists int64
		if err := tx.Model(&model.TheaterResourceModel{}).
			Where("room_id = ? AND id = ?", snapshot.RoomID, resourceID).
			Count(&exists).Error; err != nil {
			return err
		}
		if exists == 0 {
			continue
		}
		hold := model.TheaterResourceHoldModel{ResourceID: resourceID, SnapshotID: snapshot.ID, ExpiresAt: expiresAt}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&hold).Error; err != nil {
			return err
		}
	}
	return nil
}

func deleteTheaterResourceHoldsForSnapshots(tx *gorm.DB, snapshotIDs []string) error {
	if tx == nil || len(snapshotIDs) == 0 {
		return nil
	}
	return tx.Unscoped().Where("snapshot_id IN ?", snapshotIDs).Delete(&model.TheaterResourceHoldModel{}).Error
}
