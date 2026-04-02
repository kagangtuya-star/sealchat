package model

import "strings"

// WorldExternalGlossaryBindingModel 记录世界启用的外挂术语库。
type WorldExternalGlossaryBindingModel struct {
	StringPKBaseModel
	WorldID    string `json:"worldId" gorm:"size:100;uniqueIndex:idx_world_external_glossary_binding,priority:1;index"`
	LibraryID  string `json:"libraryId" gorm:"size:100;uniqueIndex:idx_world_external_glossary_binding,priority:2;index"`
	SortOrder  int    `json:"sortOrder" gorm:"default:0;index"`
	CreatedBy  string `json:"createdBy" gorm:"size:100"`
	UpdatedBy  string `json:"updatedBy" gorm:"size:100"`
}

func (*WorldExternalGlossaryBindingModel) TableName() string { return "world_external_glossary_bindings" }

func (m *WorldExternalGlossaryBindingModel) Normalize() {
	m.WorldID = strings.TrimSpace(m.WorldID)
	m.LibraryID = strings.TrimSpace(m.LibraryID)
}
