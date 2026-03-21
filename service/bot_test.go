package service

import (
	"testing"

	"sealchat/model"
	"sealchat/utils"
)

func TestSelectedBotIdByChannelId_UsesPrivateBotCounterpart(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()

	userID := "user-bot-dm-" + utils.NewID()
	botID := "bot-bot-dm-" + utils.NewID()

	if err := db.Create(&model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: userID},
		Username:          "user_" + userID,
		Nickname:          "User",
		Password:          "pw",
		Salt:              "salt",
	}).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	if err := db.Create(&model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: botID},
		Username:          "bot_" + botID,
		Nickname:          "Bot",
		Password:          "pw",
		Salt:              "salt",
		IsBot:             true,
	}).Error; err != nil {
		t.Fatalf("create bot failed: %v", err)
	}

	channel, isNew := model.ChannelPrivateNew(userID, botID)
	if channel == nil || !isNew && channel.ID == "" {
		t.Fatal("create private channel failed")
	}

	selected, err := SelectedBotIdByChannelId(channel.ID)
	if err != nil {
		t.Fatalf("SelectedBotIdByChannelId returned error: %v", err)
	}
	if selected != botID {
		t.Fatalf("selected bot id = %q, want %q", selected, botID)
	}
	if !IsBotFeatureEffectivelyEnabled(channel) {
		t.Fatal("expected private bot channel to enable bot feature effectively")
	}
	if IsBuiltInDiceEffectivelyEnabled(channel) {
		t.Fatal("expected private bot channel to disable built-in dice effectively")
	}
}

func TestIsBuiltInDiceEffectivelyEnabled_RegularPrivateChannelKeepsBuiltInDice(t *testing.T) {
	initTestDB(t)
	db := model.GetDB()

	userIDA := "user-dm-a-" + utils.NewID()
	userIDB := "user-dm-b-" + utils.NewID()

	if err := db.Create(&model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: userIDA},
		Username:          "user_" + userIDA,
		Nickname:          "UserA",
		Password:          "pw",
		Salt:              "salt",
	}).Error; err != nil {
		t.Fatalf("create user a failed: %v", err)
	}
	if err := db.Create(&model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: userIDB},
		Username:          "user_" + userIDB,
		Nickname:          "UserB",
		Password:          "pw",
		Salt:              "salt",
	}).Error; err != nil {
		t.Fatalf("create user b failed: %v", err)
	}

	channel, isNew := model.ChannelPrivateNew(userIDA, userIDB)
	if channel == nil || (!isNew && channel.ID == "") {
		t.Fatal("create private channel failed")
	}
	channel.BuiltInDiceEnabled = true
	channel.BotFeatureEnabled = false

	if IsBotFeatureEffectivelyEnabled(channel) {
		t.Fatal("expected regular private channel to keep bot feature disabled")
	}
	if !IsBuiltInDiceEffectivelyEnabled(channel) {
		t.Fatal("expected regular private channel to keep built-in dice enabled")
	}
}
