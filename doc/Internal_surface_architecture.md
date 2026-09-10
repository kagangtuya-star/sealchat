# SealChat Internal Surface 独立运行环境

## 1. 定位

`Internal Surface` 是 SealChat 面向内部业务模块的独立运行环境。

它让一个业务资源脱离聊天主界面运行，同时继续复用 SealChat 原有的登录态、Vue/Pinia Runtime、主题、Store、API 和实时通信能力。

```text
业务资源
   │
   ▼
Internal Surface URL
   │
   ▼
InternalSurfaceView
   ├─ World Context
   ├─ Channel Context
   ├─ Loading / Error
   └─ Surface Adapter
          │
          ▼
      原业务 Store / API / WS
```

同一个 Internal Surface 可以被多个宿主复用：

```text
Internal Surface
   ├─ 独立浏览器窗口
   ├─ 普通聊天页面悬浮窗
   └─ 小剧场舞台悬浮窗
```

核心原则：

> 业务模块只实现一个 Internal Surface；聊天、小剧场和独立窗口只是它的不同宿主。

Internal Surface 是 SealChat 原生运行环境，不等同于第三方 iframe。Surface 内部如果需要继续嵌入外部网页，再使用现有 Channel Embed Bridge。

---

## 2. 路由与资源协议

通用路由：

```text
/#/internal/:type/:id?world=:worldId&channel=:channelId
```

例如：

```text
/#/internal/note/NOTE_ID?world=WORLD_ID&channel=CHANNEL_ID
```

类型和链接协议定义于：

```text
ui/src/utils/internalSurfaceLink.ts
```

当前通用类型：

```ts
['iform', 'note', 'character', 'clue']
```

一个资源由以下四项确定：

```ts
{
  type,
  id,
  worldId,
  channelId
}
```

其中：

- `type`：Surface 类型。
- `id`：业务资源 ID。
- `worldId`：所属世界。
- `channelId`：运行所需频道上下文。

资源唯一 Key 使用：

```ts
buildInternalSurfaceResourceKey(...)
```

当前格式为：

```text
type:id:worldId:channelId
```

链接统一使用：

```ts
generateInternalSurfaceLink(...)
resolveInternalSurfaceLinkBase(...)
parseInternalSurfaceLink(...)
```

不要手工拼接 Internal Surface URL 或自行定义另一套资源 Key。

---

## 3. Runtime Host

通用 Runtime Host：

```text
ui/src/views/internal/InternalSurfaceView.vue
```

它负责建立 Surface 所需的 SealChat 上下文：

```text
ensureWorldReady()
    ↓
switchWorld()
    ↓
channelSwitchTo()
    ↓
校验最终 world/channel
    ↓
加载业务 Surface
```

`InternalSurfaceView` 负责：

- 解析路由。
- 校验 `type/id/world/channel`。
- 初始化世界和频道上下文。
- 防止异步旧任务覆盖新路由状态。
- 统一 Loading。
- 统一错误和资源不可用状态。
- 挂载具体 Surface。

因此业务 Surface 不应重复执行 `switchWorld()` 或 `channelSwitchTo()`。

当前 Runtime 使用 epoch/queue 控制异步上下文初始化，新增 Surface 时也应保持相同的 stale-result 防护原则。

---

## 4. Surface 标准接口

普通 Surface 注册于：

```text
ui/src/views/internal/internalSurfaceRegistry.ts
```

Surface Adapter 默认接收：

```ts
defineProps<{
  resourceId: string
  worldId: string
  channelId: string
}>()
```

统一向 Runtime Host 返回：

```ts
defineEmits<{
  ready: []
  unavailable: [message: string]
  error: [message: string]
}>()
```

语义固定：

### `ready`

资源已经完成必要初始化，可以正常显示。

### `unavailable`

请求本身合法，但资源不存在、已经删除或当前用户不可访问。

### `error`

资源理论上可以加载，但 Store、API 或 Runtime 初始化失败。

Loading 和通用错误 UI 由 `InternalSurfaceView` 负责，普通 Surface 不需要再次实现一套错误页面。

---

## 5. 当前四种接入范式

现有实现已经覆盖四种主要业务模式。

| 类型 | 实现方式 | 适用场景 |
| --- | --- | --- |
| `iform` | Component / Embed Adapter | 已有组件或内部还需嵌套 iframe |
| `note` | Realtime Store Adapter | Store + WebSocket 实时资源 |
| `character` | Runtime Adapter | 存在临时 Runtime 和副作用 |
| `clue` | Presentation Adapter | 已有成熟展示层 |

