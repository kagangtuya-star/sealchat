package model

import "strings"

// ExternalGlossaryCategoryModel 存储外挂术语库分类（可独立于术语存在）。
type ExternalGlossaryCategoryModel struct {
	StringPKBaseModel
	LibraryID  string `json:"libraryId" gorm:"size:100;index:idx_external_glossary_category_name,priority:1"`
	Name       string `json:"name" gorm:"size:100;index:idx_external_glossary_category_name,priority:2"`
	CreatedBy  string `json:"createdBy" gorm:"size:100"`
	UpdatedBy  string `json:"updatedBy" gorm:"size:100"`
}

func (*ExternalGlossaryCategoryModel) TableName() string { return "external_glossary_categories" }

func (m *ExternalGlossaryCategoryModel) Normalize() {
	m.LibraryID = strings.TrimSpace(m.LibraryID)
	m.Name = strings.TrimSpace(m.Name)
}
