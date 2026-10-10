package model

import "time"

// PersonalAPIKeyModel deliberately has no world/channel binding or soft-delete hook.
type PersonalAPIKeyModel struct {
	ID         string     `gorm:"primaryKey" json:"id"`
	UserID     string     `gorm:"index;not null" json:"-"`
	Name       string     `gorm:"size:100;not null" json:"name"`
	PublicID   string     `gorm:"size:32;uniqueIndex;not null" json:"publicId"`
	SecretHash string     `gorm:"size:64;not null" json:"-"`
	Tail       string     `gorm:"size:8;not null" json:"tail"`
	Scopes     []string   `gorm:"serializer:json;type:text;not null" json:"scopes"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	ExpiresAt  *time.Time `json:"expiresAt"`
	RevokedAt  *time.Time `json:"revokedAt"`
}

func (*PersonalAPIKeyModel) TableName() string { return "personal_api_keys" }
