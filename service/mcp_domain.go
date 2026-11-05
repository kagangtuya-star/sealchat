package service

import (
	"context"
	"errors"
	"github.com/gabriel-vasile/mimetype"
	"gorm.io/gorm"
	"hash/fnv"
	"io"
	"mime"
	"os"
	"sealchat/model"
	"sealchat/service/storage"
	"sealchat/utils"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

func isUnboundMCPAttachment(a *model.AttachmentModel, actorID string) bool {
	return a != nil && a.DeletedAt == nil && a.UserID == actorID && a.IsTemp && a.Extra == "mcp-upload" && a.RootID == "" && a.RootIDType == "" && a.ChannelID == "" && a.ParentID == "" && a.ParentIDType == "" && !strings.HasPrefix(a.Note, "mcp-consuming:")
}

// All claims use the same predicate, including nullable legacy scope columns.
func mcpUnboundAttachments(q *gorm.DB) *gorm.DB {
	return q.Where("extra = ? AND is_temp = ? AND COALESCE(root_id, '') = '' AND COALESCE(root_id_type, '') = '' AND COALESCE(channel_id, '') = '' AND COALESCE(parent_id, '') = '' AND COALESCE(parent_id_type, '') = ''", "mcp-upload", true)
}

func resolveMCPUpload(tx *gorm.DB, actorID, uploadID string) (*model.AttachmentModel, error) {
	id := strings.TrimPrefix(strings.TrimSpace(uploadID), "id:")
	if strings.TrimSpace(actorID) == "" || id == "" || len(id) > 100 {
		return nil, ErrWorldClueInvalid
	}
	var att model.AttachmentModel
	if err := tx.Where("id = ? AND user_id = ? AND deleted_at IS NULL", id, actorID).First(&att).Error; err != nil {
		return nil, err
	}
	if !isUnboundMCPAttachment(&att, actorID) {
		return nil, ErrWorldClueConflict
	}
	return &att, nil
}

// A larger transport profile must not bypass ordinary attachment limits when
// the target business binds the original bytes as a clue image or avatar.
func validateMCPAttachmentBinding(att *model.AttachmentModel, actorID string) error {
	if !isUnboundMCPAttachment(att, actorID) {
		return ErrWorldClueConflict
	}
	var cfg utils.AppConfig
	if current := utils.GetConfig(); current != nil {
		cfg = *current
	}
	limit, _, _ := MCPUploadLimits(cfg, "attachment")
	if att.Size < 0 || att.Size > limit {
		return ErrWorldClueInvalid
	}
	return nil
}

func claimMCPAttachment(tx *gorm.DB, actorID, uploadID string, fields map[string]any) error {
	r := mcpUnboundAttachments(tx.Model(&model.AttachmentModel{})).Where("id = ? AND user_id = ? AND deleted_at IS NULL AND COALESCE(note, '') NOT LIKE ?", uploadID, actorID, "mcp-consuming:%").Updates(fields)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected != 1 {
		return ErrWorldClueConflict
	}
	return nil
}

// This lightweight quota serializes uploads within one server process. The DB
// sum is authoritative for existing rows; this is not a distributed quota.
var mcpPendingLocks [64]sync.Mutex

func MCPReserveUploadCapacity(actorID string, incoming, maxBytes int64) (func(), error) {
	h := fnv.New32a()
	_, _ = h.Write([]byte(actorID))
	lock := &mcpPendingLocks[h.Sum32()%uint32(len(mcpPendingLocks))]
	lock.Lock()
	var used int64
	err := mcpUnboundAttachments(model.GetDB().Model(&model.AttachmentModel{})).Where("user_id = ? AND deleted_at IS NULL", actorID).Select("COALESCE(SUM(size), 0)").Scan(&used).Error
	if err == nil && (incoming <= 0 || incoming > maxBytes || used > maxBytes-incoming) {
		err = ErrMCPUploadPendingLimit
	}
	if err != nil {
		lock.Unlock()
		return nil, err
	}
	return lock.Unlock, nil
}

var ErrMCPUploadPendingLimit = errors.New("MCP 未绑定临时上传总容量超限")

const MCPTextUploadMaxBytes int64 = 1 << 20

func MCPTextMIME(value string) bool {
	t, _, err := mime.ParseMediaType(value)
	return err == nil && (t == "text/plain" || t == "text/html" || t == "application/xhtml+xml")
}

// A DB compare-and-set reserves a conversion without holding a transaction over
// storage I/O or native worker dispatch. On failure the upload remains reusable;
// on success its independent storage is retired using the cleanup retry path.
// Interrupted conversions remain unavailable to clients and expire via cleanup.
func consumeMCPUpload(ctx context.Context, actorID, uploadID string, maxBytes int64, consume func(*model.AttachmentModel, string) error) error {
	att, err := resolveMCPUpload(model.GetDB(), actorID, uploadID)
	if err != nil {
		return err
	}
	if att.Size <= 0 || att.Size > maxBytes {
		return ErrWorldClueInvalid
	}
	note := "mcp-consuming:" + utils.NewID()
	if err := claimMCPAttachment(model.GetDB(), actorID, att.ID, map[string]any{"note": note}); err != nil {
		return err
	}
	release := func() {
		_ = model.GetDB().Model(&model.AttachmentModel{}).Where("id = ? AND note = ? AND deleted_at IS NULL", att.ID, note).Update("note", att.Note).Error
	}
	if att.StorageType != model.StorageLocal && att.StorageType != model.StorageS3 {
		release()
		return ErrWorldClueInvalid
	}
	path, cleanup, err := resolveTheaterAttachmentPath(ctx, att)
	if err != nil {
		release()
		return errors.New("无法读取 MCP 临时上传")
	}
	defer cleanup()
	info, err := os.Stat(path)
	if err != nil || info.Size() != att.Size || info.Size() > maxBytes {
		release()
		return ErrWorldClueInvalid
	}
	if err := consume(att, path); err != nil {
		release()
		return err
	}
	now := time.Now()
	r := model.GetDB().Model(&model.AttachmentModel{}).Where("id = ? AND note = ? AND deleted_at IS NULL", att.ID, note).Updates(map[string]any{"deleted_at": now, "note": att.Note})
	if r.Error != nil || r.RowsAffected != 1 {
		// Keep the reservation on an ambiguous post-commit failure; never replay.
		return ErrWorldClueConflict
	}
	// A storage failure must not turn a committed business import into a retry.
	_ = deleteMCPTemporaryAttachment(*att)
	return nil
}

func ConsumeMCPTextUpload(ctx context.Context, actorID, uploadID string, maxBytes int64, consume func(string) error) error {
	if maxBytes > MCPTextUploadMaxBytes {
		maxBytes = MCPTextUploadMaxBytes
	}
	return consumeMCPUpload(ctx, actorID, uploadID, maxBytes, func(att *model.AttachmentModel, path string) error {
		if att.Note != "mcp-profile:text" || !MCPTextMIME(att.MimeType) {
			return ErrWorldClueInvalid
		}
		file, err := os.Open(path)
		if err != nil {
			return errors.New("无法读取 MCP 文本上传")
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
		if err != nil || int64(len(data)) > maxBytes || !utf8.Valid(data) || strings.ContainsRune(string(data), '\x00') || !MCPTextMIME(mimetype.Detect(data).String()) {
			return ErrWorldClueInvalid
		}
		return consume(string(data))
	})
}

func DiscardMCPUpload(actorID, uploadID string) error {
	id := strings.TrimSpace(uploadID)
	if id == "" || len(id) > 100 {
		return ErrWorldClueInvalid
	}
	var att model.AttachmentModel
	err := model.GetDB().Unscoped().Where("id = ? AND user_id = ?", id, actorID).First(&att).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	check := att
	check.DeletedAt = nil
	if !isUnboundMCPAttachment(&check, actorID) {
		return ErrWorldClueConflict
	}
	if att.DeletedAt == nil {
		if err := claimMCPAttachment(model.GetDB(), actorID, id, map[string]any{"deleted_at": time.Now()}); err != nil {
			return err
		}
	}
	return deleteMCPTemporaryAttachment(att)
}

func deleteMCPTemporaryAttachment(att model.AttachmentModel) error {
	manager := GetStorageManager()
	if manager == nil {
		return errors.New("MCP 临时附件存储暂不可用")
	}
	var references int64
	if err := model.GetDB().Unscoped().Model(&model.AttachmentModel{}).Where("id <> ? AND object_key = ? AND storage_type = ?", att.ID, att.ObjectKey, att.StorageType).Count(&references).Error; err != nil {
		return errors.New("MCP 临时附件引用检查失败")
	}
	if references == 0 && att.ObjectKey != "" {
		backend := storage.BackendLocal
		if att.StorageType == model.StorageS3 {
			if !manager.HasRemote() {
				return errors.New("MCP 临时附件远程存储暂不可用")
			}
			backend = storage.BackendS3
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := manager.Delete(ctx, backend, att.ObjectKey)
		cancel()
		if err != nil {
			return errors.New("MCP 临时附件存储清理失败")
		}
	}
	if err := mcpUnboundAttachments(model.GetDB().Unscoped()).Where("id = ? AND deleted_at IS NOT NULL", att.ID).Delete(&model.AttachmentModel{}).Error; err != nil {
		return errors.New("MCP 临时附件记录清理失败")
	}
	return nil
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
		return mcpUnboundAttachments(q.Unscoped()).Where("created_at < ? AND (COALESCE(note, '') NOT LIKE ? OR updated_at < ?)", now.Add(-24*time.Hour), "mcp-consuming:%", now.Add(-time.Hour))
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
		if err := deleteMCPTemporaryAttachment(att); err != nil {
			return deleted, err
		}
		deleted++
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
