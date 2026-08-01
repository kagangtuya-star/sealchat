# 小剧场对话框与角色演出资源设计方案

状态：Phase 5 已实现，待评审

基线：2026-07-16 仓库代码

关联协议：`doc/theater-bridge-protocol.md`

目标：为小剧场增加实时对话框、角色立绘、立绘装饰、对话框装饰，以及可复用的同步编辑器。

## 1. 结论

建议先实现统一的“演出外观模型 + 同步编辑控制器”，再接入角色编辑、差分、Bridge 和小剧场运行时。

关键设计：

1. 编辑器左右同步不是 Theater Bridge 问题。同一个 Vue 页面内应只有一份响应式草稿；左侧预览拖拽和右侧表单都向同一个控制器提交 typed command。不要通过两个 watcher 互相复制，也不要使用 `postMessage`。
2. Theater Bridge 只负责运行时消息。新增 `chat.message.created/updated/removed` 事件，并在 payload 内携带发送时冻结的角色演出外观，避免角色随后被修改或切换差分导致队列中的消息变脸。
3. 频道角色保存完整 `theaterPresentation`；头像差分保存 `theaterPresentationPatch`，使用“省略继承、`null` 清除、值覆盖”语义。
4. 消息表保存发送时解析后的 `sender_theater_presentation` JSON 快照。老消息为空，自动使用默认黑框且不显示自定义立绘。
5. 资源保存 attachment ID，不保存 URL。Bridge 输出时再解析为同源 URL，并严格校验 MIME、访问权、尺寸与层数。
6. 不能直接把频道角色资源挂到 `TheaterResourceModel`。该模型按 room 管理，受舞台权限、引用计数和 GC 约束；角色资源生命周期不同。应抽取现有探测/转码核心，新增频道角色演出资源流水线。
7. 小剧场运行时使用 DOM overlay，放在 Konva 舞台上方。对话文本、按钮、视频、无障碍和响应式布局都更适合 DOM；场景对象仍留在 Konva。

## 2. 当前代码事实

### 2.1 已有能力

- `ui/src/views/theater/bridge/theater-bridge-protocol.ts`
  - 已有严格 Zod 信封、256 KiB 限制、角色/差分快照、角色切换事件。
  - `CharacterAppearance` 已有名称、颜色、头像和头像装饰，适合扩展演出外观。
- `ui/src/views/embed/EmbedChatView.vue`
  - 是 chat endpoint；已负责生成角色快照并发给 stage。
- `ui/src/views/chat/chat.vue`
  - 已暴露 `getCharactersForTheater` 等方法。
  - 已消费 `chatEvent` 的 `message-created`，但没有把消息送入 Theater Bridge。
- `ui/src/bridge/sealchatBridgeSerializer.ts`
  - 已有 `normalizeBridgePlainText()` 和消息身份序列化逻辑，可抽成共享 serializer，避免第二套富文本转纯文本规则。
- `ui/src/components/avatar-decoration/AvatarDecorationEditor.vue`
  - 已实现多装饰、左侧拖拽、右侧数值编辑、透明 WEBM、静态兜底图。
  - 当前坐标范围与 68px 头像绑定，不能直接承载全舞台立绘和对话框。
- `ui/src/components/user-avatar-decoration.vue`
  - 已实现 PNG/WEBP/WEBM 分层渲染、透明 WEBM 能力判断、Safari 静态兜底和 viewport pause。
- `service/theater_media_probe.go`、`service/theater_media_worker.go`
  - 已实现 WebP 动图探测、透明 VP9 WebM 转码和派生附件生成。
  - 当前只服务 `TheaterResourceModel`。
- `model/MessageModel`
  - 已冻结发送时的 identity、variant、昵称、颜色、头像与头像装饰，适合增加演出外观快照。

### 2.2 缺口

- Theater Bridge 没有消息 created/updated/removed 事件。
- 频道角色与差分没有演出外观字段。
- 头像差分 `AppearanceJSON` 虽可塞任意 map，但服务层未对演出资源做 typed 校验。
- 现有 `AvatarEditor` 固定头像裁剪场景；立绘需要透明画布、保留原比例、旋转/缩放/位移，不应直接复用其输出约束。
- `/api/v1/upload` 可保存 WEBM，但不会把动画图片自动转为透明 WEBM。
- 小剧场舞台没有独立 overlay 队列与关闭/skip 状态机。

## 3. 范围

本方案包含：

