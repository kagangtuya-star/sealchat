package api

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

const avatarBotMutationTimeout = 15 * time.Second

type avatarBotMutationStatusRequest struct {
	ChannelID  string `json:"channelId"`
	IdentityID string `json:"identityId"`
}

type avatarBotMutationDelegateRequest struct {
	ChannelID            string   `json:"channelId"`
	IdentityID           string   `json:"identityId"`
	ExpectedSourceCardID string   `json:"expectedSourceCardId"`
	StatID               string   `json:"statId"`
	Slot                 string   `json:"slot"`
	Op                   string   `json:"op"`
	Value                *float64 `json:"value"`
}

type avatarBotMutationRespondRequest struct {
	RequestID string `json:"requestId"`
	OK        bool   `json:"ok"`
	Error     string `json:"error"`
}

type avatarBotMutationPending struct {
	requestID    string
	targetUserID string
	channelID    string
	targetConn   *WsSyncConn
	result       chan error
}

type avatarBotMutationBroker struct {
	mu        sync.Mutex
	byRequest map[string]*avatarBotMutationPending
	byLane    map[string]*avatarBotMutationPending
}

var avatarBotMutations = &avatarBotMutationBroker{
	byRequest: make(map[string]*avatarBotMutationPending),
	byLane:    make(map[string]*avatarBotMutationPending),
}

func avatarBotMutationLane(userID, channelID string) string { return userID + "\x00" + channelID }

func (b *avatarBotMutationBroker) register(p *avatarBotMutationPending) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	lane := avatarBotMutationLane(p.targetUserID, p.channelID)
	if b.byLane[lane] != nil {
		return errors.New("AVATAR_CARD_BOT_MUTATION_BUSY")
	}
	b.byLane[lane], b.byRequest[p.requestID] = p, p
	return nil
}

func (b *avatarBotMutationBroker) finish(p *avatarBotMutationPending, result error) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.byRequest[p.requestID] != p {
		return false
	}
	delete(b.byRequest, p.requestID)
	delete(b.byLane, avatarBotMutationLane(p.targetUserID, p.channelID))
	p.result <- result
	return true
}

func (b *avatarBotMutationBroker) respond(requestID, userID string, conn *WsSyncConn, result error) error {
	b.mu.Lock()
	p := b.byRequest[requestID]
	b.mu.Unlock()
	if p == nil {
		return errors.New("NOT_EDITABLE")
	}
	if p.targetUserID != userID || p.targetConn != conn {
		return errors.New("PERMISSION_DENIED")
	}
	if !b.finish(p, result) {
		return errors.New("NOT_EDITABLE")
	}
	return nil
}

func avatarBotMutationTargetConnection(userID, channelID string) (*WsSyncConn, *utils.SyncMap[*WsSyncConn, *ConnInfo]) {
	if userId2ConnInfoGlobal == nil {
		return nil, nil
	}
	conns, ok := userId2ConnInfoGlobal.Load(userID)
	if !ok || conns == nil {
		return nil, nil
	}
	var best *WsSyncConn
	var latest int64 = -1
	now := time.Now().UnixMilli()
	conns.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
		if conn == nil || info == nil || info.User == nil || info.User.ID != userID || info.User.IsBot || info.IsGuest || info.IsObserver || info.ChannelId != channelID {
			return true
		}
		select {
		case <-conn.done:
			return true
		default:
		}
		alive := info.LastAliveTime
		if alive == 0 {
			alive = info.LastPingTime
		}
		if alive <= 0 || now-alive > int64(connectionMaxIdleSeconds)*1000 {
			return true
		}
		if alive > latest {
			latest, best = alive, conn
		}
		return true
	})
	return best, conns
}

