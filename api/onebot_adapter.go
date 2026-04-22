package api

import (
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

type oneBotActionRequest struct {
	Action string          `json:"action"`
	Params json.RawMessage `json:"params"`
	Echo   any             `json:"echo,omitempty"`
}

type oneBotActionResponse struct {
	Status  string `json:"status"`
	RetCode int    `json:"retcode"`
	Data    any    `json:"data"`
	Msg     string `json:"msg,omitempty"`
	Wording string `json:"wording,omitempty"`
	Echo    any    `json:"echo,omitempty"`
}

type oneBotActionError struct {
	RetCode int
	Message string
}

type oneBotInt64Param int64

func (v *oneBotInt64Param) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*v = 0
		return nil
	}

	var numeric int64
	if len(trimmed) >= 2 && trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"' {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		text = strings.TrimSpace(text)
		if text == "" {
			*v = 0
			return nil
		}
		parsed, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return err
		}
		numeric = parsed
	} else {
		var parsed int64
		if err := json.Unmarshal(data, &parsed); err != nil {
			return err
		}
		numeric = parsed
	}

	*v = oneBotInt64Param(numeric)
	return nil
}

func (v oneBotInt64Param) Int64() int64 {
	return int64(v)
}

func (e *oneBotActionError) Error() string {
	return e.Message
}

func oneBotBadRequest(message string) error {
	return &oneBotActionError{RetCode: 1400, Message: message}
}

func oneBotForbidden(message string) error {
	return &oneBotActionError{RetCode: 1403, Message: message}
}

func oneBotNotFound(message string) error {
	return &oneBotActionError{RetCode: 1404, Message: message}
}

func oneBotFailureResponse(err error, echo any) *oneBotActionResponse {
	retCode := 1400
	msg := "请求失败"
	var actionErr *oneBotActionError
	if errors.As(err, &actionErr) {
		retCode = actionErr.RetCode
		if strings.TrimSpace(actionErr.Message) != "" {
			msg = actionErr.Message
		}
	} else if err != nil && strings.TrimSpace(err.Error()) != "" {
		msg = err.Error()
	}
	return &oneBotActionResponse{
		Status:  "failed",
		RetCode: retCode,
		Data:    nil,
		Msg:     msg,
		Wording: msg,
		Echo:    echo,
	}
}

func oneBotSuccessResponse(data any, echo any) *oneBotActionResponse {
	return &oneBotActionResponse{
		Status:  "ok",
		RetCode: 0,
		Data:    data,
		Echo:    echo,
	}
}

func resolveOneBotAccessToken(raw string) string {
	token := strings.TrimSpace(raw)
	lower := strings.ToLower(token)
	if strings.HasPrefix(lower, "bearer ") {
		return strings.TrimSpace(token[len("bearer "):])
	}
	return token
}

func resolveOneBotBotFromToken(token string) (*model.UserModel, *model.BotTokenModel, error) {
	token = resolveOneBotAccessToken(token)
	if len(token) != 32 {
		return nil, nil, oneBotForbidden("token invalid")
	}
	user, err := model.BotVerifyAccessToken(token)
	if err != nil || user == nil || !user.IsBot {
		return nil, nil, oneBotForbidden("token invalid")
	}
	if strings.TrimSpace(user.BotKind) != model.BotKindManual {
		return nil, nil, oneBotForbidden("only manual bot supports onebot")
	}
	botToken, err := model.BotTokenGet(user.ID)
	if err != nil || botToken == nil || botToken.ID == "" {
		return nil, nil, oneBotForbidden("bot token missing")
	}
	return user, botToken, nil
}

