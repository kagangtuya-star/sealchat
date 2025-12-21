# 便签功能开发文档

## 概述

便签（Sticky Note）功能允许用户在频道中创建、编辑、管理可拖拽的便签。支持实时同步、颜色自定义、推送给其他用户等功能。

---

## 功能特性

| 功能 | 描述 |
|------|------|
| 创建便签 | 在当前频道创建新便签 |
| 编辑便签 | 修改便签标题和内容（仅创建者） |
| 删除便签 | 删除便签（仅创建者） |
| 拖拽移动 | 自由拖动便签位置 |
| 调整大小 | 拖拽右下角调整便签尺寸 |
| 颜色选择 | 6种颜色主题（黄、粉、绿、蓝、紫、橙） |
| 最小化 | 将便签最小化到底部栏 |
| 实时同步 | 通过 WebSocket 实时同步更新 |
| 用户状态 | 每用户独立的位置/大小/开关状态 |
| 推送功能 | 将便签推送给指定用户 |
| Z-Index管理 | 点击便签自动置顶 |

---

## 文件结构

### 后端文件

```
sealchat/
├── model/
│   ├── sticky_note.go      # 数据模型和数据库操作
│   └── db.go               # 数据库迁移（添加便签表）
├── protocol/
│   └── protocol.go         # 事件类型和协议结构
└── api/
    ├── sticky_note.go      # REST API 和 WebSocket API
    ├── api_bind.go         # REST 路由注册
    └── chat_websocket.go   # WebSocket 路由注册
```

### 前端文件

```
ui/src/
├── stores/
│   └── stickyNote.ts                           # Pinia 状态管理
└── views/chat/components/
    ├── StickyNote.vue                          # 单个便签组件
    └── StickyNoteManager.vue                   # 便签管理器
```

---

## 数据模型

### StickyNoteModel

便签主表，存储便签的基本信息。

```go
type StickyNoteModel struct {
    StringPKBaseModel
    ChannelID   string     `json:"channel_id" gorm:"index"`    // 所属频道
    WorldID     string     `json:"world_id" gorm:"index"`      // 所属世界
    Title       string     `json:"title"`                       // 标题
    Content     string     `json:"content" gorm:"type:text"`   // HTML 富文本内容
    ContentText string     `json:"content_text"`                // 纯文本版本（用于搜索）
    Color       string     `json:"color" gorm:"default:'yellow'"` // 颜色主题
    CreatorID   string     `json:"creator_id" gorm:"index"`    // 创建者ID
    IsPublic    bool       `json:"is_public" gorm:"default:true"`
    IsPinned    bool       `json:"is_pinned" gorm:"default:false"`
    OrderIndex  int        `json:"order_index"`                 // 排序
    DefaultX    int        `json:"default_x"`                   // 默认X位置
    DefaultY    int        `json:"default_y"`                   // 默认Y位置
    DefaultW    int        `json:"default_w"`                   // 默认宽度
    DefaultH    int        `json:"default_h"`                   // 默认高度
    IsDeleted   bool       `json:"is_deleted"`
    DeletedAt   *time.Time `json:"deleted_at"`
    DeletedBy   string     `json:"deleted_by"`
}
```

### StickyNoteUserStateModel

用户状态表，存储每个用户对每个便签的个性化状态。

```go
type StickyNoteUserStateModel struct {
    StringPKBaseModel
    StickyNoteID string     `json:"sticky_note_id" gorm:"uniqueIndex:idx_note_user"`
    UserID       string     `json:"user_id" gorm:"uniqueIndex:idx_note_user"`
    IsOpen       bool       `json:"is_open" gorm:"default:false"`     // 是否打开
    LastOpenedAt *time.Time `json:"last_opened_at"`
    PositionX    int        `json:"position_x"`                        // 用户自定义X位置
    PositionY    int        `json:"position_y"`                        // 用户自定义Y位置
    Width        int        `json:"width"`                             // 用户自定义宽度
    Height       int        `json:"height"`                            // 用户自定义高度
    Minimized    bool       `json:"minimized" gorm:"default:false"`   // 是否最小化
    ZIndex       int        `json:"z_index" gorm:"default:1000"`      // 层级
}
```

---

## REST API

### 获取频道便签列表

```
GET /api/v1/channels/:channelId/sticky-notes
```

**响应:**
```json
{
  "items": [
    {
      "note": { ... },
      "userState": { ... }
    }
  ]
}
```

### 创建便签

```
POST /api/v1/channels/:channelId/sticky-notes
```

**请求体:**
```json
{
  "title": "便签标题",
  "content": "内容",
  "color": "yellow",
  "defaultX": 100,
  "defaultY": 100,
  "defaultW": 300,
  "defaultH": 250
}
```

### 获取单个便签

```
GET /api/v1/sticky-notes/:noteId
```

### 更新便签

```
PATCH /api/v1/sticky-notes/:noteId
```

**请求体:**
```json
{
  "title": "新标题",
  "content": "新内容",
  "color": "blue"
}
```

### 删除便签

```
DELETE /api/v1/sticky-notes/:noteId
```

### 更新用户状态

```
PATCH /api/v1/sticky-notes/:noteId/state
```

