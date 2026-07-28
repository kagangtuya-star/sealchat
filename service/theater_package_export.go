package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sealchat/model"
	"sealchat/service/storage"
	"sealchat/utils"
)

func exportTheaterPackage(ctx context.Context, job *model.TheaterPackageJobModel) (TheaterPackageSummary, error) {
	var summary TheaterPackageSummary
	if job == nil {
		return summary, fmt.Errorf("舞台包任务不存在")
	}
	if _, _, err := requireTheaterPermission(job.ActorUserID, job.SourceWorldID, "", TheaterPermissionAdminRestore); err != nil {
		return summary, err
	}
	world, err := GetWorldByID(job.SourceWorldID)
	if err != nil {
		return summary, err
	}
	room, err := model.TheaterRoomCreateIfMissing(job.SourceWorldID, "", job.ActorUserID)
	if err != nil {
		return summary, err
	}
	snapshot, _, err := buildTheaterSnapshot(model.GetDB(), room, true)
	if err != nil {
		return summary, err
	}

	stagingDir, err := os.MkdirTemp(theaterPackageStorageDir(), "export-"+job.ID+"-")
	if err != nil {
		return summary, err
	}
	defer os.RemoveAll(stagingDir)

	document, err := writeJSONFile(filepath.Join(stagingDir, "stage", "document.json"), snapshot)
	if err != nil {
		return summary, err
	}
	document.Path = "stage/document.json"
	manifest := TheaterPackageManifest{
		PackageVersion:       theaterPackageVersion,
		PackageKind:          TheaterPackageKindTheater,
		SchemaVersion:        model.TheaterSchemaVersion,
		PackageID:            utils.NewID(),
		CreatedAt:            time.Now().UTC(),
		SourceWorldID:        job.SourceWorldID,
		SourceWorldName:      world.Name,
		SourceRevision:       room.Revision,
		SourceInputChannelID: job.InputChannelID,
		Document:             document,
		Resources:            []TheaterPackageResource{},
		Audio:                []TheaterPackageAudio{},
	}
	effectIDs := theaterSnapshotEffectIDs(snapshot)
	organizer, err := exportTheaterPackageEffectOrganizer(room.ID, effectIDs)
	if err != nil {
		return summary, err
	}
	if len(organizer.Folders) > 0 || len(organizer.Items) > 0 {
		organizerFile, err := writeJSONFile(filepath.Join(stagingDir, "effects", "organizer.json"), organizer)
		if err != nil {
			return summary, err
		}
		organizerFile.Path = "effects/organizer.json"
		manifest.EffectOrganizer = &organizerFile
	}

	updateTheaterPackageProgress(job.ID, 0.1)
	var resources []model.TheaterResourceModel
	if err := model.GetDB().Where("room_id = ? AND status <> ?", room.ID, "deleting").Order("created_at ASC").Find(&resources).Error; err != nil {
		return summary, err
	}
	for index, resource := range resources {
		item, err := exportTheaterPackageResource(stagingDir, resource)
		if err != nil {
			return summary, fmt.Errorf("导出资源 %s 失败: %w", resource.ID, err)
		}
		manifest.Resources = append(manifest.Resources, item)
		updateTheaterPackageProgress(job.ID, 0.1+0.35*float64(index+1)/float64(maxInt(1, len(resources))))
	}

	template := world.GetTheaterPresentationTemplate()
	templateRaw, err := json.Marshal(template)
	if err != nil {
		return summary, err
	}
	if string(templateRaw) != "{}" && string(templateRaw) != "null" {
		presentation, err := writeJSONFile(filepath.Join(stagingDir, "settings", "world-presentation.json"), template)
		if err != nil {
			return summary, err
		}
		presentation.Path = "settings/world-presentation.json"
		manifest.WorldPresentation = &presentation
	}

	referencedAssetIDs := collectJSONFieldStrings(snapshot, "assetId")
	for id := range collectJSONFieldStrings(template, "assetId") {
		referencedAssetIDs[id] = struct{}{}
	}
	audioAssets, err := theaterPackageAudioAssets(job.SourceWorldID, job.InputChannelID, referencedAssetIDs)
	if err != nil {
		return summary, err
	}
	for index := range audioAssets {
		item, err := exportTheaterPackageAudio(ctx, stagingDir, &audioAssets[index])
		if err != nil {
			return summary, fmt.Errorf("导出音频 %s 失败: %w", audioAssets[index].ID, err)
		}
		manifest.Audio = append(manifest.Audio, item)
		updateTheaterPackageProgress(job.ID, 0.45+0.2*float64(index+1)/float64(maxInt(1, len(audioAssets))))
	}

	appearanceIDs := collectJSONFieldStrings(snapshot, "assetId")
	for id := range collectJSONFieldStrings(template, "assetId") {
		appearanceIDs[id] = struct{}{}
	}
	if len(appearanceIDs) > 0 {
		ids := mapKeys(appearanceIDs)
		var assets []model.TheaterAppearanceAssetModel
		if err := model.GetDB().Where("id IN ? AND status = ?", ids, "ready").Find(&assets).Error; err != nil {
			return summary, err
		}
		for index := range assets {
			item, err := exportTheaterPackageAppearanceAsset(stagingDir, assets[index])
			if err != nil {
				return summary, fmt.Errorf("导出演出资源 %s 失败: %w", assets[index].ID, err)
			}
			manifest.AppearanceAssets = append(manifest.AppearanceAssets, item)
			updateTheaterPackageProgress(job.ID, 0.65+0.15*float64(index+1)/float64(maxInt(1, len(assets))))
		}
	}

	manifestPath := filepath.Join(stagingDir, "manifest.json")
	if _, err := writeJSONFile(manifestPath, manifest); err != nil {
		return summary, err
	}
	completedDir := filepath.Join(theaterPackageStorageDir(), "completed")
	if err := os.MkdirAll(completedDir, 0o755); err != nil {
		return summary, err
	}
	temporaryZIP := filepath.Join(completedDir, job.ID+".tmp")
	outputZIP := filepath.Join(completedDir, job.ID+".zip")
	_ = os.Remove(temporaryZIP)
	defer os.Remove(temporaryZIP)
	if err := zipDirectory(stagingDir, temporaryZIP); err != nil {
		return summary, err
	}
	info, err := os.Stat(temporaryZIP)
	if err != nil {
		return summary, err
	}
	if info.Size() > theaterPackageMaxArchiveBytes {
		return summary, fmt.Errorf("舞台包超过 %d 字节限制", theaterPackageMaxArchiveBytes)
	}
	fileInfo, err := theaterPackageFile(temporaryZIP, "application/zip", "")
	if err != nil {
		return summary, err
	}
	_ = os.Remove(outputZIP)
	if err := os.Rename(temporaryZIP, outputZIP); err != nil {
		return summary, err
	}
	outputName := sanitizeTheaterPackageFilename(fmt.Sprintf("%s-小剧场-%s.zip", world.Name, time.Now().Format("20060102-150405")))
	if err := model.GetDB().Model(&model.TheaterPackageJobModel{}).Where("id = ?", job.ID).Updates(map[string]any{
		"output_file_path": outputZIP,
		"output_file_name": outputName,
		"output_file_size": info.Size(),
		"package_hash":     fileInfo.SHA256,
		"progress":         0.99,
	}).Error; err != nil {
		return summary, err
	}

	summary = summarizeTheaterSnapshot(snapshot)
	summary.PackageKind = TheaterPackageKindTheater
	summary.Effects = len(effectIDs)
	summary.Resources = len(manifest.Resources)
	summary.AudioAssets = len(manifest.Audio)
	summary.AppearanceAssets = len(manifest.AppearanceAssets)
	summary.WorldPresentationImported = manifest.WorldPresentation != nil
	return summary, nil
}

