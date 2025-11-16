package pm

import "github.com/mikespook/gorbac"

// 全局权限

// 权限相关名词表: view create edit del export import upload

var (
	PermModAdmin             = gorbac.NewStdPermission("mod_admin")               // 系统 - 查看管理界面 - 查看管理界面
	PermFuncAdminServeConfig = gorbac.NewStdPermission("func_admin_serve_config") // 系统 - 设置 - 修改serve配置
	PermFuncWorldCreate      = gorbac.NewStdPermission("func_world_create")       // 系统 - 世界 - 创建世界
	PermFuncWorldManage      = gorbac.NewStdPermission("func_world_manage")       // 系统 - 世界 - 管理世界

	PermFuncAdminBotTokenView   = gorbac.NewStdPermission("func_admin_bot_token_view")   // 系统 - 机器人 - 令牌查看
	PermFuncAdminBotTokenCreate = gorbac.NewStdPermission("func_admin_bot_token_create") // 系统 - 机器人 - 令牌创建
	PermFuncAdminBotTokenEdit   = gorbac.NewStdPermission("func_admin_bot_token_edit")   // 系统 - 机器人 - 令牌编辑
	PermFuncAdminBotTokenDelete = gorbac.NewStdPermission("func_admin_bot_token_delete") // 系统 - 机器人 - 令牌删除

	PermFuncAdminUserSetEnable     = gorbac.NewStdPermission("func_admin_user_set_enable")     // 系统 - 用户 - 禁用和启用用户
	PermFuncAdminUserPasswordReset = gorbac.NewStdPermission("func_admin_user_password_reset") // 系统 - 用户 - 重置用户密码
	PermFuncAdminUserEdit          = gorbac.NewStdPermission("func_admin_user_edit")           // 系统 - 用户 - 设置角色等

	PermFuncChannelCreatePublic    = gorbac.NewStdPermission("func_channel_create_public")     // 系统 - 频道 - 创建公开频道
	PermFuncChannelCreateNonPublic = gorbac.NewStdPermission("func_channel_create_non_public") // 系统 - 频道 - 创建个人频道

	// "g_channel_settings_edit": 1, // 频道管理 - 通用权限
)
