package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/utils"
)

const (
	ccfoliaBackupVersion       = "1.1.0"
	ccfoliaMaxDataBytes        = int64(16 << 20)
	ccfoliaMaxFileBytes        = int64(512 << 20)
	ccfoliaMaxCompressionRatio = uint64(200)
)

var ccfoliaAssetNamePattern = regexp.MustCompile(`^([0-9a-f]{64})\.(png|gif|jpe?g|webp)$`)

type ccfoliaBackup struct {
	Meta      ccfoliaMeta                          `json:"meta"`
	Entities  ccfoliaEntities                      `json:"entities"`
	Resources map[string]ccfoliaResourceDescriptor `json:"resources"`
	Unknown   map[string]json.RawMessage           `json:"-"`
}

func (item *ccfoliaBackup) UnmarshalJSON(data []byte) error {
	type plain ccfoliaBackup
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var unknown map[string]json.RawMessage
	if err := json.Unmarshal(data, &unknown); err != nil {
		return err
	}
	delete(unknown, "meta")
	delete(unknown, "entities")
	delete(unknown, "resources")
	*item = ccfoliaBackup(value)
	item.Unknown = unknown
	return nil
}

type ccfoliaMeta struct {
	Version string          `json:"version"`
	Raw     json.RawMessage `json:"-"`
}

func (item *ccfoliaMeta) UnmarshalJSON(data []byte) error {
	type plain ccfoliaMeta
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*item = ccfoliaMeta(value)
	item.Raw = append(item.Raw[:0], data...)
	return nil
}

type ccfoliaEntities struct {
	Room        ccfoliaRoom                `json:"room"`
	Items       map[string]ccfoliaItem     `json:"items"`
	Scenes      map[string]ccfoliaScene    `json:"scenes"`
	Unsupported map[string]json.RawMessage `json:"-"`
}

func (item *ccfoliaEntities) UnmarshalJSON(data []byte) error {
	type plain ccfoliaEntities
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var unsupported map[string]json.RawMessage
	if err := json.Unmarshal(data, &unsupported); err != nil {
		return err
	}
	delete(unsupported, "room")
	delete(unsupported, "items")
	delete(unsupported, "scenes")
	*item = ccfoliaEntities(value)
	item.Unsupported = unsupported
	if item.Items == nil {
		item.Items = map[string]ccfoliaItem{}
	}
	if item.Scenes == nil {
		item.Scenes = map[string]ccfoliaScene{}
	}
	return nil
}

type ccfoliaRoom struct {
	BackgroundURL     *string                  `json:"backgroundUrl"`
	ForegroundURL     *string                  `json:"foregroundUrl"`
	FieldWidth        float64                  `json:"fieldWidth"`
	FieldHeight       float64                  `json:"fieldHeight"`
	FieldObjectFit    string                   `json:"fieldObjectFit"`
	AlignWithGrid     bool                     `json:"alignWithGrid"`
	Markers           map[string]ccfoliaMarker `json:"markers"`
	BackgroundColor   string                   `json:"backgroundColor"`
	DisplayGrid       bool                     `json:"displayGrid"`
	GridSize          float64                  `json:"gridSize"`
	EnableCrossfade   bool                     `json:"enableCrossfade"`
	CrossfadeDuration float64                  `json:"crossfadeDuration"`
	Raw               json.RawMessage          `json:"-"`
}

func (item *ccfoliaRoom) UnmarshalJSON(data []byte) error {
	type plain ccfoliaRoom
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*item = ccfoliaRoom(value)
	item.Raw = append(item.Raw[:0], data...)
	if item.Markers == nil {
		item.Markers = map[string]ccfoliaMarker{}
	}
	return nil
}

type ccfoliaScene struct {
	Name           string                   `json:"name"`
	BackgroundURL  *string                  `json:"backgroundUrl"`
	ForegroundURL  *string                  `json:"foregroundUrl"`
	FieldObjectFit string                   `json:"fieldObjectFit"`
	FieldWidth     float64                  `json:"fieldWidth"`
	FieldHeight    float64                  `json:"fieldHeight"`
	DisplayGrid    bool                     `json:"displayGrid"`
	GridSize       float64                  `json:"gridSize"`
	Markers        map[string]ccfoliaMarker `json:"markers"`
	Locked         bool                     `json:"locked"`
	Order          float64                  `json:"order"`
	Raw            json.RawMessage          `json:"-"`
}

func (item *ccfoliaScene) UnmarshalJSON(data []byte) error {
	type plain ccfoliaScene
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*item = ccfoliaScene(value)
	item.Raw = append(item.Raw[:0], data...)
	if item.Markers == nil {
		item.Markers = map[string]ccfoliaMarker{}
	}
	return nil
}

type ccfoliaMarker struct {
	X           float64             `json:"x"`
	Y           float64             `json:"y"`
	Z           float64             `json:"z"`
	Width       float64             `json:"width"`
	Height      float64             `json:"height"`
	Locked      bool                `json:"locked"`
	Freezed     bool                `json:"freezed"`
	Text        string              `json:"text"`
	ImageURL    string              `json:"imageUrl"`
	ClickAction *ccfoliaClickAction `json:"clickAction"`
	Raw         json.RawMessage     `json:"-"`
}

func (item *ccfoliaMarker) UnmarshalJSON(data []byte) error {
	type plain ccfoliaMarker
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*item = ccfoliaMarker(value)
	item.Raw = append(item.Raw[:0], data...)
	return nil
}

type ccfoliaClickAction struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ccfoliaItem struct {
	X             float64             `json:"x"`
	Y             float64             `json:"y"`
	Z             float64             `json:"z"`
	Angle         float64             `json:"angle"`
	Width         float64             `json:"width"`
	Height        float64             `json:"height"`
	Locked        bool                `json:"locked"`
	Visible       bool                `json:"visible"`
	Closed        bool                `json:"closed"`
	WithoutOwner  bool                `json:"withoutOwner"`
	Freezed       bool                `json:"freezed"`
	Type          string              `json:"type"`
	Active        bool                `json:"active"`
	Memo          string              `json:"memo"`
	ImageURL      string              `json:"imageUrl"`
	CoverImageURL *string             `json:"coverImageUrl"`
	ClickAction   *ccfoliaClickAction `json:"clickAction"`
	Order         float64             `json:"order"`
	Raw           json.RawMessage     `json:"-"`
}

