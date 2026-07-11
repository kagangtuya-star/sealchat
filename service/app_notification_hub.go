package service

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	"sealchat/utils"
)

const (
	defaultAppNotificationMaxEvents = 500
	defaultAppNotificationRetention = time.Hour
)

var (
	ErrAppNotificationEventCursorExpired       = errors.New("app notification event cursor expired")
	ErrAppNotificationAuthorizationCodeInvalid = errors.New("app notification authorization code invalid")
	ErrAppNotificationManualCodeUnavailable    = errors.New("app notification manual code unavailable")
	DefaultAppNotificationHub                  = NewAppNotificationHub(AppNotificationHubOptions{})
	appNotificationCleanupOnce                 sync.Once
)

type AppNotificationEvent struct {
	SchemaVersion string                         `json:"schema_version"`
	EventID       string                         `json:"event_id"`
	Sequence      uint64                         `json:"sequence"`
	EventType     string                         `json:"event_type"`
	InstanceID    string                         `json:"instance_id"`
	CreatedAt     time.Time                      `json:"created_at"`
	ExpiresAt     time.Time                      `json:"expires_at"`
	DedupeKey     string                         `json:"dedupe_key"`
	Notification  AppNotificationDisplay         `json:"notification"`
	Context       AppNotificationEventContext    `json:"context"`
	Navigation    AppNotificationEventNavigation `json:"navigation"`
}

