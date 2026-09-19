package model

import (
	"fmt"
	"testing"

	"sealchat/utils"
)

func initAppNotificationTestDB(t *testing.T) {
	t.Helper()
	DBInit(&utils.AppConfig{
		DSN: fmt.Sprintf("file:model-app-notification-%s?mode=memory&cache=shared", utils.NewID()),
		SQLite: utils.SQLiteConfig{
			EnableWAL:       false,
			TxLockImmediate: false,
			ReadConnections: 1,
			OptimizeOnInit:  false,
		},
	})
}

func appNotificationPreferenceUserIDs(preferences []AppNotificationPreferenceModel) map[string]int {
	userIDs := make(map[string]int, len(preferences))
	for _, preference := range preferences {
		userIDs[preference.UserID]++
	}
	return userIDs
}

func TestListEnabledExternalAppNotificationPreferences(t *testing.T) {
	initAppNotificationTestDB(t)

	preferences := []AppNotificationPreferenceModel{
		{UserID: "A", WorldWhitelistEnabled: true, ServerChanEnabled: true, ServerChanSendKey: "server-chan-key"},
		{UserID: "B", WorldWhitelistEnabled: true, BarkEnabled: true, BarkDeviceKey: "bark-device", BarkServerURL: "https://bark.example"},
		{UserID: "C", WorldWhitelistEnabled: true, MeowEnabled: true, MeowNickname: "meow-user"},
		{UserID: "D", WorldWhitelistEnabled: false, ServerChanEnabled: true, ServerChanSendKey: "server-chan-key"},
		{UserID: "E", WorldWhitelistEnabled: true, ServerChanEnabled: true},
		{UserID: "F", WorldWhitelistEnabled: true, BarkEnabled: true, BarkServerURL: "https://bark.example"},
		{UserID: "G", WorldWhitelistEnabled: true, MeowEnabled: false, MeowNickname: "meow-user"},
		{
			UserID: "H", WorldWhitelistEnabled: true,
			ServerChanEnabled: true, ServerChanSendKey: "server-chan-key",
			BarkEnabled: true, BarkDeviceKey: "bark-device", BarkServerURL: "https://bark.example",
			MeowEnabled: true, MeowNickname: "meow-user",
		},
	}
	if err := GetDB().Create(&preferences).Error; err != nil {
		t.Fatalf("seed app notification preferences: %v", err)
	}

	got, err := ListEnabledExternalAppNotificationPreferences()
	if err != nil {
		t.Fatalf("list enabled external app notification preferences: %v", err)
	}
	userIDs := appNotificationPreferenceUserIDs(got)
	for _, userID := range []string{"A", "B", "C", "H"} {
		if userIDs[userID] != 1 {
			t.Errorf("user %s returned %d times, want once", userID, userIDs[userID])
		}
	}
	for _, userID := range []string{"D", "E", "F", "G"} {
		if userIDs[userID] != 0 {
			t.Errorf("user %s unexpectedly returned %d times", userID, userIDs[userID])
		}
	}
	if len(got) != 4 {
		t.Fatalf("combined preference count = %d, want 4", len(got))
	}

	serverChan, err := ListServerChanAppNotificationPreferences()
	if err != nil {
		t.Fatalf("list ServerChan preferences: %v", err)
	}
	bark, err := ListBarkAppNotificationPreferences()
	if err != nil {
		t.Fatalf("list Bark preferences: %v", err)
	}
	meow, err := ListMeowAppNotificationPreferences()
	if err != nil {
		t.Fatalf("list Meow preferences: %v", err)
	}
	for name, result := range map[string][]AppNotificationPreferenceModel{
		"ServerChan": serverChan,
		"Bark":       bark,
		"Meow":       meow,
	} {
		if appNotificationPreferenceUserIDs(result)["H"] != 1 {
			t.Errorf("%s preferences should include H exactly once", name)
		}
	}
}
