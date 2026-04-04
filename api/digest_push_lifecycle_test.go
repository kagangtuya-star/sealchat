package api

import (
	"testing"
	"time"

	"sealchat/model"
	"sealchat/service"
	"sealchat/utils"
)

func countRowsUnscoped[T any](t *testing.T, where string, args ...any) int64 {
	t.Helper()
	var count int64
	if err := model.GetDB().Unscoped().Model(new(T)).Where(where, args...).Count(&count).Error; err != nil {
		t.Fatalf("count rows failed: %v", err)
	}
	return count
}

func createDigestLifecycleTestWorldAndChannel(t *testing.T) (*model.UserModel, *model.WorldModel, *model.ChannelModel) {
	t.Helper()

	owner := createOneBotTestUser(t, "digest-owner", false, "")
	world := &model.WorldModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "digest-world-" + utils.NewIDWithLength(8)},
		Name:              "Digest Lifecycle World",
		Visibility:        model.WorldVisibilityPublic,
		Status:            "active",
		OwnerID:           owner.ID,
	}
	if err := model.GetDB().Create(world).Error; err != nil {
		t.Fatalf("create world failed: %v", err)
	}
	if err := model.GetDB().Create(&model.WorldMemberModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "wm-" + utils.NewIDWithLength(8)},
		WorldID:           world.ID,
		UserID:            owner.ID,
		Role:              model.WorldRoleOwner,
		JoinedAt:          time.Now(),
	}).Error; err != nil {
		t.Fatalf("create world owner failed: %v", err)
	}

	channel := service.ChannelNew("digest-ch-"+utils.NewIDWithLength(8), "public", "Digest Lifecycle Channel", world.ID, owner.ID, "")
	if channel == nil || channel.ID == "" {
		t.Fatal("create channel failed")
	}
	return owner, world, channel
}

func activeChannelDigestPullIntegrations(t *testing.T, channelID string) []model.ChannelWebhookIntegrationModel {
	t.Helper()
	var items []model.ChannelWebhookIntegrationModel
	if err := model.GetDB().
		Where("channel_id = ? AND source = ? AND status = ?", channelID, "digest-pull", model.WebhookIntegrationStatusActive).
		Find(&items).Error; err != nil {
		t.Fatalf("load channel digest integrations failed: %v", err)
	}
	return items
}

func activeWorldDigestPullIntegrations(t *testing.T, worldID string) []model.DigestWebhookIntegrationModel {
	t.Helper()
	var items []model.DigestWebhookIntegrationModel
	if err := model.GetDB().
		Where("scope_type = ? AND scope_id = ? AND source = ? AND status = ?", model.DigestScopeTypeWorld, worldID, "digest-pull", model.WebhookIntegrationStatusActive).
		Find(&items).Error; err != nil {
		t.Fatalf("load world digest integrations failed: %v", err)
	}
	return items
}

func TestSyncDigestPullIntegrationForRuleChannelLifecycle(t *testing.T) {
	initOneBotAPITestEnv(t)

	owner, _, channel := createDigestLifecycleTestWorldAndChannel(t)
	rule := service.NewDefaultDigestRule(model.DigestScopeTypeChannel, channel.ID)
	rule.Enabled = true
	rule.PushMode = model.DigestPushModePassive

	if err := syncDigestPullIntegrationForRule(model.DigestScopeTypeChannel, channel.ID, rule, owner.ID); err != nil {
		t.Fatalf("first sync failed: %v", err)
	}

	items := activeChannelDigestPullIntegrations(t, channel.ID)
	if len(items) != 1 {
		t.Fatalf("active channel digest integrations=%d, want 1", len(items))
	}
	firstBotID := items[0].BotUserID
	if firstBotID == "" {
		t.Fatal("expected bot user id")
	}

	if err := syncDigestPullIntegrationForRule(model.DigestScopeTypeChannel, channel.ID, rule, owner.ID); err != nil {
		t.Fatalf("second sync failed: %v", err)
	}
	items = activeChannelDigestPullIntegrations(t, channel.ID)
	if len(items) != 1 {
		t.Fatalf("after second sync active channel digest integrations=%d, want 1", len(items))
	}
	if items[0].BotUserID != firstBotID {
		t.Fatalf("bot user id changed from %q to %q", firstBotID, items[0].BotUserID)
	}

	rule.Enabled = false
	rule.PushMode = model.DigestPushModeActive
	if err := syncDigestPullIntegrationForRule(model.DigestScopeTypeChannel, channel.ID, rule, owner.ID); err != nil {
		t.Fatalf("disable sync failed: %v", err)
	}
	items = activeChannelDigestPullIntegrations(t, channel.ID)
	if len(items) != 0 {
		t.Fatalf("after disable active channel digest integrations=%d, want 0", len(items))
	}
	if count := countRowsUnscoped[model.UserModel](t, "id = ?", firstBotID); count != 0 {
		t.Fatalf("bot user rows=%d, want 0", count)
	}
	if count := countRowsUnscoped[model.BotTokenModel](t, "id = ?", firstBotID); count != 0 {
		t.Fatalf("bot token rows=%d, want 0", count)
	}
}

