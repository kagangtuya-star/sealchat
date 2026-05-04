# SealChat Bridge API

本文档描述当前 SealChat 完整频道页内置的 `postMessage` Bridge 协议，供外部宿主页面或其他仓库嵌入 `SealChat` 时读取当前频道角色列表与消息流。

## 1. 适用范围

- 目标页面必须是完整频道页：`/#/worldId/channelId`
- 不使用 `/embed` 模式
- Bridge 只负责“向父页面推送当前频道数据”
- 目前不包含“从父页面发消息到 SealChat”的写接口

## 2. 设计目标

- 默认不转发，只有握手成功后才开始推送
- 只推送当前打开频道的数据
- 以 `identityId` 作为角色绑定主键
- 角色差分不额外拆字段语义，直接体现在 `displayName`、`color`、`avatarUrl`

## 3. 通信方式

使用浏览器 `window.postMessage`。

- 父页面 -> SealChat iframe：发送握手和取消订阅指令
- SealChat iframe -> 父页面：发送握手确认、角色快照、消息事件

Bridge 运行于完整站点前端内部，入口已在 [ui/src/main.ts](/mnt/e/Code/go/sealchat/ui/src/main.ts:11) 接入。

## 4. 接入流程

1. 父页面创建 iframe，地址指向 `https://your-sealchat-host/#/worldId/channelId`
2. iframe 加载完成后，父页面向 iframe 发送一次握手消息
3. SealChat 返回 `sealchat.bridge.handshake.ack`
4. SealChat 立即返回一次 `sealchat.bridge.roles.snapshot`
5. 之后开始持续推送 `sealchat.bridge.message`
6. 如需停止，父页面发送 `sealchat.bridge.unsubscribe`

## 5. 父页面请求协议

### 5.1 握手

```json
{
  "type": "sealchat.bridge.handshake",
  "version": 1,
  "nonce": "unique-string",
  "want": ["roles", "messages"],
  "currentChannelOnly": true
}
```

字段说明：

- `type`: 固定为 `sealchat.bridge.handshake`
- `version`: 固定为 `1`
- `nonce`: 父页面生成的请求标识，握手确认时原样带回
- `want`: 当前版本要求传入 `["roles", "messages"]`
- `currentChannelOnly`: 当前版本要求传入 `true`

注意：

- `want` 和 `currentChannelOnly` 在当前实现里主要用于协议约束与向前兼容，Bridge 实际只支持“当前频道 + 角色/消息”这一种模式
- 不需要在握手里再传 `worldId`/`channelId`，iframe 已从当前路由自动解析

### 5.2 取消订阅

```json
{
  "type": "sealchat.bridge.unsubscribe"
}
```

行为说明：

- 发送后，SealChat 停止继续推送数据
- 当前版本不会返回 `ack`
- 重新订阅时再次发送握手即可

## 6. SealChat 返回协议

### 6.1 握手确认

```json
{
  "type": "sealchat.bridge.handshake.ack",
  "version": 1,
  "nonce": "unique-string",
  "ok": true,
  "worldId": "world-123",
  "channelId": "channel-456"
}
```

字段说明：

- `nonce`: 原样回传，用于父页面匹配本次握手
- `worldId`: 当前 iframe 实际所在世界 ID
- `channelId`: 当前 iframe 实际所在频道 ID

用途：

- 父页面可用它校验 iframe 是否已进入预期频道
- 父页面可直接用返回的 `worldId/channelId` 作为后续自身接口上下文

### 6.2 角色快照

类型：`sealchat.bridge.roles.snapshot`

```json
{
  "type": "sealchat.bridge.roles.snapshot",
  "worldId": "world-123",
  "channelId": "channel-456",
  "generatedAt": 1710000000000,
  "roles": [
    {
      "identityId": "role-a",
      "displayName": "阿尔文·负伤",
      "color": "#bf616a",
      "avatarUrl": "https://host/api/v1/attachment/avatar-hurt",
      "isTemporary": false,
      "icOocOnActivate": "ic",
      "activeVariantId": "variant-hurt",
      "activeVariantDisplayName": "阿尔文·负伤",
      "activeVariantColor": "#bf616a",
      "activeVariantAvatarUrl": "https://host/api/v1/attachment/avatar-hurt"
    }
  ]
}
```

