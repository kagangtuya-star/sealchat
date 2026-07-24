package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"sealchat/model"
	"sealchat/pm"
	"sealchat/utils"
)

func initWorldTheaterServiceTest(t *testing.T) (string, string, string) {
	t.Helper()
	model.DBInit(&utils.AppConfig{
		DSN: fmt.Sprintf("file:service-world-theater-%s?mode=memory&cache=shared", utils.NewID()),
		SQLite: utils.SQLiteConfig{
			EnableWAL:       false,
			TxLockImmediate: true,
			ReadConnections: 1,
			OptimizeOnInit:  false,
		},
	})
	pm.Init()
	actorID := "owner-" + utils.NewIDWithLength(8)
	worldID := "world-" + utils.NewIDWithLength(8)
	channelID := "channel-" + utils.NewIDWithLength(8)
	if err := model.GetDB().Create(&model.WorldModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: worldID},
		Name:              "World Theater", OwnerID: actorID, InviteSlug: utils.NewIDWithLength(12), Status: "active",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.GetDB().Create(&model.WorldMemberModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
		WorldID:           worldID, UserID: actorID, Role: model.WorldRoleOwner, JoinedAt: time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.GetDB().Create(&model.ChannelModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: channelID},
		WorldID:           worldID, Name: "Stage", Status: model.ChannelStatusActive,
	}).Error; err != nil {
		t.Fatal(err)
	}
	return actorID, worldID, channelID
}

func worldTheaterPayload(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func validTheaterEffectContent(t *testing.T) json.RawMessage {
	t.Helper()
	return worldTheaterPayload(t, map[string]any{
		"effect": map[string]any{
			"version": 1, "kind": "builtin", "keywords": []string{"爆击"}, "targetActorName": "法师",
			"durationMs": 3500, "cooldownMs": 0, "media": nil,
			"audio": map[string]any{"assetId": "audio-1", "name": "世界-特性音频-爆击", "volume": 0.8},
			"builtin": map[string]any{
				"theme": "brush", "format": "popout", "text": "CRITICAL HIT", "subText": "",
				"accentColor": "#e61c34", "mainTextColor": "#ffffff", "subTextColor": "#000000",
				"dimIntensity": 70, "shakeIntensity": 0,
				"mediaTransform": map[string]any{"x": 0, "y": 0, "scale": 1, "rotation": 0, "mirror": false},
			},
		},
	})
}

func TestValidateTheaterEffectContent(t *testing.T) {
	if err := validateTheaterEffectContent(validTheaterEffectContent(t)); err != nil {
		t.Fatalf("valid effect rejected: %v", err)
	}
	var value map[string]any
	if err := json.Unmarshal(validTheaterEffectContent(t), &value); err != nil {
		t.Fatal(err)
	}
	effect := value["effect"].(map[string]any)
	effect["builtin"].(map[string]any)["theme"] = "missing-assets"
	if err := validateTheaterEffectContent(worldTheaterPayload(t, value)); err == nil {
		t.Fatal("unknown effect theme accepted")
	}
	effect["builtin"].(map[string]any)["theme"] = "brush"
	effect["audio"].(map[string]any)["volume"] = 2
	if err := validateTheaterEffectContent(worldTheaterPayload(t, value)); err == nil {
		t.Fatal("invalid effect audio volume accepted")
	}
}

func TestTheaterAudioAssetName(t *testing.T) {
	if got := theaterAudioAssetName("迷雾世界", "", "thunder.mp3"); got != "迷雾世界-特性音频-thunder" {
		t.Fatalf("unexpected theater audio name: %q", got)
	}
	if got := theaterChannelAudioTag(" channel-1 "); got != "theater-channel:channel-1" {
		t.Fatalf("unexpected theater channel tag: %q", got)
	}
}

func worldTheaterRoom(t *testing.T, worldID, channelID string) *model.TheaterRoomModel {
	t.Helper()
	room, err := model.TheaterRoomFindByScope(worldID, channelID)
	if err != nil || room == nil {
		t.Fatalf("theater room: %#v, %v", room, err)
	}
	return room
}

func TestTheaterGroupEditorStatePersistsPerUser(t *testing.T) {
	actorID, worldID, channelID := initWorldTheaterServiceTest(t)
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		t.Fatal(err)
	}
	groupID := "group-" + utils.NewIDWithLength(8)
	if err := model.GetDB().Create(&model.TheaterObjectModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: groupID},
		RoomID:            room.ID, Kind: "group", Name: "Group", Scale: 1, ScaleX: 1, ScaleY: 1,
		Visible: true, AspectRatioLocked: true, ContentJSON: `{}`, ActionsJSON: `[]`, MetadataJSON: `{}`,
		SchemaVersion: model.TheaterSchemaVersion,
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := SetTheaterGroupEditorState(context.Background(), actorID, worldID, channelID, groupID, true); err != nil {
		t.Fatal(err)
	}
	state, err := GetTheaterGroupEditorState(context.Background(), actorID, worldID, channelID)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.CollapsedGroupIDs) != 1 || state.CollapsedGroupIDs[0] != groupID {
		t.Fatalf("unexpected collapsed groups: %#v", state.CollapsedGroupIDs)
	}
	otherIDs, err := model.TheaterGroupEditorCollapsedIDs(room.ID, "other-user")
	if err != nil {
		t.Fatal(err)
	}
	if len(otherIDs) != 0 {
		t.Fatalf("editor state leaked between users: %#v", otherIDs)
	}
	if err := SetTheaterGroupEditorState(context.Background(), actorID, worldID, channelID, groupID, false); err != nil {
		t.Fatal(err)
	}
	state, err = GetTheaterGroupEditorState(context.Background(), actorID, worldID, channelID)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.CollapsedGroupIDs) != 0 {
		t.Fatalf("expanded group remained persisted: %#v", state.CollapsedGroupIDs)
	}
}

