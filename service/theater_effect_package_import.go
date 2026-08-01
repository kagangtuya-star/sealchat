package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/utils"
)

func importTheaterEffectsPackage(ctx context.Context, job *model.TheaterPackageJobModel, room *model.TheaterRoomModel, extractDir string, manifest TheaterPackageManifest) (TheaterPackageSummary, error) {
	var summary TheaterPackageSummary
	if manifest.WorldPresentation != nil || len(manifest.AppearanceAssets) > 0 {
		return summary, theaterPayloadError("特效包不能包含世界演出模板或演出资源")
	}
	var document TheaterPackageEffectsDocument
	if err := decodeStrictJSONFile(theaterPackageAbsolutePath(extractDir, manifest.Document.Path), &document); err != nil || document.Version != 1 {
		return summary, newTheaterError(TheaterErrorSchemaUnsupported, "特效包文档无效", 409, nil)
	}
	if len(document.Effects) == 0 || len(document.Effects) > theaterMaxObjects {
		return summary, theaterPayloadError("特效包数量无效")
	}
	manifestResources := map[string]struct{}{}
	for _, resource := range manifest.Resources {
		manifestResources[resource.ID] = struct{}{}
	}
	for resourceID := range collectJSONFieldStrings(document, "resourceId") {
		if _, found := manifestResources[resourceID]; !found {
			return summary, fmt.Errorf("特效包引用未打包资源: %s", resourceID)
		}
	}
	manifestAudio := map[string]struct{}{}
	for _, audio := range manifest.Audio {
		manifestAudio[audio.ID] = struct{}{}
	}
	for audioID := range collectJSONFieldStrings(document, "assetId") {
		if _, found := manifestAudio[audioID]; !found {
			return summary, fmt.Errorf("特效包引用未打包音频: %s", audioID)
		}
	}

	remap := theaterPackageRemap{
		scenes: map[string]string{}, objects: map[string]string{}, resources: map[string]string{},
		audio: map[string]string{}, appearance: map[string]string{}, attachments: map[string]string{},
		sourceWorldID: manifest.SourceWorldID, sourceChannelID: manifest.SourceInputChannelID,
		worldID: job.TargetWorldID, channelID: job.InputChannelID, resourceChannelID: room.ChannelID,
	}
	seenEffects := map[string]struct{}{}
	for _, entry := range document.Effects {
		if entry.Scope != "scene" && entry.Scope != "persistent" {
			return summary, theaterPayloadError("特效包 scope 无效")
		}
		if entry.Object.Kind != "effect" || strings.TrimSpace(entry.Object.ID) == "" {
			return summary, theaterPayloadError("特效包包含非特效对象")
		}
		if _, exists := seenEffects[entry.Object.ID]; exists {
			return summary, theaterPayloadError("特效包对象 ID 重复")
		}
		seenEffects[entry.Object.ID] = struct{}{}
		remap.objects[entry.Object.ID] = utils.NewID()
	}
	for _, resource := range manifest.Resources {
		remap.resources[resource.ID] = utils.NewID()
	}

	createdAudio := make([]*model.AudioAsset, 0, len(manifest.Audio))
	persistedAttachments := make([]AttachmentLocation, 0)
	cleanupAudio := true
	cleanupAttachments := true
	defer func() {
		if !cleanupAudio {
			return
		}
		for _, asset := range createdAudio {
			audioCleanupPersistedAsset(asset)
			_ = model.GetDB().Unscoped().Delete(&model.AudioAsset{}, "id = ?", asset.ID).Error
		}
	}()
	defer func() {
		if !cleanupAttachments {
			return
		}
		manager := GetStorageManager()
		if manager == nil {
			return
		}
		for _, location := range persistedAttachments {
			_ = manager.Delete(ctx, convertModelToBackend(location.StorageType), location.ObjectKey)
		}
	}()
	for index, item := range manifest.Audio {
		worldID := job.TargetWorldID
		asset, err := AudioCreateAssetFromImport(theaterPackageAbsolutePath(extractDir, item.File.Path), AudioUploadOptions{
			Name: theaterPackageAudioImportName(item), Tags: remapTheaterAudioTags(item.Tags, job.InputChannelID, false), Description: item.Description,
			Visibility: item.Visibility, CreatedBy: job.ActorUserID, Scope: model.AudioScopeWorld, WorldID: &worldID,
		})
		if err != nil {
			return summary, fmt.Errorf("导入音频 %s 失败: %w", item.ID, err)
		}
		createdAudio = append(createdAudio, asset)
		remap.audio[item.ID] = asset.ID
		updateTheaterPackageProgress(job.ID, 0.15+0.2*float64(index+1)/float64(maxInt(1, len(manifest.Audio))))
	}

	remappedEntries := make([]TheaterPackageEffectEntry, 0, len(document.Effects))
	for _, entry := range document.Effects {
		entry.Object.SceneID = nil
		entry.Object.ParentID = nil
		mapped, changed, err := remapTheaterPackageObject(entry.Object, remap)
		if err != nil {
			return summary, err
		}
		if changed {
			summary.Warnings = appendWarning(summary.Warnings, "无法映射的身份、角色或用户引用已清空")
		}
		entry.Object = mapped
		remappedEntries = append(remappedEntries, entry)
	}
	summary.PackageKind = TheaterPackageKindEffects
	summary.Effects = len(remappedEntries)
	summary.Objects = len(remappedEntries)
	summary.Resources = len(manifest.Resources)
	summary.AudioAssets = len(manifest.Audio)
	sceneEffects := 0
	for _, entry := range remappedEntries {
		if entry.Scope == "scene" {
			sceneEffects++
		} else {
			summary.PersistentObjects++
		}
	}
	if sceneEffects > 0 {
		summary.Warnings = appendWarning(summary.Warnings, fmt.Sprintf("%d 个场景特效已导入当前场景", sceneEffects))
	}

	mutationID := "package-import-" + job.ID
	if existing, found, err := existingTheaterPackageImport(room.ID, mutationID); err != nil {
		return summary, err
	} else if found {
		return existing, nil
	}
	createdMutation := false
	alreadyImported := false
	err := model.GetDB().Transaction(func(tx *gorm.DB) error {
		var existing model.TheaterMutationModel
		if err := tx.Where("room_id = ? AND mutation_id = ?", room.ID, mutationID).Limit(1).Find(&existing).Error; err != nil {
			return err
		}
		if existing.ID != "" {
			alreadyImported = true
			return nil
		}
		var current model.TheaterRoomModel
		if err := tx.Where("id = ?", room.ID).First(&current).Error; err != nil {
			return err
		}
		if strings.TrimSpace(current.ActiveSceneID) == "" {
			return theaterPayloadError("当前小剧场没有可接收场景特效的活动场景")
		}
		var objectCount int64
		if err := tx.Model(&model.TheaterObjectModel{}).Where("room_id = ?", current.ID).Count(&objectCount).Error; err != nil {
			return err
		}
		if objectCount+int64(len(remappedEntries)) > theaterMaxObjects {
			return newTheaterError(TheaterErrorLimitExceeded, "对象数量超限", 409, map[string]any{"limit": theaterMaxObjects})
		}
		currentSnapshot, currentHash, err := buildTheaterSnapshot(tx, &current, true)
		if err != nil {
			return err
		}
		currentJSON, _, err := canonicalTheaterJSON(currentSnapshot)
		if err != nil {
			return err
		}
		preImport := &model.TheaterSnapshotModel{
			RoomID: current.ID, Revision: current.Revision, SchemaVersion: current.SchemaVersion,
			SnapshotJSON: string(currentJSON), SnapshotHash: currentHash, SnapshotBytes: int64(len(currentJSON)),
			Kind: "pre-import", Reason: "特效包导入", CreatedBy: job.ActorUserID,
		}
		if err := tx.Create(preImport).Error; err != nil {
			return err
		}
		preImportExpiresAt := time.Now().Add(theaterSnapshotRetention)
		if err := createTheaterResourceHolds(tx, preImport, &preImportExpiresAt); err != nil {
			return err
		}
		for _, resource := range manifest.Resources {
			if err := importTheaterPackageResource(tx, extractDir, &current, job, resource, remap, &persistedAttachments); err != nil {
				return fmt.Errorf("导入资源 %s 失败: %w", resource.ID, err)
			}
		}
		for _, entry := range remappedEntries {
			var sceneID *string
			if entry.Scope == "scene" {
				value := current.ActiveSceneID
				sceneID = &value
			}
			if err := createTheaterPackageObjects(tx, &current, job.ActorUserID, sceneID, map[string]TheaterObjectSnapshot{entry.Object.ID: entry.Object}); err != nil {
				return err
			}
		}
		if err := importTheaterPackageEffectOrganizer(tx, extractDir, &current, job.ActorUserID, manifest, remap); err != nil {
			return err
		}
		if err := recalculateTheaterResourceReferences(tx, current.ID); err != nil {
			return err
		}
		nextRevision := current.Revision + 1
		cas := tx.Model(&model.TheaterRoomModel{}).Where("id = ? AND revision = ?", current.ID, current.Revision).Updates(map[string]any{
			"revision": nextRevision, "updated_by": job.ActorUserID, "updated_at": time.Now(),
		})
		if cas.Error != nil {
			return cas.Error
		}
		if cas.RowsAffected != 1 {
			return errTheaterConcurrentCAS
		}
		current.Revision = nextRevision
		_, checksum, err := buildTheaterSnapshot(tx, &current, true)
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"packageId": manifest.PackageID, "packageKind": manifest.PackageKind, "jobId": job.ID, "summary": summary})
		result := TheaterMutationResult{MutationID: mutationID, RevisionBefore: nextRevision - 1, Revision: nextRevision, Type: TheaterMutationAdminPackageImport, Payload: payload, Checksum: checksum}
		resultJSON, _ := json.Marshal(result)
		if err := tx.Create(&model.TheaterMutationModel{
			RoomID: current.ID, WorldID: current.WorldID, ChannelID: current.ChannelID, MutationID: mutationID,
			ActorUserID: job.ActorUserID, ExpectedRevision: nextRevision - 1, RevisionBefore: nextRevision - 1,
			RevisionAfter: &nextRevision, Type: TheaterMutationAdminPackageImport, PayloadJSON: string(payload),
			PayloadHash: theaterJSONHash(payload), ResultJSON: string(resultJSON), Status: "applied", RequestSource: "worker", RequestID: job.ID,
		}).Error; err != nil {
			return err
		}
		if err := createTheaterAudit(tx, &current, job.ActorUserID, TheaterRequestMeta{Source: "worker", RequestID: job.ID}, mutationID, TheaterMutationAdminPackageImport, "imported", "", "特效包导入", nextRevision-1, &nextRevision, payload); err != nil {
			return err
		}
		if err := tx.Model(&model.TheaterRoomModel{}).Where("id = ?", current.ID).Update("state_hash", checksum).Error; err != nil {
			return err
		}
		createdMutation = true
		return nil
	})
	if err != nil {
		if err == errTheaterConcurrentCAS {
			return summary, newTheaterError(TheaterErrorRevisionConflict, "导入时 Theater revision 冲突", 409, nil)
		}
		return summary, err
	}
	if alreadyImported {
		existing, _, err := existingTheaterPackageImport(room.ID, mutationID)
		return existing, err
	}
	cleanupAudio = false
	cleanupAttachments = false
	if createdMutation {
		EnqueueTheaterMutation(mutationID)
	}
	updateTheaterPackageProgress(job.ID, 0.99)
	return summary, nil
}

