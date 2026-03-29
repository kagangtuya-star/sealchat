package api

import (
	"encoding/json"
	"strconv"
	"testing"

	"sealchat/model"
	"sealchat/service"
)

func TestBuildAdminBotTokenListIncludesOneBotSelfID(t *testing.T) {
	initOneBotAPITestEnv(t)

	botUser, _ := createOneBotTestBot(t, "admin-bot-onebot-id", model.BotKindManual)
	items, err := buildAdminBotTokenList("", "manual")
	if err != nil {
		t.Fatalf("buildAdminBotTokenList failed: %v", err)
	}

	var matched any
	for _, item := range items {
		if item.ID == botUser.ID {
			matched = item
			break
		}
	}
	if matched == nil {
		t.Fatalf("manual bot %s not found in list", botUser.ID)
	}

	raw, err := json.Marshal(matched)
	if err != nil {
		t.Fatalf("marshal item failed: %v", err)
	}
	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}
	gotRaw, ok := payload["oneBotSelfId"]
	if !ok {
		t.Fatal("expected oneBotSelfId in payload")
	}

	expected, err := service.GetOrCreateOneBotID(service.OneBotEntityBotUser, botUser.ID)
	if err != nil {
		t.Fatalf("GetOrCreateOneBotID failed: %v", err)
	}

	var got int64
	switch v := gotRaw.(type) {
	case float64:
		got = int64(v)
	case string:
		got, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			t.Fatalf("parse oneBotSelfId failed: %v", err)
		}
	default:
		t.Fatalf("unexpected oneBotSelfId type: %T", gotRaw)
	}
	if got != expected {
		t.Fatalf("oneBotSelfId = %d, want %d", got, expected)
	}
	if digits := len(strconv.FormatInt(got, 10)); digits != 10 {
		t.Fatalf("oneBotSelfId digits = %d, want 10", digits)
	}
}