func TestCreateTheaterGroupClearsComponentCapabilities(t *testing.T) {
	actorID, worldID, channelID := initWorldTheaterServiceTest(t)
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		t.Fatal(err)
	}
	scale := 1.0
	input := theaterObjectInput{
		ID: "group-" + utils.NewIDWithLength(8), Kind: "group", Name: "Group",
		Width: 12, Height: 8, ScaleX: &scale, ScaleY: &scale, OrderKey: "1",
		Interactive: true, Editable: true, Content: json.RawMessage(`{}`),
		Actions:  json.RawMessage(`[{"id":"action-1","type":"chat.send","payload":{"content":"x"}}]`),
		Metadata: json.RawMessage(`{}`),
	}
	if err := createTheaterObject(model.GetDB(), room, actorID, nil, &input); err != nil {
		t.Fatal(err)
	}
	var stored model.TheaterObjectModel
	if err := model.GetDB().Where("room_id = ? AND id = ?", room.ID, input.ID).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Interactive || stored.Editable || stored.ActionsJSON != "[]" {
		t.Fatalf("group capabilities not cleared: interactive=%v editable=%v actions=%s", stored.Interactive, stored.Editable, stored.ActionsJSON)
	}
	if err := applyTheaterObjectUpdate(model.GetDB(), room, actorID, &theaterObjectUpdatePayload{
		ObjectID: input.ID,
		Fields:   map[string]any{"editable": true},
	}); err == nil {
		t.Fatal("group accepted delegated editing")
	}
}

