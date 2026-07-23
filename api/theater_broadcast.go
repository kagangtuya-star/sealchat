package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	"sealchat/model"
	"sealchat/protocol"
	"sealchat/service"
	"sealchat/utils"
)

const (
	theaterQueueMaxEvents = 256
	theaterQueueMaxBytes  = 2 << 20
)

var theaterResourceBroadcastThrottle = struct {
	sync.Mutex
	last map[string]time.Time
}{last: map[string]time.Time{}}

type theaterSubscription struct {
	WorldID       string
	ChannelID     string
	KnownRevision int64
}

type theaterQueuedEvent struct {
	value any
	size  int
}

type theaterWriteQueue struct {
	mu         sync.Mutex
	cond       *sync.Cond
	items      []theaterQueuedEvent
	bytes      int
	maxEvents  int
	maxBytes   int
	pendingGap any
	closed     bool
	write      func(any) error
	closeConn  func() error
}

func newTheaterWriteQueue(write func(any) error, closeConn func() error) *theaterWriteQueue {
	return newTheaterWriteQueueWithLimits(write, closeConn, theaterQueueMaxEvents, theaterQueueMaxBytes)
}

func newTheaterWriteQueueWithLimits(write func(any) error, closeConn func() error, maxEvents, maxBytes int) *theaterWriteQueue {
	if maxEvents <= 0 {
		maxEvents = theaterQueueMaxEvents
	}
	if maxBytes <= 0 {
		maxBytes = theaterQueueMaxBytes
	}
	queue := &theaterWriteQueue{write: write, closeConn: closeConn, maxEvents: maxEvents, maxBytes: maxBytes}
	queue.cond = sync.NewCond(&queue.mu)
	go queue.run()
	return queue
}

func (queue *theaterWriteQueue) Enqueue(value any, gap any) bool {
	if queue == nil || value == nil {
		return false
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return false
	}
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if queue.closed {
		return false
	}
	if len(queue.items)+1 > queue.maxEvents || queue.bytes+len(raw) > queue.maxBytes {
		queue.items = nil
		queue.bytes = 0
		queue.pendingGap = gap
		service.RecordTheaterMetric("theater_event_gap_total", nil, 1)
		service.RecordTheaterMetric("theater_ws_slow_client_total", nil, 1)
		queue.cond.Signal()
		return false
	}
	queue.items = append(queue.items, theaterQueuedEvent{value: value, size: len(raw)})
	queue.bytes += len(raw)
	queue.cond.Signal()
	return true
}

func (queue *theaterWriteQueue) Close() {
	if queue == nil {
		return
	}
	queue.mu.Lock()
	queue.closed = true
	queue.items = nil
	queue.pendingGap = nil
	queue.cond.Broadcast()
	queue.mu.Unlock()
}

func (queue *theaterWriteQueue) run() {
	for {
		queue.mu.Lock()
		for !queue.closed && queue.pendingGap == nil && len(queue.items) == 0 {
			queue.cond.Wait()
		}
		if queue.closed {
			queue.mu.Unlock()
			return
		}
		var value any
		if queue.pendingGap != nil {
			value = queue.pendingGap
			queue.pendingGap = nil
		} else {
			item := queue.items[0]
			queue.items = queue.items[1:]
			queue.bytes -= item.size
			value = item.value
		}
		queue.mu.Unlock()
		if queue.write == nil || queue.write(value) != nil {
			service.RecordTheaterMetric("theater_ws_slow_client_total", nil, 1)
			if queue.closeConn != nil {
				_ = queue.closeConn()
			}
			queue.Close()
			return
		}
	}
}

type theaterSnapshotPayload struct {
	Revision      int64  `json:"revision"`
	SchemaVersion int    `json:"schemaVersion"`
	Checksum      string `json:"checksum"`
	Reason        string `json:"reason"`
	SnapshotURL   string `json:"snapshotUrl"`
}

func theaterSnapshotEvent(worldID, channelID, roomID string, revision int64, schemaVersion int, checksum, reason string) protocol.GatewayPayloadStructure {
	return theaterSnapshotEventWithURL(worldID, channelID, roomID, revision, schemaVersion, checksum, reason, theaterSnapshotURL(worldID, channelID, ""))
}