触发时机：

- 握手成功后立即推送一次
- 当前频道角色列表变化时重新推送
- 当前频道角色差分变化时重新推送
- 路由切换或相关状态变化时，若已订阅，也会重新推送

角色字段说明：

- `identityId`: 角色主键。对接绑定请固定使用这个字段
- `displayName`: 当前实际显示名称。若存在已激活差分，则优先使用差分名
- `color`: 当前实际显示颜色。若存在已激活差分，则优先使用差分色
- `avatarUrl`: 当前实际显示头像 URL。若存在已激活差分，则优先使用差分头像
- `isTemporary`: 是否为临时角色
- `icOocOnActivate`: 激活角色后的默认模式，可能是 `"" | "ic" | "ooc"`
- `activeVariantId`: 当前激活差分 ID，没有则为 `null`
- `activeVariantDisplayName`: 当前差分显示名，没有则为空字符串
- `activeVariantColor`: 当前差分颜色，没有则为空字符串
- `activeVariantAvatarUrl`: 当前差分头像 URL，没有则为空字符串

绑定建议：

- UI 选择列表展示 `displayName/color/avatarUrl`
- 内部绑定值只保存 `identityId`

### 6.3 消息事件

类型：`sealchat.bridge.message`

```json
{
  "type": "sealchat.bridge.message",
  "event": "message-created",
  "worldId": "world-123",
  "channelId": "channel-456",
  "messageId": "msg-001",
  "createdAt": 1710000000000,
  "icMode": "ic",
  "isWhisper": false,
  "identityId": "role-a",
  "displayName": "阿尔文·负伤",
  "color": "#bf616a",
  "avatarUrl": "https://host/api/v1/attachment/avatar-hurt",
  "contentRaw": "{\"type\":\"doc\",\"content\":[...]}",
  "contentText": "你好，@测试 [图片]"
}
```

`event` 取值：

- `message-created`
- `message-updated`
- `message-deleted`

消息字段说明：

- `worldId`: 当前消息所属世界
- `channelId`: 当前消息所属频道
- `messageId`: 消息 ID
- `createdAt`: 时间戳。部分旧数据可能缺失
- `icMode`: `ic` 或 `ooc`
- `isWhisper`: 是否为悄悄话
- `identityId`: 发送角色 ID；取不到时为 `null`
- `displayName`: 当前实际显示名称，优先使用频道内实时角色/差分状态
- `color`: 当前实际显示颜色，优先使用频道内实时角色/差分状态
- `avatarUrl`: 当前实际显示头像 URL，优先使用频道内实时角色/差分状态
- `contentRaw`: 原始消息内容
- `contentText`: 归一化后的纯文本内容

## 7. `contentText` 归一化规则

当前实现位于 [ui/src/bridge/sealchatBridgeSerializer.ts](/mnt/e/Code/go/sealchat/ui/src/bridge/sealchatBridgeSerializer.ts:1)。

规则如下：

- 若 `contentRaw` 是 TipTap JSON 文档，则提取为纯文本
- `mention` 节点会转成 `@名字`
- `hardBreak` 会转成换行
- 行内图片标记 `[[图片:...]]` 或 `[[img:...]]` 会被替换成 `[图片]`
- 机器人状态控件前缀 `[[STATE_WIDGET]]` 会被去掉
- 无法解析时，回退为原始字符串

说明：

- `contentRaw` 适合做高级渲染或后处理
- `contentText` 适合日志、字幕、JRPG 对话框等轻量展示

## 8. `avatarUrl` 规则

当前实现会自动输出“当前实际显示头像”。

优先级：

1. 当前激活差分头像
2. 角色头像
3. 消息上自带的发送者头像字段

