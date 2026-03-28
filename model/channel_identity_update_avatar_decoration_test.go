package model

import (
	"sync"
	"testing"

	"sealchat/protocol"
	"sealchat/utils"
)

var channelIdentityUpdateTestDBOnce sync.Once

func initChannelIdentityUpdateTestEnv(t *testing.T) {
	t.Helper()
	channelIdentityUpdateTestDBOnce.Do(func() {
		DBInit(&utils.AppConfig{
			DSN: ":memory:",
			SQLite: utils.SQLiteConfig{
				EnableWAL:       false,
				TxLockImmediate: false,
				ReadConnections: 1,
				OptimizeOnInit:  false,
			},
		})
	})
}

func TestChannelIdentityUpdatePersistsAvatarDecoration(t *testing.T) {
	initChannelIdentityUpdateTestEnv(t)

	item := &ChannelIdentityModel{
		StringPKBaseModel: StringPKBaseModel{ID: "identity-test-update-decoration"},
		ChannelID:         "channel-1",
		UserID:            "user-1",
		DisplayName:       "角色A",
	}
	if err := GetDB().Create(item).Error; err != nil {
		t.Fatalf("create identity failed: %v", err)
	}

	decoration := &protocol.AvatarDecoration{
		Enabled:              true,
		ResourceAttachmentID: "id:decoration-1",
		Settings: protocol.AvatarDecorationSettings{
			Scale:   1.1,
			OffsetX: 8,
			OffsetY: -3,
			ZIndex:  1,
			Opacity: 1,
		},
	}

	if err := ChannelIdentityUpdate(item.ID, map[string]any{
		"avatar_decoration": decoration,
	}); err != nil {
		t.Fatalf("update avatar decoration failed: %v", err)
	}

	updated, err := ChannelIdentityGetByID(item.ID)
	if err != nil {
		t.Fatalf("reload identity failed: %v", err)
	}
	if len(updated.AvatarDecorations) != 1 {
		t.Fatalf("expected one avatar decoration to be persisted, got %d", len(updated.AvatarDecorations))
	}
	if updated.AvatarDecorations[0].ResourceAttachmentID != "id:decoration-1" {
		t.Fatalf("expected persisted decoration resource attachment id, got %q", updated.AvatarDecorations[0].ResourceAttachmentID)
	}
}
