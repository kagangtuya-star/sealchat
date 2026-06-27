package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/model"
	"sealchat/utils"
)

type AudioAssetFilters struct {
	Query             string
	Tags              []string
	FolderID          *string
	CreatorIDs        []string
	DurationMin       float64
	DurationMax       float64
	HasSceneOnly      bool
	Page              int
	PageSize          int
	SortBy            string
	SortOrder         string
	ManualSortEnabled bool
	Scope             model.AudioAssetScope
	WorldID           *string
	IncludeCommon     bool
}

type AudioAssetUpdateInput struct {
	Name        *string
	Description *string
	Tags        []string
	Visibility  *model.AudioAssetVisibility
	FolderID    *string
	Scope       *model.AudioAssetScope
	WorldID     *string
	UpdatedBy   string
	Variants    []model.AudioAssetVariant
}

type AudioAssetUsageSummary struct {
	SceneRefCount         int      `json:"sceneRefCount"`
	PlaybackStateRefCount int      `json:"playbackStateRefCount"`
	SceneNames            []string `json:"sceneNames,omitempty"`
	PlaybackScopeLabels   []string `json:"playbackScopeLabels,omitempty"`
	Referenced            bool     `json:"referenced"`
}

type AudioAssetReferencedError struct {
	Summary AudioAssetUsageSummary
}

func (e *AudioAssetReferencedError) Error() string {
	return "audio asset is still referenced"
}

type AdminAudioAssetFilters struct {
	Query         string
	QueryField    string
	Scope         model.AudioAssetScope
	WorldID       *string
	CreatorID     *string
	Referenced    *bool
	NeverAccessed *bool
	InactiveDays  int
	SortBy        string
	SortOrder     string
	Page          int
	PageSize      int
}

type AdminAudioAssetListItem struct {
	*model.AudioAsset
	WorldName    string                 `json:"worldName"`
	CreatorName  string                 `json:"creatorName"`
	UsageSummary AudioAssetUsageSummary `json:"usageSummary"`
	SafeToDelete bool                   `json:"safeToDelete"`
}

type AudioBulkDeleteFailure struct {
	AssetID      string                  `json:"assetId"`
	Reason       string                  `json:"reason"`
	UsageSummary *AudioAssetUsageSummary `json:"usageSummary,omitempty"`
}

type AudioBulkDeleteResult struct {
	SuccessIDs                   []string                 `json:"successIds"`
	Failed                       []AudioBulkDeleteFailure `json:"failed"`
	SuccessCount                 int                      `json:"successCount"`
	FailedCount                  int                      `json:"failedCount"`
	DetachedSceneCount           int                      `json:"detachedSceneCount"`
	DetachedPlaybackStateCount   int                      `json:"detachedPlaybackStateCount"`
	DetachedReferencedAssetCount int                      `json:"detachedReferencedAssetCount"`
	PlaybackScopeLabels          []string                 `json:"playbackScopeLabels,omitempty"`
}

type AdminAudioFilterOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type AdminAudioAssetListResult struct {
	Items          []AdminAudioAssetListItem `json:"items"`
	Page           int                       `json:"page"`
	PageSize       int                       `json:"pageSize"`
	Total          int64                     `json:"total"`
	WorldOptions   []AdminAudioFilterOption  `json:"worldOptions"`
	CreatorOptions []AdminAudioFilterOption  `json:"creatorOptions"`
}

type AudioManageAssetFilters struct {
	AdminAudioAssetFilters
	ActorID       string
	IsSystemAdmin bool
}

type AudioManageAssetListResult struct {
	AdminAudioAssetListResult
	Quota *AudioQuotaSummary `json:"quota,omitempty"`
}

type AdminAudioCleanupPreview struct {
	ThresholdBefore            time.Time                 `json:"thresholdBefore"`
	TotalCandidates            int                       `json:"totalCandidates"`
	SafeCandidates             int                       `json:"safeCandidates"`
	ReferencedSkipped          int                       `json:"referencedSkipped"`
	DirectDeleteCandidates     int                       `json:"directDeleteCandidates"`
	DetachThenDeleteCandidates int                       `json:"detachThenDeleteCandidates"`
	Items                      []AdminAudioAssetListItem `json:"items"`
}

type AudioDeleteImpact struct {
	DetachedSceneCount         int      `json:"detachedSceneCount"`
	DetachedPlaybackStateCount int      `json:"detachedPlaybackStateCount"`
	SceneNames                 []string `json:"sceneNames,omitempty"`
	PlaybackScopeLabels        []string `json:"playbackScopeLabels,omitempty"`
}

type audioPlaybackDetachResult struct {
	States      []*model.AudioPlaybackState
	ScopeLabels []string
}

type AudioImportPreviewItem struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	ModTime  int64  `json:"modTime"`
	MimeType string `json:"mimeType,omitempty"`
	Valid    bool   `json:"valid"`
	Reason   string `json:"reason,omitempty"`
}

type AudioImportPreview struct {
	Items   []AudioImportPreviewItem `json:"items"`
	Total   int                      `json:"total"`
	Valid   int                      `json:"valid"`
	Invalid int                      `json:"invalid"`
}

type AudioImportDirectoryNode struct {
	Path     string                     `json:"path"`
	Name     string                     `json:"name"`
	Children []*AudioImportDirectoryNode `json:"children,omitempty"`
}

type AudioImportBrowseResult struct {
	Tree        []*AudioImportDirectoryNode `json:"tree"`
	CurrentPath string                      `json:"currentPath"`
	Items       []AudioImportPreviewItem    `json:"items"`
	Total       int                         `json:"total"`
	Valid       int                         `json:"valid"`
	Invalid     int                         `json:"invalid"`
}

type AudioImportRequest struct {
	Directory string
	All     bool
	Paths   []string
	Options AudioUploadOptions
}

