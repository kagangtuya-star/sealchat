package protocol

import (
	"encoding/json"
	"testing"
)

func TestAvatarDecorationListUnmarshalJSONSupportsLegacySingleObject(t *testing.T) {
	var list AvatarDecorationList
	if err := json.Unmarshal([]byte(`{
		"enabled": true,
		"id": "decoration-1",
		"resourceAttachmentId": "id:attachment-1",
		"settings": {
			"scale": 1.2
		}
	}`), &list); err != nil {
		t.Fatalf("unmarshal legacy single object failed: %v", err)
	}

	if len(list) != 1 {
		t.Fatalf("expected single legacy decoration to become one-item list, got %d", len(list))
	}
	if list[0].ID != "decoration-1" {
		t.Fatalf("expected legacy decoration id to be preserved, got %q", list[0].ID)
	}
	if list[0].ResourceAttachmentID != "id:attachment-1" {
		t.Fatalf("expected legacy decoration resource attachment id to be preserved, got %q", list[0].ResourceAttachmentID)
	}
}
