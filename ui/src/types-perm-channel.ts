
import type { PermResult } from "./types-perm";


export interface ChannelRolePermSheet {
  func_channel_read: PermResult; // 频道 - 消息 - 查看
  func_channel_text_send: PermResult; // 频道 - 消息 - 文本发送
  func_channel_text_send_ooc: PermResult; // 频道 - 消息 - 场外文本发送
  func_channel_file_send: PermResult; // 频道 - 消息 - 文件发送
  func_channel_audio_send: PermResult; // 频道 - 消息 - 音频发送
  func_channel_invite: PermResult; // 频道 - 常规 - 邀请加入频道
  func_channel_sub_channel_create: PermResult; // 频道 - 常规 - 创建子频道
  func_channel_member_remove: PermResult; // 频道 - 频道管理 - 踢人
  func_channel_manage_mute: PermResult; // 频道 - 频道管理 - 禁言
  func_channel_role_link: PermResult; // 频道 - 成员管理 - 添加角色
  func_channel_role_unlink: PermResult; // 频道 - 成员管理 - 移除角色
  func_channel_role_link_root: PermResult; // 频道 - 成员管理 - 添加角色 (Root管理员)
  func_channel_role_unlink_root: PermResult; // 频道 - 成员管理 - 移除角色 (Root管理员)
  func_channel_manage_info: PermResult; // 频道 - 频道设置 - 基础设置
  func_channel_manage_role: PermResult; // 频道 - 频道设置 - 权限管理
  func_channel_manage_role_root: PermResult; // 频道 - 频道设置 - 权限管理（Root管理员）
  func_channel_manage_gallery: PermResult; // 频道 - 频道设置 - 快捷表情资源管理
  func_channel_message_archive: PermResult; // 频道 - 消息 - 归档
  func_channel_iform_manage: PermResult; // 频道 - iForm - 配置管理
  func_channel_iform_broadcast: PermResult; // 频道 - iForm - 同步推送
  func_channel_read_all: PermResult; // 频道 - 特殊 - 查看所有子频道
  func_channel_text_send_all: PermResult; // 频道 - 特殊 - 在所有子频道发送文本
}
