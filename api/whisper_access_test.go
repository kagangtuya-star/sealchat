package api

import "testing"

func TestCanUserReadAllWhispersInChannelSafeDefaultFalse(t *testing.T) {
	cases := []struct {
		name      string
		userID    string
		channelID string
	}{
		{name: "empty", userID: "", channelID: ""},
		{name: "unknown-channel", userID: "u1", channelID: "c1"},
		{name: "private-channel-id", userID: "u1", channelID: "private-channel-id-xxxxxxxxxxxxx"},
		{name: "bot", userID: "BOT:1000", channelID: "c1"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if canUserReadAllWhispersInChannel(tt.userID, tt.channelID) {
				t.Fatalf("expected false, got true for user=%q channel=%q", tt.userID, tt.channelID)
			}
		})
	}
}
