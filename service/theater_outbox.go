package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"sealchat/model"
)

type TheaterEventPublisher interface {
	PublishTheaterMutation(context.Context, model.TheaterMutationModel) error
}

type TheaterResourceEventPublisher interface {
	PublishTheaterResource(context.Context, model.TheaterResourceModel) error
}

var theaterOutboxState = struct {
	sync.RWMutex
	publisher TheaterEventPublisher
	startOnce sync.Once
	queue     chan string
}{
	queue: make(chan string, 1024),
}

func SetTheaterEventPublisher(publisher TheaterEventPublisher) {
	theaterOutboxState.Lock()
	theaterOutboxState.publisher = publisher
	theaterOutboxState.Unlock()
}

func theaterEventPublisher() TheaterEventPublisher {
	theaterOutboxState.RLock()
	defer theaterOutboxState.RUnlock()
	return theaterOutboxState.publisher
}

func EnqueueTheaterMutation(mutationID string) {
	mutationID = strings.TrimSpace(mutationID)
	if mutationID == "" {
		return
	}
	select {
	case theaterOutboxState.queue <- mutationID:
	default:
	}
}

func PublishTheaterMutationNow(ctx context.Context, roomID, mutationID string) error {
	publisher := theaterEventPublisher()
	if publisher == nil {
		return nil
	}
	mutation, err := model.TheaterMutationFindByID(roomID, mutationID)
	if err != nil || mutation == nil {
		return err
	}
	return publisher.PublishTheaterMutation(ctx, *mutation)
}

func PublishTheaterResourceNow(ctx context.Context, resourceID string) error {
	publisher, ok := theaterEventPublisher().(TheaterResourceEventPublisher)
	if !ok || strings.TrimSpace(resourceID) == "" {
		return nil
	}
	var resource model.TheaterResourceModel
	err := model.GetDB().Where("id = ?", resourceID).First(&resource).Error
	if err != nil {
		return err
	}
	return publisher.PublishTheaterResource(ctx, resource)
}

func StartTheaterOutboxWorker(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	theaterOutboxState.startOnce.Do(func() {
		go runTheaterOutboxWorker(ctx)
	})
}

func runTheaterOutboxWorker(ctx context.Context) {
	_, _ = ProcessTheaterOutboxBatch(ctx, 200, true)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case mutationID := <-theaterOutboxState.queue:
			_ = processTheaterOutboxMutation(ctx, mutationID, false)
		case <-ticker.C:
			_, _ = ProcessTheaterOutboxBatch(ctx, 200, false)
		}
	}
}

func ProcessTheaterOutboxBatch(ctx context.Context, limit int, force bool) (int, error) {
	if theaterEventPublisher() == nil {
		return 0, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var rows []model.TheaterMutationModel
	if err := model.GetDB().Where("status = ? AND broadcasted_at IS NULL", "applied").Order("created_at ASC").Limit(limit).Find(&rows).Error; err != nil {
		return 0, err
	}
	if len(rows) > 0 {
		RecordTheaterMetric("theater_broadcast_pending_total", nil, float64(len(rows)))
		RecordTheaterMetric("theater_broadcast_delay_seconds", nil, time.Since(rows[0].CreatedAt).Seconds())
	}
	processed := 0
	for _, row := range rows {
		if !force && !theaterOutboxRetryDue(row, time.Now()) {
			continue
		}
		processed++
		if err := publishTheaterOutboxMutation(ctx, row); err != nil && !errors.Is(err, context.Canceled) {
			continue
		}
		if ctx != nil && ctx.Err() != nil {
			return processed, ctx.Err()
		}
	}
	return processed, nil
}

func processTheaterOutboxMutation(ctx context.Context, mutationID string, force bool) error {
	if theaterEventPublisher() == nil {
		return nil
	}
	var row model.TheaterMutationModel
	if err := model.GetDB().Where("mutation_id = ? AND status = ? AND broadcasted_at IS NULL", mutationID, "applied").Order("created_at DESC").First(&row).Error; err != nil {
		return err
	}
	if !force && !theaterOutboxRetryDue(row, time.Now()) {
		return nil
	}
	return publishTheaterOutboxMutation(ctx, row)
}

func theaterOutboxRetryDue(row model.TheaterMutationModel, now time.Time) bool {
	if row.BroadcastAttempts <= 0 {
		return true
	}
	delay := time.Second << min(row.BroadcastAttempts-1, 6)
	return !row.UpdatedAt.Add(delay).After(now)
}

func publishTheaterOutboxMutation(ctx context.Context, row model.TheaterMutationModel) error {
	publisher := theaterEventPublisher()
	if publisher == nil {
		return nil
	}
	err := publisher.PublishTheaterMutation(ctx, row)
	attempts := row.BroadcastAttempts + 1
	updates := map[string]any{"broadcast_attempts": attempts}
	if err != nil {
		updates["last_broadcast_error"] = truncateTheaterBroadcastError(err.Error())
	} else {
		now := time.Now()
		updates["broadcasted_at"] = &now
		updates["last_broadcast_error"] = ""
	}
	if updateErr := model.GetDB().Model(&model.TheaterMutationModel{}).Where("id = ? AND broadcasted_at IS NULL", row.ID).Updates(updates).Error; updateErr != nil {
		return updateErr
	}
	return err
}

func truncateTheaterBroadcastError(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 2048 {
		return value[:2048]
	}
	return value
}
