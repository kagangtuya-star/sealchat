package model

import (
	"reflect"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMessageAttachmentSyncAndBackfill(t *testing.T) {
	conn, err := gorm.Open(sqlite.Open("file:message-attachment-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := conn.DB()
	if err != nil {
		t.Fatalf("get sqlite db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := conn.AutoMigrate(&MessageModel{}, &MessageAttachmentModel{}, &MessageAttachmentBackfillState{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !conn.Migrator().HasIndex(&MessageAttachmentModel{}, "idx_message_attachment_lookup") {
		t.Fatal("missing message attachment lookup index")
	}

	message := MessageModel{
		StringPKBaseModel: StringPKBaseModel{ID: "message-new"},
		ChannelID:         "channel-a",
		Content:           `<img src="id:a" /><img src="id:b" /><img src="id:a" />`,
		DisplayOrder:      1,
	}
	if err := conn.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}

	idsByMessage, err := MessageImageAttachmentIDsByMessageIDs(conn, []string{message.ID})
	if err != nil {
		t.Fatalf("load associations: %v", err)
	}
	if want := []string{"a", "b"}; !reflect.DeepEqual(idsByMessage[message.ID], want) {
		t.Fatalf("associations = %#v, want %#v", idsByMessage[message.ID], want)
	}

	if err := ReplaceMessageImageAttachments(conn, message.ID, `<image src="id:c" /><image src="id:d" />`); err != nil {
		t.Fatalf("replace associations: %v", err)
	}
	idsByMessage, err = MessageImageAttachmentIDsByMessageIDs(conn, []string{message.ID})
	if err != nil {
		t.Fatalf("reload associations: %v", err)
	}
	if want := []string{"c", "d"}; !reflect.DeepEqual(idsByMessage[message.ID], want) {
		t.Fatalf("replaced associations = %#v, want %#v", idsByMessage[message.ID], want)
	}

	legacy := MessageModel{
		StringPKBaseModel: StringPKBaseModel{ID: "message-legacy"},
		ChannelID:         "channel-a",
		Content:           `legacy id:e and id:f and id:e`,
		DisplayOrder:      2,
	}
	if err := conn.Session(&gorm.Session{SkipHooks: true}).Create(&legacy).Error; err != nil {
		t.Fatalf("create legacy message: %v", err)
	}
	if err := backfillMessageAttachments(conn); err != nil {
		t.Fatalf("backfill associations: %v", err)
	}
	idsByMessage, err = MessageImageAttachmentIDsByMessageIDs(conn, []string{legacy.ID})
	if err != nil {
		t.Fatalf("load backfilled associations: %v", err)
	}
	if want := []string{"e", "f"}; !reflect.DeepEqual(idsByMessage[legacy.ID], want) {
		t.Fatalf("backfilled associations = %#v, want %#v", idsByMessage[legacy.ID], want)
	}
}
