package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/utils"
)

const (
	audioS3AssetRefPrefix  = "s3a:"
	audioS3FolderRefPrefix = "s3f:"
)

var (
	ErrAudioS3Unavailable   = errors.New("S3 音频素材库不可用")
	ErrAudioS3InvalidPath   = errors.New("S3 路径无效")
	ErrAudioS3Exists        = errors.New("S3 对象已存在")
	ErrAudioS3NotFound      = errors.New("S3 对象不存在")
	ErrAudioS3ForeignObject = errors.New("S3 文件夹包含非音频对象")
)

var (
	audioS3SettingsMu     sync.RWMutex
	audioS3SettingsLoaded bool
	audioS3SettingsCache  audioS3LibrarySettingsFile
)

type AudioS3LibrarySettings struct {
	Enabled   bool   `json:"enabled"`
	Prefix    string `json:"prefix"`
	Available bool   `json:"available"`
	Bucket    string `json:"bucket"`
}

type audioS3LibrarySettingsFile struct {
	Enabled bool   `json:"enabled"`
	Prefix  string `json:"prefix"`
}

type AudioS3BrowsePrefix struct {
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
}

type AudioS3BrowseResult struct {
	Current  string                `json:"current"`
	Parent   string                `json:"parent"`
	Prefixes []AudioS3BrowsePrefix `json:"prefixes"`
}

type AudioS3Asset struct {
	ID            string                     `json:"id"`
	Name          string                     `json:"name"`
	FolderID      *string                    `json:"folderId"`
	Size          int64                      `json:"size"`
	Duration      float64                    `json:"duration"`
	Bitrate       int                        `json:"bitrate"`
	StorageType   string                     `json:"storageType"`
	ObjectKey     string                     `json:"objectKey"`
	Description   string                     `json:"description,omitempty"`
	Tags          []string                   `json:"tags"`
	Visibility    model.AudioAssetVisibility `json:"visibility"`
	CreatedBy     string                     `json:"createdBy"`
	UpdatedBy     string                     `json:"updatedBy"`
	CreatedAt     time.Time                  `json:"createdAt"`
	UpdatedAt     time.Time                  `json:"updatedAt"`
	Scope         model.AudioAssetScope      `json:"scope"`
	WorldID       *string                    `json:"worldId"`
	Source        string                     `json:"source"`
	ETag          string                     `json:"etag,omitempty"`
	ContentType   string                     `json:"contentType,omitempty"`
	LastAccessed  *time.Time                 `json:"lastAccessedAt,omitempty"`
	AccessCount   int64                      `json:"accessCount,omitempty"`
	SortOrder     int                        `json:"sortOrder,omitempty"`
	ManualSorted  bool                       `json:"manualSorted,omitempty"`
	TranscodeStat string                     `json:"transcodeStatus,omitempty"`
}

type AudioS3Folder struct {
	ID        string                `json:"id"`
	ParentID  *string               `json:"parentId"`
	Name      string                `json:"name"`
	Path      string                `json:"path"`
	Prefix    string                `json:"prefix"`
	Children  []*AudioS3Folder      `json:"children,omitempty"`
	Scope     model.AudioAssetScope `json:"scope"`
	WorldID   *string               `json:"worldId"`
	CreatedBy string                `json:"createdBy,omitempty"`
	UpdatedBy string                `json:"updatedBy,omitempty"`
	CreatedAt *time.Time            `json:"createdAt,omitempty"`
	UpdatedAt *time.Time            `json:"updatedAt,omitempty"`
	Source    string                `json:"source"`
}

type AudioS3AssetListResult struct {
	Items    []*AudioS3Asset `json:"items"`
	Page     int             `json:"page"`
	PageSize int             `json:"pageSize"`
	Total    int             `json:"total"`
}

type AudioS3AssetUpdateInput struct {
	Name     string
	FolderID *string
}

type AudioS3FolderUpdateInput struct {
	Name     string
	ParentID *string
}

func audioS3SettingsPath() string {
	return filepath.Join("data", "audio-s3-library.json")
}

func NormalizeAudioS3Prefix(raw string) (string, error) {
	trimmed := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return "", nil
	}
	if strings.ContainsRune(trimmed, '\x00') {
		return "", ErrAudioS3InvalidPath
	}
	for _, part := range strings.Split(trimmed, "/") {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			return "", ErrAudioS3InvalidPath
		}
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", ErrAudioS3InvalidPath
	}
	return strings.Trim(cleaned, "/") + "/", nil
}

func normalizeAudioS3ObjectKey(raw string) (string, error) {
	trimmed := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	trimmed = strings.TrimLeft(trimmed, "/")
	if trimmed == "" || strings.HasSuffix(trimmed, "/") || strings.ContainsRune(trimmed, '\x00') {
		return "", ErrAudioS3InvalidPath
	}
	for _, part := range strings.Split(trimmed, "/") {
		if strings.TrimSpace(part) == "" || part == "." || part == ".." {
			return "", ErrAudioS3InvalidPath
		}
	}
	cleaned := path.Clean(trimmed)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", ErrAudioS3InvalidPath
	}
	return cleaned, nil
}

func readAudioS3SettingsFile() audioS3LibrarySettingsFile {
	settings := audioS3LibrarySettingsFile{}
	payload, err := os.ReadFile(audioS3SettingsPath())
	if err != nil {
		return settings
	}
	if err := json.Unmarshal(payload, &settings); err != nil {
		return audioS3LibrarySettingsFile{}
	}
	prefix, err := NormalizeAudioS3Prefix(settings.Prefix)
	if err != nil {
		settings.Prefix = ""
	} else {
		settings.Prefix = prefix
	}
	return settings
}

