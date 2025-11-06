package pm

import "github.com/mikespook/gorbac"

var (
	PermFuncChannelRead        = gorbac.NewStdPermission("func_channel_read")          // 频道 - 消息 - 查看
	PermFuncChannelTextSend    = gorbac.NewStdPermission("func_channel_text_send")     // 频道 - 消息 - 文本发送
	PermFuncChannelTextSendOOC = gorbac.NewStdPermission("func_channel_text_send_ooc") // 频道 - 消息 - 场外文本发送
	PermFuncChannelFileSend    = gorbac.NewStdPermission("func_channel_file_send")     // 频道 - 消息 - 文件发送
	PermFuncChannelAudioSend   = gorbac.NewStdPermission("func_channel_audio_send")    // 频道 - 消息 - 音频发送

	PermFuncChannelInvite           = gorbac.NewStdPermission("func_channel_invite")             // 频道 - 常规 - 邀请加入频道
	PermFuncChannelSubChannelCreate = gorbac.NewStdPermission("func_channel_sub_channel_create") // 频道 - 常规 - 创建子频道

	PermFuncChannelMemberRemove = gorbac.NewStdPermission("func_channel_member_remove") // 频道 - 频道管理 - 踢人
	PermFuncChannelManageMute   = gorbac.NewStdPermission("func_channel_manage_mute")   // 频道 - 频道管理 - 禁言

	// 成员管理

	PermFuncChannelRoleLink       = gorbac.NewStdPermission("func_channel_role_link")        // 频道 - 成员管理 - 添加角色
	PermFuncChannelRoleUnlink     = gorbac.NewStdPermission("func_channel_role_unlink")      // 频道 - 成员管理 - 移除角色
	PermFuncChannelRoleLinkRoot   = gorbac.NewStdPermission("func_channel_role_link_root")   // 频道 - 成员管理 - 添加角色 (Root管理员)
	PermFuncChannelRoleUnlinkRoot = gorbac.NewStdPermission("func_channel_role_unlink_root") // 频道 - 成员管理 - 移除角色 (Root管理员)

	// 基础设置 + 权限配置

	PermFuncChannelManageInfo     = gorbac.NewStdPermission("func_channel_manage_info")      // 频道 - 频道设置 - 基础设置
	PermFuncChannelManageRole     = gorbac.NewStdPermission("func_channel_manage_role")      // 频道 - 频道设置 - 权限管理
	PermFuncChannelManageRoleRoot = gorbac.NewStdPermission("func_channel_manage_role_root") // 频道 - 频道设置 - 权限管理（Root管理员）
	PermFuncChannelManageGallery  = gorbac.NewStdPermission("func_channel_manage_gallery")   // 频道 - 频道设置 - 快捷表情资源管理
	PermFuncChannelMessageArchive = gorbac.NewStdPermission("func_channel_message_archive")  // 频道 - 消息 - 归档

	// 准备加一个at权限

	PermFuncChannelReadAll     = gorbac.NewStdPermission("func_channel_read_all")      // 频道 - 特殊 - 查看所有子频道
	PermFuncChannelTextSendAll = gorbac.NewStdPermission("func_channel_text_send_all") // 频道 - 特殊 - 在所有子频道发送文本
)
