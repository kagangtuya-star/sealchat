# 布局频道（工作台）

布局频道是普通频道的一种展示形态：`channels.display_mode = 'workbench'`。世界、成员、角色权限、公开 / 非公开语义与普通频道完全相同。工作台始终是频道主视图，不存在“切回普通消息流”的入口；聊天应通过添加 Chat Panel 放入工作台。加载失败只提供工作台重试；空布局中管理者可选择预设或开始设计，普通成员看到尚未配置的提示。观察者模式、分屏与 Embed 链接继续显示消息流。

代码入口：

| 位置 | 内容 |
| --- | --- |
| `model/channel_workbench.go` | `channel_workbenches` 表（每频道一行）与 CAS 写入 |
| `service/channel_workbench.go` | 布局 schema、校验 / 迁移、权限、展示状态、复制频道 |
| `api/channel_workbench.go` | REST 与 WS 失效通知 |
| `ui/src/features/channel-workbench/` | 前端协议、网格操作、导入导出、宿主组件 |

## 1. 三类状态

| 状态 | 存储 | 版本 | 谁能改 |
| --- | --- | --- | --- |
| 布局定义 | `channel_workbenches.layout_json` | `layoutRevision` | 管理者（世界管理员、拥有频道“管理信息”权限者、平台管理员） |
| GM 展示状态 | `channel_workbenches.presentation_json` | `presentationRevision` | 同上 |
| 本地显示状态 | 浏览器 localStorage | — | 每个用户自己 |

本地状态：`sealchat:workbench-view:v1:<userId>:<channelId>`（折叠、展开、移动端标签、已退出的聚焦）、`sealchat:workbench-draft:v1:<userId>:<channelId>`（未保存的设计草稿）、`sealchat:workbench-floating-windows:v2:<userId>:<worldId>:<channelId>`（固定弹出的个人浮窗，由 Floating Host 持久化；临时弹出不持久化）。固定浮窗属于用户本地状态，账号之间隔离；无用户 ID 时不恢复或保存，不迁移缺少 owner 信息的旧 v1 浮窗。

## 2. 布局 schema（version 1）

```jsonc
{
  "version": 1,
  "grid": { "columns": 24, "rows": 12, "gap": 8 },          // columns ∈ {12, 24}，rows 4–60，gap 0–32
  "settings": { "mobileMode": "tabs", "panelStyle": "glass", "minimalActionsPlacement": "outside", "accent": "#3b82f6" },
  "panels": [
    {
      "id": "pc-1",                                          // ^[A-Za-z0-9_-]{1,40}$，唯一
      "title": "艾拉",                                       // ≤ 80 字
      "rect": { "x": 0, "y": 0, "w": 6, "h": 8 },            // 必须完全落在网格内
      "source": { "type": "surface", "surface": "character", "resourceId": "snapshot:<channelId>:<identityId>" },
      "hiddenByDefault": false,
      "chrome": "minimal"                                    // 省略为标准外观
    }
  ]
}
```

新添加的 Panel 与当前内置预设中的 Panel 都明确使用 `chrome: 'minimal'`。复制继承原外观；旧布局或导入模板省略 `chrome` 时仍为标准外观，不做迁移。

极简 Panel 无边框、背景、阴影或占高的标题栏，内容铺满 Panel；标题和状态保留无障碍语义。右上悬浮操作区在 hover / 键盘焦点时显示，触屏常驻可操作。布局级 `settings.minimalActionsPlacement` 缺失、空值或 `inside` 表示内部覆盖；`outside` 表示右上角轻微外展，不改变 Grid rect。移动端退化为内部覆盖。服务端可将 `inside` 规范化为空值；该可选字段保持 layout version 1。布局面板风格和全局玻璃主题不为极简 Panel 添加卡片材质。

`source` 是资源描述，不是 URL：

| type | 字段 | 渲染 |
| --- | --- | --- |
| `surface` | `surface`（character / iform / note / clue / clue-board）、`resourceId`、可选 `channelId`、`label` | `generateInternalSurfaceLink` 生成的 Internal Surface iframe |
| `chat` | 可选 `channelId` | Embed Chat（与浮窗聊天相同的 embed 地址，不接管音频） |
| `theater` | 可选 `channelId` | `/theater?pip=0&audioOwner=0` iframe |
| `web` | `url`（http/https 绝对地址，无账号信息）、`display`（`embed` / `link`） | 沙箱 iframe 或链接卡片 |