func (item *ccfoliaItem) UnmarshalJSON(data []byte) error {
	type plain ccfoliaItem
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*item = ccfoliaItem(value)
	item.Raw = append(item.Raw[:0], data...)
	return nil
}

type ccfoliaResourceDescriptor struct {
	Type string          `json:"type"`
	Raw  json.RawMessage `json:"-"`
}

func (item *ccfoliaResourceDescriptor) UnmarshalJSON(data []byte) error {
	type plain ccfoliaResourceDescriptor
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*item = ccfoliaResourceDescriptor(value)
	item.Raw = append(item.Raw[:0], data...)
	return nil
}

type ccfoliaAssetTarget struct {
	ResourceID      string
	MimeType        string
	Animated        bool
	PlaybackVariant string
}

type ccfoliaConversion struct {
	Snapshot TheaterSharedSnapshot
	Summary  TheaterPackageSummary
}

func importCCFOLIATheaterPackage(ctx context.Context, job *model.TheaterPackageJobModel) (TheaterPackageSummary, error) {
	var summary TheaterPackageSummary
	if job == nil || strings.TrimSpace(job.InputFilePath) == "" {
		return summary, fmt.Errorf("CCFOLIA 导入任务或文件不存在")
	}
	if _, _, err := requireTheaterPermission(job.ActorUserID, job.TargetWorldID, "", TheaterPermissionAdminRestore); err != nil {
		return summary, err
	}
	room, err := model.TheaterRoomCreateIfMissing(job.TargetWorldID, "", job.ActorUserID)
	if err != nil {
		return summary, err
	}
	mutationID := "ccfolia-import-" + job.ID
	if existing, found, err := existingTheaterPackageImport(room.ID, mutationID); err != nil {
		return summary, err
	} else if found {
		return existing, nil
	}

	extractDir, err := os.MkdirTemp(theaterPackageStorageDir(), "ccfolia-"+job.ID+"-")
	if err != nil {
		return summary, err
	}
	defer os.RemoveAll(extractDir)
	warnings, err := extractCCFOLIAZIP(job.InputFilePath, extractDir)
	if err != nil {
		return summary, err
	}
	backup, err := loadCCFOLIABackup(extractDir)
	if err != nil {
		return summary, err
	}
	packageFile, err := theaterPackageFile(job.InputFilePath, "application/zip", job.OriginalName)
	if err != nil {
		return summary, err
	}
	_ = model.GetDB().Model(&model.TheaterPackageJobModel{}).Where("id = ?", job.ID).Update("package_hash", packageFile.SHA256).Error

	resources, targets, animated, resourceWarnings, err := loadCCFOLIAResources(extractDir, backup)
	if err != nil {
		return summary, err
	}
	if err := validateCCFOLIAResourceQuota(room.ID, resources); err != nil {
		return summary, err
	}
	processor := NewVisualMediaProcessor(theaterMedia.config, theaterMedia.toolchain, theaterMedia.runner)
	if err := prepareCCFOLIAAnimatedResources(ctx, extractDir, resources, targets, processor); err != nil {
		return summary, err
	}
	warnings = append(warnings, resourceWarnings...)
	conversion, err := convertCCFOLIABackup(backup, job.TargetWorldID, targets)
	if err != nil {
		return summary, err
	}
	summary = conversion.Summary
	summary.Resources = len(resources)
	summary.AnimatedResources = animated
	summary.Warnings = ccfoliaUniqueWarnings(append(summary.Warnings, warnings...))
	if err := validateTheaterSharedSnapshot(conversion.Snapshot); err != nil {
		return summary, err
	}
	if err := validateTheaterPackageImportLimits(room.ID, conversion.Snapshot); err != nil {
		return summary, err
	}

	remap := theaterPackageRemap{resources: map[string]string{}}
	for sourceRef, target := range targets {
		remap.resources[sourceRef] = target.ResourceID
	}
	persistedAttachments := make([]AttachmentLocation, 0, len(resources))
	cleanupAttachments := true
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
			Kind: "pre-import", Reason: "CCFOLIA ZIP 导入", CreatedBy: job.ActorUserID,
		}
		if err := tx.Create(preImport).Error; err != nil {
			return err
		}
		preImportExpiresAt := time.Now().Add(theaterSnapshotRetention)
		if err := createTheaterResourceHolds(tx, preImport, &preImportExpiresAt); err != nil {
			return err
		}
		for _, resource := range resources {
			if err := importTheaterPackageResource(tx, extractDir, &current, job, resource, remap, &persistedAttachments); err != nil {
				return fmt.Errorf("导入 CCFOLIA 资源 %s 失败: %w", resource.ID, err)
			}
		}

		var maxOrder int64
		if err := tx.Model(&model.TheaterSceneModel{}).Where("room_id = ?", current.ID).Select("COALESCE(MAX(sort_order), 0)").Scan(&maxOrder).Error; err != nil {
			return err
		}
		sceneIDs := sortedTheaterSceneIDs(conversion.Snapshot.Scenes)
		for index, sceneID := range sceneIDs {
			scene := conversion.Snapshot.Scenes[sceneID]
			if err := tx.Create(&model.TheaterSceneModel{
				StringPKBaseModel: model.StringPKBaseModel{ID: scene.ID}, RoomID: current.ID,
				Name: scene.Name, SortOrder: maxOrder + int64(index) + 1, Locked: scene.Locked,
				StateJSON: defaultJSON(scene.State, `{}`), SchemaVersion: model.TheaterSchemaVersion,
				CreatedBy: job.ActorUserID, UpdatedBy: job.ActorUserID,
			}).Error; err != nil {
				return err
			}
		}
		for _, sceneID := range sceneIDs {
			scene := conversion.Snapshot.Scenes[sceneID]
			if err := createTheaterPackageObjects(tx, &current, job.ActorUserID, &scene.ID, scene.Objects); err != nil {
				return err
			}
		}
		if err := createTheaterPackageObjects(tx, &current, job.ActorUserID, nil, conversion.Snapshot.PersistentObjects); err != nil {
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
		if sceneCount == int64(len(conversion.Snapshot.Scenes)) && conversion.Snapshot.ActiveSceneID != nil {
			roomUpdates["active_scene_id"] = *conversion.Snapshot.ActiveSceneID
			roomUpdates["state_json"] = defaultJSON(conversion.Snapshot.LiveState, `{}`)
			current.ActiveSceneID = *conversion.Snapshot.ActiveSceneID
			current.StateJSON = defaultJSON(conversion.Snapshot.LiveState, `{}`)
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
		payload, _ := json.Marshal(map[string]any{"sourceFormat": "ccfolia-backup", "sourceVersion": ccfoliaBackupVersion, "packageHash": packageFile.SHA256, "jobId": job.ID, "summary": summary})
		payloadHash := theaterJSONHash(payload)
		result := TheaterMutationResult{MutationID: mutationID, RevisionBefore: nextRevision - 1, Revision: nextRevision, Type: TheaterMutationAdminPackageImport, Payload: payload, Checksum: checksum}
		resultJSON, _ := json.Marshal(result)
		if err := tx.Create(&model.TheaterMutationModel{
			RoomID: current.ID, WorldID: current.WorldID, ChannelID: current.ChannelID, MutationID: mutationID,
			ActorUserID: job.ActorUserID, ExpectedRevision: nextRevision - 1, RevisionBefore: nextRevision - 1,
			RevisionAfter: &nextRevision, Type: TheaterMutationAdminPackageImport, PayloadJSON: string(payload),
			PayloadHash: payloadHash, ResultJSON: string(resultJSON), Status: "applied", RequestSource: "worker", RequestID: job.ID,
		}).Error; err != nil {
			return err
		}
		if err := createTheaterAudit(tx, &current, job.ActorUserID, TheaterRequestMeta{Source: "worker", RequestID: job.ID}, mutationID, TheaterMutationAdminPackageImport, "imported", "", "CCFOLIA ZIP 导入", nextRevision-1, &nextRevision, payload); err != nil {
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
	cleanupAttachments = false
	if createdMutation {
		EnqueueTheaterMutation(mutationID)
	}
	updateTheaterPackageProgress(job.ID, 0.99)
	return summary, nil
}

func extractCCFOLIAZIP(source, target string) ([]string, error) {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return nil, fmt.Errorf("CCFOLIA ZIP 无效: %w", err)
	}
	defer archive.Close()
	if len(archive.File) == 0 || len(archive.File) > theaterPackageMaxFiles {
		return nil, fmt.Errorf("CCFOLIA ZIP 文件数量无效")
	}
	warnings := []string{}
	seen := map[string]string{}
	var expanded int64
	for _, item := range archive.File {
		if strings.Contains(item.Name, "\\") {
			return nil, fmt.Errorf("CCFOLIA ZIP 包含不安全路径: %s", item.Name)
		}
		name := filepath.ToSlash(item.Name)
		clean := filepath.ToSlash(filepath.Clean(name))
		if name == "" || strings.HasPrefix(name, "/") || clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(item.Name) || item.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("CCFOLIA ZIP 包含不安全路径: %s", item.Name)
		}
		if item.FileInfo().IsDir() {
			continue
		}
		if clean != name {
			return nil, fmt.Errorf("CCFOLIA ZIP 包含非规范路径: %s", item.Name)
		}
		if strings.Contains(clean, "/") {
			return nil, fmt.Errorf("CCFOLIA ZIP 只允许根目录文件: %s", item.Name)
		}
		lower := strings.ToLower(clean)
		if previous, exists := seen[lower]; exists {
			return nil, fmt.Errorf("CCFOLIA ZIP 文件名重复或大小写冲突: %s / %s", previous, clean)
		}
		seen[lower] = clean
		if clean == ".token" {
			continue
		}
		uncompressed := int64(item.UncompressedSize64)
		if uncompressed < 0 || uncompressed > ccfoliaMaxFileBytes || (clean == "__data.json" && uncompressed > ccfoliaMaxDataBytes) {
			return nil, fmt.Errorf("CCFOLIA ZIP 文件大小超限: %s", clean)
		}
		expanded += uncompressed
		if expanded > theaterPackageMaxExpandedBytes {
			return nil, fmt.Errorf("CCFOLIA ZIP 解压大小超过限制")
		}
		if item.UncompressedSize64 > 1<<20 && (item.CompressedSize64 == 0 || item.UncompressedSize64/item.CompressedSize64 > ccfoliaMaxCompressionRatio) {
			return nil, fmt.Errorf("CCFOLIA ZIP 压缩比异常: %s", clean)
		}
		path := filepath.Join(target, clean)
		input, err := item.Open()
		if err != nil {
			return nil, err
		}
		output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			input.Close()
			return nil, err
		}
		written, copyErr := io.Copy(output, io.LimitReader(input, ccfoliaMaxFileBytes+1))
		closeInputErr := input.Close()
		closeOutputErr := output.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		if closeInputErr != nil {
			return nil, closeInputErr
		}
		if closeOutputErr != nil {
			return nil, closeOutputErr
		}
		if written != uncompressed {
			return nil, fmt.Errorf("CCFOLIA ZIP 文件解压大小不一致: %s", clean)
		}
	}
	if _, ok := seen["__data.json"]; !ok {
		return nil, fmt.Errorf("CCFOLIA ZIP 缺少 __data.json")
	}
	return warnings, nil
}

