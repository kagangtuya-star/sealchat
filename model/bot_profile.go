package model

import (
	"errors"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"
)

type BotConnectionMode string

const (
	BotConnectionModeForwardWS BotConnectionMode = "forward_ws"
	BotConnectionModeReverseWS BotConnectionMode = "reverse_ws"
)

func normalizeBotConnectionMode(mode string) BotConnectionMode {
	switch strings.ToLower(mode) {
	case string(BotConnectionModeReverseWS):
		return BotConnectionModeReverseWS
	default:
		return BotConnectionModeForwardWS
	}
}

type BotProfileModel struct {
	StringPKBaseModel
	Name                 string            `json:"name"`
	AvatarURL            string            `json:"avatarUrl"`
	ChannelRoleName      string            `json:"channelRoleName"`
	UserID               string            `json:"userId" gorm:"index"`
	RemoteSelfID         string            `json:"remoteSelfId"`
	ConnMode             BotConnectionMode `json:"connMode" gorm:"index"`
	ForwardHost          string            `json:"forwardHost"`
	ForwardPort          int               `json:"forwardPort"`
	ForwardAPIPath       string            `json:"forwardApiPath"`
	ForwardEventPath     string            `json:"forwardEventPath"`
	ForwardUniversal     string            `json:"forwardUniversal"`
	ReverseAPIEndpoints  JSONStringSlice   `json:"reverseApiEndpoints" gorm:"type:text"`
	ReverseEventURLs     JSONStringSlice   `json:"reverseEventUrls" gorm:"type:text"`
	ReverseUniversalURLs JSONStringSlice   `json:"reverseUniversalUrls" gorm:"type:text"`
	ReverseUseUniversal  bool              `json:"reverseUseUniversal"`
	ReverseReconnectSec  int               `json:"reverseReconnectSec"`
	AccessToken          string            `json:"accessToken"`
	DefaultChannelID     string            `json:"defaultChannelId"`
	Enabled              bool              `json:"enabled" gorm:"index"`
	CreatedBy            string            `json:"createdBy"`
	UpdatedBy            string            `json:"updatedBy"`
	LastOnlineAt         *time.Time        `json:"lastOnlineAt"`
	LastError            string            `json:"lastError"`

	BotTokens []BotTokenModel `json:"botTokens,omitempty" gorm:"-"`
	Status    string          `json:"status" gorm:"-"`
}

func (*BotProfileModel) TableName() string {
	return "bot_profiles"
}

func (m *BotProfileModel) BeforeSave(tx *gorm.DB) (err error) {
	m.ConnMode = normalizeBotConnectionMode(string(m.ConnMode))
	m.RemoteSelfID = strings.TrimSpace(m.RemoteSelfID)
	if strings.TrimSpace(m.ForwardAPIPath) == "" {
		m.ForwardAPIPath = "/onebot/ws/api"
	}
	if strings.TrimSpace(m.ForwardEventPath) == "" {
		m.ForwardEventPath = "/onebot/ws/event"
	}
	if strings.TrimSpace(m.ForwardUniversal) == "" {
		m.ForwardUniversal = "/onebot/ws/"
	}
	return nil
}

type BotChannelBindingModel struct {
	StringPKBaseModel
	BotID           string `json:"botId" gorm:"index:idx_bot_channel,unique"`
	ChannelID       string `json:"channelId" gorm:"index:idx_bot_channel,unique"`
	RemoteChannelID string `json:"remoteChannelId"`
	RemoteGuildID   string `json:"remoteGuildId"`
	RemoteGroupID   string `json:"remoteGroupId"`
	RemoteNumericID string `json:"remoteNumericId"`
	Enabled         bool   `json:"enabled"`
	IsDefault       bool   `json:"isDefault" gorm:"index"`
	CreatedBy       string `json:"createdBy"`
	UpdatedBy       string `json:"updatedBy"`
}

func (*BotChannelBindingModel) TableName() string {
	return "bot_channel_bindings"
}

func BotProfileList() ([]*BotProfileModel, error) {
	var items []*BotProfileModel
	if err := db.Order("created_at asc").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func BotProfileGet(id string) (*BotProfileModel, error) {
	var item BotProfileModel
	err := db.Where("id = ?", id).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func BotProfileGetByUserID(userID string) (*BotProfileModel, error) {
	var item BotProfileModel
	err := db.Where("user_id = ?", userID).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func BotProfileSave(profile *BotProfileModel) error {
	if profile == nil {
		return errors.New("profile is nil")
	}
	profile.ConnMode = normalizeBotConnectionMode(string(profile.ConnMode))
	if profile.ID == "" {
		profile.Init()
		return db.Create(profile).Error
	}
	return db.Model(&BotProfileModel{}).
		Where("id = ?", profile.ID).
		Updates(profile).Error
}

func BotProfileDelete(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("id required")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&BotProfileModel{}, "id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Where("bot_id = ?", id).Delete(&BotChannelBindingModel{}).Error; err != nil {
			return err
		}
		return nil
	})
}

func BotChannelBindingsByChannelID(channelID string) ([]*BotChannelBindingModel, error) {
	var items []*BotChannelBindingModel
	if err := db.Where("channel_id = ?", channelID).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func BotChannelBindingsByBotID(botID string) ([]*BotChannelBindingModel, error) {
	var items []*BotChannelBindingModel
	if err := db.Where("bot_id = ?", botID).Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func BotChannelBindingUpsert(binding *BotChannelBindingModel) error {
	if binding == nil {
		return errors.New("binding is nil")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if binding.IsDefault {
			if err := tx.Model(&BotChannelBindingModel{}).
				Where("channel_id = ?", binding.ChannelID).
				Update("is_default", false).Error; err != nil {
				return err
			}
		}
		if binding.ID == "" {
			binding.Init()
			return tx.Create(binding).Error
		}
		return tx.Model(&BotChannelBindingModel{}).
			Where("id = ?", binding.ID).
			Updates(binding).Error
	})
}

func BotChannelBindingDelete(bindingID string) error {
	if strings.TrimSpace(bindingID) == "" {
		return errors.New("bindingID required")
	}
	return db.Delete(&BotChannelBindingModel{}, "id = ?", bindingID).Error
}

func BotChannelBindingDeleteByChannel(channelID string) error {
	if strings.TrimSpace(channelID) == "" {
		return errors.New("channelID required")
	}
	return db.Where("channel_id = ?", channelID).Delete(&BotChannelBindingModel{}).Error
}

func BotChannelBindingDeleteByBot(botID string) error {
	if strings.TrimSpace(botID) == "" {
		return errors.New("botID required")
	}
	return db.Where("bot_id = ?", botID).Delete(&BotChannelBindingModel{}).Error
}

func (m *BotProfileModel) NumericSelfID() string {
	if m == nil {
		return ""
	}
	if digits := extractDigits(m.RemoteSelfID); digits != "" {
		return digits
	}
	return extractDigits(m.UserID)
}

func extractDigits(val string) string {
	trimmed := strings.TrimSpace(val)
	if trimmed == "" {
		return ""
	}
	var builder strings.Builder
	for _, r := range trimmed {
		if unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