- 实时播放当前 Theater 绑定频道的新场内消息。
- 默认黑色对话框、角色名、角色色、对话内容、立绘、skip、关闭。
- 频道角色基础演出外观编辑。
- 每个头像差分的演出外观 patch。
- 多层立绘装饰与一个对话框装饰。
- PNG、JPEG、静态/动画 WEBP、透明 WEBM 的上传与处理。
- 左侧直接操作和右侧 inspector 的实时双向同步。
- 消息编辑/删除对当前或排队项的修正。

首版不包含：

- 历史消息回放、进度条、自动翻页配音。
- 任意 HTML/Markdown 直传。TipTap JSON 可通过受控可选字段复用现有富文本渲染与文字演出层；其他格式仍只显示规范化纯文本。
- 跨频道或跨 Theater session 的队列恢复。
- 多人共同远程编辑同一个角色演出外观。
- 九宫格拉伸。演出媒体固定使用原生 `<img>/<video>` 与 `object-fit: cover`；需要不变形边框时再增加 9-slice schema。

## 4. 用户行为定义

### 4.1 消息进入规则

首版默认只播放同时满足以下条件的消息：

- 属于当前 `worldId/channelId/sessionId`。
- `event=message-created`。
- `icMode=ic`。
- 非私语。
- 未删除、未撤回、未归档。
- `contentText.trim()` 非空。
- 有频道角色 identity。没有 identity 的系统消息和场外闲聊不弹框。

这些规则应封装为 `shouldEnqueueTheaterDialogue()`，不能散落在组件 watcher 中。未来可增加频道级偏好，但首版不新增配置面板。

### 4.2 播放队列

- 第一条合格消息立即打开对话框。
- 播放中到达的新消息按 Bridge 接收顺序入队。
- 同一 `messageId` 使用 LRU 去重，保留最近 512 个 ID。
- 文本默认逐字显示；速度建议 32 字/秒，最短 400 ms，最长 8 s。
- 一条完成后停留 900 ms，再播放下一条。用户点击内容区可立即完成当前逐字效果。
- 队列上限 64。超过时丢弃最旧的等待项，保留当前项和最新消息。

### 4.3 skip

- 队列不为空：丢弃所有等待项，仅保留最新一条，并从头播放最新一条。
- 队列为空且当前仍在逐字显示：立即显示当前全文。
- 队列为空且当前已完整显示：关闭当前框，等待下一条新消息。
- skip 按钮使用 `PlayerSkipForward` 图标和 tooltip，不显示文字按钮。

### 4.4 关闭

- X 清空当前项和当时已排队项。
- 记录 `dismissedThroughSequence`。已经收到的队列项不能在关闭后再次弹出。
- 下一条新 `message-created` 到达时正常重新打开。
- 切换频道、重建 Bridge 或离开 Theater 时清空全部状态。

### 4.5 更新与删除

- `chat.message.updated` 命中当前项：替换文本；已显示进度按新文本长度 clamp。
- 命中等待项：原位更新，不改变顺序。
- `chat.message.removed` 命中当前项：立即播放下一项；无下一项则关闭。
- 命中等待项：从队列删除。

## 5. 视觉与布局

### 5.1 运行时结构

`TheaterDialogueOverlay.vue` 作为 `StageApp.vue` 根容器内绝对定位 sibling，位于 Konva canvas 上方、舞台编辑工具下方。

```text
theater-stage-app
  stage-canvas
  theater-dialogue-overlay
    portrait-stack
      portrait-base
      portrait-decoration[]
    dialogue-shell
      dialogue-frame-media | default-black-background
      speaker-name
      content
      skip-button
      close-button
```

默认样式：

- 对话框：`x=0.02, y=0.69, width=0.96, height=0.28`。
- 背景：`rgba(12, 12, 14, 0.94)`，1px 半透明边线。
- 圆角：4px。
- 角色名：左上，600 字重，颜色取消息冻结的 `color`；无合法颜色时用主题主文本色。
- 正文：纯文本，保留换行，超长内容在框内滚动；按钮区不被文字覆盖。
- skip 与 X：右上固定 36px 命中区，图标 20px。
- 立绘：默认左下，位于对话框后方；自定义 zIndex 可放到框前方，但按钮始终最高。
- 所有动画资源 `muted autoplay loop playsinline`，离开 viewport 或 overlay 关闭时 pause。

### 5.2 坐标系

持久化使用归一化坐标，不绑定某台设备像素：

