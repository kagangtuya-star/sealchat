package api

import (
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/service"
	"sealchat/utils"
)

func digestRuleNeedsPassivePull(rule *model.DigestPushRuleModel) bool {
	if rule == nil {
		return false
	}
	if !rule.Enabled {
		return false
	}
	switch strings.TrimSpace(rule.PushMode) {
	case model.DigestPushModePassive, model.DigestPushModeBoth:
		return true
	default:
		return false
	}
}

func isDedicatedChannelDigestPullIntegration(item *model.ChannelWebhookIntegrationModel) bool {
	if item == nil {
		return false
	}
	if strings.TrimSpace(item.Status) != model.WebhookIntegrationStatusActive {
		return false
	}
	if strings.TrimSpace(item.Source) != "digest-pull" {
		return false
	}
	return item.HasCapability("read_digest")
}

func activeDedicatedChannelDigestPullIntegrations(channelID string) ([]*model.ChannelWebhookIntegrationModel, error) {
	items, err := model.ChannelWebhookIntegrationList(channelID)
	if err != nil {
		return nil, err
	}
	dedicated := make([]*model.ChannelWebhookIntegrationModel, 0)
	for _, item := range items {
		if isDedicatedChannelDigestPullIntegration(item) {
			dedicated = append(dedicated, item)
		}
	}
	return dedicated, nil
}

func cleanupDuplicateChannelDigestPullIntegrations(channelID string) ([]*model.ChannelWebhookIntegrationModel, error) {
	dedicated, err := activeDedicatedChannelDigestPullIntegrations(channelID)
	if err != nil {
		return nil, err
	}
	if len(dedicated) > 1 {
		for _, extra := range dedicated[1:] {
			if err := revokeChannelWebhookIntegrationByID(channelID, extra.ID); err != nil {
				return nil, err
			}
		}
		dedicated = dedicated[:1]
	}
	return dedicated, nil
}

func revokeChannelWebhookIntegrationByID(channelID, integrationID string) error {
	integrationID = strings.TrimSpace(integrationID)
	if strings.TrimSpace(channelID) == "" || integrationID == "" {
		return nil
	}
	integration, err := model.ChannelWebhookIntegrationGetByID(channelID, integrationID)
	if err != nil || integration == nil {
		return err
	}
	if strings.TrimSpace(integration.Status) != model.WebhookIntegrationStatusActive {
		return nil
	}

	tx := model.GetDB().Begin()
	if tx.Error != nil {
		return tx.Error
	}
	if err := tx.Model(&model.ChannelWebhookIntegrationModel{}).
		Where("id = ?", integration.ID).
		Update("status", model.WebhookIntegrationStatusRevoked).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Model(&model.BotTokenModel{}).
		Where("id = ?", integration.BotUserID).
		Update("expires_at", int64(0)).Error; err != nil {
		tx.Rollback()
		return err
	}
	if _, err := model.CleanupOrphanSystemBotByUserIDTx(tx, integration.BotUserID); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

func activeDigestPullIntegrations(scopeType, scopeID string) ([]*model.DigestWebhookIntegrationModel, error) {
	items, err := model.DigestWebhookIntegrationList(scopeType, scopeID)
	if err != nil {
		return nil, err
	}
	dedicated := make([]*model.DigestWebhookIntegrationModel, 0)
	for _, item := range items {
		if item == nil {
			continue
		}
		if strings.TrimSpace(item.Status) != model.WebhookIntegrationStatusActive {
			continue
		}
		if strings.TrimSpace(item.Source) != "digest-pull" {
			continue
		}
		dedicated = append(dedicated, item)
	}
	return dedicated, nil
}

func cleanupDuplicateDigestPullIntegrations(scopeType, scopeID string) ([]*model.DigestWebhookIntegrationModel, error) {
	dedicated, err := activeDigestPullIntegrations(scopeType, scopeID)
	if err != nil {
		return nil, err
	}
	if len(dedicated) > 1 {
		for _, extra := range dedicated[1:] {
			if err := revokeWorldDigestIntegrationByID(scopeType, scopeID, extra.ID); err != nil {
				return nil, err
			}
		}
		dedicated = dedicated[:1]
	}
	return dedicated, nil
}

func revokeWorldDigestIntegrationByID(scopeType, scopeID, integrationID string) error {
	integrationID = strings.TrimSpace(integrationID)
	if strings.TrimSpace(scopeType) == "" || strings.TrimSpace(scopeID) == "" || integrationID == "" {
		return nil
	}
	integration, err := model.DigestWebhookIntegrationGetByID(scopeType, scopeID, integrationID)
	if err != nil || integration == nil {
		return err
	}
	if strings.TrimSpace(integration.Status) != model.WebhookIntegrationStatusActive {
		return nil
	}

	tx := model.GetDB().Begin()
	if tx.Error != nil {
		return tx.Error
	}
	if err := tx.Model(&model.DigestWebhookIntegrationModel{}).
		Where("id = ?", integration.ID).
		Update("status", model.WebhookIntegrationStatusRevoked).Error; err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Model(&model.BotTokenModel{}).
		Where("id = ?", integration.BotUserID).
		Update("expires_at", int64(0)).Error; err != nil {
		tx.Rollback()
		return err
	}
	if _, err := model.CleanupOrphanSystemBotByUserIDTx(tx, integration.BotUserID); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
}

func ensureChannelDigestPullIntegration(channelID, actorUserID string) error {
	dedicated, err := cleanupDuplicateChannelDigestPullIntegrations(channelID)
	if err != nil {
		return err
	}
	if len(dedicated) > 0 {
		return nil
	}

	name := "摘要拉取"
	uid := utils.NewID()
	user := &model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: uid},
		Username:          utils.NewID(),
		Nickname:          name,
		Password:          "",
		Salt:              "BOT_SALT",
		IsBot:             true,
		BotKind:           model.BotKindDigestPull,
	}
	token := &model.BotTokenModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: uid},
		Name:              name,
		Token:             utils.NewIDWithLength(32),
		ExpiresAt:         time.Now().UnixMilli() + 3*365*24*60*60*1e3,
	}
	if err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		if err := tx.Create(token).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.ChannelWebhookIntegrationModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
			ChannelID:         channelID,
			Name:              name,
			BotUserID:         uid,
			Source:            "digest-pull",
			CapabilitiesJSON:  `["read_digest"]`,
			Status:            model.WebhookIntegrationStatusActive,
			CreatedBy:         strings.TrimSpace(actorUserID),
			LastUsedAt:        0,
		}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	_ = service.SyncBotUserProfile(token)
	_ = service.SyncBotMembers(token)
	_, _ = model.MemberGetByUserIDAndChannelIDBase(token.ID, channelID, strings.TrimSpace(token.Name), true)
	return nil
}

