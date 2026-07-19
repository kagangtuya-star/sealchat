package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/utils"
)

func GetTheaterSnapshot(_ context.Context, actorID, worldID, channelID string, options TheaterSnapshotOptions) (*TheaterSnapshotResult, error) {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionView); err != nil {
		return nil, err
	}
	return getTheaterSnapshot(actorID, worldID, channelID, options, listTheaterPermissions(actorID, worldID, channelID))
}

func GetTheaterSnapshotForObserver(_ context.Context, observerWorldID, channelID string, options TheaterSnapshotOptions) (*TheaterSnapshotResult, error) {
	if err := canObserverAccessTheaterScope(channelID, observerWorldID); err != nil {
		return nil, newTheaterError(TheaterErrorPermissionDenied, "没有 Theater 旁观权限", 403, nil)
	}
	return getTheaterSnapshot("observer", observerWorldID, channelID, options, []string{TheaterPermissionView})
}

func getTheaterSnapshot(actorID, worldID, channelID string, options TheaterSnapshotOptions, permissions []string) (*TheaterSnapshotResult, error) {
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return nil, err
	}
	snapshot, checksum, err := buildTheaterSnapshot(model.GetDB(), room, options.IncludeResources)
	if err != nil {
		return nil, err
	}
	result := &TheaterSnapshotResult{
		RoomID:        room.ID,
		WorldID:       worldID,
		ChannelID:     channelID,
		Revision:      room.Revision,
		SchemaVersion: room.SchemaVersion,
		Checksum:      checksum,
		Snapshot:      snapshot,
		Limits: map[string]int64{
			"snapshotBytes": theaterMaxSnapshotBytes,
			"payloadBytes":  theaterMaxPayloadBytes,
			"scenes":        theaterMaxScenes,
			"objects":       theaterMaxObjects,
		},
		Permissions: permissions,
	}
	if options.IfRevision != nil && *options.IfRevision == room.Revision {
		result.Unchanged = true
	}
	if raw, marshalErr := json.Marshal(snapshot); marshalErr == nil {
		RecordTheaterMetric("theater_snapshot_bytes", nil, float64(len(raw)))
	}
	return result, nil
}

func RestoreTheaterSnapshot(ctx context.Context, actorID string, command TheaterRestoreCommand, meta TheaterRequestMeta) (*TheaterMutationResult, error) {
	if _, _, err := requireTheaterPermission(actorID, command.WorldID, command.ChannelID, TheaterPermissionAdminRestore); err != nil {
		return nil, err
	}
	room, err := model.TheaterRoomCreateIfMissing(command.WorldID, command.ChannelID, actorID)
	if err != nil {
		return nil, err
	}
	snapshotModel, err := model.TheaterSnapshotGet(room.ID, command.SnapshotID)
	if err != nil {
		return nil, err
	}
	if snapshotModel == nil {
		return nil, newTheaterError(TheaterErrorNotFound, "checkpoint 不存在", 404, nil)
	}
	var snapshot TheaterSharedSnapshot
	if err := decodeStrictJSON([]byte(snapshotModel.SnapshotJSON), &snapshot); err != nil {
		return nil, newTheaterError(TheaterErrorSchemaUnsupported, "checkpoint schema 无效", 409, nil)
	}
	expected := room.Revision
	if command.ExpectedRevision != nil {
		expected = *command.ExpectedRevision
	}
	return replaceTheaterSnapshot(ctx, actorID, TheaterReplaceCommand{MutationID: command.MutationID, WorldID: command.WorldID, ChannelID: command.ChannelID, ExpectedRevision: expected, SchemaVersion: snapshotModel.SchemaVersion, Snapshot: snapshot, Reason: command.Reason}, meta, TheaterMutationAdminRestore, snapshotModel.ID)
}

func ReplaceTheaterSnapshot(ctx context.Context, actorID string, command TheaterReplaceCommand, meta TheaterRequestMeta) (*TheaterMutationResult, error) {
	if _, _, err := requireTheaterPermission(actorID, command.WorldID, command.ChannelID, TheaterPermissionAdminRestore); err != nil {
		return nil, err
	}
	return replaceTheaterSnapshot(ctx, actorID, command, meta, TheaterMutationAdminReplace, "")
}