```ts
interface TheaterTransform {
  x: number       // 相对 overlay viewport 宽度，允许 -1..2
  y: number       // 相对 overlay viewport 高度，允许 -1..2
  width: number   // 0.01..3
  height: number  // 0.01..3
  rotation: number // -180..180
  opacity: number // 0..1
  zIndex: number  // -100..100
}
```

归一化坐标适应 Theater 左侧宽度变化。编辑器和运行时必须调用同一个 `resolveTheaterTransformStyle()`，不能各自计算。

立绘装饰使用 `space='portrait'`，其 x/y/width/height 相对立绘根框。对话框装饰使用 `space='dialogue'`，默认铺满对话框。基础立绘与对话框根框使用 `space='viewport'`。

### 5.3 响应式约束

- overlay viewport 小于 560px 时，对话框最小高度 34%，正文和名字缩小一个固定档位；不使用 viewport 宽度计算 font-size。
- 编辑器在小剧场内通过临时预览消息直接投影到真实舞台；其他页面使用同一渲染组件提供全尺寸本地预览。
- 立绘和装饰默认 `pointer-events:none`；只有编辑模式打开 pointer events。
- 对话框最小点击区、safe area 和正文 padding 在变换后仍计算，不允许按钮被自定义 frame 遮住。

## 6. 数据模型

### 6.1 公共 schema

建议在 Go `protocol/theater_presentation.go` 与前端 `ui/src/types/theaterPresentation.ts` 定义同构类型，并为前端建立 Zod schema。

```ts
type TheaterMediaKind = 'static_image' | 'animated_image' | 'video'
type TheaterObjectFit = 'cover'

interface TheaterMediaRef {
  assetId: string
  resourceAttachmentId: string
  fallbackAttachmentId?: string
  mimeType: 'image/png' | 'image/webp' | 'video/webm'
  kind: TheaterMediaKind
  width: number
  height: number
  durationMs?: number
}

interface TheaterVisualLayer {
  id: string
  enabled: boolean
  media: TheaterMediaRef
  space: 'viewport' | 'portrait' | 'dialogue'
  transform: TheaterTransform
  fit: TheaterObjectFit
  playbackRate: number
  blendMode: string
}

interface TheaterDialogueStyle {
  transform: TheaterTransform
  frame: TheaterVisualLayer | null
  speaker: { enabled: boolean; transform: TheaterTransform }
  content: { enabled: boolean; transform: TheaterTransform }
  padding: { top: number; right: number; bottom: number; left: number }
  nameGap: number
  textAlign: 'left' | 'center' | 'right'
}

interface TheaterPresentation {
  schemaVersion: 2
  portrait: TheaterVisualLayer | null
  portraitDecorations: TheaterVisualLayer[]
  dialogue: TheaterDialogueStyle
}
```

约束：

- 最多 16 个立绘装饰。
- 每个 ID 1..128 字符，数组内唯一。
- frame 只能使用 `space='dialogue'`。
- portrait 只能使用 `space='viewport'`。
- portrait decoration 只能使用 `space='portrait'`。
- `blendMode` 首版白名单：`normal/multiply/screen/overlay`。
- 所有数值必须 finite；服务端 clamp 不代替校验，越界直接 400。
- `resourceAttachmentId/fallbackAttachmentId` 必须由服务端检查可访问性和 MIME。

### 6.2 差分 patch

```ts
interface TheaterPresentationPatch {
  portrait?: TheaterVisualLayer | null
  portraitDecorations?: TheaterVisualLayer[] | null
  dialogue?: TheaterDialogueStyle | null
}
```

合并规则：

- 字段省略：继承基础角色。
- 字段为 `null`：显式清除该部分；dialogue 为 `null` 时恢复系统默认黑框。
- 字段有值：完整替换该部分，不做数组按 ID 深合并。
- resolved 结果始终是完整 `TheaterPresentation`，dialogue 缺失时注入系统默认值。

完整替换比深层 patch 更适合编辑器保存：语义可预测，删除层不会留下幽灵字段。

### 6.3 DB 变更

`model.ChannelIdentityModel` 新增：

```go
TheaterPresentation protocol.TheaterPresentation `json:"theaterPresentation" gorm:"serializer:json;column:theater_presentation"`
```

`ChannelIdentityVariantModel.AppearanceJSON` 保留，增加受控 key：

```json
{
  "displayName": "...",
  "color": "...",
  "avatarAttachmentId": "...",
  "theaterPresentation": { "portrait": {}, "portraitDecorations": [], "dialogue": {} }
}
```

服务层必须把该 key 解析到 typed struct，验证后再写回；不能原样信任 `map[string]any`。

`model.MessageModel` 新增：

