package model

import (
	"time"

	"gorm.io/gorm/clause"

	"sealchat/utils"
)

// EmailNotificationSettingsModel 用户邮件通知设置
type EmailNotificationSettingsModel struct {
	StringPKBaseModel
	UserID       string `json:"userId" gorm:"index:idx_email_notif_user_channel,unique"`
	ChannelID    string `json:"channelId" gorm:"index:idx_email_notif_user_channel,unique"` // 空字符串表示全局设置
	Enabled      bool   `json:"enabled"`
	Email        string `json:"email"`
	DelayMinutes int    `json:"delayMinutes"` // 延迟推送时间（分钟）

	// 用户自定义 SMTP 配置（可选）
	UseCustomSMTP    bool   `json:"useCustomSmtp"`
	SMTPHost         string `json:"smtpHost" gorm:"column:smtp_host"`
	SMTPPort         int    `json:"smtpPort" gorm:"column:smtp_port"`
	SMTPUsername     string `json:"smtpUsername" gorm:"column:smtp_username"`
	SMTPPassword     string `json:"-" gorm:"column:smtp_password"` // 不输出到 JSON
	SMTPFromAddress  string `json:"smtpFromAddress" gorm:"column:smtp_from_address"`
	SMTPFromName     string `json:"smtpFromName" gorm:"column:smtp_from_name"`
	SMTPUseTLS       bool   `json:"smtpUseTls" gorm:"column:smtp_use_tls"`
}

func (*EmailNotificationSettingsModel) TableName() string {
	return "email_notification_settings"
}

// EmailNotificationSettingsGet 获取用户对某频道的邮件通知设置
func EmailNotificationSettingsGet(userID, channelID string) (*EmailNotificationSettingsModel, error) {
	var record EmailNotificationSettingsModel
	err := db.Where("user_id = ? AND channel_id = ?", userID, channelID).Limit(1).Find(&record).Error
	if err != nil {
		return nil, err
	}
	if record.ID == "" {
		return nil, nil
	}
	return &record, nil
}

// EmailNotificationSettingsUpsertParams 更新时的参数
type EmailNotificationSettingsUpsertParams struct {
	Email         string
	DelayMinutes  int
	Enabled       bool
	UseCustomSMTP bool
	SMTPHost      string
	SMTPPort      int
	SMTPUsername  string
	SMTPPassword  string
	SMTPFrom      string
	SMTPFromName  string
	SMTPUseTLS    bool
}

// EmailNotificationSettingsUpsert 创建或更新用户邮件通知设置
func EmailNotificationSettingsUpsert(userID, channelID string, params EmailNotificationSettingsUpsertParams) (*EmailNotificationSettingsModel, error) {
	record := &EmailNotificationSettingsModel{
		StringPKBaseModel: StringPKBaseModel{ID: utils.NewID()},
		UserID:            userID,
		ChannelID:         channelID,
		Enabled:           params.Enabled,
		Email:             params.Email,
		DelayMinutes:      params.DelayMinutes,
		UseCustomSMTP:     params.UseCustomSMTP,
		SMTPHost:          params.SMTPHost,
		SMTPPort:          params.SMTPPort,
		SMTPUsername:      params.SMTPUsername,
		SMTPPassword:      params.SMTPPassword,
		SMTPFromAddress:   params.SMTPFrom,
		SMTPFromName:      params.SMTPFromName,
		SMTPUseTLS:        params.SMTPUseTLS,
	}

	updateColumns := []string{
		"enabled", "email", "delay_minutes", "updated_at",
		"use_custom_smtp", "smtp_host", "smtp_port", "smtp_username",
		"smtp_from_address", "smtp_from_name", "smtp_use_tls",
	}
	// 仅当密码非空时更新
	if params.SMTPPassword != "" {
		updateColumns = append(updateColumns, "smtp_password")
	}

	err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "channel_id"}},
		DoUpdates: clause.AssignmentColumns(updateColumns),
	}).Create(record).Error
	if err != nil {
		return nil, err
	}
	// 返回实际记录
	return EmailNotificationSettingsGet(userID, channelID)
}

// ToSMTPConfig 将用户设置转为 SMTPConfig（用于发送邮件）
func (m *EmailNotificationSettingsModel) ToSMTPConfig() utils.SMTPConfig {
	return utils.SMTPConfig{
		Host:        m.SMTPHost,
		Port:        m.SMTPPort,
		Username:    m.SMTPUsername,
		Password:    m.SMTPPassword,
		FromAddress: m.SMTPFromAddress,
		FromName:    m.SMTPFromName,
		UseTLS:      m.SMTPUseTLS,
	}
}

// EmailNotificationSettingsDelete 删除用户邮件通知设置
func EmailNotificationSettingsDelete(userID, channelID string) error {
	return db.Where("user_id = ? AND channel_id = ?", userID, channelID).Delete(&EmailNotificationSettingsModel{}).Error
}

// EmailNotificationSettingsListEnabled 获取所有启用的邮件通知设置
func EmailNotificationSettingsListEnabled() ([]*EmailNotificationSettingsModel, error) {
	var records []*EmailNotificationSettingsModel
	err := db.Where("enabled = ?", true).Find(&records).Error
	return records, err
}

// EmailNotificationLogModel 邮件推送记录（用于限流）
type EmailNotificationLogModel struct {
	StringPKBaseModel
	UserID       string `json:"userId" gorm:"index:idx_email_log_user"`
	ChannelID    string `json:"channelId" gorm:"index"`
	SentAt       int64  `json:"sentAt" gorm:"index"` // 发送时间戳（毫秒）
	MessageCount int    `json:"messageCount"`        // 包含消息数量
}

func (*EmailNotificationLogModel) TableName() string {
	return "email_notification_logs"
}

// EmailNotificationLogCreate 创建邮件推送记录
func EmailNotificationLogCreate(userID, channelID string, messageCount int) error {
	record := &EmailNotificationLogModel{
		StringPKBaseModel: StringPKBaseModel{ID: utils.NewID()},
		UserID:            userID,
		ChannelID:         channelID,
		SentAt:            time.Now().UnixMilli(),
		MessageCount:      messageCount,
	}
	return db.Create(record).Error
}

// EmailNotificationLogCountInLastHour 统计用户在过去一小时内的推送次数
func EmailNotificationLogCountInLastHour(userID string) (int64, error) {
	oneHourAgo := time.Now().Add(-time.Hour).UnixMilli()
	var count int64
	err := db.Model(&EmailNotificationLogModel{}).
		Where("user_id = ? AND sent_at >= ?", userID, oneHourAgo).
		Count(&count).Error
	return count, err
}

// EmailNotificationLogGetLatest 获取用户对某频道最近一次推送记录
func EmailNotificationLogGetLatest(userID, channelID string) (*EmailNotificationLogModel, error) {
	var record EmailNotificationLogModel
	err := db.Where("user_id = ? AND channel_id = ?", userID, channelID).
		Order("sent_at DESC").
		Limit(1).
		Find(&record).Error
	if err != nil {
		return nil, err
	}
	if record.ID == "" {
		return nil, nil
	}
	return &record, nil
}

// EmailNotificationLogCleanup 清理指定天数之前的记录
func EmailNotificationLogCleanup(days int) error {
	cutoff := time.Now().AddDate(0, 0, -days).UnixMilli()
	return db.Where("sent_at < ?", cutoff).Delete(&EmailNotificationLogModel{}).Error
}
