package model

import (
	"strings"
	"time"
)

const DefaultOneBotReconnectIntervalMs int64 = 3000

type BotOneBotConfigModel struct {
	BotUserID           string    `json:"botUserId" gorm:"primaryKey;size:100"`
	Enabled             bool      `json:"enabled"`
	URL                 string    `json:"url" gorm:"size:512"`
	APIURL              string    `json:"apiUrl" gorm:"size:512"`
	EventURL            string    `json:"eventUrl" gorm:"size:512"`
	UseUniversalClient  bool      `json:"useUniversalClient"`
	ReconnectIntervalMs int64     `json:"reconnectIntervalMs"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

func (*BotOneBotConfigModel) TableName() string {
	return "bot_onebot_configs"
}

func NormalizeBotOneBotConfig(input *BotOneBotConfigModel) *BotOneBotConfigModel {
	if input == nil {
		return nil
	}
	out := *input
	out.BotUserID = strings.TrimSpace(out.BotUserID)
	out.URL = strings.TrimSpace(out.URL)
	out.APIURL = strings.TrimSpace(out.APIURL)
	out.EventURL = strings.TrimSpace(out.EventURL)
	if out.ReconnectIntervalMs <= 0 {
		out.ReconnectIntervalMs = DefaultOneBotReconnectIntervalMs
	}
	return &out
}

func BotOneBotConfigGet(botUserID string) (*BotOneBotConfigModel, error) {
	botUserID = strings.TrimSpace(botUserID)
	if botUserID == "" {
		return nil, nil
	}
	var item BotOneBotConfigModel
	if err := db.Where("bot_user_id = ?", botUserID).Limit(1).Find(&item).Error; err != nil {
		return nil, err
	}
	if item.BotUserID == "" {
		return nil, nil
	}
	return NormalizeBotOneBotConfig(&item), nil
}

func BotOneBotConfigUpsert(input *BotOneBotConfigModel) (*BotOneBotConfigModel, error) {
	item := NormalizeBotOneBotConfig(input)
	if item == nil || item.BotUserID == "" {
		return nil, nil
	}
	if err := db.Save(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func BotOneBotConfigDelete(botUserID string) error {
	botUserID = strings.TrimSpace(botUserID)
	if botUserID == "" {
		return nil
	}
	return db.Where("bot_user_id = ?", botUserID).Delete(&BotOneBotConfigModel{}).Error
}

func BotOneBotConfigListEnabled() ([]*BotOneBotConfigModel, error) {
	var items []*BotOneBotConfigModel
	if err := db.Where("enabled = ?", true).Find(&items).Error; err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []*BotOneBotConfigModel{}, nil
	}
	result := make([]*BotOneBotConfigModel, 0, len(items))
	for _, item := range items {
		result = append(result, NormalizeBotOneBotConfig(item))
	}
	return result, nil
}
