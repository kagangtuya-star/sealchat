package model

import "time"

// MCPOAuthGrantModel is an MCP-only credential, independent of login tokens and PATs.
// Secrets are never persisted or serialized in plaintext.
type MCPOAuthGrantModel struct {
	ID                string     `gorm:"primaryKey" json:"-"`
	UserID            string     `gorm:"index;not null" json:"-"`
	ClientID          string     `gorm:"type:text;not null" json:"-"`
	Resource          string     `gorm:"type:text;not null" json:"-"`
	Issuer            string     `gorm:"type:text;not null" json:"-"`
	Scopes            []string   `gorm:"serializer:json;type:text;not null" json:"-"`
	AccessPublicID    string     `gorm:"size:32;uniqueIndex;not null" json:"-"`
	AccessSecretHash  string     `gorm:"size:64;not null" json:"-"`
	AccessExpiresAt   time.Time  `json:"-"`
	RefreshPublicID   string     `gorm:"size:32;uniqueIndex;not null" json:"-"`
	RefreshSecretHash string     `gorm:"size:64;not null" json:"-"`
	RefreshExpiresAt  time.Time  `gorm:"index" json:"-"`
	RevokedAt         *time.Time `json:"-"`
	LastUsedAt        *time.Time `json:"-"`
	CreatedAt         time.Time  `json:"-"`
	UpdatedAt         time.Time  `json:"-"`
}

func (*MCPOAuthGrantModel) TableName() string { return "mcp_oauth_grants" }
