package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"sealchat/model"
	"sealchat/service"
	"sealchat/utils"
)

func TestChatExportPermissionsAPI(t *testing.T) {
	model.DBInit(&utils.AppConfig{DSN: "file:export-permissions-" + utils.NewID() + "?mode=memory&cache=shared", SQLite: utils.SQLiteConfig{ReadConnections: 1}})
	db := model.GetDB()
	for _, row := range []any{
		&model.WorldModel{StringPKBaseModel: model.StringPKBaseModel{ID: "export-world"}, Name: "World", OwnerID: "owner", Status: "active"},
		&model.WorldModel{StringPKBaseModel: model.StringPKBaseModel{ID: "other-world"}, Name: "Other", InviteSlug: "other", OwnerID: "other-owner", Status: "active"},
		&model.ChannelModel{StringPKBaseModel: model.StringPKBaseModel{ID: "export-a"}, WorldID: "export-world", Status: "active"},
		&model.ChannelModel{StringPKBaseModel: model.StringPKBaseModel{ID: "export-b"}, WorldID: "other-world", Status: "active"},
		&model.WorldMemberModel{WorldID: "export-world", UserID: "admin", Role: model.WorldRoleAdmin},
		&model.WorldMemberModel{WorldID: "export-world", UserID: "member", Role: model.WorldRoleMember},
		&model.WorldMemberModel{WorldID: "export-world", UserID: "other-member", Role: model.WorldRoleMember},
		&model.WorldMemberModel{WorldID: "export-world", UserID: "spectator", Role: model.WorldRoleSpectator},
		&model.WorldMemberModel{WorldID: "other-world", UserID: "member", Role: model.WorldRoleMember},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	app := fiber.New()
	// Supply the authenticated user directly; authorization still reads DB roles.
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("user", &model.UserModel{StringPKBaseModel: model.StringPKBaseModel{ID: c.Get("X-Test-User")}})
		return c.Next()
	})
	app.Post("/export", ChatExportCreate)
	app.Post("/batch", ChatExportBatchCreate)
	app.Get("/tasks", ChatExportList)
	app.Get("/task/:taskId", ChatExportGet)
	app.Post("/task/:taskId/retry", ChatExportRetry)
	request := func(method, path, user, body string, want int) []byte {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-User", user)
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != want {
			t.Fatalf("%s %s user=%s: status=%d want=%d body=%s", method, path, user, resp.StatusCode, want, data)
		}
		return data
	}
	for _, tc := range []struct {
		user  string
		allow bool
		want  int
	}{
		{"member", true, 200}, {"member", false, 403}, {"owner", false, 200}, {"admin", false, 200}, {"spectator", true, 403}, {"outsider", true, 403},
	} {
		if err := db.Model(&model.WorldModel{}).Where("id = ?", "export-world").Update("allow_member_export_messages", tc.allow).Error; err != nil {
			t.Fatal(err)
		}
		data := request("POST", "/export", tc.user, `{"channel_id":"export-a","format":"txt","adminExport":true}`, tc.want)
		if tc.user == "member" && !tc.allow && !strings.Contains(string(data), "当前世界未允许成员导出聊天记录") {
			t.Fatalf("missing clear denial: %s", data)
		}
	}
	if err := db.Model(&model.WorldModel{}).Where("id = ?", "other-world").Update("allow_member_export_messages", false).Error; err != nil {
		t.Fatal(err)
	}
	request("POST", "/batch", "member", `{"channel_id":"export-a","channel_ids":["export-a","export-b"],"format":"txt"}`, 403)
	// A valid anchor alone must never authorize a retry of a cross-world batch.
	if err := db.Model(&model.WorldModel{}).Where("id = ?", "other-world").Update("allow_member_export_messages", true).Error; err != nil {
		t.Fatal(err)
	}
	batch, err := service.CreateBatchMessageExportJob(&service.ExportJobOptions{UserID: "member", ChannelID: "export-a", Format: "txt"}, []string{"export-a", "export-b"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.WorldModel{}).Where("id = ?", "other-world").Update("allow_member_export_messages", false).Error; err != nil {
		t.Fatal(err)
	}
	request("POST", "/task/"+batch.ID+"/retry", "member", `{}`, 403)
	job, err := service.CreateMessageExportJob(&service.ExportJobOptions{UserID: "member", ChannelID: "export-a", Format: "txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(job).Update("upload_url", "https://example.invalid/private-export").Error; err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "?download=1"} {
		request("GET", "/task/"+job.ID+suffix, "other-member", "", 403)
	}
	request("GET", "/task/"+job.ID, "member", "", 200)
	data := request("GET", "/tasks?channel_id=export-a&status=pending", "other-member", "", 200)
	if strings.Contains(string(data), job.ID) || strings.Contains(string(data), "private-export") {
		t.Fatalf("list leaked other member task: %s", data)
	}
	if err := db.Model(&model.WorldModel{}).Where("id = ?", "export-world").Update("strict_whisper_privacy", false).Error; err != nil {
		t.Fatal(err)
	}
	for _, manager := range []string{"owner", "admin"} {
		request("GET", "/task/"+job.ID, manager, "", 200)
		if !canDeleteExportTask(manager, job) {
			t.Fatalf("%s cannot manage world task", manager)
		}
		request("GET", "/task/"+batch.ID, manager, "", 403)
		if canDeleteExportTask(manager, batch) {
			t.Fatal("anchor admin can delete cross-world task")
		}
	}
	data = request("GET", "/tasks?channel_id=export-a&status=pending", "admin", "", 200)
	if !strings.Contains(string(data), job.ID) || strings.Contains(string(data), batch.ID) {
		t.Fatalf("wrong management list: %s", data)
	}
	if err := db.Model(&model.WorldModel{}).Where("id = ?", "export-world").Update("strict_whisper_privacy", true).Error; err != nil {
		t.Fatal(err)
	}
	request("GET", "/task/"+job.ID+"?download=1", "admin", "", 403)
	if !canDeleteExportTask("admin", job) {
		t.Fatal("strict privacy unexpectedly blocked deletion")
	}
	// A creator retains file access when member export creation is disabled.
	if err := db.Model(&model.WorldModel{}).Where("id = ?", "export-world").Update("allow_member_export_messages", false).Error; err != nil {
		t.Fatal(err)
	}
	request("GET", "/task/"+job.ID, "member", "", 200)
	request("POST", "/task/"+job.ID+"/retry", "member", `{}`, 403)
	data = request("GET", "/tasks?channel_id=export-a&status=pending", "member", "", 200)
	if !strings.Contains(string(data), job.ID) {
		t.Fatal("creator lost task list")
	}
}

func TestChatExportPrivateTaskPermissions(t *testing.T) {
	model.DBInit(&utils.AppConfig{DSN: "file:export-private-" + utils.NewID() + "?mode=memory&cache=shared", SQLite: utils.SQLiteConfig{ReadConnections: 1}})
	first, second := "private-first-user-12345", "private-second-user-12345"
	channelID := first + ":" + second
	if err := model.GetDB().Create(&model.FriendModel{UserID1: first, UserID2: second}).Error; err != nil {
		t.Fatal(err)
	}
	job, err := service.CreateMessageExportJob(&service.ExportJobOptions{UserID: first, ChannelID: channelID, Format: "txt"})
	if err != nil {
		t.Fatal(err)
	}
	if !canViewExportTask(first, job) || !canViewExportTask(second, job) || canViewExportTask("outsider", job) {
		t.Fatal("private chat access changed")
	}
	if canDeleteExportTask(second, job) || !canDeleteExportTask(first, job) {
		t.Fatal("private deletion permission changed")
	}
}

func TestNormalizeExportColorMap(t *testing.T) {
	t.Parallel()

	result, err := normalizeExportColorMap(map[string]string{
		"identity:role-a": "ABC",
		"identity:role-b": "#123abc",
		"identity:":       "#112233",
		"identity:role-c": "bad-color",
		"user:role-d":     "#445566",
	})
	if err != nil {
		t.Fatalf("normalizeExportColorMap failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("unexpected normalized size: %d", len(result))
	}
	if got := result["identity:role-a"]; got != "#aabbcc" {
		t.Fatalf("unexpected normalized color for role-a: %q", got)
	}
	if got := result["identity:role-b"]; got != "#123abc" {
		t.Fatalf("unexpected normalized color for role-b: %q", got)
	}
}

func TestNormalizeExportColorMapTooManyEntries(t *testing.T) {
	t.Parallel()

	input := make(map[string]string, exportColorProfileMaxEntries+1)
	for i := 0; i < exportColorProfileMaxEntries+1; i++ {
		input[fmt.Sprintf("identity:role-%d", i)] = "#112233"
	}

	_, err := normalizeExportColorMap(input)
	if err == nil {
		t.Fatal("expected error for too many color entries, got nil")
	}
}

func TestNormalizeHexColor(t *testing.T) {
	t.Parallel()

	if got, ok := normalizeHexColor("#abc"); !ok || got != "#aabbcc" {
		t.Fatalf("expected #aabbcc, got %q, ok=%v", got, ok)
	}
	if got, ok := normalizeHexColor("ABCDEF"); !ok || got != "#abcdef" {
		t.Fatalf("expected #abcdef, got %q, ok=%v", got, ok)
	}
	if got, ok := normalizeHexColor("xyz"); ok || got != "" {
		t.Fatalf("expected invalid color, got %q, ok=%v", got, ok)
	}
}

func TestNormalizeExportColorProfiles(t *testing.T) {
	t.Parallel()

	result, err := normalizeExportColorProfiles(map[string]exportColorProfileEntry{
		"identity:role-a": {Color: "ABC", Name: "  自定义A  ", OriginalName: " 原始A "},
		"identity:role-b": {Name: "仅改名", OriginalName: "原始B"},
		"identity:role-c": {OriginalName: "原始C"},
		"user:role-d":     {Color: "#445566", Name: "忽略"},
	})
	if err != nil {
		t.Fatalf("normalizeExportColorProfiles failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("unexpected normalized profile size: %d", len(result))
	}
	if got := result["identity:role-a"]; got.Color != "#aabbcc" || got.Name != "自定义A" || got.OriginalName != "原始A" {
		t.Fatalf("unexpected normalized profile for role-a: %#v", got)
	}
	if got := result["identity:role-b"]; got.Color != "" || got.Name != "仅改名" || got.OriginalName != "原始B" {
		t.Fatalf("unexpected normalized profile for role-b: %#v", got)
	}
}

func TestParseExportColorProfileJSONSupportsLegacyColorMap(t *testing.T) {
	t.Parallel()

	profiles := parseExportColorProfileJSON(`{"identity:role-a":"#123abc"}`)
	if got := profiles["identity:role-a"]; got.Color != "#123abc" || got.Name != "" {
		t.Fatalf("unexpected legacy profile parse result: %#v", got)
	}
}

func TestParseExportColorProfileJSONSupportsStructuredProfiles(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(exportColorProfileDocument{
		Version: exportColorProfileFormatVersion,
		Profiles: map[string]exportColorProfileEntry{
			"identity:role-a": {Color: "#123abc", Name: "别名A", OriginalName: "原名A"},
		},
	})
	if err != nil {
		t.Fatalf("marshal structured profile failed: %v", err)
	}
	profiles := parseExportColorProfileJSON(string(raw))
	if got := profiles["identity:role-a"]; got.Color != "#123abc" || got.Name != "别名A" || got.OriginalName != "原名A" {
		t.Fatalf("unexpected structured profile parse result: %#v", got)
	}
}

func TestNormalizeExportProfileMatchName(t *testing.T) {
	t.Parallel()

	got := normalizeExportProfileMatchName("  Alice   The   Brave  ")
	if got != "alice the brave" {
		t.Fatalf("unexpected normalized match name: %q", got)
	}
}

// Kept in an existing tracked export test file because *_test.go is ignored.
func TestLogExportCreateAPI(t *testing.T) {
	model.DBInit(&utils.AppConfig{DSN: "file:log-export-api-" + utils.NewID() + "?mode=memory&cache=shared", SQLite: utils.SQLiteConfig{ReadConnections: 1}})
	db := model.GetDB()
	for _, row := range []any{
		&model.UserModel{StringPKBaseModel: model.StringPKBaseModel{ID: "log-export-owner"}, Username: "log-export-owner"},
		&model.UserModel{StringPKBaseModel: model.StringPKBaseModel{ID: "log-export-outsider"}, Username: "log-export-outsider"},
		&model.UserModel{StringPKBaseModel: model.StringPKBaseModel{ID: "log-export-member"}, Username: "log-export-member"},
		&model.WorldModel{StringPKBaseModel: model.StringPKBaseModel{ID: "log-export-world"}, Name: "Export", Status: "active", OwnerID: "log-export-owner", AllowMemberAccessLogLayout: true},
		&model.WorldMemberModel{WorldID: "log-export-world", UserID: "log-export-owner", Role: model.WorldRoleOwner},
		&model.WorldMemberModel{WorldID: "log-export-world", UserID: "log-export-member", Role: model.WorldRoleMember},
		&model.ChannelModel{StringPKBaseModel: model.StringPKBaseModel{ID: "log-export-source"}, WorldID: "log-export-world", UserID: "log-export-owner", Name: "Source", Status: model.ChannelStatusActive, PermType: "public"},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := service.RegisterBuiltinLogStyles(); err != nil {
		t.Fatal(err)
	}
	on := true
	p, err := service.UpdateLogProject("log-export-world", "log-export-owner", service.LogProjectUpdate{Enabled: &on})
	if err != nil {
		t.Fatal(err)
	}
	var draft service.LogDraft
	if err := json.Unmarshal(p.Draft, &draft); err != nil {
		t.Fatal(err)
	}
	draft.Sections = []service.LogSection{{ID: "00000000-0000-4000-8000-000000000001", Title: "Source", Source: service.LogSourceSpec{ChannelID: "log-export-source", ToTime: time.Now().Add(time.Hour), IdentityIDs: []string{}}}}
	raw, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := service.SaveLogDraft(p.ProjectID, "log-export-owner", service.LogDraftUpdate{ExpectedVersion: p.DraftVersion, Draft: raw})
	if err != nil {
		t.Fatal(err)
	}
	tokens := map[string]string{}
	for _, id := range []string{"log-export-owner", "log-export-outsider", "log-export-member"} {
		token, err := model.UserGenerateAccessToken(id)
		if err != nil {
			t.Fatal(err)
		}
		tokens[id] = token
	}
	app := fiber.New()
	bindLogProjectRoutes(app.Group("/api/v1"))
	app.Post("/api/v1/chat/export/:taskId/retry", logAuthentication, ChatExportRetry)
	app.Get("/api/v1/chat/export/:taskId", logAuthentication, ChatExportGet)
	path := "/api/v1/log/projects/" + p.ProjectID + "/export"
	for _, tc := range []struct {
		name, user, format          string
		stale, sourceDenied, bbcode bool
		status                      int
		code                        string
	}{
		{name: "txt", user: "log-export-owner", format: "txt", status: 200},
		{name: "docx", user: "log-export-owner", format: "docx", status: 200},
		{name: "bbcode", user: "log-export-owner", format: "txt", bbcode: true, status: 200},
		{name: "member", user: "log-export-member", format: "txt", status: 200},
		{name: "stale", user: "log-export-owner", format: "txt", stale: true, status: 409, code: "VERSION_CONFLICT"},
		{name: "project denied", user: "log-export-outsider", format: "txt", status: 403, code: "PERMISSION_DENIED"},
		{name: "source denied", user: "log-export-owner", format: "txt", sourceDenied: true, status: 403, code: "PERMISSION_DENIED"},
		{name: "html", user: "log-export-owner", format: "html", status: 200},
		{name: "json", user: "log-export-owner", format: "json", status: 200},
		{name: "bbcode format", user: "log-export-owner", format: "bbcode", status: 400, code: "INVALID_INPUT"},
		{name: "unauthenticated", format: "txt", status: 401, code: "UNAUTHENTICATED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := db.Model(&model.ChannelModel{}).Where("id = ?", "log-export-source").Update("is_private", tc.sourceDenied).Error; err != nil {
				t.Fatal(err)
			}
			version := saved.DraftVersion
			if tc.stale {
				version--
			}
			body := fmt.Sprintf(`{"expectedDraftVersion":%d,"format":%q,"textBbcodeColorize":%t}`, version, tc.format, tc.bbcode)
			req := httptest.NewRequest("POST", path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", tokens[tc.user])
			resp, err := app.Test(req, -1)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			var result struct {
				Data  chatExportResponse `json:"data"`
				Error service.LogError   `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tc.status || result.Error.Code != tc.code {
				t.Fatalf("status=%d result=%+v", resp.StatusCode, result)
			}
			if tc.status != 200 {
				return
			}
			if result.Data.Status != model.MessageExportStatusPending || result.Data.TaskID == "" || result.Data.RequestedAt == 0 {
				t.Fatalf("invalid task response: %+v", result.Data)
			}
			job, err := service.GetMessageExportJob(result.Data.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			var extra struct {
				SourceKind   string `json:"source_kind"`
				ProjectID    string `json:"log_project_id"`
				DraftVersion int64  `json:"log_draft_version"`
				BBCode       bool   `json:"text_colorize_bbcode"`
			}
			if err := json.Unmarshal([]byte(job.ExtraOptions), &extra); err != nil {
				t.Fatal(err)
			}
			if job.ChannelID != "" || job.Format != tc.format || extra.SourceKind != "log" || extra.ProjectID != p.ProjectID || extra.DraftVersion != saved.DraftVersion || extra.BBCode != tc.bbcode {
				t.Fatalf("invalid job: %+v extra=%+v", job, extra)
			}
			// Existing task endpoints work with an empty channel, including retry.
			for _, method := range []string{"GET", "POST"} {
				taskPath := "/api/v1/chat/export/" + job.ID
				if method == "POST" {
					taskPath += "/retry"
				}
				taskReq := httptest.NewRequest(method, taskPath, nil)
				taskReq.Header.Set("Authorization", tokens[tc.user])
				taskResp, err := app.Test(taskReq, -1)
				if err != nil {
					t.Fatal(err)
				}
				taskResp.Body.Close()
				if taskResp.StatusCode != 200 {
					t.Fatalf("%s existing task endpoint: %d", method, taskResp.StatusCode)
				}
			}
		})
	}
}
