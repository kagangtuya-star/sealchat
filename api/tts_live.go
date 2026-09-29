package api

import (
	"context"
	"encoding/binary"
	"os"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"sealchat/model"
	"sealchat/pkg/ttsprovider"
	"sealchat/service"
)

// One reader tails the one archival spool. The supplier's bursts do not enter
// connection queues: at most 100 ms PCM is sent per 100 ms of playback time.
// The returned finish reports whether a PCM start frame was dispatched, i.e.
// whether this stream already paced the channel lane. Delivery to a connection
// queue never proves playback; the ready announcement lets browsers decide.
func ttsBroadcastLive(ctx context.Context, job model.TTSJob, path string) func(bool) bool {
	complete := make(chan bool, 1)
	done := make(chan struct{})
	started := false
	ttsHub.Lock()
	ttsHub.epochs[job.ChannelID]++
	epoch := ttsHub.epochs[job.ChannelID]
	ttsHub.messages[job.ChannelID] = job.MessageID
	listeners := []*ttsListener{}
	for l := range ttsHub.clients {
		if l.channel == job.ChannelID && !l.fileOnly {
			listeners = append(listeners, l)
		}
	}
	ttsHub.Unlock()
	go func() {
		defer close(done)
		control := func(kind string) ttsFrame { return ttsJSON(fiber.Map{"type": kind, "epoch": epoch}) }
		cancel := func() {
			for _, l := range listeners {
				ttsDrain(l)
				ttsSend(l, control("cancel"))
			}
		}
		f, err := os.Open(path)
		if err != nil {
			return
		}
		defer f.Close()
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		var media ttsprovider.Media
		finished, success := false, false
		var sent, seq int
		var next time.Time
		deadline := time.Now().Add(5 * time.Minute)
		for {
			select {
			case <-ctx.Done():
				cancel()
				return
			case success = <-complete:
				finished = true
			case <-tick.C:
			}
			ttsHub.Lock()
			current := ttsHub.epochs[job.ChannelID] == epoch
			ttsHub.Unlock()
			if !current || time.Now().After(deadline) || !service.TTSJobMessageCurrent(&job) || (finished && !success) {
				cancel()
				return
			}
			if !started {
				header := make([]byte, 64<<10)
				n, _ := f.ReadAt(header, 0)
				var ready bool
				media, ready, err = ttsprovider.InspectWAVStreamHeader(header[:n])
				if err != nil || (!ready && finished) {
					cancel()
					return
				}
				if !ready {
					continue
				}
				eligible := listeners[:0]
				frame := ttsJSON(fiber.Map{"type": "start", "epoch": epoch, "utterance": job.ID, "messageId": job.MessageID, "media": media, "mode": "pcm", "startsAt": time.Now().Add(300 * time.Millisecond).UnixMilli()})
				frame.messageID = job.MessageID
				for _, l := range listeners {
					m, e := ttsReadMessage(l.user, job.MessageID)
					if e == nil && m.ChannelID == l.channel && ttsSend(l, frame) {
						eligible = append(eligible, l)
					}
				}
				listeners = eligible
				// Only a delivered start frame paced the lane; otherwise the
				// ready path streams the archived file on its own timeline.
				if len(listeners) == 0 {
					return
				}
				started = true
				next = time.Now()
			}
			if time.Now().Before(next) {
				continue
			}
			// Streaming headers may carry estimated lengths; only the bytes
			// actually spooled bound the stream. The header gives layout only.
			info, err := f.Stat()
			if err != nil {
				cancel()
				return
			}
			frameSize := media.ChannelCount * 2
			available := int(info.Size()) - media.DataOffset - sent
			available -= available % frameSize
			size := media.SampleRate * frameSize / 10
			if available < size {
				if !finished {
					continue
				}
				size = available
			}
			if size <= 0 {
				for _, l := range listeners {
					ttsSend(l, control("end"))
				}
				// Input end is not speaker end: retain the lane's prebuffer tail.
				time.Sleep(300 * time.Millisecond)
				return
			}
			packet := make([]byte, 8+size)
			n, _ := f.ReadAt(packet[8:], int64(media.DataOffset+sent))
			if n < size {
				if finished {
					cancel()
					return
				}
				continue
			}
			binary.BigEndian.PutUint32(packet, epoch)
			binary.BigEndian.PutUint32(packet[4:], uint32(seq))
			seq++
			sent += size
			next = time.Now().Add(time.Duration(size) * time.Second / time.Duration(media.SampleRate*media.ChannelCount*2))
			eligible := listeners[:0]
			for _, l := range listeners {
				m, e := ttsReadMessage(l.user, job.MessageID)
				if e != nil || m.ChannelID != l.channel {
					ttsDrain(l)
					ttsSend(l, control("cancel"))
					continue
				}
				if ttsSend(l, ttsFrame{kind: websocket.BinaryMessage, data: packet, messageID: job.MessageID}) {
					eligible = append(eligible, l)
				} else {
					ttsDrain(l)
					ttsSend(l, control("desynced"))
				}
			}
			listeners = eligible
		}
	}()
	return func(success bool) bool { complete <- success; <-done; return started }
}
