
import type { PermResult } from "./types-perm";


export interface ChannelRolePermSheet {
  func_channel_read: PermResult; // 频道 - 消息 - 查看
  func_channel_text_send: PermResult; // 频道 - 消息 - 文本发送
  func_channel_text_send_ooc: PermResult; // 频道 - 消息 - 场外文本发送
  func_channel_file_send: PermResult; // 频道 - 消息 - 文件发送
  func_channel_audio_send: PermResult; // 频道 - 消息 - 音频发送
  func_channel_invite: PermResult; // 频道 - 常规 - 邀请加入频道
  func_channel_sub_channel_create: PermResult; // 频道 - 常规 - 创建子频道
  func_channel_member_remove: PermResult; // 频道 - 频道设置 - 踢人
  func_channel_manage_mute: PermResult; // 频道 - 频道设置 - 禁言
  func_channel_role_link: PermResult; // 频道 - 成员管理 - 添加角色
  func_channel_role_unlink: PermResult; // 频道 - 成员管理 - 移除角色
  func_channel_role_link_root: PermResult; // 频道 - 成员管理 - 添加角色 (Root管理员)
  func_channel_role_unlink_root: PermResult; // 频道 - 成员管理 - 移除角色 (Root管理员)
  func_channel_manage_info: PermResult; // 频道 - 频道设置 - 基础设置
  func_channel_manage_role: PermResult; // 频道 - 频道设置 - 权限管理
  func_channel_manage_role_root: PermResult; // 频道 - 频道设置 - 权限管理（Root管理员）
  func_channel_manage_gallery: PermResult; // 频道 - 频道设置 - 快捷表情资源管理
  func_channel_message_pin: PermResult; // 频道 - 消息 - 置顶
  func_channel_message_archive: PermResult; // 频道 - 消息 - 归档
  func_channel_message_delete: PermResult; // 频道 - 消息 - 删除
  func_channel_message_read_whisper_all: PermResult; // 频道 - 消息 - 查看所有悄悄话
  func_channel_iform_manage: PermResult; // 频道 - iForm - 配置管理
  func_channel_iform_broadcast: PermResult; // 频道 - iForm - 同步推送
  func_channel_theater_view: PermResult; // 频道 - 小剧场 - 查看
  func_channel_theater_scene_switch: PermResult; // 频道 - 小剧场 - 切换场景
  func_channel_theater_object_edit: PermResult; // 频道 - 小剧场 - 编辑对象
  func_channel_theater_object_edit_delegated: PermResult; // 频道 - 小剧场 - 编辑授权对象
  func_channel_theater_character_edit: PermResult; // 频道 - 小剧场 - 编辑角色
  func_channel_theater_resource_upload: PermResult; // 频道 - 小剧场 - 上传资源
  func_channel_theater_resource_delete: PermResult; // 频道 - 小剧场 - 删除资源
  func_channel_theater_action_trigger: PermResult; // 频道 - 小剧场 - 触发动作
  func_channel_theater_admin_restore: PermResult; // 频道 - 小剧场 - 管理恢复
  func_channel_read_all: PermResult; // 频道 - 特殊 - 查看所有子频道
  func_channel_text_send_all: PermResult; // 频道 - 特殊 - 在所有子频道发送文本
}
