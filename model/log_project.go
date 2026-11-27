package model

import "time"

// LogProjectModel owns the single writable layout document for a world.
type LogProjectModel struct {
	StringPKBaseModel
	WorldID         string `gorm:"size:100;not null;uniqueIndex"`
	Enabled         bool   `gorm:"not null;default:false"`
	DraftVersion    int64  `gorm:"not null;default:1"`
	DraftJSON       string `gorm:"not null"`
	ActiveReleaseID string `gorm:"size:100"`
}

func (*LogProjectModel) TableName() string { return "log_projects" }

// Aliases are never removed: inactive names remain reserved globally.
type LogRouteAliasModel struct {
	Slug          string `gorm:"primaryKey;size:64"`
	ProjectID     string `gorm:"size:100;not null;index:idx_log_alias_project_kind,priority:1"`
	Kind          string `gorm:"size:8;not null;index:idx_log_alias_project_kind,priority:2"`
	Active        bool   `gorm:"not null"`
	InvalidatedAt *time.Time
}

func (*LogRouteAliasModel) TableName() string { return "log_route_aliases" }

type LogMessageExclusionModel struct {
	ProjectID string `gorm:"primaryKey;size:100"`
	SectionID string `gorm:"primaryKey;size:100"`
	MessageID string `gorm:"primaryKey;size:100"`
}

func (*LogMessageExclusionModel) TableName() string { return "log_message_exclusions" }

// LogBookmarkModel is a jump target outside the LOG draft. Legacy rows stay private.
type LogBookmarkModel struct {
	StringPKBaseModel
	ProjectID  string `gorm:"size:100;not null;index:idx_log_bookmark_owner,priority:1"`
	UserID     string `gorm:"size:100;not null;index:idx_log_bookmark_owner,priority:2"`
	SectionID  string `gorm:"size:100;not null"`
	NodeID     string `gorm:"size:100;not null"`
	Label      string `gorm:"size:200;not null"`
	Visibility string `gorm:"size:16;not null;default:private"`
}

func (*LogBookmarkModel) TableName() string { return "log_bookmarks" }
