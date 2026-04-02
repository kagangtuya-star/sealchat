package model

import "strings"

// ExternalGlossaryTermModel 存储外挂术语库中的术语条目。
type ExternalGlossaryTermModel struct {
	StringPKBaseModel
	LibraryID         string                   `json:"libraryId" gorm:"size:100;index:idx_external_glossary_term,priority:1"`
	Keyword           string                   `json:"keyword" gorm:"size:120;index:idx_external_glossary_term,priority:2"`
	Category          string                   `json:"category" gorm:"size:100;index:idx_external_glossary_term_category"`
	Aliases           JSONList[string]         `json:"aliases" gorm:"type:json"`
	MatchMode         WorldKeywordMatchMode    `json:"matchMode" gorm:"size:16;default:'plain'"`
	Description       string                   `json:"description" gorm:"type:text"`
	DescriptionFormat WorldKeywordDescFormat   `json:"descriptionFormat" gorm:"size:16;default:'plain'"`
	Display           WorldKeywordDisplayStyle `json:"display" gorm:"size:24;default:'inherit'"`
	SortOrder         int                      `json:"sortOrder" gorm:"default:0;index:idx_external_glossary_term_sort"`
	IsEnabled         bool                     `json:"isEnabled" gorm:"default:true"`
	CreatedBy         string                   `json:"createdBy" gorm:"size:100"`
	UpdatedBy         string                   `json:"updatedBy" gorm:"size:100"`
}

func (*ExternalGlossaryTermModel) TableName() string { return "external_glossary_terms" }

func (m *ExternalGlossaryTermModel) Normalize() {
	m.LibraryID = strings.TrimSpace(m.LibraryID)
	m.Keyword = strings.TrimSpace(m.Keyword)
	m.Category = strings.TrimSpace(m.Category)
	if m.MatchMode == "" {
		m.MatchMode = WorldKeywordMatchPlain
	}
	if m.DescriptionFormat == "" {
		m.DescriptionFormat = WorldKeywordDescPlain
	}
	if m.Display == "" {
		m.Display = WorldKeywordDisplayInherit
	}
}
