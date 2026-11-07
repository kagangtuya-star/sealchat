package model

import "gorm.io/gorm"

// migrateRoleGrantSources 旧记录没有来源；当前世界角色能够解释的标准角色
// 按继承处理，其余保留为手动授权。无法推断旧记录是否曾拥有双重来源。
func migrateRoleGrantSources(conn *gorm.DB) error {
	return RunSchemaMigration(conn, "user_role_grant_sources_v1", func() error {
		return conn.Transaction(func(tx *gorm.DB) error {
			inherited := tx.Table("perm_channel_roles AS cr").Select("1").
				Joins("JOIN channels AS c ON c.id = cr.channel_id").
				Joins("JOIN world_members AS wm ON wm.world_id = c.world_id").
				Where("cr.id = perm_user_role_mappings.role_id AND wm.user_id = perm_user_role_mappings.user_id").
				Where(`(wm.role IN ? AND cr.id LIKE ?) OR
					(wm.role = ? AND cr.id LIKE ?) OR
					(wm.role = ? AND cr.id LIKE ? AND c.perm_type = ? AND
					(c.root_id = '' OR c.root_id IS NULL OR EXISTS
					(SELECT 1 FROM channels AS root WHERE root.id = c.root_id AND root.perm_type = ?)))`,
					[]string{WorldRoleOwner, WorldRoleAdmin}, "%-admin",
					WorldRoleSpectator, "%-spectator", WorldRoleMember, "%-member", "public", "public")
			return tx.Model(&UserRoleMappingModel{}).
				Where("role_type = ? AND grant_source = ?", "channel", RoleGrantManual).
				Where("EXISTS (?)", inherited).
				UpdateColumn("grant_source", RoleGrantWorld).Error
		})
	})
}

// migratePublicChannelMemberExplicitCompatibility 保留旧公开频道既有成员列表语义。
// 迁移完成后新产生的世界继承 member 保持 world-only，可与显式参团成员区分。
func migratePublicChannelMemberExplicitCompatibility(conn *gorm.DB) error {
	return RunSchemaMigration(conn, "public_channel_member_explicit_compat_v1", func() error {
		publicMemberRoleIDs := conn.Table("perm_channel_roles AS cr").
			Select("cr.id").
			Joins("JOIN channels AS c ON c.id = cr.channel_id").
			Where("c.perm_type = ? AND cr.id LIKE ?", "public", "%-member")
		return conn.Model(&UserRoleMappingModel{}).
			Where("role_type = ? AND grant_source = ?", "channel", RoleGrantWorld).
			Where("role_id IN (?)", publicMemberRoleIDs).
			UpdateColumn("grant_source", RoleGrantBoth).Error
	})
}
