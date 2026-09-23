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
func newBotCommandEvent(
	channel *protocol.Channel,
	user *protocol.User,
	member *protocol.GuildMember,
	command string,
	icMode string,
	messageID string,
	clientID string,
) *protocol.Event {
	command = strings.TrimSpace(command)
	if command == "" || channel == nil || user == nil || strings.TrimSpace(messageID) == "" {
		return nil
	}
	now := time.Now()
	nowMs := now.UnixMilli()
	if strings.TrimSpace(icMode) == "" {
		icMode = "ic"
	}
	message := &protocol.Message{
		ID:           messageID,
		Channel:      channel,
		User:         user,
		Member:       member,
		Content:      command,
		Timestamp:    now.Unix(),
		CreatedAt:    nowMs,
		UpdatedAt:    nowMs,
		DisplayOrder: float64(nowMs),
		IcMode:       icMode,
		ClientID:     clientID,
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

func normalizeBotNicknameSyncCommand(command string) (string, error) {
	normalized := strings.TrimSpace(command)
	if normalized == "" {
		return "", errors.New("INVALID_PARAMS")
	}
	for _, prefix := range resolveBotCommandPrefixes() {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" || !strings.HasPrefix(normalized, prefix) {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(normalized[len(prefix):]))
		if len(fields) >= 2 && strings.EqualFold(fields[0], "nn") {
			return normalized, nil
		}
	}
	return "", errors.New("INVALID_PARAMS")
}

func sendBotNicknameSyncResult(ctx *ChatContext, echo string, err error) {
	if ctx == nil || ctx.Conn == nil {
		return
	}
	response := struct {
		Echo string         `json:"echo"`
		Err  string         `json:"err,omitempty"`
		Data map[string]any `json:"data,omitempty"`
	}{Echo: echo}
	if err != nil {
		response.Err = err.Error()
	} else {
		response.Data = map[string]any{"ok": true}
	}
	_ = ctx.Conn.WriteJSON(response)
}

func apiBotNicknameSyncDispatch(ctx *ChatContext, msg []byte) {
	data := struct {
		Data struct {
			ChannelID string `json:"channel_id"`
			Command   string `json:"command"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(msg, &data); err != nil {
		sendBotNicknameSyncResult(ctx, ctx.Echo, errors.New("INVALID_PARAMS"))
		return
	}

	channelID := strings.TrimSpace(data.Data.ChannelID)
	command, err := normalizeBotNicknameSyncCommand(data.Data.Command)
	if err != nil || channelID == "" {
		if err == nil {
			err = errors.New("INVALID_PARAMS")
		}
		sendBotNicknameSyncResult(ctx, ctx.Echo, err)
		return
	}
	if !canDispatchBotCommand(ctx, channelID) {
		sendBotNicknameSyncResult(ctx, ctx.Echo, errors.New("PERMISSION_DENIED"))
		return
	}
	channel, err := model.ChannelGet(channelID)
	if err != nil || channel == nil || channel.ID == "" {
		if err == nil {
			err = errors.New("INVALID_PARAMS")
		}
		sendBotNicknameSyncResult(ctx, ctx.Echo, err)
		return
	}
	botID, err := service.PrimaryBotIdByChannelId(channelID)
	if err != nil {
		sendBotNicknameSyncResult(ctx, ctx.Echo, err)
		return
	}

	userData := ctx.User.ToProtocolType()
	var memberData *protocol.GuildMember
	member, _ := model.MemberGetByUserIDAndChannelIDBase(ctx.User.ID, channelID, ctx.User.Nickname, false)
	if member != nil {
		memberData = member.ToProtocolType()
		memberData.Roles = []string{service.ResolveMemberRoleForProtocol(ctx.User.ID, channelID, channel.WorldID)}
	}
	event := newBotCommandEvent(
		channel.ToProtocolType(),
		userData,
		memberData,
		command,
		"ic",
		"bot-nickname-sync:"+utils.NewID(),
		"bot-nickname-sync:"+utils.NewID(),
	)
	if event == nil {
		sendBotNicknameSyncResult(ctx, ctx.Echo, errors.New("INVALID_PARAMS"))
		return
	}
	if !getOneBotRuntime().publishProtocolEvent(botID, event, ctx.OneBotSessionID) {
		sendBotNicknameSyncResult(ctx, ctx.Echo, errors.New("BOT_NICKNAME_SYNC_BOT_UNAVAILABLE"))
		return
	}
	sendBotNicknameSyncResult(ctx, ctx.Echo, nil)
}
