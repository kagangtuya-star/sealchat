package model

// ChannelAvatarCardSettingsModel stores the channel's avatar card source and two independent templates.
type ChannelAvatarCardSettingsModel struct {
	StringPKBaseModel
	ChannelID         string `json:"channelId" gorm:"size:100;not null;uniqueIndex:udx_channel_avatar_card_settings"`
	SourceMode        string `json:"sourceMode" gorm:"size:16;not null;default:''"`
	BotTemplateJSON   string `json:"botTemplateJson" gorm:"type:text;not null;default:''"`
	WorldTemplateJSON string `json:"worldTemplateJson" gorm:"type:text;not null;default:''"`
	SchemaVersion     int    `json:"schemaVersion" gorm:"not null;default:1"`
	ServerRevision    int64  `json:"serverRevision" gorm:"not null;default:0"`
	UpdatedBy         string `json:"updatedBy" gorm:"size:100;not null;default:''"`
}

func (*ChannelAvatarCardSettingsModel) TableName() string { return "channel_avatar_card_settings" }

// WorldCharacterStateModel never contains channel or BOT snapshot data.
type WorldCharacterStateModel struct {
	StringPKBaseModel
	WorldID    string `json:"worldId" gorm:"size:100;not null;uniqueIndex:udx_world_character_state,priority:1"`
	SubjectKey string `json:"subjectKey" gorm:"size:120;not null;uniqueIndex:udx_world_character_state,priority:2"`
	AttrsJSON  string `json:"attrsJson" gorm:"type:text;not null;default:'{}'"`
	Revision   int64  `json:"revision" gorm:"not null;default:0"`
	UpdatedBy  string `json:"updatedBy" gorm:"size:100;not null;default:''"`
}

func (*WorldCharacterStateModel) TableName() string { return "world_character_states" }
