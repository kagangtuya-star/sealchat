package service

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/utils"
)

func initExternalGlossaryTestDB(t *testing.T) {
	t.Helper()
	cfg := &utils.AppConfig{
		DSN: fmt.Sprintf("file:external-glossary-test-%s?mode=memory&cache=shared", utils.NewID()),
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

func createExternalGlossaryTestUser(t *testing.T, id string) *model.UserModel {
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

func grantExternalGlossarySystemAdmin(t *testing.T, userID string) {
	t.Helper()
	if _, err := model.UserRoleLink([]string{"sys-admin"}, []string{userID}); err != nil {
		t.Fatalf("grant sys-admin to %s failed: %v", userID, err)
	}
}

func createExternalGlossaryTestWorld(t *testing.T, worldID, ownerID string) {
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

func createExternalGlossaryTestWorldMember(t *testing.T, worldID, userID, role string) {
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

func TestExternalGlossaryModelsAreMigrated(t *testing.T) {
	initExternalGlossaryTestDB(t)

	db := model.GetDB()
	if !db.Migrator().HasTable(&model.ExternalGlossaryLibraryModel{}) {
		t.Fatal("external_glossary_libraries table not migrated")
	}
	if !db.Migrator().HasTable(&model.ExternalGlossaryTermModel{}) {
		t.Fatal("external_glossary_terms table not migrated")
	}
	if !db.Migrator().HasTable(&model.ExternalGlossaryCategoryModel{}) {
		t.Fatal("external_glossary_categories table not migrated")
	}
	if !db.Migrator().HasTable(&model.WorldExternalGlossaryBindingModel{}) {
		t.Fatal("world_external_glossary_bindings table not migrated")
	}
}

func TestWorldExternalGlossaryEnableRequiresWorldAdmin(t *testing.T) {
	initExternalGlossaryTestDB(t)

	owner := createExternalGlossaryTestUser(t, "owner")
	admin := createExternalGlossaryTestUser(t, "admin")
	member := createExternalGlossaryTestUser(t, "member")
	sysAdmin := createExternalGlossaryTestUser(t, "sys-admin")
	grantExternalGlossarySystemAdmin(t, sysAdmin.ID)

	createExternalGlossaryTestWorld(t, "world-1", owner.ID)
	createExternalGlossaryTestWorldMember(t, "world-1", owner.ID, model.WorldRoleOwner)
	createExternalGlossaryTestWorldMember(t, "world-1", admin.ID, model.WorldRoleAdmin)
	createExternalGlossaryTestWorldMember(t, "world-1", member.ID, model.WorldRoleMember)

	library, err := ExternalGlossaryLibraryCreate(sysAdmin.ID, ExternalGlossaryLibraryInput{
		Name: "SRD",
	})
	if err != nil {
		t.Fatalf("create library failed: %v", err)
	}

	if err := WorldExternalGlossaryEnable("world-1", library.ID, member.ID); err != ErrWorldPermission {
		t.Fatalf("member enable err = %v, want %v", err, ErrWorldPermission)
	}

	if err := WorldExternalGlossaryEnable("world-1", library.ID, admin.ID); err != nil {
		t.Fatalf("admin enable failed: %v", err)
	}

	var count int64
	if err := model.GetDB().Model(&model.WorldExternalGlossaryBindingModel{}).
		Where("world_id = ? AND library_id = ?", "world-1", library.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count binding failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("binding count = %d, want 1", count)
	}
}

func TestEffectiveWorldKeywordListFiltersDisabledLibrariesAndPrefersWorldKeyword(t *testing.T) {
	initExternalGlossaryTestDB(t)

	owner := createExternalGlossaryTestUser(t, "world-owner")
	member := createExternalGlossaryTestUser(t, "world-member")
	sysAdmin := createExternalGlossaryTestUser(t, "sys-admin-2")
	grantExternalGlossarySystemAdmin(t, sysAdmin.ID)

	createExternalGlossaryTestWorld(t, "world-2", owner.ID)
	createExternalGlossaryTestWorldMember(t, "world-2", owner.ID, model.WorldRoleOwner)
	createExternalGlossaryTestWorldMember(t, "world-2", member.ID, model.WorldRoleMember)

	if err := model.GetDB().Create(&model.WorldKeywordModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "wk-dragon"},
		WorldID:           "world-2",
		Keyword:           "龙",
		Description:       "世界内定义",
		SortOrder:         100,
		IsEnabled:         true,
		CreatedBy:         owner.ID,
		UpdatedBy:         owner.ID,
	}).Error; err != nil {
		t.Fatalf("create world keyword failed: %v", err)
	}

	enabled := true
	disabled := false
	libEnabled, err := ExternalGlossaryLibraryCreate(sysAdmin.ID, ExternalGlossaryLibraryInput{
		Name:    "启用库",
		Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("create enabled library failed: %v", err)
	}
	libDisabled, err := ExternalGlossaryLibraryCreate(sysAdmin.ID, ExternalGlossaryLibraryInput{
		Name:    "停用库",
		Enabled: &disabled,
	})
	if err != nil {
		t.Fatalf("create disabled library failed: %v", err)
	}

	for _, item := range []*model.ExternalGlossaryTermModel{
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: "ext-dragon"},
			LibraryID:         libEnabled.ID,
			Keyword:           "龙",
			Description:       "外挂定义",
			SortOrder:         90,
			IsEnabled:         true,
			CreatedBy:         sysAdmin.ID,
			UpdatedBy:         sysAdmin.ID,
		},
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: "ext-phoenix"},
			LibraryID:         libEnabled.ID,
			Keyword:           "凤凰",
			Description:       "外挂凤凰",
			SortOrder:         80,
			IsEnabled:         true,
			CreatedBy:         sysAdmin.ID,
			UpdatedBy:         sysAdmin.ID,
		},
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: "ext-disabled"},
			LibraryID:         libDisabled.ID,
			Keyword:           "麒麟",
			Description:       "停用库术语",
			SortOrder:         70,
			IsEnabled:         true,
			CreatedBy:         sysAdmin.ID,
			UpdatedBy:         sysAdmin.ID,
		},
	} {
		if err := model.GetDB().Create(item).Error; err != nil {
			t.Fatalf("create external term %s failed: %v", item.ID, err)
		}
	}

	if err := WorldExternalGlossaryEnable("world-2", libEnabled.ID, owner.ID); err != nil {
		t.Fatalf("bind enabled library failed: %v", err)
	}
	if err := model.GetDB().Create(&model.WorldExternalGlossaryBindingModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "binding-disabled"},
		WorldID:           "world-2",
		LibraryID:         libDisabled.ID,
		CreatedBy:         owner.ID,
		UpdatedBy:         owner.ID,
	}).Error; err != nil {
		t.Fatalf("seed disabled library binding failed: %v", err)
	}

	items, err := EffectiveWorldKeywordList("world-2", member.ID, EffectiveWorldKeywordListOptions{})
	if err != nil {
		t.Fatalf("effective list failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("effective item count = %d, want 2", len(items))
	}

	keywords := make([]string, 0, len(items))
	for _, item := range items {
		keywords = append(keywords, item.Keyword)
		if item.Keyword == "龙" {
			if item.SourceType != EffectiveWorldKeywordSourceWorld {
				t.Fatalf("dragon sourceType = %q, want %q", item.SourceType, EffectiveWorldKeywordSourceWorld)
			}
			if item.Description != "世界内定义" {
				t.Fatalf("dragon description = %q, want 世界内定义", item.Description)
			}
		}
		if item.Keyword == "麒麟" {
			t.Fatalf("disabled library keyword should not appear")
		}
	}
	slices.Sort(keywords)
	if !slices.Equal(keywords, []string{"凤凰", "龙"}) {
		t.Fatalf("keywords = %v, want [凤凰 龙]", keywords)
	}
}

