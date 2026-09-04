# SealChat Channel Embed API 开发指南

> Embed API v1，协议命名空间：`sealchat.embed.*`
> 适用：频道 iForm、第三方 iframe
> 本文按 Embed API 对外契约整理。原文档保留不变。

## 1. 快速理解

嵌入页通过 `SealChatEmbed.connect()` 连接父级 SealChat。初次握手使用 `window.postMessage`，后续调用和事件走独立 `MessagePort`。

```js
const config = window.__SEALCHAT_EMBED_CONFIG__ || {}
const sealchat = await SealChatEmbed.connect({
  targetOrigin: config.hostOrigin
})

const context = await sealchat.context.get()
console.log(context.currentCharacter)
```

关键边界：

- iframe 不接触 SealChat token、cookie、原始 WebSocket 或内部 Store。
- SDK 复用宿主已有 WebSocket；嵌入页不要创建第二条 SealChat WebSocket。
- Session 固定绑定当前用户、频道和 iForm。调用方不能指定其他 `channelId`、`formId`。
- Storage 和 Event 作用域固定为 `(currentChannelId, formId)`。
- 切换频道、退出登录、关闭 iForm、策略变化等会使旧 Session 失效。
- `SealChatEmbed` 和旧 `sealchat.bridge.*` 相互独立。

## 2. 接入

### 2.1 内部接入

SealChat 后端提供无需登录的 SDK 地址。频道 `srcdoc` 嵌入会自动注入 `window.__SEALCHAT_EMBED_CONFIG__`；频道 iForm 使用单独 `<iframe src>` 时，宿主会自动追加 `hostOrigin` 与 `sdkUrl` 查询参数；脱离 SealChat 宿主的外部 URL iframe 则需自行提供实际地址：

```js
window.__SEALCHAT_EMBED_CONFIG__ = {
  hostOrigin: 'https://chat.example.com',
  sdkUrl: 'https://chat.example.com/api/v1/channel-embed-sdk.js'
}
```

脚本加载后提供全局对象：

```js
window.SealChatEmbed
window.SealChatEmbedError
```

不要自行打包或加载其他版本 SDK。若实例配置了 `WebUrl`，`sdkUrl` 必须包含对应前缀，例如 `/sealchat/api/v1/channel-embed-sdk.js`。不要依赖 `document.referrer`：频道 `srcdoc` 使用 `no-referrer`，无法从 referrer 推断宿主 origin。

接口：

```text
GET /api/v1/channel-embed-sdk.js
鉴权：无
Content-Type: application/javascript; charset=utf-8
```

连接：

```js
const sealchat = await SealChatEmbed.connect({
  timeoutMs: 10_000,
  targetOrigin: 'https://chat.example.com'
})
```

参数：

- `timeoutMs`：握手超时，默认 `10000`。
- `targetOrigin`：父级 SealChat origin。生产环境应传明确 origin；默认 `*` 只适合无法提前确定 origin 的场景。

`SealChatEmbed.connect()` resolve 表示 Embed Session 握手成功，但不要把“握手成功”等同于“已经收到一次连接状态事件”。新建 Session 时，宿主不保证主动补发 `connection.changed`；`connection.onChanged()` 用于监听后续变化，客户端应在握手成功后主动调用一次 `connection.getState()` 初始化 UI 和本地状态。推荐先注册监听，再读取初始状态，以避免只依赖事件导致状态栏长期停留在“连接中”。

iForm 管理员必须启用 Embed API、允许嵌入页 origin，并授予所需 Capability。`world.admins.read`、`characterCard.read` 和 `characterCard.write` 已加入新建 iForm 默认能力；旧 iForm 的已保存 policy 不会自动升级，请在编辑器的“能力”字段手工追加所需能力后保存。

连接断开分两类：

- SealChat WebSocket 暂时离线：保留 SDK Client，监听 `connection.onChanged()`；宿主负责 WebSocket 重连。
- Embed Session 失效：监听 `client.session.onClosed()`，丢弃旧 Client，再调用 `SealChatEmbed.connect()` 建立新 Session。重连必须重新握手，不能复用旧 `sessionId`。

### 2.2 外部接入SDK

