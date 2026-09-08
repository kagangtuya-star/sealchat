package service

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"sealchat/model"
	"sealchat/utils"
)

func TestNormalizeDomainToURLIPv6(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "ipv6 with port",
			input: "[2001:db8::1]:3212",
			want:  "https://[2001:db8::1]:3212",
		},
		{
			name:  "ipv6 loopback without port",
			input: "::1",
			want:  "http://[::1]",
		},
		{
			name:  "ipv4 loopback",
			input: "127.0.0.1:8080",
			want:  "http://127.0.0.1:8080",
		},
		{
			name:  "ipv6 link-local",
			input: "fe80::1",
			want:  "http://[fe80::1]",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeDomainToURL(tt.input); got != tt.want {
				t.Fatalf("normalizeDomainToURL(%q) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestEnhancePlainContentForHTMLExportQuickFormat(t *testing.T) {
	input := "**粗体** *斜体* `代码` [链接](https://example.com)"
	result := enhancePlainContentForHTMLExport(input)

	expects := []string{
		"<strong>粗体</strong>",
		"<em>斜体</em>",
		"<code>代码</code>",
		`<a href="https://example.com" class="text-blue-500" target="_blank" rel="noopener noreferrer">链接</a>`,
	}

	for _, expected := range expects {
		if !strings.Contains(result, expected) {
			t.Fatalf("expect html contains %q, got %q", expected, result)
		}
	}
}

func TestBuildExportPayloadDoesNotRenderInlineCodeForBotCommandMessage(t *testing.T) {
	initTestDB(t)
	now := time.Unix(1700001200, 0)
	job := &model.MessageExportJobModel{
		ChannelID:       "channel-bot-command-inline-code",
		IncludeOOC:      true,
		IncludeArchived: true,
	}
	messages := []*model.MessageModel{
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: "cmd", CreatedAt: now, UpdatedAt: now},
			UserID:            "user-a",
			Content:           ".ra `1d100` **侦查**",
			ICMode:            "ic",
		},
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: "plain", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)},
			UserID:            "user-a",
			Content:           "普通消息 `1d100` **侦查**",
			ICMode:            "ic",
		},
	}

	payload := buildExportPayload(job, "BOT 指令频道", messages, nil, &exportExtraOptions{
		IncludeImages:      true,
		IncludeDiceCommand: true,
	})

	if payload == nil || len(payload.Messages) != 2 {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	commandHTML := payload.Messages[0].ContentHTML
	if strings.Contains(commandHTML, "<code>1d100</code>") {
		t.Fatalf("bot command inline code should remain literal, got %q", commandHTML)
	}
	if !strings.Contains(commandHTML, "`1d100`") {
		t.Fatalf("bot command inline code backticks should remain, got %q", commandHTML)
	}
	if strings.Contains(commandHTML, "<strong>侦查</strong>") {
		t.Fatalf("bot command should not render bold formatting, got %q", commandHTML)
	}
	if !strings.Contains(commandHTML, "**侦查**") {
		t.Fatalf("bot command bold marker should remain literal, got %q", commandHTML)
	}

	plainHTML := payload.Messages[1].ContentHTML
	if !strings.Contains(plainHTML, "<code>1d100</code>") {
		t.Fatalf("normal message should still render inline code, got %q", plainHTML)
	}
}

func TestEnhancePlainContentForHTMLExportInvalidLink(t *testing.T) {
	input := "[危险](javascript:alert(1))"
	result := enhancePlainContentForHTMLExport(input)
	if strings.Contains(result, "<a ") {
		t.Fatalf("invalid protocol should not become link, got %q", result)
	}
}

func TestEnhancePlainContentForHTMLExportNormalizesNestedEntities(t *testing.T) {
	input := "他说 &amp;quot;你好&amp;quot; 和 &amp;amp;"
	result := enhancePlainContentForHTMLExport(input)
	if strings.Contains(result, "&amp;quot;") {
		t.Fatalf("nested quote entity should be normalized, got %q", result)
	}
	if strings.Contains(result, "&amp;amp;") {
		t.Fatalf("nested amp entity should be normalized, got %q", result)
	}
	if !strings.Contains(result, "&#34;你好&#34;") && !strings.Contains(result, "&quot;你好&quot;") {
		t.Fatalf("expected canonical quote entities, got %q", result)
	}
}

func TestStripRichTextDecodesNestedEntities(t *testing.T) {
	got := stripRichText("<p>&amp;quot;你好&amp;quot; &amp;amp;</p>")
	if got != "\"你好\" &" {
		t.Fatalf("stripRichText nested entity decode failed, got %q", got)
	}
}

