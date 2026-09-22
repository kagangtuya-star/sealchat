package api

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

const (
	botInteractionDefaultTimeoutMs  = 5_000
	botInteractionMinTimeoutMs      = 1_000
	botInteractionMaxTimeoutMs      = 15_000
	botInteractionLegacyQuietWindow = 5 * time.Second

	botInteractionMessageIDPrefix = "bot-interaction:"
	botInteractionClientIDPrefix  = "bot-interaction-client:"
	botInteractionReplyIDPrefix   = "bot-interaction-reply:"
)

var (
	errBotInteractionBusy           = errors.New("BOT_INTERACTION_BUSY")
	errBotInteractionTimeout        = errors.New("BOT_INTERACTION_TIMEOUT")
	errBotInteractionBotUnavailable = errors.New("BOT_INTERACTION_BOT_UNAVAILABLE")

	botInteractions = newBotInteractionBroker()
)

type botInteractionRequest struct {
	ChannelID string `json:"channel_id"`
	Command   string `json:"command"`
	TimeoutMs int64  `json:"timeout_ms"`
}

type botInteractionResult struct {
	OK        bool            `json:"ok"`
	RequestID string          `json:"request_id"`
	MatchedBy string          `json:"matched_by"`
	Content   string          `json:"content"`
	Data      json.RawMessage `json:"data"`
}

type botInteractionOutcome struct {
	Result *botInteractionResult
	Err    error
}

type botInteractionPending struct {
	RequestID            string
	ChannelID            string
	RequestUserID        string
	BotUserID            string
	BotConn              *WsSyncConn
	EphemeralMessageID   string
	CreatedAt            time.Time
	SentAt               time.Time
	Deadline             time.Time
	Response             chan botInteractionOutcome
	Completed            bool
	LegacyContextAllowed bool
}

type botInteractionBroker struct {
	mu        sync.Mutex
	byLane    map[string]*botInteractionPending
	byRequest map[string]*botInteractionPending
}

func newBotInteractionBroker() *botInteractionBroker {
	return &botInteractionBroker{
		byLane:    make(map[string]*botInteractionPending),
		byRequest: make(map[string]*botInteractionPending),
	}
}

func botInteractionLaneKey(botUserID, channelID string) string {
	return strings.TrimSpace(botUserID) + "\x00" + strings.TrimSpace(channelID)
}

func (b *botInteractionBroker) register(pending *botInteractionPending) error {
	if b == nil || pending == nil {
		return errBotInteractionBotUnavailable
	}
	if pending.Response == nil {
		pending.Response = make(chan botInteractionOutcome, 1)
	}
	lane := botInteractionLaneKey(pending.BotUserID, pending.ChannelID)

	b.mu.Lock()
	defer b.mu.Unlock()
	if current := b.byLane[lane]; current != nil {
		if time.Now().Before(current.Deadline) && !current.Completed {
			return errBotInteractionBusy
		}
		b.finishLocked(current, botInteractionOutcome{Err: errBotInteractionTimeout})
	}
	pending.Completed = false
	b.byLane[lane] = pending
	b.byRequest[pending.RequestID] = pending
	return nil
}

func (b *botInteractionBroker) markSent(pending *botInteractionPending, sentAt time.Time) bool {
	if b == nil || pending == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.byRequest[pending.RequestID] != pending || pending.Completed {
		return false
	}
	if !sentAt.Before(pending.Deadline) {
		b.finishLocked(pending, botInteractionOutcome{Err: errBotInteractionTimeout})
		return false
	}
	pending.SentAt = sentAt
	return true
}

func (b *botInteractionBroker) setLegacyContextAllowed(pending *botInteractionPending, allowed bool) bool {
	if b == nil || pending == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.byRequest[pending.RequestID] != pending || pending.Completed {
		return false
	}
	pending.LegacyContextAllowed = allowed
	return true
}

func (b *botInteractionBroker) finishLocked(pending *botInteractionPending, outcome botInteractionOutcome) bool {
	if pending == nil || pending.Completed || b.byRequest[pending.RequestID] != pending {
		return false
	}
	delete(b.byRequest, pending.RequestID)
	delete(b.byLane, botInteractionLaneKey(pending.BotUserID, pending.ChannelID))
	pending.Completed = true
	select {
	case pending.Response <- outcome:
	default:
	}
	return true
}