Channel Embed API 不要求工具源码托管在 SealChat 内。部署在其他域名的第三方网页同样可以作为频道 iForm 的 iframe 工具接入 SealChat API。

外部工具需要加载当前 SealChat 实例提供的 SDK：

```html
<script src="https://chat.example.com/api/v1/channel-embed-sdk.js"></script>
```

如果 SealChat 配置了 `WebUrl` 前缀，应使用包含该前缀的实际 SDK 地址。

随后通过父级 SealChat 的 origin 建立 Embed Session：

```js
const sealchat = await SealChatEmbed.connect({
  targetOrigin: 'https://chat.example.com'
})

const context = await sealchat.context.get()
const currentCard = await sealchat.characterCard.getCurrent()
```

外部网址工具需要同时满足以下条件：

- 该网页通过 SealChat 的频道 iForm / iframe 打开；
- iForm 已启用 Embed API；
- iForm 的 `allowedOrigins` 允许外部网页所在的 origin；
- iForm 已授予工具实际需要的 Capability，例如 `context.read`、`characterCard.read`、`characterCard.write`、`storage.read` 等。

例如，工具部署在：

```text
https://tool.example.com
```

则应在对应 iForm 的 Embed API 策略中允许：

```text
https://tool.example.com
```

外部工具与内置工具使用完全相同的 SDK 和 Capability 模型。它不会获得 SealChat 的 Cookie、Token、原始 WebSocket 或内部 Store，只能调用宿主明确授予的 Embed API。

因此可以使用外部网页实现角色卡面板、战斗面板、地图、骰点工具、音乐控制器等功能，而无需为每个工具单独实现 SealChat 通讯桥接。

需要注意：SDK 不是独立的远程 SealChat 客户端。Embed Session 依赖父级 SealChat iframe 宿主完成握手，因此单独在普通浏览器标签页打开第三方网页，即使加载了 SDK，也无法直接建立 Embed Session。

如果外部工具需要同时兼容“SealChat 内嵌模式”和“普通网页独立模式”，可以检测是否存在宿主配置后再初始化 SDK：

```js
const config = window.__SEALCHAT_EMBED_CONFIG__

if (window.parent !== window && config?.hostOrigin) {
  const sealchat = await SealChatEmbed.connect({
    targetOrigin: config.hostOrigin
  })

  // SealChat 内嵌模式
} else {
  // 普通独立网页模式
}
```

对于通过 SealChat 创建的外部 URL iForm，宿主会提供 `hostOrigin` 与 `sdkUrl` 等接入信息；第三方工具应优先使用宿主提供的实际配置，不要硬编码某个固定 SealChat 实例地址。

## 3. API 一览

### 上下文与账号

| API | Capability | 结果 / 用途 |
| --- | --- | --- |
| `context.get()` | `context.read` | 聚合上下文：世界、频道、当前用户、成员、角色、连接、权限、Capability |
| `context.onChanged(handler)` | `context.read` | 上下文变化；返回取消订阅函数 |
| `user.getCurrent()` | `user.read` | 当前账号安全摘要，可能为 `null` |
| `member.getCurrent()` | `context.read` | 当前世界 / Guild 成员摘要，可能为 `null` |
| `channel.getState()` | `context.read` | 当前绑定的 `id`、`worldId`、`formId` |

常用结构：

```ts
interface SafeUser {
  id: string
  displayName: string
  avatar?: string
}

interface SafeMember {
  userId: string
  displayName: string
  avatar?: string
}

interface EmbedContext {
  world: { id: string; name?: string }
  guild?: { id: string; name?: string }
  channel: { id: string; name?: string; type?: string | number }
  currentUser: SafeUser | null
  currentMember: SafeMember | null
  currentCharacter: SafeCharacter | null
  connection: EmbedConnectionState
  permissions: EmbedPermissionSummary
  capabilities: string[]
  contextVersion: number
}
```

### 成员与角色

