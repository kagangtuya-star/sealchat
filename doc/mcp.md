# 平台 MCP 接入

平台管理员在“平台管理 → MCP 接入”开启服务并配置模块上限，使用标题栏的保存按钮。默认关闭。用户在共用的“个人信息 → Personal API Keys / MCP”自行创建 Key，无审批、世界或频道绑定。默认授权只读、有效期 90 天，可明确选择不过期；每用户最多 10 个未撤销 Key。

地址为公开站点基础路径加 `/mcp`，例如 `https://chat.example/mcp` 或子路径部署的 `https://chat.example/sealchat/mcp`。它与旧 `agt_` 世界访问链接无关。反向代理应保留站点基础路径，并在既有 `proxy.trustedProxies` 中配置可信代理，由可信代理提供 HTTPS 协议头；公网 HTTP 拒绝访问。MCP 路由只接受站点自身 Origin，无 Origin 的非浏览器请求可以访问。

这是 Go SDK v1.8.0 的 MCP Streamable HTTP，使用无会话状态及 JSON 响应。握手、协议版本协商与 JSON-RPC 格式由 SDK 处理。只声明 Tools；没有 Resources、Prompts、Sampling 或通用 Tasks。

## 个人 Key

每次 HTTP 请求带 `Authorization: Bearer sc_mcp_<publicId>.<secret>`。仅创建或轮换响应显示完整 Key；关闭界面后不可读取。数据库只保存 SHA-256 摘要和显示尾号，不接受 URL 参数、Cookie、网页登录 Token、BOT Token 或 `agt_` Token。

Key 只用于 `/mcp` 和专用上传端点，不能认证账号或平台管理 API。后续调用实时检查平台开关、Key 撤销及到期状态、用户存在及禁用状态。实际授权始终是 **平台允许 scopes ∩ Key scopes ∩ 当前业务权限**，不会沿用创建 Key 时的角色。

关闭平台后可以在个人信息查看和撤销已有 Key，但不能新建、编辑或轮换。停用模块只使对应授权暂时无效，保留 Key 保存的 scopes；编辑时允许保留或删除已有授权，但不能新增当前平台未开放的 scope。撤销、降权或关闭平台阻止后续调用，不能回滚已提交写入或取消已接受的原生 AI 任务。轮换使旧 secret 立即失效，不延长到期时间。

第一版是个人 Bearer 凭证接入（PAT），不实现 OAuth 授权端点、动态客户端注册或授权码流程。客户端须支持配置自定义 Bearer 凭证和 Streamable HTTP；只有 OAuth 自动授权能力的客户端不一定兼容。

## Scopes 与工具

工具由 42 个收敛为 28 个，不保留旧名称别名。17 个 scope ID 保持不变。模块的“读写”明确展开为下表中的读取与写入 scopes，不含未来能力，不使用通配符。额外授权默认关闭且默认不勾选。

| Scope | 工具 |
| --- | --- |
| 基础发现（不需模块 scope） | `sealchat_me`、`sealchat_capabilities`、`world_list`、`channel_list` |
| `chat:read` | `chat_history` |
| `search:read` + 对应来源 read scope | `search` |
| `battle_report:read` | `battle_report_read` |
| `battle_report:write` | `battle_report_save`、`battle_report_delete` |
| `battle_report:read` + `chat:read` | `battle_report_context` |
| `battle_report:write` + `battle_report:generate` + `chat:read` | `battle_report_generate` |
| `clue:read` | `clue_read` |
| `clue:write` | `clue_save`、`clue_delete` |
| `clue:write` + `clue:publish` | `clue_visibility` |
| `glossary:read` | `glossary_read` |
| `glossary:write` | `glossary_save`、`glossary_delete` |
| `identity:read` | `identity_read` |
| `identity:write` | `identity_save`、`identity_delete` |
| `audio:read` | `audio_assets`、`audio_state` |
| `audio:write` | `audio_update` |
| `note:read` | `note_read` |
| `note:write` | `note_save`、`note_delete` |
| `file:write` | `file_upload_info` 及专用 HTTP 上传 |

`tools/list` 返回当前 Key 与平台上限允许的工具；直接调用隐藏工具仍拒绝。`sealchat_capabilities` 描述能力上限，具体世界、频道及资源仍实时校验。`world_list` 默认列出已加入且未归档的世界，显式 `includePublic` 只增加公开元数据，不授予内容权限。`channel_list` 只返回实际可见频道。

