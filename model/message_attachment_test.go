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

	if err := conn.AutoMigrate(&MessageModel{}, &MessageAttachmentModel{}, &ChannelMessageAttachmentBackfillState{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !conn.Migrator().HasIndex(&MessageAttachmentModel{}, "idx_message_attachment_lookup") {
		t.Fatal("missing message attachment lookup index")
	}

	attachmentDeleteCount := 0
	const deleteCallbackName = "test:count_message_attachment_deletes"
	if err := conn.Callback().Delete().Before("gorm:delete").Register(deleteCallbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "message_attachments" {
			attachmentDeleteCount++
		}
	}); err != nil {
		t.Fatalf("register delete callback: %v", err)
	}
	t.Cleanup(func() { _ = conn.Callback().Delete().Remove(deleteCallbackName) })

	plainMessage := MessageModel{
		StringPKBaseModel: StringPKBaseModel{ID: "message-plain"},
		ChannelID:         "channel-a",
		Content:           "plain text message",
		DisplayOrder:      1,
	}
	if err := conn.Create(&plainMessage).Error; err != nil {
		t.Fatalf("create plain message: %v", err)
	}
	idsByMessage, err := MessageImageAttachmentIDsByMessageIDs(conn, []string{plainMessage.ID})
	if err != nil {
		t.Fatalf("load plain message associations: %v", err)
	}
	if len(idsByMessage[plainMessage.ID]) != 0 {
		t.Fatalf("plain message associations = %#v, want none", idsByMessage[plainMessage.ID])
	}
	if attachmentDeleteCount != 0 {
		t.Fatalf("plain message attachment delete count = %d, want 0", attachmentDeleteCount)
	}

	message := MessageModel{
		StringPKBaseModel: StringPKBaseModel{ID: "message-new"},
		ChannelID:         "channel-a",
		Content:           `<img src="id:a" /><image src="id:b" /><img src="id:a" />`,
		DisplayOrder:      2,
	}
	if err := conn.Create(&message).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}

	idsByMessage, err = MessageImageAttachmentIDsByMessageIDs(conn, []string{message.ID})
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

	if err := ReplaceMessageImageAttachments(conn, message.ID, "plain text"); err != nil {
		t.Fatalf("clear associations: %v", err)
	}
	idsByMessage, err = MessageImageAttachmentIDsByMessageIDs(conn, []string{message.ID})
	if err != nil {
		t.Fatalf("reload cleared associations: %v", err)
	}
	if len(idsByMessage[message.ID]) != 0 {
		t.Fatalf("cleared associations = %#v, want none", idsByMessage[message.ID])
	}

	legacy := MessageModel{
		StringPKBaseModel: StringPKBaseModel{ID: "message-legacy"},
		ChannelID:         "channel-a",
		Content:           `legacy id:e and id:f and id:e`,
		DisplayOrder:      3,
	}
	if err := conn.Session(&gorm.Session{SkipHooks: true}).Create(&legacy).Error; err != nil {
		t.Fatalf("create legacy message: %v", err)
	}
	if err := BackfillMessageImageAttachmentsForChannel(conn, legacy.ChannelID); err != nil {
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
