package api

import (
	"fmt"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

func apiChannelCreate(ctx *ChatContext, data *protocol.Channel) (any, error) {
	if data.PermType != "public" && data.PermType != "non-public" {
		return nil, nil
	}
	permType := data.PermType

	if permType == "public" {
		if !pm.CanWithSystemRole(ctx.User.ID, pm.PermFuncChannelCreatePublic) {
			return nil, nil
		}
	} else {
		if !pm.CanWithSystemRole(ctx.User.ID, pm.PermFuncChannelCreateNonPublic) {
			return nil, nil
		}
	}

	m := service.ChannelNew(utils.NewID(), permType, data.Name, ctx.User.ID, data.ParentID)

	return &struct {
		Channel *protocol.Channel `json:"channel"`
	}{
		Channel: &protocol.Channel{ID: m.ID, Name: m.Name},
	}, nil
}

func apiChannelPrivateCreate(ctx *ChatContext, data *struct {
	UserId string `json:"user_id"`
}) (any, error) {
	if ctx.User.ID == data.UserId {
		return &struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}{Code: http.StatusBadRequest, Msg: "不能和自己进行私聊"}, nil
	}

	ch, isNew := model.ChannelPrivateNew(ctx.User.ID, data.UserId) // 创建私聊频道
	if ch == nil {
		return &struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}{Code: http.StatusBadRequest, Msg: "指定的用户不存在或数据库异常"}, nil
	}

	if f := model.FriendRelationGet(ctx.User.ID, data.UserId); f.ID != "" {
		model.FriendRelationSetVisible(ctx.User.ID, data.UserId)
	} else {
		_ = model.FriendRelationCreate(ctx.User.ID, data.UserId, false) // 创建一个用户关系:陌生人
	}

	return &struct {
		Channel *protocol.Channel `json:"channel"`
		IsNew   bool              `json:"is_new"`
	}{Channel: ch.ToProtocolType(), IsNew: isNew}, nil
}

func apiChannelList(ctx *ChatContext, data *struct {
	UserId string `json:"user_id"`
}) (any, error) {
	items, err := service.ChannelList(ctx.User.ID)
	if err != nil {
		return nil, err
	}

	for _, item := range items {
		if x, exists := ctx.ChannelUsersMap.Load(item.ID); exists {
			if !item.IsPrivate {
				item.MembersCount = x.Len()
			}
		}
	}

	return &struct {
		Data []*model.ChannelModel `json:"data"`
		Next string                `json:"next"`
	}{
		Data: items,
	}, nil
}

type RespChannelMember struct {
	Echo string         `json:"echo"`
	Data map[string]int `json:"data"`
}

func apiChannelMemberCount(ctx *ChatContext, data *struct {
	ChannelIds []string `json:"channel_ids"`
}) (any, error) {
	id2count := map[string]int{}
	for _, chId := range data.ChannelIds {
		if strings.Contains(chId, ":") {
			// 私聊跳过
			continue
		}
		if x, exists := ctx.ChannelUsersMap.Load(chId); exists {
			id2count[chId] = x.Len()
		}
	}

	return id2count, nil
}

// 进入频道
func apiChannelEnter(ctx *ChatContext, data *struct {
	ChannelId string `json:"channel_id"`
}) (any, error) {
	channelId := data.ChannelId

	// 权限检查
	if len(channelId) < 30 { // 注意，这不是一个好的区分方式
		// 群内
		if !pm.CanWithChannelRole(ctx.User.ID, channelId, pm.PermFuncChannelRead, pm.PermFuncChannelReadAll) {
			return nil, nil
		}
	} else {
		// 好友/陌生人
		fr, _ := model.FriendRelationGetByID(channelId)
		if fr.ID == "" {
			return nil, nil
		}
	}

	// 如果有旧的，移除旧的
	if ctx.ConnInfo.ChannelId != "" {
		if s, ok := ctx.ChannelUsersMap.Load(ctx.ConnInfo.ChannelId); ok {
			s.Delete(ctx.User.ID)
		}
		ctx.BroadcastChannelPresence(ctx.ConnInfo.ChannelId)
	}

	member, err := model.MemberGetByUserIDAndChannelID(ctx.User.ID, channelId, ctx.User.Nickname)
	if err != nil {
		return nil, err
	}
	memberPT := member.ToProtocolType()

	// 然后添加新的
	chUserSet, _ := ctx.ChannelUsersMap.LoadOrStore(channelId, &utils.SyncSet[string]{})
	chUserSet.Add(ctx.User.ID)

	ctx.ConnInfo.ChannelId = channelId
	ctx.ConnInfo.Focused = true

	ctx.BroadcastEventInChannel(channelId, &protocol.Event{
		Type:   "channel-entered",
		User:   ctx.User.ToProtocolType(),
		Member: memberPT,
	})
	ctx.BroadcastChannelPresence(channelId)

	rData := &struct {
		Member *protocol.GuildMember `json:"member"`
	}{
		Member: memberPT,
	}
	return rData, nil
}

