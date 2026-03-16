package model

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/utils"
)

type ChannelIdentityModeConfigModel struct {
	StringPKBaseModel
	UserID        string `json:"userId" gorm:"index:idx_identity_mode_config_user_channel,unique;not null"`
	ChannelID     string `json:"channelId" gorm:"index:idx_identity_mode_config_user_channel,unique;not null"`
	ICIdentityID  string `json:"icIdentityId" gorm:"column:ic_identity_id;size:100;not null;default:''"`
	OOCIdentityID string `json:"oocIdentityId" gorm:"column:ooc_identity_id;size:100;not null;default:''"`
}

func (*ChannelIdentityModeConfigModel) TableName() string {
	return "channel_identity_mode_configs"
}

func ChannelIdentityModeConfigGet(userID, channelID string) (*ChannelIdentityModeConfigModel, error) {
	return channelIdentityModeConfigGetWithDB(db, userID, channelID)
}

func channelIdentityModeConfigGetWithDB(conn *gorm.DB, userID, channelID string) (*ChannelIdentityModeConfigModel, error) {
	userID = strings.TrimSpace(userID)
	channelID = strings.TrimSpace(channelID)
	if userID == "" || channelID == "" {
		return nil, nil
	}
	var record ChannelIdentityModeConfigModel
	if err := conn.Where("user_id = ? AND channel_id = ?", userID, channelID).Limit(1).Find(&record).Error; err != nil {
		return nil, err
	}
	if record.ID == "" {
		return nil, nil
	}
	return &record, nil
}

func ChannelIdentityModeConfigUpsert(userID, channelID, icIdentityID, oocIdentityID string) (*ChannelIdentityModeConfigModel, error) {
	return ChannelIdentityModeConfigUpsertTx(db, userID, channelID, icIdentityID, oocIdentityID)
}

func ChannelIdentityModeConfigUpsertTx(conn *gorm.DB, userID, channelID, icIdentityID, oocIdentityID string) (*ChannelIdentityModeConfigModel, error) {
	userID = strings.TrimSpace(userID)
	channelID = strings.TrimSpace(channelID)
	if userID == "" || channelID == "" {
		return nil, nil
	}
	record := &ChannelIdentityModeConfigModel{
		StringPKBaseModel: modelID(),
		UserID:            userID,
		ChannelID:         channelID,
		ICIdentityID:      strings.TrimSpace(icIdentityID),
		OOCIdentityID:     strings.TrimSpace(oocIdentityID),
	}
	if err := conn.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "channel_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"ic_identity_id", "ooc_identity_id", "updated_at"}),
	}).Create(record).Error; err != nil {
		return nil, err
	}
	return channelIdentityModeConfigGetWithDB(conn, userID, channelID)
}

func ChannelIdentityModeConfigDelete(userID, channelID string) error {
	return ChannelIdentityModeConfigDeleteTx(db, userID, channelID)
}

func ChannelIdentityModeConfigDeleteTx(conn *gorm.DB, userID, channelID string) error {
	userID = strings.TrimSpace(userID)
	channelID = strings.TrimSpace(channelID)
	if userID == "" || channelID == "" {
		return nil
	}
	return conn.Where("user_id = ? AND channel_id = ?", userID, channelID).Delete(&ChannelIdentityModeConfigModel{}).Error
}

func ChannelIdentityModeConfigListByChannelTx(conn *gorm.DB, channelID string) ([]ChannelIdentityModeConfigModel, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return []ChannelIdentityModeConfigModel{}, nil
	}
	var items []ChannelIdentityModeConfigModel
	if err := conn.Where("channel_id = ?", channelID).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func ChannelIdentityModeConfigClearIdentityReferences(userID, channelID, identityID string) error {
	return ChannelIdentityModeConfigClearIdentityReferencesTx(db, userID, channelID, identityID)
}

func ChannelIdentityModeConfigClearIdentityReferencesTx(conn *gorm.DB, userID, channelID, identityID string) error {
	userID = strings.TrimSpace(userID)
	channelID = strings.TrimSpace(channelID)
	identityID = strings.TrimSpace(identityID)
	if userID == "" || channelID == "" || identityID == "" {
		return nil
	}
	record, err := channelIdentityModeConfigGetWithDB(conn, userID, channelID)
	if err != nil || record == nil {
		return err
	}
	changed := false
	if record.ICIdentityID == identityID {
		record.ICIdentityID = ""
		changed = true
	}
	if record.OOCIdentityID == identityID {
		record.OOCIdentityID = ""
		changed = true
	}
	if !changed {
		return nil
	}
	if record.ICIdentityID == "" && record.OOCIdentityID == "" {
		return ChannelIdentityModeConfigDeleteTx(conn, userID, channelID)
	}
	return conn.Model(&ChannelIdentityModeConfigModel{}).
		Where("id = ?", record.ID).
		Updates(map[string]any{
			"ic_identity_id":  record.ICIdentityID,
			"ooc_identity_id": record.OOCIdentityID,
		}).Error
}

func modelID() StringPKBaseModel {
	return StringPKBaseModel{ID: utils.NewID()}
}
