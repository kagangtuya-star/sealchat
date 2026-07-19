package service

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"sealchat/model"
	"sealchat/utils"
)

const (
	theaterPackageVersion          = 1
	theaterPackageMaxArchiveBytes  = int64(2 << 30)
	theaterPackageMaxExpandedBytes = int64(4 << 30)
	theaterPackageMaxFiles         = 10_000
	theaterPackageRetention        = 7 * 24 * time.Hour
)

func TheaterPackageRequestBodyLimit() int {
	return int(theaterPackageMaxArchiveBytes + 16<<20)
}

type TheaterPackageFile struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	MimeType string `json:"mimeType,omitempty"`
	Filename string `json:"filename,omitempty"`
}

type TheaterPackageResourceVariant struct {
	Name        string             `json:"name"`
	MimeType    string             `json:"mimeType"`
	SizeBytes   int64              `json:"sizeBytes"`
	Width       *int               `json:"width,omitempty"`
	Height      *int               `json:"height,omitempty"`
	DurationMS  *int64             `json:"durationMs,omitempty"`
	ContentHash string             `json:"contentHash,omitempty"`
	File        TheaterPackageFile `json:"file"`
}

type TheaterPackageResource struct {
	ID               string                          `json:"id"`
	ClientResourceID string                          `json:"clientResourceId,omitempty"`
	Kind             string                          `json:"kind"`
	ContentHash      string                          `json:"contentHash"`
	SizeBytes        int64                           `json:"sizeBytes"`
	MimeType         string                          `json:"mimeType"`
	OriginalFilename string                          `json:"originalFilename,omitempty"`
	Width            *int                            `json:"width,omitempty"`
	Height           *int                            `json:"height,omitempty"`
	DurationMS       *int64                          `json:"durationMs,omitempty"`
	FrameCount       *int                            `json:"frameCount,omitempty"`
	FrameRate        *float64                        `json:"frameRate,omitempty"`
	Container        string                          `json:"container,omitempty"`
	VideoCodec       string                          `json:"videoCodec,omitempty"`
	AudioCodec       string                          `json:"audioCodec,omitempty"`
	PosterResourceID string                          `json:"posterResourceId,omitempty"`
	VariantsJSON     string                          `json:"variantsJson,omitempty"`
	Original         TheaterPackageFile              `json:"original"`
	Variants         []TheaterPackageResourceVariant `json:"variants"`
}

type TheaterPackageAudio struct {
	ID              string                     `json:"id"`
	Name            string                     `json:"name"`
	Description     string                     `json:"description,omitempty"`
	Tags            []string                   `json:"tags,omitempty"`
	Visibility      model.AudioAssetVisibility `json:"visibility"`
	DurationSeconds float64                    `json:"duration"`
	BitrateKbps     int                        `json:"bitrate"`
	File            TheaterPackageFile         `json:"file"`
}

type TheaterPackageAppearanceAsset struct {
	ID                   string              `json:"id"`
	Purpose              string              `json:"purpose"`
	Kind                 string              `json:"kind"`
	MimeType             string              `json:"mimeType"`
	SourceMimeType       string              `json:"sourceMimeType"`
	OriginalFilename     string              `json:"originalFilename,omitempty"`
	SizeBytes            int64               `json:"sizeBytes"`
	ContentHash          string              `json:"contentHash"`
	Width                int                 `json:"width"`
	Height               int                 `json:"height"`
	DurationMS           int64               `json:"durationMs,omitempty"`
	SourceAttachmentID   string              `json:"sourceAttachmentId"`
	DisplayAttachmentID  string              `json:"displayAttachmentId,omitempty"`
	FallbackAttachmentID string              `json:"fallbackAttachmentId,omitempty"`
	Source               TheaterPackageFile  `json:"source"`
	Display              *TheaterPackageFile `json:"display,omitempty"`
	Fallback             *TheaterPackageFile `json:"fallback,omitempty"`
}

type TheaterPackageManifest struct {
	PackageVersion       int                             `json:"packageVersion"`
	SchemaVersion        int                             `json:"schemaVersion"`
	PackageID            string                          `json:"packageId"`
	CreatedAt            time.Time                       `json:"createdAt"`
	SourceWorldID        string                          `json:"sourceWorldId"`
	SourceWorldName      string                          `json:"sourceWorldName"`
	SourceRevision       int64                           `json:"sourceRevision"`
	SourceInputChannelID string                          `json:"sourceInputChannelId,omitempty"`
	Document             TheaterPackageFile              `json:"document"`
	WorldPresentation    *TheaterPackageFile             `json:"worldPresentation,omitempty"`
	Resources            []TheaterPackageResource        `json:"resources"`
	Audio                []TheaterPackageAudio           `json:"audio"`
	AppearanceAssets     []TheaterPackageAppearanceAsset `json:"appearanceAssets,omitempty"`
}

