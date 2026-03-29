package service

import (
	"testing"
	"time"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/protocol"
	"sealchat/utils"
)

type delegatedIdentityAvatarTestContext struct {
	channelID              string
	operatorUserID         string
	targetUserID           string
	identityID             string
	variantID              string
	attachmentID           string
	decorationAttachmentID string
}

func setupDelegatedIdentityAvatarTestContext(t *testing.T) *delegatedIdentityAvatarTestContext {
	t.Helper()

	initTestDB(t)
	pm.Init()
	db := model.GetDB()

	worldID := "world-delegated-avatar-" + utils.NewID()
	channelID := "channel-delegated-avatar-" + utils.NewID()
	worldOwnerID := "world-owner-" + utils.NewID()
	operatorUserID := "admin-" + utils.NewID()
	targetUserID := "member-" + utils.NewID()
	identityID := "identity-" + utils.NewID()
	variantID := "variant-" + utils.NewID()
	attachmentID := "attachment-" + utils.NewID()
	decorationAttachmentID := "decoration-" + utils.NewID()

	users := []model.UserModel{
		{StringPKBaseModel: model.StringPKBaseModel{ID: worldOwnerID}, Username: "owner_" + worldID, Password: "pw", Salt: "salt"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: operatorUserID}, Username: "admin_" + worldID, Password: "pw", Salt: "salt"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: targetUserID}, Username: "member_" + worldID, Password: "pw", Salt: "salt"},
	}
	for _, item := range users {
		user := item
		if err := db.Create(&user).Error; err != nil {
			t.Fatalf("create user %s failed: %v", user.ID, err)
		}
	}

	if err := db.Create(&model.WorldModel{
		StringPKBaseModel:                     model.StringPKBaseModel{ID: worldID},
		Name:                                  "Delegated Avatar World",
		Visibility:                            model.WorldVisibilityPublic,
		Status:                                "active",
		OwnerID:                               worldOwnerID,
		AllowManageOtherUserChannelIdentities: true,
	}).Error; err != nil {
		t.Fatalf("create world failed: %v", err)
	}

	worldMembers := []model.WorldMemberModel{
		{StringPKBaseModel: model.StringPKBaseModel{ID: "wm-owner-" + utils.NewID()}, WorldID: worldID, UserID: worldOwnerID, Role: model.WorldRoleOwner, JoinedAt: time.Now()},
		{StringPKBaseModel: model.StringPKBaseModel{ID: "wm-admin-" + utils.NewID()}, WorldID: worldID, UserID: operatorUserID, Role: model.WorldRoleAdmin, JoinedAt: time.Now()},
		{StringPKBaseModel: model.StringPKBaseModel{ID: "wm-member-" + utils.NewID()}, WorldID: worldID, UserID: targetUserID, Role: model.WorldRoleMember, JoinedAt: time.Now()},
	}
	for _, item := range worldMembers {
		member := item
		if err := db.Create(&member).Error; err != nil {
			t.Fatalf("create world member %s failed: %v", member.ID, err)
		}
	}

	channel := ChannelNew(channelID, "non-public", "Delegated Avatar Channel", worldID, worldOwnerID, "")
	if channel == nil {
		t.Fatal("channel create returned nil")
	}

	roleAssignments := map[string][]string{
		worldOwnerID:   {buildChannelRoleID(channelID, "owner")},
		operatorUserID: {buildChannelRoleID(channelID, "admin")},
		targetUserID:   {buildChannelRoleID(channelID, "member")},
	}
	for userID, roleIDs := range roleAssignments {
		if _, err := model.UserRoleLink(roleIDs, []string{userID}); err != nil {
			t.Fatalf("link role %v to %s failed: %v", roleIDs, userID, err)
		}
	}

	if _, err := ResolveChannelIdentityActor(channelID, operatorUserID, targetUserID); err != nil {
		t.Fatalf("resolve delegated actor failed: %v", err)
	}

	if err := db.Create(&model.AttachmentModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: attachmentID},
		UserID:            targetUserID,
		ChannelID:         "user-avatar",
		Filename:          "target-avatar.png",
		MimeType:          "image/png",
		Size:              1024,
	}).Error; err != nil {
		t.Fatalf("create attachment failed: %v", err)
	}
	if err := db.Create(&model.AttachmentModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: decorationAttachmentID},
		UserID:            targetUserID,
		ChannelID:         "user-avatar",
		Filename:          "target-decoration.png",
		MimeType:          "image/png",
		Size:              2048,
	}).Error; err != nil {
		t.Fatalf("create decoration attachment failed: %v", err)
	}

	if err := db.Create(&model.ChannelIdentityModel{
		StringPKBaseModel:  model.StringPKBaseModel{ID: identityID},
		ChannelID:          channelID,
		UserID:             targetUserID,
		DisplayName:        "Target Identity",
		Color:              "#336699",
		AvatarAttachmentID: "id:" + attachmentID,
		SortOrder:          1,
		IsDefault:          true,
	}).Error; err != nil {
		t.Fatalf("create channel identity failed: %v", err)
	}

	if err := db.Create(&model.ChannelIdentityVariantModel{
		StringPKBaseModel:  model.StringPKBaseModel{ID: variantID},
		IdentityID:         identityID,
		ChannelID:          channelID,
		UserID:             targetUserID,
		SelectorEmoji:      "🎭",
		Keyword:            "alt",
		Note:               "before",
		AvatarAttachmentID: "id:" + attachmentID,
		SortOrder:          1,
		Enabled:            true,
	}).Error; err != nil {
		t.Fatalf("create identity variant failed: %v", err)
	}

	return &delegatedIdentityAvatarTestContext{
		channelID:              channelID,
		operatorUserID:         operatorUserID,
		targetUserID:           targetUserID,
		identityID:             identityID,
		variantID:              variantID,
		attachmentID:           attachmentID,
		decorationAttachmentID: decorationAttachmentID,
	}
}

