package model

import (
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"sealchat/utils"
)

// SystemRoleModel 系统角色表
type SystemRoleModel struct {
	StringPKBaseModel // ID: sys-xxx 如 sys-admin

	Name string `json:"name"` // 对外显示的名字
	Desc string `json:"desc"`
}

func (*SystemRoleModel) TableName() string {
	return "perm_system_roles"
}

// ChannelRoleModel 频道角色表
type ChannelRoleModel struct {
	StringPKBaseModel // ID: ch-{id}-xxx

	Name      string `json:"name"`
	Desc      string `json:"desc"`
	ChannelID string `json:"channelId"`
}

func (*ChannelRoleModel) TableName() string {
	return "perm_channel_roles"
}

type ChannelRoleOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

func ChannelRoleOptionListActive(channelID string) ([]*ChannelRoleOption, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return []*ChannelRoleOption{}, nil
	}
	var roleIDs []string
	err := db.Model(&MessageModel{}).
		Distinct("sender_role_id").
		Where("channel_id = ?", channelID).
		Where("sender_role_id IS NOT NULL AND sender_role_id <> ''").
		Pluck("sender_role_id", &roleIDs).Error
	if err != nil {
		return nil, err
	}
	if len(roleIDs) == 0 {
		return []*ChannelRoleOption{}, nil
	}
	roleSet := make(map[string]struct{}, len(roleIDs))
	for _, id := range roleIDs {
		if trimmed := strings.TrimSpace(id); trimmed != "" {
			roleSet[trimmed] = struct{}{}
		}
	}
	if len(roleSet) == 0 {
		return []*ChannelRoleOption{}, nil
	}
	var roles []ChannelRoleModel
	err = db.Where("channel_id = ?", channelID).
		Order("created_at ASC").
		Find(&roles).Error
	if err != nil {
		return nil, err
	}
	options := make([]*ChannelRoleOption, 0, len(roleSet))
	for _, role := range roles {
		if _, ok := roleSet[role.ID]; !ok {
			continue
		}
		label := strings.TrimSpace(role.Name)
		if label == "" {
			label = "未命名角色"
		}
		options = append(options, &ChannelRoleOption{
			ID:    role.ID,
			Label: label,
		})
	}
	return options, nil
}

// UserRoleMappingModel 定义用户-角色关系，即用户拥有什么角色
type UserRoleMappingModel struct {
	StringPKBaseModel
	RoleType string `json:"roleType" gorm:"index"` // 可以是 "channel" 或 "system"
	UserID   string `json:"userId" gorm:"index:idx_user_id;index:idx_user_role,unique"`
	RoleID   string `json:"roleId" gorm:"index:idx_user_role,unique"`

	User *UserModel `json:"user" gorm:"-"`
}

func (*UserRoleMappingModel) TableName() string {
	return "perm_user_role_mappings"
}

// RolePermissionModel 角色权限表
type RolePermissionModel struct {
	StringPKBaseModel
	RoleID       string `gorm:"index"`
	PermissionID string `gorm:"index"`
}

func (*RolePermissionModel) TableName() string {
	return "perm_role_permissions"
}

// SystemRoleCreate 创建系统角色
func SystemRoleCreate(role *SystemRoleModel) error {
	return db.Create(role).Error
}

// SystemRoleGet 获取系统角色
func SystemRoleGet(id string) (*SystemRoleModel, error) {
	var role SystemRoleModel
	err := db.First(&role, "id = ?", id).Error
	return &role, err
}

// SystemRoleUpdate 更新系统角色
func SystemRoleUpdate(role *SystemRoleModel) error {
	return db.Save(role).Error
}

// SystemRoleDelete 删除系统角色
func SystemRoleDelete(id string) error {
	return db.Delete(&SystemRoleModel{}, "id = ?", id).Error
}

// SystemRoleList 获取系统角色列表（带分页）
func SystemRoleList(page, pageSize int) ([]*SystemRoleModel, int64, error) {
	return utils.QueryPaginatedList(db, page, pageSize, &SystemRoleModel{}, nil)
}

// ChannelRoleCreate 创建频道角色
func ChannelRoleCreate(role *ChannelRoleModel) error {
	return db.Create(role).Error
}

// ChannelRoleGet 获取频道角色
func ChannelRoleGet(id string) (*ChannelRoleModel, error) {
	var role ChannelRoleModel
	err := db.Limit(1).Find(&role, "id = ?", id).Error
	return &role, err
}

// ChannelRoleDelete 删除频道角色
func ChannelRoleDelete(id string) error {
	return db.Delete(&ChannelRoleModel{}, "id = ?", id).Error
}

// ChannelRoleList 获取频道角色列表（带分页）
func ChannelRoleList(id string, page, pageSize int) ([]*ChannelRoleModel, int64, error) {
	return utils.QueryPaginatedList(db, page, pageSize, &ChannelRoleModel{}, func(q *gorm.DB) *gorm.DB {
		return q.Where("channel_id = ?", id)
	})
}

// ChannelRoleAllList 获取频道角色列表（带分页）
func ChannelRoleAllList(page, pageSize int) ([]*ChannelRoleModel, int64, error) {
	return utils.QueryPaginatedList(db, page, pageSize, &ChannelRoleModel{}, func(q *gorm.DB) *gorm.DB {
		return q
	})
}

