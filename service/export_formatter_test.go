package service

import (
	"slices"
	"strings"
	"testing"
	"time"

	"sealchat/model"
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