```go
SenderTheaterPresentation protocol.TheaterPresentation `json:"sender_theater_presentation,omitempty" gorm:"serializer:json;column:sender_theater_presentation"`
```

消息创建时，`ResolveChannelIdentityAppearance()` 同时解析基础角色和差分的演出外观，并写入消息快照。消息编辑不得重新解析角色外观。

### 6.4 资源模型

新增 `TheaterAppearanceAssetModel`，不要复用 room-scoped `TheaterResourceModel`：

```go
type TheaterAppearanceAssetModel struct {
  ID                   string
  ChannelID            string
  OwnerUserID          string
  SourceAttachmentID   string
  DisplayAttachmentID  string
  FallbackAttachmentID string
  Kind                 string
  MimeType             string
  Width                int
  Height               int
  DurationMS           int64
  Status               string // pending|processing|ready|failed
  Progress             float64
  FailureCode          string
  FailureMessage       string
}
```

引用删除策略：

- 保存角色/差分时只允许引用同频道且 owner 匹配的 ready asset。
- 删除角色、差分或替换资源时不立即删除附件。
- 每日 GC 扫描基础角色、差分 `AppearanceJSON` 和消息快照中的 asset ID。
- 消息快照仍引用的 asset 必须保留；因此首版可只做孤儿标记，不做物理删除，待消息保留策略接入后再开启删除。

## 7. 资源上传与转码 API

### 7.1 上传

```http
POST /api/v1/channels/:channelId/theater-appearance-assets
Content-Type: multipart/form-data

file=<binary>
purpose=portrait|portrait-decoration|dialogue-frame
identityId=<id>
variantId=<optional id>
targetUserId=<optional delegated user id>
```

权限复用 `resolveChannelIdentityActorFromRequest()`：操作者必须能编辑目标用户在该频道中的 identity；不要求 `stage.resource.upload`。

返回：

```json
{
  "asset": {
    "id": "...",
    "status": "pending",
    "progress": 0
  }
}
```

### 7.2 查询与删除

```http
GET    /api/v1/channels/:channelId/theater-appearance-assets/:assetId
DELETE /api/v1/channels/:channelId/theater-appearance-assets/:assetId
```

ready 响应包含最终 `TheaterMediaRef`。DELETE 只允许未被角色、差分或消息引用的 asset；被引用时返回 `409 ASSET_IN_USE`。

### 7.3 媒体处理

- PNG/JPEG：压缩为静态 WebP；保留 alpha。
- 静态 WebP：可直接作为 display；超尺寸时重编码。
- 动画 WebP/GIF：复用现有 `deriveTheaterAnimatedImageVariants()` 核心，输出透明 VP9 WebM，并生成静态 WebP fallback。
- 透明 WebM：探测无音频、VP8/VP9、alpha；满足限制时直用，否则重新编码。
- 普通带音频视频：首版拒绝。演出装饰不播放音轨。
- 最大边 4096px，display 最大边 1920px。
- duration 最大 60s，文件大小沿用服务端 attachment 上限，并增加解码像素/帧数限制。
- FFmpeg 不可用时静态资源仍可用；动画资源返回 `PROCESSOR_UNAVAILABLE`，UI 显示明确错误，不降级为未经校验的源文件。

实现方式：从 `theater_media_probe.go` 和 `theater_media_worker.go` 抽出无 DB 依赖的 `VisualMediaProcessor`。现有 TheaterResource worker 和新 AppearanceAsset worker 都调用它，避免复制 FFmpeg 参数。

## 8. 频道角色 API 变更

现有 create/update payload 增加：

```json
{
  "theaterPresentation": {
    "schemaVersion": 2,
    "portrait": null,
    "portraitDecorations": [],
    "dialogue": {}
  }
}
```

现有 variant create/update payload 增加显式字段：

```json
{
  "theaterPresentation": {
    "portrait": {},
    "portraitDecorations": null,
    "dialogue": null
  }
}
```

不要要求前端把 typed 演出结构塞进自由 `appearance` map；API handler 应有独立字段，然后由 service 合并进 `AppearanceJSON`。响应仍在 `appearance.theaterPresentation` 返回，兼容现有 variant 类型。

频道角色导入/导出、临时角色替换、委托编辑、跨频道同步必须同时处理 asset：

- 同实例跨频道复制：校验源附件访问权，创建目标 asset 引用或复制 attachment 所有权。
- JSON 导出：资源写入既有 identity asset bundle，manifest 增加 `theaterPresentation` 和每层资源文件。
- 老格式导入：字段缺失视为未配置。
- 临时角色改名生成新 identity 时，演出外观与 asset 引用必须随角色迁移；现有“差分不迁移”提示需补充演出外观行为。