func exportTheaterEffectPackage(ctx context.Context, job *model.TheaterPackageJobModel) (TheaterPackageSummary, error) {
	var summary TheaterPackageSummary
	if job == nil {
		return summary, fmt.Errorf("舞台包任务不存在")
	}
	if _, _, err := requireTheaterPermission(job.ActorUserID, job.SourceWorldID, "", TheaterPermissionAdminRestore); err != nil {
		return summary, err
	}
	world, err := GetWorldByID(job.SourceWorldID)
	if err != nil {
		return summary, err
	}
	room, err := model.TheaterRoomCreateIfMissing(job.SourceWorldID, "", job.ActorUserID)
	if err != nil {
		return summary, err
	}
	snapshot, _, err := buildTheaterSnapshot(model.GetDB(), room, true)
	if err != nil {
		return summary, err
	}
	document := TheaterPackageEffectsDocument{Version: 1, Effects: []TheaterPackageEffectEntry{}}
	for _, scene := range snapshot.Scenes {
		for _, object := range scene.Objects {
			if object.Kind == "effect" {
				document.Effects = append(document.Effects, TheaterPackageEffectEntry{Scope: "scene", SourceSceneID: scene.ID, SourceSceneName: scene.Name, Object: object})
			}
		}
	}
	for _, object := range snapshot.PersistentObjects {
		if object.Kind == "effect" {
			document.Effects = append(document.Effects, TheaterPackageEffectEntry{Scope: "persistent", Object: object})
		}
	}
	sort.Slice(document.Effects, func(i, j int) bool {
		left, right := document.Effects[i], document.Effects[j]
		if left.Scope != right.Scope {
			return left.Scope < right.Scope
		}
		if left.SourceSceneID != right.SourceSceneID {
			return left.SourceSceneID < right.SourceSceneID
		}
		return left.Object.ID < right.Object.ID
	})
	if len(document.Effects) == 0 {
		return summary, theaterPayloadError("当前小剧场没有可导出的特效")
	}

	stagingDir, err := os.MkdirTemp(theaterPackageStorageDir(), "effects-export-"+job.ID+"-")
	if err != nil {
		return summary, err
	}
	defer os.RemoveAll(stagingDir)
	documentFile, err := writeJSONFile(filepath.Join(stagingDir, "effects", "document.json"), document)
	if err != nil {
		return summary, err
	}
	documentFile.Path = "effects/document.json"
	manifest := TheaterPackageManifest{
		PackageVersion: theaterPackageVersion, PackageKind: TheaterPackageKindEffects,
		SchemaVersion: model.TheaterSchemaVersion, PackageID: utils.NewID(), CreatedAt: time.Now().UTC(),
		SourceWorldID: job.SourceWorldID, SourceWorldName: world.Name, SourceRevision: room.Revision,
		SourceInputChannelID: job.InputChannelID, Document: documentFile,
		Resources: []TheaterPackageResource{}, Audio: []TheaterPackageAudio{},
	}
	effectIDs := map[string]struct{}{}
	for _, entry := range document.Effects {
		effectIDs[entry.Object.ID] = struct{}{}
	}
	organizer, err := exportTheaterPackageEffectOrganizer(room.ID, effectIDs)
	if err != nil {
		return summary, err
	}
	if len(organizer.Folders) > 0 || len(organizer.Items) > 0 {
		file, err := writeJSONFile(filepath.Join(stagingDir, "effects", "organizer.json"), organizer)
		if err != nil {
			return summary, err
		}
		file.Path = "effects/organizer.json"
		manifest.EffectOrganizer = &file
	}

	referencedResources := collectJSONFieldStrings(document, "resourceId")
	if len(referencedResources) > 0 {
		resources, err := theaterPackageReferencedResources(room.ID, referencedResources)
		if err != nil {
			return summary, err
		}
		for index, resource := range resources {
			item, err := exportTheaterPackageResource(stagingDir, resource)
			if err != nil {
				return summary, fmt.Errorf("导出资源 %s 失败: %w", resource.ID, err)
			}
			manifest.Resources = append(manifest.Resources, item)
			updateTheaterPackageProgress(job.ID, 0.15+0.35*float64(index+1)/float64(maxInt(1, len(resources))))
		}
	}
	referencedAudio := collectJSONFieldStrings(document, "assetId")
	if len(referencedAudio) > 0 {
		var assets []model.AudioAsset
		if err := model.GetDB().Where("world_id = ? AND id IN ? AND deleted_at IS NULL", job.SourceWorldID, mapKeys(referencedAudio)).Order("id ASC").Find(&assets).Error; err != nil {
			return summary, err
		}
		if len(assets) != len(referencedAudio) {
			return summary, fmt.Errorf("特效引用的部分音频不存在或已删除")
		}
		for index := range assets {
			item, err := exportTheaterPackageAudio(ctx, stagingDir, &assets[index])
			if err != nil {
				return summary, fmt.Errorf("导出音频 %s 失败: %w", assets[index].ID, err)
			}
			manifest.Audio = append(manifest.Audio, item)
			updateTheaterPackageProgress(job.ID, 0.5+0.25*float64(index+1)/float64(maxInt(1, len(assets))))
		}
	}

	if _, err := writeJSONFile(filepath.Join(stagingDir, "manifest.json"), manifest); err != nil {
		return summary, err
	}
	completedDir := filepath.Join(theaterPackageStorageDir(), "completed")
	if err := os.MkdirAll(completedDir, 0o755); err != nil {
		return summary, err
	}
	temporaryZIP := filepath.Join(completedDir, job.ID+".tmp")
	outputZIP := filepath.Join(completedDir, job.ID+".zip")
	_ = os.Remove(temporaryZIP)
	defer os.Remove(temporaryZIP)
	if err := zipDirectory(stagingDir, temporaryZIP); err != nil {
		return summary, err
	}
	info, err := os.Stat(temporaryZIP)
	if err != nil {
		return summary, err
	}
	if info.Size() > theaterPackageMaxArchiveBytes {
		return summary, fmt.Errorf("特效包超过 %d 字节限制", theaterPackageMaxArchiveBytes)
	}
	fileInfo, err := theaterPackageFile(temporaryZIP, "application/zip", "")
	if err != nil {
		return summary, err
	}
	_ = os.Remove(outputZIP)
	if err := os.Rename(temporaryZIP, outputZIP); err != nil {
		return summary, err
	}
	outputName := sanitizeTheaterPackageFilename(fmt.Sprintf("%s-特效包-%s.zip", world.Name, time.Now().Format("20060102-150405")))
	if err := model.GetDB().Model(&model.TheaterPackageJobModel{}).Where("id = ?", job.ID).Updates(map[string]any{
		"output_file_path": outputZIP, "output_file_name": outputName, "output_file_size": info.Size(),
		"package_hash": fileInfo.SHA256, "progress": 0.99,
	}).Error; err != nil {
		return summary, err
	}
	summary = TheaterPackageSummary{PackageKind: TheaterPackageKindEffects, Effects: len(document.Effects), Objects: len(document.Effects), Resources: len(manifest.Resources), AudioAssets: len(manifest.Audio)}
	return summary, nil
}