func TestEffectiveWorldKeywordListPrefersHigherCategoryPriority(t *testing.T) {
	initExternalGlossaryTestDB(t)

	owner := createExternalGlossaryTestUser(t, "priority-owner")
	member := createExternalGlossaryTestUser(t, "priority-member")
	sysAdmin := createExternalGlossaryTestUser(t, "priority-sys-admin")
	grantExternalGlossarySystemAdmin(t, sysAdmin.ID)

	createExternalGlossaryTestWorld(t, "world-priority", owner.ID)
	createExternalGlossaryTestWorldMember(t, "world-priority", owner.ID, model.WorldRoleOwner)
	createExternalGlossaryTestWorldMember(t, "world-priority", member.ID, model.WorldRoleMember)

	if err := model.GetDB().Create(&model.WorldKeywordCategoryModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "world-cat-low"},
		WorldID:           "world-priority",
		Name:              "低优先级分类",
		Priority:          1,
		CreatedBy:         owner.ID,
		UpdatedBy:         owner.ID,
	}).Error; err != nil {
		t.Fatalf("create world category failed: %v", err)
	}
	if err := model.GetDB().Create(&model.WorldKeywordModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "world-term-low"},
		WorldID:           "world-priority",
		Keyword:           "术语冲突",
		Category:          "低优先级分类",
		Description:       "世界低优先级定义",
		SortOrder:         200,
		IsEnabled:         true,
		CreatedBy:         owner.ID,
		UpdatedBy:         owner.ID,
	}).Error; err != nil {
		t.Fatalf("create world keyword failed: %v", err)
	}

	lib, err := ExternalGlossaryLibraryCreate(sysAdmin.ID, ExternalGlossaryLibraryInput{Name: "高优先级外挂库"})
	if err != nil {
		t.Fatalf("create external library failed: %v", err)
	}
	if err := model.GetDB().Create(&model.ExternalGlossaryCategoryModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "ext-cat-high"},
		LibraryID:         lib.ID,
		Name:              "高优先级分类",
		Priority:          10,
		CreatedBy:         sysAdmin.ID,
		UpdatedBy:         sysAdmin.ID,
	}).Error; err != nil {
		t.Fatalf("create external category failed: %v", err)
	}
	if err := model.GetDB().Create(&model.ExternalGlossaryTermModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "ext-term-high"},
		LibraryID:         lib.ID,
		Keyword:           "术语冲突",
		Category:          "高优先级分类",
		Description:       "外挂高优先级定义",
		SortOrder:         1,
		IsEnabled:         true,
		CreatedBy:         sysAdmin.ID,
		UpdatedBy:         sysAdmin.ID,
	}).Error; err != nil {
		t.Fatalf("create external term failed: %v", err)
	}
	if err := WorldExternalGlossaryEnable("world-priority", lib.ID, owner.ID); err != nil {
		t.Fatalf("bind external library failed: %v", err)
	}

	items, err := EffectiveWorldKeywordList("world-priority", member.ID, EffectiveWorldKeywordListOptions{})
	if err != nil {
		t.Fatalf("effective list failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("effective item count = %d, want 1", len(items))
	}
	if items[0].SourceType != EffectiveWorldKeywordSourceExternalLibrary {
		t.Fatalf("sourceType = %q, want %q", items[0].SourceType, EffectiveWorldKeywordSourceExternalLibrary)
	}
	if items[0].Category != "高优先级分类" {
		t.Fatalf("category = %q, want 高优先级分类", items[0].Category)
	}
	if items[0].Description != "外挂高优先级定义" {
		t.Fatalf("description = %q, want 外挂高优先级定义", items[0].Description)
	}
}

