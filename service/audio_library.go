package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/gabriel-vasile/mimetype"
	"gorm.io/gorm"
	"sealchat/model"
	audiolibrarybackend "sealchat/service/audio_library_backend"
	"sealchat/utils"
)

type AudioLibrarySettings struct {
	Mode          utils.AudioLibraryMode `json:"mode"`
	Prefix        string                 `json:"prefix"`
	SelectorDepth int                    `json:"selectorDepth"`
	SourceID      string                 `json:"sourceId"`
	S3Available   bool                   `json:"s3Available"`
	BucketLabel   string                 `json:"bucketLabel"`
	CanConfigure  bool                   `json:"canConfigure"`
	Version       int                    `json:"version"`
}

type AudioLibraryPrefix struct {
	Ref    string `json:"ref"`
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
}

type AudioLibraryAsset struct {
	Ref          string    `json:"ref"`
	Name         string    `json:"name"`
	Key          string    `json:"key"`
	ParentPrefix string    `json:"parentPrefix"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"lastModified"`
	ETag         string    `json:"etag"`
	StorageClass string    `json:"storageClass,omitempty"`
	Extension    string    `json:"extension"`
	ContentType  string    `json:"contentType,omitempty"`
}

type AudioLibraryListResult struct {
	Items       []AudioLibraryAsset  `json:"items"`
	Prefixes    []AudioLibraryPrefix `json:"prefixes"`
	NextCursor  string               `json:"nextCursor,omitempty"`
	IsTruncated bool                 `json:"isTruncated"`
}

var (
	ErrAudioLibraryUnavailable  = audiolibrarybackend.ErrUnavailable
	ErrAudioLibraryNotFound     = audiolibrarybackend.ErrNotFound
	ErrAudioLibraryPermission   = audiolibrarybackend.ErrPermission
	audioLibraryBackendRegistry = audiolibrarybackend.NewRegistry()
)

type audioLibraryRuntimeConfig struct {
	WorldID       string
	Mode          utils.AudioLibraryMode
	S3Prefix      string
	S3SourceID    string
	SelectorDepth int
}

const (
	defaultAudioLibrarySelectorDepth = 2
	maxAudioLibrarySelectorDepth     = 5
)

func NormalizeAudioLibrarySelectorDepth(raw int) int {
	if raw < 0 || raw > maxAudioLibrarySelectorDepth {
		return defaultAudioLibrarySelectorDepth
	}
	return raw
}

func ResolveAudioLibraryWorldID(raw string) (string, error) {
	worldID := strings.TrimSpace(raw)
	if worldID != "" {
		world, err := GetWorldByID(worldID)
		if err != nil {
			return "", err
		}
		return world.ID, nil
	}
	world, err := GetOrCreateDefaultWorld()
	if err != nil {
		return "", err
	}
	return world.ID, nil
}

func AudioLibraryCanAccessWorld(worldID, userID string) bool {
	world, err := GetWorldByID(worldID)
	if err != nil || world == nil {
		return false
	}
	if IsWorldAdmin(worldID, userID) || world.Visibility == model.WorldVisibilityPublic {
		return true
	}
	return IsWorldMember(worldID, userID)
}

func configuredAudioLibrary(cfg *utils.AppConfig, worldID string) audioLibraryRuntimeConfig {
	worldID, err := ResolveAudioLibraryWorldID(worldID)
	if err != nil {
		return audioLibraryRuntimeConfig{}
	}
	world, err := GetWorldByID(worldID)
	if err != nil || world == nil {
		return audioLibraryRuntimeConfig{WorldID: worldID}
	}
	mode := utils.AudioLibraryMode(strings.ToLower(strings.TrimSpace(world.AudioLibraryMode)))
	if mode != utils.AudioLibraryModeS3 {
		mode = utils.AudioLibraryModeDatabase
	}
	prefix, prefixErr := NormalizeAudioLibraryPrefix(world.AudioLibraryPrefix)
	if prefixErr != nil {
		prefix = ""
	}
	if cfg == nil {
		return audioLibraryRuntimeConfig{WorldID: worldID, Mode: mode, S3Prefix: prefix, SelectorDepth: NormalizeAudioLibrarySelectorDepth(world.AudioLibrarySelectorDepth)}
	}
	return audioLibraryRuntimeConfig{
		WorldID:       worldID,
		Mode:          mode,
		S3SourceID:    audioLibrarySourceID(cfg.Storage),
		S3Prefix:      prefix,
		SelectorDepth: NormalizeAudioLibrarySelectorDepth(world.AudioLibrarySelectorDepth),
	}
}

func audioLibrarySourceID(cfg utils.StorageConfig) string {
	seed := strings.TrimSpace(cfg.S3.Endpoint) + "\x00" +
		strings.TrimSpace(cfg.S3.Region) + "\x00" + strings.TrimSpace(cfg.S3.Bucket)
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:8])
}

func ensureAudioLibraryS3Mode(cfg audioLibraryRuntimeConfig) error {
	if cfg.Mode != utils.AudioLibraryModeS3 {
		return errors.New("音频素材库当前为 database 模式")
	}
	return nil
}

