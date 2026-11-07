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
	embedStorageCapabilityRead  = "storage.read"
	embedStorageCapabilityWrite = "storage.write"
	embedEventsPublish          = "events.publish"
	embedEventPayloadMax        = 16 * 1024
)

var embedTopicPattern = service.ChannelEmbedTopicPattern
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
	ChannelID string                        `json:"channel_id"`
	FormID    string                        `json:"form_id"`
	Scope     service.ChannelExtensionScope `json:"scope"`
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
	Topic          string                              `json:"topic"`
	Payload        json.RawMessage                     `json:"payload"`
	TheaterContext *service.TheaterEmbedPublishContext `json:"theaterContext,omitempty"`
}

func resolveChannelExtensionScope(ctx *ChatContext, request embedScopeRequest, capability string) (*service.ResolvedChannelExtensionScope, string, error) {
	channelID := strings.TrimSpace(request.ChannelID)
	if channelID == "" || len(channelID) > 100 || !request.Scope.Valid() {
		return nil, "", errors.New("INVALID_PARAMS: missing channel_id or scope")
	}
	if ctx == nil || ctx.User == nil {
		return nil, "", errors.New("PERMISSION_DENIED")
	}
	if ctx.ConnInfo == nil || strings.TrimSpace(ctx.ConnInfo.ChannelId) != channelID || channelID == "" {
		return nil, "", errors.New("CONTEXT_CHANGED")
	}
	resolved, err := service.ResolveChannelExtensionScope(ctx.User.ID, channelID, request.Scope, capability, ctx.IsReadOnly(), uiCustomizationPolicy())
	return resolved, channelID, embedPublicError(err)
}

func apiChannelExtensionStorageGet(ctx *ChatContext, data *embedStorageGetRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	_, channelID, err := resolveChannelExtensionScope(ctx, data.embedScopeRequest, embedStorageCapabilityRead)
	if err != nil {
		return nil, err
	}
	item, err := model.ChannelExtensionStorageGet(channelID, data.Scope.StorageNamespaceKey(), data.Key)
	if err != nil {
		return nil, embedPublicError(err)
	}
	if item == nil {
		return nil, nil
	}
	return item, nil
}

func apiChannelExtensionStorageSnapshot(ctx *ChatContext, data *embedScopeRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	_, channelID, err := resolveChannelExtensionScope(ctx, *data, embedStorageCapabilityRead)
	if err != nil {
		return nil, err
	}
	ns, items, err := model.ChannelExtensionStorageSnapshot(channelID, data.Scope.StorageNamespaceKey())
	if err != nil {
		return nil, embedPublicError(err)
	}
	return map[string]any{"seq": ns.Seq, "documents": items}, nil
}