func replaceTheaterSnapshot(ctx context.Context, actorID string, command TheaterReplaceCommand, meta TheaterRequestMeta, mutationType, sourceSnapshotID string) (*TheaterMutationResult, error) {
	if strings.TrimSpace(command.MutationID) == "" || strings.TrimSpace(command.Reason) == "" {
		return nil, theaterPayloadError("mutationId 和 reason 必填")
	}
	if command.SchemaVersion != model.TheaterSchemaVersion {
		return nil, newTheaterError(TheaterErrorSchemaUnsupported, "不支持 snapshot schemaVersion", 409, map[string]any{"schemaVersion": command.SchemaVersion})
	}
	if err := validateTheaterSharedSnapshot(command.Snapshot); err != nil {
		return nil, err
	}
	snapshotJSON, _, err := canonicalTheaterJSON(command.Snapshot)
	if err != nil {
		return nil, err
	}
	if len(snapshotJSON) > theaterMaxSnapshotBytes {
		return nil, newTheaterError(TheaterErrorLimitExceeded, "snapshot 超过 4 MiB", 413, nil)
	}
	payload, _ := json.Marshal(map[string]any{"reason": command.Reason, "sourceSnapshotId": sourceSnapshotID, "schemaVersion": command.SchemaVersion, "snapshotHash": theaterJSONHash(snapshotJSON)})
	payloadHash := theaterJSONHash(payload)
	room, err := model.TheaterRoomCreateIfMissing(command.WorldID, command.ChannelID, actorID)
	if err != nil {
		return nil, err
	}
	var result *TheaterMutationResult
	var outcomeErr error
	createdMutation := false
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		var existing model.TheaterMutationModel
		if err := tx.Where("room_id = ? AND mutation_id = ?", room.ID, command.MutationID).Limit(1).Find(&existing).Error; err != nil {
			return err
		}
		if existing.ID != "" {
			if existing.ActorUserID != actorID || existing.Type != mutationType || existing.PayloadHash != payloadHash {
				outcomeErr = newTheaterError(TheaterErrorMutationIDReused, "mutationId 已用于不同请求", 409, nil)
				return nil
			}
			if err := json.Unmarshal([]byte(existing.ResultJSON), &result); err != nil {
				return err
			}
			result.Idempotent = true
			return nil
		}
		var current model.TheaterRoomModel
		if err := tx.Where("id = ?", room.ID).First(&current).Error; err != nil {
			return err
		}
		if current.Revision != command.ExpectedRevision {
			outcomeErr = newTheaterError(TheaterErrorRevisionConflict, "Theater revision 冲突", 409, map[string]any{"expectedRevision": command.ExpectedRevision, "currentRevision": current.Revision})
			if err := persistRejectedTheaterMutation(tx, &current, actorID, TheaterMutationCommand{MutationID: command.MutationID, WorldID: command.WorldID, ChannelID: command.ChannelID, ExpectedRevision: command.ExpectedRevision, Type: mutationType, Payload: payload}, payload, payloadHash, meta, outcomeErr.(*TheaterError)); err != nil {
				return err
			}
			createdMutation = true
			return nil
		}
		currentSnapshot, currentHash, err := buildTheaterSnapshot(tx, &current, true)
		if err != nil {
			return err
		}
		currentJSON, _, err := canonicalTheaterJSON(currentSnapshot)
		if err != nil {
			return err
		}
		preReplace := &model.TheaterSnapshotModel{RoomID: current.ID, Revision: current.Revision, SchemaVersion: current.SchemaVersion, SnapshotJSON: string(currentJSON), SnapshotHash: currentHash, SnapshotBytes: int64(len(currentJSON)), Kind: "pre-replace", Reason: command.Reason, CreatedBy: actorID}
		if err := tx.Create(preReplace).Error; err != nil {
			return err
		}
		preReplaceExpiresAt := time.Now().Add(theaterSnapshotRetention)
		if err := createTheaterResourceHolds(tx, preReplace, &preReplaceExpiresAt); err != nil {
			return err
		}
		nextRevision := current.Revision + 1
		cas := tx.Model(&model.TheaterRoomModel{}).Where("id = ? AND revision = ?", current.ID, current.Revision).Updates(map[string]any{"revision": nextRevision, "updated_by": actorID, "updated_at": time.Now()})
		if cas.Error != nil || cas.RowsAffected != 1 {
			if cas.Error != nil {
				return cas.Error
			}
			return errTheaterConcurrentCAS
		}
		if err := replaceTheaterRows(tx, &current, actorID, command.Snapshot); err != nil {
			return err
		}
		current.Revision = nextRevision
		newSnapshot, checksum, err := buildTheaterSnapshot(tx, &current, true)
		if err != nil {
			return err
		}
		_ = newSnapshot
		result = &TheaterMutationResult{MutationID: command.MutationID, RevisionBefore: command.ExpectedRevision, Revision: nextRevision, Type: mutationType, Payload: payload, Checksum: checksum}
		resultJSON, _ := json.Marshal(result)
		revisionAfter := nextRevision
		if err := tx.Create(&model.TheaterMutationModel{RoomID: current.ID, WorldID: current.WorldID, ChannelID: current.ChannelID, MutationID: command.MutationID, ActorUserID: actorID, ExpectedRevision: command.ExpectedRevision, RevisionBefore: command.ExpectedRevision, RevisionAfter: &revisionAfter, Type: mutationType, PayloadJSON: string(payload), PayloadHash: payloadHash, ResultJSON: string(resultJSON), Status: "applied", RequestSource: "admin", RequestID: meta.RequestID, SessionID: meta.SessionID}).Error; err != nil {
			return err
		}
		createdMutation = true
		if err := createTheaterAudit(tx, &current, actorID, meta, command.MutationID, mutationType, "replaced", "", command.Reason, command.ExpectedRevision, &revisionAfter, payload); err != nil {
			return err
		}
		return tx.Model(&model.TheaterRoomModel{}).Where("id = ?", current.ID).Update("state_hash", checksum).Error
	})
	if errors.Is(err, errTheaterConcurrentCAS) {
		return nil, newTheaterError(TheaterErrorRevisionConflict, "Theater revision 冲突", 409, nil)
	}
	if err != nil {
		return nil, err
	}
	if createdMutation {
		if outcomeErr != nil {
			_ = PublishTheaterMutationNow(ctx, room.ID, command.MutationID)
		} else {
			EnqueueTheaterMutation(command.MutationID)
		}
	}
	if outcomeErr != nil {
		return nil, outcomeErr
	}
	return result, nil
}

