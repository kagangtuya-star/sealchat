
import type { PermResult } from "./types-perm";


export interface SystemRolePermSheet {
  mod_admin: PermResult; // 系统 - 查看管理界面 - 查看管理界面
  func_admin_serve_config: PermResult; // 系统 - 设置 - 修改serve配置
  func_world_create: PermResult; // 系统 - 世界 - 创建
  func_world_manage: PermResult; // 系统 - 世界 - 管理
  func_admin_bot_token_view: PermResult; // 系统 - 机器人 - 令牌查看
  func_admin_bot_token_create: PermResult; // 系统 - 机器人 - 令牌创建
  func_admin_bot_token_edit: PermResult; // 系统 - 机器人 - 令牌编辑
  func_admin_bot_token_delete: PermResult; // 系统 - 机器人 - 令牌删除
  func_admin_user_set_enable: PermResult; // 系统 - 用户 - 禁用和启用用户
  func_admin_user_password_reset: PermResult; // 系统 - 用户 - 重置用户密码
  func_admin_user_edit: PermResult; // 系统 - 用户 - 设置角色等
  func_channel_create_public: PermResult; // 系统 - 频道 - 创建公开频道
  func_channel_create_non_public: PermResult; // 系统 - 频道 - 创建个人频道
}
