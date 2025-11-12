package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gabriel-vasile/mimetype"

	"sealchat/model"
	"sealchat/utils"
)

type audioService struct {
	cfg          utils.AudioConfig
	storage      *localAudioStorage
	allowedMimes map[string]struct{}
	ffmpegPath   string
	ffprobePath  string
}

var (
	audioSvc     *audioService
	audioSvcOnce sync.Once
	audioSvcErr  error
)

var (
	ErrAudioTooLarge        = errors.New("音频文件超过允许大小")
	ErrAudioUnsupportedMime = errors.New("不支持的音频格式")
)

type localAudioStorage struct {
	rootDir string
}

type AudioUploadOptions struct {
	Name        string
	FolderID    *string
	Tags        []string
	Description string
	Visibility  model.AudioAssetVisibility
	CreatedBy   string
}

func InitAudioService(cfg utils.AudioConfig) error {
	audioSvcOnce.Do(func() {
		if strings.TrimSpace(cfg.StorageDir) == "" {
			cfg.StorageDir = "./static/audio"
		}
		if strings.TrimSpace(cfg.TempDir) == "" {
			cfg.TempDir = "./data/audio-temp"
		}
		if cfg.MaxUploadSizeMB <= 0 {
			cfg.MaxUploadSizeMB = 80
		}

		storage := &localAudioStorage{rootDir: cfg.StorageDir}
		audioSvc = &audioService{
			cfg:          cfg,
			storage:      storage,
			allowedMimes: buildMimeMap(cfg.AllowedMimeTypes),
		}

		if err := os.MkdirAll(cfg.StorageDir, 0755); err != nil {
			audioSvcErr = fmt.Errorf("failed to create audio storage dir: %w", err)
			return
		}
		if err := os.MkdirAll(cfg.TempDir, 0755); err != nil {
			audioSvcErr = fmt.Errorf("failed to create audio temp dir: %w", err)
			return
		}
		if err := os.MkdirAll(storage.trashDir(), 0755); err != nil {
			audioSvcErr = fmt.Errorf("failed to create audio trash dir: %w", err)
			return
		}

		audioSvc.ffmpegPath = detectExecutable([]string{cfg.FFmpegPath, filepath.Join(filepath.Dir(os.Args[0]), "ffmpeg"), "ffmpeg"})
		audioSvc.ffprobePath = detectExecutable([]string{cfg.FFmpegPath, filepath.Join(filepath.Dir(os.Args[0]), "ffprobe"), "ffprobe"})
	})
	return audioSvcErr
}

func buildMimeMap(list []string) map[string]struct{} {
	result := map[string]struct{}{}
	defaults := []string{"audio/mpeg", "audio/ogg", "audio/wav", "audio/x-wav", "audio/webm", "audio/aac", "audio/flac", "audio/mp4"}
	if len(list) == 0 {
		list = defaults
	}
	for _, item := range list {
		trimmed := strings.TrimSpace(strings.ToLower(item))
		if trimmed == "" {
			continue
		}
		result[trimmed] = struct{}{}
	}
	return result
}

func detectExecutable(candidates []string) string {
	for _, candidate := range candidates {
		path := strings.TrimSpace(candidate)
		if path == "" {
			continue
		}
		if filepath.Base(path) == path {
			if resolved, err := exec.LookPath(path); err == nil {
				if fileExists(resolved) {
					return resolved
				}
			}
			continue
		}
		if fileExists(path) {
			return path
		}
	}
	return ""
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return true
	}
	return false
}

func (s *localAudioStorage) fullPath(objectKey string) (string, error) {
	clean := filepath.Clean(objectKey)
	if strings.HasPrefix(clean, "..") {
		return "", errors.New("invalid objectKey")
	}
	root := filepath.Clean(s.rootDir)
	full := filepath.Join(root, clean)
	return full, nil
}

func (s *localAudioStorage) ensureParent(objectKey string) error {
	full, err := s.fullPath(objectKey)
	if err != nil {
		return err
	}
	return os.MkdirAll(filepath.Dir(full), 0755)
}

