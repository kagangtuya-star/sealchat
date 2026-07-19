package service

import (
	"archive/zip"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/utils"
)

type theaterPackageRemap struct {
	scenes          map[string]string
	objects         map[string]string
	resources       map[string]string
	audio           map[string]string
	appearance      map[string]string
	attachments     map[string]string
	sourceWorldID   string
	sourceChannelID string
	worldID         string
	channelID       string
}

func importTheaterPackage(ctx context.Context, job *model.TheaterPackageJobModel) (TheaterPackageSummary, error) {
	var summary TheaterPackageSummary
	if job == nil || strings.TrimSpace(job.InputFilePath) == "" {
		return summary, fmt.Errorf("舞台包任务或文件不存在")
	}
	if _, _, err := requireTheaterPermission(job.ActorUserID, job.TargetWorldID, "", TheaterPermissionAdminRestore); err != nil {
		return summary, err
	}
	room, err := model.TheaterRoomCreateIfMissing(job.TargetWorldID, "", job.ActorUserID)
	if err != nil {
		return summary, err
	}
	mutationID := "package-import-" + job.ID
	if existing, found, err := existingTheaterPackageImport(room.ID, mutationID); err != nil {
		return summary, err
	} else if found {
		return existing, nil
	}

	extractDir, err := os.MkdirTemp(theaterPackageStorageDir(), "import-"+job.ID+"-")
	if err != nil {
		return summary, err
	}
	defer os.RemoveAll(extractDir)
	if err := extractTheaterPackageZIP(job.InputFilePath, extractDir); err != nil {
		return summary, err
	}
	manifest, err := loadAndValidateTheaterPackage(extractDir)
	if err != nil {
		return summary, err
	}
	if manifest.PackageVersion != theaterPackageVersion || manifest.SchemaVersion != model.TheaterSchemaVersion {
		return summary, newTheaterError(TheaterErrorSchemaUnsupported, "舞台包版本不受支持", 409, map[string]any{
			"packageVersion": manifest.PackageVersion, "schemaVersion": manifest.SchemaVersion,
		})
	}
	if err := validateTheaterPackageManifestEntities(manifest); err != nil {
		return summary, err
	}

	var snapshot TheaterSharedSnapshot
	if err := decodeStrictJSONFile(theaterPackageAbsolutePath(extractDir, manifest.Document.Path), &snapshot); err != nil {
		return summary, newTheaterError(TheaterErrorSchemaUnsupported, "舞台文档无效", 409, nil)
	}
	if err := validateTheaterSharedSnapshot(snapshot); err != nil {
		return summary, err
	}
	manifestResourceIDs := map[string]struct{}{}
	for _, resource := range manifest.Resources {
		manifestResourceIDs[resource.ID] = struct{}{}
	}
	for resourceID := range snapshot.Resources {
		if _, found := manifestResourceIDs[resourceID]; !found {
			return summary, fmt.Errorf("舞台文档引用未打包资源: %s", resourceID)
		}
	}
	if err := validateTheaterPackageImportLimits(room.ID, snapshot); err != nil {
		return summary, err
	}

	remap := theaterPackageRemap{
		scenes: map[string]string{}, objects: map[string]string{}, resources: map[string]string{},
		audio: map[string]string{}, appearance: map[string]string{}, attachments: map[string]string{},
		sourceWorldID: manifest.SourceWorldID, sourceChannelID: manifest.SourceInputChannelID,
		worldID: job.TargetWorldID, channelID: job.InputChannelID,
	}
	for id := range snapshot.Scenes {
		remap.scenes[id] = utils.NewID()
	}
	for _, scene := range snapshot.Scenes {
		for id := range scene.Objects {
			remap.objects[id] = utils.NewID()
		}
	}
	for id := range snapshot.PersistentObjects {
		if remap.objects[id] == "" {
			remap.objects[id] = utils.NewID()
		}
	}
	for _, resource := range manifest.Resources {
		remap.resources[resource.ID] = utils.NewID()
	}
	for _, asset := range manifest.AppearanceAssets {
		remap.appearance[asset.ID] = utils.NewID()
		for _, attachmentID := range []string{asset.SourceAttachmentID, asset.DisplayAttachmentID, asset.FallbackAttachmentID} {
			if attachmentID != "" {
				remap.attachments[attachmentID] = utils.NewID()
			}
		}
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
		path := theaterPackageAbsolutePath(extractDir, item.File.Path)
		tags := remapTheaterAudioTags(item.Tags, job.InputChannelID)
		worldID := job.TargetWorldID
		asset, err := AudioCreateAssetFromImport(path, AudioUploadOptions{
			Name: item.Name, Tags: tags, Description: item.Description, Visibility: item.Visibility,
			CreatedBy: job.ActorUserID, Scope: model.AudioScopeWorld, WorldID: &worldID,
		})
		if err != nil {
			return summary, fmt.Errorf("导入音频 %s 失败: %w", item.ID, err)
		}
		createdAudio = append(createdAudio, asset)
		remap.audio[item.ID] = asset.ID
		updateTheaterPackageProgress(job.ID, 0.15+0.2*float64(index+1)/float64(maxInt(1, len(manifest.Audio))))
	}

	remappedSnapshot, identityWarnings, err := remapTheaterPackageSnapshot(snapshot, remap)
	if err != nil {
		return summary, err
	}
	summary = summarizeTheaterSnapshot(remappedSnapshot)
	summary.Resources = len(manifest.Resources)
	summary.AudioAssets = len(manifest.Audio)
	summary.AppearanceAssets = len(manifest.AppearanceAssets)
	summary.Warnings = append(summary.Warnings, identityWarnings...)
	for _, sceneID := range remap.scenes {
		summary.ImportedSceneIDs = append(summary.ImportedSceneIDs, sceneID)
	}
	sort.Strings(summary.ImportedSceneIDs)

	updateTheaterPackageProgress(job.ID, 0.4)
	createdMutation := false
	alreadyImported := false
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
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
			Kind: "pre-import", Reason: "舞台包导入", CreatedBy: job.ActorUserID,
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
		appearanceAllowed := job.InputChannelID != ""
		if !appearanceAllowed && len(manifest.AppearanceAssets) > 0 {
			summary.Warnings = append(summary.Warnings, "目标频道缺失，世界演出资源与模板未导入")
			summary.AppearanceAssets = 0
		}
		if appearanceAllowed {
			for _, asset := range manifest.AppearanceAssets {
				if err := importTheaterPackageAppearanceAsset(tx, extractDir, job, asset, remap, &persistedAttachments); err != nil {
					return fmt.Errorf("导入演出资源 %s 失败: %w", asset.ID, err)
				}
			}
		}

		var maxOrder int64
		_ = tx.Model(&model.TheaterSceneModel{}).Where("room_id = ?", current.ID).Select("COALESCE(MAX(sort_order), 0)").Scan(&maxOrder).Error
		sceneIDs := make([]string, 0, len(remappedSnapshot.Scenes))
		for id := range remappedSnapshot.Scenes {
			sceneIDs = append(sceneIDs, id)
		}
		sort.Slice(sceneIDs, func(i, j int) bool {
			left, right := remappedSnapshot.Scenes[sceneIDs[i]], remappedSnapshot.Scenes[sceneIDs[j]]
			if left.Order == right.Order {
				return left.ID < right.ID
			}
			return left.Order < right.Order
		})
		for index, id := range sceneIDs {
			scene := remappedSnapshot.Scenes[id]
			if err := tx.Create(&model.TheaterSceneModel{
				StringPKBaseModel: model.StringPKBaseModel{ID: scene.ID}, RoomID: current.ID,
				Name: scene.Name, SortOrder: maxOrder + int64(index) + 1, Locked: scene.Locked,
				StateJSON: defaultJSON(scene.State, `{}`), SchemaVersion: model.TheaterSchemaVersion,
				CreatedBy: job.ActorUserID, UpdatedBy: job.ActorUserID,
			}).Error; err != nil {
				return err
			}
		}
		for _, id := range sceneIDs {
			scene := remappedSnapshot.Scenes[id]
			if err := createTheaterPackageObjects(tx, &current, job.ActorUserID, &scene.ID, scene.Objects); err != nil {
				return err
			}
		}
		if err := createTheaterPackageObjects(tx, &current, job.ActorUserID, nil, remappedSnapshot.PersistentObjects); err != nil {
			return err
		}
		if err := recalculateTheaterResourceReferences(tx, current.ID); err != nil {
			return err
		}

		var sceneCount int64
		if err := tx.Model(&model.TheaterSceneModel{}).Where("room_id = ?", current.ID).Count(&sceneCount).Error; err != nil {
			return err
		}
		roomUpdates := map[string]any{}
		if sceneCount == int64(len(remappedSnapshot.Scenes)) && remappedSnapshot.ActiveSceneID != nil {
			roomUpdates["active_scene_id"] = *remappedSnapshot.ActiveSceneID
			roomUpdates["state_json"] = defaultJSON(remappedSnapshot.LiveState, `{}`)
			current.ActiveSceneID = *remappedSnapshot.ActiveSceneID
			current.StateJSON = defaultJSON(remappedSnapshot.LiveState, `{}`)
		}

		if manifest.WorldPresentation != nil {
			var targetWorld model.WorldModel
			if err := tx.Where("id = ?", job.TargetWorldID).First(&targetWorld).Error; err != nil {
				return err
			}
			if worldTheaterTemplateEmpty(targetWorld.TheaterPresentationTemplateJSON) && (appearanceAllowed || len(manifest.AppearanceAssets) == 0) {
				raw, err := os.ReadFile(theaterPackageAbsolutePath(extractDir, manifest.WorldPresentation.Path))
				if err != nil {
					return err
				}
				remapped, _, err := remapTheaterPackageJSON(raw, remap)
				if err != nil {
					return err
				}
				if err := tx.Model(&model.WorldModel{}).Where("id = ?", job.TargetWorldID).Update("theater_presentation_template_json", string(remapped)).Error; err != nil {
					return err
				}
				summary.WorldPresentationImported = true
			} else {
				summary.Warnings = append(summary.Warnings, "目标世界已有演出模板，已保留原模板")
			}
		}

		nextRevision := current.Revision + 1
		roomUpdates["revision"] = nextRevision
		roomUpdates["updated_by"] = job.ActorUserID
		roomUpdates["updated_at"] = time.Now()
		cas := tx.Model(&model.TheaterRoomModel{}).Where("id = ? AND revision = ?", current.ID, current.Revision).Updates(roomUpdates)
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
		payload, _ := json.Marshal(map[string]any{"packageId": manifest.PackageID, "jobId": job.ID, "summary": summary})
		payloadHash := theaterJSONHash(payload)
		result := TheaterMutationResult{
			MutationID: mutationID, RevisionBefore: nextRevision - 1, Revision: nextRevision,
			Type: TheaterMutationAdminPackageImport, Payload: payload, Checksum: checksum,
		}
		resultJSON, _ := json.Marshal(result)
		if err := tx.Create(&model.TheaterMutationModel{
			RoomID: current.ID, WorldID: current.WorldID, ChannelID: current.ChannelID, MutationID: mutationID,
			ActorUserID: job.ActorUserID, ExpectedRevision: nextRevision - 1, RevisionBefore: nextRevision - 1,
			RevisionAfter: &nextRevision, Type: TheaterMutationAdminPackageImport, PayloadJSON: string(payload),
			PayloadHash: payloadHash, ResultJSON: string(resultJSON), Status: "applied", RequestSource: "worker",
			RequestID: job.ID,
		}).Error; err != nil {
			return err
		}
		if err := createTheaterAudit(tx, &current, job.ActorUserID, TheaterRequestMeta{Source: "worker", RequestID: job.ID}, mutationID, TheaterMutationAdminPackageImport, "imported", "", "舞台包导入", nextRevision-1, &nextRevision, payload); err != nil {
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

func extractTheaterPackageZIP(source, target string) error {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return fmt.Errorf("舞台包 ZIP 无效: %w", err)
	}
	defer archive.Close()
	if len(archive.File) == 0 || len(archive.File) > theaterPackageMaxFiles {
		return fmt.Errorf("舞台包文件数量无效")
	}
	var expanded int64
	for _, item := range archive.File {
		name := filepath.ToSlash(item.Name)
		clean := filepath.ToSlash(filepath.Clean(name))
		if name == "" || strings.HasPrefix(name, "/") || clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(item.Name) {
			return fmt.Errorf("舞台包包含不安全路径: %s", item.Name)
		}
		if item.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("舞台包不允许符号链接: %s", item.Name)
		}
		expanded += int64(item.UncompressedSize64)
		if expanded > theaterPackageMaxExpandedBytes {
			return fmt.Errorf("舞台包解压大小超过限制")
		}
		path := filepath.Join(target, filepath.FromSlash(clean))
		if item.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		input, err := item.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			input.Close()
			return err
		}
		written, copyErr := io.Copy(output, io.LimitReader(input, theaterPackageMaxExpandedBytes+1))
		closeInputErr := input.Close()
		closeOutputErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeInputErr != nil {
			return closeInputErr
		}
		if closeOutputErr != nil {
			return closeOutputErr
		}
		if written != int64(item.UncompressedSize64) {
			return fmt.Errorf("舞台包文件解压大小不一致: %s", item.Name)
		}
	}
	return nil
}

