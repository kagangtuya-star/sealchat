package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/samber/lo"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/protocol/onebotv11"
	"sealchat/service"
	onebot "sealchat/service/onebot"
	"sealchat/utils"
)

func init() {
	onebot.SetDispatcher(&oneBotDispatcher{})
}

type oneBotDispatcher struct{}

func (d *oneBotDispatcher) HandleAction(ctx context.Context, profile *model.BotProfileModel, frame *onebotv11.ActionFrame) (*onebotv11.ActionResponse, error) {
	switch frame.Action {
	case "send_group_msg":
		return d.handleSendGroupMessage(profile, frame)
	case "send_private_msg":
		return d.handleSendPrivateMessage(profile, frame)
	case "get_group_info":
		return d.handleGetGroupInfo(profile, frame)
	default:
		return onebotv11.NewErrorResponse(frame.Echo, 1404, fmt.Sprintf("unsupported action: %s", frame.Action)), nil
	}
}

func (d *oneBotDispatcher) HandleEvent(ctx context.Context, profile *model.BotProfileModel, event *onebotv11.Event) error {
	if event == nil || profile == nil {
		return nil
	}
	log.Printf("onebot event recv bot=%s postType=%s msgType=%s group=%s channel=%s user=%s", profile.ID, event.PostType, event.MessageType, event.GroupID.String(), event.ChannelID.String(), event.UserID.String())
	if !strings.EqualFold(event.PostType, "message") {
		return nil
	}
	content := strings.TrimSpace(event.RawMessage)
	if content == "" {
		content = segmentsToPlainText(event.Message)
	}
	if content == "" {
		log.Printf("onebot event ignored: empty content bot=%s user=%s", profile.ID, event.UserID.String())
		return nil
	}
	if strings.TrimSpace(event.UserID.String()) == "" || event.UserID.String() == profile.UserID {
		log.Printf("onebot event ignored: invalid sender bot=%s user=%s", profile.ID, event.UserID.String())
		return nil
	}
	nickname := ""
	if event.Sender != nil {
		nickname = coalesce(event.Sender.Nickname, event.Sender.Card)
	}
	remoteUser, err := service.EnsureOneBotExternalUser(profile, event.UserID.String(), nickname)
	if err != nil {
		log.Printf("onebot ensure external user failed bot=%s user=%s err=%v", profile.ID, event.UserID.String(), err)
		return err
	}

	var (
		channelID string
		groupID   string
	)
	switch strings.ToLower(event.MessageType) {
	case "group":
		groupID = coalesce(event.GroupID.String(), event.ChannelID.String())
		channelID, err = resolveChannelForGroup(profile.ID, "", groupID)
	default:
		channelID, err = resolveChannelForPrivateEvent(profile, remoteUser, event)
	}
	if err != nil {
		log.Printf("onebot resolve channel failed bot=%s user=%s group=%s err=%v", profile.ID, event.UserID.String(), groupID, err)
		return err
	}
	log.Printf("onebot event mapped bot=%s channel=%s remoteUser=%s", profile.ID, channelID, remoteUser.ID)
	if err := service.EnsureOneBotChannelAccess(channelID, remoteUser, nickname); err != nil {
		log.Printf("onebot ensure channel access failed bot=%s channel=%s user=%s err=%v", profile.ID, channelID, remoteUser.ID, err)
		return err
	}

	ctxSim := &ChatContext{
		User:            remoteUser,
		ChannelUsersMap: channelUsersMapGlobal,
		UserId2ConnInfo: userId2ConnInfoGlobal,
		Echo:            "onebot:event",
	}
	payload := &struct {
		ChannelID  string `json:"channel_id"`
		QuoteID    string `json:"quote_id"`
		Content    string `json:"content"`
		WhisperTo  string `json:"whisper_to"`
		ClientID   string `json:"client_id"`
		IdentityID string `json:"identity_id"`
		ICMode     string `json:"ic_mode"`
	}{
		ChannelID: channelID,
		Content:   content,
		ICMode:    "ic",
		ClientID:  fmt.Sprintf("onebot:event:%s", utils.NewID()),
	}

	log.Printf("onebot event writing message bot=%s channel=%s as user=%s", profile.ID, channelID, remoteUser.ID)
	_, err = apiMessageCreate(ctxSim, payload)
	if err != nil {
		log.Printf("onebot event message create failed bot=%s channel=%s err=%v", profile.ID, channelID, err)
	} else {
		log.Printf("onebot event message created bot=%s channel=%s", profile.ID, channelID)
	}
	return err
}

