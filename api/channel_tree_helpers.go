package api

import (
	"strings"

	"sealchat/model"
	"sealchat/pm"
)

func broadcastChannelTreeInvalidatedByRoleID(operator *model.UserModel, roleID, reason string) {
	broadcastChannelTreeInvalidatedByRoleIDs(operator, []string{roleID}, reason)
}

func broadcastChannelTreeInvalidatedByRoleIDs(operator *model.UserModel, roleIDs []string, reason string) {
	seenChannelIDs := make(map[string]struct{}, len(roleIDs))
	for _, roleID := range roleIDs {
		channelID := strings.TrimSpace(model.ExtractChIdFromRoleId(roleID))
		if channelID == "" {
			continue
		}
		if _, exists := seenChannelIDs[channelID]; exists {
			continue
		}
		seenChannelIDs[channelID] = struct{}{}
		broadcastChannelTreeInvalidatedByChannelID(operator, channelID, reason)
	}
}

func loadChannelWorldIDs(channelIDs []string) ([]string, error) {
	normalizedIDs := make([]string, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		channelID = strings.TrimSpace(channelID)
		if channelID == "" {
			continue
		}
		normalizedIDs = append(normalizedIDs, channelID)
	}
	if len(normalizedIDs) == 0 {
		return nil, nil
	}

	var worldIDs []string
	if err := model.GetDB().Model(&model.ChannelModel{}).
		Where("id IN ?", normalizedIDs).
		Pluck("world_id", &worldIDs).Error; err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(worldIDs))
	result := make([]string, 0, len(worldIDs))
	for _, worldID := range worldIDs {
		worldID = strings.TrimSpace(worldID)
		if worldID == "" {
			continue
		}
		if _, exists := seen[worldID]; exists {
			continue
		}
		seen[worldID] = struct{}{}
		result = append(result, worldID)
	}
	return result, nil
}

func channelRoleVisibilityPermissionsChanged(oldPerms, newPerms []string) bool {
	oldRead := containsPermissionID(oldPerms, pm.PermFuncChannelRead.ID())
	oldReadAll := containsPermissionID(oldPerms, pm.PermFuncChannelReadAll.ID())
	newRead := containsPermissionID(newPerms, pm.PermFuncChannelRead.ID())
	newReadAll := containsPermissionID(newPerms, pm.PermFuncChannelReadAll.ID())
	return oldRead != newRead || oldReadAll != newReadAll
}

func containsPermissionID(permissionIDs []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, permissionID := range permissionIDs {
		if strings.TrimSpace(permissionID) == target {
			return true
		}
	}
	return false
}
