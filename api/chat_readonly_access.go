package api

import (
	"fmt"
	"strings"

	"sealchat/model"
	"sealchat/service"
)

func isObserverWorldScoped(ctx *ChatContext, worldID string) bool {
	if ctx == nil || !ctx.IsObserver() {
		return false
	}
	observerWorldID := ctx.ObserverWorldID()
	if observerWorldID == "" {
		return false
	}
	return strings.TrimSpace(worldID) == observerWorldID
}

func listReadOnlyChannelsByWorld(ctx *ChatContext, worldID string) ([]*model.ChannelModel, error) {
	worldID = strings.TrimSpace(worldID)
	if worldID == "" {
		return nil, fmt.Errorf("未找到世界")
	}
	if isObserverWorldScoped(ctx, worldID) {
		return service.ChannelListByWorld(worldID)
	}
	world, err := service.GetWorldByID(worldID)
	if err != nil {
		return nil, err
	}
	if world == nil || strings.ToLower(strings.TrimSpace(world.Visibility)) != model.WorldVisibilityPublic {
		return nil, fmt.Errorf("世界未开放公开访问")
	}
	return service.ChannelListPublicByWorld(worldID)
}

func checkReadOnlyChannelAccess(ctx *ChatContext, channelID string) (*model.ChannelModel, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, fmt.Errorf("频道ID不能为空")
	}
	observerWorldID := ""
	if ctx != nil {
		observerWorldID = ctx.ObserverWorldID()
	}
	if ctx != nil && ctx.IsObserver() && observerWorldID != "" {
		return service.CanObserverAccessChannel(channelID, observerWorldID)
	}
	return service.CanGuestAccessChannel(channelID)
}
