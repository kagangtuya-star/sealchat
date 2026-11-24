package api

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"sync"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

// Each channel has at most one authorized chat connection evaluating its live
// messages. The candidates are ordered by registration; a handoff starts a new
// counter scope. This state has the same process lifetime as the chat gateway.
type presentationControllerRoom struct {
	candidates []*WsSyncConn
	generation string
	recent     map[string]presentationMessageGrant
	order      []string
}

type presentationMessageGrant struct {
	generation string
	played     map[string]struct{}
}

type presentationControllers struct {
	sync.Mutex
	rooms map[string]*presentationControllerRoom
}

var worldPresentationControllers = presentationControllers{rooms: make(map[string]*presentationControllerRoom)}

func (p *presentationControllers) set(channelID string, conn *WsSyncConn, enabled bool) {
	p.Lock()
	defer p.Unlock()
	room := p.rooms[channelID]
	if enabled {
		if room == nil {
			room = &presentationControllerRoom{generation: utils.NewID(), recent: make(map[string]presentationMessageGrant)}
			p.rooms[channelID] = room
		}
		if !slices.Contains(room.candidates, conn) {
			room.candidates = append(room.candidates, conn)
		}
		return
	}
	if room == nil {
		return
	}
	if index := slices.Index(room.candidates, conn); index >= 0 {
		room.candidates = slices.Delete(room.candidates, index, index+1)
		if len(room.candidates) == 0 {
			delete(p.rooms, channelID)
		} else if index == 0 {
			room.generation = utils.NewID()
		}
	}
}

// Mark only the controller's copy of an actual public message-created event.
func (p *presentationControllers) grant(channelID string, event *protocol.Event) (*WsSyncConn, string) {
	if event.Type != protocol.EventMessageCreated || event.Channel == nil || event.Channel.WorldID == "" ||
		event.Message == nil || event.Message.ID == "" || event.Message.IsArchived || event.Message.IsDeleted ||
		eventContainsWhisper(event) || (event.MessageContext != nil && event.MessageContext.IsHiddenDice) {
		return nil, ""
	}
	p.Lock()
	defer p.Unlock()
	room := p.rooms[channelID]
	if room == nil {
		return nil, ""
	}
	messageID := event.Message.ID
	if _, exists := room.recent[messageID]; exists {
		return nil, ""
	}
	room.recent[messageID] = presentationMessageGrant{generation: room.generation, played: make(map[string]struct{})}
	room.order = append(room.order, messageID)
	if len(room.order) > 256 {
		delete(room.recent, room.order[0])
		room.order = room.order[1:]
	}
	return room.candidates[0], room.generation
}

func (p *presentationControllers) accept(channelID string, conn *WsSyncConn, messageID, generation, executionID string) bool {
	p.Lock()
	defer p.Unlock()
	room := p.rooms[channelID]
	if room == nil || room.candidates[0] != conn || room.generation != generation {
		return false
	}
	grant, exists := room.recent[messageID]
	if !exists || grant.generation != generation {
		return false
	}
	if _, played := grant.played[executionID]; played {
		return false
	}
	grant.played[executionID] = struct{}{}
	return true
}

type worldPresentationChannel struct {
	WorldID   string `json:"worldId"`
	ChannelID string `json:"channelId"`
}

func canControlWorldPresentation(ctx *ChatContext, scope worldPresentationChannel) bool {
	if ctx.IsReadOnly() || ctx.User == nil || ctx.User.IsBot || ctx.ConnInfo == nil ||
		scope.WorldID == "" || scope.ChannelID == "" || ctx.ConnInfo.ChannelId != scope.ChannelID ||
		ctx.ConnInfo.currentWorldID() != scope.WorldID {
		return false
	}
	if !service.IsWorldAdmin(scope.WorldID, ctx.User.ID) ||
		!pm.CanWithChannelRole(ctx.User.ID, scope.ChannelID, pm.PermFuncChannelRead, pm.PermFuncChannelReadAll) {
		return false
	}
	channel, err := model.ChannelGet(scope.ChannelID)
	return err == nil && channel != nil && channel.WorldID == scope.WorldID && !service.IsChannelDeletedForAccess(channel)
}

func apiWorldPresentationControl(ctx *ChatContext, input *struct {
	worldPresentationChannel
	Enabled bool `json:"enabled"`
}) (any, error) {
	if input == nil || ctx.ConnInfo == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	if !input.Enabled {
		if ctx.ConnInfo.ChannelId == input.ChannelID && ctx.ConnInfo.currentWorldID() == input.WorldID {
			worldPresentationControllers.set(input.ChannelID, ctx.Conn, false)
		}
		return nil, nil
	}
	if !canControlWorldPresentation(ctx, input.worldPresentationChannel) {
		return nil, errors.New("PERMISSION_DENIED")
	}
	worldPresentationControllers.set(input.ChannelID, ctx.Conn, true)
	return nil, nil
}

func apiWorldPresentationPlay(ctx *ChatContext, input *struct {
	worldPresentationChannel
	Revision   string `json:"revision"`
	Kind       string `json:"kind"`
	TargetID   string `json:"targetId"`
	MessageID  string `json:"messageId"`
	Controller string `json:"controller"`
	RequestID  string `json:"requestId"`
}) (any, error) {
	if input == nil || len(input.TargetID) == 0 || len(input.TargetID) > 512 || len(input.MessageID) > 128 ||
		len(input.Controller) > 128 || len(input.RequestID) > 128 || (input.MessageID == "" && input.RequestID == "") {
		return nil, errors.New("INVALID_PARAMS")
	}
	if !canControlWorldPresentation(ctx, input.worldPresentationChannel) {
		return nil, errors.New("PERMISSION_DENIED")
	}
	settings, err := service.ReadWorldPresentation(input.WorldID)
	if err != nil || settings.Revision != input.Revision || !settings.CanPlay(input.Kind, input.TargetID) {
		return nil, errors.New("PRESENTATION_UNAVAILABLE")
	}
	source := "manual:" + ctx.User.ID + ":" + input.RequestID
	if input.MessageID != "" {
		source = "message:" + input.MessageID
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(input.ChannelID+"\x00"+source+"\x00"+input.Kind+"\x00"+input.TargetID)))
	if input.MessageID != "" && !worldPresentationControllers.accept(input.ChannelID, ctx.Conn, input.MessageID, input.Controller, key) {
		return nil, nil
	}
	ctx.BroadcastEventInChannel(input.ChannelID, &protocol.Event{
		Type:    protocol.EventWorldPresentationPlay,
		Channel: &protocol.Channel{ID: input.ChannelID, WorldID: input.WorldID},
		WorldPresentationPlayback: &protocol.WorldPresentationPlayback{
			WorldID: input.WorldID, ChannelID: input.ChannelID, Revision: settings.Revision,
			Kind: input.Kind, TargetID: input.TargetID, ExecutionID: key,
		},
	})
	return nil, nil
}
