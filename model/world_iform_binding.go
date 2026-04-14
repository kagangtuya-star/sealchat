package model

import "strings"

// WorldIFormBindingModel 记录世界共享到各频道的 iForm 引用关系。
type WorldIFormBindingModel struct {
	StringPKBaseModel
	WorldID   string `json:"worldId" gorm:"size:100;uniqueIndex:idx_world_iform_binding,priority:1;index"`
	FormID    string `json:"formId" gorm:"size:100;uniqueIndex:idx_world_iform_binding,priority:2;index"`
	CreatedBy string `json:"createdBy" gorm:"size:100"`
	UpdatedBy string `json:"updatedBy" gorm:"size:100"`
}

func (*WorldIFormBindingModel) TableName() string {
	return "world_iform_bindings"
}

func (m *WorldIFormBindingModel) Normalize() {
	m.WorldID = strings.TrimSpace(m.WorldID)
	m.FormID = strings.TrimSpace(m.FormID)
	m.CreatedBy = strings.TrimSpace(m.CreatedBy)
	m.UpdatedBy = strings.TrimSpace(m.UpdatedBy)
}
