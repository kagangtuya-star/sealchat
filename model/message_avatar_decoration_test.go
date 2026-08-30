package model

import (
	"encoding/json"
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
		SenderIdentityID:          "identity-1",
		SenderIdentityName:        "频道角色",
		SenderIdentityAvatarID:    "avatar-1",
		SenderIdentityDecorations: protocol.AvatarDecorationList{*decoration},
	}).ToProtocolType2(nil)

	if msg.Identity == nil {
		t.Fatalf("expected message identity to be present")
	}
	if len(msg.Identity.AvatarDecorations) != 1 {
		t.Fatalf("expected one message identity avatar decoration, got %d", len(msg.Identity.AvatarDecorations))
	}
	if msg.Identity.AvatarDecoration == nil {
		t.Fatalf("expected legacy message identity avatar decoration to be present")
	}
	if msg.Identity.AvatarDecoration.ResourceAttachmentID != "id:decoration-1" {
		t.Fatalf("expected message identity avatar decoration resource attachment id to be preserved, got %q", msg.Identity.AvatarDecoration.ResourceAttachmentID)
	}
}

func TestMessageModelJSONUsesIdentityAvatarDecorationField(t *testing.T) {
	decoration := protocol.AvatarDecoration{
		Enabled:              true,
		ResourceAttachmentID: "id:decoration-1",
	}
	raw, err := json.Marshal(&MessageModel{
		SenderIdentityDecorations: protocol.AvatarDecorationList{decoration},
	})
	if err != nil {
		t.Fatalf("marshal message failed: %v", err)
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal message payload failed: %v", err)
	}
	encoded, ok := payload["sender_identity_decoration"]
	if !ok {
		t.Fatalf("expected historical message payload field sender_identity_decoration, got %s", raw)
	}
	if _, ok := payload["sender_identity_decorations"]; ok {
		t.Fatalf("unexpected incompatible message payload field sender_identity_decorations: %s", raw)
	}

	var decorations protocol.AvatarDecorationList
	if err := json.Unmarshal(encoded, &decorations); err != nil {
		t.Fatalf("unmarshal message avatar decorations failed: %v", err)
	}
	if len(decorations) != 1 || decorations[0].ResourceAttachmentID != decoration.ResourceAttachmentID {
		t.Fatalf("expected complete avatar decorations in historical message payload, got %+v", decorations)
	}
}
