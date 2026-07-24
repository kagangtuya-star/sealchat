package model

// ChannelCharacterSnapshotSettingsModel stores channel-wide presentation templates.
type ChannelCharacterSnapshotSettingsModel struct {
	StringPKBaseModel
	ChannelID                  string `json:"channelId" gorm:"size:100;not null;uniqueIndex:udx_channel_character_snapshot_settings"`
	BadgeTemplate              string `json:"badgeTemplate" gorm:"type:text;not null;default:''"`
	TheaterOverlayTemplateJSON string `json:"theaterOverlayTemplateJson" gorm:"type:text;not null"`
	SchemaVersion              int    `json:"schemaVersion" gorm:"not null;default:1"`
	ServerRevision             int64  `json:"serverRevision" gorm:"not null;default:0;index"`
	UpdatedBy                  string `json:"updatedBy" gorm:"size:100;not null;default:''"`
}

func (*ChannelCharacterSnapshotSettingsModel) TableName() string {
	return "channel_character_snapshot_settings"
}

// ChannelCharacterSnapshotPreferenceModel stores per-user template overrides.
type ChannelCharacterSnapshotPreferenceModel struct {
	StringPKBaseModel
	ChannelID                  string `json:"channelId" gorm:"size:100;not null;uniqueIndex:udx_channel_character_snapshot_preference,priority:1;index"`
	UserID                     string `json:"userId" gorm:"size:100;not null;uniqueIndex:udx_channel_character_snapshot_preference,priority:2;index"`
	BadgeTemplateMode          string `json:"badgeTemplateMode" gorm:"size:16;not null;default:'inherit'"`
	BadgeTemplate              string `json:"badgeTemplate" gorm:"type:text;not null;default:''"`
	TheaterOverlayTemplateMode string `json:"theaterOverlayTemplateMode" gorm:"size:16;not null;default:'inherit'"`
	TheaterOverlayTemplateJSON string `json:"theaterOverlayTemplateJson" gorm:"type:text;not null"`
	SchemaVersion              int    `json:"schemaVersion" gorm:"not null;default:1"`
	ServerRevision             int64  `json:"serverRevision" gorm:"not null;default:0;index"`
}

func (*ChannelCharacterSnapshotPreferenceModel) TableName() string {
	return "channel_character_snapshot_preferences"
}

// ChannelCharacterSnapshotModel is a persistent read model for one active identity.
type ChannelCharacterSnapshotModel struct {
	StringPKBaseModel
	ChannelID       string `json:"channelId" gorm:"size:100;not null;uniqueIndex:udx_channel_character_snapshot_identity,priority:1;index:idx_channel_character_snapshot_user,priority:1"`
	IdentityID      string `json:"identityId" gorm:"size:100;not null;uniqueIndex:udx_channel_character_snapshot_identity,priority:2"`
	UserID          string `json:"userId" gorm:"size:100;not null;index:idx_channel_character_snapshot_user,priority:2"`
	IsActive        bool   `json:"isActive" gorm:"not null;default:true;index"`
	SourceType      string `json:"sourceType" gorm:"size:32;not null;default:'client'"`
	SourceCardID    string `json:"sourceCardId" gorm:"size:100;not null;default:''"`
	PayloadJSON     string `json:"payloadJson" gorm:"type:text;not null"`
	ContentHash     string `json:"contentHash" gorm:"size:64;not null;index"`
	ServerRevision  int64  `json:"serverRevision" gorm:"not null;default:0;index"`
	SourceUpdatedAt int64  `json:"sourceUpdatedAt" gorm:"not null;default:0"`
	LastSeenAt      int64  `json:"lastSeenAt" gorm:"not null;default:0;index"`
}

func (*ChannelCharacterSnapshotModel) TableName() string {
	return "channel_character_snapshots"
}
