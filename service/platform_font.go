package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/service/storage"
	"sealchat/utils"
)

var (
	ErrPlatformFontNotFound   = errors.New("platform font not found")
	ErrPlatformFontPermission = errors.New("platform font permission denied")
	ErrPlatformFontInvalid    = errors.New("platform font invalid")
)

type PlatformFontCreateInput struct {
	DisplayName string
	Family      string
	Weight      string
	Style       string
	PreviewText string
	CreatedBy   string
}

type PlatformFontUpdateInput struct {
	DisplayName  *string
	Family       *string
	Weight       *string
	Style        *string
	PreviewText  *string
	Status       *model.PlatformFontStatus
	DeliveryMode *model.PlatformFontDeliveryMode
	LastError    *string
	UpdatedBy    string
}

type PlatformFontListOptions struct {
	Query           string
	IncludeDisabled bool
	Page            int
	PageSize        int
}

type PlatformFontSubsetManifestChunk struct {
	Name         string `json:"name"`
	Url          string `json:"url,omitempty"`
	UnicodeRange string `json:"unicodeRange,omitempty"`
	MimeType     string `json:"mimeType,omitempty"`
}

type PlatformFontSubsetManifestData struct {
	Mode      string                            `json:"mode,omitempty"`
	Entry     string                            `json:"entry,omitempty"`
	CssUrl    string                            `json:"cssUrl,omitempty"`
	CssName   string                            `json:"cssName,omitempty"`
	FontUrls  []string                          `json:"fontUrls,omitempty"`
	FontFiles []string                          `json:"fontFiles,omitempty"`
	Chunks    []PlatformFontSubsetManifestChunk `json:"chunks,omitempty"`
}

type PlatformFontSubsetUploadFile struct {
	Name        string
	LocalPath    string
	ContentType string
}

type PlatformFontSubsetPackageInput struct {
	ActorID  string
	Manifest PlatformFontSubsetManifestData
	Files    []PlatformFontSubsetUploadFile
}

func ensurePlatformFontAdmin(actorID string) error {
	if strings.TrimSpace(actorID) == "" || !pm.CanWithSystemRole(actorID, pm.PermModAdmin) {
		return ErrPlatformFontPermission
	}
	return nil
}

func normalizePlatformFontName(value string, fallback string, maxRunes int) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		trimmed = strings.TrimSpace(fallback)
	}
	if trimmed == "" {
		return ""
	}
	if utf8.RuneCountInString(trimmed) > maxRunes {
		trimmed = string([]rune(trimmed)[:maxRunes])
	}
	return trimmed
}

func normalizePlatformFontStyle(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "italic":
		return "italic"
	default:
		return "normal"
	}
}

func normalizePlatformFontWeight(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "400"
	}
	if len(trimmed) > 16 {
		return trimmed[:16]
	}
	return trimmed
}

func normalizePlatformFontSubsetPath(value string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if trimmed == "" {
		return ""
	}
	cleaned := strings.TrimLeft(filepath.ToSlash(filepath.Clean(trimmed)), "/")
	if cleaned == "." || cleaned == "" || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return ""
	}
	return cleaned
}

func detectPlatformFontMime(fileHeader *multipart.FileHeader) string {
	contentType := strings.ToLower(strings.TrimSpace(fileHeader.Header.Get("Content-Type")))
	if idx := strings.Index(contentType, ";"); idx >= 0 {
		contentType = strings.TrimSpace(contentType[:idx])
	}
	if contentType != "" && contentType != "application/octet-stream" {
		return contentType
	}
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	switch ext {
	case ".woff2":
		return "font/woff2"
	case ".woff":
		return "font/woff"
	case ".ttf":
		return "font/ttf"
	case ".otf":
		return "font/otf"
	default:
		return "application/octet-stream"
	}
}

func validatePlatformFontFileHeader(fileHeader *multipart.FileHeader) (string, error) {
	if fileHeader == nil {
		return "", fmt.Errorf("%w: 缺少字体文件", ErrPlatformFontInvalid)
	}
	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	switch ext {
	case ".ttf", ".otf", ".woff", ".woff2":
	default:
		return "", fmt.Errorf("%w: 仅支持 ttf/otf/woff/woff2 字体文件", ErrPlatformFontInvalid)
	}
	mimeType := detectPlatformFontMime(fileHeader)
	switch mimeType {
	case "font/woff2", "font/woff", "font/ttf", "font/otf", "application/x-font-ttf", "application/font-sfnt", "application/octet-stream":
	default:
		return "", fmt.Errorf("%w: 不支持的字体 MIME %s", ErrPlatformFontInvalid, mimeType)
	}
	maxSizeMB := int64(32)
	if cfg := utils.GetConfig(); cfg != nil && cfg.Storage.MaxSizeMB > 0 {
		maxSizeMB = cfg.Storage.MaxSizeMB
	}
	if fileHeader.Size > maxSizeMB*1024*1024 {
		return "", fmt.Errorf("%w: 字体文件超过大小限制", ErrPlatformFontInvalid)
	}
	return mimeType, nil
}

