package service

import (
	"testing"

	"sealchat/utils"
)

func TestOneBotIDMappingGetOrCreateAndResolve(t *testing.T) {
	initTestDB(t)

	entityID := "ob-user-" + utils.NewID()

	firstID, err := GetOrCreateOneBotID(OneBotEntityUser, entityID)
	if err != nil {
		t.Fatalf("GetOrCreateOneBotID first failed: %v", err)
	}
	if firstID <= 0 {
		t.Fatalf("expected positive numeric id, got %d", firstID)
	}

	secondID, err := GetOrCreateOneBotID(OneBotEntityUser, entityID)
	if err != nil {
		t.Fatalf("GetOrCreateOneBotID second failed: %v", err)
	}
	if firstID != secondID {
		t.Fatalf("expected stable numeric id, got %d and %d", firstID, secondID)
	}

	channelID, err := GetOrCreateOneBotID(OneBotEntityChannel, entityID)
	if err != nil {
		t.Fatalf("GetOrCreateOneBotID for channel failed: %v", err)
	}
	if channelID <= 0 {
		t.Fatalf("expected positive channel numeric id, got %d", channelID)
	}
	if channelID == firstID {
		t.Fatalf("expected different entity types to use different numeric ids, both got %d", channelID)
	}

	resolvedEntityID, err := ResolveInternalID(OneBotEntityUser, firstID)
	if err != nil {
		t.Fatalf("ResolveInternalID failed: %v", err)
	}
	if resolvedEntityID != entityID {
		t.Fatalf("ResolveInternalID returned %q, want %q", resolvedEntityID, entityID)
	}

	if _, err := ResolveInternalID(OneBotEntityUser, channelID); err == nil {
		t.Fatal("expected type mismatch lookup to fail")
	}

	if _, err := ResolveInternalID(OneBotEntityMessage, 999999999); err == nil {
		t.Fatal("expected missing numeric id lookup to fail")
	}
}
