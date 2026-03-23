package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"sealchat/model"
)

func TestBuildWorldDigestPreviewMergesSelectedChannels(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()
	suffix := strings.ReplaceAll(time.Now().Format("150405.000000"), ".", "")

	worldID := "digest-world-" + suffix
	channelA := "digest-ch-a-" + suffix
	channelB := "digest-ch-b-" + suffix
	channelC := "digest-ch-c-" + suffix

	if err := db.Create(&model.WorldModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: worldID},
		Name:              "测试世界",
		Status:            "active",
	}).Error; err != nil {
		t.Fatalf("create world failed: %v", err)
	}

	channels := []model.ChannelModel{
		{StringPKBaseModel: model.StringPKBaseModel{ID: channelA}, WorldID: worldID, Name: "频道A", PermType: "public", Status: "active"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: channelB}, WorldID: worldID, Name: "频道B", PermType: "public", Status: "active"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: channelC}, WorldID: worldID, Name: "频道C", PermType: "public", Status: "active"},
	}
	for i := range channels {
		if err := db.Create(&channels[i]).Error; err != nil {
			t.Fatalf("create channel failed: %v", err)
		}
	}

	members := []model.MemberModel{
		{StringPKBaseModel: model.StringPKBaseModel{ID: "member-a1-" + suffix}, ChannelID: channelA, UserID: "user-1-" + suffix, Nickname: "成员1"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: "member-a2-" + suffix}, ChannelID: channelA, UserID: "user-2-" + suffix, Nickname: "成员2"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: "member-b1-" + suffix}, ChannelID: channelB, UserID: "user-1-" + suffix, Nickname: "成员1"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: "member-b3-" + suffix}, ChannelID: channelB, UserID: "user-3-" + suffix, Nickname: "成员3"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: "member-c4-" + suffix}, ChannelID: channelC, UserID: "user-4-" + suffix, Nickname: "成员4"},
	}
	for i := range members {
		if err := db.Create(&members[i]).Error; err != nil {
			t.Fatalf("create member failed: %v", err)
		}
	}

	windowStart, windowEnd := AlignDigestWindow(1700000000000, 3600)

	visitors := []struct {
		channelID string
		userID    string
	}{
		{channelA, "user-1-" + suffix},
		{channelA, "user-2-" + suffix},
		{channelB, "user-1-" + suffix},
		{channelB, "user-3-" + suffix},
		{channelC, "user-4-" + suffix},
	}
	for _, visitor := range visitors {
		if err := model.DigestWindowVisitorUpsert(model.DigestScopeTypeChannel, visitor.channelID, 3600, windowStart, windowEnd, visitor.userID); err != nil {
			t.Fatalf("upsert visitor failed: %v", err)
		}
	}

	speakerEvents := []struct {
		channelID string
		key       string
		name      string
		at        int64
	}{
		{channelA, "speaker-1-" + suffix, "发言者甲", windowStart + 1000},
		{channelA, "speaker-1-" + suffix, "发言者甲", windowStart + 2000},
		{channelA, "speaker-2-" + suffix, "发言者乙", windowStart + 3000},
		{channelB, "speaker-3-" + suffix, "发言者丙", windowStart + 4000},
		{channelB, "speaker-3-" + suffix, "发言者丙", windowStart + 5000},
		{channelC, "speaker-4-" + suffix, "发言者丁", windowStart + 6000},
	}
	for _, event := range speakerEvents {
		if err := model.DigestWindowSpeakerUpsert(model.DigestScopeTypeChannel, event.channelID, 3600, windowStart, windowEnd, event.key, event.name, event.at); err != nil {
			t.Fatalf("upsert speaker failed: %v", err)
		}
	}

	selectedIDs, _ := json.Marshal([]string{channelA, channelB})
	rule := NewDefaultDigestRule(model.DigestScopeTypeWorld, worldID)
	rule.Enabled = true
	rule.WindowSeconds = 3600
	rule.ActiveUserThresholdMode = model.DigestThresholdModeChannelMemberCount
	rule.SelectedChannelIDsJSON = string(selectedIDs)

	preview, err := BuildDigestPreviewForRule(rule, windowStart)
	if err != nil {
		t.Fatalf("BuildDigestPreviewForRule failed: %v", err)
	}

	if preview.ActiveUserCount != 3 {
		t.Fatalf("activeUserCount=%d want 3", preview.ActiveUserCount)
	}
	if preview.ThresholdValue != 3 {
		t.Fatalf("thresholdValue=%d want 3", preview.ThresholdValue)
	}
	if !preview.ThresholdSatisfied {
		t.Fatalf("expected threshold satisfied")
	}
	if preview.MessageCount != 5 {
		t.Fatalf("messageCount=%d want 5", preview.MessageCount)
	}
	if preview.ChannelCount != 2 {
		t.Fatalf("channelCount=%d want 2", preview.ChannelCount)
	}
	if len(preview.Channels) != 2 {
		t.Fatalf("len(preview.Channels)=%d want 2", len(preview.Channels))
	}
	if strings.Contains(preview.RenderedText, "频道C") {
		t.Fatalf("renderedText should not include unselected channel: %q", preview.RenderedText)
	}
	if !strings.Contains(preview.RenderedText, "频道A") || !strings.Contains(preview.RenderedText, "频道B") {
		t.Fatalf("renderedText should include selected channels, got %q", preview.RenderedText)
	}
	if !strings.Contains(preview.RenderedJSON, "\"text\"") {
		t.Fatalf("renderedJson should include text field, got %q", preview.RenderedJSON)
	}
}