| API | Capability | 结果 / 用途 |
| --- | --- | --- |
| `members.list({ scope: 'online' })` | `members.read` | 当前频道在线成员数组 |
| `members.list({ scope: 'guild', cursor? })` | `members.read` | 当前 Guild 可见成员数组 |
| `members.list({ scope: 'world-admins' })` | `members.read` + `world.admins.read` | 当前 World 的 Owner/Admin 安全摘要 |
| `members.onChanged(handler)` | `members.read` | 在线成员变化；返回取消订阅函数 |
| `characters.list()` | `characters.read` | 当前频道可用角色数组 |
| `characters.getCurrent()` | `characters.read` | 当前激活角色；无法确定时返回 `null` |
| `characters.onChanged(handler)` | `characters.read` | 角色列表、激活角色或变体变化 |

```ts
interface SafeCharacter {
  id: string
  displayName: string
  name: string
  avatar?: string
  color?: string
  isActive: boolean
  activeVariant: {
    id: string
    displayName?: string
  } | null
  variants: Array<{
    id: string
    displayName?: string
  }>
}
```

`character` 是频道 identity，不是 Guild 权限 Role。不要在业务代码中把它命名为 `role`。

### 人物卡属性与人物卡快照

`characters.*` 是频道 identity API；`characterCard.*` 才是人物卡属性 API。两者命名空间和权限独立。

| API | Capability | 说明 |
| --- | --- | --- |
| `characterCard.getStatus()` | `characterCard.read` | 读取当前频道 BOT 人物卡 API 可用状态及禁用原因 |
| `characterCard.getCurrent()` | `characterCard.read` | 读取当前用户在当前频道的实时活动人物卡 |
| `characterCard.listSnapshots()` | `characterCard.read` | 读取当前频道已授权的人物卡快照 |
| `characterCard.getSnapshot({ identityId })` | `characterCard.read` | 按 `identityId` 读取当前频道单个快照 |
| `characterCard.updateAttrs(attrsPatch)` | `characterCard.write` | 更新当前登录用户自己的当前活动人物卡 |

当前用户实时活动人物卡的数据链路：

```text
Embed API
→ characterCard Store
→ character.get / character.set
→ 当前频道主控 BOT / SealDice
```

当前频道其他用户人物卡的数据链路：

```text
Embed API
→ ChannelCharacterSnapshot Store
→ character.snapshot.list
→ SealChat 服务端快照
```

`getCurrent()` 返回当前用户的实时 BOT 人物卡。BOT 人物卡 API 未开启、不支持或不可用时，`status.available` 为 `false`，`status.reason` 保留 SealChat 现有提示文本，`card` 为 `null`。这与 API 可用但没有活动人物卡是两种状态。

`listSnapshots()` 和 `getSnapshot()` 返回当前频道经服务端授权的快照。用户没有发布人物卡时，快照仍可能包含 identity 和用户信息，但 `card` 为 `null`。快照不会向 BOT 查询其他用户的实时人物卡。BOT 人物卡 API 未开启时，快照读取仍可工作；只有实时当前人物卡读取和写入受 availability 影响。

Embed API 不允许指定任意 `user_id`、`channelId` 或人物卡名称。`updateAttrs()` 只能修改当前登录用户自己的当前活动人物卡。无频道成员身份或处于 observer mode 时，`characterCard.write` 不会出现在 effective capabilities 中。Capability 不会跳过现有人物卡或频道权限检查。

`attrsPatch` 使用根级浅合并。`{ hp: 8 }` 只覆盖根属性 `hp`，不删除其他根属性。嵌套对象不做 deep merge；修改 `$忍神` 一类根节点时，调用方必须提交该根节点的完整对象。

```js
const client = await SealChatEmbed.connect({
  targetOrigin: HOST_ORIGIN
})

const status = await client.characterCard.getStatus()
const current = await client.characterCard.getCurrent()
const snapshots = await client.characterCard.listSnapshots()
const another = await client.characterCard.getSnapshot({
  identityId: 'identity-id'
})

await client.characterCard.updateAttrs({
  hp: 8,
  san: 42
})
```

不可用状态处理：

```js
const current = await client.characterCard.getCurrent()

if (!current.status.available) {
  console.log(current.status.reason)
}
```

旧 iForm 不会自动获得 `characterCard.read` / `characterCard.write`；管理员需在能力字段手工追加后保存。

### 权限与连接

