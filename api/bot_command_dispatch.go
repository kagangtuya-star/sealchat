package api

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

type normalizedBotCommandDispatchData struct {
	ChannelID string
	UserID    string
	Command   string
	Silent    bool
	Reason    string
}

func normalizeBotCommandDispatchData(channelID, userID, command string, silent bool, reason string) (*normalizedBotCommandDispatchData, error) {
	normalized := &normalizedBotCommandDispatchData{
		ChannelID: strings.TrimSpace(channelID),
		UserID:    strings.TrimSpace(userID),
		Command:   strings.TrimSpace(command),
		Silent:    silent,
		Reason:    strings.TrimSpace(reason),
	}
	if normalized.ChannelID == "" {
		return nil, errors.New("缺少频道ID")
	}
	if normalized.Command == "" {
		return nil, errors.New("缺少指令内容")
	}
	return normalized, nil
}

func canDispatchBotCommand(ctx *ChatContext, channelID string) bool {
	if ctx == nil || ctx.User == nil {
		return false
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return false
	}
	if len(channelID) < 30 {
		return pm.CanWithChannelRole(ctx.User.ID, channelID, pm.PermFuncChannelTextSend, pm.PermFuncChannelTextSendAll)
	}
	fr, _ := model.FriendRelationGetByID(channelID)
	if fr.ID == "" {
		return false
	}
	return fr.UserID1 == ctx.User.ID || fr.UserID2 == ctx.User.ID
}

func sendBotCommandDispatchResult(ctx *ChatContext, echo string, ok bool, errMsg string) {
	resp := map[string]any{
		"api":  "",
		"echo": echo,
		"data": map[string]any{
			"ok": ok,
		},
	}
	if !ok && strings.TrimSpace(errMsg) != "" {
		resp["data"].(map[string]any)["error"] = strings.TrimSpace(errMsg)
	}
	_ = ctx.Conn.WriteJSON(resp)
}

func newBotCommandDispatchEvent(
	channel *protocol.Channel,
	user *protocol.User,
	member *protocol.GuildMember,
	command string,
	icMode string,
) *protocol.Event {
	command = strings.TrimSpace(command)
	if command == "" || channel == nil || user == nil {
		return nil
	}
	now := time.Now()
	nowMs := now.UnixMilli()
	if strings.TrimSpace(icMode) == "" {
		icMode = "ic"
	}
	message := &protocol.Message{
		ID:           "bot-command-dispatch:" + utils.NewID(),
		Channel:      channel,
		User:         user,
		Member:       member,
		Content:      command,
		Timestamp:    now.Unix(),
		CreatedAt:    nowMs,
		UpdatedAt:    nowMs,
		DisplayOrder: float64(nowMs),
		IcMode:       icMode,
		ClientID:     "bot-command-dispatch:" + utils.NewID(),
	}
	return &protocol.Event{
		Type:    protocol.EventMessageCreated,
		Channel: channel,
		User:    user,
		Member:  member,
		Message: message,
		MessageContext: &protocol.MessageContext{
			ICMode:       icMode,
			SenderUserID: user.ID,
		},
	}
}

func apiBotCommandDispatch(ctx *ChatContext, msg []byte) {
	data := struct {
		Echo string `json:"echo"`
		Data struct {
			ChannelID string `json:"channel_id"`
			UserID    string `json:"user_id"`
			Command   string `json:"command"`
			Silent    bool   `json:"silent"`
			Reason    string `json:"reason"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(msg, &data); err != nil {
		sendBotCommandDispatchResult(ctx, data.Echo, false, "请求解析失败")
		return
	}

	normalized, err := normalizeBotCommandDispatchData(
		data.Data.ChannelID,
		data.Data.UserID,
		data.Data.Command,
		data.Data.Silent,
		data.Data.Reason,
	)
	if err != nil {
		sendBotCommandDispatchResult(ctx, data.Echo, false, err.Error())
		return
	}
	if normalized.UserID == "" && ctx != nil && ctx.User != nil {
		normalized.UserID = strings.TrimSpace(ctx.User.ID)
	}
	if !canDispatchBotCommand(ctx, normalized.ChannelID) {
		sendBotCommandDispatchResult(ctx, data.Echo, false, "无权在该频道调度 BOT 指令")
		return
	}

	channel, err := model.ChannelGet(normalized.ChannelID)
	if err != nil {
		sendBotCommandDispatchResult(ctx, data.Echo, false, err.Error())
		return
	}
	if channel == nil || channel.ID == "" {
		sendBotCommandDispatchResult(ctx, data.Echo, false, "频道不存在")
		return
	}
	if _, err := service.SelectedBotIdByChannelId(normalized.ChannelID); err != nil {
		sendBotCommandDispatchResult(ctx, data.Echo, false, err.Error())
		return
	}

	userData := ctx.User.ToProtocolType()
	var memberData *protocol.GuildMember
	member, _ := model.MemberGetByUserIDAndChannelIDBase(ctx.User.ID, normalized.ChannelID, ctx.User.Nickname, false)
	if member != nil {
		memberData = member.ToProtocolType()
		memberData.Roles = []string{service.ResolveMemberRoleForProtocol(ctx.User.ID, normalized.ChannelID, channel.WorldID)}
	}

	event := newBotCommandDispatchEvent(
		channel.ToProtocolType(),
		userData,
		memberData,
		normalized.Command,
		"ic",
	)
	if event == nil {
		sendBotCommandDispatchResult(ctx, data.Echo, false, "BOT 指令构造失败")
		return
	}
	if normalized.UserID != "" && event.MessageContext != nil {
		event.MessageContext.SenderUserID = normalized.UserID
	}

	ctx.BroadcastEventInChannelForBot(normalized.ChannelID, event)
	sendBotCommandDispatchResult(ctx, data.Echo, true, "")
}