func loadCCFOLIABackup(root string) (ccfoliaBackup, error) {
	var backup ccfoliaBackup
	raw, err := os.ReadFile(filepath.Join(root, "__data.json"))
	if err != nil {
		return backup, err
	}
	if len(raw) == 0 || int64(len(raw)) > ccfoliaMaxDataBytes || !utf8.Valid(raw) {
		return backup, fmt.Errorf("CCFOLIA __data.json 大小或编码无效")
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return backup, fmt.Errorf("CCFOLIA __data.json 无效: %w", err)
	}
	for _, field := range []string{"meta", "entities", "resources"} {
		if _, ok := top[field]; !ok {
			return backup, fmt.Errorf("CCFOLIA __data.json 缺少 %s", field)
		}
	}
	var entities map[string]json.RawMessage
	if err := json.Unmarshal(top["entities"], &entities); err != nil {
		return backup, fmt.Errorf("CCFOLIA entities 无效: %w", err)
	}
	for _, field := range []string{"room", "items", "scenes"} {
		if _, ok := entities[field]; !ok {
			return backup, fmt.Errorf("CCFOLIA entities 缺少 %s", field)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&backup); err != nil {
		return backup, fmt.Errorf("CCFOLIA __data.json 无效: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return backup, fmt.Errorf("CCFOLIA __data.json 只能包含一个 JSON 值")
	}
	if backup.Meta.Version != ccfoliaBackupVersion {
		return backup, newTheaterError(TheaterErrorSchemaUnsupported, "CCFOLIA 备份版本不受支持", 409, map[string]any{"version": backup.Meta.Version})
	}
	if backup.Resources == nil {
		return backup, fmt.Errorf("CCFOLIA resources 缺失")
	}
	return backup, nil
}

func loadCCFOLIAResources(root string, backup ccfoliaBackup) ([]TheaterPackageResource, map[string]ccfoliaAssetTarget, int, []string, error) {
	refs := make([]string, 0, len(backup.Resources))
	for ref := range backup.Resources {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	resources := make([]TheaterPackageResource, 0, len(refs))
	targets := make(map[string]ccfoliaAssetTarget, len(refs))
	mediaConfig := normalizeTheaterMediaConfig(theaterMedia.config)
	animated := 0
	for _, ref := range refs {
		match := ccfoliaAssetNamePattern.FindStringSubmatch(ref)
		if match == nil {
			return nil, nil, 0, nil, fmt.Errorf("CCFOLIA 资源文件名无效: %s", ref)
		}
		path := filepath.Join(root, ref)
		fileInfo, err := theaterPackageFile(path, backup.Resources[ref].Type, ref)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil, 0, nil, fmt.Errorf("CCFOLIA 资源文件缺失: %s", ref)
			}
			return nil, nil, 0, nil, err
		}
		if fileInfo.SHA256 != match[1] {
			return nil, nil, 0, nil, fmt.Errorf("CCFOLIA 资源哈希不匹配: %s", ref)
		}
		declared := strings.ToLower(strings.TrimSpace(backup.Resources[ref].Type))
		expected := ccfoliaMIMEForExtension(match[2])
		if declared != expected {
			return nil, nil, 0, nil, fmt.Errorf("CCFOLIA 资源 MIME 与扩展名不一致: %s", ref)
		}
		metadata, detected, err := inspectCCFOLIAImage(path)
		if err != nil {
			return nil, nil, 0, nil, fmt.Errorf("CCFOLIA 资源格式无效 %s: %w", ref, err)
		}
		if !ccfoliaMIMECompatible(declared, detected) {
			return nil, nil, 0, nil, fmt.Errorf("CCFOLIA 资源 MIME 与文件内容不一致: %s", ref)
		}
		mediaMIME := declared
		if detected == "image/apng" {
			mediaMIME = detected
		}
		if fileInfo.Size > mediaConfig.ImageMaxSizeMB<<20 {
			return nil, nil, 0, nil, newTheaterError(TheaterErrorResourceLimitExceeded, "CCFOLIA 图片大小超过限制", 413, map[string]any{"file": ref, "limitBytes": mediaConfig.ImageMaxSizeMB << 20})
		}
		metadata, err = validateTheaterMediaMetadata(metadata, mediaConfig)
		if err != nil {
			return nil, nil, 0, nil, fmt.Errorf("CCFOLIA 资源超出媒体限制 %s: %w", ref, err)
		}
		resourceID := utils.NewID()
		isAnimated := metadata.Kind == "animated_image"
		if isAnimated {
			animated++
		}
		width, height := metadata.Width, metadata.Height
		frameCount := metadata.FrameCount
		var duration *int64
		if metadata.DurationMS > 0 {
			value := metadata.DurationMS
			duration = &value
		}
		resource := TheaterPackageResource{
			ID: ref, Kind: metadata.Kind, ContentHash: fileInfo.SHA256, SizeBytes: fileInfo.Size,
			MimeType: mediaMIME, OriginalFilename: ref, Width: &width, Height: &height,
			DurationMS: duration, FrameCount: &frameCount, Container: ccfoliaContainer(detected, isAnimated),
			Original: TheaterPackageFile{Path: ref, SHA256: fileInfo.SHA256, Size: fileInfo.Size, MimeType: mediaMIME, Filename: ref},
			Variants: []TheaterPackageResourceVariant{},
		}
		resources = append(resources, resource)
		targets[ref] = ccfoliaAssetTarget{ResourceID: resourceID, MimeType: mediaMIME, Animated: isAnimated}
	}
	warnings := []string{}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil, 0, nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "__data.json" || entry.Name() == ".token" {
			continue
		}
		if _, ok := backup.Resources[entry.Name()]; !ok {
			warnings = appendWarning(warnings, "ZIP 包含未声明文件，已忽略: "+entry.Name())
		}
	}
	references := ccfoliaAssetReferences(backup)
	for ref, paths := range references {
		if _, ok := targets[ref]; !ok {
			return nil, nil, 0, nil, fmt.Errorf("CCFOLIA 引用资源未声明或缺失: %s (%s)", ref, strings.Join(paths, ", "))
		}
	}
	for ref := range targets {
		if len(references[ref]) == 0 {
			warnings = appendWarning(warnings, "存在未被已支持实体引用的资源: "+ref)
		}
	}
	return resources, targets, animated, warnings, nil
}

