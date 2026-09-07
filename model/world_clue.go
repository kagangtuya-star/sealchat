package model

import "time"

const (
	WorldClueKindText   = "text"
	WorldClueKindImage  = "image"
	WorldClueKindIframe = "iframe"

	WorldClueContentPlain  = "plain"
	WorldClueContentTipTap = "tiptap"

	WorldClueAccessInherit = "inherit"
	WorldClueAccessNone    = "none"
	WorldClueAccessView    = "view"
	WorldClueAccessEdit    = "edit"

	WorldClueStatusDraft     = "draft"
	WorldClueStatusPublished = "published"
	WorldClueStatusArchived  = "archived"

	WorldClueFolderShared   = "shared"
	WorldClueFolderPersonal = "personal"
)

// WorldClueModel stores world-scoped shared clue content. API responses use
// dedicated DTOs so manager-only fields never leak through model serialization.
type WorldClueModel struct {
	StringPKBaseModel
	WorldID           string     `json:"-" gorm:"size:100;not null;index:idx_world_clue_order,priority:1;index"`
	SharedFolderID    string     `json:"-" gorm:"size:100;index"`
	Title             string     `json:"-" gorm:"size:255;not null"`
	Kind              string     `json:"-" gorm:"size:16;not null;default:'text'"`
	ContentFormat     string     `json:"-" gorm:"size:16;not null;default:'plain'"`
	Content           string     `json:"-" gorm:"type:text"`
	ContentText       string     `json:"-" gorm:"type:text"`
	ImageAttachmentID string     `json:"-" gorm:"size:100;index"`
	ImageURL          string     `json:"-" gorm:"type:text"`
	EmbedURL          string     `json:"-" gorm:"type:text"`
	PresentationJSON  string     `json:"-" gorm:"type:text"`
	DefaultAccess     string     `json:"-" gorm:"size:16;not null;default:'view'"`
	Status            string     `json:"-" gorm:"size:16;not null;default:'draft';index"`
	Revision          int64      `json:"-" gorm:"not null;default:1"`
	PublishSeq        int64      `json:"-" gorm:"not null;default:0"`
	OrderIndex        int        `json:"-" gorm:"not null;default:0;index:idx_world_clue_order,priority:2"`
	ManagerNoteFormat string     `json:"-" gorm:"size:16;not null;default:'plain'"`
	ManagerNote       string     `json:"-" gorm:"type:text"`
	ManagerNoteText   string     `json:"-" gorm:"type:text"`
	CreatorID         string     `json:"-" gorm:"size:100;not null;index"`
	UpdatedBy         string     `json:"-" gorm:"size:100;not null"`
	PublishedBy       string     `json:"-" gorm:"size:100"`
	PublishedAt       *time.Time `json:"-" gorm:"index"`
}

func (*WorldClueModel) TableName() string { return "world_clues" }

// WorldClueEditLockModel stores one temporary lease per editable clue field.
// It intentionally lives in its own table so multiple fields can be locked at once.
type WorldClueEditLockModel struct {
	StringPKBaseModel
	WorldID   string    `json:"-" gorm:"size:100;not null;index:idx_world_clue_edit_lock_scope,priority:1;index;uniqueIndex:idx_world_clue_edit_lock_unique,priority:1"`
	ClueID    string    `json:"-" gorm:"size:100;not null;index:idx_world_clue_edit_lock_scope,priority:2;index;uniqueIndex:idx_world_clue_edit_lock_unique,priority:2"`
	Field     string    `json:"-" gorm:"size:160;not null;uniqueIndex:idx_world_clue_edit_lock_unique,priority:3"`
	UserID    string    `json:"-" gorm:"size:100;not null;index"`
	SessionID string    `json:"-" gorm:"size:160;not null"`
	ExpireAt  time.Time `json:"-" gorm:"not null;index"`
}

func (*WorldClueEditLockModel) TableName() string { return "world_clue_edit_locks" }

type WorldClueAccessModel struct {
	StringPKBaseModel
	WorldID              string `json:"-" gorm:"size:100;not null;index;uniqueIndex:idx_world_clue_access,priority:1"`
	ClueID               string `json:"-" gorm:"size:100;not null;index;uniqueIndex:idx_world_clue_access,priority:2"`
	UserID               string `json:"-" gorm:"size:100;not null;index;uniqueIndex:idx_world_clue_access,priority:3"`
	AccessOverride       string `json:"-" gorm:"size:16;not null;default:'inherit'"`
	PrivateContentFormat string `json:"-" gorm:"size:16;not null;default:'plain'"`
	PrivateContent       string `json:"-" gorm:"type:text"`
	PrivateContentText   string `json:"-" gorm:"type:text"`
	PrivateRevision      int64  `json:"-" gorm:"not null;default:0"`
	UpdatedBy            string `json:"-" gorm:"size:100"`
}

func (*WorldClueAccessModel) TableName() string { return "world_clue_access" }

type WorldClueRosterMemberModel struct {
	StringPKBaseModel
	WorldID    string `json:"-" gorm:"size:100;not null;index;uniqueIndex:idx_world_clue_roster_member,priority:1"`
	UserID     string `json:"-" gorm:"size:100;not null;uniqueIndex:idx_world_clue_roster_member,priority:2"`
	OrderIndex int    `json:"-" gorm:"not null;default:0"`
}

func (*WorldClueRosterMemberModel) TableName() string { return "world_clue_roster_members" }

type WorldClueFolderModel struct {
	StringPKBaseModel
	WorldID     string `json:"-" gorm:"size:100;not null;index:idx_world_clue_folder_order,priority:1;index"`
	Scope       string `json:"-" gorm:"size:16;not null;index:idx_world_clue_folder_order,priority:2"`
	OwnerUserID string `json:"-" gorm:"size:100;index:idx_world_clue_folder_order,priority:3;index"`
	ParentID    string `json:"-" gorm:"size:100;index:idx_world_clue_folder_order,priority:4;index"`
	Name        string `json:"-" gorm:"size:120;not null"`
	Color       string `json:"-" gorm:"size:32"`
	OrderIndex  int    `json:"-" gorm:"not null;default:0;index:idx_world_clue_folder_order,priority:5"`
	IsDeleted   bool   `json:"-" gorm:"not null;default:false;index"`
}

func (*WorldClueFolderModel) TableName() string { return "world_clue_folders" }

type WorldClueUserStateModel struct {
	StringPKBaseModel
	WorldID                  string     `json:"-" gorm:"size:100;not null;index;uniqueIndex:idx_world_clue_user_state,priority:1"`
	ClueID                   string     `json:"-" gorm:"size:100;not null;index;uniqueIndex:idx_world_clue_user_state,priority:2"`
	UserID                   string     `json:"-" gorm:"size:100;not null;index;uniqueIndex:idx_world_clue_user_state,priority:3"`
	PersonalFolderID         string     `json:"-" gorm:"size:100;index"`
	PersonalOrder            int        `json:"-" gorm:"not null;default:0"`
	Favorite                 bool       `json:"-" gorm:"not null;default:false"`
	SeenRevision             int64      `json:"-" gorm:"not null;default:0"`
	SeenPrivateRevision      int64      `json:"-" gorm:"not null;default:0"`
	AssignedPresentationSeq  int64      `json:"-" gorm:"not null;default:0;index"`
	PresentedPresentationSeq int64      `json:"-" gorm:"not null;default:0"`
	LastOpenedAt             *time.Time `json:"-"`
}

func (*WorldClueUserStateModel) TableName() string { return "world_clue_user_states" }
