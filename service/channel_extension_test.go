package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/utils"
)

func extensionServiceFixture(t *testing.T) (*UICustomizationPackage, *utils.UICustomizationPolicyConfig) {
	t.Helper()
	model.DBInit(&utils.AppConfig{DSN: fmt.Sprintf("file:extension-service-%s?mode=memory&cache=shared", utils.NewID())})
	for _, row := range []any{
		&model.WorldModel{StringPKBaseModel: model.StringPKBaseModel{ID: "W"}, Name: "W", Status: "active", InviteSlug: "extension-W"},
		&model.WorldMemberModel{WorldID: "W", UserID: "U", Role: model.WorldRoleOwner},
		&model.WorldMemberModel{WorldID: "W", UserID: "reader", Role: model.WorldRoleMember},
		&model.ChannelModel{StringPKBaseModel: model.StringPKBaseModel{ID: "C"}, WorldID: "W", Name: "C", PermType: "public", Status: "active"},
		&model.ChannelRoleModel{StringPKBaseModel: model.StringPKBaseModel{ID: "ch-C-extension"}, ChannelID: "C"},
		&model.RolePermissionModel{RoleID: "ch-C-extension", PermissionID: pm.PermFuncChannelTextSend.ID()},
		&model.UserRoleMappingModel{RoleType: "channel", UserID: "U", RoleID: "ch-C-extension"},
	} {
		if err := model.GetDB().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	pm.Init()
	policy := &utils.UICustomizationPolicyConfig{AllowWorldJS: true, AllowWorldCSS: true}
	name, js := "extension", "sc.channel()"
	surfaces, capabilities := []UISurface{UISurfaceChat}, []string{"storage.read", "storage.write", "events.subscribe", "events.publish", "messages.send", "characterCard.write", "attachments.upload"}
	pkg, err := CreateUICustomization(UICustomizationScope{Type: "world", ID: "W"}, "U", UICustomizationPatch{Name: &name, JS: &js, Surfaces: &surfaces, ChannelCapabilities: &capabilities}, policy)
	if err != nil {
		t.Fatal(err)
	}
	return pkg, policy
}

func TestChannelExtensionUICustomizationAuthorizationAndCleanup(t *testing.T) {
	pkg, policy := extensionServiceFixture(t)
	scope := ChannelExtensionScope{Type: "ui-customization", ID: pkg.ID}
	resolve := func(actor, cap string, readOnly bool, p *utils.UICustomizationPolicyConfig) error {
		_, err := ResolveChannelExtensionScope(actor, "C", scope, cap, readOnly, p)
		return err
	}
	for _, cap := range pkg.ChannelCapabilities {
		if err := resolve("U", cap, false, policy); err != nil {
			t.Fatalf("%s: %v", cap, err)
		}
	}
	for _, tc := range []struct {
		actor, cap string
		readOnly   bool
		policy     *utils.UICustomizationPolicyConfig
		code       string
	}{
		{"outsider", "storage.read", false, policy, "PERMISSION_DENIED"},
		{"U", "storage.write", true, policy, "PERMISSION_DENIED"},
		{"U", "events.publish", true, policy, "PERMISSION_DENIED"},
		{"U", "characterCard.write", true, policy, "PERMISSION_DENIED"},
		{"U", "attachments.upload", true, policy, "PERMISSION_DENIED"},
		{"reader", "storage.write", true, policy, "PERMISSION_DENIED"},
		{"reader", "messages.send", false, policy, "PERMISSION_DENIED"},
		{"reader", "events.publish", false, policy, "PERMISSION_DENIED"},
		{"U", "storage.read", false, nil, "CAPABILITY_DENIED"},
	} {
		if err := resolve(tc.actor, tc.cap, tc.readOnly, tc.policy); err == nil || !strings.HasPrefix(err.Error(), tc.code) {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
	if err := resolve("reader", "storage.read", false, policy); err != nil {
		t.Fatal("reader lost read access", err)
	}
	for _, cap := range []string{"storage.write", "characterCard.write", "attachments.upload"} {
		if err := resolve("reader", cap, false, policy); err != nil {
			t.Fatalf("%s incorrectly requires channel text send permission: %v", cap, err)
		}
	}
	for _, change := range []map[string]any{
		{"js": ""}, {"scope_id": "other-world"}, {"channel_capabilities_json": `[]`},
	} {
		if err := model.GetDB().Model(&model.UICustomizationPackageModel{}).Where("id = ?", pkg.ID).Updates(change).Error; err != nil {
			t.Fatal(err)
		}
		if err := resolve("U", "storage.read", false, policy); err == nil || !strings.HasPrefix(err.Error(), "CAPABILITY_DENIED") {
			t.Fatalf("saved package state ignored: %+v: %v", change, err)
		}
		caps, _ := json.Marshal(pkg.ChannelCapabilities)
		if err := model.GetDB().Model(&model.UICustomizationPackageModel{}).Where("id = ?", pkg.ID).Updates(map[string]any{"js": pkg.JS, "scope_id": "W", "channel_capabilities_json": string(caps)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, channel := range []string{"C", "D"} {
		if _, err := model.ChannelExtensionStorageSet(channel, scope.StorageNamespaceKey(), "k", json.RawMessage(`1`), nil, "U"); err != nil {
			t.Fatal(err)
		}
	}
	newJS := "// updated\nsc.channel()"
	if _, err := UpdateUICustomization(UICustomizationScope{Type: "world", ID: "W"}, "U", pkg.ID, UICustomizationPatch{JS: &newJS}, policy); err != nil {
		t.Fatal(err)
	}
	if item, _ := model.ChannelExtensionStorageGet("C", scope.StorageNamespaceKey(), "k"); item == nil {
		t.Fatal("update removed data")
	}
	disabled := false
	if _, err := UpdateUICustomization(UICustomizationScope{Type: "world", ID: "W"}, "U", pkg.ID, UICustomizationPatch{Enabled: &disabled}, policy); err != nil {
		t.Fatal(err)
	}
	if err := resolve("U", "storage.read", false, policy); err == nil {
		t.Fatal("disabled package authorized")
	}
	if item, _ := model.ChannelExtensionStorageGet("C", scope.StorageNamespaceKey(), "k"); item == nil {
		t.Fatal("disable removed data")
	}
	if err := DeleteUICustomization(UICustomizationScope{Type: "world", ID: "W"}, "U", pkg.ID); err != nil {
		t.Fatal(err)
	}
	for _, channel := range []string{"C", "D"} {
		if item, _ := model.ChannelExtensionStorageGet(channel, scope.StorageNamespaceKey(), "k"); item != nil {
			t.Fatal("delete did not clean package storage")
		}
	}
	if err := resolve("U", "storage.read", false, policy); err == nil || !strings.HasPrefix(err.Error(), "NOT_FOUND") {
		t.Fatal("deleted package", err)
	}
}

func TestChannelExtensionFingerprintTransferAndRuntime(t *testing.T) {
	pkg, policy := extensionServiceFixture(t)
	if pkg.RuntimeFingerprint == pkg.JSHash {
		t.Fatal("capabilities not bound to trust")
	}
	if UICustomizationRuntimeFingerprint(pkg.JS, nil) != pkg.JSHash {
		t.Fatal("legacy empty-capability trust broken")
	}
	a := UICustomizationRuntimeFingerprint(pkg.JS, []string{"storage.read", "storage.write"})
	if a != UICustomizationRuntimeFingerprint(pkg.JS, []string{"storage.write", "storage.read"}) {
		t.Fatal("order changed trust")
	}
	if a == UICustomizationRuntimeFingerprint(pkg.JS, []string{"storage.read"}) {
		t.Fatal("removed capability retained trust")
	}
	runtime, err := ResolveUICustomizations(UICustomizationContext{Surface: UISurfaceChat, WorldID: "W", ChannelID: "C"}, "U", policy)
	if err != nil || len(runtime.Packages) != 1 || runtime.Packages[0].RuntimeFingerprint != pkg.RuntimeFingerprint {
		t.Fatalf("runtime fields: %+v %v", runtime, err)
	}
	for _, version := range []int{1, 2} {
		raw := []byte(fmt.Sprintf(`{"format":"sealchat.ui-package","formatVersion":1,"runtimeApiVersion":%d,"manifest":{"name":"imported"},"targets":["chat"],"js":"sc.channel()","channelCapabilities":["storage.read"]}`, version))
		items, err := ImportUICustomizations(UICustomizationScope{Type: "world", ID: "W"}, "U", raw, policy)
		if err != nil || len(items) != 1 || len(items[0].ChannelCapabilities) != 1 || items[0].Enabled {
			t.Fatalf("transfer v%d: %+v %v", version, items, err)
		}
	}
	legacy, err := ImportUICustomizations(UICustomizationScope{Type: "world", ID: "W"}, "U", []byte(`{"format":"sealchat.ui-package","formatVersion":1,"manifest":{"name":"legacy"},"targets":["chat"],"js":"sc.context()"}`), policy)
	if err != nil || len(legacy) != 1 || legacy[0].ChannelCapabilities == nil || len(legacy[0].ChannelCapabilities) != 0 || legacy[0].RuntimeFingerprint != legacy[0].JSHash {
		t.Fatalf("legacy import silently granted capabilities or lost trust compatibility: %+v %v", legacy, err)
	}
	bad := []string{"not.a.capability"}
	if _, err := UpdateUICustomization(UICustomizationScope{Type: "world", ID: "W"}, "U", pkg.ID, UICustomizationPatch{ChannelCapabilities: &bad}, policy); err == nil {
		t.Fatal("invalid capability accepted")
	}
}

func TestChannelExtensionIFormSavedPolicyAndMissingTemplate(t *testing.T) {
	_, policy := extensionServiceFixture(t)
	for _, row := range []any{
		&model.ChannelIFormModel{StringPKBaseModel: model.StringPKBaseModel{ID: "enabled"}, ChannelID: "C", BridgePolicy: model.ChannelIFormBridgePolicy{Enabled: true, Capabilities: []string{"storage.read", "storage.write", "messages.send", "events.publish"}}},
		&model.ChannelIFormModel{StringPKBaseModel: model.StringPKBaseModel{ID: "local-missing"}, ChannelID: "C", TemplateRef: "platform:missing", BridgePolicy: model.ChannelIFormBridgePolicy{Enabled: true, Capabilities: []string{"storage.read"}}},
		&model.ChannelIFormModel{StringPKBaseModel: model.StringPKBaseModel{ID: "shared-missing"}, ChannelID: "D", TemplateRef: "platform:missing", BridgePolicy: model.ChannelIFormBridgePolicy{Enabled: true, Capabilities: []string{"storage.read"}}},
		&model.WorldIFormBindingModel{WorldID: "W", FormID: "shared-missing"},
		&model.ChannelIFormModel{StringPKBaseModel: model.StringPKBaseModel{ID: "disabled"}, ChannelID: "C", BridgePolicy: model.ChannelIFormBridgePolicy{Capabilities: []string{"storage.read"}}},
	} {
		if err := model.GetDB().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	for id, code := range map[string]string{"local-missing": "NOT_FOUND", "shared-missing": "NOT_FOUND", "disabled": "CAPABILITY_DENIED", "not-saved": "NOT_FOUND"} {
		if _, err := ResolveChannelExtensionScope("U", "C", ChannelExtensionScope{Type: "iform", ID: id}, "storage.read", false, policy); err == nil || !strings.HasPrefix(err.Error(), code) {
			t.Fatalf("%s: %v", id, err)
		}
	}
	scope := ChannelExtensionScope{Type: "iform", ID: "enabled"}
	if _, err := ResolveChannelExtensionScope("reader", "C", scope, "storage.read", false, policy); err != nil {
		t.Fatal("iForm read access changed", err)
	}
	for _, capability := range []string{"storage.write", "messages.send", "events.publish"} {
		if _, err := ResolveChannelExtensionScope("reader", "C", scope, capability, false, policy); err == nil || !strings.HasPrefix(err.Error(), "PERMISSION_DENIED") {
			t.Fatalf("iForm %s no longer requires text send permission: %v", capability, err)
		}
	}
}

func TestChannelExtensionPlatformWorldActivationMatchesRuntime(t *testing.T) {
	_, policy := extensionServiceFixture(t)
	pkg := model.UICustomizationPackageModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "platform-extension"},
		ScopeType:         "platform", Name: "platform", JS: "sc.channel()", SurfacesJSON: `["chat"]`,
		ChannelCapabilitiesJSON: `["storage.read","storage.write"]`,
	}
	for _, row := range []any{
		&pkg,
		&model.WorldModel{StringPKBaseModel: model.StringPKBaseModel{ID: "V"}, Name: "V", Status: "active", InviteSlug: "extension-V"},
		&model.WorldMemberModel{WorldID: "V", UserID: "reader", Role: model.WorldRoleMember},
		&model.ChannelModel{StringPKBaseModel: model.StringPKBaseModel{ID: "D"}, WorldID: "V", PermType: "public", Status: "active"},
	} {
		if err := model.GetDB().Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	cssDisabled := &utils.UICustomizationPolicyConfig{AllowWorldJS: true}
	jsDisabled := &utils.UICustomizationPolicyConfig{AllowWorldCSS: true}
	for _, tc := range []struct {
		name                string
		enabled, referenced bool
		worldID, channelID  string
		policy              *utils.UICustomizationPolicyConfig
		allowed             bool
	}{
		{"disabled", false, false, "W", "C", policy, false},
		{"world-reference", false, true, "W", "C", policy, true},
		{"reference-is-world-local", false, true, "V", "D", policy, false},
		{"reference-css-policy", false, true, "W", "C", cssDisabled, false},
		{"platform-js-policy", false, true, "W", "C", jsDisabled, true},
		{"global", true, false, "W", "C", policy, true},
		{"global-css-policy", true, false, "W", "C", cssDisabled, true},
		{"global-other-world", true, false, "V", "D", policy, true},
		{"global-with-reference", true, true, "W", "C", policy, true},
		{"global-reference-removed", true, false, "W", "C", policy, true},
		{"reference-restored", false, true, "W", "C", policy, true},
		{"reference-removed", false, false, "W", "C", policy, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := model.GetDB().Model(&pkg).Update("enabled", tc.enabled).Error; err != nil {
				t.Fatal(err)
			}
			if err := SetWorldPlatformUICustomizationReference("W", "U", pkg.ID, tc.referenced); err != nil {
				t.Fatal(err)
			}
			runtime, err := ResolveUICustomizations(UICustomizationContext{Surface: UISurfaceChat, WorldID: tc.worldID, ChannelID: tc.channelID}, "reader", tc.policy)
			if err != nil {
				t.Fatal(err)
			}
			loaded := false
			for _, item := range runtime.Packages {
				if item.ID == pkg.ID && item.JS == pkg.JS {
					loaded = true
				}
			}
			if loaded != tc.allowed {
				t.Fatalf("runtime loaded=%v, want %v", loaded, tc.allowed)
			}
			for _, capability := range []string{"storage.read", "storage.write"} {
				_, err := ResolveChannelExtensionScope("reader", tc.channelID, ChannelExtensionScope{Type: "ui-customization", ID: pkg.ID}, capability, false, tc.policy)
				if tc.allowed && err != nil {
					t.Fatalf("%s: %v", capability, err)
				}
				if !tc.allowed && (err == nil || !strings.Contains(err.Error(), "CAPABILITY_DENIED: package is not enabled in this world")) {
					t.Fatalf("%s: missing activation denial: %v", capability, err)
				}
			}
		})
	}
}