func theaterSnapshotEventForConnection(info *ConnInfo, worldID, channelID, roomID string, revision int64, schemaVersion int, checksum, reason string) protocol.GatewayPayloadStructure {
	observerSlug := ""
	if info != nil && info.IsObserver && info.ObserverWorldID == worldID {
		observerSlug = info.ObserverSlug
	}
	return theaterSnapshotEventWithURL(worldID, channelID, roomID, revision, schemaVersion, checksum, reason, theaterSnapshotURL(worldID, channelID, observerSlug))
}

func theaterSnapshotEventWithURL(worldID, channelID, roomID string, revision int64, schemaVersion int, checksum, reason, snapshotURL string) protocol.GatewayPayloadStructure {
	payload := theaterSnapshotPayload{
		Revision: revision, SchemaVersion: schemaVersion, Checksum: checksum, Reason: reason,
		SnapshotURL: snapshotURL,
	}
	return theaterGatewayEvent(protocol.EventTheaterSnapshot, worldID, channelID, roomID, revision, "snapshot:"+roomID+":"+reason, payload)
}

func theaterSnapshotURL(worldID, channelID, observerSlug string) string {
	if strings.TrimSpace(channelID) == "" && observerSlug != "" {
		return "/api/v1/public/ob/" + url.PathEscape(observerSlug) + "/theater"
	}
	if strings.TrimSpace(channelID) == "" {
		return "/api/v1/worlds/" + url.PathEscape(worldID) + "/theater"
	}
	if observerSlug != "" {
		return "/api/v1/public/ob/channels/" + url.PathEscape(channelID) + "/theater?ob_slug=" + url.QueryEscape(observerSlug)
	}
	return "/api/v1/worlds/" + url.PathEscape(worldID) + "/channels/" + url.PathEscape(channelID) + "/theater"
}

func theaterGatewayEvent(eventType protocol.EventName, worldID, channelID, roomID string, revision int64, eventID string, payload any) protocol.GatewayPayloadStructure {
	now := time.Now()
	return protocol.GatewayPayloadStructure{Op: protocol.OpEvent, Body: protocol.Event{
		Type: eventType, Timestamp: now.Unix(), Theater: &protocol.TheaterEventPayload{
			WorldID: worldID, ChannelID: channelID, RoomID: roomID, Revision: revision,
			EventID: eventID, Timestamp: now.UnixMilli(), Payload: payload,
		},
	}}
}

func (info *ConnInfo) setTheaterSubscription(subscription *theaterSubscription) {
	info.theaterMu.Lock()
	info.theaterSubscription = subscription
	info.theaterMu.Unlock()
}

func (info *ConnInfo) theaterState() (*theaterSubscription, *theaterWriteQueue) {
	if info == nil {
		return nil, nil
	}
	info.theaterMu.RLock()
	defer info.theaterMu.RUnlock()
	return info.theaterSubscription, info.theaterQueue
}

func (info *ConnInfo) ensureTheaterQueue() *theaterWriteQueue {
	if info == nil || info.Conn == nil {
		return nil
	}
	info.theaterMu.Lock()
	defer info.theaterMu.Unlock()
	if info.theaterQueue == nil {
		info.theaterQueue = newTheaterWriteQueue(info.Conn.WriteJSON, info.Conn.Close)
	}
	return info.theaterQueue
}

func (info *ConnInfo) closeTheaterQueue() {
	if info == nil {
		return
	}
	info.theaterMu.Lock()
	queue := info.theaterQueue
	info.theaterQueue = nil
	info.theaterSubscription = nil
	info.theaterMu.Unlock()
	if queue != nil {
		queue.Close()
	}
}

type LocalTheaterEventPublisher struct{}

func publishTheaterEffectTriggered(worldID, channelID string, effect *service.TheaterEffectActionResult) error {
	if effect == nil || userId2ConnInfoGlobal == nil {
		return nil
	}
	var room model.TheaterRoomModel
	if err := model.GetDB().Where("id = ? AND world_id = ? AND channel_id = ?", effect.RoomID, worldID, channelID).First(&room).Error; err != nil {
		return err
	}
	event := theaterGatewayEvent(protocol.EventTheaterEffectTriggered, worldID, channelID, room.ID, effect.Revision, effect.TriggerID, map[string]any{
		"triggerId": effect.TriggerID,
		"effectId":  effect.EffectID,
	})
	userId2ConnInfoGlobal.Range(func(userID string, connMap *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		connMap.Range(func(_ *WsSyncConn, info *ConnInfo) bool {
			if info == nil || info.User == nil || info.User.IsBot || !canConnectionViewTheater(userID, info, worldID, channelID) {
				return true
			}
			subscription, queue := info.theaterState()
			if subscription == nil || queue == nil || subscription.WorldID != worldID || subscription.ChannelID != channelID {
				return true
			}
			gap := theaterSnapshotEventForConnection(info, worldID, channelID, room.ID, room.Revision, room.SchemaVersion, room.StateHash, "gap")
			queue.Enqueue(event, gap)
			return true
		})
		return true
	})
	return nil
}

