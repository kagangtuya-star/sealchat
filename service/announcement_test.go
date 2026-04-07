package service

import (
	"fmt"
	"testing"
	"time"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/utils"
)

func initAnnouncementTestDB(t *testing.T) {
	t.Helper()
	cfg := &utils.AppConfig{
		DSN: fmt.Sprintf("file:announcement-test-%s?mode=memory&cache=shared", utils.NewID()),
		SQLite: utils.SQLiteConfig{
			EnableWAL:       false,
			TxLockImmediate: false,
			ReadConnections: 1,
			OptimizeOnInit:  false,
		},
	}
	model.DBInit(cfg)
	pm.Init()
}

func createAnnouncementTestUser(t *testing.T, id string) *model.UserModel {
	t.Helper()
	user := &model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: id},
		Username:          "user_" + id,
		Nickname:          "nick_" + id,
		Password:          "pw",
		Salt:              "salt",
	}
	if err := model.GetDB().Create(user).Error; err != nil {
		t.Fatalf("create user %s failed: %v", id, err)
	}
	return user
}

func createAnnouncementTestWorld(t *testing.T, worldID, ownerID string) {
	t.Helper()
	world := &model.WorldModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: worldID},
		Name:              "World " + worldID,
		OwnerID:           ownerID,
		Status:            "active",
		Visibility:        model.WorldVisibilityPublic,
	}
	if err := model.GetDB().Create(world).Error; err != nil {
		t.Fatalf("create world %s failed: %v", worldID, err)
	}
}

func createAnnouncementTestWorldMember(t *testing.T, worldID, userID, role string) {
	t.Helper()
	member := &model.WorldMemberModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "wm-" + userID + "-" + utils.NewIDWithLength(6)},
		WorldID:           worldID,
		UserID:            userID,
		Role:              role,
		JoinedAt:          time.Now(),
	}
	if err := model.GetDB().Create(member).Error; err != nil {
		t.Fatalf("create world member %s failed: %v", userID, err)
	}
}

func grantAnnouncementSystemAdmin(t *testing.T, userID string) {
	t.Helper()
	if _, err := model.UserRoleLink([]string{"sys-admin"}, []string{userID}); err != nil {
		t.Fatalf("grant sys-admin to %s failed: %v", userID, err)
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func TestAnnouncementCreateWorldScopeForcesTickerOff(t *testing.T) {
	initAnnouncementTestDB(t)

	owner := createAnnouncementTestUser(t, "world-owner")
	createAnnouncementTestWorld(t, "world-ticker", owner.ID)
	createAnnouncementTestWorldMember(t, "world-ticker", owner.ID, model.WorldRoleOwner)

	item, err := AnnouncementCreate("world", "world-ticker", owner.ID, AnnouncementInput{
		Title:         "世界公告",
		Content:       "内容",
		ContentFormat: "plain",
		Status:        "published",
		ShowInTicker:  true,
	})
	if err != nil {
		t.Fatalf("create world announcement failed: %v", err)
	}

	if item.ShowInTicker {
		t.Fatalf("world announcement showInTicker=%v, want false", item.ShowInTicker)
	}

	stored, err := loadAnnouncementOrError(item.ID)
	if err != nil {
		t.Fatalf("load announcement failed: %v", err)
	}
	if stored.ShowInTicker {
		t.Fatalf("stored world announcement showInTicker=%v, want false", stored.ShowInTicker)
	}
}

func TestAnnouncementListLobbyTickerFilterOnlyReturnsPublishedTickerItems(t *testing.T) {
	initAnnouncementTestDB(t)

	admin := createAnnouncementTestUser(t, "lobby-admin")
	reader := createAnnouncementTestUser(t, "lobby-reader")
	grantAnnouncementSystemAdmin(t, admin.ID)

	if _, err := AnnouncementCreate("lobby", "", admin.ID, AnnouncementInput{
		Title:         "普通大厅公告",
		Content:       "不进广播区",
		ContentFormat: "plain",
		Status:        "published",
		ShowInTicker:  false,
	}); err != nil {
		t.Fatalf("create non ticker announcement failed: %v", err)
	}

	expected, err := AnnouncementCreate("lobby", "", admin.ID, AnnouncementInput{
		Title:         "广播公告",
		Content:       "进入广播区",
		ContentFormat: "plain",
		Status:        "published",
		ShowInTicker:  true,
	})
	if err != nil {
		t.Fatalf("create ticker announcement failed: %v", err)
	}

	if _, err := AnnouncementCreate("lobby", "", admin.ID, AnnouncementInput{
		Title:         "草稿广播公告",
		Content:       "不应出现在发布列表",
		ContentFormat: "plain",
		Status:        "draft",
		ShowInTicker:  true,
	}); err != nil {
		t.Fatalf("create draft ticker announcement failed: %v", err)
	}

	items, total, err := AnnouncementList("lobby", "", reader.ID, AnnouncementListOptions{
		ShowInTicker: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("list ticker announcements failed: %v", err)
	}

	if total != 1 {
		t.Fatalf("ticker total=%d, want 1", total)
	}
	if len(items) != 1 {
		t.Fatalf("ticker len=%d, want 1", len(items))
	}
	if items[0].ID != expected.ID {
		t.Fatalf("ticker id=%s, want %s", items[0].ID, expected.ID)
	}
	if !items[0].ShowInTicker {
		t.Fatalf("ticker item showInTicker=%v, want true", items[0].ShowInTicker)
	}
}
