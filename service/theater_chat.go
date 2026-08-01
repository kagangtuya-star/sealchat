package service

import (
	"context"
	"errors"
	"strings"
	"sync"
)

type TheaterChatSendRequest struct {
	ActorID           string
	WorldID           string
	ChannelID         string
	ClientID          string
	Content           string
	IdentityID        string
	IdentityVariantID string
	ICMode            string
}

type TheaterChatSendResult struct {
	MessageID string `json:"messageId"`
}

type TheaterChatSender interface {
	SendTheaterChat(context.Context, TheaterChatSendRequest) (*TheaterChatSendResult, error)
}

var theaterChatSenderState = struct {
	sync.RWMutex
	sender TheaterChatSender
}{}

func SetTheaterChatSender(sender TheaterChatSender) {
	theaterChatSenderState.Lock()
	theaterChatSenderState.sender = sender
	theaterChatSenderState.Unlock()
}

func getTheaterChatSender() TheaterChatSender {
	theaterChatSenderState.RLock()
	defer theaterChatSenderState.RUnlock()
	return theaterChatSenderState.sender
}

type theaterChatSendPayload struct {
	Content           string `json:"content"`
	ChannelID         string `json:"channelId"`
	IdentityID        string `json:"identityId"`
	IdentityVariantID string `json:"identityVariantId"`
	ICMode            string `json:"icMode"`
}

func normalizeTheaterChatSendPayload(payload theaterChatSendPayload) (theaterChatSendPayload, error) {
	payload.Content = strings.TrimSpace(payload.Content)
	payload.ChannelID = strings.TrimSpace(payload.ChannelID)
	payload.IdentityID = strings.TrimSpace(payload.IdentityID)
	payload.IdentityVariantID = strings.TrimSpace(payload.IdentityVariantID)
	payload.ICMode = strings.ToLower(strings.TrimSpace(payload.ICMode))
	if payload.Content == "" || len(payload.Content) > 10000 {
		return payload, theaterPayloadError("chat.send content 无效")
	}
	if payload.ICMode == "" {
		payload.ICMode = "ic"
	}
	if payload.ICMode != "ic" && payload.ICMode != "ooc" {
		return payload, theaterPayloadError("chat.send icMode 无效")
	}
	if err := rejectUnsafeTheaterJSON(payload.Content); err != nil {
		return payload, err
	}
	return payload, nil
}

func sendTheaterChat(ctx context.Context, actorID, worldID, channelID, actionRequestID string, raw []byte) (*TheaterChatSendResult, error) {
	var payload theaterChatSendPayload
	if err := decodeStrictJSON(raw, &payload); err != nil {
		return nil, theaterPayloadError(err.Error())
	}
	payload, err := normalizeTheaterChatSendPayload(payload)
	if err != nil {
		return nil, err
	}
	if payload.ChannelID != "" {
		channelID = payload.ChannelID
	}
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, theaterPayloadError("chat.send 缺少 inputChannelId")
	}
	if _, _, err := resolveTheaterScope(worldID, channelID); err != nil {
		return nil, err
	}
	sender := getTheaterChatSender()
	if sender == nil {
		return nil, errors.New("theater chat sender unavailable")
	}
	return sender.SendTheaterChat(ctx, TheaterChatSendRequest{
		ActorID: actorID, WorldID: worldID, ChannelID: channelID, ClientID: "theater:" + actionRequestID,
		Content: payload.Content, IdentityID: payload.IdentityID, IdentityVariantID: payload.IdentityVariantID, ICMode: payload.ICMode,
	})
}
