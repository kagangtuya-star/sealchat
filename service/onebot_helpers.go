package service

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/utils"
)

func EnsureOneBotExternalUser(profile *model.BotProfileModel, remoteID, nickname string) (*model.UserModel, error) {
	if profile == nil {
		return nil, errors.New("profile is nil")
	}
	remoteID = strings.TrimSpace(remoteID)
	if remoteID == "" {
		remoteID = utils.NewID()
	}
	userID := fmt.Sprintf("ob-%s-%s", profile.ID, remoteID)
	if user, err := model.UserGetEx(userID); err == nil {
		if nickname != "" && strings.TrimSpace(user.Nickname) == "" {
			user.Nickname = nickname
			user.SaveInfo()
		}
		return user, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	username := fmt.Sprintf("ob_%s_%s", profile.ID, remoteID)
	user := &model.UserModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: userID},
		Username:          username,
		Nickname:          nickname,
		Password:          utils.NewID(),
		Salt:              "ONEBOT_REMOTE",
		IsBot:             false,
	}
	if err := model.GetDB().Create(user).Error; err != nil {
		return nil, err
	}
	return user, nil
}

func EnsureOneBotChannelAccess(channelID string, user *model.UserModel, displayName string) error {
	if user == nil {
		return errors.New("user is nil")
	}
	if _, err := model.MemberGetByUserIDAndChannelID(user.ID, channelID, displayName); err != nil {
		return err
	}
	roleID := fmt.Sprintf("ch-%s-%s", channelID, "member")
	return ensureUserRoleMapping(user.ID, roleID)
}

func EnsureOneBotPrivateChannel(botUserID, remoteUserID string) (string, error) {
	if botUserID == "" || remoteUserID == "" {
		return "", errors.New("invalid private channel params")
	}
	ch, _ := model.ChannelPrivateNew(botUserID, remoteUserID)
	if ch == nil {
		return "", errors.New("failed to create private channel")
	}
	return ch.ID, nil
}