func (b *botInteractionBroker) finish(pending *botInteractionPending, outcome botInteractionOutcome, now time.Time) bool {
	if b == nil || pending == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !now.Before(pending.Deadline) {
		b.finishLocked(pending, botInteractionOutcome{Err: errBotInteractionTimeout})
		return false
	}
	return b.finishLocked(pending, outcome)
}

func (b *botInteractionBroker) cancel(pending *botInteractionPending, err error) {
	if b == nil || pending == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.finishLocked(pending, botInteractionOutcome{Err: err})
}

func (b *botInteractionBroker) pendingForLaneState(botUserID, channelID string, now time.Time) (*botInteractionPending, bool) {
	if b == nil {
		return nil, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	pending := b.byLane[botInteractionLaneKey(botUserID, channelID)]
	if pending != nil && !now.Before(pending.Deadline) {
		b.finishLocked(pending, botInteractionOutcome{Err: errBotInteractionTimeout})
		return nil, false
	}
	if pending == nil {
		return nil, false
	}
	return pending, pending.LegacyContextAllowed
}

func (b *botInteractionBroker) pendingForLane(botUserID, channelID string, now time.Time) *botInteractionPending {
	pending, _ := b.pendingForLaneState(botUserID, channelID, now)
	return pending
}

func (b *botInteractionBroker) pendingForRequest(requestID string, now time.Time) *botInteractionPending {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	pending := b.byRequest[strings.TrimSpace(requestID)]
	if pending != nil && !now.Before(pending.Deadline) {
		b.finishLocked(pending, botInteractionOutcome{Err: errBotInteractionTimeout})
		return nil
	}
	return pending
}

func (b *botInteractionBroker) wait(pending *botInteractionPending) (*botInteractionResult, error) {
	timer := time.NewTimer(time.Until(pending.Deadline))
	defer timer.Stop()
	select {
	case outcome := <-pending.Response:
		return outcome.Result, outcome.Err
	case <-timer.C:
		b.cancel(pending, errBotInteractionTimeout)
		outcome := <-pending.Response
		return outcome.Result, outcome.Err
	}
}

func newBotInteractionEvent(
	channel *protocol.Channel,
	user *protocol.User,
	member *protocol.GuildMember,
	command string,
	requestID string,
) *protocol.Event {
	event := newBotCommandEvent(
		channel,
		user,
		member,
		command,
		"ic",
		botInteractionMessageIDPrefix+requestID,
		botInteractionClientIDPrefix+requestID,
	)
	if event == nil || event.MessageContext == nil {
		return event
	}
	event.MessageContext.InteractionID = requestID
	event.MessageContext.IsEphemeral = true
	event.MessageContext.SenderUserID = user.ID
	return event
}

func normalizeBotInteractionTimeout(timeoutMs int64) time.Duration {
	if timeoutMs == 0 {
		timeoutMs = botInteractionDefaultTimeoutMs
	}
	if timeoutMs < botInteractionMinTimeoutMs {
		timeoutMs = botInteractionMinTimeoutMs
	}
	if timeoutMs > botInteractionMaxTimeoutMs {
		timeoutMs = botInteractionMaxTimeoutMs
	}
	return time.Duration(timeoutMs) * time.Millisecond
}

func isBotInteractionContextQuiet(marker BotMessageEventMarker, exists bool, now time.Time) bool {
	if !exists {
		return true
	}
	return now.UnixMilli()-marker.At >= botInteractionLegacyQuietWindow.Milliseconds()
}

func cacheBotInteractionEventContext(info *ConnInfo, channelID string, event *protocol.Event, now time.Time) bool {
	if info == nil || strings.TrimSpace(channelID) == "" || event == nil {
		return false
	}

	info.botMessageContextMu.Lock()
	defer info.botMessageContextMu.Unlock()

	var marker BotMessageEventMarker
	var exists bool
	if info.BotLastMessageEvent != nil {
		marker, exists = info.BotLastMessageEvent.Load(channelID)
	}
	allowed := isBotInteractionContextQuiet(marker, exists, now)
	cacheBotEventContextLocked(info, channelID, event)
	return allowed
}

func isBotInteractionLatestMessageEvent(info *ConnInfo, channelID, messageID string) bool {
	if info == nil {
		return false
	}
	info.botMessageContextMu.Lock()
	defer info.botMessageContextMu.Unlock()
	if info.BotLastMessageEvent == nil {
		return false
	}
	marker, ok := info.BotLastMessageEvent.Load(channelID)
	return ok && marker.MessageID == messageID
}

func startBotInteraction(ctx *ChatContext, data *botInteractionRequest) (*botInteractionPending, error) {
	if ctx == nil || ctx.User == nil || data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	channelID := strings.TrimSpace(data.ChannelID)
	command := strings.TrimSpace(data.Command)
	if channelID == "" || command == "" {
		return nil, errors.New("INVALID_PARAMS")
	}
	if !canDispatchBotCommand(ctx, channelID) {
		return nil, errors.New("PERMISSION_DENIED")
	}

	channel, err := model.ChannelGet(channelID)
	if err != nil || channel == nil || channel.ID == "" {
		return nil, errors.New("INVALID_PARAMS")
	}
	botConn, botInfo, err := findBotConnectionForChannel(ctx, channelID)
	if err != nil || botConn == nil || botInfo == nil || botInfo.User == nil || !botInfo.User.IsBot {
		return nil, errBotInteractionBotUnavailable
	}
	botUserID := strings.TrimSpace(botInfo.User.ID)
	connMap, ok := userId2ConnInfoGlobal.Load(botUserID)
	if !ok || connMap == nil {
		return nil, errBotInteractionBotUnavailable
	}

	requestID := utils.NewID()
	createdAt := time.Now()
	timeout := normalizeBotInteractionTimeout(data.TimeoutMs)
	pending := &botInteractionPending{
		RequestID:          requestID,
		ChannelID:          channelID,
		RequestUserID:      ctx.User.ID,
		BotUserID:          botUserID,
		BotConn:            botConn,
		EphemeralMessageID: botInteractionMessageIDPrefix + requestID,
		CreatedAt:          createdAt,
		Deadline:           createdAt.Add(timeout),
		Response:           make(chan botInteractionOutcome, 1),
	}
	if err := botInteractions.register(pending); err != nil {
		return nil, err
	}

	userData := ctx.User.ToProtocolType()
	var memberData *protocol.GuildMember
	member, _ := model.MemberGetByUserIDAndChannelIDBase(ctx.User.ID, channelID, ctx.User.Nickname, false)
	if member != nil {
		memberData = member.ToProtocolType()
		memberData.Roles = []string{service.ResolveMemberRoleForProtocol(ctx.User.ID, channelID, channel.WorldID)}
	}
	event := newBotInteractionEvent(channel.ToProtocolType(), userData, memberData, command, requestID)
	if event == nil {
		botInteractions.cancel(pending, errBotInteractionBotUnavailable)
		return nil, errBotInteractionBotUnavailable
	}
	event = normalizeEventForBot(event)
	event.Timestamp = time.Now().Unix()
	if !botInteractions.markSent(pending, time.Now()) {
		return nil, errBotInteractionTimeout
	}
	legacyContextAllowed := cacheBotInteractionEventContext(botInfo, channelID, event, time.Now())
	if !botInteractions.setLegacyContextAllowed(pending, legacyContextAllowed) {
		return nil, errBotInteractionTimeout
	}
	envelope := struct {
		protocol.Event
		Op protocol.Opcode `json:"op"`
	}{
		Event: *event,
		Op:    protocol.OpEvent,
	}
	if !writeConnReliableJSONAndPrune(connMap, botConn, wsReliableClassBotEvent, envelope) {
		botInteractions.cancel(pending, errBotInteractionBotUnavailable)
		return nil, errBotInteractionBotUnavailable
	}

	return pending, nil
}

func apiBotInteractWs(ctx *ChatContext, msg []byte) {
	if ctx == nil || ctx.Conn == nil {
		return
	}
	conn := ctx.Conn
	echo := ctx.Echo
	request := struct {
		Data *botInteractionRequest `json:"data"`
	}{}
	if err := json.Unmarshal(msg, &request); err != nil {
		_ = conn.EnqueueJSON(&struct {
			Echo string `json:"echo"`
			Err  string `json:"err"`
		}{Echo: echo, Err: "INVALID_PARAMS"})
		return
	}
	pending, err := startBotInteraction(ctx, request.Data)
	if err != nil {
		_ = conn.EnqueueJSON(&struct {
			Echo string `json:"echo"`
			Err  string `json:"err"`
		}{Echo: echo, Err: err.Error()})
		return
	}

	go func(conn *WsSyncConn, echo string, pending *botInteractionPending) {
		result, err := botInteractions.wait(pending)
		if err != nil {
			_ = conn.EnqueueJSON(&struct {
				Echo string `json:"echo"`
				Err  string `json:"err"`
			}{Echo: echo, Err: err.Error()})
			return
		}
		_ = conn.EnqueueJSON(&struct {
			Echo string                `json:"echo"`
			Data *botInteractionResult `json:"data"`
		}{Echo: echo, Data: result})
	}(conn, echo, pending)
}

func tryCaptureBotInteractionMessageCreate(
	ctx *ChatContext,
	channelID string,
	quoteID string,
	content string,
	messageContext *protocol.MessageContext,
) (*protocol.Message, bool) {
	return tryCaptureBotInteractionMessageCreateWithBroker(botInteractions, ctx, channelID, quoteID, content, messageContext)
}

func tryCaptureBotInteractionMessageCreateWithBroker(
	broker *botInteractionBroker,
	ctx *ChatContext,
	channelID string,
	quoteID string,
	content string,
	messageContext *protocol.MessageContext,
) (*protocol.Message, bool) {
	if broker == nil || ctx == nil || ctx.User == nil || !ctx.User.IsBot || ctx.Conn == nil {
		return nil, false
	}
	now := time.Now()
	pending, legacyContextAllowed := broker.pendingForLaneState(ctx.User.ID, channelID, now)
	if pending == nil || pending.BotUserID != ctx.User.ID || pending.ChannelID != channelID || pending.BotConn != ctx.Conn {
		return nil, false
	}

	matchedBy := ""
	if quoteID == pending.EphemeralMessageID {
		matchedBy = "quote"
	} else if legacyContextAllowed &&
		messageContext != nil &&
		messageContext.IsEphemeral &&
		messageContext.InteractionID == pending.RequestID &&
		isBotInteractionLatestMessageEvent(ctx.ConnInfo, channelID, pending.EphemeralMessageID) {
		matchedBy = "context"
	}
	if matchedBy == "" {
		return nil, false
	}

	result := &botInteractionResult{
		OK:        true,
		RequestID: pending.RequestID,
		MatchedBy: matchedBy,
		Content:   content,
	}
	if !broker.finish(pending, botInteractionOutcome{Result: result}, now) {
		return nil, false
	}
	return &protocol.Message{
		ID:        botInteractionReplyIDPrefix + pending.RequestID,
		Channel:   &protocol.Channel{ID: channelID},
		User:      ctx.User.ToProtocolType(),
		Content:   content,
		Timestamp: now.Unix(),
		CreatedAt: now.UnixMilli(),
		UpdatedAt: now.UnixMilli(),
	}, true
}

func HandleBotInteractionResponse(ctx *ChatContext, echo string, data json.RawMessage) bool {
	return handleBotInteractionResponseWithBroker(botInteractions, ctx, echo, data)
}

func handleBotInteractionResponseWithBroker(broker *botInteractionBroker, ctx *ChatContext, echo string, data json.RawMessage) bool {
	if broker == nil || ctx == nil || ctx.User == nil || !ctx.User.IsBot || ctx.Conn == nil {
		return false
	}
	now := time.Now()
	pending := broker.pendingForRequest(echo, now)
	if pending == nil || pending.BotUserID != ctx.User.ID || pending.BotConn != ctx.Conn {
		return false
	}
	result := &botInteractionResult{
		OK:        true,
		RequestID: pending.RequestID,
		MatchedBy: "structured",
		Data:      append(json.RawMessage(nil), data...),
	}
	return broker.finish(pending, botInteractionOutcome{Result: result}, now)
}