func NormalizeAudioLibraryPrefix(raw string) (string, error) {
	raw = strings.ReplaceAll(raw, "\\", "/")
	raw = strings.TrimLeft(raw, "/")
	if raw == "" {
		return "", nil
	}
	encodedLower := strings.ToLower(raw)
	if strings.Contains(encodedLower, "%2e") || strings.Contains(encodedLower, "%00") {
		return "", errors.New("S3 prefix 包含编码路径穿越")
	}
	for _, r := range raw {
		if unicode.IsControl(r) {
			return "", errors.New("S3 prefix 包含控制字符")
		}
	}
	parts := strings.Split(raw, "/")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		if part == "." || part == ".." || strings.Contains(part, "\x00") {
			return "", errors.New("S3 prefix 包含非法路径段")
		}
		clean = append(clean, part)
	}
	if len(clean) == 0 {
		return "", nil
	}
	return strings.Join(clean, "/") + "/", nil
}

func JoinS3LibraryPrefix(prefix, relative string) (string, error) {
	base, err := NormalizeAudioLibraryPrefix(prefix)
	if err != nil {
		return "", err
	}
	if relative == "" {
		return base, nil
	}
	// A directory prefix must retain its trailing slash. With delimiter="/",
	// dropping it changes `test/` into `test`, causing COS to return `test/`
	// as a CommonPrefix instead of returning objects inside that directory.
	keepTrailingSlash := strings.HasSuffix(relative, "/")
	if strings.Contains(relative, "\\") {
		return "", errors.New("S3 object key 不允许反斜杠")
	}
	relativeLower := strings.ToLower(relative)
	if strings.Contains(relativeLower, "%2e") || strings.Contains(relativeLower, "%00") {
		return "", errors.New("S3 object key 包含编码路径穿越")
	}
	parts := strings.Split(strings.Trim(relative, "/"), "/")
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", errors.New("S3 object key 包含非法路径段")
		}
		for _, r := range part {
			if unicode.IsControl(r) {
				return "", errors.New("S3 object key 包含控制字符")
			}
		}
		clean = append(clean, part)
	}
	joined := base + strings.Join(clean, "/")
	if keepTrailingSlash && joined != "" && !strings.HasSuffix(joined, "/") {
		joined += "/"
	}
	return joined, nil
}

func AudioLibraryModeIsS3(worldID string) bool {
	cfg := utils.GetConfig()
	return cfg != nil && configuredAudioLibrary(cfg, worldID).Mode == utils.AudioLibraryModeS3
}

func AudioLibrarySettingsGet(worldID string, canConfigure bool) AudioLibrarySettings {
	cfg := utils.GetConfig()
	if cfg == nil {
		return AudioLibrarySettings{Mode: utils.AudioLibraryModeDatabase, SelectorDepth: defaultAudioLibrarySelectorDepth, CanConfigure: canConfigure}
	}
	normalized := configuredAudioLibrary(cfg, worldID)
	backend, backendErr := audioLibraryBackendRegistry.Get(cfg.Storage)
	s3Available := backendErr == nil
	bucketLabel := ""
	if backend != nil {
		bucketLabel = backend.BucketLabel()
	}
	return AudioLibrarySettings{
		Mode: normalized.Mode, Prefix: normalized.S3Prefix, SelectorDepth: normalized.SelectorDepth, SourceID: normalized.S3SourceID,
		S3Available:  s3Available,
		BucketLabel:  bucketLabel,
		CanConfigure: canConfigure, Version: 1,
	}
}

func AudioLibrarySaveSettings(worldID, actorID string, mode utils.AudioLibraryMode, rawPrefix string, selectorDepth int) (AudioLibrarySettings, error) {
	cfg := utils.GetConfig()
	if cfg == nil {
		return AudioLibrarySettings{}, errors.New("配置未初始化")
	}
	worldID, err := ResolveAudioLibraryWorldID(worldID)
	if err != nil {
		return AudioLibrarySettings{}, err
	}
	_ = actorID // API layer performs world/system permission check.
	normalizedMode := utils.AudioLibraryMode(strings.ToLower(strings.TrimSpace(string(mode))))
	if normalizedMode != utils.AudioLibraryModeS3 {
		normalizedMode = utils.AudioLibraryModeDatabase
	}
	prefix, err := NormalizeAudioLibraryPrefix(rawPrefix)
	if err != nil {
		return AudioLibrarySettings{}, err
	}
	selectorDepth = NormalizeAudioLibrarySelectorDepth(selectorDepth)
	if normalizedMode == utils.AudioLibraryModeS3 {
		store, storeErr := audioLibraryBackendRegistry.Get(cfg.Storage)
		if storeErr != nil || store == nil {
			return AudioLibrarySettings{}, ErrAudioLibraryUnavailable
		}
		// Validate connection against bucket root. A virtual prefix may not
		// have a marker object yet; S3 should still accept it as an empty folder.
		if _, err := store.List(contextBackground(), "", "", 1); err != nil {
			return AudioLibrarySettings{}, fmt.Errorf("S3 prefix 不可读取: %w", err)
		}
		if prefix != "" {
			if _, err := store.List(contextBackground(), prefix, "", 1); err != nil && !errors.Is(err, ErrAudioLibraryNotFound) {
				return AudioLibrarySettings{}, fmt.Errorf("S3 prefix 不可读取: %w", err)
			}
		}
	}
	if err := model.GetDB().Model(&model.WorldModel{}).Where("id = ?", worldID).Updates(map[string]any{
		"audio_library_mode":           string(normalizedMode),
		"audio_library_prefix":         prefix,
		"audio_library_selector_depth": selectorDepth,
		"updated_at":                   time.Now(),
	}).Error; err != nil {
		return AudioLibrarySettings{}, err
	}
	return AudioLibrarySettingsGet(worldID, true), nil
}

func contextBackground() context.Context { return context.Background() }

