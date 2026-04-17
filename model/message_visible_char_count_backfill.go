package model

import (
	"errors"
	"log"
	"strings"
	"time"

	"sealchat/pkg/contentstats"
	"sealchat/utils"

	"gorm.io/gorm"
)

const (
	messageVisibleCharCountBatchSize        = 500
	messageVisibleCharCountBackfillLeaseTTL = 5 * time.Minute
)

var messageVisibleCharCountBackfillWorkerRunner = runMessageVisibleCharCountBackfillWorker

var ErrMessageVisibleCharCountRebuildRunning = errors.New("message visible char count rebuild is already running")

func BackfillMessageVisibleCharCount() error {
	return runMessageVisibleCharCountBackfillWorker(db)
}

func RebuildMessageVisibleCharCount() error {
	return rebuildMessageVisibleCharCount(db)
}

func StartMessageVisibleCharCountBackfillWorker() {
	conn := db
	if conn == nil {
		return
	}
	runner := messageVisibleCharCountBackfillWorkerRunner
	go func() {
		if err := runner(conn); err != nil {
			log.Printf("回填消息可见字数失败: %v", err)
		}
	}()
}

func rebuildMessageVisibleCharCount(conn *gorm.DB) error {
	if conn == nil {
		return nil
	}

	now := time.Now()
	leaseToken := utils.NewID()
	acquired, err := acquireMessageVisibleCharCountRebuildLease(conn, now, leaseToken)
	if err != nil {
		return err
	}
	if !acquired {
		return ErrMessageVisibleCharCountRebuildRunning
	}

	lastID := ""
	for {
		batch, hasMore, err := loadMessageVisibleCharCountRebuildBatch(conn, lastID, messageVisibleCharCountBatchSize)
		if err != nil {
			markMessageVisibleCharCountBackfillFailed(conn, time.Now(), leaseToken, err)
			return err
		}
		if len(batch) == 0 {
			break
		}
		if err := applyMessageVisibleCharCountBackfillBatch(conn, batch); err != nil {
			markMessageVisibleCharCountBackfillFailed(conn, time.Now(), leaseToken, err)
			return err
		}

		lastID = batch[len(batch)-1].ID
		if err := updateMessageVisibleCharCountBackfillState(conn, leaseToken, map[string]any{
			"status":          messageVisibleCharCountBackfillStatusRunning,
			"mode":            messageVisibleCharCountBackfillModeRebuildAll,
			"phase":           messageVisibleCharCountBackfillPhaseFullRebuild,
			"last_id":         lastID,
			"processed_count": gorm.Expr("processed_count + ?", len(batch)),
			"heartbeat_at":    time.Now(),
			"updated_at":      time.Now(),
		}); err != nil {
			markMessageVisibleCharCountBackfillFailed(conn, time.Now(), leaseToken, err)
			return err
		}

		if !hasMore {
			break
		}
	}

	finishedAt := time.Now()
	if err := updateMessageVisibleCharCountBackfillState(conn, leaseToken, map[string]any{
		"status":                        messageVisibleCharCountBackfillStatusDone,
		"mode":                          messageVisibleCharCountBackfillModeRebuildAll,
		"phase":                         messageVisibleCharCountBackfillPhaseFullRebuild,
		"last_id":                       "",
		"lease_token":                   "",
		"last_error":                    "",
		"heartbeat_at":                  finishedAt,
		"completed_at":                  finishedAt,
		"legacy_migration_completed_at": finishedAt,
		"updated_at":                    finishedAt,
	}); err != nil {
		return err
	}
	return nil
}

func runMessageVisibleCharCountBackfillWorker(conn *gorm.DB) error {
	if conn == nil {
		return nil
	}
	leaseToken := utils.NewID()
	for {
		more, err := runMessageVisibleCharCountBackfillBatch(conn, time.Now(), leaseToken, messageVisibleCharCountBatchSize)
		if err != nil {
			return err
		}
		if !more {
			return nil
		}
	}
}

func acquireMessageVisibleCharCountRebuildLease(conn *gorm.DB, now time.Time, leaseToken string) (bool, error) {
	if err := ensureMessageVisibleCharCountBackfillState(conn); err != nil {
		return false, err
	}

	expiredBefore := now.Add(-messageVisibleCharCountBackfillLeaseTTL)
	tx := conn.Model(&MessageVisibleCharCountBackfillState{}).
		Where("id = ?", messageVisibleCharCountBackfillStateID).
		Where("(status <> ? OR heartbeat_at IS NULL OR heartbeat_at < ?)", messageVisibleCharCountBackfillStatusRunning, expiredBefore).
		Updates(map[string]any{
			"status":          messageVisibleCharCountBackfillStatusRunning,
			"mode":            messageVisibleCharCountBackfillModeRebuildAll,
			"phase":           messageVisibleCharCountBackfillPhaseFullRebuild,
			"last_id":         "",
			"processed_count": 0,
			"heartbeat_at":    now,
			"lease_token":     leaseToken,
			"last_error":      "",
			"completed_at":    nil,
			"updated_at":      now,
		})
	if tx.Error != nil {
		return false, tx.Error
	}
	return tx.RowsAffected > 0, nil
}