`channelId` 为空表示布局频道自身。服务端严格校验（未知字段、越界、重叠面板、重复 ID、非法 URL 一律 400），surface / chat / theater 来源上的 `url` 字段会被丢弃，生成地址永远不会入库。高于服务端支持的 `version` 返回 400 并说明版本过新；升级 schema 时在 `migrateChannelWorkbenchLayout` 中按版本迁移。

## 3. 接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/v1/channels/:id/workbench` | 读取视图。非管理者只收到当前对其可见的面板，展示状态只含聚焦（不含操作者与显隐表） |
| PUT | `/api/v1/channels/:id/workbench/layout` | `{ layout, baseRevision }`，CAS；冲突返回 409 `{ message, layoutRevision }` |
| PUT | `/api/v1/channels/:id/workbench/mode` | `{ displayMode: '' \| 'workbench' }`，变化时广播频道树失效 |
| POST | `/api/v1/channels/:id/workbench/presentation` | `{ action: 'visibility', panelId, visibility: 'shown'\|'hidden'\|'default' }`、`{ action: 'spotlight', panelId, reveal? }`、`{ action: 'clear-spotlight' }`、`{ action: 'reset' }` |

WS 事件 `channel-workbench-updated`（`channelWorkbench: { channelId, layoutRevision, presentationRevision, reason, operatorId }`）只是失效通知，客户端重新 GET，隐藏面板不会经广播泄露。聚焦隐藏面板必须带 `reveal: true`，会同时把它设为对玩家显示；隐藏正在聚焦的面板会结束聚焦。复制频道时复制布局与展示形态，不复制展示状态。

隐藏只是展示控制，不是资源权限：被引用的资源仍由各自的 Internal Surface / Embed 权限检查。

## 4. 导入导出

```jsonc
{
  "kind": "sealchat.workbench",
  "version": 1,
  "exportType": "template",          // template：去掉 resourceId / channelId / label；design：完整资源描述
  "exportedAt": "2026-11-15T00:00:00.000Z",
  "name": "角色卡墙",
  "source": { "worldId": "...", "channelId": "...", "channelName": "..." },  // 仅 design
  "layout": { /* §2 */ }
}
```

- 导入只做宽松修复（越界夹回、重复 ID 重新生成、非法来源跳过并给出警告），不会因为资源缺失而失败；结果进入设计草稿，管理者确认后保存。
- 检测到重叠时尝试在最多 60 行内保持面板尺寸自动排列，并提示警告；仍放不下时保留问题布局及全部有效面板，由前端保存校验与服务端校验阻止保存，需手动调整。
- 未绑定或引用了不可见频道 / 资源的面板在设计画布中标记“待绑定”，检查器提供资源目录与手动输入以重新绑定。
- 预设“角色卡墙”“地图 + 聊天”“空白工作台”就是内置的 template 文档，走同一条导入路径。
- 导出不包含生成的内部链接、令牌或任何凭据。

## 5. 外部网页安全边界

外部网页面板使用 `sandbox="allow-same-origin allow-scripts allow-forms allow-pointer-lock allow-popups"`、`referrerpolicy="no-referrer"`，并拒绝与 SealChat 同源的地址，因此网页不会获得 SealChat 登录态、API 或内部权限，也不会接入 Channel Embed Bridge（需要 Bridge 的内容请使用 IForm 面板，沿用 IForm 的授权模型）。拒绝被嵌入（X-Frame-Options / CSP）的网站不做任何绕过：长时间未加载时提示“新窗口打开”，也可以把面板设为链接卡片。

## 6. 界面自定义

布局频道沿用 `chat` Surface。正式的 CSS 与脚本接口说明见 `doc/ui-customization-package/ui-customization-package-development-guide.md` 的“布局频道（Workbench）”小节。

稳定接口包括 `data-sc-ui="workbench"`、`workbench-toolbar`、`workbench-spotlight`、`workbench-design-canvas`、`workbench-inspector` 和 `workbench-panel`。工作台根元素还提供 `data-sc-workbench-mode`、`data-sc-workbench-layout`、`data-sc-panel-style`、`data-sc-minimal-actions-placement` 和 `data-sc-immersive`；Panel 提供 `data-sc-panel-id`、`data-sc-panel-type`、`data-sc-panel-state` 和 `data-sc-actions-placement`。视觉变量统一使用 `--sc-workbench-*`。

JavaScript 包可通过 `sc.context().workbench` 读取当前工作台状态，并监听 `workbench-change`。聊天、小剧场和 Internal Surface Panel 的样式边界以各自 Surface 为准，父级工作台 CSS 不穿透 iframe。
