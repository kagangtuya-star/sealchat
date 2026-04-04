package model

import "strings"

// ExternalGlossaryLibraryModel 表示平台级外挂术语库。
type ExternalGlossaryLibraryModel struct {
	StringPKBaseModel
	Name        string `json:"name" gorm:"size:120;not null;uniqueIndex"`
	Description string `json:"description" gorm:"size:500"`
	IsEnabled   bool   `json:"isEnabled" gorm:"default:true;index"`
	SortOrder   int    `json:"sortOrder" gorm:"default:0;index"`
	CreatedBy   string `json:"createdBy" gorm:"size:100;index"`
	UpdatedBy   string `json:"updatedBy" gorm:"size:100;index"`
}

func (*ExternalGlossaryLibraryModel) TableName() string { return "external_glossary_libraries" }

func (m *ExternalGlossaryLibraryModel) Normalize() {
	m.Name = strings.TrimSpace(m.Name)
	m.Description = strings.TrimSpace(m.Description)
}