func TestEffectiveWorldKeywordListPrefersHigherCategoryPriorityOverNewerEdit(t *testing.T) {
	initExternalGlossaryTestDB(t)

	owner := createExternalGlossaryTestUser(t, "priority-edit-owner")
	member := createExternalGlossaryTestUser(t, "priority-edit-member")
	sysAdmin := createExternalGlossaryTestUser(t, "priority-edit-sys-admin")
	grantExternalGlossarySystemAdmin(t, sysAdmin.ID)

	createExternalGlossaryTestWorld(t, "world-priority-edit", owner.ID)
	createExternalGlossaryTestWorldMember(t, "world-priority-edit", owner.ID, model.WorldRoleOwner)
	createExternalGlossaryTestWorldMember(t, "world-priority-edit", member.ID, model.WorldRoleMember)

	older := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	newer := older.Add(24 * time.Hour)

	if err := model.GetDB().Create(&model.WorldKeywordCategoryModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "world-cat-low-newer"},
		WorldID:           "world-priority-edit",
		Name:              "低优先级分类",
		Priority:          1,
		CreatedBy:         owner.ID,
		UpdatedBy:         owner.ID,
	}).Error; err != nil {
		t.Fatalf("create world category failed: %v", err)
	}
	if err := model.GetDB().Create(&model.WorldKeywordModel{
		StringPKBaseModel: model.StringPKBaseModel{
			ID:        "world-term-low-newer",
			CreatedAt: newer,
			UpdatedAt: newer,
		},
		WorldID:     "world-priority-edit",
		Keyword:     "术语冲突",
		Category:    "低优先级分类",
		Description: "世界低优先级但更新更晚",
		SortOrder:   200,
		IsEnabled:   true,
		CreatedBy:   owner.ID,
		UpdatedBy:   owner.ID,
	}).Error; err != nil {
		t.Fatalf("create world keyword failed: %v", err)
	}

	lib, err := ExternalGlossaryLibraryCreate(sysAdmin.ID, ExternalGlossaryLibraryInput{Name: "高优先级外挂库-新旧验证"})
	if err != nil {
		t.Fatalf("create external library failed: %v", err)
	}
	if err := model.GetDB().Create(&model.ExternalGlossaryCategoryModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "ext-cat-high-older"},
		LibraryID:         lib.ID,
		Name:              "高优先级分类",
		Priority:          10,
		CreatedBy:         sysAdmin.ID,
		UpdatedBy:         sysAdmin.ID,
	}).Error; err != nil {
		t.Fatalf("create external category failed: %v", err)
	}
	if err := model.GetDB().Create(&model.ExternalGlossaryTermModel{
		StringPKBaseModel: model.StringPKBaseModel{
			ID:        "ext-term-high-older",
			CreatedAt: older,
			UpdatedAt: older,
		},
		LibraryID:   lib.ID,
		Keyword:     "术语冲突",
		Category:    "高优先级分类",
		Description: "外挂高优先级但更新更早",
		SortOrder:   1,
		IsEnabled:   true,
		CreatedBy:   sysAdmin.ID,
		UpdatedBy:   sysAdmin.ID,
	}).Error; err != nil {
		t.Fatalf("create external term failed: %v", err)
	}
	if err := WorldExternalGlossaryEnable("world-priority-edit", lib.ID, owner.ID); err != nil {
		t.Fatalf("bind external library failed: %v", err)
	}

	items, err := EffectiveWorldKeywordList("world-priority-edit", member.ID, EffectiveWorldKeywordListOptions{})
	if err != nil {
		t.Fatalf("effective list failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("effective item count = %d, want 1", len(items))
	}
	if items[0].SourceType != EffectiveWorldKeywordSourceExternalLibrary {
		t.Fatalf("sourceType = %q, want %q", items[0].SourceType, EffectiveWorldKeywordSourceExternalLibrary)
	}
	if items[0].Category != "高优先级分类" {
		t.Fatalf("category = %q, want 高优先级分类", items[0].Category)
	}
	if items[0].Description != "外挂高优先级但更新更早" {
		t.Fatalf("description = %q, want 外挂高优先级但更新更早", items[0].Description)
	}
}

