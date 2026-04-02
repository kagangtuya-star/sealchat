package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/service"
	"sealchat/utils"
)

func initExternalGlossaryAPITestDB(t *testing.T) {
	t.Helper()
	model.DBInit(&utils.AppConfig{
		DSN: fmt.Sprintf("file:api-external-glossary-%s?mode=memory&cache=shared", utils.NewID()),
		SQLite: utils.SQLiteConfig{
			EnableWAL:       false,
			TxLockImmediate: false,
			ReadConnections: 1,
			OptimizeOnInit:  false,
		},
	})
	pm.Init()
}

func createExternalGlossaryAPIUser(t *testing.T, id string) *model.UserModel {
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

func createExternalGlossaryAPIWorld(t *testing.T, worldID, ownerID string) {
	t.Helper()
	if err := model.GetDB().Create(&model.WorldModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: worldID},
		Name:              "World " + worldID,
		OwnerID:           ownerID,
		Status:            "active",
		Visibility:        model.WorldVisibilityPublic,
	}).Error; err != nil {
		t.Fatalf("create world failed: %v", err)
	}
}

func createExternalGlossaryAPIWorldMember(t *testing.T, worldID, userID, role string) {
	t.Helper()
	if err := model.GetDB().Create(&model.WorldMemberModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "wm-" + userID + "-" + utils.NewIDWithLength(6)},
		WorldID:           worldID,
		UserID:            userID,
		Role:              role,
		JoinedAt:          time.Now(),
	}).Error; err != nil {
		t.Fatalf("create world member failed: %v", err)
	}
}

func grantExternalGlossaryAPISystemAdmin(t *testing.T, userID string) {
	t.Helper()
	if _, err := model.UserRoleLink([]string{"sys-admin"}, []string{userID}); err != nil {
		t.Fatalf("grant sys-admin failed: %v", err)
	}
}

func newExternalGlossaryAPIApp(user *model.UserModel, register func(app *fiber.App)) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		if user != nil {
			c.Locals("user", user)
		}
		return c.Next()
	})
	register(app)
	return app
}

func TestWorldExternalGlossaryEnableHandlerRejectsMember(t *testing.T) {
	initExternalGlossaryAPITestDB(t)

	owner := createExternalGlossaryAPIUser(t, "owner")
	member := createExternalGlossaryAPIUser(t, "member")
	sysAdmin := createExternalGlossaryAPIUser(t, "sys-admin")
	grantExternalGlossaryAPISystemAdmin(t, sysAdmin.ID)

	createExternalGlossaryAPIWorld(t, "world-api-1", owner.ID)
	createExternalGlossaryAPIWorldMember(t, "world-api-1", owner.ID, model.WorldRoleOwner)
	createExternalGlossaryAPIWorldMember(t, "world-api-1", member.ID, model.WorldRoleMember)

	library, err := service.ExternalGlossaryLibraryCreate(sysAdmin.ID, service.ExternalGlossaryLibraryInput{Name: "API 库"})
	if err != nil {
		t.Fatalf("create library failed: %v", err)
	}

	app := newExternalGlossaryAPIApp(member, func(app *fiber.App) {
		app.Post("/api/v1/worlds/:worldId/external-glossaries/:libraryId/enable", WorldExternalGlossaryEnableHandler)
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/worlds/world-api-1/external-glossaries/"+library.ID+"/enable", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestEffectiveWorldKeywordListHandlerReturnsMergedItems(t *testing.T) {
	initExternalGlossaryAPITestDB(t)

	owner := createExternalGlossaryAPIUser(t, "owner-2")
	member := createExternalGlossaryAPIUser(t, "member-2")
	sysAdmin := createExternalGlossaryAPIUser(t, "sys-admin-2")
	grantExternalGlossaryAPISystemAdmin(t, sysAdmin.ID)

	createExternalGlossaryAPIWorld(t, "world-api-2", owner.ID)
	createExternalGlossaryAPIWorldMember(t, "world-api-2", owner.ID, model.WorldRoleOwner)
	createExternalGlossaryAPIWorldMember(t, "world-api-2", member.ID, model.WorldRoleMember)

	if err := model.GetDB().Create(&model.WorldKeywordModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "wk-api"},
		WorldID:           "world-api-2",
		Keyword:           "龙",
		Description:       "世界定义",
		IsEnabled:         true,
		SortOrder:         100,
		CreatedBy:         owner.ID,
		UpdatedBy:         owner.ID,
	}).Error; err != nil {
		t.Fatalf("create world keyword failed: %v", err)
	}
	lib, err := service.ExternalGlossaryLibraryCreate(sysAdmin.ID, service.ExternalGlossaryLibraryInput{Name: "外部库"})
	if err != nil {
		t.Fatalf("create external library failed: %v", err)
	}
	if err := model.GetDB().Create(&model.ExternalGlossaryTermModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "ext-api"},
		LibraryID:         lib.ID,
		Keyword:           "凤凰",
		Description:       "外挂定义",
		IsEnabled:         true,
		SortOrder:         90,
		CreatedBy:         sysAdmin.ID,
		UpdatedBy:         sysAdmin.ID,
	}).Error; err != nil {
		t.Fatalf("create external term failed: %v", err)
	}
	if err := service.WorldExternalGlossaryEnable("world-api-2", lib.ID, owner.ID); err != nil {
		t.Fatalf("enable external glossary failed: %v", err)
	}

	app := newExternalGlossaryAPIApp(member, func(app *fiber.App) {
		app.Get("/api/v1/worlds/:worldId/keywords/effective", EffectiveWorldKeywordListHandler)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/worlds/world-api-2/keywords/effective", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var payload struct {
		Items []service.EffectiveWorldKeywordItem `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if len(payload.Items) != 2 {
		t.Fatalf("item count = %d, want 2", len(payload.Items))
	}
}