func TestTheaterGroupSceneScopeAdaptsWithoutChangingMembers(t *testing.T) {
	actorID, worldID, channelID := initWorldTheaterServiceTest(t)
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		t.Fatal(err)
	}
	sceneID := "scene-" + utils.NewIDWithLength(8)
	if err := model.GetDB().Create(&model.TheaterSceneModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: sceneID}, RoomID: room.ID,
		Name: "Scene", StateJSON: `{}`, SchemaVersion: model.TheaterSchemaVersion,
		CreatedBy: actorID, UpdatedBy: actorID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	groupID := "group-" + utils.NewIDWithLength(8)
	memberID := "member-" + utils.NewIDWithLength(8)
	objects := []model.TheaterObjectModel{
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: groupID}, RoomID: room.ID, SceneID: sceneID,
			Kind: "group", Name: "Group", Scale: 1, ScaleX: 1, ScaleY: 1, Visible: true,
			AspectRatioLocked: true, ContentJSON: `{}`, ActionsJSON: `[]`, MetadataJSON: `{}`,
			SchemaVersion: model.TheaterSchemaVersion,
		},
		{
			StringPKBaseModel: model.StringPKBaseModel{ID: memberID}, RoomID: room.ID,
			Kind: "image", Name: "Fixed", Scale: 1, ScaleX: 1, ScaleY: 1, Visible: true,
			AspectRatioLocked: true, ContentJSON: `{}`, ActionsJSON: `[]`, MetadataJSON: `{}`,
			SchemaVersion: model.TheaterSchemaVersion,
		},
	}
	if err := model.GetDB().Create(&objects).Error; err != nil {
		t.Fatal(err)
	}
	if err := applyTheaterObjectUpdate(model.GetDB(), room, actorID, &theaterObjectUpdatePayload{
		ObjectID: groupID, Fields: map[string]any{"sceneId": ""},
	}); err != nil {
		t.Fatal(err)
	}
	if err := applyTheaterObjectUpdate(model.GetDB(), room, actorID, &theaterObjectUpdatePayload{
		ObjectID: memberID, Fields: map[string]any{"parentId": groupID},
	}); err != nil {
		t.Fatal(err)
	}
	if err := validateTheaterObjectHierarchy(model.GetDB(), room.ID); err != nil {
		t.Fatal(err)
	}
	var storedGroup, storedMember model.TheaterObjectModel
	if err := model.GetDB().Where("id = ?", groupID).First(&storedGroup).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.GetDB().Where("id = ?", memberID).First(&storedMember).Error; err != nil {
		t.Fatal(err)
	}
	if storedGroup.SceneID != "" || storedMember.SceneID != "" || storedMember.ParentID != groupID {
		t.Fatalf("adaptive group scope mismatch: group=%q member=%q parent=%q", storedGroup.SceneID, storedMember.SceneID, storedMember.ParentID)
	}
	if err := applyDecodedTheaterMutation(model.GetDB(), room, actorID, TheaterMutationObjectBatchUpdate, &theaterObjectBatchUpdatePayload{
		Updates: []theaterObjectUpdatePayload{
			{ObjectID: memberID, Fields: map[string]any{"parentId": ""}},
			{ObjectID: groupID, Fields: map[string]any{"sceneId": sceneID}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := model.GetDB().Where("id = ?", groupID).First(&storedGroup).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.GetDB().Where("id = ?", memberID).First(&storedMember).Error; err != nil {
		t.Fatal(err)
	}
	if storedGroup.SceneID != sceneID || storedMember.SceneID != "" || storedMember.ParentID != "" {
		t.Fatalf("detached group scope mismatch: group=%q member=%q parent=%q", storedGroup.SceneID, storedMember.SceneID, storedMember.ParentID)
	}
	if err := applyTheaterObjectUpdate(model.GetDB(), room, actorID, &theaterObjectUpdatePayload{
		ObjectID: memberID, Fields: map[string]any{"sceneId": sceneID},
	}); err == nil {
		t.Fatal("component scene scope was changed by group adaptation")
	}
}

type worldTheaterChatSenderFunc func(context.Context, TheaterChatSendRequest) (*TheaterChatSendResult, error)

func (function worldTheaterChatSenderFunc) SendTheaterChat(ctx context.Context, request TheaterChatSendRequest) (*TheaterChatSendResult, error) {
	return function(ctx, request)
}

func TestWorldTheaterDrawingActionChatSendUsesInputChannel(t *testing.T) {
	testWorldTheaterActionChatSendUsesInputChannel(t, "drawing")
}

func TestWorldTheaterTextActionChatSendUsesInputChannel(t *testing.T) {
	testWorldTheaterActionChatSendUsesInputChannel(t, "text")
}

func testWorldTheaterActionChatSendUsesInputChannel(t *testing.T, kind string) {
	t.Helper()
	actorID, worldID, inputChannelID := initWorldTheaterServiceTest(t)
	if _, err := ApplyTheaterMutation(nil, actorID, TheaterMutationCommand{
		MutationID: "world-scene", WorldID: worldID, Type: TheaterMutationSceneCreate,
		Payload: worldTheaterPayload(t, map[string]any{"sceneId": "world-scene", "name": "World", "order": 1, "state": map[string]any{}}),
	}, TheaterRequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyTheaterMutation(nil, actorID, TheaterMutationCommand{
		MutationID: "world-object", WorldID: worldID, ExpectedRevision: 1, Type: TheaterMutationObjectCreate,
		Payload: worldTheaterPayload(t, map[string]any{"sceneId": "world-scene", "object": map[string]any{
			"id": "world-" + kind, "kind": kind, "name": "Send", "x": 0, "y": 0, "width": 10, "height": 10,
			"rotation": 0, "z": 0, "orderKey": "a", "visible": true, "interactive": true,
			"content": map[string]any{}, "metadata": map[string]any{},
			"actions": []map[string]any{{"id": "send", "type": "chat.send", "payload": map[string]any{"content": "World hello"}}},
		}}),
	}, TheaterRequestMeta{}); err != nil {
		t.Fatal(err)
	}
	var received TheaterChatSendRequest
	SetTheaterChatSender(worldTheaterChatSenderFunc(func(_ context.Context, request TheaterChatSendRequest) (*TheaterChatSendResult, error) {
		received = request
		return &TheaterChatSendResult{MessageID: "message"}, nil
	}))
	t.Cleanup(func() { SetTheaterChatSender(nil) })
	if _, err := TriggerTheaterAction(context.Background(), actorID, TheaterActionCommand{
		ActionRequestID: "world-action", WorldID: worldID, InputChannelID: inputChannelID,
		ObjectID: "world-" + kind, ActionID: "send", ExpectedRevision: 2,
	}, TheaterRequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if received.ChannelID != inputChannelID {
		t.Fatalf("chat channel = %q", received.ChannelID)
	}
}

func TestWorldTheaterMemberActionUsesComponentGrant(t *testing.T) {
	ownerID, worldID, _ := initWorldTheaterServiceTest(t)
	memberID := "member-" + utils.NewIDWithLength(8)
	if err := model.GetDB().Create(&model.WorldMemberModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: utils.NewID()},
		WorldID:           worldID, UserID: memberID, Role: model.WorldRoleMember, JoinedAt: time.Now(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	for index, sceneID := range []string{"scene-one", "scene-two"} {
		if _, err := ApplyTheaterMutation(nil, ownerID, TheaterMutationCommand{
			MutationID: fmt.Sprintf("scene-%d", index), WorldID: worldID, ExpectedRevision: int64(index), Type: TheaterMutationSceneCreate,
			Payload: worldTheaterPayload(t, map[string]any{"sceneId": sceneID, "name": sceneID, "order": index, "state": map[string]any{}}),
		}, TheaterRequestMeta{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ApplyTheaterMutation(nil, ownerID, TheaterMutationCommand{
		MutationID: "create-switch", WorldID: worldID, ExpectedRevision: 2, Type: TheaterMutationObjectCreate,
		Payload: worldTheaterPayload(t, map[string]any{
			"sceneId": "scene-one",
			"object": map[string]any{
				"id": "scene-switch", "kind": "button", "name": "Switch",
				"x": 0, "y": 0, "width": 10, "height": 10, "rotation": 0,
				"z": 0, "orderKey": "a", "visible": true, "interactive": true,
				"content": map[string]any{}, "metadata": map[string]any{},
				"actions": []map[string]any{{
					"id": "switch", "type": TheaterMutationSceneApply,
					"payload": map[string]any{"sceneId": "scene-two"},
				}},
			},
		}),
	}, TheaterRequestMeta{}); err != nil {
		t.Fatal(err)
	}

	if _, err := ApplyTheaterMutation(nil, memberID, TheaterMutationCommand{
		MutationID: "direct-switch", WorldID: worldID, ExpectedRevision: 3, Type: TheaterMutationSceneApply,
		Payload: worldTheaterPayload(t, map[string]any{"sceneId": "scene-two"}),
	}, TheaterRequestMeta{}); !IsTheaterErrorCode(err, TheaterErrorPermissionDenied) {
		t.Fatalf("direct scene switch error = %v", err)
	}
	if _, err := ApplyTheaterMutation(nil, memberID, TheaterMutationCommand{
		MutationID: "direct-toggle", WorldID: worldID, ExpectedRevision: 3, Type: TheaterMutationObjectToggle,
		Payload: worldTheaterPayload(t, map[string]any{"objectId": "scene-switch"}),
	}, TheaterRequestMeta{}); !IsTheaterErrorCode(err, TheaterErrorMutationTypeUnsupported) {
		t.Fatalf("direct object toggle error = %v", err)
	}

	result, err := TriggerTheaterAction(context.Background(), memberID, TheaterActionCommand{
		ActionRequestID: "member-switch", WorldID: worldID, ObjectID: "scene-switch", ActionID: "switch", ExpectedRevision: 3,
	}, TheaterRequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mutation == nil || result.Mutation.Revision != 4 {
		t.Fatalf("unexpected action result: %#v", result)
	}
	room := worldTheaterRoom(t, worldID, "")
	if room.ActiveSceneID != "scene-two" {
		t.Fatalf("active scene = %q", room.ActiveSceneID)
	}

	if _, err := ApplyTheaterMutation(nil, ownerID, TheaterMutationCommand{
		MutationID: "disable-switch", WorldID: worldID, ExpectedRevision: 4, Type: TheaterMutationObjectUpdate,
		Payload: worldTheaterPayload(t, map[string]any{"objectId": "scene-switch", "fields": map[string]any{"interactive": false}}),
	}, TheaterRequestMeta{}); err != nil {
		t.Fatal(err)
	}
	if _, err := TriggerTheaterAction(context.Background(), memberID, TheaterActionCommand{
		ActionRequestID: "member-switch-disabled", WorldID: worldID, ObjectID: "scene-switch", ActionID: "switch", ExpectedRevision: 5,
	}, TheaterRequestMeta{}); !IsTheaterErrorCode(err, TheaterErrorPermissionDenied) {
		t.Fatalf("disabled component action error = %v", err)
	}
}

func TestWorldTheaterSceneReorderIsAtomic(t *testing.T) {
	actorID, worldID, _ := initWorldTheaterServiceTest(t)
	for index, sceneID := range []string{"scene-one", "scene-two", "scene-three"} {
		if _, err := ApplyTheaterMutation(nil, actorID, TheaterMutationCommand{
			MutationID: fmt.Sprintf("scene-%d", index), WorldID: worldID, ExpectedRevision: int64(index), Type: TheaterMutationSceneCreate,
			Payload: worldTheaterPayload(t, map[string]any{"sceneId": sceneID, "name": sceneID, "order": index, "state": map[string]any{}}),
		}, TheaterRequestMeta{}); err != nil {
			t.Fatal(err)
		}
	}

	result, err := ApplyTheaterMutation(nil, actorID, TheaterMutationCommand{
		MutationID: "reorder-scenes", WorldID: worldID, ExpectedRevision: 3, Type: TheaterMutationSceneReorder,
		Payload: worldTheaterPayload(t, map[string]any{"sceneIds": []string{"scene-three", "scene-one", "scene-two"}}),
	}, TheaterRequestMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Revision != 4 {
		t.Fatalf("revision = %d", result.Revision)
	}

	var room model.TheaterRoomModel
	if err := model.GetDB().Where("world_id = ? AND channel_id = ?", worldID, "").First(&room).Error; err != nil {
		t.Fatal(err)
	}
	var scenes []model.TheaterSceneModel
	if err := model.GetDB().Where("room_id = ?", room.ID).Order("sort_order asc").Find(&scenes).Error; err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(scenes))
	for index, scene := range scenes {
		got[index] = scene.ID
	}
	want := []string{"scene-three", "scene-one", "scene-two"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("scene order = %v, want %v", got, want)
	}
}

func TestMergeTheaterRoomsToWorld(t *testing.T) {
	actorID, worldID, firstChannelID := initWorldTheaterServiceTest(t)
	secondChannelID := "channel-" + utils.NewIDWithLength(8)
	if err := model.GetDB().Create(&model.ChannelModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: secondChannelID},
		WorldID:           worldID,
		Name:              "Second Stage",
		Status:            model.ChannelStatusActive,
	}).Error; err != nil {
		t.Fatal(err)
	}

	createStage := func(channelID, sceneID, objectID string) *model.TheaterRoomModel {
		t.Helper()
		if _, err := ApplyTheaterMutation(nil, actorID, TheaterMutationCommand{
			MutationID: "scene-" + sceneID, WorldID: worldID, ChannelID: channelID,
			Type: TheaterMutationSceneCreate,
			Payload: worldTheaterPayload(t, map[string]any{
				"sceneId": sceneID, "name": sceneID, "order": 20, "state": map[string]any{},
			}),
		}, TheaterRequestMeta{}); err != nil {
			t.Fatal(err)
		}
		if _, err := ApplyTheaterMutation(nil, actorID, TheaterMutationCommand{
			MutationID: "object-" + objectID, WorldID: worldID, ChannelID: channelID,
			ExpectedRevision: 1, Type: TheaterMutationObjectCreate,
			Payload: worldTheaterPayload(t, map[string]any{
				"sceneId": sceneID,
				"object": map[string]any{
					"id": objectID, "kind": "button", "name": objectID,
					"x": 0, "y": 0, "width": 10, "height": 10, "rotation": 0,
					"z": 0, "orderKey": objectID, "visible": true, "interactive": true,
					"content": map[string]any{}, "metadata": map[string]any{},
					"actions": []map[string]any{{
						"id": "apply", "type": TheaterMutationSceneApply,
						"payload": map[string]any{"sceneId": sceneID},
					}},
				},
			}),
		}, TheaterRequestMeta{}); err != nil {
			t.Fatal(err)
		}
		return worldTheaterRoom(t, worldID, channelID)
	}

	firstRoom := createStage(firstChannelID, "scene-first", "object-first")
	secondRoom := createStage(secondChannelID, "scene-second", "object-second")
	attachment := model.AttachmentModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "attachment-" + utils.NewIDWithLength(8)},
		Filename:          "stage.png",
		MimeType:          "image/png",
		RootID:            firstRoom.ID,
		RootIDType:        "theater_resource",
	}
	if err := model.GetDB().Create(&attachment).Error; err != nil {
		t.Fatal(err)
	}
	readyResource := model.TheaterResourceModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "resource-ready-" + utils.NewIDWithLength(8)},
		RoomID:            firstRoom.ID, AttachmentID: attachment.ID, Kind: "static_image",
		MimeType: "image/png", Status: "ready", CreatedBy: actorID,
	}
	pendingResource := model.TheaterResourceModel{
		StringPKBaseModel: model.StringPKBaseModel{ID: "resource-pending-" + utils.NewIDWithLength(8)},
		RoomID:            secondRoom.ID, AttachmentID: attachment.ID, Kind: "static_image",
		MimeType: "image/png", Status: "pending", CreatedBy: actorID,
	}
	if err := model.GetDB().Create(&readyResource).Error; err != nil {
		t.Fatal(err)
	}
	if err := model.GetDB().Create(&pendingResource).Error; err != nil {
		t.Fatal(err)
	}

	if err := MergeTheaterRoomsToWorld(worldID); err != nil {
		t.Fatal(err)
	}
	worldRoom, err := model.TheaterRoomFindByWorld(worldID)
	if err != nil || worldRoom == nil {
		t.Fatalf("world room: %#v, %v", worldRoom, err)
	}
	if worldRoom.ScopeType != model.TheaterScopeWorld || worldRoom.ChannelID != "" || worldRoom.Revision != 0 {
		t.Fatalf("world room = %#v", worldRoom)
	}
	snapshot, err := GetTheaterSnapshot(nil, actorID, worldID, "", TheaterSnapshotOptions{IncludeResources: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Snapshot.Scenes) != 2 || len(snapshot.Snapshot.Resources) != 2 {
		t.Fatalf("merged snapshot = %#v", snapshot.Snapshot)
	}
	if snapshot.Snapshot.Scenes["scene-first"].Order == snapshot.Snapshot.Scenes["scene-second"].Order {
		t.Fatal("merged scene order must be unique")
	}
	for _, resourceID := range []string{readyResource.ID, pendingResource.ID} {
		var resource model.TheaterResourceModel
		if err := model.GetDB().Where("id = ?", resourceID).First(&resource).Error; err != nil {
			t.Fatal(err)
		}
		if resource.RoomID != worldRoom.ID {
			t.Fatalf("resource %s room = %s", resourceID, resource.RoomID)
		}
	}
	var movedAttachment model.AttachmentModel
	if err := model.GetDB().Where("id = ?", attachment.ID).First(&movedAttachment).Error; err != nil {
		t.Fatal(err)
	}
	if movedAttachment.RootID != worldRoom.ID {
		t.Fatalf("attachment root = %s", movedAttachment.RootID)
	}
	oldRooms, err := model.TheaterRoomListByWorld(worldID)
	if err != nil || len(oldRooms) != 0 {
		t.Fatalf("old rooms = %#v, %v", oldRooms, err)
	}

	if err := MergeTheaterRoomsToWorld(worldID); err != nil {
		t.Fatal(err)
	}
	again, err := model.TheaterRoomFindByWorld(worldID)
	if err != nil || again == nil || again.ID != worldRoom.ID {
		t.Fatalf("idempotent room = %#v, %v", again, err)
	}
}

func TestProjectTheaterSnapshotForMemberHidesSpoilers(t *testing.T) {
	activeSceneID := "scene-active"
	hiddenSceneID := "scene-hidden"
	visible := TheaterObjectSnapshot{
		ID: "visible", Kind: "button", Name: "幕后线索按钮", Visible: true, Interactive: true,
		Content: json.RawMessage(`{"image":{"resourceId":"resource-visible"}}`),
		Actions: json.RawMessage(`[
			{"id":"send","type":"chat.send","payload":{"content":"隐藏台词"}},
			{"id":"jump","type":"scene.apply","payload":{"sceneId":"scene-hidden"}}
		]`),
		Metadata: json.RawMessage(`{}`),
	}
	hidden := TheaterObjectSnapshot{
		ID: "hidden", Kind: "button", Name: "尚未公开的线索", Visible: false,
		Content: json.RawMessage(`{}`), Actions: json.RawMessage(`[]`), Metadata: json.RawMessage(`{}`),
	}
	delegated := TheaterObjectSnapshot{
		ID: "delegated", Kind: "text", Name: "授权编辑组件", Visible: false, Editable: true,
		Content: json.RawMessage(`{"text":"可编辑"}`), Actions: json.RawMessage(`[]`), Metadata: json.RawMessage(`{}`),
	}
	snapshot := TheaterSharedSnapshot{
		ActiveSceneID: &activeSceneID,
		LiveState:     json.RawMessage(`{}`),
		Scenes: map[string]TheaterSceneSnapshot{
			activeSceneID: {
				ID: activeSceneID, Name: "当前场景", SwitchText: "切换台词", State: json.RawMessage(`{}`),
				Objects: map[string]TheaterObjectSnapshot{visible.ID: visible, hidden.ID: hidden, delegated.ID: delegated},
			},
			hiddenSceneID: {
				ID: hiddenSceneID, Name: "最终真相", State: json.RawMessage(`{}`), Objects: map[string]TheaterObjectSnapshot{},
			},
		},
		PersistentObjects: map[string]TheaterObjectSnapshot{},
		Characters:        map[string]TheaterObjectSnapshot{"character-secret": hidden},
		Resources: map[string]TheaterResourcePublic{
			"resource-visible": {},
			"resource-hidden":  {},
		},
	}

	projected, checksum := projectTheaterSnapshotForMember(snapshot)
	if checksum == "" {
		t.Fatal("projected snapshot checksum must not be empty")
	}
	if len(projected.Scenes) != 1 {
		t.Fatalf("member must receive only active scene, got %d", len(projected.Scenes))
	}
	scene := projected.Scenes[activeSceneID]
	if scene.SwitchText != "" {
		t.Fatal("member snapshot must hide scene switch text")
	}
	if _, ok := scene.Objects[hidden.ID]; ok {
		t.Fatal("hidden uneditable object leaked")
	}
	if scene.Objects[visible.ID].Name != "组件" {
		t.Fatalf("visible uneditable object name leaked: %q", scene.Objects[visible.ID].Name)
	}
	if scene.Objects[delegated.ID].Name != delegated.Name {
		t.Fatal("delegated editable object name must remain available")
	}
	if len(projected.Characters) != 0 {
		t.Fatal("character management snapshot leaked")
	}
	if _, ok := projected.Resources["resource-visible"]; !ok {
		t.Fatal("referenced resource missing")
	}
	if _, ok := projected.Resources["resource-hidden"]; ok {
		t.Fatal("unreferenced resource leaked")
	}

	var actions []map[string]any
	if err := json.Unmarshal(scene.Objects[visible.ID].Actions, &actions); err != nil {
		t.Fatal(err)
	}
	if actions[0]["payload"].(map[string]any)["content"] != "redacted" {
		t.Fatal("chat action content leaked")
	}
	if actions[1]["payload"].(map[string]any)["sceneId"] != "redacted" {
		t.Fatal("scene action target leaked")
	}
}