func prepareCCFOLIAAnimatedResources(ctx context.Context, root string, resources []TheaterPackageResource, targets map[string]ccfoliaAssetTarget, processor *VisualMediaProcessor) error {
	if processor == nil {
		return fmt.Errorf("CCFOLIA 动图处理器不存在")
	}
	derivedRoot := filepath.Join(root, ".sealchat-derived")
	for index := range resources {
		resource := &resources[index]
		if resource.Kind != "animated_image" {
			continue
		}
		processed, err := processor.ProcessTheaterResource(ctx, filepath.Join(root, resource.Original.Path), resource.Kind, resource.MimeType)
		if err != nil {
			return fmt.Errorf("转换 CCFOLIA 动图 %s 失败: %w", resource.ID, err)
		}
		func() {
			defer processed.Cleanup()
			for _, output := range processed.Outputs {
				if output.Name != VisualMediaOutputDisplay || output.MimeType != "video/webm" {
					continue
				}
				relative := filepath.ToSlash(filepath.Join(".sealchat-derived", resource.ContentHash+".display.webm"))
				targetPath := filepath.Join(derivedRoot, resource.ContentHash+".display.webm")
				if output.IsSource {
					targetPath = filepath.Join(root, resource.Original.Path)
					relative = resource.Original.Path
				} else if err = copyTheaterPackageFile(output.Path, targetPath); err != nil {
					return
				}
				var file TheaterPackageFile
				file, err = theaterPackageFile(targetPath, output.MimeType, filepath.Base(targetPath))
				if err != nil {
					return
				}
				file.Path = relative
				resource.Variants = append(resource.Variants, TheaterPackageResourceVariant{
					Name: output.Name, MimeType: output.MimeType, SizeBytes: file.Size,
					Width: intPtr(output.Width), Height: intPtr(output.Height), DurationMS: optionalInt64(output.DurationMS),
					ContentHash: file.SHA256, File: file,
				})
				target := targets[resource.ID]
				target.MimeType = output.MimeType
				target.PlaybackVariant = output.Name
				targets[resource.ID] = target
				return
			}
			err = fmt.Errorf("转换 CCFOLIA 动图 %s 未生成 display WebM", resource.ID)
		}()
		if err != nil {
			return err
		}
	}
	return nil
}