func validateTheaterSharedSnapshot(snapshot TheaterSharedSnapshot) error {
	if len(snapshot.Scenes) > theaterMaxScenes || len(snapshot.PersistentObjects)+len(snapshot.Characters) > theaterMaxObjects {
		return newTheaterError(TheaterErrorLimitExceeded, "snapshot 实体数量超限", 409, nil)
	}
	if snapshot.ActiveSceneID != nil {
		if _, ok := snapshot.Scenes[*snapshot.ActiveSceneID]; !ok {
			return theaterPayloadError("activeSceneId 不存在")
		}
	}
	totalObjects := 0
	for id, scene := range snapshot.Scenes {
		if id != scene.ID {
			return theaterPayloadError("scene map key 与 id 不一致")
		}
		var state map[string]any
		if err := json.Unmarshal(scene.State, &state); err != nil || validateSceneState(state) != nil {
			return theaterPayloadError("scene state 无效")
		}
		totalObjects += len(scene.Objects)
	}
	if totalObjects+len(snapshot.PersistentObjects) > theaterMaxObjects {
		return newTheaterError(TheaterErrorLimitExceeded, "snapshot 对象数量超限", 409, nil)
	}
	return nil
}

func replaceTheaterRows(tx *gorm.DB, room *model.TheaterRoomModel, actorID string, snapshot TheaterSharedSnapshot) error {
	if err := tx.Unscoped().Where("room_id = ?", room.ID).Delete(&model.TheaterObjectModel{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("room_id = ?", room.ID).Delete(&model.TheaterSceneModel{}).Error; err != nil {
		return err
	}
	room.ActiveSceneID = derefString(snapshot.ActiveSceneID)
	room.StateJSON = defaultJSON(snapshot.LiveState, `{}`)
	if err := tx.Model(&model.TheaterRoomModel{}).Where("id = ?", room.ID).Updates(map[string]any{"active_scene_id": room.ActiveSceneID, "state_json": room.StateJSON, "schema_version": model.TheaterSchemaVersion}).Error; err != nil {
		return err
	}
	for id, scene := range snapshot.Scenes {
		row := model.TheaterSceneModel{StringPKBaseModel: model.StringPKBaseModel{ID: id}, RoomID: room.ID, Name: scene.Name, SortOrder: scene.Order, Locked: scene.Locked, StateJSON: defaultJSON(scene.State, `{}`), SchemaVersion: model.TheaterSchemaVersion, CreatedBy: actorID, UpdatedBy: actorID}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	createSnapshotObject := func(item TheaterObjectSnapshot, sceneID *string) error {
		scale := item.Scale
		if scale <= 0 {
			scale = 1
		}
		scaleX := item.ScaleX
		if scaleX <= 0 {
			scaleX = scale
		}
		scaleY := item.ScaleY
		if scaleY <= 0 {
			scaleY = scale
		}
		input := theaterObjectInput{ID: item.ID, ParentID: item.ParentID, Kind: item.Kind, Name: item.Name, X: item.X, Y: item.Y, Width: item.Width, Height: item.Height, Rotation: item.Rotation, Scale: &scale, ScaleX: &scaleX, ScaleY: &scaleY, Z: item.Z, OrderKey: item.OrderKey, Visible: &item.Visible, Locked: item.Locked, AspectRatioLocked: item.AspectRatioLocked, Interactive: item.Interactive, Editable: item.Editable, OwnerUserID: item.OwnerUserID, CharacterIdentityID: item.CharacterIdentityID, Content: item.Content, Actions: item.Actions, Metadata: item.Metadata}
		if err := validateObjectInput(&input); err != nil {
			return err
		}
		return createTheaterObject(tx, room, actorID, sceneID, &input)
	}
	for _, scene := range snapshot.Scenes {
		for _, object := range scene.Objects {
			sceneID := scene.ID
			if err := createSnapshotObject(object, &sceneID); err != nil {
				return err
			}
		}
	}
	for _, object := range snapshot.PersistentObjects {
		if err := createSnapshotObject(object, nil); err != nil {
			return err
		}
	}
	if err := recalculateTheaterResourceReferences(tx, room.ID); err != nil {
		return err
	}
	referencedResources := map[string]int64{}
	if snapshotJSON, err := json.Marshal(snapshot); err == nil {
		countResourceIDsInJSON(string(snapshotJSON), referencedResources)
	}
	for resourceID := range referencedResources {
		var count int64
		if err := tx.Model(&model.TheaterResourceModel{}).Where("room_id = ? AND id = ? AND status = ?", room.ID, resourceID, "ready").Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return theaterPayloadError(fmt.Sprintf("resource %s 不属于房间或未 ready", resourceID))
		}
	}
	return nil
}

func CreateTheaterCheckpoint(actorID, worldID, channelID, kind, reason string) (*model.TheaterSnapshotModel, error) {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionAdminRestore); err != nil {
		return nil, err
	}
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return nil, err
	}
	snapshot, hash, err := buildTheaterSnapshot(model.GetDB(), room, true)
	if err != nil {
		return nil, err
	}
	raw, _, err := canonicalTheaterJSON(snapshot)
	if err != nil {
		return nil, err
	}
	item := &model.TheaterSnapshotModel{StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()}, RoomID: room.ID, Revision: room.Revision, SchemaVersion: room.SchemaVersion, SnapshotJSON: string(raw), SnapshotHash: hash, SnapshotBytes: int64(len(raw)), Kind: kind, Reason: reason, CreatedBy: actorID}
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(item).Error; err != nil {
			return err
		}
		return createTheaterResourceHolds(tx, item, nil)
	})
	return item, err
}

