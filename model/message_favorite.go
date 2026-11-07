package model

const (
	MessageFavoriteScopePersonal = "personal"
	MessageFavoriteScopeWorld    = "world"
)

// MessageFavoriteModel only stores a reference; message content remains in messages.
type MessageFavoriteModel struct {
	StringPKBaseModel
	Scope           string `json:"scope" gorm:"size:16;not null;uniqueIndex:idx_message_favorite_unique,priority:1;index:idx_mf_personal_order,priority:1;index:idx_mf_world_order,priority:1"`
	OwnerUserID     string `json:"ownerUserId" gorm:"size:100;not null;default:'';uniqueIndex:idx_message_favorite_unique,priority:2;index:idx_mf_personal_order,priority:2"`
	WorldID         string `json:"worldId" gorm:"size:100;not null;default:'';uniqueIndex:idx_message_favorite_unique,priority:3;index:idx_mf_world_order,priority:2"`
	ChannelID       string `json:"channelId" gorm:"size:100;not null"`
	MessageID       string `json:"messageId" gorm:"size:100;not null;uniqueIndex:idx_message_favorite_unique,priority:4"`
	SortOrder       int64  `json:"sortOrder" gorm:"not null;index:idx_mf_personal_order,priority:3;index:idx_mf_world_order,priority:3"`
	CreatedByUserID string `json:"createdByUserId" gorm:"size:100;not null"`
}

func (*MessageFavoriteModel) TableName() string { return "message_favorites" }

// Each post has its own author and edit lease. An author may post more than once.
type MessageFavoriteNoteModel struct {
	StringPKBaseModel
	FavoriteID        string `json:"favoriteId" gorm:"size:100;not null;index:idx_mfn_favorite"`
	AuthorUserID      string `json:"authorUserId" gorm:"size:100;not null"`
	Content           string `json:"content" gorm:"size:2000;not null"`
	EditLockTokenHash string `json:"-" gorm:"size:64;not null;default:''"`
	EditLockExpiresAt int64  `json:"-" gorm:"not null;default:0"`
}

func (*MessageFavoriteNoteModel) TableName() string { return "message_favorite_notes" }

type MessageFavoriteTagModel struct {
	StringPKBaseModel
	Scope           string `json:"scope" gorm:"size:16;not null;uniqueIndex:idx_message_favorite_tag_unique,priority:1"`
	OwnerUserID     string `json:"ownerUserId" gorm:"size:100;not null;default:'';uniqueIndex:idx_message_favorite_tag_unique,priority:2"`
	WorldID         string `json:"worldId" gorm:"size:100;not null;default:'';uniqueIndex:idx_message_favorite_tag_unique,priority:3"`
	Name            string `json:"name" gorm:"size:128;not null"`
	NormalizedName  string `json:"-" gorm:"size:128;not null;uniqueIndex:idx_message_favorite_tag_unique,priority:4"`
	CreatedByUserID string `json:"createdByUserId" gorm:"size:100;not null"`
}

func (*MessageFavoriteTagModel) TableName() string { return "message_favorite_tags" }

type MessageFavoriteTagLinkModel struct {
	FavoriteID string `json:"favoriteId" gorm:"size:100;primaryKey"`
	TagID      string `json:"tagId" gorm:"size:100;primaryKey;index:idx_mftl_tag"`
}

func (*MessageFavoriteTagLinkModel) TableName() string { return "message_favorite_tag_links" }