func audioLibraryConfig(worldID string) (audioLibraryRuntimeConfig, audiolibrarybackend.Backend, error) {
	cfg := utils.GetConfig()
	if cfg == nil {
		return audioLibraryRuntimeConfig{}, nil, ErrAudioLibraryUnavailable
	}
	store, err := audioLibraryBackendRegistry.Get(cfg.Storage)
	if err != nil || store == nil {
		return audioLibraryRuntimeConfig{}, nil, ErrAudioLibraryUnavailable
	}
	return configuredAudioLibrary(cfg, worldID), store, nil
}

func resolveAudioLibraryPrefix(worldID, raw string) (string, audioLibraryRuntimeConfig, audiolibrarybackend.Backend, error) {
	cfg, store, err := audioLibraryConfig(worldID)
	if err != nil {
		return "", cfg, store, err
	}
	prefix, err := joinConfiguredAudioLibraryPrefix(cfg, raw)
	if err != nil {
		return "", cfg, store, err
	}
	return prefix, cfg, store, nil
}

func joinConfiguredAudioLibraryPrefix(cfg audioLibraryRuntimeConfig, raw string) (string, error) {
	prefix, err := NormalizeAudioLibraryPrefix(raw)
	if err != nil {
		return "", err
	}
	base := cfg.S3Prefix
	if prefix == "" {
		return base, nil
	}
	if base != "" && (prefix == base || strings.HasPrefix(prefix, base)) {
		return prefix, nil
	}
	return JoinS3LibraryPrefix(base, prefix)
}

func ListAudioLibraryPrefixes(worldID, rawPrefix, cursor string, limit int) (*AudioLibraryListResult, error) {
	prefix, cfg, store, err := resolveAudioLibraryPrefix(worldID, rawPrefix)
	if err != nil {
		return nil, err
	}
	result, err := store.List(contextBackground(), prefix, strings.TrimSpace(cursor), limit)
	if err != nil {
		if errors.Is(err, ErrAudioLibraryNotFound) {
			return &AudioLibraryListResult{Items: []AudioLibraryAsset{}, Prefixes: []AudioLibraryPrefix{}}, nil
		}
		return nil, err
	}
	items := make([]AudioLibraryPrefix, 0, len(result.Prefixes))
	seen := make(map[string]struct{}, len(result.Prefixes))
	appendPrefix := func(child string) {
		normalizedChild, normalizeErr := NormalizeAudioLibraryPrefix(child)
		if normalizeErr != nil || normalizedChild != child {
			return
		}
		child = normalizedChild
		if _, ok := seen[child]; ok {
			return
		}
		name := strings.TrimSuffix(strings.TrimPrefix(child, prefix), "/")
		if strings.Contains(name, "/") {
			name = strings.Split(name, "/")[0]
		}
		if name == "" {
			return
		}
		ref, _ := BuildS3AudioPrefixRef(cfg.S3SourceID, child)
		items = append(items, AudioLibraryPrefix{Ref: ref, Name: name, Prefix: child})
		seen[child] = struct{}{}
	}
	for _, child := range result.Prefixes {
		appendPrefix(child)
	}
	// Some S3-compatible services return directory markers or flat object keys
	// in Contents instead of CommonPrefixes. Derive immediate child prefixes as
	// a compatibility fallback for this direct-reading endpoint.
	for _, object := range result.Objects {
		if !strings.HasPrefix(object.Key, prefix) {
			continue
		}
		remainder := strings.TrimPrefix(object.Key, prefix)
		separator := strings.IndexByte(remainder, '/')
		if separator < 0 {
			continue
		}
		appendPrefix(prefix + remainder[:separator+1])
	}
	return &AudioLibraryListResult{Items: []AudioLibraryAsset{}, Prefixes: items, NextCursor: result.NextCursor, IsTruncated: result.IsTruncated}, nil
}

func ListAudioLibraryAssets(worldID, rawPrefix, cursor string, limit int) (*AudioLibraryListResult, error) {
	prefix, cfg, store, err := resolveAudioLibraryPrefix(worldID, rawPrefix)
	if err != nil {
		return nil, err
	}
	result, err := store.List(contextBackground(), prefix, strings.TrimSpace(cursor), limit)
	if err != nil {
		if errors.Is(err, ErrAudioLibraryNotFound) {
			return &AudioLibraryListResult{Items: []AudioLibraryAsset{}, Prefixes: []AudioLibraryPrefix{}}, nil
		}
		return nil, err
	}
	items := make([]AudioLibraryAsset, 0, len(result.Objects))
	for _, object := range result.Objects {
		if strings.HasSuffix(object.Key, "/") || !isLikelyAudioKey(object.Key) || !isWithinAudioPrefix(cfg.S3Prefix, object.Key) {
			continue
		}
		name := filepath.Base(object.Key)
		parent := strings.TrimSuffix(strings.TrimSuffix(object.Key, name), "/")
		ref, refErr := BuildS3AudioRef(cfg.S3SourceID, object.Key)
		if refErr != nil {
			continue
		}
		items = append(items, AudioLibraryAsset{Ref: ref, Name: name, Key: object.Key, ParentPrefix: parent, Size: object.Size, LastModified: object.LastModified, ETag: object.ETag, StorageClass: object.StorageClass, Extension: strings.ToLower(filepath.Ext(name)), ContentType: object.ContentType})
	}
	return &AudioLibraryListResult{Items: items, Prefixes: []AudioLibraryPrefix{}, NextCursor: result.NextCursor, IsTruncated: result.IsTruncated}, nil
}