func ListTheaterCheckpoints(actorID, worldID, channelID string, limit int) ([]model.TheaterSnapshotModel, error) {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionAdminRestore); err != nil {
		return nil, err
	}
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	var items []model.TheaterSnapshotModel
	err = model.GetDB().Select("id", "room_id", "revision", "schema_version", "snapshot_hash", "snapshot_bytes", "kind", "reason", "created_by", "created_at").Where("room_id = ?", room.ID).Order("created_at DESC").Limit(limit).Find(&items).Error
	return items, err
}

func buildTheaterSnapshot(conn *gorm.DB, room *model.TheaterRoomModel, includeResources bool) (TheaterSharedSnapshot, string, error) {
	result := TheaterSharedSnapshot{
		LiveState:         normalizedRawJSON(room.StateJSON, `{}`),
		Scenes:            map[string]TheaterSceneSnapshot{},
		PersistentObjects: map[string]TheaterObjectSnapshot{},
		Characters:        map[string]TheaterObjectSnapshot{},
		Resources:         map[string]TheaterResourcePublic{},
	}
	if strings.TrimSpace(room.ActiveSceneID) != "" {
		value := room.ActiveSceneID
		result.ActiveSceneID = &value
	}
	var scenes []model.TheaterSceneModel
	if err := conn.Where("room_id = ?", room.ID).Order("sort_order ASC, id ASC").Find(&scenes).Error; err != nil {
		return result, "", err
	}
	for _, scene := range scenes {
		result.Scenes[scene.ID] = TheaterSceneSnapshot{ID: scene.ID, Name: scene.Name, Order: scene.SortOrder, Locked: scene.Locked, State: normalizedRawJSON(scene.StateJSON, `{}`), Objects: map[string]TheaterObjectSnapshot{}}
	}
	var objects []model.TheaterObjectModel
	if err := conn.Where("room_id = ?", room.ID).Order("order_key ASC, id ASC").Find(&objects).Error; err != nil {
		return result, "", err
	}
	for _, object := range objects {
		item := theaterObjectSnapshotFromModel(object)
		if object.Kind == "character" {
			result.Characters[object.ID] = item
		}
		if object.SceneID == "" {
			result.PersistentObjects[object.ID] = item
			continue
		}
		scene, ok := result.Scenes[object.SceneID]
		if ok {
			scene.Objects[object.ID] = item
			result.Scenes[object.SceneID] = scene
		}
	}
	if includeResources {
		var resources []model.TheaterResourceModel
		if err := conn.Where("room_id = ? AND status NOT IN ?", room.ID, []string{"deleting", "purging"}).Find(&resources).Error; err != nil {
			return result, "", err
		}
		for _, resource := range resources {
			public, err := theaterResourcePublicFromModel(conn, resource)
			if err != nil {
				return result, "", err
			}
			result.Resources[resource.ID] = public
		}
	}
	raw, checksum, err := canonicalTheaterJSON(result)
	if err != nil {
		return result, "", err
	}
	if len(raw) > theaterMaxSnapshotBytes {
		return result, "", newTheaterError(TheaterErrorLimitExceeded, "Theater snapshot 超过上限", 413, map[string]any{"limit": theaterMaxSnapshotBytes})
	}
	return result, checksum, nil
}

