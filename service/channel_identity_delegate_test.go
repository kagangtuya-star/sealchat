package service

import (
	"testing"
	"time"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/utils"
)

func TestWorldUpdateAllowManageOtherUserChannelIdentities(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()

	worldID := "world-manage-identities-" + utils.NewID()
	ownerID := "owner-manage-identities-" + utils.NewID()

	if err := db.Create(&model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: ownerID},
		Username:          "owner_" + worldID,
		Nickname:          "Owner",
		Password:          "pw",
		Salt:              "salt",
	}).Error; err != nil {
		t.Fatalf("create owner failed: %v", err)
	}

	if err := db.Create(&model.WorldModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: worldID},
		Name:              "Manage Identity World",
		Visibility:        model.WorldVisibilityPublic,
		Status:            "active",
		OwnerID:           ownerID,
	}).Error; err != nil {
		t.Fatalf("create world failed: %v", err)
	}

	if err := db.Create(&model.WorldMemberModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "wm-owner-" + utils.NewID()},
		WorldID:           worldID,
		UserID:            ownerID,
		Role:              model.WorldRoleOwner,
		JoinedAt:          time.Now(),
	}).Error; err != nil {
		t.Fatalf("create owner world member failed: %v", err)
	}

	enable := true
	world, err := WorldUpdate(worldID, ownerID, WorldUpdateParams{
		AllowManageOtherUserChannelIdentities: &enable,
	})
	if err != nil {
		t.Fatalf("world update failed: %v", err)
	}
	if world == nil {
		t.Fatal("world update returned nil world")
	}
	if !world.AllowManageOtherUserChannelIdentities {
		t.Fatal("expected world allowManageOtherUserChannelIdentities to be true")
	}

	disable := false
	world, err = WorldUpdate(worldID, ownerID, WorldUpdateParams{
		AllowManageOtherUserChannelIdentities: &disable,
	})
	if err != nil {
		t.Fatalf("world update disable failed: %v", err)
	}
	if world.AllowManageOtherUserChannelIdentities {
		t.Fatal("expected world allowManageOtherUserChannelIdentities to be false")
	}
}