func TestBuildBBCodeTextLineFromQuickFormat(t *testing.T) {
	payload := &ExportPayload{WithoutTimestamp: true}
	msg := &ExportMessage{
		SenderName:  "测试",
		SenderColor: "#123abc",
		CreatedAt:   time.Unix(1700000000, 0),
		Content:     "**粗体** *斜体* `代码` [链接](https://example.com)",
	}

	line := buildBBCodeTextLine(payload, msg)
	expects := []string{"[b]粗体[/b]", "[i]斜体[/i]", "[code]代码[/code]", "[url=https://example.com]链接[/url]"}
	for _, expected := range expects {
		if !strings.Contains(line, expected) {
			t.Fatalf("expect bbcode contains %q, got %q", expected, line)
		}
	}
}

func TestBuildBBCodeTextLineUsesOverrideColorMap(t *testing.T) {
	payload := &ExportPayload{
		WithoutTimestamp: true,
		ExtraMeta: map[string]interface{}{
			"text_colorize_bbcode":     true,
			"text_colorize_bbcode_map": map[string]string{"identity:role-a": "#aabbcc"},
		},
	}
	msg := &ExportMessage{
		SenderIdentityID: "role-a",
		SenderName:       "测试",
		SenderColor:      "#123abc",
		CreatedAt:        time.Unix(1700000000, 0),
		Content:          "hello",
	}

	line := buildBBCodeTextLine(payload, msg)
	if !strings.Contains(line, "[color=#aabbcc]") {
		t.Fatalf("expected override color in line, got %q", line)
	}
}

func TestBuildBBCodeTextLineFallsBackWhenOverrideInvalid(t *testing.T) {
	payload := &ExportPayload{
		WithoutTimestamp: true,
		ExtraMeta: map[string]interface{}{
			"text_colorize_bbcode":     true,
			"text_colorize_bbcode_map": map[string]string{"identity:role-a": "bad-color"},
		},
	}
	msg := &ExportMessage{
		SenderIdentityID: "role-a",
		SenderName:       "测试",
		SenderColor:      "#123abc",
		CreatedAt:        time.Unix(1700000000, 0),
		Content:          "hello",
	}

	line := buildBBCodeTextLine(payload, msg)
	if !strings.Contains(line, "[color=#123abc]") {
		t.Fatalf("expected sender snapshot color fallback in line, got %q", line)
	}
}

func TestBuildBBCodeTextLineUsesOverrideNameMap(t *testing.T) {
	payload := &ExportPayload{
		WithoutTimestamp: true,
		ExtraMeta: map[string]interface{}{
			"text_colorize_bbcode":          true,
			"text_colorize_bbcode_name_map": map[string]string{"identity:role-a": "阿尔法"},
		},
	}
	msg := &ExportMessage{
		SenderIdentityID: "role-a",
		SenderName:       "测试",
		SenderColor:      "#123abc",
		CreatedAt:        time.Unix(1700000000, 0),
		Content:          "hello",
	}

	line := buildBBCodeTextLine(payload, msg)
	if !strings.Contains(line, "<阿尔法>") {
		t.Fatalf("expected override name in line, got %q", line)
	}
}

func TestBuildBBCodeTextLineNormalizesNestedEntitiesForPlainText(t *testing.T) {
	payload := &ExportPayload{WithoutTimestamp: true}
	msg := &ExportMessage{
		SenderName:  "测试",
		SenderColor: "#123abc",
		CreatedAt:   time.Unix(1700000001, 0),
		Content:     "他说 &amp;quot;你好&amp;quot; 和 &amp;amp;",
	}

	line := buildBBCodeTextLine(payload, msg)
	if strings.Contains(line, "&amp;quot;") || strings.Contains(line, "&quot;") {
		t.Fatalf("nested quote entity should be normalized in bbcode line, got %q", line)
	}
	if !strings.Contains(line, "\"你好\"") {
		t.Fatalf("expected decoded quote text in bbcode line, got %q", line)
	}
	if !strings.Contains(line, "和 &") {
		t.Fatalf("expected decoded ampersand in bbcode line, got %q", line)
	}
}

func TestEnhancePlainContentForHTMLExportDoesNotRenderCodeFence(t *testing.T) {
	input := "```\nconst a = 1\n```"
	result := enhancePlainContentForHTMLExport(input)
	if strings.Contains(result, "<pre><code>") {
		t.Fatalf("code fence should not become code block, got %q", result)
	}
	if !strings.Contains(result, "```") {
		t.Fatalf("code fence should remain literal text, got %q", result)
	}
}