| API | Capability | 结果 / 用途 |
| --- | --- | --- |
| `permissions.getCurrent()` | `permissions.read` | 当前 Embed 权限摘要 |
| `permissions.onChanged(handler)` | `permissions.read` | 权限变化 |
| `connection.getState()` | `context.read` | 读取当前连接状态；新建 Session 后应主动调用一次 |
| `connection.onChanged(handler)` | `context.read` | 监听后续连接状态变化；不保证订阅时补发当前状态 |
| `session.onClosed(handler)` | 无 | Session 失效或 Host 关闭时通知，需重新 `connect()` |

```ts
interface EmbedConnectionState {
  state: 'connected' | 'reconnecting' | 'offline'
  latencyMs?: number
}

interface EmbedPermissionSummary {
  canSendMessage: boolean
  canReadMembers: boolean
  canReadCharacters: boolean
  canReadCharacterCard: boolean
  canWriteCharacterCard: boolean
  canReadStorage: boolean
  canWriteStorage: boolean
  canPublishEvents: boolean
  worldRole: 'owner' | 'admin' | 'member' | 'spectator' | null
  isWorldOwner: boolean
  isWorldAdmin: boolean
  isSystemAdmin: boolean
  canManageWorld: boolean
}
```

连接状态初始化建议采用“先订阅、再主动读取”的模式：

```js
const offConnection = sealchat.connection.onChanged(state => {
  renderConnection(state)
})

// 新 Session 建立后不会保证收到一条初始 connection.changed。
// 必须主动读取当前状态，不能只等 onChanged。
const initialState = await sealchat.connection.getState()
renderConnection(initialState)
```

`connect()` 成功后如果 UI 仍显示“连接中”，而其他 API 已可调用，首先检查是否漏掉了这次初始 `getState()`；不要通过等待下一次 `connection.onChanged()` 来初始化状态。

```ts
interface EmbedWorldAdmin {
  userId: string
  displayName: string
  avatar?: string
  role: 'owner' | 'admin'
}
```

Capability 表示接口是否开放；Permission 摘要表示当前上下文中是否可执行。`isWorldAdmin` 只表示 World `owner/admin`，`isSystemAdmin` 表示系统 `mod_admin`，`canManageWorld` 为两者任一为真。不要用 `storage.write`、`messages.send` 反推管理员身份。权限可能运行时撤销，每次写操作仍由宿主和服务端校验；权限变化通过 `permissions.onChanged()` 或 `context.onChanged()` 更新。缺少 `world.admins.read` 时，管理员列表返回 `CAPABILITY_DENIED`。

### JSON Storage

Storage 是频道 + iForm 隔离的轻量 JSON KV。适合计分板、轮次、投票、共享面板状态；不适合文件、日志、聊天归档或高频事件流。

| API | Capability | 说明 |
| --- | --- | --- |
| `storage.get(key)` | `storage.read` | 读取；不存在返回 `null` |
| `storage.set(key, value, options?)` | `storage.write` | 新增或覆盖；更新已有文档时可用 `ifRevision` 做乐观锁 |
| `storage.delete(key, options?)` | `storage.write` | 删除；支持 `ifRevision` |
| `storage.list({ prefix?, cursor? })` | `storage.read` | 按前缀列出 |
| `storage.snapshot()` | `storage.read` | 获取当前 namespace 权威快照 |
| `storage.onChanged(handler)` | `storage.read` | 监听 `set`、`delete`、`resynced` |

```ts
interface StorageDocument<T = unknown> {
  key: string
  value: T
  revision: number
  seq: number
}

interface StorageMutationOptions {
  ifRevision?: number
}
```

普通写入：

```js
await sealchat.storage.set('board', { round: 1 })
const document = await sealchat.storage.get('board')
await sealchat.storage.delete('board')
```

`ifRevision` 只用于“基于一个已经存在的 Storage 文档版本进行更新或删除”。其类型是可选 `number`，不是 `number | null`。`storage.get(key)` 在 key 不存在时返回 `null`；这种首次创建场景应直接调用 `storage.set(key, value)`，省略 `options`。不要把 `null` 作为 `ifRevision` 传入，也不要依赖把 `undefined` 放进 options 后由传输层自动忽略。需要乐观锁时，应传入 `storage.get()` 返回文档中的实际 `revision`。同样地，调用 `storage.delete()` 并使用乐观锁时，也只应传入已存在文档的有效 `revision`。

