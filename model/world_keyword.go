package model

import (
	"strings"
)

// WorldKeywordMatchMode 表示匹配模式。
type WorldKeywordMatchMode string

const (
	WorldKeywordMatchPlain WorldKeywordMatchMode = "plain"
	WorldKeywordMatchRegex WorldKeywordMatchMode = "regex"
)

// WorldKeywordDisplayStyle 控制前端展示。
type WorldKeywordDisplayStyle string

const (
	WorldKeywordDisplayStandard WorldKeywordDisplayStyle = "standard"
	WorldKeywordDisplayMinimal  WorldKeywordDisplayStyle = "minimal"
	WorldKeywordDisplayInherit  WorldKeywordDisplayStyle = "inherit"
)

// WorldKeywordModel 存储世界术语配置。
type WorldKeywordModel struct {
	StringPKBaseModel
	WorldID     string                   `json:"worldId" gorm:"size:100;index:idx_world_keyword,priority:1"`
	Keyword     string                   `json:"keyword" gorm:"size:120;index:idx_world_keyword,priority:2"`
	Category    string                   `json:"category" gorm:"size:100;index:idx_world_keyword_category"`
	Aliases     JSONList[string]         `json:"aliases" gorm:"type:json"`
	MatchMode   WorldKeywordMatchMode    `json:"matchMode" gorm:"size:16;default:'plain'"`
	Description string                   `json:"description" gorm:"type:text"`
	Display     WorldKeywordDisplayStyle `json:"display" gorm:"size:24;default:'inherit'"`
	SortOrder   int                      `json:"sortOrder" gorm:"default:0;index:idx_world_keyword_sort"`
	IsEnabled   bool                     `json:"isEnabled" gorm:"default:true"`
	CreatedBy   string                   `json:"createdBy" gorm:"size:100"`
	UpdatedBy   string                   `json:"updatedBy" gorm:"size:100"`
	MatchedVia  string                   `json:"matchedVia,omitempty" gorm:"-"`
}

func (*WorldKeywordModel) TableName() string { return "world_keywords" }

// Normalize 对关键字段做裁剪。
func (m *WorldKeywordModel) Normalize() {
	m.WorldID = strings.TrimSpace(m.WorldID)
	m.Keyword = strings.TrimSpace(m.Keyword)
	if m.MatchMode == "" {
		m.MatchMode = WorldKeywordMatchPlain
	}
	if m.Display == "" {
		m.Display = WorldKeywordDisplayInherit
	}
}