func loadAndValidateTheaterPackage(root string) (TheaterPackageManifest, error) {
	var manifest TheaterPackageManifest
	if err := decodeStrictJSONFile(filepath.Join(root, "manifest.json"), &manifest); err != nil {
		return manifest, fmt.Errorf("manifest.json 无效: %w", err)
	}
	files := []TheaterPackageFile{manifest.Document}
	if manifest.WorldPresentation != nil {
		files = append(files, *manifest.WorldPresentation)
	}
	for _, resource := range manifest.Resources {
		files = append(files, resource.Original)
		for _, variant := range resource.Variants {
			files = append(files, variant.File)
		}
	}
	for _, audio := range manifest.Audio {
		files = append(files, audio.File)
	}
	for _, asset := range manifest.AppearanceAssets {
		files = append(files, asset.Source)
		if asset.Display != nil {
			files = append(files, *asset.Display)
		}
		if asset.Fallback != nil {
			files = append(files, *asset.Fallback)
		}
	}
	seen := map[string]struct{}{}
	for _, item := range files {
		if !validTheaterPackageRelativePath(item.Path) || item.Size < 0 || len(item.SHA256) != 64 {
			return manifest, fmt.Errorf("manifest 文件声明无效")
		}
		if _, exists := seen[item.Path]; exists {
			return manifest, fmt.Errorf("manifest 文件路径重复: %s", item.Path)
		}
		seen[item.Path] = struct{}{}
		path := theaterPackageAbsolutePath(root, item.Path)
		actual, err := theaterPackageFile(path, item.MimeType, item.Filename)
		if err != nil {
			return manifest, err
		}
		if actual.Size != item.Size || !strings.EqualFold(actual.SHA256, item.SHA256) {
			return manifest, fmt.Errorf("舞台包文件校验失败: %s", item.Path)
		}
	}
	return manifest, nil
}