func apiChannelExtensionStorageList(ctx *ChatContext, data *embedStorageListRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	_, channelID, err := resolveChannelExtensionScope(ctx, data.embedScopeRequest, embedStorageCapabilityRead)
	if err != nil {
		return nil, err
	}
	prefix := strings.TrimSpace(data.Prefix)
	cursor := strings.TrimSpace(data.Cursor)
	if len(prefix) > model.EmbedStorageKeyMax || len(cursor) > model.EmbedStorageKeyMax {
		return nil, errors.New("INVALID_PARAMS: prefix or cursor too long")
	}
	ns, items, err := model.ChannelExtensionStorageList(channelID, data.Scope.StorageNamespaceKey(), prefix, cursor, data.Limit)
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

func apiChannelExtensionStorageSet(ctx *ChatContext, data *embedStorageSetRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	if ctx == nil || ctx.User == nil || !allowEmbedRate(ctx.User.ID, "storage.write", 30) {
		return nil, errors.New("RATE_LIMITED")
	}
	_, channelID, err := resolveChannelExtensionScope(ctx, data.embedScopeRequest, embedStorageCapabilityWrite)
	if err != nil {
		return nil, err
	}
	expiresAt, err := parseEmbedStorageExpiresAt(data.ExpiresAt)
	if err != nil {
		return nil, err
	}
	mutation, err := model.ChannelExtensionStorageSetWithExpiry(channelID, data.Scope.StorageNamespaceKey(), data.Key, data.Value, data.IfRevision, ctx.User.ID, expiresAt)
	if err != nil {
		return nil, embedPublicError(err)
	}
	if mutation == nil || mutation.Item == nil || mutation.Namespace == nil {
		return nil, errors.New("INTERNAL_ERROR")
	}
	broadcastEmbedStorageEvent(ctx, channelID, data.Scope, mutation.Namespace.Seq, mutation.Item, "set", mutation.Item.Value)
	return mutation.Item, nil
}

func apiChannelExtensionStorageDelete(ctx *ChatContext, data *embedStorageDeleteRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	if ctx == nil || ctx.User == nil || !allowEmbedRate(ctx.User.ID, "storage.write", 30) {
		return nil, errors.New("RATE_LIMITED")
	}
	_, channelID, err := resolveChannelExtensionScope(ctx, data.embedScopeRequest, embedStorageCapabilityWrite)
	if err != nil {
		return nil, err
	}
	mutation, err := model.ChannelExtensionStorageDelete(channelID, data.Scope.StorageNamespaceKey(), data.Key, data.IfRevision, ctx.User.ID)
	if err != nil {
		return nil, embedPublicError(err)
	}
	if mutation == nil || mutation.Namespace == nil {
		return nil, errors.New("INTERNAL_ERROR")
	}
	if mutation.Deleted && mutation.Item != nil {
		broadcastEmbedStorageEvent(ctx, channelID, data.Scope, mutation.Namespace.Seq, mutation.Item, "delete", nil)
	}
	result := map[string]any{"key": strings.TrimSpace(data.Key), "seq": mutation.Namespace.Seq, "deleted": mutation.Deleted}
	if mutation.Item != nil {
		result["revision"] = mutation.Item.Revision
	}
	return result, nil
}

func apiChannelExtensionEventPublish(ctx *ChatContext, data *embedEventPublishRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	if ctx == nil || ctx.User == nil || !allowEmbedRate(ctx.User.ID, "events.publish", 60) {
		return nil, errors.New("RATE_LIMITED")
	}
	resolved, channelID, err := resolveChannelExtensionScope(ctx, data.embedScopeRequest, embedEventsPublish)
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
	// A regular channel publish keeps its existing behavior. Theater context is
	// only a locator: the service revalidates permissions and saved object state.
	var recordReceipt func()
	if data.TheaterContext != nil {
		if data.Scope.Type != "iform" {
			return nil, errors.New("PERMISSION_DENIED: invalid Theater context")
		}
		if resolved.WorldID == "" {
			return nil, errors.New("NOT_FOUND: channel")
		}
		scope, err := service.ValidateTheaterEmbedEventPublish(mcpConfigSnapshot(), ctx.User.ID, resolved.WorldID, channelID, data.Scope.ID, topic, *data.TheaterContext)
		if err != nil {
			return nil, errors.New("PERMISSION_DENIED: invalid Theater context")
		}
		recordReceipt = func() { service.RecordTheaterEmbedEventReceipt(eventID, scope) }
	}
	embedEvent := &protocol.ChannelIFormEmbedEventPayload{EventID: eventID, ChannelID: channelID, FormID: extensionFormID(data.Scope), ScopeType: data.Scope.Type, ScopeID: data.Scope.ID, Topic: topic, Op: "event", Payload: payload, At: time.Now().UnixMilli()}
	if service.HasChannelExtensionCapability(resolved.Capabilities, "events.subscribe") {
		markExtensionSubscription(ctx, channelID, data.Scope, topic)
	}
	if !dispatchEmbedEvent(channelID, embedEvent, true) {
		return nil, errors.New("INTERNAL_ERROR")
	}
	if recordReceipt != nil {
		recordReceipt()
	}
	return map[string]any{"eventId": eventID}, nil
}

type embedEventSubscribeRequest struct {
	embedScopeRequest
	Topic string `json:"topic"`
}

func apiChannelExtensionEventSubscribe(ctx *ChatContext, data *embedEventSubscribeRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	_, channelID, err := resolveChannelExtensionScope(ctx, data.embedScopeRequest, "events.subscribe")
	if err != nil {
		return nil, err
	}
	topic := strings.TrimSpace(data.Topic)
	if !embedTopicPattern.MatchString(topic) {
		return nil, errors.New("INVALID_PARAMS: invalid topic")
	}
	markExtensionSubscription(ctx, channelID, data.Scope, topic)
	return map[string]any{"subscribed": true}, nil
}

func broadcastEmbedStorageEvent(ctx *ChatContext, channelID string, scope service.ChannelExtensionScope, seq uint64, item *model.ChannelIFormStorageItem, op string, value json.RawMessage) {
	if ctx == nil || item == nil {
		return
	}
	// Storage changes stay metadata-only on gateway; authorized Embed Host fetches
	// value through storage.get, preventing value disclosure to unrelated clients.
	_ = value
	dispatchEmbedEvent(channelID, &protocol.ChannelIFormEmbedEventPayload{EventID: utils.NewID(), ChannelID: channelID, FormID: extensionFormID(scope), ScopeType: scope.Type, ScopeID: scope.ID, Seq: seq, Key: item.Key, Op: op, Revision: item.Revision, At: time.Now().UnixMilli()}, false)
}

func broadcastEmbedStorageGCEvent(channelID, formID string, mutation model.ChannelIFormStorageExpiredMutation) {
	item := mutation.Item
	scope, ok := service.ChannelExtensionScopeForNamespace(formID)
	if !ok {
		return
	}
	dispatchEmbedEvent(channelID, &protocol.ChannelIFormEmbedEventPayload{
		EventID: utils.NewID(), ChannelID: channelID, FormID: extensionFormID(scope), ScopeType: scope.Type, ScopeID: scope.ID,
		Seq: item.Seq, Key: item.Key, Op: "delete", Revision: item.Revision,
		At: time.Now().UnixMilli(),
	}, false)
}

func extensionScopeKey(channelID string, scope service.ChannelExtensionScope, topic string) string {
	return strings.TrimSpace(channelID) + "\x00" + scope.Type + "\x00" + scope.ID + "\x00" + strings.TrimSpace(topic)
}

func markExtensionSubscription(ctx *ChatContext, channelID string, scope service.ChannelExtensionScope, topic string) {
	if ctx == nil || ctx.ConnInfo == nil {
		return
	}
	key := extensionScopeKey(channelID, scope, topic)
	if channelID == "" || !scope.Valid() || !embedTopicPattern.MatchString(topic) {
		return
	}
	ctx.ConnInfo.embedMu.Lock()
	if ctx.ConnInfo.embedSubscriptions == nil {
		ctx.ConnInfo.embedSubscriptions = map[string]struct{}{}
	}
	ctx.ConnInfo.embedSubscriptions[key] = struct{}{}
	ctx.ConnInfo.embedMu.Unlock()
}

func hasExtensionSubscription(info *ConnInfo, channelID string, scope service.ChannelExtensionScope, topic string) bool {
	if info == nil {
		return false
	}
	key := extensionScopeKey(channelID, scope, topic)
	info.embedMu.RLock()
	_, ok := info.embedSubscriptions[key]
	info.embedMu.RUnlock()
	return ok
}

func dispatchEmbedEvent(channelID string, payload *protocol.ChannelIFormEmbedEventPayload, embedOnly bool) bool {
	if payload == nil || channelUsersMapGlobal == nil || userId2ConnInfoGlobal == nil {
		return false
	}
	event := &protocol.Event{Type: protocol.EventChannelIFormEmbed, Channel: &protocol.Channel{ID: channelID}, ChannelIFormEmbed: payload}
	ctx := &ChatContext{ChannelUsersMap: channelUsersMapGlobal, UserId2ConnInfo: userId2ConnInfoGlobal}
	if !embedOnly {
		ctx.BroadcastEventInChannel(channelID, event)
		return true
	}
	dispatched := true
	ctx.rangeChannelConnMaps(channelID, func(_ string, connMap *utils.SyncMap[*WsSyncConn, *ConnInfo], indexed bool) bool {
		connMap.Range(func(conn *WsSyncConn, info *ConnInfo) bool {
			if info == nil || !((indexed && info.ChannelId == "") || info.ChannelId == channelID) || !hasExtensionSubscription(info, channelID, extensionEventScope(payload), payload.Topic) {
				return true
			}
			if info.User == nil {
				return true
			}
			if _, err := service.ResolveChannelExtensionScope(info.User.ID, channelID, extensionEventScope(payload), "events.subscribe", info.IsGuest || info.IsObserver, uiCustomizationPolicy()); err != nil {
				return true
			}
			if !writeConnJSONAndPrune(connMap, conn, struct {
				protocol.Event
				Op protocol.Opcode `json:"op"`
			}{Event: *event, Op: protocol.OpEvent}) {
				dispatched = false
			}
			return true
		})
		return true
	})
	return dispatched
}

func extensionFormID(scope service.ChannelExtensionScope) string {
	if scope.Type == "iform" {
		return scope.ID
	}
	return ""
}
func extensionEventScope(payload *protocol.ChannelIFormEmbedEventPayload) service.ChannelExtensionScope {
	if payload.ScopeType != "" {
		return service.ChannelExtensionScope{Type: payload.ScopeType, ID: payload.ScopeID}
	}
	return service.ChannelExtensionScope{Type: "iform", ID: payload.FormID}
}
func legacyIFormScope(request embedScopeRequest) embedScopeRequest {
	request.Scope = service.ChannelExtensionScope{Type: "iform", ID: strings.TrimSpace(request.FormID)}
	return request
}

func authorizeChannelExtensionMessage(ctx *ChatContext, channelID, clientID string) error {
	var scope service.ChannelExtensionScope
	if strings.HasPrefix(clientID, "iform_embed:") {
		parts := strings.SplitN(strings.TrimPrefix(clientID, "iform_embed:"), ":", 2)
		if len(parts) != 2 {
			return errors.New("INVALID_PARAMS")
		}
		scope = service.ChannelExtensionScope{Type: "iform", ID: strings.TrimSpace(parts[0])}
	} else if strings.HasPrefix(clientID, "channel_extension:") {
		parts := strings.SplitN(strings.TrimPrefix(clientID, "channel_extension:"), ":", 3)
		if len(parts) != 3 {
			return errors.New("INVALID_PARAMS")
		}
		scope = service.ChannelExtensionScope{Type: parts[0], ID: parts[1]}
	} else {
		return nil
	}
	if !scope.Valid() {
		return errors.New("INVALID_PARAMS")
	}
	_, _, err := resolveChannelExtensionScope(ctx, embedScopeRequest{ChannelID: channelID, Scope: scope}, "messages.send")
	return err
}
func apiIFormStorageGet(ctx *ChatContext, data *embedStorageGetRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	copy := *data
	copy.embedScopeRequest = legacyIFormScope(data.embedScopeRequest)
	return apiChannelExtensionStorageGet(ctx, &copy)
}
func apiIFormStorageSet(ctx *ChatContext, data *embedStorageSetRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	copy := *data
	copy.embedScopeRequest = legacyIFormScope(data.embedScopeRequest)
	return apiChannelExtensionStorageSet(ctx, &copy)
}
func apiIFormStorageDelete(ctx *ChatContext, data *embedStorageDeleteRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	copy := *data
	copy.embedScopeRequest = legacyIFormScope(data.embedScopeRequest)
	return apiChannelExtensionStorageDelete(ctx, &copy)
}
func apiIFormStorageList(ctx *ChatContext, data *embedStorageListRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	copy := *data
	copy.embedScopeRequest = legacyIFormScope(data.embedScopeRequest)
	return apiChannelExtensionStorageList(ctx, &copy)
}
func apiIFormStorageSnapshot(ctx *ChatContext, data *embedScopeRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	copy := *data
	copy = legacyIFormScope(*data)
	return apiChannelExtensionStorageSnapshot(ctx, &copy)
}
func apiIFormEventPublish(ctx *ChatContext, data *embedEventPublishRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	copy := *data
	copy.embedScopeRequest = legacyIFormScope(data.embedScopeRequest)
	return apiChannelExtensionEventPublish(ctx, &copy)
}
func apiIFormEventSubscribe(ctx *ChatContext, data *embedEventSubscribeRequest) (any, error) {
	if data == nil {
		return nil, errors.New("INVALID_PARAMS")
	}
	copy := *data
	copy.embedScopeRequest = legacyIFormScope(data.embedScopeRequest)
	return apiChannelExtensionEventSubscribe(ctx, &copy)
}
