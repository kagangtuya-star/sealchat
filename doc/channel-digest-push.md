# 频道未读提醒文档

## 1. 功能概述

频道未读提醒用于替代旧的“未读邮件提醒”主链路。

系统不再围绕“某个用户是否未读”来触发提醒，而是围绕“某个频道在一个时间周期内是否发生了足够活跃的消息事件”来生成摘要，并通过以下两种方式对外提供：

1. 主动推送：SealChat 主动向你的 HTTP 接收地址发送摘要 JSON。
2. 被动拉取：SealChat 对外暴露带 token 的摘要读取接口，由你的 Bot 或外部系统轮询。

当前实现支持两种作用域：

1. 频道级摘要
2. 世界级多频道合并摘要

其中世界级摘要支持选择指定频道，并将命中的频道内容合并为一条推送消息，合并后的摘要正文会写入 JSON 的 `text` 字段。

## 2. 触发规则

### 2.1 时间周期

摘要按离散窗口统计与生成，当前支持：

1. `5m`
2. `15m`
3. `30m`
4. `1h`
5. `2h`
6. `6h`
7. `24h`

默认值为 `1h`。

### 2.2 登录用户阈值

“登录用户阈值”在当前实现中，实际指：

1. 一个统计周期内访问过该频道的不同成员数
2. 访问行为以 `channel.enter` 成功为准
3. 不依赖 presence 心跳，不取在线快照

支持两种模式：

1. `channel_member_count`
说明：阈值动态等于当前频道成员数，默认采用此模式。

2. `fixed`
说明：手动指定固定人数阈值。

### 2.3 满足条件

窗口结束后，若同时满足以下条件，则生成摘要记录：

1. 该窗口内消息数大于 `0`
2. 该窗口内访问成员数大于等于阈值

若不满足，则该窗口不会生成摘要记录。

## 3. 摘要内容

### 3.1 发言人名称来源

摘要中的发言人名称按以下优先级解析：

1. `sender_identity_name`
2. `sender_member_name`
3. `member.nickname`
4. `user.nickname`
5. `user.username`

因此摘要会优先使用频道角色昵称或身份昵称，而不是裸用户名。

### 3.2 默认文本模板

默认文本模板：

```text
在 {{window_label}}，{{speaker_names}} 在 {{channel_name}} 频道发送了 {{message_count}} 条消息。
```

### 3.3 默认 JSON 模板

默认 JSON 模板：

```json
{
  "scopeType": {{scope_type}},
  "scopeId": {{scope_id}},
  "window": {
    "start": {{window_start_ts}},
    "end": {{window_end_ts}},
    "label": {{window_label}},
    "seconds": {{window_seconds}}
  },
  "channel": {
    "id": {{channel_id}},
    "name": {{channel_name}}
  },
  "world": {
    "id": {{world_id}},
    "name": {{world_name}}
  },
  "messageCount": {{message_count}},
  "activeUserCount": {{active_user_count}},
  "speakerNames": {{speaker_names_array}},
  "speakerSummary": {{speaker_summary}},
  "speakers": {{speakers}},
  "text": {{rendered_text}}
}
```

### 3.4 可用占位符

文本模板支持：

1. `{{window_start}}`
2. `{{window_end}}`
3. `{{window_label}}`
4. `{{channel_name}}`
5. `{{world_name}}`
6. `{{message_count}}`
7. `{{active_user_count}}`
8. `{{speaker_names}}`
9. `{{speaker_summary}}`

JSON 模板支持：

1. `{{scope_type}}`
2. `{{scope_id}}`
3. `{{window_start_ts}}`
4. `{{window_end_ts}}`
5. `{{window_label}}`
6. `{{window_seconds}}`
7. `{{channel_id}}`
8. `{{channel_name}}`
9. `{{world_id}}`
10. `{{world_name}}`
11. `{{message_count}}`
12. `{{active_user_count}}`
13. `{{speaker_names_array}}`
14. `{{speaker_summary}}`
15. `{{speakers}}`
16. `{{rendered_text}}`

注意：

1. JSON 模板中的字符串占位符不要自行再加引号。
2. `{{rendered_text}}` 已经会被渲染为合法 JSON 字符串。
3. 若 JSON 模板为空，前端与后端都会自动回落到默认 JSON 模板。

## 4. 推送模式

### 4.1 被动拉取

被动拉取表示：

1. SealChat 先把摘要落库到 `digest_records`
2. 外部系统再通过接口读取

读取的是“已生成的摘要记录”，不是现算结果。

#### 接口

获取摘要列表：

