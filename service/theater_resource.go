package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/utils"
)

const (
	TheaterMediaErrorUnsupported          = "UNSUPPORTED_MEDIA_TYPE"
	TheaterMediaErrorProcessorUnavailable = "MEDIA_PROCESSOR_UNAVAILABLE"
	TheaterMediaErrorProbeFailed          = "MEDIA_PROBE_FAILED"
	TheaterMediaErrorTranscodeFailed      = "MEDIA_TRANSCODE_FAILED"
	TheaterMediaErrorLimitExceeded        = "MEDIA_LIMIT_EXCEEDED"
)

type TheaterResourceUploadInput struct {
	Reader            io.Reader
	Size              int64
	Filename          string
	ClientResourceID  string
	MediaKind         string
	ProcessingProfile string
}

type TheaterResourceUploadResult struct {
	Resource     TheaterResourcePublic
	Deduplicated bool
}

type TheaterResourceContent struct {
	Resource   *model.TheaterResourceModel
	Variant    *model.TheaterResourceVariantModel
	Attachment *model.AttachmentModel
}

type theaterMediaService struct {
	config          utils.TheaterMediaConfig
	toolchain       MediaToolchain
	runner          MediaCommandRunner
	queue           chan string
	appearanceQueue chan string
	ctx             context.Context
	cancel          context.CancelFunc
	once            sync.Once
}

var theaterMedia = &theaterMediaService{}

func InitTheaterMediaService(config utils.TheaterMediaConfig, toolchain MediaToolchain) {
	theaterMedia.once.Do(func() {
		theaterMedia.config = normalizeTheaterMediaConfig(config)
		theaterMedia.toolchain = toolchain
		theaterMedia.runner = execMediaCommandRunner{}
		theaterMedia.ctx, theaterMedia.cancel = context.WithCancel(context.Background())
		theaterMedia.queue = make(chan string, 256)
		theaterMedia.appearanceQueue = make(chan string, 256)
		for index := 0; index < theaterMedia.config.WorkerConcurrency; index++ {
			go theaterMediaWorker(theaterMedia.ctx, theaterMedia)
			go theaterAppearanceAssetWorker(theaterMedia.ctx, theaterMedia)
		}
		go theaterMedia.scanPendingJobs(theaterMedia.ctx)
		go theaterMedia.scanPendingAppearanceAssets(theaterMedia.ctx)
		go runTheaterAppearanceOrphanScanner(theaterMedia.ctx)
	})
}

func normalizeTheaterMediaConfig(config utils.TheaterMediaConfig) utils.TheaterMediaConfig {
	if config.WorkerConcurrency <= 0 {
		config.WorkerConcurrency = 2
	}
	if config.ImageMaxSizeMB <= 0 {
		config.ImageMaxSizeMB = 20
	}
	if config.VideoMaxSizeMB <= 0 {
		config.VideoMaxSizeMB = 200
	}
	if config.RoomQuotaMB <= 0 {
		config.RoomQuotaMB = 2048
	}
	if config.MaxDimension <= 0 {
		config.MaxDimension = 16384
	}
	if config.MaxAnimatedFrames <= 0 {
		config.MaxAnimatedFrames = 500
	}
	if config.MaxAnimatedDurationMS <= 0 {
		config.MaxAnimatedDurationMS = 300000
	}
	if config.MaxAnimatedPixelFrames <= 0 {
		config.MaxAnimatedPixelFrames = 512000000
	}
	if config.VideoMaxDurationMS <= 0 {
		config.VideoMaxDurationMS = 900000
	}
	if config.VideoMaxWidth <= 0 {
		config.VideoMaxWidth = 3840
	}
	if config.VideoMaxHeight <= 0 {
		config.VideoMaxHeight = 2160
	}
	if config.VideoMaxFrameRate <= 0 {
		config.VideoMaxFrameRate = 60
	}
	if config.ProbeTimeoutSeconds <= 0 {
		config.ProbeTimeoutSeconds = 30
	}
	if config.TranscodeTimeoutSeconds <= 0 {
		config.TranscodeTimeoutSeconds = 900
	}
	return config
}

