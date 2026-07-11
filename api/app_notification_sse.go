package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/model"
	"sealchat/service"
)

const appNotificationMaxEventBytes = 64 * 1024

type appNotificationStreamRegistration struct {
	generation uint64
	cancel     chan struct{}
}

var appNotificationStreamRegistry = struct {
	sync.Mutex
	next    uint64
	streams map[string]appNotificationStreamRegistration
}{streams: map[string]appNotificationStreamRegistration{}}

func AppNotificationStream(c *fiber.Ctx) error {
	device := appNotificationDeviceFromContext(c)
	cursor := strings.TrimSpace(c.Get("Last-Event-ID"))
	initial, notify, cancelSubscription, err := service.DefaultAppNotificationHub.SubscribeAfter(device.ID, cursor)
	if err != nil {
		latestEventID, latestSequence := service.DefaultAppNotificationHub.LatestCursor(device.ID)
		if latestSequence == 0 {
			latestSequence = device.LastSequence
		}
		return c.Status(http.StatusGone).JSON(fiber.Map{
			"error":        fiber.Map{"code": "event_cursor_expired", "message": "指定的事件位置已失效", "request_id": "req_" + device.ID},
			"stream_reset": fiber.Map{"latest_event_id": latestEventID, "latest_sequence": latestSequence},
		})
	}
	streamCanceled, releaseStream := replaceAppNotificationStream(device.ID)
	_ = model.MarkAppNotificationDeviceConnected(device.ID, time.Now())
	c.Set(fiber.HeaderContentType, "text/event-stream; charset=utf-8")
	c.Set(fiber.HeaderCacheControl, "no-cache, no-transform")
	c.Set("X-Accel-Buffering", "no")
	c.Context().SetBodyStreamWriter(func(writer *bufio.Writer) {
		defer releaseStream()
		defer cancelSubscription()
		lastEventID := cursor
		if !writeAppNotificationEvents(writer, initial, &lastEventID) {
			return
		}
		ticker := time.NewTicker(appNotificationHeartbeat * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-streamCanceled:
				return
			case <-notify:
				events, eventErr := service.DefaultAppNotificationHub.EventsAfter(device.ID, lastEventID)
				if eventErr != nil || !writeAppNotificationEvents(writer, events, &lastEventID) {
					return
				}
			case now := <-ticker.C:
				if _, err := fmt.Fprintf(writer, ": heartbeat %s\n\n", now.UTC().Format(time.RFC3339)); err != nil || writer.Flush() != nil {
					return
				}
			}
		}
	})
	return nil
}

func AppNotificationAcks(c *fiber.Ctx) error {
	device := appNotificationDeviceFromContext(c)
	var body struct {
		DeviceID string                       `json:"device_id"`
		Acks     []service.AppNotificationAck `json:"acks"`
	}
	if err := c.BodyParser(&body); err != nil || strings.TrimSpace(body.DeviceID) != device.ID || len(body.Acks) > 100 {
		return sendAppNotificationError(c, http.StatusBadRequest, "invalid_request", "ACK 请求无效")
	}
	return c.JSON(service.DefaultAppNotificationHub.Ack(device.ID, body.Acks))
}

func encodeAppNotificationSSE(event service.AppNotificationEvent) ([]byte, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	if len(data) > appNotificationMaxEventBytes {
		return nil, fmt.Errorf("notification event exceeds %d bytes", appNotificationMaxEventBytes)
	}
	var frame bytes.Buffer
	fmt.Fprintf(&frame, "id: %s\nevent: notification\nretry: 5000\ndata: ", event.EventID)
	frame.Write(data)
	frame.WriteString("\n\n")
	return frame.Bytes(), nil
}

func writeAppNotificationEvents(writer *bufio.Writer, events []service.AppNotificationEvent, lastEventID *string) bool {
	for _, event := range events {
		frame, err := encodeAppNotificationSSE(event)
		if err != nil {
			continue
		}
		if _, err := writer.Write(frame); err != nil || writer.Flush() != nil {
			return false
		}
		*lastEventID = event.EventID
	}
	return true
}

func replaceAppNotificationStream(deviceID string) (<-chan struct{}, func()) {
	deviceID = strings.TrimSpace(deviceID)
	canceled := make(chan struct{})

	appNotificationStreamRegistry.Lock()
	previous, hasPrevious := appNotificationStreamRegistry.streams[deviceID]
	appNotificationStreamRegistry.next++
	generation := appNotificationStreamRegistry.next
	appNotificationStreamRegistry.streams[deviceID] = appNotificationStreamRegistration{
		generation: generation,
		cancel:     canceled,
	}
	appNotificationStreamRegistry.Unlock()

	if hasPrevious {
		close(previous.cancel)
	}

	return canceled, func() {
		appNotificationStreamRegistry.Lock()
		current, ok := appNotificationStreamRegistry.streams[deviceID]
		if ok && current.generation == generation {
			delete(appNotificationStreamRegistry.streams, deviceID)
		}
		appNotificationStreamRegistry.Unlock()
	}
}

func cancelAppNotificationStream(deviceID string) {
	deviceID = strings.TrimSpace(deviceID)
	appNotificationStreamRegistry.Lock()
	current, ok := appNotificationStreamRegistry.streams[deviceID]
	if ok {
		delete(appNotificationStreamRegistry.streams, deviceID)
	}
	appNotificationStreamRegistry.Unlock()
	if ok {
		close(current.cancel)
	}
}