func PlatformFontList(actorID string, opts PlatformFontListOptions) ([]*model.PlatformFontAsset, int64, error) {
	if err := ensurePlatformFontAdmin(actorID); err != nil {
		return nil, 0, err
	}
	if opts.Page <= 0 {
		opts.Page = 1
	}
	if opts.PageSize <= 0 {
		opts.PageSize = 50
	}
	if opts.PageSize > 500 {
		opts.PageSize = 500
	}
	query := model.GetDB().Model(&model.PlatformFontAsset{})
	if !opts.IncludeDisabled {
		query = query.Where("status <> ?", model.PlatformFontStatusDisabled)
	}
	if trimmed := strings.TrimSpace(opts.Query); trimmed != "" {
		like := "%" + trimmed + "%"
		query = query.Where("display_name LIKE ? OR family LIKE ? OR source_file_name LIKE ?", like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return []*model.PlatformFontAsset{}, 0, nil
	}
	var items []*model.PlatformFontAsset
	if err := query.Order("updated_at DESC").Offset((opts.Page - 1) * opts.PageSize).Limit(opts.PageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func PlatformFontListPublic() ([]*model.PlatformFontAsset, error) {
	var items []*model.PlatformFontAsset
	if err := model.GetDB().
		Where("status = ?", model.PlatformFontStatusReady).
		Order("display_name ASC, updated_at DESC").
		Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func PlatformFontGet(id string) (*model.PlatformFontAsset, error) {
	var item model.PlatformFontAsset
	if err := model.GetDB().Where("id = ?", strings.TrimSpace(id)).Limit(1).Find(&item).Error; err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, ErrPlatformFontNotFound
	}
	return &item, nil
}

func PlatformFontCreateFromUpload(fileHeader *multipart.FileHeader, input PlatformFontCreateInput) (*model.PlatformFontAsset, error) {
	if err := ensurePlatformFontAdmin(input.CreatedBy); err != nil {
		return nil, err
	}
	mimeType, err := validatePlatformFontFileHeader(fileHeader)
	if err != nil {
		return nil, err
	}
	fileName := strings.TrimSpace(fileHeader.Filename)
	item := &model.PlatformFontAsset{
		DisplayName:  normalizePlatformFontName(input.DisplayName, strings.TrimSuffix(fileName, filepath.Ext(fileName)), 120),
		Family:       normalizePlatformFontName(input.Family, strings.TrimSuffix(fileName, filepath.Ext(fileName)), 120),
		Weight:       normalizePlatformFontWeight(input.Weight),
		Style:        normalizePlatformFontStyle(input.Style),
		Status:       model.PlatformFontStatusProcessing,
		DeliveryMode: model.PlatformFontDeliverySingle,
		PreviewText:  normalizePlatformFontName(input.PreviewText, "永字八法", 120),
		SourceFileName: fileName,
		SourceMimeType: mimeType,
		SourceSize:     fileHeader.Size,
		CreatedBy:      input.CreatedBy,
		UpdatedBy:      input.CreatedBy,
	}
	item.Init()
	if item.DisplayName == "" || item.Family == "" {
		return nil, fmt.Errorf("%w: 字体名称不能为空", ErrPlatformFontInvalid)
	}

	tmpDir := "./data/temp"
	if cfg := utils.GetConfig(); cfg != nil && strings.TrimSpace(cfg.Storage.Local.TempDir) != "" {
		tmpDir = cfg.Storage.Local.TempDir
	}
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return nil, err
	}
	tempFile, err := os.CreateTemp(tmpDir, "*.font-upload")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tempFile.Close()
	}()
	src, err := fileHeader.Open()
	if err != nil {
		_ = os.Remove(tempFile.Name())
		return nil, err
	}
	defer src.Close()
	if _, err := io.Copy(tempFile, src); err != nil {
		_ = os.Remove(tempFile.Name())
		return nil, err
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempFile.Name())
		return nil, err
	}

	objectKey := storage.BuildPlatformFontObjectKey(item.ID, fileName)
	location, err := PersistPlatformFontFile(tempFile.Name(), objectKey, mimeType)
	if err != nil {
		_ = os.Remove(tempFile.Name())
		return nil, err
	}
	item.OriginalStorageType = location.StorageType
	item.OriginalObjectKey = location.ObjectKey
	item.Status = model.PlatformFontStatusReady
	now := time.Now()
	item.LastPublishedAt = &now

	if err := model.GetDB().Create(item).Error; err != nil {
		_ = DeletePlatformFontFile(item.OriginalStorageType, item.OriginalObjectKey)
		return nil, err
	}
	return item, nil
}