func validateTheaterPackageManifestEntities(manifest TheaterPackageManifest) error {
	if strings.TrimSpace(manifest.PackageID) == "" {
		return fmt.Errorf("manifest packageId 缺失")
	}
	resourceIDs := map[string]struct{}{}
	for _, item := range manifest.Resources {
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("manifest 资源 ID 缺失")
		}
		if _, exists := resourceIDs[item.ID]; exists {
			return fmt.Errorf("manifest 资源 ID 重复: %s", item.ID)
		}
		resourceIDs[item.ID] = struct{}{}
		variantNames := map[string]struct{}{}
		for _, variant := range item.Variants {
			if strings.TrimSpace(variant.Name) == "" {
				return fmt.Errorf("manifest 资源 variant 名称缺失: %s", item.ID)
			}
			if _, exists := variantNames[variant.Name]; exists {
				return fmt.Errorf("manifest 资源 variant 重复: %s/%s", item.ID, variant.Name)
			}
			variantNames[variant.Name] = struct{}{}
		}
	}
	for _, item := range manifest.Resources {
		if item.PosterResourceID != "" {
			if _, exists := resourceIDs[item.PosterResourceID]; !exists {
				return fmt.Errorf("manifest poster 资源不存在: %s", item.PosterResourceID)
			}
		}
	}
	audioIDs := map[string]struct{}{}
	for _, item := range manifest.Audio {
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("manifest 音频 ID 缺失")
		}
		if _, exists := audioIDs[item.ID]; exists {
			return fmt.Errorf("manifest 音频 ID 重复: %s", item.ID)
		}
		audioIDs[item.ID] = struct{}{}
	}
	appearanceIDs := map[string]struct{}{}
	for _, item := range manifest.AppearanceAssets {
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("manifest 演出资源 ID 缺失")
		}
		if strings.TrimSpace(item.SourceAttachmentID) == "" {
			return fmt.Errorf("manifest 演出资源 sourceAttachmentId 缺失: %s", item.ID)
		}
		if (item.Display == nil) != (item.DisplayAttachmentID == "") || (item.Fallback == nil) != (item.FallbackAttachmentID == "") {
			return fmt.Errorf("manifest 演出资源附件声明不一致: %s", item.ID)
		}
		if _, exists := appearanceIDs[item.ID]; exists {
			return fmt.Errorf("manifest 演出资源 ID 重复: %s", item.ID)
		}
		appearanceIDs[item.ID] = struct{}{}
	}
	return nil
}

