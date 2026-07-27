package api

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"sealchat/model"
)

func TestQueryChannelImagesUsesStructuredAssociations(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:channel-images-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlite db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(
		&model.UserModel{},
		&model.MessageModel{},
		&model.MessageAttachmentModel{},
		&model.MessageWhisperRecipientModel{},
		&model.AttachmentModel{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	viewerID := "viewer"
	otherID := "other"
	for _, user := range []model.UserModel{
		{StringPKBaseModel: model.StringPKBaseModel{ID: viewerID}, Username: "viewer", Password: "x", Salt: "x", Nickname: "Viewer"},
		{StringPKBaseModel: model.StringPKBaseModel{ID: otherID}, Username: "other", Password: "x", Salt: "x", Nickname: "Other"},
	} {
		if err := db.Create(&user).Error; err != nil {
			t.Fatalf("create user: %v", err)
		}
	}

	baseTime := time.Unix(1_700_000_000, 0)
	createMessage := func(id, channelID, content string, order float64, deleted, revoked, whisper bool, whisperTo string) {
		t.Helper()
		message := model.MessageModel{
			StringPKBaseModel: model.StringPKBaseModel{ID: id, CreatedAt: baseTime.Add(time.Duration(order) * time.Second)},
			UserID:            otherID,
			ChannelID:         channelID,
			Content:           content,
			DisplayOrder:      order,
			SenderMemberName:  "Other",
			ICMode:            "ic",
			IsDeleted:         deleted,
			IsRevoked:         revoked,
			IsWhisper:         whisper,
			WhisperTo:         whisperTo,
		}
		if err := db.Create(&message).Error; err != nil {
			t.Fatalf("create message %s: %v", id, err)
		}
	}

	createMessage("m1", "channel-a", `<img src="id:a" />`, 100, false, false, false, "")
	createMessage("m2", "channel-a", `<img src="id:b" /><img src="id:c" /><img src="id:b" />`, 200, false, false, false, "")
	createMessage("m3", "channel-a", `<image src="id:b" /><image src="id:d" />`, 300, false, false, false, "")
	createMessage("other-channel", "channel-b", `<img src="id:x" />`, 400, false, false, false, "")
	createMessage("deleted", "channel-a", `<img src="id:e" />`, 500, true, false, false, "")
	createMessage("revoked", "channel-a", `<img src="id:f" />`, 600, false, true, false, "")
	createMessage("hidden-whisper", "channel-a", `<img src="id:g" />`, 700, false, false, true, "someone-else")
	createMessage("visible-whisper", "channel-a", `<img src="id:h" />`, 800, false, false, true, viewerID)

	deletedAttachment := model.AttachmentModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "d"},
		Filename:          "deleted.png",
		MimeType:          "image/png",
		UserID:            otherID,
		ChannelID:         "channel-a",
	}
	if err := db.Create(&deletedAttachment).Error; err != nil {
		t.Fatalf("create attachment: %v", err)
	}
	if err := db.Unscoped().Delete(&deletedAttachment).Error; err != nil {
		t.Fatalf("delete attachment: %v", err)
	}

	resp, err := queryChannelImages(db, viewerID, "channel-a", "all", "desc", 1, 10, false)
	if err != nil {
		t.Fatalf("query images: %v", err)
	}
	if resp.Total != 4 {
		t.Fatalf("total = %d, want 4 image messages", resp.Total)
	}
	wantIDs := []string{"h", "b", "d", "c", "a"}
	if len(resp.Items) != len(wantIDs) {
		t.Fatalf("item count = %d, want %d: %#v", len(resp.Items), len(wantIDs), resp.Items)
	}
	for i, want := range wantIDs {
		if resp.Items[i].AttachmentID != want {
			t.Fatalf("item %d attachment = %q, want %q", i, resp.Items[i].AttachmentID, want)
		}
	}

	asc, err := queryChannelImages(db, viewerID, "channel-a", "all", "asc", 1, 10, false)
	if err != nil {
		t.Fatalf("query asc images: %v", err)
	}
	wantAsc := []string{"a", "b", "c", "d", "h"}
	for i, want := range wantAsc {
		if asc.Items[i].AttachmentID != want {
			t.Fatalf("asc item %d attachment = %q, want %q", i, asc.Items[i].AttachmentID, want)
		}
	}

	page1, err := queryChannelImages(db, viewerID, "channel-a", "all", "desc", 1, 2, false)
	if err != nil {
		t.Fatalf("query page 1: %v", err)
	}
	page2, err := queryChannelImages(db, viewerID, "channel-a", "all", "desc", 2, 2, false)
	if err != nil {
		t.Fatalf("query page 2: %v", err)
	}
	if len(page1.Items) != 2 || page1.Items[0].AttachmentID != "h" || page1.Items[1].AttachmentID != "b" || !page1.HasMore {
		t.Fatalf("unexpected page 1: %#v", page1)
	}
	if len(page2.Items) != 2 || page2.Items[0].AttachmentID != "b" || page2.Items[1].AttachmentID != "c" || page2.HasMore {
		t.Fatalf("unexpected page 2: %#v", page2)
	}
}
