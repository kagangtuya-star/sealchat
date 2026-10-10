# 平台 MCP 接入

平台管理员在“平台管理 → MCP 接入”开启服务并配置模块上限，使用标题栏的保存按钮。默认关闭。用户在共用的“个人信息 → Personal API Keys / MCP”自行创建 Key，无审批、世界或频道绑定。默认授权只读、有效期 90 天，可明确选择不过期；每用户最多 10 个未撤销 Key。

地址为公开站点基础路径加 `/mcp`，例如 `https://chat.example/mcp` 或子路径部署的 `https://chat.example/sealchat/mcp`。它与旧 `agt_` 世界访问链接无关。`config.yaml` 的 `domain` 可用 `;` 分隔多个公开域名，例如 `https://chat.example;https://chat-tunnel.example`；第一个域名仍作为需要单一公开地址的默认主域名，MCP Host/Origin 校验接受配置中的任意域名。反向代理应保留站点基础路径，并在既有 `proxy.trustedProxies` 中配置可信代理，由可信代理提供 HTTPS 协议头；默认仅信任本机回环代理 `127.0.0.1` 与 `::1`，Docker/远程代理仍需显式配置实际地址或网段。公网 HTTP 拒绝访问。MCP 路由只接受当前请求对应的已配置站点 Origin，无 Origin 的非浏览器请求可以访问。

这是 Go SDK v1.8.0 的 MCP Streamable HTTP，使用无会话状态及 JSON 响应。握手、协议版本协商与 JSON-RPC 格式由 SDK 处理。只声明 Tools；没有 Resources、Prompts、Sampling 或通用 Tasks。

## 个人 Key

每次 HTTP 请求带 `Authorization: Bearer sc_mcp_<publicId>.<secret>`。仅创建或轮换响应显示完整 Key；关闭界面后不可读取。数据库只保存 SHA-256 摘要和显示尾号，不接受 URL 参数、Cookie、网页登录 Token、BOT Token 或 `agt_` Token。

Key 只用于 `/mcp` 和专用上传端点，不能认证账号或平台管理 API。后续调用实时检查平台开关、Key 撤销及到期状态、用户存在及禁用状态。实际授权始终是 **平台允许 scopes ∩ Key scopes ∩ 当前业务权限**，不会沿用创建 Key 时的角色。

关闭平台后可以在个人信息查看和撤销已有 Key，但不能新建、编辑或轮换。停用模块只使对应授权暂时无效，保留 Key 保存的 scopes；编辑时允许保留或删除已有授权，但不能新增当前平台未开放的 scope。撤销、降权或关闭平台阻止后续调用，不能回滚已提交写入或取消已接受的原生 AI 任务。轮换使旧 secret 立即失效，不延长到期时间。

SealChat MCP 同时支持 **Personal API Key / PAT** 和 **OAuth 2.1 Authorization Code + PKCE**。PAT 适合 Claude、Codex 和支持自定义 Bearer 的普通 MCP Client；OAuth 主要用于 ChatGPT 等标准 OAuth MCP Client。两种凭证最终解析成同一个 `MCPActor`，完整复用 39 个工具、24 个 scopes、实时业务 ACL、限流和审计日志。OAuth access/refresh token 不能认证普通 SealChat REST API，也不会转换成网页登录 token。

## ChatGPT OAuth

