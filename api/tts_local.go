package api

import (
	"context"
	"encoding/binary"
	"io"
	"os"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"sealchat/model"
	"sealchat/pkg/ttsprovider"
	"sealchat/service"
)

// Ephemeral playback never issues a ticket or claims a durable resource exists.
// Its epoch remains active while ready announces the archive with a newer epoch.
func ttsBroadcastLocalPlayback(ctx context.Context, j model.TTSJob, media ttsprovider.Media, path string) (bool, <-chan struct{}) {
	if ctx.Err() != nil || !service.TTSJobMessageCurrent(&j) {
		return false, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return false, nil
	}
	if media.Container != "wav" || media.Codec != "pcm_s16le" || media.SampleRate <= 0 || media.ChannelCount <= 0 || media.DataSize <= 0 {
		f.Close()
		return false, nil
	}
	if _, err = f.Seek(int64(media.DataOffset), io.SeekStart); err != nil {
		f.Close()
		return false, nil
	}
	ttsHub.Lock()
	if ttsHub.suppressed[j.ChannelID] == j.MessageID {
		ttsHub.Unlock()
		f.Close()
		return false, nil
	}
	listeners := []*ttsListener{}
	for l := range ttsHub.clients {
		if l.channel == j.ChannelID && !l.fileOnly {
			listeners = append(listeners, l)
		}
	}
	ttsHub.Unlock()
	eligible := []*ttsListener{}
	for _, l := range listeners {
		if m, err := ttsReadMessage(l.user, j.MessageID); err == nil && m.ChannelID == j.ChannelID {
			eligible = append(eligible, l)
		}
	}
	ttsHub.Lock()
	if ctx.Err() != nil || ttsHub.suppressed[j.ChannelID] == j.MessageID || len(eligible) == 0 {
		ttsHub.Unlock()
		f.Close()
		return false, nil
	}
	ttsHub.epochs[j.ChannelID]++
	epoch := ttsHub.epochs[j.ChannelID]
	ttsHub.messages[j.ChannelID] = j.MessageID
	start := ttsJSON(fiber.Map{"type": "start", "epoch": epoch, "utterance": j.ID, "messageId": j.MessageID, "media": media, "mode": "pcm", "startsAt": time.Now().Add(300 * time.Millisecond).UnixMilli()})
	start.messageID = j.MessageID
	next := eligible[:0]
	for _, l := range eligible {
		if ttsSend(l, start) {
			next = append(next, l)
		}
	}
	eligible = next
	if len(eligible) == 0 {
		ttsHub.Unlock()
		f.Close()
		return false, nil
	}
	ttsHub.localPCM[j.ChannelID] = epoch
	ttsHub.Unlock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer f.Close()
		defer func() {
			ttsHub.Lock()
			if ttsHub.localPCM[j.ChannelID] == epoch {
				delete(ttsHub.localPCM, j.ChannelID)
			}
			ttsHub.Unlock()
		}()
		ttsPumpLocalPCM(ctx, j, media, f, epoch, eligible, true)
	}()
	return true, done
}

// Both durable local WAV playback and ephemeral completed-spool playback use
// the existing PCM packets, pacing, authorization and desync behavior.
func ttsPumpLocalPCM(ctx context.Context, j model.TTSJob, media ttsprovider.Media, f io.Reader, epoch uint32, eligible []*ttsListener, ephemeral bool) {
	reader := io.LimitReader(f, int64(media.DataSize))
	buf := make([]byte, media.SampleRate*media.ChannelCount*2/10)
	seq := uint32(0)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		ttsHub.Lock()
		current := ttsHub.epochs[j.ChannelID] == epoch
		if ephemeral {
			current = ttsHub.localPCM[j.ChannelID] == epoch
		}
		ttsHub.Unlock()
		if !current || ctx.Err() != nil || (ephemeral && !service.TTSJobMessageCurrent(&j)) {
			if ephemeral {
				for _, l := range eligible {
					ttsSend(l, ttsJSON(fiber.Map{"type": "cancel", "epoch": epoch}))
				}
			}
			return
		}
		n, err := reader.Read(buf)
		if n > 0 {
			packet := make([]byte, 8+n)
			binary.BigEndian.PutUint32(packet, epoch)
			binary.BigEndian.PutUint32(packet[4:], seq)
			copy(packet[8:], buf[:n])
			seq++
			next := eligible[:0]
			for _, l := range eligible {
				if _, e := ttsReadMessage(l.user, j.MessageID); e != nil {
					ttsSend(l, ttsJSON(fiber.Map{"type": "cancel", "epoch": epoch}))
					continue
				}
				if ttsSend(l, ttsFrame{kind: websocket.BinaryMessage, data: packet, messageID: j.MessageID}) {
					next = append(next, l)
				} else {
					ttsDrain(l)
					ttsSend(l, ttsJSON(fiber.Map{"type": "desynced", "epoch": epoch}))
				}
			}
			eligible = next
		}
		if err != nil {
			break
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			// Send the existing cancel frame on the next iteration.
		}
	}
	for _, l := range eligible {
		ttsSend(l, ttsJSON(fiber.Map{"type": "end", "epoch": epoch}))
	}
	if ephemeral {
		// Sending end flushes browser input; startsAt's lead still belongs to
		// this lane even though all PCM has now been dispatched.
		timer := time.NewTimer(300 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
		}
	}
}