func theaterSnapshotEffectIDs(snapshot TheaterSharedSnapshot) map[string]struct{} {
	result := map[string]struct{}{}
	for _, scene := range snapshot.Scenes {
		for id, object := range scene.Objects {
			if object.Kind == "effect" {
				result[id] = struct{}{}
			}
		}
	}
	for id, object := range snapshot.PersistentObjects {
		if object.Kind == "effect" {
			result[id] = struct{}{}
		}
	}
	return result
}

func exportTheaterPackageEffectOrganizer(roomID string, effectIDs map[string]struct{}) (TheaterPackageEffectOrganizer, error) {
	result := TheaterPackageEffectOrganizer{Folders: []TheaterPackageEffectFolder{}, Items: []TheaterPackageEffectItem{}}
	var folders []model.TheaterPanelFolderModel
	if err := model.GetDB().Where("room_id = ? AND domain = ?", roomID, TheaterPanelDomainEffect).Order("sort_order ASC, id ASC").Find(&folders).Error; err != nil {
		return result, err
	}
	for _, folder := range folders {
		result.Folders = append(result.Folders, TheaterPackageEffectFolder{ID: folder.ID, Name: folder.Name, SortOrder: folder.SortOrder})
	}
	var items []model.TheaterPanelItemModel
	if err := model.GetDB().Where("room_id = ? AND domain = ?", roomID, TheaterPanelDomainEffect).Order("folder_id ASC, sort_order ASC, id ASC").Find(&items).Error; err != nil {
		return result, err
	}
	for _, item := range items {
		if _, included := effectIDs[item.TargetID]; included {
			result.Items = append(result.Items, TheaterPackageEffectItem{TargetID: item.TargetID, FolderID: item.FolderID, SortOrder: item.SortOrder})
		}
	}
	return result, nil
}