func TestBuildBBCodeTextLineDoesNotRenderCodeFence(t *testing.T) {
	payload := &ExportPayload{WithoutTimestamp: true}
	msg := &ExportMessage{
		SenderName:  "测试",
		SenderColor: "#123abc",
		CreatedAt:   time.Unix(1700000000, 0),
		Content:     "```hello```",
	}

	line := buildBBCodeTextLine(payload, msg)
	if strings.Contains(line, "[code]") {
		t.Fatalf("code fence should not convert to [code], got %q", line)
	}
	if !strings.Contains(line, "```hello```") {
		t.Fatalf("code fence should remain literal text, got %q", line)
	}
}

func TestBuildBBCodeTextLineDoesNotRenderInlineCodeForBotCommandMessage(t *testing.T) {
	payload := &ExportPayload{WithoutTimestamp: true}
	msg := &ExportMessage{
		SenderName:  "测试",
		SenderColor: "#123abc",
		CreatedAt:   time.Unix(1700000000, 0),
		Content:     ".ra `1d100` **侦查**",
	}

	line := buildBBCodeTextLine(payload, msg)
	if strings.Contains(line, "[code]1d100[/code]") {
		t.Fatalf("bot command inline code should remain literal, got %q", line)
	}
	if !strings.Contains(line, "`1d100`") {
		t.Fatalf("bot command inline code backticks should remain, got %q", line)
	}
	if strings.Contains(line, "[b]侦查[/b]") {
		t.Fatalf("bot command should not render bold formatting, got %q", line)
	}
	if !strings.Contains(line, "**侦查**") {
		t.Fatalf("bot command bold marker should remain literal, got %q", line)
	}
}

func TestStripInlineCodeTagsFromHTMLPreservesBackticks(t *testing.T) {
	got := stripInlineCodeTagsFromHTML(`前缀 <code>1d100</code> 后缀`)
	want := "前缀 `1d100` 后缀"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestBuildExportPayloadUsesMessageIdentitySnapshotAvatar(t *testing.T) {
	job := &model.MessageExportJobModel{
		ChannelID: "channel-export-variant",
	}
	createdAt := time.Unix(1700000200, 0)
	msg := &model.MessageModel{
		StringPKBaseModel:       model.StringPKBaseModel{ID: "msg-variant", CreatedAt: createdAt},
		UserID:                  "user-1",
		Content:                 "测试消息",
		ICMode:                  "ic",
		SenderIdentityID:        "identity-1",
		SenderIdentityVariantID: "variant-1",
		SenderIdentityName:      "战斗形态",
		SenderIdentityColor:     "#ff6600",
		SenderIdentityAvatarID:  "avatar-variant-1",
		User: &model.UserModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: "user-1"},
			Username:          "fallback_user",
			Nickname:          "回退昵称",
			Avatar:            "https://example.com/fallback.png",
		},
	}

	payload := buildExportPayload(job, "测试频道", []*model.MessageModel{msg}, nil, nil)
	if payload == nil || len(payload.Messages) != 1 {
		t.Fatalf("expected 1 export message, got %+v", payload)
	}
	exported := payload.Messages[0]
	if exported.SenderIdentityID != "identity-1" {
		t.Fatalf("expected sender identity id snapshot, got %q", exported.SenderIdentityID)
	}
	if exported.SenderName != "战斗形态" {
		t.Fatalf("expected sender name from snapshot, got %q", exported.SenderName)
	}
	if exported.SenderAvatar != "id:avatar-variant-1" {
		t.Fatalf("expected variant avatar snapshot to be exported, got %q", exported.SenderAvatar)
	}
	if exported.SenderColor != "#ff6600" {
		t.Fatalf("expected sender color snapshot, got %q", exported.SenderColor)
	}
}

func TestBuildExportPayloadDoesNotFallbackToUserAvatarForTemporaryIdentity(t *testing.T) {
	job := &model.MessageExportJobModel{
		ChannelID: "channel-export-temp",
	}
	createdAt := time.Unix(1700000300, 0)
	msg := &model.MessageModel{
		StringPKBaseModel:         model.StringPKBaseModel{ID: "msg-temp", CreatedAt: createdAt},
		UserID:                    "user-temp",
		Content:                   "临时角色消息",
		ICMode:                    "ic",
		SenderIdentityID:          "identity-temp",
		SenderIdentityName:        "路人甲",
		SenderIdentityIsTemporary: true,
		User: &model.UserModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: "user-temp"},
			Username:          "temp_user",
			Avatar:            "https://example.com/user-avatar.png",
		},
	}

	payload := buildExportPayload(job, "测试频道", []*model.MessageModel{msg}, nil, nil)
	if payload == nil || len(payload.Messages) != 1 {
		t.Fatalf("expected 1 export message, got %+v", payload)
	}
	if payload.Messages[0].SenderAvatar != "" {
		t.Fatalf("temporary identity should not fallback to user avatar, got %q", payload.Messages[0].SenderAvatar)
	}
}

