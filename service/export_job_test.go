package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"sealchat/model"
)

func TestChannelExportVisibility(t *testing.T) {
	for _, tc := range []struct {
		name, user                string
		respect, strict, filtered bool
	}{
		{"member visibility", "export-member", true, true, true},
		{"member full", "export-member", false, true, false},
		{"admin full", "export-admin", true, false, false},
		{"admin strict", "export-admin", true, true, true},
		{"admin strict overrides full", "export-admin", false, true, true},
		{"owner full", "export-owner", true, false, false},
		{"owner strict", "export-owner", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logExportFixture(t, nil)
			createExportTestUser(t, "export-admin", "export-admin", "Admin")
			if err := model.GetDB().Create(&model.WorldMemberModel{WorldID: "export-world", UserID: "export-admin", Role: model.WorldRoleAdmin}).Error; err != nil {
				t.Fatal(err)
			}
			if err := model.GetDB().Model(&model.WorldModel{}).Where("id = ?", "export-world").Updates(map[string]any{
				"export_respect_message_visibility": tc.respect, "strict_whisper_privacy": tc.strict,
			}).Error; err != nil {
				t.Fatal(err)
			}
			seedExportVisibilityMessages(t, "export-a", tc.user)
			job, err := CreateMessageExportJob(&ExportJobOptions{UserID: tc.user, ChannelID: "export-a", Format: "txt", IncludeOOC: true, IncludeImages: true, IncludeDiceCommand: true})
			if err != nil {
				t.Fatal(err)
			}
			messages, err := loadMessagesForExport(job)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, m := range messages {
				got = append(got, m.Content)
			}
			want := []string{"public", "sent", "direct", "multi"}
			if !tc.filtered {
				want = append(want, "hidden")
			}
			if !slices.Equal(got, want) {
				t.Fatalf("got %v want %v", got, want)
			}
			// Filtering must also precede merging.
			job.MergeMessages = true
			messages, err = loadMessagesForExport(job)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range messages {
				if tc.filtered && strings.Contains(m.Content, "hidden") {
					t.Fatal("hidden whisper reached merge")
				}
			}
		})
	}
}

func TestChannelExportRequiresMemberChannelReadAccess(t *testing.T) {
	logExportFixture(t, nil)
	db := model.GetDB()
	if err := db.Create(&model.ChannelModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "export-restricted"},
		WorldID:           "export-world",
		UserID:            "export-owner",
		Name:              "restricted",
		Status:            model.ChannelStatusActive,
		PermType:          "non-public",
	}).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := CreateMessageExportJob(&ExportJobOptions{UserID: "export-member", ChannelID: "export-restricted", Format: "txt"}); !errors.Is(err, ErrExportPermission) {
		t.Fatalf("member exported unreadable channel: %v", err)
	}
	if _, err := CreateBatchMessageExportJob(&ExportJobOptions{UserID: "export-member", ChannelID: "export-a", Format: "txt"}, []string{"export-a", "export-restricted"}); !errors.Is(err, ErrExportPermission) {
		t.Fatalf("batch export accepted unreadable child: %v", err)
	}

	job, err := CreateMessageExportJob(&ExportJobOptions{UserID: "export-member", ChannelID: "export-a", Format: "txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.ChannelModel{}).Where("id = ?", "export-a").Update("perm_type", "non-public").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := RetryMessageExportJob(job); !errors.Is(err, ErrExportPermission) {
		t.Fatalf("retry bypassed current channel read access: %v", err)
	}
}

func seedExportVisibilityMessages(t *testing.T, channelID, viewer string) {
	t.Helper()
	for i, label := range []string{"public", "sent", "direct", "multi", "hidden"} {
		logExportMessage(t, channelID+"-"+label, channelID, time.Now(), float64(i+1), func(m *model.MessageModel) {
			m.Content, m.UserID = label, "export-target"
			m.SenderIdentityID = "identity-not-a-user"
			m.IsWhisper = label != "public"
			m.WhisperTo = "export-owner"
			if label == "sent" {
				m.UserID = viewer
			}
			if label == "direct" {
				m.WhisperTo = viewer
			}
			if label == "hidden" {
				m.WhisperTo = "unrelated-user"
			}
		})
	}
	if err := model.CreateWhisperRecipients(channelID+"-multi", []string{"unrelated-user", viewer}); err != nil {
		t.Fatal(err)
	}
}

func TestChannelExportBatchSnapshotAndRetry(t *testing.T) {
	logExportFixture(t, nil)
	db := model.GetDB()
	for _, row := range []any{
		&model.WorldModel{StringPKBaseModel: model.StringPKBaseModel{ID: "other-world"}, Name: "Other", InviteSlug: "other", Status: "active"},
		&model.WorldMemberModel{WorldID: "other-world", UserID: "export-member", Role: model.WorldRoleMember},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&model.ChannelModel{}).Where("id = ?", "export-b").Update("world_id", "other-world").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.WorldModel{}).Where("id = ?", "other-world").Update("export_respect_message_visibility", false).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"export-a", "export-b"} {
		seedExportVisibilityMessages(t, id, "export-member")
	}
	job, err := CreateBatchMessageExportJob(&ExportJobOptions{UserID: "export-member", ChannelID: "export-a", Format: "txt", IncludeOOC: true}, []string{"export-a", "export-b"})
	if err != nil {
		t.Fatal(err)
	}
	// Reverse the world settings. Retry must preserve both original scopes.
	for id, value := range map[string]bool{"export-world": false, "other-world": true} {
		if err := db.Model(&model.WorldModel{}).Where("id = ?", id).Update("export_respect_message_visibility", value).Error; err != nil {
			t.Fatal(err)
		}
	}
	retry, err := RetryMessageExportJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range []*model.MessageExportJobModel{job, retry} {
		extra := parseExportExtraOptions(j.ExtraOptions)
		if !extra.VisibilityByChannel["export-a"] || extra.VisibilityByChannel["export-b"] {
			t.Fatalf("wrong snapshot: %v", extra.VisibilityByChannel)
		}
		if err := processExportJob(j, MessageExportWorkerConfig{StorageDir: t.TempDir()}); err != nil {
			t.Fatal(err)
		}
		stored, err := GetMessageExportJob(j.ID)
		if err != nil {
			t.Fatal(err)
		}
		archive, err := zip.OpenReader(stored.FilePath)
		if err != nil {
			t.Fatal(err)
		}
		for i, f := range archive.File {
			r, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(r)
			r.Close()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "hidden") != (i == 1) {
				t.Fatalf("wrong child visibility %s: %s", f.Name, data)
			}
		}
		archive.Close()
	}
	if err := db.Model(&model.WorldModel{}).Where("id = ?", "other-world").Update("allow_member_export_messages", false).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := RetryMessageExportJob(job); !errors.Is(err, ErrMemberExportDisabled) {
		t.Fatalf("retry bypassed child authorization: %v", err)
	}
}