// The BOT is queried live. An ambiguous name/type match is never treated as an ID.
func avatarBotMutationActiveCardID(channelID, userID string) string {
	botConn, info, err := findBotConnectionForChannel(nil, channelID)
	if err != nil || info == nil || info.BotCharacterSupport != BotCharacterSupportYes {
		return ""
	}
	request := map[string]string{"group_id": channelID, "user_id": userID}
	get := forwardCharacterRequest(botConn, "character.get", "avatar-card-get-"+utils.NewID(), request)
	var active struct {
		OK   bool   `json:"ok"`
		ID   string `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if json.Unmarshal(get, &active) != nil || !active.OK {
		return ""
	}
	if strings.TrimSpace(active.ID) != "" {
		return strings.TrimSpace(active.ID)
	}
	if active.Name == "" {
		return ""
	}
	list := forwardCharacterRequest(botConn, "character.list", "avatar-card-list-"+utils.NewID(), request)
	var cards struct {
		OK   bool `json:"ok"`
		List []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			SheetType string `json:"sheet_type"`
		} `json:"list"`
	}
	if json.Unmarshal(list, &cards) != nil || !cards.OK {
		return ""
	}
	matched := ""
	for _, card := range cards.List {
		if card.Name != active.Name || (active.Type != "" && card.SheetType != active.Type) {
			continue
		}
		if card.ID == "" || matched != "" {
			return ""
		}
		matched = card.ID
	}
	return matched
}

func avatarBotMutationCapability(channelID string) (bool, error) {
	capable, _ := GetChannelCharacterAPICapability(channelID, nil)
	if capable {
		return true, nil
	}

	channel, err := model.ChannelGet(strings.TrimSpace(channelID))
	if err != nil || channel == nil || (!channel.IsPrivate && !channel.BotFeatureEnabled) {
		return false, errors.New("NOT_EDITABLE")
	}
	_, info, err := findBotConnectionForChannel(nil, channelID)
	if err != nil || info == nil || info.User == nil || !info.User.IsBot {
		return false, errors.New("NOT_EDITABLE")
	}
	if info.BotCharacterProbeOn {
		startBotCharacterCapabilityProbe(info)
		return false, errors.New("BOT_CHARACTER_CAPABILITY_PENDING")
	}
	if info.BotCharacterSupport == BotCharacterSupportYes {
		return true, nil
	}
	if info.BotCharacterSupport == BotCharacterSupportNo {
		return false, errors.New("NOT_EDITABLE")
	}
	startBotCharacterCapabilityProbe(info)
	if info.BotCharacterSupport == BotCharacterSupportYes {
		return true, nil
	}
	if info.BotCharacterSupport == BotCharacterSupportNo {
		return false, errors.New("NOT_EDITABLE")
	}
	return false, errors.New("BOT_CHARACTER_CAPABILITY_PENDING")
}

func avatarBotMutationPreflight(ctx *ChatContext, channelID, identityID, expectedCardID string) (*service.AvatarBotMutationContext, error) {
	if ctx == nil || ctx.User == nil || ctx.IsReadOnly() {
		return nil, errors.New("PERMISSION_DENIED")
	}
	capable, err := avatarBotMutationCapability(channelID)
	if err != nil || !capable {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("NOT_EDITABLE")
	}
	resolved, err := service.AvatarBotMutationAuthorize(channelID, identityID, ctx.User.ID, expectedCardID, true)
	if err != nil {
		return nil, err
	}
	if conn, _ := avatarBotMutationTargetConnection(resolved.TargetUserID, channelID); conn == nil {
		return nil, errors.New("TARGET_OFFLINE")
	}
	activeCardID := avatarBotMutationActiveCardID(channelID, resolved.TargetUserID)
	if conn, _ := avatarBotMutationTargetConnection(resolved.TargetUserID, channelID); conn == nil {
		return nil, errors.New("TARGET_OFFLINE")
	}
	if activeCardID != resolved.SourceCardID {
		return nil, errors.New("CARD_CHANGED")
	}
	// Permission, source and snapshot may have changed during the live BOT round trip.
	return service.AvatarBotMutationAuthorize(channelID, identityID, ctx.User.ID, resolved.SourceCardID, true)
}

func avatarBotMutationReply(conn *WsSyncConn, echo string, data any, err error) {
	if err != nil {
		_ = conn.EnqueueJSON(struct {
			Echo string `json:"echo"`
			Err  string `json:"err"`
		}{echo, err.Error()})
		return
	}
	_ = conn.EnqueueJSON(struct {
		Echo string `json:"echo"`
		Data any    `json:"data"`
	}{echo, data})
}

func apiAvatarBotMutationStatusWs(ctx *ChatContext, msg []byte) {
	if ctx == nil || ctx.Conn == nil {
		return
	}
	var request struct {
		Data avatarBotMutationStatusRequest `json:"data"`
	}
	if json.Unmarshal(msg, &request) != nil {
		avatarBotMutationReply(ctx.Conn, ctx.Echo, nil, errors.New("INVALID_PARAMS"))
		return
	}
	go func() {
		resolved, err := avatarBotMutationPreflight(ctx, request.Data.ChannelID, request.Data.IdentityID, "")
		if err != nil {
			avatarBotMutationReply(ctx.Conn, ctx.Echo, map[string]any{"editable": false, "reason": err.Error()}, nil)
			return
		}
		avatarBotMutationReply(ctx.Conn, ctx.Echo, map[string]any{"editable": true, "sourceCardId": resolved.SourceCardID}, nil)
	}()
}

func apiAvatarBotMutationDelegateWs(ctx *ChatContext, msg []byte) {
	if ctx == nil || ctx.Conn == nil {
		return
	}
	var request struct {
		Data avatarBotMutationDelegateRequest `json:"data"`
	}
	if json.Unmarshal(msg, &request) != nil {
		avatarBotMutationReply(ctx.Conn, ctx.Echo, nil, errors.New("INVALID_PARAMS"))
		return
	}
	go func() {
		data := request.Data
		if data.ExpectedSourceCardID == "" || data.Value == nil || math.IsNaN(*data.Value) || math.IsInf(*data.Value, 0) ||
			(data.Op != "set" && data.Op != "add") || (data.Slot != "current" && data.Slot != "max") ||
			(data.Slot == "max" && data.Op != "set") {
			avatarBotMutationReply(ctx.Conn, ctx.Echo, nil, errors.New("INVALID_PARAMS"))
			return
		}
		resolved, err := avatarBotMutationPreflight(ctx, data.ChannelID, data.IdentityID, data.ExpectedSourceCardID)
		if err != nil {
			avatarBotMutationReply(ctx.Conn, ctx.Echo, nil, err)
			return
		}
		path, err := service.AvatarBotMutationTemplatePath(data.ChannelID, data.StatID, data.Slot)
		if err != nil {
			avatarBotMutationReply(ctx.Conn, ctx.Echo, nil, err)
			return
		}
		if _, err = service.AvatarBotMutationAuthorize(data.ChannelID, data.IdentityID, ctx.User.ID, resolved.SourceCardID, true); err != nil {
			avatarBotMutationReply(ctx.Conn, ctx.Echo, nil, err)
			return
		}
		targetConn, connMap := avatarBotMutationTargetConnection(resolved.TargetUserID, data.ChannelID)
		if targetConn == nil {
			avatarBotMutationReply(ctx.Conn, ctx.Echo, nil, errors.New("TARGET_OFFLINE"))
			return
		}
		pending := &avatarBotMutationPending{requestID: utils.NewID(), targetUserID: resolved.TargetUserID,
			channelID: data.ChannelID, targetConn: targetConn, result: make(chan error, 1)}
		if err := avatarBotMutations.register(pending); err != nil {
			avatarBotMutationReply(ctx.Conn, ctx.Echo, nil, err)
			return
		}
		event := &protocol.Event{Type: protocol.EventAvatarCardBotMutationRequest,
			Channel: &protocol.Channel{ID: data.ChannelID},
			AvatarCardBotMutation: &protocol.AvatarBotMutationPayload{
				RequestID: pending.requestID, ChannelID: data.ChannelID, IdentityID: data.IdentityID,
				ExpectedSourceCardID: resolved.SourceCardID, StatID: data.StatID,
				Slot: data.Slot, SourcePath: path, Op: data.Op, Value: *data.Value,
			},
		}
		if !writeConnReliableJSONAndPrune(connMap, targetConn, wsReliableClassOther,
			struct {
				protocol.Event
				Op protocol.Opcode `json:"op"`
			}{*event, protocol.OpEvent}) {
			avatarBotMutations.finish(pending, errors.New("TARGET_OFFLINE"))
		}
		select {
		case err = <-pending.result:
		case <-targetConn.done:
			avatarBotMutations.finish(pending, errors.New("TARGET_OFFLINE"))
			err = <-pending.result
		case <-time.After(avatarBotMutationTimeout):
			avatarBotMutations.finish(pending, errors.New("AVATAR_CARD_BOT_MUTATION_TIMEOUT"))
			err = <-pending.result
		}
		if err != nil {
			avatarBotMutationReply(ctx.Conn, ctx.Echo, nil, err)
			return
		}
		avatarBotMutationReply(ctx.Conn, ctx.Echo, map[string]any{"ok": true}, nil)
	}()
}

func apiAvatarBotMutationRespond(ctx *ChatContext, data *avatarBotMutationRespondRequest) (any, error) {
	if ctx == nil || ctx.User == nil || ctx.Conn == nil || ctx.IsReadOnly() {
		return nil, errors.New("PERMISSION_DENIED")
	}
	if data == nil || data.RequestID == "" {
		return nil, errors.New("INVALID_PARAMS")
	}
	var result error
	if !data.OK {
		code := strings.TrimSpace(data.Error)
		if code != "CARD_CHANGED" && code != "BOT_INTERACTION_BUSY" && code != "NOT_EDITABLE" && code != "TARGET_OFFLINE" {
			code = "AVATAR_CARD_BOT_MUTATION_FAILED"
		}
		result = errors.New(code)
	}
	if err := avatarBotMutations.respond(data.RequestID, ctx.User.ID, ctx.Conn, result); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true}, nil
}