func TestExtractWhisperTargetsPreferRoleNameOverUserName(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()
	if err := db.Create(&model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "u1"},
		Username:          "target_user_name",
		Password:          "test-password",
		Salt:              "test-salt",
		Nickname:          "目标用户昵称",
	}).Error; err != nil {
		t.Fatalf("create user u1 failed: %v", err)
	}
	if err := db.Create(&model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "u2"},
		Username:          "target_user_name_2",
		Password:          "test-password",
		Salt:              "test-salt",
		Nickname:          "目标乙昵称",
	}).Error; err != nil {
		t.Fatalf("create user u2 failed: %v", err)
	}

	msg := &model.MessageModel{
		IsWhisper:               true,
		WhisperTo:               "u1",
		WhisperTargetMemberName: "角色甲",
		WhisperTarget: &model.UserModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: "u1"},
			Username:          "target_user_name",
			Nickname:          "目标用户昵称",
		},
		WhisperTargets: []*model.UserModel{
			{
				StringPKBaseModel: model.StringPKBaseModel{ID: "u1"},
				Username:          "target_user_name",
				Nickname:          "目标用户昵称",
			},
			{
				StringPKBaseModel: model.StringPKBaseModel{ID: "u2"},
				Username:          "target_user_name_2",
				Nickname:          "目标乙昵称",
			},
		},
	}

	targets := extractWhisperTargets(msg, "", nil)
	if len(targets) == 0 {
		t.Fatalf("extractWhisperTargets returned empty")
	}
	if targets[0] != "角色甲" {
		t.Fatalf("expected first target to be role name, got %v", targets)
	}
	if slices.Contains(targets, "目标用户昵称") {
		t.Fatalf("primary target should prefer role name instead of user nickname, got %v", targets)
	}
	if !slices.Contains(targets, "目标乙昵称") {
		t.Fatalf("secondary targets should still be present, got %v", targets)
	}
}

func TestBuildExportPayloadFiltersImagesWhenDisabled(t *testing.T) {
	initTestDB(t)
	now := time.Unix(1700001000, 0)
	job := &model.MessageExportJobModel{
		ChannelID:       "channel-filter-image",
		IncludeOOC:      true,
		IncludeArchived: true,
		MergeMessages:   false,
	}
	messages := []*model.MessageModel{
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: "img-only", CreatedAt: now, UpdatedAt: now},
			UserID:            "user-a",
			Content:           `<img src="https://example.com/a.png" />`,
			ICMode:            "ic",
		},
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: "img-mixed", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)},
			UserID:            "user-a",
			Content:           `带图文本 <img src="https://example.com/b.png" />`,
			ICMode:            "ic",
		},
	}

	payload := buildExportPayload(job, "图片过滤频道", messages, nil, &exportExtraOptions{
		IncludeImages:      false,
		IncludeDiceCommand: true,
	})

	if payload == nil {
		t.Fatalf("payload should not be nil")
	}
	if len(payload.Messages) != 1 {
		t.Fatalf("expected 1 message after image filtering, got %d", len(payload.Messages))
	}
	if payload.Messages[0].ID != "img-mixed" {
		t.Fatalf("expected mixed message to remain, got %q", payload.Messages[0].ID)
	}
	if strings.Contains(payload.Messages[0].ContentHTML, "<img") {
		t.Fatalf("html content should not contain img tag, got %q", payload.Messages[0].ContentHTML)
	}
	line := buildPlainTextLine(payload, &payload.Messages[0])
	if strings.Contains(line, "[CQ:image") {
		t.Fatalf("plain text line should not contain image CQ token, got %q", line)
	}
}

