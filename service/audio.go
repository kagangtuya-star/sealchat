package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gabriel-vasile/mimetype"

	"sealchat/model"
	"sealchat/service/storage"
	"sealchat/utils"
)

type audioService struct {
	cfg          utils.AudioConfig
	storage      *localAudioStorage
	objectStore  *storage.Manager
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

var ffmpegDurationPattern = regexp.MustCompile(`Duration:\s*([0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]+)?)`)

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
	Scope       model.AudioAssetScope
	WorldID     *string
}

func InitAudioService(cfg utils.AudioConfig, store *storage.Manager) error {
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
			objectStore:  store,
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

		audioSvc.ffmpegPath, audioSvc.ffprobePath = resolveFFmpegPaths(&cfg)
	})
	return audioSvcErr
}

func executableDir() string {
	if exe, err := os.Executable(); err == nil {
		exe = strings.TrimSpace(exe)
		if exe != "" {
			return filepath.Dir(exe)
		}
	}
	if len(os.Args) > 0 {
		arg0 := strings.TrimSpace(os.Args[0])
		if arg0 != "" {
			return filepath.Dir(arg0)
		}
	}
	return ""
}

func resolveFFmpegPaths(cfg *utils.AudioConfig) (ffmpegPath, ffprobePath string) {
	configSpecified := strings.TrimSpace(cfg.FFmpegPath) != ""

	if configSpecified {
		ffmpegPath = detectExecutableWithVerify([]string{cfg.FFmpegPath}, verifyFFmpegBinary)
		if ffmpegPath != "" {
			ffprobePath = detectExecutableWithVerify(ffprobeCandidatesFromFFmpegPath(ffmpegPath), verifyFFmpegBinary)
		}
	}

	if ffmpegPath == "" {
		ffmpegPath = discoverFFmpegInExeDir(ffmpegCandidateNames())
	}
	if ffprobePath == "" {
		ffprobePath = discoverFFmpegInExeDir(ffprobeCandidateNames())
	}

	if ffmpegPath != "" && !configSpecified {
		cfg.FFmpegPath = ffmpegPath
		go func() {
			appCfg := utils.GetConfig()
			if appCfg != nil {
				appCfg.Audio.FFmpegPath = ffmpegPath
				utils.WriteConfig(appCfg)
				log.Printf("[音频] 已自动发现 FFmpeg 并写入配置: %s", ffmpegPath)
			}
		}()
	}

	if ffmpegPath == "" {
		ffmpegPath = detectExecutableWithVerify(ffmpegCandidateNames(), verifyFFmpegBinary)
	}
	if ffprobePath == "" {
		ffprobePath = detectExecutableWithVerify(ffprobeCandidateNames(), verifyFFmpegBinary)
	}
	return
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
	return detectExecutableWithVerify(candidates, nil)
}

func detectExecutableWithVerify(candidates []string, verify func(string) bool) string {
	for _, candidate := range candidates {
		path := strings.TrimSpace(candidate)
		if path == "" {
			continue
		}
		var resolved string
		if filepath.Base(path) == path {
			if r, err := exec.LookPath(path); err == nil && fileExists(r) {
				resolved = r
			}
		} else if fileExists(path) {
			resolved = path
		}
		if resolved == "" {
			continue
		}
		if verify != nil && !verify(resolved) {
			continue
		}
		return resolved
	}
	return ""
}

func verifyFFmpegBinary(path string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "-version")
	return cmd.Run() == nil
}

func ffmpegCandidateNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"ffmpeg.exe"}
	}
	return []string{"ffmpeg"}
}

func ffprobeCandidateNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"ffprobe.exe"}
	}
	return []string{"ffprobe"}
}

func ffprobeCandidatesFromFFmpegPath(ffmpegPath string) []string {
	dir := filepath.Dir(ffmpegPath)
	names := ffprobeCandidateNames()
	candidates := make([]string, 0, len(names))
	for _, name := range names {
		candidates = append(candidates, filepath.Join(dir, name))
	}
	return candidates
}

func discoverFFmpegInExeDir(names []string) string {
	exeDir := executableDir()
	if exeDir == "" {
		return ""
	}
	for _, name := range names {
		candidate := filepath.Join(exeDir, name)
		if fileExists(candidate) && verifyFFmpegBinary(candidate) {
			return candidate
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

func (svc *audioService) importFromPath(filePath string, opts AudioUploadOptions) (*model.AudioAsset, error) {
	if audioSvc == nil {
		return nil, errors.New("audio service not initialized")
	}
	trimmed := strings.TrimSpace(filePath)
	if trimmed == "" {
		return nil, errors.New("文件路径为空")
	}
	if strings.TrimSpace(svc.cfg.TempDir) == "" {
		return nil, errors.New("音频临时目录未配置")
	}
	info, err := os.Stat(trimmed)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, errors.New("不是有效的音频文件")
	}
	if info.Size() > svc.maxUploadBytes() {
		return nil, fmt.Errorf("%w (最大 %d MB)", ErrAudioTooLarge, svc.cfg.MaxUploadSizeMB)
	}
	src, err := os.Open(trimmed)
	if err != nil {
		return nil, err
	}
	defer src.Close()
	mimeType, err := svc.validateMime(src)
	if err != nil {
		return nil, err
	}
	tempName := fmt.Sprintf("import-%s", utils.NewID())
	tempPath := filepath.Join(svc.cfg.TempDir, tempName)
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return nil, err
	}
	_, copyErr := io.Copy(tempFile, src)
	if closeErr := tempFile.Close(); copyErr == nil && closeErr != nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		_ = os.Remove(tempPath)
		return nil, copyErr
	}
	asset, err := svc.persistTempFile(tempPath, filepath.Base(trimmed), mimeType, opts)
	_ = os.Remove(tempPath)
	if err != nil {
		return nil, err
	}
	return asset, nil
}