func GetAudioS3LibrarySettings() AudioS3LibrarySettings {
	audioS3SettingsMu.RLock()
	if audioS3SettingsLoaded {
		settings := audioS3SettingsCache
		audioS3SettingsMu.RUnlock()
		return buildAudioS3LibrarySettings(settings)
	}
	audioS3SettingsMu.RUnlock()

	audioS3SettingsMu.Lock()
	if !audioS3SettingsLoaded {
		audioS3SettingsCache = readAudioS3SettingsFile()
		audioS3SettingsLoaded = true
	}
	settings := audioS3SettingsCache
	audioS3SettingsMu.Unlock()
	return buildAudioS3LibrarySettings(settings)
}

func buildAudioS3LibrarySettings(settings audioS3LibrarySettingsFile) AudioS3LibrarySettings {
	result := AudioS3LibrarySettings{
		Enabled: settings.Enabled,
		Prefix:  settings.Prefix,
	}
	if cfg := utils.GetConfig(); cfg != nil {
		result.Bucket = strings.TrimSpace(cfg.Storage.S3.Bucket)
		result.Available = cfg.Storage.S3.Enabled &&
			strings.TrimSpace(cfg.Storage.S3.Endpoint) != "" &&
			result.Bucket != ""
	}
	if !result.Available {
		result.Enabled = false
	}
	return result
}

func SaveAudioS3LibrarySettings(enabled bool, rawPrefix string) (AudioS3LibrarySettings, error) {
	prefix, err := NormalizeAudioS3Prefix(rawPrefix)
	if err != nil {
		return AudioS3LibrarySettings{}, err
	}
	current := GetAudioS3LibrarySettings()
	if enabled && !current.Available {
		return AudioS3LibrarySettings{}, ErrAudioS3Unavailable
	}
	if enabled {
		if _, _, err := newAudioS3Client(); err != nil {
			return AudioS3LibrarySettings{}, err
		}
	}

	audioS3SettingsMu.Lock()
	defer audioS3SettingsMu.Unlock()
	filePath := audioS3SettingsPath()
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return AudioS3LibrarySettings{}, err
	}
	payload, err := json.MarshalIndent(audioS3LibrarySettingsFile{Enabled: enabled, Prefix: prefix}, "", "  ")
	if err != nil {
		return AudioS3LibrarySettings{}, err
	}
	tempPath := filePath + ".tmp"
	if err := os.WriteFile(tempPath, payload, 0o600); err != nil {
		return AudioS3LibrarySettings{}, err
	}
	if err := os.Rename(tempPath, filePath); err != nil {
		_ = os.Remove(tempPath)
		return AudioS3LibrarySettings{}, err
	}
	audioS3SettingsCache = audioS3LibrarySettingsFile{Enabled: enabled, Prefix: prefix}
	audioS3SettingsLoaded = true
	result := current
	result.Enabled = enabled
	result.Prefix = prefix
	return result, nil
}

func newAudioS3Client() (*minio.Client, utils.S3StorageConfig, error) {
	cfg := utils.GetConfig()
	if cfg == nil || !cfg.Storage.S3.Enabled {
		return nil, utils.S3StorageConfig{}, ErrAudioS3Unavailable
	}
	s3cfg := cfg.Storage.S3
	endpoint := strings.TrimSpace(s3cfg.Endpoint)
	bucket := strings.TrimSpace(s3cfg.Bucket)
	if endpoint == "" || bucket == "" {
		return nil, s3cfg, ErrAudioS3Unavailable
	}
	secure := s3cfg.UseSSL
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Host == "" {
			return nil, s3cfg, fmt.Errorf("%w: endpoint", ErrAudioS3InvalidPath)
		}
		secure = parsed.Scheme == "https"
		endpoint = parsed.Host
	}
	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(s3cfg.AccessKey, s3cfg.SecretKey, s3cfg.SessionToken),
		Secure: secure,
		Region: strings.TrimSpace(s3cfg.Region),
	}
	if s3cfg.ForcePathStyle {
		opts.BucketLookup = minio.BucketLookupPath
	}
	client, err := minio.New(endpoint, opts)
	if err != nil {
		return nil, s3cfg, err
	}
	return client, s3cfg, nil
}

func EncodeAudioS3AssetRef(objectKey string) string {
	return audioS3AssetRefPrefix + base64.RawURLEncoding.EncodeToString([]byte(objectKey))
}

func DecodeAudioS3AssetRef(ref string) (string, error) {
	if !strings.HasPrefix(ref, audioS3AssetRefPrefix) {
		return "", ErrAudioS3InvalidPath
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(ref, audioS3AssetRefPrefix))
	if err != nil {
		return "", ErrAudioS3InvalidPath
	}
	return normalizeAudioS3ObjectKey(string(decoded))
}

func EncodeAudioS3FolderRef(prefix string) string {
	return audioS3FolderRefPrefix + base64.RawURLEncoding.EncodeToString([]byte(prefix))
}

func DecodeAudioS3FolderRef(ref string) (string, error) {
	if !strings.HasPrefix(ref, audioS3FolderRefPrefix) {
		return "", ErrAudioS3InvalidPath
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(ref, audioS3FolderRefPrefix))
	if err != nil {
		return "", ErrAudioS3InvalidPath
	}
	return NormalizeAudioS3Prefix(string(decoded))
}

