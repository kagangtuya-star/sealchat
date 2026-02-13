# Webhook 外部数据操作 API（v1）

> 本文档描述 **对外（外部系统）** 使用的 Webhook API（Pull 轮询 + 写入），以及其推荐的幂等/同步方式。
>
> 对应需求与约束见：`specs/2025-12-17-webhook-api.md`。

## 基本约定

- BasePath：`/api/v1`
- Content-Type：`application/json; charset=utf-8`
- 鉴权：每个请求都必须包含 `channelId` 与 `token`
- 时区：所有时间戳均为 Unix milliseconds（ms）

## 鉴权

推荐请求头：

- `Authorization: Bearer <token>`

兼容（如果实现未解析 Bearer，可临时用纯 token）：

- `Authorization: <token>`

## 外部 Webhook API

### 1) GET 获取最近消息变更（Change Feed）

`GET /api/v1/webhook/channels/:channelId/changes`

Query 参数：

- `cursor`：可选，变更游标（上一次响应的 `nextCursor`）；不传则返回“最近 `limit` 条”附近的事件
- `limit`：可选，默认 100，最大 500
- `excludeSource`：可选，例如 `foundry`（用于防回环）

预留但 v1 暂未实现（需要时可扩展）：

- `includeArchived`
- `includeWhisper`（默认会排除悄悄话）
- `format`

响应（示例）：

```json
{
  "channelId": "ch_xxx",
  "cursor": "1024",
  "nextCursor": "1037",
  "serverTime": 1760000000000,
  "events": [
    {
      "seq": 1035,
      "type": "message-created",
      "message": {
        "id": "msg_xxx",
        "content": "hello",
        "createdAt": 1760000000000,
        "updatedAt": 0,
        "isDeleted": false,
        "deletedAt": 0,
        "user": { "id": "u_bot", "nickname": "Foundry", "avatar": "id:att_xxx", "is_bot": true },
        "identity": { "id": "ident_xxx", "displayName": "Goblin", "color": "#22c55e", "avatarAttachment": "att_avatar" }
      },
      "origin": {
        "integrationId": "whk_xxx",
        "source": "foundry",
        "externalId": "foundry-message-123"
      }
    },
    {
      "seq": 1036,
      "type": "message-updated",
      "message": { "id": "msg_xxx", "content": "hello!!", "updatedAt": 1760000001000, "isEdited": true, "editCount": 1 }
    },
    {
      "seq": 1037,
      "type": "message-removed",
      "message": { "id": "msg_xxx", "isDeleted": true, "deletedAt": 1760000002000 }
    }
  ]
}
```

错误响应（统一格式建议）：

```json
{ "error": "unauthorized", "message": "token invalid or revoked" }
```

常见状态码：

- `200`：成功
- `401`：token 无效/过期/已撤销
- `403`：token 对该频道无效或无相应 capability
- `400`：参数错误（cursor 解析失败等）

### 2) POST 写入信息（创建/编辑/删除）

`POST /api/v1/webhook/channels/:channelId/messages`

请求体（统一 envelope）：

```json
{
  "op": "message.upsert",
  "idempotencyKey": "optional-string",
  "externalRef": { "source": "foundry", "externalId": "foundry-message-123" },
  "identity": {
    "externalActorId": "actor-1",
    "displayName": "Goblin",
    "color": "#22c55e",
    "avatarAttachmentId": "att_avatar"
  },
  "message": {
    "content": "hello",
    "quoteExternalId": "foundry-message-100",
    "quoteMessageId": "",
    "icMode": "ic",
    "displayOrder": 1760000000000
  }
}
```

`op` 可选值（v1 建议）：

- `message.upsert`：按 `externalRef` 幂等创建；若已存在则更新（需 `write_create` + `write_update_own`）
- `message.create`：强制创建（不幂等，仍可用 `idempotencyKey`）
- `message.update`：更新（通过 `messageId` 或 `externalRef` 定位）
- `message.delete`：删除（建议软删除，对应 SealChat 的 `message.remove` 语义）

说明：

- `idempotencyKey` 当前仅作为预留字段，v1 实现以 `externalRef(source+externalId)` 为幂等键。

定位字段优先级建议：

1. `messageId`
2. `externalRef(source+externalId)`（推荐，便于 Foundry）

响应（示例）：

```json
{
  "ok": true,
  "channelId": "ch_xxx",
  "result": {
    "messageId": "msg_xxx",
    "externalRef": { "source": "foundry", "externalId": "foundry-message-123" },
    "created": true,
    "updated": false
  }
}
```

错误响应（示例）：

```json
{ "ok": false, "error": "forbidden", "message": "capability write_update_own required" }
```

## 附件与头像（推荐做法）

Foundry 若需要“消息头像/角色头像”呈现于 SealChat，推荐流程：

1. 使用现有上传接口上传图片得到 `attachmentId`：
   - `POST /api/v1/attachment-upload`（multipart/form-data）
   - 或 `POST /api/v1/attachment-upload-quick`（已存在哈希时）
2. 在 Webhook POST 的 `identity.avatarAttachmentId` 中引用该 `attachmentId`。

> 注：是否允许服务端从外部 URL 抓取图片（`avatarUrl`）建议作为后续增强项；v1 可先依赖上传接口闭环。

## 同步建议（Foundry 侧）

### 初始化快照

- 启动时先调用一次 GET `changes`（不带 cursor），拿到 `nextCursor` 作为起点。
- 如需要历史消息，可另行扩展 `GET /messages` 快照接口（见 Spec 的扩展项）。

### 增量轮询

- 定时（例如 1~3 秒）调用 GET `changes?cursor=...`
- 处理 `message-created/updated/removed`
- 记录并更新 `nextCursor`

### 防回环（Loop Prevention）

- 若事件中携带 `origin.source=foundry` 且 `origin.externalId` 在本地已处理，可跳过。
- 或按 `excludeSource=foundry` 让服务端过滤（需要服务端实现）。
