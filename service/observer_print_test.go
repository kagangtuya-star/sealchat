package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"sealchat/model"
)

func TestLoadObserverPrintableMessagesFiltersScopeArchivedAndWhisper(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	worldID := "world-ob-print-" + suffix
	channelID := "channel-ob-print-" + suffix
	userID := "user-ob-print-" + suffix
	now := time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC)

	if err := db.Create(&model.WorldModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: worldID, CreatedAt: now, UpdatedAt: now},
		Name:              "OB 打印测试世界",
		Status:            "active",
		Visibility:        model.WorldVisibilityPublic,
	}).Error; err != nil {
		t.Fatalf("create world failed: %v", err)
	}

	if err := db.Create(&model.ChannelModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: channelID, CreatedAt: now, UpdatedAt: now},
		WorldID:           worldID,
		Name:              "战报频道",
		PermType:          "public",
		Status:            model.ChannelStatusActive,
	}).Error; err != nil {
		t.Fatalf("create channel failed: %v", err)
	}

	if err := db.Create(&model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: userID, CreatedAt: now, UpdatedAt: now},
		Username:          "observer_print_user_" + suffix,
		Password:          "test-password",
		Salt:              "test-salt",
		Nickname:          "记录员",
	}).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}

	messages := []model.MessageModel{
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: "msg-ob-ic-" + suffix, CreatedAt: now, UpdatedAt: now},
			ChannelID:         channelID,
			UserID:            userID,
			Content:           "场内推进",
			DisplayOrder:      float64(now.UnixMilli()),
			ICMode:            "ic",
		},
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: "msg-ob-ooc-" + suffix, CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second)},
			ChannelID:         channelID,
			UserID:            userID,
			Content:           "场外讨论",
			DisplayOrder:      float64(now.Add(time.Second).UnixMilli()),
			ICMode:            "ooc",
		},
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: "msg-ob-arch-" + suffix, CreatedAt: now.Add(2 * time.Second), UpdatedAt: now.Add(2 * time.Second)},
			ChannelID:         channelID,
			UserID:            userID,
			Content:           "归档记录",
			DisplayOrder:      float64(now.Add(2 * time.Second).UnixMilli()),
			ICMode:            "ic",
			IsArchived:        true,
		},
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: "msg-ob-whisper-" + suffix, CreatedAt: now.Add(3 * time.Second), UpdatedAt: now.Add(3 * time.Second)},
			ChannelID:         channelID,
			UserID:            userID,
			Content:           "悄悄话内容",
			DisplayOrder:      float64(now.Add(3 * time.Second).UnixMilli()),
			ICMode:            "ic",
			IsWhisper:         true,
		},
	}

	for i := range messages {
		if err := db.Create(&messages[i]).Error; err != nil {
			t.Fatalf("create message %s failed: %v", messages[i].ID, err)
		}
	}

	allMessages, err := LoadObserverPrintableMessages(channelID, ObserverPrintOptions{MessageScope: 0, ShowArchived: true, ShowTimestamp: true, ShowColorCode: false})
	if err != nil {
		t.Fatalf("LoadObserverPrintableMessages(all) failed: %v", err)
	}
	if len(allMessages) != 3 {
		t.Fatalf("expected 3 visible messages, got %d", len(allMessages))
	}

	oocMessages, err := LoadObserverPrintableMessages(channelID, ObserverPrintOptions{MessageScope: 1, ShowArchived: true, ShowTimestamp: true, ShowColorCode: false})
	if err != nil {
		t.Fatalf("LoadObserverPrintableMessages(ooc) failed: %v", err)
	}
	if len(oocMessages) != 1 || oocMessages[0].ICMode != "ooc" {
		t.Fatalf("expected only one ooc message, got %+v", oocMessages)
	}

	icMessages, err := LoadObserverPrintableMessages(channelID, ObserverPrintOptions{MessageScope: 2, ShowArchived: false, ShowTimestamp: true, ShowColorCode: false})
	if err != nil {
		t.Fatalf("LoadObserverPrintableMessages(ic) failed: %v", err)
	}
	if len(icMessages) != 1 || icMessages[0].ID != "msg-ob-ic-"+suffix {
		t.Fatalf("expected only non-archived ic message, got %+v", icMessages)
	}
}

func TestBuildObserverPrintPageDataAndRenderHTML(t *testing.T) {
	initTestDB(t)
	now := time.Date(2024, time.January, 2, 3, 4, 5, 0, time.UTC)
	world := &model.WorldModel{StringPKBaseModel: model.StringPKBaseModel{ID: "world-render"}, Name: "战役世界"}
	channel := &model.ChannelModel{StringPKBaseModel: model.StringPKBaseModel{ID: "channel-render"}, Name: "战报频道"}
	user := &model.UserModel{StringPKBaseModel: model.StringPKBaseModel{ID: "user-render"}, Nickname: "旁白"}
	messages := []*model.MessageModel{
		{
			StringPKBaseModel:   model.StringPKBaseModel{ID: "msg-render", CreatedAt: now, UpdatedAt: now},
			ChannelID:           channel.ID,
			UserID:              user.ID,
			User:                user,
			Content:             "场外记录",
			DisplayOrder:        float64(now.UnixMilli()),
			ICMode:              "ooc",
			SenderIdentityColor: "#55ccaa",
		},
	}

	data := BuildObserverPrintPageData(world, channel, "test-slug", messages, ObserverPrintOptions{MessageScope: 1, ShowArchived: true, ShowTimestamp: false, ShowColorCode: true})
	if data.Count != 1 {
		t.Fatalf("expected count=1, got %d", data.Count)
	}
	if len(data.Messages) != 1 {
		t.Fatalf("expected 1 rendered message, got %d", len(data.Messages))
	}
	if !strings.Contains(data.Messages[0].Body, "场外记录") {
		t.Fatalf("expected body to contain message text, got %q", data.Messages[0].Body)
	}
	html, err := RenderObserverPrintHTML(data)
	if err != nil {
		t.Fatalf("RenderObserverPrintHTML failed: %v", err)
	}
	body := string(html)
	if !strings.Contains(body, "战役世界 / 战报频道") {
		t.Fatalf("expected html to contain world/channel title, got %q", body)
	}
	if !strings.Contains(body, "test-slug") {
		t.Fatalf("expected html to contain slug, got %q", body)
	}
	if !strings.Contains(body, "场外记录") {
		t.Fatalf("expected html to contain message body, got %q", body)
	}
	if strings.Contains(body, now.Local().Format("2006-01-02 15:04:05")) {
		t.Fatalf("expected html to hide timestamp, got %q", body)
	}
	if !strings.Contains(body, "#55ccaa") {
		t.Fatalf("expected html to contain sender color code, got %q", body)
	}
}