func audioS3RootPrefix() string {
	return GetAudioS3LibrarySettings().Prefix
}

func ensureAudioS3PrefixAllowed(prefix string) error {
	root := audioS3RootPrefix()
	if root == "" {
		return nil
	}
	if prefix == root || strings.HasPrefix(prefix, root) {
		return nil
	}
	return ErrAudioS3InvalidPath
}

func ensureAudioS3KeyAllowed(key string) error {
	root := audioS3RootPrefix()
	if root == "" || strings.HasPrefix(key, root) {
		return nil
	}
	return ErrAudioS3InvalidPath
}

func parentAudioS3Prefix(key string) string {
	dir := path.Dir(strings.TrimSuffix(key, "/"))
	if dir == "." || dir == "/" || dir == "" {
		return ""
	}
	return strings.Trim(dir, "/") + "/"
}

func audioS3FolderIDForParent(prefix string) *string {
	root := audioS3RootPrefix()
	if prefix == "" || prefix == root {
		return nil
	}
	id := EncodeAudioS3FolderRef(prefix)
	return &id
}

func audioS3ContentType(objectKey, fallback string) string {
	if value := strings.TrimSpace(fallback); value != "" && value != "application/octet-stream" {
		return value
	}
	if value := mime.TypeByExtension(strings.ToLower(path.Ext(objectKey))); value != "" {
		return value
	}
	return "application/octet-stream"
}

func isAudioS3Object(objectKey string) bool {
	switch strings.ToLower(path.Ext(objectKey)) {
	case ".mp3", ".ogg", ".oga", ".opus", ".wav", ".flac", ".m4a", ".aac", ".webm", ".mp4":
		return true
	default:
		return false
	}
}

func audioS3AssetFromObject(info minio.ObjectInfo) *AudioS3Asset {
	updatedAt := info.LastModified
	if updatedAt.IsZero() {
		updatedAt = time.Unix(0, 0).UTC()
	}
	return &AudioS3Asset{
		ID:            EncodeAudioS3AssetRef(info.Key),
		Name:          path.Base(info.Key),
		FolderID:      audioS3FolderIDForParent(parentAudioS3Prefix(info.Key)),
		Size:          info.Size,
		Duration:      0,
		Bitrate:       0,
		StorageType:   "s3",
		ObjectKey:     info.Key,
		Description:   info.Key,
		Tags:          []string{},
		Visibility:    model.AudioVisibilityPublic,
		CreatedBy:     "S3",
		UpdatedBy:     "S3",
		CreatedAt:     updatedAt,
		UpdatedAt:     updatedAt,
		Scope:         model.AudioScopeCommon,
		WorldID:       nil,
		Source:        "s3",
		ETag:          strings.Trim(info.ETag, "\""),
		ContentType:   audioS3ContentType(info.Key, info.ContentType),
		TranscodeStat: "ready",
	}
}

func listAudioS3Objects(ctx context.Context, prefix string) ([]minio.ObjectInfo, error) {
	client, cfg, err := newAudioS3Client()
	if err != nil {
		return nil, err
	}
	items := make([]minio.ObjectInfo, 0)
	for object := range client.ListObjects(ctx, cfg.Bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if object.Err != nil {
			return nil, object.Err
		}
		items = append(items, object)
	}
	return items, nil
}