在 ChatGPT 自定义 MCP 中选择 **OAuth** Authentication 模式，再选择 **CIMD**（public client token endpoint authentication 为 `none`），填写本站公开 HTTPS MCP URL，不提供静态 client credentials。此版本只预认可 ChatGPT stable public client：`client_id=https://chatgpt.com/oauth/client.json`，回调精确为 `https://chatgpt.com/connector_platform_oauth_redirect`。不填写 client secret；若客户端要求任意 client ID、secret 或动态注册，本版不支持该连接方式。不支持 DCR、OIDC、ID Token、JWT/JWKS、client_credentials、password 或 implicit flow，默认不需要额外 OAuth 平台配置。选择纯 OAuth，因为 MCP 握手和 tools/list 同样要求认证；官方接入说明见 [Add custom MCP server](https://developers.openai.com/api/docs/guides/custom-mcp-server) 和 [Authentication](https://developers.openai.com/plugins/build/auth)。

默认执行在线 CIMD 获取与校验：只 GET 上述固定 HTTPS URL，不接受任意 client URL，也不跟随 HTTP redirect。专用 HTTP Client 总超时 5 秒，JSON body 上限 64 KiB，要求 2xx 与 JSON Content-Type。文档须声明精确 client_id、固定 redirect，以及 `code`、`authorization_code`、`none` 能力；可以同时列出其他认证方法，但 SealChat 只接受 public client `none`。成功验证的必要 metadata 在进程内缓存 1 小时，TTL 内不重复请求；冷启动或缓存过期后获取失败时返回 `temporarily_unavailable`，不使用过期缓存放行。参数校验先于远程请求，非法 client_id 不触发网络访问。两种模式均保留 `client_id_metadata_document_supported=true`，使 ChatGPT 继续选择同一个 CIMD client identity；兼容模式仅改变 SealChat 是否在线获取 metadata。

标准公开 discovery 路径（不需要 Bearer，响应 `application/json`、`Cache-Control: no-store`）：

- `GET /.well-known/oauth-protected-resource`
- `GET /.well-known/oauth-authorization-server`

这些路径始终注册在站点根目录。子路径部署同时兼容 `<webUrl>/.well-known/...`，以及 `/.well-known/oauth-protected-resource<webUrl>/mcp` 的 resource 路径形式。MCP 401 响应的 `WWW-Authenticate` 指向当前域名的根 protected-resource metadata。

完整流程：

1. ChatGPT 发现 metadata，使用 `response_type=code`、固定 client/redirect、空格分隔 scope、opaque state、PKCE `S256` challenge 和精确 resource 访问 `<webUrl>/oauth/authorize`。
2. 服务端验证平台开关、固定 client/redirect、PKCE S256、精确 resource、每个已知且开放的 scope 和当前可信 Host，默认完成上述在线 CIMD 校验；显式开启兼容模式时使用内置固定身份。随后创建 5 分钟有效的随机 authorization request，跳转到 `/#/oauth/mcp/authorize?request=...`（保留部署子路径）。
3. 前端复用现有登录页和 API Authorization header。登录后显示当前账号、中文权限描述和写入、AI 额度、线索发布风险，用户明确允许或拒绝。`GET/POST <webUrl>/api/v1/mcp/oauth/requests/:requestId` 仅供普通网页登录用户使用。
4. 允许将当前登录 userId 绑定请求，返回 ChatGPT 回调 URL；一次性 code 有效 2 分钟。拒绝返回 `error=access_denied`。授权成功及安全错误跳转都携带原样 state 和 `iss`；非法 client/redirect 在本站直接拒绝。
5. ChatGPT 以 `application/x-www-form-urlencoded` POST 到 `<webUrl>/oauth/token`，携带 `grant_type=authorization_code`、code、client_id、redirect_uri、resource、code_verifier。服务端校验 S256 和 issuer/resource 后返回 opaque access/refresh token，无 ID Token。
6. access token 格式 `sc_oauth_<publicId>.<secret>`，有效 **1 小时**；refresh token 格式 `sc_oauth_r_<publicId>.<secret>`，有效 **30 天**。数据库 `mcp_oauth_grants` 只保存 SHA-256 secret 摘要。`grant_type=refresh_token` 必须携带固定 client_id、原 resource 和 refresh_token；每次刷新轮换两个 secret，旧 access/refresh 立即失效，refresh 原始到期点不延长。每个连接独立创建 grant。

每次调用仍取 **平台 AllowedScopes ∩ OAuth 原授权 scopes ∩ 当前实时业务权限**。平台关闭某 scope 不改写原 grant，但立即影响 `tools/list`、`tools/call` 与上传；刷新不自动扩大授权。用户禁用、删除、BOT 状态、凭证撤销或到期均拒绝后续调用。MCP 关闭时 discovery 仍可读取，authorize/token 不签发权限，MCP 返回既有 `503 mcp_disabled`。

多 domain 的 OAuth issuer 是**当前已匹配 config.domain 的 HTTPS origin**，resource 是 `origin + <webUrl>/mcp`，不固定使用主域名。请求 Host 必须先匹配已配置域名；仅信任现有 trusted proxy 提供的 forwarded Host/proto。domainA 与 domainB 各自独立签发，domainA 的 code、refresh 和 access token 不能用于 domainB。OAuth 始终要求 HTTPS；PAT 既有本机回环开发例外保留。

临时 authorization request/code 保存在带 mutex 的进程内存中；**服务重启会使进行中的授权流程失效**，需要从 ChatGPT 重新连接。多实例部署应使 authorize、consent 与 code exchange 落到同一进程；持久 grant 刷新使用数据库条件更新防止并发重放。现有数据库清理 worker 删除 refresh 到期超过 7 天的 grant，不增加独立 worker。

公开 authorize/token 另有进程内 IP 固定窗口限流：每 IP 每分钟分别最多 30/60 次，IP 复用 Fiber 配置的可信代理规则。两个端点共用最多 4096 个 bucket，过期条目懒清理；容量满时拒绝新 bucket。命中限流返回本站 HTTP 429、OAuth `temporarily_unavailable` JSON、`Cache-Control: no-store` 和 `Retry-After`，不跳转未验证回调；discovery 不限流。authorization request/code 合计最多 4096 条。这些限额按进程计算，不是分布式限流。

用户可在“个人信息 → Personal API Keys / MCP”下方的“OAuth 连接”查看 ChatGPT 原授权 scopes、创建/最近使用/refresh 到期时间及状态，并确认撤销。`GET /api/v1/user/mcp-oauth-grants` 和 `DELETE /api/v1/user/mcp-oauth-grants/:id`（均保留部署基础路径）只接受现有网页登录认证，查询和撤销仅限自己的 grant；平台关闭后仍可操作。列表使用明确字段白名单，不返回 token、public token ID、hash、issuer 或 resource。撤销使该连接 access/refresh 同时失效；其他连接和 domain 的 grant 不受影响。不支持编辑 scope，变更授权须重新连接；本版没有 RFC7009 撤销端点。

SDK v1.8.0 尚无顶层 Tool `securitySchemes`，本版为所有工具输出 `_meta.securitySchemes=[{type: "oauth2", scopes: [...]}]`，作为兼容镜像；这是当前 SDK 的字段限制，并不提供标准顶层字段。基础发现工具的 scopes 为空数组但仍需认证。保持原 annotations 和 SDK 版本。

OAuth 直接调用缺少 scope 的工具时，仅当工具要求的 scopes 仍全部由平台开放、且原 grant 缺少授权时，错误结果额外携带 `_meta["mcp/www_authenticate"]` challenge 数组，例如 `Bearer resource_metadata="https://当前域名/.well-known/oauth-protected-resource", error="insufficient_scope", error_description="Additional authorization is required", scope="clue:write"`；多个缺失 scope 去重并稳定排序。PAT scope 不足、平台关闭 scope、业务 ACL 拒绝、参数错误、revision conflict 和资源不存在均不发此信号。`structuredContent`、文本内容与 `isError=true` 保持原格式；HTTP 401 使用同一标准 Bearer challenge 生成逻辑并返回 `invalid_token`。`tools/list` 继续按有效 scope 过滤，不为重授权暴露隐藏工具，因此客户端能否自动触发重授权仍取决于其缓存或直接调用行为。

排障顺序：

1. 确认 MCP 已开启、模块 scopes 已开放，ChatGPT 选择 OAuth 且没有 client secret。
2. 在实际连接域名读取两个根 discovery 路径，核对 issuer、resource、authorize/token URL 和 HTTPS；子路径部署的反向代理也要转发根 well-known 路径。
3. `invalid_host` 检查 `config.domain` 和代理 Host；`https_required` 检查 trusted proxy 与 forwarded proto，不通过信任任意 Host 解决。
4. `invalid_scope` 检查请求是否只含 24 个已知 scope 且平台当前开放；`invalid_client/invalid_request` 检查固定 client/redirect、resource、S256 和 form 编码。
5. `expired/invalid_grant` 检查授权超时、服务重启、跨 domain、code 重放、verifier、连接撤销和 refresh rotation，重新连接获取新请求。MCP 401 可先查看 `resource_metadata` challenge，再检查用户状态和 access token 到期；503 检查平台开关，429 检查公开端点 IP 限流或共用用户限流。authorize 的 `temporarily_unavailable` 也可能是 CIMD 不可达/不符合必要能力或临时 store 已满。

真实 ChatGPT OAuth 联调仍需要部署域名、HTTPS/代理配置及根 well-known 路由；默认模式还需要服务器可访问固定 CIMD URL，显式兼容模式无需这项出站访问。本地 stub/HTTP 测试不代表 ChatGPT 端到端连接已验证。

### ChatGPT 固定客户端兼容模式

默认关闭（`mcp.chatgptFixedClient: false`，旧配置未包含该字段时也为 false）。默认 SealChat 在线读取并验证 `https://chatgpt.com/oauth/client.json`，服务器网络允许时推荐保持关闭。

仅当 SealChat 服务器无法访问 `chatgpt.com` 时，由管理员在“平台管理 → MCP 接入 → OAuth 兼容设置”（默认折叠）显式开启“ChatGPT 固定客户端兼容模式”，或设置 `mcp.chatgptFixedClient: true`。开启后使用 pinned ChatGPT CIMD identity，不执行远程 metadata fetch；新 authorization request 只接受内置固定参数：

- `client_id`: `https://chatgpt.com/oauth/client.json`
- `redirect_uri`: `https://chatgpt.com/connector_platform_oauth_redirect`

此模式不会放宽为任意 OAuth client，不接受 client secret、DCR client 或其他 CIMD URL，也不是通用 predefined OAuth client registry。平台开关、scope、resource、response_type=code 和 PKCE S256 校验继续生效。不会在 CIMD 失败时自动 fallback，可以随时关闭并恢复在线 CIMD 校验。

ChatGPT 仍填写原公开 `/mcp` 地址并选择 OAuth/CIMD，无需新建 client secret、手工填写新 client_id、改用 DCR 或更改 callback；用户登录和授权流程不变。切换模式不会主动撤销或改变已有 OAuth grant，access/refresh token 继续按既有到期、撤销、用户状态、平台 scope 与 resource 规则验证。PAT 不受影响，token endpoint 不进行 CIMD 网络请求。

## Scopes 与工具

原有 28 个工具、17 个 scope ID 保持不变；小剧场第一期新增 7 个工具及 5 个 scopes；频道嵌入（小剧场第二期）新增 4 个工具及 `embed:read`、`embed:write` 2 个 scopes。模块的“读写”明确展开为下表中的读取与写入 scopes，不使用通配符。Theater 与频道嵌入模块默认关闭；控制、截图、聊天副作用额外授权默认关闭。旧 Key 不会自动获得新增 scopes，`theater:write` 也不包含 `embed:write`。

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
| `theater:read` | `theater_read`、`theater_catalog`、`theater_view(get)` |
| `theater:write` | `theater_scene`、`theater_object`；scene apply 另需 control |
| `theater:control` | `theater_control`；view 写操作另需 read |
| `theater:capture` | `theater_capture` |
| `theater:chat` | control 执行保存的 `chat.send/insert/random-table` 时的额外权限，不是通用发送接口 |
| `embed:read` | `embed_read`、`embed_catalog` |
| `embed:write` | `embed_save`、`embed_delete`；仍需频道 iForm 管理权限，不含平台模板管理 |

`tools/list` 返回当前 Key 与平台上限允许的工具；直接调用隐藏工具仍拒绝。`sealchat_capabilities` 描述能力上限，具体世界、频道及资源仍实时校验。`world_list` 默认列出已加入且未归档的世界，显式 `includePublic` 只增加公开元数据，不授予内容权限。`channel_list` 只返回实际可见频道。

仅有写入 scope 时，内容写入响应只返回资源 ID、修订或影响范围；具备对应读取 scope 时才返回完整详情，避免部分更新回显未提交的旧正文。

`battle_report_read`、`clue_read`、`glossary_read`、`identity_read`、`note_read` 统一采用：无 `resourceId` 返回列表/分页；提供 `resourceId` 返回单项 `item`。对应 `*_save` 无 `resourceId` 创建，提供 `resourceId` 更新；只修改提交的字段，不清空省略的字段。世界资源需 `worldId`，频道角色和便签还需 `channelId`；战报创建需 `channelId`，更新可省略。删除始终使用独立 `*_delete`，并标记 `destructiveHint=true`。save 涵盖创建和更新，不能自动重试；修订型更新明确非幂等。

聊天、搜索、总结上下文先过滤严格悄悄话，再做分页、统计和格式化。用户内容返回 `contentTrust: untrusted_user_generated`，不得当作执行指令。不提供通用聊天发送、BOT 指令、TTS 合成、平台配置或任意 REST 代理；仅 Theater 保存动作在显式 `theater:chat` 授权和真实频道权限下可产生聊天副作用。

小剧场结构化操作、浏览器协作授权、截图、频道嵌入 MCP 与嵌入事件绑定见 [Theater MCP](theater-mcp.md)。这不是 MCP 通用 Tasks：任务状态通过 Theater 工具自己的 `execution_status` / `status` 查询。

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

HTTP 层区分 401（MCP 凭证无效）、403（Origin/HTTPS/上传 scope）、503（平台停用）和 429（限流）。进入工具执行后的参数、权限、资源修订冲突等返回 SDK `CallToolResult`：`isError=true`，`structuredContent` 与文本内容含稳定 `code/message`，例如 `invalid_argument`、`scope_denied`、`forbidden`、`not_found`、`conflict`、`rate_limited`、`operation_failed`。未知协议方法及错误版本按 SDK 处理。所有带用户数据响应禁止共享缓存。

默认每用户每分钟 120 次业务调用，其中写入最多 30 次；多个 PAT/OAuth grant 共用用户额度。HTTP 请求另有有界保护额度。限流不替代 AI 配额或文件大小限制。写入日志仅包含追踪 requestId、公开凭证 ID（兼容原 `keyId` 字段）、credentialType、操作人、工具、目标、结果和耗时；requestId 不是幂等键。`sealchat_me.keyId` 同样返回当前 PAT 或 OAuth grant 的公开记录 ID，不返回 secret。

## 文件上传

先调用 `file_upload_info` 获取同站点相对路径和实际 `maxBytes`。客户端必须另外具备 HTTP multipart 文件上传能力；任意 MCP 客户端不一定能直接上传本地文件。不要把大文件 Base64 放进 MCP JSON。

```bash
curl 'https://chat.example/sealchat/api/v1/mcp/uploads' \
  -H 'Authorization: Bearer sc_mcp_<publicId>.<secret>' \
  -F 'file=@./clue.png'
```

只接受一个 `file` 字段，不接受 owner/rootId/parentId、服务器路径或 URL 代下载。服务端检测真实 MIME、限制大小，沿用本地/S3 保存路径；没有全局哈希秒传，MCP 附件也不参加旧上传链路的哈希复用。响应仅返回附件 ID、文件名、大小、MIME 和临时标记，不暴露存储内部字段。

上传本身不授予资源写入权。将返回 ID 用于 `clue_save.imageAttachmentId` 或 `identity_save.avatarAttachmentId` 时，目标业务检查当前权限和附件归属后才绑定转正。没有通用 attachment-confirm 工具；未绑定的 MCP 临时附件超过 24 小时后，由既有每小时清理 worker 分批清理。已绑定附件不参与此清理；存储清理失败会在后续轮次重试。Key 不出现在上传地址或工具结果中。