### IForm

入口：

```text
ui/src/views/internal/surfaces/IFormInternalSurface.vue
```

直接复用：

```text
IFormEmbedFrame.vue
```

Surface 只负责根据 `channelId/resourceId` 找到业务资源并报告生命周期。

如果 IForm 内部继续嵌入第三方页面，则由已有 Channel Embed Bridge 负责第三方 iframe 通信。

因此：

```text
Internal Surface ≠ Channel Embed Bridge
```

前者负责 SealChat 原生模块独立运行，后者负责与外部嵌入页面通信。

### Sticky Note

入口：

```text
ui/src/views/internal/surfaces/StickyNoteInternalSurface.vue
```

继续使用原有：

```text
StickyNote Store
chatEvent
```

并监听便签的创建、更新、删除和推送事件。

这说明 Surface 不应创建第二个 WebSocket 或第二套实时状态源。

组件卸载时必须解除事件监听。

### Character

入口：

```text
ui/src/views/internal/surfaces/CharacterInternalSurface.vue
```

人物卡需要创建临时 Character Sheet Runtime，并可能临时切换 active card。

因此 Surface 自己负责：

```text
创建临时 Runtime
→ 使用资源
→ 关闭 Runtime
→ 恢复原 active card / nickname
```

这类临时业务副作用应由对应 Surface Adapter 自己管理，而不是放入 `InternalSurfaceView`。

### Clue

线索直接复用：

```text
WorldCluePresentationOverlay.vue
```

并增加：

```text
mode="surface"
```

用于去除普通全屏 Presentation 不适用于独立窗口的外层行为。

这是推荐模式：

> 已有成熟业务 UI 时优先增加 Surface 模式，不复制一份新的组件。

目前 `clue` 在 `InternalSurfaceView` 中存在轻量特殊分支。它是当前实现的最小方案，不应成为后续模块继续向 Host 添加 `isFooSurface` 的模板。

新模块默认仍应使用 Registry + Adapter。

---

## 6. 浮窗宿主

Internal Surface 与浮窗系统通过统一资源描述连接。

定义位于：

```text
ui/src/utils/theaterFloatingBridge.ts
```

核心结构：

```ts
interface TheaterFloatingResource {
  key: string
  url: string
  title: string

  presentation?: {
    chrome?: 'default' | 'minimal'
    minimized?: boolean
    avatarUrl?: string
    width?: number
    height?: number
  }
}
```

浮窗 Host：

```text
ui/src/views/theater/host/TheaterFloatingHost.vue
```

当前支持两种运行模式。

### 普通页面

```ts
hostMode="viewport"
```

Host 覆盖当前 SealChat 浏览器视口。

### 小剧场

```ts
hostMode="stage"
```

Host 位于小剧场舞台层。

小剧场 iframe 与舞台 Host 之间通过：

```text
theaterFloatingBridge.ts
```

进行同源 `postMessage` 通信，并区分：

```ts
intent: 'open'
intent: 'transfer'
```

即直接打开资源和拖拽转移资源。

---

## 7. 独立浏览器窗口

Internal Surface 也可以不经过浮窗 Host，直接打开：

```ts
openInternalSurfaceLink(url, {
  width,
  height,
})
```

桌面环境会创建可调整大小的浏览器窗口，移动端则使用普通新标签页行为。

因此业务模块无需实现：

```text
普通页面版本
小剧场版本
独立窗口版本
```

只需要提供同一个 Internal Surface URL。

---

## 8. 新模块接入

假设新增业务模块：

```text
foo
```

### 第一步：注册类型

修改：

```text
ui/src/utils/internalSurfaceLink.ts
```

例如：

```ts
export const INTERNAL_SURFACE_TYPES = [
  'iform',
  'note',
  'character',
  'clue',
  'foo',
] as const
```

### 第二步：创建 Adapter

新增：

```text
ui/src/views/internal/surfaces/FooInternalSurface.vue
```

基本结构：

```ts
const props = defineProps<{
  resourceId: string
  worldId: string
  channelId: string
}>()

const emit = defineEmits<{
  ready: []
  unavailable: [message: string]
  error: [message: string]
}>()
```

Adapter 负责：

```text
加载业务资源
订阅必要实时事件
复用现有业务组件
管理业务 Runtime
清理自身副作用
```

但不负责 world/channel 初始化。

### 第三步：注册组件

修改：