func apiChannelMemberListOnline(ctx *ChatContext, data *struct {
	ChannelId string `json:"channel_id"`
	Next      string `json:"next"`
}) (any, error) {
	return apiUserListCommon(data.Next, func(q *gorm.DB) {
		var arr []string
		if x, exists := ctx.ChannelUsersMap.Load(data.ChannelId); exists {
			x.Range(func(key string) bool {
				arr = append(arr, key)
				return true
			})
		}
		q = q.Where("id in ?", arr)
	})
}

func apiChannelMemberList(ctx *ChatContext, data *struct {
	ChannelId string `json:"channel_id"`
	Next      string `json:"next"`
}) (any, error) {
	return apiUserListCommon(data.Next, func(q *gorm.DB) {
		var arr []string
		if x, exists := ctx.ChannelUsersMap.Load(data.ChannelId); exists {
			x.Range(func(key string) bool {
				arr = append(arr, key)
				return true
			})
		}
		q = q.Where("id in ?", arr)
	})
}

func apiChannelDefaultDiceUpdate(ctx *ChatContext, data *struct {
	ChannelID       string `json:"channel_id"`
	DefaultDiceExpr string `json:"default_dice_expr"`
}) (any, error) {
	if data.ChannelID == "" {
		return nil, fmt.Errorf("频道ID不能为空")
	}
	if !pm.CanWithChannelRole(ctx.User.ID, data.ChannelID, pm.PermFuncChannelManageInfo, pm.PermFuncChannelRoleLink) {
		return nil, fmt.Errorf("您没有权限修改默认骰")
	}
	channel, err := model.ChannelGet(data.ChannelID)
	if err != nil {
		return nil, err
	}
	if channel.ID == "" {
		return nil, fmt.Errorf("频道不存在")
	}
	normalized, err := service.NormalizeDefaultDiceExpr(data.DefaultDiceExpr)
	if err != nil {
		return nil, err
	}
	if err := model.GetDB().Model(&model.ChannelModel{}).
		Where("id = ?", channel.ID).
		Update("default_dice_expr", normalized).Error; err != nil {
		return nil, err
	}
	channel.DefaultDiceExpr = normalized
	channelData := channel.ToProtocolType()
	ev := &protocol.Event{
		Type:    protocol.EventChannelUpdated,
		Channel: channelData,
		User:    ctx.User.ToProtocolType(),
	}
	ctx.BroadcastEventInChannel(channel.ID, ev)
	ctx.BroadcastEventInChannelForBot(channel.ID, ev)

	return &struct {
		ChannelID       string `json:"channel_id"`
		DefaultDiceExpr string `json:"default_dice_expr"`
	}{ChannelID: channel.ID, DefaultDiceExpr: normalized}, nil
}

func apiChannelFeatureUpdate(ctx *ChatContext, data *struct {
	ChannelID          string `json:"channel_id"`
	BuiltInDiceEnabled *bool  `json:"built_in_dice_enabled"`
	BotFeatureEnabled  *bool  `json:"bot_feature_enabled"`
}) (any, error) {
	if data.ChannelID == "" {
		return nil, fmt.Errorf("频道ID不能为空")
	}
	if data.BuiltInDiceEnabled == nil && data.BotFeatureEnabled == nil {
		return nil, fmt.Errorf("没有可更新的字段")
	}
	if !pm.CanWithChannelRole(ctx.User.ID, data.ChannelID, pm.PermFuncChannelManageInfo, pm.PermFuncChannelRoleLink) {
		return nil, fmt.Errorf("您没有权限更新频道特性")
	}

	channel, err := model.ChannelGet(data.ChannelID)
	if err != nil {
		return nil, err
	}
	if channel.ID == "" {
		return nil, fmt.Errorf("频道不存在")
	}

	updates := map[string]interface{}{}
	if data.BuiltInDiceEnabled != nil {
		channel.BuiltInDiceEnabled = *data.BuiltInDiceEnabled
		updates["built_in_dice_enabled"] = channel.BuiltInDiceEnabled
	}
	if data.BotFeatureEnabled != nil {
		channel.BotFeatureEnabled = *data.BotFeatureEnabled
		updates["bot_feature_enabled"] = channel.BotFeatureEnabled
	}
	if len(updates) == 0 {
		return nil, fmt.Errorf("没有可更新的字段")
	}

	if !channel.BuiltInDiceEnabled && !channel.BotFeatureEnabled {
		channel.BuiltInDiceEnabled = true
		updates["built_in_dice_enabled"] = true
	}

	if err := model.GetDB().Model(&model.ChannelModel{}).
		Where("id = ?", channel.ID).
		Updates(updates).Error; err != nil {
		return nil, err
	}

	channelData := channel.ToProtocolType()
	ev := &protocol.Event{
		Type:    protocol.EventChannelUpdated,
		Channel: channelData,
		User:    ctx.User.ToProtocolType(),
	}
	ctx.BroadcastEventInChannel(channel.ID, ev)
	ctx.BroadcastEventInChannelForBot(channel.ID, ev)

	return &struct {
		ChannelID          string `json:"channel_id"`
		BuiltInDiceEnabled bool   `json:"built_in_dice_enabled"`
		BotFeatureEnabled  bool   `json:"bot_feature_enabled"`
	}{
		ChannelID:          channel.ID,
		BuiltInDiceEnabled: channel.BuiltInDiceEnabled,
		BotFeatureEnabled:  channel.BotFeatureEnabled,
	}, nil
}
