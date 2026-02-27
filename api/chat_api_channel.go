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
	worldID := strings.TrimSpace(data.WorldID)
	if worldID == "" {
		return nil, fmt.Errorf("world_id 缺失")
	}
	if !service.IsWorldAdmin(worldID, ctx.User.ID) && !pm.CanWithSystemRole(ctx.User.ID, pm.PermModAdmin) {
		return nil, fmt.Errorf("无权在该世界创建频道")
	}

	m := service.ChannelNew(utils.NewID(), permType, data.Name, worldID, ctx.User.ID, data.ParentID)
	if m != nil {
		ev := &protocol.Event{
			Type:    protocol.EventChannelUpdated,
			Channel: m.ToProtocolType(),
			Argv: &protocol.Argv{
				Options: map[string]interface{}{
					"treeChanged": true,
					"worldId":     worldID,
					"action":      "create",
				},
			},
		}
		if ctx.User != nil {
			ev.User = ctx.User.ToProtocolType()
		}
		broadcastEventToWorld(worldID, ev)
	}

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
	WorldID string `json:"world_id"`
}) (any, error) {
	worldID := strings.TrimSpace(data.WorldID)
	if ctx.IsReadOnly() {
		if worldID == "" {
			return nil, fmt.Errorf("未找到世界")
		}
		world, err := service.GetWorldByID(worldID)
		if err != nil {
			return nil, err
		}
		if world == nil || strings.ToLower(strings.TrimSpace(world.Visibility)) != model.WorldVisibilityPublic {
			return nil, fmt.Errorf("世界未开放公开访问")
		}
		items, err := service.ChannelListPublicByWorld(worldID)
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
			Data    []*model.ChannelModel `json:"data"`
			WorldID string                `json:"world_id"`
		}{
			Data:    items,
			WorldID: worldID,
		}, nil
	}
	if worldID == "" {
		if w, err := service.GetOrCreateDefaultWorld(); err == nil && w != nil {
			worldID = w.ID
		}
	}
	if worldID == "" {
		return nil, fmt.Errorf("未找到世界")
	}
	if !service.IsWorldMember(worldID, ctx.User.ID) && !pm.CanWithSystemRole(ctx.User.ID, pm.PermModAdmin) {
		return nil, fmt.Errorf("尚未加入该世界")
	}
	items, err := service.ChannelList(ctx.User.ID, worldID)
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
		Data    []*model.ChannelModel `json:"data"`
		WorldID string                `json:"world_id"`
	}{
		Data:    items,
		WorldID: worldID,
	}, nil
}

