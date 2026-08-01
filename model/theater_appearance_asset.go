package model

import "time"

type TheaterAppearanceAssetModel struct {
	StringPKBaseModel
	ChannelID            string     `json:"channelId" gorm:"size:100;not null;index:idx_theater_appearance_scope_status,priority:1;index:idx_theater_appearance_channel_owner,priority:1"`
	OwnerUserID          string     `json:"ownerUserId" gorm:"size:100;not null;index:idx_theater_appearance_scope_status,priority:2;index:idx_theater_appearance_channel_owner,priority:2"`
	IdentityID           string     `json:"identityId" gorm:"size:100;not null;index"`
	VariantID            string     `json:"variantId,omitempty" gorm:"size:100;index"`
	Purpose              string     `json:"purpose" gorm:"size:32;not null"`
	SourceAttachmentID   string     `json:"sourceAttachmentId" gorm:"size:100;not null;uniqueIndex"`
	DisplayAttachmentID  string     `json:"displayAttachmentId,omitempty" gorm:"size:100;index"`
	FallbackAttachmentID string     `json:"fallbackAttachmentId,omitempty" gorm:"size:100;index"`
	Kind                 string     `json:"kind" gorm:"size:32;not null"`
	MimeType             string     `json:"mimeType" gorm:"size:128;not null"`
	SourceMimeType       string     `json:"sourceMimeType" gorm:"size:128;not null"`
	OriginalFilename     string     `json:"originalFilename" gorm:"size:255"`
	SizeBytes            int64      `json:"sizeBytes" gorm:"not null"`
	ContentHash          string     `json:"contentHash" gorm:"size:64;not null;index"`
	Width                int        `json:"width"`
	Height               int        `json:"height"`
	DurationMS           int64      `json:"durationMs,omitempty"`
	Status               string     `json:"status" gorm:"size:16;not null;index:idx_theater_appearance_scope_status,priority:3"`
	Progress             float64    `json:"progress" gorm:"not null;default:0"`
	FailureCode          string     `json:"failureCode,omitempty" gorm:"size:64"`
	FailureMessage       string     `json:"failureMessage,omitempty" gorm:"size:2048"`
	CreatedBy            string     `json:"createdBy" gorm:"size:100;not null;index"`
	ReadyAt              *time.Time `json:"readyAt,omitempty"`
	OrphanedAt           *time.Time `json:"orphanedAt,omitempty" gorm:"index"`
}

func (*TheaterAppearanceAssetModel) TableName() string {
	return "theater_appearance_assets"
}