## 9. Theater Bridge 1.0 扩展

保持 `protocol/version` 为 `sealchat.theater/1.0`。新增字段均为新 message schema，不改变信封；角色外观字段以 optional 方式扩展，兼容同仓单体前后端同步发布。

### 9.1 capability

`THEATER_CHAT_CAPABILITIES` 增加：

```text
chat.message.created
chat.message.updated
chat.message.removed
```

`chat.character.subscribe` 继续不承担消息订阅。消息事件在 `system.initialized` 后自动推送。

### 9.2 角色快照

`CharacterAppearance` 增加：

```ts
theaterPresentation: ResolvedTheaterPresentation | null
```

`CharacterAppearancePatch` 增加：

```ts
theaterPresentation?: TheaterPresentationPatch | null
```

所有 media URL 继续走 `isSafeStageImageUrl()`；Zod 对 mime、数组长度、transform 范围做 strict 校验。

### 9.3 消息事件 payload

```ts
interface TheaterDialogueMessagePayload {
  messageId: string
  createdAt: number
  displayOrder?: number
  icMode: 'ic' | 'ooc'
  isWhisper: boolean
  isArchived: boolean
  isDeleted: boolean
  contentText: string
  contentRichText?: string
  hasPerformanceContent?: boolean
  actor: {
    identityId: string | null
    variantId: string | null
    displayName: string
    color: string
    appearance: CharacterAppearance
  }
}
```

事件：

- `event:chat.message.created`：完整 payload。
- `event:chat.message.updated`：完整 payload，便于幂等替换。
- `event:chat.message.removed`：`{ messageId }`。

始终传纯文本 `contentText`，不传 HTML。原消息为合法 TipTap JSON 时，可附带原始 `contentRichText` 与 `hasPerformanceContent`；Stage 先复核 TipTap 格式，再复用 `RichTextContent`/`TwinLayerMessage` 和既有 DOMPurify 路径。`TheaterDialogueOverlay` 自身不使用 `v-html`。纯文本转换继续复用 `normalizeBridgePlainText()`，也是所有失败场景的可读降级。

### 9.4 路由

```text
chatEvent
  EmbedChatView serializer
  TheaterBridgeClient(chat)
  TheaterHostBridge
  TheaterBridgeClient(stage)
  TheaterDialogueQueue
  TheaterDialogueOverlay
```

- `EmbedChatView` 在 Theater mode 下订阅 `chatEvent`，且只发送当前 channel。
- 订阅必须保存具体 handler 并在 unmount/Bridge stop 时移除，禁止 `off(event, '*')` 影响其他消费者。
- Host 只接受 `chat -> stage`、上下文匹配且 chat 已 initialized 的消息事件。
- Host 不把消息事件放入现有“stage 发往离线 chat”的队列。chat 离线期间不会补历史消息。
- Stage 对 `messageId` 去重，因此 WebSocket 重连重复事件不会重复播放。

### 9.5 文档同步

实现协议时同步更新 `doc/theater-bridge-protocol.md`：

- 第 6 节 capability。
- 第 7 节新增 Chat message events。
- 第 8 节角色快照 schema。
- 第 10 节说明消息事件不做离线补发。
- 第 13 节补充纯文本和私语过滤边界。
- 第 16 节移除“无 Bridge 测试”缺口（测试完成后）。

## 10. 同步编辑器设计

### 10.1 组件边界

```text
TheaterPresentationEditorModal
  useTheaterPresentationEditor
    draft
    selection
    revision
    history
    dispatch(command)
  TheaterPresentationPreview
    TheaterDialogueOverlay previewMode
    TransformHandles
  TheaterPresentationInspector
    asset controls
    numeric transform controls
    layer list
```

`TheaterDialogueOverlay` 同时用于编辑预览与运行时，确保视觉一致。编辑态只额外渲染选中框和 resize/rotate handles。

### 10.2 编辑 command

```ts
type TheaterEditorCommand =
  | { type: 'select'; target: TheaterSelection }
  | { type: 'set-transform'; target: TheaterSelection; transform: Partial<TheaterTransform> }
  | { type: 'set-media'; target: TheaterSelection; media: TheaterMediaRef | null }
  | { type: 'add-decoration'; layer: TheaterVisualLayer }
  | { type: 'remove-decoration'; id: string }
  | { type: 'reorder-decoration'; id: string; beforeId: string | null }
  | { type: 'set-dialogue-padding'; padding: Partial<TheaterDialogueStyle['padding']> }
  | { type: 'reset-section'; section: 'portrait' | 'decorations' | 'dialogue' }
```