func TestSyncDigestPullIntegrationForRuleChannelDeduplicatesAndKeepsNewestToken(t *testing.T) {
	initOneBotAPITestEnv(t)

	owner, _, channel := createDigestLifecycleTestWorldAndChannel(t)
	oldBot, oldToken := createOneBotTestBot(t, "digest-old", model.BotKindDigestPull)
	newBot, newToken := createOneBotTestBot(t, "digest-new", model.BotKindDigestPull)

	oldCreatedAt := time.Now().Add(-2 * time.Hour)
	newCreatedAt := time.Now().Add(-1 * time.Hour)
	if err := model.GetDB().Create(&model.ChannelWebhookIntegrationModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "whk-old-" + utils.NewIDWithLength(8), CreatedAt: oldCreatedAt, UpdatedAt: oldCreatedAt},
		ChannelID:         channel.ID,
		Name:              "摘要拉取旧",
		BotUserID:         oldBot.ID,
		Source:            "digest-pull",
		CapabilitiesJSON:  `["read_digest"]`,
		Status:            model.WebhookIntegrationStatusActive,
		CreatedBy:         owner.ID,
	}).Error; err != nil {
		t.Fatalf("create old integration failed: %v", err)
	}
	if err := model.GetDB().Create(&model.ChannelWebhookIntegrationModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "whk-new-" + utils.NewIDWithLength(8), CreatedAt: newCreatedAt, UpdatedAt: newCreatedAt},
		ChannelID:         channel.ID,
		Name:              "摘要拉取新",
		BotUserID:         newBot.ID,
		Source:            "digest-pull",
		CapabilitiesJSON:  `["read_digest"]`,
		Status:            model.WebhookIntegrationStatusActive,
		CreatedBy:         owner.ID,
	}).Error; err != nil {
		t.Fatalf("create new integration failed: %v", err)
	}

	rule := service.NewDefaultDigestRule(model.DigestScopeTypeChannel, channel.ID)
	rule.Enabled = true
	rule.PushMode = model.DigestPushModePassive
	if err := syncDigestPullIntegrationForRule(model.DigestScopeTypeChannel, channel.ID, rule, owner.ID); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	items := activeChannelDigestPullIntegrations(t, channel.ID)
	if len(items) != 1 {
		t.Fatalf("active channel digest integrations=%d, want 1", len(items))
	}
	if items[0].BotUserID != newBot.ID {
		t.Fatalf("kept bot user id=%q, want newest %q", items[0].BotUserID, newBot.ID)
	}
	stillThere, err := model.BotTokenGet(newBot.ID)
	if err != nil {
		t.Fatalf("load kept bot token failed: %v", err)
	}
	if stillThere == nil || stillThere.Token != newToken.Token {
		t.Fatalf("kept bot token changed, got %#v want %q", stillThere, newToken.Token)
	}
	if orphan, err := model.BotTokenGet(oldBot.ID); err != nil {
		t.Fatalf("load old bot token failed: %v", err)
	} else if orphan != nil {
		t.Fatalf("old bot token should be cleaned, got %q", orphan.Token)
	}
	if oldToken.Token == newToken.Token {
		t.Fatal("test precondition failed: old and new tokens should differ")
	}
}

func TestSyncDigestPullIntegrationForRuleWorldLifecycle(t *testing.T) {
	initOneBotAPITestEnv(t)

	owner, world, _ := createDigestLifecycleTestWorldAndChannel(t)
	rule := service.NewDefaultDigestRule(model.DigestScopeTypeWorld, world.ID)
	rule.Enabled = true
	rule.PushMode = model.DigestPushModeBoth

	if err := syncDigestPullIntegrationForRule(model.DigestScopeTypeWorld, world.ID, rule, owner.ID); err != nil {
		t.Fatalf("first sync failed: %v", err)
	}

	items := activeWorldDigestPullIntegrations(t, world.ID)
	if len(items) != 1 {
		t.Fatalf("active world digest integrations=%d, want 1", len(items))
	}
	firstBotID := items[0].BotUserID
	if firstBotID == "" {
		t.Fatal("expected bot user id")
	}

	rule.PushMode = model.DigestPushModeActive
	if err := syncDigestPullIntegrationForRule(model.DigestScopeTypeWorld, world.ID, rule, owner.ID); err != nil {
		t.Fatalf("active-only sync failed: %v", err)
	}
	items = activeWorldDigestPullIntegrations(t, world.ID)
	if len(items) != 0 {
		t.Fatalf("after active-only sync active world digest integrations=%d, want 0", len(items))
	}
	if count := countRowsUnscoped[model.UserModel](t, "id = ?", firstBotID); count != 0 {
		t.Fatalf("bot user rows=%d, want 0", count)
	}
}
