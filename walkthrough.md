# 未读消息邮件通知功能 - 代码审查清单

## 功能概述

实现未读消息邮件推送提醒功能，包括：
- 后台定时检查未读消息并发送邮件摘要
- 用户可配置接收邮箱、延迟时间
- 支持用户自定义 SMTP 服务器（可选）
- 管理员后台开关与 SMTP 测试功能

---

## 后端代码更改

### 1. 配置模块

#### [config.go](file:///e:/Code/go/sealchat/utils/config.go)

| 行号范围 | 更改内容 |
|---------|---------|
| 123-133 | 新增 `SMTPConfig` 结构体 |
| 135-143 | 新增 `EmailNotificationConfig` 结构体 |
| 167 | `AppConfig` 添加 `EmailNotification` 字段 |
| 559-565 | `WriteConfig` 添加邮件通知配置持久化 |

> [!IMPORTANT]
> **审查要点**：确认 SMTP 密码字段使用 `json:"-"` 标签，不会通过 API 暴露。

---

### 2. 数据模型

#### [NEW] [email_notification.go](file:///e:/Code/go/sealchat/model/email_notification.go)

| 结构/函数 | 说明 |
|----------|------|
| `EmailNotificationSettingsModel` | 用户邮件通知设置（含自定义 SMTP 字段） |
| `EmailNotificationLogModel` | 推送日志（用于频率限制） |
| `EmailNotificationSettingsUpsertParams` | Upsert 参数结构体 |
| `ToSMTPConfig()` | 将用户设置转换为 SMTPConfig |

> [!NOTE]
> 密码字段 `SMTPPassword` 使用 `json:"-"` 标签，不返回给前端。

#### [db.go](file:///e:/Code/go/sealchat/model/db.go)

| 行号 | 更改内容 |
|-----|---------|
| 139-141 | AutoMigrate 添加新表 |

---

### 3. 服务层

#### [NEW] [email.go](file:///e:/Code/go/sealchat/service/email.go)

| 函数 | 说明 |
|-----|------|
| `NewEmailService()` | 创建邮件服务实例 |
| `IsConfigured()` | 检查 SMTP 是否已配置 |
| `SendEmail()` | 发送 HTML 邮件 |
| `BuildUnreadDigestHTML()` | 构建未读消息摘要 HTML |

#### [NEW] [unread_notification_worker.go](file:///e:/Code/go/sealchat/service/unread_notification_worker.go)

| 函数 | 说明 |
|-----|------|
| `StartUnreadNotificationWorker()` | 启动后台 Worker |
| `processUnreadNotifications()` | 处理所有启用的通知设置 |
| `processUserChannelNotification()` | 处理单个用户频道的通知 |

> [!IMPORTANT]
> **审查要点**：Worker 会根据 `setting.UseCustomSMTP` 选择使用用户自定义 SMTP 或系统 SMTP。

---

### 4. API 层

#### [NEW] [email_notification.go](file:///e:/Code/go/sealchat/api/email_notification.go)

| 端点 | 函数 | 说明 |
|-----|------|------|
| GET `/channels/:channelId/email-notification` | `EmailNotificationSettingsGet` | 获取用户设置 |
| POST `/channels/:channelId/email-notification` | `EmailNotificationSettingsUpsert` | 保存设置 |
| DELETE `/channels/:channelId/email-notification` | `EmailNotificationSettingsDelete` | 删除设置 |
| POST `/email-notification/test` | `EmailNotificationTestSend` | 测试邮件（支持自定义 SMTP） |
| POST `/admin/email-test` | `AdminEmailTestSend` | 管理员测试 SMTP |

#### [api_bind.go](file:///e:/Code/go/sealchat/api/api_bind.go)

| 行号 | 更改内容 |
|-----|---------|
| 145-151 | v1Auth 组注册邮件通知路由 |
| 283-284 | v1AuthAdmin 组注册管理员测试路由 |

---

### 5. 应用入口

#### [main.go](file:///e:/Code/go/sealchat/main.go)

| 行号 | 更改内容 |
|-----|---------|
| 113-123 | 条件启动 `UnreadNotificationWorker` |

---

## 前端代码更改

### 1. 类型定义

#### [types.ts](file:///e:/Code/go/sealchat/ui/src/types.ts)

| 行号 | 更改内容 |
|-----|---------|
| 115-120 | `ServerConfig` 添加 `emailNotification` 字段 |

---

### 2. 组件

#### [NEW] [EmailNotificationManager.vue](file:///e:/Code/go/sealchat/ui/src/views/split/components/EmailNotificationManager.vue)

完整的邮件通知设置面板组件，包含：
- 启用/禁用开关
- 接收邮箱输入
- 延迟时间滑块
- 自定义 SMTP 配置（可折叠）
- 测试邮件发送
- localStorage 跨频道 SMTP 缓存

#### [ChatActionRibbon.vue](file:///e:/Code/go/sealchat/ui/src/views/chat/components/ChatActionRibbon.vue)

| 行号 | 更改内容 |
|-----|---------|
| 15-17, 47-49, 63-64, 114-120, 259-264 | 添加邮件提醒按钮入口 |

#### [chat.vue](file:///e:/Code/go/sealchat/ui/src/views/chat/chat.vue)

| 更改内容 |
|---------|
| 导入 `EmailNotificationManager` 组件 |
| 添加 `emailNotificationDrawerVisible` 状态 |
| 添加抽屉组件和事件处理 |

#### [admin-settings-base.vue](file:///e:/Code/go/sealchat/ui/src/views/admin/admin-settings-base.vue)

| 行号 | 更改内容 |
|-----|---------|
| 26 | model 添加 `emailNotification` 默认值 |
| 196-214 | 添加 SMTP 测试状态和函数 |
| 239-247 | 添加启用开关和测试 UI |

---

## 配置示例

#### [config.yaml.example](file:///e:/Code/go/sealchat/config.yaml.example)

新增 `emailNotification` 配置块，包含 SMTP 服务器配置示例。

---

## 审查检查点

### 安全性
- [ ] SMTP 密码字段使用 `json:"-"` 标签
- [ ] 用户 SMTP 密码不通过 API 返回
- [ ] 管理员 SMTP 测试端点有权限验证

### 功能完整性
- [ ] 管理员可在后台开启/关闭邮件通知功能
- [ ] 管理员可测试 SMTP 配置
- [ ] 用户可配置接收邮箱和延迟时间
- [ ] 用户可选择使用自定义 SMTP
- [ ] 用户可测试自定义 SMTP 配置
- [ ] 自定义 SMTP 配置支持跨频道本地缓存

### 边界情况
- [ ] 功能未启用时 API 返回友好提示
- [ ] SMTP 未配置时返回明确错误
- [ ] 频率限制正常工作
- [ ] 用户自定义 SMTP 密码更新逻辑（空密码保留原值）

### 数据库
- [ ] 新表自动迁移
- [ ] 唯一索引正确设置

---

## 环境变量

| 变量名 | 用途 |
|-------|------|
| `SEALCHAT_SMTP_PASSWORD` | 覆盖 config.yaml 中的 SMTP 密码 |