func dispatchOneBotAction(session *oneBotSession, req *oneBotActionRequest) *oneBotActionResponse {
	if session == nil || session.BotUser == nil {
		return oneBotFailureResponse(oneBotForbidden("session unavailable"), nil)
	}
	if req == nil {
		return oneBotFailureResponse(oneBotBadRequest("invalid request"), nil)
	}
	if session.Role == oneBotSessionRoleEvent {
		return oneBotFailureResponse(oneBotForbidden("event connection cannot call api"), req.Echo)
	}

	var (
		data any
		err  error
	)
	switch strings.TrimSpace(req.Action) {
	case "send_private_msg":
		data, err = oneBotActionSendPrivateMessage(session, req.Params)
	case "send_group_msg":
		data, err = oneBotActionSendGroupMessage(session, req.Params)
	case "send_msg":
		data, err = oneBotActionSendMessage(session, req.Params)
	case "delete_msg":
		data, err = oneBotActionDeleteMessage(session, req.Params)
	case "get_msg":
		data, err = oneBotActionGetMessage(session, req.Params)
	case "get_login_info":
		data, err = oneBotActionGetLoginInfo(session)
	case "get_stranger_info":
		data, err = oneBotActionGetStrangerInfo(session, req.Params)
	case "get_friend_list":
		data, err = oneBotActionGetFriendList(session)
	case "get_group_info":
		data, err = oneBotActionGetGroupInfo(session, req.Params)
	case "get_group_list":
		data, err = oneBotActionGetGroupList(session)
	case "get_group_member_info":
		data, err = oneBotActionGetGroupMemberInfo(session, req.Params)
	case "get_group_member_list":
		data, err = oneBotActionGetGroupMemberList(session, req.Params)
	case "can_send_image":
		data, err = map[string]any{"yes": true}, nil
	case "get_status":
		data, err = buildOneBotStatus(session), nil
	case "get_version_info":
		data, err = map[string]any{
			"app_name":         "sealchat-onebot",
			"app_version":      utils.BuildVersion,
			"protocol_version": "v11",
		}, nil
	default:
		err = oneBotNotFound("unsupported action")
	}
	if err != nil {
		return oneBotFailureResponse(err, req.Echo)
	}
	return oneBotSuccessResponse(data, req.Echo)
}

func isOneBotSupportedAction(action string) bool {
	switch strings.TrimSpace(action) {
	case "send_private_msg",
		"send_group_msg",
		"send_msg",
		"delete_msg",
		"get_msg",
		"get_login_info",
		"get_stranger_info",
		"get_friend_list",
		"get_group_info",
		"get_group_list",
		"get_group_member_info",
		"get_group_member_list",
		"can_send_image",
		"get_status",
		"get_version_info":
		return true
	default:
		return false
	}
}

func oneBotChatContext(session *oneBotSession) *ChatContext {
	ctx := &ChatContext{
		User:            session.BotUser,
		ChannelUsersMap: getChannelUsersMap(),
		UserId2ConnInfo: getUserConnInfoMap(),
		ConnInfo:        session.ConnInfo,
		OneBotSessionID: session.ID,
	}
	return ctx
}

func oneBotCodecHooks() service.OneBotMessageCodecHooks {
	return service.OneBotMessageCodecHooks{
		ResolveUserID: func(numericID int64) (string, error) {
			return service.ResolveInternalID(service.OneBotEntityUser, numericID)
		},
		ResolveMessageID: func(numericID int64) (string, error) {
			return service.ResolveInternalID(service.OneBotEntityMessage, numericID)
		},
		ResolveUserOneBotID: func(userID string) (int64, error) {
			return service.GetOrCreateOneBotID(service.OneBotEntityUser, userID)
		},
		ResolveMessageOneBotID: func(messageID string) (int64, error) {
			return service.GetOrCreateOneBotID(service.OneBotEntityMessage, messageID)
		},
		ResolveAttachmentURL: func(token string) (string, error) {
			return resolveOneBotAttachmentURL(token)
		},
	}
}

func resolveOneBotAttachmentURL(token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", nil
	}
	if strings.HasPrefix(token, "http://") || strings.HasPrefix(token, "https://") {
		return token, nil
	}
	att, err := service.ResolveAttachment(token)
	if err != nil {
		return "", err
	}
	if att != nil {
		if publicURL := service.AttachmentPublicURL(att); strings.TrimSpace(publicURL) != "" {
			return publicURL, nil
		}
		return buildOneBotAttachmentDownloadURL(att.ID), nil
	}
	return buildOneBotAttachmentDownloadURL(token), nil
}

