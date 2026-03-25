package service

import (
	"testing"

	"sealchat/model"
	"sealchat/utils"
)

func createTestUser(t *testing.T, id, username, nickname string, isBot bool) {
	t.Helper()
	if err := model.GetDB().Create(&model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: id},
		Username:          username,
		Nickname:          nickname,
		Password:          "pw",
		Salt:              "salt",
		IsBot:             isBot,
	}).Error; err != nil {
		t.Fatalf("create user %s failed: %v", id, err)
	}
}

func countRowsUnscoped[T any](t *testing.T, where string, args ...any) int64 {
	t.Helper()
	var count int64
	if err := model.GetDB().Unscoped().Model(new(T)).Where(where, args...).Count(&count).Error; err != nil {
		t.Fatalf("count rows failed: %v", err)
	}
	return count
}

func currentInternalBotCleanupExpectations(t *testing.T) *model.WebhookBotFriendCleanupStats {
	t.Helper()

	stats := &model.WebhookBotFriendCleanupStats{}
	internalBotSet, err := model.InternalBotUserIDSet(nil)
	if err != nil {
		t.Fatalf("InternalBotUserIDSet failed: %v", err)
	}
	if len(internalBotSet) == 0 {
		return stats
	}

	systemBotIDs := make([]string, 0, len(internalBotSet))
	for id := range internalBotSet {
		systemBotIDs = append(systemBotIDs, id)
	}

	type friendCleanupRow struct {
		ID      string
		UserID1 string
		UserID2 string
	}
	var friendRows []friendCleanupRow
	if err := model.GetDB().Model(&model.FriendModel{}).
		Select("id", "user_id1", "user_id2").
		Where("user_id1 IN ? OR user_id2 IN ?", systemBotIDs, systemBotIDs).
		Find(&friendRows).Error; err != nil {
		t.Fatalf("load friend cleanup rows failed: %v", err)
	}

	type botLocalStats struct {
		userRoles     int64
		members       int64
		worldMembers  int64
		friends       int64
		privateChat   int64
		users         int64
		botTokens     int64
		hasAnyCleanup bool
	}
	perBot := map[string]*botLocalStats{}
	getBotStats := func(botID string) *botLocalStats {
		item := perBot[botID]
		if item == nil {
			item = &botLocalStats{}
			perBot[botID] = item
		}
		return item
	}

	for _, botID := range systemBotIDs {
		refCount, err := model.ActiveSystemBotReferenceCount(botID)
		if err != nil {
			t.Fatalf("ActiveSystemBotReferenceCount failed: %v", err)
		}
		if refCount > 0 {
			stats.ActiveReferenceSkippedCount++
			continue
		}
		item := getBotStats(botID)
		item.userRoles = countRowsUnscoped[model.UserRoleMappingModel](t, "user_id = ?", botID)
		item.members = countRowsUnscoped[model.MemberModel](t, "user_id = ?", botID)
		item.worldMembers = countRowsUnscoped[model.WorldMemberModel](t, "user_id = ?", botID)
		item.users = countRowsUnscoped[model.UserModel](t, "id = ?", botID)
		item.botTokens = countRowsUnscoped[model.BotTokenModel](t, "id = ?", botID)
	}

	for _, row := range friendRows {
		friendID := row.ID
		isPrivateCount := countRowsUnscoped[model.ChannelModel](t, "id = ? AND (is_private = ? OR perm_type = ?)", friendID, true, "private")
		for _, botID := range []string{row.UserID1, row.UserID2} {
			if _, ok := internalBotSet[botID]; !ok {
				continue
			}
			refCount, err := model.ActiveSystemBotReferenceCount(botID)
			if err != nil {
				t.Fatalf("ActiveSystemBotReferenceCount failed: %v", err)
			}
			if refCount > 0 {
				continue
			}
			item := getBotStats(botID)
			item.friends++
			item.privateChat += isPrivateCount
		}
	}

	for _, item := range perBot {
		stats.UserRoleMappingDeleted += item.userRoles
		stats.MemberDeleted += item.members
		stats.WorldMemberDeleted += item.worldMembers
		stats.FriendRelationDeleted += item.friends
		stats.PrivateChannelDeleted += item.privateChat
		stats.UserDeleted += item.users
		stats.BotTokenDeleted += item.botTokens
		if item.userRoles > 0 || item.members > 0 || item.worldMembers > 0 || item.friends > 0 || item.privateChat > 0 || item.users > 0 || item.botTokens > 0 {
			item.hasAnyCleanup = true
			stats.WebhookBotCount++
		}
	}

	return stats
}