func TestResolveChannelIdentityActor(t *testing.T) {
	initTestDB(t)
	pm.Init()
	db := model.GetDB()

	worldID := "world-channel-identity-delegate-" + utils.NewID()
	channelID := "channel-channel-identity-delegate-" + utils.NewID()
	ownerID := "owner-" + utils.NewID()
	adminAID := "admin-a-" + utils.NewID()
	adminBID := "admin-b-" + utils.NewID()
	memberID := "member-" + utils.NewID()
	spectatorID := "spectator-" + utils.NewID()
	outsiderID := "outsider-" + utils.NewID()

	users := []model.UserModel{
		{StringPKBaseModel: model.StringPKBaseModel{ID: ownerID}, Username: "owner_" + worldID, Password: "pw", Salt: "salt"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: adminAID}, Username: "adminA_" + worldID, Password: "pw", Salt: "salt"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: adminBID}, Username: "adminB_" + worldID, Password: "pw", Salt: "salt"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: memberID}, Username: "member_" + worldID, Password: "pw", Salt: "salt"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: spectatorID}, Username: "spectator_" + worldID, Password: "pw", Salt: "salt"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: outsiderID}, Username: "outsider_" + worldID, Password: "pw", Salt: "salt"},
	}
	for _, item := range users {
		user := item
		if err := db.Create(&user).Error; err != nil {
			t.Fatalf("create user %s failed: %v", user.ID, err)
		}
	}

	if err := db.Create(&model.WorldModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: worldID},
		Name:                              "Delegate World",
		Visibility:                        model.WorldVisibilityPublic,
		Status:                            "active",
		OwnerID:                           ownerID,
		AllowManageOtherUserChannelIdentities: true,
	}).Error; err != nil {
		t.Fatalf("create world failed: %v", err)
	}

	worldMembers := []model.WorldMemberModel{
		{StringPKBaseModel: model.StringPKBaseModel{ID: "wm-owner-" + utils.NewID()}, WorldID: worldID, UserID: ownerID, Role: model.WorldRoleOwner, JoinedAt: time.Now()},
		{StringPKBaseModel: model.StringPKBaseModel{ID: "wm-admin-a-" + utils.NewID()}, WorldID: worldID, UserID: adminAID, Role: model.WorldRoleAdmin, JoinedAt: time.Now()},
		{StringPKBaseModel: model.StringPKBaseModel{ID: "wm-admin-b-" + utils.NewID()}, WorldID: worldID, UserID: adminBID, Role: model.WorldRoleAdmin, JoinedAt: time.Now()},
		{StringPKBaseModel: model.StringPKBaseModel{ID: "wm-member-" + utils.NewID()}, WorldID: worldID, UserID: memberID, Role: model.WorldRoleMember, JoinedAt: time.Now()},
		{StringPKBaseModel: model.StringPKBaseModel{ID: "wm-spectator-" + utils.NewID()}, WorldID: worldID, UserID: spectatorID, Role: model.WorldRoleSpectator, JoinedAt: time.Now()},
		{StringPKBaseModel: model.StringPKBaseModel{ID: "wm-outsider-" + utils.NewID()}, WorldID: worldID, UserID: outsiderID, Role: model.WorldRoleMember, JoinedAt: time.Now()},
	}
	for _, item := range worldMembers {
		member := item
		if err := db.Create(&member).Error; err != nil {
			t.Fatalf("create world member %s failed: %v", member.ID, err)
		}
	}

	channel := ChannelNew(channelID, "non-public", "Delegate Channel", worldID, ownerID, "")
	if channel == nil {
		t.Fatal("channel create returned nil")
	}

	roleAssignments := map[string][]string{
		ownerID:     {buildChannelRoleID(channelID, "owner")},
		adminAID:    {buildChannelRoleID(channelID, "admin")},
		adminBID:    {buildChannelRoleID(channelID, "admin")},
		memberID:    {buildChannelRoleID(channelID, "member")},
		spectatorID: {buildChannelRoleID(channelID, "spectator")},
	}
	for userID, roleIDs := range roleAssignments {
		if _, err := model.UserRoleLink(roleIDs, []string{userID}); err != nil {
			t.Fatalf("link role %v to %s failed: %v", roleIDs, userID, err)
		}
	}

	ctx, err := ResolveChannelIdentityActor(channelID, adminAID, adminBID)
	if err != nil {
		t.Fatalf("admin should manage same-rank target: %v", err)
	}
	if ctx == nil || ctx.TargetUserID != adminBID || !ctx.IsDelegated {
		t.Fatalf("unexpected context for same-rank delegation: %+v", ctx)
	}

	ctx, err = ResolveChannelIdentityActor(channelID, adminAID, spectatorID)
	if err != nil {
		t.Fatalf("admin should manage spectator: %v", err)
	}
	if ctx.TargetRank != 1 || ctx.OperatorRank != 3 {
		t.Fatalf("unexpected rank result: operator=%d target=%d", ctx.OperatorRank, ctx.TargetRank)
	}

	ctx, err = ResolveChannelIdentityActor(channelID, adminAID, "")
	if err != nil {
		t.Fatalf("self context should succeed: %v", err)
	}
	if ctx.TargetUserID != adminAID || ctx.IsDelegated {
		t.Fatalf("unexpected self context: %+v", ctx)
	}

	if _, err = ResolveChannelIdentityActor(channelID, memberID, adminAID); err == nil {
		t.Fatal("member managing admin should fail")
	}

	if _, err = ResolveChannelIdentityActor(channelID, adminAID, outsiderID); err == nil {
		t.Fatal("target not in channel should fail")
	}

	if err := db.Model(&model.WorldModel{}).
		Where("id = ?", worldID).
		Update("allow_manage_other_user_channel_identities", false).Error; err != nil {
		t.Fatalf("disable world switch failed: %v", err)
	}
	if _, err = ResolveChannelIdentityActor(channelID, adminAID, memberID); err == nil {
		t.Fatal("delegation should fail when world switch is disabled")
	}
}