func validateCCFOLIAResourceQuota(roomID string, resources []TheaterPackageResource) error {
	config := normalizeTheaterMediaConfig(theaterMedia.config)
	var used int64
	if err := model.GetDB().Model(&model.TheaterResourceModel{}).Where("room_id = ? AND status NOT IN ?", roomID, []string{"failed", "deleting", "purging"}).Select("COALESCE(SUM(size_bytes), 0)").Scan(&used).Error; err != nil {
		return err
	}
	var incoming int64
	for _, resource := range resources {
		incoming += resource.SizeBytes
	}
	limit := config.RoomQuotaMB << 20
	if used+incoming > limit {
		return newTheaterError(TheaterErrorResourceLimitExceeded, "导入后房间资源将超过配额", 413, map[string]any{"limitBytes": limit, "usedBytes": used, "incomingBytes": incoming})
	}
	return nil
}

func inspectCCFOLIAImage(path string) (theaterMediaMetadata, string, error) {
	head, err := readFilePrefix(path, 1<<20)
	if err != nil {
		return theaterMediaMetadata{}, "", err
	}
	detected, kind := detectTheaterMediaType(head)
	if detected == "" || (kind != "static_image" && kind != "animated_image") {
		return theaterMediaMetadata{}, "", fmt.Errorf("不支持媒体格式")
	}
	if detected == "image/webp" {
		webp, err := parseWebPMetadata(path)
		if err != nil {
			return theaterMediaMetadata{}, "", err
		}
		kind = "static_image"
		if webp.Animated && webp.FrameCount > 1 {
			kind = "animated_image"
		}
		return theaterMediaMetadata{Kind: kind, MimeType: detected, Width: webp.Width, Height: webp.Height, FrameCount: webp.FrameCount, DurationMS: webp.DurationMS}, detected, nil
	}
	if kind == "animated_image" {
		metadata, err := probeAnimatedImage(path, detected)
		return metadata, detected, err
	}
	file, err := os.Open(path)
	if err != nil {
		return theaterMediaMetadata{}, "", err
	}
	defer file.Close()
	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return theaterMediaMetadata{}, "", err
	}
	return theaterMediaMetadata{Kind: "static_image", MimeType: detected, Width: config.Width, Height: config.Height, FrameCount: 1}, detected, nil
}

func ccfoliaMIMEForExtension(extension string) string {
	switch strings.ToLower(extension) {
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return ""
	}
}

func ccfoliaMIMECompatible(declared, detected string) bool {
	return declared == detected || (declared == "image/png" && detected == "image/apng")
}

func ccfoliaContainer(mimeType string, animated bool) string {
	if !animated {
		return ""
	}
	switch mimeType {
	case "image/gif":
		return "gif"
	case "image/apng":
		return "apng"
	case "image/webp":
		return "webp"
	default:
		return ""
	}
}