func (svc *audioService) persistTempFile(tempPath, originalName, mimeType string, opts AudioUploadOptions) (*model.AudioAsset, error) {
	asset := svc.newAssetRecord(originalName, opts)
	if svc.shouldUseObjectStore() {
		if remote, err := svc.persistWithObjectStore(asset, tempPath, mimeType, originalName); err == nil {
			return remote, nil
		} else {
			log.Printf("[audio] 上传对象存储失败，使用本地存储: %v", err)
		}
	}
	if svc.cfg.EnableTranscode && svc.ffmpegPath != "" {
		objectKey := filepath.ToSlash(filepath.Join("original", fmt.Sprintf("%s%s", asset.ID, pickExtension(mimeType, tempPath))))
		size, err := svc.storage.moveFromTemp(tempPath, objectKey)
		if err != nil {
			return nil, err
		}
		asset.StorageType = model.StorageLocal
		asset.ObjectKey = objectKey
		asset.Size = size
		asset.DurationSeconds, _ = svc.probeDuration(objectKey)
		asset.BitrateKbps = svc.cfg.DefaultBitrateKbps
		asset.Variants = nil
		asset.TranscodeStatus = model.AudioTranscodePending
		return asset, nil
	}
	return svc.persistLocalAsset(asset, tempPath, mimeType)
}

func (svc *audioService) newAssetRecord(originalName string, opts AudioUploadOptions) *model.AudioAsset {
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
	asset.StorageType = model.StorageLocal
	scope := opts.Scope
	if scope == "" {
		scope = model.AudioScopeCommon
	}
	asset.Scope = scope
	asset.WorldID = cloneStringPtr(opts.WorldID)
	return asset
}

func (svc *audioService) shouldUseObjectStore() bool {
	return svc.objectStore != nil && svc.objectStore.ActiveBackendForAudio() == storage.BackendS3
}

func (svc *audioService) persistLocalAsset(asset *model.AudioAsset, tempPath, mimeType string) (*model.AudioAsset, error) {
	result, err := svc.generateVariants(tempPath, asset.ID, mimeType)
	if err != nil {
		return nil, err
	}
	asset.StorageType = model.StorageLocal
	asset.ObjectKey = result.Primary.ObjectKey
	asset.BitrateKbps = result.Primary.BitrateKbps
	asset.DurationSeconds = result.Primary.Duration
	asset.Size = result.Primary.Size
	asset.Variants = model.JSONList[model.AudioAssetVariant](result.Extras)
	asset.TranscodeStatus = result.TranscodeStatus
	return asset, nil
}

func (svc *audioService) persistWithObjectStore(asset *model.AudioAsset, tempPath, mimeType, originalName string) (*model.AudioAsset, error) {
	if svc.objectStore == nil {
		return nil, errors.New("对象存储未配置")
	}
	objectKey := storage.BuildAudioObjectKey(asset.ID, originalName)
	duration, _ := svc.probeDurationFromFile(tempPath)
	result, err := svc.objectStore.UploadToS3(context.Background(), storage.UploadInput{
		ObjectKey:   objectKey,
		LocalPath:   tempPath,
		ContentType: mimeType,
	})
	if err != nil {
		return nil, err
	}
	_ = os.Remove(tempPath)
	asset.StorageType = model.StorageS3
	asset.ObjectKey = result.ObjectKey
	asset.Size = result.Size
	asset.DurationSeconds = duration
	asset.BitrateKbps = svc.cfg.DefaultBitrateKbps
	asset.Variants = nil
	asset.TranscodeStatus = model.AudioTranscodeReady
	return asset, nil
}

type variantResult struct {
	Primary         model.AudioAssetVariant
	Extras          []model.AudioAssetVariant
	TranscodeStatus model.AudioTranscodeStatus
}

func (svc *audioService) scheduleTranscode(assetID, sourceKey string) {
	if svc == nil || svc.ffmpegPath == "" {
		return
	}
	go func() {
		if err := svc.transcodeAsset(assetID, sourceKey); err != nil {
			log.Printf("[audio] transcode failed for %s: %v", assetID, err)
		}
	}()
}