仅有写入 scope 时，内容写入响应只返回资源 ID、修订或影响范围；具备对应读取 scope 时才返回完整详情，避免部分更新回显未提交的旧正文。

`battle_report_read`、`clue_read`、`glossary_read`、`identity_read`、`note_read` 统一采用：无 `resourceId` 返回列表/分页；提供 `resourceId` 返回单项 `item`。对应 `*_save` 无 `resourceId` 创建，提供 `resourceId` 更新；只修改提交的字段，不清空省略的字段。世界资源需 `worldId`，频道角色和便签还需 `channelId`；战报创建需 `channelId`，更新可省略。删除始终使用独立 `*_delete`，并标记 `destructiveHint=true`。save 涵盖创建和更新，不能自动重试；修订型更新明确非幂等。

聊天、搜索、总结上下文先过滤严格悄悄话，再做分页、统计和格式化。用户内容返回 `contentTrust: untrusted_user_generated`，不得当作执行指令。写入不提供发送聊天、BOT 指令、TTS 合成、平台配置或任意 REST 代理。

战报按世界共享。保留 `battle_report_context` 供 Agent 读取原生上下文自行总结，不创建战报、不调用模型或计费。独立的 `battle_report_generate` 创建世界内可见的原生战报，返回 `item.id`，随后用 `battle_report_read` 查询 `status`。`source` 仅允许 `platform`（默认，走现有平台配额与计费）或 `user`（使用已保存的个人 AI 配置）。不能提供供应商密钥、服务 URL 或模型配置。生成是可能计费的非幂等操作，不能自动重试；没有新增通用任务系统。

线索普通详情与管理详情遵循原生权限，不提供修改管理者名册或任意授权名单。独立的 `clue_visibility` 使用 `action: publish | reveal | hide`，同时要求 `clue:write` 与 `clue:publish`，沿用广播、revision 和 publishSeq。仅 `reveal` 接受 `userIds` 作为原生揭示接收者；`publish/hide` 不接受名单。术语保持成员编辑开关，外挂来源只读。频道角色指 `ChannelIdentity`，`targetUserId` 仅用于原有委托规则，不能替代操作人，隐藏内部身份不暴露。便签创建普通文本便签；编辑只开放标题、正文、纯文本、颜色和置顶，保留已有类型、外观、可见范围、名单及布局。

## 综合搜索

`search:read` 是综合搜索入口权限：允许 Agent 在已经获得读取权限的内容类型中执行统一检索；本权限不会额外授予任何数据读取权限。实际授权为 **search:read + 来源 read scope + 当前用户的业务资源权限**。

| source | 额外读取 scope |
| --- | --- |
| `worlds`、`channels` | 无；仍检查导航元数据/频道权限 |
| `messages` | `chat:read` |
| `clues` | `clue:read` |
| `battle_reports` | `battle_report:read` |
| `glossary` | `glossary:read` |
| `notes` | `note:read` |
| `identities` | `identity:read` |
| `audio_assets` | `audio:read` |

```json
{
  "query": "关键词",
  "sources": ["messages", "clues", "battle_reports", "glossary"],
  "worldId": "世界 ID",
  "channelIds": ["频道 ID"],
  "match": "fuzzy",
  "scope": "all",
  "page": 1,
  "limit": 30
}
```

省略 `sources` 自动展开当前 Key 实际授权且上下文适用的来源，包括基础导航来源；缺少 `worldId` 时只搜索世界元数据，其余已授权来源进入 `skippedSources`，音频工作台未开放时也跳过音频来源。显式 `sources` 包含未授权来源时返回 `scope_denied`，不静默忽略；显式来源缺少必要上下文时返回 `invalid_argument`，显式请求未开放的音频工作台仍返回权限错误。只拥有 `search:read` 的 Key 只能检索基础导航元数据，不能读取消息、线索等内容。

除 `worlds` 外都需要 `worldId`。省略 `channelIds` 时，消息、便签和身份搜索该世界内当前用户可见的频道；这些来源及频道元数据的显式频道必须通过原频道访问检查。战报沿用世界共享权限，`channelIds` 仅缩小来源范围。身份检索仅覆盖当前用户的正常可见身份，委托目标的详情仍由 `identity_read({ targetUserId, resourceId, ... })` 获取。`worlds` 默认仅已加入世界；显式 `includePublic` 增加公开元数据，仍不授予世界内容权限。

