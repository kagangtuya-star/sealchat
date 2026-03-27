package model

import "testing"

func TestChannelToProtocolTypeIncludesBotCommandPrefixes(t *testing.T) {
	channel := (&ChannelModel{
		StringPKBaseModel: StringPKBaseModel{ID: "channel-1"},
		WorldID:           "world-1",
		Name:              "测试频道",
		DefaultDiceExpr:   "d20",
	}).ToProtocolType()

	if channel == nil {
		t.Fatalf("expected protocol channel")
	}
	if len(channel.BotCommandPrefixes) == 0 {
		t.Fatalf("expected bot command prefixes to be exposed")
	}
	if channel.BotCommandPrefixes[0] != "." {
		t.Fatalf("expected first bot command prefix to be '.', got %#v", channel.BotCommandPrefixes)
	}
}
