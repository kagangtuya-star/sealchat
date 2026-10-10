package service

import (
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"time"

	"sealchat/model"
	"sealchat/utils"
)

const (
	theaterEmbedEventReceiptTTL   = 5 * time.Minute
	theaterEmbedEventReceiptLimit = 4096
)

type theaterActionSource uint8

const (
	theaterActionSourceBrowser theaterActionSource = iota
	theaterActionSourceMCPControl
)

// This is event provenance only. No iframe payload enters the action executor.
type TheaterEmbedEventContext struct {
	EventID string `json:"eventId"`
	FormID  string `json:"formId"`
	Topic   string `json:"topic"`
}

type theaterEmbedEventReceiptScope struct {
	UserID, WorldID, ChannelID, FormID, Topic string
	RoomID, ObjectID, IframeURL               string
}

type theaterEmbedEventReceipt struct {
	Scope          theaterEmbedEventReceiptScope
	ExpiresAt      time.Time
	Used, Inflight map[string]bool
}

type theaterEmbedEventReceiptStore struct {
	sync.Mutex
	items map[string]*theaterEmbedEventReceipt
}

var theaterEmbedEventReceipts = theaterEmbedEventReceiptStore{items: map[string]*theaterEmbedEventReceipt{}}

func (s *theaterEmbedEventReceiptStore) prune(now time.Time) {
	for id, receipt := range s.items {
		if !now.Before(receipt.ExpiresAt) {
			delete(s.items, id)
		}
	}
}

func (s *theaterEmbedEventReceiptStore) record(eventID string, scope theaterEmbedEventReceiptScope, now time.Time) {
	s.Lock()
	defer s.Unlock()
	s.prune(now)
	if len(s.items) >= theaterEmbedEventReceiptLimit {
		var oldestID string
		var oldest time.Time
		for id, receipt := range s.items {
			if oldestID == "" || receipt.ExpiresAt.Before(oldest) {
				oldestID, oldest = id, receipt.ExpiresAt
			}
		}
		delete(s.items, oldestID)
	}
	s.items[eventID] = &theaterEmbedEventReceipt{Scope: scope, ExpiresAt: now.Add(theaterEmbedEventReceiptTTL), Used: map[string]bool{}, Inflight: map[string]bool{}}
}

// A reservation holds the original receipt even if TTL/capacity pruning evicts it.
type TheaterEmbedEventUse struct {
	store   *theaterEmbedEventReceiptStore
	eventID string
	receipt *theaterEmbedEventReceipt
	keys    []string
}

func (s *theaterEmbedEventReceiptStore) begin(eventID string, scope theaterEmbedEventReceiptScope, keys []string, now time.Time) *TheaterEmbedEventUse {
	s.Lock()
	defer s.Unlock()
	s.prune(now)
	receipt, ok := s.items[eventID]
	if !ok || receipt.Scope != scope || len(keys) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, key := range keys {
		if key == "" || seen[key] || receipt.Used[key] || receipt.Inflight[key] {
			return nil
		}
		seen[key] = true
	}
	for _, key := range keys {
		receipt.Inflight[key] = true
	}
	return &TheaterEmbedEventUse{store: s, eventID: eventID, receipt: receipt, keys: append([]string(nil), keys...)}
}

func (s *theaterEmbedEventReceiptStore) commit(use *TheaterEmbedEventUse, now time.Time) {
	if use == nil || len(use.keys) == 0 {
		return
	}
	s.Lock()
	defer s.Unlock()
	for _, key := range use.keys {
		delete(use.receipt.Inflight, key)
		use.receipt.Used[key] = true
	}
	// A successful execution unit keeps the same event execution alive for
	// subsequent saved actions/sequence steps without reviving an evicted receipt.
	if receipt, ok := s.items[use.eventID]; ok && receipt == use.receipt {
		receipt.ExpiresAt = now.Add(theaterEmbedEventReceiptTTL)
	}
	use.keys = nil
}

func CommitTheaterEmbedEventUse(use *TheaterEmbedEventUse) {
	if use == nil {
		return
	}
	use.store.commit(use, time.Now())
}

func ReleaseTheaterEmbedEventUse(use *TheaterEmbedEventUse) {
	if use == nil {
		return
	}
	use.store.Lock()
	defer use.store.Unlock()
	for _, key := range use.keys {
		delete(use.receipt.Inflight, key)
	}
	use.keys = nil
}

// Theater context only locates existing state; it grants no trust or action selection.
type TheaterEmbedPublishContext struct {
	WorldID   string `json:"worldId"`
	ScopeType string `json:"scopeType"`
	ChannelID string `json:"channelId"`
	ObjectID  string `json:"objectId"`
}