func TestWorldKeywordCategoryRenamePreservesPriority(t *testing.T) {
	initExternalGlossaryTestDB(t)

	owner := createExternalGlossaryTestUser(t, "rename-owner")

	createExternalGlossaryTestWorld(t, "world-rename", owner.ID)
	createExternalGlossaryTestWorldMember(t, "world-rename", owner.ID, model.WorldRoleOwner)

	if err := model.GetDB().Create(&model.WorldKeywordCategoryModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "rename-cat"},
		WorldID:           "world-rename",
		Name:              "旧分类",
		Priority:          7,
		CreatedBy:         owner.ID,
		UpdatedBy:         owner.ID,
	}).Error; err != nil {
		t.Fatalf("create world category failed: %v", err)
	}

	updated, newName, err := WorldKeywordCategoryRename("world-rename", owner.ID, "旧分类", "新分类")
	if err != nil {
		t.Fatalf("rename category failed: %v", err)
	}
	if updated != 0 {
		t.Fatalf("updated = %d, want 0", updated)
	}
	if newName != "新分类" {
		t.Fatalf("newName = %q, want 新分类", newName)
	}

	var renamed model.WorldKeywordCategoryModel
	if err := model.GetDB().Where("world_id = ? AND name = ?", "world-rename", "新分类").First(&renamed).Error; err != nil {
		t.Fatalf("load renamed category failed: %v", err)
	}
	if renamed.Priority != 7 {
		t.Fatalf("priority = %d, want 7", renamed.Priority)
	}
}

