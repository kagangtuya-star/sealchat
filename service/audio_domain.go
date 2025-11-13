package service

import (
	"errors"
	"fmt"
	"mime/multipart"
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/utils"
)

type AudioAssetFilters struct {
	Query        string
	Tags         []string
	FolderID     *string
	CreatorIDs   []string
	DurationMin  float64
	DurationMax  float64
	HasSceneOnly bool
	Page         int
	PageSize     int
}

type AudioAssetUpdateInput struct {
	Name        *string
	Description *string
	Tags        []string
	Visibility  *model.AudioAssetVisibility
	FolderID    *string
	UpdatedBy   string
	Variants    []model.AudioAssetVariant
}

type AudioFolderNode struct {
	*model.AudioFolder
	Children []*AudioFolderNode `json:"children,omitempty"`
}

type AudioFolderPayload struct {
	Name     string
	ParentID *string
	ActorID  string
}

type AudioSceneInput struct {
	Name         string
	Description  string
	Tracks       []model.AudioSceneTrack
	Tags         []string
	Order        int
	ChannelScope *string
	ActorID      string
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
	if opts.FolderID != nil && *opts.FolderID != "" {
		if _, err := getAudioFolder(*opts.FolderID); err != nil {
			return nil, err
		}
	}
	asset, err := AudioProcessUpload(file, opts)
	if err != nil {
		return nil, err
	}
	if err := model.GetDB().Create(asset).Error; err != nil {
		return nil, err
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
		return q.Order("updated_at DESC")
	})
}

func normalizeTrackStates(items []AudioTrackState) []AudioTrackState {
	if items == nil {
		return nil
	}
	result := make([]AudioTrackState, 0, len(items))
	for _, item := range items {
		t := AudioTrackState{
			Type:    strings.TrimSpace(item.Type),
			Volume:  item.Volume,
			Muted:   item.Muted,
			Solo:    item.Solo,
			FadeIn:  item.FadeIn,
			FadeOut: item.FadeOut,
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
	var state model.AudioPlaybackState
	err := model.GetDB().Where("channel_id = ?", channelID).First(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
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
	if input.FolderID != nil {
		trimmed := strings.TrimSpace(*input.FolderID)
		if trimmed != "" {
			if _, err := getAudioFolder(trimmed); err != nil {
				return nil, err
			}
			updates["folder_id"] = trimmed
			asset.FolderID = cloneStringPtr(&trimmed)
		} else {
			updates["folder_id"] = nil
			asset.FolderID = nil
		}
	}
	if input.Tags != nil {
		updates["tags"] = model.JSONList[string](normalizeTags(input.Tags))
		asset.Tags = model.JSONList[string](normalizeTags(input.Tags))
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
	var folders []*model.AudioFolder
	if err := model.GetDB().Order("path").Find(&folders).Error; err != nil {
		return nil, err
	}
	nodeMap := map[string]*AudioFolderNode{}
	var roots []*AudioFolderNode
	for _, folder := range folders {
		node := &AudioFolderNode{AudioFolder: folder}
		nodeMap[folder.ID] = node
	}
	for _, node := range nodeMap {
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
	if payload.ParentID != nil && *payload.ParentID != "" {
		parent, err := getAudioFolder(*payload.ParentID)
		if err != nil {
			return nil, err
		}
		path = buildFolderPath(parent.Path, name)
	} else {
		path = buildFolderPath("", name)
	}
	folder := &model.AudioFolder{}
	folder.StringPKBaseModel.Init()
	folder.Name = name
	folder.ParentID = cloneStringPtr(payload.ParentID)
	folder.Path = path
	folder.CreatedBy = payload.ActorID
	folder.UpdatedBy = payload.ActorID
	if err := model.GetDB().Create(folder).Error; err != nil {
		return nil, err
	}
	return folder, nil
}

func AudioUpdateFolder(id string, payload AudioFolderPayload) (*model.AudioFolder, error) {
	folder, err := getAudioFolder(id)
	if err != nil {
		return nil, err
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
		if err := updateFolderPath(folder.Path, newPath); err != nil {
			return nil, err
		}
		updates["path"] = newPath
	}
	if err := model.GetDB().Model(folder).Updates(updates).Error; err != nil {
		return nil, err
	}
	folder.Name = name
	folder.ParentID = cloneStringPtr(payload.ParentID)
	folder.Path = newPath
	folder.UpdatedBy = payload.ActorID
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
	q := model.GetDB().Order("`order`, created_at")
	if channelScope != "" {
		q = q.Where("channel_scope = ?", channelScope)
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
	if err := model.GetDB().Create(scene).Error; err != nil {
		return nil, err
	}
	return scene, nil
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
		if err := tx.Model(&model.AudioFolder{}).
			Where("path = ?", oldPath).
			Update("path", newPath).Error; err != nil {
			return err
		}
		return tx.Model(&model.AudioFolder{}).
			Where("path LIKE ?", oldPath+"/%").
			Update("path", gorm.Expr("REPLACE(path, ?, ?)", oldPath+"/", newPath+"/")).Error
	})
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
