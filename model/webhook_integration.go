package model

import (
	"encoding/json"
	"strings"
	"time"

	"sealchat/utils"
)

const (
	WebhookIntegrationStatusActive  = "active"
	WebhookIntegrationStatusRevoked = "revoked"
)

type ChannelWebhookIntegrationModel struct {
	StringPKBaseModel
	ChannelID         string `json:"channelId" gorm:"size:100;index"`
	Name              string `json:"name" gorm:"size:64"`
	BotUserID         string `json:"botUserId" gorm:"size:100;index"`
	Source            string `json:"source" gorm:"size:32;index"`
	CapabilitiesJSON  string `json:"capabilitiesJson" gorm:"type:text"`
	Status            string `json:"status" gorm:"size:16;index"`
	CreatedBy         string `json:"createdBy" gorm:"size:100"`
	LastUsedAt        int64  `json:"lastUsedAt"`
	TokenTailFragment string `json:"tokenTailFragment" gorm:"size:12"`
}

func (*ChannelWebhookIntegrationModel) TableName() string {
	return "channel_webhook_integrations"
}

func (m *ChannelWebhookIntegrationModel) InitDefault() {
	if m.ID == "" {
		m.ID = utils.NewID()
	}
	if strings.TrimSpace(m.Status) == "" {
		m.Status = WebhookIntegrationStatusActive
	}
}

func (m *ChannelWebhookIntegrationModel) Capabilities() []string {
	raw := strings.TrimSpace(m.CapabilitiesJSON)
	if raw == "" {
		return []string{}
	}
	var caps []string
	_ = json.Unmarshal([]byte(raw), &caps)
	// 去重与 trim
	set := map[string]struct{}{}
	var out []string
	for _, c := range caps {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if _, ok := set[c]; ok {
			continue
		}
		set[c] = struct{}{}
		out = append(out, c)
	}
	return out
}

func (m *ChannelWebhookIntegrationModel) HasCapability(cap string) bool {
	cap = strings.TrimSpace(cap)
	if cap == "" {
		return false
	}
	for _, c := range m.Capabilities() {
		if c == cap {
			return true
		}
	}
	return false
}

func ChannelWebhookIntegrationGetByChannelAndBot(channelID, botUserID string) (*ChannelWebhookIntegrationModel, error) {
	channelID = strings.TrimSpace(channelID)
	botUserID = strings.TrimSpace(botUserID)
	if channelID == "" || botUserID == "" {
		return nil, nil
	}
	var item ChannelWebhookIntegrationModel
	if err := db.Where("channel_id = ? AND bot_user_id = ? AND status = ?", channelID, botUserID, WebhookIntegrationStatusActive).
		Limit(1).Find(&item).Error; err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, nil
	}
	return &item, nil
}

func ChannelWebhookIntegrationGetByID(channelID, id string) (*ChannelWebhookIntegrationModel, error) {
	channelID = strings.TrimSpace(channelID)
	id = strings.TrimSpace(id)
	if channelID == "" || id == "" {
		return nil, nil
	}
	var item ChannelWebhookIntegrationModel
	if err := db.Where("channel_id = ? AND id = ?", channelID, id).Limit(1).Find(&item).Error; err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, nil
	}
	return &item, nil
}

func ChannelWebhookIntegrationList(channelID string) ([]*ChannelWebhookIntegrationModel, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return []*ChannelWebhookIntegrationModel{}, nil
	}
	var items []*ChannelWebhookIntegrationModel
	if err := db.Where("channel_id = ?", channelID).Order("created_at DESC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// WebhookBotUserIDSet returns webhook-bot user ids from integration records.
// When botUserIDs is empty, it returns all webhook bot ids.
func WebhookBotUserIDSet(botUserIDs []string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	q := db.Model(&ChannelWebhookIntegrationModel{}).
		Select("DISTINCT bot_user_id").
		Where("bot_user_id <> ''")

	if len(botUserIDs) > 0 {
		dedup := map[string]struct{}{}
		cleaned := make([]string, 0, len(botUserIDs))
		for _, id := range botUserIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := dedup[id]; ok {
				continue
			}
			dedup[id] = struct{}{}
			cleaned = append(cleaned, id)
		}
		if len(cleaned) == 0 {
			return out, nil
		}
		q = q.Where("bot_user_id IN ?", cleaned)
	}

	var ids []string
	if err := q.Pluck("bot_user_id", &ids).Error; err != nil {
		return nil, err
	}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		out[id] = struct{}{}
	}
	return out, nil
}

func IsWebhookBotUser(userID string) (bool, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false, nil
	}
	set, err := WebhookBotUserIDSet([]string{userID})
	if err != nil {
		return false, err
	}
	_, ok := set[userID]
	return ok, nil
}

func ChannelWebhookIntegrationCreate(channelID, name, source, botUserID, createdBy string, capabilities []string) (*ChannelWebhookIntegrationModel, error) {
	channelID = strings.TrimSpace(channelID)
	name = strings.TrimSpace(name)
	source = strings.TrimSpace(source)
	botUserID = strings.TrimSpace(botUserID)
	createdBy = strings.TrimSpace(createdBy)
	if channelID == "" || botUserID == "" {
		return nil, nil
	}
	if name == "" {
		name = "Webhook"
	}
	capsJSON, _ := json.Marshal(capabilities)
	item := &ChannelWebhookIntegrationModel{
		StringPKBaseModel: StringPKBaseModel{
			ID: utils.NewID(),
		},
		ChannelID:        channelID,
		Name:             name,
		BotUserID:        botUserID,
		Source:           source,
		CapabilitiesJSON: string(capsJSON),
		Status:           WebhookIntegrationStatusActive,
		CreatedBy:        createdBy,
		LastUsedAt:       0,
	}
	if err := db.Create(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func ChannelWebhookIntegrationTouchUsage(channelID, botUserID string, now time.Time) error {
	channelID = strings.TrimSpace(channelID)
	botUserID = strings.TrimSpace(botUserID)
	if channelID == "" || botUserID == "" {
		return nil
	}
	return db.Model(&ChannelWebhookIntegrationModel{}).
		Where("channel_id = ? AND bot_user_id = ? AND status = ?", channelID, botUserID, WebhookIntegrationStatusActive).
		Update("last_used_at", now.UnixMilli()).Error
}
