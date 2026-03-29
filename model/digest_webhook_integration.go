package model

import (
	"strings"
	"time"

	"sealchat/utils"
)

type DigestWebhookIntegrationModel struct {
	StringPKBaseModel
	ScopeType         string `json:"scopeType" gorm:"size:32;index:idx_digest_webhook_scope,priority:1"`
	ScopeID           string `json:"scopeId" gorm:"size:100;index:idx_digest_webhook_scope,priority:2"`
	Name              string `json:"name" gorm:"size:64"`
	Source            string `json:"source" gorm:"size:32;index"`
	BotUserID         string `json:"botUserId" gorm:"size:100;index"`
	Status            string `json:"status" gorm:"size:16;index"`
	CreatedBy         string `json:"createdBy" gorm:"size:100"`
	LastUsedAt        int64  `json:"lastUsedAt"`
	TokenTailFragment string `json:"tokenTailFragment" gorm:"size:12"`
}

func (*DigestWebhookIntegrationModel) TableName() string {
	return "digest_webhook_integrations"
}

func DigestWebhookIntegrationGetByScopeAndBot(scopeType, scopeID, botUserID string) (*DigestWebhookIntegrationModel, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	botUserID = strings.TrimSpace(botUserID)
	if scopeType == "" || scopeID == "" || botUserID == "" {
		return nil, nil
	}
	var item DigestWebhookIntegrationModel
	if err := db.Where("scope_type = ? AND scope_id = ? AND bot_user_id = ? AND status = ?", scopeType, scopeID, botUserID, WebhookIntegrationStatusActive).
		Limit(1).
		Find(&item).Error; err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, nil
	}
	return &item, nil
}

func DigestWebhookIntegrationGetByID(scopeType, scopeID, id string) (*DigestWebhookIntegrationModel, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	id = strings.TrimSpace(id)
	if scopeType == "" || scopeID == "" || id == "" {
		return nil, nil
	}
	var item DigestWebhookIntegrationModel
	if err := db.Where("scope_type = ? AND scope_id = ? AND id = ?", scopeType, scopeID, id).Limit(1).Find(&item).Error; err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, nil
	}
	return &item, nil
}

func DigestWebhookIntegrationList(scopeType, scopeID string) ([]*DigestWebhookIntegrationModel, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return []*DigestWebhookIntegrationModel{}, nil
	}
	var items []*DigestWebhookIntegrationModel
	if err := db.Where("scope_type = ? AND scope_id = ?", scopeType, scopeID).Order("created_at DESC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func DigestWebhookIntegrationCreate(scopeType, scopeID, name, source, botUserID, createdBy string) (*DigestWebhookIntegrationModel, error) {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	name = strings.TrimSpace(name)
	source = strings.TrimSpace(source)
	botUserID = strings.TrimSpace(botUserID)
	createdBy = strings.TrimSpace(createdBy)
	if scopeType == "" || scopeID == "" || botUserID == "" {
		return nil, nil
	}
	if name == "" {
		name = "摘要拉取"
	}
	if source == "" {
		source = "digest-pull"
	}
	item := &DigestWebhookIntegrationModel{
		StringPKBaseModel: StringPKBaseModel{ID: utils.NewID()},
		ScopeType:         scopeType,
		ScopeID:           scopeID,
		Name:              name,
		Source:            source,
		BotUserID:         botUserID,
		Status:            WebhookIntegrationStatusActive,
		CreatedBy:         createdBy,
		LastUsedAt:        0,
	}
	if err := db.Create(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func DigestWebhookIntegrationTouchUsage(scopeType, scopeID, botUserID string, now time.Time) error {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	botUserID = strings.TrimSpace(botUserID)
	if scopeType == "" || scopeID == "" || botUserID == "" {
		return nil
	}
	return db.Model(&DigestWebhookIntegrationModel{}).
		Where("scope_type = ? AND scope_id = ? AND bot_user_id = ? AND status = ?", scopeType, scopeID, botUserID, WebhookIntegrationStatusActive).
		Update("last_used_at", now.UnixMilli()).Error
}