func (LocalTheaterEventPublisher) PublishTheaterMutation(_ context.Context, mutation model.TheaterMutationModel) error {
	if mutation.Status == "rejected" {
		return publishRejectedTheaterMutation(mutation)
	}
	if mutation.Status != "applied" || mutation.RevisionAfter == nil {
		return nil
	}
	if userId2ConnInfoGlobal == nil {
		return errors.New("websocket connection registry unavailable")
	}
	var result service.TheaterMutationResult
	_ = json.Unmarshal([]byte(mutation.ResultJSON), &result)
	payload := map[string]any{
		"mutationId": mutation.MutationID, "revisionBefore": mutation.RevisionBefore,
		"revision": *mutation.RevisionAfter, "type": mutation.Type,
		"payload": json.RawMessage(mutation.PayloadJSON), "actorUserId": mutation.ActorUserID,
		"checksum": result.Checksum,
	}
	event := theaterGatewayEvent(protocol.EventTheaterMutationApplied, mutation.WorldID, mutation.ChannelID, mutation.RoomID, *mutation.RevisionAfter, mutation.ID, payload)
	managementMutation := mutation.Type == service.TheaterMutationAdminRestore || mutation.Type == service.TheaterMutationAdminReplace || mutation.Type == service.TheaterMutationAdminPackageImport
	userId2ConnInfoGlobal.Range(func(userID string, connMap *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		connMap.Range(func(_ *WsSyncConn, info *ConnInfo) bool {
			if info == nil || info.User == nil || info.User.IsBot {
				return true
			}
			if !canConnectionViewTheater(userID, info, mutation.WorldID, mutation.ChannelID) {
				return true
			}
			subscription, queue := info.theaterState()
			if subscription == nil || queue == nil || subscription.WorldID != mutation.WorldID || subscription.ChannelID != mutation.ChannelID {
				return true
			}
			gap := theaterSnapshotEventForConnection(info, mutation.WorldID, mutation.ChannelID, mutation.RoomID, *mutation.RevisionAfter, model.TheaterSchemaVersion, result.Checksum, "gap")
			if !service.CanReceiveFullTheaterState(userID, mutation.WorldID, mutation.ChannelID) {
				queue.Enqueue(theaterSnapshotEventForConnection(info, mutation.WorldID, mutation.ChannelID, mutation.RoomID, *mutation.RevisionAfter, model.TheaterSchemaVersion, result.Checksum, "mutation"), gap)
				return true
			}
			if managementMutation && !service.CanAdministerTheater(userID, mutation.WorldID, mutation.ChannelID) {
				queue.Enqueue(theaterSnapshotEventForConnection(info, mutation.WorldID, mutation.ChannelID, mutation.RoomID, *mutation.RevisionAfter, model.TheaterSchemaVersion, result.Checksum, "admin-replace"), gap)
				return true
			}
			queue.Enqueue(event, gap)
			if managementMutation {
				queue.Enqueue(theaterSnapshotEventForConnection(info, mutation.WorldID, mutation.ChannelID, mutation.RoomID, *mutation.RevisionAfter, model.TheaterSchemaVersion, result.Checksum, "admin-replace"), gap)
			}
			return true
		})
		return true
	})
	return nil
}

