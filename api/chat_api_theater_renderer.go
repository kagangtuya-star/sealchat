package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"sealchat/protocol"
	"sealchat/service"
)

func theaterRendererConnectionID(info *ConnInfo) string { return fmt.Sprintf("%p", info) }
func theaterRendererContext(ctx *ChatContext, s service.TheaterScope) error {
	if ctx == nil || ctx.ConnInfo == nil || ctx.User == nil || ctx.User.IsBot || ctx.IsObserver() {
		return errors.New("renderer requires authenticated user")
	}
	subscription, _ := ctx.ConnInfo.theaterState()
	if subscription == nil || subscription.WorldID != s.WorldID || subscription.ChannelID != s.ChannelID || ctx.ConnInfo.WorldId != s.WorldID {
		return errors.New("renderer scope does not match active theater subscription")
	}
	if s.InputChannelID != "" && s.InputChannelID != ctx.ConnInfo.ChannelId {
		return errors.New("renderer inputChannelId does not match active channel")
	}
	return nil
}
func apiTheaterRendererRegisterWs(ctx *ChatContext, msg []byte) {
	var envelope struct {
		Data service.TheaterRendererRegistration `json:"data"`
	}
	if len(msg) > 160<<10 || json.Unmarshal(msg, &envelope) != nil {
		writeTheaterWSResponse(ctx, nil, errors.New("invalid renderer registration"))
		return
	}
	r := envelope.Data
	if err := theaterRendererContext(ctx, r.TheaterScope); err != nil {
		writeTheaterWSResponse(ctx, nil, err)
		return
	}
	info := ctx.ConnInfo
	err := service.TheaterRenderers.Register(ctx.User.ID, theaterRendererConnectionID(info), r, func(command service.TheaterRendererCommand) error {
		subscription, queue := info.theaterState()
		if subscription == nil || queue == nil || subscription.WorldID != command.Scope.WorldID || subscription.ChannelID != command.Scope.ChannelID {
			return errors.New("renderer disconnected")
		}
		event := theaterGatewayEvent(protocol.EventTheaterRendererCommand, command.Scope.WorldID, command.Scope.ChannelID, "", command.ExpectedRevision, command.RequestID, command)
		if !queue.Enqueue(event, nil) {
			return errors.New("renderer command queue unavailable")
		}
		return nil
	})
	writeTheaterWSResponse(ctx, map[string]any{"rendererId": r.RendererID, "registered": err == nil, "ttlMs": 45000}, err)
}
func apiTheaterRendererReplyWs(ctx *ChatContext, msg []byte) {
	var envelope struct {
		Data service.TheaterRendererReply `json:"data"`
	}
	if len(msg) > 3<<20 || json.Unmarshal(msg, &envelope) != nil {
		writeTheaterWSResponse(ctx, nil, errors.New("invalid renderer reply"))
		return
	}
	if err := theaterRendererContext(ctx, envelope.Data.Scope); err != nil {
		writeTheaterWSResponse(ctx, nil, err)
		return
	}
	err := service.TheaterRenderers.Reply(ctx.User.ID, theaterRendererConnectionID(ctx.ConnInfo), envelope.Data)
	writeTheaterWSResponse(ctx, map[string]any{"accepted": err == nil}, err)
}
func apiTheaterRendererStepWs(ctx *ChatContext, msg []byte) {
	var envelope struct {
		Data struct {
			service.TheaterRendererReply
			StepID string `json:"stepId"`
		} `json:"data"`
	}
	if len(msg) > 4096 || json.Unmarshal(msg, &envelope) != nil {
		writeTheaterWSResponse(ctx, nil, errors.New("invalid renderer step"))
		return
	}
	if err := theaterRendererContext(ctx, envelope.Data.Scope); err != nil {
		writeTheaterWSResponse(ctx, nil, err)
		return
	}
	callCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := service.TheaterRenderers.Step(callCtx, ctx.User.ID, theaterRendererConnectionID(ctx.ConnInfo), envelope.Data.TheaterRendererReply, envelope.Data.StepID)
	// Echo responses contain no bearer credential and never execute as ctx.User.
	writeTheaterWSResponse(ctx, result, err)
}
func apiTheaterRendererUnregisterWs(ctx *ChatContext, _ []byte) {
	if ctx != nil && ctx.ConnInfo != nil {
		service.TheaterRenderers.Disconnect(theaterRendererConnectionID(ctx.ConnInfo))
	}
	writeTheaterWSResponse(ctx, map[string]any{"registered": false}, nil)
}
