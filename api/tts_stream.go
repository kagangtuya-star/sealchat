package api

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"sealchat/model"
	"sealchat/pkg/ttsprovider"
	"sealchat/pm"
	"sealchat/service"
)

type ttsFrame struct {
	kind      int
	data      []byte
	messageID string
}
type ttsListener struct {
	user, channel string
	fileOnly      bool
	send          chan ttsFrame
	done          chan struct{}
}

var ttsHub = struct {
	sync.Mutex
	clients    map[*ttsListener]bool
	epochs     map[string]uint32
	messages   map[string]string
	suppressed map[string]string
	localPCM   map[string]uint32
}{clients: map[*ttsListener]bool{}, epochs: map[string]uint32{}, messages: map[string]string{}, suppressed: map[string]string{}, localPCM: map[string]uint32{}}

func ttsWSUpgrade(c *fiber.Ctx) error {
	if !ttsOriginMatchesHost(c.Get("Origin"), string(c.Context().Request.Header.Host())) {
		return c.SendStatus(403)
	}
	t, ok := service.TTSReadTicket(c.Query("ticket"), true)
	if !ok || t.Purpose != "ws" || !pm.CanWithChannelRole(t.UserID, t.ChannelID, pm.PermFuncChannelRead, pm.PermFuncChannelReadAll) {
		return c.SendStatus(403)
	}
	c.Locals("ttsUser", t.UserID)
	c.Locals("ttsChannel", t.ChannelID)
	c.Locals("ttsFileOnly", c.Query("mode") == "file")
	return c.Next()
}
func ttsWSHandler() fiber.Handler {
	return websocket.New(func(conn *websocket.Conn) {
		u, _ := conn.Locals("ttsUser").(string)
		ch, _ := conn.Locals("ttsChannel").(string)
		fileOnly, _ := conn.Locals("ttsFileOnly").(bool)
		l := &ttsListener{user: u, channel: ch, fileOnly: fileOnly, send: make(chan ttsFrame, 24), done: make(chan struct{})}
		ttsHub.Lock()
		count := 0
		for c := range ttsHub.clients {
			if c.user == u {
				count++
			}
		}
		if count >= 3 || len(ttsHub.clients) >= 100 {
			ttsHub.Unlock()
			conn.Close()
			return
		}
		ttsHub.clients[l] = true
		ttsHub.Unlock()
		defer func() { ttsHub.Lock(); delete(ttsHub.clients, l); ttsHub.Unlock(); close(l.done); conn.Close() }()
		readerDone := make(chan struct{})
		conn.SetReadLimit(4096)
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		})
		go func() {
			defer close(readerDone)
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
		ping := time.NewTicker(20 * time.Second)
		defer ping.Stop()
		for {
			select {
			case <-readerDone:
				return
			case f := <-l.send:
				if f.messageID != "" {
					m, err := ttsReadMessage(l.user, f.messageID)
					if err != nil || m.ChannelID != l.channel {
						return
					}
				}
				conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
				if conn.WriteMessage(f.kind, f.data) != nil {
					return
				}
			case <-ping.C:
				if !pm.CanWithChannelRole(u, ch, pm.PermFuncChannelRead, pm.PermFuncChannelReadAll) {
					return
				}
				conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
				if conn.WriteMessage(websocket.PingMessage, nil) != nil {
					return
				}
			}
		}
	})
}
func ttsWSTicket(c *fiber.Ctx) error {
	user := getCurUser(c)
	ch := c.Params("id")
	if !pm.CanWithChannelRole(user.ID, ch, pm.PermFuncChannelRead, pm.PermFuncChannelReadAll) {
		return c.SendStatus(403)
	}
	key, err := service.TTSIssueTicket(service.TTSTicket{UserID: user.ID, ChannelID: ch, Purpose: "ws", Expires: time.Now().Add(30 * time.Second)})
	if err != nil {
		return ttsError(c, err)
	}
	return c.JSON(fiber.Map{"ticket": key, "path": joinWebPath(appConfig.WebUrl, "api/v1/tts/ws")})
}
func ttsJSON(v any) ttsFrame {
	b, _ := json.Marshal(v)
	return ttsFrame{kind: websocket.TextMessage, data: b}
}
func ttsSend(l *ttsListener, f ttsFrame) bool {
	select {
	case <-l.done:
		return false
	default:
	}
	select {
	case l.send <- f:
		return true
	default:
		return false
	}
}