// ListAudioLibrarySelectorAssets returns audio files in the configured S3
// prefix and its child directories up to selectorDepth levels. It is kept
// separate from ListAudioLibraryAssets so the asset manager retains its
// current-directory semantics.
func ListAudioLibrarySelectorAssets(worldID, rawPrefix string, selectorDepth, limit int) (*AudioLibraryListResult, error) {
	prefix, cfg, store, err := resolveAudioLibraryPrefix(worldID, rawPrefix)
	if err != nil {
		return nil, err
	}
	selectorDepth = NormalizeAudioLibrarySelectorDepth(selectorDepth)
	if limit <= 0 {
		limit = 1000
	}
	if limit > 5000 {
		limit = 5000
	}

	type pendingPrefix struct {
		prefix string
		level  int
	}
	queue := []pendingPrefix{{prefix: prefix, level: 0}}
	visited := map[string]struct{}{}
	seenObjects := map[string]struct{}{}
	items := make([]AudioLibraryAsset, 0, minInt(limit, 128))
	truncated := false

	appendChild := func(child string, level int) {
		if level > selectorDepth {
			return
		}
		normalized, normalizeErr := NormalizeAudioLibraryPrefix(child)
		if normalizeErr != nil || normalized != child || !isWithinAudioPrefix(prefix, strings.TrimSuffix(child, "/")) {
			return
		}
		if _, ok := visited[child]; ok {
			return
		}
		for _, item := range queue {
			if item.prefix == child {
				return
			}
		}
		queue = append(queue, pendingPrefix{prefix: child, level: level})
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, ok := visited[current.prefix]; ok {
			continue
		}
		visited[current.prefix] = struct{}{}
		cursor := ""
		for {
			result, listErr := store.List(contextBackground(), current.prefix, cursor, 1000)
			if listErr != nil {
				if errors.Is(listErr, ErrAudioLibraryNotFound) {
					break
				}
				return nil, listErr
			}
			for _, child := range result.Prefixes {
				appendChild(child, current.level+1)
			}
			for _, object := range result.Objects {
				if !strings.HasPrefix(object.Key, current.prefix) {
					continue
				}
				remainder := strings.TrimPrefix(object.Key, current.prefix)
				separator := strings.IndexByte(remainder, '/')
				if separator >= 0 {
					if current.level < selectorDepth {
						appendChild(current.prefix+remainder[:separator+1], current.level+1)
					}
					continue
				}
				if strings.HasSuffix(object.Key, "/") || !isLikelyAudioKey(object.Key) || !isWithinAudioPrefix(cfg.S3Prefix, object.Key) {
					continue
				}
				if _, ok := seenObjects[object.Key]; ok {
					continue
				}
				seenObjects[object.Key] = struct{}{}
				ref, refErr := BuildS3AudioRef(cfg.S3SourceID, object.Key)
				if refErr != nil {
					continue
				}
				items = append(items, AudioLibraryAsset{
					Ref: ref, Name: filepath.Base(object.Key), Key: object.Key,
					ParentPrefix: strings.TrimSuffix(strings.TrimSuffix(object.Key, filepath.Base(object.Key)), "/"),
					Size:         object.Size, LastModified: object.LastModified, ETag: object.ETag,
					StorageClass: object.StorageClass, Extension: strings.ToLower(filepath.Ext(object.Key)), ContentType: object.ContentType,
				})
				if len(items) >= limit {
					truncated = true
					break
				}
			}
			if truncated || !result.IsTruncated || strings.TrimSpace(result.NextCursor) == "" {
				break
			}
			cursor = result.NextCursor
		}
		if truncated {
			break
		}
	}

	return &AudioLibraryListResult{Items: items, Prefixes: []AudioLibraryPrefix{}, IsTruncated: truncated}, nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func ResolveAudioLibraryAssets(worldID string, refs []string) ([]AudioLibraryAsset, error) {
	cfg, store, err := audioLibraryConfig(worldID)
	if err != nil {
		return nil, err
	}
	if err := ensureAudioLibraryS3Mode(cfg); err != nil {
		return nil, err
	}
	result := make([]AudioLibraryAsset, 0, len(refs))
	for _, raw := range refs {
		ref, parseErr := ParseAudioRef(raw)
		if parseErr != nil || ref.Source != AudioRefSourceS3 || ref.IsPrefix || ref.SourceID != cfg.S3SourceID || !isWithinAudioPrefix(cfg.S3Prefix, ref.ObjectKey) {
			continue
		}
		object, statErr := store.Stat(contextBackground(), ref.ObjectKey)
		if statErr != nil {
			continue
		}
		if !isLikelyAudioKey(object.Key) {
			continue
		}
		refValue, _ := BuildS3AudioRef(cfg.S3SourceID, object.Key)
		result = append(result, AudioLibraryAsset{Ref: refValue, Name: filepath.Base(object.Key), Key: object.Key, ParentPrefix: strings.TrimSuffix(strings.TrimSuffix(object.Key, filepath.Base(object.Key)), "/"), Size: object.Size, LastModified: object.LastModified, ETag: object.ETag, StorageClass: object.StorageClass, Extension: strings.ToLower(filepath.Ext(object.Key)), ContentType: object.ContentType})
	}
	return result, nil
}

func UploadAudioLibraryAsset(worldID string, file *multipart.FileHeader, rawPrefix string) (AudioLibraryAsset, error) {
	if file == nil {
		return AudioLibraryAsset{}, errors.New("上传文件为空")
	}
	prefix, cfg, store, err := resolveAudioLibraryPrefix(worldID, rawPrefix)
	if err != nil {
		return AudioLibraryAsset{}, err
	}
	if err := ensureAudioLibraryS3Mode(cfg); err != nil {
		return AudioLibraryAsset{}, err
	}
	if !isLikelyAudioKey(file.Filename) {
		return AudioLibraryAsset{}, ErrAudioUnsupportedMime
	}
	if audioSvc != nil && audioSvc.maxUploadBytes() > 0 && file.Size > audioSvc.maxUploadBytes() {
		return AudioLibraryAsset{}, ErrAudioTooLarge
	}
	name := filepath.Base(file.Filename)
	if name == "" || name == "." || name == ".." || strings.ContainsAny(file.Filename, "/\\") {
		return AudioLibraryAsset{}, errors.New("音频文件名包含非法路径")
	}
	key, err := JoinS3LibraryPrefix(prefix, name)
	if err != nil {
		return AudioLibraryAsset{}, err
	}
	tmp, err := os.CreateTemp(audioSvc.cfg.TempDir, "audio-library-")
	if err != nil {
		return AudioLibraryAsset{}, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	in, err := file.Open()
	if err != nil {
		tmp.Close()
		return AudioLibraryAsset{}, err
	}
	_, copyErr := io.Copy(tmp, in)
	_ = in.Close()
	_ = tmp.Close()
	if copyErr != nil {
		return AudioLibraryAsset{}, copyErr
	}
	if detected, detectErr := mimetype.DetectFile(tmpPath); detectErr == nil && audioSvc != nil && len(audioSvc.allowedMimes) > 0 {
		if _, allowed := audioSvc.allowedMimes[strings.ToLower(detected.String())]; !allowed {
			return AudioLibraryAsset{}, ErrAudioUnsupportedMime
		}
	}
	contentType := strings.TrimSpace(file.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(name))
	}
	if _, err := store.Put(contextBackground(), audiolibrarybackend.UploadInput{ObjectKey: key, LocalPath: tmpPath, ContentType: contentType}); err != nil {
		return AudioLibraryAsset{}, err
	}
	return ResolveAudioLibraryAssetsForKeys([]string{key}, cfg, store)
}

func ResolveAudioLibraryAssetsForKeys(keys []string, cfg audioLibraryRuntimeConfig, store audiolibrarybackend.Backend) (AudioLibraryAsset, error) {
	if len(keys) == 0 {
		return AudioLibraryAsset{}, errors.New("S3 object key 为空")
	}
	object, err := store.Stat(contextBackground(), keys[0])
	if err != nil {
		return AudioLibraryAsset{}, err
	}
	ref, err := BuildS3AudioRef(cfg.S3SourceID, object.Key)
	if err != nil {
		return AudioLibraryAsset{}, err
	}
	name := filepath.Base(object.Key)
	return AudioLibraryAsset{Ref: ref, Name: name, Key: object.Key, ParentPrefix: strings.TrimSuffix(strings.TrimSuffix(object.Key, name), "/"), Size: object.Size, LastModified: object.LastModified, ETag: object.ETag, StorageClass: object.StorageClass, Extension: strings.ToLower(filepath.Ext(name)), ContentType: object.ContentType}, nil
}

func isWithinAudioPrefix(base, key string) bool {
	normalizedBase, err := NormalizeAudioLibraryPrefix(base)
	if err != nil {
		return false
	}
	base = normalizedBase
	key = strings.TrimLeft(strings.ReplaceAll(key, "\\", "/"), "/")
	if key == "" || strings.Contains(strings.ToLower(key), "%2e") || strings.Contains(strings.ToLower(key), "%00") {
		return false
	}
	for _, part := range strings.Split(key, "/") {
		if part == "." || part == ".." {
			return false
		}
		for _, r := range part {
			if unicode.IsControl(r) {
				return false
			}
		}
	}
	return base == "" || key == strings.TrimSuffix(base, "/") || strings.HasPrefix(key, base)
}

func isLikelyAudioKey(key string) bool {
	ext := strings.ToLower(filepath.Ext(key))
	switch ext {
	case ".mp3", ".ogg", ".oga", ".wav", ".webm", ".aac", ".flac", ".m4a", ".mp4", ".opus", ".aif", ".aiff", ".mka", ".mid", ".midi":
		return true
	default:
		return false
	}
}

type AudioLibraryReferenceUsage struct {
	SceneRefCount         int      `json:"sceneRefCount"`
	PlaybackStateRefCount int      `json:"playbackStateRefCount"`
	SceneNames            []string `json:"sceneNames,omitempty"`
	Referenced            bool     `json:"referenced"`
}

type AudioLibraryConflictError struct{ Message string }

func (e *AudioLibraryConflictError) Error() string { return e.Message }

func AudioLibraryReferenceUsageForRef(ref string) (AudioLibraryReferenceUsage, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return AudioLibraryReferenceUsage{}, errors.New("audio ref 为空")
	}
	var scenes []*model.AudioScene
	if err := model.GetDB().Find(&scenes).Error; err != nil {
		return AudioLibraryReferenceUsage{}, err
	}
	usage := AudioLibraryReferenceUsage{}
	for _, scene := range scenes {
		if scene == nil || !tracksContainAudioRef(scene.Tracks, ref) {
			continue
		}
		usage.SceneRefCount++
		usage.SceneNames = append(usage.SceneNames, scene.Name)
	}
	var states []*model.AudioPlaybackState
	if err := model.GetDB().Find(&states).Error; err != nil {
		return AudioLibraryReferenceUsage{}, err
	}
	for _, state := range states {
		if state != nil && tracksContainAudioRef(state.Tracks, ref) {
			usage.PlaybackStateRefCount++
		}
	}
	usage.Referenced = usage.SceneRefCount > 0 || usage.PlaybackStateRefCount > 0
	return usage, nil
}