func (d *oneBotDispatcher) handleSendGroupMessage(profile *model.BotProfileModel, frame *onebotv11.ActionFrame) (*onebotv11.ActionResponse, error) {
	var params struct {
		GroupID    string          `json:"group_id"`
		ChannelID  string          `json:"channel_id"`
		Message    json.RawMessage `json:"message"`
		AutoEscape bool            `json:"auto_escape"`
		Content    string          `json:"content"`
	}
	if err := json.Unmarshal(frame.Params, &params); err != nil {
		return onebotv11.NewErrorResponse(frame.Echo, 1402, "invalid params"), nil
	}
	content := pickMessageContent(params.Message, params.Content)
	if strings.TrimSpace(content) == "" {
		return onebotv11.NewErrorResponse(frame.Echo, 1403, "message content required"), nil
	}
	log.Printf("onebot action send_group_msg bot=%s group=%s channelPref=%s", profile.ID, params.GroupID, params.ChannelID)

	channelID, err := resolveChannelForGroup(profile.ID, params.ChannelID, params.GroupID)
	if err != nil {
		log.Printf("onebot action send_group_msg resolve failed bot=%s group=%s err=%v", profile.ID, params.GroupID, err)
		return onebotv11.NewErrorResponse(frame.Echo, 1500, err.Error()), nil
	}
	log.Printf("onebot action send_group_msg resolved bot=%s group=%s channel=%s", profile.ID, params.GroupID, channelID)

	resultMsg, err := createMessageAsBot(profile, channelID, content, "")
	if err != nil {
		log.Printf("onebot action send_group_msg create message failed bot=%s channel=%s err=%v", profile.ID, channelID, err)
		return onebotv11.NewErrorResponse(frame.Echo, 1500, err.Error()), nil
	}
	log.Printf("onebot action send_group_msg message created bot=%s channel=%s message=%s", profile.ID, channelID, resultMsg.ID)
	mapping, err := model.EnsureBotMessageMapping(profile.ID, channelID, resultMsg.ID)
	if err != nil {
		log.Printf("onebot action send_group_msg mapping failed bot=%s channel=%s message=%s err=%v", profile.ID, channelID, resultMsg.ID, err)
		return onebotv11.NewErrorResponse(frame.Echo, 1500, "failed to record message mapping"), nil
	}
	return onebotv11.NewOKResponse(frame.Echo, map[string]int64{
		"message_id": mapping.ID,
	}), nil
}

func (d *oneBotDispatcher) handleGetGroupInfo(profile *model.BotProfileModel, frame *onebotv11.ActionFrame) (*onebotv11.ActionResponse, error) {
	var params struct {
		GroupID   string `json:"group_id"`
		ChannelID string `json:"channel_id"`
	}
	if err := json.Unmarshal(frame.Params, &params); err != nil {
		return onebotv11.NewErrorResponse(frame.Echo, 1402, "invalid params"), nil
	}
	remoteID := service.NormalizeOneBotNumericID(params.GroupID)
	if remoteID == "" {
		remoteID = service.NormalizeOneBotNumericID(params.ChannelID)
	}
	if remoteID == "" {
		return onebotv11.NewErrorResponse(frame.Echo, 1403, "group_id required"), nil
	}
	binding, err := findBindingByRemoteNumeric(profile.ID, remoteID)
	if err != nil {
		return nil, err
	}
	if binding == nil {
		return onebotv11.NewErrorResponse(frame.Echo, 1404, "group not found"), nil
	}
	channel, _ := model.ChannelGet(binding.ChannelID)
	groupName := ""
	if channel != nil {
		groupName = channel.Name
	}
	memberCount := int64(0)
	if binding.ChannelID != "" {
		model.GetDB().Model(&model.MemberModel{}).Where("channel_id = ?", binding.ChannelID).Count(&memberCount)
	}
	resp := map[string]interface{}{
		"group_id":         remoteID,
		"group_name":       groupName,
		"member_count":     memberCount,
		"max_member_count": memberCount,
	}
	return onebotv11.NewOKResponse(frame.Echo, resp), nil
}

