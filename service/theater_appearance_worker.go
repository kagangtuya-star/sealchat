package service

import (
	"context"
	"time"

	"sealchat/model"
)

func theaterAppearanceAssetWorker(ctx context.Context, service *theaterMediaService) {
	for {
		select {
		case <-ctx.Done():
			return
		case assetID := <-service.appearanceQueue:
			service.processAppearanceAsset(ctx, assetID)
		}
	}
}

func (service *theaterMediaService) processAppearanceAsset(ctx context.Context, assetID string) {
	claim := model.GetDB().Model(&model.TheaterAppearanceAssetModel{}).
		Where("id = ? AND status IN ? AND deleted_at IS NULL", assetID, []string{"pending", "processing"}).
		Updates(map[string]any{"status": "processing", "progress": 0.1, "failure_code": "", "failure_message": ""})
	if claim.Error != nil || claim.RowsAffected != 1 {
		return
	}
	var asset model.TheaterAppearanceAssetModel
	if err := model.GetDB().Where("id = ?", assetID).First(&asset).Error; err != nil {
		return
	}
	var source model.AttachmentModel
	if err := model.GetDB().Where("id = ?", asset.SourceAttachmentID).First(&source).Error; err != nil {
		theaterAppearanceFailure(asset.ID, TheaterMediaErrorProbeFailed, err)
		return
	}
	path, cleanup, err := resolveTheaterAttachmentPath(ctx, &source)
	if err != nil {
		theaterAppearanceFailure(asset.ID, TheaterMediaErrorProbeFailed, err)
		return
	}
	defer cleanup()
	processor := NewVisualMediaProcessor(service.config, service.toolchain, service.runner)
	processed, err := processor.ProcessAppearance(ctx, path, asset.Kind, asset.SourceMimeType)
	if err != nil {
		theaterAppearanceFailure(asset.ID, classifyTheaterAppearanceError(err), err)
		return
	}
	defer processed.Cleanup()
	_ = model.GetDB().Model(&model.TheaterAppearanceAssetModel{}).Where("id = ?", asset.ID).Update("progress", 0.6).Error
	displayAttachmentID := ""
	fallbackAttachmentID := ""
	displayMIME := ""
	displayWidth := 0
	displayHeight := 0
	displayDurationMS := int64(0)
	for _, output := range processed.Outputs {
		attachmentID, err := persistTheaterAppearanceOutput(&asset, output)
		if err != nil {
			theaterAppearanceFailure(asset.ID, TheaterMediaErrorTranscodeFailed, err)
			return
		}
		switch output.Name {
		case VisualMediaOutputDisplay:
			displayAttachmentID = attachmentID
			displayMIME = output.MimeType
			displayWidth = output.Width
			displayHeight = output.Height
			displayDurationMS = output.DurationMS
		case VisualMediaOutputFallback:
			fallbackAttachmentID = attachmentID
		}
	}
	if displayAttachmentID == "" {
		theaterAppearanceFailure(asset.ID, TheaterMediaErrorTranscodeFailed, nil)
		return
	}
	readyAt := time.Now()
	if err := model.GetDB().Model(&model.TheaterAppearanceAssetModel{}).Where("id = ? AND deleted_at IS NULL", asset.ID).Updates(map[string]any{
		"display_attachment_id": displayAttachmentID, "fallback_attachment_id": fallbackAttachmentID,
		"kind": processed.Metadata.Kind, "mime_type": displayMIME, "width": displayWidth, "height": displayHeight,
		"duration_ms": displayDurationMS, "status": "ready", "progress": 1, "failure_code": "", "failure_message": "", "ready_at": &readyAt,
	}).Error; err != nil {
		theaterAppearanceFailure(asset.ID, TheaterMediaErrorTranscodeFailed, err)
	}
}