func revokeChannelDigestPullIntegrations(channelID string) error {
	items, err := model.ChannelWebhookIntegrationList(channelID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if !isDedicatedChannelDigestPullIntegration(item) {
			continue
		}
		if err := revokeChannelWebhookIntegrationByID(channelID, item.ID); err != nil {
			return err
		}
	}
	return nil
}

func ensureWorldDigestPullIntegration(worldID, actorUserID string) error {
	dedicated, err := cleanupDuplicateDigestPullIntegrations(model.DigestScopeTypeWorld, worldID)
	if err != nil {
		return err
	}
	if len(dedicated) > 0 {
		return nil
	}

	name := "世界摘要拉取"
	uid := utils.NewID()
	user := &model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: uid},
		Username:          utils.NewID(),
		Nickname:          name,
		Password:          "",
		Salt:              "BOT_SALT",
		IsBot:             true,
		BotKind:           model.BotKindDigestPull,
	}
	token := &model.BotTokenModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: uid},
		Name:              name,
		Token:             utils.NewIDWithLength(32),
		ExpiresAt:         time.Now().UnixMilli() + 3*365*24*60*60*1e3,
	}
	if err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		if err := tx.Create(token).Error; err != nil {
			return err
		}
		return tx.Create(&model.DigestWebhookIntegrationModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
			ScopeType:         model.DigestScopeTypeWorld,
			ScopeID:           worldID,
			Name:              name,
			Source:            "digest-pull",
			BotUserID:         uid,
			Status:            model.WebhookIntegrationStatusActive,
			CreatedBy:         strings.TrimSpace(actorUserID),
			LastUsedAt:        0,
		}).Error
	}); err != nil {
		return err
	}
	return service.SyncBotUserProfile(token)
}

func revokeWorldDigestPullIntegrations(worldID string) error {
	items, err := model.DigestWebhookIntegrationList(model.DigestScopeTypeWorld, worldID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		if strings.TrimSpace(item.Status) != model.WebhookIntegrationStatusActive || strings.TrimSpace(item.Source) != "digest-pull" {
			continue
		}
		if err := revokeWorldDigestIntegrationByID(model.DigestScopeTypeWorld, worldID, item.ID); err != nil {
			return err
		}
	}
	return nil
}

func syncDigestPullIntegrationForRule(scopeType, scopeID string, rule *model.DigestPushRuleModel, actorUserID string) error {
	scopeType = strings.TrimSpace(scopeType)
	scopeID = strings.TrimSpace(scopeID)
	if scopeType == "" || scopeID == "" {
		return nil
	}
	if digestRuleNeedsPassivePull(rule) {
		switch scopeType {
		case model.DigestScopeTypeChannel:
			return ensureChannelDigestPullIntegration(scopeID, actorUserID)
		case model.DigestScopeTypeWorld:
			return ensureWorldDigestPullIntegration(scopeID, actorUserID)
		default:
			return nil
		}
	}
	switch scopeType {
	case model.DigestScopeTypeChannel:
		return revokeChannelDigestPullIntegrations(scopeID)
	case model.DigestScopeTypeWorld:
		return revokeWorldDigestPullIntegrations(scopeID)
	default:
		return nil
	}
}

func cleanupOrphanSystemBotsForAdmin() (*model.WebhookBotFriendCleanupStats, error) {
	var channelIDs []string
	if err := model.GetDB().
		Model(&model.ChannelWebhookIntegrationModel{}).
		Distinct("channel_id").
		Where("status = ? AND source = ?", model.WebhookIntegrationStatusActive, "digest-pull").
		Pluck("channel_id", &channelIDs).Error; err != nil {
		return nil, err
	}
	for _, channelID := range channelIDs {
		if _, err := cleanupDuplicateChannelDigestPullIntegrations(channelID); err != nil {
			return nil, err
		}
	}

	type digestScopeRow struct {
		ScopeType string
		ScopeID   string
	}
	var digestScopes []digestScopeRow
	if err := model.GetDB().
		Model(&model.DigestWebhookIntegrationModel{}).
		Select("DISTINCT scope_type, scope_id").
		Where("status = ? AND source = ?", model.WebhookIntegrationStatusActive, "digest-pull").
		Scan(&digestScopes).Error; err != nil {
		return nil, err
	}
	for _, item := range digestScopes {
		if _, err := cleanupDuplicateDigestPullIntegrations(item.ScopeType, item.ScopeID); err != nil {
			return nil, err
		}
	}

	return model.CleanupWebhookBotFriendData()
}