func TestUserBotListExcludesWebhookAndDigestInternalBots(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()

	manualBotID := "manual-bot-" + utils.NewID()
	webhookBotID := "webhook-bot-" + utils.NewID()
	digestBotID := "digest-bot-" + utils.NewID()

	createTestUser(t, manualBotID, "manual_"+manualBotID, "Manual Bot", true)
	createTestUser(t, webhookBotID, "webhook_"+webhookBotID, "Webhook Bot", true)
	createTestUser(t, digestBotID, "digest_"+digestBotID, "Digest Bot", true)

	if err := db.Create(&model.ChannelWebhookIntegrationModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "whk-" + utils.NewID()},
		ChannelID:         "channel-" + utils.NewID(),
		Name:              "Webhook",
		BotUserID:         webhookBotID,
		Source:            "external",
		Status:            model.WebhookIntegrationStatusActive,
	}).Error; err != nil {
		t.Fatalf("create webhook integration failed: %v", err)
	}
	if err := db.Create(&model.DigestWebhookIntegrationModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "dg-" + utils.NewID()},
		ScopeType:         model.DigestScopeTypeWorld,
		ScopeID:           "world-" + utils.NewID(),
		Name:              "Digest",
		BotUserID:         digestBotID,
		Source:            "digest-pull",
		Status:            model.WebhookIntegrationStatusActive,
	}).Error; err != nil {
		t.Fatalf("create digest integration failed: %v", err)
	}

	internalBotSet, err := model.InternalBotUserIDSet(nil)
	if err != nil {
		t.Fatalf("InternalBotUserIDSet failed: %v", err)
	}
	if _, ok := internalBotSet[webhookBotID]; !ok {
		t.Fatalf("expected webhook bot %s to be treated as internal", webhookBotID)
	}
	if _, ok := internalBotSet[digestBotID]; !ok {
		t.Fatalf("expected digest bot %s to be treated as internal", digestBotID)
	}
	if _, ok := internalBotSet[manualBotID]; ok {
		t.Fatalf("did not expect manual bot %s to be treated as internal", manualBotID)
	}

	bots, err := model.UserBotList()
	if err != nil {
		t.Fatalf("UserBotList failed: %v", err)
	}
	if len(bots) != 1 {
		t.Fatalf("UserBotList returned %d bots, want 1", len(bots))
	}
	if bots[0].ID != manualBotID {
		t.Fatalf("UserBotList returned %s, want %s", bots[0].ID, manualBotID)
	}
}

func TestFriendListAndChannelsExcludeInternalBots(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()

	userID := "friend-user-" + utils.NewID()
	manualBotID := "friend-manual-bot-" + utils.NewID()
	digestBotID := "friend-digest-bot-" + utils.NewID()

	createTestUser(t, userID, "user_"+userID, "Friend User", false)
	createTestUser(t, manualBotID, "manual_"+manualBotID, "Manual Bot", true)
	createTestUser(t, digestBotID, "digest_"+digestBotID, "Digest Bot", true)

	if _, err := model.FriendRelationFriendApprove(userID, manualBotID); err != nil {
		t.Fatalf("approve manual bot friendship failed: %v", err)
	}
	if _, err := model.FriendRelationFriendApprove(userID, digestBotID); err != nil {
		t.Fatalf("approve digest bot friendship failed: %v", err)
	}
	if _, isNew := model.ChannelPrivateNew(userID, manualBotID); !isNew {
		if ch, err := model.ChannelPrivateGet(userID, manualBotID); err != nil || ch == nil || ch.ID == "" {
			t.Fatalf("ensure manual bot private channel failed: %v", err)
		}
	}
	if _, isNew := model.ChannelPrivateNew(userID, digestBotID); !isNew {
		if ch, err := model.ChannelPrivateGet(userID, digestBotID); err != nil || ch == nil || ch.ID == "" {
			t.Fatalf("ensure digest bot private channel failed: %v", err)
		}
	}

	if err := db.Create(&model.DigestWebhookIntegrationModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "dg-friend-" + utils.NewID()},
		ScopeType:         model.DigestScopeTypeWorld,
		ScopeID:           "world-friend-" + utils.NewID(),
		Name:              "Digest Friend",
		BotUserID:         digestBotID,
		Source:            "digest-pull",
		Status:            model.WebhookIntegrationStatusActive,
	}).Error; err != nil {
		t.Fatalf("create digest integration failed: %v", err)
	}

	friends, err := model.FriendList(userID, true)
	if err != nil {
		t.Fatalf("FriendList failed: %v", err)
	}
	if len(friends) != 1 {
		t.Fatalf("FriendList returned %d items, want 1", len(friends))
	}
	if friends[0].UserInfo == nil || friends[0].UserInfo.ID != manualBotID {
		t.Fatalf("FriendList returned unexpected friend user")
	}

	channels, err := model.FriendChannelList(userID)
	if err != nil {
		t.Fatalf("FriendChannelList failed: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("FriendChannelList returned %d channels, want 1", len(channels))
	}
	if channels[0].FriendInfo == nil || channels[0].FriendInfo.UserInfo == nil || channels[0].FriendInfo.UserInfo.ID != manualBotID {
		t.Fatalf("FriendChannelList returned unexpected friend info")
	}
}