func TestChannelExportSingleRetrySnapshot(t *testing.T) {
	logExportFixture(t, nil)
	job, err := CreateMessageExportJob(&ExportJobOptions{UserID: "export-member", ChannelID: "export-a", Format: "txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := model.GetDB().Model(&model.WorldModel{}).Where("id = ?", "export-world").Update("export_respect_message_visibility", false).Error; err != nil {
		t.Fatal(err)
	}
	retry, err := RetryMessageExportJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !parseExportExtraOptions(retry.ExtraOptions).VisibilityByChannel["export-a"] {
		t.Fatal("retry broadened original scope")
	}
	legacy := *job
	legacy.ExtraOptions = ""
	retry, err = RetryMessageExportJob(&legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !parseExportExtraOptions(retry.ExtraOptions).VisibilityByChannel["export-a"] {
		t.Fatal("legacy retry gained unrestricted access")
	}
}

func TestWorldExportSettingsMigrationDefaults(t *testing.T) {
	// Start from an actual old table, then use the same AutoMigrate as DBInit.
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.Exec("CREATE TABLE worlds (id text PRIMARY KEY, name text NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO worlds (id, name) VALUES (?, ?)", "legacy", "Legacy").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.WorldModel{}); err != nil {
		t.Fatal(err)
	}
	var legacy model.WorldModel
	if err := db.First(&legacy, "id = ?", "legacy").Error; err != nil {
		t.Fatal(err)
	}
	if !legacy.AllowMemberExportMessages || !legacy.ExportRespectMessageVisibility {
		t.Fatalf("old world defaults: %+v", legacy)
	}
	fresh := &model.WorldModel{Name: "Fresh", InviteSlug: "fresh"}
	if err := db.Create(fresh).Error; err != nil {
		t.Fatal(err)
	}
	var loaded model.WorldModel
	if err := db.First(&loaded, "id = ?", fresh.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !loaded.AllowMemberExportMessages || !loaded.ExportRespectMessageVisibility {
		t.Fatal("new world defaults are not true")
	}
	if err := db.Model(&legacy).Updates(map[string]any{"allow_member_export_messages": false, "export_respect_message_visibility": false}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.WorldModel{}); err != nil {
		t.Fatal(err)
	}
	loaded = model.WorldModel{}
	if err := db.First(&loaded, "id = ?", legacy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if loaded.AllowMemberExportMessages || loaded.ExportRespectMessageVisibility {
		t.Fatal("migration reset explicit false")
	}
}

func TestWorldExportSettingsUpdate(t *testing.T) {
	logExportFixture(t, nil)
	off := false
	w, err := WorldUpdate("export-world", "export-owner", WorldUpdateParams{ExportRespectMessageVisibility: &off, AllowMemberExportMessages: &off})
	if err != nil {
		t.Fatal(err)
	}
	if w.ExportRespectMessageVisibility || w.AllowMemberExportMessages {
		t.Fatal("false settings not persisted")
	}
	w, err = WorldUpdate("export-world", "export-owner", WorldUpdateParams{Name: "Renamed"})
	if err != nil {
		t.Fatal(err)
	}
	if w.ExportRespectMessageVisibility || w.AllowMemberExportMessages {
		t.Fatal("omitted fields changed")
	}
	if _, err := WorldUpdate("export-world", "export-member", WorldUpdateParams{AllowMemberExportMessages: &off}); !errors.Is(err, ErrWorldPermission) {
		t.Fatalf("member changed settings: %v", err)
	}
}

// Keep LOG export regression coverage in this tracked file (new *_test.go files are ignored).
func logExportFixture(t *testing.T, sections []LogSection) *model.LogProjectModel {
	t.Helper()
	initTestDB(t)
	db := model.GetDB()
	for _, id := range []string{"export-owner", "export-member", "export-target"} {
		createExportTestUser(t, id, id, id)
	}
	rows := []any{
		&model.WorldModel{StringPKBaseModel: model.StringPKBaseModel{ID: "export-world"}, Name: "Export", OwnerID: "export-owner", Status: "active", AllowMemberAccessLogLayout: true},
		&model.WorldMemberModel{WorldID: "export-world", UserID: "export-owner", Role: model.WorldRoleOwner},
		&model.WorldMemberModel{WorldID: "export-world", UserID: "export-member", Role: model.WorldRoleMember},
	}
	for _, id := range []string{"export-a", "export-b", "export-c"} {
		rows = append(rows, &model.ChannelModel{StringPKBaseModel: model.StringPKBaseModel{ID: id}, WorldID: "export-world", UserID: "export-owner", Name: id, Status: model.ChannelStatusActive, PermType: "public"})
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	var draft LogDraft
	if err := json.Unmarshal([]byte(defaultLogDraft("Export narrative")), &draft); err != nil {
		t.Fatal(err)
	}
	draft.Sections = sections
	raw, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	p := &model.LogProjectModel{WorldID: "export-world", Enabled: true, DraftVersion: 7, DraftJSON: string(raw)}
	if err := db.Create(p).Error; err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLogDraftContentOverrideAndUnknownField(t *testing.T) {
	d, err := parseLogDraft([]byte(defaultLogDraft("校对")))
	if err != nil {
		t.Fatal(err)
	}
	d.MessageContentOverrides = map[string]string{"msg1": "你好，世界"}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseLogDraft(raw)
	if err != nil || parsed.MessageContentOverrides["msg1"] != "你好，世界" {
		t.Fatalf("content override round trip: %v", err)
	}
	// 旧工程的 replay 字段需要往返保留，不可因为一次校对保存而丢失。
	d.Replay = json.RawMessage(`{"enabled":true,"events":[{"at":1200,"action":"scene"}],"extra":{"future":1}}`)
	raw, err = json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = parseLogDraft(raw)
	if err != nil || string(parsed.Replay) != string(d.Replay) {
		t.Fatalf("replay metadata lost: %v, got %s", err, parsed.Replay)
	}
	roundtrip, err := json.Marshal(parsed)
	if err != nil {
		t.Fatal(err)
	}
	var after LogDraft
	if err := json.Unmarshal(roundtrip, &after); err != nil || string(after.Replay) != string(d.Replay) {
		t.Fatalf("replay metadata round trip failed: %v", err)
	}
	var unknown map[string]any
	if err := json.Unmarshal(raw, &unknown); err != nil {
		t.Fatal(err)
	}
	unknown["unexpectedField"] = true
	invalid, _ := json.Marshal(unknown)
	if _, err := parseLogDraft(invalid); err == nil || !strings.Contains(err.Error(), "unexpectedField") {
		t.Fatalf("unknown draft field must produce actionable error, got %v", err)
	}
}

func TestLogDisplayRulesDraftValidation(t *testing.T) {
	d, err := parseLogDraft([]byte(defaultLogDraft("legacy")))
	if err != nil || d.DisplayRules != nil || d.MessageTemplateOverrides != nil {
		t.Fatalf("legacy draft: %v", err)
	}
	rule := LogDisplayRule{ID: "00000000-0000-4000-8000-000000000001", Match: LogDisplayMatch{MessageType: "ic", IdentityID: "role", UserID: "user", VariantID: "variant"}, TemplateID: "missing-template"}
	d.DisplayRules, d.MessageTemplateOverrides = []LogDisplayRule{rule}, map[string]string{"message": "missing-template"}
	raw, _ := json.Marshal(d)
	parsed, err := parseLogDraft(raw)
	if err != nil || len(parsed.DisplayRules) != 1 || parsed.DisplayRules[0] != rule || parsed.MessageTemplateOverrides["message"] != "missing-template" {
		t.Fatalf("optional fields round trip: %+v %v", parsed, err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*LogDraft)
	}{
		{"empty match", func(d *LogDraft) { d.DisplayRules[0].Match = LogDisplayMatch{} }},
		{"bad type", func(d *LogDraft) { d.DisplayRules[0].Match.MessageType = "ic|ooc" }},
		{"bad rule ID", func(d *LogDraft) { d.DisplayRules[0].ID = "bad" }},
		{"duplicate ID", func(d *LogDraft) { d.DisplayRules = append(d.DisplayRules, d.DisplayRules[0]) }},
		{"bad template", func(d *LogDraft) { d.DisplayRules[0].TemplateID = "<script>" }},
		{"bad user ID", func(d *LogDraft) { d.DisplayRules[0].Match.UserID = strings.Repeat("x", 101) }},
		{"empty override", func(d *LogDraft) { d.MessageTemplateOverrides["message"] = "" }},
		{"empty message ID", func(d *LogDraft) { d.MessageTemplateOverrides[""] = "message" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var next LogDraft
			if err := json.Unmarshal(raw, &next); err != nil {
				t.Fatal(err)
			}
			tc.mutate(&next)
			encoded, _ := json.Marshal(next)
			if _, err := parseLogDraft(encoded); err == nil {
				t.Fatal("accepted invalid display configuration")
			}
		})
	}
	projected := logPublicDisplayRules("r1", d.DisplayRules)
	if d.DisplayRules[0] != rule || projected[0].Match.UserID == rule.Match.UserID || projected[0].Match.UserID == logPublicKey("r1", "identity", "user") || projected[0].Match.UserID == logPublicDisplayRules("r2", d.DisplayRules)[0].Match.UserID {
		t.Fatal("public rule mapping violates domain/release privacy or mutated the draft")
	}
}

func TestLogDisplayRulesReleasePrivacy(t *testing.T) {
	section := logExportSection("00000000-0000-4000-8000-000000000001", "export-a", 0)
	p := logExportFixture(t, []LogSection{section})
	if err := RegisterBuiltinLogStyles(); err != nil {
		t.Fatal(err)
	}
	var d LogDraft
	if err := json.Unmarshal([]byte(p.DraftJSON), &d); err != nil {
		t.Fatal(err)
	}
	d.DisplayRules = []LogDisplayRule{
		{ID: "00000000-0000-4000-8000-000000000011", Match: LogDisplayMatch{MessageType: "ic", UserID: "export-owner"}, TemplateID: "message"},
		{ID: "00000000-0000-4000-8000-000000000012", Match: LogDisplayMatch{IdentityID: "private-role", VariantID: "private-variant"}, TemplateID: "missing-template"},
	}
	d.MessageTemplateOverrides = map[string]string{"private-message-a": "message", "private-message-b": "missing-template"}
	b, _ := json.Marshal(d)
	saved, err := SaveLogDraft(p.ID, "export-owner", LogDraftUpdate{ExpectedVersion: p.DraftVersion, Draft: b})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Draft.DisplayRules) != 2 || saved.Draft.MessageTemplateOverrides["private-message-b"] != "missing-template" {
		t.Fatal("save lost display configuration")
	}
	for i, id := range []string{"private-message-a", "private-message-b"} {
		logExportMessage(t, id, "export-a", time.Now().Add(time.Duration(i)*time.Second), float64(i+1), func(m *model.MessageModel) {
			m.Content = "public body"
			m.SenderIdentityID, m.SenderIdentityVariantID = "private-role", "private-variant"
		})
	}
	job, err := QueueLogRelease(p.ID, "export-owner", LogReleaseInput{ExpectedVersion: saved.DraftVersion, ClientOpID: "display-rules"})
	if err != nil {
		t.Fatal(err)
	}
	if err := runLogReleaseJob(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, err := GetLogJob(p.ID, job["jobId"].(string), "export-owner")
	if err != nil || state["status"] != "done" {
		t.Fatalf("release: %v %v", state, err)
	}
	releaseID := state["releaseId"].(string)
	const slug = "displayrules123"
	if err := model.GetDB().Create(&model.LogRouteAliasModel{ProjectID: p.ID, Kind: "view", Slug: slug, Active: true}).Error; err != nil {
		t.Fatal(err)
	}
	metadata, err := GetPublicLog(slug)
	if err != nil {
		t.Fatal(err)
	}
	page, err := QueryPublicLog(slug, section.ID, "", "", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	items := page["items"].([]map[string]any)
	if len(items) != 2 || items[0]["identityKey"] != logPublicKey(releaseID, "identity", "private-role") || items[0]["userKey"] != logPublicKey(releaseID, "user", "export-owner") || items[0]["templateOverride"] != "message" || items[1]["templateOverride"] != "missing-template" {
		t.Fatalf("public node projection lost independent keys/overrides: %v", items)
	}
	publicJSON, _ := json.Marshal(metadata)
	var projection struct {
		DisplayRules []LogDisplayRule `json:"displayRules"`
	}
	if err := json.Unmarshal(publicJSON, &projection); err != nil {
		t.Fatal(err)
	}
	if len(projection.DisplayRules) != 2 || projection.DisplayRules[0].Match.UserID != items[0]["userKey"] || projection.DisplayRules[1].Match.IdentityID != items[0]["identityKey"] || projection.DisplayRules[1].Match.VariantID != items[0]["speaker"].(map[string]any)["variantKey"] {
		t.Fatalf("rule/node mapping differs: %s", publicJSON)
	}
	nodeJSON, _ := json.Marshal(items)
	for _, private := range []string{"export-owner", "private-role", "private-variant", "private-message", "messageTemplateOverrides", "00000000-0000-4000-8000-000000000011"} {
		if strings.Contains(string(nodeJSON), private) || strings.Contains(string(publicJSON), private) {
			t.Fatalf("public payload leaked %q", private)
		}
	}
	if strings.Contains(string(nodeJSON), "userId") {
		t.Fatal("public message contains raw userId field")
	}
}

func logExportSection(id, channel string, order int) LogSection {
	return LogSection{ID: id, Order: order, Source: LogSourceSpec{ChannelID: channel, ToTime: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), IdentityIDs: []string{}, IncludeOOC: true, IncludeArchived: true}}
}

func logExportMessage(t *testing.T, id, channel string, created time.Time, order float64, mutate func(*model.MessageModel)) {
	t.Helper()
	m := &model.MessageModel{StringPKBaseModel: model.StringPKBaseModel{ID: id, CreatedAt: created}, ChannelID: channel, UserID: "export-owner", SenderIdentityID: "export-role", SenderIdentityName: "Narrator", SenderIdentityColor: "#123abc", Content: id, ICMode: "ic", DisplayOrder: order}
	if mutate != nil {
		mutate(m)
	}
	if err := model.GetDB().Create(m).Error; err != nil {
		t.Fatal(err)
	}
}

func logExportJob(t *testing.T, p *model.LogProjectModel, merge bool) *model.MessageExportJobModel {
	t.Helper()
	job, err := CreateLogMessageExportJob(p.ID, "export-owner", LogExportInput{ExpectedDraftVersion: p.DraftVersion, Format: "txt", MergeMessages: &merge})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func assertLogExportIDs(t *testing.T, job *model.MessageExportJobModel, want ...string) []*model.MessageModel {
	t.Helper()
	messages, err := loadLogMessagesForExport(job, parseExportExtraOptions(job.ExtraOptions))
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(messages))
	for i, m := range messages {
		got[i] = m.ID
	}
	if !slices.Equal(got, want) {
		t.Fatalf("section sequence: got %v want %v", got, want)
	}
	return messages
}

func TestLoadLogMessagesForExportRespectsSectionOrder(t *testing.T) {
	a, b := logExportSection("a", "export-a", 1), logExportSection("b", "export-b", 0)
	p := logExportFixture(t, []LogSection{a, b}) // Deliberately unsorted JSON.
	base := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	logExportMessage(t, "A1", "export-a", base, 1, nil)
	logExportMessage(t, "A2", "export-a", base.Add(2*time.Minute), 2, nil)
	logExportMessage(t, "B1", "export-b", base.Add(4*time.Hour), 1, nil)
	logExportMessage(t, "B2", "export-b", base.Add(4*time.Hour+time.Minute), 2, nil)
	assertLogExportIDs(t, logExportJob(t, p, false), "B1", "B2", "A1", "A2")
}

func TestLoadLogMessagesForExportAllowsSameChannelMultipleSections(t *testing.T) {
	base := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	a, b := logExportSection("late", "export-a", 0), logExportSection("early", "export-a", 1)
	from := base.Add(2 * time.Minute)
	a.Source.FromTime = &from
	b.Source.ToTime = base.Add(time.Minute)
	p := logExportFixture(t, []LogSection{a, b})
	for i := 0; i < 4; i++ {
		logExportMessage(t, fmt.Sprintf("m%d", i+1), "export-a", base.Add(time.Duration(i)*time.Minute), float64(i+1), nil)
	}
	assertLogExportIDs(t, logExportJob(t, p, false), "m3", "m4", "m1", "m2")
}

func TestLoadLogMessagesForExportUsesDisplayOrderAndStableTies(t *testing.T) {
	p := logExportFixture(t, []LogSection{logExportSection("a", "export-a", 0)})
	base := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	logExportMessage(t, "created-first", "export-a", base, 2, nil)
	logExportMessage(t, "tie-b", "export-a", base.Add(time.Hour), 1, nil)
	logExportMessage(t, "tie-a", "export-a", base.Add(time.Hour), 1, nil)
	assertLogExportIDs(t, logExportJob(t, p, false), "tie-a", "tie-b", "created-first")
}

func TestLoadLogMessagesForExportSourceFilters(t *testing.T) {
	base := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name      string
		configure func(*LogSection)
		want      []string
	}{
		{"inclusive", func(s *LogSection) { s.Source.FromTime = &base; s.Source.ToTime = base }, []string{"boundary"}},
		{"exclusive", func(s *LogSection) {
			s.Source.FromTime = &base
			s.Source.FromExclusive = true
			s.Source.ToTime = base.Add(time.Second)
		}, []string{"after"}},
		{"ooc", func(s *LogSection) { s.Source.IncludeOOC = false }, []string{"before", "boundary", "after", "archived", "other-identity", "own-whisper"}},
		{"archive", func(s *LogSection) { s.Source.IncludeArchived = false }, []string{"before", "boundary", "after", "ooc", "other-identity", "own-whisper"}},
		{"identity", func(s *LogSection) { s.Source.IdentityIDs = []string{"another-role"} }, []string{"other-identity"}},
		{"all", func(s *LogSection) {}, []string{"before", "boundary", "after", "ooc", "archived", "other-identity", "own-whisper"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := logExportSection("source", "export-a", 0)
			tc.configure(&s)
			p := logExportFixture(t, []LogSection{s})
			for i, id := range []string{"before", "boundary", "after", "ooc", "archived", "other-identity", "excluded", "own-whisper", "hidden-whisper", "deleted", "revoked"} {
				logExportMessage(t, id, "export-a", base.Add(time.Duration(i-1)*time.Second), float64(i+1), func(m *model.MessageModel) {
					switch id {
					case "ooc":
						m.ICMode = "ooc"
					case "archived":
						m.IsArchived = true
					case "other-identity":
						m.SenderIdentityID = "another-role"
					case "own-whisper":
						m.IsWhisper = true
						m.WhisperTo = "export-target"
					case "hidden-whisper":
						m.IsWhisper = true
						m.UserID = "export-target"
						m.WhisperTo = "export-member"
					case "deleted":
						m.IsDeleted = true
					case "revoked":
						m.IsRevoked = true
					}
				})
			}
			if err := model.GetDB().Create(&model.LogMessageExclusionModel{ProjectID: p.ID, SectionID: s.ID, MessageID: "excluded"}).Error; err != nil {
				t.Fatal(err)
			}
			job := logExportJob(t, p, false)
			// These job-level flags must never remove messages selected by a LOG source.
			job.IncludeOOC, job.IncludeArchived = false, false
			messages := assertLogExportIDs(t, job, tc.want...)
			for _, m := range messages {
				if m.ID == "own-whisper" && (m.User == nil || len(m.WhisperTargets) != 1 || m.WhisperTargets[0].ID != "export-target") {
					t.Fatalf("missing exporter associations: %+v", m)
				}
			}
		})
	}
}

func TestLogExportDoesNotMergeAcrossSections(t *testing.T) {
	base := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	a, b := logExportSection("a", "export-a", 0), logExportSection("b", "export-a", 1)
	a.Source.ToTime = base.Add(time.Second)
	from := a.Source.ToTime
	b.Source.FromTime, b.Source.FromExclusive = &from, true
	p := logExportFixture(t, []LogSection{a, b})
	for i, id := range []string{"first", "second", "third", "fourth"} {
		logExportMessage(t, id, "export-a", base.Add(time.Duration(i)*time.Second), float64(i+1), nil)
	}
	messages := assertLogExportIDs(t, logExportJob(t, p, true), "first", "third")
	if messages[0].Content != "first\nsecond" || messages[1].Content != "third\nfourth" || messages[0].MergedMessages != 2 || messages[1].MergedMessages != 2 {
		t.Fatalf("section-local merge failed: %+v", messages)
	}
}

func TestLogExportAppliesTraditionalOutputOptions(t *testing.T) {
	p := logExportFixture(t, []LogSection{logExportSection("a", "export-a", 0)})
	base := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	logExportMessage(t, "dice", "export-a", base, 1, func(m *model.MessageModel) { m.Content = ".r1d6" })
	logExportMessage(t, "image", "export-a", base.Add(time.Second), 2, func(m *model.MessageModel) { m.Content = "[CQ:image,file=picture.png]" })
	logExportMessage(t, "text", "export-a", base.Add(2*time.Second), 3, func(m *model.MessageModel) { m.Content = "你好,世界!" })
	job := logExportJob(t, p, true)
	messages := assertLogExportIDs(t, job, "text")
	if messages[0].Content != "你好，世界！" {
		t.Fatalf("punctuation correction not applied: %q", messages[0].Content)
	}
	var original model.MessageModel
	if err := model.GetDB().Where("id = ?", "text").Take(&original).Error; err != nil {
		t.Fatal(err)
	}
	if original.Content != "你好,世界!" {
		t.Fatal("export changed the original message")
	}
	for _, enabled := range []bool{false, true} {
		raw, err := buildExportExtraOptions(&ExportJobOptions{SourceKind: "log", LogProjectID: p.ID, LogDraftVersion: p.DraftVersion, IncludeImages: true, IncludeDiceCommand: true, AutoCorrectPunctuation: enabled})
		if err != nil {
			t.Fatal(err)
		}
		job.ExtraOptions = raw
		extra := parseExportExtraOptions(raw)
		messages, err := loadLogMessagesForExport(job, extra)
		if err != nil || len(messages) != 1 || !strings.Contains(messages[0].Content, ".r1d6") || !strings.Contains(messages[0].Content, "[CQ:image") {
			t.Fatalf("include options not applied: %+v %v", messages, err)
		}
		want := "你好,世界!"
		if enabled {
			want = "你好，世界！"
		}
		if !strings.Contains(messages[0].Content, want) {
			t.Fatalf("punctuation option: %q", messages[0].Content)
		}
	}
}

func TestChannelExportOmitsLogSourceMetadata(t *testing.T) {
	raw, err := buildExportExtraOptions(&ExportJobOptions{IncludeImages: true, IncludeDiceCommand: true, AutoCorrectPunctuation: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"source_kind", "log_project_id", "log_draft_version"} {
		if strings.Contains(raw, field) {
			t.Fatalf("channel job wrote %s: %s", field, raw)
		}
	}
	legacy := parseExportExtraOptions(`{"include_images":false,"include_dice_commands":true}`)
	if legacy.SourceKind != "" || legacy.LogProjectID != "" || legacy.LogDraftVersion != 0 || legacy.IncludeImages || !legacy.IncludeDiceCommand || !legacy.AutoCorrectPunctuation {
		t.Fatalf("legacy options changed: %+v", legacy)
	}
}

func TestLogExportRejectsStaleDraftVersion(t *testing.T) {
	p := logExportFixture(t, []LogSection{logExportSection("a", "export-a", 0)})
	_, err := CreateLogMessageExportJob(p.ID, "export-owner", LogExportInput{ExpectedDraftVersion: p.DraftVersion - 1, Format: "txt"})
	logAssertExportCode(t, err, "VERSION_CONFLICT")
	job := logExportJob(t, p, false)
	if err := model.GetDB().Model(p).UpdateColumn("draft_version", p.DraftVersion+1).Error; err != nil {
		t.Fatal(err)
	}
	if err := processExportJob(job, MessageExportWorkerConfig{StorageDir: t.TempDir()}); err == nil {
		t.Fatal("worker exported a newer draft")
	}
	stored, err := GetMessageExportJob(job.ID)
	if err != nil || stored.Status != model.MessageExportStatusFailed || !strings.Contains(stored.ErrorMsg, "LOG 排版已变化，请重新导出") || stored.FilePath != "" {
		t.Fatalf("stale job: %+v %v", stored, err)
	}
}

func logAssertExportCode(t *testing.T, err error, code string) {
	t.Helper()
	var domain *LogError
	if !errors.As(err, &domain) || domain.Code != code {
		t.Fatalf("got %v want %s", err, code)
	}
}

func TestRetryMessageExportJobPreservesLogSource(t *testing.T) {
	p := logExportFixture(t, []LogSection{logExportSection("a", "export-a", 0)})
	job := logExportJob(t, p, true)
	retried, err := RetryMessageExportJob(job)
	if err != nil {
		t.Fatal(err)
	}
	extra := parseExportExtraOptions(retried.ExtraOptions)
	if extra.SourceKind != "log" || extra.LogProjectID != p.ID || extra.LogDraftVersion != p.DraftVersion || retried.ChannelID != "" || retried.ID == job.ID {
		t.Fatalf("lost LOG source on retry: %+v %+v", retried, extra)
	}
}

func TestLogExportWorkerRechecksSourcePermission(t *testing.T) {
	p := logExportFixture(t, []LogSection{logExportSection("a", "export-a", 0)})
	job := logExportJob(t, p, false)
	if err := model.GetDB().Model(&model.ChannelModel{}).Where("id = ?", "export-a").Update("is_private", true).Error; err != nil {
		t.Fatal(err)
	}
	_, err := loadLogMessagesForExport(job, parseExportExtraOptions(job.ExtraOptions))
	logAssertExportCode(t, err, "PERMISSION_DENIED")
}

func TestLogExportGlobalMessageLimit(t *testing.T) {
	sections := make([]LogSection, 200)
	for i := range sections {
		sections[i] = logExportSection(fmt.Sprintf("s%d", i), "export-a", i)
	}
	p := logExportFixture(t, sections)
	base := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	for i := 0; i < 330; i++ {
		logExportMessage(t, fmt.Sprintf("m%d", i), "export-a", base, float64(i+1), nil)
	}
	job := logExportJob(t, p, false)
	messages, err := loadLogMessagesForExport(job, parseExportExtraOptions(job.ExtraOptions))
	if err != nil || len(messages) != messageExportLimit || messages[len(messages)-1].ID != fmt.Sprintf("m%d", (messageExportLimit-1)%330) {
		t.Fatalf("global limit: len=%d err=%v", len(messages), err)
	}
}

func TestLogExportWorkerReusesTraditionalFormats(t *testing.T) {
	for _, format := range []string{"txt", "bbcode-txt", "docx", "json", "html"} {
		t.Run(format, func(t *testing.T) {
			p := logExportFixture(t, []LogSection{logExportSection("b", "export-b", 0), logExportSection("a", "export-a", 1), logExportSection("c", "export-c", 2)})
			base := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
			for i, channel := range []string{"export-a", "export-c", "export-b"} {
				logExportMessage(t, "content-"+channel, channel, base.Add(time.Duration(i)*time.Hour), 1, nil)
			}
			input := LogExportInput{ExpectedDraftVersion: p.DraftVersion, Format: format, TextBBCodeColorize: format == "bbcode-txt"}
			if format == "bbcode-txt" {
				input.Format = "txt"
			}
			job, err := CreateLogMessageExportJob(p.ID, "export-owner", input)
			if err != nil {
				t.Fatal(err)
			}
			if err := processExportJob(job, MessageExportWorkerConfig{StorageDir: t.TempDir()}); err != nil {
				t.Fatal(err)
			}
			stored, err := GetMessageExportJob(job.ID)
			if err != nil || stored.Status != model.MessageExportStatusDone || stored.ChannelID != "" {
				t.Fatalf("job: %+v %v", stored, err)
			}
			data, err := os.ReadFile(stored.FilePath)
			if err != nil {
				t.Fatal(err)
			}
			if format == "docx" || format == "html" {
				archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
				if err != nil {
					t.Fatal(err)
				}
				data = nil
				for _, f := range archive.File {
					want := format == "docx" && f.Name == "word/document.xml"
					want = want || (format == "html" && strings.HasPrefix(f.Name, "parts/part-") && strings.HasSuffix(f.Name, ".html"))
					if !want {
						continue
					}
					r, err := f.Open()
					if err != nil {
						t.Fatal(err)
					}
					part, err := io.ReadAll(r)
					r.Close()
					if err != nil {
						t.Fatal(err)
					}
					data = append(data, part...)
				}
			}
			body := string(data)
			b, a, c := strings.Index(body, "content-export-b"), strings.Index(body, "content-export-a"), strings.Index(body, "content-export-c")
			if b < 0 || a <= b || c <= a {
				t.Fatalf("formatter reordered sections: %s", body)
			}
			if format == "bbcode-txt" && !strings.Contains(body, "[color=#123abc]") {
				t.Fatalf("missing snapshot BBCode color: %s", body)
			}
			if format == "docx" && !strings.Contains(strings.ToLower(body), "123abc") {
				t.Fatalf("missing snapshot DOCX color: %s", body)
			}
		})
	}
}

func TestLogExportDocxUsesPerMessageChannelImageLayout(t *testing.T) {
	initTestDB(t)
	sourcePayload := buildExportPayload(
		&model.MessageExportJobModel{UserID: "export-owner", Format: "docx"},
		"",
		[]*model.MessageModel{{StringPKBaseModel: model.StringPKBaseModel{ID: "source"}, ChannelID: "export-a", Content: "source"}},
		nil,
		&exportExtraOptions{IncludeImages: true, IncludeDiceCommand: true},
	)
	if len(sourcePayload.Messages) != 1 || sourcePayload.Messages[0].ChannelID != "export-a" {
		t.Fatalf("message channel was not carried into export payload: %+v", sourcePayload.Messages)
	}
	const attachmentID = "log-export-docx-layout"
	createWorldClueExportImageAttachment(t, attachmentID, "export-owner", "export-world")
	for _, tc := range []struct {
		channel string
		width   int
		height  int
	}{
		{channel: "export-a", width: 120, height: 80},
		{channel: "export-b", width: 240, height: 160},
	} {
		if err := model.ChannelAttachmentImageLayoutUpsertBatch(tc.channel, "export-owner", []model.ChannelAttachmentImageLayoutUpsertItem{{AttachmentID: attachmentID, Width: tc.width, Height: tc.height}}); err != nil {
			t.Fatal(err)
		}
	}
	content := &AgentRichDocument{Type: "document", Blocks: []AgentRichNode{{Type: "paragraph", Children: []AgentRichNode{{Type: "image", Attrs: map[string]string{"attachment_id": attachmentID}}}}}}
	payload := &ExportPayload{IncludeImages: true, Messages: []ExportMessage{
		{ID: "a", ChannelID: "export-a", SenderName: "A", ContentRich: content},
		{ID: "b", ChannelID: "export-b", SenderName: "B", ContentRich: content},
	}}
	data, err := (docxFormatter{}).Build(payload)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var document []byte
	for _, f := range archive.File {
		if f.Name != "word/document.xml" {
			continue
		}
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		document, err = io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	matches := regexp.MustCompile(`<wp:extent cx="([0-9]+)" cy="([0-9]+)"`).FindAllStringSubmatch(string(document), -1)
	if len(matches) < 2 {
		t.Fatalf("expected two image extents, got %d: %s", len(matches), document)
	}
	firstX, _ := strconv.ParseInt(matches[0][1], 10, 64)
	firstY, _ := strconv.ParseInt(matches[0][2], 10, 64)
	secondX, _ := strconv.ParseInt(matches[1][1], 10, 64)
	secondY, _ := strconv.ParseInt(matches[1][2], 10, 64)
	if secondX != firstX*2 || secondY != firstY*2 {
		t.Fatalf("channel image layouts were not preserved: first=%dx%d second=%dx%d", firstX, firstY, secondX, secondY)
	}
}

func TestLoadMessagesForExportHydratesMultiWhisperTargets(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	channelID := "ch-export-whisper-" + suffix
	if err := db.Create(&model.ChannelModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: channelID},
		Name:              "Export Whisper Channel",
		PermType:          "public",
		Status:            model.ChannelStatusActive,
	}).Error; err != nil {
		t.Fatalf("create channel failed: %v", err)
	}

	sender := createExportTestUser(t, "u-export-sender-"+suffix, "export_sender_"+suffix, "发送者")
	target1 := createExportTestUser(t, "u-export-target1-"+suffix, "export_target1_"+suffix, "目标甲")
	target2 := createExportTestUser(t, "u-export-target2-"+suffix, "export_target2_"+suffix, "目标乙")

	msgID := "msg-export-whisper-" + suffix
	now := time.Now()
	if err := db.Create(&model.MessageModel{
		StringPKBaseModel: model.StringPKBaseModel{
			ID:        msgID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		ChannelID:    channelID,
		UserID:       sender.ID,
		Content:      "多人悄悄话测试",
		DisplayOrder: float64(now.UnixMilli()),
		ICMode:       "ic",
		IsWhisper:    true,
		WhisperTo:    target1.ID,
	}).Error; err != nil {
		t.Fatalf("create whisper message failed: %v", err)
	}

	if err := model.CreateWhisperRecipients(msgID, []string{target2.ID, target1.ID}); err != nil {
		t.Fatalf("create whisper recipients failed: %v", err)
	}

	job := &model.MessageExportJobModel{
		ChannelID:       channelID,
		IncludeOOC:      true,
		IncludeArchived: true,
		MergeMessages:   false,
	}
	messages, err := loadMessagesForExport(job)
	if err != nil {
		t.Fatalf("loadMessagesForExport failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	gotIDs := make([]string, 0, len(messages[0].WhisperTargets))
	for _, target := range messages[0].WhisperTargets {
		if target == nil || target.ID == "" {
			continue
		}
		gotIDs = append(gotIDs, target.ID)
	}
	slices.Sort(gotIDs)
	wantIDs := []string{target1.ID, target2.ID}
	slices.Sort(wantIDs)
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("whisper targets mismatch, got=%v want=%v", gotIDs, wantIDs)
	}

	payload := buildExportPayload(job, "Export Whisper Channel", messages, nil, nil)
	if payload == nil || len(payload.Messages) != 1 {
		t.Fatalf("unexpected payload messages: %+v", payload)
	}
	targetNames := payload.Messages[0].WhisperTargets
	if len(targetNames) < 2 {
		t.Fatalf("expected at least 2 whisper target names, got %v", targetNames)
	}
	if !slices.Contains(targetNames, target1.Nickname) {
		t.Fatalf("payload missing target1 nickname %q, got %v", target1.Nickname, targetNames)
	}
	if !slices.Contains(targetNames, target2.Nickname) {
		t.Fatalf("payload missing target2 nickname %q, got %v", target2.Nickname, targetNames)
	}
}

func TestExportColorizeMapsSurviveExtraOptionRoundTrip(t *testing.T) {
	raw, err := buildExportExtraOptions(&ExportJobOptions{
		TextColorizeBBCode:        true,
		TextColorizeBBCodeMap:     map[string]string{"identity:role-a": "#123456"},
		TextColorizeBBCodeNameMap: map[string]string{"identity:role-a": "别名"},
	})
	if err != nil {
		t.Fatal(err)
	}
	extra := parseExportExtraOptions(raw)
	if extra == nil || !extra.TextColorizeBBCode || extra.TextColorizeBBCodeMap["identity:role-a"] != "#123456" || extra.TextColorizeBBCodeNameMap["identity:role-a"] != "别名" {
		t.Fatalf("colorize maps were not preserved: %#v", extra)
	}
}

func TestBuildExportPayloadUsesMappedWhisperTargetDisplayNames(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	channelID := "ch-export-whisper-mapped-" + suffix
	if err := db.Create(&model.ChannelModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: channelID},
		Name:              "Export Whisper Mapped Channel",
		PermType:          "public",
		Status:            model.ChannelStatusActive,
	}).Error; err != nil {
		t.Fatalf("create channel failed: %v", err)
	}

	sender := createExportTestUser(t, "u-export-map-sender-"+suffix, "export_map_sender_"+suffix, "发送者")
	target1 := createExportTestUser(t, "u-export-map-target1-"+suffix, "export_map_target1_"+suffix, "目标甲")
	target2 := createExportTestUser(t, "u-export-map-target2-"+suffix, "export_map_target2_"+suffix, "目标乙")

	target1Ic := &model.ChannelIdentityModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "ci-target1-ic-" + suffix},
		ChannelID:         channelID,
		UserID:            target1.ID,
		DisplayName:       "甲的场内角色",
	}
	target2Ic := &model.ChannelIdentityModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "ci-target2-ic-" + suffix},
		ChannelID:         channelID,
		UserID:            target2.ID,
		DisplayName:       "乙的场内角色",
	}
	if err := db.Create(target1Ic).Error; err != nil {
		t.Fatalf("create target1 identity failed: %v", err)
	}
	if err := db.Create(target2Ic).Error; err != nil {
		t.Fatalf("create target2 identity failed: %v", err)
	}
	if _, err := model.ChannelIdentityModeConfigUpsert(target1.ID, channelID, target1Ic.ID, ""); err != nil {
		t.Fatalf("upsert target1 ic/ooc config failed: %v", err)
	}
	if _, err := model.ChannelIdentityModeConfigUpsert(target2.ID, channelID, target2Ic.ID, ""); err != nil {
		t.Fatalf("upsert target2 ic/ooc config failed: %v", err)
	}

	msgID := "msg-export-whisper-mapped-" + suffix
	now := time.Now()
	if err := db.Create(&model.MessageModel{
		StringPKBaseModel: model.StringPKBaseModel{
			ID:        msgID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		ChannelID:    channelID,
		UserID:       sender.ID,
		Content:      "多人悄悄话映射测试",
		DisplayOrder: float64(now.UnixMilli()),
		ICMode:       "ic",
		IsWhisper:    true,
		WhisperTo:    target1.ID,
	}).Error; err != nil {
		t.Fatalf("create whisper message failed: %v", err)
	}
	if err := model.CreateWhisperRecipients(msgID, []string{target1.ID, target2.ID}); err != nil {
		t.Fatalf("create whisper recipients failed: %v", err)
	}

	job := &model.MessageExportJobModel{
		ChannelID:       channelID,
		IncludeOOC:      true,
		IncludeArchived: true,
		MergeMessages:   false,
	}
	messages, err := loadMessagesForExport(job)
	if err != nil {
		t.Fatalf("loadMessagesForExport failed: %v", err)
	}
	payload := buildExportPayload(job, "Export Whisper Mapped Channel", messages, nil, nil)
	if payload == nil || len(payload.Messages) != 1 {
		t.Fatalf("unexpected payload messages: %+v", payload)
	}
	targetNames := payload.Messages[0].WhisperTargets
	if !slices.Equal(targetNames, []string{"甲的场内角色", "乙的场内角色"}) {
		t.Fatalf("expected mapped whisper target names, got %v", targetNames)
	}
}

func TestParseExportExtraOptionsDefaultsIncludeFlags(t *testing.T) {
	extra := parseExportExtraOptions("")
	if extra == nil {
		t.Fatalf("extra options should not be nil")
	}
	if !extra.IncludeImages {
		t.Fatalf("default include_images should be true")
	}
	if !extra.IncludeDiceCommand {
		t.Fatalf("default include_dice_commands should be true")
	}
}

func TestBuildAndParseExportExtraOptionsPreserveBBCodeColorMap(t *testing.T) {
	raw, err := buildExportExtraOptions(&ExportJobOptions{
		IncludeImages:             true,
		IncludeDiceCommand:        true,
		TextColorizeBBCode:        true,
		TextColorizeBBCodeMap:     map[string]string{"identity:role-a": "#123abc"},
		TextColorizeBBCodeNameMap: map[string]string{"identity:role-a": "阿尔法"},
		SliceLimit:                5000,
		MaxConcurrency:            2,
	})
	if err != nil {
		t.Fatalf("buildExportExtraOptions failed: %v", err)
	}
	extra := parseExportExtraOptions(raw)
	if extra == nil {
		t.Fatalf("extra should not be nil")
	}
	if !extra.TextColorizeBBCode {
		t.Fatalf("text colorize flag should be true")
	}
	if got := extra.TextColorizeBBCodeMap["identity:role-a"]; got != "#123abc" {
		t.Fatalf("unexpected color map value: %q", got)
	}
	if got := extra.TextColorizeBBCodeNameMap["identity:role-a"]; got != "阿尔法" {
		t.Fatalf("unexpected name map value: %q", got)
	}
}

func TestBuildExportResultFileNameUsesDisplayNameAndTaskID(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, time.March, 7, 9, 8, 7, 0, time.Local)
	got := BuildExportResultFileName("  三月导出.txt ", "task-123", "txt", ts)
	want := "三月导出-task-123-20260307-090807.txt"
	if got != want {
		t.Fatalf("unexpected file name, got=%q want=%q", got, want)
	}
}