func theaterPackageAbsolutePath(root, relative string) string {
	return filepath.Join(root, filepath.FromSlash(filepath.ToSlash(filepath.Clean(relative))))
}

func validTheaterPackageRelativePath(value string) bool {
	value = filepath.ToSlash(strings.TrimSpace(value))
	clean := filepath.ToSlash(filepath.Clean(value))
	return value != "" && !strings.HasPrefix(value, "/") && !filepath.IsAbs(value) && clean != ".." && !strings.HasPrefix(clean, "../")
}

func decodeStrictJSONFile(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return decodeStrictJSON(raw, target)
}

func validateTheaterPackageImportLimits(roomID string, snapshot TheaterSharedSnapshot) error {
	var scenes int64
	if err := model.GetDB().Model(&model.TheaterSceneModel{}).Where("room_id = ?", roomID).Count(&scenes).Error; err != nil {
		return err
	}
	if scenes+int64(len(snapshot.Scenes)) > theaterMaxScenes {
		return newTheaterError(TheaterErrorLimitExceeded, "导入后场景数量超限", 409, map[string]any{"limit": theaterMaxScenes})
	}
	var objects int64
	if err := model.GetDB().Model(&model.TheaterObjectModel{}).Where("room_id = ?", roomID).Count(&objects).Error; err != nil {
		return err
	}
	imported := len(snapshot.PersistentObjects)
	for _, scene := range snapshot.Scenes {
		if len(scene.Objects) > theaterMaxSceneObjects {
			return newTheaterError(TheaterErrorLimitExceeded, "导入场景对象数量超限", 409, map[string]any{"limit": theaterMaxSceneObjects})
		}
		imported += len(scene.Objects)
	}
	if objects+int64(imported) > theaterMaxObjects {
		return newTheaterError(TheaterErrorLimitExceeded, "导入后对象数量超限", 409, map[string]any{"limit": theaterMaxObjects})
	}
	return nil
}

