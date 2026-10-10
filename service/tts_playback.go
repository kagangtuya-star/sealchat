package service

import (
	"context"
	"time"

	"sealchat/model"
	"sealchat/pkg/ttsprovider"
	"sealchat/service/storage"
	"sealchat/utils"
)

func ttsCanStreamCompletedLocalWAV(media ttsprovider.Media) bool {
	return media.Container == "wav" && media.Codec == "pcm_s16le" &&
		media.SampleRate > 0 && media.ChannelCount > 0 && media.DataSize > 0
}

func ttsLocalFirstEligible(cfg *utils.SpeechConfig, media ttsprovider.Media) bool {
	m := GetStorageManager()
	return cfg != nil && cfg.LocalFirstPlayback && ttsCanStreamCompletedLocalWAV(media) &&
		m != nil && m.ActiveBackendForTTS() == storage.BackendS3
}

// The archive owns the spool and the lane. Playback only borrows a read-only
// path; its done notification includes the server's complete PCM timeline.
type ttsCompletedPlayback struct {
	ctx                context.Context
	cancel             context.CancelFunc
	job                model.TTSJob
	media              ttsprovider.Media
	stop               chan struct{}
	watchDone          chan struct{}
	started, attempted bool
	done               <-chan struct{}
}

func ttsNewCompletedPlayback(ctx context.Context, j model.TTSJob, s TTSSnapshot, media ttsprovider.Media) *ttsCompletedPlayback {
	ctx, cancel := context.WithCancel(ctx)
	p := &ttsCompletedPlayback{ctx: ctx, cancel: cancel, job: j, media: media, stop: make(chan struct{}), watchDone: make(chan struct{})}
	ttsTimelines.Lock()
	ttsTimelines.items[j.MessageID] = p.stop
	ttsTimelines.Unlock()
	go func() {
		defer close(p.watchDone)
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-p.stop:
				cancel()
				return
			case <-ticker.C:
				if !ttsMessageCurrent(&j, s) {
					cancel()
					return
				}
			}
		}
	}()
	return p
}

func (p *ttsCompletedPlayback) start() {
	if p.attempted {
		return
	}
	p.attempted = true
	select {
	case <-p.stop:
		p.cancel()
		return
	default:
	}
	if p.ctx.Err() != nil {
		return
	}
	ttsCallbacks.RLock()
	fn := ttsCallbacks.localPlayback
	ttsCallbacks.RUnlock()
	if fn != nil {
		p.started, p.done = fn(p.ctx, p.job, p.media, p.job.SpoolPath)
	}
}

func (p *ttsCompletedPlayback) wait() {
	if p.started {
		select {
		case <-p.done:
			return
		case <-p.ctx.Done():
		case <-p.stop:
			p.cancel()
		}
		// Cancellation stops pacing. Join the reader before its owner removes
		// the spool, including on platforms that cannot unlink an open file.
		<-p.done
	}
}

func (p *ttsCompletedPlayback) close() {
	p.wait()
	p.cancel()
	<-p.watchDone
	ttsTimelines.Lock()
	if ttsTimelines.items[p.job.MessageID] == p.stop {
		delete(ttsTimelines.items, p.job.MessageID)
	}
	ttsTimelines.Unlock()
}