func TestBuildExportPayloadAppliesStoredImageLayoutToHTML(t *testing.T) {
	initTestDB(t)
	now := time.Unix(1700001500, 0)
	channelID := "channel-image-layout-export"
	if err := model.ChannelAttachmentImageLayoutUpsertBatch(channelID, "tester", []model.ChannelAttachmentImageLayoutUpsertItem{
		{
			AttachmentID: "layout-att-1",
			Width:        320,
			Height:       180,
		},
	}); err != nil {
		t.Fatalf("upsert image layout failed: %v", err)
	}

	job := &model.MessageExportJobModel{
		ChannelID:       channelID,
		IncludeOOC:      true,
		IncludeArchived: true,
		MergeMessages:   false,
	}
	messages := []*model.MessageModel{
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: "img-layout", CreatedAt: now, UpdatedAt: now},
			UserID:            "user-a",
			Content:           `<img src="id:layout-att-1" alt="图" />`,
			ICMode:            "ic",
		},
	}

	payload := buildExportPayload(job, "图片尺寸频道", messages, nil, &exportExtraOptions{
		IncludeImages:      true,
		IncludeDiceCommand: true,
	})

	if payload == nil || len(payload.Messages) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	html := payload.Messages[0].ContentHTML
	expects := []string{
		`data-attachment-id="layout-att-1"`,
		`width:320px;`,
		`height:180px;`,
		`max-width:none;`,
		`max-height:none;`,
	}
	for _, expected := range expects {
		if !strings.Contains(html, expected) {
			t.Fatalf("expected content html contains %q, got %q", expected, html)
		}
	}
}

func TestBuildExportPayloadFiltersSingleLineDiceCommandWhenDisabled(t *testing.T) {
	initTestDB(t)
	now := time.Unix(1700002000, 0)
	job := &model.MessageExportJobModel{
		ChannelID:       "channel-filter-dice-command",
		IncludeOOC:      true,
		IncludeArchived: true,
		MergeMessages:   false,
	}
	messages := []*model.MessageModel{
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: "cmd", CreatedAt: now, UpdatedAt: now},
			UserID:            "user-a",
			Content:           ".ra 侦查",
			ICMode:            "ic",
		},
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: "result", CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)},
			UserID:            "bot-a",
			Content:           "检定结果 D100=42 困难成功",
			ICMode:            "ic",
		},
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: "multi-line", CreatedAt: now.Add(2 * time.Second), UpdatedAt: now.Add(2 * time.Second)},
			UserID:            "user-a",
			Content:           ".ra\n继续说明",
			ICMode:            "ic",
		},
	}

	payload := buildExportPayload(job, "指令过滤频道", messages, nil, &exportExtraOptions{
		IncludeImages:      true,
		IncludeDiceCommand: false,
	})

	if payload == nil {
		t.Fatalf("payload should not be nil")
	}
	if len(payload.Messages) != 2 {
		t.Fatalf("expected 2 messages after dice command filtering, got %d", len(payload.Messages))
	}
	ids := []string{payload.Messages[0].ID, payload.Messages[1].ID}
	if slices.Contains(ids, "cmd") {
		t.Fatalf("single-line command should be filtered, got ids %v", ids)
	}
	if !slices.Contains(ids, "result") || !slices.Contains(ids, "multi-line") {
		t.Fatalf("result and multiline message should remain, got ids %v", ids)
	}
}

func TestIsSingleLineDiceCommandDefaultPrefixes(t *testing.T) {
	if !isSingleLineDiceCommand(".ra 侦查") {
		t.Fatalf("dot prefix should match by default")
	}
	if !isSingleLineDiceCommand("。掷骰 侦查") {
		t.Fatalf("chinese dot prefix should match by default")
	}
	if !isSingleLineDiceCommand("@守秘人。r1d100 侦查") {
		t.Fatalf("mention token with command should match")
	}
	if !isSingleLineDiceCommand("请看 .ra 侦查") {
		t.Fatalf("command token in message should match")
	}
	if isSingleLineDiceCommand("普通消息。然后继续") {
		t.Fatalf("normal sentence should not match as command")
	}
	if isSingleLineDiceCommand("/ra 侦查") {
		t.Fatalf("slash prefix should not match by default")
	}
}

func TestIsSingleLineDiceCommandWithCustomPrefixes(t *testing.T) {
	if !isSingleLineDiceCommandWithPrefixes("/ra 侦查", []string{"/"}) {
		t.Fatalf("slash prefix should match when customized")
	}
	if isSingleLineDiceCommandWithPrefixes(".ra 侦查", []string{"/"}) {
		t.Fatalf("dot prefix should not match when only slash is configured")
	}
}

func TestBuildExportPayloadMarksMergedMessages(t *testing.T) {
	initTestDB(t)

	payload := buildExportPayload(
		&model.MessageExportJobModel{ChannelID: "ch-1", IncludeOOC: true, IncludeArchived: true},
		"测试频道",
		[]*model.MessageModel{
			{
				StringPKBaseModel: model.StringPKBaseModel{
					ID:        "msg-1",
					CreatedAt: time.Unix(1700000000, 0),
					UpdatedAt: time.Unix(1700000000, 0),
				},
				UserID:         "user-1",
				Content:        "hello",
				ICMode:         "ic",
				MergedMessages: 2,
			},
		},
		nil,
		nil,
	)

	if payload == nil || len(payload.Messages) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if !payload.Messages[0].IsMerged {
		t.Fatalf("expected merged export message flag, got %+v", payload.Messages[0])
	}
}