控制器规则：

- `draft` 是唯一真源。
- 左侧 pointer drag/resize 和右侧 input 都只调用 `dispatch()`。
- 每个 command 先过同一 normalize/validate 函数，再生成新 immutable state。
- `revision` 每次有效变更加 1。
- pointer move 使用 `requestAnimationFrame` 合并显示更新；pointerup 生成一个 undo history entry。
- 数值输入在 input 时实时更新，blur 时提交 history entry。
- 外层表单只在 modal 点击“应用”时接收最终 draft；取消不污染角色表单。
- 上传成功先加入 modal draft，角色真正保存前不写 identity。未保存 asset 由 orphan GC 回收。

不需要额外事件协议。typed command 是组件内部协议；未来若做多人编辑，再把 command 包入有 revision 的后端 mutation。

### 10.3 左右同步

左侧操作：

- 点击选择立绘、框体或某个装饰层。
- 拖拽修改 x/y。
- 四角 resize 修改 width/height；Shift 锁定资源原比例。
- 旋转 handle 修改 rotation。
- 滚轮在选中对象上调整统一 scale 可选；首版可不提供，避免与页面滚动冲突。

右侧操作：

- x/y/width/height、rotation、opacity 数值输入。
- fit、blendMode、playbackRate 控件。
- 图层启用、排序、删除、重置。
- 预览 viewport preset。
- 差分模式显示“继承基础角色 / 自定义 / 清除”三态控制。

双方共享 `selection` 和 `draft`，不需要防 watcher 回环。

### 10.4 图片简易编辑器

上传立绘后先进入 `TheaterMediaPrepareDialog`：

- 静态图：裁剪透明边缘、旋转、水平/垂直翻转、裁剪框、输出 WebP。
- 动画图/WEBM：不做逐帧裁剪；只提供首帧预览、整体裁剪参数和转码确认。裁剪参数提交服务端 FFmpeg。
- “保持完整画布”作为默认，避免立绘被头像式正方形裁掉。
- 完成后进入同步编辑器调整舞台位置和大小。

可复用 `MessageImageEditor` 的静态图处理 composable，但不要复用其聊天消息 UI。`AvatarEditor` 的正方形和最小尺寸规则不适用。

## 11. 频道角色 UI

### 11.1 基础角色

在 `编辑频道角色` 的“头像装饰”之后增加：

- 按钮：`编辑小剧场立绘`。
- 状态：`已配置` / `未配置`。
- 说明：仅用于小剧场消息演出，不影响频道消息头像。

按钮打开 `TheaterPresentationEditorModal`，传入基础角色完整 presentation。

为避免“立绘装饰”和“对话框装饰”产生多个嵌套 modal，编辑器内部使用 tabs：

- `立绘`
- `立绘装饰`
- `对话框`

### 11.2 头像差分

每个差分编辑 dialog 增加同名按钮，传入：

- 基础角色 resolved presentation，作为只读 inherited base。
- 当前差分 patch，作为 editable draft。
- 模式 `variant`。

编辑器每个 section 提供三态：

- 继承角色设置。
- 使用差分设置。
- 清除此项。

差分保存时，把 typed patch 放入独立 API 字段。角色或差分切换后，现有 `chat.character.variant.selected` 全快照会立即刷新舞台预览。

### 11.3 立绘装饰

- 多层叠加，最多 16 层。
- 每层支持 PNG、WEBP、透明 WEBM 和静态 fallback。
- 相对立绘本地坐标编辑；基础立绘移动时装饰一起移动。
- zIndex 在装饰列表内排序；允许负值把装饰放在立绘后方。
- 不复用 `AvatarDecorationSettings` 的 `offsetX=-128..128` 限制；复用媒体渲染和 fallback 策略。

### 11.4 对话框装饰

- 一个 frame 资源，位于对话框内容后方。
- 上传和转码规则同立绘装饰。
- 调整的是整个 dialogue shell 的 viewport transform；名字、正文、skip、关闭跟随移动。
- inspector 可调四边 padding，解决自定义边框遮住文字问题。
- 资源加载失败、浏览器不支持透明 WebM 或用户偏好静态装饰时，先用 fallback；仍失败则退回默认黑框，绝不留下透明不可读文本。