func convertCCFOLIABackup(backup ccfoliaBackup, worldID string, targets map[string]ccfoliaAssetTarget) (ccfoliaConversion, error) {
	warnings := []string{}
	unsupported := ccfoliaUnsupportedEntityNames(backup.Entities.Unsupported)
	if len(backup.Unknown) > 0 {
		warnings = appendWarning(warnings, "备份包含未知顶层字段，已保存在导入元数据")
	}
	if len(unsupported) > 0 {
		warnings = appendWarning(warnings, "暂不支持实体已保存在导入元数据: "+strings.Join(unsupported, ", "))
	}

	sceneEntries := make([]struct {
		SourceID string
		Scene    ccfoliaScene
	}, 0, len(backup.Entities.Scenes))
	for sourceID, scene := range backup.Entities.Scenes {
		sceneEntries = append(sceneEntries, struct {
			SourceID string
			Scene    ccfoliaScene
		}{SourceID: sourceID, Scene: scene})
	}
	sort.Slice(sceneEntries, func(i, j int) bool {
		if sceneEntries[i].Scene.Order == sceneEntries[j].Scene.Order {
			return sceneEntries[i].SourceID < sceneEntries[j].SourceID
		}
		return sceneEntries[i].Scene.Order < sceneEntries[j].Scene.Order
	})
	sceneNameIDs := map[string][]string{}
	sceneTargetIDs := map[string]string{}
	for _, entry := range sceneEntries {
		targetID := utils.NewID()
		sceneTargetIDs[entry.SourceID] = targetID
		name := strings.TrimSpace(entry.Scene.Name)
		sceneNameIDs[name] = append(sceneNameIDs[name], targetID)
	}

	currentSceneID := utils.NewID()
	currentState, currentObjects, stateWarnings, err := ccfoliaRoomState(backup, worldID, currentSceneID, targets, sceneNameIDs)
	if err != nil {
		return ccfoliaConversion{}, err
	}
	warnings = append(warnings, stateWarnings...)
	snapshot := TheaterSharedSnapshot{
		ActiveSceneID: &currentSceneID, LiveState: currentState,
		Scenes: map[string]TheaterSceneSnapshot{}, PersistentObjects: map[string]TheaterObjectSnapshot{},
		Characters: map[string]TheaterObjectSnapshot{}, Resources: map[string]TheaterResourcePublic{},
	}
	snapshot.Scenes[currentSceneID] = TheaterSceneSnapshot{
		ID: currentSceneID, Name: "CCFOLIA 当前房间", Order: 0, Locked: false, State: currentState, Objects: currentObjects,
	}
	for index, entry := range sceneEntries {
		targetID := sceneTargetIDs[entry.SourceID]
		state, objects, sceneWarnings, err := ccfoliaSceneState(entry.SourceID, entry.Scene, worldID, targetID, targets, sceneNameIDs)
		if err != nil {
			return ccfoliaConversion{}, err
		}
		warnings = append(warnings, sceneWarnings...)
		name := ccfoliaName(entry.Scene.Name, "未命名场景")
		snapshot.Scenes[targetID] = TheaterSceneSnapshot{ID: targetID, Name: name, Order: int64(index + 1), Locked: entry.Scene.Locked, State: state, Objects: objects}
	}
	persistent, itemWarnings, err := ccfoliaItems(backup.Entities.Items, worldID, targets, sceneNameIDs)
	if err != nil {
		return ccfoliaConversion{}, err
	}
	warnings = append(warnings, itemWarnings...)
	warnings = ccfoliaUniqueWarnings(warnings)
	snapshot.PersistentObjects = persistent
	summary := summarizeTheaterSnapshot(snapshot)
	summary.SourceFormat = "ccfolia-backup"
	summary.SourceVersion = backup.Meta.Version
	summary.CurrentRoomObjects = len(currentObjects)
	summary.UnsupportedEntities = unsupported
	summary.Warnings = warnings
	for sceneID := range snapshot.Scenes {
		summary.ImportedSceneIDs = append(summary.ImportedSceneIDs, sceneID)
	}
	sort.Strings(summary.ImportedSceneIDs)
	return ccfoliaConversion{Snapshot: snapshot, Summary: summary}, nil
}

func ccfoliaRoomState(backup ccfoliaBackup, worldID, sceneID string, targets map[string]ccfoliaAssetTarget, sceneNameIDs map[string][]string) (json.RawMessage, map[string]TheaterObjectSnapshot, []string, error) {
	room := backup.Entities.Room
	metadata := map[string]any{
		"sourceType": "current", "sourceVersion": backup.Meta.Version,
		"sourceRaw": ccfoliaRawWithout(room.Raw, "markers"),
		"metaRaw":   string(backup.Meta.Raw), "unknownTopLevel": ccfoliaRawMapStrings(backup.Unknown),
		"unsupportedEntities": ccfoliaRawMapStrings(backup.Entities.Unsupported),
		"resourceDescriptors": ccfoliaResourceRawMap(backup.Resources),
	}
	state, warnings, err := ccfoliaStageState(room.BackgroundURL, room.ForegroundURL, room.FieldWidth, room.FieldHeight, room.FieldObjectFit, room.BackgroundColor, room.DisplayGrid, room.GridSize, room.AlignWithGrid, room.EnableCrossfade, room.CrossfadeDuration, metadata, worldID, targets)
	if err != nil {
		return nil, nil, warnings, err
	}
	objects, objectWarnings, err := ccfoliaMarkers(room.Markers, "current", "current", sceneID, worldID, targets, sceneNameIDs)
	return state, objects, append(warnings, objectWarnings...), err
}

func ccfoliaSceneState(sourceID string, scene ccfoliaScene, worldID, sceneID string, targets map[string]ccfoliaAssetTarget, sceneNameIDs map[string][]string) (json.RawMessage, map[string]TheaterObjectSnapshot, []string, error) {
	metadata := map[string]any{"sourceType": "scene", "sourceSceneId": sourceID, "sourceOrder": scene.Order, "sourceRaw": ccfoliaRawWithout(scene.Raw, "markers")}
	state, warnings, err := ccfoliaStageState(scene.BackgroundURL, scene.ForegroundURL, scene.FieldWidth, scene.FieldHeight, scene.FieldObjectFit, "", scene.DisplayGrid, scene.GridSize, false, false, 0, metadata, worldID, targets)
	if err != nil {
		return nil, nil, warnings, err
	}
	objects, objectWarnings, err := ccfoliaMarkers(scene.Markers, "scene", sourceID, sceneID, worldID, targets, sceneNameIDs)
	return state, objects, append(warnings, objectWarnings...), err
}