type TheaterPackageSummary struct {
	Scenes                    int      `json:"scenes"`
	Objects                   int      `json:"objects"`
	PersistentObjects         int      `json:"persistentObjects"`
	Resources                 int      `json:"resources"`
	AudioAssets               int      `json:"audioAssets"`
	AppearanceAssets          int      `json:"appearanceAssets"`
	WorldPresentationImported bool     `json:"worldPresentationImported"`
	Warnings                  []string `json:"warnings,omitempty"`
	ImportedSceneIDs          []string `json:"importedSceneIds,omitempty"`
	SourceFormat              string   `json:"sourceFormat,omitempty"`
	SourceVersion             string   `json:"sourceVersion,omitempty"`
	CurrentRoomObjects        int      `json:"currentRoomObjects,omitempty"`
	AnimatedResources         int      `json:"animatedResources,omitempty"`
	UnsupportedEntities       []string `json:"unsupportedEntities,omitempty"`
}

type theaterPackageWorkerConfig struct {
	StorageDir string
}

var theaterPackageWorkerState = struct {
	sync.RWMutex
	startOnce sync.Once
	config    theaterPackageWorkerConfig
}{config: theaterPackageWorkerConfig{StorageDir: "./data/exports/theater-packages"}}

func StartTheaterPackageWorker(ctx context.Context, storageDir string) {
	if ctx == nil {
		ctx = context.Background()
	}
	storageDir = strings.TrimSpace(storageDir)
	if storageDir == "" {
		storageDir = "./data/exports"
	}
	storageDir = filepath.Join(storageDir, "theater-packages")
	theaterPackageWorkerState.Lock()
	theaterPackageWorkerState.config.StorageDir = storageDir
	theaterPackageWorkerState.Unlock()
	theaterPackageWorkerState.startOnce.Do(func() {
		if err := os.MkdirAll(storageDir, 0o755); err != nil {
			log.Printf("theater package: 创建任务目录失败: %v", err)
		}
		_ = model.GetDB().Model(&model.TheaterPackageJobModel{}).
			Where("status = ?", model.TheaterPackageJobStatusRunning).
			Updates(map[string]any{"status": model.TheaterPackageJobStatusPending, "started_at": nil}).Error
		go runTheaterPackageWorker(ctx)
	})
}

func theaterPackageStorageDir() string {
	theaterPackageWorkerState.RLock()
	defer theaterPackageWorkerState.RUnlock()
	return theaterPackageWorkerState.config.StorageDir
}

func runTheaterPackageWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	cleanupTicker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	defer cleanupTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-cleanupTicker.C:
			_ = cleanupExpiredTheaterPackageJobs()
		case <-ticker.C:
			job, err := acquireNextTheaterPackageJob()
			if err != nil {
				log.Printf("theater package: 获取任务失败: %v", err)
				continue
			}
			if job == nil {
				continue
			}
			if err := processTheaterPackageJob(ctx, job); err != nil {
				log.Printf("theater package: 任务 %s 失败: %v", job.ID, err)
			}
		}
	}
}

func acquireNextTheaterPackageJob() (*model.TheaterPackageJobModel, error) {
	var job model.TheaterPackageJobModel
	if err := model.GetDB().Where("status = ?", model.TheaterPackageJobStatusPending).
		Order("created_at ASC").Limit(1).Find(&job).Error; err != nil {
		return nil, err
	}
	if job.ID == "" {
		return nil, nil
	}
	now := time.Now()
	result := model.GetDB().Model(&model.TheaterPackageJobModel{}).
		Where("id = ? AND status = ?", job.ID, model.TheaterPackageJobStatusPending).
		Updates(map[string]any{"status": model.TheaterPackageJobStatusRunning, "started_at": &now, "progress": 0.01, "error_code": "", "error_message": ""})
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	job.Status = model.TheaterPackageJobStatusRunning
	job.StartedAt = &now
	return &job, nil
}