func TestListChannelIdentityManageCandidates(t *testing.T) {
	initTestDB(t)
	pm.Init()
	db := model.GetDB()

	worldID := "world-manage-candidates-" + utils.NewID()
	channelID := "channel-manage-candidates-" + utils.NewID()
	ownerID := "owner-" + utils.NewID()
	adminAID := "admin-a-" + utils.NewID()
	adminBID := "admin-b-" + utils.NewID()
	memberID := "member-" + utils.NewID()
	spectatorID := "spectator-" + utils.NewID()
	outsiderID := "outsider-" + utils.NewID()

	users := []model.UserModel{
		{StringPKBaseModel: model.StringPKBaseModel{ID: ownerID}, Username: "owner_" + worldID, Nickname: "Owner", Password: "pw", Salt: "salt"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: adminAID}, Username: "adminA_" + worldID, Nickname: "Admin A", Password: "pw", Salt: "salt"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: adminBID}, Username: "adminB_" + worldID, Nickname: "Admin B", Password: "pw", Salt: "salt"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: memberID}, Username: "member_" + worldID, Nickname: "Member", Password: "pw", Salt: "salt"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: spectatorID}, Username: "spectator_" + worldID, Nickname: "Spectator", Password: "pw", Salt: "salt"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: outsiderID}, Username: "outsider_" + worldID, Nickname: "Outsider", Password: "pw", Salt: "salt"},
	}
	for _, item := range users {
		user := item
		if err := db.Create(&user).Error; err != nil {
			t.Fatalf("create user %s failed: %v", user.ID, err)
		}
	}

	if err := db.Create(&model.WorldModel{
		StringPKBaseModel:                   model.StringPKBaseModel{ID: worldID},
		Name:                                "Candidate World",
		Visibility:                          model.WorldVisibilityPublic,
		Status:                              "active",
		OwnerID:                             ownerID,
		AllowManageOtherUserChannelIdentities: true,
	}).Error; err != nil {
		t.Fatalf("create world failed: %v", err)
	}

	worldMembers := []model.WorldMemberModel{
		{StringPKBaseModel: model.StringPKBaseModel{ID: "wm-owner-" + utils.NewID()}, WorldID: worldID, UserID: ownerID, Role: model.WorldRoleOwner, JoinedAt: time.Now()},
		{StringPKBaseModel: model.StringPKBaseModel{ID: "wm-admin-a-" + utils.NewID()}, WorldID: worldID, UserID: adminAID, Role: model.WorldRoleAdmin, JoinedAt: time.Now()},
		{StringPKBaseModel: model.StringPKBaseModel{ID: "wm-admin-b-" + utils.NewID()}, WorldID: worldID, UserID: adminBID, Role: model.WorldRoleAdmin, JoinedAt: time.Now()},
		{StringPKBaseModel: model.StringPKBaseModel{ID: "wm-member-" + utils.NewID()}, WorldID: worldID, UserID: memberID, Role: model.WorldRoleMember, JoinedAt: time.Now()},
		{StringPKBaseModel: model.StringPKBaseModel{ID: "wm-spectator-" + utils.NewID()}, WorldID: worldID, UserID: spectatorID, Role: model.WorldRoleSpectator, JoinedAt: time.Now()},
		{StringPKBaseModel: model.StringPKBaseModel{ID: "wm-outsider-" + utils.NewID()}, WorldID: worldID, UserID: outsiderID, Role: model.WorldRoleMember, JoinedAt: time.Now()},
	}
	for _, item := range worldMembers {
		member := item
		if err := db.Create(&member).Error; err != nil {
			t.Fatalf("create world member %s failed: %v", member.ID, err)
		}
	}

	channel := ChannelNew(channelID, "non-public", "Candidate Channel", worldID, ownerID, "")
	if channel == nil {
		t.Fatal("channel create returned nil")
	}

	roleAssignments := map[string][]string{
		ownerID:     {buildChannelRoleID(channelID, "owner")},
		adminAID:    {buildChannelRoleID(channelID, "admin")},
		adminBID:    {buildChannelRoleID(channelID, "admin")},
		memberID:    {buildChannelRoleID(channelID, "member")},
		spectatorID: {buildChannelRoleID(channelID, "spectator")},
	}
	for userID, roleIDs := range roleAssignments {
		if _, err := model.UserRoleLink(roleIDs, []string{userID}); err != nil {
			t.Fatalf("link role %v to %s failed: %v", roleIDs, userID, err)
		}
	}

	result, err := ListChannelIdentityManageCandidates(ChannelIdentityManageCandidateQuery{
		ChannelID: channelID,
		ActorID:   adminAID,
		Page:      1,
		PageSize:  20,
	})
	if err != nil {
		t.Fatalf("list candidates failed: %v", err)
	}
	got := map[string]bool{}
	for _, item := range result.Items {
		got[item.UserID] = true
	}
	if got[ownerID] {
		t.Fatal("admin should not see owner as manageable candidate")
	}
	for _, userID := range []string{adminAID, adminBID, memberID, spectatorID} {
		if !got[userID] {
			t.Fatalf("expected candidate list to include %s", userID)
		}
	}
	if got[outsiderID] {
		t.Fatal("candidate list should exclude world members not in current channel")
	}

	filtered, err := ListChannelIdentityManageCandidates(ChannelIdentityManageCandidateQuery{
		ChannelID: channelID,
		ActorID:   adminAID,
		Page:      1,
		PageSize:  20,
		Keyword:   "spectator",
	})
	if err != nil {
		t.Fatalf("list filtered candidates failed: %v", err)
	}
	if len(filtered.Items) != 1 || filtered.Items[0].UserID != spectatorID {
		t.Fatalf("expected filtered result to contain only spectator, got %+v", filtered.Items)
	}
}
