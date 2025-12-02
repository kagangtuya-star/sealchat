package model

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// WorldKeywordModel 记录世界重要词语，供前端提示使用。
type WorldKeywordModel struct {
	StringPKBaseModel
	WorldID           string `json:"worldId" gorm:"size:100;index:idx_world_keyword_unique,priority:1"`
	Keyword           string `json:"keyword" gorm:"size:128"`
	KeywordNormalized string `json:"keywordNormalized" gorm:"size:128;index:idx_world_keyword_unique,priority:2"`
	Description       string `json:"description" gorm:"size:1024"`
	CreatedBy         string `json:"createdBy" gorm:"size:100"`
	UpdatedBy         string `json:"updatedBy" gorm:"size:100"`
}

func (*WorldKeywordModel) TableName() string {
	return "world_keywords"
}

func (m *WorldKeywordModel) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.Init()
	}
	m.applyNormalization()
	return nil
}

func (m *WorldKeywordModel) BeforeUpdate(tx *gorm.DB) error {
	m.UpdatedAt = time.Now()
	m.applyNormalization()
	return nil
}

func (m *WorldKeywordModel) applyNormalization() {
	trimmed := strings.TrimSpace(m.Keyword)
	m.Keyword = trimmed
	m.KeywordNormalized = strings.ToLower(trimmed)
}
