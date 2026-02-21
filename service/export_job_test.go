package service

import (
	"fmt"
	"slices"
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

	payload := buildExportPayload(job, "Export Whisper Channel", messages, nil)
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