func importTheaterPackageResource(tx *gorm.DB, root string, room *model.TheaterRoomModel, job *model.TheaterPackageJobModel, item TheaterPackageResource, remap theaterPackageRemap, persisted *[]AttachmentLocation) error {
	attachmentID, err := importTheaterPackageAttachment(tx, root, item.Original, job, room.ID, "theater-resource", "", persisted)
	if err != nil {
		return err
	}
	now := time.Now()
	variantsJSON := item.VariantsJSON
	if variantsJSON == "" {
		variantsJSON = "[]"
	} else if raw, _, err := remapTheaterPackageJSON([]byte(variantsJSON), remap); err == nil {
		variantsJSON = string(raw)
	}
	resource := model.TheaterResourceModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: remap.resources[item.ID]}, RoomID: room.ID,
		ClientResourceID: "", AttachmentID: attachmentID, Kind: item.Kind, ContentHash: item.ContentHash,
		SizeBytes: item.SizeBytes, MimeType: item.MimeType, OriginalFilename: item.OriginalFilename,
		Width: item.Width, Height: item.Height, DurationMS: item.DurationMS, FrameCount: item.FrameCount,
		FrameRate: item.FrameRate, Container: item.Container, VideoCodec: item.VideoCodec, AudioCodec: item.AudioCodec,
		Status: "ready", ProcessingProgress: 1, PosterResourceID: remap.resources[item.PosterResourceID],
		VariantsJSON: variantsJSON, CreatedBy: job.ActorUserID, ReadyAt: &now,
	}
	if err := tx.Create(&resource).Error; err != nil {
		return err
	}
	for _, variant := range item.Variants {
		variantAttachmentID, err := importTheaterPackageAttachment(tx, root, variant.File, job, room.ID, "theater-resource-variant", "", persisted)
		if err != nil {
			return err
		}
		if err := tx.Create(&model.TheaterResourceVariantModel{
			ResourceID: resource.ID, Name: variant.Name, AttachmentID: variantAttachmentID,
			MimeType: variant.MimeType, SizeBytes: variant.SizeBytes, Width: variant.Width,
			Height: variant.Height, DurationMS: variant.DurationMS, Status: "ready", ContentHash: variant.ContentHash,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func importTheaterPackageAppearanceAsset(tx *gorm.DB, root string, job *model.TheaterPackageJobModel, item TheaterPackageAppearanceAsset, remap theaterPackageRemap, persisted *[]AttachmentLocation) error {
	sourceID, err := importTheaterPackageAttachment(tx, root, item.Source, job, job.TargetWorldID, "theater-appearance", remap.attachments[item.SourceAttachmentID], persisted)
	if err != nil {
		return err
	}
	displayID := ""
	if item.Display != nil {
		displayID, err = importTheaterPackageAttachment(tx, root, *item.Display, job, job.TargetWorldID, "theater-appearance", remap.attachments[item.DisplayAttachmentID], persisted)
		if err != nil {
			return err
		}
	}
	fallbackID := ""
	if item.Fallback != nil {
		fallbackID, err = importTheaterPackageAttachment(tx, root, *item.Fallback, job, job.TargetWorldID, "theater-appearance", remap.attachments[item.FallbackAttachmentID], persisted)
		if err != nil {
			return err
		}
	}
	if item.SourceAttachmentID != "" {
		remap.attachments[item.SourceAttachmentID] = sourceID
	}
	if item.DisplayAttachmentID != "" {
		remap.attachments[item.DisplayAttachmentID] = displayID
	}
	if item.FallbackAttachmentID != "" {
		remap.attachments[item.FallbackAttachmentID] = fallbackID
	}
	now := time.Now()
	return tx.Create(&model.TheaterAppearanceAssetModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: remap.appearance[item.ID]}, ChannelID: job.InputChannelID,
		OwnerUserID: job.ActorUserID, IdentityID: "", VariantID: "", Purpose: item.Purpose,
		SourceAttachmentID: sourceID, DisplayAttachmentID: displayID, FallbackAttachmentID: fallbackID,
		Kind: item.Kind, MimeType: item.MimeType, SourceMimeType: item.SourceMimeType,
		OriginalFilename: item.OriginalFilename, SizeBytes: item.SizeBytes, ContentHash: item.ContentHash,
		Width: item.Width, Height: item.Height, DurationMS: item.DurationMS, Status: "ready", Progress: 1,
		CreatedBy: job.ActorUserID, ReadyAt: &now,
	}).Error
}

func importTheaterPackageAttachment(tx *gorm.DB, root string, item TheaterPackageFile, job *model.TheaterPackageJobModel, rootID, rootType, attachmentID string, persisted *[]AttachmentLocation) (string, error) {
	hash, err := hex.DecodeString(item.SHA256)
	if err != nil {
		return "", err
	}
	if attachmentID != "" {
		var existing model.AttachmentModel
		if err := tx.Select("id", "hash", "size").Where("id = ?", attachmentID).Limit(1).Find(&existing).Error; err != nil {
			return "", err
		}
		if existing.ID != "" {
			if existing.Size != item.Size || !strings.EqualFold(hex.EncodeToString(existing.Hash), item.SHA256) {
				return "", fmt.Errorf("复用附件 %s 的文件内容不一致", attachmentID)
			}
			return existing.ID, nil
		}
	}
	path := theaterPackageAbsolutePath(root, item.Path)
	location, err := PersistAttachmentFileForceNew(hash, item.Size, path, item.MimeType, item.Filename)
	if err != nil {
		return "", err
	}
	if persisted != nil {
		*persisted = append(*persisted, *location)
	}
	attachment := model.AttachmentModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: attachmentID},
		Hash:              model.ByteArray(hash), Filename: item.Filename, Size: item.Size, MimeType: item.MimeType,
		UserID: job.ActorUserID, ChannelID: job.InputChannelID, StorageType: location.StorageType,
		ObjectKey: location.ObjectKey, ExternalURL: location.ExternalURL, RootID: rootID, RootIDType: rootType, IsTemp: false,
	}
	if err := tx.Create(&attachment).Error; err != nil {
		return "", err
	}
	return attachment.ID, nil
}

