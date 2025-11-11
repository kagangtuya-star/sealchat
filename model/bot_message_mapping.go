package model

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// BotMessageMappingModel 维护单个机器人可理解的整型 message_id，与平台内部消息 ID 做映射
type BotMessageMappingModel struct {
	ID                int64     `json:"messageId" gorm:"primaryKey;autoIncrement"`
	BotID             string    `json:"botId" gorm:"index;size:64"`
	ChannelID         string    `json:"channelId" gorm:"size:64"`
	InternalMessageID string    `json:"internalMessageId" gorm:"size:64;index"`
	CreatedAt         time.Time `json:"createdAt"`
}

func (*BotMessageMappingModel) TableName() string {
	return "bot_message_mappings"
}

// EnsureBotMessageMapping 返回（或创建） 特定机器人与内部消息之间的映射
func EnsureBotMessageMapping(botID, channelID, internalMessageID string) (*BotMessageMappingModel, error) {
	botID = strings.TrimSpace(botID)
	internalMessageID = strings.TrimSpace(internalMessageID)
	if botID == "" || internalMessageID == "" {
		return nil, errors.New("botID/internalMessageID required")
	}
	var mapping BotMessageMappingModel
	if err := db.Where("bot_id = ? AND internal_message_id = ?", botID, internalMessageID).First(&mapping).Error; err == nil {
		return &mapping, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	entry := &BotMessageMappingModel{
		BotID:             botID,
		ChannelID:         channelID,
		InternalMessageID: internalMessageID,
		CreatedAt:         time.Now(),
	}
	if err := db.Create(entry).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return EnsureBotMessageMapping(botID, channelID, internalMessageID)
		}
		return nil, err
	}
	return entry, nil
}

func BotMessageMappingGet(botID string, externalID int64) (*BotMessageMappingModel, error) {
	if strings.TrimSpace(botID) == "" || externalID <= 0 {
		return nil, errors.New("invalid params")
	}
	var mapping BotMessageMappingModel
	if err := db.Where("bot_id = ? AND id = ?", botID, externalID).First(&mapping).Error; err != nil {
		return nil, err
	}
	return &mapping, nil
}

func BotMessageMappingGetByInternal(botID, internalMessageID string) (*BotMessageMappingModel, error) {
	if strings.TrimSpace(botID) == "" || strings.TrimSpace(internalMessageID) == "" {
		return nil, errors.New("invalid params")
	}
	var mapping BotMessageMappingModel
	if err := db.Where("bot_id = ? AND internal_message_id = ?", botID, internalMessageID).First(&mapping).Error; err != nil {
		return nil, err
	}
	return &mapping, nil
}
