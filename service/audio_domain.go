package service

import (
	"errors"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/utils"
)

type AudioAssetFilters struct {
	Query         string
	Tags          []string
	FolderID      *string
	CreatorIDs    []string
	DurationMin   float64
	DurationMax   float64
	HasSceneOnly  bool
	Page          int
	PageSize      int
	Scope         model.AudioAssetScope
	WorldID       *string
	IncludeCommon bool
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

type AudioImportRequest struct {
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
	ChannelID    string
	SceneID      *string
	Tracks       []AudioTrackState
	IsPlaying    bool
	Position     float64
	LoopEnabled  bool
	PlaybackRate float64
	WorldPlaybackEnabled bool
	ActorID      string
}

func (f *AudioAssetFilters) normalize() {
	f.Query = strings.TrimSpace(f.Query)
	if f.Page <= 0 {
		f.Page = 1
	}
	if f.PageSize <= 0 || f.PageSize > 500 {
		f.PageSize = 200
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
	asset, err := AudioProcessUpload(file, opts)
	if err != nil {
		return nil, err
	}
	if err := model.GetDB().Create(asset).Error; err != nil {
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
	asset, err := svc.importFromPath(filePath, opts)
	if err != nil {
		return nil, err
	}
	if err := model.GetDB().Create(asset).Error; err != nil {
		return nil, err
	}
	if asset.TranscodeStatus == model.AudioTranscodePending {
		svc.scheduleTranscode(asset.ID, asset.ObjectKey)
	}
	return asset, nil
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
		return q.Order("updated_at DESC")
		})
}

func GetAudioImportPreview() (*AudioImportPreview, error) {
	svc := GetAudioService()
	if svc == nil {
		return nil, errors.New("音频服务未初始化")
	}
	importDir, err := getAudioImportDir(svc)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(importDir)
	if err != nil {
		return nil, err
	}
	items := make([]AudioImportPreviewItem, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if shouldSkipImportEntry(name, entry) {
			continue
		}
		fullPath := filepath.Join(importDir, name)
		item := buildAudioImportPreviewItem(svc, fullPath, name)
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	validCount := 0
	for _, item := range items {
		if item.Valid {
			validCount++
		}
	}
	return &AudioImportPreview{
		Items:   items,
		Total:   len(items),
		Valid:   validCount,
		Invalid: len(items) - validCount,
	}, nil
}

func AudioImportFromDir(req AudioImportRequest) (*AudioImportResult, error) {
	svc := GetAudioService()
	if svc == nil {
		return nil, errors.New("音频服务未初始化")
	}
	importDir, err := getAudioImportDir(svc)
	if err != nil {
		return nil, err
	}
	result := &AudioImportResult{}
	paths := make([]string, 0)
	seen := map[string]struct{}{}
	if req.All {
		preview, err := GetAudioImportPreview()
		if err != nil {
			return nil, err
		}
		for _, item := range preview.Items {
			if !item.Valid {
				result.Skipped = append(result.Skipped, AudioImportResultItem{
					Path:   item.Path,
					Name:   item.Name,
					Reason: item.Reason,
				})
				continue
			}
			if _, ok := seen[item.Path]; ok {
				continue
			}
			seen[item.Path] = struct{}{}
			paths = append(paths, item.Path)
		}
	} else {
		for _, raw := range req.Paths {
			name := strings.TrimSpace(raw)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			paths = append(paths, name)
		}
	}
	for _, name := range paths {
		fullPath, err := resolveAudioImportPath(importDir, name)
		if err != nil {
			result.Skipped = append(result.Skipped, AudioImportResultItem{
				Path:   name,
				Name:   name,
				Reason: err.Error(),
			})
			continue
		}
		previewItem := buildAudioImportPreviewItem(svc, fullPath, name)
		if !previewItem.Valid {
			result.Skipped = append(result.Skipped, AudioImportResultItem{
				Path:   name,
				Name:   previewItem.Name,
				Reason: previewItem.Reason,
			})
			continue
		}
		asset, err := AudioCreateAssetFromImport(fullPath, req.Options)
		if err != nil {
			if errors.Is(err, ErrAudioTooLarge) || errors.Is(err, ErrAudioUnsupportedMime) {
				result.Skipped = append(result.Skipped, AudioImportResultItem{
					Path:   name,
					Name:   previewItem.Name,
					Reason: err.Error(),
				})
				continue
			}
			result.Failed = append(result.Failed, AudioImportResultItem{
				Path:  name,
				Name:  previewItem.Name,
				Error: err.Error(),
			})
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
		result.Imported = append(result.Imported, item)
	}
	return result, nil
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
	if strings.HasPrefix(trimmed, ".") {
		return true
	}
	if entry.IsDir() {
		return true
	}
	if entry.Type()&os.ModeSymlink != 0 {
		return true
	}
	return false
}

func resolveAudioImportPath(importDir, name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", errors.New("文件名为空")
	}
	if filepath.Base(trimmed) != trimmed {
		return "", errors.New("非法文件路径")
	}
	if strings.HasPrefix(trimmed, ".") {
		return "", errors.New("隐藏文件")
	}
	fullPath := filepath.Join(importDir, trimmed)
	relPath, err := filepath.Rel(importDir, fullPath)
	if err != nil {
		return "", errors.New("非法文件路径")
	}
	if relPath == "." || strings.HasPrefix(relPath, "..") || strings.HasPrefix(filepath.Clean(relPath), "..") {
		return "", errors.New("非法文件路径")
	}
	return fullPath, nil
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

func normalizeTrackStates(items []AudioTrackState) []AudioTrackState {
	if items == nil {
		return nil
	}
	result := make([]AudioTrackState, 0, len(items))
	for _, item := range items {
		t := AudioTrackState{
			Type:         strings.TrimSpace(item.Type),
			Volume:       item.Volume,
			Muted:        item.Muted,
			Solo:         item.Solo,
			FadeIn:       item.FadeIn,
			FadeOut:      item.FadeOut,
			IsPlaying:    item.IsPlaying,
			Position:     item.Position,
			LoopEnabled:  item.LoopEnabled,
			PlaybackRate: item.PlaybackRate,
		}
		if t.PlaybackRate <= 0 {
			t.PlaybackRate = 1
		}
		if t.Position < 0 {
			t.Position = 0
		}
		if item.AssetID != nil {
			trimmed := strings.TrimSpace(*item.AssetID)
			if trimmed != "" {
				val := trimmed
				t.AssetID = &val
			}
		}
		result = append(result, t)
	}
	return result
}

func AudioGetPlaybackState(channelID string) (*model.AudioPlaybackState, error) {
	if strings.TrimSpace(channelID) == "" {
		return nil, errors.New("channelId 必填")
	}
	db := model.GetDB()
	var state model.AudioPlaybackState
	err := db.Where("channel_id = ?", channelID).
		Order("updated_at desc").
		Limit(1).
		First(&state).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	// 查找频道所属世界，世界模式下优先返回最新的世界状态
	ch, chErr := model.ChannelGet(channelID)
	if chErr == nil && ch != nil && ch.WorldID != "" {
		var worldState model.AudioPlaybackState
		worldErr := db.Table("audio_playback_states AS aps").
			Joins("JOIN channels c ON c.id = aps.channel_id").
			Where("c.world_id = ?", ch.WorldID).
			Order("aps.updated_at desc").
			Limit(1).
			First(&worldState).Error
		if worldErr != nil && !errors.Is(worldErr, gorm.ErrRecordNotFound) {
			return nil, worldErr
		}
		if worldErr == nil && worldState.WorldPlaybackEnabled {
			return &worldState, nil
		}
	}
	if err != nil {
		return nil, nil
	}
	return &state, nil
}

func AudioUpsertPlaybackState(input AudioPlaybackUpdateInput) (*model.AudioPlaybackState, error) {
	if strings.TrimSpace(input.ChannelID) == "" {
		return nil, errors.New("channelId 必填")
	}
	if input.PlaybackRate <= 0 {
		input.PlaybackRate = 1
	}
	if input.Position < 0 {
		input.Position = 0
	}
	db := model.GetDB()
	var state model.AudioPlaybackState
	err := db.Where("channel_id = ?", input.ChannelID).First(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		state = model.AudioPlaybackState{
			ChannelID: input.ChannelID,
			CreatedAt: time.Now(),
		}
	} else if err != nil {
		return nil, err
	}
	state.SceneID = input.SceneID
	if state.SceneID != nil {
		trimmed := strings.TrimSpace(*state.SceneID)
		if trimmed == "" {
			state.SceneID = nil
		} else {
			val := trimmed
			state.SceneID = &val
		}
	}
	state.Tracks = model.JSONList[AudioTrackState](normalizeTrackStates(input.Tracks))
	state.IsPlaying = input.IsPlaying
	state.Position = input.Position
	state.LoopEnabled = input.LoopEnabled
	state.PlaybackRate = input.PlaybackRate
	state.WorldPlaybackEnabled = input.WorldPlaybackEnabled
	state.UpdatedBy = input.ActorID
	state.UpdatedAt = time.Now()
	if err := db.Save(&state).Error; err != nil {
		return nil, err
	}
	return &state, nil
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

func AudioDeleteAsset(id string, hard bool) error {
	asset, err := AudioGetAsset(id)
	if err != nil {
		return err
	}
	svc := GetAudioService()
	if svc != nil {
		svc.removeAssetObject(asset.StorageType, asset.ObjectKey)
		for _, variant := range asset.Variants {
			svc.removeAssetObject(variant.StorageType, variant.ObjectKey)
		}
	}
	if hard {
		return model.GetDB().Unscoped().Delete(&model.AudioAsset{}, "id = ?", id).Error
	}
	return model.GetDB().Model(&model.AudioAsset{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{"deleted_at": time.Now()}).Error
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
	var assetsCount int64
	if err := model.GetDB().Model(&model.AudioAsset{}).
		Where("folder_id = ?", id).
		Count(&assetsCount).Error; err != nil {
		return err
	}
	if assetsCount > 0 {
		return errors.New("文件夹内仍有素材，无法删除")
	}
	return model.GetDB().Delete(folder).Error
}

func AudioListScenes(channelScope string) ([]*model.AudioScene, error) {
	return AudioListScenesWithFilters(AudioSceneFilters{ChannelScope: channelScope, IncludeCommon: true})
}

func AudioListScenesWithFilters(filters AudioSceneFilters) ([]*model.AudioScene, error) {
	q := model.GetDB().Order("`order`, created_at")
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
		if track.AssetID != nil && *track.AssetID != "" {
			value := strings.TrimSpace(*track.AssetID)
			item.AssetID = &value
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