```http
GET /api/v1/webhook/channels/:channelId/digests?token=...&limit=30
```

获取最新一条摘要：

```http
GET /api/v1/webhook/channels/:channelId/digests/latest?token=...
```

#### 鉴权

支持两种方式：

1. `Authorization: Bearer <token>`
2. Query 参数：`?token=<token>`

未读提醒面板会自动创建专用 integration，生成 token，并直接拼成可访问 URL。

#### 限制

1. 列表接口最大 `limit=30`
2. 每个频道的摘要记录数据库只保留最近 `30` 条
3. 测试推送生成的记录也会落库，因此 `latest` 能读取到测试结果

#### 列表示例

```json
{
  "channelId": "mTbPod4bjCmOG5Ga",
  "cursor": "0",
  "nextCursor": "1774262367316",
  "items": [
    {
      "id": "sM48AWNeRyRJTBXx",
      "ruleId": "test:channel:mTbPod4bjCmOG5Ga",
      "scopeType": "channel",
      "scopeId": "mTbPod4bjCmOG5Ga",
      "windowSeconds": 3600,
      "windowStart": 1774260000000,
      "windowEnd": 1774263600000,
      "messageCount": 5,
      "activeUserCount": 1,
      "speakerNames": ["怜青", "埃蒙岚青|星尘"],
      "speakerSummary": "怜青(4)、埃蒙岚青|星尘(1)",
      "renderedText": "在 2026-03-23 18:00 至 19:00，怜青、埃蒙岚青|星尘 在 示例频道 频道发送了 5 条消息。",
      "renderedJson": "{\"scopeType\":\"channel\"}",
      "status": "test",
      "generatedAt": 1774262367316,
      "triggeredBy": "test",
      "deliveryAttempts": 0
    }
  ],
  "integration": {
    "id": "J40ctqKBlySwmPfk",
    "source": "digest-pull"
  }
}
```

### 4.2 主动推送

主动推送表示：

1. SealChat 在窗口满足条件时主动向目标地址发起 HTTP 请求
2. 请求体使用渲染后的 `rendered_json`
3. `Content-Type` 固定为 `application/json`

#### 支持的请求方法

1. `POST`
2. `PUT`
3. `PATCH`

#### 可配置项

1. 目标 URL
2. 请求方法
3. 请求头 JSON 对象
4. 签名密钥
5. JSON 模板
6. 文本模板

#### 请求头配置格式

必须是 JSON 对象，例如：

```json
{
  "X-Env": "prod",
  "X-Source": "sealchat"
}
```

如果为空，系统会自动当作 `{}` 处理。

#### 固定附加请求头

SealChat 会自动附加：

1. `Content-Type: application/json`
2. `User-Agent: sealchat-digest-push/1.0`
3. `X-SealChat-Timestamp: <unix-seconds>`

如果设置了签名密钥，还会附加：

4. `X-SealChat-Signature: sha256=<hex>`

签名算法：

```text
signature = HMAC_SHA256(secret, timestamp + "." + rawBody)
```

其中：

1. `timestamp` 为 `X-SealChat-Timestamp`
2. `rawBody` 为原始请求体字节串
3. 输出前缀固定为 `sha256=`

#### 主动推送请求示例

```http
POST /digest HTTP/1.1
Host: your-bot.example.com
Content-Type: application/json
User-Agent: sealchat-digest-push/1.0
X-SealChat-Timestamp: 1774262367
X-SealChat-Signature: sha256=9a6d...
X-Env: prod
```

请求体示例：

```json
{
  "scopeType": "channel",
  "scopeId": "mTbPod4bjCmOG5Ga",
  "window": {
    "start": 1774260000000,
    "end": 1774263600000,
    "label": "2026-03-23 18:00 至 19:00",
    "seconds": 3600
  },
  "channel": {
    "id": "mTbPod4bjCmOG5Ga",
    "name": "示例频道"
  },
  "world": {
    "id": "EWIJ5Fa1y4NG4tui",
    "name": "颂神之人"
  },
  "messageCount": 5,
  "activeUserCount": 1,
  "speakerNames": ["怜青", "埃蒙岚青|星尘"],
  "speakerSummary": "怜青(4)、埃蒙岚青|星尘(1)",
  "speakers": [
    { "key": "efSbFMwVaUs2g4V9", "name": "怜青", "messageCount": 4 },
    { "key": "gNWjuyMjTkbcA7Q9", "name": "埃蒙岚青|星尘", "messageCount": 1 }
  ],
  "text": "在 2026-03-23 18:00 至 19:00，怜青、埃蒙岚青|星尘 在 示例频道 频道发送了 5 条消息。"
}
```

