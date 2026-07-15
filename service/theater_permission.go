package service

import (
	"strings"

	"github.com/mikespook/gorbac"

	"sealchat/model"
	"sealchat/pm"
)

var theaterPermissionMap = map[string]gorbac.Permission{
	TheaterPermissionView:                pm.PermFuncChannelTheaterView,
	TheaterPermissionSceneSwitch:         pm.PermFuncChannelTheaterSceneSwitch,
	TheaterPermissionObjectEdit:          pm.PermFuncChannelTheaterObjectEdit,
	TheaterPermissionObjectEditDelegated: pm.PermFuncChannelTheaterObjectEditDelegated,
	TheaterPermissionCharacterEdit:       pm.PermFuncChannelTheaterCharacterEdit,
	TheaterPermissionResourceUpload:      pm.PermFuncChannelTheaterResourceUpload,
	TheaterPermissionResourceDelete:      pm.PermFuncChannelTheaterResourceDelete,
	TheaterPermissionActionTrigger:       pm.PermFuncChannelTheaterActionTrigger,
	TheaterPermissionAdminRestore:        pm.PermFuncChannelTheaterAdminRestore,
}

func resolveTheaterScope(worldID, channelID string) (*model.WorldModel, *model.ChannelModel, error) {
	worldID = strings.TrimSpace(worldID)
	channelID = strings.TrimSpace(channelID)
	if worldID == "" || channelID == "" {
		return nil, nil, newTheaterError(TheaterErrorChannelWorldMismatch, "worldId 和 channelId 必填", 400, nil)
	}
	world, err := GetWorldByID(worldID)
	if err != nil {
		return nil, nil, err
	}
	if world == nil || world.ID == "" {
		return nil, nil, newTheaterError(TheaterErrorWorldNotFound, "世界不存在", 404, nil)
	}
	channel, err := model.ChannelGet(channelID)
	if err != nil {
		return nil, nil, err
	}
	if channel == nil || channel.ID == "" {
		return nil, nil, newTheaterError(TheaterErrorChannelNotFound, "频道不存在", 404, nil)
	}
	if channel.WorldID != worldID {
		return nil, nil, newTheaterError(TheaterErrorChannelWorldMismatch, "频道不属于指定世界", 400, nil)
	}
	return world, channel, nil
}

func requireTheaterPermission(actorID, worldID, channelID, permission string) (*model.WorldModel, *model.ChannelModel, error) {
	if strings.TrimSpace(actorID) == "" {
		return nil, nil, newTheaterError(TheaterErrorAuthRequired, "需要登录", 401, nil)
	}
	world, channel, err := resolveTheaterScope(worldID, channelID)
	if err != nil {
		return nil, nil, err
	}
	if pm.CanWithSystemRole(actorID, pm.PermModAdmin) || world.OwnerID == actorID || IsWorldAdmin(worldID, actorID) {
		return world, channel, nil
	}
	permissionObject, ok := theaterPermissionMap[permission]
	if !ok || !pm.CanWithChannelRole(actorID, channelID, permissionObject) {
		return nil, nil, newTheaterError(TheaterErrorPermissionDenied, "没有 Theater 权限", 403, map[string]any{"permission": permission})
	}
	return world, channel, nil
}

func CanViewTheater(actorID, worldID, channelID string) bool {
	_, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionView)
	return err == nil
}

func CanManageTheaterResources(actorID, worldID, channelID string) bool {
	if _, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionResourceUpload); err == nil {
		return true
	}
	_, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionResourceDelete)
	return err == nil
}

func CanAdministerTheater(actorID, worldID, channelID string) bool {
	_, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionAdminRestore)
	return err == nil
}

func theaterPermissionForMutation(mutationType string) string {
	switch mutationType {
	case TheaterMutationSceneApply:
		return TheaterPermissionSceneSwitch
	case TheaterMutationCharacterBind, TheaterMutationCharacterUpdate:
		return TheaterPermissionCharacterEdit
	case TheaterMutationObjectToggle:
		return TheaterPermissionActionTrigger
	case TheaterMutationSceneCreate, TheaterMutationSceneUpdate, TheaterMutationSceneDelete,
		TheaterMutationObjectCreate, TheaterMutationObjectUpdate, TheaterMutationObjectBatchUpdate, TheaterMutationObjectDelete,
		TheaterMutationResourceAttach, TheaterMutationResourceDetach:
		return TheaterPermissionObjectEdit
	default:
		return ""
	}
}

func listTheaterPermissions(actorID, worldID, channelID string) []string {
	result := make([]string, 0, len(theaterPermissionMap))
	world, _, err := resolveTheaterScope(worldID, channelID)
	if err != nil {
		return result
	}
	admin := pm.CanWithSystemRole(actorID, pm.PermModAdmin) || world.OwnerID == actorID || IsWorldAdmin(worldID, actorID)
	order := []string{TheaterPermissionView, TheaterPermissionSceneSwitch, TheaterPermissionObjectEdit, TheaterPermissionObjectEditDelegated, TheaterPermissionCharacterEdit, TheaterPermissionResourceUpload, TheaterPermissionResourceDelete, TheaterPermissionActionTrigger, TheaterPermissionAdminRestore}
	for _, name := range order {
		if admin || pm.CanWithChannelRole(actorID, channelID, theaterPermissionMap[name]) {
			result = append(result, name)
		}
	}
	return result
}
