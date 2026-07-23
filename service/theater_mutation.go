package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/utils"
)

type theaterMutationAuthorization int

const (
	theaterMutationAuthorizationDirect theaterMutationAuthorization = iota
	theaterMutationAuthorizationAction
)

func ApplyTheaterMutation(ctx context.Context, actorID string, command TheaterMutationCommand, meta TheaterRequestMeta) (*TheaterMutationResult, error) {
	return applyTheaterMutation(ctx, actorID, command, meta, theaterMutationAuthorizationDirect)
}

func applyTheaterActionMutation(ctx context.Context, actorID string, command TheaterMutationCommand, meta TheaterRequestMeta) (*TheaterMutationResult, error) {
	return applyTheaterMutation(ctx, actorID, command, meta, theaterMutationAuthorizationAction)
}

func applyTheaterMutation(ctx context.Context, actorID string, command TheaterMutationCommand, meta TheaterRequestMeta, authorization theaterMutationAuthorization) (*TheaterMutationResult, error) {
	startedAt := time.Now()
	defer func() {
		RecordTheaterMetric("theater_mutation_latency_ms", map[string]string{"type": command.Type}, float64(time.Since(startedAt).Milliseconds()))
	}()
	command.MutationID = strings.TrimSpace(command.MutationID)
	command.Type = strings.TrimSpace(command.Type)
	if command.MutationID == "" || len(command.MutationID) > 128 {
		return nil, theaterPayloadError("mutationId 无效")
	}
	permission := theaterPermissionForMutation(command.Type)
	if authorization == theaterMutationAuthorizationAction {
		if command.Type != TheaterMutationSceneApply && command.Type != TheaterMutationObjectToggle && command.Type != TheaterMutationObjectBatchUpdate {
			return nil, newTheaterError(TheaterErrorMutationTypeUnsupported, "动作不支持此 mutation type", 400, map[string]any{"type": command.Type})
		}
		permission = TheaterPermissionActionTrigger
	}
	if permission == "" {
		return nil, newTheaterError(TheaterErrorMutationTypeUnsupported, "不支持 mutation type", 400, map[string]any{"type": command.Type})
	}
	decoded, normalizedPayload, err := decodeTheaterPayload(command.Type, command.Payload)
	if err != nil {
		return nil, err
	}
	_, channel, err := requireTheaterPermission(actorID, command.WorldID, command.ChannelID, permission)
	delegatedObjectEdit := false
	if err != nil && authorization == theaterMutationAuthorizationDirect && command.Type == TheaterMutationObjectUpdate {
		if _, delegatedChannel, delegatedErr := requireTheaterPermission(actorID, command.WorldID, command.ChannelID, TheaterPermissionObjectEditDelegated); delegatedErr == nil {
			channel = delegatedChannel
			delegatedObjectEdit = true
			err = nil
		}
	}
	if err != nil {
		RecordTheaterMetric("theater_permission_denied_total", map[string]string{"permission": permission}, 1)
		return nil, err
	}
	if channel != nil && channel.Status != "" && channel.Status != model.ChannelStatusActive {
		return nil, newTheaterError(TheaterErrorPermissionDenied, "归档频道不可写 Theater", 403, nil)
	}
	payloadHash := theaterJSONHash(normalizedPayload)
	room, err := model.TheaterRoomCreateIfMissing(command.WorldID, command.ChannelID, actorID)
	if err != nil {
		return nil, err
	}

	var result *TheaterMutationResult
	var outcomeErr error
	createdMutation := false
	err = model.GetDB().Transaction(func(tx *gorm.DB) error {
		var existing model.TheaterMutationModel
		findErr := tx.Where("room_id = ? AND mutation_id = ?", room.ID, command.MutationID).Limit(1).Find(&existing).Error
		if findErr != nil {
			return findErr
		}
		if existing.ID != "" {
			if existing.ActorUserID != actorID || existing.Type != command.Type || existing.PayloadHash != payloadHash {
				outcomeErr = newTheaterError(TheaterErrorMutationIDReused, "mutationId 已用于不同请求", 409, nil)
				return nil
			}
			if existing.Status == "rejected" {
				outcomeErr = theaterErrorFromRejectedMutation(existing)
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
			if err := persistRejectedTheaterMutation(tx, &current, actorID, command, normalizedPayload, payloadHash, meta, outcomeErr.(*TheaterError)); err != nil {
				return err
			}
			createdMutation = true
			return nil
		}
		if delegatedObjectEdit {
			if err := validateDelegatedTheaterObjectUpdate(tx, current.ID, decoded.(*theaterObjectUpdatePayload)); err != nil {
				return err
			}
		}
		nextRevision := current.Revision + 1
		cas := tx.Model(&model.TheaterRoomModel{}).Where("id = ? AND revision = ?", current.ID, current.Revision).
			Updates(map[string]any{"revision": nextRevision, "updated_by": actorID, "updated_at": time.Now()})
		if cas.Error != nil {
			return cas.Error
		}
		if cas.RowsAffected != 1 {
			return errTheaterConcurrentCAS
		}
		current.Revision = nextRevision
		current.UpdatedBy = actorID
		if err := applyDecodedTheaterMutation(tx, &current, actorID, command.Type, decoded); err != nil {
			return err
		}
		if err := recalculateTheaterResourceReferences(tx, current.ID); err != nil {
			return err
		}
		snapshot, checksum, err := buildTheaterSnapshot(tx, &current, true)
		if err != nil {
			return err
		}
		result = &TheaterMutationResult{MutationID: command.MutationID, RevisionBefore: command.ExpectedRevision, Revision: nextRevision, Type: command.Type, Payload: normalizedPayload, Checksum: checksum}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return err
		}
		revisionAfter := nextRevision
		mutation := model.TheaterMutationModel{
			RoomID: current.ID, WorldID: command.WorldID, ChannelID: command.ChannelID, MutationID: command.MutationID,
			ActorUserID: actorID, ExpectedRevision: command.ExpectedRevision, RevisionBefore: command.ExpectedRevision, RevisionAfter: &revisionAfter,
			Type: command.Type, PayloadJSON: string(normalizedPayload), PayloadHash: payloadHash, ResultJSON: string(resultJSON), Status: "applied",
			RequestSource: normalizedRequestSource(meta.Source), RequestID: meta.RequestID, SessionID: meta.SessionID,
		}
		if err := tx.Create(&mutation).Error; err != nil {
			return err
		}
		createdMutation = true
		if err := createTheaterAudit(tx, &current, actorID, meta, command.MutationID, command.Type, "applied", "", "", command.ExpectedRevision, &revisionAfter, normalizedPayload); err != nil {
			return err
		}
		if err := tx.Model(&model.TheaterRoomModel{}).Where("id = ?", current.ID).Updates(map[string]any{"state_hash": checksum}).Error; err != nil {
			return err
		}
		if nextRevision%100 == 0 {
			snapshotJSON, snapshotHash, err := canonicalTheaterJSON(snapshot)
			if err == nil {
				item := &model.TheaterSnapshotModel{RoomID: current.ID, Revision: nextRevision, SchemaVersion: current.SchemaVersion, SnapshotJSON: string(snapshotJSON), SnapshotHash: snapshotHash, SnapshotBytes: int64(len(snapshotJSON)), Kind: "automatic", CreatedBy: actorID}
				if err := tx.Create(item).Error; err != nil {
					return err
				}
				if err := createTheaterResourceHolds(tx, item, nil); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if errors.Is(err, errTheaterConcurrentCAS) {
		fresh, findErr := model.TheaterRoomFindByScope(command.WorldID, command.ChannelID)
		if findErr != nil {
			return nil, findErr
		}
		currentRevision := int64(0)
		if fresh != nil {
			currentRevision = fresh.Revision
		}
		conflict := newTheaterError(TheaterErrorRevisionConflict, "Theater revision 冲突", 409, map[string]any{"expectedRevision": command.ExpectedRevision, "currentRevision": currentRevision})
		RecordTheaterMetric("theater_revision_conflict_total", nil, 1)
		RecordTheaterMetric("theater_mutation_total", map[string]string{"type": command.Type, "outcome": "rejected"}, 1)
		persistErr := model.GetDB().Transaction(func(tx *gorm.DB) error {
			return persistRejectedTheaterMutation(tx, fresh, actorID, command, normalizedPayload, payloadHash, meta, conflict)
		})
		if persistErr == nil && fresh != nil {
			createdMutation = true
			_ = PublishTheaterMutationNow(ctx, fresh.ID, command.MutationID)
		}
		return nil, conflict
	}
	if err != nil {
		if theaterErr, ok := err.(*TheaterError); ok {
			return nil, theaterErr
		}
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
		RecordTheaterMetric("theater_mutation_total", map[string]string{"type": command.Type, "outcome": "rejected"}, 1)
		return nil, outcomeErr
	}
	if createdMutation {
		RecordTheaterMetric("theater_mutation_total", map[string]string{"type": command.Type, "outcome": "applied"}, 1)
	}
	return result, nil
}

var delegatedTheaterObjectFields = map[string]bool{
	"name": true, "x": true, "y": true, "width": true, "height": true,
	"rotation": true, "scale": true, "scaleX": true, "scaleY": true, "z": true, "orderKey": true, "content": true,
}

func validateDelegatedTheaterObjectUpdate(tx *gorm.DB, roomID string, payload *theaterObjectUpdatePayload) error {
	object, err := loadTheaterObject(tx, roomID, payload.ObjectID)
	if err != nil {
		return err
	}
	if !object.Editable || object.Locked {
		return newTheaterError(TheaterErrorPermissionDenied, "对象未开放成员编辑", 403, map[string]any{"permission": TheaterPermissionObjectEditDelegated})
	}
	for field := range payload.Fields {
		if !delegatedTheaterObjectFields[field] {
			return newTheaterError(TheaterErrorPermissionDenied, "成员不能修改对象管理字段", 403, map[string]any{"field": field})
		}
	}
	return nil
}

var errTheaterConcurrentCAS = errors.New("theater revision CAS conflict")

func normalizedRequestSource(value string) string {
	switch value {
	case "http", "websocket", "bridge", "admin":
		return value
	default:
		return "http"
	}
}

func theaterErrorFromRejectedMutation(mutation model.TheaterMutationModel) *TheaterError {
	status := 400
	if mutation.RejectCode == TheaterErrorRevisionConflict || mutation.RejectCode == TheaterErrorMutationIDReused {
		status = 409
	}
	return newTheaterError(mutation.RejectCode, mutation.RejectReason, status, map[string]any{"currentRevision": mutation.RevisionBefore})
}

func persistRejectedTheaterMutation(tx *gorm.DB, room *model.TheaterRoomModel, actorID string, command TheaterMutationCommand, payload json.RawMessage, payloadHash string, meta TheaterRequestMeta, theaterErr *TheaterError) error {
	if room == nil {
		return nil
	}
	resultJSON, _ := json.Marshal(map[string]any{"ok": false, "error": theaterErr})
	mutation := model.TheaterMutationModel{
		RoomID: room.ID, WorldID: command.WorldID, ChannelID: command.ChannelID, MutationID: command.MutationID,
		ActorUserID: actorID, ExpectedRevision: command.ExpectedRevision, RevisionBefore: room.Revision, Type: command.Type,
		PayloadJSON: string(payload), PayloadHash: payloadHash, ResultJSON: string(resultJSON), Status: "rejected", RejectCode: theaterErr.Code, RejectReason: theaterErr.Message,
		RequestSource: normalizedRequestSource(meta.Source), RequestID: meta.RequestID, SessionID: meta.SessionID,
	}
	if err := tx.Create(&mutation).Error; err != nil {
		return err
	}
	return createTheaterAudit(tx, room, actorID, meta, command.MutationID, command.Type, "rejected", theaterErr.Code, theaterErr.Message, room.Revision, nil, payload)
}

func createTheaterAudit(tx *gorm.DB, room *model.TheaterRoomModel, actorID string, meta TheaterRequestMeta, mutationID, mutationType, outcome, reasonCode, reasonMessage string, revisionBefore int64, revisionAfter *int64, payload json.RawMessage) error {
	remoteHash := sha256.Sum256([]byte(meta.RemoteIP))
	userAgentHash := sha256.Sum256([]byte(meta.UserAgent))
	summary := map[string]any{"payloadBytes": len(payload), "payloadHash": theaterJSONHash(payload)}
	summaryJSON, _ := json.Marshal(summary)
	return tx.Create(&model.TheaterAuditLogModel{
		RoomID: room.ID, WorldID: room.WorldID, ChannelID: room.ChannelID, ActorUserID: actorID, ActorNameSnapshot: meta.ActorName,
		MutationID: mutationID, RevisionBefore: revisionBefore, RevisionAfter: revisionAfter, MutationType: mutationType, Outcome: outcome,
		ReasonCode: reasonCode, ReasonMessage: reasonMessage, RequestSource: normalizedRequestSource(meta.Source), RequestID: meta.RequestID, SessionID: meta.SessionID,
		RemoteIPHash: hex.EncodeToString(remoteHash[:]), UserAgentHash: hex.EncodeToString(userAgentHash[:]), SummaryJSON: string(summaryJSON),
	}).Error
}

func applyDecodedTheaterMutation(tx *gorm.DB, room *model.TheaterRoomModel, actorID, mutationType string, decoded any) error {
	switch payload := decoded.(type) {
	case *theaterSceneCreatePayload:
		return applyTheaterSceneCreate(tx, room, actorID, payload)
	case *theaterSceneUpdatePayload:
		return applyTheaterSceneUpdate(tx, room, actorID, payload)
	case *theaterSceneDeletePayload:
		return applyTheaterSceneDelete(tx, room, payload)
	case *theaterSceneApplyPayload:
		return applyTheaterSceneApply(tx, room, payload)
	case *theaterObjectCreatePayload:
		return applyTheaterObjectCreate(tx, room, actorID, payload)
	case *theaterObjectUpdatePayload:
		if err := applyTheaterObjectUpdate(tx, room, actorID, payload); err != nil {
			return err
		}
		if _, scopeChanged := payload.Fields["sceneId"]; scopeChanged {
			return nil
		}
		return validateTheaterObjectHierarchy(tx, room.ID)
	case *theaterObjectBatchUpdatePayload:
		scopeChanged := false
		for i := range payload.Updates {
			if _, ok := payload.Updates[i].Fields["sceneId"]; ok {
				scopeChanged = true
			}
			if err := applyTheaterObjectUpdate(tx, room, actorID, &payload.Updates[i]); err != nil {
				return err
			}
		}
		if scopeChanged {
			return nil
		}
		return validateTheaterObjectHierarchy(tx, room.ID)
	case *theaterObjectDeletePayload:
		if err := applyTheaterObjectDelete(tx, room, payload); err != nil {
			return err
		}
		return validateTheaterObjectHierarchy(tx, room.ID)
	case *theaterObjectTogglePayload:
		return applyTheaterObjectToggle(tx, room, payload)
	case *theaterCharacterBindPayload:
		return applyTheaterCharacterBind(tx, room, actorID, payload)
	case *theaterResourceReferencePayload:
		return applyTheaterResourceReference(tx, room, mutationType == TheaterMutationResourceAttach, payload)
	default:
		return newTheaterError(TheaterErrorMutationTypeUnsupported, "mutation 未实现", 400, nil)
	}
}

func applyTheaterSceneCreate(tx *gorm.DB, room *model.TheaterRoomModel, actorID string, payload *theaterSceneCreatePayload) error {
	var count int64
	if err := tx.Model(&model.TheaterSceneModel{}).Where("room_id = ?", room.ID).Count(&count).Error; err != nil {
		return err
	}
	if count >= theaterMaxScenes {
		return newTheaterError(TheaterErrorLimitExceeded, "场景数量超限", 409, map[string]any{"limit": theaterMaxScenes})
	}
	var existing int64
	if err := tx.Model(&model.TheaterSceneModel{}).Where("id = ?", payload.SceneID).Count(&existing).Error; err != nil {
		return err
	}
	if existing > 0 {
		return theaterPayloadError("sceneId 已存在")
	}
	state, _ := json.Marshal(payload.State)
	scene := model.TheaterSceneModel{StringPKBaseModel: model.StringPKBaseModel{ID: payload.SceneID}, RoomID: room.ID, Name: strings.TrimSpace(payload.Name), SwitchText: payload.SwitchText, SortOrder: payload.Order, StateJSON: string(state), SchemaVersion: model.TheaterSchemaVersion, CreatedBy: actorID, UpdatedBy: actorID}
	if err := tx.Create(&scene).Error; err != nil {
		return err
	}
	if room.ActiveSceneID == "" {
		room.ActiveSceneID = scene.ID
		return tx.Model(&model.TheaterRoomModel{}).Where("id = ?", room.ID).Update("active_scene_id", scene.ID).Error
	}
	return nil
}

func loadTheaterScene(tx *gorm.DB, roomID, sceneID string) (*model.TheaterSceneModel, error) {
	var scene model.TheaterSceneModel
	if err := tx.Where("room_id = ? AND id = ?", roomID, sceneID).Limit(1).Find(&scene).Error; err != nil {
		return nil, err
	}
	if scene.ID == "" {
		return nil, newTheaterError(TheaterErrorNotFound, "场景不存在", 404, nil)
	}
	return &scene, nil
}

func applyTheaterSceneUpdate(tx *gorm.DB, room *model.TheaterRoomModel, actorID string, payload *theaterSceneUpdatePayload) error {
	scene, err := loadTheaterScene(tx, room.ID, payload.SceneID)
	if err != nil {
		return err
	}
	if scene.Locked && !pm.CanWithSystemRole(actorID, pm.PermModAdmin) && !IsWorldAdmin(room.WorldID, actorID) {
		return newTheaterError(TheaterErrorPermissionDenied, "场景已锁定", 403, nil)
	}
	updates := map[string]any{"updated_by": actorID, "updated_at": time.Now()}
	for key, value := range payload.Fields {
		switch key {
		case "name":
			updates["name"] = strings.TrimSpace(fmt.Sprint(value))
		case "switchText":
			updates["switch_text"] = fmt.Sprint(value)
		case "order":
			updates["sort_order"] = jsonNumberInt64(value)
		case "locked":
			updates["locked"] = value
		case "state":
			raw, _ := json.Marshal(value)
			updates["state_json"] = string(raw)
		}
	}
	return tx.Model(scene).Updates(updates).Error
}

func applyTheaterSceneDelete(tx *gorm.DB, room *model.TheaterRoomModel, payload *theaterSceneDeletePayload) error {
	if _, err := loadTheaterScene(tx, room.ID, payload.SceneID); err != nil {
		return err
	}
	var count int64
	if err := tx.Model(&model.TheaterSceneModel{}).Where("room_id = ?", room.ID).Count(&count).Error; err != nil {
		return err
	}
	if count <= 1 {
		return theaterPayloadError("至少保留一个场景")
	}
	if room.ActiveSceneID == payload.SceneID {
		if payload.FallbackSceneID == "" || payload.FallbackSceneID == payload.SceneID {
			return theaterPayloadError("删除当前场景必须指定 fallbackSceneId")
		}
		if _, err := loadTheaterScene(tx, room.ID, payload.FallbackSceneID); err != nil {
			return err
		}
		room.ActiveSceneID = payload.FallbackSceneID
		if err := tx.Model(&model.TheaterRoomModel{}).Where("id = ?", room.ID).Update("active_scene_id", payload.FallbackSceneID).Error; err != nil {
			return err
		}
	}
	objectIDs := tx.Model(&model.TheaterObjectModel{}).Select("id").Where("room_id = ? AND scene_id = ?", room.ID, payload.SceneID)
	if err := tx.Unscoped().Where("room_id = ? AND domain = ? AND target_id IN (?)", room.ID, TheaterPanelDomainEffect, objectIDs).Delete(&model.TheaterPanelItemModel{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("room_id = ? AND scene_id = ?", room.ID, payload.SceneID).Delete(&model.TheaterObjectModel{}).Error; err != nil {
		return err
	}
	return tx.Unscoped().Where("room_id = ? AND id = ?", room.ID, payload.SceneID).Delete(&model.TheaterSceneModel{}).Error
}

func applyTheaterSceneApply(tx *gorm.DB, room *model.TheaterRoomModel, payload *theaterSceneApplyPayload) error {
	if _, err := loadTheaterScene(tx, room.ID, payload.SceneID); err != nil {
		return err
	}
	room.ActiveSceneID = payload.SceneID
	return tx.Model(&model.TheaterRoomModel{}).Where("id = ?", room.ID).Update("active_scene_id", payload.SceneID).Error
}

func applyTheaterObjectCreate(tx *gorm.DB, room *model.TheaterRoomModel, actorID string, payload *theaterObjectCreatePayload) error {
	return createTheaterObject(tx, room, actorID, payload.SceneID, &payload.Object)
}

func createTheaterObject(tx *gorm.DB, room *model.TheaterRoomModel, actorID string, sceneID *string, input *theaterObjectInput) error {
	var total int64
	if err := tx.Model(&model.TheaterObjectModel{}).Where("room_id = ?", room.ID).Count(&total).Error; err != nil {
		return err
	}
	if total >= theaterMaxObjects {
		return newTheaterError(TheaterErrorLimitExceeded, "对象数量超限", 409, map[string]any{"limit": theaterMaxObjects})
	}
	sceneValue := ""
	if sceneID != nil && strings.TrimSpace(*sceneID) != "" {
		sceneValue = strings.TrimSpace(*sceneID)
		if _, err := loadTheaterScene(tx, room.ID, sceneValue); err != nil {
			return err
		}
		var sceneCount int64
		if err := tx.Model(&model.TheaterObjectModel{}).Where("room_id = ? AND scene_id = ?", room.ID, sceneValue).Count(&sceneCount).Error; err != nil {
			return err
		}
		if sceneCount >= theaterMaxSceneObjects {
			return newTheaterError(TheaterErrorLimitExceeded, "场景对象数量超限", 409, map[string]any{"limit": theaterMaxSceneObjects})
		}
	}
	if input.ParentID != nil && *input.ParentID != "" {
		parent, err := loadTheaterObject(tx, room.ID, *input.ParentID)
		if err != nil {
			return err
		}
		if parent.SceneID != sceneValue {
			return theaterPayloadError("parent 必须与对象处于同一范围")
		}
		if parent.Kind != "group" {
			return theaterPayloadError("parent 必须是组")
		}
	}
	if input.Kind == "group" {
		input.Interactive = false
		input.Editable = false
		input.Actions = json.RawMessage(`[]`)
	}
	visible := true
	if input.Visible != nil {
		visible = *input.Visible
	}
	scale := 1.0
	if input.Scale != nil {
		scale = *input.Scale
	}
	scaleX := scale
	if input.ScaleX != nil {
		scaleX = *input.ScaleX
	}
	scaleY := scale
	if input.ScaleY != nil {
		scaleY = *input.ScaleY
	}
	aspectRatioLocked := true
	if input.AspectRatioLocked != nil {
		aspectRatioLocked = *input.AspectRatioLocked
	}
	object := model.TheaterObjectModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: input.ID}, RoomID: room.ID, SceneID: sceneValue, ParentID: derefString(input.ParentID), Kind: input.Kind, Name: input.Name,
		X: input.X, Y: input.Y, Width: input.Width, Height: input.Height, Rotation: input.Rotation, Scale: scaleX, ScaleX: scaleX, ScaleY: scaleY, Z: input.Z, OrderKey: input.OrderKey,
		Visible: visible, Locked: input.Locked, AspectRatioLocked: aspectRatioLocked, Interactive: input.Interactive, Editable: input.Editable,
		OwnerUserID: derefString(input.OwnerUserID), CharacterIdentityID: derefString(input.CharacterIdentityID), ContentJSON: defaultJSON(input.Content, `{}`), ActionsJSON: defaultJSON(input.Actions, `[]`), MetadataJSON: defaultJSON(input.Metadata, `{}`),
		SchemaVersion: model.TheaterSchemaVersion, CreatedBy: actorID, UpdatedBy: actorID,
	}
	if err := tx.Select("*").Create(&object).Error; err != nil {
		return err
	}
	if !visible {
		return tx.Exec("UPDATE theater_objects SET visible = ? WHERE room_id = ? AND id = ?", false, room.ID, object.ID).Error
	}
	return nil
}

func loadTheaterObject(tx *gorm.DB, roomID, objectID string) (*model.TheaterObjectModel, error) {
	var object model.TheaterObjectModel
	if err := tx.Where("room_id = ? AND id = ?", roomID, objectID).Limit(1).Find(&object).Error; err != nil {
		return nil, err
	}
	if object.ID == "" {
		return nil, newTheaterError(TheaterErrorNotFound, "对象不存在", 404, nil)
	}
	return &object, nil
}

func applyTheaterObjectUpdate(tx *gorm.DB, room *model.TheaterRoomModel, actorID string, payload *theaterObjectUpdatePayload) error {
	object, err := loadTheaterObject(tx, room.ID, payload.ObjectID)
	if err != nil {
		return err
	}
	if object.Locked && !IsWorldAdmin(room.WorldID, actorID) && !pm.CanWithSystemRole(actorID, pm.PermModAdmin) {
		return newTheaterError(TheaterErrorPermissionDenied, "对象已锁定", 403, nil)
	}
	if object.Kind == "group" {
		if interactive, ok := payload.Fields["interactive"].(bool); ok && interactive {
			return theaterPayloadError("组不能设置为可交互")
		}
		if editable, ok := payload.Fields["editable"].(bool); ok && editable {
			return theaterPayloadError("组不能开放成员编辑")
		}
		if actions, ok := payload.Fields["actions"].([]any); ok && len(actions) > 0 {
			return theaterPayloadError("组不能设置点击动作")
		}
	}
	desiredSceneID := object.SceneID
	if value, ok := payload.Fields["sceneId"]; ok {
		if object.Kind != "group" {
			return theaterPayloadError("只有组可以自适应跨场景属性")
		}
		desiredSceneID = strings.TrimSpace(fmt.Sprint(value))
		if desiredSceneID != "" {
			if _, err := loadTheaterScene(tx, room.ID, desiredSceneID); err != nil {
				return err
			}
		}
	}
	updates := map[string]any{"updated_by": actorID, "updated_at": time.Now()}
	columnMap := map[string]string{"parentId": "parent_id", "name": "name", "x": "x", "y": "y", "width": "width", "height": "height", "rotation": "rotation", "z": "z", "orderKey": "order_key", "visible": "visible", "locked": "locked", "aspectRatioLocked": "aspect_ratio_locked", "interactive": "interactive", "editable": "editable"}
	for key, value := range payload.Fields {
		if key == "sceneId" {
			updates["scene_id"] = desiredSceneID
			continue
		}
		if key == "scale" {
			updates["scale"] = value
			updates["scale_x"] = value
			updates["scale_y"] = value
			continue
		}
		if key == "scaleX" {
			updates["scale"] = value
			updates["scale_x"] = value
			continue
		}
		if key == "scaleY" {
			updates["scale_y"] = value
			continue
		}
		if key == "parentId" {
			parentID := strings.TrimSpace(fmt.Sprint(value))
			if parentID == object.ID {
				return theaterPayloadError("parent 循环")
			}
			if parentID != "" {
				parent, err := loadTheaterObject(tx, room.ID, parentID)
				if err != nil {
					return err
				}
				if parent.SceneID != desiredSceneID || theaterObjectHasAncestor(tx, room.ID, parentID, object.ID) {
					return theaterPayloadError("parent 范围或循环无效")
				}
				if parent.Kind != "group" {
					return theaterPayloadError("parent 必须是组")
				}
			}
			updates["parent_id"] = parentID
			continue
		}
		if column, ok := columnMap[key]; ok {
			updates[column] = value
			continue
		}
		switch key {
		case "content":
			raw, _ := json.Marshal(value)
			if object.Kind == "effect" {
				if err := validateTheaterEffectContent(raw); err != nil {
					return err
				}
			}
			updates["content_json"] = string(raw)
		case "actions":
			raw, _ := json.Marshal(value)
			updates["actions_json"] = string(raw)
		case "metadata":
			raw, _ := json.Marshal(value)
			updates["metadata_json"] = string(raw)
		}
	}
	return tx.Model(object).Updates(updates).Error
}

func theaterObjectHasAncestor(tx *gorm.DB, roomID, startID, targetID string) bool {
	seen := map[string]bool{}
	current := startID
	for current != "" && !seen[current] {
		if current == targetID {
			return true
		}
		seen[current] = true
		var object model.TheaterObjectModel
		if err := tx.Select("id", "parent_id").Where("room_id = ? AND id = ?", roomID, current).Limit(1).Find(&object).Error; err != nil || object.ID == "" {
			return false
		}
		current = object.ParentID
	}
	return false
}

func validateTheaterObjectHierarchy(tx *gorm.DB, roomID string) error {
	var objects []model.TheaterObjectModel
	if err := tx.Select("id", "parent_id", "kind", "scene_id").Where("room_id = ?", roomID).Find(&objects).Error; err != nil {
		return err
	}
	byID := make(map[string]model.TheaterObjectModel, len(objects))
	for _, object := range objects {
		byID[object.ID] = object
	}
	for _, object := range objects {
		if object.ParentID == "" {
			continue
		}
		parent, ok := byID[object.ParentID]
		if !ok || parent.Kind != "group" || parent.SceneID != object.SceneID {
			return theaterPayloadError("组成员必须处于同一场景范围")
		}
	}
	return nil
}

func applyTheaterObjectDelete(tx *gorm.DB, room *model.TheaterRoomModel, payload *theaterObjectDeletePayload) error {
	if _, err := loadTheaterObject(tx, room.ID, payload.ObjectID); err != nil {
		return err
	}
	var children []model.TheaterObjectModel
	if err := tx.Where("room_id = ? AND parent_id = ?", room.ID, payload.ObjectID).Find(&children).Error; err != nil {
		return err
	}
	if len(children) > 0 && !payload.Cascade {
		return theaterPayloadError("对象存在子对象，必须显式 cascade")
	}
	ids := []string{payload.ObjectID}
	if payload.Cascade {
		for index := 0; index < len(ids); index++ {
			var childIDs []string
			if err := tx.Model(&model.TheaterObjectModel{}).Where("room_id = ? AND parent_id = ?", room.ID, ids[index]).Pluck("id", &childIDs).Error; err != nil {
				return err
			}
			ids = append(ids, childIDs...)
		}
	}
	if err := tx.Unscoped().Where("room_id = ? AND object_id IN ?", room.ID, ids).Delete(&model.TheaterGroupEditorStateModel{}).Error; err != nil {
		return err
	}
	if err := tx.Unscoped().Where("room_id = ? AND domain = ? AND target_id IN ?", room.ID, TheaterPanelDomainEffect, ids).Delete(&model.TheaterPanelItemModel{}).Error; err != nil {
		return err
	}
	return tx.Unscoped().Where("room_id = ? AND id IN ?", room.ID, ids).Delete(&model.TheaterObjectModel{}).Error
}

func applyTheaterObjectToggle(tx *gorm.DB, room *model.TheaterRoomModel, payload *theaterObjectTogglePayload) error {
	object, err := loadTheaterObject(tx, room.ID, payload.ObjectID)
	if err != nil {
		return err
	}
	visible := !object.Visible
	if payload.Visible != nil {
		visible = *payload.Visible
	}
	return tx.Model(object).Update("visible", visible).Error
}

func applyTheaterCharacterBind(tx *gorm.DB, room *model.TheaterRoomModel, actorID string, payload *theaterCharacterBindPayload) error {
	admin := IsWorldAdmin(room.WorldID, actorID) || pm.CanWithSystemRole(actorID, pm.PermModAdmin)
	if payload.OwnerUserID != actorID && !admin {
		return newTheaterError(TheaterErrorPermissionDenied, "不能绑定他人角色", 403, nil)
	}
	var identity model.ChannelIdentityModel
	if err := tx.Where("id = ? AND channel_id = ? AND user_id = ?", payload.IdentityID, room.ChannelID, payload.OwnerUserID).Limit(1).Find(&identity).Error; err != nil {
		return err
	}
	if identity.ID == "" {
		return theaterPayloadError("identity 不属于当前频道或用户")
	}
	payload.Object.Kind = "character"
	payload.Object.CharacterIdentityID = &payload.IdentityID
	payload.Object.OwnerUserID = &payload.OwnerUserID
	return createTheaterObject(tx, room, actorID, payload.SceneID, &payload.Object)
}

func applyTheaterResourceReference(tx *gorm.DB, room *model.TheaterRoomModel, attach bool, payload *theaterResourceReferencePayload) error {
	var resource model.TheaterResourceModel
	if err := tx.Where("room_id = ? AND id = ?", room.ID, payload.ResourceID).Limit(1).Find(&resource).Error; err != nil {
		return err
	}
	if resource.ID == "" {
		return newTheaterError(TheaterErrorResourceNotFound, "资源不存在", 404, nil)
	}
	if attach && resource.Status != "ready" {
		return newTheaterError(TheaterErrorResourceNotReady, "资源尚未 ready", 409, nil)
	}
	ref := map[string]any{"resourceId": resource.ID, "variant": "display"}
	if len(payload.Config) > 0 {
		var config any
		_ = json.Unmarshal(payload.Config, &config)
		ref["config"] = config
	}
	switch payload.TargetType {
	case "room":
		updated, err := updateTheaterResourceSlot(room.StateJSON, payload.Slot, attach, ref, resource.ID)
		if err != nil {
			return err
		}
		room.StateJSON = updated
		return tx.Model(&model.TheaterRoomModel{}).Where("id = ?", room.ID).Update("state_json", updated).Error
	case "scene":
		scene, err := loadTheaterScene(tx, room.ID, payload.TargetID)
		if err != nil {
			return err
		}
		updated, err := updateTheaterResourceSlot(scene.StateJSON, payload.Slot, attach, ref, resource.ID)
		if err != nil {
			return err
		}
		return tx.Model(scene).Update("state_json", updated).Error
	case "object":
		object, err := loadTheaterObject(tx, room.ID, payload.TargetID)
		if err != nil {
			return err
		}
		updated, err := updateTheaterResourceSlot(object.ContentJSON, payload.Slot, attach, ref, resource.ID)
		if err != nil {
			return err
		}
		return tx.Model(object).Update("content_json", updated).Error
	}
	return theaterPayloadError("targetType 无效")
}

func updateTheaterResourceSlot(raw, slot string, attach bool, ref map[string]any, resourceID string) (string, error) {
	value := map[string]any{}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return "", theaterPayloadError("目标 JSON 无效")
		}
	}
	resources, _ := value["resources"].(map[string]any)
	if resources == nil {
		resources = map[string]any{}
	}
	if attach {
		resources[slot] = ref
	} else {
		existing, ok := resources[slot].(map[string]any)
		if !ok || fmt.Sprint(existing["resourceId"]) != resourceID {
			return "", theaterPayloadError("资源引用不存在")
		}
		delete(resources, slot)
	}
	value["resources"] = resources
	normalized, _ := json.Marshal(value)
	return string(normalized), nil
}

func recalculateTheaterResourceReferences(tx *gorm.DB, roomID string) error {
	var room model.TheaterRoomModel
	if err := tx.Where("id = ?", roomID).First(&room).Error; err != nil {
		return err
	}
	var previouslyReferencedIDs []string
	if err := tx.Model(&model.TheaterResourceModel{}).
		Where("room_id = ? AND reference_count > 0", roomID).
		Pluck("id", &previouslyReferencedIDs).Error; err != nil {
		return err
	}
	counts := map[string]int64{}
	countResourceIDsInJSON(room.StateJSON, counts)
	var scenes []model.TheaterSceneModel
	if err := tx.Where("room_id = ?", roomID).Find(&scenes).Error; err != nil {
		return err
	}
	for _, scene := range scenes {
		countResourceIDsInJSON(scene.StateJSON, counts)
	}
	var objects []model.TheaterObjectModel
	if err := tx.Where("room_id = ?", roomID).Find(&objects).Error; err != nil {
		return err
	}
	for _, object := range objects {
		countResourceIDsInJSON(object.ContentJSON, counts)
	}
	if err := tx.Model(&model.TheaterResourceModel{}).Where("room_id = ?", roomID).Update("reference_count", 0).Error; err != nil {
		return err
	}
	for resourceID, count := range counts {
		if err := tx.Model(&model.TheaterResourceModel{}).Where("room_id = ? AND id = ?", roomID, resourceID).Update("reference_count", count).Error; err != nil {
			return err
		}
	}
	now := time.Now()
	if len(previouslyReferencedIDs) > 0 {
		if err := tx.Model(&model.TheaterResourceModel{}).
			Where("room_id = ? AND id IN ? AND reference_count = 0 AND status = ?", roomID, previouslyReferencedIDs, "ready").
			Updates(map[string]any{
				"status":             "deleting",
				"deleted_at":         &now,
				"cleanup_reason":     theaterResourceCleanupOrphan,
				"cleanup_after":      now.Add(theaterResourceDeleteGrace),
				"cleanup_attempts":   0,
				"cleanup_last_error": "",
			}).Error; err != nil {
			return err
		}
	}
	if err := tx.Model(&model.TheaterResourceModel{}).
		Where("room_id = ? AND reference_count > 0 AND status = ? AND cleanup_reason = ?", roomID, "deleting", theaterResourceCleanupOrphan).
		Updates(map[string]any{
			"status":             "ready",
			"deleted_at":         nil,
			"cleanup_reason":     "",
			"cleanup_after":      nil,
			"cleanup_attempts":   0,
			"cleanup_last_error": "",
		}).Error; err != nil {
		return err
	}
	var purgingReferences int64
	if err := tx.Model(&model.TheaterResourceModel{}).
		Where("room_id = ? AND reference_count > 0 AND status = ?", roomID, "purging").
		Count(&purgingReferences).Error; err != nil {
		return err
	}
	if purgingReferences > 0 {
		return newTheaterError(TheaterErrorResourceNotReady, "资源正在清理，无法重新引用", 409, nil)
	}
	return nil
}

func countResourceIDsInJSON(raw string, counts map[string]int64) {
	var value any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return
	}
	var walk func(any)
	walk = func(current any) {
		switch item := current.(type) {
		case map[string]any:
			if resourceID, ok := item["resourceId"].(string); ok && resourceID != "" {
				counts[resourceID]++
			}
			for _, child := range item {
				walk(child)
			}
		case []any:
			for _, child := range item {
				walk(child)
			}
		}
	}
	walk(value)
}

func jsonNumberInt64(value any) int64 {
	switch current := value.(type) {
	case json.Number:
		result, _ := current.Int64()
		return result
	case float64:
		return int64(current)
	case int64:
		return current
	default:
		return 0
	}
}

func defaultJSON(raw json.RawMessage, fallback string) string {
	if len(raw) == 0 {
		return fallback
	}
	return string(raw)
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func newTheaterInternalMutationID(prefix string) string {
	return prefix + "-" + utils.NewID()
}