func (s *localAudioStorage) moveFromTemp(tempPath, objectKey string) (int64, error) {
	if err := s.ensureParent(objectKey); err != nil {
		return 0, err
	}
	full, err := s.fullPath(objectKey)
	if err != nil {
		return 0, err
	}
	if err := os.Rename(tempPath, full); err != nil {
		return 0, err
	}
	info, err := os.Stat(full)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (s *localAudioStorage) open(objectKey string) (*os.File, os.FileInfo, error) {
	full, err := s.fullPath(objectKey)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return f, info, nil
}

func (s *localAudioStorage) trashDir() string {
	return filepath.Join(s.rootDir, "trash")
}

func (svc *audioService) maxUploadBytes() int64 {
	return svc.cfg.MaxUploadSizeMB * 1024 * 1024
}

func (svc *audioService) validateMime(file multipart.File) (string, error) {
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	mt := mimetype.Detect(buffer[:n])
	mimeType := strings.ToLower(mt.String())
	if len(svc.allowedMimes) > 0 {
		if _, ok := svc.allowedMimes[mimeType]; !ok {
			return "", fmt.Errorf("%w: %s", ErrAudioUnsupportedMime, mimeType)
		}
	}
	return mimeType, nil
}

func (svc *audioService) Upload(fileHeader *multipart.FileHeader, opts AudioUploadOptions) (*model.AudioAsset, error) {
	if audioSvc == nil {
		return nil, errors.New("audio service not initialized")
	}
	if fileHeader == nil {
		return nil, errors.New("未选择上传文件")
	}
	if fileHeader.Size > svc.maxUploadBytes() {
		return nil, fmt.Errorf("%w (最大 %d MB)", ErrAudioTooLarge, svc.cfg.MaxUploadSizeMB)
	}
	src, err := fileHeader.Open()
	if err != nil {
		return nil, err
	}
	defer src.Close()
	mimeType, err := svc.validateMime(src)
	if err != nil {
		return nil, err
	}
	tempName := fmt.Sprintf("upload-%d", time.Now().UnixNano())
	tempPath := filepath.Join(svc.cfg.TempDir, tempName)
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(tempFile, src); err != nil {
		_ = tempFile.Close()
		return nil, err
	}
	_ = tempFile.Close()

	defer os.Remove(tempPath)
	asset, err := svc.persistTempFile(tempPath, fileHeader.Filename, mimeType, opts)
	if err != nil {
		return nil, err
	}
	return asset, nil
}

func (svc *audioService) persistTempFile(tempPath, originalName, mimeType string, opts AudioUploadOptions) (*model.AudioAsset, error) {
	asset := &model.AudioAsset{}
	asset.StringPKBaseModel.Init()
	asset.Name = chooseName(opts.Name, originalName)
	asset.Description = strings.TrimSpace(opts.Description)
	asset.Visibility = opts.Visibility
	if asset.Visibility == "" {
		asset.Visibility = model.AudioVisibilityPublic
	}
	asset.CreatedBy = opts.CreatedBy
	asset.UpdatedBy = opts.CreatedBy
	asset.Tags = model.JSONList[string](normalizeTags(opts.Tags))
	asset.FolderID = cloneStringPtr(opts.FolderID)
	asset.StorageType = model.AudioStorageLocal

	result, err := svc.generateVariants(tempPath, asset.ID, mimeType)
	if err != nil {
		return nil, err
	}
	asset.ObjectKey = result.Primary.ObjectKey
	asset.BitrateKbps = result.Primary.BitrateKbps
	asset.DurationSeconds = result.Primary.Duration
	asset.Size = result.Primary.Size
	asset.Variants = model.JSONList[model.AudioAssetVariant](result.Extras)
	asset.TranscodeStatus = result.TranscodeStatus

	return asset, nil
}

type variantResult struct {
	Primary         model.AudioAssetVariant
	Extras          []model.AudioAssetVariant
	TranscodeStatus model.AudioTranscodeStatus
}

func (svc *audioService) generateVariants(tempPath, assetID, mimeType string) (*variantResult, error) {
	primary := model.AudioAssetVariant{
		Label:       "default",
		BitrateKbps: svc.cfg.DefaultBitrateKbps,
		StorageType: model.AudioStorageLocal,
	}
	result := &variantResult{TranscodeStatus: model.AudioTranscodeReady}
	transcoded := false

	if svc.cfg.EnableTranscode && svc.ffmpegPath != "" {
		profiles := []int{}
		if svc.cfg.DefaultBitrateKbps > 0 {
			profiles = append(profiles, svc.cfg.DefaultBitrateKbps)
		}
		profiles = append(profiles, svc.cfg.AlternateBitrates...)
		profiles = loUniqInt(profiles)
		sort.Ints(profiles)
		if len(profiles) > 0 {
			var extras []model.AudioAssetVariant
			primarySet := false
			for _, bitrate := range profiles {
				label := fmt.Sprintf("%dk", bitrate)
				objectKey := filepath.ToSlash(filepath.Join("opus", fmt.Sprintf("%s_%s.ogg", assetID, label)))
				variantPath := filepath.Join(svc.cfg.TempDir, fmt.Sprintf("%s-%s.ogg", assetID, label))
				if err := svc.runFFmpeg(tempPath, variantPath, bitrate); err != nil {
					return nil, err
				}
				size, err := svc.storage.moveFromTemp(variantPath, objectKey)
				if err != nil {
					return nil, err
				}
				variant := model.AudioAssetVariant{
					Label:       label,
					BitrateKbps: bitrate,
					ObjectKey:   objectKey,
					Size:        size,
					StorageType: model.AudioStorageLocal,
				}
				duration, err := svc.probeDuration(objectKey)
				if err == nil {
					variant.Duration = duration
				}
				if !primarySet {
					primary = variant
					primarySet = true
				} else {
					extras = append(extras, variant)
				}
			}
			result.Primary = primary
			result.Extras = extras
			transcoded = true
		}
	}

	if transcoded {
		return result, nil
	}

	// fallback: store original file
	objectKey := filepath.ToSlash(filepath.Join("original", fmt.Sprintf("%s%s", assetID, pickExtension(mimeType, tempPath))))
	size, err := svc.storage.moveFromTemp(tempPath, objectKey)
	if err != nil {
		return nil, err
	}
	primary.ObjectKey = objectKey
	primary.Size = size
	primary.BitrateKbps = svc.cfg.DefaultBitrateKbps
	primary.Label = "source"
	primary.Duration, _ = svc.probeDuration(objectKey)
	result.Primary = primary
	if svc.cfg.EnableTranscode && svc.ffmpegPath == "" {
		result.TranscodeStatus = model.AudioTranscodeFailed
	}
	return result, nil
}

func (svc *audioService) runFFmpeg(srcPath, dstPath string, bitrate int) error {
	args := []string{"-y", "-i", srcPath, "-vn", "-c:a", "libopus"}
	if bitrate > 0 {
		args = append(args, "-b:a", fmt.Sprintf("%dk", bitrate))
	}
	args = append(args, dstPath)
	cmd := exec.CommandContext(context.Background(), svc.ffmpegPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (svc *audioService) probeDuration(objectKey string) (float64, error) {
	if svc.ffprobePath == "" {
		return 0, errors.New("ffprobe not available")
	}
	full, err := svc.storage.fullPath(objectKey)
	if err != nil {
		return 0, err
	}
	args := []string{"-v", "error", "-show_entries", "format=duration", "-of", "default=nokey=1:noprint_wrappers=1", full}
	cmd := exec.CommandContext(context.Background(), svc.ffprobePath, args...)
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	value := strings.TrimSpace(string(output))
	if value == "" {
		return 0, errors.New("empty duration")
	}
	return strconv.ParseFloat(value, 64)
}

func chooseName(name, fallback string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed != "" {
		return trimmed
	}
	base := strings.TrimSpace(fallback)
	if base == "" {
		return "新音频"
	}
	return base
}

func pickExtension(mimeType, tempPath string) string {
	switch mimeType {
	case "audio/ogg", "audio/opus":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	case "audio/webm":
		return ".webm"
	case "audio/aac":
		return ".aac"
	case "audio/wav", "audio/x-wav":
		return ".wav"
	case "audio/flac":
		return ".flac"
	default:
		return filepath.Ext(tempPath)
	}
}

func normalizeTags(tags []string) []string {
	var result []string
	seen := map[string]struct{}{}
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func loUniqInt(values []int) []int {
	seen := map[int]struct{}{}
	var result []int
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		result = append(result, v)
	}
	return result
}

func GetAudioService() *audioService {
	return audioSvc
}

func (svc *audioService) DebugSummary() map[string]interface{} {
	return map[string]interface{}{
		"storageDir": svc.cfg.StorageDir,
		"tempDir":    svc.cfg.TempDir,
		"ffmpeg":     svc.ffmpegPath,
		"ffprobe":    svc.ffprobePath,
		"enableX":    svc.cfg.EnableTranscode,
	}
}

func AudioProcessUpload(fileHeader *multipart.FileHeader, opts AudioUploadOptions) (*model.AudioAsset, error) {
	svc := GetAudioService()
	if svc == nil {
		return nil, errors.New("音频服务未初始化")
	}
	return svc.Upload(fileHeader, opts)
}

func (svc *audioService) ResolveLocalFile(asset *model.AudioAsset, variantLabel string) (*os.File, os.FileInfo, model.AudioAssetVariant, error) {
	variant := selectVariant(asset, variantLabel)
	if variant.StorageType != model.AudioStorageLocal {
		return nil, nil, variant, errors.New("variant is not local")
	}
	f, info, err := svc.storage.open(variant.ObjectKey)
	return f, info, variant, err
}

func selectVariant(asset *model.AudioAsset, variantLabel string) model.AudioAssetVariant {
	if asset == nil {
		return model.AudioAssetVariant{}
	}
	var selected model.AudioAssetVariant
	if variantLabel == "" || variantLabel == "default" {
		return model.AudioAssetVariant{
			Label:       "default",
			BitrateKbps: asset.BitrateKbps,
			ObjectKey:   asset.ObjectKey,
			Size:        asset.Size,
			StorageType: asset.StorageType,
			Duration:    asset.DurationSeconds,
		}
	}
	for _, v := range asset.Variants {
		if v.Label == variantLabel || fmt.Sprintf("%dk", v.BitrateKbps) == variantLabel {
			selected = v
			break
		}
	}
	if selected.ObjectKey == "" {
		selected = model.AudioAssetVariant{
			Label:       "default",
			BitrateKbps: asset.BitrateKbps,
			ObjectKey:   asset.ObjectKey,
			Size:        asset.Size,
			StorageType: asset.StorageType,
			Duration:    asset.DurationSeconds,
		}
	}
	return selected
}

func AudioVariantFor(asset *model.AudioAsset, variantLabel string) model.AudioAssetVariant {
	return selectVariant(asset, variantLabel)
}

func AudioOpenLocalVariant(asset *model.AudioAsset, variantLabel string) (*os.File, os.FileInfo, model.AudioAssetVariant, error) {
	svc := GetAudioService()
	if svc == nil {
		return nil, nil, model.AudioAssetVariant{}, errors.New("音频服务未初始化")
	}
	return svc.ResolveLocalFile(asset, variantLabel)
}

func (svc *audioService) RemoveLocalAsset(objectKey string) error {
	full, err := svc.storage.fullPath(objectKey)
	if err != nil {
		return err
	}
	trashPath := filepath.Join(svc.storage.trashDir(), filepath.Base(objectKey)+fmt.Sprintf("-%d", time.Now().Unix()))
	return os.Rename(full, trashPath)
}

func (svc *audioService) FFmpegAvailable() bool {
	return svc.ffmpegPath != ""
}

func (svc *audioService) PlatformInfo() map[string]string {
	return map[string]string{
		"os":      runtime.GOOS,
		"arch":    runtime.GOARCH,
		"ffmpeg":  svc.ffmpegPath,
		"ffprobe": svc.ffprobePath,
	}
}

func (svc *audioService) SerializeConfig() map[string]interface{} {
	buf, _ := json.Marshal(svc.cfg)
	var out map[string]interface{}
	_ = json.Unmarshal(buf, &out)
	return out
}