func (d *oneBotDispatcher) handleSendPrivateMessage(profile *model.BotProfileModel, frame *onebotv11.ActionFrame) (*onebotv11.ActionResponse, error) {
	var params struct {
		UserID    string          `json:"user_id"`
		ChannelID string          `json:"channel_id"`
		Message   json.RawMessage `json:"message"`
		Content   string          `json:"content"`
	}
	if err := json.Unmarshal(frame.Params, &params); err != nil {
		return onebotv11.NewErrorResponse(frame.Echo, 1402, "invalid params"), nil
	}
	content := pickMessageContent(params.Message, params.Content)
	if strings.TrimSpace(content) == "" {
		return onebotv11.NewErrorResponse(frame.Echo, 1403, "message content required"), nil
	}
	channelID, err := resolveChannelForPrivate(profile.ID, params.ChannelID, params.UserID)
	if err != nil {
		log.Printf("onebot action send_private_msg resolve failed bot=%s user=%s err=%v", profile.ID, params.UserID, err)
		return onebotv11.NewErrorResponse(frame.Echo, 1500, err.Error()), nil
	}
	log.Printf("onebot action send_private_msg resolved bot=%s user=%s channel=%s", profile.ID, params.UserID, channelID)
	resultMsg, err := createMessageAsBot(profile, channelID, content, params.UserID)
	if err != nil {
		log.Printf("onebot action send_private_msg create message failed bot=%s channel=%s err=%v", profile.ID, channelID, err)
		return onebotv11.NewErrorResponse(frame.Echo, 1500, err.Error()), nil
	}
	log.Printf("onebot action send_private_msg message created bot=%s channel=%s msg=%s", profile.ID, channelID, resultMsg.ID)
	mapping, err := model.EnsureBotMessageMapping(profile.ID, channelID, resultMsg.ID)
	if err != nil {
		log.Printf("onebot action send_private_msg mapping failed bot=%s channel=%s msg=%s err=%v", profile.ID, channelID, resultMsg.ID, err)
		return onebotv11.NewErrorResponse(frame.Echo, 1500, "failed to record message mapping"), nil
	}
	return onebotv11.NewOKResponse(frame.Echo, map[string]int64{
		"message_id": mapping.ID,
	}), nil
}

func pickMessageContent(raw json.RawMessage, fallback string) string {
	if len(raw) == 0 {
		return fallback
	}
	var plain string
	if err := json.Unmarshal(raw, &plain); err == nil {
		return plain
	}
	var segments []onebotv11.MessageSegment
	if err := json.Unmarshal(raw, &segments); err == nil && len(segments) > 0 {
		parts := lo.Map(segments, func(seg onebotv11.MessageSegment, _ int) string {
			if strings.EqualFold(seg.Type, "text") {
				return seg.Data["text"]
			}
			return ""
		})
		return strings.TrimSpace(strings.Join(parts, ""))
	}
	return fallback
}

func pickGroupIdentifier(event *onebotv11.Event) string {
	if event == nil {
		return ""
	}
	if strings.TrimSpace(event.GroupID.String()) != "" {
		return event.GroupID.String()
	}
	return strings.TrimSpace(event.ChannelID.String())
}

func resolveChannelForGroup(botID, preferredChannel, groupID string) (string, error) {
	if strings.TrimSpace(preferredChannel) != "" {
		return preferredChannel, nil
	}
	target := strings.TrimSpace(groupID)
	if target == "" {
		return "", fmt.Errorf("group_id required")
	}
	bindings, err := model.BotChannelBindingsByBotID(botID)
	if err != nil {
		return "", err
	}
	for _, binding := range bindings {
		if binding == nil || !binding.Enabled {
			continue
		}
		if matchGroupBinding(binding, target) {
			return binding.ChannelID, nil
		}
	}
	normTarget := service.NormalizeOneBotRemoteID(target)
	for _, binding := range bindings {
		if binding == nil {
			continue
		}
		log.Printf("onebot resolveChannelForGroup miss bot=%s binding=%s enabled=%v normTarget=%s remoteGroup=%s remoteNumeric=%s remoteChannel=%s channel=%s",
			botID, binding.ID, binding.Enabled, normTarget,
			normalizedOrEmpty(binding.RemoteGroupID), normalizedOrEmpty(binding.RemoteNumericID), normalizedOrEmpty(binding.RemoteChannelID), binding.ChannelID)
	}
	return "", fmt.Errorf("no channel bound with group_id %s", groupID)
}

