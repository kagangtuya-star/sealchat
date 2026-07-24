package service

import (
	"errors"
	"strings"

	"github.com/mikespook/gorbac"

	"sealchat/model"
	"sealchat/pm"
)

func canObserverAccessTheaterScope(channelID, observerWorldID string) error {
	channelID = strings.TrimSpace(channelID)
	observerWorldID = strings.TrimSpace(observerWorldID)
	if channelID != "" {
		_, err := CanObserverAccessChannel(channelID, observerWorldID)
		return err
	}
	if observerWorldID == "" {
		return errors.New("OB 旁观范围无效")
	}
	world, err := GetWorldByID(observerWorldID)
	if err != nil {
		return err
	}
	if world == nil || strings.TrimSpace(world.ID) == "" || !strings.EqualFold(strings.TrimSpace(world.Status), "active") {
		return errors.New("世界不可访问")
	}
	return nil
}

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
	if worldID == "" {
		return nil, nil, newTheaterError(TheaterErrorChannelWorldMismatch, "worldId 必填", 400, nil)
	}
	world, err := GetWorldByID(worldID)
	if err != nil {
		return nil, nil, err
	}
	if world == nil || world.ID == "" {
		return nil, nil, newTheaterError(TheaterErrorWorldNotFound, "世界不存在", 404, nil)
	}
	if channelID == "" {
		return world, nil, nil
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
	admin := pm.CanWithSystemRole(actorID, pm.PermModAdmin) || world.OwnerID == actorID || IsWorldAdmin(worldID, actorID)
	if channelID == "" {
		if admin {
			return world, nil, nil
		}
		if !IsWorldMember(worldID, actorID) {
			return nil, nil, newTheaterError(TheaterErrorPermissionDenied, "没有 World Theater 权限", 403, map[string]any{"permission": permission})
		}
		switch permission {
		case TheaterPermissionView, TheaterPermissionObjectEditDelegated, TheaterPermissionActionTrigger:
			return world, nil, nil
		default:
			return nil, nil, newTheaterError(TheaterErrorPermissionDenied, "没有 World Theater 权限", 403, map[string]any{"permission": permission})
		}
	}
	if admin {
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

func CanReceiveFullTheaterState(actorID, worldID, channelID string) bool {
	for _, permission := range []string{TheaterPermissionObjectEdit, TheaterPermissionSceneSwitch, TheaterPermissionAdminRestore} {
		if _, _, err := requireTheaterPermission(actorID, worldID, channelID, permission); err == nil {
			return true
		}
	}
	return false
}

func theaterPermissionsAllowFullState(permissions []string) bool {
	for _, permission := range permissions {
		if permission == TheaterPermissionObjectEdit || permission == TheaterPermissionSceneSwitch || permission == TheaterPermissionAdminRestore {
			return true
		}
	}
	return false
}

func CanSwitchTheaterScene(actorID, worldID, channelID string) bool {
	_, _, err := requireTheaterPermission(actorID, worldID, channelID, TheaterPermissionSceneSwitch)
	return err == nil
}

func theaterPermissionForMutation(mutationType string) string {
	switch mutationType {
	case TheaterMutationSceneApply:
		return TheaterPermissionSceneSwitch
	case TheaterMutationCharacterBind, TheaterMutationCharacterUpdate:
		return TheaterPermissionCharacterEdit
	case TheaterMutationSceneCreate, TheaterMutationSceneUpdate, TheaterMutationSceneReorder, TheaterMutationSceneDelete,
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
	if channelID == "" {
		if !IsWorldMember(worldID, actorID) && !admin {
			return result
		}
		if admin {
			for _, name := range []string{TheaterPermissionView, TheaterPermissionSceneSwitch, TheaterPermissionObjectEdit, TheaterPermissionObjectEditDelegated, TheaterPermissionCharacterEdit, TheaterPermissionResourceUpload, TheaterPermissionResourceDelete, TheaterPermissionActionTrigger, TheaterPermissionAdminRestore} {
				result = append(result, name)
			}
		} else {
			result = append(result, TheaterPermissionView, TheaterPermissionObjectEditDelegated, TheaterPermissionActionTrigger)
		}
		return result
	}
	order := []string{TheaterPermissionView, TheaterPermissionSceneSwitch, TheaterPermissionObjectEdit, TheaterPermissionObjectEditDelegated, TheaterPermissionCharacterEdit, TheaterPermissionResourceUpload, TheaterPermissionResourceDelete, TheaterPermissionActionTrigger, TheaterPermissionAdminRestore}
	for _, name := range order {
		if admin || pm.CanWithChannelRole(actorID, channelID, theaterPermissionMap[name]) {
			result = append(result, name)
		}
	}
	return result
}
