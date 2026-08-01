package model

import (
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/protocol"
	"sealchat/utils"
)

type WorldMemberDice3DProfileModel struct {
	StringPKBaseModel
	WorldID     string `json:"worldId" gorm:"size:100;uniqueIndex:idx_world_member_dice3d,priority:1;not null"`
	UserID      string `json:"userId" gorm:"size:100;uniqueIndex:idx_world_member_dice3d,priority:2;not null"`
	ProfileJSON string `json:"-" gorm:"type:text;not null"`
	Revision    int64  `json:"revision" gorm:"not null;default:1"`
}

func (*WorldMemberDice3DProfileModel) TableName() string {
	return "world_member_dice3d_profiles"
}

func (m *WorldMemberDice3DProfileModel) GetProfile() protocol.Dice3DMemberProfile {
	var value protocol.Dice3DMemberProfile
	if strings.TrimSpace(m.ProfileJSON) != "" {
		_ = json.Unmarshal([]byte(m.ProfileJSON), &value)
	}
	return value
}

func WorldMemberDice3DProfileGet(worldID, userID string) (*WorldMemberDice3DProfileModel, error) {
	var item WorldMemberDice3DProfileModel
	if err := db.Where("world_id = ? AND user_id = ?", worldID, userID).Limit(1).Find(&item).Error; err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, nil
	}
	return &item, nil
}

func WorldMemberDice3DProfileUpsert(worldID, userID, raw string) (*WorldMemberDice3DProfileModel, error) {
	item := &WorldMemberDice3DProfileModel{
		StringPKBaseModel: StringPKBaseModel{ID: utils.NewID()},
		WorldID:           worldID,
		UserID:            userID,
		ProfileJSON:       raw,
		Revision:          1,
	}
	err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "world_id"}, {Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"profile_json": raw,
			"revision":     gorm.Expr("revision + 1"),
			"updated_at":   time.Now(),
		}),
	}).Create(item).Error
	if err != nil {
		return nil, err
	}
	return WorldMemberDice3DProfileGet(worldID, userID)
}