func TestParseOnlyWorldClueEmbedTargets(t *testing.T) {
	first := "https://sealchat.example/#/world-a/channel-a?clue=clue-1"
	second := "https://sealchat.example/#/world-a/channel-a?clue=clue-2"
	targets, ok := parseOnlyWorldClueEmbedTargets(first + "\n" + second)
	if !ok || len(targets) != 2 || targets[0].ClueID != "clue-1" || targets[1].ClueID != "clue-2" {
		t.Fatalf("unexpected clue targets: ok=%v targets=%+v", ok, targets)
	}
	tiptap := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + first + `","marks":[{"type":"link","attrs":{"href":"` + first + `"}}]}]}]}`
	targets, ok = parseOnlyWorldClueEmbedTargets(tiptap)
	if !ok || len(targets) != 1 || targets[0].ClueID != "clue-1" {
		t.Fatalf("TipTap clue target mismatch: ok=%v targets=%+v", ok, targets)
	}
	if _, ok := parseOnlyWorldClueEmbedTargets(first + " 普通正文"); ok {
		t.Fatal("clue link mixed with ordinary text must not be recognized")
	}
}

func createWorldClueExportImageAttachment(t *testing.T, id, userID, worldID string) {
	t.Helper()
	cfg := utils.ReadConfig()
	oldUploadDir := cfg.Storage.Local.UploadDir
	uploadDir := t.TempDir()
	cfg.Storage.Local.UploadDir = uploadDir
	t.Cleanup(func() {
		cfg.Storage.Local.UploadDir = oldUploadDir
	})
	imageData, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO7Z0ioAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatalf("decode clue png fixture failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uploadDir, id), imageData, 0644); err != nil {
		t.Fatalf("write clue image fixture failed: %v", err)
	}
	hash := sha256.Sum256(imageData)
	attachment := &model.AttachmentModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: id, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		Hash:              model.ByteArray(hash[:]),
		Filename:          "clue.png",
		Size:              int64(len(imageData)),
		MimeType:          "image/png",
		UserID:            userID,
		StorageType:       model.StorageLocal,
		ObjectKey:         id,
		RootID:            worldID,
		RootIDType:        "world_clue",
		IsTemp:            true,
	}
	if err := model.GetDB().Create(attachment).Error; err != nil {
		t.Fatalf("create clue image attachment failed: %v", err)
	}
}

func TestWorldClueExportResolverHonorsScopeAndPrivateVisibility(t *testing.T) {
	users := setupWorldClueTest(t)
	channelID := "clue-export-channel"
	if err := model.GetDB().Create(&model.ChannelModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: channelID}, WorldID: users.worldID,
		Name: "线索导出频道", PermType: "public", Status: model.ChannelStatusActive,
	}).Error; err != nil {
		t.Fatalf("create clue export channel: %v", err)
	}
	clue := createWorldClueForTest(t, users, model.WorldClueAccessView)
	if _, err := WorldCluePublish(users.worldID, clue.ID, users.owner, clue.PublishSeq); err != nil {
		t.Fatalf("publish clue: %v", err)
	}
	if _, err := WorldCluePutPrivate(users.worldID, clue.ID, users.owner, users.memberA, model.WorldClueContentPlain, "member A secret", 0); err != nil {
		t.Fatalf("put member A private content: %v", err)
	}
	if _, err := WorldCluePutPrivate(users.worldID, clue.ID, users.owner, users.memberB, model.WorldClueContentPlain, "member B secret", 0); err != nil {
		t.Fatalf("put member B private content: %v", err)
	}
	link := "https://sealchat.example/#/" + users.worldID + "/" + channelID + "?clue=" + clue.ID
	memberRender, ok := newWorldClueExportResolver(channelID, users.memberA).render(link, true)
	if !ok || !strings.Contains(memberRender.Plain, "member A secret") || strings.Contains(memberRender.Plain, "member B secret") {
		t.Fatalf("member private visibility mismatch: ok=%v plain=%q", ok, memberRender.Plain)
	}
	adminRender, ok := newWorldClueExportResolver(channelID, users.owner).render(link, true)
	if !ok || !strings.Contains(adminRender.Plain, "member A secret") || !strings.Contains(adminRender.Plain, "member B secret") || !strings.Contains(adminRender.Plain, "Ancient Key") || strings.Contains(adminRender.Plain, "manager secret") {
		t.Fatalf("admin clue export mismatch: ok=%v plain=%q", ok, adminRender.Plain)
	}
	wrongWorld := "https://sealchat.example/#/other-world/" + channelID + "?clue=" + clue.ID
	if _, ok := newWorldClueExportResolver(channelID, users.memberA).render(wrongWorld, true); ok {
		t.Fatal("cross-world clue link must not expand")
	}
}

