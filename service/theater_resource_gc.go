package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
)

const (
	theaterResourceDeleteGrace = 7 * 24 * time.Hour
	theaterResourcePurgeLease  = 15 * time.Minute

	theaterResourceCleanupOrphan      = "orphan"
	theaterResourceCleanupExplicit    = "explicit"
	theaterResourceCleanupFailed      = "failed"
	theaterResourceCleanupScopeDelete = "scope_deleted"
)

func RecalculateTheaterResourceReferences(roomID string) error {
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		return recalculateTheaterResourceReferences(tx, roomID)
	})
}

func RunTheaterResourceGC(ctx context.Context, grace time.Duration, limit int) (int, error) {
	if grace <= 0 {
		grace = theaterResourceDeleteGrace
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	now := time.Now()
	legacyCutoff := now.Add(-grace)
	var resources []model.TheaterResourceModel
	if err := model.GetDB().
		Where("reference_count = 0").
		Where("(status = ? AND ((cleanup_after IS NOT NULL AND cleanup_after <= ?) OR (cleanup_after IS NULL AND deleted_at IS NOT NULL AND deleted_at < ?))) OR (status = ? AND ((cleanup_after IS NOT NULL AND cleanup_after <= ?) OR (cleanup_after IS NULL AND updated_at < ?)))", "deleting", now, legacyCutoff, "purging", now, legacyCutoff).
		Order("COALESCE(cleanup_after, deleted_at, updated_at) ASC").
		Limit(limit).
		Find(&resources).Error; err != nil {
		return 0, err
	}

	deleted := 0
	var cleanupErrors []error
	for _, resource := range resources {
		select {
		case <-ctx.Done():
			return deleted, errors.Join(append(cleanupErrors, ctx.Err())...)
		default:
		}
		held, err := theaterResourceHasActiveHold(model.GetDB(), resource.ID, now)
		if err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("resource %s hold check: %w", resource.ID, err))
			continue
		}
		if held {
			continue
		}
		claim := model.GetDB().Model(&model.TheaterResourceModel{}).
			Where("id = ? AND reference_count = 0", resource.ID).
			Where("(status = ? AND ((cleanup_after IS NOT NULL AND cleanup_after <= ?) OR (cleanup_after IS NULL AND deleted_at IS NOT NULL AND deleted_at < ?))) OR (status = ? AND ((cleanup_after IS NOT NULL AND cleanup_after <= ?) OR (cleanup_after IS NULL AND updated_at < ?)))", "deleting", now, legacyCutoff, "purging", now, legacyCutoff).
			Updates(map[string]any{"status": "purging", "cleanup_after": now.Add(theaterResourcePurgeLease), "updated_at": now})
		if claim.Error != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("resource %s claim: %w", resource.ID, claim.Error))
			continue
		}
		if claim.RowsAffected != 1 {
			continue
		}
		resource.Status = "purging"
		if err := deleteTheaterResourcePhysical(ctx, resource); err != nil {
			resetTheaterResourceCleanup(resource.ID, err)
			cleanupErrors = append(cleanupErrors, fmt.Errorf("resource %s purge: %w", resource.ID, err))
			continue
		}
		deleted++
	}
	return deleted, errors.Join(cleanupErrors...)
}

