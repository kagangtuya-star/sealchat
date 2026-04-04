package api

import (
	"strings"
	"testing"

	"sealchat/protocol"
)

func TestNormalizeCharacterRemarkContent(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantContent string
		wantClear   bool
		wantErr     string
	}{
		{
			name:        "trim surrounding whitespace",
			input:       "  前排侦察  ",
			wantContent: "前排侦察",
			wantClear:   false,
		},
		{
			name:        "blank content becomes clear",
			input:       "   \n\t  ",
			wantContent: "",
			wantClear:   true,
		},
		{
			name:    "over limit returns error",
			input:   strings.Repeat("角", 81),
			wantErr: "角色备注长度需在80个字符以内",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotContent, gotClear, err := normalizeCharacterRemarkContent(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("expected error %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotContent != tt.wantContent {
				t.Fatalf("expected content %q, got %q", tt.wantContent, gotContent)
			}
			if gotClear != tt.wantClear {
				t.Fatalf("expected clear=%v, got %v", tt.wantClear, gotClear)
			}
		})
	}
}

func TestCharacterRemarkCacheSnapshotAndRemove(t *testing.T) {
	cache := newCharacterRemarkCache()

	cache.upsert("channel-1", &protocol.CharacterRemarkEventPayload{
		IdentityID: "identity-1",
		UserID:     "user-1",
		Content:    "前排侦察",
		Action:     "update",
	})
	cache.upsert("channel-1", &protocol.CharacterRemarkEventPayload{
		IdentityID: "identity-2",
		UserID:     "user-2",
		Content:    "后排支援",
		Action:     "update",
	})

	items := cache.snapshot("channel-1")
	if len(items) != 2 {
		t.Fatalf("expected 2 items in snapshot, got %d", len(items))
	}

	items[0].Content = "tampered"
	itemsAgain := cache.snapshot("channel-1")
	if len(itemsAgain) != 2 {
		t.Fatalf("expected snapshot size remain 2, got %d", len(itemsAgain))
	}
	for _, item := range itemsAgain {
		if item.IdentityID == "identity-1" && item.Content != "前排侦察" {
			t.Fatalf("expected cached snapshot to remain unchanged, got %q", item.Content)
		}
	}

	cache.remove("channel-1", "identity-1")
	itemsAfterRemove := cache.snapshot("channel-1")
	if len(itemsAfterRemove) != 1 {
		t.Fatalf("expected 1 item after remove, got %d", len(itemsAfterRemove))
	}
	if itemsAfterRemove[0].IdentityID != "identity-2" {
		t.Fatalf("expected remaining identity to be identity-2, got %q", itemsAfterRemove[0].IdentityID)
	}
}