func remapTheaterPackageSnapshot(snapshot TheaterSharedSnapshot, remap theaterPackageRemap) (TheaterSharedSnapshot, []string, error) {
	warnings := []string{}
	result := TheaterSharedSnapshot{
		Scenes: map[string]TheaterSceneSnapshot{}, PersistentObjects: map[string]TheaterObjectSnapshot{},
		Characters: map[string]TheaterObjectSnapshot{}, Resources: map[string]TheaterResourcePublic{},
	}
	if snapshot.ActiveSceneID != nil {
		if id := remap.scenes[*snapshot.ActiveSceneID]; id != "" {
			result.ActiveSceneID = &id
		}
	}
	var changed bool
	var err error
	result.LiveState, changed, err = remapTheaterPackageJSON(snapshot.LiveState, remap)
	if err != nil {
		return result, warnings, err
	}
	if changed {
		warnings = appendWarning(warnings, "部分世界、频道或身份引用已按目标世界重写")
	}
	for oldID, scene := range snapshot.Scenes {
		newID := remap.scenes[oldID]
		state, sceneChanged, err := remapTheaterPackageJSON(scene.State, remap)
		if err != nil {
			return result, warnings, err
		}
		if sceneChanged {
			warnings = appendWarning(warnings, "部分世界、频道或身份引用已按目标世界重写")
		}
		newScene := TheaterSceneSnapshot{ID: newID, Name: scene.Name, Order: scene.Order, Locked: scene.Locked, State: state, Objects: map[string]TheaterObjectSnapshot{}}
		for objectID, object := range scene.Objects {
			mapped, objectChanged, err := remapTheaterPackageObject(object, remap)
			if err != nil {
				return result, warnings, err
			}
			if objectChanged {
				warnings = appendWarning(warnings, "无法映射的身份、角色或用户引用已清空")
			}
			newScene.Objects[remap.objects[objectID]] = mapped
		}
		result.Scenes[newID] = newScene
	}
	for oldID, object := range snapshot.PersistentObjects {
		mapped, objectChanged, err := remapTheaterPackageObject(object, remap)
		if err != nil {
			return result, warnings, err
		}
		if objectChanged {
			warnings = appendWarning(warnings, "无法映射的身份、角色或用户引用已清空")
		}
		result.PersistentObjects[remap.objects[oldID]] = mapped
		if mapped.Kind == "character" {
			result.Characters[mapped.ID] = mapped
		}
	}
	return result, warnings, nil
}