func CreateTheaterResourceUpload(ctx context.Context, actorID, worldID, channelID string, input TheaterResourceUploadInput) (*TheaterResourceUploadResult, error) {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionResourceUpload); err != nil {
		return nil, err
	}
	if input.Reader == nil {
		return nil, theaterPayloadError("file 必填")
	}
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return nil, err
	}
	config := normalizeTheaterMediaConfig(theaterMedia.config)
	var used int64
	if err := model.GetDB().Model(&model.TheaterResourceModel{}).Where("room_id = ? AND status NOT IN ?", room.ID, []string{"failed", "deleting"}).Select("COALESCE(SUM(size_bytes), 0)").Scan(&used).Error; err != nil {
		return nil, err
	}
	if used >= config.RoomQuotaMB<<20 {
		return nil, newTheaterError(TheaterErrorResourceLimitExceeded, "房间资源配额已满", 413, map[string]any{"limitBytes": config.RoomQuotaMB << 20})
	}
	temp, err := os.CreateTemp("", "sealchat-theater-upload-*")
	if err != nil {
		return nil, err
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()
	hasher := sha256.New()
	maxBytes := config.VideoMaxSizeMB << 20
	if maxBytes < config.ImageMaxSizeMB<<20 {
		maxBytes = config.ImageMaxSizeMB << 20
	}
	written, err := io.Copy(io.MultiWriter(temp, hasher), io.LimitReader(input.Reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if written == 0 || written > maxBytes {
		return nil, newTheaterError(TheaterErrorResourceLimitExceeded, "资源文件大小超限", 413, map[string]any{"limitBytes": maxBytes})
	}
	if err := temp.Close(); err != nil {
		return nil, err
	}
	head, err := readFilePrefix(tempPath, 4096)
	if err != nil {
		return nil, err
	}
	mimeType, kind := detectTheaterMediaType(head)
	if mimeType == "" {
		return nil, newTheaterError(TheaterMediaErrorUnsupported, "不支持媒体格式", 415, nil)
	}
	requestedKind := strings.ToLower(strings.TrimSpace(input.MediaKind))
	if mimeType == "video/webm" && (requestedKind == "image" || requestedKind == "animated_image") {
		kind = "animated_image"
	}
	limit := config.ImageMaxSizeMB << 20
	if kind == "video" {
		limit = config.VideoMaxSizeMB << 20
	}
	if written > limit {
		return nil, newTheaterError(TheaterErrorResourceLimitExceeded, "资源文件大小超限", 413, map[string]any{"limitBytes": limit})
	}
	hashBytes := hasher.Sum(nil)
	hashHex := hex.EncodeToString(hashBytes)
	clientID := strings.TrimSpace(input.ClientResourceID)
	if clientID != "" {
		var existing model.TheaterResourceModel
		if err := model.GetDB().Where("room_id = ? AND client_resource_id = ?", room.ID, clientID).Limit(1).Find(&existing).Error; err != nil {
			return nil, err
		}
		if existing.ID != "" {
			if existing.ContentHash != hashHex || existing.SizeBytes != written {
				return nil, newTheaterError("RESOURCE_ID_REUSED", "clientResourceId 已用于不同内容", 409, nil)
			}
			public, err := theaterResourcePublicFromModel(model.GetDB(), existing)
			return &TheaterResourceUploadResult{Resource: public, Deduplicated: true}, err
		}
	}
	var duplicate model.TheaterResourceModel
	if err := model.GetDB().Where("room_id = ? AND content_hash = ? AND size_bytes = ? AND status <> ?", room.ID, hashHex, written, "deleting").Order("created_at ASC").Limit(1).Find(&duplicate).Error; err != nil {
		return nil, err
	}
	if duplicate.ID != "" {
		public, err := theaterResourcePublicFromModel(model.GetDB(), duplicate)
		return &TheaterResourceUploadResult{Resource: public, Deduplicated: true}, err
	}
	location, err := PersistAttachmentFile(hashBytes, written, tempPath, mimeType)
	if err != nil {
		return nil, err
	}
	attachment := &model.AttachmentModel{Hash: hashBytes, Filename: sanitizeTheaterFilename(input.Filename), Size: written, MimeType: mimeType, IsAnimated: kind == "animated_image", UserID: actorID, ChannelID: channelID, StorageType: location.StorageType, ObjectKey: location.ObjectKey, ExternalURL: location.ExternalURL, RootID: room.ID, RootIDType: "theater_resource", IsTemp: false}
	if tx, _ := model.AttachmentCreate(attachment); tx.Error != nil {
		return nil, tx.Error
	}
	resource := model.TheaterResourceModel{
		RoomID: room.ID, ClientResourceID: clientID, AttachmentID: attachment.ID, Kind: kind, ContentHash: hashHex, SizeBytes: written, MimeType: mimeType,
		OriginalFilename: attachment.Filename, Status: "pending", VariantsJSON: "[]", CreatedBy: actorID,
	}
	job := model.TheaterResourceJobModel{RequestID: "initial", Type: "process", Status: "pending"}
	if err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&resource).Error; err != nil {
			return err
		}
		job.ResourceID = resource.ID
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		return tx.Model(&resource).Update("processing_job_id", job.ID).Error
	}); err != nil {
		return nil, err
	}
	theaterMedia.enqueue(resource.ID)
	auditTheaterResourceState(resource.ID, "pending", "")
	public, err := theaterResourcePublicFromModel(model.GetDB(), resource)
	return &TheaterResourceUploadResult{Resource: public}, err
}