// Reports whether the lane must wait for the playback timeline of this file.
// After a PCM stream (streamed) the service owns any remaining lane wait: the
// archive is only announced, in file mode, so that connections which did not
// finish it can play the file. Whether a browser actually heard an utterance
// is known only to that browser, which drops archives it already played.
func ttsBroadcastReady(j model.TTSJob, media ttsprovider.Media, path string, streamed bool) bool {
	// Cache metadata intentionally omits file offsets. Recover them from the
	// validated archive before sending raw PCM, never send RIFF headers as PCM.
	if path != "" && media.Container == "wav" && !streamed {
		b, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		media, err = ttsprovider.InspectMedia(b)
		if err != nil {
			return false
		}
	}
	ttsHub.Lock()
	// Skip/stop of this message may land between realtime end and archive.
	if suppressed := ttsHub.suppressed[j.ChannelID]; suppressed != "" {
		delete(ttsHub.suppressed, j.ChannelID)
		if suppressed == j.MessageID {
			ttsHub.Unlock()
			return false
		}
	}
	ttsHub.epochs[j.ChannelID]++
	epoch := ttsHub.epochs[j.ChannelID]
	ttsHub.messages[j.ChannelID] = j.MessageID
	listeners := []*ttsListener{}
	for l := range ttsHub.clients {
		if l.channel == j.ChannelID {
			listeners = append(listeners, l)
		}
	}
	ttsHub.Unlock()
	eligible := []*ttsListener{}
	for _, l := range listeners {
		if _, err := ttsReadMessage(l.user, j.MessageID); err == nil {
			eligible = append(eligible, l)
		}
	}
	if len(eligible) == 0 {
		return false
	}
	if streamed {
		// Sent before the lane is released, so the next utterance cannot
		// supersede the announcement; sends never block.
		frame := ttsJSON(fiber.Map{"type": "start", "epoch": epoch, "utterance": j.ID, "messageId": j.MessageID, "media": media, "mode": "file", "startsAt": time.Now().UnixMilli()})
		ttsHub.Lock()
		if ttsHub.epochs[j.ChannelID] == epoch {
			for _, l := range eligible {
				ttsSend(l, frame)
			}
		}
		ttsHub.Unlock()
		return false
	}
	go func() {
		mode := "file"
		var snapshot service.TTSSnapshot
		if path != "" && media.Container == "wav" && media.Codec == "pcm_s16le" && json.Unmarshal([]byte(j.Snapshot), &snapshot) == nil && ttsprovider.SupportsLivePCM(snapshot.Provider.EffectiveProviderKind(), snapshot.Provider.Model) {
			mode = "pcm"
		}
		start := ttsJSON(fiber.Map{"type": "start", "epoch": epoch, "utterance": j.ID, "messageId": j.MessageID, "media": media, "mode": mode, "startsAt": time.Now().Add(300 * time.Millisecond).UnixMilli()})
		fileStart := ttsJSON(fiber.Map{"type": "start", "epoch": epoch, "utterance": j.ID, "messageId": j.MessageID, "media": media, "mode": "file", "startsAt": time.Now().Add(300 * time.Millisecond).UnixMilli()})
		ttsHub.Lock()
		if ttsHub.epochs[j.ChannelID] != epoch {
			ttsHub.Unlock()
			return
		}
		for _, l := range eligible {
			if l.fileOnly {
				ttsSend(l, fileStart)
			} else {
				ttsSend(l, start)
			}
		}
		ttsHub.Unlock()
		if mode == "file" {
			return
		}
		streaming := eligible[:0]
		for _, l := range eligible {
			if !l.fileOnly {
				streaming = append(streaming, l)
			}
		}
		eligible = streaming
		if len(eligible) == 0 {
			return
		}
		f, err := os.Open(path)
		if err != nil {
			return
		}
		defer f.Close()
		if _, err := f.Seek(int64(media.DataOffset), io.SeekStart); err != nil {
			return
		}
		ttsPumpLocalPCM(context.Background(), j, media, f, epoch, eligible, false)
	}()
	return true
}

// The writer consumes concurrently; a length check followed by receive can block.
func ttsDrain(l *ttsListener) {
	for {
		select {
		case <-l.send:
		default:
			return
		}
	}
}
func ttsBroadcastCancel(messageID string) {
	ttsHub.Lock()
	defer ttsHub.Unlock()
	for ch, msg := range ttsHub.messages {
		if msg == messageID {
			delete(ttsHub.localPCM, ch)
			// Preserve skip/stop across the short realtime-end -> ready gap.
			ttsHub.suppressed[ch] = messageID
			epoch := ttsHub.epochs[ch]
			ttsHub.epochs[ch]++
			for l := range ttsHub.clients {
				if l.channel == ch {
					ttsDrain(l)
					ttsSend(l, ttsJSON(fiber.Map{"type": "cancel", "epoch": epoch}))
				}
			}
		}
	}
}
func ttsChannelControl(c *fiber.Ctx) error {
	ch := c.Params("id")
	if !pm.CanWithChannelRole(getCurUser(c).ID, ch, pm.PermFuncChannelManageInfo) {
		return c.SendStatus(403)
	}
	var b struct {
		Action string `json:"action"`
	}
	if err := c.BodyParser(&b); err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	if b.Action != "skip" && b.Action != "stop" && b.Action != "clear" {
		return c.SendStatus(400)
	}
	ttsHub.Lock()
	msg := ttsHub.messages[ch]
	ttsHub.Unlock()
	service.TTSStopPlayback(msg)
	if b.Action == "clear" || b.Action == "stop" {
		if err := service.TTSClearChannelPending(ch); err != nil {
			return ttsError(c, err)
		}
		var jobs []model.TTSJob
		if err := model.GetDB().Where("channel_id = ? AND status = ?", ch, "queued").Find(&jobs).Error; err != nil {
			return ttsError(c, err)
		}
		for _, j := range jobs {
			service.TTSCancelMessage(j.MessageID)
		}
	}
	return c.JSON(fiber.Map{"ok": true})
}

func ttsChannelQueue(c *fiber.Ctx) error {
	userID, channelID := getCurUser(c).ID, c.Params("id")
	if !pm.CanWithChannelRole(userID, channelID, pm.PermFuncChannelRead, pm.PermFuncChannelReadAll) {
		return c.SendStatus(403)
	}
	var jobs []model.TTSJob
	if err := model.GetDB().Where("channel_id = ? AND status IN ? AND deleted_at IS NULL", channelID, []string{"queued", "running", "storage_pending", "archiving"}).Order("queue_order ASC, created_at ASC, id ASC").Limit(100).Find(&jobs).Error; err != nil {
		return ttsError(c, err)
	}
	items := []fiber.Map{}
	for _, job := range jobs {
		if _, err := ttsReadMessage(userID, job.MessageID); err == nil {
			items = append(items, fiber.Map{"id": job.ID, "messageId": job.MessageID, "status": job.Status})
		}
	}
	return c.JSON(fiber.Map{"items": items, "canControl": pm.CanWithChannelRole(userID, channelID, pm.PermFuncChannelManageInfo)})
}
