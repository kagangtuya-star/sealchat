package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
	onebot "sealchat/service/onebot"
	//"sealchat/utils"
)

type BotProfileView struct {
	*model.BotProfileModel
	Runtime onebot.RuntimeState `json:"runtime"`
}

func buildBotProfileView(profile *model.BotProfileModel) *BotProfileView {
	state := onebot.ManagerInstance().GetStatus(profile.ID)
	if profile.Enabled == false && state.Status != onebot.RuntimeStatusConnected {
		state.Status = onebot.RuntimeStatusDisabled
	}
	return &BotProfileView{
		BotProfileModel: profile,
		Runtime:         state,
	}
}

func ListBotProfiles(ctx context.Context) ([]*BotProfileView, error) {
	items, err := model.BotProfileList()
	if err != nil {
		return nil, err
	}
	views := make([]*BotProfileView, 0, len(items))
	for _, item := range items {
		views = append(views, buildBotProfileView(item))
	}
	return views, nil
}

func GetBotProfileView(ctx context.Context, botID string) (*BotProfileView, error) {
	item, err := model.BotProfileGet(botID)
	if err != nil {
		return nil, err
	}
	return buildBotProfileView(item), nil
}

func UpsertBotProfile(ctx context.Context, profile *model.BotProfileModel) (*BotProfileView, error) {
	if profile == nil {
		return nil, errors.New("profile payload is nil")
	}
	if strings.TrimSpace(profile.Name) == "" {
		return nil, errors.New("机器人名称不能为空")
	}
	if err := ensureBotProfileUser(profile); err != nil {
		return nil, err
	}
	if err := model.BotProfileSave(profile); err != nil {
		return nil, err
	}
	onebot.ManagerInstance().NotifyProfileChanged(profile.ID)
	onebot.TriggerReverseRefresh()
	return buildBotProfileView(profile), nil
}

func ensureBotProfileUser(profile *model.BotProfileModel) error {
	db := model.GetDB()
	if profile.UserID == "" {
		for i := 0; i < 5; i++ {
			numericAccount, err := generateNumericAccount(16)
			if err != nil {
				numericAccount = fmt.Sprintf("%d", time.Now().UnixMilli())
			}
			user := &model.UserModel{
				StringPKBaseModel: model.StringPKBaseModel{ID: numericAccount},
				Username:          numericAccount,
				Nickname:          profile.Name,
				Avatar:            profile.AvatarURL,
				Password:          "",
				Salt:              "ONEBOT",
				IsBot:             true,
			}
			if err := db.Create(user).Error; err != nil {
				if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
					continue
				}
				return err
			}
			profile.UserID = user.ID
			return nil
		}
		return errors.New("无法分配机器人账号，请稍后重试")
	}
	updates := map[string]interface{}{
		"nickname": profile.Name,
		"avatar":   profile.AvatarURL,
	}
	return db.Model(&model.UserModel{}).Where("id = ?", profile.UserID).Updates(updates).Error
}

func DeleteBotProfile(ctx context.Context, botID string) error {
	if strings.TrimSpace(botID) == "" {
		return errors.New("botID required")
	}
	if err := model.BotProfileDelete(botID); err != nil {
		return err
	}
	onebot.ManagerInstance().NotifyProfileChanged(botID)
	onebot.TriggerReverseRefresh()
	return nil
}

type ChannelBotSettings struct {
	ChannelID       string `json:"channelId"`
	BindingID       string `json:"bindingId,omitempty"`
	BotID           string `json:"botId,omitempty"`
	RemoteChannelID string `json:"remoteChannelId,omitempty"`
	RemoteGroupID   string `json:"remoteGroupId,omitempty"`
	RemoteNumericID string `json:"remoteNumericId,omitempty"`
	Enabled         bool   `json:"enabled"`
	UpdatedAt       int64  `json:"updatedAt,omitempty"`
}

type ChannelBotSettingsUpdate struct {
	BotID           string `json:"botId"`
	RemoteChannelID string `json:"remoteChannelId"`
	RemoteGroupID   string `json:"remoteGroupId"`
	RemoteNumericID string `json:"remoteNumericId"`
	Enabled         bool   `json:"enabled"`
}

