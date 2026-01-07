package service

import (
	"fmt"
	"reflect"
	"sort"
	"sync"
	"testing"

	"sealchat/model"
	"sealchat/utils"
)

var testDBOnce sync.Once

func initTestDB(t *testing.T) {
	t.Helper()
	testDBOnce.Do(func() {
		cfg := &utils.AppConfig{
			DSN: ":memory:",
			SQLite: utils.SQLiteConfig{
				EnableWAL:       false,
				TxLockImmediate: false,
				ReadConnections: 1,
				OptimizeOnInit:  false,
			},
		}
		model.DBInit(cfg)
	})
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
