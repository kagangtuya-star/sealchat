package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
)

func theaterMediaWorker(ctx context.Context, service *theaterMediaService) {
	for {
		select {
		case <-ctx.Done():
			return
		case resourceID := <-service.queue:
			service.processResource(ctx, resourceID)
		}
	}
}

func (service *theaterMediaService) processResource(ctx context.Context, resourceID string) {
	var resource model.TheaterResourceModel
	claim := model.GetDB().Model(&model.TheaterResourceModel{}).
		Where("id = ? AND status IN ?", resourceID, []string{"pending", "probing", "transcoding"}).
		Updates(map[string]any{"status": "probing", "processing_progress": 0.1, "failure_code": "", "failure_message": ""})
	if claim.Error != nil || claim.RowsAffected != 1 {
		return
	}
	if err := model.GetDB().Where("id = ?", resourceID).First(&resource).Error; err != nil {
		return
	}
	_ = PublishTheaterResourceNow(ctx, resource.ID)
	auditTheaterResourceState(resource.ID, "processing", "")
	attachment, err := theaterResourceAttachment(&resource)
	if err != nil {
		theaterMediaFailure(resource.ID, TheaterMediaErrorProbeFailed, err, true)
		return
	}
	path, cleanup, err := resolveTheaterAttachmentPath(ctx, attachment)
	if err != nil {
		theaterMediaFailure(resource.ID, TheaterMediaErrorProbeFailed, err, true)
		return
	}
	defer cleanup()
	processor := NewVisualMediaProcessor(service.config, service.toolchain, service.runner)
	processed, err := processor.ProcessTheaterResource(ctx, path, resource.Kind, resource.MimeType)
	if err != nil {
		code := TheaterMediaErrorProbeFailed
		retryable := true
		if strings.Contains(err.Error(), TheaterMediaErrorProcessorUnavailable) {
			code = TheaterMediaErrorProcessorUnavailable
		} else if strings.Contains(err.Error(), TheaterMediaErrorLimitExceeded) {
			code = TheaterMediaErrorLimitExceeded
			retryable = false
		} else if strings.Contains(err.Error(), "IMAGE_DECODE_FAILED") {
			code = "IMAGE_DECODE_FAILED"
			retryable = false
		}
		theaterMediaFailure(resource.ID, code, err, retryable)
		return
	}
	defer processed.Cleanup()
	metadata := processed.Metadata
	if metadata.Kind == "video" || metadata.Kind == "animated_image" {
		_ = model.GetDB().Model(&resource).Updates(map[string]any{"status": "transcoding", "processing_progress": 0.5}).Error
		_ = PublishTheaterResourceNow(ctx, resource.ID)
	}
	variant := model.TheaterResourceVariantModel{ResourceID: resource.ID, Name: "original", AttachmentID: resource.AttachmentID, MimeType: resource.MimeType, SizeBytes: resource.SizeBytes, Width: intPtr(metadata.Width), Height: intPtr(metadata.Height), DurationMS: optionalInt64(metadata.DurationMS), Status: "ready", ContentHash: resource.ContentHash}
	derived := make([]model.TheaterResourceVariantModel, 0, len(processed.Outputs))
	for _, output := range processed.Outputs {
		if output.IsSource {
			derived = append(derived, model.TheaterResourceVariantModel{
				ResourceID: resource.ID, Name: output.Name, AttachmentID: resource.AttachmentID, MimeType: output.MimeType,
				SizeBytes: resource.SizeBytes, Width: intPtr(output.Width), Height: intPtr(output.Height), DurationMS: optionalInt64(output.DurationMS),
				Status: "ready", ContentHash: resource.ContentHash,
			})
			continue
		}
		item, persistErr := persistTheaterDerivedVariant(&resource, output.Name, output.Path, output.MimeType, output.Width, output.Height, output.DurationMS)
		if persistErr != nil {
			theaterMediaFailure(resource.ID, TheaterMediaErrorTranscodeFailed, persistErr, true)
			return
		}
		derived = append(derived, item)
	}
	readyAt := time.Now()
	if err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		for _, item := range append([]model.TheaterResourceVariantModel{variant}, derived...) {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&item).Error; err != nil {
				return err
			}
		}
		var variantRows []model.TheaterResourceVariantModel
		if err := tx.Where("resource_id = ? AND status = ?", resource.ID, "ready").Order("name ASC").Find(&variantRows).Error; err != nil {
			return err
		}
		variantPublic := make([]map[string]any, 0, len(variantRows))
		for _, item := range variantRows {
			variantPublic = append(variantPublic, map[string]any{"name": item.Name, "mimeType": item.MimeType, "sizeBytes": item.SizeBytes, "width": item.Width, "height": item.Height})
		}
		variantsJSON, _ := json.Marshal(variantPublic)
		updates := map[string]any{
			"status": "ready", "processing_progress": 1, "kind": metadata.Kind, "width": metadata.Width, "height": metadata.Height,
			"duration_ms": nullableMediaInt64(metadata.DurationMS), "frame_count": nullableMediaInt(metadata.FrameCount), "frame_rate": nullableMediaFloat(metadata.FrameRate),
			"container": metadata.Container, "video_codec": metadata.VideoCodec, "audio_codec": metadata.AudioCodec,
			"variants_json": string(variantsJSON), "failure_code": "", "failure_message": "", "retryable": false, "ready_at": &readyAt,
		}
		if err := tx.Model(&model.TheaterResourceModel{}).Where("id = ?", resource.ID).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Model(&model.TheaterResourceJobModel{}).Where("resource_id = ? AND status IN ?", resource.ID, []string{"pending", "probing", "transcoding"}).Updates(map[string]any{"status": "ready", "progress": 1, "finished_at": readyAt}).Error
	}); err != nil {
		theaterMediaFailure(resource.ID, TheaterMediaErrorTranscodeFailed, err, true)
		return
	}
	_ = PublishTheaterResourceNow(ctx, resource.ID)
	auditTheaterResourceState(resource.ID, "ready", "")
	RecordTheaterMetric("theater_resource_upload_total", map[string]string{"status": "ready", "mime": resource.MimeType}, 1)
	RecordTheaterMetric("theater_resource_bytes_total", map[string]string{"mime": resource.MimeType}, float64(resource.SizeBytes))
}