```js
const current = await sealchat.storage.get('board')

if (current === null) {
  // 首次创建：省略 options，不要传 { ifRevision: null }
  await sealchat.storage.set('board', { round: 1 })
} else {
  // 更新已有文档：使用实际 revision 做乐观锁
  await sealchat.storage.set(
    'board',
    { ...current.value, round: current.value.round + 1 },
    { ifRevision: current.revision }
  )
}
```

不要写成下面这样：

```js
const current = await sealchat.storage.get('board')
const revision = current ? current.revision : null

// 错误：key 不存在时会显式发送 ifRevision: null，可能返回 INVALID_PARAMS。
await sealchat.storage.set('board', nextValue, { ifRevision: revision })
```

Embed API v1 没有在本契约中定义用 `null` 表示“仅当 key 不存在时创建”的语义。如果多个客户端可能并发首次创建同一 key，不要自行发明 revision 哨兵值；首次创建完成后应通过 Storage 变更事件或重新 `get()` / `snapshot()` 同步权威状态。

其他客户端先写入时，调用抛出 `REVISION_CONFLICT`。重新 `get()` 或 `snapshot()`，基于最新值决定是否重试。SDK 遇到 Storage `seq` 缺口会自动读取快照，并触发 `kind: 'resynced'`。

当前限制：key 最长 128 字符，单个 JSON 值最大 64 KiB；具体 namespace 配额和限流由服务端决定。

### 瞬时事件

| API | Capability | 说明 |
| --- | --- | --- |
| `events.publish(topic, payload)` | `events.publish` | 向当前频道 + 当前 iForm 发布瞬时事件 |
| `events.on(topic, handler)` | `events.subscribe` | 订阅 topic；返回取消订阅函数 |
| `events.off(topic, handler)` | `events.subscribe` | 使用原 handler 取消订阅 |

```js
const off = sealchat.events.on('dice.result', payload => {
  console.log(payload)
})

await sealchat.events.publish('dice.result', { value: 18 })
off()
```

Event 不持久化，刷新后不重放。topic 最长 64 字符，payload 最大 16 KiB。需要刷新后保留的数据用 Storage。

### 发送频道消息

| API | Capability | 说明 |
| --- | --- | --- |
| `messages.send(options)` | `messages.send` | 复用 SealChat 正常消息链路向当前频道发送消息 |

```ts
interface SendMessageOptions {
  text: string
  replyTo?: string
  identityId?: string
  identityVariantId?: string
  icMode?: 'ic' | 'ooc'
}
```

```js
await sealchat.messages.send({
  text: '本轮行动结束',
  icMode: 'ooc'
})
```

未指定 `identityId` 时使用当前激活角色。指定角色或变体时，必须属于当前频道且当前用户有权使用。

### Client 通用方法

命名空间 API 足够覆盖常规开发。底层方法用于封装或调试：

```ts
sealchat.request(method, params?, timeoutMs?)
sealchat.on(topic, handler)
sealchat.off(topic, handler)
sealchat.close()
```

`close()` 关闭当前 MessagePort，之后该 Client 的请求返回 `SESSION_EXPIRED`。

## 4. 错误处理

```js
try {
  const current = await sealchat.storage.get('board')
  if (current === null) {
    await sealchat.storage.set('board', nextValue)
  } else {
    await sealchat.storage.set('board', nextValue, {
      ifRevision: current.revision
    })
  }
} catch (error) {
  if (error instanceof SealChatEmbedError) {
    console.error(error.code, error.message, error.details)
  }
}
```

常见错误：

- `HANDSHAKE_FAILED`、`ORIGIN_DENIED`：连接或 origin 配置错误。
- `SESSION_EXPIRED`：旧 Session 不可用，重新连接。
- `CONTEXT_CHANGED`：上下文已变化；重新读取 Context，再重试当前操作。
- `CAPABILITY_DENIED`、`PERMISSION_DENIED`：iForm 未授权或当前用户无权限。
- `REVISION_CONFLICT`：Storage 乐观锁冲突。
- `WS_OFFLINE`、`TIMEOUT`：宿主连接不可用或调用超时。
- `INVALID_PARAMS`、`NOT_FOUND`：参数或方法错误。遇到 `INVALID_PARAMS` 应同时检查 `error.message` 和 `error.details`；Storage 常见误用之一是在 key 不存在时发送 `{ ifRevision: null }`。
- `PAYLOAD_TOO_LARGE`、`QUOTA_EXCEEDED`、`RATE_LIMITED`：超过服务端限制。

