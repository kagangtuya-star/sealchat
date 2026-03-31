package api

import (
	"testing"
	"time"

	"sealchat/model"
	"sealchat/utils"
)

func TestCleanupOrphanSystemBotsForAdminRemovesOnlyOrphans(t *testing.T) {
	initOneBotAPITestEnv(t)
	db := model.GetDB()

	manualBot, _ := createOneBotTestBot(t, "cleanup-manual", model.BotKindManual)
	activeBot, _ := createOneBotTestBot(t, "cleanup-active", model.BotKindDigestPull)
	orphanBot, _ := createOneBotTestBot(t, "cleanup-orphan", model.BotKindDigestPull)

	worldID := "cleanup-world-" + utils.NewIDWithLength(8)
	if err := db.Create(&model.WorldModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: worldID},
		Name:              "Cleanup World",
		Status:            "active",
	}).Error; err != nil {
		t.Fatalf("create world failed: %v", err)
	}
	if err := db.Create(&model.DigestWebhookIntegrationModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "dg-" + utils.NewIDWithLength(8)},
		ScopeType:         model.DigestScopeTypeWorld,
		ScopeID:           worldID,
		Name:              "Active Digest Pull",
		BotUserID:         activeBot.ID,
		Source:            "digest-pull",
		Status:            model.WebhookIntegrationStatusActive,
		CreatedBy:         manualBot.ID,
		LastUsedAt:        time.Now().UnixMilli(),
	}).Error; err != nil {
		t.Fatalf("create active digest integration failed: %v", err)
	}

	stats, err := cleanupOrphanSystemBotsForAdmin()
	if err != nil {
		t.Fatalf("cleanupOrphanSystemBotsForAdmin failed: %v", err)
	}
	if stats == nil {
		t.Fatal("expected cleanup stats")
	}
	if stats.UserDeleted == 0 {
		t.Fatal("expected at least one orphan system bot user to be deleted")
	}
	if stats.ActiveReferenceSkippedCount == 0 {
		t.Fatal("expected active referenced system bot to be skipped")
	}

	if count := countRowsUnscoped[model.UserModel](t, "id = ?", orphanBot.ID); count != 0 {
		t.Fatalf("orphan bot user rows=%d, want 0", count)
	}
	if count := countRowsUnscoped[model.BotTokenModel](t, "id = ?", orphanBot.ID); count != 0 {
		t.Fatalf("orphan bot token rows=%d, want 0", count)
	}
	if count := countRowsUnscoped[model.UserModel](t, "id = ?", activeBot.ID); count == 0 {
		t.Fatal("active referenced bot should remain")
	}
	if count := countRowsUnscoped[model.UserModel](t, "id = ?", manualBot.ID); count == 0 {
		t.Fatal("manual bot should remain")
	}
}

func TestCleanupOrphanSystemBotsForAdminDeduplicatesChannelDigestPullAndKeepsNewestToken(t *testing.T) {
	initOneBotAPITestEnv(t)

	owner, _, channel := createDigestLifecycleTestWorldAndChannel(t)
	oldBot, oldToken := createOneBotTestBot(t, "cleanup-digest-old", model.BotKindDigestPull)
	newBot, newToken := createOneBotTestBot(t, "cleanup-digest-new", model.BotKindDigestPull)

	oldCreatedAt := time.Now().Add(-2 * time.Hour)
	newCreatedAt := time.Now().Add(-1 * time.Hour)
	if err := model.GetDB().Create(&model.ChannelWebhookIntegrationModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "cleanup-whk-old-" + utils.NewIDWithLength(8), CreatedAt: oldCreatedAt, UpdatedAt: oldCreatedAt},
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
		StringPKBaseModel: model.StringPKBaseModel{ID: "cleanup-whk-new-" + utils.NewIDWithLength(8), CreatedAt: newCreatedAt, UpdatedAt: newCreatedAt},
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

	stats, err := cleanupOrphanSystemBotsForAdmin()
	if err != nil {
		t.Fatalf("cleanupOrphanSystemBotsForAdmin failed: %v", err)
	}
	if stats == nil {
		t.Fatal("expected cleanup stats")
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