func importTheaterPackageEffectOrganizer(tx *gorm.DB, extractDir string, room *model.TheaterRoomModel, actorID string, manifest TheaterPackageManifest, remap theaterPackageRemap) error {
	if manifest.EffectOrganizer == nil {
		return nil
	}
	var organizer TheaterPackageEffectOrganizer
	if err := decodeStrictJSONFile(theaterPackageAbsolutePath(extractDir, manifest.EffectOrganizer.Path), &organizer); err != nil {
		return theaterPayloadError("特效文件夹文档无效")
	}
	if len(organizer.Folders) > 1000 || len(organizer.Items) > theaterMaxObjects {
		return theaterPayloadError("特效文件夹数据超限")
	}
	sort.Slice(organizer.Folders, func(i, j int) bool {
		if organizer.Folders[i].SortOrder == organizer.Folders[j].SortOrder {
			return organizer.Folders[i].ID < organizer.Folders[j].ID
		}
		return organizer.Folders[i].SortOrder < organizer.Folders[j].SortOrder
	})
	var existing []model.TheaterPanelFolderModel
	if err := tx.Where("room_id = ? AND domain = ?", room.ID, TheaterPanelDomainEffect).Find(&existing).Error; err != nil {
		return err
	}
	usedNames := map[string]struct{}{}
	seenFolderIDs := map[string]struct{}{}
	maximumOrder := int64(-1)
	for _, folder := range existing {
		usedNames[folder.Name] = struct{}{}
		if folder.SortOrder > maximumOrder {
			maximumOrder = folder.SortOrder
		}
	}
	folderRemap := map[string]string{}
	for index, source := range organizer.Folders {
		if strings.TrimSpace(source.ID) == "" || strings.TrimSpace(source.Name) == "" {
			return theaterPayloadError("特效文件夹 ID 或名称缺失")
		}
		if _, duplicate := seenFolderIDs[source.ID]; duplicate {
			return theaterPayloadError("特效文件夹 ID 重复")
		}
		seenFolderIDs[source.ID] = struct{}{}
		name := strings.TrimSpace(source.Name)
		if nameLength := len([]rune(name)); nameLength == 0 || nameLength > 128 {
			return theaterPayloadError("特效文件夹名称长度必须为 1-128")
		}
		if _, exists := usedNames[name]; exists {
			baseRunes := []rune(name)
			if len(baseRunes) > 112 {
				baseRunes = baseRunes[:112]
			}
			base := string(baseRunes)
			for suffix := 2; ; suffix++ {
				candidate := fmt.Sprintf("%s（导入 %d）", base, suffix)
				if _, taken := usedNames[candidate]; !taken {
					name = candidate
					break
				}
			}
		}
		usedNames[name] = struct{}{}
		folder := &model.TheaterPanelFolderModel{RoomID: room.ID, Domain: TheaterPanelDomainEffect, Name: name, SortOrder: maximumOrder + int64(index) + 1, CreatedBy: actorID, UpdatedBy: actorID}
		folder.Init()
		if err := tx.Create(folder).Error; err != nil {
			return err
		}
		folderRemap[source.ID] = folder.ID
	}
	seenTargets := map[string]struct{}{}
	mappedTargets := map[string]string{}
	for _, source := range organizer.Items {
		if strings.TrimSpace(source.TargetID) == "" {
			return theaterPayloadError("特效文件夹项目 targetId 缺失")
		}
		if _, duplicate := seenTargets[source.TargetID]; duplicate {
			return theaterPayloadError("特效文件夹项目重复")
		}
		seenTargets[source.TargetID] = struct{}{}
		targetID := remap.objects[source.TargetID]
		if targetID == "" {
			continue
		}
		mappedTargets[source.TargetID] = targetID
	}
	if len(mappedTargets) > 0 {
		ids := make([]string, 0, len(mappedTargets))
		for _, targetID := range mappedTargets {
			ids = append(ids, targetID)
		}
		var effectCount int64
		if err := tx.Model(&model.TheaterObjectModel{}).Where("room_id = ? AND id IN ? AND kind = ?", room.ID, ids, "effect").Count(&effectCount).Error; err != nil {
			return err
		}
		if effectCount != int64(len(ids)) {
			return theaterPayloadError("特效文件夹项目目标不是特效")
		}
	}
	for _, source := range organizer.Items {
		targetID := mappedTargets[source.TargetID]
		if targetID == "" {
			continue
		}
		folderID := ""
		if source.FolderID != "" {
			folderID = folderRemap[source.FolderID]
			if folderID == "" {
				return theaterPayloadError("特效文件夹引用不存在")
			}
		}
		item := &model.TheaterPanelItemModel{RoomID: room.ID, Domain: TheaterPanelDomainEffect, TargetID: targetID, FolderID: folderID, SortOrder: source.SortOrder}
		item.Init()
		if err := tx.Create(item).Error; err != nil {
			return err
		}
	}
	return nil
}