func AudioS3Browse(ctx context.Context, rawPrefix string) (*AudioS3BrowseResult, error) {
	prefix, err := NormalizeAudioS3Prefix(rawPrefix)
	if err != nil {
		return nil, err
	}
	client, cfg, err := newAudioS3Client()
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	items := make([]AudioS3BrowsePrefix, 0)
	for object := range client.ListObjects(ctx, cfg.Bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: false,
	}) {
		if object.Err != nil {
			return nil, object.Err
		}
		key := object.Key
		if !strings.HasSuffix(key, "/") {
			remainder := strings.TrimPrefix(key, prefix)
			if slash := strings.Index(remainder, "/"); slash >= 0 {
				key = prefix + remainder[:slash+1]
			} else {
				continue
			}
		}
		child, err := NormalizeAudioS3Prefix(key)
		if err != nil || child == prefix {
			continue
		}
		if _, ok := seen[child]; ok {
			continue
		}
		seen[child] = struct{}{}
		items = append(items, AudioS3BrowsePrefix{
			Name:   path.Base(strings.TrimSuffix(child, "/")),
			Prefix: child,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	parent := parentAudioS3Prefix(prefix)
	return &AudioS3BrowseResult{Current: prefix, Parent: parent, Prefixes: items}, nil
}

func AudioS3ListAssets(ctx context.Context, folderID, query string, recursive bool, pageNumber, pageSize int, sortBy, sortOrder string) (*AudioS3AssetListResult, error) {
	prefix := audioS3RootPrefix()
	if strings.TrimSpace(folderID) != "" {
		decoded, err := DecodeAudioS3FolderRef(strings.TrimSpace(folderID))
		if err != nil {
			return nil, err
		}
		prefix = decoded
	}
	if err := ensureAudioS3PrefixAllowed(prefix); err != nil {
		return nil, err
	}
	objects, err := listAudioS3Objects(ctx, prefix)
	if err != nil {
		return nil, err
	}
	keyword := strings.ToLower(strings.TrimSpace(query))
	assets := make([]*AudioS3Asset, 0, len(objects))
	for _, object := range objects {
		if object.Key == "" || strings.HasSuffix(object.Key, "/") || !isAudioS3Object(object.Key) {
			continue
		}
		if !recursive && parentAudioS3Prefix(object.Key) != prefix {
			continue
		}
		if keyword != "" {
			name := strings.ToLower(path.Base(object.Key))
			key := strings.ToLower(object.Key)
			if !strings.Contains(name, keyword) && !strings.Contains(key, keyword) {
				continue
			}
		}
		assets = append(assets, audioS3AssetFromObject(object))
	}
	desc := strings.EqualFold(sortOrder, "desc")
	lessAsset := func(left, right *AudioS3Asset) bool {
		switch sortBy {
		case "updatedAt":
			return left.UpdatedAt.Before(right.UpdatedAt)
		case "size":
			return left.Size < right.Size
		default:
			return strings.ToLower(left.Name) < strings.ToLower(right.Name)
		}
	}
	sort.SliceStable(assets, func(i, j int) bool {
		if desc {
			return lessAsset(assets[j], assets[i])
		}
		return lessAsset(assets[i], assets[j])
	})
	if pageNumber <= 0 {
		pageNumber = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 500 {
		pageSize = 500
	}
	total := len(assets)
	start := (pageNumber - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return &AudioS3AssetListResult{
		Items:    assets[start:end],
		Page:     pageNumber,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

func addAudioS3FolderPrefix(prefixes map[string]struct{}, root, candidate string) {
	candidate, err := NormalizeAudioS3Prefix(candidate)
	if err != nil || candidate == "" || candidate == root {
		return
	}
	if root != "" && !strings.HasPrefix(candidate, root) {
		return
	}
	current := candidate
	for current != "" && current != root {
		prefixes[current] = struct{}{}
		parent := parentAudioS3Prefix(current)
		if parent == current {
			break
		}
		current = parent
	}
}

func AudioS3ListFolders(ctx context.Context) ([]*AudioS3Folder, error) {
	root := audioS3RootPrefix()
	objects, err := listAudioS3Objects(ctx, root)
	if err != nil {
		return nil, err
	}
	prefixes := map[string]struct{}{}
	for _, object := range objects {
		if strings.HasSuffix(object.Key, "/") {
			addAudioS3FolderPrefix(prefixes, root, object.Key)
			continue
		}
		addAudioS3FolderPrefix(prefixes, root, parentAudioS3Prefix(object.Key))
	}
	ordered := make([]string, 0, len(prefixes))
	for prefix := range prefixes {
		ordered = append(ordered, prefix)
	}
	sort.Slice(ordered, func(i, j int) bool {
		leftDepth := strings.Count(strings.TrimSuffix(ordered[i], "/"), "/")
		rightDepth := strings.Count(strings.TrimSuffix(ordered[j], "/"), "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return strings.ToLower(ordered[i]) < strings.ToLower(ordered[j])
	})
	byPrefix := make(map[string]*AudioS3Folder, len(ordered))
	roots := make([]*AudioS3Folder, 0)
	for _, prefix := range ordered {
		parent := parentAudioS3Prefix(prefix)
		var parentID *string
		if parent != "" && parent != root {
			value := EncodeAudioS3FolderRef(parent)
			parentID = &value
		}
		relative := strings.TrimPrefix(prefix, root)
		folder := &AudioS3Folder{
			ID:        EncodeAudioS3FolderRef(prefix),
			ParentID:  parentID,
			Name:      path.Base(strings.TrimSuffix(prefix, "/")),
			Path:      "/" + strings.TrimSuffix(relative, "/"),
			Prefix:    prefix,
			Scope:     model.AudioScopeCommon,
			WorldID:   nil,
			CreatedBy: "S3",
			UpdatedBy: "S3",
			Source:    "s3",
		}
		byPrefix[prefix] = folder
		if parent == "" || parent == root {
			roots = append(roots, folder)
		} else if parentFolder := byPrefix[parent]; parentFolder != nil {
			parentFolder.Children = append(parentFolder.Children, folder)
		} else {
			roots = append(roots, folder)
		}
	}
	var sortFolders func([]*AudioS3Folder)
	sortFolders = func(items []*AudioS3Folder) {
		sort.Slice(items, func(i, j int) bool {
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		})
		for _, item := range items {
			sortFolders(item.Children)
		}
	}
	sortFolders(roots)
	return roots, nil
}

func AudioS3GetAsset(ctx context.Context, ref string) (*AudioS3Asset, error) {
	key, err := DecodeAudioS3AssetRef(ref)
	if err != nil {
		return nil, err
	}
	if err := ensureAudioS3KeyAllowed(key); err != nil {
		return nil, err
	}
	client, cfg, err := newAudioS3Client()
	if err != nil {
		return nil, err
	}
	info, err := client.StatObject(ctx, cfg.Bucket, key, minio.StatObjectOptions{})
	if err != nil {
		response := minio.ToErrorResponse(err)
		if response.StatusCode == 404 || response.Code == "NoSuchKey" {
			return nil, ErrAudioS3NotFound
		}
		return nil, err
	}
	return audioS3AssetFromObject(info), nil
}

func AudioS3PresignedURL(ctx context.Context, ref string) (string, time.Time, error) {
	key, err := DecodeAudioS3AssetRef(ref)
	if err != nil {
		return "", time.Time{}, err
	}
	if err := ensureAudioS3KeyAllowed(key); err != nil {
		return "", time.Time{}, err
	}
	client, cfg, err := newAudioS3Client()
	if err != nil {
		return "", time.Time{}, err
	}
	ttl := cfg.PresignTTL
	if ttl <= 0 {
		if appCfg := utils.GetConfig(); appCfg != nil {
			ttl = appCfg.Storage.PresignTTL
		}
	}
	if ttl <= 0 {
		ttl = 900
	}
	duration := time.Duration(ttl) * time.Second
	target, err := client.PresignedGetObject(ctx, cfg.Bucket, key, duration, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	return target.String(), time.Now().Add(duration), nil
}

func resolveAudioS3FolderPrefix(folderID string) (string, error) {
	if strings.TrimSpace(folderID) == "" {
		return audioS3RootPrefix(), nil
	}
	prefix, err := DecodeAudioS3FolderRef(strings.TrimSpace(folderID))
	if err != nil {
		return "", err
	}
	if err := ensureAudioS3PrefixAllowed(prefix); err != nil {
		return "", err
	}
	return prefix, nil
}

func sanitizeAudioS3Name(raw string) (string, error) {
	trimmed := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if trimmed == "" || strings.Contains(trimmed, "/") || trimmed == "." || trimmed == ".." || strings.ContainsRune(trimmed, '\x00') {
		return "", ErrAudioS3InvalidPath
	}
	return trimmed, nil
}

func audioS3ObjectExists(ctx context.Context, client *minio.Client, bucket, key string) (bool, error) {
	_, err := client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}
	response := minio.ToErrorResponse(err)
	if response.StatusCode == 404 || response.Code == "NoSuchKey" || response.Code == "NotFound" {
		return false, nil
	}
	return false, err
}

func audioS3PrefixHasObjects(ctx context.Context, client *minio.Client, bucket, prefix string) (bool, error) {
	for object := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
		MaxKeys:   1,
	}) {
		if object.Err != nil {
			return false, object.Err
		}
		return true, nil
	}
	return false, nil
}

func AudioS3UploadAsset(ctx context.Context, fileHeader *multipart.FileHeader, folderID string) (*AudioS3Asset, error) {
	settings := GetAudioS3LibrarySettings()
	if !settings.Enabled {
		return nil, ErrAudioS3Unavailable
	}
	if fileHeader == nil {
		return nil, errors.New("未选择上传文件")
	}
	if cfg := utils.GetConfig(); cfg != nil && cfg.Audio.MaxUploadSizeMB > 0 {
		if fileHeader.Size > cfg.Audio.MaxUploadSizeMB*1024*1024 {
			return nil, ErrAudioTooLarge
		}
	}
	name, err := sanitizeAudioS3Name(filepath.Base(fileHeader.Filename))
	if err != nil {
		return nil, err
	}
	if !isAudioS3Object(name) {
		return nil, ErrAudioUnsupportedMime
	}
	prefix, err := resolveAudioS3FolderPrefix(folderID)
	if err != nil {
		return nil, err
	}
	key, err := normalizeAudioS3ObjectKey(path.Join(prefix, name))
	if err != nil {
		return nil, err
	}
	if err := ensureAudioS3KeyAllowed(key); err != nil {
		return nil, err
	}
	client, cfg, err := newAudioS3Client()
	if err != nil {
		return nil, err
	}
	exists, err := audioS3ObjectExists(ctx, client, cfg.Bucket, key)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrAudioS3Exists
	}
	file, err := fileHeader.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	contentType := audioS3ContentType(key, fileHeader.Header.Get("Content-Type"))
	if svc := GetAudioService(); svc != nil {
		detected, detectErr := svc.validateMime(file)
		if detectErr != nil {
			return nil, detectErr
		}
		contentType = detected
	}
	info, err := client.PutObject(ctx, cfg.Bucket, key, file, fileHeader.Size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return nil, err
	}
	return audioS3AssetFromObject(minio.ObjectInfo{
		Key:          key,
		Size:         info.Size,
		ETag:         info.ETag,
		LastModified: time.Now().UTC(),
		ContentType:  contentType,
	}), nil
}

func AudioS3CreateFolder(ctx context.Context, parentID, rawName string) (*AudioS3Folder, error) {
	name, err := sanitizeAudioS3Name(rawName)
	if err != nil {
		return nil, err
	}
	parent, err := resolveAudioS3FolderPrefix(parentID)
	if err != nil {
		return nil, err
	}
	prefix, err := NormalizeAudioS3Prefix(path.Join(parent, name))
	if err != nil {
		return nil, err
	}
	if err := ensureAudioS3PrefixAllowed(prefix); err != nil {
		return nil, err
	}
	client, cfg, err := newAudioS3Client()
	if err != nil {
		return nil, err
	}
	exists, err := audioS3PrefixHasObjects(ctx, client, cfg.Bucket, prefix)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrAudioS3Exists
	}
	if _, err := client.PutObject(ctx, cfg.Bucket, prefix, bytes.NewReader(nil), 0, minio.PutObjectOptions{
		ContentType: "application/x-directory",
	}); err != nil {
		return nil, err
	}
	return audioS3FolderFromPrefix(prefix), nil
}

func audioS3FolderFromPrefix(prefix string) *AudioS3Folder {
	root := audioS3RootPrefix()
	parent := parentAudioS3Prefix(prefix)
	var parentID *string
	if parent != "" && parent != root {
		value := EncodeAudioS3FolderRef(parent)
		parentID = &value
	}
	relative := strings.TrimPrefix(prefix, root)
	return &AudioS3Folder{
		ID:        EncodeAudioS3FolderRef(prefix),
		ParentID:  parentID,
		Name:      path.Base(strings.TrimSuffix(prefix, "/")),
		Path:      "/" + strings.TrimSuffix(relative, "/"),
		Prefix:    prefix,
		Scope:     model.AudioScopeCommon,
		WorldID:   nil,
		CreatedBy: "S3",
		UpdatedBy: "S3",
		Source:    "s3",
	}
}

func copyAudioS3Object(ctx context.Context, client *minio.Client, bucket, sourceKey, targetKey string) error {
	_, err := client.CopyObject(ctx,
		minio.CopyDestOptions{Bucket: bucket, Object: targetKey},
		minio.CopySrcOptions{Bucket: bucket, Object: sourceKey},
	)
	return err
}

func removeAudioS3Object(ctx context.Context, client *minio.Client, bucket, key string) error {
	err := client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
	if err == nil {
		return nil
	}
	response := minio.ToErrorResponse(err)
	if response.StatusCode == 404 || response.Code == "NoSuchKey" || response.Code == "NotFound" {
		return nil
	}
	return err
}

func AudioS3UpdateAsset(ctx context.Context, ref string, input AudioS3AssetUpdateInput) (*AudioS3Asset, error) {
	oldKey, err := DecodeAudioS3AssetRef(ref)
	if err != nil {
		return nil, err
	}
	if err := ensureAudioS3KeyAllowed(oldKey); err != nil {
		return nil, err
	}
	name := path.Base(oldKey)
	if strings.TrimSpace(input.Name) != "" {
		name, err = sanitizeAudioS3Name(input.Name)
		if err != nil {
			return nil, err
		}
	}
	if !isAudioS3Object(name) {
		return nil, ErrAudioUnsupportedMime
	}
	prefix := parentAudioS3Prefix(oldKey)
	if input.FolderID != nil {
		prefix, err = resolveAudioS3FolderPrefix(strings.TrimSpace(*input.FolderID))
		if err != nil {
			return nil, err
		}
	}
	newKey, err := normalizeAudioS3ObjectKey(path.Join(prefix, name))
	if err != nil {
		return nil, err
	}
	if err := ensureAudioS3KeyAllowed(newKey); err != nil {
		return nil, err
	}
	if oldKey == newKey {
		return AudioS3GetAsset(ctx, ref)
	}
	client, cfg, err := newAudioS3Client()
	if err != nil {
		return nil, err
	}
	exists, err := audioS3ObjectExists(ctx, client, cfg.Bucket, newKey)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrAudioS3Exists
	}
	if err := copyAudioS3Object(ctx, client, cfg.Bucket, oldKey, newKey); err != nil {
		return nil, err
	}
	oldRef := EncodeAudioS3AssetRef(oldKey)
	newRef := EncodeAudioS3AssetRef(newKey)
	if _, err := rewriteAudioS3References(map[string]string{oldRef: newRef}); err != nil {
		_ = removeAudioS3Object(ctx, client, cfg.Bucket, newKey)
		return nil, err
	}
	if err := removeAudioS3Object(ctx, client, cfg.Bucket, oldKey); err != nil {
		return nil, err
	}
	return AudioS3GetAsset(ctx, newRef)
}

func AudioS3DeleteAsset(ctx context.Context, ref string, forceDetach bool) (*AudioDeleteImpact, error) {
	key, err := DecodeAudioS3AssetRef(ref)
	if err != nil {
		return nil, err
	}
	if err := ensureAudioS3KeyAllowed(key); err != nil {
		return nil, err
	}
	usage, err := AudioGetAssetUsageSummary(ref)
	if err != nil {
		return nil, err
	}
	if usage.Referenced && !forceDetach {
		return nil, &AudioAssetReferencedError{Summary: usage}
	}
	impact := &AudioDeleteImpact{}
	if forceDetach {
		impact, err = rewriteAudioS3References(map[string]string{ref: ""})
		if err != nil {
			return nil, err
		}
	}
	client, cfg, err := newAudioS3Client()
	if err != nil {
		return nil, err
	}
	if err := removeAudioS3Object(ctx, client, cfg.Bucket, key); err != nil {
		return nil, err
	}
	return impact, nil
}

func AudioS3UpdateFolder(ctx context.Context, ref string, input AudioS3FolderUpdateInput) (*AudioS3Folder, error) {
	oldPrefix, err := DecodeAudioS3FolderRef(ref)
	if err != nil {
		return nil, err
	}
	if err := ensureAudioS3PrefixAllowed(oldPrefix); err != nil {
		return nil, err
	}
	name := path.Base(strings.TrimSuffix(oldPrefix, "/"))
	if strings.TrimSpace(input.Name) != "" {
		name, err = sanitizeAudioS3Name(input.Name)
		if err != nil {
			return nil, err
		}
	}
	parent := parentAudioS3Prefix(oldPrefix)
	if input.ParentID != nil {
		parent, err = resolveAudioS3FolderPrefix(strings.TrimSpace(*input.ParentID))
		if err != nil {
			return nil, err
		}
	}
	newPrefix, err := NormalizeAudioS3Prefix(path.Join(parent, name))
	if err != nil {
		return nil, err
	}
	if err := ensureAudioS3PrefixAllowed(newPrefix); err != nil {
		return nil, err
	}
	if oldPrefix == newPrefix {
		return audioS3FolderFromPrefix(oldPrefix), nil
	}
	if strings.HasPrefix(newPrefix, oldPrefix) {
		return nil, errors.New("不能将文件夹移动到自己的子目录")
	}
	client, cfg, err := newAudioS3Client()
	if err != nil {
		return nil, err
	}
	targetExists, err := audioS3PrefixHasObjects(ctx, client, cfg.Bucket, newPrefix)
	if err != nil {
		return nil, err
	}
	if targetExists {
		return nil, ErrAudioS3Exists
	}
	objects, err := listAudioS3Objects(ctx, oldPrefix)
	if err != nil {
		return nil, err
	}
	if len(objects) == 0 {
		objects = append(objects, minio.ObjectInfo{Key: oldPrefix})
	}
	for _, object := range objects {
		if strings.HasSuffix(object.Key, "/") || isAudioS3Object(object.Key) {
			continue
		}
		return nil, fmt.Errorf("%w: %s", ErrAudioS3ForeignObject, object.Key)
	}
	copied := make([]string, 0, len(objects))
	mapping := map[string]string{}
	for _, object := range objects {
		suffix := strings.TrimPrefix(object.Key, oldPrefix)
		targetKey := newPrefix + suffix
		exists, err := audioS3ObjectExists(ctx, client, cfg.Bucket, targetKey)
		if err != nil {
			for _, key := range copied {
				_ = removeAudioS3Object(ctx, client, cfg.Bucket, key)
			}
			return nil, err
		}
		if exists {
			for _, key := range copied {
				_ = removeAudioS3Object(ctx, client, cfg.Bucket, key)
			}
			return nil, ErrAudioS3Exists
		}
		if object.Key == oldPrefix && strings.HasSuffix(object.Key, "/") {
			if _, err := client.PutObject(ctx, cfg.Bucket, targetKey, bytes.NewReader(nil), 0, minio.PutObjectOptions{ContentType: "application/x-directory"}); err != nil {
				return nil, err
			}
		} else if err := copyAudioS3Object(ctx, client, cfg.Bucket, object.Key, targetKey); err != nil {
			for _, key := range copied {
				_ = removeAudioS3Object(ctx, client, cfg.Bucket, key)
			}
			return nil, err
		}
		copied = append(copied, targetKey)
		if isAudioS3Object(object.Key) {
			mapping[EncodeAudioS3AssetRef(object.Key)] = EncodeAudioS3AssetRef(targetKey)
		}
	}
	if len(mapping) > 0 {
		if _, err := rewriteAudioS3References(mapping); err != nil {
			for _, key := range copied {
				_ = removeAudioS3Object(ctx, client, cfg.Bucket, key)
			}
			return nil, err
		}
	}
	for _, object := range objects {
		if err := removeAudioS3Object(ctx, client, cfg.Bucket, object.Key); err != nil {
			return nil, err
		}
	}
	return audioS3FolderFromPrefix(newPrefix), nil
}

func AudioS3DeleteFolder(ctx context.Context, ref string, forceDetach bool) (*AudioDeleteImpact, error) {
	prefix, err := DecodeAudioS3FolderRef(ref)
	if err != nil {
		return nil, err
	}
	if err := ensureAudioS3PrefixAllowed(prefix); err != nil {
		return nil, err
	}
	objects, err := listAudioS3Objects(ctx, prefix)
	if err != nil {
		return nil, err
	}
	for _, object := range objects {
		if strings.HasSuffix(object.Key, "/") || isAudioS3Object(object.Key) {
			continue
		}
		return nil, fmt.Errorf("%w: %s", ErrAudioS3ForeignObject, object.Key)
	}
	mapping := map[string]string{}
	aggregate := AudioAssetUsageSummary{}
	for _, object := range objects {
		if !isAudioS3Object(object.Key) {
			continue
		}
		assetRef := EncodeAudioS3AssetRef(object.Key)
		mapping[assetRef] = ""
		usage, err := AudioGetAssetUsageSummary(assetRef)
		if err != nil {
			return nil, err
		}
		aggregate.SceneRefCount += usage.SceneRefCount
		aggregate.PlaybackStateRefCount += usage.PlaybackStateRefCount
		aggregate.SceneNames = appendUniqueStrings(aggregate.SceneNames, usage.SceneNames...)
		aggregate.PlaybackScopeLabels = appendUniqueStrings(aggregate.PlaybackScopeLabels, usage.PlaybackScopeLabels...)
	}
	aggregate.Referenced = aggregate.SceneRefCount > 0 || aggregate.PlaybackStateRefCount > 0
	if aggregate.Referenced && !forceDetach {
		return nil, &AudioAssetReferencedError{Summary: aggregate}
	}
	impact := &AudioDeleteImpact{}
	if forceDetach && len(mapping) > 0 {
		impact, err = rewriteAudioS3References(mapping)
		if err != nil {
			return nil, err
		}
	}
	client, cfg, err := newAudioS3Client()
	if err != nil {
		return nil, err
	}
	for _, object := range objects {
		if err := removeAudioS3Object(ctx, client, cfg.Bucket, object.Key); err != nil {
			return nil, err
		}
	}
	if len(objects) == 0 {
		_ = removeAudioS3Object(ctx, client, cfg.Bucket, prefix)
	}
	return impact, nil
}

func rewriteAudioS3References(mapping map[string]string) (*AudioDeleteImpact, error) {
	if len(mapping) == 0 {
		return &AudioDeleteImpact{}, nil
	}
	impact := &AudioDeleteImpact{}
	updatedStates := make([]*model.AudioPlaybackState, 0)
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		var scenes []*model.AudioScene
		if err := tx.Find(&scenes).Error; err != nil {
			return err
		}
		for _, scene := range scenes {
			if scene == nil {
				continue
			}
			tracks, changed := rewriteAudioS3SceneTracks(scene.Tracks, mapping)
			if !changed {
				continue
			}
			now := time.Now()
			if err := tx.Model(&model.AudioScene{}).Where("id = ?", scene.ID).Updates(map[string]interface{}{
				"tracks":     tracks,
				"updated_at": now,
			}).Error; err != nil {
				return err
			}
			impact.DetachedSceneCount++
			if name := strings.TrimSpace(scene.Name); name != "" {
				impact.SceneNames = appendUniqueStrings(impact.SceneNames, name)
			}
		}

		var states []*model.AudioPlaybackState
		if err := tx.Find(&states).Error; err != nil {
			return err
		}
		for _, state := range states {
			if state == nil {
				continue
			}
			tracks, changed := rewriteAudioS3PlaybackTracks(state.Tracks, mapping)
			if !changed {
				continue
			}
			state.Tracks = tracks
			if allPlaybackTracksIdle(state.Tracks) {
				state.IsPlaying = false
				state.Position = 0
			}
			state.UpdatedAt = time.Now()
			state.CapturedAtMs = state.UpdatedAt.UnixMilli()
			state.Revision++
			if err := tx.Model(&model.AudioPlaybackState{}).Where("channel_id = ?", state.ChannelID).Updates(map[string]interface{}{
				"tracks":         state.Tracks,
				"is_playing":     state.IsPlaying,
				"position":       state.Position,
				"updated_at":     state.UpdatedAt,
				"captured_at_ms": state.CapturedAtMs,
				"revision":       state.Revision,
			}).Error; err != nil {
				return err
			}
			updatedStates = append(updatedStates, state)
			impact.DetachedPlaybackStateCount++
			if label := strings.TrimSpace(state.ChannelID); label != "" {
				impact.PlaybackScopeLabels = appendUniqueStrings(impact.PlaybackScopeLabels, label)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, state := range updatedStates {
		syncPlaybackRuntimeState(state)
	}
	return impact, nil
}

func rewriteAudioS3SceneTracks(tracks model.JSONList[model.AudioSceneTrack], mapping map[string]string) (model.JSONList[model.AudioSceneTrack], bool) {
	result := make([]model.AudioSceneTrack, 0, len(tracks))
	changed := false
	for _, track := range tracks {
		next := track
		if next.AssetID != nil {
			current := strings.TrimSpace(*next.AssetID)
			if replacement, ok := mapping[current]; ok {
				changed = true
				if replacement == "" {
					next.AssetID = nil
				} else {
					value := replacement
					next.AssetID = &value
				}
			}
		}
		next.PlaylistAssetIDs, changed = rewriteAudioS3IDList(next.PlaylistAssetIDs, mapping, changed)
		if len(next.PlaylistAssetIDs) == 0 {
			next.PlaylistIndex = 0
		} else if next.PlaylistIndex >= len(next.PlaylistAssetIDs) {
			next.PlaylistIndex = len(next.PlaylistAssetIDs) - 1
		}
		result = append(result, next)
	}
	return model.JSONList[model.AudioSceneTrack](result), changed
}

func rewriteAudioS3PlaybackTracks(tracks model.JSONList[model.AudioTrackState], mapping map[string]string) (model.JSONList[model.AudioTrackState], bool) {
	result := make([]model.AudioTrackState, 0, len(tracks))
	changed := false
	for _, track := range tracks {
		next := track
		directRemoved := false
		if next.AssetID != nil {
			current := strings.TrimSpace(*next.AssetID)
			if replacement, ok := mapping[current]; ok {
				changed = true
				if replacement == "" {
					next.AssetID = nil
					directRemoved = true
				} else {
					value := replacement
					next.AssetID = &value
				}
			}
		}
		next.PlaylistAssetIDs, changed = rewriteAudioS3IDList(next.PlaylistAssetIDs, mapping, changed)
		if len(next.PlaylistAssetIDs) == 0 {
			next.PlaylistIndex = 0
		} else if next.PlaylistIndex >= len(next.PlaylistAssetIDs) {
			next.PlaylistIndex = len(next.PlaylistAssetIDs) - 1
		}
		if directRemoved || (next.AssetID == nil && len(next.PlaylistAssetIDs) == 0) {
			next.IsPlaying = false
			next.Position = 0
		}
		result = append(result, next)
	}
	return model.JSONList[model.AudioTrackState](result), changed
}

func rewriteAudioS3IDList(ids []string, mapping map[string]string, changed bool) ([]string, bool) {
	if len(ids) == 0 {
		return ids, changed
	}
	result := make([]string, 0, len(ids))
	seen := map[string]struct{}{}
	for _, id := range ids {
		next := strings.TrimSpace(id)
		if replacement, ok := mapping[next]; ok {
			changed = true
			next = replacement
		}
		if next == "" {
			continue
		}
		if _, ok := seen[next]; ok {
			continue
		}
		seen[next] = struct{}{}
		result = append(result, next)
	}
	return result, changed
}