func GetChannelBotSettings(ctx context.Context, channelID string) (*ChannelBotSettings, error) {
	bindings, err := model.BotChannelBindingsByChannelID(channelID)
	if err != nil {
		return nil, err
	}
	settings := &ChannelBotSettings{
		ChannelID: channelID,
	}
	for _, binding := range bindings {
		if !binding.IsDefault {
			continue
		}
		settings.BindingID = binding.ID
		settings.BotID = binding.BotID
		settings.Enabled = binding.Enabled
		settings.RemoteChannelID = binding.RemoteChannelID
		settings.RemoteGroupID = binding.RemoteGroupID
		settings.RemoteNumericID = binding.RemoteNumericID
		settings.UpdatedAt = binding.UpdatedAt.UnixMilli()
		return settings, nil
	}
	return settings, nil
}

func SaveChannelBotSettings(ctx context.Context, channelID, actorID string, payload *ChannelBotSettingsUpdate) (*ChannelBotSettings, error) {
	if payload == nil {
		return nil, errors.New("payload is nil")
	}
	if !payload.Enabled || strings.TrimSpace(payload.BotID) == "" {
		if err := model.BotChannelBindingDeleteByChannel(channelID); err != nil {
			return nil, err
		}
		return &ChannelBotSettings{
			ChannelID: channelID,
			Enabled:   false,
		}, nil
	}
	bot, err := model.BotProfileGet(payload.BotID)
	if err != nil {
		return nil, err
	}
	if !bot.Enabled {
		return nil, fmt.Errorf("机器人 %s 已禁用，无法绑定", bot.Name)
	}
	binding := &model.BotChannelBindingModel{
		BotID:           bot.ID,
		ChannelID:       channelID,
		RemoteChannelID: strings.TrimSpace(payload.RemoteChannelID),
		RemoteGroupID:   strings.TrimSpace(payload.RemoteGroupID),
		RemoteNumericID: NormalizeOneBotNumericID(payload.RemoteNumericID),
		Enabled:         payload.Enabled,
		IsDefault:       true,
		UpdatedBy:       actorID,
	}
	exists, err := model.BotChannelBindingsByChannelID(channelID)
	if err != nil {
		return nil, err
	}
	var defaultBinding *model.BotChannelBindingModel
	for _, b := range exists {
		if b.IsDefault {
			defaultBinding = b
			break
		}
	}
	if defaultBinding != nil {
		binding.ID = defaultBinding.ID
		binding.CreatedBy = defaultBinding.CreatedBy
	} else {
		binding.CreatedBy = actorID
	}
	if err := model.BotChannelBindingUpsert(binding); err != nil {
		return nil, err
	}
	if err := ensureBotChannelAccess(channelID, bot); err != nil {
		return nil, err
	}
	return &ChannelBotSettings{
		ChannelID:       channelID,
		BindingID:       binding.ID,
		BotID:           binding.BotID,
		RemoteChannelID: binding.RemoteChannelID,
		RemoteGroupID:   binding.RemoteGroupID,
		RemoteNumericID: binding.RemoteNumericID,
		Enabled:         binding.Enabled,
		UpdatedAt:       time.Now().UnixMilli(),
	}, nil
}

func ensureBotChannelAccess(channelID string, bot *model.BotProfileModel) error {
	if bot == nil {
		return errors.New("bot profile is nil")
	}
	if _, err := model.MemberGetByUserIDAndChannelID(bot.UserID, channelID, bot.Name); err != nil {
		return err
	}
	roleID := fmt.Sprintf("ch-%s-%s", channelID, "bot")
	return ensureUserRoleMapping(bot.UserID, roleID)
}

func generateNumericAccount(length int) (string, error) {
	if length <= 0 {
		length = 12
	}
	const digits = "0123456789"
	builder := strings.Builder{}
	builder.Grow(length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", err
		}
		builder.WriteByte(digits[n.Int64()])
	}
	return builder.String(), nil
}

func ensureUserRoleMapping(userID, roleID string) error {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(roleID) == "" {
		return errors.New("invalid role assignment params")
	}
	db := model.GetDB()
	var count int64
	if err := db.Model(&model.UserRoleMappingModel{}).
		Where("user_id = ? AND role_id = ?", userID, roleID).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return model.UserRoleMappingCreate(&model.UserRoleMappingModel{
		UserID:   userID,
		RoleID:   roleID,
		RoleType: "channel",
	})
}
