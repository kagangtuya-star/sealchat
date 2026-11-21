package service

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"sealchat/model"
	"sealchat/utils"
)

func TestWorldInviteLifecycle(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()
	world := &model.WorldModel{
		Name: "Invite World", OwnerID: "invite-owner", Visibility: model.WorldVisibilityPrivate,
	}
	if err := db.Create(world).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.WorldMemberModel{
		WorldID: world.ID, UserID: world.OwnerID, Role: model.WorldRoleOwner,
	}).Error; err != nil {
		t.Fatal(err)
	}
	createInvite := func(ttl, maxUse int) *model.WorldInviteModel {
		t.Helper()
		invite, err := WorldInviteCreate(world.ID, world.OwnerID, ttl, maxUse, "", model.WorldRoleMember)
		if err != nil {
			t.Fatal(err)
		}
		return invite
	}
	consume := func(slug, userID string, alreadyJoined bool) {
		t.Helper()
		_, joinedWorld, member, already, err := WorldInviteConsume(slug, userID)
		if err != nil || already != alreadyJoined || joinedWorld == nil || joinedWorld.ID != world.ID || member == nil {
			t.Fatalf("consume: world=%v member=%v already=%v err=%v", joinedWorld, member, already, err)
		}
	}
	reject := func(slug string) {
		t.Helper()
		if _, _, _, _, err := WorldInviteConsume(slug, "rejected-user"); !errors.Is(err, ErrWorldInviteInvalid) {
			t.Fatalf("expected invalid invite, got %v", err)
		}
	}
	checkCounts := func(invite *model.WorldInviteModel, uses int, userID string) {
		t.Helper()
		var loaded model.WorldInviteModel
		if err := db.First(&loaded, "id = ?", invite.ID).Error; err != nil {
			t.Fatal(err)
		}
		var count int64
		if err := db.Model(&model.WorldMemberModel{}).Where("world_id = ? AND user_id = ?", world.ID, userID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if loaded.UsedCount != uses || count != 1 {
			t.Fatalf("usedCount=%d members=%d, want %d and 1", loaded.UsedCount, count, uses)
		}
	}

	permanent := createInvite(0, 0)
	if permanent.ExpireAt != nil || permanent.MaxUse != 0 {
		t.Fatal("zero TTL/use limit must stay unlimited")
	}
	consume(permanent.Slug, "invite-user-1", false)
	consume(permanent.Slug, "invite-user-2", false)
	consume(permanent.Slug, "invite-user-1", true)
	checkCounts(permanent, 2, "invite-user-1")
	if _, err := WorldJoin(world.ID, "invite-user-1", model.WorldRoleMember); err != nil {
		t.Fatal(err)
	}
	checkCounts(permanent, 2, "invite-user-1")

	limited := createInvite(60, 2)
	if limited.ExpireAt == nil || !limited.ExpireAt.After(time.Now()) {
		t.Fatal("positive TTL must expire in the future")
	}
	reject(permanent.Slug) // Recreating the same role archives the old link.
	consume(limited.Slug, "invite-user-3", false)
	consume(limited.Slug, "invite-user-3", true)
	checkCounts(limited, 1, "invite-user-3")
	consume(limited.Slug, "invite-user-4", false)
	reject(limited.Slug)
	checkCounts(limited, 2, "invite-user-4")

	expired := createInvite(0, 0)
	if err := db.Model(expired).Update("expire_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	reject(expired.Slug)
	reject("missing-invite")
	reject("")
}

func initTestDB(t *testing.T) {
	t.Helper()
	cfg := &utils.AppConfig{
		DSN: fmt.Sprintf("file:service-test-%s?mode=memory&cache=shared", utils.NewID()),
		SQLite: utils.SQLiteConfig{
			EnableWAL:       false,
			TxLockImmediate: false,
			ReadConnections: 1,
			OptimizeOnInit:  false,
		},
	}
	model.DBInit(cfg)
}

func TestInitTestDBResetsDatabaseState(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()

	userID := "reset-user-" + utils.NewID()
	if err := db.Create(&model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: userID},
		Username:          "reset_" + userID,
		Nickname:          "Reset User",
		Password:          "pw",
		Salt:              "salt",
	}).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	var beforeCount int64
	if err := db.Model(&model.UserModel{}).Where("id = ?", userID).Count(&beforeCount).Error; err != nil {
		t.Fatalf("count existing user failed: %v", err)
	}
	if beforeCount != 1 {
		t.Fatalf("before reset user count=%d, want 1", beforeCount)
	}

	initTestDB(t)
	db = model.GetDB()

	var afterCount int64
	if err := db.Model(&model.UserModel{}).Where("id = ?", userID).Count(&afterCount).Error; err != nil {
		t.Fatalf("count user after reset failed: %v", err)
	}
	if afterCount != 0 {
		t.Fatalf("after reset user count=%d, want 0", afterCount)
	}
}

func TestSyncWorldChannelRolesMemberPublicOnly(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()

	worldID := "world-test"
	userID := "user-test"

	if err := db.Create(&model.WorldModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: worldID},
		Name:              "Test World",
		Status:            "active",
	}).Error; err != nil {
		t.Fatalf("create world failed: %v", err)
	}

	rootPublicID := "ch-public-root"
	rootNonPublicID := "ch-nonpublic-root"
	childPublicID := "ch-public-child"
	childNonPublicRootID := "ch-public-child-nonpublic-root"

	channels := []model.ChannelModel{
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: rootPublicID},
			WorldID:           worldID,
			Name:              "Public Root",
			PermType:          "public",
			Status:            "active",
		},
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: rootNonPublicID},
			WorldID:           worldID,
			Name:              "Non-Public Root",
			PermType:          "non-public",
			Status:            "active",
		},
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: childPublicID},
			WorldID:           worldID,
			Name:              "Public Child",
			PermType:          "public",
			Status:            "active",
			RootId:            rootPublicID,
			ParentID:          rootPublicID,
		},
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: childNonPublicRootID},
			WorldID:           worldID,
			Name:              "Public Child Under Non-Public Root",
			PermType:          "public",
			Status:            "active",
			RootId:            rootNonPublicID,
			ParentID:          rootNonPublicID,
		},
	}

	for i := range channels {
		if err := db.Create(&channels[i]).Error; err != nil {
			t.Fatalf("create channel %s failed: %v", channels[i].ID, err)
		}
	}

	if err := syncWorldChannelRoles(worldID, userID, model.WorldRoleMember); err != nil {
		t.Fatalf("syncWorldChannelRoles failed: %v", err)
	}

	var roleIDs []string
	if err := db.Model(&model.UserRoleMappingModel{}).
		Where("user_id = ?", userID).
		Pluck("role_id", &roleIDs).Error; err != nil {
		t.Fatalf("load role ids failed: %v", err)
	}

	sort.Strings(roleIDs)
	expected := []string{
		fmt.Sprintf("ch-%s-member", rootPublicID),
		fmt.Sprintf("ch-%s-member", childPublicID),
	}
	sort.Strings(expected)

	if !reflect.DeepEqual(roleIDs, expected) {
		t.Fatalf("role ids=%v expect %v", roleIDs, expected)
	}
}