## 12. 服务端解析与安全

- 所有角色/差分写入继续经过现有 owner/operator/channel 三方访问校验。
- 每个 asset ID 必须关联目标 channel 和 owner；委托操作者可使用目标用户已有 asset，但不能引用操作者私有的其他频道 asset，除非上传接口明确创建目标所有权 asset。
- 不信任客户端 MIME、宽高、duration、alpha 标记；全部服务端 probe。
- Bridge URL 限制同现有 `isSafeStageImageUrl()`，不允许任意 data URL、blob URL 或跨域 URL。
- `contentText` 只作为文本节点渲染。可选 TipTap JSON 只交给现有净化后的富文本组件；不接受 Bridge HTML。
- 私语在 chat endpoint 过滤，stage 再防御性过滤 `isWhisper=true`。
- 动画默认 muted；拒绝音频轨，避免隐藏音频绕过 Theater 音频 owner。
- 限制层数、消息长度、资源尺寸、帧数、时长、播放速率和 blendMode。
- Bridge 仍受 120 条/秒和 256 KiB 限制。消息事件不得内联 base64。

## 13. 状态与错误处理

编辑器：

- asset `pending/processing` 时显示进度，禁止“应用”该资源。
- `failed` 保留源文件错误码，允许重试转码或移除。
- 角色保存失败不关闭 editor draft。
- API 409 `ASSET_IN_USE` 只提示资源仍被引用，不从 UI 本地删层。

运行时：

- Bridge 未 initialized 时不展示旧消息。
- 资源加载失败按 primary、fallback、默认 UI 顺序降级。
- 队列中的单条 payload schema 错误只丢该条，不停止 Bridge。
- channel/session 变化立即 dispose media、timer、queue 和 dedupe set。
- `prefers-reduced-motion` 下取消逐字动画和装饰过渡；视频仍可由显示设置选择静态 fallback。

## 14. 实施顺序

### Phase 1：schema 与纯函数

- 增加 Go/TS/Zod 演出外观类型。
- 实现 default、normalize、validate、base+variant resolve。
- 增加 transform style、消息 eligibility、队列 reducer 纯函数。
- 单元测试先行。

### Phase 2：资源流水线

- 抽取 `VisualMediaProcessor`。
- 新增 appearance asset model、API、worker、权限和 GC 标记。
- 复用现有 FFmpeg 参数并补静态 fallback。

### Phase 3：持久化与角色编辑

- identity JSON column、variant typed patch、message snapshot column。
- API/store/types/import/export/sync 全链路。
- `TheaterPresentationEditorModal` 接入基础角色和差分。

### Phase 4：Bridge 消息事件

- 扩展 protocol schema/capability。
- 抽取共享消息 serializer。
- EmbedChatView 订阅 chatEvent。
- Host route 和 stage queue 接入。
- 同步更新 `doc/theater-bridge-protocol.md`。

### Phase 5：运行时 overlay

- DOM overlay、默认黑框、立绘/装饰/frame。
- skip/关闭/逐字/更新/删除状态机。
- 响应式、reduced motion、fallback。

实现状态：已完成。运行时使用单一 `TheaterDialogueRuntime` 驱动既有 queue reducer；Host typed callbacks 直接进入该实例，Bridge/channel/session 重建与卸载统一 reset/dispose。富文本消息复用现有 `RichTextContent` 与 `TwinLayerMessage`；普通消息继续按 Unicode code point 逐字显示。

### Phase 6：回归与发布

- 数据迁移、老消息/老角色兼容。
- 桌面、窄屏、Safari fallback、无 FFmpeg、Bridge 重连测试。
- 先用前端 feature flag `theaterDialogueEnabled` 灰度；数据字段可提前写入，不影响旧 UI。

## 15. 文件级改动清单

前端核心：

- `ui/src/types.ts`
- `ui/src/types/theaterPresentation.ts`（新增）
- `ui/src/views/theater/bridge/theater-bridge-protocol.ts`
- `ui/src/views/theater/bridge/theater-dialogue-queue.ts`（新增）
- `ui/src/views/theater/bridge/theater-character-snapshot.ts`
- `ui/src/views/embed/EmbedChatView.vue`
- `ui/src/views/chat/chat.vue`
- `ui/src/bridge/sealchatBridgeSerializer.ts`
- `ui/src/views/theater/host/TheaterView.vue`
- `ui/src/views/theater/stage/StageApp.vue`
- `ui/src/views/theater/dialogue/TheaterDialogueOverlay.vue`（新增）
- `ui/src/components/theater-presentation/TheaterPresentationEditorModal.vue`（新增）
- `ui/src/components/theater-presentation/TheaterPresentationPreview.vue`（新增）
- `ui/src/components/theater-presentation/TheaterPresentationInspector.vue`（新增）
- `ui/src/composables/useTheaterPresentationEditor.ts`（新增）
- `ui/src/stores/chat.ts`