func PlatformFontUpdate(id string, input PlatformFontUpdateInput) (*model.PlatformFontAsset, error) {
	if err := ensurePlatformFontAdmin(input.UpdatedBy); err != nil {
		return nil, err
	}
	item, err := PlatformFontGet(id)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{
		"updated_by": input.UpdatedBy,
	}
	if input.DisplayName != nil {
		updates["display_name"] = normalizePlatformFontName(*input.DisplayName, item.DisplayName, 120)
	}
	if input.Family != nil {
		updates["family"] = normalizePlatformFontName(*input.Family, item.Family, 120)
	}
	if input.Weight != nil {
		updates["weight"] = normalizePlatformFontWeight(*input.Weight)
	}
	if input.Style != nil {
		updates["style"] = normalizePlatformFontStyle(*input.Style)
	}
	if input.PreviewText != nil {
		updates["preview_text"] = normalizePlatformFontName(*input.PreviewText, item.PreviewText, 120)
	}
	if input.Status != nil {
		updates["status"] = *input.Status
		if *input.Status == model.PlatformFontStatusReady {
			now := time.Now()
			updates["last_published_at"] = &now
		}
	}
	if input.DeliveryMode != nil {
		updates["delivery_mode"] = *input.DeliveryMode
	}
	if input.LastError != nil {
		updates["last_error"] = strings.TrimSpace(*input.LastError)
	}
	if err := model.GetDB().Model(item).Updates(updates).Error; err != nil {
		return nil, err
	}
	return PlatformFontGet(item.ID)
}

func PlatformFontDelete(id string, actorID string) error {
	if err := ensurePlatformFontAdmin(actorID); err != nil {
		return err
	}
	item, err := PlatformFontGet(id)
	if err != nil {
		return err
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&model.PlatformFontAsset{}, "id = ?", item.ID).Error; err != nil {
			return err
		}
		if err := DeletePlatformFontFile(item.OriginalStorageType, item.OriginalObjectKey); err != nil {
			return err
		}
		if err := DeletePlatformFontPrefix(item.SubsetStorageType, item.SubsetObjectKey); err != nil {
			return err
		}
		if err := DeletePlatformFontFile(item.ManifestStorageType, item.ManifestObjectKey); err != nil {
			return err
		}
		return nil
	})
}

