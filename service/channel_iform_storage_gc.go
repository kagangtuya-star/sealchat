package service

import (
	"context"
	"log"
	"sync"
	"time"

	"sealchat/model"
)

const (
	ChannelIFormStorageGCInterval  = 10 * time.Minute
	channelIFormStorageGCBatchSize = 256
)

var channelIFormStorageGCOnce sync.Once
var channelIFormStorageGCNotifier struct {
	sync.RWMutex
	fn func(channelID, formID string, mutation model.ChannelIFormStorageExpiredMutation)
}

func SetChannelIFormStorageGCNotifier(fn func(channelID, formID string, mutation model.ChannelIFormStorageExpiredMutation)) {
	channelIFormStorageGCNotifier.Lock()
	channelIFormStorageGCNotifier.fn = fn
	channelIFormStorageGCNotifier.Unlock()
}

func notifyChannelIFormStorageGC(channelID, formID string, mutation model.ChannelIFormStorageExpiredMutation) {
	channelIFormStorageGCNotifier.RLock()
	fn := channelIFormStorageGCNotifier.fn
	channelIFormStorageGCNotifier.RUnlock()
	if fn != nil {
		fn(channelID, formID, mutation)
	}
}

// RunChannelIFormStorageGC deletes expired documents in bounded batches.
func RunChannelIFormStorageGC(now time.Time) (int64, error) {
	var total int64
	for {
		mutations, err := model.ChannelIFormStorageDeleteExpired(now, channelIFormStorageGCBatchSize)
		if err != nil {
			return total, err
		}
		total += int64(len(mutations))
		for _, mutation := range mutations {
			notifyChannelIFormStorageGC(mutation.ChannelID, mutation.FormID, mutation)
		}
		if len(mutations) < channelIFormStorageGCBatchSize {
			return total, nil
		}
	}
}

// StartChannelIFormStorageGCWorker runs one startup sweep, then periodic sweeps.
func StartChannelIFormStorageGCWorker(ctx context.Context) {
	channelIFormStorageGCOnce.Do(func() {
		if ctx == nil {
			ctx = context.Background()
		}
		go func() {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if deleted, err := RunChannelIFormStorageGC(time.Now()); err != nil {
				log.Printf("iform-storage-gc: startup sweep failed: %v", err)
			} else if deleted > 0 {
				log.Printf("iform-storage-gc: startup deleted %d expired documents", deleted)
			}
			ticker := time.NewTicker(ChannelIFormStorageGCInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case now := <-ticker.C:
					deleted, err := RunChannelIFormStorageGC(now)
					if err != nil {
						log.Printf("iform-storage-gc: sweep failed: %v", err)
					} else if deleted > 0 {
						log.Printf("iform-storage-gc: deleted %d expired documents", deleted)
					}
				}
			}
		}()
	})
}