额外处理：

- 若底层附件解析结果是协议相对地址，例如 `//127.0.0.1:13211/api/...`
- Bridge 会自动补全为绝对地址
- 优先使用当前页面 `location.protocol`
- 若无法取得协议，则回退为 `https:`

因此，对接方可以直接把 `avatarUrl` 用在 `img.src`，不需要再二次补协议。

## 9. 当前频道约束

Bridge 只推送“当前打开频道”的数据。

实现细节：

- 当前频道优先从 `chat.curChannel.id` 读取
- 若 store 尚未就绪，会回退到路由参数 `/#/worldId/channelId`
- 若收到的消息不属于当前频道，则不会转发

这意味着：

- 父页面如需切换订阅频道，应直接切换 iframe URL
- 不建议尝试用一个 iframe 同时订阅多个频道

## 10. Origin 与安全边界

当前运行时位于 [ui/src/bridge/sealchatBridgeRuntime.ts](/mnt/e/Code/go/sealchat/ui/src/bridge/sealchatBridgeRuntime.ts:47)。

行为：

- 仅接受来自 `window.parent` 的消息
- 成功握手后，后续回发默认使用该次握手的 `origin`
- 若握手来源是空字符串或 `"null"`，会规范成 `"*"`

为什么会出现 `"null"`：

- 常见于父页面是 `file://`、opaque origin、某些本地调试环境

注意：

- `file://` 调试可用，但生产环境仍建议使用明确 origin 的网页宿主
- 若宿主页自己还做二次分发，建议在宿主层再加来源校验

## 11. 最小接入示例

```html
<iframe
  id="sealchat-frame"
  src="https://sealchat.example.com/#/world-123/channel-456"
></iframe>
<script>
  const frame = document.getElementById('sealchat-frame')

  function handshake() {
    frame.contentWindow.postMessage({
      type: 'sealchat.bridge.handshake',
      version: 1,
      nonce: `obr-${Date.now()}`,
      want: ['roles', 'messages'],
      currentChannelOnly: true,
    }, '*')
  }

  window.addEventListener('message', (event) => {
    const data = event.data
    if (!data || typeof data !== 'object') return
    if (typeof data.type !== 'string') return
    if (!data.type.startsWith('sealchat.bridge.')) return

    if (data.type === 'sealchat.bridge.handshake.ack') {
      console.log('握手成功', data.worldId, data.channelId)
      return
    }

    if (data.type === 'sealchat.bridge.roles.snapshot') {
      console.log('角色列表', data.roles)
      return
    }

    if (data.type === 'sealchat.bridge.message') {
      console.log('消息事件', data.event, data.messageId, data.contentText)
    }
  })

  frame.addEventListener('load', handshake)
</script>
```

## 12. 调试页面

仓库已提供独立单页调试工具：

- [ui/public/sealchat-bridge-debug.html](/mnt/e/Code/go/sealchat/ui/public/sealchat-bridge-debug.html:1)

用途：

- 加载完整频道页 iframe
- 手动发送握手
- 查看角色快照
- 查看消息事件日志
- 发送取消订阅

特点：

- 消息日志独立滚动
- 每条日志默认折叠
- 自动忽略非 `sealchat.bridge.*` 消息
- 可从目标 URL 自动解析并回填 `worldId/channelId`
- 可在 `file://` 父页面场景下调试

## 13. 对接建议

推荐接入策略：

- 角色选择器使用 `roles.snapshot`
- 保存绑定时只保存 `identityId`
- JRPG 对话渲染优先使用 `contentText`
- 若需要富文本或自定义图片解析，再结合 `contentRaw`
- 频道切换由 iframe URL 驱动，不要自行伪造频道上下文

## 14. 当前能力边界

当前版本不提供：

- 发送消息 API
- 历史消息拉取 API
- 多频道并发订阅
- `want` 字段的细粒度过滤
- 角色差分切换指令

如需这些能力，建议另开协议版本，不要直接在 v1 上做不兼容扩展。