func buildOneBotAttachmentDownloadURL(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	path := "/api/v1/attachment/" + token
	if base := resolveOneBotAttachmentBaseURL(); base != "" {
		return strings.TrimRight(base, "/") + path
	}
	return path
}

func resolveOneBotAttachmentBaseURL() string {
	cfg := utils.GetConfig()
	if cfg == nil {
		return ""
	}
	if base := strings.TrimSpace(cfg.ImageBaseURL); base != "" {
		return normalizeOneBotDomainToURL(base)
	}
	if domain := strings.TrimSpace(cfg.Domain); domain != "" {
		return normalizeOneBotDomainToURL(domain)
	}
	return ""
}

func normalizeOneBotDomainToURL(domain string) string {
	trimmed := strings.TrimSpace(strings.TrimRight(domain, "/"))
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return trimmed
	}
	host, port, err := net.SplitHostPort(trimmed)
	formatted := trimmed
	if err == nil && host != "" && port != "" {
		formatted = utils.FormatHostPort(host, port)
	}
	scheme := "https"
	hostForScheme := host
	if hostForScheme == "" {
		hostForScheme = trimmed
	}
	hostForScheme = strings.Trim(hostForScheme, "[]")
	if ip := net.ParseIP(hostForScheme); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			scheme = "http"
		}
	} else if strings.EqualFold(hostForScheme, "localhost") || strings.HasPrefix(hostForScheme, "127.") {
		scheme = "http"
	}
	return scheme + "://" + formatted
}

func decodeOneBotMessageParam(raw json.RawMessage, autoEscape bool) (*service.OneBotDecodedMessage, error) {
	return service.DecodeOneBotMessageRaw(raw, autoEscape, oneBotCodecHooks())
}

func oneBotActionSendPrivateMessage(session *oneBotSession, raw json.RawMessage) (any, error) {
	var params struct {
		UserID     oneBotInt64Param `json:"user_id"`
		Message    json.RawMessage  `json:"message"`
		AutoEscape bool             `json:"auto_escape"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, oneBotBadRequest("invalid params")
	}
	targetUserID, err := service.ResolveInternalID(service.OneBotEntityUser, params.UserID.Int64())
	if err != nil {
		return nil, oneBotNotFound("user not found")
	}
	channel, err := ensureOneBotPrivateChannel(session.BotUser.ID, targetUserID)
	if err != nil {
		return nil, err
	}
	return oneBotActionSendIntoChannel(session, channel, params.Message, params.AutoEscape)
}

func oneBotActionSendGroupMessage(session *oneBotSession, raw json.RawMessage) (any, error) {
	var params struct {
		GroupID    oneBotInt64Param `json:"group_id"`
		Message    json.RawMessage  `json:"message"`
		AutoEscape bool             `json:"auto_escape"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, oneBotBadRequest("invalid params")
	}
	channelID, err := service.ResolveInternalID(service.OneBotEntityChannel, params.GroupID.Int64())
	if err != nil {
		return nil, oneBotNotFound("group not found")
	}
	channel, err := ensureOneBotGroupChannel(session.BotUser.ID, channelID)
	if err != nil {
		return nil, err
	}
	return oneBotActionSendIntoChannel(session, channel, params.Message, params.AutoEscape)
}

