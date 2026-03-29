package api

import (
	"testing"

	"sealchat/protocol"
)

func TestBuildChannelIdentityRefreshRecipientsDeduplicatesUsers(t *testing.T) {
	recipients := buildChannelIdentityRefreshRecipients("target-user", "operator-user")
	if len(recipients) != 2 {
		t.Fatalf("expected two recipients, got %d (%v)", len(recipients), recipients)
	}
	if recipients[0] != "operator-user" {
		t.Fatalf("expected operator recipient first, got %q", recipients[0])
	}
	if recipients[1] != "target-user" {
		t.Fatalf("expected target recipient second, got %q", recipients[1])
	}

	sameRecipients := buildChannelIdentityRefreshRecipients("same-user", "same-user")
	if len(sameRecipients) != 1 || sameRecipients[0] != "same-user" {
		t.Fatalf("expected deduplicated recipients for same user, got %v", sameRecipients)
	}
}

func TestBuildChannelIdentityRefreshEventIncludesScopeMetadata(t *testing.T) {
	event := buildChannelIdentityRefreshEvent(channelIdentityRefreshPayload{
		ChannelID:      "channel-1",
		TargetUserID:   "target-user",
		OperatorUserID: "operator-user",
		Reason:         "identity-update",
	})
	if event == nil {
		t.Fatal("expected event")
	}
	if event.Type != protocol.EventChannelIdentitiesUpdated {
		t.Fatalf("expected event type %q, got %q", protocol.EventChannelIdentitiesUpdated, event.Type)
	}
	if event.Channel == nil || event.Channel.ID != "channel-1" {
		t.Fatalf("expected channel metadata, got %+v", event.Channel)
	}
	if event.Argv == nil || event.Argv.Options == nil {
		t.Fatal("expected argv options")
	}
	if got := event.Argv.Options["targetUserId"]; got != "target-user" {
		t.Fatalf("expected targetUserId in options, got %#v", got)
	}
	if got := event.Argv.Options["operatorUserId"]; got != "operator-user" {
		t.Fatalf("expected operatorUserId in options, got %#v", got)
	}
	if got := event.Argv.Options["reason"]; got != "identity-update" {
		t.Fatalf("expected reason in options, got %#v", got)
	}
	if got := event.Argv.Options["forceReload"]; got != true {
		t.Fatalf("expected forceReload=true, got %#v", got)
	}
}