func GetTheaterResource(ctx context.Context, actorID, worldID, channelID, resourceID string) (*TheaterResourcePublic, error) {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionView); err != nil {
		return nil, err
	}
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return nil, err
	}
	resource, err := model.TheaterResourceGet(room.ID, resourceID)
	if err != nil {
		return nil, err
	}
	if resource == nil || resource.Status == "deleting" {
		return nil, newTheaterError(TheaterErrorResourceNotFound, "资源不存在", 404, nil)
	}
	public, err := theaterResourcePublicFromModel(model.GetDB(), *resource)
	return &public, err
}

func GetTheaterResourceForObserver(ctx context.Context, observerWorldID, channelID, resourceID string) (*TheaterResourcePublic, error) {
	if _, err := CanObserverAccessChannel(channelID, observerWorldID); err != nil {
		return nil, newTheaterError(TheaterErrorPermissionDenied, "没有 Theater 旁观权限", 403, nil)
	}
	room, err := model.TheaterRoomCreateIfMissing(observerWorldID, channelID, "observer")
	if err != nil {
		return nil, err
	}
	resource, err := model.TheaterResourceGet(room.ID, resourceID)
	if err != nil {
		return nil, err
	}
	if resource == nil || resource.Status == "deleting" {
		return nil, newTheaterError(TheaterErrorResourceNotFound, "资源不存在", 404, nil)
	}
	public, err := theaterResourcePublicFromModel(model.GetDB(), *resource)
	return &public, err
}