func apiChannelFavoriteList(ctx *ChatContext, data *struct {
	WorldID string `json:"world_id"`
}) (any, error) {
	worldID := strings.TrimSpace(data.WorldID)
	if ctx.IsReadOnly() {
		if worldID == "" {
			return nil, fmt.Errorf("未找到世界")
		}
		world, err := service.GetWorldByID(worldID)
		if err != nil {
			return nil, err
		}
		if world == nil || strings.ToLower(strings.TrimSpace(world.Visibility)) != model.WorldVisibilityPublic {
			return nil, fmt.Errorf("世界未开放公开访问")
		}
		items, err := service.ChannelListPublicByWorld(worldID)
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
			Data    []*model.ChannelModel `json:"data"`
			WorldID string                `json:"world_id"`
		}{
			Data:    items,
			WorldID: worldID,
		}, nil
	}
	if worldID == "" {
		if w, err := service.GetOrCreateDefaultWorld(); err == nil && w != nil {
			worldID = w.ID
		}
	}
	if worldID == "" {
		return nil, fmt.Errorf("未找到世界")
	}
	if !service.IsWorldMember(worldID, ctx.User.ID) && !pm.CanWithSystemRole(ctx.User.ID, pm.PermModAdmin) {
		return nil, fmt.Errorf("尚未加入该世界")
	}
	items, err := service.ChannelList(ctx.User.ID, worldID)
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
		Data    []*model.ChannelModel `json:"data"`
		WorldID string                `json:"world_id"`
	}{
		Data:    items,
		WorldID: worldID,
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
	ChannelId       string   `json:"channel_id"`
	IncludeArchived bool     `json:"include_archived"`
	ICFilter        string   `json:"ic_filter"`
	RoleIDs         []string `json:"role_ids"`
	IncludeRoleless bool     `json:"include_roleless"`
}) (any, error) {
	channelId := data.ChannelId
	channelWorldID := ""

	// 权限检查
	if ctx.IsReadOnly() {
		channel, err := service.CanGuestAccessChannel(channelId)
		if err != nil {
			return nil, err
		}
		if channel != nil {
			channelWorldID = channel.WorldID
		}
		if ctx.ConnInfo.ChannelId != "" {
			ctx.ConnInfo.ChannelId = ""
			ctx.ConnInfo.WorldId = ""
		}
		ctx.ConnInfo.ChannelId = channelId
		ctx.ConnInfo.WorldId = channelWorldID
		ctx.ConnInfo.Focused = true
		member := &model.MemberModel{
			UserID:    ctx.User.ID,
			ChannelID: channelId,
			Nickname:  ctx.User.Nickname,
		}
		memberPT := member.ToProtocolType()
		return &struct {
			Member *protocol.GuildMember `json:"member"`
		}{
			Member: memberPT,
		}, nil
	}
	if len(channelId) < 30 { // 注意，这不是一个好的区分方式
		// 群内
		if ch, err := model.ChannelGet(channelId); err == nil && ch != nil {
			if ch.ID == "" {
				return nil, fmt.Errorf("频道不存在")
			}
			channelWorldID = ch.WorldID
			if ch.WorldID != "" && !service.IsWorldMember(ch.WorldID, ctx.User.ID) && !pm.CanWithSystemRole(ctx.User.ID, pm.PermModAdmin) {
				return nil, fmt.Errorf("尚未加入该世界")
			}
		}
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
		oldChannelId := ctx.ConnInfo.ChannelId
		if s, ok := ctx.ChannelUsersMap.Load(oldChannelId); ok {
			excludeConn := ctx.ConnInfo.Conn
			if !userHasChannelConnection(ctx.User.ID, oldChannelId, ctx.UserId2ConnInfo, excludeConn) {
				s.Delete(ctx.User.ID)
			}
		}
		ctx.BroadcastChannelPresence(oldChannelId)
		ctx.ConnInfo.WorldId = ""
	}

	member, err := model.MemberGetByUserIDAndChannelID(ctx.User.ID, channelId, ctx.User.Nickname)
	if err != nil {
		return nil, err
	}

	// 确保用户有隐形默认身份（群内频道才需要）
	if len(channelId) < 30 {
		_, _ = service.EnsureHiddenDefaultIdentity(ctx.User.ID, channelId)
	}

	memberPT := member.ToProtocolType()

	// 然后添加新的
	chUserSet, _ := ctx.ChannelUsersMap.LoadOrStore(channelId, &utils.SyncSet[string]{})
	chUserSet.Add(ctx.User.ID)

	ctx.ConnInfo.ChannelId = channelId
	ctx.ConnInfo.WorldId = channelWorldID
	ctx.ConnInfo.Focused = true

	ctx.BroadcastEventInChannel(channelId, &protocol.Event{
		Type:   "channel-entered",
		User:   ctx.User.ToProtocolType(),
		Member: memberPT,
	})
	ctx.BroadcastChannelPresence(channelId)

	// 获取第一条未读消息信息
	firstUnreadMsgId, firstUnreadMsgTime, _ := model.ChannelGetFirstUnreadInfo(channelId, ctx.User.ID, &model.FirstUnreadFilterOptions{
		IncludeArchived: data.IncludeArchived,
		ICFilter:        data.ICFilter,
		RoleIDs:         data.RoleIDs,
		IncludeRoleless: data.IncludeRoleless,
		ReadAllWhispers: canUserReadAllWhispersInChannel(ctx.User.ID, channelId),
	})

	rData := &struct {
		Member               *protocol.GuildMember `json:"member"`
		FirstUnreadMessageId string                `json:"first_unread_message_id,omitempty"`
		FirstUnreadMsgTime   int64                 `json:"first_unread_msg_time,omitempty"`
		CharacterAPIEnabled  bool                  `json:"character_api_enabled"`
		CharacterAPIReason   string                `json:"character_api_reason,omitempty"`
	}{
		Member:               memberPT,
		FirstUnreadMessageId: firstUnreadMsgId,
		FirstUnreadMsgTime:   firstUnreadMsgTime,
	}
	characterEnabled, characterReason := GetChannelCharacterAPICapability(channelId, nil)
	rData.CharacterAPIEnabled = characterEnabled
	if !characterEnabled && characterReason != "" {
		rData.CharacterAPIReason = characterReason
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

	if data.BotFeatureEnabled != nil && *data.BotFeatureEnabled {
		roleId := fmt.Sprintf("ch-%s-%s", channel.ID, "bot")
		userIds, err := model.UserRoleMappingUserIdListByRoleId(roleId)
		if err != nil {
			return nil, err
		}
		if len(userIds) == 0 {
			return nil, fmt.Errorf("启用机器人骰点前，请先在成员管理中将机器人加入“机器人”角色")
		}
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

func apiChannelBotWhisperForwardUpdate(ctx *ChatContext, data *struct {
	ChannelID    string `json:"channel_id"`
	ConfigJSON   string `json:"config_json"`
	ApplyToWorld bool   `json:"apply_to_world"`
}) (any, error) {
	channelID := strings.TrimSpace(data.ChannelID)
	if channelID == "" {
		return nil, fmt.Errorf("频道ID不能为空")
	}
	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return nil, err
	}
	if channel == nil || channel.ID == "" {
		return nil, fmt.Errorf("频道不存在")
	}
	if strings.TrimSpace(channel.WorldID) == "" {
		return nil, fmt.Errorf("该频道不支持 BOT 私聊转发配置")
	}
	if !service.IsWorldAdmin(channel.WorldID, ctx.User.ID) && !pm.CanWithSystemRole(ctx.User.ID, pm.PermModAdmin) {
		return nil, fmt.Errorf("您没有权限修改 BOT 私聊转发配置")
	}
	_, normalizedJSON, err := validateAndNormalizeBotWhisperForwardConfig(data.ConfigJSON)
	if err != nil {
		return nil, err
	}

	targetChannels := make([]*model.ChannelModel, 0, 8)
	if data.ApplyToWorld {
		var channels []*model.ChannelModel
		if err := model.GetDB().
			Where("world_id = ? AND (is_private = ? OR is_private IS NULL)", channel.WorldID, false).
			Find(&channels).Error; err != nil {
			return nil, err
		}
		if len(channels) == 0 {
			return nil, fmt.Errorf("当前世界没有可更新的频道")
		}
		channelIDs := make([]string, 0, len(channels))
		for _, ch := range channels {
			if ch == nil || ch.ID == "" {
				continue
			}
			channelIDs = append(channelIDs, ch.ID)
		}
		if len(channelIDs) == 0 {
			return nil, fmt.Errorf("当前世界没有可更新的频道")
		}
		if err := model.GetDB().Model(&model.ChannelModel{}).
			Where("id IN ?", channelIDs).
			Update("bot_whisper_forward_config", normalizedJSON).Error; err != nil {
			return nil, err
		}
		targetChannels = channels
	} else {
		if err := model.GetDB().Model(&model.ChannelModel{}).
			Where("id = ?", channel.ID).
			Update("bot_whisper_forward_config", normalizedJSON).Error; err != nil {
			return nil, err
		}
		targetChannels = append(targetChannels, channel)
	}

	updatedChannelIDs := make([]string, 0, len(targetChannels))
	for _, target := range targetChannels {
		if target == nil || target.ID == "" {
			continue
		}
		target.BotWhisperForwardConfig = normalizedJSON
		updatedChannelIDs = append(updatedChannelIDs, target.ID)
		ev := &protocol.Event{
			Type:    protocol.EventChannelUpdated,
			Channel: target.ToProtocolType(),
			User:    ctx.User.ToProtocolType(),
		}
		ctx.BroadcastEventInChannel(target.ID, ev)
		ctx.BroadcastEventInChannelForBot(target.ID, ev)
	}

	return &struct {
		ChannelID    string   `json:"channel_id"`
		ChannelIDs   []string `json:"channel_ids"`
		WorldID      string   `json:"world_id"`
		ApplyToWorld bool     `json:"apply_to_world"`
		UpdatedCount int      `json:"updated_count"`
		ConfigJSON   string   `json:"config_json"`
	}{
		ChannelID:    channel.ID,
		ChannelIDs:   updatedChannelIDs,
		WorldID:      channel.WorldID,
		ApplyToWorld: data.ApplyToWorld,
		UpdatedCount: len(updatedChannelIDs),
		ConfigJSON:   normalizedJSON,
	}, nil
}