```text
ui/src/views/internal/internalSurfaceRegistry.ts
```

例如：

```ts
foo: defineAsyncComponent(
  () => import('./surfaces/FooInternalSurface.vue')
)
```

### 第四步：生成资源链接

```ts
const params = {
  type: 'foo' as const,
  id: resource.id,
  worldId,
  channelId,
}

const url = generateInternalSurfaceLink(params, {
  base: resolveInternalSurfaceLinkBase(config),
})

const key = buildInternalSurfaceResourceKey(params)
```

### 第五步：需要浮窗时包装资源

```ts
const floatingResource = {
  key,
  url,
  title: resource.title,
  presentation: {
    width: 720,
    height: 560,
  },
}
```

之后同一个 `floatingResource.url` 可以进入 viewport、stage 或独立浏览器窗口。

---

## 9. Runtime Host 扩展边界

`InternalSurfaceView.vue` 应保持为 Runtime Host，而不是逐渐演变成另一个业务页面。

不要因为某个模块存在特殊加载逻辑，就直接把业务 Store、API 和大量模板加入：

```text
InternalSurfaceView.vue
```

默认处理顺序应该是：

```text
业务特殊性
   ↓
Surface Adapter
   ↓
已有业务组件 / Store / Runtime
```

只有真正属于所有 Surface 的能力才进入公共 Runtime，例如：

```text
world/channel 初始化
统一 Loading
统一错误状态
通用生命周期协议
```

同样：

```text
TheaterFloatingHost.vue
```

只负责窗口层行为：

```text
位置
尺寸
z-index
拖动
调整大小
最小化
chrome
持久化
Surface URL
窗口转移
```

不要把具体业务权限、资源请求或业务状态管理加入 Window Host。

---

## 10. 状态与实时通信原则

Internal Surface 不建立新的业务数据体系。

正确的数据关系是：

```text
原业务 API
    ↓
原业务 Store
    ↓
Internal Surface Adapter
    ↓
业务 Component
```

需要实时数据时继续复用：

```text
现有 WebSocket
现有 chatEvent
现有 Store event handler
```

不要创建：

```text
InternalSurfaceFooStore
```

来复制已有 `FooStore` 的数据，也不要单独建立一个 Surface WebSocket。

Surface 是新的展示和运行入口，不是新的业务数据源。

---

## 11. 生命周期要求

所有 Surface 都必须考虑路由或资源快速变化。

异步操作至少需要保证旧任务不会覆盖新资源。

典型方式：

```ts
const epoch = ++taskEpoch

const result = await loadResource()

if (epoch !== taskEpoch) return
```

组件卸载时：

```ts
taskEpoch += 1
```

如果注册了：

```text
chatEvent
window event
observer
temporary runtime
```

必须在卸载阶段配对释放。

如果 Surface 临时修改了业务状态，例如人物卡 active state，也必须在退出后恢复。

---

## 12. 权限边界

Internal Surface URL 是：

```text
资源地址 + 运行上下文
```

不是权限凭据。

例如：

```text
/#/internal/clue/xxx?world=xxx&channel=xxx
```

知道 URL 不代表自动拥有查看权限。

实际权限仍由：

```text
登录态
业务 API
Store
World / Channel 权限
具体资源权限
```

决定。

Surface 应沿用原业务接口的权限判断，不额外设计一套前端授权系统。

---

## 13. 浮窗持久化与敏感资源

Window Host 可以记忆：

```text
位置
尺寸
最小化状态
层级
窗口类型
presentation
```

但需要注意多账号使用同一浏览器的情况。

对于线索等敏感资源，当前实现已经考虑用户身份隔离。

后续如果新增具有类似隐私属性的 Surface，需要明确判断：

```text
该窗口是否允许跨登录用户恢复？
```

如果不能，应采用与线索相同的账号隔离策略，而不是仅按：

```text
worldId + channelId
```

恢复。

---

## 14. 旧嵌入协议兼容性

Internal Surface 是新的原生独立运行入口，不代表已有 Embed Link 被废弃。

目前仍存在：

```text
/#/:worldId/:channelId?iform=...
/#/:worldId/:channelId?snote=...
/#/:worldId/:channelId?clue=...
```

对应代码包括：

```text
ui/src/utils/iformEmbedLink.ts
ui/src/utils/stickyNoteEmbedLink.ts
ui/src/utils/worldClueEmbedLink.ts
```

这些链接可能已经用于：

```text
历史消息
富文本
分享链接
已有组件
外部调用
```