func oneBotActionSendMessage(session *oneBotSession, raw json.RawMessage) (any, error) {
	var params struct {
		MessageType string           `json:"message_type"`
		UserID      oneBotInt64Param `json:"user_id"`
		GroupID     oneBotInt64Param `json:"group_id"`
		Message     json.RawMessage  `json:"message"`
		AutoEscape  bool             `json:"auto_escape"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, oneBotBadRequest("invalid params")
	}
	messageType := strings.TrimSpace(strings.ToLower(params.MessageType))
	if messageType == "" {
		switch {
		case params.UserID.Int64() > 0:
			messageType = "private"
		case params.GroupID.Int64() > 0:
			messageType = "group"
		default:
			return nil, oneBotBadRequest("message_type invalid")
		}
	}

	switch messageType {
	case "private":
		if params.UserID.Int64() <= 0 {
			return nil, oneBotBadRequest("user_id missing")
		}
		targetUserID, err := service.ResolveInternalID(service.OneBotEntityUser, params.UserID.Int64())
		if err != nil {
			return nil, oneBotNotFound("user not found")
		}
		channel, err := ensureOneBotPrivateChannel(session.BotUser.ID, targetUserID)
		if err != nil {
			return nil, err
		}
		return oneBotActionSendIntoChannel(session, channel, params.Message, params.AutoEscape)
	case "group":
		if params.GroupID.Int64() <= 0 {
			return nil, oneBotBadRequest("group_id missing")
		}
		channelID, err := service.ResolveInternalID(service.OneBotEntityChannel, params.GroupID.Int64())
		if err != nil {
			return nil, oneBotNotFound("group not found")
		}
		channel, err := ensureOneBotGroupChannel(session.BotUser.ID, channelID)
		if err != nil {
			return nil, err
		}
		return oneBotActionSendIntoChannel(session, channel, params.Message, params.AutoEscape)
	}
	return nil, oneBotBadRequest("message_type invalid")
}

func oneBotActionSendIntoChannel(session *oneBotSession, channel *model.ChannelModel, rawMessage json.RawMessage, autoEscape bool) (any, error) {
	if session == nil || channel == nil || channel.ID == "" {
		return nil, oneBotBadRequest("channel missing")
	}
	decoded, err := decodeOneBotMessageParam(rawMessage, autoEscape)
	if err != nil {
		return nil, oneBotBadRequest(err.Error())
	}
	if shouldSuppressBotNicknameSyncAck(session, channel.ID, decoded.Content) {
		messageID, err := service.GetOrCreateOneBotID(service.OneBotEntityMessage, "suppressed-bot-nickname-sync:"+utils.NewID())
		if err != nil {
			return nil, err
		}
		return map[string]any{"message_id": messageID}, nil
	}
	resp, err := apiMessageCreate(oneBotChatContext(session), &struct {
		ChannelID         string   `json:"channel_id"`
		QuoteID           string   `json:"quote_id"`
		Content           string   `json:"content"`
		WhisperTo         string   `json:"whisper_to"`
		WhisperToIds      []string `json:"whisper_to_ids"`
		ClientID          string   `json:"client_id"`
		IdentityID        string   `json:"identity_id"`
		IdentityVariantID string   `json:"identity_variant_id"`
		ICMode            string   `json:"ic_mode"`
		BeforeID          string   `json:"before_id"`
		AfterID           string   `json:"after_id"`
		DisplayOrder      *float64 `json:"display_order"`
		TypingDurationMs  *int64   `json:"typing_duration_ms"`
	}{
		ChannelID: channel.ID,
		QuoteID:   decoded.QuoteID,
		Content:   decoded.Content,
	})
	if err != nil {
		return nil, err
	}
	message, _ := resp.(*protocol.Message)
	if message == nil || message.ID == "" {
		return nil, oneBotBadRequest("message create failed")
	}
	messageID, err := service.GetOrCreateOneBotID(service.OneBotEntityMessage, message.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"message_id": messageID}, nil
}

func shouldSuppressBotNicknameSyncAck(session *oneBotSession, channelID, content string) bool {
	if session == nil || session.ConnInfo == nil || channelID == "" {
		return false
	}
	pendingMap := session.ConnInfo.BotNicknameSyncPending
	if pendingMap == nil {
		return false
	}
	pending, ok := pendingMap.Load(channelID)
	if !ok || pending == nil {
		return false
	}
	if time.Now().UnixMilli()-pending.CreatedAt > 3_000 {
		pendingMap.Delete(channelID)
		return false
	}
	pendingMap.Delete(channelID)
	return isBotNicknameSyncAckContent(content, pending.TargetName)
}

func isBotNicknameSyncAckContent(content, targetName string) bool {
	content = strings.TrimSpace(content)
	targetName = strings.TrimSpace(targetName)
	if content == "" || targetName == "" {
		return false
	}
	return strings.Contains(content, targetName)
}

func oneBotActionDeleteMessage(session *oneBotSession, raw json.RawMessage) (any, error) {
	var params struct {
		MessageID oneBotInt64Param `json:"message_id"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, oneBotBadRequest("invalid params")
	}
	msg, err := loadOneBotMessageModel(params.MessageID.Int64())
	if err != nil {
		return nil, err
	}
	_, err = apiMessageDelete(oneBotChatContext(session), &messageDeletePayload{
		ChannelID: msg.ChannelID,
		MessageID: msg.ID,
	})
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func oneBotActionGetMessage(session *oneBotSession, raw json.RawMessage) (any, error) {
	var params struct {
		MessageID oneBotInt64Param `json:"message_id"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, oneBotBadRequest("invalid params")
	}
	msg, err := loadOneBotMessageModel(params.MessageID.Int64())
	if err != nil {
		return nil, err
	}
	channel, err := model.ChannelGet(msg.ChannelID)
	if err != nil || channel == nil || channel.ID == "" {
		return nil, oneBotNotFound("channel not found")
	}
	return buildOneBotMessageResponseFromModel(channel, msg)
}

func oneBotActionGetLoginInfo(session *oneBotSession) (any, error) {
	userID, err := service.GetOrCreateOneBotID(service.OneBotEntityBotUser, session.BotUser.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"user_id":  userID,
		"nickname": strings.TrimSpace(session.BotUser.Nickname),
	}, nil
}

func oneBotActionGetStrangerInfo(_ *oneBotSession, raw json.RawMessage) (any, error) {
	var params struct {
		UserID oneBotInt64Param `json:"user_id"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, oneBotBadRequest("invalid params")
	}
	internalID, err := service.ResolveInternalID(service.OneBotEntityUser, params.UserID.Int64())
	if err != nil {
		return nil, oneBotNotFound("user not found")
	}
	user := model.UserGet(internalID)
	if user == nil {
		return nil, oneBotNotFound("user not found")
	}
	return map[string]any{
		"user_id":  params.UserID.Int64(),
		"nickname": strings.TrimSpace(user.Nickname),
		"sex":      "unknown",
		"age":      0,
	}, nil
}

func oneBotActionGetFriendList(session *oneBotSession) (any, error) {
	items, err := model.FriendList(session.BotUser.ID, true)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item == nil || item.UserInfo == nil {
			continue
		}
		userID, err := service.GetOrCreateOneBotID(service.OneBotEntityUser, item.UserInfo.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, map[string]any{
			"user_id":  userID,
			"nickname": strings.TrimSpace(item.UserInfo.Nickname),
			"remark":   "",
		})
	}
	return result, nil
}

