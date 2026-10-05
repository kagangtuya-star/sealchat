package service

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"sealchat/model"
	"sealchat/service/storage"
	"strings"
	"time"
)

func isUnboundMCPAttachment(a *model.AttachmentModel, actorID string) bool {
	return a != nil && a.DeletedAt == nil && a.UserID == actorID && a.IsTemp && a.Extra == "mcp-upload" && a.RootID == "" && a.RootIDType == "" && a.ChannelID == ""
}

// Use the existing hourly cleanup worker. Claim expired unbound attachments
// before removing storage so concurrent business binding cannot adopt a deleted file.
// Soft-deleted claims are retried after storage failures, then removed permanently.
func CleanupMCPTemporaryAttachments(now time.Time) (int64, error) {
	manager := GetStorageManager()
	if manager == nil {
		return 0, nil
	}
	eligible := func(q *gorm.DB) *gorm.DB {
		return q.Unscoped().Where("extra = ? AND is_temp = ? AND created_at < ? AND COALESCE(root_id, '') = '' AND COALESCE(root_id_type, '') = '' AND COALESCE(channel_id, '') = '' AND COALESCE(parent_id, '') = ''", "mcp-upload", true, now.Add(-24*time.Hour))
	}
	var rows []model.AttachmentModel
	if err := eligible(model.GetDB()).Order("created_at ASC, id ASC").Limit(200).Find(&rows).Error; err != nil {
		return 0, errors.New("MCP 临时附件清理查询失败")
	}
	var deleted int64
	for _, att := range rows {
		if att.DeletedAt == nil {
			r := eligible(model.GetDB().Model(&model.AttachmentModel{})).Where("id = ? AND deleted_at IS NULL", att.ID).UpdateColumn("deleted_at", now)
			if r.Error != nil {
				return deleted, errors.New("MCP 临时附件清理认领失败")
			}
			if r.RowsAffected != 1 {
				continue
			}
		}
		var references int64
		if err := model.GetDB().Unscoped().Model(&model.AttachmentModel{}).Where("id <> ? AND object_key = ? AND storage_type = ?", att.ID, att.ObjectKey, att.StorageType).Count(&references).Error; err != nil {
			return deleted, errors.New("MCP 临时附件引用检查失败")
		}
		if references == 0 && att.ObjectKey != "" {
			backend := storage.BackendLocal
			if att.StorageType == model.StorageS3 {
				if !manager.HasRemote() {
					return deleted, errors.New("MCP 临时附件远程存储暂不可用")
				}
				backend = storage.BackendS3
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := manager.Delete(ctx, backend, att.ObjectKey)
			cancel()
			if err != nil {
				return deleted, errors.New("MCP 临时附件存储清理失败")
			}
		}
		r := eligible(model.GetDB()).Where("id = ? AND deleted_at IS NOT NULL", att.ID).Delete(&model.AttachmentModel{})
		if r.Error != nil {
			return deleted, errors.New("MCP 临时附件记录清理失败")
		}
		deleted += r.RowsAffected
	}
	return deleted, nil
}

// Editing locks remain advisory for the browser. An automated editor respects
// live field locks without acquiring or inventing a global lock.
func MCPWorldClueCheckLocks(worldID, clueID, actorID string, fields ...string) error {
	locks, err := WorldClueListEditLocks(worldID, clueID, actorID)
	if err != nil {
		return err
	}
	for _, lock := range locks {
		if lock.ExpireAt <= time.Now().UnixMilli() {
			continue
		}
		for _, field := range fields {
			if field == "*" || field == lock.Field {
				return ErrWorldClueConflict
			}
		}
	}
	return nil
}

func MCPWorldKeywordGet(worldID, keywordID, actorID string) (*model.WorldKeywordModel, error) {
	if err := ensureWorldKeywordPermission(worldID, actorID, false); err != nil {
		return nil, err
	}
	var row model.WorldKeywordModel
	if err := model.GetDB().Where("id = ? AND world_id = ?", keywordID, worldID).First(&row).Error; err != nil {
		return nil, err
	}
	if !row.IsEnabled && !IsWorldAdmin(worldID, actorID) {
		return nil, ErrWorldKeywordNotFound
	}
	return &row, nil
}

func MCPWorldKeywordCanWrite(worldID, actorID string) bool {
	return ensureWorldKeywordPermission(worldID, actorID, true) == nil
}

func MCPCanDeleteStickyNote(note *model.StickyNoteModel, userID string) bool {
	if note == nil {
		return false
	}
	if note.CreatorID == userID {
		return true
	}
	roles, err := model.UserRoleMappingListByUserID(userID, note.ChannelID, "channel")
	if err != nil {
		return false
	}
	for _, role := range roles {
		if strings.HasSuffix(role, "-owner") || strings.HasSuffix(role, "-admin") {
			return true
		}
	}
	return false
}