func theaterPackageReferencedResources(roomID string, referenced map[string]struct{}) ([]model.TheaterResourceModel, error) {
	loaded := map[string]model.TheaterResourceModel{}
	pending := map[string]struct{}{}
	for id := range referenced {
		pending[id] = struct{}{}
	}
	for len(pending) > 0 {
		ids := mapKeys(pending)
		var batch []model.TheaterResourceModel
		if err := model.GetDB().Where("room_id = ? AND id IN ? AND status <> ?", roomID, ids, "deleting").Find(&batch).Error; err != nil {
			return nil, err
		}
		if len(batch) != len(ids) {
			return nil, fmt.Errorf("特效引用的部分资源不存在或正在删除")
		}
		pending = map[string]struct{}{}
		for _, resource := range batch {
			if _, exists := loaded[resource.ID]; exists {
				continue
			}
			loaded[resource.ID] = resource
			if resource.PosterResourceID != "" {
				if _, exists := loaded[resource.PosterResourceID]; !exists {
					pending[resource.PosterResourceID] = struct{}{}
				}
			}
		}
	}
	result := make([]model.TheaterResourceModel, 0, len(loaded))
	for _, resource := range loaded {
		result = append(result, resource)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func exportTheaterPackageResource(stagingDir string, resource model.TheaterResourceModel) (TheaterPackageResource, error) {
	original, err := exportAttachmentToPackage(stagingDir, filepath.ToSlash(filepath.Join("resources", resource.ID, "original")), resource.AttachmentID)
	if err != nil {
		return TheaterPackageResource{}, err
	}
	item := TheaterPackageResource{
		ID: resource.ID, ClientResourceID: resource.ClientResourceID, Kind: resource.Kind,
		ContentHash: resource.ContentHash, SizeBytes: resource.SizeBytes, MimeType: resource.MimeType,
		OriginalFilename: resource.OriginalFilename, Width: resource.Width, Height: resource.Height,
		DurationMS: resource.DurationMS, FrameCount: resource.FrameCount, FrameRate: resource.FrameRate, LoopCount: resource.LoopCount,
		Container: resource.Container, VideoCodec: resource.VideoCodec, AudioCodec: resource.AudioCodec,
		PosterResourceID: resource.PosterResourceID, VariantsJSON: resource.VariantsJSON, Original: original,
		Variants: []TheaterPackageResourceVariant{},
	}
	var variants []model.TheaterResourceVariantModel
	if err := model.GetDB().Where("resource_id = ? AND status = ?", resource.ID, "ready").Order("name ASC").Find(&variants).Error; err != nil {
		return item, err
	}
	for _, variant := range variants {
		file, err := exportAttachmentToPackage(stagingDir, filepath.ToSlash(filepath.Join("resources", resource.ID, "variants", variant.Name)), variant.AttachmentID)
		if err != nil {
			return item, err
		}
		item.Variants = append(item.Variants, TheaterPackageResourceVariant{
			Name: variant.Name, MimeType: variant.MimeType, SizeBytes: variant.SizeBytes,
			Width: variant.Width, Height: variant.Height, DurationMS: variant.DurationMS,
			ContentHash: variant.ContentHash, File: file,
		})
	}
	return item, nil
}

func exportAttachmentToPackage(stagingDir, relativePath, attachmentID string) (TheaterPackageFile, error) {
	var attachment model.AttachmentModel
	if err := model.GetDB().Where("id = ?", attachmentID).First(&attachment).Error; err != nil {
		return TheaterPackageFile{}, err
	}
	temporary, err := MaterializeAttachmentToTempFile(&attachment)
	if err != nil {
		return TheaterPackageFile{}, err
	}
	defer os.Remove(temporary)
	target := filepath.Join(stagingDir, filepath.FromSlash(relativePath))
	if err := copyTheaterPackageFile(temporary, target); err != nil {
		return TheaterPackageFile{}, err
	}
	file, err := theaterPackageFile(target, attachment.MimeType, attachment.Filename)
	if err != nil {
		return TheaterPackageFile{}, err
	}
	file.Path = relativePath
	return file, nil
}

func theaterPackageAudioAssets(worldID, channelID string, referenced map[string]struct{}) ([]model.AudioAsset, error) {
	var assets []model.AudioAsset
	if err := model.GetDB().Where("world_id = ? AND deleted_at IS NULL", worldID).Find(&assets).Error; err != nil {
		return nil, err
	}
	result := make([]model.AudioAsset, 0)
	channelTag := theaterChannelAudioTag(channelID)
	for _, asset := range assets {
		_, isReferenced := referenced[asset.ID]
		if isReferenced || hasAudioTag(&asset, theaterFeatureAudioTag) || (channelID != "" && hasAudioTag(&asset, channelTag)) {
			result = append(result, asset)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func exportTheaterPackageAudio(ctx context.Context, stagingDir string, asset *model.AudioAsset) (TheaterPackageAudio, error) {
	if asset == nil {
		return TheaterPackageAudio{}, fmt.Errorf("音频不存在")
	}
	extension := filepath.Ext(asset.ObjectKey)
	if extension == "" {
		extension = filepath.Ext(asset.Name)
	}
	relative := filepath.ToSlash(filepath.Join("audio", asset.ID, "original"+extension))
	target := filepath.Join(stagingDir, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return TheaterPackageAudio{}, err
	}
	if asset.StorageType == model.StorageS3 {
		manager := GetStorageManager()
		if manager == nil {
			return TheaterPackageAudio{}, fmt.Errorf("存储服务未初始化")
		}
		if err := manager.DownloadToPath(ctx, storage.BackendS3, asset.ObjectKey, target); err != nil {
			return TheaterPackageAudio{}, err
		}
	} else {
		input, _, _, err := AudioOpenLocalVariant(asset, "default")
		if err != nil {
			return TheaterPackageAudio{}, err
		}
		defer input.Close()
		output, err := os.Create(target)
		if err != nil {
			return TheaterPackageAudio{}, err
		}
		_, copyErr := io.Copy(output, input)
		closeErr := output.Close()
		if copyErr != nil {
			return TheaterPackageAudio{}, copyErr
		}
		if closeErr != nil {
			return TheaterPackageAudio{}, closeErr
		}
	}
	file, err := theaterPackageFile(target, "audio/*", filepath.Base(asset.ObjectKey))
	if err != nil {
		return TheaterPackageAudio{}, err
	}
	file.Path = relative
	return TheaterPackageAudio{
		ID: asset.ID, Name: asset.Name, Description: asset.Description, Tags: []string(asset.Tags),
		Visibility: asset.Visibility, DurationSeconds: asset.DurationSeconds, BitrateKbps: asset.BitrateKbps, File: file,
	}, nil
}

func exportTheaterPackageAppearanceAsset(stagingDir string, asset model.TheaterAppearanceAssetModel) (TheaterPackageAppearanceAsset, error) {
	item := TheaterPackageAppearanceAsset{
		ID: asset.ID, Purpose: asset.Purpose, Kind: asset.Kind, MimeType: asset.MimeType,
		SourceMimeType: asset.SourceMimeType, OriginalFilename: asset.OriginalFilename,
		SizeBytes: asset.SizeBytes, ContentHash: asset.ContentHash, Width: asset.Width,
		Height: asset.Height, DurationMS: asset.DurationMS, SourceAttachmentID: asset.SourceAttachmentID,
		DisplayAttachmentID: asset.DisplayAttachmentID, FallbackAttachmentID: asset.FallbackAttachmentID,
	}
	var err error
	item.Source, err = exportAttachmentToPackage(stagingDir, filepath.ToSlash(filepath.Join("appearance", asset.ID, "source")), asset.SourceAttachmentID)
	if err != nil {
		return item, err
	}
	if asset.DisplayAttachmentID != "" {
		file, err := exportAttachmentToPackage(stagingDir, filepath.ToSlash(filepath.Join("appearance", asset.ID, "display")), asset.DisplayAttachmentID)
		if err != nil {
			return item, err
		}
		item.Display = &file
	}
	if asset.FallbackAttachmentID != "" {
		file, err := exportAttachmentToPackage(stagingDir, filepath.ToSlash(filepath.Join("appearance", asset.ID, "fallback")), asset.FallbackAttachmentID)
		if err != nil {
			return item, err
		}
		item.Fallback = &file
	}
	return item, nil
}

func collectJSONFieldStrings(value any, field string) map[string]struct{} {
	result := map[string]struct{}{}
	raw, err := json.Marshal(value)
	if err != nil {
		return result
	}
	var document any
	if json.Unmarshal(raw, &document) != nil {
		return result
	}
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if key == field {
					if text, ok := child.(string); ok && strings.TrimSpace(text) != "" {
						result[text] = struct{}{}
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(document)
	return result
}

func copyTheaterPackageFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func summarizeTheaterSnapshot(snapshot TheaterSharedSnapshot) TheaterPackageSummary {
	summary := TheaterPackageSummary{Scenes: len(snapshot.Scenes), PersistentObjects: len(snapshot.PersistentObjects)}
	for _, scene := range snapshot.Scenes {
		summary.Objects += len(scene.Objects)
	}
	summary.Objects += len(snapshot.PersistentObjects)
	return summary
}

func mapKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