func tracksContainAudioRef[T interface {
	~[]model.AudioSceneTrack | ~[]model.AudioTrackState
}](tracks T, ref string) bool {
	switch values := any(tracks).(type) {
	case model.JSONList[model.AudioSceneTrack]:
		for _, track := range values {
			if trackRefMatches(track.AssetID, track.PlaylistAssetIDs, ref) || (track.PlaylistFolderID != nil && strings.TrimSpace(*track.PlaylistFolderID) == ref) {
				return true
			}
		}
	case model.JSONList[model.AudioTrackState]:
		for _, track := range values {
			if trackRefMatches(track.AssetID, track.PlaylistAssetIDs, ref) || (track.PlaylistFolderID != nil && strings.TrimSpace(*track.PlaylistFolderID) == ref) {
				return true
			}
		}
	}
	return false
}

func trackRefMatches(assetID *string, playlist []string, ref string) bool {
	if assetID != nil && strings.TrimSpace(*assetID) == ref {
		return true
	}
	for _, item := range playlist {
		if strings.TrimSpace(item) == ref {
			return true
		}
	}
	return false
}

func loadReferencedAudioRefs() (map[string]struct{}, error) {
	refs := make(map[string]struct{})
	var scenes []*model.AudioScene
	if err := model.GetDB().Find(&scenes).Error; err != nil {
		return nil, err
	}
	for _, scene := range scenes {
		if scene == nil {
			continue
		}
		for _, track := range scene.Tracks {
			if track.AssetID != nil && strings.TrimSpace(*track.AssetID) != "" {
				refs[strings.TrimSpace(*track.AssetID)] = struct{}{}
			}
			if track.PlaylistFolderID != nil && strings.TrimSpace(*track.PlaylistFolderID) != "" {
				refs[strings.TrimSpace(*track.PlaylistFolderID)] = struct{}{}
			}
			for _, assetRef := range track.PlaylistAssetIDs {
				if normalized := strings.TrimSpace(assetRef); normalized != "" {
					refs[normalized] = struct{}{}
				}
			}
		}
	}
	var states []*model.AudioPlaybackState
	if err := model.GetDB().Find(&states).Error; err != nil {
		return nil, err
	}
	for _, state := range states {
		if state == nil {
			continue
		}
		for _, track := range state.Tracks {
			if track.AssetID != nil && strings.TrimSpace(*track.AssetID) != "" {
				refs[strings.TrimSpace(*track.AssetID)] = struct{}{}
			}
			if track.PlaylistFolderID != nil && strings.TrimSpace(*track.PlaylistFolderID) != "" {
				refs[strings.TrimSpace(*track.PlaylistFolderID)] = struct{}{}
			}
			for _, assetRef := range track.PlaylistAssetIDs {
				if normalized := strings.TrimSpace(assetRef); normalized != "" {
					refs[normalized] = struct{}{}
				}
			}
		}
	}
	return refs, nil
}