func PlatformFontSaveSubsetPackage(id string, input PlatformFontSubsetPackageInput) (*model.PlatformFontAsset, error) {
	if err := ensurePlatformFontAdmin(input.ActorID); err != nil {
		return nil, err
	}
	item, err := PlatformFontGet(id)
	if err != nil {
		return nil, err
	}

	normalizedEntry := normalizePlatformFontSubsetPath(input.Manifest.Entry)
	if normalizedEntry == "" {
		return nil, fmt.Errorf("%w: 分片清单缺少 entry", ErrPlatformFontInvalid)
	}
	normalizedCssName := normalizePlatformFontSubsetPath(input.Manifest.CssName)
	if normalizedCssName == "" {
		normalizedCssName = normalizedEntry
	}

	filesByName := make(map[string]PlatformFontSubsetUploadFile, len(input.Files))
	for _, file := range input.Files {
		name := normalizePlatformFontSubsetPath(file.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: 分片文件名无效", ErrPlatformFontInvalid)
		}
		if strings.TrimSpace(file.LocalPath) == "" {
			return nil, fmt.Errorf("%w: 分片文件缺少本地路径", ErrPlatformFontInvalid)
		}
		if _, statErr := os.Stat(file.LocalPath); statErr != nil {
			return nil, fmt.Errorf("%w: 分片文件不存在", ErrPlatformFontInvalid)
		}
		file.Name = name
		filesByName[name] = file
	}
	if _, ok := filesByName[normalizedEntry]; !ok {
		return nil, fmt.Errorf("%w: 缺少入口样式文件 %s", ErrPlatformFontInvalid, normalizedEntry)
	}

	normalizedChunks := make([]PlatformFontSubsetManifestChunk, 0, len(input.Manifest.Chunks))
	fontFiles := make([]string, 0, len(input.Manifest.Chunks))
	fontURLs := make([]string, 0, len(input.Manifest.Chunks))
	for _, chunk := range input.Manifest.Chunks {
		name := normalizePlatformFontSubsetPath(chunk.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: 分片清单存在无效 chunk 名称", ErrPlatformFontInvalid)
		}
		if _, ok := filesByName[name]; !ok {
			return nil, fmt.Errorf("%w: 缺少分片文件 %s", ErrPlatformFontInvalid, name)
		}
		chunk.Name = name
		chunk.Url = ""
		if strings.TrimSpace(chunk.MimeType) == "" {
			chunk.MimeType = detectPlatformFontSubsetContentType(name)
		}
		normalizedChunks = append(normalizedChunks, chunk)
		fontFiles = append(fontFiles, name)
		fontURLs = append(fontURLs, name)
	}
	if len(normalizedChunks) == 0 {
		return nil, fmt.Errorf("%w: 至少需要一个字体分片", ErrPlatformFontInvalid)
	}

	manifest := PlatformFontSubsetManifestData{
		Mode:      strings.TrimSpace(input.Manifest.Mode),
		Entry:     normalizedEntry,
		CssName:   normalizedCssName,
		CssUrl:    normalizedEntry,
		FontFiles: fontFiles,
		FontUrls:  fontURLs,
		Chunks:    normalizedChunks,
	}
	if manifest.Mode == "" {
		manifest.Mode = "cn-font-split"
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}

	subsetRoot := storage.BuildPlatformFontSubsetObjectKey(item.ID, "")
	manifestObjectKey := storage.BuildPlatformFontSubsetObjectKey(item.ID, "manifest.json")

	type persistedFile struct {
		storageType model.StorageType
		objectKey   string
	}
	persisted := make([]persistedFile, 0, len(filesByName)+1)
	cleanupPersisted := func() {
		for _, file := range persisted {
			_ = DeletePlatformFontFile(file.storageType, file.objectKey)
		}
	}

	var subsetStorageType model.StorageType
	for _, file := range filesByName {
		objectKey := storage.BuildPlatformFontSubsetObjectKey(item.ID, file.Name)
		location, persistErr := PersistPlatformFontFile(file.LocalPath, objectKey, detectPlatformFontSubsetContentType(file.Name, file.ContentType))
		if persistErr != nil {
			cleanupPersisted()
			return nil, persistErr
		}
		persisted = append(persisted, persistedFile{
			storageType: location.StorageType,
			objectKey:   location.ObjectKey,
		})
		if subsetStorageType == "" {
			subsetStorageType = location.StorageType
		}
	}

	tempDir := "./data/temp"
	if cfg := utils.GetConfig(); cfg != nil && strings.TrimSpace(cfg.Storage.Local.TempDir) != "" {
		tempDir = cfg.Storage.Local.TempDir
	}
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		cleanupPersisted()
		return nil, err
	}
	manifestFile, err := os.CreateTemp(tempDir, "*.platform-font-manifest.json")
	if err != nil {
		cleanupPersisted()
		return nil, err
	}
	manifestTempPath := manifestFile.Name()
	if _, err := manifestFile.Write(manifestBytes); err != nil {
		_ = manifestFile.Close()
		_ = os.Remove(manifestTempPath)
		cleanupPersisted()
		return nil, err
	}
	if err := manifestFile.Close(); err != nil {
		_ = os.Remove(manifestTempPath)
		cleanupPersisted()
		return nil, err
	}
	manifestLocation, err := PersistPlatformFontFile(manifestTempPath, manifestObjectKey, "application/json")
	if err != nil {
		_ = os.Remove(manifestTempPath)
		cleanupPersisted()
		return nil, err
	}
	persisted = append(persisted, persistedFile{
		storageType: manifestLocation.StorageType,
		objectKey:   manifestLocation.ObjectKey,
	})

	now := time.Now()
	updates := map[string]any{
		"delivery_mode":         model.PlatformFontDeliverySubset,
		"status":                model.PlatformFontStatusReady,
		"subset_storage_type":   subsetStorageType,
		"subset_object_key":     subsetRoot,
		"manifest_storage_type": manifestLocation.StorageType,
		"manifest_object_key":   manifestLocation.ObjectKey,
		"subset_count":          len(normalizedChunks),
		"last_error":            "",
		"updated_by":            input.ActorID,
		"last_published_at":     &now,
	}
	if err := model.GetDB().Model(item).Updates(updates).Error; err != nil {
		cleanupPersisted()
		return nil, err
	}

	if strings.TrimSpace(item.SubsetObjectKey) != "" && item.SubsetObjectKey != subsetRoot {
		_ = DeletePlatformFontPrefix(item.SubsetStorageType, item.SubsetObjectKey)
	}
	if strings.TrimSpace(item.ManifestObjectKey) != "" && item.ManifestObjectKey != manifestLocation.ObjectKey {
		_ = DeletePlatformFontFile(item.ManifestStorageType, item.ManifestObjectKey)
	}

	return PlatformFontGet(item.ID)
}

func detectPlatformFontSubsetContentType(name string, explicit ...string) string {
	for _, value := range explicit {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" && trimmed != "application/octet-stream" {
			return trimmed
		}
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".css":
		return "text/css"
	case ".woff2":
		return "font/woff2"
	case ".woff":
		return "font/woff"
	case ".ttf":
		return "font/ttf"
	case ".otf":
		return "font/otf"
	case ".json":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}