结果为统一摘要 envelope：`items[{source,id,title,snippet,worldId,channelId,updatedAt}]`、`searchedSources`、`skippedSources`、`page`、`limit`、`hasMore`、`contentTrust`。不返回完整资源正文、管理者备注、其他用户私人线索、不可见便签或音频存储路径。snippet 在业务可见性过滤后生成；全文通过对应 `*_read` 或 `chat_history` 获取。

消息复用现有全文检索、中文 fallback、`match=fuzzy|exact`、`scope=all|ic|ooc`、归档及严格悄悄话过滤；其他来源沿用现有关键词匹配。`from/to` 使用 RFC3339 时间，消息按创建时间、其他资源按更新时间过滤。底层未提供稳定可比较的相关度，因此不返回虚构 score，按时间倒序合并，同一时间按 source/id 确定排序。

采用有界 `page + limit`，不提供综合搜索 cursor：默认 `limit=30`、最大 100，`page` 最大 100 且 `page*limit` 最大 1000。provider 只保留受限候选，权限过滤使用分批查询，不先载入全部资源。到达 1000 结果上限后不再续页，有更多候选时返回 `resultLimitReached=true`；缩小关键词或范围继续检索。页码不代表读取授权，每次调用重新检查当前用户与 scopes。资源更新可能使后续页面移动。

## 分页、并发及错误

普通列表默认 `limit=50`、最多 200，沿用更小原生上限的列表会使用该上限。分页列表返回 `page/limit/total/hasMore`；`chat_history` 默认 `mode=messages`，返回 `nextCursor/hasMore/snapshotTo`，游标绑定当前用户、频道与筛选条件。续页须保留原筛选条件，不能换频道或移用他人游标。需要总数时使用同一工具的 `mode=count`，沿用同一可见性与筛选条件，不接受 cursor。

线索内容编辑/删除须传读取到的真实 `expectedRevision`；发布、揭示、隐藏须传 `expectedPublishSeq`（包括首次 0）。自动化编辑尊重原生字段编辑锁，便签也遵循当前编辑锁和删除权限。

`audio_update` 须传 `expectedScopeType/expectedScopeId/expectedRevision`，对应最近 `audio_state` 的 `scopeType/scopeId/revision`；作用范围变化也会冲突，不以相同 revision 混淆两个范围。未提交的音轨、位置、循环、倍速、场景和 `worldPlaybackEnabled` 保留；改变播放范围须显式提交 `worldPlaybackEnabled`。更新结果返回真实影响范围和新 revision。

HTTP 层区分 401（Key 无效）、403（Origin/HTTPS/上传 scope）、503（平台停用）和 429（限流）。进入工具执行后的参数、权限、资源修订冲突等返回 SDK `CallToolResult`：`isError=true`，`structuredContent` 与文本内容含稳定 `code/message`，例如 `invalid_argument`、`scope_denied`、`forbidden`、`not_found`、`conflict`、`rate_limited`、`operation_failed`。未知协议方法及错误版本按 SDK 处理。所有带用户数据响应禁止共享缓存。

默认每用户每分钟 120 次业务调用，其中写入最多 30 次；多个 Key 共用用户额度。HTTP 请求另有有界保护额度。限流不替代 AI 配额或文件大小限制。写入日志仅包含追踪 requestId、公开 keyId、操作人、工具、目标、结果和耗时；requestId 不是幂等键。

## 文件上传

先调用 `file_upload_info` 获取同站点相对路径和实际 `maxBytes`。客户端必须另外具备 HTTP multipart 文件上传能力；任意 MCP 客户端不一定能直接上传本地文件。不要把大文件 Base64 放进 MCP JSON。

```bash
curl 'https://chat.example/sealchat/api/v1/mcp/uploads' \
  -H 'Authorization: Bearer sc_mcp_<publicId>.<secret>' \
  -F 'file=@./clue.png'
```

只接受一个 `file` 字段，不接受 owner/rootId/parentId、服务器路径或 URL 代下载。服务端检测真实 MIME、限制大小，沿用本地/S3 保存路径；没有全局哈希秒传，MCP 附件也不参加旧上传链路的哈希复用。响应仅返回附件 ID、文件名、大小、MIME 和临时标记，不暴露存储内部字段。

上传本身不授予资源写入权。将返回 ID 用于 `clue_save.imageAttachmentId` 或 `identity_save.avatarAttachmentId` 时，目标业务检查当前权限和附件归属后才绑定转正。没有通用 attachment-confirm 工具；未绑定的 MCP 临时附件超过 24 小时后，由既有每小时清理 worker 分批清理。已绑定附件不参与此清理；存储清理失败会在后续轮次重试。Key 不出现在上传地址或工具结果中。