func remapTheaterPackageObject(object TheaterObjectSnapshot, remap theaterPackageRemap) (TheaterObjectSnapshot, bool, error) {
	object.ID = remap.objects[object.ID]
	if object.ParentID != nil {
		if parent := remap.objects[*object.ParentID]; parent != "" {
			object.ParentID = &parent
		} else {
			object.ParentID = nil
		}
	}
	identityChanged := object.OwnerUserID != nil || object.CharacterIdentityID != nil
	object.OwnerUserID = nil
	object.CharacterIdentityID = nil
	var changed bool
	var err error
	object.Content, changed, err = remapTheaterPackageJSON(object.Content, remap)
	if err != nil {
		return object, identityChanged, err
	}
	identityChanged = identityChanged || changed
	object.Actions, changed, err = remapTheaterPackageJSON(object.Actions, remap)
	if err != nil {
		return object, identityChanged, err
	}
	identityChanged = identityChanged || changed
	object.Metadata, changed, err = remapTheaterPackageJSON(object.Metadata, remap)
	return object, identityChanged || changed, err
}

func remapTheaterPackageJSON(raw []byte, remap theaterPackageRemap) (json.RawMessage, bool, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return raw, false, nil
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, false, err
	}
	changed := false
	identityFields := map[string]struct{}{
		"identityId": {}, "identityVariantId": {}, "characterId": {}, "targetUserId": {},
		"ownerUserId": {}, "userId": {},
	}
	var walk func(any) any
	walk = func(current any) any {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if _, clear := identityFields[key]; clear {
					if child != nil && child != "" {
						changed = true
					}
					typed[key] = nil
					continue
				}
				if text, ok := child.(string); ok {
					mapped := ""
					knownReference := false
					switch key {
					case "sceneId":
						knownReference = true
						mapped = remap.scenes[text]
					case "objectId", "parentId":
						knownReference = true
						mapped = remap.objects[text]
					case "resourceId", "posterResourceId":
						knownReference = true
						mapped = remap.resources[text]
					case "assetId":
						knownReference = true
						mapped = remap.audio[text]
						if mapped == "" {
							mapped = remap.appearance[text]
						}
					case "worldId":
						knownReference = true
						mapped = remap.worldID
					case "channelId", "inputChannelId", "targetChannelId", "sourceChannelId":
						knownReference = true
						mapped = remap.channelID
					case "attachmentId", "sourceAttachmentId", "displayAttachmentId", "resourceAttachmentId", "fallbackAttachmentId":
						knownReference = true
						mapped = remap.attachments[text]
					}
					if mapped != "" && mapped != text {
						typed[key] = mapped
						changed = true
						continue
					}
					if knownReference && text != "" {
						typed[key] = nil
						changed = true
						continue
					}
					replaced := remapTheaterPackageString(text, remap)
					if replaced != text {
						typed[key] = replaced
						changed = true
					}
					continue
				}
				typed[key] = walk(child)
			}
			return typed
		case []any:
			for index, child := range typed {
				typed[index] = walk(child)
			}
			return typed
		default:
			return current
		}
	}
	value = walk(value)
	result, err := json.Marshal(value)
	return result, changed, err
}