// Validate before dispatch, then record only after successful dispatch.
func ValidateTheaterEmbedEventPublish(cfg utils.AppConfig, actorID, worldID, channelID, formID, topic string, target TheaterEmbedPublishContext) (*theaterEmbedEventReceiptScope, error) {
	if target.WorldID != worldID || (target.ScopeType != "world" && target.ScopeType != "channel") || (target.ScopeType == "world" && target.ChannelID != "") || (target.ScopeType == "channel" && target.ChannelID != channelID) {
		return nil, theaterPayloadError("Theater 嵌入上下文不匹配")
	}
	if _, _, err := requireTheaterPermission(actorID, worldID, target.ChannelID, TheaterPermissionActionTrigger); err != nil {
		return nil, err
	}
	room, err := model.TheaterRoomFindByScope(worldID, target.ChannelID)
	if err != nil {
		return nil, err
	}
	if room == nil || room.Status != "active" {
		return nil, theaterPayloadError("Theater 房间不存在")
	}
	object, err := loadTheaterObject(model.GetDB(), room.ID, target.ObjectID)
	if err != nil {
		return nil, err
	}
	u, err := theaterIframeURL(object)
	if err != nil || !IsTheaterInternalIFormURL(cfg, u) || !theaterIframeFormMatches(u, worldID, channelID, formID) || object.Kind != "iframe" || !object.Interactive {
		return nil, theaterPayloadError("Theater 对象不是当前频道的内部 iForm")
	}
	bindings, _, err := theaterObjectEmbedEventBindings([]byte(object.MetadataJSON))
	if err != nil {
		return nil, err
	}
	for _, binding := range bindings {
		if binding.Topic == topic && len(binding.ActionIDs) > 0 {
			return &theaterEmbedEventReceiptScope{actorID, worldID, channelID, formID, topic, room.ID, object.ID, u.String()}, nil
		}
	}
	return nil, theaterPayloadError("Theater 对象未绑定该 topic")
}

// Receipts are bounded process-local state, never persisted or broadcast.
func RecordTheaterEmbedEventReceipt(eventID string, scope *theaterEmbedEventReceiptScope) {
	if eventID != "" && scope != nil {
		theaterEmbedEventReceipts.record(eventID, *scope, time.Now())
	}
}

// IsTheaterInternalIFormURL validates the configured public origins and webUrl.
// DomainList preserves config.domain's semicolon-separated multi-domain semantics.
func IsTheaterInternalIFormURL(cfg utils.AppConfig, u *url.URL) bool {
	if u == nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	base := "/" + strings.Trim(cfg.WebUrl, "/")
	if base != "/" && u.Path != base && !strings.HasPrefix(u.Path, base+"/") {
		return false
	}
	for _, domain := range utils.DomainList(cfg.Domain) {
		if !strings.Contains(domain, "://") {
			domain = u.Scheme + "://" + domain
		}
		public, err := url.Parse(domain)
		if err == nil && public.User == nil && public.RawQuery == "" && public.Fragment == "" && strings.EqualFold(public.Scheme, u.Scheme) && strings.EqualFold(public.Host, u.Host) {
			return true
		}
	}
	return false
}

func theaterIframeURL(object *model.TheaterObjectModel) (*url.URL, error) {
	var content struct {
		Iframe struct {
			URL string `json:"url"`
		} `json:"iframe"`
	}
	if err := json.Unmarshal([]byte(object.ContentJSON), &content); err != nil {
		return nil, err
	}
	return url.Parse(strings.TrimSpace(content.Iframe.URL))
}

func theaterIframeFormMatches(u *url.URL, worldID, channelID, formID string) bool {
	fragment, err := url.Parse(u.Fragment)
	if err != nil || fragment.Path != "/internal/iform/"+formID {
		return false
	}
	query, err := url.ParseQuery(fragment.RawQuery)
	return err == nil && len(query["world"]) == 1 && len(query["channel"]) == 1 && query.Get("world") == worldID && query.Get("channel") == channelID && channelID != ""
}

func theaterEmbedEventUseKey(actionID, stepID, entryID string) string {
	key, _ := json.Marshal([]string{actionID, strings.TrimSpace(stepID), strings.TrimSpace(entryID)})
	return string(key)
}

func BeginTheaterEmbedEventUse(actorID, worldID, channelID, inputChannelID string, object *model.TheaterObjectModel, actionIDs []string, stepID, entryID string, event *TheaterEmbedEventContext) (*TheaterEmbedEventUse, error) {
	denied := func() error {
		return newTheaterError(TheaterErrorPermissionDenied, "iframe 动作需要有效的嵌入事件凭据", 403, nil)
	}
	if event == nil || event.EventID == "" || len(event.EventID) > 128 || event.FormID == "" || len(event.FormID) > 100 || !ChannelEmbedTopicPattern.MatchString(event.Topic) {
		return nil, denied()
	}
	bindings, _, err := theaterObjectEmbedEventBindings([]byte(object.MetadataJSON))
	if err != nil {
		return nil, denied()
	}
	bound := map[string]bool{}
	for _, binding := range bindings {
		if binding.Topic == event.Topic {
			for _, id := range binding.ActionIDs {
				bound[id] = true
			}
		}
	}
	for _, id := range actionIDs {
		if !bound[id] {
			return nil, denied()
		}
	}
	u, err := theaterIframeURL(object)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, denied()
	}
	fragment, err := url.Parse(u.Fragment)
	if err != nil || fragment.Path != "/internal/iform/"+event.FormID {
		return nil, denied()
	}
	query, err := url.ParseQuery(fragment.RawQuery)
	if err != nil || len(query["world"]) != 1 || len(query["channel"]) != 1 || query.Get("world") != worldID {
		return nil, denied()
	}
	frameChannelID := query.Get("channel")
	if frameChannelID == "" || (channelID != "" && frameChannelID != channelID) || (strings.TrimSpace(inputChannelID) != "" && frameChannelID != strings.TrimSpace(inputChannelID)) {
		return nil, denied()
	}
	keys := make([]string, len(actionIDs))
	for i, id := range actionIDs {
		keys[i] = theaterEmbedEventUseKey(id, stepID, entryID)
	}
	use := theaterEmbedEventReceipts.begin(event.EventID, theaterEmbedEventReceiptScope{actorID, worldID, frameChannelID, event.FormID, event.Topic, object.RoomID, object.ID, u.String()}, keys, time.Now())
	if use == nil {
		return nil, denied()
	}
	return use, nil
}
