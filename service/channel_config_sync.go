package service

import (
	"fmt"
	"strings"

	"github.com/mikespook/gorbac"
	"github.com/samber/lo"
	"gorm.io/gorm"

	"sealchat/model"
	"sealchat/pm"
)

type ChannelConfigSyncScope string

const (
	ChannelConfigSyncScopeRoles ChannelConfigSyncScope = "roles"
)

type ChannelConfigSyncOptions struct {
	UserID           string
	SourceChannelID  string
	TargetChannelIDs []string
	Scopes           []ChannelConfigSyncScope
}

type ChannelConfigSyncResult struct {
	Source  string                    `json:"source"`
	Targets []ChannelConfigSyncTarget `json:"targets"`
}

type ChannelConfigSyncTarget struct {
	ChannelID string   `json:"channelId"`
	Scopes    []string `json:"scopes"`
	Error     string   `json:"error,omitempty"`
}

func ChannelConfigSync(opts *ChannelConfigSyncOptions) (*ChannelConfigSyncResult, error) {
	if opts == nil {
		return nil, fmt.Errorf("参数错误")
	}
	if strings.TrimSpace(opts.SourceChannelID) == "" {
		return nil, fmt.Errorf("缺少主频道")
	}
	if len(opts.TargetChannelIDs) == 0 {
		return nil, fmt.Errorf("请选择目标频道")
	}
	scopes := opts.Scopes
	if len(scopes) == 0 {
		scopes = []ChannelConfigSyncScope{ChannelConfigSyncScopeRoles}
	}
	scopes = lo.Uniq(scopes)
	if !pm.CanWithChannelRole(opts.UserID, opts.SourceChannelID, pm.PermFuncChannelManageRole) {
		return nil, fmt.Errorf("无权同步主频道配置")
	}
	sourceChannel, err := model.ChannelGet(opts.SourceChannelID)
	if err != nil {
		return nil, err
	}
	if sourceChannel.ID == "" {
		return nil, fmt.Errorf("主频道不存在")
	}

	targets := lo.Filter(opts.TargetChannelIDs, func(id string, _ int) bool {
		return strings.TrimSpace(id) != "" && id != opts.SourceChannelID
	})
	targets = lo.Uniq(targets)
	if len(targets) == 0 {
		return nil, fmt.Errorf("没有有效的目标频道")
	}

	var resultTargets []ChannelConfigSyncTarget
	for _, targetID := range targets {
		target := ChannelConfigSyncTarget{ChannelID: targetID}
		if !pm.CanWithChannelRole(opts.UserID, targetID, pm.PermFuncChannelManageRole) {
			target.Error = "没有权限同步该频道"
			resultTargets = append(resultTargets, target)
			continue
		}
		var applied []string
		for _, scope := range scopes {
			switch scope {
			case ChannelConfigSyncScopeRoles:
				if err := syncChannelRoles(opts.SourceChannelID, targetID); err != nil {
					target.Error = err.Error()
				} else {
					applied = append(applied, string(scope))
				}
			default:
				target.Error = fmt.Sprintf("未知的同步范围: %s", scope)
			}
			if target.Error != "" {
				break
			}
		}
		if target.Error == "" {
			target.Scopes = applied
		}
		resultTargets = append(resultTargets, target)
	}

	return &ChannelConfigSyncResult{
		Source:  opts.SourceChannelID,
		Targets: resultTargets,
	}, nil
}

func syncChannelRoles(sourceChannelID, targetChannelID string) error {
	roles, err := model.ChannelRoleListByChannelID(sourceChannelID)
	if err != nil {
		return err
	}
	db := model.GetDB()
	return db.Transaction(func(tx *gorm.DB) error {
		for _, role := range roles {
			key := deriveRoleKey(role.ID, sourceChannelID)
			if key == "" {
				key = strings.ToLower(strings.ReplaceAll(role.Name, " ", "-"))
			}
			targetRoleID := fmt.Sprintf("ch-%s-%s", targetChannelID, key)
			values := map[string]any{
				"name":       role.Name,
				"desc":       role.Desc,
				"channel_id": targetChannelID,
			}
			var exists model.ChannelRoleModel
			if err := tx.Where("id = ?", targetRoleID).First(&exists).Error; err != nil {
				if err := tx.Create(&model.ChannelRoleModel{
					StringPKBaseModel: model.StringPKBaseModel{ID: targetRoleID},
					Name:              role.Name,
					Desc:              role.Desc,
					ChannelID:         targetChannelID,
				}).Error; err != nil {
					return err
				}
			} else {
				if err := tx.Model(&model.ChannelRoleModel{}).
					Where("id = ?", targetRoleID).
					Updates(values).Error; err != nil {
					return err
				}
			}
			sourcePerms, err := model.RolePermissionList(role.ID)
			if err != nil {
				return err
			}
			if err := tx.Where("role_id = ?", targetRoleID).Delete(&model.RolePermissionModel{}).Error; err != nil {
				return err
			}
			if len(sourcePerms) > 0 {
				var perms []model.RolePermissionModel
				for _, p := range sourcePerms {
					perms = append(perms, model.RolePermissionModel{
						RoleID:       targetRoleID,
						PermissionID: p,
					})
				}
				if err := tx.Create(&perms).Error; err != nil {
					return err
				}
			}
			gorbacPerms := make([]gorbac.Permission, 0, len(sourcePerms))
			for _, pid := range sourcePerms {
				gorbacPerms = append(gorbacPerms, gorbac.NewStdPermission(pid))
			}
			pm.ChannelRoleSetWithoutDB(targetRoleID, gorbacPerms)
		}
		return nil
	})
}

func deriveRoleKey(roleID, channelID string) string {
	prefix := fmt.Sprintf("ch-%s-", channelID)
	if strings.HasPrefix(roleID, prefix) {
		return strings.TrimPrefix(roleID, prefix)
	}
	if idx := strings.LastIndex(roleID, "-"); idx > -1 && idx < len(roleID)-1 {
		return roleID[idx+1:]
	}
	return roleID
}
