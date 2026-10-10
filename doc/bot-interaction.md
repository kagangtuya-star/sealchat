# 隐式 BOT Interaction

`bot.interact` 用于从前端向频道的 primary BOT 发送一次临时指令并等待结果。请求消息以及被成功捕获的首条 BOT 回复都不会写入聊天消息数据库、不会出现在聊天界面，也不会触发 `message-created`、未读、通知、Webhook 或 digest 等正常消息副作用。

## 前端调用

```ts
const result = await chat.botInteract(channelId, '.xxx')
```

可通过第三个参数覆盖服务端等待时间：

```ts
const result = await chat.botInteract(channelId, '.xxx', { timeoutMs: 8000 })
```

`legacy_quiet` 可显式启用 legacy context 的发送前 quiet window 等待：

```ts
const result = await chat.botInteract(channelId, '.nn Alice', { timeoutMs: 5000, legacyQuiet: true })
```

默认 timeout 为 5 秒，服务端会将 `timeout_ms` 限制在 1000～15000ms。`timeout_ms` 始终表示指令实际发出后的 BOT 回复等待时间；`legacy_quiet: true` 时，如果频道尚未安静 5 秒，服务端会在发送前异步等待，最长额外等待约 5 秒，持续活跃则返回 `BOT_INTERACTION_BUSY`。服务端异步处理，不会阻塞当前用户连接继续处理其他聊天 WebSocket API。

## WebSocket API

请求：

```json
{
  "api": "bot.interact",
  "echo": "frontend-echo",
  "data": {
    "channel_id": "channel-id",
    "command": ".xxx",
    "timeout_ms": 5000,
    "legacy_quiet": false
  }
}
```

- `channel_id`：必填，目标频道。
- `command`：必填，trim 后不可为空。
- `timeout_ms`：可选，默认 5000。
- `legacy_quiet`：可选，默认 `false`。`false` 时立即发送，适合使用 quote 或 structured response 精确关联的 BOT；`true` 时发送前等待 legacy quiet window，最长额外约 5 秒，持续活跃则返回 `BOT_INTERACTION_BUSY`。人物卡昵称同步使用 `true`。

成功响应：

```json
{
  "echo": "frontend-echo",
  "data": {
    "ok": true,
    "request_id": "interaction-id",
    "matched_by": "context",
    "content": "legacy BOT reply",
    "data": null
  }
}
```

`matched_by` 为 `quote`、`context` 或 `structured`。legacy `message.create` 回复使用 `content`；structured 回复使用 `data`。同一 `(BOT, channel)` 同时只允许一个请求，忙时返回 `BOT_INTERACTION_BUSY`；超时返回 `BOT_INTERACTION_TIMEOUT`；primary BOT 没有可用的标准 SealChat WebSocket 连接时返回 `BOT_INTERACTION_BOT_UNAVAILABLE`。

## BOT 回复

legacy BOT 可以继续使用 `message.create`。cached `MessageContext` fallback 不是绝对可靠的关联协议，因此服务端按以下优先级进行保守匹配：

1. `quote_id` 精确等于临时消息 ID；
2. structured response 的 `echo` 精确等于 `interactionId`；
3. 该 BOT 连接缓存的 `MessageContext` 同时包含 `isEphemeral: true` 和对应的 `interactionId`。

`quote_id` 和 structured response 是精确关联。`legacy_quiet: true` 时，cached `MessageContext` 只有在 interaction 发送前该频道对目标 BOT 已至少安静 5 秒，并且 interaction 之后没有新的 `message-created` 发给该 BOT 时才允许匹配；同时仍要求频道、BOT 用户、BOT WebSocket 连接和有效期一致。`legacy_quiet: false` 时不等待 quiet window，legacy context fallback 在频道不安静时关闭，但 quote / structured 精确关联不受影响。

服务端不会使用“仅凭 BOT + channel + 时间窗口”的宽松匹配，也不改变 `quote`、`structured`、`context` 三种匹配优先级。在已发送的 interaction 之后如果无法安全确认，legacy context 回复会按普通消息处理，interaction 可能最终 timeout。这是刻意的安全降级：保守降低误判风险，无法确认时不捕获。

支持 structured response 的 BOT 可读取临时事件中的 `messageContext.interactionId`，并直接回复：

```json
{
  "api": "",
  "echo": "<interactionId>",
  "data": {
    "value": 123
  }
}
```

首版只捕获第一条匹配的 legacy `message.create` 回复。interaction 完成、失败或超时后，旧 ephemeral `MessageContext` 会自动视为失效；BOT 后续发送的消息以及超时后的迟到回复都按普通消息处理。当前只保证标准 SealChat BOT WebSocket，OneBot 不在首版范围内。

## 开发注意事项

- `bot.interact` 必须只面向频道的 **primary BOT**，不要复用会广播给多个 BOT 的频道事件发送接口。
- 不要通过 BOT 回复文本、BOT + channel + 时间窗口等方式猜测回复归属；只能使用 `quote_id`、structured `echo` 或受保护的 ephemeral `MessageContext`。
- `legacy_quiet` 仅用于无法精确关联回复的 legacy BOT，默认保持关闭。频道不 quiet 时，普通 interaction 不得覆盖最近普通消息的 `BotLastMessageEvent`，否则会破坏后续 quiet 判断。
- 同一 `(BOT, channel)` 只允许一个 interaction。业务侧如果可能连续产生状态切换，应自行采用 **latest-wins**，不要简单丢弃 `BOT_INTERACTION_BUSY` 后的最新状态。
- `timeout_ms` 从指令真正发送后开始计算；发送前 quiet wait 不占回复超时。等待期间客户端连接关闭时应立即取消，避免发送已经失效的延迟指令。
- interaction 代表某个业务状态时，应在该业务状态确认成功后再发送。例如人物卡切换必须先 `tagCard` 成功，再同步 BOT 昵称。
- hidden interaction 必须在正常消息入库流程前完成捕获；无法安全确认的回复宁可按普通消息处理，也不要扩大匹配范围。
- OneBot fallback 不属于 `bot.interact` 的隐式回复捕获范围；不要恢复基于昵称回复文本的 ACK suppress。如需完全隐式支持，应增加真正的 correlation。