type AudioImportResultItem struct {
	Path    string `json:"path"`
	Name    string `json:"name,omitempty"`
	AssetID string `json:"assetId,omitempty"`
	Error   string `json:"error,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Warning string `json:"warning,omitempty"`
}

type AudioImportResult struct {
	Imported []AudioImportResultItem `json:"imported"`
	Failed   []AudioImportResultItem `json:"failed"`
	Skipped  []AudioImportResultItem `json:"skipped"`
}

type AudioImportJobStatus struct {
	JobID          string                  `json:"jobId"`
	Status         string                  `json:"status"`
	Directory      string                  `json:"directory"`
	TotalFiles     int                     `json:"totalFiles"`
	ProcessedFiles int                     `json:"processedFiles"`
	ImportedCount  int                     `json:"importedCount"`
	SkippedCount   int                     `json:"skippedCount"`
	FailedCount    int                     `json:"failedCount"`
	ErrorMessage   string                  `json:"errorMessage,omitempty"`
	Percentage     int                     `json:"percentage"`
	Imported       []AudioImportResultItem `json:"imported"`
	Failed         []AudioImportResultItem `json:"failed"`
	Skipped        []AudioImportResultItem `json:"skipped"`
	StartedAt      *time.Time              `json:"startedAt,omitempty"`
	FinishedAt     *time.Time              `json:"finishedAt,omitempty"`
}

const (
	AudioImportJobStatusPending = model.AudioImportJobStatusPending
	AudioImportJobStatusRunning = model.AudioImportJobStatusRunning
	AudioImportJobStatusDone    = model.AudioImportJobStatusDone
	AudioImportJobStatusFailed  = model.AudioImportJobStatusFailed
)

type AudioFolderNode struct {
	*model.AudioFolder
	Children []*AudioFolderNode `json:"children,omitempty"`
}

type AudioFolderPayload struct {
	Name     string
	ParentID *string
	ActorID  string
	Scope    model.AudioAssetScope
	WorldID  *string
}

type AudioFolderFilters struct {
	Scope         model.AudioAssetScope
	WorldID       *string
	IncludeCommon bool
}

type AudioSceneInput struct {
	Name         string
	Description  string
	Tracks       []model.AudioSceneTrack
	Tags         []string
	Order        int
	ChannelScope *string
	ActorID      string
	Scope        model.AudioAssetScope
	WorldID      *string
}

type AudioSceneFilters struct {
	ChannelScope  string
	Scope         model.AudioAssetScope
	WorldID       *string
	IncludeCommon bool
}

type AudioTrackState = model.AudioTrackState

type AudioPlaybackUpdateInput struct {
	ChannelID            string
	SceneID              *string
	Tracks               []AudioTrackState
	IsPlaying            bool
	Position             float64
	CapturedAtMs         int64
	LoopEnabled          bool
	PlaybackRate         float64
	WorldPlaybackEnabled bool
	BaseRevision         int64
	ActorID              string
	Persist              bool
	SyncReason           string
}

type AudioPlaybackRevisionConflictError struct {
	CurrentState *AudioPlaybackStateSnapshot
}

func (e *AudioPlaybackRevisionConflictError) Error() string {
	return "audio playback revision conflict"
}

const (
	AudioPlaybackScopeChannel = "channel"
	AudioPlaybackScopeWorld   = "world"
	audioWorldScopeRowPrefix  = "__world__:"
)

type AudioPlaybackStateSnapshot struct {
	ChannelID            string
	SceneID              *string
	Tracks               []AudioTrackState
	IsPlaying            bool
	Position             float64
	BasePositionSec      float64
	CapturedAtMs         int64
	LoopEnabled          bool
	PlaybackRate         float64
	WorldPlaybackEnabled bool
	Revision             int64
	UpdatedBy            string
	UpdatedAt            time.Time
	ScopeType            string
	ScopeID              string
}

type audioPlaybackRuntimeState struct {
	ChannelID            string
	SceneID              *string
	Tracks               []AudioTrackState
	IsPlaying            bool
	BasePositionSec      float64
	CapturedAtMs         int64
	LoopEnabled          bool
	PlaybackRate         float64
	WorldPlaybackEnabled bool
	Revision             int64
	UpdatedBy            string
	UpdatedAt            time.Time
	ScopeType            string
	ScopeID              string
}

var audioPlaybackRuntimeStore = struct {
	sync.RWMutex
	states map[string]*audioPlaybackRuntimeState
}{
	states: map[string]*audioPlaybackRuntimeState{},
}

func (f *AudioAssetFilters) normalize() {
	f.Query = strings.TrimSpace(f.Query)
	f.SortBy = normalizeAudioAssetSortField(f.SortBy)
	f.SortOrder = normalizeAdminAudioSortOrder(f.SortOrder)
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PageSize <= 0 || f.PageSize > 500 {
		f.PageSize = 200
	}
}

func (f *AdminAudioAssetFilters) normalize() {
	f.Query = strings.TrimSpace(f.Query)
	f.QueryField = normalizeAdminAudioQueryField(f.QueryField)
	f.SortBy = normalizeAdminAudioSortField(f.SortBy)
	f.SortOrder = normalizeAdminAudioSortOrder(f.SortOrder)
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PageSize <= 0 || f.PageSize > 200 {
		f.PageSize = 20
	}
	if f.InactiveDays < 0 {
		f.InactiveDays = 0
	}
}

func AudioCreateAssetFromUpload(file *multipart.FileHeader, opts AudioUploadOptions) (*model.AudioAsset, error) {
	if opts.CreatedBy == "" {
		return nil, errors.New("缺少上传者标识")
	}
	if opts.FolderID != nil && strings.TrimSpace(*opts.FolderID) != "" {
		trimmed := strings.TrimSpace(*opts.FolderID)
		folder, err := getAudioFolder(trimmed)
		if err != nil {
			return nil, err
		}
		if err := validateFolderScopeMatch(folder, opts.Scope, opts.WorldID); err != nil {
			return nil, err
		}
		opts.FolderID = &trimmed
	}
	if _, err := EnsureAudioQuotaForIncoming(opts.CreatedBy, file.Size); err != nil {
		return nil, err
	}
	asset, err := AudioProcessUpload(file, opts)
	if err != nil {
		return nil, err
	}
	if err := withAudioQuotaUserLock(opts.CreatedBy, func() error {
		usedBytes, err := GetAudioUsedBytes(opts.CreatedBy)
		if err != nil {
			return err
		}
		summary, err := buildAudioQuotaSummary(opts.CreatedBy, usedBytes)
		if err != nil {
			return err
		}
		if summary.Limited && summary.QuotaBytes != nil && usedBytes+asset.Size > *summary.QuotaBytes {
			return &AudioQuotaExceededError{
				UsedBytes:     usedBytes,
				QuotaBytes:    *summary.QuotaBytes,
				IncomingBytes: asset.Size,
			}
		}
		return model.GetDB().Create(asset).Error
	}); err != nil {
		audioCleanupPersistedAsset(asset)
		return nil, err
	}
	if asset.TranscodeStatus == model.AudioTranscodePending {
		if svc := GetAudioService(); svc != nil {
			svc.scheduleTranscode(asset.ID, asset.ObjectKey)
		}
	}
	return asset, nil
}

func AudioCreateAssetFromImport(filePath string, opts AudioUploadOptions) (*model.AudioAsset, error) {
	if opts.CreatedBy == "" {
		return nil, errors.New("缺少上传者标识")
	}
	if opts.FolderID != nil && strings.TrimSpace(*opts.FolderID) != "" {
		trimmed := strings.TrimSpace(*opts.FolderID)
		folder, err := getAudioFolder(trimmed)
		if err != nil {
			return nil, err
		}
		if err := validateFolderScopeMatch(folder, opts.Scope, opts.WorldID); err != nil {
			return nil, err
		}
		opts.FolderID = &trimmed
	}
	svc := GetAudioService()
	if svc == nil {
		return nil, errors.New("音频服务未初始化")
	}
	if info, err := os.Stat(filePath); err == nil {
		if _, checkErr := EnsureAudioQuotaForIncoming(opts.CreatedBy, info.Size()); checkErr != nil {
			return nil, checkErr
		}
	}
	asset, err := svc.importFromPath(filePath, opts)
	if err != nil {
		return nil, err
	}
	if err := withAudioQuotaUserLock(opts.CreatedBy, func() error {
		usedBytes, err := GetAudioUsedBytes(opts.CreatedBy)
		if err != nil {
			return err
		}
		summary, err := buildAudioQuotaSummary(opts.CreatedBy, usedBytes)
		if err != nil {
			return err
		}
		if summary.Limited && summary.QuotaBytes != nil && usedBytes+asset.Size > *summary.QuotaBytes {
			return &AudioQuotaExceededError{
				UsedBytes:     usedBytes,
				QuotaBytes:    *summary.QuotaBytes,
				IncomingBytes: asset.Size,
			}
		}
		return model.GetDB().Create(asset).Error
	}); err != nil {
		audioCleanupPersistedAsset(asset)
		return nil, err
	}
	if asset.TranscodeStatus == model.AudioTranscodePending {
		svc.scheduleTranscode(asset.ID, asset.ObjectKey)
	}
	return asset, nil
}

func audioCleanupPersistedAsset(asset *model.AudioAsset) {
	if asset == nil {
		return
	}
	svc := GetAudioService()
	if svc == nil {
		return
	}
	svc.removeAssetObject(asset.StorageType, asset.ObjectKey)
	for _, variant := range asset.Variants {
		svc.removeAssetObject(variant.StorageType, variant.ObjectKey)
	}
}

func AudioGetAsset(id string) (*model.AudioAsset, error) {
	var asset model.AudioAsset
	if err := model.GetDB().Where("id = ? AND deleted_at IS NULL", id).First(&asset).Error; err != nil {
		return nil, err
	}
	return &asset, nil
}

func AudioListAssets(filters AudioAssetFilters) ([]*model.AudioAsset, int64, error) {
	filters.normalize()
	db := model.GetDB()
	var sceneAssetIDs []string
	if filters.HasSceneOnly {
		ids, err := audioAssetIDsInScenes()
		if err != nil {
			return nil, 0, err
		}
		if len(ids) == 0 {
			return []*model.AudioAsset{}, 0, nil
		}
		sceneAssetIDs = ids
	}
	return utils.QueryPaginatedList(db, filters.Page, filters.PageSize, &model.AudioAsset{}, func(q *gorm.DB) *gorm.DB {
		q = q.Where("deleted_at IS NULL")
		if filters.HasSceneOnly {
			q = q.Where("id IN ?", sceneAssetIDs)
		}
		if filters.Query != "" {
			keyword := fmt.Sprintf("%%%s%%", filters.Query)
			q = q.Where("name LIKE ? OR description LIKE ?", keyword, keyword)
		}
		if len(filters.Tags) > 0 {
			for _, tag := range filters.Tags {
				trimmed := strings.TrimSpace(tag)
				if trimmed == "" {
					continue
				}
				q = q.Where("tags LIKE ?", fmt.Sprintf("%%\"%s\"%%", trimmed))
			}
		}
		if filters.FolderID != nil {
			if *filters.FolderID == "" {
				q = q.Where("folder_id IS NULL")
			} else {
				q = q.Where("folder_id = ?", *filters.FolderID)
			}
		}
		if len(filters.CreatorIDs) > 0 {
			q = q.Where("created_by IN ?", filters.CreatorIDs)
		}
		if filters.DurationMin > 0 {
			q = q.Where("duration >= ?", filters.DurationMin)
		}
		if filters.DurationMax > 0 {
			q = q.Where("duration <= ?", filters.DurationMax)
		}
		// scope/worldId 过滤
		if filters.Scope != "" {
			if filters.Scope == model.AudioScopeWorld && filters.WorldID != nil {
				if filters.IncludeCommon {
					q = q.Where("(scope = ? AND world_id = ?) OR scope = ?", model.AudioScopeWorld, *filters.WorldID, model.AudioScopeCommon)
				} else {
					q = q.Where("scope = ? AND world_id = ?", model.AudioScopeWorld, *filters.WorldID)
				}
			} else {
				q = q.Where("scope = ?", filters.Scope)
			}
		} else if filters.WorldID != nil {
			if filters.IncludeCommon {
				q = q.Where("(scope = ? AND world_id = ?) OR scope = ?", model.AudioScopeWorld, *filters.WorldID, model.AudioScopeCommon)
			} else {
				q = q.Where("scope = ? AND world_id = ?", model.AudioScopeWorld, *filters.WorldID)
			}
		}
		return applyAudioAssetListOrder(q, filters.SortBy, filters.SortOrder, filters.ManualSortEnabled)
	})
}

func normalizeAudioAssetSortField(value string) string {
	switch strings.TrimSpace(value) {
	case "", "manual":
		return "manual"
	case "name", "scope", "duration", "updatedAt":
		return strings.TrimSpace(value)
	default:
		return "manual"
	}
}

func applyAudioAssetListOrder(q *gorm.DB, sortBy, sortOrder string, manualSortEnabled bool) *gorm.DB {
	desc := sortOrder == "desc"
	manualFirst := func(q *gorm.DB) *gorm.DB {
		if manualSortEnabled {
			return q.Order("manual_sorted DESC").Order("CASE WHEN manual_sorted THEN sort_order ELSE 0 END ASC")
		}
		return q
	}
	withStableTail := func(q *gorm.DB) *gorm.DB {
		return q.Order("id ASC")
	}
	switch sortBy {
	case "name":
		if desc {
			return withStableTail(manualFirst(q).Order("LOWER(name) DESC"))
		}
		return withStableTail(manualFirst(q).Order("LOWER(name) ASC"))
	case "scope":
		if desc {
			return withStableTail(manualFirst(q).Order("scope DESC"))
		}
		return withStableTail(manualFirst(q).Order("scope ASC"))
	case "duration":
		if desc {
			return withStableTail(manualFirst(q).Order("duration DESC"))
		}
		return withStableTail(manualFirst(q).Order("duration ASC"))
	case "updatedAt":
		if desc {
			return withStableTail(manualFirst(q).Order("updated_at DESC"))
		}
		return withStableTail(manualFirst(q).Order("updated_at ASC"))
	default:
		return q.Order("sort_order ASC").Order("id ASC")
	}
}

func GetAudioImportBrowser(currentPath string) (*AudioImportBrowseResult, error) {
	svc := GetAudioService()
	if svc == nil {
		return nil, errors.New("音频服务未初始化")
	}
	importDir, err := getAudioImportDir(svc)
	if err != nil {
		return nil, err
	}
	tree, err := buildAudioImportDirectoryTree(importDir, "")
	if err != nil {
		return nil, err
	}
	fullDir, normalizedPath, err := resolveAudioImportRelativePath(importDir, currentPath, true)
	if err != nil {
		return nil, err
	}
	items, err := listAudioImportDirectoryItems(svc, importDir, fullDir, normalizedPath)
	if err != nil {
		return nil, err
	}
	validCount := countValidAudioImportItems(items)
	return &AudioImportBrowseResult{
		Tree:        tree,
		CurrentPath: normalizedPath,
		Items:   items,
		Total:   len(items),
		Valid:   validCount,
		Invalid: len(items) - validCount,
	}, nil
}

func GetAudioImportPreview() (*AudioImportBrowseResult, error) {
	return GetAudioImportBrowser("")
}

func StartAudioImportJob(req AudioImportRequest) (*AudioImportJobStatus, error) {
	svc := GetAudioService()
	if svc == nil {
		return nil, errors.New("音频服务未初始化")
	}
	importDir, err := getAudioImportDir(svc)
	if err != nil {
		return nil, err
	}
	if req.All {
		if _, _, err := resolveAudioImportRelativePath(importDir, req.Directory, true); err != nil {
			return nil, err
		}
	}
	job := &model.AudioImportJobModel{}
	job.StringPKBaseModel.Init()
	job.Status = model.AudioImportJobStatusPending
	job.CreatedBy = strings.TrimSpace(req.Options.CreatedBy)
	job.Scope = strings.TrimSpace(string(req.Options.Scope))
	job.WorldID = cloneStringPtr(req.Options.WorldID)
	job.FolderID = cloneStringPtr(req.Options.FolderID)
	job.DirectoryPath = strings.TrimSpace(req.Directory)
	job.ImportedJSON = "[]"
	job.SkippedJSON = "[]"
	job.FailedJSON = "[]"
	if err := model.GetDB().Create(job).Error; err != nil {
		return nil, err
	}
	go executeAudioImportJob(job.ID, req)
	return audioImportJobModelToStatus(job)
}

func getAudioImportDir(svc *audioService) (string, error) {
	if svc == nil {
		return "", errors.New("音频服务未初始化")
	}
	importDir := strings.TrimSpace(svc.cfg.ImportDir)
	if importDir == "" {
		return "", errors.New("音频导入目录未配置")
	}
	return importDir, nil
}

func shouldSkipImportEntry(name string, entry os.DirEntry) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return true
	}
	if isHiddenImportPath(trimmed) {
		return true
	}
	if entry.Type()&os.ModeSymlink != 0 {
		return true
	}
	return false
}

func resolveAudioImportPath(importDir, name string) (string, error) {
	fullPath, _, err := resolveAudioImportRelativePath(importDir, name, false)
	return fullPath, err
}

func buildAudioImportPreviewItem(svc *audioService, fullPath, name string) AudioImportPreviewItem {
	item := AudioImportPreviewItem{
		Path: name,
		Name: name,
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		item.Valid = false
		item.Reason = "读取文件信息失败"
		return item
	}
	if !info.Mode().IsRegular() {
		item.Valid = false
		item.Reason = "不是普通文件"
		return item
	}
	item.Size = info.Size()
	item.ModTime = info.ModTime().UnixMilli()
	if item.Size > svc.maxUploadBytes() {
		item.Valid = false
		item.Reason = fmt.Sprintf("文件超过最大限制（%d MB）", svc.cfg.MaxUploadSizeMB)
		return item
	}
	file, err := os.Open(fullPath)
	if err != nil {
		item.Valid = false
		item.Reason = "读取文件失败"
		return item
	}
	defer file.Close()
	mimeType, err := svc.validateMime(file)
	if err != nil {
		item.Valid = false
		item.Reason = err.Error()
		return item
	}
	item.MimeType = mimeType
	item.Valid = true
	return item
}

func resolveAudioImportRelativePath(importDir, rawPath string, allowRoot bool) (string, string, error) {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		if allowRoot {
			return importDir, "", nil
		}
		return "", "", errors.New("文件路径为空")
	}
	rawNormalized := strings.ReplaceAll(trimmed, "\\", "/")
	for _, segment := range strings.Split(rawNormalized, "/") {
		segment = strings.TrimSpace(segment)
		if segment == ".." || segment == "." {
			return "", "", errors.New("非法文件路径")
		}
		if strings.HasPrefix(segment, ".") {
			return "", "", errors.New("隐藏文件")
		}
	}
	normalized := filepath.ToSlash(filepath.Clean(rawNormalized))
	if normalized == "." {
		if allowRoot {
			return importDir, "", nil
		}
		return "", "", errors.New("文件路径为空")
	}
	if normalized == ".." || strings.HasPrefix(normalized, "../") || strings.HasPrefix(normalized, "/") {
		return "", "", errors.New("非法文件路径")
	}
	segments := strings.Split(normalized, "/")
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" || segment == "." || segment == ".." {
			return "", "", errors.New("非法文件路径")
		}
	}
	fullPath := filepath.Join(importDir, filepath.FromSlash(normalized))
	relPath, err := filepath.Rel(importDir, fullPath)
	if err != nil {
		return "", "", errors.New("非法文件路径")
	}
	relPath = filepath.ToSlash(filepath.Clean(relPath))
	if relPath == "." {
		if allowRoot {
			return importDir, "", nil
		}
		return "", "", errors.New("非法文件路径")
	}
	if relPath == ".." || strings.HasPrefix(relPath, "../") {
		return "", "", errors.New("非法文件路径")
	}
	return fullPath, relPath, nil
}

func isHiddenImportPath(name string) bool {
	normalized := filepath.ToSlash(strings.TrimSpace(name))
	if normalized == "" {
		return false
	}
	for _, segment := range strings.Split(normalized, "/") {
		if strings.HasPrefix(strings.TrimSpace(segment), ".") {
			return true
		}
	}
	return false
}

func buildAudioImportDirectoryTree(importDir, currentRelative string) ([]*AudioImportDirectoryNode, error) {
	fullDir, _, err := resolveAudioImportRelativePath(importDir, currentRelative, true)
	if err != nil {
		return nil, err
	}
	return buildAudioImportDirectoryNodes(importDir, fullDir)
}

func buildAudioImportDirectoryNodes(importDir, currentDir string) ([]*AudioImportDirectoryNode, error) {
	entries, err := os.ReadDir(currentDir)
	if err != nil {
		return nil, err
	}
	nodes := make([]*AudioImportDirectoryNode, 0)
	for _, entry := range entries {
		name := entry.Name()
		if shouldSkipImportEntry(name, entry) || !entry.IsDir() {
			continue
		}
		fullPath := filepath.Join(currentDir, name)
		relPath, err := filepath.Rel(importDir, fullPath)
		if err != nil {
			return nil, err
		}
		relPath = filepath.ToSlash(filepath.Clean(relPath))
		children, err := buildAudioImportDirectoryNodes(importDir, fullPath)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, &AudioImportDirectoryNode{
			Path:     relPath,
			Name:     name,
			Children: children,
		})
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Name < nodes[j].Name
	})
	return nodes, nil
}

func listAudioImportDirectoryItems(svc *audioService, importDir, fullDir, currentRelative string) ([]AudioImportPreviewItem, error) {
	entries, err := os.ReadDir(fullDir)
	if err != nil {
		return nil, err
	}
	items := make([]AudioImportPreviewItem, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if shouldSkipImportEntry(name, entry) || entry.IsDir() {
			continue
		}
		fullPath := filepath.Join(fullDir, name)
		relName := name
		if currentRelative != "" {
			relName = filepath.ToSlash(filepath.Join(currentRelative, name))
		}
		item := buildAudioImportPreviewItem(svc, fullPath, relName)
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items, nil
}

func countValidAudioImportItems(items []AudioImportPreviewItem) int {
	count := 0
	for _, item := range items {
		if item.Valid {
			count++
		}
	}
	return count
}

func executeAudioImportJob(jobID string, req AudioImportRequest) {
	job := &model.AudioImportJobModel{}
	if err := model.GetDB().Where("id = ?", jobID).First(job).Error; err != nil {
		return
	}
	startedAt := time.Now()
	job.Status = model.AudioImportJobStatusRunning
	job.StartedAt = &startedAt
	_ = persistAudioImportJob(job, nil, nil, nil)

	svc := GetAudioService()
	if svc == nil {
		finishAudioImportJobWithFatal(job, "音频服务未初始化")
		return
	}
	importDir, err := getAudioImportDir(svc)
	if err != nil {
		finishAudioImportJobWithFatal(job, err.Error())
		return
	}
	paths, err := collectRequestedAudioImportPaths(svc, importDir, req)
	if err != nil {
		finishAudioImportJobWithFatal(job, err.Error())
		return
	}
	job.TotalFiles = len(paths)
	_ = persistAudioImportJob(job, nil, nil, nil)
	if len(paths) == 0 {
		finishedAt := time.Now()
		job.Status = model.AudioImportJobStatusDone
		job.FinishedAt = &finishedAt
		_ = persistAudioImportJob(job, []AudioImportResultItem{}, []AudioImportResultItem{}, []AudioImportResultItem{})
		return
	}

	imported := make([]AudioImportResultItem, 0, len(paths))
	failed := make([]AudioImportResultItem, 0)
	skipped := make([]AudioImportResultItem, 0)

	for _, name := range paths {
		fullPath, err := resolveAudioImportPath(importDir, name)
		if err != nil {
			skipped = append(skipped, AudioImportResultItem{Path: name, Name: name, Reason: err.Error()})
			job.SkippedCount++
			job.ProcessedFiles++
			_ = persistAudioImportJob(job, imported, skipped, failed)
			continue
		}
		previewItem := buildAudioImportPreviewItem(svc, fullPath, name)
		if !previewItem.Valid {
			skipped = append(skipped, AudioImportResultItem{
				Path:   name,
				Name:   previewItem.Name,
				Reason: previewItem.Reason,
			})
			job.SkippedCount++
			job.ProcessedFiles++
			_ = persistAudioImportJob(job, imported, skipped, failed)
			continue
		}
		asset, err := AudioCreateAssetFromImport(fullPath, req.Options)
		if err != nil {
			if errors.Is(err, ErrAudioTooLarge) || errors.Is(err, ErrAudioUnsupportedMime) {
				skipped = append(skipped, AudioImportResultItem{
					Path:   name,
					Name:   previewItem.Name,
					Reason: err.Error(),
				})
				job.SkippedCount++
			} else {
				failed = append(failed, AudioImportResultItem{
					Path:  name,
					Name:  previewItem.Name,
					Error: err.Error(),
				})
				job.FailedCount++
			}
			job.ProcessedFiles++
			_ = persistAudioImportJob(job, imported, skipped, failed)
			continue
		}
		item := AudioImportResultItem{
			Path:    name,
			Name:    asset.Name,
			AssetID: asset.ID,
		}
		if err := os.Remove(fullPath); err != nil {
			item.Warning = fmt.Sprintf("导入成功但清理失败: %v", err)
		}
		imported = append(imported, item)
		job.ImportedCount++
		job.ProcessedFiles++
		_ = persistAudioImportJob(job, imported, skipped, failed)
	}

	finishedAt := time.Now()
	job.Status = model.AudioImportJobStatusDone
	job.FinishedAt = &finishedAt
	_ = persistAudioImportJob(job, imported, skipped, failed)
}

func finishAudioImportJobWithFatal(job *model.AudioImportJobModel, message string) {
	finishedAt := time.Now()
	job.Status = model.AudioImportJobStatusFailed
	job.ErrorMessage = strings.TrimSpace(message)
	job.FinishedAt = &finishedAt
	_ = persistAudioImportJob(job, nil, nil, nil)
}

func collectRequestedAudioImportPaths(svc *audioService, importDir string, req AudioImportRequest) ([]string, error) {
	seen := map[string]struct{}{}
	paths := make([]string, 0)
	if req.All {
		fullDir, normalizedDir, err := resolveAudioImportRelativePath(importDir, req.Directory, true)
		if err != nil {
			return nil, err
		}
		items, err := listAudioImportDirectoryItems(svc, importDir, fullDir, normalizedDir)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if _, ok := seen[item.Path]; ok {
				continue
			}
			seen[item.Path] = struct{}{}
			paths = append(paths, item.Path)
		}
		return paths, nil
	}
	for _, raw := range req.Paths {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		paths = append(paths, filepath.ToSlash(filepath.Clean(strings.ReplaceAll(name, "\\", "/"))))
	}
	sort.Strings(paths)
	return paths, nil
}

func persistAudioImportJob(job *model.AudioImportJobModel, imported, skipped, failed []AudioImportResultItem) error {
	if job == nil {
		return nil
	}
	if imported != nil {
		if payload, err := json.Marshal(imported); err == nil {
			job.ImportedJSON = string(payload)
		} else {
			return err
		}
	}
	if skipped != nil {
		if payload, err := json.Marshal(skipped); err == nil {
			job.SkippedJSON = string(payload)
		} else {
			return err
		}
	}
	if failed != nil {
		if payload, err := json.Marshal(failed); err == nil {
			job.FailedJSON = string(payload)
		} else {
			return err
		}
	}
	return model.GetDB().Model(&model.AudioImportJobModel{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
		"status":          job.Status,
		"directory_path":  job.DirectoryPath,
		"total_files":     job.TotalFiles,
		"processed_files": job.ProcessedFiles,
		"imported_count":  job.ImportedCount,
		"skipped_count":   job.SkippedCount,
		"failed_count":    job.FailedCount,
		"error_message":   job.ErrorMessage,
		"imported_json":   job.ImportedJSON,
		"skipped_json":    job.SkippedJSON,
		"failed_json":     job.FailedJSON,
		"started_at":      job.StartedAt,
		"finished_at":     job.FinishedAt,
	}).Error
}

func GetAudioImportJobStatus(jobID string) (*AudioImportJobStatus, error) {
	job := &model.AudioImportJobModel{}
	if err := model.GetDB().Where("id = ?", strings.TrimSpace(jobID)).First(job).Error; err != nil {
		return nil, err
	}
	return audioImportJobModelToStatus(job)
}

func audioImportJobModelToStatus(job *model.AudioImportJobModel) (*AudioImportJobStatus, error) {
	if job == nil {
		return nil, errors.New("导入任务不存在")
	}
	imported, err := decodeAudioImportResultItems(job.ImportedJSON)
	if err != nil {
		return nil, err
	}
	skipped, err := decodeAudioImportResultItems(job.SkippedJSON)
	if err != nil {
		return nil, err
	}
	failed, err := decodeAudioImportResultItems(job.FailedJSON)
	if err != nil {
		return nil, err
	}
	percentage := 0
	if job.TotalFiles > 0 {
		percentage = int(float64(job.ProcessedFiles) / float64(job.TotalFiles) * 100)
		if percentage > 100 {
			percentage = 100
		}
	}
	return &AudioImportJobStatus{
		JobID:          job.ID,
		Status:         job.Status,
		Directory:      job.DirectoryPath,
		TotalFiles:     job.TotalFiles,
		ProcessedFiles: job.ProcessedFiles,
		ImportedCount:  job.ImportedCount,
		SkippedCount:   job.SkippedCount,
		FailedCount:    job.FailedCount,
		ErrorMessage:   job.ErrorMessage,
		Percentage:     percentage,
		Imported:       imported,
		Failed:         failed,
		Skipped:        skipped,
		StartedAt:      job.StartedAt,
		FinishedAt:     job.FinishedAt,
	}, nil
}

func decodeAudioImportResultItems(raw string) ([]AudioImportResultItem, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []AudioImportResultItem{}, nil
	}
	var items []AudioImportResultItem
	if err := json.Unmarshal([]byte(trimmed), &items); err != nil {
		return nil, err
	}
	if items == nil {
		return []AudioImportResultItem{}, nil
	}
	return items, nil
}

func normalizeTrackStates(items []AudioTrackState) []AudioTrackState {
	if items == nil {
		return nil
	}
	result := make([]AudioTrackState, 0, len(items))
	for _, item := range items {
		t := AudioTrackState{
			Type:             strings.TrimSpace(item.Type),
			Volume:           item.Volume,
			Muted:            item.Muted,
			Solo:             item.Solo,
			FadeIn:           item.FadeIn,
			FadeOut:          item.FadeOut,
			IsPlaying:        item.IsPlaying,
			Position:         item.Position,
			LoopEnabled:      item.LoopEnabled,
			PlaybackRate:     item.PlaybackRate,
			PlaylistAssetIDs: append([]string(nil), item.PlaylistAssetIDs...),
			PlaylistIndex:    item.PlaylistIndex,
		}
		if t.PlaybackRate <= 0 {
			t.PlaybackRate = 1
		}
		if t.Position < 0 {
			t.Position = 0
		}
		if t.PlaylistIndex < 0 {
			t.PlaylistIndex = 0
		}
		if item.AssetID != nil {
			trimmed := strings.TrimSpace(*item.AssetID)
			if trimmed != "" {
				val := trimmed
				t.AssetID = &val
			}
		}
		if item.PlaylistFolderID != nil {
			trimmed := strings.TrimSpace(*item.PlaylistFolderID)
			if trimmed != "" {
				val := trimmed
				t.PlaylistFolderID = &val
			}
		}
		if item.PlaylistMode != nil {
			trimmed := strings.TrimSpace(*item.PlaylistMode)
			if trimmed != "" {
				val := trimmed
				t.PlaylistMode = &val
			}
		}
		if len(t.PlaylistAssetIDs) > 0 {
			filtered := make([]string, 0, len(t.PlaylistAssetIDs))
			for _, id := range t.PlaylistAssetIDs {
				trimmed := strings.TrimSpace(id)
				if trimmed != "" {
					filtered = append(filtered, trimmed)
				}
			}
			t.PlaylistAssetIDs = filtered
			if len(t.PlaylistAssetIDs) == 0 {
				t.PlaylistIndex = 0
			} else if t.PlaylistIndex >= len(t.PlaylistAssetIDs) {
				t.PlaylistIndex = len(t.PlaylistAssetIDs) - 1
			}
		} else {
			t.PlaylistIndex = 0
		}
		result = append(result, t)
	}
	return result
}

func playbackScopeKey(scopeType, scopeID string) string {
	return scopeType + ":" + scopeID
}

func resolvePlaybackScope(channelID string, worldPlaybackEnabled bool) (scopeType string, scopeID string, err error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return "", "", errors.New("channelId 必填")
	}
	if !worldPlaybackEnabled {
		return AudioPlaybackScopeChannel, channelID, nil
	}
	channel, getErr := model.ChannelGet(channelID)
	if getErr != nil {
		return "", "", getErr
	}
	if channel == nil || strings.TrimSpace(channel.WorldID) == "" {
		return "", "", errors.New("世界模式缺少 worldId")
	}
	return AudioPlaybackScopeWorld, strings.TrimSpace(channel.WorldID), nil
}

func worldScopeRowID(worldID string) string {
	worldID = strings.TrimSpace(worldID)
	if worldID == "" {
		return ""
	}
	return audioWorldScopeRowPrefix + worldID
}

func calcPlaybackPosition(basePositionSec float64, capturedAtMs int64, isPlaying bool, rate float64, nowMs int64) float64 {
	position := basePositionSec
	if position < 0 {
		position = 0
	}
	if !isPlaying || capturedAtMs <= 0 || nowMs <= capturedAtMs {
		return position
	}
	if rate <= 0 {
		rate = 1
	}
	position += float64(nowMs-capturedAtMs) / 1000 * rate
	if position < 0 {
		return 0
	}
	return position
}

func projectTrackStates(base []AudioTrackState, isPlaying bool, capturedAtMs int64, nowMs int64, fallbackRate float64) []AudioTrackState {
	tracks := append([]AudioTrackState(nil), base...)
	if !isPlaying || capturedAtMs <= 0 || nowMs <= capturedAtMs {
		return tracks
	}
	deltaSec := float64(nowMs-capturedAtMs) / 1000
	for i := range tracks {
		if !tracks[i].IsPlaying || tracks[i].Muted {
			continue
		}
		rate := tracks[i].PlaybackRate
		if rate <= 0 {
			rate = fallbackRate
		}
		if rate <= 0 {
			rate = 1
		}
		tracks[i].Position += deltaSec * rate
		if tracks[i].Position < 0 {
			tracks[i].Position = 0
		}
	}
	return tracks
}

func runtimeToSnapshot(runtime *audioPlaybackRuntimeState, now time.Time) *AudioPlaybackStateSnapshot {
	if runtime == nil {
		return nil
	}
	nowMs := now.UnixMilli()
	tracks := projectTrackStates(runtime.Tracks, runtime.IsPlaying, runtime.CapturedAtMs, nowMs, runtime.PlaybackRate)
	return &AudioPlaybackStateSnapshot{
		ChannelID:            runtime.ChannelID,
		SceneID:              cloneStringPtr(runtime.SceneID),
		Tracks:               tracks,
		IsPlaying:            runtime.IsPlaying,
		Position:             calcPlaybackPosition(runtime.BasePositionSec, runtime.CapturedAtMs, runtime.IsPlaying, runtime.PlaybackRate, nowMs),
		BasePositionSec:      runtime.BasePositionSec,
		CapturedAtMs:         runtime.CapturedAtMs,
		LoopEnabled:          runtime.LoopEnabled,
		PlaybackRate:         runtime.PlaybackRate,
		WorldPlaybackEnabled: runtime.WorldPlaybackEnabled,
		Revision:             runtime.Revision,
		UpdatedBy:            runtime.UpdatedBy,
		UpdatedAt:            runtime.UpdatedAt,
		ScopeType:            runtime.ScopeType,
		ScopeID:              runtime.ScopeID,
	}
}

func modelToRuntimeState(state *model.AudioPlaybackState, scopeType, scopeID string) *audioPlaybackRuntimeState {
	if state == nil {
		return nil
	}
	capturedAtMs := state.CapturedAtMs
	if capturedAtMs <= 0 {
		capturedAtMs = state.UpdatedAt.UnixMilli()
	}
	if capturedAtMs <= 0 {
		capturedAtMs = time.Now().UnixMilli()
	}
	return &audioPlaybackRuntimeState{
		ChannelID:            state.ChannelID,
		SceneID:              cloneStringPtr(state.SceneID),
		Tracks:               normalizeTrackStates([]AudioTrackState(state.Tracks)),
		IsPlaying:            state.IsPlaying,
		BasePositionSec:      state.Position,
		CapturedAtMs:         capturedAtMs,
		LoopEnabled:          state.LoopEnabled,
		PlaybackRate:         state.PlaybackRate,
		WorldPlaybackEnabled: state.WorldPlaybackEnabled,
		Revision:             state.Revision,
		UpdatedBy:            state.UpdatedBy,
		UpdatedAt:            state.UpdatedAt,
		ScopeType:            scopeType,
		ScopeID:              scopeID,
	}
}

func loadPlaybackStateFromDB(channelID string) (*model.AudioPlaybackState, string, string, error) {
	db := model.GetDB()
	var state model.AudioPlaybackState
	err := db.Where("channel_id = ?", channelID).
		Order("updated_at desc").
		Limit(1).
		First(&state).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, "", "", err
	}
	scopeType := AudioPlaybackScopeChannel
	scopeID := channelID
	channel, chErr := model.ChannelGet(channelID)
	if chErr == nil && channel != nil && strings.TrimSpace(channel.WorldID) != "" {
		worldID := strings.TrimSpace(channel.WorldID)
		scopeRowID := worldScopeRowID(worldID)
		if scopeRowID != "" {
			var worldScopeState model.AudioPlaybackState
			scopeErr := db.Where("channel_id = ?", scopeRowID).First(&worldScopeState).Error
			if scopeErr != nil && !errors.Is(scopeErr, gorm.ErrRecordNotFound) {
				return nil, "", "", scopeErr
			}
			if scopeErr == nil {
				if worldScopeState.WorldPlaybackEnabled {
					if err != nil || worldScopeState.UpdatedAt.After(state.UpdatedAt) {
						return &worldScopeState, AudioPlaybackScopeWorld, worldID, nil
					}
				} else {
					if err != nil {
						return nil, scopeType, scopeID, nil
					}
					return &state, scopeType, scopeID, nil
				}
			}
		}
		var worldState model.AudioPlaybackState
		worldErr := db.Table("audio_playback_states AS aps").
			Joins("JOIN channels c ON c.id = aps.channel_id").
			Where("c.world_id = ?", worldID).
			Order("aps.updated_at desc").
			Limit(1).
			First(&worldState).Error
		if worldErr != nil && !errors.Is(worldErr, gorm.ErrRecordNotFound) {
			return nil, "", "", worldErr
		}
		if worldErr == nil && worldState.WorldPlaybackEnabled {
			if err != nil || worldState.UpdatedAt.After(state.UpdatedAt) {
				return &worldState, AudioPlaybackScopeWorld, worldID, nil
			}
		}
	}
	if err != nil {
		return nil, scopeType, scopeID, nil
	}
	return &state, scopeType, scopeID, nil
}

func upsertRuntimeState(scopeType, scopeID string, seeded *audioPlaybackRuntimeState) *audioPlaybackRuntimeState {
	key := playbackScopeKey(scopeType, scopeID)
	audioPlaybackRuntimeStore.Lock()
	defer audioPlaybackRuntimeStore.Unlock()
	existing := audioPlaybackRuntimeStore.states[key]
	if existing != nil {
		return existing
	}
	if seeded != nil {
		audioPlaybackRuntimeStore.states[key] = seeded
		return seeded
	}
	created := &audioPlaybackRuntimeState{
		ScopeType: scopeType,
		ScopeID:   scopeID,
	}
	audioPlaybackRuntimeStore.states[key] = created
	return created
}

func getRuntimeState(scopeType, scopeID string) *audioPlaybackRuntimeState {
	key := playbackScopeKey(scopeType, scopeID)
	audioPlaybackRuntimeStore.RLock()
	defer audioPlaybackRuntimeStore.RUnlock()
	return audioPlaybackRuntimeStore.states[key]
}

func persistPlaybackState(input AudioPlaybackUpdateInput, snapshot *AudioPlaybackStateSnapshot) error {
	if snapshot == nil {
		return nil
	}
	channelID := strings.TrimSpace(input.ChannelID)
	if channelID == "" {
		channelID = strings.TrimSpace(snapshot.ChannelID)
	}
	if snapshot.ScopeType == AudioPlaybackScopeWorld {
		if worldScopeID := strings.TrimSpace(snapshot.ScopeID); worldScopeID != "" {
			channelID = worldScopeRowID(worldScopeID)
		}
	}
	if channelID == "" {
		return nil
	}
	db := model.GetDB()
	var state model.AudioPlaybackState
	err := db.Where("channel_id = ?", channelID).First(&state).Error
	isNew := errors.Is(err, gorm.ErrRecordNotFound)
	if isNew {
		state = model.AudioPlaybackState{
			ChannelID: channelID,
			CreatedAt: time.Now(),
		}
	} else if err != nil {
		return err
	}
	state.SceneID = cloneStringPtr(snapshot.SceneID)
	state.Tracks = model.JSONList[AudioTrackState](normalizeTrackStates(snapshot.Tracks))
	state.IsPlaying = snapshot.IsPlaying
	state.Position = snapshot.BasePositionSec
	state.LoopEnabled = snapshot.LoopEnabled
	state.PlaybackRate = snapshot.PlaybackRate
	state.WorldPlaybackEnabled = snapshot.WorldPlaybackEnabled
	state.Revision = snapshot.Revision
	state.UpdatedBy = snapshot.UpdatedBy
	updatedAt := snapshot.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now()
	}
	state.UpdatedAt = updatedAt
	capturedAtMs := snapshot.CapturedAtMs
	if capturedAtMs <= 0 {
		capturedAtMs = updatedAt.UnixMilli()
	}
	state.CapturedAtMs = capturedAtMs
	return db.Save(&state).Error
}

func persistWorldScopeModeOff(channelID, actorID string, updatedAt time.Time, capturedAtMs int64) error {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil
	}
	channel, err := model.ChannelGet(channelID)
	if err != nil || channel == nil || strings.TrimSpace(channel.WorldID) == "" {
		return err
	}
	scopeRowID := worldScopeRowID(channel.WorldID)
	if scopeRowID == "" {
		return nil
	}
	db := model.GetDB()
	var state model.AudioPlaybackState
	findErr := db.Where("channel_id = ?", scopeRowID).First(&state).Error
	isNew := errors.Is(findErr, gorm.ErrRecordNotFound)
	if isNew {
		state = model.AudioPlaybackState{
			ChannelID: scopeRowID,
			CreatedAt: time.Now(),
			Revision:  0,
		}
	} else if findErr != nil {
		return findErr
	}
	state.SceneID = nil
	state.Tracks = nil
	state.IsPlaying = false
	state.Position = 0
	state.LoopEnabled = false
	state.PlaybackRate = 1
	state.WorldPlaybackEnabled = false
	if state.Revision < 0 {
		state.Revision = 0
	}
	state.Revision += 1
	state.UpdatedBy = actorID
	normalizedUpdatedAt := updatedAt
	if normalizedUpdatedAt.IsZero() {
		normalizedUpdatedAt = time.Now()
	}
	state.UpdatedAt = normalizedUpdatedAt
	normalizedCapturedAtMs := capturedAtMs
	if normalizedCapturedAtMs <= 0 {
		normalizedCapturedAtMs = normalizedUpdatedAt.UnixMilli()
	}
	state.CapturedAtMs = normalizedCapturedAtMs
	return db.Save(&state).Error
}

func AudioGetPlaybackState(channelID string) (*AudioPlaybackStateSnapshot, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, errors.New("channelId 必填")
	}
	var worldRuntime *audioPlaybackRuntimeState
	channel, chErr := model.ChannelGet(channelID)
	if chErr == nil && channel != nil && strings.TrimSpace(channel.WorldID) != "" {
		worldScopeID := strings.TrimSpace(channel.WorldID)
		worldRuntime = getRuntimeState(AudioPlaybackScopeWorld, worldScopeID)
	}
	channelRuntime := getRuntimeState(AudioPlaybackScopeChannel, channelID)
	now := time.Now()
	if worldRuntime != nil && worldRuntime.WorldPlaybackEnabled {
		if channelRuntime == nil || worldRuntime.UpdatedAt.After(channelRuntime.UpdatedAt) {
			return runtimeToSnapshot(worldRuntime, now), nil
		}
	}
	if channelRuntime != nil {
		return runtimeToSnapshot(channelRuntime, now), nil
	}
	if worldRuntime != nil && worldRuntime.WorldPlaybackEnabled {
		return runtimeToSnapshot(worldRuntime, now), nil
	}
	state, scopeType, scopeID, err := loadPlaybackStateFromDB(channelID)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, nil
	}
	runtime := upsertRuntimeState(scopeType, scopeID, modelToRuntimeState(state, scopeType, scopeID))
	return runtimeToSnapshot(runtime, time.Now()), nil
}

func AudioUpsertPlaybackState(input AudioPlaybackUpdateInput) (*AudioPlaybackStateSnapshot, error) {
	input.ChannelID = strings.TrimSpace(input.ChannelID)
	if input.ChannelID == "" {
		return nil, errors.New("channelId 必填")
	}
	if input.PlaybackRate <= 0 {
		input.PlaybackRate = 1
	}
	if input.Position < 0 {
		input.Position = 0
	}
	worldScopeIDForDisable := ""
	if !input.WorldPlaybackEnabled {
		if channel, chErr := model.ChannelGet(input.ChannelID); chErr == nil && channel != nil {
			worldScopeIDForDisable = strings.TrimSpace(channel.WorldID)
		}
	}
	scopeType, scopeID, err := resolvePlaybackScope(input.ChannelID, input.WorldPlaybackEnabled)
	if err != nil {
		return nil, err
	}
	seededRuntime := getRuntimeState(scopeType, scopeID)
	if seededRuntime == nil {
		if persistedState, persistedScopeType, _, loadErr := loadPlaybackStateFromDB(input.ChannelID); loadErr != nil {
			return nil, loadErr
		} else if persistedState != nil {
			seedScopeType := scopeType
			seedScopeID := scopeID
			if persistedScopeType == scopeType {
				seedScopeID = scopeID
			}
			seededRuntime = modelToRuntimeState(persistedState, seedScopeType, seedScopeID)
		}
	}
	runtime := upsertRuntimeState(scopeType, scopeID, seededRuntime)
	audioPlaybackRuntimeStore.Lock()
	if input.BaseRevision > 0 && runtime.Revision > 0 && input.BaseRevision != runtime.Revision {
		current := runtimeToSnapshot(runtime, time.Now())
		audioPlaybackRuntimeStore.Unlock()
		return nil, &AudioPlaybackRevisionConflictError{CurrentState: current}
	}
	now := time.Now()
	capturedAtMs := input.CapturedAtMs
	if capturedAtMs <= 0 {
		capturedAtMs = now.UnixMilli()
	}
	runtime.ChannelID = input.ChannelID
	runtime.SceneID = cloneStringPtr(input.SceneID)
	runtime.Tracks = normalizeTrackStates(input.Tracks)
	runtime.IsPlaying = input.IsPlaying
	runtime.BasePositionSec = input.Position
	runtime.CapturedAtMs = capturedAtMs
	runtime.LoopEnabled = input.LoopEnabled
	runtime.PlaybackRate = input.PlaybackRate
	runtime.WorldPlaybackEnabled = input.WorldPlaybackEnabled
	runtime.ScopeType = scopeType
	runtime.ScopeID = scopeID
	if !input.WorldPlaybackEnabled && worldScopeIDForDisable != "" {
		targetRevision := runtime.Revision + 1
		worldKey := playbackScopeKey(AudioPlaybackScopeWorld, worldScopeIDForDisable)
		worldRuntime := audioPlaybackRuntimeStore.states[worldKey]
		if worldRuntime == nil {
			worldRuntime = &audioPlaybackRuntimeState{
				ScopeType: AudioPlaybackScopeWorld,
				ScopeID:   worldScopeIDForDisable,
			}
			audioPlaybackRuntimeStore.states[worldKey] = worldRuntime
		}
		worldRuntime.ChannelID = input.ChannelID
		worldRuntime.SceneID = nil
		worldRuntime.Tracks = nil
		worldRuntime.IsPlaying = false
		worldRuntime.BasePositionSec = 0
		worldRuntime.CapturedAtMs = capturedAtMs
		worldRuntime.LoopEnabled = false
		worldRuntime.PlaybackRate = 1
		worldRuntime.WorldPlaybackEnabled = false
		if worldRuntime.Revision < targetRevision {
			worldRuntime.Revision = targetRevision
		}
		worldRuntime.UpdatedBy = input.ActorID
		worldRuntime.UpdatedAt = now
	}
	if runtime.Revision < 0 {
		runtime.Revision = 0
	}
	runtime.Revision += 1
	runtime.UpdatedBy = input.ActorID
	runtime.UpdatedAt = now
	snapshot := runtimeToSnapshot(runtime, now)
	audioPlaybackRuntimeStore.Unlock()
	if input.Persist {
		if persistErr := persistPlaybackState(input, snapshot); persistErr != nil {
			return nil, persistErr
		}
		if !input.WorldPlaybackEnabled {
			if persistErr := persistWorldScopeModeOff(input.ChannelID, input.ActorID, snapshot.UpdatedAt, snapshot.CapturedAtMs); persistErr != nil {
				return nil, persistErr
			}
		}
	}
	return snapshot, nil
}

func AudioUpdateAsset(id string, input AudioAssetUpdateInput) (*model.AudioAsset, error) {
	asset, err := AudioGetAsset(id)
	if err != nil {
		return nil, err
	}
	targetScope := asset.Scope
	targetWorldID := cloneStringPtr(asset.WorldID)
	updates := map[string]interface{}{"updated_at": time.Now(), "updated_by": input.UpdatedBy}
	if input.Name != nil {
		updates["name"] = strings.TrimSpace(*input.Name)
		asset.Name = strings.TrimSpace(*input.Name)
	}
	if input.Description != nil {
		updates["description"] = strings.TrimSpace(*input.Description)
		asset.Description = strings.TrimSpace(*input.Description)
	}
	if input.Visibility != nil {
		updates["visibility"] = *input.Visibility
		asset.Visibility = *input.Visibility
	}
	if input.Tags != nil {
		updates["tags"] = model.JSONList[string](normalizeTags(input.Tags))
		asset.Tags = model.JSONList[string](normalizeTags(input.Tags))
	}
	if input.Scope != nil {
		scope := *input.Scope
		switch scope {
		case model.AudioScopeCommon:
			targetScope = scope
			targetWorldID = nil
			updates["scope"] = scope
			updates["world_id"] = nil
			asset.Scope = scope
			asset.WorldID = nil
		case model.AudioScopeWorld:
			worldID := ""
			if input.WorldID != nil {
				worldID = strings.TrimSpace(*input.WorldID)
			}
			if worldID == "" {
				return nil, errors.New("世界级素材必须指定 worldId")
			}
			targetScope = scope
			targetWorldID = &worldID
			updates["scope"] = scope
			updates["world_id"] = worldID
			asset.Scope = scope
			asset.WorldID = &worldID
		default:
			return nil, errors.New("素材级别无效")
		}
	}
	if input.FolderID != nil {
		trimmed := strings.TrimSpace(*input.FolderID)
		if trimmed != "" {
			folder, err := getAudioFolder(trimmed)
			if err != nil {
				return nil, err
			}
			if err := validateFolderScopeMatch(folder, targetScope, targetWorldID); err != nil {
				return nil, err
			}
			updates["folder_id"] = trimmed
			asset.FolderID = cloneStringPtr(&trimmed)
		} else {
			updates["folder_id"] = nil
			asset.FolderID = nil
		}
	} else if input.Scope != nil && asset.FolderID != nil {
		trimmed := strings.TrimSpace(*asset.FolderID)
		if trimmed != "" {
			folder, err := getAudioFolder(trimmed)
			if err != nil {
				return nil, err
			}
			if err := validateFolderScopeMatch(folder, targetScope, targetWorldID); err != nil {
				return nil, err
			}
		}
	}
	if len(input.Variants) > 0 {
		updates["variants"] = model.JSONList[model.AudioAssetVariant](input.Variants)
		asset.Variants = model.JSONList[model.AudioAssetVariant](input.Variants)
	}
	if err := model.GetDB().Model(asset).Updates(updates).Error; err != nil {
		return nil, err
	}
	return asset, nil
}

func AudioReorderAssets(assetIDs []string, movedAssetIDs []string, actorID string, isSystemAdmin bool) ([]*model.AudioAsset, error) {
	if len(assetIDs) == 0 {
		return []*model.AudioAsset{}, nil
	}
	cleaned := make([]string, 0, len(assetIDs))
	seen := map[string]struct{}{}
	for _, rawID := range assetIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		cleaned = append(cleaned, id)
	}
	if len(cleaned) == 0 {
		return []*model.AudioAsset{}, nil
	}
	movedSet := map[string]struct{}{}
	for _, rawID := range movedAssetIDs {
		id := strings.TrimSpace(rawID)
		if id != "" {
			movedSet[id] = struct{}{}
		}
	}
	if len(movedSet) == 0 && len(cleaned) > 0 {
		movedSet[cleaned[0]] = struct{}{}
	}

	assets := make([]*model.AudioAsset, 0, len(cleaned))
	assetByID := map[string]*model.AudioAsset{}
	if err := model.GetDB().Where("id IN ? AND deleted_at IS NULL", cleaned).Find(&assets).Error; err != nil {
		return nil, err
	}
	if len(assets) != len(cleaned) {
		return nil, gorm.ErrRecordNotFound
	}
	for _, asset := range assets {
		ok, err := audioManageAssetInScope(actorID, asset, isSystemAdmin)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, gorm.ErrRecordNotFound
		}
		assetByID[asset.ID] = asset
	}

	baseSortOrder := 1000
	for index, id := range cleaned {
		asset := assetByID[id]
		if asset == nil || asset.SortOrder <= 0 {
			continue
		}
		if index == 0 || asset.SortOrder < baseSortOrder {
			baseSortOrder = asset.SortOrder
		}
	}
	now := time.Now()
	if err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		for index, id := range cleaned {
			if _, ok := assetByID[id]; !ok {
				return gorm.ErrRecordNotFound
			}
			sortOrder := baseSortOrder + index*1000
			_, manualSorted := movedSet[id]
			if err := tx.Model(&model.AudioAsset{}).
				Where("id = ?", id).
				Updates(map[string]interface{}{
					"sort_order":    sortOrder,
					"manual_sorted": manualSorted || assetByID[id].ManualSorted,
					"updated_by":    actorID,
					"updated_at":    now,
				}).Error; err != nil {
				return err
			}
			assetByID[id].SortOrder = sortOrder
			assetByID[id].ManualSorted = manualSorted || assetByID[id].ManualSorted
			assetByID[id].UpdatedBy = actorID
			assetByID[id].UpdatedAt = now
		}
		return nil
	}); err != nil {
		return nil, err
	}

	ordered := make([]*model.AudioAsset, 0, len(cleaned))
	for _, id := range cleaned {
		ordered = append(ordered, assetByID[id])
	}
	return ordered, nil
}

func AudioDeleteAsset(id string, hard bool) error {
	asset, err := AudioGetAsset(id)
	if err != nil {
		return err
	}
	audioDeleteAssetObjects(asset)
	return audioDeleteAssetRecordTx(model.GetDB(), id, hard)
}

func AudioTouchAssetAccess(assetID string) error {
	trimmed := strings.TrimSpace(assetID)
	if trimmed == "" {
		return errors.New("asset id is empty")
	}
	now := time.Now()
	return model.GetDB().Model(&model.AudioAsset{}).
		Where("id = ? AND deleted_at IS NULL", trimmed).
		Updates(map[string]interface{}{
			"last_accessed_at": &now,
			"access_count":     gorm.Expr("access_count + 1"),
			"updated_at":       now,
		}).Error
}

func AudioGetAssetUsageSummary(assetID string) (AudioAssetUsageSummary, error) {
	trimmed := strings.TrimSpace(assetID)
	if trimmed == "" {
		return AudioAssetUsageSummary{}, errors.New("asset id is empty")
	}

	summary := AudioAssetUsageSummary{}
	var scenes []*model.AudioScene
	if err := model.GetDB().Find(&scenes).Error; err != nil {
		return summary, err
	}
	for _, scene := range scenes {
		if scene == nil {
			continue
		}
		if !sceneReferencesAsset(scene.Tracks, trimmed) {
			continue
		}
		summary.SceneRefCount++
		if name := strings.TrimSpace(scene.Name); name != "" {
			summary.SceneNames = append(summary.SceneNames, name)
		}
	}

	var playbackStates []*model.AudioPlaybackState
	if err := model.GetDB().Find(&playbackStates).Error; err != nil {
		return summary, err
	}
	for _, state := range playbackStates {
		if state == nil {
			continue
		}
		if !playbackStateReferencesAsset(state.Tracks, trimmed) {
			continue
		}
		summary.PlaybackStateRefCount++
		label := strings.TrimSpace(state.ChannelID)
		if label != "" {
			summary.PlaybackScopeLabels = append(summary.PlaybackScopeLabels, label)
		}
	}

	summary.Referenced = summary.SceneRefCount > 0 || summary.PlaybackStateRefCount > 0
	return summary, nil
}

func AudioSafeDeleteAsset(id string, hard bool) error {
	summary, err := AudioGetAssetUsageSummary(id)
	if err != nil {
		return err
	}
	if summary.Referenced {
		return &AudioAssetReferencedError{Summary: summary}
	}
	return AudioDeleteAsset(id, hard)
}

func AudioSafeDeleteAssets(ids []string, hard bool) (*AudioBulkDeleteResult, error) {
	result := &AudioBulkDeleteResult{
		SuccessIDs: make([]string, 0, len(ids)),
		Failed:     make([]AudioBulkDeleteFailure, 0),
	}
	seen := map[string]struct{}{}
	for _, rawID := range ids {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if err := AudioSafeDeleteAsset(id, hard); err != nil {
			failure := AudioBulkDeleteFailure{
				AssetID: id,
				Reason:  err.Error(),
			}
			var referencedErr *AudioAssetReferencedError
			if errors.As(err, &referencedErr) {
				summary := referencedErr.Summary
				failure.UsageSummary = &summary
				failure.Reason = "素材仍被引用，无法安全删除"
			}
			result.Failed = append(result.Failed, failure)
			continue
		}
		result.SuccessIDs = append(result.SuccessIDs, id)
	}
	result.SuccessCount = len(result.SuccessIDs)
	result.FailedCount = len(result.Failed)
	return result, nil
}

func AdminAudioDeleteAsset(id string, hard bool) (*AudioDeleteImpact, error) {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return nil, errors.New("asset id is empty")
	}
	var deletedAsset *model.AudioAsset
	impact := &AudioDeleteImpact{}
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		asset, sceneImpact, playbackImpact, txErr := audioDetachAndDeleteAssetTx(tx, trimmed, hard)
		if txErr != nil {
			return txErr
		}
		deletedAsset = asset
		impact = mergeDeleteImpact(sceneImpact, playbackImpact)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if deletedAsset != nil {
		audioDeleteAssetObjects(deletedAsset)
	}
	return impact, nil
}

func AdminAudioDeleteAssets(ids []string, hard bool) (*AudioBulkDeleteResult, error) {
	result := &AudioBulkDeleteResult{
		SuccessIDs: make([]string, 0, len(ids)),
		Failed:     make([]AudioBulkDeleteFailure, 0),
	}
	seen := map[string]struct{}{}
	for _, rawID := range ids {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		impact, err := AdminAudioDeleteAsset(id, hard)
		if err != nil {
			failure := AudioBulkDeleteFailure{
				AssetID: id,
				Reason:  err.Error(),
			}
			var referencedErr *AudioAssetReferencedError
			if errors.As(err, &referencedErr) {
				summary := referencedErr.Summary
				failure.UsageSummary = &summary
			}
			result.Failed = append(result.Failed, failure)
			continue
		}
		result.SuccessIDs = append(result.SuccessIDs, id)
		if impact != nil {
			result.DetachedSceneCount += impact.DetachedSceneCount
			result.DetachedPlaybackStateCount += impact.DetachedPlaybackStateCount
			if impact.DetachedSceneCount > 0 || impact.DetachedPlaybackStateCount > 0 {
				result.DetachedReferencedAssetCount++
			}
			if len(impact.PlaybackScopeLabels) > 0 {
				result.PlaybackScopeLabels = appendUniqueStrings(result.PlaybackScopeLabels, impact.PlaybackScopeLabels...)
			}
		}
	}
	result.SuccessCount = len(result.SuccessIDs)
	result.FailedCount = len(result.Failed)
	return result, nil
}

func audioDetachAndDeleteAssetTx(tx *gorm.DB, id string, hard bool) (*model.AudioAsset, *AudioDeleteImpact, *AudioDeleteImpact, error) {
	var asset model.AudioAsset
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND deleted_at IS NULL", id).
		First(&asset).Error; err != nil {
		return nil, nil, nil, err
	}
	sceneImpact, err := detachAssetFromScenesTx(tx, asset.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	playbackImpact, err := detachAssetFromPlaybackStatesTx(tx, asset.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	summary, err := audioGetAssetUsageSummaryTx(tx, asset.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	if summary.Referenced {
		return nil, nil, nil, &AudioAssetReferencedError{Summary: summary}
	}
	if err := audioDeleteAssetRecordTx(tx, asset.ID, hard); err != nil {
		return nil, nil, nil, err
	}
	return &asset, sceneImpact, playbackImpact, nil
}

func audioDeleteAssetRecordTx(tx *gorm.DB, id string, hard bool) error {
	if hard {
		return tx.Unscoped().Delete(&model.AudioAsset{}, "id = ?", id).Error
	}
	return tx.Model(&model.AudioAsset{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{"deleted_at": time.Now()}).Error
}

func audioDeleteAssetObjects(asset *model.AudioAsset) {
	if asset == nil {
		return
	}
	svc := GetAudioService()
	if svc == nil {
		return
	}
	svc.removeAssetObject(asset.StorageType, asset.ObjectKey)
	for _, variant := range asset.Variants {
		svc.removeAssetObject(variant.StorageType, variant.ObjectKey)
	}
}

func mergeDeleteImpact(sceneImpact, playbackImpact *AudioDeleteImpact) *AudioDeleteImpact {
	result := &AudioDeleteImpact{}
	if sceneImpact != nil {
		result.DetachedSceneCount += sceneImpact.DetachedSceneCount
		result.SceneNames = append(result.SceneNames, sceneImpact.SceneNames...)
	}
	if playbackImpact != nil {
		result.DetachedPlaybackStateCount += playbackImpact.DetachedPlaybackStateCount
		result.PlaybackScopeLabels = append(result.PlaybackScopeLabels, playbackImpact.PlaybackScopeLabels...)
	}
	return result
}

func detachAssetFromScenesTx(tx *gorm.DB, assetID string) (*AudioDeleteImpact, error) {
	impact := &AudioDeleteImpact{}
	var scenes []*model.AudioScene
	if err := tx.Find(&scenes).Error; err != nil {
		return nil, err
	}
	for _, scene := range scenes {
		if scene == nil {
			continue
		}
		updatedTracks, changed := detachAssetFromSceneTracks(scene.Tracks, assetID)
		if !changed {
			continue
		}
		scene.Tracks = updatedTracks
		scene.UpdatedAt = time.Now()
		if err := tx.Model(&model.AudioScene{}).Where("id = ?", scene.ID).Updates(map[string]interface{}{
			"tracks":      scene.Tracks,
			"updated_at":  scene.UpdatedAt,
			"description": scene.Description,
		}).Error; err != nil {
			return nil, err
		}
		impact.DetachedSceneCount++
		if name := strings.TrimSpace(scene.Name); name != "" {
			impact.SceneNames = append(impact.SceneNames, name)
		}
	}
	return impact, nil
}

func detachAssetFromPlaybackStatesTx(tx *gorm.DB, assetID string) (*AudioDeleteImpact, error) {
	impact := &AudioDeleteImpact{}
	var states []*model.AudioPlaybackState
	if err := tx.Find(&states).Error; err != nil {
		return nil, err
	}
	for _, state := range states {
		if state == nil {
			continue
		}
		updatedTracks, changed := detachAssetFromPlaybackTracks(state.Tracks, assetID)
		if !changed {
			continue
		}
		state.Tracks = updatedTracks
		if allPlaybackTracksIdle(state.Tracks) {
			state.IsPlaying = false
			state.Position = 0
		}
		state.UpdatedAt = time.Now()
		state.Revision += 1
		if state.CapturedAtMs <= 0 {
			state.CapturedAtMs = state.UpdatedAt.UnixMilli()
		} else {
			state.CapturedAtMs = state.UpdatedAt.UnixMilli()
		}
		if err := tx.Model(&model.AudioPlaybackState{}).Where("channel_id = ?", state.ChannelID).Updates(map[string]interface{}{
			"tracks":                 state.Tracks,
			"is_playing":             state.IsPlaying,
			"position":               state.Position,
			"updated_at":             state.UpdatedAt,
			"revision":               state.Revision,
			"captured_at_ms":         state.CapturedAtMs,
			"loop_enabled":           state.LoopEnabled,
			"playback_rate":          state.PlaybackRate,
			"updated_by":             state.UpdatedBy,
			"scene_id":               state.SceneID,
			"world_playback_enabled": state.WorldPlaybackEnabled,
		}).Error; err != nil {
			return nil, err
		}
		syncPlaybackRuntimeState(state)
		impact.DetachedPlaybackStateCount++
		if label := strings.TrimSpace(state.ChannelID); label != "" {
			impact.PlaybackScopeLabels = append(impact.PlaybackScopeLabels, label)
		}
	}
	return impact, nil
}

func audioGetAssetUsageSummaryTx(tx *gorm.DB, assetID string) (AudioAssetUsageSummary, error) {
	trimmed := strings.TrimSpace(assetID)
	if trimmed == "" {
		return AudioAssetUsageSummary{}, errors.New("asset id is empty")
	}
	summary := AudioAssetUsageSummary{}
	var scenes []*model.AudioScene
	if err := tx.Find(&scenes).Error; err != nil {
		return summary, err
	}
	for _, scene := range scenes {
		if scene == nil || !sceneReferencesAsset(scene.Tracks, trimmed) {
			continue
		}
		summary.SceneRefCount++
		if name := strings.TrimSpace(scene.Name); name != "" {
			summary.SceneNames = append(summary.SceneNames, name)
		}
	}
	var playbackStates []*model.AudioPlaybackState
	if err := tx.Find(&playbackStates).Error; err != nil {
		return summary, err
	}
	for _, state := range playbackStates {
		if state == nil || !playbackStateReferencesAsset(state.Tracks, trimmed) {
			continue
		}
		summary.PlaybackStateRefCount++
		if label := strings.TrimSpace(state.ChannelID); label != "" {
			summary.PlaybackScopeLabels = append(summary.PlaybackScopeLabels, label)
		}
	}
	summary.Referenced = summary.SceneRefCount > 0 || summary.PlaybackStateRefCount > 0
	return summary, nil
}

func detachAssetFromSceneTracks(tracks model.JSONList[model.AudioSceneTrack], assetID string) (model.JSONList[model.AudioSceneTrack], bool) {
	result := make([]model.AudioSceneTrack, 0, len(tracks))
	changed := false
	for _, track := range tracks {
		next := track
		if next.AssetID != nil && strings.TrimSpace(*next.AssetID) == assetID {
			next.AssetID = nil
			changed = true
		}
		filtered, removed := removeAssetIDFromList(next.PlaylistAssetIDs, assetID)
		if removed {
			next.PlaylistAssetIDs = filtered
			changed = true
		}
		if len(next.PlaylistAssetIDs) == 0 {
			next.PlaylistIndex = 0
		} else if next.PlaylistIndex >= len(next.PlaylistAssetIDs) {
			next.PlaylistIndex = len(next.PlaylistAssetIDs) - 1
		}
		result = append(result, next)
	}
	return model.JSONList[model.AudioSceneTrack](result), changed
}

func detachAssetFromPlaybackTracks(tracks model.JSONList[model.AudioTrackState], assetID string) (model.JSONList[model.AudioTrackState], bool) {
	result := make([]model.AudioTrackState, 0, len(tracks))
	changed := false
	for _, track := range tracks {
		next := track
		directDetached := false
		if next.AssetID != nil && strings.TrimSpace(*next.AssetID) == assetID {
			next.AssetID = nil
			directDetached = true
			changed = true
		}
		filtered, removed := removeAssetIDFromList(next.PlaylistAssetIDs, assetID)
		if removed {
			next.PlaylistAssetIDs = filtered
			changed = true
		}
		if len(next.PlaylistAssetIDs) == 0 {
			next.PlaylistIndex = 0
		} else if next.PlaylistIndex >= len(next.PlaylistAssetIDs) {
			next.PlaylistIndex = len(next.PlaylistAssetIDs) - 1
		}
		if directDetached || (next.AssetID == nil && len(next.PlaylistAssetIDs) == 0) {
			next.IsPlaying = false
			next.Position = 0
		}
		result = append(result, next)
	}
	return model.JSONList[model.AudioTrackState](result), changed
}

func removeAssetIDFromList(ids []string, assetID string) ([]string, bool) {
	if len(ids) == 0 {
		return ids, false
	}
	filtered := make([]string, 0, len(ids))
	removed := false
	for _, id := range ids {
		if strings.TrimSpace(id) == assetID {
			removed = true
			continue
		}
		filtered = append(filtered, id)
	}
	return filtered, removed
}

func allPlaybackTracksIdle(tracks model.JSONList[model.AudioTrackState]) bool {
	for _, track := range tracks {
		if track.IsPlaying && track.AssetID != nil && strings.TrimSpace(*track.AssetID) != "" {
			return false
		}
	}
	return true
}

func syncPlaybackRuntimeState(state *model.AudioPlaybackState) {
	if state == nil {
		return
	}
	scopeType := AudioPlaybackScopeChannel
	scopeID := strings.TrimSpace(state.ChannelID)
	if strings.HasPrefix(scopeID, audioWorldScopeRowPrefix) {
		scopeType = AudioPlaybackScopeWorld
		scopeID = strings.TrimPrefix(scopeID, audioWorldScopeRowPrefix)
	}
	audioPlaybackRuntimeStore.Lock()
	audioPlaybackRuntimeStore.states[playbackScopeKey(scopeType, scopeID)] = modelToRuntimeState(state, scopeType, scopeID)
	audioPlaybackRuntimeStore.Unlock()
}

func appendUniqueStrings(base []string, values ...string) []string {
	if len(values) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base))
	for _, item := range base {
		seen[item] = struct{}{}
	}
	for _, item := range values {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		base = append(base, trimmed)
	}
	return base
}

func AdminAudioListAssets(filters AdminAudioAssetFilters) (*AdminAudioAssetListResult, error) {
	return listAudioAssetManagementItems(filters, nil)
}

func listAudioAssetManagementItems(filters AdminAudioAssetFilters, applyScope func(*gorm.DB) *gorm.DB) (*AdminAudioAssetListResult, error) {
	filters.normalize()
	db := model.GetDB()
	q := db.Model(&model.AudioAsset{}).Where("deleted_at IS NULL")
	if applyScope != nil {
		q = applyScope(q)
	}
	if filters.Scope != "" {
		q = q.Where("scope = ?", filters.Scope)
	}
	if filters.WorldID != nil && strings.TrimSpace(*filters.WorldID) != "" {
		q = q.Where("world_id = ?", strings.TrimSpace(*filters.WorldID))
	}
	if filters.CreatorID != nil && strings.TrimSpace(*filters.CreatorID) != "" {
		q = q.Where("created_by = ?", strings.TrimSpace(*filters.CreatorID))
	}
	if filters.NeverAccessed != nil {
		if *filters.NeverAccessed {
			q = q.Where("last_accessed_at IS NULL")
		} else {
			q = q.Where("last_accessed_at IS NOT NULL")
		}
	}
	if filters.InactiveDays > 0 {
		threshold := time.Now().Add(-time.Duration(filters.InactiveDays) * 24 * time.Hour)
		q = q.Where("(last_accessed_at IS NULL OR last_accessed_at < ?)", threshold)
	}

	var assets []*model.AudioAsset
	if err := q.Order("updated_at DESC").Find(&assets).Error; err != nil {
		return nil, err
	}
	items, err := buildAdminAudioAssetItems(assets)
	if err != nil {
		return nil, err
	}
	items = filterAdminAudioAssetItemsByQuery(items, filters.Query, filters.QueryField)
	if filters.Referenced != nil {
		filtered := make([]AdminAudioAssetListItem, 0, len(items))
		for _, item := range items {
			if item.UsageSummary.Referenced == *filters.Referenced {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	sortAdminAudioAssetItems(items, filters.SortBy, filters.SortOrder)
	total := int64(len(items))
	start := (filters.Page - 1) * filters.PageSize
	if start > len(items) {
		start = len(items)
	}
	end := start + filters.PageSize
	if end > len(items) {
		end = len(items)
	}
	worldOptions, creatorOptions, err := buildAdminAudioFilterOptions(assets)
	if err != nil {
		return nil, err
	}
	return &AdminAudioAssetListResult{
		Items:          items[start:end],
		Page:           filters.Page,
		PageSize:       filters.PageSize,
		Total:          total,
		WorldOptions:   worldOptions,
		CreatorOptions: creatorOptions,
	}, nil
}

func AudioManageListAssets(filters AudioManageAssetFilters) (*AudioManageAssetListResult, error) {
	actorID := strings.TrimSpace(filters.ActorID)
	if actorID == "" {
		return nil, errors.New("用户ID不能为空")
	}
	var managedWorldIDs []string
	var err error
	if !filters.IsSystemAdmin {
		managedWorldIDs, err = ListManagedWorldIDs(actorID)
		if err != nil {
			return nil, err
		}
	}
	result, err := listAudioAssetManagementItems(filters.AdminAudioAssetFilters, func(q *gorm.DB) *gorm.DB {
		if filters.IsSystemAdmin {
			return q
		}
		if len(managedWorldIDs) == 0 {
			return q.Where("created_by = ?", actorID)
		}
		return q.Where("(created_by = ? OR (scope = ? AND world_id IN ?))", actorID, model.AudioScopeWorld, managedWorldIDs)
	})
	if err != nil {
		return nil, err
	}
	quota, err := GetAudioQuotaSummary(actorID)
	if err != nil {
		return nil, err
	}
	return &AudioManageAssetListResult{
		AdminAudioAssetListResult: *result,
		Quota:                     quota,
	}, nil
}

func AudioManageGetAssetUsage(actorID, assetID string, isSystemAdmin bool) (*AdminAudioAssetListItem, error) {
	asset, err := audioManageGetScopedAsset(actorID, assetID, isSystemAdmin)
	if err != nil {
		return nil, err
	}
	items, err := buildAdminAudioAssetItems([]*model.AudioAsset{asset})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &items[0], nil
}

func AudioManageDeleteAsset(actorID, assetID string, isSystemAdmin bool, hard bool) (*AudioDeleteImpact, error) {
	if _, err := audioManageGetScopedAsset(actorID, assetID, isSystemAdmin); err != nil {
		return nil, err
	}
	return AdminAudioDeleteAsset(assetID, hard)
}

func AudioManageDeleteAssets(actorID string, assetIDs []string, isSystemAdmin bool, hard bool) (*AudioBulkDeleteResult, error) {
	scopedIDs := make([]string, 0, len(assetIDs))
	result := &AudioBulkDeleteResult{
		SuccessIDs: make([]string, 0, len(assetIDs)),
		Failed:     make([]AudioBulkDeleteFailure, 0),
	}
	seen := map[string]struct{}{}
	for _, rawID := range assetIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if _, err := audioManageGetScopedAsset(actorID, id, isSystemAdmin); err != nil {
			result.Failed = append(result.Failed, AudioBulkDeleteFailure{
				AssetID: id,
				Reason:  "素材不存在或无权管理",
			})
			continue
		}
		scopedIDs = append(scopedIDs, id)
	}
	if len(scopedIDs) > 0 {
		deleted, err := AdminAudioDeleteAssets(scopedIDs, hard)
		if err != nil {
			return nil, err
		}
		result.SuccessIDs = append(result.SuccessIDs, deleted.SuccessIDs...)
		result.Failed = append(result.Failed, deleted.Failed...)
		result.DetachedSceneCount += deleted.DetachedSceneCount
		result.DetachedPlaybackStateCount += deleted.DetachedPlaybackStateCount
		result.DetachedReferencedAssetCount += deleted.DetachedReferencedAssetCount
		result.PlaybackScopeLabels = appendUniqueStrings(result.PlaybackScopeLabels, deleted.PlaybackScopeLabels...)
	}
	result.SuccessCount = len(result.SuccessIDs)
	result.FailedCount = len(result.Failed)
	return result, nil
}

func audioManageGetScopedAsset(actorID, assetID string, isSystemAdmin bool) (*model.AudioAsset, error) {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return nil, errors.New("用户ID不能为空")
	}
	asset, err := AudioGetAsset(assetID)
	if err != nil {
		return nil, err
	}
	ok, err := audioManageAssetInScope(actorID, asset, isSystemAdmin)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return asset, nil
}

func audioManageAssetInScope(actorID string, asset *model.AudioAsset, isSystemAdmin bool) (bool, error) {
	if isSystemAdmin {
		return true, nil
	}
	if asset == nil {
		return false, nil
	}
	if strings.TrimSpace(asset.CreatedBy) == actorID {
		return true, nil
	}
	if asset.Scope != model.AudioScopeWorld {
		return false, nil
	}
	worldID := strings.TrimSpace(normalizeOptionalString(asset.WorldID))
	if worldID == "" {
		return false, nil
	}
	return IsWorldAdmin(worldID, actorID), nil
}

func normalizeAdminAudioQueryField(value string) string {
	switch strings.TrimSpace(value) {
	case "", "all":
		return "all"
	case "name", "worldName", "creatorName", "scope":
		return strings.TrimSpace(value)
	default:
		return "all"
	}
}

func normalizeAdminAudioSortField(value string) string {
	switch strings.TrimSpace(value) {
	case "", "updatedAt":
		return "updatedAt"
	case "name", "scope", "worldName", "creatorName", "size", "lastAccessedAt":
		return strings.TrimSpace(value)
	default:
		return "updatedAt"
	}
}

func normalizeAdminAudioSortOrder(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "asc":
		return "asc"
	case "desc", "":
		return "desc"
	default:
		return "desc"
	}
}

func filterAdminAudioAssetItemsByQuery(items []AdminAudioAssetListItem, query, queryField string) []AdminAudioAssetListItem {
	keyword := strings.ToLower(strings.TrimSpace(query))
	if keyword == "" {
		return items
	}
	filtered := make([]AdminAudioAssetListItem, 0, len(items))
	for _, item := range items {
		if adminAudioItemMatchesQuery(item, keyword, queryField) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func adminAudioItemMatchesQuery(item AdminAudioAssetListItem, keyword, queryField string) bool {
	values := adminAudioItemQueryValues(item, queryField)
	for _, value := range values {
		if strings.Contains(strings.ToLower(strings.TrimSpace(value)), keyword) {
			return true
		}
	}
	return false
}

func adminAudioItemQueryValues(item AdminAudioAssetListItem, queryField string) []string {
	switch queryField {
	case "name":
		return []string{item.Name, item.Description, strings.Join(item.Tags, " ")}
	case "worldName":
		return []string{item.WorldName, normalizeOptionalString(item.WorldID)}
	case "creatorName":
		return []string{item.CreatorName, item.CreatedBy}
	case "scope":
		return []string{string(item.Scope)}
	default:
		return []string{
			item.Name,
			item.Description,
			strings.Join(item.Tags, " "),
			item.WorldName,
			normalizeOptionalString(item.WorldID),
			item.CreatorName,
			item.CreatedBy,
			string(item.Scope),
		}
	}
}

func sortAdminAudioAssetItems(items []AdminAudioAssetListItem, sortBy, sortOrder string) {
	desc := sortOrder != "asc"
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i]
		right := items[j]
		var less bool
		switch sortBy {
		case "name":
			less = strings.ToLower(left.Name) < strings.ToLower(right.Name)
		case "scope":
			less = strings.ToLower(string(left.Scope)) < strings.ToLower(string(right.Scope))
		case "worldName":
			less = strings.ToLower(left.WorldName) < strings.ToLower(right.WorldName)
		case "creatorName":
			less = strings.ToLower(left.CreatorName) < strings.ToLower(right.CreatorName)
		case "size":
			less = left.Size < right.Size
		case "lastAccessedAt":
			less = compareOptionalTime(left.LastAccessedAt, right.LastAccessedAt)
		case "updatedAt":
			fallthrough
		default:
			less = left.UpdatedAt.Before(right.UpdatedAt)
		}
		if desc {
			return !less && !adminAudioItemEqualOnSort(left, right, sortBy)
		}
		return less && !adminAudioItemEqualOnSort(left, right, sortBy)
	})
}

func adminAudioItemEqualOnSort(left, right AdminAudioAssetListItem, sortBy string) bool {
	switch sortBy {
	case "name":
		return strings.EqualFold(left.Name, right.Name)
	case "scope":
		return strings.EqualFold(string(left.Scope), string(right.Scope))
	case "worldName":
		return strings.EqualFold(left.WorldName, right.WorldName)
	case "creatorName":
		return strings.EqualFold(left.CreatorName, right.CreatorName)
	case "size":
		return left.Size == right.Size
	case "lastAccessedAt":
		return optionalTimeEqual(left.LastAccessedAt, right.LastAccessedAt)
	case "updatedAt":
		fallthrough
	default:
		return left.UpdatedAt.Equal(right.UpdatedAt)
	}
}

func compareOptionalTime(left, right *time.Time) bool {
	if left == nil && right == nil {
		return false
	}
	if left == nil {
		return true
	}
	if right == nil {
		return false
	}
	return left.Before(*right)
}

func optionalTimeEqual(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func AdminAudioPreviewUnusedAssets(days int, filters AdminAudioAssetFilters) (*AdminAudioCleanupPreview, error) {
	filters.InactiveDays = days
	filters.Page = 1
	if filters.PageSize <= 0 {
		filters.PageSize = 100
	}
	list, err := AdminAudioListAssets(filters)
	if err != nil {
		return nil, err
	}
	preview := &AdminAudioCleanupPreview{
		ThresholdBefore: time.Now().Add(-time.Duration(days) * 24 * time.Hour),
		Items:           make([]AdminAudioAssetListItem, 0),
	}
	for _, item := range list.Items {
		preview.TotalCandidates++
		preview.Items = append(preview.Items, item)
		if item.SafeToDelete {
			preview.SafeCandidates++
			preview.DirectDeleteCandidates++
		} else {
			preview.ReferencedSkipped++
			preview.DetachThenDeleteCandidates++
		}
	}
	return preview, nil
}

func AdminAudioCleanupUnusedAssets(days int, filters AdminAudioAssetFilters, hard bool) (*AudioBulkDeleteResult, error) {
	filters.InactiveDays = days
	filters.Page = 1
	if filters.PageSize <= 0 {
		filters.PageSize = 1000
	}
	list, err := AdminAudioListAssets(filters)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(list.Items))
	for _, item := range list.Items {
		ids = append(ids, item.ID)
	}
	return AdminAudioDeleteAssets(ids, hard)
}

func buildAdminAudioAssetItems(assets []*model.AudioAsset) ([]AdminAudioAssetListItem, error) {
	if len(assets) == 0 {
		return []AdminAudioAssetListItem{}, nil
	}
	worldIDs := make([]string, 0)
	userIDs := make([]string, 0)
	worldSeen := map[string]struct{}{}
	userSeen := map[string]struct{}{}
	for _, asset := range assets {
		if asset == nil {
			continue
		}
		if asset.WorldID != nil {
			worldID := strings.TrimSpace(*asset.WorldID)
			if worldID != "" {
				if _, ok := worldSeen[worldID]; !ok {
					worldSeen[worldID] = struct{}{}
					worldIDs = append(worldIDs, worldID)
				}
			}
		}
		userID := strings.TrimSpace(asset.CreatedBy)
		if userID != "" {
			if _, ok := userSeen[userID]; !ok {
				userSeen[userID] = struct{}{}
				userIDs = append(userIDs, userID)
			}
		}
	}

	worldNames := map[string]string{}
	if len(worldIDs) > 0 {
		var worlds []*model.WorldModel
		if err := model.GetDB().Where("id IN ?", worldIDs).Find(&worlds).Error; err != nil {
			return nil, err
		}
		for _, world := range worlds {
			if world != nil {
				worldNames[world.ID] = strings.TrimSpace(world.Name)
			}
		}
	}

	userNames := map[string]string{}
	if len(userIDs) > 0 {
		var users []*model.UserModel
		if err := model.GetDB().Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return nil, err
		}
		for _, user := range users {
			if user == nil {
				continue
			}
			name := strings.TrimSpace(user.Nickname)
			if name == "" {
				name = strings.TrimSpace(user.Username)
			}
			if name == "" {
				name = user.ID
			}
			userNames[user.ID] = name
		}
	}

	items := make([]AdminAudioAssetListItem, 0, len(assets))
	for _, asset := range assets {
		if asset == nil {
			continue
		}
		usage, err := AudioGetAssetUsageSummary(asset.ID)
		if err != nil {
			return nil, err
		}
		worldName := ""
		if asset.WorldID != nil {
			worldName = worldNames[strings.TrimSpace(*asset.WorldID)]
		}
		items = append(items, AdminAudioAssetListItem{
			AudioAsset:   asset,
			WorldName:    worldName,
			CreatorName:  userNames[strings.TrimSpace(asset.CreatedBy)],
			UsageSummary: usage,
			SafeToDelete: !usage.Referenced,
		})
	}
	return items, nil
}

func buildAdminAudioFilterOptions(assets []*model.AudioAsset) ([]AdminAudioFilterOption, []AdminAudioFilterOption, error) {
	worldOptionMap := map[string]string{}
	creatorOptionMap := map[string]string{}
	for _, asset := range assets {
		if asset == nil {
			continue
		}
		if asset.WorldID != nil {
			worldID := strings.TrimSpace(*asset.WorldID)
			if worldID != "" {
				worldOptionMap[worldID] = worldID
			}
		}
		userID := strings.TrimSpace(asset.CreatedBy)
		if userID != "" {
			creatorOptionMap[userID] = userID
		}
	}

	worldIDs := make([]string, 0, len(worldOptionMap))
	for id := range worldOptionMap {
		worldIDs = append(worldIDs, id)
	}
	sort.Strings(worldIDs)
	worldOptions := make([]AdminAudioFilterOption, 0, len(worldIDs))
	if len(worldIDs) > 0 {
		var worlds []*model.WorldModel
		if err := model.GetDB().Where("id IN ?", worldIDs).Find(&worlds).Error; err != nil {
			return nil, nil, err
		}
		worldNames := map[string]string{}
		for _, world := range worlds {
			if world != nil {
				worldNames[world.ID] = strings.TrimSpace(world.Name)
			}
		}
		for _, id := range worldIDs {
			label := worldNames[id]
			if label == "" {
				label = id
			}
			worldOptions = append(worldOptions, AdminAudioFilterOption{Label: label, Value: id})
		}
	}

	creatorIDs := make([]string, 0, len(creatorOptionMap))
	for id := range creatorOptionMap {
		creatorIDs = append(creatorIDs, id)
	}
	sort.Strings(creatorIDs)
	creatorOptions := make([]AdminAudioFilterOption, 0, len(creatorIDs))
	if len(creatorIDs) > 0 {
		var users []*model.UserModel
		if err := model.GetDB().Where("id IN ?", creatorIDs).Find(&users).Error; err != nil {
			return nil, nil, err
		}
		userNames := map[string]string{}
		for _, user := range users {
			if user == nil {
				continue
			}
			label := strings.TrimSpace(user.Nickname)
			if label == "" {
				label = strings.TrimSpace(user.Username)
			}
			if label == "" {
				label = user.ID
			}
			userNames[user.ID] = label
		}
		for _, id := range creatorIDs {
			label := userNames[id]
			if label == "" {
				label = id
			}
			creatorOptions = append(creatorOptions, AdminAudioFilterOption{Label: label, Value: id})
		}
	}
	return worldOptions, creatorOptions, nil
}

func sceneReferencesAsset(tracks model.JSONList[model.AudioSceneTrack], assetID string) bool {
	for _, track := range tracks {
		if track.AssetID != nil && strings.TrimSpace(*track.AssetID) == assetID {
			return true
		}
		for _, playlistAssetID := range track.PlaylistAssetIDs {
			if strings.TrimSpace(playlistAssetID) == assetID {
				return true
			}
		}
	}
	return false
}

func playbackStateReferencesAsset(tracks model.JSONList[model.AudioTrackState], assetID string) bool {
	for _, track := range tracks {
		if track.AssetID != nil && strings.TrimSpace(*track.AssetID) == assetID {
			return true
		}
		for _, playlistAssetID := range track.PlaylistAssetIDs {
			if strings.TrimSpace(playlistAssetID) == assetID {
				return true
			}
		}
	}
	return false
}

func AudioListFolders() ([]*AudioFolderNode, error) {
	return AudioListFoldersWithFilters(AudioFolderFilters{IncludeCommon: true})
}

func AudioListFoldersWithFilters(filters AudioFolderFilters) ([]*AudioFolderNode, error) {
	var folders []*model.AudioFolder
	q := model.GetDB().Order("path")
	// scope/worldId 过滤
	if filters.Scope != "" {
		if filters.Scope == model.AudioScopeWorld && filters.WorldID != nil {
			if filters.IncludeCommon {
				q = q.Where("(scope = ? AND world_id = ?) OR scope = ?", model.AudioScopeWorld, *filters.WorldID, model.AudioScopeCommon)
			} else {
				q = q.Where("scope = ? AND world_id = ?", model.AudioScopeWorld, *filters.WorldID)
			}
		} else {
			q = q.Where("scope = ?", filters.Scope)
		}
	} else if filters.WorldID != nil {
		if filters.IncludeCommon {
			q = q.Where("(scope = ? AND world_id = ?) OR scope = ?", model.AudioScopeWorld, *filters.WorldID, model.AudioScopeCommon)
		} else {
			q = q.Where("scope = ? AND world_id = ?", model.AudioScopeWorld, *filters.WorldID)
		}
	}
	if err := q.Find(&folders).Error; err != nil {
		return nil, err
	}
	nodeMap := map[string]*AudioFolderNode{}
	var roots []*AudioFolderNode
	for _, folder := range folders {
		node := &AudioFolderNode{AudioFolder: folder}
		nodeMap[folder.ID] = node
	}
	for _, folder := range folders {
		node := nodeMap[folder.ID]
		if node == nil {
			continue
		}
		if node.ParentID != nil && *node.ParentID != "" {
			parent, ok := nodeMap[*node.ParentID]
			if ok {
				parent.Children = append(parent.Children, node)
				continue
			}
		}
		roots = append(roots, node)
	}
	return roots, nil
}

func AudioCreateFolder(payload AudioFolderPayload) (*model.AudioFolder, error) {
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		return nil, errors.New("文件夹名称不能为空")
	}
	var path string
	worldID := normalizeOptionalStringPtr(payload.WorldID)
	scope := payload.Scope
	if payload.ParentID != nil && *payload.ParentID != "" {
		parent, err := getAudioFolder(*payload.ParentID)
		if err != nil {
			return nil, err
		}
		if scope == "" {
			scope = parent.Scope
		} else if scope != parent.Scope {
			return nil, errors.New("父级文件夹与子文件夹级别不一致")
		}
		parentWorldID := normalizeOptionalString(parent.WorldID)
		if worldID == nil && parentWorldID != "" {
			worldID = cloneStringPtr(parent.WorldID)
		} else if worldID != nil && parentWorldID != "" && normalizeOptionalString(worldID) != parentWorldID {
			return nil, errors.New("父级文件夹与子文件夹世界不一致")
		} else if worldID != nil && parentWorldID == "" {
			return nil, errors.New("父级为通用文件夹，不能创建世界级子文件夹")
		}
		path = buildFolderPath(parent.Path, name)
	} else {
		path = buildFolderPath("", name)
	}
	if scope == "" {
		scope = model.AudioScopeCommon
	}
	if scope == model.AudioScopeWorld {
		if normalizeOptionalString(worldID) == "" {
			return nil, errors.New("世界级文件夹必须指定 worldId")
		}
	} else if scope == model.AudioScopeCommon {
		if worldID != nil {
			return nil, errors.New("通用文件夹不能指定 worldId")
		}
	}
	folder := &model.AudioFolder{}
	folder.StringPKBaseModel.Init()
	folder.Name = name
	folder.ParentID = cloneStringPtr(payload.ParentID)
	folder.Path = path
	folder.CreatedBy = payload.ActorID
	folder.UpdatedBy = payload.ActorID
	folder.Scope = scope
	folder.WorldID = cloneStringPtr(worldID)
	if err := model.GetDB().Create(folder).Error; err != nil {
		return nil, err
	}
	return folder, nil
}

func AudioGetFolder(id string) (*model.AudioFolder, error) {
	return getAudioFolder(id)
}

func AudioUpdateFolder(id string, payload AudioFolderPayload) (*model.AudioFolder, error) {
	folder, err := getAudioFolder(id)
	if err != nil {
		return nil, err
	}
	targetScope := folder.Scope
	targetWorldID := cloneStringPtr(folder.WorldID)
	if payload.Scope != "" {
		if payload.Scope != model.AudioScopeCommon && payload.Scope != model.AudioScopeWorld {
			return nil, errors.New("文件夹级别无效")
		}
		targetScope = payload.Scope
	}
	if payload.WorldID != nil {
		trimmed := strings.TrimSpace(*payload.WorldID)
		if trimmed == "" {
			targetWorldID = nil
		} else {
			targetWorldID = &trimmed
		}
	}
	if targetScope == model.AudioScopeCommon {
		targetWorldID = nil
	}
	if targetScope == model.AudioScopeWorld && normalizeOptionalString(targetWorldID) == "" {
		return nil, errors.New("世界级文件夹必须指定 worldId")
	}
	var parentPath string
	if payload.ParentID != nil && *payload.ParentID != "" {
		if *payload.ParentID == id {
			return nil, errors.New("不能将父级设置为自己")
		}
		parent, err := getAudioFolder(*payload.ParentID)
		if err != nil {
			return nil, err
		}
		if parent.Scope != targetScope {
			return nil, errors.New("父级文件夹与子文件夹级别不一致")
		}
		parentWorldID := normalizeOptionalString(parent.WorldID)
		folderWorldID := normalizeOptionalString(targetWorldID)
		if parentWorldID != folderWorldID {
			return nil, errors.New("父级文件夹与子文件夹世界不一致")
		}
		if strings.HasPrefix(parent.Path, folder.Path) {
			return nil, errors.New("不能移动到子目录")
		}
		parentPath = parent.Path
	}
	name := strings.TrimSpace(payload.Name)
	if name == "" {
		name = folder.Name
	}
	newPath := buildFolderPath(parentPath, name)
	updates := map[string]interface{}{
		"name":       name,
		"updated_by": payload.ActorID,
		"updated_at": time.Now(),
	}
	if payload.ParentID != nil {
		updates["parent_id"] = cloneStringPtr(payload.ParentID)
	}
	if newPath != folder.Path {
		updates["path"] = newPath
	}
	scopeChanged := folder.Scope != targetScope
	worldChanged := normalizeOptionalString(folder.WorldID) != normalizeOptionalString(targetWorldID)
	if payload.Scope != "" || payload.WorldID != nil {
		updates["scope"] = targetScope
		updates["world_id"] = targetWorldID
	}
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		if newPath != folder.Path {
			if err := updateFolderPathWithTx(tx, folder.Path, newPath); err != nil {
				return err
			}
		}
		if scopeChanged || worldChanged {
			pathForScope := newPath
			if pathForScope == "" {
				pathForScope = folder.Path
			}
			if err := tx.Model(&model.AudioFolder{}).
				Where("path = ? OR path LIKE ?", pathForScope, pathForScope+"/%").
				Updates(map[string]interface{}{"scope": targetScope, "world_id": targetWorldID}).Error; err != nil {
				return err
			}
			sub := tx.Model(&model.AudioFolder{}).
				Select("id").
				Where("path = ? OR path LIKE ?", pathForScope, pathForScope+"/%")
			if err := tx.Model(&model.AudioAsset{}).
				Where("folder_id IN (?)", sub).
				Updates(map[string]interface{}{"scope": targetScope, "world_id": targetWorldID}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(folder).Updates(updates).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	folder.Name = name
	folder.ParentID = cloneStringPtr(payload.ParentID)
	folder.Path = newPath
	folder.UpdatedBy = payload.ActorID
	folder.Scope = targetScope
	folder.WorldID = cloneStringPtr(targetWorldID)
	return folder, nil
}

func AudioDeleteFolder(id string) error {
	folder, err := getAudioFolder(id)
	if err != nil {
		return err
	}
	var childrenCount int64
	if err := model.GetDB().Model(&model.AudioFolder{}).
		Where("parent_id = ?", id).
		Count(&childrenCount).Error; err != nil {
		return err
	}
	if childrenCount > 0 {
		return errors.New("请先删除子文件夹")
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.AudioAsset{}).
			Where("folder_id = ?", id).
			Update("folder_id", nil).Error; err != nil {
			return err
		}
		return tx.Delete(folder).Error
	})
}

func AudioListScenes(channelScope string) ([]*model.AudioScene, error) {
	return AudioListScenesWithFilters(AudioSceneFilters{ChannelScope: channelScope, IncludeCommon: true})
}

func AudioListScenesWithFilters(filters AudioSceneFilters) ([]*model.AudioScene, error) {
	q := model.ApplyOrderBy(
		model.GetDB(),
		model.OrderField{Name: "order"},
		model.OrderField{Name: "created_at"},
	)
	if filters.ChannelScope != "" {
		q = q.Where("channel_scope = ?", filters.ChannelScope)
	}
	// scope/worldId 过滤
	if filters.Scope != "" {
		if filters.Scope == model.AudioScopeWorld && filters.WorldID != nil {
			if filters.IncludeCommon {
				q = q.Where("(scope = ? AND world_id = ?) OR scope = ?", model.AudioScopeWorld, *filters.WorldID, model.AudioScopeCommon)
			} else {
				q = q.Where("scope = ? AND world_id = ?", model.AudioScopeWorld, *filters.WorldID)
			}
		} else {
			q = q.Where("scope = ?", filters.Scope)
		}
	} else if filters.WorldID != nil {
		if filters.IncludeCommon {
			q = q.Where("(scope = ? AND world_id = ?) OR scope = ?", model.AudioScopeWorld, *filters.WorldID, model.AudioScopeCommon)
		} else {
			q = q.Where("scope = ? AND world_id = ?", model.AudioScopeWorld, *filters.WorldID)
		}
	}
	var scenes []*model.AudioScene
	if err := q.Find(&scenes).Error; err != nil {
		return nil, err
	}
	return scenes, nil
}

func AudioCreateScene(input AudioSceneInput) (*model.AudioScene, error) {
	if strings.TrimSpace(input.Name) == "" {
		return nil, errors.New("场景名称不能为空")
	}
	scope := input.Scope
	if scope == "" {
		scope = model.AudioScopeCommon
	}
	scene := &model.AudioScene{}
	scene.StringPKBaseModel.Init()
	scene.Name = strings.TrimSpace(input.Name)
	scene.Description = strings.TrimSpace(input.Description)
	scene.Tracks = model.JSONList[model.AudioSceneTrack](normalizeSceneTracks(input.Tracks))
	scene.Tags = model.JSONList[string](normalizeTags(input.Tags))
	scene.Order = input.Order
	scene.ChannelScope = input.ChannelScope
	scene.CreatedBy = input.ActorID
	scene.UpdatedBy = input.ActorID
	scene.Scope = scope
	scene.WorldID = cloneStringPtr(input.WorldID)
	if err := model.GetDB().Create(scene).Error; err != nil {
		return nil, err
	}
	return scene, nil
}

func AudioGetScene(id string) (*model.AudioScene, error) {
	return getAudioScene(id)
}

func AudioUpdateScene(id string, input AudioSceneInput) (*model.AudioScene, error) {
	scene, err := getAudioScene(id)
	if err != nil {
		return nil, err
	}
	updates := map[string]interface{}{
		"updated_at": time.Now(),
		"updated_by": input.ActorID,
	}
	if strings.TrimSpace(input.Name) != "" {
		updates["name"] = strings.TrimSpace(input.Name)
		scene.Name = strings.TrimSpace(input.Name)
	}
	updates["description"] = strings.TrimSpace(input.Description)
	scene.Description = strings.TrimSpace(input.Description)
	updates["tracks"] = model.JSONList[model.AudioSceneTrack](normalizeSceneTracks(input.Tracks))
	scene.Tracks = model.JSONList[model.AudioSceneTrack](normalizeSceneTracks(input.Tracks))
	updates["tags"] = model.JSONList[string](normalizeTags(input.Tags))
	scene.Tags = model.JSONList[string](normalizeTags(input.Tags))
	updates["order"] = input.Order
	scene.Order = input.Order
	if input.ChannelScope != nil {
		updates["channel_scope"] = input.ChannelScope
		scene.ChannelScope = input.ChannelScope
	}
	if err := model.GetDB().Model(scene).Updates(updates).Error; err != nil {
		return nil, err
	}
	return scene, nil
}

func AudioDeleteScene(id string) error {
	return model.GetDB().Delete(&model.AudioScene{}, "id = ?", id).Error
}

func normalizeSceneTracks(tracks []model.AudioSceneTrack) []model.AudioSceneTrack {
	result := make([]model.AudioSceneTrack, 0, len(tracks))
	for _, track := range tracks {
		if strings.TrimSpace(track.Type) == "" {
			continue
		}
		item := model.AudioSceneTrack{
			Type:    strings.TrimSpace(track.Type),
			Volume:  track.Volume,
			FadeIn:  track.FadeIn,
			FadeOut: track.FadeOut,
		}
		playbackRate := 1.0
		if track.PlaybackRate != nil && *track.PlaybackRate > 0 {
			playbackRate = *track.PlaybackRate
		}
		item.PlaybackRate = &playbackRate
		loopEnabled := true
		if track.LoopEnabled != nil {
			loopEnabled = *track.LoopEnabled
		}
		item.LoopEnabled = &loopEnabled
		if track.AssetID != nil && *track.AssetID != "" {
			value := strings.TrimSpace(*track.AssetID)
			item.AssetID = &value
		}
		if track.PlaylistFolderID != nil && strings.TrimSpace(*track.PlaylistFolderID) != "" {
			value := strings.TrimSpace(*track.PlaylistFolderID)
			item.PlaylistFolderID = &value
		}
		if track.PlaylistMode != nil {
			mode := strings.TrimSpace(*track.PlaylistMode)
			switch mode {
			case "single", "sequential", "shuffle":
				item.PlaylistMode = &mode
			}
		}
		if len(track.PlaylistAssetIDs) > 0 {
			ids := make([]string, 0, len(track.PlaylistAssetIDs))
			for _, id := range track.PlaylistAssetIDs {
				trimmed := strings.TrimSpace(id)
				if trimmed != "" {
					ids = append(ids, trimmed)
				}
			}
			item.PlaylistAssetIDs = ids
		}
		if len(item.PlaylistAssetIDs) == 0 {
			item.PlaylistIndex = 0
		} else if track.PlaylistIndex < 0 {
			item.PlaylistIndex = 0
		} else if track.PlaylistIndex >= len(item.PlaylistAssetIDs) {
			item.PlaylistIndex = len(item.PlaylistAssetIDs) - 1
		} else {
			item.PlaylistIndex = track.PlaylistIndex
		}
		result = append(result, item)
	}
	return result
}

func buildFolderPath(parentPath, name string) string {
	cleanName := strings.TrimSpace(name)
	if parentPath == "" {
		return fmt.Sprintf("/%s", cleanName)
	}
	return fmt.Sprintf("%s/%s", strings.TrimSuffix(parentPath, "/"), cleanName)
}

func getAudioFolder(id string) (*model.AudioFolder, error) {
	var folder model.AudioFolder
	if err := model.GetDB().Where("id = ?", id).First(&folder).Error; err != nil {
		return nil, err
	}
	return &folder, nil
}

func getAudioScene(id string) (*model.AudioScene, error) {
	var scene model.AudioScene
	if err := model.GetDB().Where("id = ?", id).First(&scene).Error; err != nil {
		return nil, err
	}
	return &scene, nil
}

func updateFolderPath(oldPath, newPath string) error {
	if oldPath == newPath {
		return nil
	}
	return model.GetDB().Transaction(func(tx *gorm.DB) error {
		return updateFolderPathWithTx(tx, oldPath, newPath)
	})
}

func updateFolderPathWithTx(tx *gorm.DB, oldPath, newPath string) error {
	if oldPath == newPath {
		return nil
	}
	if err := tx.Model(&model.AudioFolder{}).
		Where("path = ?", oldPath).
		Update("path", newPath).Error; err != nil {
		return err
	}
	return tx.Model(&model.AudioFolder{}).
		Where("path LIKE ?", oldPath+"/%").
		Update("path", gorm.Expr("REPLACE(path, ?, ?)", oldPath+"/", newPath+"/")).Error
}

func audioAssetIDsInScenes() ([]string, error) {
	var scenes []*model.AudioScene
	if err := model.GetDB().Find(&scenes).Error; err != nil {
		return nil, err
	}
	set := map[string]struct{}{}
	for _, scene := range scenes {
		for _, track := range scene.Tracks {
			if track.AssetID != nil && *track.AssetID != "" {
				set[*track.AssetID] = struct{}{}
			}
		}
	}
	var ids []string
	for id := range set {
		ids = append(ids, id)
	}
	return ids, nil
}

func cloneStringPtr(src *string) *string {
	if src == nil {
		return nil
	}
	value := *src
	return &value
}

func normalizeOptionalString(src *string) string {
	if src == nil {
		return ""
	}
	return strings.TrimSpace(*src)
}

func normalizeOptionalStringPtr(src *string) *string {
	if src == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*src)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func validateFolderScopeMatch(folder *model.AudioFolder, scope model.AudioAssetScope, worldID *string) error {
	if folder == nil {
		return errors.New("文件夹不存在")
	}
	if scope == "" {
		scope = model.AudioScopeCommon
	}
	switch scope {
	case model.AudioScopeCommon:
		if folder.Scope != model.AudioScopeCommon {
			return errors.New("文件夹级别与素材级别不一致")
		}
		if normalizeOptionalString(folder.WorldID) != "" {
			return errors.New("通用素材不能绑定世界级文件夹")
		}
	case model.AudioScopeWorld:
		if folder.Scope != model.AudioScopeWorld {
			return errors.New("文件夹级别与素材级别不一致")
		}
		if normalizeOptionalString(worldID) == "" {
			return errors.New("世界级素材必须指定 worldId")
		}
		if normalizeOptionalString(folder.WorldID) != normalizeOptionalString(worldID) {
			return errors.New("文件夹不属于目标世界")
		}
	default:
		return errors.New("素材级别无效")
	}
	return nil
}