func TestBuildExportResultFileNameFallsBackToDefaultBaseName(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, time.March, 7, 18, 30, 45, 0, time.Local)
	got := BuildExportResultFileName("", "task-456", "html", ts)
	want := "频道记录-task-456-20260307-183045.html"
	if got != want {
		t.Fatalf("unexpected default file name, got=%q want=%q", got, want)
	}
}

func TestLoadMessagesForExportFiltersDiceCommandBeforeMerge(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	channelID := "ch-export-filter-before-merge-" + suffix
	if err := db.Create(&model.ChannelModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: channelID},
		Name:              "Export Filter Before Merge",
		PermType:          "public",
		Status:            model.ChannelStatusActive,
	}).Error; err != nil {
		t.Fatalf("create channel failed: %v", err)
	}

	sender := createExportTestUser(t, "u-export-filter-sender-"+suffix, "export_filter_sender_"+suffix, "过滤发送者")
	now := time.Now()
	commandMsgID := "msg-export-filter-cmd-" + suffix
	textMsgID := "msg-export-filter-text-" + suffix
	if err := db.Create(&model.MessageModel{
		StringPKBaseModel: model.StringPKBaseModel{
			ID:        commandMsgID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		ChannelID:    channelID,
		UserID:       sender.ID,
		Content:      "@守秘人。r1d5 掷骰异形表",
		DisplayOrder: float64(now.UnixMilli()),
		ICMode:       "ic",
	}).Error; err != nil {
		t.Fatalf("create command message failed: %v", err)
	}
	if err := db.Create(&model.MessageModel{
		StringPKBaseModel: model.StringPKBaseModel{
			ID:        textMsgID,
			CreatedAt: now.Add(time.Second),
			UpdatedAt: now.Add(time.Second),
		},
		ChannelID:    channelID,
		UserID:       sender.ID,
		Content:      "这是正常结果描述",
		DisplayOrder: float64(now.Add(time.Second).UnixMilli()),
		ICMode:       "ic",
	}).Error; err != nil {
		t.Fatalf("create text message failed: %v", err)
	}

	job := &model.MessageExportJobModel{
		ChannelID:       channelID,
		IncludeOOC:      true,
		IncludeArchived: true,
		MergeMessages:   true,
		ExtraOptions:    `{"include_images":true,"include_dice_commands":false}`,
	}
	messages, err := loadMessagesForExport(job)
	if err != nil {
		t.Fatalf("loadMessagesForExport failed: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message after pre-merge filter, got %d", len(messages))
	}
	if messages[0].ID != textMsgID {
		t.Fatalf("expected remaining message to be text message, got %q", messages[0].ID)
	}
	if strings.Contains(messages[0].Content, ".r1d5") {
		t.Fatalf("dice command should be filtered before merge, got %q", messages[0].Content)
	}
}

