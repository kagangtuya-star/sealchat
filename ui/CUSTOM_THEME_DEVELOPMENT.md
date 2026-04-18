# SealChat 自定义主题开发文档

## 目录

- [概述](#概述)
- [快速开始](#快速开始)
- [主题配置格式](#主题配置格式)
- [颜色字段详解](#颜色字段详解)
- [JSON 导入导出](#json-导入导出)
- [预设主题开发](#预设主题开发)
- [CSS 变量映射](#css-变量映射)
- [示例主题](#示例主题)

---

## 概述

SealChat 自定义主题系统允许用户创建个性化的配色方案，覆盖系统默认的日间/夜间主题。主题配置保存在浏览器的 `localStorage` 中，支持通过 JSON 文件导入导出。

### 功能特性

- **可视化编辑器**：通过颜色选择器配置各个颜色字段
- **预设主题**：内置护眼主题（如豆沙绿）可一键导入
- **JSON 导入/导出**：支持主题配置的备份与分享
- **实时预览**：颜色变更立即生效
- **多主题管理**：保存多套主题配置，随时切换

---

## 快速开始

### 启用自定义主题

1. 打开 **设置面板** → **常规设置**
2. 找到 **自定义主题** 区域
3. 点击开关启用自定义主题
4. 点击配置按钮打开主题编辑器

### 创建新主题

1. 在主题编辑器中输入主题名称
2. 分组配置各颜色字段（背景、文字、聊天区域等）
3. 点击 **创建主题** 按钮保存

### 导入预设主题

1. 在 **导入预设主题** 区域
2. 从下拉菜单选择预设（如"豆沙绿护眼"）
3. 主题将自动导入并激活

---

## 主题配置格式

### 完整类型定义

```typescript
interface CustomThemeColors {
  // 背景色
  bgSurface?: string        // 主背景
  bgElevated?: string       // 卡片/弹窗背景
  bgInput?: string          // 输入框背景
  bgHeader?: string         // 顶栏背景
  
  // 文字色
  textPrimary?: string      // 主文字
  textSecondary?: string    // 次要文字
  
  // 聊天区域
  chatIcBg?: string         // 场内消息背景
  chatOocBg?: string        // 场外消息背景
  chatStageBg?: string      // 聊天舞台背景
  chatPreviewBg?: string    // 预览区背景
  chatPreviewDot?: string   // 预览区圆点
  
  // 边框
  borderMute?: string       // 淡边框
  borderStrong?: string     // 强边框
  
  // 强调色
  primaryColor?: string     // 主题强调色
  primaryColorHover?: string // 悬停态强调色
  actionRibbonHoverText?: string // 多功能区按钮悬停文字色
  
  // 术语高亮
  keywordBg?: string        // 术语高亮背景
  keywordBorder?: string    // 术语高亮下划线
}

interface CustomTheme {
  id: string                // 唯一标识符
  name: string              // 主题名称
  colors: CustomThemeColors // 颜色配置
  createdAt: number         // 创建时间戳
  updatedAt: number         // 更新时间戳
}
```

---

## 颜色字段详解

### 背景色 (Background)

| 字段名 | 说明 | 影响区域 |
|--------|------|----------|
| `bgSurface` | 主背景色 | 整体页面背景 |
| `bgElevated` | 卡片/弹窗背景 | 弹出层、卡片组件 |
| `bgInput` | 输入框背景 | 文本输入区域 |
| `bgHeader` | 顶栏背景 | 页面顶部导航区 |

### 文字色 (Text)

| 字段名 | 说明 | 影响区域 |
|--------|------|----------|
| `textPrimary` | 主文字颜色 | 标题、正文内容 |
| `textSecondary` | 次要文字颜色 | 描述文本、辅助信息 |

### 聊天区域 (Chat)

| 字段名 | 说明 | 影响区域 |
|--------|------|----------|
| `chatIcBg` | 场内消息背景 | IC (In-Character) 消息气泡 |
| `chatOocBg` | 场外消息背景 | OOC (Out-Of-Character) 消息 |
| `chatStageBg` | 聊天舞台背景 | 聊天区域整体背景 |
| `chatPreviewBg` | 预览区背景 | 输入预览区域背景 |
| `chatPreviewDot` | 预览区圆点 | 预览区点状背景图案 |

### 边框色 (Border)

| 字段名 | 说明 | 影响区域 |
|--------|------|----------|
| `borderMute` | 淡边框 | 分隔线、轻量边框 |
| `borderStrong` | 强边框 | 聚焦态、明显边框 |

### 强调色 (Accent)

| 字段名 | 说明 | 影响区域 |
|--------|------|----------|
| `primaryColor` | 主题强调色 | 主要按钮、链接、高亮 |
| `primaryColorHover` | 悬停态强调色 | 鼠标悬停时的强调色 |
| `actionRibbonHoverText` | 多功能区按钮悬停文字色 | 聊天页/分屏页多功能区按钮 hover 文字和图标 |

### 术语高亮 (Keyword)

| 字段名 | 说明 | 影响区域 |
|--------|------|----------|
| `keywordBg` | 高亮背景 | 世界术语的背景色 |
| `keywordBorder` | 下划线色 | 世界术语的下划线颜色 |

---

## JSON 导入导出

### 导出格式

导出的 JSON 文件格式如下：

```json
{
  "name": "主题名称",
  "colors": {
    "bgSurface": "#f6f9f4",
    "bgElevated": "#fcfdf9",
    "textPrimary": "#2a3b21",
    "...": "..."
  },
  "exportedAt": "2024-12-11T08:00:00.000Z",
  "version": "1.0"
}
```

### 导入要求

导入的 JSON 文件必须包含：

| 字段 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `name` | `string` | ✅ | 主题名称 |
| `colors` | `object` | ✅ | 颜色配置对象 |

### 使用说明

**导出主题：**
1. 在已保存的主题列表中
2. 点击主题旁的 **导出** 按钮
3. 浏览器将自动下载 `.json` 文件

**导入主题：**
1. 在 **导入/导出** 区域
2. 点击 **从 JSON 文件导入** 按钮
3. 选择有效的主题 JSON 文件
4. 导入成功后主题将自动激活

---

## 预设主题开发

### 添加新预设

在 `src/config/presetThemes.ts` 文件中添加预设主题：

```typescript
import type { CustomThemeColors } from '@/stores/display'

export interface PresetTheme {
  id: string
  name: string
  description: string
  colors: CustomThemeColors
}

// 添加新预设
export const myCustomTheme: PresetTheme = {
  id: 'preset_my_theme',
  name: '我的主题',
  description: '主题描述文字',
  colors: {
    bgSurface: '#ffffff',
    bgElevated: '#f8f8f8',
    textPrimary: '#333333',
    // ... 其他颜色字段
  },
}

// 在预设列表中注册
export const presetThemes: PresetTheme[] = [
  dushaGreenTheme,
  myCustomTheme,  // 添加到列表
]
```

---

## CSS 变量映射

自定义主题通过 CSS 变量应用到页面。以下是字段与 CSS 变量的映射关系：

| 配置字段 | CSS 变量 |
|----------|----------|
| `bgSurface` | `--sc-bg-surface` |
| `bgElevated` | `--sc-bg-elevated` |
| `bgInput` | `--sc-bg-input` |
| `bgHeader` | `--sc-bg-header` |
| `textPrimary` | `--sc-text-primary` |
| `textSecondary` | `--sc-text-secondary` |
| `chatIcBg` | `--custom-chat-ic-bg` |
| `chatOocBg` | `--custom-chat-ooc-bg` |
| `chatStageBg` | `--custom-chat-stage-bg` |
| `chatPreviewBg` | `--custom-chat-preview-bg` |
| `chatPreviewDot` | `--custom-chat-preview-dot` |
| `borderMute` | `--sc-border-mute` |
| `borderStrong` | `--sc-border-strong` |
| `primaryColor` | `--primary-color` |
| `primaryColorHover` | `--primary-color-hover` |
| `actionRibbonHoverText` | `--sc-action-ribbon-hover-text` |
| `keywordBg` | `--custom-keyword-bg` |
| `keywordBorder` | `--custom-keyword-border` |

---

## 示例主题

### 豆沙绿护眼主题

参考菠萝平台的护眼豆沙绿颜色设计，使用低饱和度暖绿色系，减少视觉疲劳。

```json
{
  "name": "豆沙绿护眼",
  "colors": {
    "bgSurface": "#f6f9f4",
    "bgElevated": "#fcfdf9",
    "bgInput": "#edf3e8",
    "bgHeader": "#f8faf6",
    "textPrimary": "#2a3b21",
    "textSecondary": "#557542",
    "chatIcBg": "#f4f8f0",
    "chatOocBg": "#fcfdf9",
    "chatStageBg": "#edf3e8",
    "chatPreviewBg": "#dce8d4",
    "chatPreviewDot": "#c5d9b8",
    "borderMute": "#dce8d4",
    "borderStrong": "#a8c494",
    "primaryColor": "#6d9255",
    "primaryColorHover": "#8bae72",
    "actionRibbonHoverText": "#5f7f49",
    "keywordBg": "rgba(139, 174, 114, 0.2)",
    "keywordBorder": "#8bae72"
  },
  "version": "1.0"
}
```

### 暖色调阅读主题

```json
{
  "name": "暖色调阅读",
  "colors": {
    "bgSurface": "#faf8f5",
    "bgElevated": "#fffef9",
    "bgInput": "#f5f0e8",
    "bgHeader": "#f8f5f0",
    "textPrimary": "#3d3528",
    "textSecondary": "#6b5f4e",
    "chatIcBg": "#faf6f0",
    "chatOocBg": "#fffef9",
    "borderMute": "#e8e0d4",
    "borderStrong": "#c9b99a",
    "primaryColor": "#8b7355",
    "primaryColorHover": "#a08870",
    "actionRibbonHoverText": "#6f5b42"
  },
  "version": "1.0"
}
```

---

## 常见问题

### Q: 自定义主题没有生效？

1. 确保已在设置中 **启用** 自定义主题开关
2. 确保已选中/激活一个保存的主题
3. 检查浏览器控制台是否有错误

### Q: 导入 JSON 失败？

1. 确保文件是有效的 JSON 格式
2. 确保包含必需的 `name` 和 `colors` 字段
3. 颜色值应为有效的 CSS 颜色格式（如 `#ffffff` 或 `rgba(...)`)

### Q: 如何分享我的主题？

1. 在主题列表中点击 **导出** 按钮下载 JSON 文件
2. 将 JSON 文件分享给其他用户
3. 其他用户通过 **从 JSON 文件导入** 即可使用

---

*文档版本: 1.0 | 最后更新: 2025-12*