func deleteTheaterResourcePhysical(ctx context.Context, resource model.TheaterResourceModel) error {
	var variants []model.TheaterResourceVariantModel
	if err := model.GetDB().Where("resource_id = ?", resource.ID).Find(&variants).Error; err != nil {
		return err
	}
	attachmentIDs := uniqueTheaterAttachmentIDs(resource, variants)
	attachments, err := removableTheaterAttachments(model.GetDB(), resource.ID, attachmentIDs)
	if err != nil {
		return err
	}
	physicalAttachments, err := unsharedTheaterAttachmentObjects(model.GetDB(), attachments)
	if err != nil {
		return err
	}
	manager := GetStorageManager()
	for _, attachment := range physicalAttachments {
		if strings.TrimSpace(attachment.ObjectKey) == "" {
			continue
		}
		if manager == nil {
			return errors.New("存储服务未初始化")
		}
		if err := manager.Delete(ctx, convertModelToBackend(attachment.StorageType), attachment.ObjectKey); err != nil {
			return err
		}
	}

	now := time.Now()
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		var current model.TheaterResourceModel
		if err := tx.Where("id = ?", resource.ID).Limit(1).Find(&current).Error; err != nil {
			return err
		}
		if current.ID == "" {
			return nil
		}
		if current.Status != "purging" || current.ReferenceCount != 0 {
			return errors.New("资源清理状态已变化")
		}
		held, err := theaterResourceHasActiveHold(tx, resource.ID, now)
		if err != nil {
			return err
		}
		if held {
			return errors.New("资源仍被快照保留")
		}
		if err := tx.Unscoped().Where("resource_id = ?", resource.ID).Delete(&model.TheaterResourceVariantModel{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("resource_id = ?", resource.ID).Delete(&model.TheaterResourceJobModel{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("resource_id = ?", resource.ID).Delete(&model.TheaterResourceHoldModel{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("id = ? AND status = ? AND reference_count = 0", resource.ID, "purging").Delete(&model.TheaterResourceModel{}).Error; err != nil {
			return err
		}
		for _, attachment := range attachments {
			var resourceRefs int64
			if err := tx.Model(&model.TheaterResourceModel{}).Where("attachment_id = ?", attachment.ID).Count(&resourceRefs).Error; err != nil {
				return err
			}
			var variantRefs int64
			if err := tx.Model(&model.TheaterResourceVariantModel{}).Where("attachment_id = ?", attachment.ID).Count(&variantRefs).Error; err != nil {
				return err
			}
			if resourceRefs+variantRefs == 0 {
				if err := tx.Unscoped().Where("id = ?", attachment.ID).Delete(&model.AttachmentModel{}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func uniqueTheaterAttachmentIDs(resource model.TheaterResourceModel, variants []model.TheaterResourceVariantModel) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(variants)+1)
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	add(resource.AttachmentID)
	for _, variant := range variants {
		add(variant.AttachmentID)
	}
	return result
}

func removableTheaterAttachments(db *gorm.DB, resourceID string, attachmentIDs []string) ([]model.AttachmentModel, error) {
	result := make([]model.AttachmentModel, 0, len(attachmentIDs))
	for _, attachmentID := range attachmentIDs {
		var resourceRefs int64
		if err := db.Model(&model.TheaterResourceModel{}).Where("attachment_id = ? AND id <> ?", attachmentID, resourceID).Count(&resourceRefs).Error; err != nil {
			return nil, err
		}
		var variantRefs int64
		if err := db.Model(&model.TheaterResourceVariantModel{}).Where("attachment_id = ? AND resource_id <> ?", attachmentID, resourceID).Count(&variantRefs).Error; err != nil {
			return nil, err
		}
		if resourceRefs+variantRefs > 0 {
			continue
		}
		var attachment model.AttachmentModel
		if err := db.Where("id = ?", attachmentID).Limit(1).Find(&attachment).Error; err != nil {
			return nil, err
		}
		if attachment.ID != "" {
			result = append(result, attachment)
		}
	}
	return result, nil
}

func unsharedTheaterAttachmentObjects(db *gorm.DB, attachments []model.AttachmentModel) ([]model.AttachmentModel, error) {
	removingIDs := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		removingIDs = append(removingIDs, attachment.ID)
	}
	seen := map[string]struct{}{}
	result := make([]model.AttachmentModel, 0, len(attachments))
	for _, attachment := range attachments {
		objectKey := strings.TrimSpace(attachment.ObjectKey)
		if objectKey == "" {
			continue
		}
		identity := string(attachment.StorageType) + "\x00" + objectKey
		if _, ok := seen[identity]; ok {
			continue
		}
		seen[identity] = struct{}{}
		query := db.Model(&model.AttachmentModel{}).Where("storage_type = ? AND object_key = ?", attachment.StorageType, objectKey)
		if len(removingIDs) > 0 {
			query = query.Where("id NOT IN ?", removingIDs)
		}
		var otherRows int64
		if err := query.Count(&otherRows).Error; err != nil {
			return nil, err
		}
		if otherRows == 0 {
			result = append(result, attachment)
		}
	}
	return result, nil
}

func theaterResourceHasActiveHold(db *gorm.DB, resourceID string, now time.Time) (bool, error) {
	var count int64
	err := db.Model(&model.TheaterResourceHoldModel{}).
		Where("resource_id = ? AND (expires_at IS NULL OR expires_at > ?)", resourceID, now).
		Count(&count).Error
	return count > 0, err
}

func resetTheaterResourceCleanup(resourceID string, cleanupErr error) {
	message := ""
	if cleanupErr != nil {
		message = cleanupErr.Error()
		if len(message) > 2048 {
			message = message[:2048]
		}
	}
	_ = model.GetDB().Model(&model.TheaterResourceModel{}).
		Where("id = ? AND status = ?", resourceID, "purging").
		Updates(map[string]any{
			"status":             "deleting",
			"cleanup_attempts":   gorm.Expr("cleanup_attempts + 1"),
			"cleanup_last_error": message,
			"cleanup_after":      time.Now().Add(time.Hour),
		}).Error
}