func normalizedRawJSON(value, fallback string) json.RawMessage {
	if !json.Valid([]byte(value)) {
		return json.RawMessage(fallback)
	}
	return json.RawMessage(value)
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	copy := value
	return &copy
}

func theaterObjectSnapshotFromModel(object model.TheaterObjectModel) TheaterObjectSnapshot {
	var sceneID *string
	if object.SceneID != "" {
		value := object.SceneID
		sceneID = &value
	}
	scale := object.Scale
	if scale <= 0 {
		scale = 1
	}
	scaleX := object.ScaleX
	if scaleX <= 0 {
		scaleX = scale
	}
	scaleY := object.ScaleY
	if scaleY <= 0 {
		scaleY = scale
	}
	aspectRatioLocked := object.AspectRatioLocked
	return TheaterObjectSnapshot{
		ID: object.ID, SceneID: sceneID, ParentID: optionalString(object.ParentID), Kind: object.Kind, Name: object.Name,
		X: object.X, Y: object.Y, Width: object.Width, Height: object.Height, Rotation: object.Rotation, Scale: scale, ScaleX: scaleX, ScaleY: scaleY, Z: object.Z, OrderKey: object.OrderKey,
		Visible: object.Visible, Locked: object.Locked, AspectRatioLocked: &aspectRatioLocked, Interactive: object.Interactive, Editable: object.Editable,
		OwnerUserID: optionalString(object.OwnerUserID), CharacterIdentityID: optionalString(object.CharacterIdentityID),
		Content: normalizedRawJSON(object.ContentJSON, `{}`), Actions: normalizedRawJSON(object.ActionsJSON, `[]`), Metadata: normalizedRawJSON(object.MetadataJSON, `{}`),
	}
}