func processTheaterPackageJob(ctx context.Context, job *model.TheaterPackageJobModel) error {
	var summary TheaterPackageSummary
	var err error
	switch job.Type {
	case model.TheaterPackageJobTypeExport:
		summary, err = exportTheaterPackage(ctx, job)
	case model.TheaterPackageJobTypeImport:
		summary, err = importTheaterPackage(ctx, job)
	case model.TheaterPackageJobTypeImportCCFOLIA:
		summary, err = importCCFOLIATheaterPackage(ctx, job)
	default:
		err = fmt.Errorf("未知舞台包任务类型: %s", job.Type)
	}
	if err != nil {
		code := "PACKAGE_PROCESS_FAILED"
		if job.Type == model.TheaterPackageJobTypeImportCCFOLIA {
			code = "CCFOLIA_IMPORT_FAILED"
		}
		_ = failTheaterPackageJob(job.ID, code, err)
		return err
	}
	raw, _ := json.Marshal(summary)
	now := time.Now()
	expiresAt := now.Add(theaterPackageRetention)
	return model.GetDB().Model(&model.TheaterPackageJobModel{}).Where("id = ?", job.ID).Updates(map[string]any{
		"status": model.TheaterPackageJobStatusDone, "progress": 1, "summary_json": string(raw),
		"finished_at": &now, "expires_at": &expiresAt, "error_code": "", "error_message": "",
	}).Error
}

func updateTheaterPackageProgress(jobID string, progress float64) {
	if progress < 0 {
		progress = 0
	}
	if progress > 0.99 {
		progress = 0.99
	}
	_ = model.GetDB().Model(&model.TheaterPackageJobModel{}).Where("id = ? AND status = ?", jobID, model.TheaterPackageJobStatusRunning).Update("progress", progress).Error
}

func failTheaterPackageJob(jobID, code string, cause error) error {
	now := time.Now()
	expiresAt := now.Add(theaterPackageRetention)
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	return model.GetDB().Model(&model.TheaterPackageJobModel{}).Where("id = ?", jobID).Updates(map[string]any{
		"status": model.TheaterPackageJobStatusFailed, "error_code": code, "error_message": message,
		"finished_at": &now, "expires_at": &expiresAt,
	}).Error
}

func CreateTheaterPackageExportJob(actorID, worldID, inputChannelID string) (*model.TheaterPackageJobModel, error) {
	if _, _, err := requireTheaterPermission(actorID, worldID, "", TheaterPermissionAdminRestore); err != nil {
		return nil, err
	}
	if strings.TrimSpace(inputChannelID) != "" {
		if _, _, err := resolveTheaterScope(worldID, inputChannelID); err != nil {
			return nil, err
		}
	}
	job := &model.TheaterPackageJobModel{
		Type: model.TheaterPackageJobTypeExport, Status: model.TheaterPackageJobStatusPending,
		ActorUserID: actorID, SourceWorldID: strings.TrimSpace(worldID), InputChannelID: strings.TrimSpace(inputChannelID),
	}
	return job, model.GetDB().Create(job).Error
}

func CreateTheaterPackageImportJob(actorID, targetWorldID, inputChannelID, filename string, reader io.Reader, size int64) (*model.TheaterPackageJobModel, error) {
	return createTheaterPackageImportJob(model.TheaterPackageJobTypeImport, actorID, targetWorldID, inputChannelID, filename, reader, size)
}

func CreateTheaterCCFOLIAImportJob(actorID, targetWorldID, inputChannelID, filename string, reader io.Reader, size int64) (*model.TheaterPackageJobModel, error) {
	return createTheaterPackageImportJob(model.TheaterPackageJobTypeImportCCFOLIA, actorID, targetWorldID, inputChannelID, filename, reader, size)
}

func createTheaterPackageImportJob(jobType, actorID, targetWorldID, inputChannelID, filename string, reader io.Reader, size int64) (*model.TheaterPackageJobModel, error) {
	if _, _, err := requireTheaterPermission(actorID, targetWorldID, "", TheaterPermissionAdminRestore); err != nil {
		return nil, err
	}
	if strings.TrimSpace(inputChannelID) != "" {
		if _, _, err := resolveTheaterScope(targetWorldID, inputChannelID); err != nil {
			return nil, err
		}
	}
	if reader == nil || size <= 0 || size > theaterPackageMaxArchiveBytes {
		return nil, newTheaterError(TheaterErrorResourceLimitExceeded, "舞台包大小无效", 413, map[string]any{"limitBytes": theaterPackageMaxArchiveBytes})
	}
	job := &model.TheaterPackageJobModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
		Type:              jobType, Status: model.TheaterPackageJobStatusPending,
		ActorUserID: actorID, TargetWorldID: strings.TrimSpace(targetWorldID), InputChannelID: strings.TrimSpace(inputChannelID),
		OriginalName: sanitizeTheaterPackageFilename(filename),
	}
	incomingDir := filepath.Join(theaterPackageStorageDir(), "incoming")
	if err := os.MkdirAll(incomingDir, 0o755); err != nil {
		return nil, err
	}
	inputPath := filepath.Join(incomingDir, job.ID+".zip")
	output, err := os.OpenFile(inputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	written, copyErr := io.Copy(output, io.LimitReader(reader, theaterPackageMaxArchiveBytes+1))
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil || written != size || written > theaterPackageMaxArchiveBytes {
		_ = os.Remove(inputPath)
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		return nil, errors.New("舞台包上传不完整")
	}
	job.InputFilePath = inputPath
	if err := model.GetDB().Create(job).Error; err != nil {
		_ = os.Remove(inputPath)
		return nil, err
	}
	return job, nil
}