func ccfoliaStageState(backgroundRef, foregroundRef *string, width, height float64, fit, backgroundColor string, displayGrid bool, gridSize float64, align, crossfade bool, crossfadeDuration float64, metadata map[string]any, worldID string, targets map[string]ccfoliaAssetTarget) (json.RawMessage, []string, error) {
	warnings := []string{}
	background, err := ccfoliaImageRef(backgroundRef, "背景", worldID, targets)
	if err != nil {
		return nil, warnings, err
	}
	foreground, err := ccfoliaImageRef(foregroundRef, "前景", worldID, targets)
	if err != nil {
		return nil, warnings, err
	}
	targetWidth := width
	if targetWidth <= 0 {
		targetWidth = 1
		warnings = appendWarning(warnings, "存在零或负画布宽度；源值已保留，SealChat 渲染宽度使用 1")
	}
	targetHeight := height
	if targetHeight <= 0 {
		targetHeight = 1
		warnings = appendWarning(warnings, "存在零或负画布高度；源值已保留，SealChat 渲染高度使用 1")
	}
	targetGridSize := gridSize
	if targetGridSize <= 0 {
		targetGridSize = 1
		warnings = appendWarning(warnings, "存在无效网格尺寸；源值已保留，SealChat 使用 1")
	}
	targetFit := strings.ToLower(strings.TrimSpace(fit))
	if targetFit != "fill" && targetFit != "cover" && targetFit != "contain" {
		targetFit = "cover"
		warnings = appendWarning(warnings, "存在未知 fieldObjectFit；源值已保留，SealChat 使用 cover")
	}
	color := strings.TrimSpace(backgroundColor)
	if color == "" {
		color = "#111827"
	}
	durationMS := int64(0)
	transitionType := "none"
	if crossfade {
		transitionType = "crossfade"
		durationMS = int64(math.Round(crossfadeDuration * 1000))
		if durationMS < 0 {
			durationMS = 0
		}
		if durationMS > 60000 {
			durationMS = 60000
			warnings = appendWarning(warnings, "交叉淡化时长超过 SealChat 上限，已限制为 60000ms；源值已保留")
		}
	}
	style := func() map[string]any {
		return map[string]any{"brightness": 1, "blurPx": 0, "opacity": 1, "fit": targetFit, "overlay": map[string]any{"enabled": false, "color": "#000000", "opacity": 0.4}}
	}
	state := map[string]any{
		"background": background, "foreground": foreground,
		"surfaceStyles": map[string]any{"background": style(), "foreground": style()},
		"fieldWidth":    targetWidth, "fieldHeight": targetHeight,
		"grid":       map[string]any{"backgroundColor": color, "objectFit": targetFit, "display": displayGrid, "size": targetGridSize, "align": align},
		"transition": map[string]any{"type": transitionType, "durationMs": durationMS}, "ccfolia": metadata,
	}
	raw, err := json.Marshal(state)
	return raw, warnings, err
}

func ccfoliaMarkers(markers map[string]ccfoliaMarker, scopeType, scopeID, sceneID, worldID string, targets map[string]ccfoliaAssetTarget, sceneNameIDs map[string][]string) (map[string]TheaterObjectSnapshot, []string, error) {
	type entry struct {
		SourceID string
		Marker   ccfoliaMarker
	}
	entries := make([]entry, 0, len(markers))
	for sourceID, marker := range markers {
		entries = append(entries, entry{SourceID: sourceID, Marker: marker})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Marker.Z == entries[j].Marker.Z {
			return entries[i].SourceID < entries[j].SourceID
		}
		return entries[i].Marker.Z < entries[j].Marker.Z
	})
	objects := make(map[string]TheaterObjectSnapshot, len(entries))
	warnings := []string{}
	for index, entry := range entries {
		marker := entry.Marker
		if strings.TrimSpace(marker.ImageURL) == "" {
			return nil, warnings, fmt.Errorf("CCFOLIA marker 图片引用缺失: %s/%s", scopeID, entry.SourceID)
		}
		if marker.Width < 0 || marker.Height < 0 {
			return nil, warnings, fmt.Errorf("CCFOLIA marker 尺寸无效: %s/%s", scopeID, entry.SourceID)
		}
		imageRef, err := ccfoliaImageRef(&marker.ImageURL, marker.Text, worldID, targets)
		if err != nil {
			return nil, warnings, fmt.Errorf("CCFOLIA marker %s/%s: %w", scopeID, entry.SourceID, err)
		}
		content, _ := json.Marshal(map[string]any{"image": imageRef, "text": marker.Text})
		actions, actionWarnings := ccfoliaActions(marker.ClickAction, sceneNameIDs)
		warnings = append(warnings, actionWarnings...)
		metadata, _ := json.Marshal(map[string]any{"ccfolia": map[string]any{"scopeType": scopeType, "scopeId": scopeID, "sourceMarkerId": entry.SourceID, "freezed": marker.Freezed, "sourceRaw": string(marker.Raw)}})
		objectID := utils.NewID()
		aspect := true
		objects[objectID] = TheaterObjectSnapshot{
			ID: objectID, SceneID: &sceneID, Kind: "image", Name: ccfoliaName(marker.Text, "CCFOLIA Marker"),
			X: marker.X + marker.Width/2, Y: marker.Y + marker.Height/2, Width: marker.Width, Height: marker.Height,
			Rotation: 0, Scale: 1, ScaleX: 1, ScaleY: 1, Z: marker.Z, OrderKey: strconv.Itoa(index + 1),
			Visible: true, Locked: marker.Locked, AspectRatioLocked: &aspect, Interactive: true, Editable: false,
			Content: content, Actions: actions, Metadata: metadata,
		}
	}
	return objects, warnings, nil
}

func ccfoliaItems(items map[string]ccfoliaItem, worldID string, targets map[string]ccfoliaAssetTarget, sceneNameIDs map[string][]string) (map[string]TheaterObjectSnapshot, []string, error) {
	type entry struct {
		SourceID string
		Item     ccfoliaItem
	}
	entries := make([]entry, 0, len(items))
	for sourceID, item := range items {
		entries = append(entries, entry{SourceID: sourceID, Item: item})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Item.Order == entries[j].Item.Order {
			return entries[i].SourceID < entries[j].SourceID
		}
		return entries[i].Item.Order < entries[j].Item.Order
	})
	objects := make(map[string]TheaterObjectSnapshot, len(entries))
	warnings := []string{}
	for index, entry := range entries {
		item := entry.Item
		if strings.TrimSpace(item.ImageURL) == "" {
			return nil, warnings, fmt.Errorf("CCFOLIA item 图片引用缺失: %s", entry.SourceID)
		}
		if item.Width < 0 || item.Height < 0 {
			return nil, warnings, fmt.Errorf("CCFOLIA item 尺寸无效: %s", entry.SourceID)
		}
		imageRef, err := ccfoliaImageRef(&item.ImageURL, item.Memo, worldID, targets)
		if err != nil {
			return nil, warnings, fmt.Errorf("CCFOLIA item %s: %w", entry.SourceID, err)
		}
		content, _ := json.Marshal(map[string]any{"image": imageRef, "text": item.Memo})
		actions, actionWarnings := ccfoliaActions(item.ClickAction, sceneNameIDs)
		warnings = append(warnings, actionWarnings...)
		if item.CoverImageURL != nil && strings.TrimSpace(*item.CoverImageURL) != "" {
			if _, err := ccfoliaImageRef(item.CoverImageURL, item.Memo+"封面", worldID, targets); err != nil {
				return nil, warnings, fmt.Errorf("CCFOLIA item %s coverImageUrl: %w", entry.SourceID, err)
			}
			warnings = appendWarning(warnings, "item.coverImageUrl 暂不映射，已保存在对象元数据")
		}
		metadata, _ := json.Marshal(map[string]any{"ccfolia": map[string]any{"sourceItemId": entry.SourceID, "sourceOrder": item.Order, "sourceRaw": string(item.Raw)}})
		objectID := utils.NewID()
		aspect := true
		objects[objectID] = TheaterObjectSnapshot{
			ID: objectID, Kind: "image", Name: ccfoliaName(item.Memo, "CCFOLIA Item"),
			X: item.X + item.Width/2, Y: item.Y + item.Height/2, Width: item.Width, Height: item.Height,
			Rotation: item.Angle, Scale: 1, ScaleX: 1, ScaleY: 1, Z: item.Z, OrderKey: strconv.Itoa(index + 1),
			Visible: item.Visible, Locked: item.Locked, AspectRatioLocked: &aspect, Interactive: true, Editable: false,
			Content: content, Actions: actions, Metadata: metadata,
		}
	}
	return objects, warnings, nil
}

