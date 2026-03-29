package service

import (
	"fmt"
	"reflect"
	"sort"
	"testing"

	"sealchat/model"
	"sealchat/utils"
)

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