func oneBotActionGetGroupInfo(session *oneBotSession, raw json.RawMessage) (any, error) {
	var params struct {
		GroupID oneBotInt64Param `json:"group_id"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, oneBotBadRequest("invalid params")
	}
	channelID, err := service.ResolveInternalID(service.OneBotEntityChannel, params.GroupID.Int64())
	if err != nil {
		return nil, oneBotNotFound("group not found")
	}
	channel, err := ensureOneBotGroupChannel(session.BotUser.ID, channelID)
	if err != nil {
		return nil, err
	}
	return buildOneBotGroupInfo(channel)
}

func oneBotActionGetGroupList(session *oneBotSession) (any, error) {
	items, err := listOneBotGroupChannels(session.BotUser.ID)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		groupInfo, err := buildOneBotGroupInfo(item)
		if err != nil {
			return nil, err
		}
		result = append(result, groupInfo)
	}
	return result, nil
}

func oneBotActionGetGroupMemberInfo(session *oneBotSession, raw json.RawMessage) (any, error) {
	var params struct {
		GroupID oneBotInt64Param `json:"group_id"`
		UserID  oneBotInt64Param `json:"user_id"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, oneBotBadRequest("invalid params")
	}
	channelID, err := service.ResolveInternalID(service.OneBotEntityChannel, params.GroupID.Int64())
	if err != nil {
		return nil, oneBotNotFound("group not found")
	}
	channel, err := ensureOneBotGroupChannel(session.BotUser.ID, channelID)
	if err != nil {
		return nil, err
	}
	userID, err := service.ResolveInternalID(service.OneBotEntityUser, params.UserID.Int64())
	if err != nil {
		return nil, oneBotNotFound("user not found")
	}
	member, err := model.MemberGetByUserIDAndChannelIDBase(userID, channel.ID, "", false)
	if err != nil || member == nil || member.ID == "" {
		return nil, oneBotNotFound("member not found")
	}
	return buildOneBotGroupMemberInfo(channel, member)
}

