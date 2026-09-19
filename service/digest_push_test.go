package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"sealchat/model"
)

func TestRecordDigestWindowMessageWritesAndIncrementsAllSupportedWindows(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()
	channelID := "digest-message-channel"
	userID := "digest-message-user"
	speakerKey := "digest-message-speaker"
	createdAt := time.UnixMilli(1700000000000)

	message := &model.MessageModel{
		StringPKBaseModel:  model.StringPKBaseModel{ID: "digest-message-1", CreatedAt: createdAt},
		UserID:             userID,
		SenderIdentityID:   speakerKey,
		SenderIdentityName: "测试发言者",
	}
	if err := RecordDigestWindowMessage(channelID, message); err != nil {
		t.Fatalf("RecordDigestWindowMessage first call failed: %v", err)
	}

	var visitors []model.DigestWindowVisitorModel
	if err := db.Where("scope_type = ? AND scope_id = ? AND user_id = ?", model.DigestScopeTypeChannel, channelID, userID).
		Find(&visitors).Error; err != nil {
		t.Fatalf("query digest visitors failed: %v", err)
	}
	assertDigestWindowSeconds(t, visitorWindowSeconds(visitors))

	var speakers []model.DigestWindowSpeakerModel
	if err := db.Where("scope_type = ? AND scope_id = ? AND speaker_key = ?", model.DigestScopeTypeChannel, channelID, speakerKey).
		Find(&speakers).Error; err != nil {
		t.Fatalf("query digest speakers failed: %v", err)
	}
	assertDigestWindowSeconds(t, speakerWindowSeconds(speakers))
	for _, speaker := range speakers {
		if speaker.MessageCount != 1 {
			t.Fatalf("first message count for window %d = %d, want 1", speaker.WindowSeconds, speaker.MessageCount)
		}
	}

	secondMessage := *message
	secondMessage.ID = "digest-message-2"
	if err := RecordDigestWindowMessage(channelID, &secondMessage); err != nil {
		t.Fatalf("RecordDigestWindowMessage second call failed: %v", err)
	}

	visitors = nil
	if err := db.Where("scope_type = ? AND scope_id = ? AND user_id = ?", model.DigestScopeTypeChannel, channelID, userID).
		Find(&visitors).Error; err != nil {
		t.Fatalf("query digest visitors after second message failed: %v", err)
	}
	assertDigestWindowSeconds(t, visitorWindowSeconds(visitors))

	speakers = nil
	if err := db.Where("scope_type = ? AND scope_id = ? AND speaker_key = ?", model.DigestScopeTypeChannel, channelID, speakerKey).
		Find(&speakers).Error; err != nil {
		t.Fatalf("query digest speakers after second message failed: %v", err)
	}
	assertDigestWindowSeconds(t, speakerWindowSeconds(speakers))
	for _, speaker := range speakers {
		if speaker.MessageCount != 2 {
			t.Fatalf("second message count for window %d = %d, want 2", speaker.WindowSeconds, speaker.MessageCount)
		}
	}
}

func TestRecordDigestWindowVisitKeepsOneRowPerSupportedWindow(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()
	channelID := "digest-visit-channel"
	userID := "digest-visit-user"

	if err := RecordDigestWindowVisit(channelID, userID); err != nil {
		t.Fatalf("RecordDigestWindowVisit first call failed: %v", err)
	}
	if err := RecordDigestWindowVisit(channelID, userID); err != nil {
		t.Fatalf("RecordDigestWindowVisit second call failed: %v", err)
	}

	var visitors []model.DigestWindowVisitorModel
	if err := db.Where("scope_type = ? AND scope_id = ? AND user_id = ?", model.DigestScopeTypeChannel, channelID, userID).
		Find(&visitors).Error; err != nil {
		t.Fatalf("query digest visitors failed: %v", err)
	}
	assertDigestWindowSeconds(t, visitorWindowSeconds(visitors))
}

func assertDigestWindowSeconds(t *testing.T, actual []int) {
	t.Helper()
	expected := DigestSupportedWindowSeconds()
	if len(actual) != len(expected) {
		t.Fatalf("digest window row count = %d, want %d (actual windows: %v)", len(actual), len(expected), actual)
	}
	seen := make(map[int]struct{}, len(actual))
	for _, windowSeconds := range actual {
		seen[windowSeconds] = struct{}{}
	}
	for _, windowSeconds := range expected {
		if _, ok := seen[windowSeconds]; !ok {
			t.Fatalf("missing digest window %d in %v", windowSeconds, actual)
		}
	}
}

func visitorWindowSeconds(records []model.DigestWindowVisitorModel) []int {
	windows := make([]int, 0, len(records))
	for _, record := range records {
		windows = append(windows, record.WindowSeconds)
	}
	return windows
}

func speakerWindowSeconds(records []model.DigestWindowSpeakerModel) []int {
	windows := make([]int, 0, len(records))
	for _, record := range records {
		windows = append(windows, record.WindowSeconds)
	}
	return windows
}

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
