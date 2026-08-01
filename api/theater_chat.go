package api

import (
	"context"
	"errors"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
)

type LocalTheaterChatSender struct{}

func (LocalTheaterChatSender) SendTheaterChat(_ context.Context, request service.TheaterChatSendRequest) (*service.TheaterChatSendResult, error) {
	if channelUsersMapGlobal == nil || userId2ConnInfoGlobal == nil {
		return nil, errors.New("chat connection registry unavailable")
	}
	var user model.UserModel
	if err := model.GetDB().Where("id = ?", request.ActorID).First(&user).Error; err != nil {
		return nil, err
	}
	var members []*model.MemberModel
	if err := model.GetDB().Where("user_id = ?", request.ActorID).Find(&members).Error; err != nil {
		return nil, err
	}
	ctx := &ChatContext{
		User: &user, Members: members, Echo: request.ClientID,
		ChannelUsersMap: channelUsersMapGlobal, UserId2ConnInfo: userId2ConnInfoGlobal,
	}
	value, err := apiMessageCreate(ctx, &struct {
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
		ChannelID: request.ChannelID, Content: request.Content, ClientID: request.ClientID,
		IdentityID: request.IdentityID, IdentityVariantID: request.IdentityVariantID, ICMode: request.ICMode,
	})
	if err != nil {
		return nil, err
	}
	message, ok := value.(*protocol.Message)
	if !ok || message == nil || message.ID == "" {
		return nil, errors.New("chat message send denied")
	}
	return &service.TheaterChatSendResult{MessageID: message.ID}, nil
}