func TestMergeSequentialMessagesCollapsesBoundaryBlankLines(t *testing.T) {
	t.Parallel()

	baseTime := time.Now()
	messages := []*model.MessageModel{
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: "msg-1", CreatedAt: baseTime, UpdatedAt: baseTime},
			UserID:            "user-1",
			Content:           "第一句\n",
			ICMode:            "ic",
		},
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: "msg-2", CreatedAt: baseTime.Add(10 * time.Second), UpdatedAt: baseTime.Add(10 * time.Second)},
			UserID:            "user-1",
			Content:           "\n第二句",
			ICMode:            "ic",
		},
	}

	merged := mergeSequentialMessages(messages)
	if len(merged) != 1 {
		t.Fatalf("expected merged length 1, got %d", len(merged))
	}
	if merged[0].MergedMessages != 2 {
		t.Fatalf("expected merged count 2, got %d", merged[0].MergedMessages)
	}
	if merged[0].Content != "第一句\n第二句" {
		t.Fatalf("expected merged content without blank line, got %q", merged[0].Content)
	}
}

func TestExportEmbedMessagesRemainMergeBoundaries(t *testing.T) {
	now := time.Now()
	clue := "https://sealchat.example/#/world-a/channel-a?clue=clue-1"
	messages := []*model.MessageModel{
		{StringPKBaseModel: model.StringPKBaseModel{ID: "plain-before", CreatedAt: now}, UserID: "user-a", Content: "普通正文", ICMode: "ic"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: "clue", CreatedAt: now.Add(time.Second)}, UserID: "user-a", Content: clue, ICMode: "ic"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: "plain-after", CreatedAt: now.Add(2 * time.Second)}, UserID: "user-a", Content: "另一条正文", ICMode: "ic"},
	}
	merged := mergeSequentialMessagesForExport(messages, &exportExtraOptions{IncludeImages: true, IncludeDiceCommand: true}, true)
	if len(merged) != 3 {
		t.Fatalf("standalone clue should isolate adjacent messages, got %d: %+v", len(merged), merged)
	}
	if !isStandaloneExportEmbedMessage(clue) || isStandaloneExportEmbedMessage("普通正文") {
		t.Fatal("standalone embed classification mismatch")
	}
	sticky := "https://sealchat.example/#/world-a/channel-a?snote=note-1"
	if !isStandaloneExportEmbedMessage(sticky) {
		t.Fatal("standalone sticky-note classification mismatch")
	}
	ordinary := mergeSequentialMessagesForExport([]*model.MessageModel{
		{StringPKBaseModel: model.StringPKBaseModel{ID: "ordinary-a", CreatedAt: now}, UserID: "user-a", Content: "连续甲", ICMode: "ic"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: "ordinary-b", CreatedAt: now.Add(time.Second)}, UserID: "user-a", Content: "连续乙", ICMode: "ic"},
	}, &exportExtraOptions{IncludeImages: true, IncludeDiceCommand: true}, true)
	if len(ordinary) != 1 || !strings.Contains(ordinary[0].Content, "连续甲\n连续乙") {
		t.Fatalf("ordinary sequential messages should still merge: %+v", ordinary)
	}

	clue1 := "https://sealchat.example/#/world-a/channel-a?clue=clue-1"
	clue2 := "https://sealchat.example/#/world-a/channel-a?clue=clue-2"
	wrappedTargets, ok := parseOnlyWorldClueEmbedTargets("（" + clue1 + "\n" + clue2 + "）")
	if !ok || len(wrappedTargets) != 2 {
		t.Fatalf("wrapped multi-clue embed should parse as two targets: ok=%v targets=%+v", ok, wrappedTargets)
	}
	mergedOOC := mergeSequentialMessagesForExport([]*model.MessageModel{
		{StringPKBaseModel: model.StringPKBaseModel{ID: "ooc-clues", CreatedAt: now}, UserID: "user-a", Content: clue1 + "\n" + clue2, ICMode: "ooc"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: "ooc-ordinary", CreatedAt: now.Add(time.Second)}, UserID: "user-a", Content: "OOC 普通文本", ICMode: "ooc"},
	}, &exportExtraOptions{IncludeImages: true, IncludeDiceCommand: true, WithoutOOCParentheses: false}, true)
	if len(mergedOOC) != 2 {
		t.Fatalf("OOC multi-clue embed should remain a merge boundary, got %d: %+v", len(mergedOOC), mergedOOC)
	}
	if !isStandaloneExportEmbedMessage(mergedOOC[0].Content) {
		t.Fatalf("wrapped OOC multi-clue embed should remain standalone: %q", mergedOOC[0].Content)
	}
}

func createExportTestUser(t *testing.T, id, username, nickname string) *model.UserModel {
	t.Helper()
	user := &model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{
			ID:        id,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		},
		Username: username,
		Password: "test-password",
		Salt:     "test-salt",
		Nickname: nickname,
	}
	if err := model.GetDB().Create(user).Error; err != nil {
		t.Fatalf("create user %s failed: %v", id, err)
	}
	return user
}
