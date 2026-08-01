package service

import (
	"context"
	"strings"

	"sealchat/model"
)

type TheaterGroupEditorState struct {
	CollapsedGroupIDs []string `json:"collapsedGroupIds"`
}

func GetTheaterGroupEditorState(_ context.Context, actorID, worldID, channelID string) (*TheaterGroupEditorState, error) {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionObjectEdit); err != nil {
		return nil, err
	}
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return nil, err
	}
	objectIDs, err := model.TheaterGroupEditorCollapsedIDs(room.ID, actorID)
	if err != nil {
		return nil, err
	}
	return &TheaterGroupEditorState{CollapsedGroupIDs: objectIDs}, nil
}

func SetTheaterGroupEditorState(_ context.Context, actorID, worldID, channelID, objectID string, collapsed bool) error {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionObjectEdit); err != nil {
		return err
	}
	room, err := model.TheaterRoomCreateIfMissing(worldID, channelID, actorID)
	if err != nil {
		return err
	}
	objectID = strings.TrimSpace(objectID)
	if err := validateTheaterID(objectID, "objectId"); err != nil {
		return err
	}
	var object model.TheaterObjectModel
	if err := model.GetDB().Where("room_id = ? AND id = ?", room.ID, objectID).Limit(1).Find(&object).Error; err != nil {
		return err
	}
	if object.ID == "" || object.Kind != "group" {
		return newTheaterError(TheaterErrorNotFound, "组不存在", 404, nil)
	}
	return model.TheaterGroupEditorStateSet(room.ID, actorID, objectID, collapsed)
}