func TestInternalBotUserSetPrefersBotKindField(t *testing.T) {
	initTestDB(t)

	manualBotID := "kind-manual-bot-" + utils.NewID()
	webhookBotID := "kind-webhook-bot-" + utils.NewID()
	digestBotID := "kind-digest-bot-" + utils.NewID()

	if err := model.GetDB().Create(&model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: manualBotID},
		Username:          "manual_" + manualBotID,
		Nickname:          "Manual Kind Bot",
		Password:          "pw",
		Salt:              "salt",
		IsBot:             true,
		BotKind:           model.BotKindManual,
	}).Error; err != nil {
		t.Fatalf("create manual kind bot failed: %v", err)
	}
	if err := model.GetDB().Create(&model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: webhookBotID},
		Username:          "webhook_" + webhookBotID,
		Nickname:          "Webhook Kind Bot",
		Password:          "pw",
		Salt:              "salt",
		IsBot:             true,
		BotKind:           model.BotKindChannelWebhook,
	}).Error; err != nil {
		t.Fatalf("create webhook kind bot failed: %v", err)
	}
	if err := model.GetDB().Create(&model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: digestBotID},
		Username:          "digest_" + digestBotID,
		Nickname:          "Digest Kind Bot",
		Password:          "pw",
		Salt:              "salt",
		IsBot:             true,
		BotKind:           model.BotKindDigestPull,
	}).Error; err != nil {
		t.Fatalf("create digest kind bot failed: %v", err)
	}

	internalBotSet, err := model.InternalBotUserIDSet(nil)
	if err != nil {
		t.Fatalf("InternalBotUserIDSet failed: %v", err)
	}
	if _, ok := internalBotSet[webhookBotID]; !ok {
		t.Fatalf("expected webhook kind bot %s to be treated as internal", webhookBotID)
	}
	if _, ok := internalBotSet[digestBotID]; !ok {
		t.Fatalf("expected digest kind bot %s to be treated as internal", digestBotID)
	}
	if _, ok := internalBotSet[manualBotID]; ok {
		t.Fatalf("did not expect manual kind bot %s to be treated as internal", manualBotID)
	}

	bots, err := model.UserBotList()
	if err != nil {
		t.Fatalf("UserBotList failed: %v", err)
	}
	for _, bot := range bots {
		if bot.ID == webhookBotID || bot.ID == digestBotID {
			t.Fatalf("internal bot %s leaked into UserBotList", bot.ID)
		}
	}
}