func TestChannelIdentityUpdateWithAccessAllowsTargetOwnedAvatar(t *testing.T) {
	ctx := setupDelegatedIdentityAvatarTestContext(t)

	updated, err := ChannelIdentityUpdateWithAccess(ctx.targetUserID, ctx.operatorUserID, ctx.identityID, &ChannelIdentityInput{
		ChannelID:          ctx.channelID,
		DisplayName:        "Target Identity Updated",
		Color:              "#224466",
		AvatarAttachmentID: "id:" + ctx.attachmentID,
		IsDefault:          true,
	})
	if err != nil {
		t.Fatalf("expected delegated identity update to keep target-owned avatar, got error: %v", err)
	}
	if updated == nil {
		t.Fatal("expected updated identity")
	}
	if updated.AvatarAttachmentID != "id:"+ctx.attachmentID {
		t.Fatalf("expected avatar attachment to stay unchanged, got %q", updated.AvatarAttachmentID)
	}
}

func TestChannelIdentityVariantUpdateWithAccessAllowsTargetOwnedAvatar(t *testing.T) {
	ctx := setupDelegatedIdentityAvatarTestContext(t)

	updated, err := ChannelIdentityVariantUpdateWithAccess(ctx.targetUserID, ctx.operatorUserID, ctx.variantID, &ChannelIdentityVariantInput{
		ChannelID:          ctx.channelID,
		IdentityID:         ctx.identityID,
		SelectorEmoji:      "🎯",
		Keyword:            "alt",
		Note:               "updated",
		AvatarAttachmentID: "id:" + ctx.attachmentID,
		Enabled:            true,
	})
	if err != nil {
		t.Fatalf("expected delegated variant update to keep target-owned avatar, got error: %v", err)
	}
	if updated == nil {
		t.Fatal("expected updated variant")
	}
	if updated.AvatarAttachmentID != "id:"+ctx.attachmentID {
		t.Fatalf("expected variant avatar attachment to stay unchanged, got %q", updated.AvatarAttachmentID)
	}
}

func TestChannelIdentityUpdateWithAccessAllowsTargetOwnedAvatarDecoration(t *testing.T) {
	ctx := setupDelegatedIdentityAvatarTestContext(t)

	updated, err := ChannelIdentityUpdateWithAccess(ctx.targetUserID, ctx.operatorUserID, ctx.identityID, &ChannelIdentityInput{
		ChannelID:          ctx.channelID,
		DisplayName:        "Target Identity Updated",
		Color:              "#224466",
		AvatarAttachmentID: "id:" + ctx.attachmentID,
		AvatarDecorations: protocol.AvatarDecorationList{
			{
				Enabled:              true,
				ResourceAttachmentID: "id:" + ctx.decorationAttachmentID,
			},
		},
		IsDefault: true,
	})
	if err != nil {
		t.Fatalf("expected delegated identity update to keep target-owned avatar decoration, got error: %v", err)
	}
	if updated == nil {
		t.Fatal("expected updated identity")
	}
	if len(updated.AvatarDecorations) != 1 {
		t.Fatalf("expected avatar decoration to be preserved, got %d", len(updated.AvatarDecorations))
	}
	if updated.AvatarDecorations[0].ResourceAttachmentID != "id:"+ctx.decorationAttachmentID {
		t.Fatalf("expected avatar decoration resource to stay unchanged, got %q", updated.AvatarDecorations[0].ResourceAttachmentID)
	}
}
