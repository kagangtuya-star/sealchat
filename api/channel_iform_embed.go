package api

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

const (
	embedStorageCapabilityRead  = "storage.read"
	embedStorageCapabilityWrite = "storage.write"
	embedEventsPublish          = "events.publish"
	embedEventPayloadMax        = 16 * 1024
)

var embedTopicPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`)
var embedRateState = struct {
	sync.Mutex
	items map[string]embedRateEntry
}{items: map[string]embedRateEntry{}}

type embedRateEntry struct {
	Window int64
	Count  int
}

// Embed RPC errors are part of an intentionally small public surface. Never
// return raw SQL/GORM or internal service details to an iframe.
func embedPublicError(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	for _, code := range []string{
		"ORIGIN_DENIED", "HANDSHAKE_FAILED", "SESSION_EXPIRED", "CONTEXT_CHANGED",
		"CAPABILITY_DENIED", "PERMISSION_DENIED", "INVALID_PARAMS", "NOT_FOUND",
		"REVISION_CONFLICT", "QUOTA_EXCEEDED", "PAYLOAD_TOO_LARGE", "RATE_LIMITED",
		"WS_OFFLINE", "TIMEOUT", "INTERNAL_ERROR",
	} {
		if strings.HasPrefix(message, code) {
			return err
		}
	}
	return errors.New("INTERNAL_ERROR")
}

func allowEmbedRate(userID, operation string, limit int) bool {
	now := time.Now().Unix()
	embedRateState.Lock()
	defer embedRateState.Unlock()
	key := strings.TrimSpace(userID) + "\x00" + operation
	entry := embedRateState.items[key]
	if entry.Window != now {
		entry = embedRateEntry{Window: now}
	}
	if entry.Count >= limit {
		embedRateState.items[key] = entry
		return false
	}
	entry.Count++
	embedRateState.items[key] = entry
	return true
}

type embedScopeRequest struct {
	ChannelID string `json:"channel_id"`
	FormID    string `json:"form_id"`
}

type embedStorageGetRequest struct {
	embedScopeRequest
	Key string `json:"key"`
}
type embedStorageSetRequest struct {
	embedScopeRequest
	Key        string          `json:"key"`
	Value      json.RawMessage `json:"value"`
	IfRevision *uint64         `json:"if_revision"`
	ExpiresAt  json.RawMessage `json:"expires_at"`
}
type embedStorageDeleteRequest struct {
	embedScopeRequest
	Key        string  `json:"key"`
	IfRevision *uint64 `json:"if_revision"`
}

func parseEmbedStorageExpiresAt(raw json.RawMessage) (*time.Time, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return nil, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		if millis, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(text)); err == nil {
			return &millis, nil
		}
		if numeric, err := json.Number(strings.TrimSpace(text)).Int64(); err == nil {
			value := time.UnixMilli(numeric)
			return &value, nil
		}
		return nil, errors.New("INVALID_PARAMS: expires_at must be RFC3339 or Unix milliseconds")
	}
	var numeric int64
	if err := json.Unmarshal(raw, &numeric); err != nil {
		return nil, errors.New("INVALID_PARAMS: expires_at must be RFC3339 or Unix milliseconds")
	}
	value := time.UnixMilli(numeric)
	return &value, nil
}

type embedStorageListRequest struct {
	embedScopeRequest
	Prefix string `json:"prefix"`
	Cursor string `json:"cursor"`
	Limit  int    `json:"limit"`
}
type embedEventPublishRequest struct {
	embedScopeRequest
	Topic   string          `json:"topic"`
	Payload json.RawMessage `json:"payload"`
}

func resolveEmbedForm(ctx *ChatContext, scope embedScopeRequest, capability string) (*model.ChannelIFormModel, string, error) {
	channelID := strings.TrimSpace(scope.ChannelID)
	formID := strings.TrimSpace(scope.FormID)
	if channelID == "" || formID == "" || len(channelID) > 100 || len(formID) > 100 {
		return nil, "", errors.New("INVALID_PARAMS: missing channel_id or form_id")
	}
	if ctx == nil || ctx.User == nil {
		return nil, "", errors.New("PERMISSION_DENIED")
	}
	if strings.HasPrefix(capability, "storage.") {
		if ctx.ConnInfo == nil || strings.TrimSpace(ctx.ConnInfo.ChannelId) == "" || strings.TrimSpace(ctx.ConnInfo.ChannelId) != channelID {
			return nil, "", errors.New("CONTEXT_CHANGED")
		}
	}
	if ctx.IsReadOnly() && (capability == embedStorageCapabilityWrite || capability == embedEventsPublish || capability == "messages.send") {
		return nil, "", errors.New("PERMISSION_DENIED")
	}
	if !service.CanReadChannelByUserId(ctx.User.ID, channelID) {
		return nil, "", errors.New("PERMISSION_DENIED")
	}
	if capability == embedStorageCapabilityWrite || capability == embedEventsPublish || capability == "messages.send" {
		if !pm.CanWithChannelRole(ctx.User.ID, channelID, pm.PermFuncChannelTextSend, pm.PermFuncChannelTextSendAll) {
			return nil, "", errors.New("PERMISSION_DENIED")
		}
	}
	var form *model.ChannelIFormModel
	local, err := model.ChannelIFormGet(channelID, formID)
	if err != nil {
		return nil, "", embedPublicError(err)
	}
	form = local
	if form == nil {
		forms, listErr := service.ListEffectiveChannelIForms(channelID)
		if listErr != nil {
			return nil, "", embedPublicError(listErr)
		}
		for _, item := range forms {
			if item != nil && item.ID == formID {
				form = item.ChannelIFormModel
				break
			}
		}
	}
	if form == nil {
		return nil, "", errors.New("NOT_FOUND: form")
	}
	policy := form.BridgePolicy
	if !policy.Enabled {
		return nil, "", errors.New("CAPABILITY_DENIED")
	}
	if !containsEmbedCapability(policy.Capabilities, capability) {
		return nil, "", errors.New("CAPABILITY_DENIED")
	}
	// Channel membership/read and channel text-send authorization form the
	// user-side boundary; form policy capability controls whether operation is enabled.
	return form, channelID, nil
}

func containsEmbedCapability(capabilities []string, expected string) bool {
	for _, capability := range capabilities {
		if strings.TrimSpace(capability) == expected {
			return true
		}
	}
	return false
}

func apiIFormStorageGet(ctx *ChatContext, data *embedStorageGetRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	_, channelID, err := resolveEmbedForm(ctx, data.embedScopeRequest, embedStorageCapabilityRead)
	if err != nil {
		return nil, err
	}
	item, err := model.ChannelIFormStorageGet(channelID, data.FormID, data.Key)
	if err != nil {
		return nil, embedPublicError(err)
	}
	if item == nil {
		return nil, nil
	}
	return item, nil
}

func apiIFormStorageSnapshot(ctx *ChatContext, data *embedScopeRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	_, channelID, err := resolveEmbedForm(ctx, *data, embedStorageCapabilityRead)
	if err != nil {
		return nil, err
	}
	ns, items, err := model.ChannelIFormStorageSnapshot(channelID, data.FormID)
	if err != nil {
		return nil, embedPublicError(err)
	}
	return map[string]any{"seq": ns.Seq, "documents": items}, nil
}

func apiIFormStorageList(ctx *ChatContext, data *embedStorageListRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	_, channelID, err := resolveEmbedForm(ctx, data.embedScopeRequest, embedStorageCapabilityRead)
	if err != nil {
		return nil, err
	}
	prefix := strings.TrimSpace(data.Prefix)
	cursor := strings.TrimSpace(data.Cursor)
	if len(prefix) > model.EmbedStorageKeyMax || len(cursor) > model.EmbedStorageKeyMax {
		return nil, errors.New("INVALID_PARAMS: prefix or cursor too long")
	}
	ns, items, err := model.ChannelIFormStorageList(channelID, data.FormID, prefix, cursor, data.Limit)
	if err != nil {
		return nil, embedPublicError(err)
	}
	next := ""
	limit := data.Limit
	if limit <= 0 || limit > 256 {
		limit = 256
	}
	if len(items) > 0 && len(items) == limit {
		next = items[len(items)-1].Key
	}
	return map[string]any{"seq": ns.Seq, "items": items, "cursor": next, "next": next}, nil
}

func apiIFormStorageSet(ctx *ChatContext, data *embedStorageSetRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	if ctx == nil || ctx.User == nil || !allowEmbedRate(ctx.User.ID, "storage.write", 30) {
		return nil, errors.New("RATE_LIMITED")
	}
	_, channelID, err := resolveEmbedForm(ctx, data.embedScopeRequest, embedStorageCapabilityWrite)
	if err != nil {
		return nil, err
	}
	expiresAt, err := parseEmbedStorageExpiresAt(data.ExpiresAt)
	if err != nil {
		return nil, err
	}
	mutation, err := model.ChannelIFormStorageSetWithExpiry(channelID, data.FormID, data.Key, data.Value, data.IfRevision, ctx.User.ID, expiresAt)
	if err != nil {
		return nil, embedPublicError(err)
	}
	if mutation == nil || mutation.Item == nil || mutation.Namespace == nil {
		return nil, errors.New("INTERNAL_ERROR")
	}
	broadcastEmbedStorageEvent(ctx, channelID, data.FormID, mutation.Namespace.Seq, mutation.Item, "set", mutation.Item.Value)
	return mutation.Item, nil
}

func apiIFormStorageDelete(ctx *ChatContext, data *embedStorageDeleteRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	if ctx == nil || ctx.User == nil || !allowEmbedRate(ctx.User.ID, "storage.write", 30) {
		return nil, errors.New("RATE_LIMITED")
	}
	_, channelID, err := resolveEmbedForm(ctx, data.embedScopeRequest, embedStorageCapabilityWrite)
	if err != nil {
		return nil, err
	}
	mutation, err := model.ChannelIFormStorageDelete(channelID, data.FormID, data.Key, data.IfRevision, ctx.User.ID)
	if err != nil {
		return nil, embedPublicError(err)
	}
	if mutation == nil || mutation.Namespace == nil {
		return nil, errors.New("INTERNAL_ERROR")
	}
	if mutation.Deleted && mutation.Item != nil {
		broadcastEmbedStorageEvent(ctx, channelID, data.FormID, mutation.Namespace.Seq, mutation.Item, "delete", nil)
	}
	result := map[string]any{"key": strings.TrimSpace(data.Key), "seq": mutation.Namespace.Seq, "deleted": mutation.Deleted}
	if mutation.Item != nil {
		result["revision"] = mutation.Item.Revision
	}
	return result, nil
}

func apiIFormEventPublish(ctx *ChatContext, data *embedEventPublishRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	if ctx == nil || ctx.User == nil || !allowEmbedRate(ctx.User.ID, "events.publish", 60) {
		return nil, errors.New("RATE_LIMITED")
	}
	form, channelID, err := resolveEmbedForm(ctx, data.embedScopeRequest, embedEventsPublish)
	if err != nil {
		return nil, err
	}
	topic := strings.TrimSpace(data.Topic)
	if !embedTopicPattern.MatchString(topic) {
		return nil, errors.New("INVALID_PARAMS: invalid topic")
	}
	if len(data.Payload) == 0 || len(data.Payload) > embedEventPayloadMax {
		return nil, errors.New("PAYLOAD_TOO_LARGE")
	}
	var payload any
	if err := json.Unmarshal(data.Payload, &payload); err != nil {
		return nil, errors.New("INVALID_PARAMS: payload must be JSON")
	}
	eventID := utils.NewID()
	embedEvent := &protocol.ChannelIFormEmbedEventPayload{EventID: eventID, ChannelID: channelID, FormID: data.FormID, Topic: topic, Op: "event", Payload: payload, At: time.Now().UnixMilli()}
	if form != nil && containsEmbedCapability(form.BridgePolicy.Capabilities, "events.subscribe") {
		markEmbedSubscription(ctx, channelID, data.FormID, topic)
	}
	dispatchEmbedEvent(channelID, embedEvent, true)
	return map[string]any{"eventId": eventID}, nil
}

type embedEventSubscribeRequest struct {
	embedScopeRequest
	Topic string `json:"topic"`
}

func apiIFormEventSubscribe(ctx *ChatContext, data *embedEventSubscribeRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	_, channelID, err := resolveEmbedForm(ctx, data.embedScopeRequest, "events.subscribe")
	if err != nil {
		return nil, err
	}
	topic := strings.TrimSpace(data.Topic)
	if !embedTopicPattern.MatchString(topic) {
		return nil, errors.New("INVALID_PARAMS: invalid topic")
	}
	markEmbedSubscription(ctx, channelID, data.FormID, topic)
	return map[string]any{"subscribed": true}, nil
}

func broadcastEmbedStorageEvent(ctx *ChatContext, channelID, formID string, seq uint64, item *model.ChannelIFormStorageItem, op string, value json.RawMessage) {
	if ctx == nil || item == nil {
		return
	}
	// Storage changes stay metadata-only on gateway; authorized Embed Host fetches
	// value through storage.get, preventing value disclosure to unrelated clients.
	_ = value
	dispatchEmbedEvent(channelID, &protocol.ChannelIFormEmbedEventPayload{EventID: utils.NewID(), ChannelID: channelID, FormID: formID, Seq: seq, Key: item.Key, Op: op, Revision: item.Revision, At: time.Now().UnixMilli()}, false)
}

func broadcastEmbedStorageGCEvent(channelID, formID string, mutation model.ChannelIFormStorageExpiredMutation) {
	item := mutation.Item
	dispatchEmbedEvent(channelID, &protocol.ChannelIFormEmbedEventPayload{
		EventID: utils.NewID(), ChannelID: channelID, FormID: formID,
		Seq: item.Seq, Key: item.Key, Op: "delete", Revision: item.Revision,
		At: time.Now().UnixMilli(),
	}, false)
}

func embedScopeKey(channelID, formID, topic string) string {
	return strings.TrimSpace(channelID) + "\x00" + strings.TrimSpace(formID) + "\x00" + strings.TrimSpace(topic)
}

func markEmbedSubscription(ctx *ChatContext, channelID, formID, topic string) {
	if ctx == nil || ctx.ConnInfo == nil {
		return
	}
	key := embedScopeKey(channelID, formID, topic)
	if key == "\x00\x00" {
		return
	}
	ctx.ConnInfo.embedMu.Lock()
	if ctx.ConnInfo.embedSubscriptions == nil {
		ctx.ConnInfo.embedSubscriptions = map[string]struct{}{}
	}
	ctx.ConnInfo.embedSubscriptions[key] = struct{}{}
	ctx.ConnInfo.embedMu.Unlock()
}

func hasEmbedSubscription(info *ConnInfo, channelID, formID, topic string) bool {
	if info == nil {
		return false
	}
	key := embedScopeKey(channelID, formID, topic)
	info.embedMu.RLock()
	_, ok := info.embedSubscriptions[key]
	info.embedMu.RUnlock()
	return ok
}

func dispatchEmbedEvent(channelID string, payload *protocol.ChannelIFormEmbedEventPayload, embedOnly bool) {
	if payload == nil || channelUsersMapGlobal == nil || userId2ConnInfoGlobal == nil {
		return
	}
	event := &protocol.Event{Type: protocol.EventChannelIFormEmbed, Channel: &protocol.Channel{ID: channelID}, ChannelIFormEmbed: payload}
	ctx := &ChatContext{ChannelUsersMap: channelUsersMapGlobal, UserId2ConnInfo: userId2ConnInfoGlobal}
	if !embedOnly {
		ctx.BroadcastEventInChannel(channelID, event)
		return
	}
	ctx.rangeChannelConnMaps(channelID, func(_ string, connMap *utils.SyncMap[*WsSyncConn, *ConnInfo], indexed bool) bool {
		connMap.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
			if info == nil || !((indexed && info.ChannelId == "") || info.ChannelId == channelID) || !hasEmbedSubscription(info, channelID, payload.FormID, payload.Topic) {
				return true
			}
			writeConnJSONAndPrune(connMap, conn, struct {
				protocol.Event
				Op protocol.Opcode `json:"op"`
			}{Event: *event, Op: protocol.OpEvent})
			return true
		})
		return true
	})
}