func runMessageVisibleCharCountBackfillBatch(conn *gorm.DB, now time.Time, leaseToken string, batchSize int) (bool, error) {
	if conn == nil {
		return false, nil
	}
	if batchSize <= 0 {
		batchSize = messageVisibleCharCountBatchSize
	}

	state, acquired, err := acquireMessageVisibleCharCountBackfillLease(conn, now, leaseToken)
	if err != nil {
		return false, err
	}
	if !acquired {
		return false, nil
	}

	phase := resolveMessageVisibleCharCountBackfillPhase(state)
	batch, hasMoreInPhase, err := loadMessageVisibleCharCountBackfillBatch(conn, phase, state.LastID, batchSize)
	if err != nil {
		markMessageVisibleCharCountBackfillFailed(conn, now, leaseToken, err)
		return false, err
	}
	if len(batch) == 0 {
		return advanceMessageVisibleCharCountBackfillState(conn, phase, now, leaseToken)
	}

	if err := applyMessageVisibleCharCountBackfillBatch(conn, batch); err != nil {
		markMessageVisibleCharCountBackfillFailed(conn, now, leaseToken, err)
		return false, err
	}

	lastID := batch[len(batch)-1].ID
	updates := map[string]any{
		"status":          messageVisibleCharCountBackfillStatusRunning,
		"mode":            messageVisibleCharCountBackfillModeBackfillMissing,
		"phase":           phase,
		"last_id":         lastID,
		"processed_count": gorm.Expr("processed_count + ?", len(batch)),
		"heartbeat_at":    now,
		"last_error":      "",
		"completed_at":    nil,
		"updated_at":      now,
	}

	if phase == messageVisibleCharCountBackfillPhaseLegacyZeroMigration && !hasMoreInPhase {
		updates["phase"] = messageVisibleCharCountBackfillPhaseSentinel
		updates["last_id"] = ""
		updates["legacy_migration_completed_at"] = now
	}
	if phase == messageVisibleCharCountBackfillPhaseSentinel && !hasMoreInPhase {
		updates["status"] = messageVisibleCharCountBackfillStatusDone
		updates["last_id"] = ""
		updates["completed_at"] = now
		updates["lease_token"] = ""
	}

	if err := updateMessageVisibleCharCountBackfillState(conn, leaseToken, updates); err != nil {
		markMessageVisibleCharCountBackfillFailed(conn, now, leaseToken, err)
		return false, err
	}

	if phase == messageVisibleCharCountBackfillPhaseLegacyZeroMigration && !hasMoreInPhase {
		return true, nil
	}
	return phase != messageVisibleCharCountBackfillPhaseSentinel || hasMoreInPhase, nil
}

func acquireMessageVisibleCharCountBackfillLease(conn *gorm.DB, now time.Time, leaseToken string) (*MessageVisibleCharCountBackfillState, bool, error) {
	if err := ensureMessageVisibleCharCountBackfillState(conn); err != nil {
		return nil, false, err
	}

	expiredBefore := now.Add(-messageVisibleCharCountBackfillLeaseTTL)
	tx := conn.Model(&MessageVisibleCharCountBackfillState{}).
		Where("id = ?", messageVisibleCharCountBackfillStateID).
		Where("(status <> ? OR heartbeat_at IS NULL OR heartbeat_at < ? OR lease_token = ?)", messageVisibleCharCountBackfillStatusRunning, expiredBefore, leaseToken).
		Updates(map[string]any{
			"status":       messageVisibleCharCountBackfillStatusRunning,
			"mode":         messageVisibleCharCountBackfillModeBackfillMissing,
			"heartbeat_at": now,
			"lease_token":  leaseToken,
			"last_error":   "",
			"completed_at": nil,
			"updated_at":   now,
		})
	if tx.Error != nil {
		return nil, false, tx.Error
	}

	state, err := getMessageVisibleCharCountBackfillState(conn)
	if err != nil {
		return nil, false, err
	}
	return state, tx.RowsAffected > 0, nil
}