func (svc *audioService) transcodeAsset(assetID, sourceKey string) error {
	full, err := svc.storage.fullPath(sourceKey)
	if err != nil {
		return err
	}
	result, err := svc.generateVariants(full, assetID, "")
	if err != nil {
		return model.GetDB().Model(&model.AudioAsset{}).
			Where("id = ?", assetID).
			Updates(map[string]interface{}{
				"transcode_status": model.AudioTranscodeFailed,
				"updated_at":       time.Now(),
			}).Error
	}
	updates := map[string]interface{}{
		"object_key":       result.Primary.ObjectKey,
		"bitrate_kbps":     result.Primary.BitrateKbps,
		"duration":         result.Primary.Duration,
		"size":             result.Primary.Size,
		"variants":         model.JSONList[model.AudioAssetVariant](result.Extras),
		"transcode_status": result.TranscodeStatus,
		"updated_at":       time.Now(),
	}
	if err := model.GetDB().Model(&model.AudioAsset{}).Where("id = ?", assetID).Updates(updates).Error; err != nil {
		return err
	}
	if result.TranscodeStatus == model.AudioTranscodeReady {
		svc.removeAssetObject(model.StorageLocal, sourceKey)
	}
	return nil
}

func (svc *audioService) generateVariants(tempPath, assetID, mimeType string) (*variantResult, error) {
	primary := model.AudioAssetVariant{
		Label:       "default",
		BitrateKbps: svc.cfg.DefaultBitrateKbps,
		StorageType: model.StorageLocal,
	}
	result := &variantResult{TranscodeStatus: model.AudioTranscodeReady}
	transcoded := false

	if svc.cfg.EnableTranscode && svc.ffmpegPath != "" {
		// 仅生成一份转码产物：避免默认+备用码率带来的额外存储占用。
		// 选择策略：优先使用 DefaultBitrateKbps；若无效则回退到 AlternateBitrates 中的第一个正值；最后允许 bitrate=0 让 ffmpeg 自行决定。
		bitrate := svc.cfg.DefaultBitrateKbps
		if bitrate <= 0 {
			for _, candidate := range svc.cfg.AlternateBitrates {
				if candidate > 0 {
					bitrate = candidate
					break
				}
			}
		}
		label := "default"
		objectName := fmt.Sprintf("%s.ogg", assetID)
		if bitrate > 0 {
			label = fmt.Sprintf("%dk", bitrate)
			objectName = fmt.Sprintf("%s_%s.ogg", assetID, label)
		}
		objectKey := filepath.ToSlash(filepath.Join("opus", objectName))
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
			StorageType: model.StorageLocal,
		}
		duration, err := svc.probeDuration(objectKey)
		if err == nil {
			variant.Duration = duration
		}
		primary = variant
		result.Primary = primary
		result.Extras = nil
		transcoded = true
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
	full, err := svc.storage.fullPath(objectKey)
	if err != nil {
		return 0, err
	}
	return svc.probeDurationFromFile(full)
}

func (svc *audioService) probeDurationFromFile(path string) (float64, error) {
	var ffprobeErr error
	if svc.ffprobePath == "" {
		ffprobeErr = errors.New("ffprobe not available")
	} else {
		duration, err := svc.probeDurationWithFFprobe(path)
		if err == nil {
			return duration, nil
		}
		ffprobeErr = err
	}

	if svc.ffmpegPath == "" {
		return 0, ffprobeErr
	}

	duration, err := svc.probeDurationWithFFmpeg(path)
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed: %w; ffmpeg fallback failed: %v", ffprobeErr, err)
	}
	return duration, nil
}

func (svc *audioService) probeDurationWithFFprobe(path string) (float64, error) {
	args := []string{"-v", "error", "-show_entries", "format=duration", "-of", "default=nokey=1:noprint_wrappers=1", path}
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

func (svc *audioService) probeDurationWithFFmpeg(path string) (float64, error) {
	cmd := exec.CommandContext(context.Background(), svc.ffmpegPath, "-hide_banner", "-i", path)
	output, err := cmd.CombinedOutput()
	if duration, ok := parseFFmpegDuration(string(output)); ok {
		return duration, nil
	}
	if err != nil {
		return 0, err
	}
	return 0, errors.New("duration not found in ffmpeg output")
}

func parseFFmpegDuration(output string) (float64, bool) {
	match := ffmpegDurationPattern.FindStringSubmatch(output)
	if len(match) < 2 {
		return 0, false
	}
	return parseTimestampToSeconds(match[1])
}

func parseTimestampToSeconds(value string) (float64, bool) {
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0, false
	}
	hours, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, false
	}
	minutes, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, false
	}
	seconds, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return 0, false
	}
	return float64(hours*3600+minutes*60) + seconds, true
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
	if variant.StorageType != model.StorageLocal {
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

func (svc *audioService) removeAssetObject(storageType model.StorageType, objectKey string) {
	if strings.TrimSpace(objectKey) == "" {
		return
	}
	switch storageType {
	case model.StorageS3:
		if svc.objectStore != nil {
			_ = svc.objectStore.Delete(context.Background(), storage.BackendS3, objectKey)
		}
	default:
		_ = svc.RemoveLocalAsset(objectKey)
	}
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
