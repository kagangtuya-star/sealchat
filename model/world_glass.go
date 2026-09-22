package model

import (
	"time"

	"gorm.io/gorm"
)

type WorldGlassPresetModel struct {
	StringPKBaseModel
	WorldID      string `json:"worldId" gorm:"size:100;not null;index:idx_world_glass_order,priority:1"`
	Name         string `json:"name" gorm:"size:80;not null"`
	AttachmentID string `json:"attachmentId" gorm:"size:100;index"`
	SettingsJSON string `json:"-" gorm:"type:text"`
	SortOrder    int    `json:"sortOrder" gorm:"default:0;index:idx_world_glass_order,priority:2"`
	CreatedBy    string `json:"createdBy" gorm:"size:100;index"`
	UpdatedBy    string `json:"updatedBy" gorm:"size:100;index"`
}

func (*WorldGlassPresetModel) TableName() string { return "world_glass_presets" }

type WorldGlassStateModel struct {
	WorldID        string    `json:"worldId" gorm:"primaryKey;size:100"`
	Enabled        bool      `json:"enabled"`
	ActivePresetID string    `json:"activePresetId" gorm:"size:100;index"`
	Revision       uint64    `json:"revision" gorm:"not null;default:0"`
	UpdatedBy      string    `json:"updatedBy" gorm:"size:100"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

func (*WorldGlassStateModel) TableName() string { return "world_glass_states" }

type WorldGlassTriggerModel struct {
	StringPKBaseModel
	WorldID     string `json:"worldId" gorm:"size:100;not null;uniqueIndex:idx_world_glass_trigger,priority:1"`
	PresetID    string `json:"presetId" gorm:"size:100;not null;index"`
	TriggerType string `json:"triggerType" gorm:"size:32;not null;uniqueIndex:idx_world_glass_trigger,priority:2"`
	TriggerKey  string `json:"triggerKey" gorm:"size:120;not null;uniqueIndex:idx_world_glass_trigger,priority:3"`
	ConfigJSON  string `json:"-" gorm:"type:text"`
	Enabled     bool   `json:"enabled"`
	SortOrder   int    `json:"sortOrder" gorm:"default:0"`
	CreatedBy   string `json:"createdBy" gorm:"size:100"`
	UpdatedBy   string `json:"updatedBy" gorm:"size:100"`
}

func (*WorldGlassTriggerModel) TableName() string { return "world_glass_triggers" }

func WorldGlassStateGet(db *gorm.DB, worldID string) (WorldGlassStateModel, error) {
	state := WorldGlassStateModel{WorldID: worldID}
	err := db.Where("world_id = ?", worldID).Find(&state).Error
	return state, err
}

func WorldGlassPresetList(db *gorm.DB, worldID string) ([]WorldGlassPresetModel, error) {
	items := []WorldGlassPresetModel{}
	err := db.Where("world_id = ?", worldID).Order("sort_order ASC, id ASC").Find(&items).Error
	return items, err
}

func WorldGlassTriggerList(db *gorm.DB, worldID string) ([]WorldGlassTriggerModel, error) {
	items := []WorldGlassTriggerModel{}
	err := db.Where("world_id = ?", worldID).Order("sort_order DESC, id ASC").Find(&items).Error
	return items, err
}
