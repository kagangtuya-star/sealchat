package model

import (
	"testing"

	"sealchat/protocol"
)

func TestMessageModelToProtocolType2IncludesIdentityAvatarDecoration(t *testing.T) {
	decoration := &protocol.AvatarDecoration{
		Enabled:              true,
		ResourceAttachmentID: "id:decoration-1",
		Settings: protocol.AvatarDecorationSettings{
			Scale:   1.15,
			OffsetX: 6,
			OffsetY: -4,
			ZIndex:  1,
			Opacity: 0.9,
		},
	}
	msg := (&MessageModel{
		SenderIdentityID:        "identity-1",
		SenderIdentityName:      "频道角色",
		SenderIdentityAvatarID:  "avatar-1",
		SenderIdentityDecoration: decoration,
	}).ToProtocolType2(nil)

	if msg.Identity == nil {
		t.Fatalf("expected message identity to be present")
	}
	if msg.Identity.AvatarDecoration == nil {
		t.Fatalf("expected message identity avatar decoration to be present")
	}
	if msg.Identity.AvatarDecoration.ResourceAttachmentID != "id:decoration-1" {
		t.Fatalf("expected message identity avatar decoration resource attachment id to be preserved, got %q", msg.Identity.AvatarDecoration.ResourceAttachmentID)
	}
}