因此属于兼容性接口。

新增独立运行能力时优先使用 Internal Surface，但不要因为接入 Internal Surface 就删除或修改旧 URL 协议。

需要废弃旧协议时应单独设计迁移过程。

---

## 15. 不建议的实现

不要为每个功能新建：

```text
FooPopup.vue
FooTheater.vue
FooStandalone.vue
FooEmbed.vue
```

如果它们展示的是同一个业务资源，应首先考虑一个：

```text
FooInternalSurface.vue
```

也不要过早建设：

```text
Surface Plugin Framework
Surface DI Container
Surface Lifecycle Framework
Surface Capability System
```

当前 `Type + Registry + Adapter + Host` 已足够覆盖主要场景。

只有未来多个 Surface 真正出现重复元数据需求时，再考虑把 Registry 从：

```ts
type -> Component
```

轻量扩展为：

```ts
interface InternalSurfaceDescriptor {
  component: Component
  chrome?: 'default' | 'minimal'
}
```

不要预先建立复杂框架。

---

## 16. 新模块接入检查

新增 Internal Surface 完成前确认：

- 已加入 `INTERNAL_SURFACE_TYPES`。
- 使用 `generateInternalSurfaceLink()`。
- 使用 `buildInternalSurfaceResourceKey()`。
- 未手工实现另一套 Internal URL。
- Surface 使用 `resourceId/worldId/channelId` 标准参数。
- 使用 `ready/unavailable/error` 生命周期。
- 复用已有 Store/API/WS。
- 没有在 Surface 内重复切换 world/channel。
- 异步加载具有 stale guard。
- Event/Observer/临时 Runtime 能在卸载时释放。
- 删除或失去权限后能进入 `unavailable`。
- 已有成熟 UI 时优先复用，而非复制。
- viewport、stage、独立窗口使用同一 Surface URL。
- Window Host 中没有加入业务逻辑。
- 未破坏已有 Embed Link 和历史入口。
- 敏感窗口持久化已考虑账号隔离。

---

## 17. 主要代码入口

Internal Surface：

```text
ui/src/utils/internalSurfaceLink.ts
ui/src/router/index.ts
ui/src/views/internal/InternalSurfaceView.vue
ui/src/views/internal/internalSurfaceRegistry.ts
ui/src/views/internal/surfaces/
```

窗口系统：

```text
ui/src/utils/theaterFloatingBridge.ts
ui/src/views/theater/host/TheaterFloatingHost.vue
```

现有参考实现：

```text
ui/src/views/internal/surfaces/IFormInternalSurface.vue
ui/src/views/internal/surfaces/StickyNoteInternalSurface.vue
ui/src/views/internal/surfaces/CharacterInternalSurface.vue
ui/src/components/world-clue/WorldCluePresentationOverlay.vue
```

旧 Embed Link：

```text
ui/src/utils/iformEmbedLink.ts
ui/src/utils/stickyNoteEmbedLink.ts
ui/src/utils/worldClueEmbedLink.ts
```

修改前同时阅读对应目录的：

```text
AGENTS.md
ui/AGENTS.md
ui/src/views/chat/AGENTS.md   # 涉及聊天宿主时
```

---

## 18. 最终原则

Internal Surface 的职责可以归纳为：

```text
同一个业务资源
      ↓
同一个 Surface
      ↓
多个 Host
```

业务数据仍属于原业务模块，Surface 只负责让该业务资源能够在完整 SealChat Runtime 中独立运行。

因此后续新增可独立展示、可悬浮、可进入小剧场或可弹出的 SealChat 原生模块时，应优先判断：

> 能否先将该模块实现为一个 Internal Surface，再由不同 Host 复用？

如果答案是可以，就不应重新实现另一套独立页面体系。

## 19 嵌套内部界面框架问题

SealChat 使用 Hash 路由。当运行在 iframe 中的页面再次打开另一个 Internal Surface iframe 时，不要只依赖 Hash 来区分子文档：Chromium 在执行自引用检查时，可能会比较不包含 URL Fragment（`#...`）的祖先页面 URL，从而导致嵌套 iframe 最终停留在 `about:blank`而无法显示。

对于嵌套的 Internal Surface 链接，应保留 `internalSurfaceLink.ts` 中现有的、用于保障 iframe 安全的 Hash 前文档标记。仅当当前页面本身已经运行在 iframe 内时应用该标记；普通顶层页面生成的 Internal Surface URL 不应添加此标记。