后端核心：

- `protocol/theater_presentation.go`（新增）
- `protocol/protocol.go`
- `model/channel_identity.go`
- `model/channel_identity_variant.go`
- `model/message.go`
- `model/theater_appearance_asset.go`（新增）
- `service/channel_identity.go`
- `service/channel_identity_variant.go`
- `service/theater_appearance.go`（新增）
- `service/visual_media_processor.go`（新增/从 theater media 抽取）
- `api/channel_identity.go`
- `api/channel_identity_variant.go`
- `api/theater_appearance_asset.go`（新增）
- `api/chat_api_message.go`
- `api/api_bind.go`

文档：

- `doc/theater-bridge-protocol.md`
- 本文档

## 16. 测试方案

### 16.1 Go

- identity 基础 presentation create/update/list round trip。
- variant 省略/`null`/覆盖三态解析。
- resolved appearance 与消息快照冻结。
- 委托编辑附件访问权。
- 不同 channel/owner asset 注入拒绝。
- MIME、transform、层数、重复 ID、NaN/Inf 拒绝。
- animated WebP 转透明 WebM + fallback。
- FFmpeg 缺失、超时、坏文件、带音轨 WEBM。
- asset in-use 删除冲突与孤儿标记。
- SQLite/MySQL/PostgreSQL JSON serializer 迁移兼容。

### 16.2 前端纯函数/脚本测试

- Bridge Zod 正常与非法 payload。
- `chat.message.*` 上下文、source、target、大小校验。
- content plain-text serializer。
- 消息 eligibility。
- queue created/update/remove/dedupe/overflow。
- skip 三种状态、关闭后只等未来消息。
- base+variant patch resolve。
- transform clamp、preview/runtime style 一致。
- editor undo coalescing 和左右 dispatch 同步。

沿用现有 `ui/scripts/theater-*-runtime.test.ts` 形式补：

- `theater-dialogue-bridge-runtime.test.ts`
- `theater-dialogue-queue-runtime.test.ts`
- `theater-presentation-editor-runtime.test.ts`

### 16.3 浏览器验收

- 1440x900、1920x1080、窄屏 390x844。
- 基础角色和差分切换。
- PNG、静态 WEBP、动画 WEBP、透明 WEBM。
- 左拖拽后右值实时变；右输入后左预览同帧更新。
- 多装饰层顺序与差分继承。
- 自定义 frame 失败回退黑框。
- 消息 burst、编辑、删除、重连、频道切换。
- X 后已有队列不重开，下一条新消息重开。
- skip 落到最新消息。
- Safari/iOS 使用静态 fallback。
- canvas pixel/screenshot 检查非空、无重叠、按钮可点击。

## 17. 验收标准

- 新场内频道角色消息在 Theater stage 上显示冻结的昵称、颜色、立绘和正文。
- 默认无配置时始终有可读黑框。
- skip、关闭语义与第 4 节一致。
- 基础角色和每个差分都能编辑立绘、多个立绘装饰和对话框装饰。
- 左侧拖拽/resize 与右侧 inspector 共用单一状态，更新无延迟、无回环抖动。
- 动画 WebP 能生成透明 WebM，支持浏览器使用 fallback。
- 角色或差分修改不改变已发送消息的演出外观快照。
- 私语和场外消息默认不进入对话框。
- 频道/session 切换不串消息。
- 无新增外部前端依赖；复用 Vue、现有媒体组件和现有 FFmpeg 能力。

## 18. 评审时需要确认的产品决定

以下默认值已在方案中给出，实施前只需确认，不阻塞技术架构：

1. 首版是否坚持只播放 `ic`、非私语、有 identity 的消息。
2. skip 是“最新一条从头播放”（本方案）还是“最新一条立即显示全文”。
3. 对话框逐字速度与完成后停留时间是否需要用户设置；本方案首版固定，不增加设置项。
4. 对话框装饰首版是否接受普通拉伸；若美术资源要求边框不变形，需要在 Phase 1 直接加入 9-slice inset。
5. 消息快照长期保留 asset 会增加存储；本方案优先历史一致性，不在首版物理 GC 被消息引用的演出资源。