func resolveChannelForPrivate(botID, preferredChannel, remoteUser string) (string, error) {
	if strings.TrimSpace(preferredChannel) != "" {
		return preferredChannel, nil
	}
	target := strings.TrimSpace(remoteUser)
	if target == "" {
		return "", fmt.Errorf("user_id required")
	}
	bindings, err := model.BotChannelBindingsByBotID(botID)
	if err != nil {
		return "", err
	}
	for _, binding := range bindings {
		if binding == nil || !binding.Enabled {
			continue
		}
		if matchPrivateBinding(binding, target) {
			return binding.ChannelID, nil
		}
	}
	normTarget := service.NormalizeOneBotRemoteID(target)
	for _, binding := range bindings {
		if binding == nil {
			continue
		}
		log.Printf("onebot resolveChannelForPrivate miss bot=%s binding=%s enabled=%v normTarget=%s remoteChannel=%s channel=%s",
			botID, binding.ID, binding.Enabled, normTarget,
			normalizedOrEmpty(binding.RemoteChannelID), binding.ChannelID)
	}
	return "", fmt.Errorf("no channel bound with user_id %s", remoteUser)
}

func normalizedOrEmpty(val string) string {
	norm := service.NormalizeOneBotRemoteID(val)
	if norm != "" {
		return norm
	}
	return service.NormalizeOneBotNumericID(val)
}

func matchGroupBinding(binding *model.BotChannelBindingModel, target string) bool {
	if binding == nil {
		return false
	}
	normTarget := service.NormalizeOneBotRemoteID(target)
	numericTarget := service.NormalizeOneBotNumericID(target)
	if normTarget == "" && numericTarget == "" {
		return false
	}
	if binding.RemoteGroupID != "" {
		normGroup := service.NormalizeOneBotRemoteID(binding.RemoteGroupID)
		if normGroup != "" && strings.EqualFold(normGroup, normTarget) {
			return true
		}
	}
	if binding.RemoteChannelID != "" {
		normChannel := service.NormalizeOneBotRemoteID(binding.RemoteChannelID)
		if normChannel != "" && strings.EqualFold(normChannel, normTarget) {
			return true
		}
	}
	if binding.RemoteNumericID != "" {
		normNumeric := service.NormalizeOneBotNumericID(binding.RemoteNumericID)
		if normNumeric != "" {
			if numericTarget != "" && strings.EqualFold(normNumeric, numericTarget) {
				return true
			}
			if normTarget != "" && strings.EqualFold(normNumeric, normTarget) {
				return true
			}
		}
	}
	normChannelID := service.NormalizeOneBotRemoteID(binding.ChannelID)
	if normChannelID != "" && strings.EqualFold(normChannelID, normTarget) {
		return true
	}
	if normTarget != "" && strings.EqualFold(binding.ChannelID, normTarget) {
		return true
	}
	if numericTarget != "" {
		normChannelNumeric := service.NormalizeOneBotNumericID(binding.ChannelID)
		return normChannelNumeric != "" && strings.EqualFold(normChannelNumeric, numericTarget)
	}
	return false
}