func TestBackfillBotKinds(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()

	manualBotID := "backfill-manual-" + utils.NewID()
	webhookBotID := "backfill-webhook-" + utils.NewID()
	digestBotID := "backfill-digest-" + utils.NewID()

	createTestUser(t, manualBotID, "manual_"+manualBotID, "Manual Backfill Bot", true)
	createTestUser(t, webhookBotID, "webhook_"+webhookBotID, "Webhook Backfill Bot", true)
	createTestUser(t, digestBotID, "digest_"+digestBotID, "Digest Backfill Bot", true)

	if err := db.Create(&model.ChannelWebhookIntegrationModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "whk-backfill-" + utils.NewID()},
		ChannelID:         "channel-backfill-" + utils.NewID(),
		Name:              "Webhook Backfill",
		BotUserID:         webhookBotID,
		Source:            "external",
		Status:            model.WebhookIntegrationStatusActive,
	}).Error; err != nil {
		t.Fatalf("create webhook integration failed: %v", err)
	}
	if err := db.Create(&model.DigestWebhookIntegrationModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "dg-backfill-" + utils.NewID()},
		ScopeType:         model.DigestScopeTypeWorld,
		ScopeID:           "world-backfill-" + utils.NewID(),
		Name:              "Digest Backfill",
		BotUserID:         digestBotID,
		Source:            "digest-pull",
		Status:            model.WebhookIntegrationStatusActive,
	}).Error; err != nil {
		t.Fatalf("create digest integration failed: %v", err)
	}

	if err := model.BackfillBotKinds(); err != nil {
		t.Fatalf("BackfillBotKinds failed: %v", err)
	}

	manualBot := model.UserGet(manualBotID)
	if manualBot == nil || manualBot.BotKind != model.BotKindManual {
		t.Fatalf("manual bot kind=%q, want %q", manualBot.BotKind, model.BotKindManual)
	}
	webhookBot := model.UserGet(webhookBotID)
	if webhookBot == nil || webhookBot.BotKind != model.BotKindChannelWebhook {
		t.Fatalf("webhook bot kind=%q, want %q", webhookBot.BotKind, model.BotKindChannelWebhook)
	}
	digestBot := model.UserGet(digestBotID)
	if digestBot == nil || digestBot.BotKind != model.BotKindDigestPull {
		t.Fatalf("digest bot kind=%q, want %q", digestBot.BotKind, model.BotKindDigestPull)
	}
}