只对瞬时连接错误做退避重试。权限、参数、配额错误必须由配置或业务逻辑处理。`INVALID_PARAMS` 不应通过重复提交解决；应修正请求结构，并在调试阶段保留 `error.code`、`error.message`、`error.details`。

## 5. 最小 HTML 示例

白色背景，无框架。包含连接、Session 自动重连、连接巡检、当前角色、在线列表、当前权限、World 管理员列表、Storage 写入/读取/删除。

SDK 由 SealChat 后端提供，无需开发者维护。把下面 `script` 的域名和 `HOST_ORIGIN` 改为 SealChat 页面地址。

```html
<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>SealChat Embed</title>
  <style>
    :root { color-scheme: light; font: 14px/1.5 system-ui, sans-serif; }
    body { margin: 0; padding: 20px; color: #222; background: #fff; }
    main { max-width: 720px; margin: auto; }
    section { margin: 0 0 20px; }
    input, button { box-sizing: border-box; padding: 8px 10px; }
    input { width: 220px; border: 1px solid #bbb; background: #fff; color: #222; }
    button { border: 1px solid #999; border-radius: 4px; background: #f5f5f5; color: #222; cursor: pointer; }
    pre { min-height: 60px; padding: 12px; overflow: auto; background: #f6f6f6; white-space: pre-wrap; }
    .row { display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 8px; }
    #status[data-state="connected"] { color: #087a2f; }
    #status[data-state="offline"] { color: #b42318; }
  </style>
</head>
<body>
  <main>
    <h1>SealChat Embed</h1>

    <section>
      <b>连接：</b><span id="status" data-state="offline">未连接</span>
    </section>

    <section>
      <h2>当前角色</h2>
      <pre id="character">-</pre>
      <h2>在线成员</h2>
      <pre id="members">-</pre>
      <h2>当前权限</h2>
      <pre id="permissions">-</pre>
      <h2>World 管理员</h2>
      <pre id="admins">-</pre>
    </section>

    <section>
      <h2>Storage</h2>
      <div class="row">
        <input id="key" value="demo:key" placeholder="key">
        <input id="value" value='{"count":1}' placeholder="JSON value">
      </div>
      <div class="row">
        <button id="set">写入</button>
        <button id="get">读取</button>
        <button id="delete">删除</button>
      </div>
      <pre id="output">-</pre>
    </section>
  </main>

  <script>
    const embedConfig = window.__SEALCHAT_EMBED_CONFIG__ || {}
    const query = new URLSearchParams(location.search)
    const HOST_ORIGIN = query.get('hostOrigin') || embedConfig.hostOrigin || ''
    const SDK_URL = query.get('sdkUrl') || embedConfig.sdkUrl || ''
    const status = document.querySelector('#status')
    const character = document.querySelector('#character')
    const members = document.querySelector('#members')
    const permissions = document.querySelector('#permissions')
    const admins = document.querySelector('#admins')
    const output = document.querySelector('#output')
    const key = document.querySelector('#key')
    const value = document.querySelector('#value')

    let client = null
    let connecting = null
    let retryTimer = 0
    let retryDelay = 1000
    let connectionGeneration = 0
    let subscriptions = []
    let manuallyClosed = false

    const retryableCodes = new Set(['HANDSHAKE_FAILED', 'SESSION_EXPIRED', 'CONTEXT_CHANGED', 'TIMEOUT', 'WS_OFFLINE', 'INTERNAL_ERROR'])

    function cleanupSubscriptions() {
      subscriptions.splice(0).forEach(off => { try { off?.() } catch {} })
    }

    function ensureSdk() {
      if (window.SealChatEmbed) return Promise.resolve()
      if (!SDK_URL) {
        const error = new Error('缺少 sdkUrl；请由宿主注入 Embed 配置或填写实际 SDK 地址')
        error.code = 'CONFIG_ERROR'
        return Promise.reject(error)
      }
      return new Promise((resolve, reject) => {
        const script = document.createElement('script')
        script.src = SDK_URL
        script.async = true
        script.onload = () => window.SealChatEmbed ? resolve() : reject(new Error('SealChatEmbed 未加载'))
        script.onerror = () => reject(new Error(`无法加载 Embed SDK：${SDK_URL}`))
        document.head.appendChild(script)
      })
    }

    function showStatus(state, detail = '') {
      status.dataset.state = state
      status.textContent = detail ? `${state}：${detail}` : state
    }

    function scheduleReconnect(error) {
      if (manuallyClosed || retryTimer) return
      cleanupSubscriptions()
      client = null
      if (error?.code && !retryableCodes.has(error.code)) {
        showStatus('offline', `${error.code}：${error.message || ''}`)
        return
      }
      showStatus('offline', `等待重连；${error?.code || error?.message || error}`)
      retryTimer = window.setTimeout(() => {
        retryTimer = 0
        void connect().catch(() => {})
      }, retryDelay)
      retryDelay = Math.min(retryDelay * 2, 30_000)
    }

    async function connect() {
      if (client) return client
      if (connecting) return connecting

      manuallyClosed = false
      showStatus('reconnecting')
      connecting = ensureSdk().then(() => window.SealChatEmbed.connect({
        targetOrigin: HOST_ORIGIN || undefined,
        timeoutMs: 10_000
      })).then(async next => {
        client = next
        retryDelay = 1000
        const generation = ++connectionGeneration
        cleanupSubscriptions()

        subscriptions.push(next.connection.onChanged(state => {
          if (generation !== connectionGeneration) return
          showStatus(state.state, state.latencyMs == null ? '' : `${state.latencyMs} ms`)
        }))
        subscriptions.push(next.characters.onChanged(items => {
          if (generation !== connectionGeneration) return
          renderCharacter(items.find(item => item.isActive) || null)
        }))
        subscriptions.push(next.members.onChanged(items => {
          if (generation !== connectionGeneration) return
          renderMembers(items)
        }))
        subscriptions.push(next.permissions.onChanged(summary => {
          if (generation !== connectionGeneration) return
          renderPermissions(summary)
        }))
        subscriptions.push(next.session?.onClosed?.(() => {
          if (generation !== connectionGeneration || manuallyClosed) return
          scheduleReconnect({ code: 'SESSION_EXPIRED', message: 'Embed session closed' })
        }))

        // onChanged 不保证为新 Session 补发当前状态。握手成功后主动读取一次。
        const initialState = await next.connection.getState()
        if (generation === connectionGeneration) {
          showStatus(
            initialState.state,
            initialState.latencyMs == null ? '' : `${initialState.latencyMs} ms`
          )
        }

        await refresh(next).catch(error => {
          output.textContent = `${error.code || 'ERROR'}: ${error.message}`
          throw error
        })
        return next
      }).catch(error => {
        scheduleReconnect(error)
        throw error
      }).finally(() => {
        connecting = null
      })

      return connecting
    }

    function renderCharacter(item) {
      character.textContent = item
        ? `${item.displayName} (${item.id})`
        : '未选择角色'
    }

    function renderMembers(items) {
      members.textContent = items.length
        ? items.map(item => `${item.displayName} (${item.userId})`).join('\n')
        : '无人在线'
    }

    function renderPermissions(summary) {
      permissions.textContent = JSON.stringify({
        worldRole: summary.worldRole,
        isWorldOwner: summary.isWorldOwner,
        isWorldAdmin: summary.isWorldAdmin,
        isSystemAdmin: summary.isSystemAdmin,
        canManageWorld: summary.canManageWorld
      }, null, 2)
    }

    function renderAdmins(itemsOrError) {
      admins.textContent = Array.isArray(itemsOrError)
        ? (itemsOrError.length
          ? itemsOrError.map(item => `${item.displayName} [${item.role}] (${item.userId})`).join('\n')
          : '暂无 World 管理员')
        : String(itemsOrError)
    }

    async function refresh(activeClient) {
      const [currentCharacter, onlineMembers, context, permissionSummary] = await Promise.all([
        activeClient.characters.getCurrent(),
        activeClient.members.list({ scope: 'online' }),
        activeClient.context.get(),
        activeClient.permissions.getCurrent()
      ])
      renderCharacter(currentCharacter)
      renderMembers(onlineMembers)
      renderPermissions(permissionSummary)

      if (!context.capabilities.includes('world.admins.read')) {
        renderAdmins('未授予 world.admins.read')
      } else {
        try {
          renderAdmins(await activeClient.members.list({ scope: 'world-admins' }))
        } catch (error) {
          renderAdmins(`${error.code || 'ERROR'}: ${error.message}`)
        }
      }
    }

    async function useClient(operation) {
      try {
        return await operation(await connect())
      } catch (error) {
        if (error?.code && retryableCodes.has(error.code)) {
          scheduleReconnect(error)
        }
        throw error
      }
    }

    document.querySelector('#set').onclick = async () => {
      try {
        const saved = await useClient(api =>
          api.storage.set(key.value, JSON.parse(value.value))
        )
        output.textContent = JSON.stringify(saved, null, 2)
      } catch (error) {
        output.textContent = `${error.code || 'ERROR'}: ${error.message}`
      }
    }

    document.querySelector('#get').onclick = async () => {
      try {
        const document = await useClient(api => api.storage.get(key.value))
        output.textContent = JSON.stringify(document, null, 2)
      } catch (error) {
        output.textContent = `${error.code || 'ERROR'}: ${error.message}`
      }
    }

    document.querySelector('#delete').onclick = async () => {
      try {
        const result = await useClient(api => api.storage.delete(key.value))
        output.textContent = JSON.stringify(result, null, 2)
      } catch (error) {
        output.textContent = `${error.code || 'ERROR'}: ${error.message}`
      }
    }

    // MessagePort 巡检。宿主 WebSocket 离线时保留 Client，等待宿主自动恢复。
    window.setInterval(() => {
      void useClient(api => api.connection.getState())
        .then(state => showStatus(state.state))
        .catch(() => {})
    }, 15_000)

    window.addEventListener('online', () => void connect().catch(() => {}))
    window.addEventListener('beforeunload', () => {
      manuallyClosed = true
      cleanupSubscriptions()
      try { client?.close?.() } catch {}
    })
    void connect().catch(() => {})
  </script>
</body>
</html>
```