func publishRejectedTheaterMutation(mutation model.TheaterMutationModel) error {
	if userId2ConnInfoGlobal == nil {
		return errors.New("websocket connection registry unavailable")
	}
	connMap, ok := userId2ConnInfoGlobal.Load(mutation.ActorUserID)
	if !ok || connMap == nil {
		return nil
	}
	payload := map[string]any{
		"mutationId": mutation.MutationID, "errorCode": mutation.RejectCode,
		"currentRevision": mutation.RevisionBefore,
		"resyncRequired":  mutation.RejectCode == service.TheaterErrorRevisionConflict,
	}
	event := theaterGatewayEvent(protocol.EventTheaterMutationRejected, mutation.WorldID, mutation.ChannelID, mutation.RoomID, mutation.RevisionBefore, mutation.ID, payload)
	gap := theaterSnapshotEvent(mutation.WorldID, mutation.ChannelID, mutation.RoomID, mutation.RevisionBefore, model.TheaterSchemaVersion, "", "gap")
	connMap.Range(func(_ *WsSyncConn, info *ConnInfo) bool {
		if info == nil || info.User == nil || info.User.IsBot {
			return true
		}
		subscription, queue := info.theaterState()
		if subscription != nil && queue != nil && subscription.WorldID == mutation.WorldID && subscription.ChannelID == mutation.ChannelID {
			queue.Enqueue(event, gap)
		}
		return true
	})
	return nil
}

func (LocalTheaterEventPublisher) PublishTheaterResource(_ context.Context, resource model.TheaterResourceModel) error {
	if userId2ConnInfoGlobal == nil {
		return errors.New("websocket connection registry unavailable")
	}
	var room model.TheaterRoomModel
	if err := model.GetDB().Where("id = ?", resource.RoomID).First(&room).Error; err != nil {
		return err
	}
	eventType := protocol.EventTheaterResourceProcessing
	switch resource.Status {
	case "ready":
		eventType = protocol.EventTheaterResourceReady
	case "failed":
		eventType = protocol.EventTheaterResourceFailed
	default:
		if !allowTheaterResourceProgressBroadcast(resource.ID, time.Now()) {
			return nil
		}
	}
	public, err := service.TheaterResourcePublicForEvent(resource)
	if err != nil {
		return err
	}
	userId2ConnInfoGlobal.Range(func(userID string, connMap *utils.SyncMap[*WsSyncConn, *ConnInfo]) bool {
		full := userID == resource.CreatedBy || service.CanManageTheaterResources(userID, room.WorldID, room.ChannelID)
		connMap.Range(func(_ *WsSyncConn, info *ConnInfo) bool {
			if info == nil || info.User == nil || info.User.IsBot {
				return true
			}
			if !canConnectionViewTheater(userID, info, room.WorldID, room.ChannelID) || (!full && resource.ReferenceCount <= 0) {
				return true
			}
			payload := any(public)
			if !full && resource.Status == "failed" {
				payload = map[string]any{"id": resource.ID, "status": "failed", "errorCode": "RESOURCE_UNAVAILABLE", "retryable": false}
			} else if !full && eventType == protocol.EventTheaterResourceProcessing {
				payload = map[string]any{"id": resource.ID, "status": resource.Status, "progress": float64(int(resource.ProcessingProgress*4)) / 4}
			}
			event := theaterGatewayEvent(eventType, room.WorldID, room.ChannelID, room.ID, room.Revision, resource.ID+":"+resource.Status+":"+resource.UpdatedAt.UTC().Format(time.RFC3339Nano), payload)
			subscription, queue := info.theaterState()
			if subscription != nil && queue != nil && subscription.WorldID == room.WorldID && subscription.ChannelID == room.ChannelID {
				gap := theaterSnapshotEventForConnection(info, room.WorldID, room.ChannelID, room.ID, room.Revision, room.SchemaVersion, room.StateHash, "gap")
				queue.Enqueue(event, gap)
			}
			return true
		})
		return true
	})
	return nil
}

func canConnectionViewTheater(userID string, info *ConnInfo, worldID, channelID string) bool {
	if service.CanViewTheater(userID, worldID, channelID) {
		return true
	}
	if info == nil || !info.IsObserver || info.ObserverWorldID != worldID {
		return false
	}
	world, _, err := service.ResolveWorldObserverLink(info.ObserverSlug)
	if err != nil || world == nil || world.ID != worldID {
		return false
	}
	if strings.TrimSpace(channelID) == "" {
		return true
	}
	_, err = service.CanObserverAccessChannel(channelID, worldID)
	return err == nil
}

func allowTheaterResourceProgressBroadcast(resourceID string, now time.Time) bool {
	theaterResourceBroadcastThrottle.Lock()
	defer theaterResourceBroadcastThrottle.Unlock()
	last := theaterResourceBroadcastThrottle.last[resourceID]
	if !last.IsZero() && now.Sub(last) < 500*time.Millisecond {
		return false
	}
	theaterResourceBroadcastThrottle.last[resourceID] = now
	return true
}