func remapTheaterPackageString(value string, remap theaterPackageRemap) string {
	result := value
	for oldID, newID := range remap.resources {
		if strings.Contains(result, oldID) {
			result = strings.ReplaceAll(result, oldID, newID)
		}
	}
	for oldID, newID := range remap.audio {
		if strings.Contains(result, oldID) {
			result = strings.ReplaceAll(result, oldID, newID)
		}
	}
	if remap.sourceWorldID != "" && remap.worldID != "" && remap.sourceWorldID != remap.worldID {
		result = strings.ReplaceAll(result, remap.sourceWorldID, remap.worldID)
	}
	if remap.sourceChannelID != "" && remap.channelID != "" && remap.sourceChannelID != remap.channelID {
		result = strings.ReplaceAll(result, remap.sourceChannelID, remap.channelID)
	}
	return result
}

func createTheaterPackageObjects(tx *gorm.DB, room *model.TheaterRoomModel, actorID string, sceneID *string, objects map[string]TheaterObjectSnapshot) error {
	pending := make(map[string]TheaterObjectSnapshot, len(objects))
	for id, object := range objects {
		pending[id] = object
	}
	for len(pending) > 0 {
		progress := false
		ids := make([]string, 0, len(pending))
		for id := range pending {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			item := pending[id]
			if item.ParentID != nil {
				if _, stillPending := pending[*item.ParentID]; stillPending {
					continue
				}
			}
			scale := item.Scale
			if scale <= 0 {
				scale = 1
			}
			scaleX, scaleY := item.ScaleX, item.ScaleY
			if scaleX <= 0 {
				scaleX = scale
			}
			if scaleY <= 0 {
				scaleY = scale
			}
			input := theaterObjectInput{
				ID: item.ID, ParentID: item.ParentID, Kind: item.Kind, Name: item.Name,
				X: item.X, Y: item.Y, Width: item.Width, Height: item.Height, Rotation: item.Rotation,
				Scale: &scale, ScaleX: &scaleX, ScaleY: &scaleY, Z: item.Z, OrderKey: item.OrderKey,
				Visible: &item.Visible, Locked: item.Locked, AspectRatioLocked: item.AspectRatioLocked,
				Interactive: item.Interactive, Editable: item.Editable, Content: item.Content,
				Actions: item.Actions, Metadata: item.Metadata,
			}
			if err := validateObjectInput(&input); err != nil {
				return err
			}
			if err := createTheaterObject(tx, room, actorID, sceneID, &input); err != nil {
				return err
			}
			delete(pending, id)
			progress = true
		}
		if !progress {
			return theaterPayloadError("导入对象存在无效 parent 循环")
		}
	}
	return nil
}

func existingTheaterPackageImport(roomID, mutationID string) (TheaterPackageSummary, bool, error) {
	var mutation model.TheaterMutationModel
	if err := model.GetDB().Where("room_id = ? AND mutation_id = ?", roomID, mutationID).Limit(1).Find(&mutation).Error; err != nil {
		return TheaterPackageSummary{}, false, err
	}
	if mutation.ID == "" {
		return TheaterPackageSummary{}, false, nil
	}
	var payload struct {
		Summary TheaterPackageSummary `json:"summary"`
	}
	if err := json.Unmarshal([]byte(mutation.PayloadJSON), &payload); err != nil {
		return TheaterPackageSummary{}, false, err
	}
	return payload.Summary, true, nil
}

func remapTheaterAudioTags(tags []string, channelID string) []string {
	result := make([]string, 0, len(tags)+1)
	hasFeature := false
	for _, tag := range tags {
		if strings.HasPrefix(tag, "theater-channel:") {
			if channelID != "" {
				result = append(result, theaterChannelAudioTag(channelID))
			}
			continue
		}
		if tag == theaterFeatureAudioTag {
			hasFeature = true
		}
		result = append(result, tag)
	}
	if !hasFeature {
		result = append(result, theaterFeatureAudioTag)
	}
	return result
}

func worldTheaterTemplateEmpty(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	return trimmed == "" || trimmed == "{}" || trimmed == "null"
}

func appendWarning(warnings []string, value string) []string {
	for _, existing := range warnings {
		if existing == value {
			return warnings
		}
	}
	return append(warnings, value)
}
