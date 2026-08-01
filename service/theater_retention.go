package service

import (
	"context"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
)

const (
	theaterAppliedRetention  = 30 * 24 * time.Hour
	theaterRejectedRetention = 7 * 24 * time.Hour
	theaterAuditRetention    = 180 * 24 * time.Hour
	theaterSnapshotRetention = 30 * 24 * time.Hour
	theaterFailedRetention   = 7 * 24 * time.Hour
	theaterAppliedMinimum    = 10000
	theaterSnapshotMinimum   = 20
)

type TheaterRetentionResult struct {
	AppliedDeleted  int64 `json:"appliedDeleted"`
	RejectedDeleted int64 `json:"rejectedDeleted"`
	AuditDeleted    int64 `json:"auditDeleted"`
	SnapshotDeleted int64 `json:"snapshotDeleted"`
	JobDeleted      int64 `json:"jobDeleted"`
	HoldsDeleted    int64 `json:"holdsDeleted"`
	ResourcesQueued int64 `json:"resourcesQueued"`
	ResourcesGC     int   `json:"resourcesGc"`
}

func (result TheaterRetentionResult) Total() int64 {
	return result.AppliedDeleted + result.RejectedDeleted + result.AuditDeleted + result.SnapshotDeleted + result.JobDeleted + result.HoldsDeleted + result.ResourcesQueued + int64(result.ResourcesGC)
}

func RunTheaterRetention(now time.Time, batch int) (*TheaterRetentionResult, error) {
	if now.IsZero() {
		now = time.Now()
	}
	if batch <= 0 || batch > 2000 {
		batch = 500
	}
	result := &TheaterRetentionResult{}
	var roomIDs []string
	if err := model.GetDB().Model(&model.TheaterRoomModel{}).Pluck("id", &roomIDs).Error; err != nil {
		return result, err
	}
	for _, roomID := range roomIDs {
		var mutationIDs []string
		if err := model.GetDB().Model(&model.TheaterMutationModel{}).
			Where("room_id = ? AND status = ? AND created_at < ?", roomID, "applied", now.Add(-theaterAppliedRetention)).
			Order("revision_after DESC").Offset(theaterAppliedMinimum).Limit(batch).Pluck("id", &mutationIDs).Error; err != nil {
			return result, err
		}
		if len(mutationIDs) > 0 {
			deleted := model.GetDB().Unscoped().Where("id IN ?", mutationIDs).Delete(&model.TheaterMutationModel{})
			if deleted.Error != nil {
				return result, deleted.Error
			}
			result.AppliedDeleted += deleted.RowsAffected
		}
		var snapshotIDs []string
		if err := model.GetDB().Model(&model.TheaterSnapshotModel{}).
			Where("room_id = ? AND kind = ? AND created_at < ?", roomID, "automatic", now.Add(-theaterSnapshotRetention)).
			Order("revision DESC").Offset(theaterSnapshotMinimum).Limit(batch).Pluck("id", &snapshotIDs).Error; err != nil {
			return result, err
		}
		if len(snapshotIDs) > 0 {
			if err := model.GetDB().Transaction(func(tx *gorm.DB) error {
				if err := deleteTheaterResourceHoldsForSnapshots(tx, snapshotIDs); err != nil {
					return err
				}
				deleted := tx.Unscoped().Where("id IN ?", snapshotIDs).Delete(&model.TheaterSnapshotModel{})
				if deleted.Error != nil {
					return deleted.Error
				}
				result.SnapshotDeleted += deleted.RowsAffected
				return nil
			}); err != nil {
				return result, err
			}
		}
	}
	deleted := model.GetDB().Unscoped().Where("status = ? AND created_at < ?", "rejected", now.Add(-theaterRejectedRetention)).Limit(batch).Delete(&model.TheaterMutationModel{})
	if deleted.Error != nil {
		return result, deleted.Error
	}
	result.RejectedDeleted = deleted.RowsAffected
	deleted = model.GetDB().Unscoped().Where("created_at < ?", now.Add(-theaterAuditRetention)).Limit(batch).Delete(&model.TheaterAuditLogModel{})
	if deleted.Error != nil {
		return result, deleted.Error
	}
	result.AuditDeleted = deleted.RowsAffected
	deleted = model.GetDB().Unscoped().Where("status = ? AND created_at < ?", "failed", now.Add(-theaterFailedRetention)).Limit(batch).Delete(&model.TheaterResourceJobModel{})
	if deleted.Error != nil {
		return result, deleted.Error
	}
	result.JobDeleted = deleted.RowsAffected
	deleted = model.GetDB().Unscoped().Where("expires_at IS NOT NULL AND expires_at <= ?", now).Delete(&model.TheaterResourceHoldModel{})
	if deleted.Error != nil {
		return result, deleted.Error
	}
	result.HoldsDeleted = deleted.RowsAffected
	queued := model.GetDB().Model(&model.TheaterResourceModel{}).
		Where("status = ? AND created_at < ?", "failed", now.Add(-theaterFailedRetention)).Limit(batch).
		Updates(map[string]any{
			"status":             "deleting",
			"reference_count":    0,
			"deleted_at":         now,
			"cleanup_reason":     theaterResourceCleanupFailed,
			"cleanup_after":      now.Add(theaterResourceDeleteGrace),
			"cleanup_attempts":   0,
			"cleanup_last_error": "",
		})
	if queued.Error != nil {
		return result, queued.Error
	}
	result.ResourcesQueued = queued.RowsAffected
	gc, err := RunTheaterResourceGC(context.Background(), theaterResourceDeleteGrace, batch)
	if err != nil {
		return result, err
	}
	result.ResourcesGC = gc
	return result, nil
}