type AppNotificationDisplay struct {
	Channel     string `json:"channel"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	CollapseKey string `json:"collapse_key"`
	Silent      bool   `json:"silent"`
	Sensitive   bool   `json:"sensitive"`
}

type AppNotificationEntity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type AppNotificationMessageContext struct {
	ID        string `json:"id"`
	IsWhisper bool   `json:"is_whisper"`
}

type AppNotificationSenderContext struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

type AppNotificationEventContext struct {
	World   AppNotificationEntity         `json:"world"`
	Channel AppNotificationEntity         `json:"channel"`
	Message AppNotificationMessageContext `json:"message"`
	Sender  AppNotificationSenderContext  `json:"sender"`
}

type AppNotificationEventNavigation struct {
	OpenPath     string `json:"open_path"`
	FallbackPath string `json:"fallback_path"`
}

type AppNotificationAuthorizationRequest struct {
	UserID         string
	InstallationID string
	CodeChallenge  string
	ClientID       string
	RedirectURI    string
	State          string
	ExpiresAt      time.Time
}

type AppNotificationAckState string

const (
	AppNotificationAckReceived  AppNotificationAckState = "received"
	AppNotificationAckDisplayed AppNotificationAckState = "displayed"
	AppNotificationAckOpened    AppNotificationAckState = "opened"
	AppNotificationAckDismissed AppNotificationAckState = "dismissed"
)

type AppNotificationAck struct {
	EventID string                  `json:"event_id"`
	State   AppNotificationAckState `json:"state"`
	At      time.Time               `json:"at"`
}

type AppNotificationAckRejected struct {
	EventID string `json:"event_id"`
	Reason  string `json:"reason"`
}

type AppNotificationAckResult struct {
	Accepted []string                     `json:"accepted"`
	Rejected []AppNotificationAckRejected `json:"rejected"`
}

type AppNotificationHubOptions struct {
	MaxEventsPerDevice int
	Retention          time.Duration
	Now                func() time.Time
}

type appNotificationDeviceQueue struct {
	events      []AppNotificationEvent
	dedupeKeys  map[string]struct{}
	acks        map[string]AppNotificationAck
	subscribers map[uint64]chan struct{}
	nextSubID   uint64
}

type AppNotificationHub struct {
	mu             sync.Mutex
	queues         map[string]*appNotificationDeviceQueue
	authorizations map[string]AppNotificationAuthorizationRequest
	codes          map[string]AppNotificationAuthorizationRequest
	manualCodes    map[string]AppNotificationAuthorizationRequest
	maxEvents      int
	retention      time.Duration
	now            func() time.Time
}

func NewAppNotificationHub(options AppNotificationHubOptions) *AppNotificationHub {
	if options.MaxEventsPerDevice <= 0 {
		options.MaxEventsPerDevice = defaultAppNotificationMaxEvents
	}
	if options.Retention <= 0 {
		options.Retention = defaultAppNotificationRetention
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return &AppNotificationHub{
		queues:         map[string]*appNotificationDeviceQueue{},
		authorizations: map[string]AppNotificationAuthorizationRequest{},
		codes:          map[string]AppNotificationAuthorizationRequest{},
		manualCodes:    map[string]AppNotificationAuthorizationRequest{},
		maxEvents:      options.MaxEventsPerDevice,
		retention:      options.Retention,
		now:            options.Now,
	}
}

func (h *AppNotificationHub) CreateManualAuthorizationCode(userID string, expiresAt time.Time) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanupLocked()
	userID = strings.TrimSpace(userID)
	if userID == "" || !expiresAt.After(h.now()) {
		return "", ErrAppNotificationManualCodeUnavailable
	}
	for code, request := range h.manualCodes {
		if request.UserID == userID {
			delete(h.manualCodes, code)
		}
	}
	for range 20 {
		value, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
		if err != nil {
			return "", err
		}
		code := fmt.Sprintf("%06d", value.Int64())
		if _, exists := h.manualCodes[code]; exists {
			continue
		}
		h.manualCodes[code] = AppNotificationAuthorizationRequest{UserID: userID, ExpiresAt: expiresAt}
		return code, nil
	}
	return "", ErrAppNotificationManualCodeUnavailable
}

func (h *AppNotificationHub) RedeemManualAuthorizationCode(code string) (AppNotificationAuthorizationRequest, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanupLocked()
	code = strings.TrimSpace(code)
	request, ok := h.manualCodes[code]
	delete(h.manualCodes, code)
	if !ok || !request.ExpiresAt.After(h.now()) {
		return AppNotificationAuthorizationRequest{}, ErrAppNotificationAuthorizationCodeInvalid
	}
	return request, nil
}

func StartAppNotificationCleanup() {
	appNotificationCleanupOnce.Do(func() {
		hub := DefaultAppNotificationHub
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				hub.CleanupExpired()
			}
		}()
	})
}

func (h *AppNotificationHub) StoreAuthorizationRequest(request AppNotificationAuthorizationRequest) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanupLocked()
	id := "authreq_" + utils.NewID()
	h.authorizations[id] = request
	return id
}

func (h *AppNotificationHub) ApproveAuthorization(requestID string) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanupLocked()
	request, ok := h.authorizations[strings.TrimSpace(requestID)]
	if !ok || !request.ExpiresAt.After(h.now()) {
		return "", ErrAppNotificationAuthorizationCodeInvalid
	}
	delete(h.authorizations, requestID)
	code := "ac_" + utils.NewID()
	h.codes[code] = request
	return code, nil
}

func (h *AppNotificationHub) GetAuthorizationRequest(requestID string) (AppNotificationAuthorizationRequest, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanupLocked()
	request, ok := h.authorizations[strings.TrimSpace(requestID)]
	return request, ok
}

func (h *AppNotificationHub) DenyAuthorization(requestID string) (AppNotificationAuthorizationRequest, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanupLocked()
	requestID = strings.TrimSpace(requestID)
	request, ok := h.authorizations[requestID]
	delete(h.authorizations, requestID)
	return request, ok
}

func (h *AppNotificationHub) RedeemAuthorizationCode(code string) (AppNotificationAuthorizationRequest, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanupLocked()
	code = strings.TrimSpace(code)
	request, ok := h.codes[code]
	delete(h.codes, code)
	if !ok || !request.ExpiresAt.After(h.now()) {
		return AppNotificationAuthorizationRequest{}, ErrAppNotificationAuthorizationCodeInvalid
	}
	return request, nil
}

func (h *AppNotificationHub) Enqueue(deviceID string, event AppNotificationEvent) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" || event.EventID == "" || event.DedupeKey == "" {
		return false
	}
	queue := h.queueLocked(deviceID)
	h.cleanupQueueLocked(queue)
	if _, exists := queue.dedupeKeys[event.DedupeKey]; exists {
		return false
	}
	if event.ExpiresAt.IsZero() || event.ExpiresAt.After(h.now().Add(h.retention)) {
		event.ExpiresAt = h.now().Add(h.retention)
	}
	queue.events = append(queue.events, event)
	queue.dedupeKeys[event.DedupeKey] = struct{}{}
	for len(queue.events) > h.maxEvents {
		h.removeOldestEventLocked(queue)
	}
	for _, subscriber := range queue.subscribers {
		select {
		case subscriber <- struct{}{}:
		default:
		}
	}
	return true
}

func (h *AppNotificationHub) EventsAfter(deviceID, eventID string) ([]AppNotificationEvent, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	queue := h.queueLocked(strings.TrimSpace(deviceID))
	h.cleanupQueueLocked(queue)
	return appNotificationEventsAfterLocked(queue, eventID)
}

func (h *AppNotificationHub) SubscribeAfter(deviceID, eventID string) ([]AppNotificationEvent, <-chan struct{}, func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	queue := h.queueLocked(strings.TrimSpace(deviceID))
	h.cleanupQueueLocked(queue)
	events, err := appNotificationEventsAfterLocked(queue, eventID)
	if err != nil {
		return nil, nil, func() {}, err
	}
	queue.nextSubID++
	id := queue.nextSubID
	ch := make(chan struct{}, 1)
	queue.subscribers[id] = ch
	cancel := func() {
		h.mu.Lock()
		delete(queue.subscribers, id)
		h.mu.Unlock()
	}
	return events, ch, cancel, nil
}

func (h *AppNotificationHub) LatestCursor(deviceID string) (string, uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	queue := h.queueLocked(strings.TrimSpace(deviceID))
	h.cleanupQueueLocked(queue)
	if len(queue.events) == 0 {
		return "", 0
	}
	latest := queue.events[len(queue.events)-1]
	return latest.EventID, latest.Sequence
}

func appNotificationEventsAfterLocked(queue *appNotificationDeviceQueue, eventID string) ([]AppNotificationEvent, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return append([]AppNotificationEvent{}, queue.events...), nil
	}
	for index := range queue.events {
		if queue.events[index].EventID == eventID {
			return append([]AppNotificationEvent{}, queue.events[index+1:]...), nil
		}
	}
	return nil, ErrAppNotificationEventCursorExpired
}

func (h *AppNotificationHub) ResetDevice(deviceID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	queue := h.queueLocked(strings.TrimSpace(deviceID))
	queue.events = nil
	queue.dedupeKeys = map[string]struct{}{}
	queue.acks = map[string]AppNotificationAck{}
}

func (h *AppNotificationHub) Ack(deviceID string, acks []AppNotificationAck) AppNotificationAckResult {
	h.mu.Lock()
	defer h.mu.Unlock()
	queue := h.queueLocked(strings.TrimSpace(deviceID))
	h.cleanupQueueLocked(queue)
	result := AppNotificationAckResult{Accepted: []string{}, Rejected: []AppNotificationAckRejected{}}
	for _, ack := range acks {
		if !queueHasEvent(queue, ack.EventID) {
			result.Rejected = append(result.Rejected, AppNotificationAckRejected{EventID: ack.EventID, Reason: "event_not_found"})
			continue
		}
		if appNotificationAckRank(ack.State) == 0 {
			result.Rejected = append(result.Rejected, AppNotificationAckRejected{EventID: ack.EventID, Reason: "invalid_state"})
			continue
		}
		if previous, ok := queue.acks[ack.EventID]; ok && appNotificationAckRank(ack.State) < appNotificationAckRank(previous.State) {
			result.Rejected = append(result.Rejected, AppNotificationAckRejected{EventID: ack.EventID, Reason: "state_regression"})
			continue
		}
		queue.acks[ack.EventID] = ack
		result.Accepted = append(result.Accepted, ack.EventID)
	}
	return result
}

func (h *AppNotificationHub) Subscribe(deviceID string) (<-chan struct{}, func()) {
	h.mu.Lock()
	queue := h.queueLocked(strings.TrimSpace(deviceID))
	queue.nextSubID++
	id := queue.nextSubID
	ch := make(chan struct{}, 1)
	queue.subscribers[id] = ch
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(queue.subscribers, id)
		h.mu.Unlock()
	}
}

func (h *AppNotificationHub) CleanupExpired() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanupLocked()
}

func (h *AppNotificationHub) queueLocked(deviceID string) *appNotificationDeviceQueue {
	queue, ok := h.queues[deviceID]
	if !ok {
		queue = &appNotificationDeviceQueue{
			dedupeKeys:  map[string]struct{}{},
			acks:        map[string]AppNotificationAck{},
			subscribers: map[uint64]chan struct{}{},
		}
		h.queues[deviceID] = queue
	}
	return queue
}

func (h *AppNotificationHub) cleanupLocked() {
	now := h.now()
	for id, request := range h.authorizations {
		if !request.ExpiresAt.After(now) {
			delete(h.authorizations, id)
		}
	}
	for code, request := range h.codes {
		if !request.ExpiresAt.After(now) {
			delete(h.codes, code)
		}
	}
	for code, request := range h.manualCodes {
		if !request.ExpiresAt.After(now) {
			delete(h.manualCodes, code)
		}
	}
	for _, queue := range h.queues {
		h.cleanupQueueLocked(queue)
	}
}

func (h *AppNotificationHub) cleanupQueueLocked(queue *appNotificationDeviceQueue) {
	for len(queue.events) > 0 && !queue.events[0].ExpiresAt.After(h.now()) {
		h.removeOldestEventLocked(queue)
	}
}

func (h *AppNotificationHub) removeOldestEventLocked(queue *appNotificationDeviceQueue) {
	if len(queue.events) == 0 {
		return
	}
	oldest := queue.events[0]
	queue.events = queue.events[1:]
	delete(queue.dedupeKeys, oldest.DedupeKey)
	delete(queue.acks, oldest.EventID)
}

func queueHasEvent(queue *appNotificationDeviceQueue, eventID string) bool {
	for _, event := range queue.events {
		if event.EventID == eventID {
			return true
		}
	}
	return false
}

func appNotificationAckRank(state AppNotificationAckState) int {
	switch state {
	case AppNotificationAckReceived:
		return 1
	case AppNotificationAckDisplayed:
		return 2
	case AppNotificationAckOpened, AppNotificationAckDismissed:
		return 3
	default:
		return 0
	}
}
