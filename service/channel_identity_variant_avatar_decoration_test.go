package service

import (
	"testing"

	"sealchat/model"
	"sealchat/protocol"
)

func TestResolveChannelIdentityAppearanceKeepsIdentityAvatarDecoration(t *testing.T) {
	decoration := &protocol.AvatarDecoration{
		Enabled:              true,
		ResourceAttachmentID: "id:decoration-identity",
		Settings: protocol.AvatarDecorationSettings{
			Scale: 1.05,
		},
	}
	appearance := ResolveChannelIdentityAppearance(&model.ChannelIdentityModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "identity-1"},
		DisplayName:       "频道角色",
		Color:             "#123456",
		AvatarAttachmentID: "avatar-1",
		AvatarDecoration:  decoration,
	}, nil)

	if appearance == nil {
		t.Fatalf("expected resolved appearance")
	}
	if appearance.AvatarDecoration == nil {
		t.Fatalf("expected avatar decoration to be kept")
	}
	if appearance.AvatarDecoration.ResourceAttachmentID != "id:decoration-identity" {
		t.Fatalf("expected avatar decoration resource attachment id to be preserved, got %q", appearance.AvatarDecoration.ResourceAttachmentID)
	}
}