func matchPrivateBinding(binding *model.BotChannelBindingModel, target string) bool {
	if binding == nil {
		return false
	}
	normTarget := service.NormalizeOneBotRemoteID(target)
	numericTarget := service.NormalizeOneBotNumericID(target)
	if normTarget == "" && numericTarget == "" {
		return false
	}
	if binding.RemoteChannelID != "" {
		normRemote := service.NormalizeOneBotRemoteID(binding.RemoteChannelID)
		if normRemote != "" && strings.EqualFold(normRemote, normTarget) {
			return true
		}
		normRemoteNumeric := service.NormalizeOneBotNumericID(binding.RemoteChannelID)
		if normRemoteNumeric != "" && numericTarget != "" && strings.EqualFold(normRemoteNumeric, numericTarget) {
			return true
		}
	}
	normChannelID := service.NormalizeOneBotRemoteID(binding.ChannelID)
	if normChannelID != "" && strings.EqualFold(normChannelID, normTarget) {
		return true
	}
	if numericTarget != "" {
		normChannelNumeric := service.NormalizeOneBotNumericID(binding.ChannelID)
		if normChannelNumeric != "" && strings.EqualFold(normChannelNumeric, numericTarget) {
			return true
		}
	}
	return normTarget != "" && strings.EqualFold(binding.ChannelID, normTarget)
}

func resolveChannelForPrivateEvent(profile *model.BotProfileModel, remoteUser *model.UserModel, event *onebotv11.Event) (string, error) {
	if profile == nil || remoteUser == nil {
		return "", fmt.Errorf("profile or user missing")
	}
	bindings, err := model.BotChannelBindingsByBotID(profile.ID)
	if err != nil {
		return "", err
	}
	for _, binding := range bindings {
		if binding == nil || !binding.Enabled {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(binding.RemoteChannelID), strings.TrimSpace(event.UserID.String())) {
			return binding.ChannelID, nil
		}
	}
	return service.EnsureOneBotPrivateChannel(profile.UserID, remoteUser.ID)
}

func findBindingByRemoteNumeric(botID, remoteNumeric string) (*model.BotChannelBindingModel, error) {
	numeric := service.NormalizeOneBotNumericID(remoteNumeric)
	if numeric == "" {
		return nil, nil
	}
	bindings, err := model.BotChannelBindingsByBotID(botID)
	if err != nil {
		return nil, err
	}
	for _, binding := range bindings {
		if binding == nil {
			continue
		}
		candidates := []string{
			service.NormalizeOneBotNumericID(binding.RemoteNumericID),
			service.NormalizeOneBotNumericID(binding.RemoteGroupID),
			service.NormalizeOneBotNumericID(binding.RemoteChannelID),
		}
		for _, candidate := range candidates {
			if candidate != "" && candidate == numeric {
				return binding, nil
			}
		}
	}
	return nil, nil
}

func createMessageAsBot(profile *model.BotProfileModel, channelID, content, whisperTo string) (*protocol.Message, error) {
	botUser := model.UserGet(profile.UserID)
	if botUser == nil {
		return nil, fmt.Errorf("bot user missing")
	}
	ctx := &ChatContext{
		User:            botUser,
		ChannelUsersMap: channelUsersMapGlobal,
		UserId2ConnInfo: userId2ConnInfoGlobal,
		Echo:            "onebot",
	}
	payload := &struct {
		ChannelID  string `json:"channel_id"`
		QuoteID    string `json:"quote_id"`
		Content    string `json:"content"`
		WhisperTo  string `json:"whisper_to"`
		ClientID   string `json:"client_id"`
		IdentityID string `json:"identity_id"`
		ICMode     string `json:"ic_mode"`
	}{
		ChannelID: channelID,
		Content:   content,
		WhisperTo: whisperTo,
		ICMode:    "ic",
		ClientID:  fmt.Sprintf("onebot:%s", utils.NewID()),
	}

	result, err := apiMessageCreate(ctx, payload)
	if err != nil {
		return nil, err
	}
	messageData, ok := result.(*protocol.Message)
	if !ok {
		return nil, fmt.Errorf("message creation failed")
	}
	return messageData, nil
}

func segmentsToPlainText(segments []onebotv11.MessageSegment) string {
	if len(segments) == 0 {
		return ""
	}
	builder := strings.Builder{}
	for _, seg := range segments {
		if strings.EqualFold(seg.Type, "text") {
			builder.WriteString(seg.Data["text"])
		}
	}
	return strings.TrimSpace(builder.String())
}

func coalesce(values ...string) string {
	for _, val := range values {
		if trimmed := strings.TrimSpace(val); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
