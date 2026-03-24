package service

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"sealchat/model"
)

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