**请求体:**
```json
{
  "isOpen": true,
  "positionX": 200,
  "positionY": 150,
  "width": 350,
  "height": 300,
  "minimized": false,
  "zIndex": 1001
}
```

### 推送便签

```
POST /api/v1/sticky-notes/:noteId/push
```

**请求体:**
```json
{
  "targetUserIds": ["user1", "user2"]
}
```

---

## WebSocket API

### 更新便签

```
API: sticky-note.update
```

**请求数据:**
```json
{
  "data": {
    "noteId": "xxx",
    "title": "新标题",
    "content": "新内容",
    "contentText": "纯文本内容"
  }
}
```

### 删除便签

```
API: sticky-note.delete
```

**请求数据:**
```json
{
  "data": {
    "noteId": "xxx"
  }
}
```

### 推送便签

```
API: sticky-note.push
```

**请求数据:**
```json
{
  "data": {
    "noteId": "xxx",
    "targetUserIds": ["user1", "user2"]
  }
}
```

---

## WebSocket 事件

### 事件类型

| 事件名 | 描述 |
|--------|------|
| `sticky-note-created` | 便签创建 |
| `sticky-note-updated` | 便签更新 |
| `sticky-note-deleted` | 便签删除 |
| `sticky-note-pushed` | 便签推送 |

### 事件负载结构

```typescript
interface StickyNoteEventPayload {
  note: StickyNote
  action: 'create' | 'update' | 'delete' | 'push'
  targetUserIds?: string[]  // 仅 push 时存在
}
```

---

## 前端状态管理

### Pinia Store: useStickyNoteStore

```typescript
// 状态
notes: Record<string, StickyNote>           // 便签数据
userStates: Record<string, StickyNoteUserState>  // 用户状态
activeNoteIds: string[]                      // 打开的便签ID
editingNoteId: string | null                 // 正在编辑的便签ID
currentChannelId: string                     // 当前频道ID
loading: boolean                             // 加载状态
maxZIndex: number                            // 最大z-index

// 计算属性
noteList: StickyNote[]                       // 便签列表
activeNotes: StickyNote[]                    // 打开的便签
pinnedNotes: StickyNote[]                    // 置顶的便签

// 方法
loadChannelNotes(channelId)                  // 加载频道便签
createNote(params)                           // 创建便签
updateNote(noteId, updates)                  // 更新便签
deleteNote(noteId)                           // 删除便签
updateUserState(noteId, updates)             // 更新用户状态
pushNote(noteId, targetUserIds)              // 推送便签
openNote(noteId)                             // 打开便签
closeNote(noteId)                            // 关闭便签
bringToFront(noteId)                         // 置顶便签
minimizeNote(noteId)                         // 最小化便签
restoreNote(noteId)                          // 恢复便签
startEditing(noteId)                         // 开始编辑
stopEditing()                                // 结束编辑
handleStickyNoteEvent(event)                 // 处理WebSocket事件
reset()                                      // 重置状态
```

---

## 组件说明

### StickyNote.vue

单个便签组件，负责：
- 渲染便签内容和样式
- 处理拖拽移动
- 处理调整大小
- 编辑模式切换
- 颜色选择
- 复制/最小化/关闭操作

**Props:**
```typescript
noteId: string  // 便签ID
```

### StickyNoteManager.vue

便签管理器组件，负责：
- 渲染所有活跃的便签
- 显示最小化便签栏
- 提供创建新便签的 FAB 按钮
- 显示便签列表面板

**Props:**
```typescript
channelId: string  // 频道ID
```

---

## 颜色主题

| 颜色值 | 显示颜色 | CSS 渐变 |
|--------|----------|----------|
| yellow | 黄色 | #fff9c4 → #fff59d |
| pink | 粉色 | #f8bbd9 → #f48fb1 |
| green | 绿色 | #c8e6c9 → #a5d6a7 |
| blue | 蓝色 | #bbdefb → #90caf9 |
| purple | 紫色 | #e1bee7 → #ce93d8 |
| orange | 橙色 | #ffe0b2 → #ffcc80 |

---

## 权限控制

- **创建便签**: 任何频道成员
- **编辑便签**: 仅创建者
- **删除便签**: 仅创建者
- **查看便签**: 所有频道成员
- **推送便签**: 任何频道成员

---

## 使用示例

### 创建便签

1. 进入任意频道
2. 点击右下角的 **+** 按钮
3. 新便签将自动打开

### 编辑便签

1. 点击便签头部的编辑图标
2. 修改标题和内容
3. 点击编辑图标退出编辑模式

### 移动便签

1. 按住便签头部
2. 拖动到目标位置
3. 释放鼠标

### 调整大小

1. 将鼠标移到便签右下角
2. 按住并拖动
3. 释放鼠标

### 更改颜色

1. 进入编辑模式
2. 点击底部的颜色按钮

---

## 注意事项

1. 便签内容目前支持纯文本，未来可扩展为富文本编辑器
2. 用户状态（位置、大小）是每用户独立的，不会影响其他用户
3. WebSocket 事件会实时同步便签的创建、更新、删除
4. 便签使用软删除，数据不会真正从数据库移除