func ccfoliaActions(action *ccfoliaClickAction, sceneNameIDs map[string][]string) (json.RawMessage, []string) {
	if action == nil {
		return json.RawMessage(`[]`), nil
	}
	text := strings.TrimSpace(action.Text)
	if action.Type == "message" && strings.HasPrefix(text, "/scene ") {
		name := strings.TrimSpace(strings.TrimPrefix(text, "/scene "))
		ids := sceneNameIDs[name]
		if len(ids) == 1 {
			raw, _ := json.Marshal([]map[string]any{{"id": utils.NewID(), "type": TheaterMutationSceneApply, "payload": map[string]any{"sceneId": ids[0]}}})
			return raw, nil
		}
		if len(ids) > 1 {
			return json.RawMessage(`[]`), []string{"存在重名场景，/scene 点击动作仅保留为源元数据"}
		}
	}
	return json.RawMessage(`[]`), []string{"存在未映射 clickAction，已作为惰性源元数据保留"}
}

func ccfoliaImageRef(sourceRef *string, alt, worldID string, targets map[string]ccfoliaAssetTarget) (any, error) {
	if sourceRef == nil || strings.TrimSpace(*sourceRef) == "" {
		return nil, nil
	}
	ref := strings.TrimSpace(*sourceRef)
	target, ok := targets[ref]
	if !ok {
		return nil, fmt.Errorf("引用资源不存在: %s", ref)
	}
	contentPath := fmt.Sprintf("/api/v1/worlds/%s/theater/resources/%s/content", url.PathEscape(worldID), url.PathEscape(target.ResourceID))
	if target.PlaybackVariant != "" {
		contentPath = fmt.Sprintf("/api/v1/worlds/%s/theater/resources/%s/variants/%s/content", url.PathEscape(worldID), url.PathEscape(target.ResourceID), url.PathEscape(target.PlaybackVariant))
	}
	result := map[string]any{
		"resourceId": target.ResourceID,
		"url":        contentPath,
		"mimeType":   target.MimeType,
	}
	if strings.TrimSpace(alt) != "" {
		result["alt"] = alt
	}
	if target.Animated {
		result["animated"] = true
	}
	return result, nil
}

func ccfoliaUnsupportedEntityNames(values map[string]json.RawMessage) []string {
	result := []string{}
	for name, raw := range values {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("{}")) || bytes.Equal(trimmed, []byte("[]")) {
			continue
		}
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func ccfoliaRawMapStrings(values map[string]json.RawMessage) map[string]string {
	result := make(map[string]string, len(values))
	for key, raw := range values {
		result[key] = string(raw)
	}
	return result
}

func ccfoliaResourceRawMap(values map[string]ccfoliaResourceDescriptor) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = string(value.Raw)
	}
	return result
}

func ccfoliaAssetReferences(backup ccfoliaBackup) map[string][]string {
	result := map[string][]string{}
	add := func(ref *string, path string) {
		if ref == nil || strings.TrimSpace(*ref) == "" {
			return
		}
		value := strings.TrimSpace(*ref)
		result[value] = append(result[value], path)
	}
	add(backup.Entities.Room.BackgroundURL, "entities.room.backgroundUrl")
	add(backup.Entities.Room.ForegroundURL, "entities.room.foregroundUrl")
	for markerID, marker := range backup.Entities.Room.Markers {
		add(&marker.ImageURL, "entities.room.markers."+markerID+".imageUrl")
	}
	for sceneID, scene := range backup.Entities.Scenes {
		add(scene.BackgroundURL, "entities.scenes."+sceneID+".backgroundUrl")
		add(scene.ForegroundURL, "entities.scenes."+sceneID+".foregroundUrl")
		for markerID, marker := range scene.Markers {
			add(&marker.ImageURL, "entities.scenes."+sceneID+".markers."+markerID+".imageUrl")
		}
	}
	for itemID, item := range backup.Entities.Items {
		add(&item.ImageURL, "entities.items."+itemID+".imageUrl")
		add(item.CoverImageURL, "entities.items."+itemID+".coverImageUrl")
	}
	return result
}

func ccfoliaUniqueWarnings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = appendWarning(result, value)
	}
	return result
}

func ccfoliaRawWithout(raw json.RawMessage, fields ...string) string {
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil {
		return string(raw)
	}
	for _, field := range fields {
		delete(value, field)
	}
	result, err := json.Marshal(value)
	if err != nil {
		return string(raw)
	}
	return string(result)
}

func ccfoliaName(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if len([]rune(value)) <= 512 {
		return value
	}
	return string([]rune(value)[:512])
}

func sortedTheaterSceneIDs(scenes map[string]TheaterSceneSnapshot) []string {
	ids := make([]string, 0, len(scenes))
	for id := range scenes {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		left, right := scenes[ids[i]], scenes[ids[j]]
		if left.Order == right.Order {
			return left.ID < right.ID
		}
		return left.Order < right.Order
	})
	return ids
}