func GetTheaterPackageJob(actorID, jobID string) (*model.TheaterPackageJobModel, error) {
	var job model.TheaterPackageJobModel
	if err := model.GetDB().Where("id = ?", strings.TrimSpace(jobID)).Limit(1).Find(&job).Error; err != nil {
		return nil, err
	}
	if job.ID == "" {
		return nil, newTheaterError(TheaterErrorNotFound, "舞台包任务不存在", 404, nil)
	}
	worldID := job.SourceWorldID
	if isTheaterPackageImportJob(job.Type) {
		worldID = job.TargetWorldID
	}
	if job.ActorUserID != actorID {
		if _, _, err := requireTheaterPermission(actorID, worldID, "", TheaterPermissionAdminRestore); err != nil {
			return nil, err
		}
	}
	return &job, nil
}

func isTheaterPackageImportJob(jobType string) bool {
	return jobType == model.TheaterPackageJobTypeImport || jobType == model.TheaterPackageJobTypeImportCCFOLIA
}

func DeleteTheaterPackageJob(actorID, jobID string) error {
	job, err := GetTheaterPackageJob(actorID, jobID)
	if err != nil {
		return err
	}
	if job.Status == model.TheaterPackageJobStatusPending || job.Status == model.TheaterPackageJobStatusRunning {
		return errors.New("任务进行中，无法删除")
	}
	for _, path := range []string{job.InputFilePath, job.OutputFilePath} {
		if strings.TrimSpace(path) != "" {
			_ = os.Remove(path)
		}
	}
	return model.GetDB().Delete(&model.TheaterPackageJobModel{}, "id = ?", job.ID).Error
}

func cleanupExpiredTheaterPackageJobs() error {
	now := time.Now()
	var jobs []model.TheaterPackageJobModel
	if err := model.GetDB().Where("expires_at IS NOT NULL AND expires_at < ?", now).Limit(200).Find(&jobs).Error; err != nil {
		return err
	}
	for _, job := range jobs {
		for _, path := range []string{job.InputFilePath, job.OutputFilePath} {
			if strings.TrimSpace(path) != "" {
				_ = os.Remove(path)
			}
		}
		_ = model.GetDB().Delete(&model.TheaterPackageJobModel{}, "id = ?", job.ID).Error
	}
	return nil
}

func sanitizeTheaterPackageFilename(value string) string {
	name := filepath.Base(strings.TrimSpace(value))
	name = strings.ReplaceAll(strings.ReplaceAll(name, "\r", ""), "\n", "")
	if name == "." || name == "" {
		return "theater-package.zip"
	}
	if len(name) > 255 {
		name = name[:255]
	}
	return name
}

func theaterPackageFile(path, mimeType, filename string) (TheaterPackageFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return TheaterPackageFile{}, err
	}
	defer file.Close()
	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return TheaterPackageFile{}, err
	}
	return TheaterPackageFile{SHA256: hex.EncodeToString(hasher.Sum(nil)), Size: size, MimeType: mimeType, Filename: filename}, nil
}

func writeJSONFile(path string, value any) (TheaterPackageFile, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return TheaterPackageFile{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return TheaterPackageFile{}, err
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return TheaterPackageFile{}, err
	}
	return theaterPackageFile(path, "application/json", filepath.Base(path))
}

func zipDirectory(sourceDir, targetPath string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	zipWriter := zip.NewWriter(output)
	walkErr := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(relative)
		header.Method = zip.Deflate
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, input)
		closeErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	closeZipErr := zipWriter.Close()
	closeOutputErr := output.Close()
	if walkErr != nil {
		return walkErr
	}
	if closeZipErr != nil {
		return closeZipErr
	}
	return closeOutputErr
}
