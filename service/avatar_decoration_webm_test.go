package service

import (
	"testing"

	"sealchat/model"
	"sealchat/protocol"
)

func TestNormalizeAvatarDecorationAllowsWebMWithoutFallback(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()

	userID := "user-avatar-decoration-webm"
	attachmentID := "attachment-avatar-decoration-webm"
	if err := db.Create(&model.AttachmentModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: attachmentID},
		UserID:            userID,
		Filename:          "decoration.webm",
		MimeType:          "video/webm",
		Size:              1024,
	}).Error; err != nil {
		t.Fatalf("create webm attachment failed: %v", err)
	}

	decoration, err := NormalizeAvatarDecoration(userID, &protocol.AvatarDecoration{
		Enabled:              true,
		ResourceAttachmentID: "id:" + attachmentID,
		Settings: protocol.AvatarDecorationSettings{
			Scale:        1,
			ZIndex:       1,
			Opacity:      1,
			PlaybackRate: 1.5,
		},
	})
	if err != nil {
		t.Fatalf("expected webm avatar decoration to be accepted, got error: %v", err)
	}
	if decoration == nil {
		t.Fatalf("expected normalized avatar decoration")
	}
	if decoration.ResourceAttachmentID != "id:"+attachmentID {
		t.Fatalf("expected normalized resource attachment id to be preserved, got %q", decoration.ResourceAttachmentID)
	}
	if decoration.Settings.PlaybackRate != 1.5 {
		t.Fatalf("expected playback rate to be preserved, got %v", decoration.Settings.PlaybackRate)
	}
}

func TestNormalizeAvatarDecorationRejectsInvalidPlaybackRate(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()

	userID := "user-avatar-decoration-webm-invalid-rate"
	attachmentID := "attachment-avatar-decoration-webm-invalid-rate"
	if err := db.Create(&model.AttachmentModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: attachmentID},
		UserID:            userID,
		Filename:          "decoration.webm",
		MimeType:          "video/webm",
		Size:              1024,
	}).Error; err != nil {
		t.Fatalf("create webm attachment failed: %v", err)
	}

	_, err := NormalizeAvatarDecoration(userID, &protocol.AvatarDecoration{
		Enabled:              true,
		ResourceAttachmentID: "id:" + attachmentID,
		Settings: protocol.AvatarDecorationSettings{
			Scale:        1,
			ZIndex:       1,
			Opacity:      1,
			PlaybackRate: 3,
		},
	})
	if err == nil {
		t.Fatalf("expected invalid playback rate to be rejected")
	}
}