func transcodeTheaterDisplay(ctx context.Context, sourcePath, tempDir string, toolchain MediaToolchain, runner MediaCommandRunner) (string, string, error) {
	mp4Path := filepath.Join(tempDir, "display.mp4")
	output, err := runner.Run(ctx, toolchain.FFmpegPath, "-y", "-i", sourcePath, "-map", "0:v:0", "-map", "0:a?", "-vf", "scale=min(1920\\,iw):-2", "-c:v", "libx264", "-preset", "medium", "-crf", "23", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart", mp4Path)
	if err == nil {
		return mp4Path, "video/mp4", nil
	}
	webmPath := filepath.Join(tempDir, "display.webm")
	output, fallbackErr := runner.Run(ctx, toolchain.FFmpegPath, "-y", "-i", sourcePath, "-map", "0:v:0", "-map", "0:a?", "-vf", "scale=min(1920\\,iw):-2", "-c:v", "libvpx-vp9", "-crf", "32", "-b:v", "0", "-c:a", "libopus", "-b:a", "96k", webmPath)
	if fallbackErr != nil {
		return "", "", fmt.Errorf("display: %s: %w", truncateTheaterBroadcastError(string(output)), fallbackErr)
	}
	return webmPath, "video/webm", nil
}

func persistTheaterDerivedVariant(resource *model.TheaterResourceModel, name, path, mimeType string, width, height int, duration int64) (model.TheaterResourceVariantModel, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return model.TheaterResourceVariantModel{}, err
	}
	hash := sha256.Sum256(data)
	location, err := PersistAttachmentFile(hash[:], int64(len(data)), path, mimeType)
	if err != nil {
		return model.TheaterResourceVariantModel{}, err
	}
	attachment := &model.AttachmentModel{
		Hash: hash[:], Filename: name + filepath.Ext(path), Size: int64(len(data)), MimeType: mimeType,
		IsAnimated: strings.HasPrefix(mimeType, "video/"),
		UserID:     resource.CreatedBy, StorageType: location.StorageType, ObjectKey: location.ObjectKey, ExternalURL: location.ExternalURL,
		RootID: resource.ID, RootIDType: "theater_resource_variant", IsTemp: false,
	}
	if tx, _ := model.AttachmentCreate(attachment); tx.Error != nil {
		return model.TheaterResourceVariantModel{}, tx.Error
	}
	return model.TheaterResourceVariantModel{
		ResourceID: resource.ID, Name: name, AttachmentID: attachment.ID, MimeType: mimeType, SizeBytes: int64(len(data)),
		Width: intPtr(width), Height: intPtr(height), DurationMS: optionalInt64(duration), Status: "ready", ContentHash: fmt.Sprintf("%x", hash[:]),
	}, nil
}

func scaledTheaterDimensions(width, height, maxWidth int) (int, int) {
	if width <= 0 || height <= 0 || maxWidth <= 0 || width <= maxWidth {
		return width, height
	}
	return maxWidth, height * maxWidth / width
}

func intPtr(value int) *int {
	if value <= 0 {
		return nil
	}
	copy := value
	return &copy
}

func optionalInt64(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	copy := value
	return &copy
}

func nullableMediaInt(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nullableMediaInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nullableMediaFloat(value float64) any {
	if value <= 0 {
		return nil
	}
	return value
}