func oneBotActionGetGroupMemberList(session *oneBotSession, raw json.RawMessage) (any, error) {
	var params struct {
		GroupID oneBotInt64Param `json:"group_id"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, oneBotBadRequest("invalid params")
	}
	channelID, err := service.ResolveInternalID(service.OneBotEntityChannel, params.GroupID.Int64())
	if err != nil {
		return nil, oneBotNotFound("group not found")
	}
	channel, err := ensureOneBotGroupChannel(session.BotUser.ID, channelID)
	if err != nil {
		return nil, err
	}
	var members []model.MemberModel
	if err := model.GetDB().Where("channel_id = ?", channel.ID).Find(&members).Error; err != nil {
		return nil, err
	}
	result := make([]map[string]any, 0, len(members))
	for i := range members {
		memberInfo, err := buildOneBotGroupMemberInfo(channel, &members[i])
		if err != nil {
			return nil, err
		}
		result = append(result, memberInfo)
	}
	return result, nil
}

func ensureOneBotPrivateChannel(botUserID, targetUserID string) (*model.ChannelModel, error) {
	if strings.TrimSpace(botUserID) == "" || strings.TrimSpace(targetUserID) == "" {
		return nil, oneBotBadRequest("missing private user id")
	}
	if rel := model.FriendRelationGet(botUserID, targetUserID); rel == nil || rel.ID == "" {
		if err := model.FriendRelationCreate(botUserID, targetUserID, false); err != nil {
			return nil, err
		}
	} else {
		model.FriendRelationSetVisible(botUserID, targetUserID)
	}
	ch, _ := model.ChannelPrivateNew(botUserID, targetUserID)
	if ch == nil || ch.ID == "" {
		ch, _ = model.ChannelPrivateGet(botUserID, targetUserID)
	}
	if ch == nil || ch.ID == "" {
		return nil, oneBotNotFound("private channel not found")
	}
	return ch, nil
}

func ensureOneBotGroupChannel(botUserID, channelID string) (*model.ChannelModel, error) {
	channel, err := model.ChannelGet(channelID)
	if err != nil || channel == nil || channel.ID == "" {
		return nil, oneBotNotFound("group not found")
	}
	if channel.IsPrivate || strings.EqualFold(strings.TrimSpace(channel.PermType), "private") {
		return nil, oneBotNotFound("group not found")
	}
	selectedBotID, err := service.SelectedBotIdByChannelId(channel.ID)
	if err != nil || selectedBotID != botUserID {
		return nil, oneBotForbidden("bot not bound to group")
	}
	return channel, nil
}

func listOneBotGroupChannels(botUserID string) ([]*model.ChannelModel, error) {
	roleIDs, err := model.UserRoleMappingListByUserID(botUserID, "", "channel")
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	result := make([]*model.ChannelModel, 0)
	for _, roleID := range roleIDs {
		if !strings.HasSuffix(roleID, "-bot") {
			continue
		}
		channelID := model.ExtractChIdFromRoleId(roleID)
		if channelID == "" {
			continue
		}
		if _, ok := seen[channelID]; ok {
			continue
		}
		channel, err := model.ChannelGet(channelID)
		if err != nil || channel == nil || channel.ID == "" {
			continue
		}
		if channel.IsPrivate || strings.EqualFold(strings.TrimSpace(channel.PermType), "private") {
			continue
		}
		seen[channelID] = struct{}{}
		result = append(result, channel)
	}
	return result, nil
}

func buildOneBotGroupInfo(channel *model.ChannelModel) (map[string]any, error) {
	groupID, err := service.GetOrCreateOneBotID(service.OneBotEntityChannel, channel.ID)
	if err != nil {
		return nil, err
	}
	var memberCount int64
	if err := model.GetDB().Model(&model.MemberModel{}).Where("channel_id = ?", channel.ID).Count(&memberCount).Error; err != nil {
		return nil, err
	}
	return map[string]any{
		"group_id":         groupID,
		"group_name":       strings.TrimSpace(channel.Name),
		"member_count":     memberCount,
		"max_member_count": memberCount,
	}, nil
}

func buildOneBotGroupMemberInfo(channel *model.ChannelModel, member *model.MemberModel) (map[string]any, error) {
	user := model.UserGet(member.UserID)
	if user == nil {
		return nil, oneBotNotFound("user not found")
	}
	groupID, err := service.GetOrCreateOneBotID(service.OneBotEntityChannel, channel.ID)
	if err != nil {
		return nil, err
	}
	userID, err := service.GetOrCreateOneBotID(service.OneBotEntityUser, user.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"group_id":          groupID,
		"user_id":           userID,
		"nickname":          strings.TrimSpace(user.Nickname),
		"card":              strings.TrimSpace(member.Nickname),
		"sex":               "unknown",
		"age":               0,
		"area":              "",
		"join_time":         member.CreatedAt.Unix(),
		"last_sent_time":    member.RecentSentAt / 1000,
		"level":             "",
		"role":              service.ResolveMemberRoleForProtocol(user.ID, channel.ID, channel.WorldID),
		"unfriendly":        false,
		"title":             "",
		"title_expire_time": 0,
		"card_changeable":   true,
	}, nil
}

func loadOneBotMessageModel(numericMessageID int64) (*model.MessageModel, error) {
	internalID, err := service.ResolveInternalID(service.OneBotEntityMessage, numericMessageID)
	if err != nil {
		return nil, oneBotNotFound("message not found")
	}
	var msg model.MessageModel
	query := model.GetDB().
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, username, nickname, avatar, is_bot")
		}).
		Preload("Member", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, nickname, user_id, channel_id")
		}).
		Where("id = ? AND is_deleted = ?", internalID, false)
	if err := query.Limit(1).Find(&msg).Error; err != nil {
		return nil, err
	}
	if msg.ID == "" {
		return nil, oneBotNotFound("message not found")
	}
	if msg.WhisperTo != "" {
		msg.WhisperTarget = loadWhisperTargetForChannel(msg.ChannelID, msg.WhisperTo)
	}
	if msg.IsWhisper {
		msg.WhisperTargets = loadWhisperTargetsForMessage(msg.ChannelID, msg.ID, msg.WhisperTarget)
	}
	msg.EnsureWhisperMeta()
	return &msg, nil
}

func buildOneBotMessageResponseFromModel(channel *model.ChannelModel, msg *model.MessageModel) (map[string]any, error) {
	channelData := channel.ToProtocolType()
	messageData := buildProtocolMessage(msg, channelData)
	quoteID := strings.TrimSpace(msg.QuoteID)
	content, err := service.EncodeOneBotMessage(messageData.Content, quoteID, oneBotCodecHooks())
	if err != nil {
		return nil, err
	}
	messageID, err := service.GetOrCreateOneBotID(service.OneBotEntityMessage, msg.ID)
	if err != nil {
		return nil, err
	}
	userID, err := service.GetOrCreateOneBotID(service.OneBotEntityUser, msg.UserID)
	if err != nil {
		return nil, err
	}
	sender := buildOneBotSender(channel, messageData, msg.UserID)
	resp := map[string]any{
		"time":         msg.CreatedAt.Unix(),
		"message_type": oneBotMessageTypeByChannel(channel),
		"message_id":   messageID,
		"real_id":      messageID,
		"user_id":      userID,
		"message":      content,
		"sender":       sender,
	}
	if !channel.IsPrivate && !strings.EqualFold(channel.PermType, "private") {
		groupID, err := service.GetOrCreateOneBotID(service.OneBotEntityChannel, channel.ID)
		if err != nil {
			return nil, err
		}
		resp["group_id"] = groupID
	}
	return resp, nil
}

func projectProtocolEventToOneBot(session *oneBotSession, event *protocol.Event) (map[string]any, bool) {
	if session == nil || event == nil || event.Message == nil || event.Channel == nil {
		return nil, false
	}
	if event.Type != protocol.EventMessageCreated {
		return nil, false
	}
	isPrivate := event.Channel.Type == protocol.DirectChannelType
	if event.Message.IsWhisper && !isPrivate {
		return nil, false
	}

	messageID, err := service.GetOrCreateOneBotID(service.OneBotEntityMessage, event.Message.ID)
	if err != nil {
		return nil, false
	}
	userIDSource := ""
	if event.User != nil {
		userIDSource = strings.TrimSpace(event.User.ID)
	}
	if userIDSource == "" && event.Message.User != nil {
		userIDSource = strings.TrimSpace(event.Message.User.ID)
	}
	if userIDSource == "" {
		return nil, false
	}
	userID, err := service.GetOrCreateOneBotID(service.OneBotEntityUser, userIDSource)
	if err != nil {
		return nil, false
	}
	quoteID := ""
	if event.Message.Quote != nil {
		quoteID = strings.TrimSpace(event.Message.Quote.ID)
	}
	content, err := service.EncodeOneBotMessage(event.Message.Content, quoteID, oneBotCodecHooks())
	if err != nil {
		return nil, false
	}
	payload := map[string]any{
		"time":        event.Timestamp,
		"self_id":     session.SelfID,
		"post_type":   "message",
		"message_id":  messageID,
		"user_id":     userID,
		"message":     content,
		"raw_message": content,
		"font":        0,
		"sender":      buildOneBotSenderFromEvent(event, userIDSource),
	}
	if isPrivate {
		payload["message_type"] = "private"
		payload["sub_type"] = "friend"
		return payload, true
	}
	groupID, err := service.GetOrCreateOneBotID(service.OneBotEntityChannel, event.Channel.ID)
	if err != nil {
		return nil, false
	}
	payload["message_type"] = "group"
	payload["sub_type"] = "normal"
	payload["group_id"] = groupID
	return payload, true
}

func buildOneBotSender(channel *model.ChannelModel, msg *protocol.Message, userID string) map[string]any {
	protocolRole := ""
	card := ""
	if msg != nil && msg.Member != nil {
		card = strings.TrimSpace(msg.Member.Nick)
		if len(msg.Member.Roles) > 0 {
			protocolRole = strings.TrimSpace(msg.Member.Roles[0])
		}
	}
	user := model.UserGet(userID)
	nickname := ""
	if user != nil {
		nickname = strings.TrimSpace(user.Nickname)
	}
	if nickname == "" && msg != nil && msg.User != nil {
		nickname = strings.TrimSpace(msg.User.Nick)
	}
	result := map[string]any{
		"nickname": nickname,
		"sex":      "unknown",
		"age":      0,
	}
	if mappedUserID, err := service.GetOrCreateOneBotID(service.OneBotEntityUser, userID); err == nil && mappedUserID > 0 {
		result["user_id"] = mappedUserID
	}
	if channel != nil && !channel.IsPrivate && !strings.EqualFold(channel.PermType, "private") {
		result["card"] = card
		if protocolRole == "" {
			protocolRole = service.ResolveMemberRoleForProtocol(userID, channel.ID, channel.WorldID)
		}
		result["role"] = protocolRole
		result["title"] = ""
		result["area"] = ""
		result["level"] = ""
	}
	return result
}

func buildOneBotSenderFromEvent(event *protocol.Event, userID string) map[string]any {
	channel := &model.ChannelModel{}
	if event.Channel != nil {
		channel.ID = event.Channel.ID
		channel.Name = event.Channel.Name
		if event.Channel.Type == protocol.DirectChannelType {
			channel.IsPrivate = true
			channel.PermType = "private"
		}
	}
	msg := event.Message
	if msg != nil && event.User != nil {
		msgCopy := *msg
		userCopy := *event.User
		msgCopy.User = &userCopy
		msg = &msgCopy
	}
	return buildOneBotSender(channel, msg, userID)
}

func oneBotMessageTypeByChannel(channel *model.ChannelModel) string {
	if channel != nil && (channel.IsPrivate || strings.EqualFold(channel.PermType, "private")) {
		return "private"
	}
	return "group"
}
