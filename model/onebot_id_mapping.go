package model

import "time"

type OneBotIDMappingModel struct {
	NumericID  int64     `json:"numericId" gorm:"primaryKey;autoIncrement"`
	EntityType string    `json:"entityType" gorm:"size:32;not null;uniqueIndex:udx_onebot_entity,priority:1"`
	EntityID   string    `json:"entityId" gorm:"size:100;not null;uniqueIndex:udx_onebot_entity,priority:2"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (*OneBotIDMappingModel) TableName() string {
	return "onebot_id_mappings"
}