## 5. 测试推送

未读提醒面板提供测试能力。

支持：

1. 直接测试最近一个已结束窗口
2. 手工指定测试时间范围
3. 可选同时触发主动推送

测试推送行为：

1. 先生成一条 `status=test` 的摘要记录
2. 写入 `digest_records`
3. 若勾选“同时触发主动推送”，则立刻向外部地址发送一次请求

因此：

1. 测试结果会出现在“测试推送”区域
2. 被动 `latest` 接口也能读到测试落库记录

## 6. 数据保留策略

当前实现中，系统会自动清理历史数据，避免数据库无限增长。

### 6.1 摘要记录

每个频道最多保留最近 `30` 条：

1. `digest_records`
2. 对应的 `digest_delivery_logs`

### 6.2 窗口统计中间数据

每个频道、每个周期最多保留最近 `30` 个窗口的：

1. `digest_window_visitors`
2. `digest_window_speakers`

## 7. 前端使用流程

1. 打开频道
2. 点击原“邮件提醒”位置的入口，现名称为“未读提醒”
3. 配置事件周期
4. 配置登录用户阈值模式
5. 选择推送方式：
   - 被动拉取
   - 主动推送
   - 主动 + 被动
6. 如启用主动推送，填写：
   - 推送 URL
   - 请求方法
   - 请求头 JSON 对象
   - 签名密钥
7. 如使用被动拉取，直接复制面板生成的带 token 链接
8. 点击“保存”
9. 使用“执行测试”验证

## 8. 排障指南

### 8.1 测试后 `latest` 返回为空

排查：

1. 确认后端已升级到包含 `DigestRecordUpsert` 和唯一索引迁移的版本
2. 确认服务已重启，使启动期迁移真正执行
3. 确认测试窗口内确实有消息

### 8.2 主动推送失败

优先检查：

1. 推送 URL 是否可从 SealChat 服务器访问
2. 接收端是否允许 `POST/PUT/PATCH`
3. 接收端是否要求签名，而密钥不一致
4. 请求头 JSON 是否为合法对象
5. 外部服务是否返回了非 `2xx`

可在测试面板查看：

1. 状态码
2. 耗时
3. 错误文本
4. 最新记录中的 `deliveryAttempts`

当前实现：

1. 每次主动推送失败后，会自动重试 3 次
2. 即单次摘要最多尝试 4 次投递
3. 每次尝试都会写入一条 `digest_delivery_logs`
4. 重试同时会递增 `digest_records.delivery_attempts`

### 8.3 明明有消息但没有生成摘要

当前规则必须同时满足：

1. 窗口内消息数大于 `0`
2. 访问成员数达到阈值

常见原因：

1. 频道成员很多，默认阈值等于频道人数，导致窗口访问人数不足
2. 用户未重新进入频道，因此没有产生 `channel.enter` 访问记录
3. 测试窗口选错，没有落在消息所在周期内

### 8.4 被动 URL 无法直接访问

确认：

1. 使用的是面板中带 `token` 的完整 URL
2. token 未被轮换
3. integration 未被撤销
4. 该 integration 具有 `read_digest` capability

## 9. 主动推送接收校验程序

仓库附带了一个可运行的示例接收器：

`cmd/digest-push-receiver/main.go`

用途：

1. 接收主动推送请求
2. 打印请求头与请求体
3. 可选校验 `X-SealChat-Timestamp`
4. 可选校验 `X-SealChat-Signature`
5. 常驻监听，直到你手动停止进程

运行方式见下一节文档或直接执行：

```bash
go run ./cmd/digest-push-receiver
```

带签名校验：

```bash
DIGEST_PUSH_SECRET=your-secret go run ./cmd/digest-push-receiver
```

说明：

1. 该程序是持续监听的 HTTP 服务，不是“一次收包即退出”的脚本
2. 未设置 `DIGEST_PUSH_SECRET` 时，程序不会强制要求 `X-SealChat-Timestamp`
3. 已设置 `DIGEST_PUSH_SECRET` 时，会同时校验时间戳与签名
4. 默认监听地址为 `:18081`，默认路径为 `/digest`

## 10. 当前实现边界

1. 当前支持频道级摘要与世界级多频道合并摘要
2. 当前窗口统计主要围绕“消息创建”和“频道进入”采集
3. 本期不追算消息编辑、删除对历史窗口的影响
4. 当前主动推送采用进程内固定重试 3 次，不含持久化延迟队列
5. 旧邮件提醒代码仍保留在仓库中，但不再是主链路
