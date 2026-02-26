package model

import "strings"

// WorldKeywordCategoryModel 存储世界术语分类（可独立于术语存在）。
type WorldKeywordCategoryModel struct {
	StringPKBaseModel
	WorldID   string `json:"worldId" gorm:"size:100;index:idx_world_keyword_category_name,priority:1"`
	Name      string `json:"name" gorm:"size:100;index:idx_world_keyword_category_name,priority:2"`
	CreatedBy string `json:"createdBy" gorm:"size:100"`
	UpdatedBy string `json:"updatedBy" gorm:"size:100"`
}

func (*WorldKeywordCategoryModel) TableName() string { return "world_keyword_categories" }

func (m *WorldKeywordCategoryModel) Normalize() {
	m.WorldID = strings.TrimSpace(m.WorldID)
	m.Name = strings.TrimSpace(m.Name)
}
