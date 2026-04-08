package model

import "strings"

type WorldCharacterCardTemplateBindingModel struct {
	StringPKBaseModel
	WorldID    string `json:"worldId" gorm:"size:100;uniqueIndex:idx_world_character_card_template_binding,priority:1;index"`
	TemplateID string `json:"templateId" gorm:"size:100;uniqueIndex:idx_world_character_card_template_binding,priority:2;index"`
	CreatedBy  string `json:"createdBy" gorm:"size:100"`
	UpdatedBy  string `json:"updatedBy" gorm:"size:100"`
}

func (*WorldCharacterCardTemplateBindingModel) TableName() string {
	return "world_character_card_template_bindings"
}

func (m *WorldCharacterCardTemplateBindingModel) Normalize() {
	m.WorldID = strings.TrimSpace(m.WorldID)
	m.TemplateID = strings.TrimSpace(m.TemplateID)
	m.CreatedBy = strings.TrimSpace(m.CreatedBy)
	m.UpdatedBy = strings.TrimSpace(m.UpdatedBy)
}