func TestWorldClueExportHTMLRenderingHonorsImagesAndIframeSafety(t *testing.T) {
	block := WorldClueExportBlock{
		Title:           "资料 <一>",
		Kind:            model.WorldClueKindIframe,
		Public:          AgentRichDocument{Type: "document", Blocks: []AgentRichNode{{Type: "paragraph", Children: []AgentRichNode{{Type: "text", Text: "正文"}, {Type: "image", Attrs: map[string]string{"src": "https://cdn.example/image.png", "alt": "图片"}}}}}},
		Private:         []WorldClueExportPrivateSection{{MemberName: "成员甲", Content: buildRichDocumentForExport("专属内容", true)}},
		IframeSourceURL: "https://example.com/source?a=1&b=2",
	}
	html := renderWorldClueHTML(block, true)
	for _, expected := range []string{"export-world-clue", "资料 &lt;一&gt;", "正文", "<img", "成员甲", "https://example.com/source?a=1&amp;b=2"} {
		if !strings.Contains(html, expected) {
			t.Fatalf("clue html missing %q: %s", expected, html)
		}
	}
	if strings.Contains(html, "<iframe") {
		t.Fatalf("iframe clue must render a source link only: %s", html)
	}
	withoutImages := renderWorldClueHTML(block, false)
	if strings.Contains(withoutImages, "<img") {
		t.Fatalf("clue images must be omitted when disabled: %s", withoutImages)
	}
}

func TestWorldClueExportHTMLAttachmentUsesInlineAsset(t *testing.T) {
	users := setupWorldClueTest(t)
	channelID := "clue-export-html-image-channel"
	if err := model.GetDB().Create(&model.ChannelModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: channelID}, WorldID: users.worldID,
		Name: "线索图片导出频道", PermType: "public", Status: model.ChannelStatusActive,
	}).Error; err != nil {
		t.Fatalf("create clue export channel: %v", err)
	}
	const attachmentID = "clue-export-html-image"
	createWorldClueExportImageAttachment(t, attachmentID, users.owner, users.worldID)
	clue, err := WorldClueCreate(users.worldID, users.owner, WorldClueCreateInput{
		Title: "主媒体线索", Kind: model.WorldClueKindImage, ContentFormat: model.WorldClueContentPlain,
		Content: "线索正文", ImageAttachmentID: attachmentID, DefaultAccess: model.WorldClueAccessView,
	})
	if err != nil {
		t.Fatalf("create image clue: %v", err)
	}
	if _, err := WorldCluePublish(users.worldID, clue.ID, users.owner, clue.PublishSeq); err != nil {
		t.Fatalf("publish image clue: %v", err)
	}
	link := "https://sealchat.example/#/" + users.worldID + "/" + channelID + "?clue=" + clue.ID
	job := &model.MessageExportJobModel{UserID: users.owner, ChannelID: channelID, Format: "html", IncludeOOC: true, IncludeArchived: true}
	messages := []*model.MessageModel{{StringPKBaseModel: model.StringPKBaseModel{ID: "clue-html-image-message", CreatedAt: time.Now()}, UserID: users.owner, Content: link, ICMode: "ic"}}
	payload := buildExportPayload(job, "线索图片导出频道", messages, nil, &exportExtraOptions{IncludeImages: true, IncludeDiceCommand: true})
	if payload == nil || len(payload.Messages) != 1 {
		t.Fatalf("unexpected image clue payload: %+v", payload)
	}
	newInlineImageEmbedder().inlinePayload(payload)
	html := payload.Messages[0].ContentHTML
	for _, expected := range []string{`class="export-world-clue`, `class="export-world-clue__media"`, "<img", `src="scasset:`} {
		if !strings.Contains(html, expected) {
			t.Fatalf("clue html missing %q: %s", expected, html)
		}
	}
	if len(payload.InlineAssets) == 0 {
		t.Fatalf("expected clue attachment in inline assets: %+v", payload.InlineAssets)
	}
	if strings.Contains(html, "图片来源") {
		t.Fatalf("attachment clue should render the image instead of a source-only fallback: %s", html)
	}

	withoutImages := buildExportPayload(job, "线索图片导出频道", messages, nil, &exportExtraOptions{IncludeImages: false, IncludeDiceCommand: true})
	if withoutImages == nil || len(withoutImages.Messages) != 1 {
		t.Fatalf("unexpected image-disabled clue payload: %+v", withoutImages)
	}
	if strings.Contains(withoutImages.Messages[0].ContentHTML, "export-world-clue__media") || strings.Contains(withoutImages.Messages[0].ContentHTML, "<img") {
		t.Fatalf("clue main media should be omitted when images are disabled: %s", withoutImages.Messages[0].ContentHTML)
	}
}