func RetryTheaterResource(ctx context.Context, actorID, worldID, channelID, resourceID, requestID string) (*TheaterResourcePublic, error) {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionResourceUpload); err != nil {
		return nil, err
	}
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return nil, err
	}
	resource, err := model.TheaterResourceGet(room.ID, resourceID)
	if err != nil || resource == nil {
		return nil, newTheaterError(TheaterErrorResourceNotFound, "资源不存在", 404, nil)
	}
	if resource.Status != "failed" || !resource.Retryable {
		return nil, newTheaterError("RESOURCE_PROCESSING_NOT_RETRYABLE", "资源不可重试", 409, nil)
	}
	job := model.TheaterResourceJobModel{ResourceID: resource.ID, RequestID: strings.TrimSpace(requestID), Type: "retry", Status: "pending"}
	if job.RequestID == "" {
		return nil, theaterPayloadError("processingRequestId 必填")
	}
	if err := model.GetDB().Create(&job).Error; err != nil {
		var existing model.TheaterResourceJobModel
		if findErr := model.GetDB().Where("resource_id = ? AND request_id = ?", resource.ID, job.RequestID).First(&existing).Error; findErr != nil {
			return nil, err
		}
	}
	if err := model.GetDB().Model(resource).Updates(map[string]any{"status": "pending", "processing_progress": 0, "failure_code": "", "failure_message": "", "processing_job_id": job.ID}).Error; err != nil {
		return nil, err
	}
	theaterMedia.enqueue(resource.ID)
	auditTheaterResourceState(resource.ID, "pending", "")
	public, err := theaterResourcePublicFromModel(model.GetDB(), *resource)
	return &public, err
}

func DeleteTheaterResource(ctx context.Context, actorID, worldID, channelID, resourceID string) error {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionResourceDelete); err != nil {
		return err
	}
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return err
	}
	resource, err := model.TheaterResourceGet(room.ID, resourceID)
	if err != nil {
		return err
	}
	if resource == nil || resource.Status == "deleting" {
		return nil
	}
	if resource.ReferenceCount > 0 {
		return newTheaterError(TheaterErrorResourceInUse, "资源仍被共享状态引用", 409, map[string]any{"referenceCount": resource.ReferenceCount})
	}
	now := time.Now()
	if err := model.GetDB().Model(resource).Updates(map[string]any{"status": "deleting", "deleted_at": &now}).Error; err != nil {
		return err
	}
	auditTheaterResourceState(resource.ID, "deleting", "")
	return nil
}

func ResolveTheaterResourceContent(ctx context.Context, actorID, worldID, channelID, resourceID, variantName string) (*TheaterResourceContent, error) {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionView); err != nil {
		return nil, err
	}
	return resolveTheaterResourceContent(ctx, actorID, worldID, channelID, resourceID, variantName)
}

func ResolveTheaterResourceContentForObserver(ctx context.Context, observerWorldID, channelID, resourceID, variantName string) (*TheaterResourceContent, error) {
	if _, err := CanObserverAccessChannel(channelID, observerWorldID); err != nil {
		return nil, newTheaterError(TheaterErrorPermissionDenied, "没有 Theater 旁观权限", 403, nil)
	}
	return resolveTheaterResourceContent(ctx, "observer", observerWorldID, channelID, resourceID, variantName)
}

func resolveTheaterResourceContent(ctx context.Context, actorID, worldID, channelID, resourceID, variantName string) (*TheaterResourceContent, error) {
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return nil, err
	}
	resource, err := model.TheaterResourceGet(room.ID, resourceID)
	if err != nil || resource == nil {
		return nil, newTheaterError(TheaterErrorResourceNotFound, "资源不存在", 404, nil)
	}
	if resource.Status != "ready" {
		return nil, newTheaterError(TheaterErrorResourceNotReady, "资源尚未 ready", 409, nil)
	}
	attachmentID := resource.AttachmentID
	var variant *model.TheaterResourceVariantModel
	if strings.TrimSpace(variantName) != "" {
		var row model.TheaterResourceVariantModel
		if err := model.GetDB().Where("resource_id = ? AND name = ? AND status = ?", resource.ID, variantName, "ready").Limit(1).Find(&row).Error; err != nil {
			return nil, err
		}
		if row.ID == "" {
			return nil, newTheaterError("RESOURCE_VARIANT_NOT_READY", "资源派生版本尚未 ready", 409, nil)
		}
		variant = &row
		attachmentID = row.AttachmentID
	}
	var attachment model.AttachmentModel
	if err := model.GetDB().Where("id = ?", attachmentID).First(&attachment).Error; err != nil {
		return nil, err
	}
	return &TheaterResourceContent{Resource: resource, Variant: variant, Attachment: &attachment}, nil
}