func TestCleanupWebhookBotFriendDataIncludesDigestBots(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()

	userID := "cleanup-user-" + utils.NewID()
	manualBotID := "cleanup-manual-" + utils.NewID()
	webhookBotID := "cleanup-webhook-" + utils.NewID()
	digestBotID := "cleanup-digest-" + utils.NewID()

	createTestUser(t, userID, "user_"+userID, "Cleanup User", false)
	createTestUser(t, manualBotID, "manual_"+manualBotID, "Cleanup Manual Bot", true)
	createTestUser(t, webhookBotID, "webhook_"+webhookBotID, "Cleanup Webhook Bot", true)
	createTestUser(t, digestBotID, "digest_"+digestBotID, "Cleanup Digest Bot", true)

	for _, pair := range [][2]string{{userID, manualBotID}, {userID, webhookBotID}, {userID, digestBotID}} {
		if _, err := model.FriendRelationFriendApprove(pair[0], pair[1]); err != nil {
			t.Fatalf("approve friendship %v failed: %v", pair, err)
		}
		if ch, _ := model.ChannelPrivateNew(pair[0], pair[1]); ch == nil || ch.ID == "" {
			t.Fatalf("create private channel %v failed", pair)
		}
	}

	for _, botID := range []string{manualBotID, webhookBotID, digestBotID} {
		if err := db.Create(&model.BotTokenModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: botID},
			Name:              "Token " + botID,
			Token:             "token-" + botID,
			ExpiresAt:         123456789,
		}).Error; err != nil {
			t.Fatalf("create bot token %s failed: %v", botID, err)
		}
		if err := db.Create(&model.MemberModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: "member-" + botID},
			ChannelID:         "channel-member-" + botID,
			UserID:            botID,
			Nickname:          "Member " + botID,
		}).Error; err != nil {
			t.Fatalf("create member %s failed: %v", botID, err)
		}
		if err := db.Create(&model.UserRoleMappingModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: "mapping-" + botID},
			RoleType:          "channel",
			UserID:            botID,
			RoleID:            "role-" + botID,
		}).Error; err != nil {
			t.Fatalf("create role mapping %s failed: %v", botID, err)
		}
		if err := db.Create(&model.WorldMemberModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: "wm-" + botID},
			WorldID:           "world-" + botID,
			UserID:            botID,
			Role:              model.WorldRoleMember,
		}).Error; err != nil {
			t.Fatalf("create world member %s failed: %v", botID, err)
		}
	}

	webhookBot := model.UserGet(webhookBotID)
	digestBot := model.UserGet(digestBotID)
	manualBot := model.UserGet(manualBotID)
	webhookBot.BotKind = model.BotKindChannelWebhook
	digestBot.BotKind = model.BotKindDigestPull
	manualBot.BotKind = model.BotKindManual
	if err := model.GetDB().Model(webhookBot).Update("bot_kind", webhookBot.BotKind).Error; err != nil {
		t.Fatalf("update webhook bot kind failed: %v", err)
	}
	if err := model.GetDB().Model(digestBot).Update("bot_kind", digestBot.BotKind).Error; err != nil {
		t.Fatalf("update digest bot kind failed: %v", err)
	}
	if err := model.GetDB().Model(manualBot).Update("bot_kind", manualBot.BotKind).Error; err != nil {
		t.Fatalf("update manual bot kind failed: %v", err)
	}

	expectedStats := currentInternalBotCleanupExpectations(t)

	stats, err := model.CleanupWebhookBotFriendData()
	if err != nil {
		t.Fatalf("CleanupWebhookBotFriendData failed: %v", err)
	}
	if stats.WebhookBotCount != expectedStats.WebhookBotCount {
		t.Fatalf("system bot count=%d, want %d", stats.WebhookBotCount, expectedStats.WebhookBotCount)
	}
	if stats.FriendRelationDeleted != expectedStats.FriendRelationDeleted {
		t.Fatalf("friend relation deleted=%d, want %d", stats.FriendRelationDeleted, expectedStats.FriendRelationDeleted)
	}
	if stats.PrivateChannelDeleted != expectedStats.PrivateChannelDeleted {
		t.Fatalf("private channel deleted=%d, want %d", stats.PrivateChannelDeleted, expectedStats.PrivateChannelDeleted)
	}
	if stats.UserRoleMappingDeleted != expectedStats.UserRoleMappingDeleted {
		t.Fatalf("user role mapping deleted=%d, want %d", stats.UserRoleMappingDeleted, expectedStats.UserRoleMappingDeleted)
	}
	if stats.MemberDeleted != expectedStats.MemberDeleted {
		t.Fatalf("member deleted=%d, want %d", stats.MemberDeleted, expectedStats.MemberDeleted)
	}
	if stats.WorldMemberDeleted != expectedStats.WorldMemberDeleted {
		t.Fatalf("world member deleted=%d, want %d", stats.WorldMemberDeleted, expectedStats.WorldMemberDeleted)
	}
	if stats.UserDeleted != expectedStats.UserDeleted {
		t.Fatalf("user deleted=%d, want %d", stats.UserDeleted, expectedStats.UserDeleted)
	}
	if stats.BotTokenDeleted != expectedStats.BotTokenDeleted {
		t.Fatalf("bot token deleted=%d, want %d", stats.BotTokenDeleted, expectedStats.BotTokenDeleted)
	}

	if model.FriendRelationGet(userID, webhookBotID).ID != "" {
		t.Fatalf("webhook bot friendship should be deleted")
	}
	if model.FriendRelationGet(userID, digestBotID).ID != "" {
		t.Fatalf("digest bot friendship should be deleted")
	}
	if model.FriendRelationGet(userID, manualBotID).ID == "" {
		t.Fatalf("manual bot friendship should remain")
	}
	if ch, _ := model.ChannelPrivateGet(userID, webhookBotID); ch != nil && ch.ID != "" {
		t.Fatalf("webhook bot private channel should be deleted")
	}
	if ch, _ := model.ChannelPrivateGet(userID, digestBotID); ch != nil && ch.ID != "" {
		t.Fatalf("digest bot private channel should be deleted")
	}
	if ch, _ := model.ChannelPrivateGet(userID, manualBotID); ch == nil || ch.ID == "" {
		t.Fatalf("manual bot private channel should remain")
	}
	if user := model.UserGet(webhookBotID); user != nil && user.ID != "" {
		t.Fatalf("webhook bot user should be deleted")
	}
	if user := model.UserGet(digestBotID); user != nil && user.ID != "" {
		t.Fatalf("digest bot user should be deleted")
	}
	if user := model.UserGet(manualBotID); user == nil || user.ID == "" {
		t.Fatalf("manual bot user should remain")
	}
	if countRowsUnscoped[model.MemberModel](t, "user_id = ?", webhookBotID) != 0 {
		t.Fatalf("webhook bot member rows should be deleted")
	}
	if countRowsUnscoped[model.MemberModel](t, "user_id = ?", digestBotID) != 0 {
		t.Fatalf("digest bot member rows should be deleted")
	}
	if countRowsUnscoped[model.MemberModel](t, "user_id = ?", manualBotID) == 0 {
		t.Fatalf("manual bot member rows should remain")
	}
	if countRowsUnscoped[model.UserRoleMappingModel](t, "user_id = ?", webhookBotID) != 0 {
		t.Fatalf("webhook bot role mappings should be deleted")
	}
	if countRowsUnscoped[model.UserRoleMappingModel](t, "user_id = ?", digestBotID) != 0 {
		t.Fatalf("digest bot role mappings should be deleted")
	}
	if countRowsUnscoped[model.UserRoleMappingModel](t, "user_id = ?", manualBotID) == 0 {
		t.Fatalf("manual bot role mappings should remain")
	}
	if countRowsUnscoped[model.WorldMemberModel](t, "user_id = ?", webhookBotID) != 0 {
		t.Fatalf("webhook bot world members should be deleted")
	}
	if countRowsUnscoped[model.WorldMemberModel](t, "user_id = ?", digestBotID) != 0 {
		t.Fatalf("digest bot world members should be deleted")
	}
	if countRowsUnscoped[model.WorldMemberModel](t, "user_id = ?", manualBotID) == 0 {
		t.Fatalf("manual bot world members should remain")
	}
	if countRowsUnscoped[model.BotTokenModel](t, "id = ?", webhookBotID) != 0 {
		t.Fatalf("webhook bot token should be deleted")
	}
	if countRowsUnscoped[model.BotTokenModel](t, "id = ?", digestBotID) != 0 {
		t.Fatalf("digest bot token should be deleted")
	}
	if countRowsUnscoped[model.BotTokenModel](t, "id = ?", manualBotID) == 0 {
		t.Fatalf("manual bot token should remain")
	}
}

