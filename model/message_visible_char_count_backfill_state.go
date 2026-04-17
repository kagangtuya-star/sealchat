package model

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	messageVisibleCharCountBackfillStateID                  = "message-visible-char-count-backfill"
	messageVisibleCharCountBackfillStatusIdle               = "idle"
	messageVisibleCharCountBackfillStatusRunning            = "running"
	messageVisibleCharCountBackfillStatusDone               = "done"
	messageVisibleCharCountBackfillStatusFailed             = "failed"
	messageVisibleCharCountBackfillModeBackfillMissing      = "backfill_missing"
	messageVisibleCharCountBackfillModeRebuildAll           = "rebuild_all"
	messageVisibleCharCountBackfillPhaseLegacyZeroMigration = "legacy_zero_migration"
	messageVisibleCharCountBackfillPhaseSentinel            = "sentinel"
	messageVisibleCharCountBackfillPhaseFullRebuild         = "full_rebuild"
)

type MessageVisibleCharCountBackfillState struct {
	StringPKBaseModel
	Status                     string     `json:"status" gorm:"size:32;index"`
	Mode                       string     `json:"mode" gorm:"size:32"`
	Phase                      string     `json:"phase" gorm:"size:64"`
	LastID                     string     `json:"last_id" gorm:"size:100"`
	ProcessedCount             int64      `json:"processed_count" gorm:"not null;default:0"`
	LastError                  string     `json:"last_error" gorm:"type:text"`
	LeaseToken                 string     `json:"lease_token" gorm:"size:64"`
	HeartbeatAt                *time.Time `json:"heartbeat_at" gorm:"index"`
	LegacyMigrationCompletedAt *time.Time `json:"legacy_migration_completed_at"`
	CompletedAt                *time.Time `json:"completed_at"`
}

func (*MessageVisibleCharCountBackfillState) TableName() string {
	return "message_visible_char_count_backfill_state"
}

func ensureMessageVisibleCharCountBackfillState(conn *gorm.DB) error {
	now := time.Now()
	return conn.Clauses(clause.OnConflict{DoNothing: true}).Create(&MessageVisibleCharCountBackfillState{
		StringPKBaseModel: StringPKBaseModel{
			ID:        messageVisibleCharCountBackfillStateID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Status:         messageVisibleCharCountBackfillStatusIdle,
		Mode:           messageVisibleCharCountBackfillModeBackfillMissing,
		Phase:          messageVisibleCharCountBackfillPhaseLegacyZeroMigration,
		ProcessedCount: 0,
	}).Error
}

func MessageVisibleCharCountBackfillStateGet() (*MessageVisibleCharCountBackfillState, error) {
	if db == nil {
		return nil, nil
	}
	var item MessageVisibleCharCountBackfillState
	if err := db.Where("id = ?", messageVisibleCharCountBackfillStateID).Limit(1).Find(&item).Error; err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, nil
	}
	return &item, nil
}