func audioLibraryPrefixHasReferences(prefix string, cfg audioLibraryRuntimeConfig, store audiolibrarybackend.Backend) (bool, error) {
	referenced, err := loadReferencedAudioRefs()
	if err != nil {
		return false, err
	}
	folderRef, err := BuildS3AudioPrefixRef(cfg.S3SourceID, prefix)
	if err == nil {
		if _, ok := referenced[folderRef]; ok {
			return true, nil
		}
	}
	queue := []string{prefix}
	visited := make(map[string]struct{})
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, ok := visited[current]; ok {
			continue
		}
		visited[current] = struct{}{}
		cursor := ""
		for {
			result, listErr := store.List(contextBackground(), current, cursor, 1000)
			if listErr != nil {
				return false, listErr
			}
			for _, child := range result.Prefixes {
				if isWithinAudioPrefix(prefix, child) {
					queue = append(queue, child)
				}
			}
			for _, object := range result.Objects {
				if strings.HasSuffix(object.Key, "/") || !isLikelyAudioKey(object.Key) {
					continue
				}
				assetRef, refErr := BuildS3AudioRef(cfg.S3SourceID, object.Key)
				if refErr == nil {
					if _, ok := referenced[assetRef]; ok {
						return true, nil
					}
				}
			}
			if !result.IsTruncated || strings.TrimSpace(result.NextCursor) == "" {
				break
			}
			cursor = result.NextCursor
		}
	}
	return false, nil
}