func TestCleanupWebhookBotFriendDataSkipsActiveReferencedBots(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()

	userID := "cleanup-active-user-" + utils.NewID()
	webhookBotID := "cleanup-active-webhook-" + utils.NewID()

	createTestUser(t, userID, "user_"+userID, "Cleanup Active User", false)
	createTestUser(t, webhookBotID, "webhook_"+webhookBotID, "Cleanup Active Webhook Bot", true)
	if err := db.Create(&model.BotTokenModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: webhookBotID},
		Name:              "Token " + webhookBotID,
		Token:             "token-" + webhookBotID,
		ExpiresAt:         123456789,
	}).Error; err != nil {
		t.Fatalf("create bot token failed: %v", err)
	}

	if _, err := model.FriendRelationFriendApprove(userID, webhookBotID); err != nil {
		t.Fatalf("approve friendship failed: %v", err)
	}
	if ch, _ := model.ChannelPrivateNew(userID, webhookBotID); ch == nil || ch.ID == "" {
		t.Fatalf("create private channel failed")
	}
	if err := db.Create(&model.ChannelWebhookIntegrationModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "whk-active-" + utils.NewID()},
		ChannelID:         "channel-active-" + utils.NewID(),
		Name:              "Webhook Active",
		BotUserID:         webhookBotID,
		Source:            "external",
		Status:            model.WebhookIntegrationStatusActive,
	}).Error; err != nil {
		t.Fatalf("create active webhook integration failed: %v", err)
	}
	if err := db.Model(&model.UserModel{}).Where("id = ?", webhookBotID).Update("bot_kind", model.BotKindChannelWebhook).Error; err != nil {
		t.Fatalf("update webhook bot kind failed: %v", err)
	}

	stats, err := model.CleanupWebhookBotFriendData()
	if err != nil {
		t.Fatalf("CleanupWebhookBotFriendData failed: %v", err)
	}
	if stats.WebhookBotCount != 0 {
		t.Fatalf("system bot count=%d, want 0", stats.WebhookBotCount)
	}
	if stats.ActiveReferenceSkippedCount == 0 {
		t.Fatalf("expected active referenced bots to be skipped")
	}
	if model.FriendRelationGet(userID, webhookBotID).ID == "" {
		t.Fatalf("active referenced bot friendship should remain")
	}
	if ch, _ := model.ChannelPrivateGet(userID, webhookBotID); ch == nil || ch.ID == "" {
		t.Fatalf("active referenced bot private channel should remain")
	}
	if user := model.UserGet(webhookBotID); user == nil || user.ID == "" {
		t.Fatalf("active referenced bot user should remain")
	}
	if countRowsUnscoped[model.BotTokenModel](t, "id = ?", webhookBotID) == 0 {
		t.Fatalf("active referenced bot token should remain")
	}
}
