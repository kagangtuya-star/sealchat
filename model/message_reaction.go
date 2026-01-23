package model

type MessageReactionModel struct {
	StringPKBaseModel
	MessageID string `json:"message_id" gorm:"size:100;uniqueIndex:idx_message_reaction_unique,priority:1"`
	UserID    string `json:"user_id" gorm:"size:100;uniqueIndex:idx_message_reaction_unique,priority:2"`
	Emoji     string `json:"emoji" gorm:"size:100;uniqueIndex:idx_message_reaction_unique,priority:3"`
	IdentityID string `json:"identity_id" gorm:"size:100;index"`
}

func (*MessageReactionModel) TableName() string {
	return "message_reactions"
}

type MessageReactionCountModel struct {
	StringPKBaseModel
	MessageID string `json:"message_id" gorm:"size:100;uniqueIndex:idx_message_reaction_count_unique,priority:1"`
	Emoji     string `json:"emoji" gorm:"size:100;uniqueIndex:idx_message_reaction_count_unique,priority:2"`
	Count     int    `json:"count" gorm:"default:0"`
}

func (*MessageReactionCountModel) TableName() string {
	return "message_reaction_counts"
}

type MessageReactionListItem struct {
	Emoji     string `json:"emoji"`
	Count     int    `json:"count"`
	MeReacted bool   `json:"meReacted"`
}
