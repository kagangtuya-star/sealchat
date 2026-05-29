package model

import (
	"time"

	"gorm.io/gorm"
)

// WorldArchiveModel records per-user hidden worlds without changing membership.
type WorldArchiveModel struct {
	StringPKBaseModel
	WorldID    string    `json:"worldId" gorm:"size:100;uniqueIndex:idx_world_archive_user_world,priority:2;index"`
	UserID     string    `json:"userId" gorm:"size:100;uniqueIndex:idx_world_archive_user_world,priority:1;index"`
	ArchivedAt time.Time `json:"archivedAt"`
}

func (*WorldArchiveModel) TableName() string {
	return "world_archives"
}

func (m *WorldArchiveModel) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.Init()
	}
	if m.ArchivedAt.IsZero() {
		m.ArchivedAt = time.Now()
	}
	return nil
}