func (service *theaterMediaService) enqueue(resourceID string) {
	if service == nil || service.queue == nil {
		return
	}
	select {
	case service.queue <- resourceID:
	default:
	}
}

func (service *theaterMediaService) scanPendingJobs(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var ids []string
			_ = model.GetDB().Model(&model.TheaterResourceModel{}).Where("status IN ?", []string{"pending", "probing", "transcoding"}).Limit(100).Pluck("id", &ids).Error
			for _, id := range ids {
				service.enqueue(id)
			}
		}
	}
}

func readFilePrefix(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, limit))
}

func sanitizeTheaterFilename(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	value = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, value)
	if value == "" {
		value = "resource"
	}
	runes := []rune(value)
	if len(runes) > 255 {
		value = string(runes[:255])
	}
	return value
}

func resolveTheaterAttachmentPath(ctx context.Context, attachment *model.AttachmentModel) (string, func(), error) {
	if attachment == nil {
		return "", func() {}, errors.New("attachment missing")
	}
	if attachment.StorageType == model.StorageLocal {
		path, err := ResolveLocalAttachmentPath(attachment.ObjectKey)
		return path, func() {}, err
	}
	manager := GetStorageManager()
	if manager == nil {
		return "", func() {}, errors.New("storage unavailable")
	}
	temp, err := os.CreateTemp("", "sealchat-theater-source-*")
	if err != nil {
		return "", func() {}, err
	}
	path := temp.Name()
	_ = temp.Close()
	cleanup := func() { _ = os.Remove(path) }
	if err := manager.DownloadToPath(ctx, convertModelToBackend(attachment.StorageType), attachment.ObjectKey, path); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return path, cleanup, nil
}

func theaterResourceAttachment(resource *model.TheaterResourceModel) (*model.AttachmentModel, error) {
	if resource == nil {
		return nil, errors.New("resource missing")
	}
	var attachment model.AttachmentModel
	if err := model.GetDB().Where("id = ?", resource.AttachmentID).First(&attachment).Error; err != nil {
		return nil, err
	}
	return &attachment, nil
}

func theaterMediaFailure(resourceID, code string, err error, retryable bool) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	_ = model.GetDB().Model(&model.TheaterResourceModel{}).Where("id = ?", resourceID).Updates(map[string]any{"status": "failed", "failure_code": code, "failure_message": message, "retryable": retryable, "processing_progress": 0}).Error
	_ = model.GetDB().Model(&model.TheaterResourceJobModel{}).Where("resource_id = ? AND status IN ?", resourceID, []string{"pending", "probing", "transcoding"}).Updates(map[string]any{"status": "failed", "error_code": code, "error": message, "finished_at": time.Now()}).Error
	_ = PublishTheaterResourceNow(context.Background(), resourceID)
	auditTheaterResourceState(resourceID, "failed", code)
	var resource model.TheaterResourceModel
	if findErr := model.GetDB().Where("id = ?", resourceID).First(&resource).Error; findErr == nil {
		RecordTheaterMetric("theater_resource_upload_total", map[string]string{"status": "failed", "mime": resource.MimeType}, 1)
	}
}

func waitForTheaterResourceStatus(ctx context.Context, resourceID string) (*model.TheaterResourceModel, error) {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var resource model.TheaterResourceModel
		if err := model.GetDB().Where("id = ?", resourceID).First(&resource).Error; err != nil {
			return nil, err
		}
		if resource.Status == "ready" || resource.Status == "failed" {
			return &resource, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