func TestWorldClueExportDocxRendering(t *testing.T) {
	payload := &ExportPayload{
		WithoutTimestamp: true,
		Messages: []ExportMessage{{
			SenderName:  "角色",
			SenderColor: "#123456",
			WorldClues: []WorldClueExportBlock{{
				Title:           "线索标题",
				Kind:            model.WorldClueKindIframe,
				Public:          buildRichDocumentForExport("公共正文", true),
				Private:         []WorldClueExportPrivateSection{{MemberName: "成员甲", Content: buildRichDocumentForExport("专属正文", true)}},
				IframeSourceURL: "https://example.com/clue",
			}},
		}},
	}
	data, err := (docxFormatter{}).Build(payload)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var documentXML string
	for _, file := range reader.File {
		if file.Name != "word/document.xml" {
			continue
		}
		body, openErr := file.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		value, readErr := io.ReadAll(body)
		_ = body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		documentXML = string(value)
		break
	}
	for _, expected := range []string{"线索", "线索标题", "公共正文", "专属信息", "成员甲", "专属正文", "https://example.com/clue"} {
		if !strings.Contains(documentXML, expected) {
			t.Fatalf("document.xml missing clue text %q: %s", expected, documentXML)
		}
	}
	if strings.Contains(documentXML, "<iframe") {
		t.Fatalf("iframe clue must not be embedded as HTML: %s", documentXML)
	}
}

func TestWorldClueExportDocxAttachmentEmbedsMedia(t *testing.T) {
	users := setupWorldClueTest(t)
	channelID := "clue-export-docx-image-channel"
	if err := model.GetDB().Create(&model.ChannelModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: channelID}, WorldID: users.worldID,
		Name: "线索图片 DOCX 频道", PermType: "public", Status: model.ChannelStatusActive,
	}).Error; err != nil {
		t.Fatalf("create clue export channel: %v", err)
	}
	const attachmentID = "clue-export-docx-image"
	createWorldClueExportImageAttachment(t, attachmentID, users.owner, users.worldID)
	clue, err := WorldClueCreate(users.worldID, users.owner, WorldClueCreateInput{
		Title: "DOCX 主媒体线索", Kind: model.WorldClueKindImage, ContentFormat: model.WorldClueContentPlain,
		Content: "DOCX 线索正文", ImageAttachmentID: attachmentID, DefaultAccess: model.WorldClueAccessView,
	})
	if err != nil {
		t.Fatalf("create image clue: %v", err)
	}
	if _, err := WorldCluePublish(users.worldID, clue.ID, users.owner, clue.PublishSeq); err != nil {
		t.Fatalf("publish image clue: %v", err)
	}
	link := "https://sealchat.example/#/" + users.worldID + "/" + channelID + "?clue=" + clue.ID
	job := &model.MessageExportJobModel{UserID: users.owner, ChannelID: channelID, Format: "docx", IncludeOOC: true, IncludeArchived: true}
	messages := []*model.MessageModel{{StringPKBaseModel: model.StringPKBaseModel{ID: "clue-docx-image-message", CreatedAt: time.Now()}, UserID: users.owner, Content: link, ICMode: "ic"}}
	payload := buildExportPayload(job, "线索图片 DOCX 频道", messages, nil, &exportExtraOptions{IncludeImages: true, IncludeDiceCommand: true})
	if payload == nil || len(payload.Messages) != 1 || len(payload.Messages[0].WorldClues) != 1 {
		t.Fatalf("unexpected DOCX image clue payload: %+v", payload)
	}
	data, err := (docxFormatter{}).Build(payload)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	foundMedia := false
	foundDocumentImageRef := false
	for _, file := range reader.File {
		if strings.HasPrefix(file.Name, "word/media/") {
			foundMedia = true
		}
		if file.Name != "word/document.xml" {
			continue
		}
		body, openErr := file.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		xml, readErr := io.ReadAll(body)
		_ = body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		foundDocumentImageRef = strings.Contains(string(xml), "r:embed")
	}
	if !foundMedia {
		t.Fatal("expected clue attachment bytes in word/media")
	}
	if !foundDocumentImageRef {
		t.Fatal("expected document.xml to reference clue media")
	}
}