func replaceAudioLibraryRefsTx(tx *gorm.DB, oldRef, newRef string) error {
	var scenes []*model.AudioScene
	if err := tx.Find(&scenes).Error; err != nil {
		return err
	}
	for _, scene := range scenes {
		changed := false
		for i := range scene.Tracks {
			track := &scene.Tracks[i]
			if track.AssetID != nil && strings.TrimSpace(*track.AssetID) == oldRef {
				if newRef == "" {
					track.AssetID = nil
				} else {
					value := newRef
					track.AssetID = &value
				}
				changed = true
			}
			filtered := track.PlaylistAssetIDs[:0]
			for _, item := range track.PlaylistAssetIDs {
				if item != oldRef {
					filtered = append(filtered, item)
				} else if newRef != "" {
					filtered = append(filtered, newRef)
				}
			}
			if len(filtered) != len(track.PlaylistAssetIDs) || changed {
				track.PlaylistAssetIDs = filtered
				changed = true
			}
		}
		if changed {
			if err := tx.Model(scene).Update("tracks", scene.Tracks).Error; err != nil {
				return err
			}
		}
	}
	var states []*model.AudioPlaybackState
	if err := tx.Find(&states).Error; err != nil {
		return err
	}
	for _, state := range states {
		changed := false
		for i := range state.Tracks {
			track := &state.Tracks[i]
			if track.AssetID != nil && strings.TrimSpace(*track.AssetID) == oldRef {
				if newRef == "" {
					track.AssetID = nil
				} else {
					value := newRef
					track.AssetID = &value
				}
				changed = true
			}
			filtered := track.PlaylistAssetIDs[:0]
			for _, item := range track.PlaylistAssetIDs {
				if item != oldRef {
					filtered = append(filtered, item)
				} else if newRef != "" {
					filtered = append(filtered, newRef)
				}
			}
			if len(filtered) != len(track.PlaylistAssetIDs) || changed {
				track.PlaylistAssetIDs = filtered
				changed = true
			}
		}
		if changed {
			if err := tx.Model(state).Update("tracks", state.Tracks).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func AudioLibraryMoveAsset(worldID, ref, targetPrefix, name, expectedETag string) (AudioLibraryAsset, error) {
	parsed, err := ParseAudioRef(ref)
	if err != nil {
		return AudioLibraryAsset{}, err
	}
	cfg, store, err := audioLibraryConfig(worldID)
	if err != nil {
		return AudioLibraryAsset{}, err
	}
	if err := ensureAudioLibraryS3Mode(cfg); err != nil {
		return AudioLibraryAsset{}, err
	}
	if parsed.Source != AudioRefSourceS3 || parsed.IsPrefix || parsed.SourceID != cfg.S3SourceID || !isWithinAudioPrefix(cfg.S3Prefix, parsed.ObjectKey) || !isLikelyAudioKey(parsed.ObjectKey) {
		return AudioLibraryAsset{}, errors.New("audio ref 不属于当前 S3 source")
	}
	if strings.TrimSpace(expectedETag) != "" {
		current, statErr := store.Stat(contextBackground(), parsed.ObjectKey)
		if statErr != nil {
			return AudioLibraryAsset{}, statErr
		}
		if strings.Trim(current.ETag, "\"") != strings.Trim(strings.TrimSpace(expectedETag), "\"") {
			return AudioLibraryAsset{}, &AudioLibraryConflictError{Message: "S3 对象已被外部修改"}
		}
	}
	if strings.TrimSpace(targetPrefix) == "" {
		// Rename without an explicit destination keeps object in its current
		// folder; callers can pass configured root explicitly to move there.
		targetPrefix = strings.TrimSuffix(parsed.ObjectKey, filepath.Base(parsed.ObjectKey))
	}
	prefix, _, _, err := resolveAudioLibraryPrefix(cfg.WorldID, targetPrefix)
	if err != nil {
		return AudioLibraryAsset{}, err
	}
	if name == "" {
		name = filepath.Base(parsed.ObjectKey)
	}
	destination, err := JoinS3LibraryPrefix(prefix, filepath.Base(name))
	if err != nil {
		return AudioLibraryAsset{}, err
	}
	if destination == parsed.ObjectKey {
		return ResolveAudioLibraryAssetsForKeys([]string{destination}, cfg, store)
	}
	if _, err := store.Copy(contextBackground(), parsed.ObjectKey, destination, expectedETag); err != nil {
		if errors.Is(err, audiolibrarybackend.ErrETagMismatch) {
			return AudioLibraryAsset{}, &AudioLibraryConflictError{Message: err.Error()}
		}
		return AudioLibraryAsset{}, err
	}
	newRef, err := BuildS3AudioRef(cfg.S3SourceID, destination)
	if err != nil {
		_ = store.DeleteObjects(contextBackground(), []string{destination})
		return AudioLibraryAsset{}, err
	}
	if err := model.GetDB().Transaction(func(tx *gorm.DB) error { return replaceAudioLibraryRefsTx(tx, ref, newRef) }); err != nil {
		_ = store.DeleteObjects(contextBackground(), []string{destination})
		return AudioLibraryAsset{}, err
	}
	if err := store.DeleteObjects(contextBackground(), []string{parsed.ObjectKey}); err != nil {
		_ = model.GetDB().Transaction(func(tx *gorm.DB) error { return replaceAudioLibraryRefsTx(tx, newRef, ref) })
		_ = store.DeleteObjects(contextBackground(), []string{destination})
		return AudioLibraryAsset{}, err
	}
	return ResolveAudioLibraryAssetsForKeys([]string{destination}, cfg, store)
}

func AudioLibraryUpdateContentType(worldID, ref, contentType, expectedETag string) (AudioLibraryAsset, error) {
	parsed, err := ParseAudioRef(ref)
	if err != nil {
		return AudioLibraryAsset{}, err
	}
	cfg, store, err := audioLibraryConfig(worldID)
	if err != nil {
		return AudioLibraryAsset{}, err
	}
	if err := ensureAudioLibraryS3Mode(cfg); err != nil {
		return AudioLibraryAsset{}, err
	}
	if parsed.Source != AudioRefSourceS3 || parsed.IsPrefix || parsed.SourceID != cfg.S3SourceID || !isWithinAudioPrefix(cfg.S3Prefix, parsed.ObjectKey) || !isLikelyAudioKey(parsed.ObjectKey) {
		return AudioLibraryAsset{}, errors.New("audio ref 不属于当前 S3 source")
	}
	if _, err := store.UpdateContentType(contextBackground(), parsed.ObjectKey, contentType, expectedETag); err != nil {
		if errors.Is(err, audiolibrarybackend.ErrETagMismatch) {
			return AudioLibraryAsset{}, &AudioLibraryConflictError{Message: err.Error()}
		}
		if strings.Contains(err.Error(), "外部修改") {
			return AudioLibraryAsset{}, &AudioLibraryConflictError{Message: err.Error()}
		}
		return AudioLibraryAsset{}, err
	}
	return ResolveAudioLibraryAssetsForKeys([]string{parsed.ObjectKey}, cfg, store)
}

func AudioLibraryDeleteAsset(worldID, ref, expectedETag string, forceDetach bool) error {
	parsed, err := ParseAudioRef(ref)
	if err != nil {
		return err
	}
	cfg, store, err := audioLibraryConfig(worldID)
	if err != nil {
		return err
	}
	if err := ensureAudioLibraryS3Mode(cfg); err != nil {
		return err
	}
	if parsed.Source != AudioRefSourceS3 || parsed.IsPrefix || parsed.SourceID != cfg.S3SourceID || !isWithinAudioPrefix(cfg.S3Prefix, parsed.ObjectKey) || !isLikelyAudioKey(parsed.ObjectKey) {
		return errors.New("audio ref 不属于当前 S3 source")
	}
	if strings.TrimSpace(expectedETag) != "" {
		current, statErr := store.Stat(contextBackground(), parsed.ObjectKey)
		if statErr != nil {
			return statErr
		}
		if strings.Trim(current.ETag, "\"") != strings.Trim(strings.TrimSpace(expectedETag), "\"") {
			return &AudioLibraryConflictError{Message: "S3 对象已被外部修改"}
		}
	}
	usage, err := AudioLibraryReferenceUsageForRef(ref)
	if err != nil {
		return err
	}
	if usage.Referenced && !forceDetach {
		return &AudioLibraryConflictError{Message: "audio asset is still referenced"}
	}
	if forceDetach {
		if err := model.GetDB().Transaction(func(tx *gorm.DB) error { return replaceAudioLibraryRefsTx(tx, ref, "") }); err != nil {
			return err
		}
	}
	return store.DeleteObjects(contextBackground(), []string{parsed.ObjectKey})
}

func AudioLibraryCreateFolder(worldID, rawPrefix string) (AudioLibraryPrefix, error) {
	prefix, cfg, store, err := resolveAudioLibraryPrefix(worldID, rawPrefix)
	if err != nil {
		return AudioLibraryPrefix{}, err
	}
	if err := ensureAudioLibraryS3Mode(cfg); err != nil {
		return AudioLibraryPrefix{}, err
	}
	if prefix == "" || !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	marker := prefix + ".sealchat-folder"
	if _, err := store.PutBytes(contextBackground(), marker, "application/octet-stream", nil); err != nil {
		return AudioLibraryPrefix{}, err
	}
	ref, _ := BuildS3AudioPrefixRef(cfg.S3SourceID, prefix)
	return AudioLibraryPrefix{Ref: ref, Name: strings.TrimSuffix(filepath.Base(strings.TrimSuffix(prefix, "/")), "/"), Prefix: prefix}, nil
}

func AudioLibraryDeleteFolder(worldID, rawPrefix string) error {
	prefix, cfg, store, err := resolveAudioLibraryPrefix(worldID, rawPrefix)
	if err != nil {
		return err
	}
	if err := ensureAudioLibraryS3Mode(cfg); err != nil {
		return err
	}
	if prefix == "" || prefix == cfg.S3Prefix {
		return errors.New("禁止删除音频素材库根目录")
	}
	referenced, err := audioLibraryPrefixHasReferences(prefix, cfg, store)
	if err != nil {
		return err
	}
	if referenced {
		return &AudioLibraryConflictError{Message: "audio folder is still referenced"}
	}
	return store.DeletePrefix(contextBackground(), prefix)
}

func AudioLibraryPlayToken(worldID, userID, ref string) (*AudioPlayTokenGrant, string, error) {
	parsed, err := ParseAudioRef(ref)
	if err != nil {
		return nil, "", err
	}
	cfg, store, err := audioLibraryConfig(worldID)
	if err != nil {
		return nil, "", err
	}
	if err := ensureAudioLibraryS3Mode(cfg); err != nil {
		return nil, "", err
	}
	if parsed.Source != AudioRefSourceS3 || parsed.IsPrefix || parsed.SourceID != cfg.S3SourceID || !isWithinAudioPrefix(cfg.S3Prefix, parsed.ObjectKey) || !isLikelyAudioKey(parsed.ObjectKey) {
		return nil, "", errors.New("audio ref 不属于当前 S3 source")
	}
	if _, err := store.Stat(contextBackground(), parsed.ObjectKey); err != nil {
		return nil, "", err
	}
	grant, err := IssueAudioPlayToken(userID, ref)
	if err != nil {
		return nil, "", err
	}
	url, err := store.PresignedGetURL(contextBackground(), parsed.ObjectKey)
	if err != nil {
		return nil, "", err
	}
	return grant, url, nil
}
