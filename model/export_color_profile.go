package model

import (
	"strings"

	"gorm.io/gorm/clause"

	"sealchat/utils"
)

// ExportColorProfileModel 保存用户在频道内的导出 BBCode 颜色覆盖配置。
type ExportColorProfileModel struct {
	StringPKBaseModel
	UserID     string `json:"userId" gorm:"index:idx_export_color_profile_user_channel,unique;not null"`
	ChannelID  string `json:"channelId" gorm:"index:idx_export_color_profile_user_channel,unique;not null"`
	ColorsJSON string `json:"colorsJson" gorm:"column:colors_json;type:text;not null;default:''"`
}

func (*ExportColorProfileModel) TableName() string {
	return "export_color_profiles"
}

func ExportColorProfileGet(userID, channelID string) (*ExportColorProfileModel, error) {
	userID = strings.TrimSpace(userID)
	channelID = strings.TrimSpace(channelID)
	if userID == "" || channelID == "" {
		return nil, nil
	}
	var record ExportColorProfileModel
	if err := db.Where("user_id = ? AND channel_id = ?", userID, channelID).Limit(1).Find(&record).Error; err != nil {
		return nil, err
	}
	if record.ID == "" {
		return nil, nil
	}
	return &record, nil
}

func ExportColorProfileUpsert(userID, channelID, colorsJSON string) (*ExportColorProfileModel, error) {
	userID = strings.TrimSpace(userID)
	channelID = strings.TrimSpace(channelID)
	if userID == "" || channelID == "" {
		return nil, nil
	}
	record := &ExportColorProfileModel{
		StringPKBaseModel: StringPKBaseModel{ID: utils.NewID()},
		UserID:            userID,
		ChannelID:         channelID,
		ColorsJSON:        colorsJSON,
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "channel_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"colors_json", "updated_at"}),
	}).Create(record).Error; err != nil {
		return nil, err
	}
	return ExportColorProfileGet(userID, channelID)
}

func ExportColorProfileDelete(userID, channelID string) error {
	userID = strings.TrimSpace(userID)
	channelID = strings.TrimSpace(channelID)
	if userID == "" || channelID == "" {
		return nil
	}
	return db.Where("user_id = ? AND channel_id = ?", userID, channelID).Delete(&ExportColorProfileModel{}).Error
}
