package model

import (
	"strings"
	"time"
)

type AnnouncementScopeType string

const (
	AnnouncementScopeWorld AnnouncementScopeType = "world"
	AnnouncementScopeLobby AnnouncementScopeType = "lobby"
)

type AnnouncementStatus string

const (
	AnnouncementStatusDraft     AnnouncementStatus = "draft"
	AnnouncementStatusPublished AnnouncementStatus = "published"
	AnnouncementStatusArchived  AnnouncementStatus = "archived"
)

type AnnouncementContentFormat string

const (
	AnnouncementContentPlain AnnouncementContentFormat = "plain"
	AnnouncementContentRich  AnnouncementContentFormat = "rich"
)

type AnnouncementPopupMode string

const (
	AnnouncementPopupNone           AnnouncementPopupMode = "none"
	AnnouncementPopupOncePerVersion AnnouncementPopupMode = "once_per_version"
	AnnouncementPopupEveryEntry     AnnouncementPopupMode = "every_entry"
)

type AnnouncementReminderScope string

const (
	AnnouncementReminderScopeLobbyOnly AnnouncementReminderScope = "lobby_only"
	AnnouncementReminderScopeSiteWide  AnnouncementReminderScope = "site_wide"
)

type AnnouncementModel struct {
	StringPKBaseModel
	ScopeType     AnnouncementScopeType     `json:"scopeType" gorm:"size:16;index:idx_ann_scope,priority:1"`
	ScopeID       string                    `json:"scopeId" gorm:"size:100;index:idx_ann_scope,priority:2"`
	Title         string                    `json:"title" gorm:"size:120;not null"`
	Content       string                    `json:"content" gorm:"type:text"`
	ContentFormat AnnouncementContentFormat `json:"contentFormat" gorm:"size:16;default:'rich'"`
	Status        AnnouncementStatus        `json:"status" gorm:"size:16;default:'draft';index"`
	IsPinned      bool                      `json:"isPinned" gorm:"default:false;index"`
	PinOrder      int                       `json:"pinOrder" gorm:"default:0;index"`
	ShowInTicker  bool                      `json:"showInTicker" gorm:"default:false;index"`
	PopupMode     AnnouncementPopupMode     `json:"popupMode" gorm:"size:24;default:'none'"`
	ReminderScope AnnouncementReminderScope `json:"reminderScope" gorm:"size:24;default:'lobby_only'"`
	RequireAck    bool                      `json:"requireAck" gorm:"default:false"`
	Version       int                       `json:"version" gorm:"default:1"`
	PublishedAt   *time.Time                `json:"publishedAt"`
	CreatedBy     string                    `json:"createdBy" gorm:"size:100;index"`
	UpdatedBy     string                    `json:"updatedBy" gorm:"size:100;index"`
}

func (*AnnouncementModel) TableName() string { return "announcements" }

func (m *AnnouncementModel) Normalize() {
	m.ScopeID = strings.TrimSpace(m.ScopeID)
	m.Title = strings.TrimSpace(m.Title)
	if m.ScopeType == "" {
		m.ScopeType = AnnouncementScopeWorld
	}
	if m.ContentFormat == "" {
		m.ContentFormat = AnnouncementContentRich
	}
	if m.Status == "" {
		m.Status = AnnouncementStatusDraft
	}
	if m.PopupMode == "" {
		m.PopupMode = AnnouncementPopupNone
	}
	if m.ReminderScope == "" {
		m.ReminderScope = AnnouncementReminderScopeLobbyOnly
	}
	if m.Version <= 0 {
		m.Version = 1
	}
}

type AnnouncementUserStateModel struct {
	StringPKBaseModel
	AnnouncementID  string     `json:"announcementId" gorm:"size:100;uniqueIndex:idx_ann_user_state,priority:1"`
	UserID          string     `json:"userId" gorm:"size:100;uniqueIndex:idx_ann_user_state,priority:2"`
	LastSeenVersion int        `json:"lastSeenVersion" gorm:"default:0"`
	LastPopupAt     *time.Time `json:"lastPopupAt"`
	AckVersion      int        `json:"ackVersion" gorm:"default:0"`
	AckAt           *time.Time `json:"ackAt"`
}

func (*AnnouncementUserStateModel) TableName() string { return "announcement_user_states" }