func theaterResourcePublicFromModel(conn *gorm.DB, resource model.TheaterResourceModel) (TheaterResourcePublic, error) {
	var variants []model.TheaterResourceVariantModel
	if err := conn.Where("resource_id = ? AND status = ?", resource.ID, "ready").Order("name ASC").Find(&variants).Error; err != nil {
		return TheaterResourcePublic{}, err
	}
	publicVariants := make([]TheaterResourceVariantPublic, 0, len(variants))
	playbackVariant := "original"
	playbackMimeType := resource.MimeType
	for _, variant := range variants {
		publicVariants = append(publicVariants, TheaterResourceVariantPublic{Name: variant.Name, MimeType: variant.MimeType, Width: variant.Width, Height: variant.Height, SizeBytes: variant.SizeBytes})
		if resource.Kind == "animated_image" && variant.Name == "display" && variant.MimeType == "video/webm" {
			playbackVariant = variant.Name
			playbackMimeType = variant.MimeType
		}
	}
	return TheaterResourcePublic{
		ID: resource.ID, Kind: resource.Kind, Status: resource.Status, MimeType: resource.MimeType, SizeBytes: resource.SizeBytes,
		Width: resource.Width, Height: resource.Height, DurationMS: resource.DurationMS, FrameCount: resource.FrameCount, FrameRate: resource.FrameRate,
		Animated: resource.Kind == "animated_image", PlaybackVariant: playbackVariant, PlaybackMimeType: playbackMimeType,
		PosterResourceID: optionalString(resource.PosterResourceID), Variants: publicVariants,
		Processing: TheaterResourceProcessing{Progress: resource.ProcessingProgress, Retryable: resource.Retryable, ErrorCode: resource.FailureCode},
	}, nil
}

func TheaterResourcePublicForEvent(resource model.TheaterResourceModel) (TheaterResourcePublic, error) {
	return theaterResourcePublicFromModel(model.GetDB(), resource)
}

func ListTheaterEvents(_ context.Context, actorID, worldID, channelID string, afterRevision int64, limit int) (*TheaterEventsResult, error) {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionView); err != nil {
		return nil, err
	}
	return listTheaterEvents(actorID, worldID, channelID, afterRevision, limit, CanAdministerTheater(actorID, worldID, channelID))
}

func ListTheaterEventsForObserver(_ context.Context, observerWorldID, channelID string, afterRevision int64, limit int) (*TheaterEventsResult, error) {
	if err := canObserverAccessTheaterScope(channelID, observerWorldID); err != nil {
		return nil, newTheaterError(TheaterErrorPermissionDenied, "没有 Theater 旁观权限", 403, nil)
	}
	return listTheaterEvents("observer", observerWorldID, channelID, afterRevision, limit, false)
}

func listTheaterEvents(actorID, worldID, channelID string, afterRevision int64, limit int, allowManagement bool) (*TheaterEventsResult, error) {
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	items, err := model.TheaterMutationListAfterRevision(room.ID, afterRevision, limit+1)
	if err != nil {
		return nil, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	result := &TheaterEventsResult{FromRevision: afterRevision, ToRevision: afterRevision, CurrentRevision: room.Revision, HasMore: hasMore, Events: []TheaterEvent{}}
	for _, item := range items {
		if !allowManagement && isTheaterManagementMutation(item.Type) {
			return nil, newTheaterError(TheaterErrorHistoryExpired, "Theater event history 包含全量同步点", 410, nil)
		}
		if item.RevisionAfter == nil || *item.RevisionAfter != result.ToRevision+1 {
			return nil, newTheaterError(TheaterErrorHistoryExpired, "Theater event history 不连续", 410, nil)
		}
		result.Events = append(result.Events, TheaterEvent{MutationID: item.MutationID, RevisionBefore: item.RevisionBefore, Revision: *item.RevisionAfter, Type: item.Type, Payload: normalizedRawJSON(item.PayloadJSON, `{}`), CreatedAt: item.CreatedAt})
		result.ToRevision = *item.RevisionAfter
	}
	return result, nil
}

func isTheaterManagementMutation(mutationType string) bool {
	return mutationType == TheaterMutationAdminRestore || mutationType == TheaterMutationAdminReplace || mutationType == TheaterMutationAdminPackageImport
}