func TestWorldKeywordCategoryBulkUpdatePriorityAppliesDescendingOrder(t *testing.T) {
	initExternalGlossaryTestDB(t)

	owner := createExternalGlossaryTestUser(t, "bulk-priority-owner")
	createExternalGlossaryTestWorld(t, "world-bulk-priority", owner.ID)
	createExternalGlossaryTestWorldMember(t, "world-bulk-priority", owner.ID, model.WorldRoleOwner)

	for _, item := range []model.WorldKeywordCategoryModel{
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: "bulk-cat-a"},
			WorldID:           "world-bulk-priority",
			Name:              "A",
			Priority:          1,
			CreatedBy:         owner.ID,
			UpdatedBy:         owner.ID,
		},
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: "bulk-cat-b"},
			WorldID:           "world-bulk-priority",
			Name:              "B",
			Priority:          2,
			CreatedBy:         owner.ID,
			UpdatedBy:         owner.ID,
		},
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: "bulk-cat-c"},
			WorldID:           "world-bulk-priority",
			Name:              "C",
			Priority:          3,
			CreatedBy:         owner.ID,
			UpdatedBy:         owner.ID,
		},
	} {
		if err := model.GetDB().Create(&item).Error; err != nil {
			t.Fatalf("create category %s failed: %v", item.Name, err)
		}
	}

	updated, err := WorldKeywordCategoryBulkUpdatePriority("world-bulk-priority", owner.ID, []KeywordCategoryPriorityUpdate{
		{Name: "C", Priority: 3},
		{Name: "A", Priority: 2},
		{Name: "B", Priority: 1},
	})
	if err != nil {
		t.Fatalf("bulk update priority failed: %v", err)
	}
	if updated != 3 {
		t.Fatalf("updated = %d, want 3", updated)
	}

	infos, err := WorldKeywordListCategoryInfos("world-bulk-priority", owner.ID)
	if err != nil {
		t.Fatalf("list category infos failed: %v", err)
	}
	if len(infos) != 3 {
		t.Fatalf("len(infos) = %d, want 3", len(infos))
	}
	if infos[0].Name != "C" || infos[0].Priority != 3 {
		t.Fatalf("infos[0] = %+v, want C priority 3", infos[0])
	}
	if infos[1].Name != "A" || infos[1].Priority != 2 {
		t.Fatalf("infos[1] = %+v, want A priority 2", infos[1])
	}
	if infos[2].Name != "B" || infos[2].Priority != 1 {
		t.Fatalf("infos[2] = %+v, want B priority 1", infos[2])
	}
}