// RolePermissionBatchDelete 批量删除角色权限
func RolePermissionBatchDelete(roleId string, permissions []string) error {
	return db.Unscoped().Delete(&RolePermissionModel{}, "role_id = ? and permission_id in ?", roleId, permissions).Error
}

// RolePermissionBatchCreate 批量创建角色权限
func RolePermissionBatchCreate(roleID string, permissionIDs []string) error {
	var rolePermissions []RolePermissionModel
	for _, permissionID := range permissionIDs {
		rolePermissions = append(rolePermissions, RolePermissionModel{
			StringPKBaseModel: StringPKBaseModel{ID: utils.NewID()},
			RoleID:            roleID,
			PermissionID:      permissionID,
		})
	}
	return db.Create(&rolePermissions).Error
}

// RolePermissionGet 获取角色权限
func RolePermissionGet(id string) (*RolePermissionModel, error) {
	var rolePermission RolePermissionModel
	err := db.First(&rolePermission, "id = ?", id).Error
	return &rolePermission, err
}

// RolePermissionDeleteById 删除角色权限
func RolePermissionDeleteById(id string) error {
	return db.Unscoped().Delete(&RolePermissionModel{}, "id = ?", id).Error
}

// RolePermissionList 根据RoleID获取PermissionID的集合
func RolePermissionList(roleID string) ([]string, error) {
	var permissionIDs []string
	err := db.Model(&RolePermissionModel{}).
		Where("role_id = ?", roleID).
		Pluck("permission_id", &permissionIDs).
		Error
	return permissionIDs, err
}

// UserRoleMappingCreate 创建用户角色关系
func UserRoleMappingCreate(userRole *UserRoleMappingModel) error {
	userRole.Init()
	return db.Create(userRole).Error
}

// UserRoleGet 获取用户角色关系
func UserRoleMappingGet(id string) (*UserRoleMappingModel, error) {
	var userRole UserRoleMappingModel
	err := db.First(&userRole, "id = ?", id).Error
	return &userRole, err
}

// UserRoleUpdate 更新用户角色关系
func UserRoleUpdate(userRole *UserRoleMappingModel) error {
	return db.Save(userRole).Error
}

// UserRoleUnlink 删除用户角色关系
func UserRoleUnlink(roleIds []string, userIds []string) (int64, error) {
	// 直接删除用户角色
	result := db.Unscoped().Where("user_id in ? AND role_id in ?", userIds, roleIds).Delete(&UserRoleMappingModel{})
	if err := result.Error; err != nil {
		return 0, err
	}

	// 返回受影响的行数
	return result.RowsAffected, nil
}

// UserRoleLink
func UserRoleLink(roleIds []string, userIds []string) (int64, error) {
	tx := db.Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	for _, roleId := range roleIds {
		roleType := "channel"
		if !strings.HasPrefix(roleId, "ch-") {
			roleType = "system"
		}
		for _, uid := range userIds {
			item := &UserRoleMappingModel{
				UserID:   uid,
				RoleID:   roleId,
				RoleType: roleType,
			}
			item.Init()
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&item).Error; err != nil {
				tx.Rollback()
				return 0, err
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		return 0, err
	}

	return int64(len(userIds)), nil
}

// UserRoleMappingListByUserID 获取用户所属的角色列表
func UserRoleMappingListByUserID(userID string, channelId string, roleType string) ([]string, error) {
	var roleIDs []string
	q := db.Model(&UserRoleMappingModel{}).
		Where("user_id = ?", userID)

	switch roleType {
	case "system":
		q = q.Where("role_type = ?", roleType)

	case "channel":
		q = q.Where("role_type = ?", roleType)
		if channelId != "" {
			q = q.Where("role_id LIKE ?", "ch-"+channelId+"-%")
		}

	case "":
		if channelId != "" {
			q = q.Where("(role_type = ?) OR (role_id LIKE ?)", "system", "ch-"+channelId+"-%")
		}
	}

	err := q.Pluck("role_id", &roleIDs).Error
	return roleIDs, err
}

// UserRoleMappingListByChannelID 获取用户-角色关联列表
func UserRoleMappingListByChannelID(chId string, page, pageSize int) ([]*UserRoleMappingModel, int64, error) {
	return utils.QueryPaginatedList(db, page, pageSize, &UserRoleMappingModel{}, func(q *gorm.DB) *gorm.DB {
		q = q.Where("role_type = ?", "channel")
		return q.Where("role_id LIKE ?", "ch-"+chId+"-%")
	})
}

// UserRoleMappingListByChannelIDAll 获取频道内所有用户-角色关联
func UserRoleMappingListByChannelIDAll(chId string) ([]*UserRoleMappingModel, error) {
	var items []*UserRoleMappingModel
	err := db.Model(&UserRoleMappingModel{}).
		Where("role_type = ?", "channel").
		Where("role_id LIKE ?", "ch-"+chId+"-%").
		Find(&items).Error
	return items, err
}

// UserRoleMappingUserIdListByRoleId 获取用户-角色关联列表
func UserRoleMappingUserIdListByRoleId(roleId string) ([]string, error) {
	var ids []string
	q := db.Model(&UserRoleMappingModel{}).
		Where("role_id = ?", roleId)

	err := q.Pluck("user_id", &ids).Error
	return ids, err
}

func ExtractChIdFromRoleId(text string) string {
	parts := strings.Split(text, "-")
	if len(parts) >= 3 {
		return strings.Join(parts[1:len(parts)-1], "-")
	}
	return ""
}
