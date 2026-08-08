package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

var ccfoliaHashedAssetNamePattern = regexp.MustCompile(`^([0-9a-f]{64})\.(png|gif|jpe?g|webp)$`)
var ccfoliaSourceHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var ccfoliaRandomTableEntryPattern = regexp.MustCompile(`^\s*([0-9]+)(?:\s*-\s*([0-9]+))?\s*:(.*)$`)

type ccfoliaBackup struct {
	Meta      ccfoliaMeta                          `json:"meta"`
	Entities  ccfoliaEntities                      `json:"entities"`
	Resources map[string]ccfoliaResourceDescriptor `json:"resources"`
	Unknown   map[string]json.RawMessage           `json:"-"`
	SourceRaw json.RawMessage                      `json:"-"`
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
	Room        ccfoliaRoom                 `json:"room"`
	Items       map[string]ccfoliaItem      `json:"items"`
	Characters  map[string]ccfoliaCharacter `json:"characters"`
	Scenes      map[string]ccfoliaScene     `json:"scenes"`
	Notes       map[string]ccfoliaNote      `json:"notes"`
	Savedatas   map[string]ccfoliaSavedata  `json:"savedatas"`
	Snapshots   map[string]ccfoliaSnapshot  `json:"snapshots"`
	Unsupported map[string]json.RawMessage  `json:"-"`
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
	delete(unsupported, "characters")
	delete(unsupported, "scenes")
	delete(unsupported, "notes")
	delete(unsupported, "savedatas")
	delete(unsupported, "snapshots")
	*item = ccfoliaEntities(value)
	item.Unsupported = unsupported
	if item.Items == nil {
		item.Items = map[string]ccfoliaItem{}
	}
	if item.Characters == nil {
		item.Characters = map[string]ccfoliaCharacter{}
	}
	if item.Scenes == nil {
		item.Scenes = map[string]ccfoliaScene{}
	}
	if item.Notes == nil {
		item.Notes = map[string]ccfoliaNote{}
	}
	if item.Savedatas == nil {
		item.Savedatas = map[string]ccfoliaSavedata{}
	}
	if item.Snapshots == nil {
		item.Snapshots = map[string]ccfoliaSnapshot{}
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
	Text           string                   `json:"text"`
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

type ccfoliaCharacter struct {
	Name       string          `json:"name"`
	PlayerName string          `json:"playerName"`
	Memo       string          `json:"memo"`
	IconURL    *string         `json:"iconUrl"`
	X          float64         `json:"x"`
	Y          float64         `json:"y"`
	Z          float64         `json:"z"`
	Angle      float64         `json:"angle"`
	Width      float64         `json:"width"`
	Height     float64         `json:"height"`
	Active     bool            `json:"active"`
	Secret     bool            `json:"secret"`
	Invisible  bool            `json:"invisible"`
	Color      string          `json:"color"`
	Faces      []ccfoliaFace   `json:"faces"`
	Raw        json.RawMessage `json:"-"`
}

func (item *ccfoliaCharacter) UnmarshalJSON(data []byte) error {
	type plain ccfoliaCharacter
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*item = ccfoliaCharacter(value)
	item.Raw = append(item.Raw[:0], data...)
	return nil
}

type ccfoliaFace struct {
	Label   string  `json:"label"`
	IconURL *string `json:"iconUrl"`
}

type ccfoliaNote struct {
	Name    string          `json:"name"`
	Text    string          `json:"text"`
	IconURL *string         `json:"iconUrl"`
	Order   float64         `json:"order"`
	Raw     json.RawMessage `json:"-"`
}

func (item *ccfoliaNote) UnmarshalJSON(data []byte) error {
	type plain ccfoliaNote
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*item = ccfoliaNote(value)
	item.Raw = append(item.Raw[:0], data...)
	return nil
}

type ccfoliaSavedata struct {
	Name            string          `json:"name"`
	Thumbnail       *string         `json:"thumbnail"`
	SnapshotVersion string          `json:"snapshotVersion"`
	SnapshotID      string          `json:"snapshotId"`
	Order           float64         `json:"order"`
	Raw             json.RawMessage `json:"-"`
}

func (item *ccfoliaSavedata) UnmarshalJSON(data []byte) error {
	type plain ccfoliaSavedata
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*item = ccfoliaSavedata(value)
	item.Raw = append(item.Raw[:0], data...)
	return nil
}

type ccfoliaSnapshot struct {
	Room       ccfoliaRoom                 `json:"room"`
	Items      map[string]ccfoliaItem      `json:"items"`
	Characters map[string]ccfoliaCharacter `json:"characters"`
	Scenes     map[string]ccfoliaScene     `json:"scenes"`
	Notes      map[string]ccfoliaNote      `json:"notes"`
	Raw        json.RawMessage             `json:"-"`
}

func (item *ccfoliaSnapshot) UnmarshalJSON(data []byte) error {
	type plain ccfoliaSnapshot
	var value plain
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*item = ccfoliaSnapshot(value)
	item.Raw = append(item.Raw[:0], data...)
	if item.Items == nil {
		item.Items = map[string]ccfoliaItem{}
	}
	if item.Characters == nil {
		item.Characters = map[string]ccfoliaCharacter{}
	}
	if item.Scenes == nil {
		item.Scenes = map[string]ccfoliaScene{}
	}
	if item.Notes == nil {
		item.Notes = map[string]ccfoliaNote{}
	}
	return nil
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
	LoopCount       *int
	Width           int
	Height          int
}

type ccfoliaConversion struct {
	Snapshot TheaterSharedSnapshot
	Summary  TheaterPackageSummary
}

type ccfoliaSourceArchive struct {
	SHA256    string
	SizeBytes int64
	Truncated bool
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
	sourceArchive := newCCFOLIASourceArchive(backup.SourceRaw)
	archiveCreated, err := persistCCFOLIASourceArchive(backup.SourceRaw, sourceArchive)
	if err != nil {
		return summary, err
	}
	archiveCommitted := false
	defer func() {
		if archiveCreated && !archiveCommitted {
			removeUnreferencedCCFOLIASourceArchive(sourceArchive.SHA256)
		}
	}()

	remap := theaterPackageRemap{
		resources: map[string]string{},
		worldID:   room.WorldID, resourceChannelID: room.ChannelID,
	}
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
		archiveRow := model.TheaterSourceArchiveModel{
			WorldID: current.WorldID, RoomID: current.ID,
			SourceFormat: "ccfolia-" + ccfoliaBackupVersion, SHA256: sourceArchive.SHA256,
			SizeBytes: sourceArchive.SizeBytes, Truncated: sourceArchive.Truncated, Status: model.TheaterSourceArchiveStatusActive,
		}
		if err := tx.Where("room_id = ? AND sha256 = ?", current.ID, sourceArchive.SHA256).FirstOrCreate(&archiveRow).Error; err != nil {
			return err
		}
		if err := tx.Model(&archiveRow).Updates(map[string]any{
			"size_bytes": sourceArchive.SizeBytes, "truncated": sourceArchive.Truncated,
			"status": model.TheaterSourceArchiveStatusActive, "cleanup_after": nil,
		}).Error; err != nil {
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
				Name: scene.Name, SwitchText: scene.SwitchText, SortOrder: maxOrder + int64(index) + 1, Locked: scene.Locked,
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
	archiveCommitted = true
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
	backup.SourceRaw = append(backup.SourceRaw[:0], raw...)
	return backup, nil
}

func loadCCFOLIAResources(root string, backup ccfoliaBackup) ([]TheaterPackageResource, map[string]ccfoliaAssetTarget, int, []string, error) {
	recoveryWarnings, err := recoverCCFOLIAAssetReferences(root, &backup)
	if err != nil {
		return nil, nil, 0, nil, err
	}
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
		extension, ok := ccfoliaAssetExtension(ref)
		if !ok {
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
		if match := ccfoliaHashedAssetNamePattern.FindStringSubmatch(ref); match != nil && fileInfo.SHA256 != match[1] {
			return nil, nil, 0, nil, fmt.Errorf("CCFOLIA 资源哈希不匹配: %s", ref)
		}
		declared := strings.ToLower(strings.TrimSpace(backup.Resources[ref].Type))
		expected := ccfoliaMIMEForExtension(extension)
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
			metadata.LoopCount = normalizedTheaterLoopCount(metadata)
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
			DurationMS: duration, FrameCount: &frameCount, LoopCount: metadata.LoopCount, Container: ccfoliaContainer(detected, isAnimated),
			Original: TheaterPackageFile{Path: ref, SHA256: fileInfo.SHA256, Size: fileInfo.Size, MimeType: mediaMIME, Filename: ref},
			Variants: []TheaterPackageResourceVariant{},
		}
		resources = append(resources, resource)
		targets[ref] = ccfoliaAssetTarget{ResourceID: resourceID, MimeType: mediaMIME, Animated: isAnimated, LoopCount: metadata.LoopCount, Width: metadata.Width, Height: metadata.Height}
	}
	warnings := recoveryWarnings
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
				if output.Name != VisualMediaOutputDisplay {
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
			err = fmt.Errorf("转换 CCFOLIA 动图 %s 未生成 display 资源", resource.ID)
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
		return theaterMediaMetadata{Kind: kind, MimeType: detected, Width: webp.Width, Height: webp.Height, FrameCount: webp.FrameCount, DurationMS: webp.DurationMS, LoopCount: webp.LoopCount}, detected, nil
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

func recoverCCFOLIAAssetReferences(root string, backup *ccfoliaBackup) ([]string, error) {
	if backup == nil {
		return nil, fmt.Errorf("CCFOLIA 备份不存在")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	assetExtensions := map[string]string{}
	itemAssets := map[string][]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		extension, ok := ccfoliaAssetExtension(name)
		if !ok {
			continue
		}
		assetExtensions[name] = extension
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		itemAssets[stem] = append(itemAssets[stem], name)
	}
	for stem := range itemAssets {
		sort.Strings(itemAssets[stem])
	}
	warnings := []string{}
	ensureResource := func(ref, warning string) bool {
		extension, exists := assetExtensions[ref]
		if !exists {
			return false
		}
		if _, declared := backup.Resources[ref]; !declared {
			backup.Resources[ref] = ccfoliaResourceDescriptor{Type: ccfoliaMIMEForExtension(extension)}
			if warning != "" {
				warnings = appendWarning(warnings, warning+ref)
			}
		}
		return true
	}

	recoveredItemRefs := map[string]struct{}{}
	recoverItems := func(scope string, items map[string]ccfoliaItem) error {
		itemIDs := make([]string, 0, len(items))
		for itemID := range items {
			itemIDs = append(itemIDs, itemID)
		}
		sort.Strings(itemIDs)
		for _, itemID := range itemIDs {
			item := items[itemID]
			if strings.TrimSpace(item.ImageURL) != "" {
				continue
			}
			candidates := itemAssets[itemID]
			if len(candidates) > 1 {
				return fmt.Errorf("CCFOLIA item 图片候选不唯一: %s/%s (%s)", scope, itemID, strings.Join(candidates, ", "))
			}
			if len(candidates) == 0 {
				continue
			}
			ref := candidates[0]
			ensureResource(ref, "")
			item.ImageURL = ref
			items[itemID] = item
			if _, reported := recoveredItemRefs[ref]; !reported {
				recoveredItemRefs[ref] = struct{}{}
				warnings = appendWarning(warnings, "CCFOLIA item 图片引用缺失，已按 item ID 文件恢复: "+ref)
			}
		}
		return nil
	}
	if err := recoverItems("current", backup.Entities.Items); err != nil {
		return nil, err
	}
	snapshotIDs := make([]string, 0, len(backup.Entities.Snapshots))
	for snapshotID := range backup.Entities.Snapshots {
		snapshotIDs = append(snapshotIDs, snapshotID)
	}
	sort.Strings(snapshotIDs)
	for _, snapshotID := range snapshotIDs {
		snapshot := backup.Entities.Snapshots[snapshotID]
		if err := recoverItems("snapshot/"+snapshotID, snapshot.Items); err != nil {
			return nil, err
		}
		backup.Entities.Snapshots[snapshotID] = snapshot
	}

	for ref := range ccfoliaAssetReferences(*backup) {
		if _, declared := backup.Resources[ref]; declared {
			continue
		}
		ensureResource(ref, "JSON 引用图片未在 resources 声明，已按文件恢复: ")
	}
	return warnings, nil
}

func ccfoliaAssetExtension(ref string) (string, bool) {
	if ref == "" || ref != strings.TrimSpace(ref) || strings.ContainsAny(ref, `/\\`) || filepath.Base(ref) != ref {
		return "", false
	}
	extension := strings.TrimPrefix(strings.ToLower(filepath.Ext(ref)), ".")
	if strings.TrimSuffix(ref, filepath.Ext(ref)) == "" || ccfoliaMIMEForExtension(extension) == "" {
		return "", false
	}
	return extension, true
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
	savedataEntries := make([]struct {
		SourceID string
		Savedata ccfoliaSavedata
	}, 0, len(backup.Entities.Savedatas))
	for sourceID, savedata := range backup.Entities.Savedatas {
		savedataEntries = append(savedataEntries, struct {
			SourceID string
			Savedata ccfoliaSavedata
		}{SourceID: sourceID, Savedata: savedata})
	}
	sort.Slice(savedataEntries, func(i, j int) bool {
		if savedataEntries[i].Savedata.Order == savedataEntries[j].Savedata.Order {
			return savedataEntries[i].SourceID < savedataEntries[j].SourceID
		}
		return savedataEntries[i].Savedata.Order < savedataEntries[j].Savedata.Order
	})
	sceneNameIDs := map[string][]string{}
	sceneTargetIDs := map[string]string{}
	for _, entry := range sceneEntries {
		targetID := utils.NewID()
		sceneTargetIDs[entry.SourceID] = targetID
		name := entry.Scene.Name
		sceneNameIDs[name] = append(sceneNameIDs[name], targetID)
	}
	savedataTargetIDs := map[string]string{}
	for _, entry := range savedataEntries {
		if _, exists := backup.Entities.Snapshots[entry.Savedata.SnapshotID]; !exists {
			continue
		}
		targetID := utils.NewID()
		savedataTargetIDs[entry.SourceID] = targetID
		sceneNameIDs[entry.Savedata.Name] = append(sceneNameIDs[entry.Savedata.Name], targetID)
	}

	currentSceneID := utils.NewID()
	sourceArchive := newCCFOLIASourceArchive(backup.SourceRaw)
	currentState, currentObjects, stateWarnings, err := ccfoliaRoomState(backup, sourceArchive, worldID, currentSceneID, targets, sceneNameIDs)
	if err != nil {
		return ccfoliaConversion{}, err
	}
	warnings = append(warnings, stateWarnings...)
	characters, characterWarnings, err := ccfoliaCharacters(backup.Entities.Characters, currentSceneID, worldID, targets)
	if err != nil {
		return ccfoliaConversion{}, err
	}
	for objectID, character := range characters {
		currentObjects[objectID] = character
	}
	warnings = append(warnings, characterWarnings...)
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
		state, objects, sceneWarnings, err := ccfoliaSceneState(entry.SourceID, entry.Scene, sourceArchive, worldID, targetID, targets, sceneNameIDs)
		if err != nil {
			return ccfoliaConversion{}, err
		}
		warnings = append(warnings, sceneWarnings...)
		name := ccfoliaName(entry.Scene.Name, "未命名场景")
		switchText := entry.Scene.Text
		if runes := []rune(switchText); len(runes) > theaterMaxSwitchText {
			switchText = string(runes[:theaterMaxSwitchText])
			warnings = appendWarning(warnings, "场景切换文本超过 SealChat 上限，已截断为 10000 字符: "+name)
		}
		snapshot.Scenes[targetID] = TheaterSceneSnapshot{ID: targetID, Name: name, SwitchText: switchText, Order: int64(index + 1), Locked: entry.Scene.Locked, State: state, Objects: objects}
	}
	for index, entry := range savedataEntries {
		targetID, exists := savedataTargetIDs[entry.SourceID]
		if !exists {
			warnings = appendWarning(warnings, "savedata 引用的 snapshot 不存在，已跳过: "+entry.SourceID+"/"+entry.Savedata.SnapshotID)
			continue
		}
		sourceSnapshot := backup.Entities.Snapshots[entry.Savedata.SnapshotID]
		state, objects, savedataWarnings, err := ccfoliaSavedataScene(entry.SourceID, entry.Savedata, sourceSnapshot, sourceArchive, worldID, targetID, targets, sceneNameIDs)
		if err != nil {
			return ccfoliaConversion{}, err
		}
		warnings = append(warnings, savedataWarnings...)
		name := ccfoliaName(entry.Savedata.Name, "CCFOLIA 存档")
		snapshot.Scenes[targetID] = TheaterSceneSnapshot{
			ID: targetID, Name: name, Order: int64(len(sceneEntries) + index + 1), Locked: false, State: state, Objects: objects,
		}
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

func ccfoliaRoomState(backup ccfoliaBackup, sourceArchive ccfoliaSourceArchive, worldID, sceneID string, targets map[string]ccfoliaAssetTarget, sceneNameIDs map[string][]string) (json.RawMessage, map[string]TheaterObjectSnapshot, []string, error) {
	room := backup.Entities.Room
	metadata := map[string]any{
		"sourceType": "current", "sourceVersion": backup.Meta.Version,
	}
	addCCFOLIASourceArchiveMetadata(metadata, sourceArchive)
	notes, err := ccfoliaNotesMetadata(backup.Entities.Notes, worldID, targets)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("CCFOLIA current notes: %w", err)
	}
	if len(notes) > 0 {
		metadata["notes"] = notes
	}
	state, warnings, err := ccfoliaStageState(room.BackgroundURL, room.ForegroundURL, room.FieldWidth, room.FieldHeight, room.FieldObjectFit, room.BackgroundColor, room.DisplayGrid, room.GridSize, room.AlignWithGrid, room.EnableCrossfade, room.CrossfadeDuration, ccfoliaRoomCanvasBounds(backup), metadata, worldID, targets)
	if err != nil {
		return nil, nil, warnings, err
	}
	objects, objectWarnings, err := ccfoliaMarkers(room.Markers, "current", "current", sceneID, worldID, targets, sceneNameIDs, false)
	return state, objects, append(warnings, objectWarnings...), err
}

func ccfoliaSceneState(sourceID string, scene ccfoliaScene, sourceArchive ccfoliaSourceArchive, worldID, sceneID string, targets map[string]ccfoliaAssetTarget, sceneNameIDs map[string][]string) (json.RawMessage, map[string]TheaterObjectSnapshot, []string, error) {
	metadata := map[string]any{"sourceType": "scene", "sourceSceneId": sourceID, "sourceOrder": scene.Order}
	addCCFOLIASourceArchiveMetadata(metadata, sourceArchive)
	state, warnings, err := ccfoliaStageState(scene.BackgroundURL, scene.ForegroundURL, scene.FieldWidth, scene.FieldHeight, scene.FieldObjectFit, "", scene.DisplayGrid, scene.GridSize, false, false, 0, ccfoliaMarkerCanvasBounds(scene.Markers), metadata, worldID, targets)
	if err != nil {
		return nil, nil, warnings, err
	}
	objects, objectWarnings, err := ccfoliaMarkers(scene.Markers, "scene", sourceID, sceneID, worldID, targets, sceneNameIDs, true)
	return state, objects, append(warnings, objectWarnings...), err
}

func ccfoliaSavedataScene(sourceID string, savedata ccfoliaSavedata, snapshot ccfoliaSnapshot, sourceArchive ccfoliaSourceArchive, worldID, sceneID string, targets map[string]ccfoliaAssetTarget, sceneNameIDs map[string][]string) (json.RawMessage, map[string]TheaterObjectSnapshot, []string, error) {
	warnings := []string{}
	metadata := map[string]any{
		"sourceType":            "savedata",
		"sourceSavedataId":      sourceID,
		"sourceSnapshotId":      savedata.SnapshotID,
		"sourceSnapshotVersion": savedata.SnapshotVersion,
		"sourceOrder":           savedata.Order,
		"sourceSceneCount":      len(snapshot.Scenes),
	}
	addCCFOLIASourceArchiveMetadata(metadata, sourceArchive)
	if savedata.SnapshotVersion != "" && savedata.SnapshotVersion != "2" {
		warnings = appendWarning(warnings, "savedata snapshotVersion 未知，已按兼容模式导入: "+savedata.SnapshotVersion)
	}
	thumbnail, err := ccfoliaImageRef(savedata.Thumbnail, savedata.Name, worldID, targets)
	if err != nil {
		return nil, nil, warnings, fmt.Errorf("CCFOLIA savedata %s thumbnail: %w", sourceID, err)
	}
	if thumbnail != nil {
		metadata["thumbnail"] = thumbnail
	}
	notes, err := ccfoliaNotesMetadata(snapshot.Notes, worldID, targets)
	if err != nil {
		return nil, nil, warnings, fmt.Errorf("CCFOLIA savedata %s notes: %w", sourceID, err)
	}
	if len(notes) > 0 {
		metadata["notes"] = notes
	}
	room := snapshot.Room
	state, stateWarnings, err := ccfoliaStageState(room.BackgroundURL, room.ForegroundURL, room.FieldWidth, room.FieldHeight, room.FieldObjectFit, room.BackgroundColor, room.DisplayGrid, room.GridSize, room.AlignWithGrid, room.EnableCrossfade, room.CrossfadeDuration, ccfoliaSnapshotCanvasBounds(snapshot), metadata, worldID, targets)
	warnings = append(warnings, stateWarnings...)
	if err != nil {
		return nil, nil, warnings, err
	}
	objects, markerWarnings, err := ccfoliaMarkers(room.Markers, "savedata", sourceID, sceneID, worldID, targets, sceneNameIDs, true)
	warnings = append(warnings, markerWarnings...)
	if err != nil {
		return nil, nil, warnings, err
	}
	items, itemWarnings, err := ccfoliaItems(snapshot.Items, worldID, targets, sceneNameIDs)
	warnings = append(warnings, itemWarnings...)
	if err != nil {
		return nil, nil, warnings, fmt.Errorf("CCFOLIA savedata %s items: %w", sourceID, err)
	}
	for objectID, object := range items {
		object.SceneID = &sceneID
		objects[objectID] = object
	}
	characters, characterWarnings, err := ccfoliaCharacters(snapshot.Characters, sceneID, worldID, targets)
	warnings = append(warnings, characterWarnings...)
	if err != nil {
		return nil, nil, warnings, fmt.Errorf("CCFOLIA savedata %s characters: %w", sourceID, err)
	}
	for objectID, object := range characters {
		objects[objectID] = object
	}
	return state, objects, warnings, nil
}

func ccfoliaNotesMetadata(notes map[string]ccfoliaNote, worldID string, targets map[string]ccfoliaAssetTarget) ([]map[string]any, error) {
	type entry struct {
		SourceID string
		Note     ccfoliaNote
	}
	entries := make([]entry, 0, len(notes))
	for sourceID, note := range notes {
		entries = append(entries, entry{SourceID: sourceID, Note: note})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Note.Order == entries[j].Note.Order {
			return entries[i].SourceID < entries[j].SourceID
		}
		return entries[i].Note.Order < entries[j].Note.Order
	})
	result := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		icon, err := ccfoliaImageRef(entry.Note.IconURL, entry.Note.Name, worldID, targets)
		if err != nil {
			return nil, fmt.Errorf("note %s iconUrl: %w", entry.SourceID, err)
		}
		value := map[string]any{
			"sourceNoteId": entry.SourceID,
			"name":         entry.Note.Name,
			"text":         entry.Note.Text,
			"order":        entry.Note.Order,
			"sourceRaw":    string(entry.Note.Raw),
		}
		if icon != nil {
			value["icon"] = icon
		}
		result = append(result, value)
	}
	return result, nil
}

func ccfoliaStageState(backgroundRef, foregroundRef *string, width, height float64, fit, backgroundColor string, displayGrid bool, gridSize float64, align, crossfade bool, crossfadeDuration float64, bounds ccfoliaCanvasBounds, metadata map[string]any, worldID string, targets map[string]ccfoliaAssetTarget) (json.RawMessage, []string, error) {
	warnings := []string{}
	background, err := ccfoliaImageRef(backgroundRef, "背景", worldID, targets)
	if err != nil {
		return nil, warnings, err
	}
	foreground, err := ccfoliaImageRef(foregroundRef, "前景", worldID, targets)
	if err != nil {
		return nil, warnings, err
	}
	targetWidth, targetHeight := width, height
	if targetWidth <= 0 || targetHeight <= 0 {
		autoWidth, autoHeight := ccfoliaAutoCanvasSize(backgroundRef, bounds, targets)
		if targetWidth <= 0 {
			targetWidth = autoWidth
		}
		if targetHeight <= 0 {
			targetHeight = autoHeight
		}
		metadata["autoCanvasSize"] = map[string]any{"width": targetWidth, "height": targetHeight}
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
		return map[string]any{"brightness": 1, "blurPx": 0, "opacity": 1, "zoom": 1, "fit": targetFit, "overlay": map[string]any{"enabled": false, "color": "#000000", "opacity": 0.4}}
	}
	backgroundStyle := style()
	backgroundStyle["blurPx"] = 10
	backgroundStyle["opacity"] = 0.9
	state := map[string]any{
		"background": background, "foreground": foreground,
		"surfaceStyles": map[string]any{"background": backgroundStyle, "foreground": style()},
		"fieldWidth":    targetWidth, "fieldHeight": targetHeight,
		"grid":       map[string]any{"backgroundColor": color, "objectFit": targetFit, "display": displayGrid, "size": targetGridSize, "align": align},
		"transition": map[string]any{"type": transitionType, "durationMs": durationMS}, "ccfolia": metadata,
	}
	raw, err := json.Marshal(state)
	return raw, warnings, err
}

type ccfoliaCanvasBounds struct {
	minX, minY    float64
	maxX, maxY    float64
	hasComponents bool
}

func (bounds *ccfoliaCanvasBounds) add(x, y, width, height float64) {
	if width < 0 || height < 0 {
		return
	}
	maxX, maxY := x+width, y+height
	if !bounds.hasComponents {
		bounds.minX, bounds.minY, bounds.maxX, bounds.maxY = x, y, maxX, maxY
		bounds.hasComponents = true
		return
	}
	bounds.minX = math.Min(bounds.minX, x)
	bounds.minY = math.Min(bounds.minY, y)
	bounds.maxX = math.Max(bounds.maxX, maxX)
	bounds.maxY = math.Max(bounds.maxY, maxY)
}

func ccfoliaMarkerCanvasBounds(markers map[string]ccfoliaMarker) ccfoliaCanvasBounds {
	var bounds ccfoliaCanvasBounds
	for _, marker := range markers {
		bounds.add(marker.X, marker.Y, marker.Width, marker.Height)
	}
	return bounds
}

func ccfoliaRoomCanvasBounds(backup ccfoliaBackup) ccfoliaCanvasBounds {
	bounds := ccfoliaMarkerCanvasBounds(backup.Entities.Room.Markers)
	for _, item := range backup.Entities.Items {
		bounds.add(item.X, item.Y, item.Width, item.Height)
	}
	for _, character := range backup.Entities.Characters {
		bounds.add(character.X, character.Y, character.Width, character.Height)
	}
	return bounds
}

func ccfoliaSnapshotCanvasBounds(snapshot ccfoliaSnapshot) ccfoliaCanvasBounds {
	bounds := ccfoliaMarkerCanvasBounds(snapshot.Room.Markers)
	for _, item := range snapshot.Items {
		bounds.add(item.X, item.Y, item.Width, item.Height)
	}
	for _, character := range snapshot.Characters {
		bounds.add(character.X, character.Y, character.Width, character.Height)
	}
	return bounds
}

func ccfoliaAutoCanvasSize(backgroundRef *string, bounds ccfoliaCanvasBounds, targets map[string]ccfoliaAssetTarget) (float64, float64) {
	width, height := 1024.0, 768.0
	if backgroundRef != nil {
		if target, ok := targets[strings.TrimSpace(*backgroundRef)]; ok {
			width = math.Max(width, float64(target.Width))
			height = math.Max(height, float64(target.Height))
		}
	}
	if bounds.hasComponents {
		width = math.Max(width, bounds.maxX-bounds.minX+192)
		height = math.Max(height, bounds.maxY-bounds.minY+192)
	}
	return math.Ceil(width), math.Ceil(height)
}

func ccfoliaMarkers(markers map[string]ccfoliaMarker, scopeType, scopeID, sceneID, worldID string, targets map[string]ccfoliaAssetTarget, sceneNameIDs map[string][]string, importActions bool) (map[string]TheaterObjectSnapshot, []string, error) {
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
		actions := json.RawMessage(`[]`)
		markerMetadata := map[string]any{"scopeType": scopeType, "scopeId": scopeID, "sourceMarkerId": entry.SourceID, "freezed": marker.Freezed, "sourceRaw": string(marker.Raw)}
		if importActions {
			actionMetadata, actionWarnings := ccfoliaActions(marker.ClickAction, sceneNameIDs)
			actions = actionMetadata.Actions
			if actionMetadata.Metadata != nil {
				markerMetadata["clickAction"] = actionMetadata.Metadata
			}
			warnings = append(warnings, actionWarnings...)
		}
		metadata, _ := json.Marshal(map[string]any{"ccfolia": markerMetadata})
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

func ccfoliaCharacters(characters map[string]ccfoliaCharacter, sceneID, worldID string, targets map[string]ccfoliaAssetTarget) (map[string]TheaterObjectSnapshot, []string, error) {
	type entry struct {
		SourceID  string
		Character ccfoliaCharacter
	}
	entries := make([]entry, 0, len(characters))
	for sourceID, character := range characters {
		entries = append(entries, entry{SourceID: sourceID, Character: character})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Character.Z == entries[j].Character.Z {
			return entries[i].SourceID < entries[j].SourceID
		}
		return entries[i].Character.Z < entries[j].Character.Z
	})
	objects := make(map[string]TheaterObjectSnapshot, len(entries))
	for index, entry := range entries {
		character := entry.Character
		if character.Width < 0 || character.Height < 0 {
			return nil, nil, fmt.Errorf("CCFOLIA character 尺寸无效: %s", entry.SourceID)
		}
		imageRef, err := ccfoliaImageRef(character.IconURL, character.Name, worldID, targets)
		if err != nil {
			return nil, nil, fmt.Errorf("CCFOLIA character %s: %w", entry.SourceID, err)
		}
		faces := make([]map[string]any, 0, len(character.Faces))
		for _, face := range character.Faces {
			faceImage, err := ccfoliaImageRef(face.IconURL, face.Label, worldID, targets)
			if err != nil {
				return nil, nil, fmt.Errorf("CCFOLIA character %s face %s: %w", entry.SourceID, face.Label, err)
			}
			if faceImage == nil {
				continue
			}
			faces = append(faces, map[string]any{"label": face.Label, "image": faceImage})
			if imageRef == nil {
				imageRef = faceImage
			}
		}
		content, _ := json.Marshal(map[string]any{"image": imageRef, "text": character.Name})
		ccfoliaMetadata := map[string]any{
			"sourceCharacterId": entry.SourceID, "playerName": character.PlayerName, "memo": character.Memo,
			"secret": character.Secret, "color": character.Color, "sourceRaw": string(character.Raw),
		}
		if len(faces) > 0 {
			ccfoliaMetadata["faces"] = faces
		}
		metadata, _ := json.Marshal(map[string]any{"ccfolia": ccfoliaMetadata})
		objectID := utils.NewID()
		aspect := true
		objects[objectID] = TheaterObjectSnapshot{
			ID: objectID, SceneID: &sceneID, Kind: "image", Name: ccfoliaName(character.Name, "CCFOLIA Character"),
			X: character.X + character.Width/2, Y: character.Y + character.Height/2, Width: character.Width, Height: character.Height,
			Rotation: character.Angle, Scale: 1, ScaleX: 1, ScaleY: 1, Z: character.Z, OrderKey: strconv.Itoa(index + 1),
			Visible: character.Active && !character.Invisible, Locked: false, AspectRatioLocked: &aspect, Interactive: false, Editable: false,
			Content: content, Actions: json.RawMessage(`[]`), Metadata: metadata,
		}
	}
	return objects, nil, nil
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
		contentValue := map[string]any{"image": imageRef, "text": item.Memo}
		if memo := strings.TrimSpace(item.Memo); memo != "" {
			contentValue["annotation"] = defaultTheaterImageAnnotation(memo)
		}
		content, _ := json.Marshal(contentValue)
		actionMetadata, actionWarnings := ccfoliaActions(item.ClickAction, sceneNameIDs)
		actions := actionMetadata.Actions
		warnings = append(warnings, actionWarnings...)
		if item.CoverImageURL != nil && strings.TrimSpace(*item.CoverImageURL) != "" {
			if _, err := ccfoliaImageRef(item.CoverImageURL, item.Memo+"封面", worldID, targets); err != nil {
				return nil, warnings, fmt.Errorf("CCFOLIA item %s coverImageUrl: %w", entry.SourceID, err)
			}
			warnings = appendWarning(warnings, "item.coverImageUrl 暂不映射，已保存在对象元数据")
		}
		itemMetadata := map[string]any{"sourceItemId": entry.SourceID, "sourceOrder": item.Order, "sourceRaw": string(item.Raw)}
		if actionMetadata.Metadata != nil {
			itemMetadata["clickAction"] = actionMetadata.Metadata
		}
		metadata, _ := json.Marshal(map[string]any{"ccfolia": itemMetadata})
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

type ccfoliaActionConversion struct {
	Actions  json.RawMessage
	Metadata map[string]any
}

func ccfoliaActions(action *ccfoliaClickAction, sceneNameIDs map[string][]string) (ccfoliaActionConversion, []string) {
	if action == nil {
		return ccfoliaActionConversion{Actions: json.RawMessage(`[]`)}, nil
	}
	conversion := ccfoliaActionConversion{
		Actions: json.RawMessage(`[]`),
		Metadata: map[string]any{
			"source":   map[string]any{"format": "ccfolia", "type": action.Type, "text": action.Text},
			"resolved": false,
		},
	}
	if action.Type != "message" {
		conversion.Metadata["reason"] = "unsupported-action-type"
		return conversion, []string{"存在未映射 clickAction，已保留为未解析源元数据"}
	}
	if strings.HasPrefix(action.Text, "/scene ") {
		name := strings.TrimPrefix(action.Text, "/scene ")
		conversion.Metadata["type"] = "scene-switch"
		conversion.Metadata["targetSceneName"] = name
		if name == "" {
			conversion.Metadata["reason"] = "empty-target-scene-name"
			return conversion, []string{"存在空 /scene 点击动作，已保留为未解析源元数据"}
		}
		ids := sceneNameIDs[name]
		if len(ids) == 1 {
			conversion.Actions, _ = json.Marshal([]map[string]any{{"id": utils.NewID(), "type": TheaterMutationSceneApply, "payload": map[string]any{"sceneId": ids[0]}}})
			conversion.Metadata["targetSceneId"] = ids[0]
			conversion.Metadata["resolved"] = true
			return conversion, nil
		}
		if len(ids) > 1 {
			conversion.Metadata["reason"] = "ambiguous-target-scene-name"
			return conversion, []string{"存在重名场景，/scene 点击动作已保留为未解析源元数据"}
		}
		conversion.Metadata["reason"] = "target-scene-not-found"
		return conversion, []string{"/scene 点击动作目标场景不存在，已保留为未解析源元数据"}
	}
	trimmedActionText := strings.TrimSpace(action.Text)
	if trimmedActionText == "/roll-table" || strings.HasPrefix(trimmedActionText, "/roll-table\n") || strings.HasPrefix(trimmedActionText, "/roll-table\r") {
		conversion.Metadata["type"] = "random-table"
		randomTablePayload, err := parseCCFOLIARandomTable(action.Text)
		if err == nil {
			conversion.Actions, _ = json.Marshal([]map[string]any{{"id": utils.NewID(), "type": theaterActionChatRandomTable, "payload": randomTablePayload}})
			conversion.Metadata["resolved"] = true
			return conversion, nil
		}
		conversion.Metadata["reason"] = "random-table-parse-failed"
		conversion.Metadata["failureReason"] = err.Error()
		fallbackPayload, sendErr := normalizeTheaterChatSendPayload(theaterChatSendPayload{Content: action.Text})
		if sendErr == nil {
			conversion.Actions, _ = json.Marshal([]map[string]any{{"id": utils.NewID(), "type": "chat.send", "payload": map[string]any{"content": fallbackPayload.Content}}})
		}
		return conversion, []string{"随机表解析失败，已按普通消息导入"}
	}
	content := action.Text
	if strings.HasPrefix(content, "/send ") {
		content = strings.TrimPrefix(content, "/send ")
	}
	conversion.Metadata["type"] = "chat-send"
	payload, err := normalizeTheaterChatSendPayload(theaterChatSendPayload{Content: content})
	if err != nil {
		conversion.Metadata["reason"] = "invalid-send-content"
		return conversion, []string{"message 点击动作内容无效，已保留为未解析源元数据"}
	}
	conversion.Actions, _ = json.Marshal([]map[string]any{{"id": utils.NewID(), "type": "chat.send", "payload": map[string]any{"content": payload.Content}}})
	conversion.Metadata["resolved"] = true
	return conversion, nil
}

func parseCCFOLIARandomTable(source string) (theaterRandomTablePayload, error) {
	normalized := strings.ReplaceAll(strings.ReplaceAll(source, "\r\n", "\n"), "\r", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) < 4 || strings.TrimSpace(lines[0]) != "/roll-table" {
		return theaterRandomTablePayload{}, theaterPayloadError("随机表第一行必须为 /roll-table")
	}
	payload := theaterRandomTablePayload{
		Name:    strings.TrimSpace(lines[1]),
		Formula: strings.TrimSpace(lines[2]),
		Entries: make([]theaterRandomTableEntry, 0, len(lines)-3),
	}
	for _, line := range lines[3:] {
		matches := ccfoliaRandomTableEntryPattern.FindStringSubmatch(line)
		if matches == nil {
			if len(payload.Entries) == 0 {
				if strings.TrimSpace(line) == "" {
					continue
				}
				return theaterRandomTablePayload{}, theaterPayloadError("随机表结果行缺少点数")
			}
			payload.Entries[len(payload.Entries)-1].Text += "\n" + line
			continue
		}
		minimum, minErr := strconv.ParseInt(matches[1], 10, 32)
		maximum := minimum
		var maxErr error
		if matches[2] != "" {
			maximum, maxErr = strconv.ParseInt(matches[2], 10, 32)
		}
		if minErr != nil || maxErr != nil {
			return theaterRandomTablePayload{}, theaterPayloadError("随机表点数超出限制")
		}
		payload.Entries = append(payload.Entries, theaterRandomTableEntry{
			Min: int(minimum), Max: int(maximum), Text: matches[3],
		})
	}
	return normalizeTheaterRandomTablePayload(payload)
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
	if target.LoopCount != nil {
		result["loopCount"] = *target.LoopCount
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

func ccfoliaAssetReferences(backup ccfoliaBackup) map[string][]string {
	result := map[string][]string{}
	add := func(ref *string, path string) {
		if ref == nil || strings.TrimSpace(*ref) == "" {
			return
		}
		value := strings.TrimSpace(*ref)
		result[value] = append(result[value], path)
	}
	addRoom := func(room ccfoliaRoom, path string) {
		add(room.BackgroundURL, path+".backgroundUrl")
		add(room.ForegroundURL, path+".foregroundUrl")
		for markerID, marker := range room.Markers {
			add(&marker.ImageURL, path+".markers."+markerID+".imageUrl")
		}
	}
	addScenes := func(scenes map[string]ccfoliaScene, path string) {
		for sceneID, scene := range scenes {
			add(scene.BackgroundURL, path+"."+sceneID+".backgroundUrl")
			add(scene.ForegroundURL, path+"."+sceneID+".foregroundUrl")
			for markerID, marker := range scene.Markers {
				add(&marker.ImageURL, path+"."+sceneID+".markers."+markerID+".imageUrl")
			}
		}
	}
	addItems := func(items map[string]ccfoliaItem, path string) {
		for itemID, item := range items {
			add(&item.ImageURL, path+"."+itemID+".imageUrl")
			add(item.CoverImageURL, path+"."+itemID+".coverImageUrl")
		}
	}
	addCharacters := func(characters map[string]ccfoliaCharacter, path string) {
		for characterID, character := range characters {
			characterPath := path + "." + characterID
			add(character.IconURL, characterPath+".iconUrl")
			for index, face := range character.Faces {
				add(face.IconURL, characterPath+".faces."+strconv.Itoa(index)+".iconUrl")
			}
		}
	}
	addNotes := func(notes map[string]ccfoliaNote, path string) {
		for noteID, note := range notes {
			add(note.IconURL, path+"."+noteID+".iconUrl")
		}
	}

	addRoom(backup.Entities.Room, "entities.room")
	addScenes(backup.Entities.Scenes, "entities.scenes")
	addItems(backup.Entities.Items, "entities.items")
	addCharacters(backup.Entities.Characters, "entities.characters")
	addNotes(backup.Entities.Notes, "entities.notes")
	for savedataID, savedata := range backup.Entities.Savedatas {
		add(savedata.Thumbnail, "entities.savedatas."+savedataID+".thumbnail")
	}
	for snapshotID, snapshot := range backup.Entities.Snapshots {
		path := "entities.snapshots." + snapshotID
		addRoom(snapshot.Room, path+".room")
		addScenes(snapshot.Scenes, path+".scenes")
		addItems(snapshot.Items, path+".items")
		addCharacters(snapshot.Characters, path+".characters")
		addNotes(snapshot.Notes, path+".notes")
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

func newCCFOLIASourceArchive(raw []byte) ccfoliaSourceArchive {
	if len(raw) == 0 {
		return ccfoliaSourceArchive{}
	}
	hash := sha256.Sum256(raw)
	return ccfoliaSourceArchive{SHA256: hex.EncodeToString(hash[:]), SizeBytes: int64(len(raw))}
}

func addCCFOLIASourceArchiveMetadata(metadata map[string]any, archive ccfoliaSourceArchive) {
	if archive.SHA256 == "" {
		return
	}
	metadata["sourceRawExternal"] = true
	metadata["sourceRawSha256"] = archive.SHA256
	metadata["sourceRawBytes"] = archive.SizeBytes
	metadata["sourceRawTruncated"] = archive.Truncated
}

func ccfoliaSourceArchivePath(hash string) string {
	return filepath.Join(theaterPackageStorageDir(), "ccfolia-source", hash[:2], hash+".json")
}

func persistCCFOLIASourceArchive(raw []byte, archive ccfoliaSourceArchive) (bool, error) {
	if archive.SHA256 == "" || len(raw) == 0 {
		return false, fmt.Errorf("CCFOLIA 原始数据为空")
	}
	target := ccfoliaSourceArchivePath(archive.SHA256)
	if err := verifyCCFOLIASourceArchive(target, archive); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return false, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".ccfolia-source-*")
	if err != nil {
		return false, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return false, err
	}
	if _, err := temporary.Write(raw); err != nil {
		_ = temporary.Close()
		return false, err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return false, err
	}
	if err := temporary.Close(); err != nil {
		return false, err
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return false, err
	}
	if err := verifyCCFOLIASourceArchive(target, archive); err != nil {
		_ = os.Remove(target)
		return false, err
	}
	return true, nil
}

func verifyCCFOLIASourceArchive(path string, archive ccfoliaSourceArchive) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != archive.SizeBytes {
		return fmt.Errorf("CCFOLIA 原始数据文件大小不一致")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	actual := newCCFOLIASourceArchive(raw)
	if actual.SHA256 != archive.SHA256 {
		return fmt.Errorf("CCFOLIA 原始数据文件哈希不一致")
	}
	return nil
}

func removeUnreferencedCCFOLIASourceArchive(hash string) {
	if !ccfoliaSourceHashPattern.MatchString(hash) {
		return
	}
	var references int64
	if err := model.GetDB().Model(&model.TheaterSourceArchiveModel{}).Where("sha256 = ?", hash).Count(&references).Error; err != nil || references > 0 {
		return
	}
	_ = os.Remove(ccfoliaSourceArchivePath(hash))
}

func cleanupExpiredCCFOLIASourceArchives(now time.Time) error {
	var archives []model.TheaterSourceArchiveModel
	if err := model.GetDB().Where("status = ? AND cleanup_after IS NOT NULL AND cleanup_after <= ?", model.TheaterSourceArchiveStatusDeleting, now).Limit(200).Find(&archives).Error; err != nil {
		return err
	}
	for _, archive := range archives {
		if !ccfoliaSourceHashPattern.MatchString(archive.SHA256) {
			return fmt.Errorf("CCFOLIA 原始数据哈希无效: %s", archive.ID)
		}
		var otherReferences int64
		if err := model.GetDB().Model(&model.TheaterSourceArchiveModel{}).Where("sha256 = ? AND id <> ?", archive.SHA256, archive.ID).Count(&otherReferences).Error; err != nil {
			return err
		}
		if otherReferences == 0 {
			if err := os.Remove(ccfoliaSourceArchivePath(archive.SHA256)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		if err := model.GetDB().Unscoped().Delete(&archive).Error; err != nil {
			return err
		}
	}
	return cleanupOrphanedCCFOLIASourceArchives(now)
}

func cleanupOrphanedCCFOLIASourceArchives(now time.Time) error {
	root := filepath.Join(theaterPackageStorageDir(), "ccfolia-source")
	prefixes, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	cutoff := now.Add(-theaterResourceDeleteGrace)
	for _, prefix := range prefixes {
		if !prefix.IsDir() || len(prefix.Name()) != 2 {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, prefix.Name()))
		if err != nil {
			return err
		}
		for _, file := range files {
			if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
				continue
			}
			hash := strings.TrimSuffix(file.Name(), ".json")
			if !ccfoliaSourceHashPattern.MatchString(hash) || !strings.HasPrefix(hash, prefix.Name()) {
				continue
			}
			info, err := file.Info()
			if err != nil {
				return err
			}
			if info.ModTime().After(cutoff) {
				continue
			}
			var references int64
			if err := model.GetDB().Model(&model.TheaterSourceArchiveModel{}).Where("sha256 = ?", hash).Count(&references).Error; err != nil {
				return err
			}
			if references == 0 {
				if err := os.Remove(filepath.Join(root, prefix.Name(), file.Name())); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
		}
	}
	return nil
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