重连规则：`HANDSHAKE_FAILED`、`SESSION_EXPIRED`、`CONTEXT_CHANGED`、`TIMEOUT`、`WS_OFFLINE`、`INTERNAL_ERROR` 属于瞬时连接错误，使用指数退避重新调用 `SealChatEmbed.connect()`；每次新连接都要重新订阅事件，并主动调用一次 `connection.getState()` 初始化当前连接态。`ORIGIN_DENIED`、`CAPABILITY_DENIED`、`PERMISSION_DENIED`、`CONFIG_ERROR` 等配置/权限错误不应无限重试，应先修正 iForm 或 URL 配置。宿主 WebSocket 短暂离线时，SDK Client 可保留，按 `connection.onChanged()` 更新状态并等待宿主恢复；`session.onClosed()` 则表示旧 Session 已失效，必须丢弃旧 Client 后重连。

## 6. 开发约定

- 页面卸载前执行各订阅返回的取消函数；需要主动终止时调用 `sealchat.close()`。
- 所有 UI 都要允许 `currentUser`、`currentMember`、`currentCharacter` 为 `null`。
- 收到 `context.onChanged()` 后，用新 Context 更新权限与当前角色。
- 新 Session 握手成功后，先注册 `connection.onChanged()`，再主动 `connection.getState()` 初始化一次；不要假设宿主会自动推送初始 `connection.changed`。
- 更新已经存在的共享对象时优先传有效的 `ifRevision`，避免多人修改静默覆盖；首次创建 key 时省略该选项，不传 `null` 占位值。
- 不用 Storage 模拟高频消息流；不用 Event 保存必须恢复的数据。
- 不缓存或猜测 `channelId`、`formId`；以当前 Session 返回值为准。
- 不向 iframe 暴露 token、内部错误栈、任意 HTTP API 或原始 WebSocket。
- 频道 iframe 可能处于 sandbox；不要使用 `window.alert()`、`window.confirm()`、`window.prompt()`，确认交互改用页面内 HTML `<dialog>` 或其他 DOM UI。