func loadMessageVisibleCharCountBackfillBatch(conn *gorm.DB, phase string, lastID string, batchSize int) ([]MessageModel, bool, error) {
	var batch []MessageModel
	query := conn.
		Model(&MessageModel{}).
		Select("id", "content", "visible_char_count").
		Order("id ASC").
		Limit(batchSize + 1)

	switch phase {
	case messageVisibleCharCountBackfillPhaseLegacyZeroMigration:
		query = query.Where("visible_char_count = ?", 0)
	default:
		query = query.Where("visible_char_count = ?", -1)
	}
	if lastID != "" {
		query = query.Where("id > ?", lastID)
	}

	if err := query.Find(&batch).Error; err != nil {
		return nil, false, err
	}
	hasMoreInPhase := len(batch) > batchSize
	if hasMoreInPhase {
		batch = batch[:batchSize]
	}
	return batch, hasMoreInPhase, nil
}

func loadMessageVisibleCharCountRebuildBatch(conn *gorm.DB, lastID string, batchSize int) ([]MessageModel, bool, error) {
	var batch []MessageModel
	query := conn.
		Model(&MessageModel{}).
		Select("id", "content", "visible_char_count").
		Order("id ASC").
		Limit(batchSize + 1)

	if lastID != "" {
		query = query.Where("id > ?", lastID)
	}

	if err := query.Find(&batch).Error; err != nil {
		return nil, false, err
	}
	hasMore := len(batch) > batchSize
	if hasMore {
		batch = batch[:batchSize]
	}
	return batch, hasMore, nil
}

func applyMessageVisibleCharCountBackfillBatch(conn *gorm.DB, batch []MessageModel) error {
	for _, item := range batch {
		nextCount := contentstats.CountVisibleTextChars(item.Content)
		if item.VisibleCharCount == nextCount {
			continue
		}
		if err := conn.Model(&MessageModel{}).Where("id = ?", item.ID).Update("visible_char_count", nextCount).Error; err != nil {
			return err
		}
	}
	return nil
}

func advanceMessageVisibleCharCountBackfillState(conn *gorm.DB, phase string, now time.Time, leaseToken string) (bool, error) {
	updates := map[string]any{
		"heartbeat_at": now,
		"updated_at":   now,
		"last_error":   "",
	}

	if phase == messageVisibleCharCountBackfillPhaseLegacyZeroMigration {
		updates["status"] = messageVisibleCharCountBackfillStatusRunning
		updates["phase"] = messageVisibleCharCountBackfillPhaseSentinel
		updates["last_id"] = ""
		updates["completed_at"] = nil
		updates["legacy_migration_completed_at"] = now
		if err := updateMessageVisibleCharCountBackfillState(conn, leaseToken, updates); err != nil {
			return false, err
		}
		return true, nil
	}

	updates["status"] = messageVisibleCharCountBackfillStatusDone
	updates["phase"] = messageVisibleCharCountBackfillPhaseSentinel
	updates["last_id"] = ""
	updates["completed_at"] = now
	updates["lease_token"] = ""
	if err := updateMessageVisibleCharCountBackfillState(conn, leaseToken, updates); err != nil {
		return false, err
	}
	return false, nil
}

func updateMessageVisibleCharCountBackfillState(conn *gorm.DB, leaseToken string, updates map[string]any) error {
	query := conn.Model(&MessageVisibleCharCountBackfillState{}).Where("id = ?", messageVisibleCharCountBackfillStateID)
	if leaseToken != "" {
		query = query.Where("lease_token = ?", leaseToken)
	}
	return query.Updates(updates).Error
}

func markMessageVisibleCharCountBackfillFailed(conn *gorm.DB, now time.Time, leaseToken string, err error) {
	if conn == nil || err == nil {
		return
	}
	_ = updateMessageVisibleCharCountBackfillState(conn, leaseToken, map[string]any{
		"status":       messageVisibleCharCountBackfillStatusFailed,
		"last_error":   strings.TrimSpace(err.Error()),
		"heartbeat_at": now,
		"completed_at": nil,
		"lease_token":  "",
		"updated_at":   now,
	})
}

func resolveMessageVisibleCharCountBackfillPhase(state *MessageVisibleCharCountBackfillState) string {
	if state == nil || state.LegacyMigrationCompletedAt == nil {
		return messageVisibleCharCountBackfillPhaseLegacyZeroMigration
	}
	return messageVisibleCharCountBackfillPhaseSentinel
}

func getMessageVisibleCharCountBackfillState(conn *gorm.DB) (*MessageVisibleCharCountBackfillState, error) {
	var item MessageVisibleCharCountBackfillState
	if err := conn.Where("id = ?", messageVisibleCharCountBackfillStateID).Limit(1).Find(&item).Error; err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, nil
	}
	return &item, nil
}
