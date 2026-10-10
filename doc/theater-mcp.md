# Theater MCP

第一期复用现有场景、普通对象、StageAction、序列器、频道 iForm 和 `surfaceEmbeds`。不增加 widget、代码执行环境、嵌入编辑器、通用 Bridge 注入、Playwright 或布局事务。全部 durable 写入经 `ApplyTheaterMutation`，沿用权限、schema、CAS、checksum、审计和房间事件。

第二期增加频道嵌入 MCP（`embed_*` 工具，独立 `embed:read/embed:write`）和普通 iframe 对象的“嵌入事件 → 已保存 StageAction”绑定，见文末[第二期](#第二期频道嵌入-mcp-与嵌入事件联动)。仍不新增 widget 类型、第二套 SDK、动作执行器或序列器。

第三期增加 `theater_design` 的 Design Plan 原子事务、确定性布局、场景/对象子树复制及 Theater 专用音频发现，见[第三期](#第三期design-plan--ai-布景闭环)。复用同一原生 Theater document、mutation、权限和 renderer。

## 作用域和坐标

所有工具均传 `worldId`、`scopeType: world | channel`。主小剧场为 `scopeType=world, channelId=""`（可省略 channelId）；频道 Theater 必须传该世界内的 channelId。`inputChannelId` 只表示可读的聊天上下文，绝不替代 Theater 房间 channelId。响应 `scope` / 任务 `scope` 给出实际房间。

普通对象的 x/y 是中心锚点世界坐标，`WORLD_UNIT_PX=24`；子对象还受父组变换影响。相机 x/y 是视口平移像素，zoom 为缩放。特效使用独立 1920×1080 设计坐标，不可直接把截图像素当对象或特效坐标。

`theater_read(summary)` 返回投影后的 revision/checksum、当前场景、场景目录、指定或当前场景加常驻对象摘要、权限和同账号 renderer。目录与对象分别分页；`page/limit` 默认 1/50，最多 200。`scene` 返回一个场景的完整 state 和分页对象，`object` 返回一个完整对象，`events` 按 afterRevision 读取原生权限过滤事件，`renderers` 读取协作端。隐藏对象、隐藏父组与未发布场景沿用快照权限投影，不由 MCP 放行。

`theater_catalog` 的 `object_types/effects/overlays/resources/limits` 复用原生类型/特效 allowlist、场景叠层 preset 服务与房间资源。浏览器注册时提供现有叠层注册表及特效默认配置；未授权在线 renderer 时，这部分 runtimeCatalog 为空。房间 resources 不是公共素材全库搜索。

## 工具操作

| 工具 | operation |
| --- | --- |
| `theater_read` | summary、scene、object、events、renderers |
| `theater_catalog` | object_types、effects、overlays、resources、audio_resources、limits |
| `theater_design` | validate、apply |
| `theater_scene` | create、update、duplicate、reorder、delete、apply、folders_update、field_update、surface_update、surface_embed_set、surface_embed_clear、overlay_update、music_update、transition_update、sequence_update |
| `theater_object` | create、update、duplicate、batch_update、delete、toggle、bind_character |
| `theater_control` | apply_scene、trigger_action、trigger_sequence、execution_status、cancel_execution |
| `theater_view` | get、camera_set、fit_scene、focus_object、select_objects、clear_selection |
| `theater_capture` | capture、status、cancel |
| `embed_read` / `embed_catalog` / `embed_save` / `embed_delete` | 频道嵌入（第二期，独立 scopes，非 operation 工具） |

各工具 JSON Schema 列出类型化字段及 operation 枚举。禁止任意 mutation type/payload、管理员恢复及 JavaScript。对象类型维持 `group/drawing/text/image/button/character/video/effect/iframe`，原生 schema 是最终限制；未知类型/字段拒绝。

场景 create 用 `sceneId, fields.name`；update 仅接受 name/switchText/order/folderId/locked/published。场景局部配置分别使用对应 operation，不能提交整份 state：surface_update 修改单个前景/背景图片、样式及场地尺寸；overlay_update/music_update/transition_update/sequence_update 分别接受 overlays/music（及 switchAudio）/transition/sequences。music_update 的 clear 清除音乐和切换音效。

对象 create 用 `objectId, sceneId, kind, fields`（sceneId 省略/null 为常驻）；update 用 objectId/fields。fields 支持名称、父组、位置、大小、旋转、缩放、z/orderKey、显隐、锁定、交互、可编辑、content/actions，以及仅 iframe 可用的 embedEventBindings（第二期）。content/actions/embedEventBindings 遵循整体替换语义，修改前先读取完整字段。`batch_update` 用 `updates[{objectId,fields}]` 复用原子 `object.batchUpdate`，只能更新已有对象；不支持混合创建、更新、删除事务。toggle 要求显式 visible 目标，重试不反向翻转。bind_character 用 identityId/ownerUserId，世界 Theater 的角色频道由 inputChannelId 指定；原生 character.bind 新增可选 inputChannelId，省略时保持原频道房间语义。

每次持久化请求必须带新的 `mutationId` 和读取到的 `expectedRevision`。相同 Key、相同参数的成功请求可幂等重试；已失败 ID、跨 Key 或改参数/改 revision 的复用拒绝。revision conflict 返回 currentRevision；重新读取、规划并使用新 ID，不自动覆盖。局部 scene state 在相同 revision 读取完整状态、合并目标字段、调用原生 scene.update，保留另一 surface、音乐、叠层、序列和其他配置。MCP 来源通过原生 mutation/audit 的 `RequestSource=mcp` 与凭证关联记录。

## 已有频道嵌入

普通 iframe：

```json
{
  "worldId": "WORLD", "scopeType": "world", "inputChannelId": "CHAT",
  "operation": "create", "expectedRevision": 7, "mutationId": "layout-iframe-1",
  "sceneId": "SCENE", "objectId": "FRAME", "kind": "iframe",
  "fields": {
    "x": 0, "y": 0, "width": 16, "height": 9, "interactive": true,
    "content": {"iframe": {"url": "https://chat.example/#/internal/iform/FORM?world=WORLD&channel=CHAT", "scale": 1}}
  }
}
```

后续 update/batch_update 可改坐标、尺寸、parentId、z/orderKey 和 interactive。内部链接必须使用配置可信 domain/webUrl，匹配 world/inputChannel；频道 Theater 还要匹配 channelId，并验证 Actor 能读该频道且已有 effective iForm 存在。沿用 StageIframeFrame 的内部解析、IFormEmbedFrame、theaterCharacterSource 及频道权限，不复制 embedCode。

当前场景前景/背景用 `surface_embed_set`，字段 `sceneId, target: background|foreground, url, scale(默认1), interactive(默认false)`；clear 用 sceneId/target。持久化仍为 `surfaceEmbeds.background/foreground` 的 StageSurfaceEmbed，不改变视觉层序。scale 合法范围 .25..5；关闭 interactive 时舞台接管操作，开启时内部接收输入，原开关语义不变。

## 权限与执行

平台 Theater 模块默认 off；read/write 不自动包含额外控制、截图或聊天。Key/OAuth 需分别明确授权 `theater:read/write/control/capture/chat`，并受实时业务权限限制。`theater:write` 不授予频道嵌入编辑权限；嵌入代码只能经独立的 `embed:write`（第二期）修改。scene apply 另检查原生场景切换权限；read 无法切换场景。

control 只执行已保存的动作（objectId，可选 actionId）或当前场景序列（sequenceId）；iframe 对象没有点击语义，必须显式传 actionId 且该动作已被其 embedEventBindings 引用，场景序列的 object.trigger 也不能指向 iframe。必须传当前 sceneId、expectedRevision、rendererId。服务端展开最多 256 节点、8 层；原生动作和序列 timing/schedule 调度在指定浏览器执行。每个业务 leaf 经服务器以发起 MCP Actor 和当前有效凭证重新鉴权，浏览器不能供应动作 payload 或借用自己的登录权限。chat.send/insert/random-table 另需 theater:chat；clue.execute 另需 clue:write/publish 和线索权限；带音频的 effect.play 另需 audio:write 及共享音频业务权限。

`object.trigger` 属于现有场景序列器步骤（触发目标对象已保存动作），不是普通对象可保存的原子点击动作；catalog 分别列出 actions 和 sceneSequenceActions，不扩展原生动作 schema。

返回任务 `requestId`（作为 executionId/captureId）、rendererId、scope、sceneId、revision 和 pending/running/stopping/completed/failed/cancelled。查询/取消须保留同一账号、MCP 凭证和 Theater room（worldId/scopeType/channelId）；execute 还须保留 inputChannelId，view/capture 不要求同一聊天上下文。任务 scope 保留 MCP 调用方上下文，视觉命令使用 renderer 自己登记的完整 scope。服务端 descriptor 不是完成状态；只有浏览器回应完成且各 leaf 已确认才 completed。视觉 effect 仅发给指定 renderer，不广播重复播放。MCP scene.apply 只提交原生切换，不调用手动切场景附带的聊天/共享音频副作用。取消停止后续步骤，不能回滚已发生的业务副作用；超时、断线、revision/场景变化、未提交编辑均拒绝或失败。

若取消、超时或断线发生在服务端步骤执行期间，任务先进入 `stopping`，待该步骤返回后再变为终态，期间拒绝新步骤。`inFlightStepId` 表示正在运行的步骤，`completedStepIds` 记录已确认的服务端步骤，`partialSideEffectsPossible` 表示失败/取消后可能仍有部分业务效果（即使服务端操作曾返回错误也不能保证零副作用）。视觉渲染或本地动作失败不清除已确认步骤；不能直接重放整个 execution，应先检查当前房间状态，再明确决定后续动作。

## 浏览器协作和本地视图

用户在小剧场点击顶部闪电图标打开“聊天桥接”弹层，在“舞台同步”下方手动开启“AI 协作”开关并确认，随后浏览器会请求共享当前标签页以提供完整截图；应选择当前 SealChat 标签页。完成后才登记本页面 renderer；关闭开关、停止共享、关闭页面、显式断开、身份改变或切世界都会撤销授权并停止捕获流。Gateway 进入 connecting/reconnecting 时保留 rendererId 与共享流，并给予约 10 秒恢复窗口；重新 connected 后尝试恢复登记，只有登记成功才结束恢复，窗口超时才撤销。同一 World Theater 内切聊天频道只更新登记的 inputChannelId，保留 rendererId 和共享流。首次登记必须成功；后续 scene/revision 变化、heartbeat 和命令完成只在同步 clean 时刷新，普通失败按立即、300ms、1000ms 有限重试，连续三次失败才撤销；连接恢复窗口内的登记失败按短间隔重试但不会超过恢复 deadline。dirty 草稿不会被自动提交或导致撤销。默认没有授权，普通玩家不会自动登记。MCP Actor 必须与浏览器同账号、同 Theater room；execute 还必须同聊天上下文。每次只向一个 renderer 定向分发；没有授权端返回 renderer_unavailable。

登记包含 userId/worldId/scopeType/channelId/inputChannelId、activeSceneId、revision、viewport/camera、capabilities 和注册表。heartbeat 15 秒、登记 TTL 60 秒，过期懒清理；运行请求另有有界超时。仅支持当前单实例进程内 broker，无 Redis、无跨实例路由；服务重启丢失协作登记及截图。任务/结果保留约 2 分钟（懒清理），每 renderer 同时 1 个、每用户 2 个、全局 8 个任务，最多保留 128 个任务/renderer 登记。

view get 返回实际相机/视口/选择（任务 result）及场景/revision；camera_set 的 camera 为 x/y/zoom，fit_scene 按场地，focus_object 接一个 objectIds，select_objects 最多 200，clear_selection 清除选择。这些只修改现有 Store 的本地相机/选择，不发 Theater durable mutation，不影响其他客户端。非 get 操作额外需要 theater:control。view 超时 8 秒。

view/capture 可以省略 rendererId，sceneId 和 expectedRevision 仍必填。仅筛选同账号、同 room、当前场景一致、具备对应 capability 且未过 TTL 的登记：零个返回 renderer_unavailable，一个自动选择，多个返回 renderer_ambiguous 和候选 rendererIds。显式 rendererId 只使用指定实例，不自动替换；execute 仍必须显式选择 renderer。

## 截图

capture 请求传可选 rendererId、当前 sceneId、expectedRevision、mode(viewport/object)、可选 objectId、maxEdge(默认1600，64..1600)、maxBytes(默认2097152，1024..2097152)。object 模式裁剪该对象在当前视口内的边界，不自动改相机；完全在视口外时失败。字体、图片、可见视频 readiness、入场/场景媒体/切换状态采用事件或帧检查与有界超时，不用固定 sleep。renderer 不提交或覆盖用户的未提交编辑；截图期间相机、视口、scene/revision 或草稿变化拒绝。

截图优先使用浏览器共享的当前标签页合成画面：隐藏舞台编辑辅助层后等待下一帧，按 Stage 根节点在浏览器视口中的实际位置裁剪，因此 Konva、DOM、内部频道 iForm、surfaceEmbeds、普通跨域 iframe 和视频都按浏览器最终合成像素进入结果；object 模式继续在此基础上二次裁剪对象区域。该路径返回 includedLayers=[browser-composite]，正常情况下为 complete。

若浏览器不支持标签页捕获、共享流失效或画面尺寸无法可靠映射，则回退原有 DOM 合成：Konva canvas 使用原生导出，DOM 克隆通过成对锚点维持层序，iframe/iForm 仍使用灰色占位并返回 partial。回退路径不放宽 iframe sandbox，也不尝试跨域读取。complete 表示浏览器合成路径成功或回退路径未检测到缺失；真实像素仍需浏览器验收。

capture 启动返回 captureId/task；用 status 查询。完成的 status 返回真正 MCP ImageContent（PNG/JPEG）和 structuredContent/TextContent 元数据：captureId、rendererId、sceneId、revision、status(complete/partial)、width/height/mimeType、coordinateMapping、includedLayers、missingLayers、unsupportedObjects、warnings。图片只在进程内短期保存，不上传图库/CDN，仅发起用户与同一 Key/OAuth 凭证可读取/取消，capture 工具不能被 control status 绕过。

coordinateMapping 中 `worldOriginPx` 表示世界原点在输出图上的像素，`pixelsPerWorldUnit` 为普通世界单位对应像素：`worldX=(imageX-worldOriginPx.x)/pixelsPerWorldUnit`，Y 同理。另含 camera、crop、worldUnitPx=24、anchor=center 和 effectDesignSize；这是舞台世界坐标，不是父组局部坐标或特效设计坐标。截图任务超时 15 秒；取消/超时不保证底层不可取消 DOM 克隆立即结束，但其响应不再被接受，临时标记会在 finally 恢复。

## 验证边界

新增 Go 用例覆盖 scope/projection、state merge/CAS/幂等、对象 iframe 与 surfaceEmbeds、内部链接、scopes、renderer 生命周期和任务/图片结果；前端脚本覆盖定向消息、dirty/replay/断线、序列取消、真实克隆回调语义和 partial 坐标映射。运行 `node scripts/theater-mcp-renderer.spec.cjs` 以及现有 `node scripts/run-theater-dialogue-tests.cjs`。

单元测试不等同真实浏览器合成验证。真实双客户端同步、混合图层光栅、iForm 交互、字体/媒体以及普通用户完整手动流程仍需浏览器验收；不声称这些已经验证。

## 第二期：频道嵌入 MCP 与嵌入事件联动

### 工具与 scopes

平台“MCP 接入”新增“频道嵌入”模块（默认关闭），`read` 展开为 `embed:read`，`write` 展开为 `embed:read + embed:write`。旧 Key、`theater:*` 不会获得这些 scope；Key/OAuth 仍需逐项授权，所有调用继续检查 Key 所属用户的世界成员、频道可读与频道 iForm 管理权限（`func_channel_iform_manage` 或频道创建者）。

| 工具 | scope | 说明 |
| --- | --- | --- |
| `embed_read` | `embed:read` | 无 `resourceId`：当前频道有效 iForm（含世界共享引用）的轻量列表，只给 `embedCodeBytes`，不返回源码；有 `resourceId`：详情（embedCode、URL、尺寸、媒体、BridgePolicy、模板引用/覆盖、共享来源） |
| `embed_catalog` | `embed:read` | 可安装的 builtin/platform 模板（公开视图，管理员也不返回引用计数）、Channel Embed capabilities、当前 `embedCodeMaxKB`、事件 topic/payload 限制和 Theater 绑定约束 |
| `embed_save` | `embed:write` | 无 `resourceId` 创建，有则只更新提交字段 |
| `embed_delete` | `embed:write` | 永久删除当前频道拥有的 iForm 与其 Storage，并移除世界共享绑定 |

不开放平台模板 CRUD、world-share、migrate、push。

### 复用的原生业务链

`embed_save/embed_delete` 构造与 REST `POST/PATCH/DELETE /channels/:channelId/iforms` 等价的 JSON body，调用从 REST handler 中提取的同一组函数 `createChannelIForm / updateChannelIForm / deleteChannelIForm`（`api/channel_iform.go`），REST handler 也改为调用它们：

- 名称 1–64 字、URL 仅 http/https、`embedCode` 不超过平台 `channelEmbedTools.maxCodeSizeKB`（默认 128 KB）；
- `templateRef` 创建时不能带 url/embedCode；模板引用不能修改 url/embedCode/templateRef，只能写 `templateOverrides`，其键白名单与“键为 `null` 恢复模板默认”的稀疏语义不变；
- BridgePolicy 去空白、去重；省略时 Embed API 关闭；
- 更新通过 `resolveEffectiveIFormForMutation` 找到世界共享源表单并写回源频道；删除只允许拥有该表单的频道；
- 成功后沿用 `channel-iform-updated` 快照广播。

模板目录与 REST `GET /channel-embed-tools/catalog` 共用 `listChannelIFormTemplateCatalog`。

### Theater 引用

`embed_read`/`embed_save` 的每个结果带 `theater`：

```json
{"worldId": "W", "channelId": "C", "formId": "F",
 "url": "https://chat.example/sub/#/internal/iform/F?channel=C&world=W",
 "alternateUrls": ["https://chat-tunnel.example/sub/#/internal/iform/F?channel=C&world=W"],
 "internalPath": "#/internal/iform/F?channel=C&world=W"}
```

`url` 使用 `config.domain` 的第一个公开域名与 `webUrl`（未配置域名时回退到当前 MCP 请求 origin），可直接作为 `theater_object content.iframe.url` 或 `theater_scene surface_embed_set url`，并通过第一期 `mcpTheaterValidateIframe` 校验。使用时 Theater scope 的 `inputChannelId`（频道 Theater 还有 `channelId`）必须等于 `theater.channelId`；对世界共享引用，`channelId` 是读取它的频道。浏览器会识别当前页面 origin 以及 `config.domain` 中所有已配置公开域名，并继续要求匹配 `webUrl` base path；`alternateUrls` 用于让调用方在多域名部署中选择合适的公开入口。

### 嵌入事件绑定

只有普通 iframe StageObject 支持；background/foreground `surfaceEmbeds` 继续可加载 iForm、交互和截图，但不挂动作。

持久化在对象 `metadata.embedEventBindings`：

```json
{"embedEventBindings": [{"topic": "door.open", "actionIds": ["open-door", "show-hint"]}]}
```

- topic 与 Channel Embed 相同：`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`，精确匹配，同一对象内不可重复；最多 16 个绑定，每个绑定 1–16 个不重复 actionId；
- actionId 只能引用同一对象已保存的 `actions`；服务端在 create/update 的最终对象上校验，删除动作而不同步删除绑定会被拒绝（编辑器删除动作时在同一次编辑中裁剪绑定）；复制/场景复制会把绑定重映射到新动作 ID；
- 一个 topic 可绑定多个动作：执行顺序取对象 `actions` 的保存顺序，并行/顺序沿用 `metadata.actionExecutionMode`，时延沿用各 action 的 `schedule`，`action.sequence` 按原组合执行；
- 绑定是配置数据；事件是瞬时的；事件 payload 不参与动作寻址，也不能指定 actionId/sequenceId/objectId，没有 JSONPath、条件或脚本匹配；
- MCP 用 `theater_object` 的 `fields.embedEventBindings` 写入，服务端在 `expectedRevision` 读取对象 metadata 后只替换这一键，保留 `actionExecutionMode`、mediaFx 等其他键；revision 过期直接冲突，不 rebase。

动作能力与点击语义拆分：drawing/text/image/button 保持原点击动作语义；iframe 可以保存动作，但只有被绑定的动作可执行（服务端 `TriggerTheaterAction`/batch、浏览器 HostBridge 与 MCP `trigger_action` 一致校验），iframe 不会成为舞台点击、序列“点击组件”触发或 `object.trigger` 目标。执行仍要求对象 `interactive` 与 `stage.action.trigger` 权限。

### 数据流

1. 嵌入代码调用 `SealChatEmbed.events.publish(topic, payload)`；Host 照旧检查 BridgePolicy capability、频道成员/observer、contextVersion、topic/payload 大小，服务端 `iform.event.publish` 照旧检查频道可读、发言权限、只读、BridgePolicy 和每用户 60 次/秒限流。
2. Theater iframe 宿主额外传入 `theaterContext: {worldId, scopeType, channelId, objectId}`（世界级 channelId 为空）；普通频道、浮窗和 surface embed 不传。服务端重新验证已保存的对象与绑定，成功广播后才创建该 object 专属 receipt，并返回 `eventId`。照旧向订阅连接广播 `channel-iform-embed`（只服务 `events.subscribe`）。
3. 仅当这个 `ChannelEmbedHost` 由普通 Theater iframe 对象创建（传入可选 `theaterEventSink` 与上述上下文）时，发起请求的 Host 在 publish 成功、会话上下文仍有效后，把 `{eventId, formId, channelId, topic}` 交回自己的宿主；同一 eventId 只报告一次。被拒绝、超时、上下文变化的 publish 不报告；Gateway 广播不进入此路径，因此其他客户端、同一 form 的其他实例都不会触发。
4. `StageIframeFrame` 只转发当前渲染 form 的事件，`StageIframeVisualObject` 附上自己的 objectId，经 `StageTextOverlay` 上抛到 `StageApp`。
5. `StageApp` 按 eventId 去重，检查 `stage.action.trigger`、对象可见/可交互、未开启“禁用网页交互”，按该对象绑定解析动作，以现有 `actionTriggered` 交给 `TheaterHostBridge`。
6. HostBridge 校验对象动作与保存值一致且被绑定，然后走原有 `triggerStageAction` → `POST .../actions/trigger(-batch)` → `TriggerTheaterAction`，场景切换、显隐、特效、聊天、线索和组合动作继续使用原权限、revision、广播、幂等与副作用；其他客户端通过原生 mutation/effect/chat/clue 同步得到结果。

Theater 登录态、MCP Key、renderer 权限不会进入嵌入 iframe；嵌入代码仍只看到 BridgePolicy 授予且 Host 判定有效的 capability。

### 最小 AI 流程

1. `embed_catalog` 查看 `embedCodeMaxKB` 与 capabilities。
2. `embed_save` 创建嵌入：

```json
{"worldId": "W", "channelId": "C", "name": "开门按钮",
 "embedCode": "<button id=go>开门</button><script>const c=window.__SEALCHAT_EMBED_CONFIG__||{};const t=document.createElement('script');t.src=c.sdkUrl;t.onload=()=>SealChatEmbed.connect({targetOrigin:c.hostOrigin}).then(s=>{go.onclick=()=>s.events.publish('door.open',{by:'button'})});document.head.append(t)</script>",
 "bridgePolicy": {"enabled": true, "allowedOrigins": [], "capabilities": ["events.publish"]}}
```

启用 Embed API 后宿主注入 `__SEALCHAT_EMBED_CONFIG__`，`sdkUrl` 已包含部署 webUrl。返回 `item.theater.url`。
3. `theater_object create`（`kind: "iframe"`、`interactive: true`），`content.iframe.url` 用上一步 URL，同时保存动作与绑定：

```json
{"worldId": "W", "scopeType": "world", "inputChannelId": "C", "operation": "create",
 "expectedRevision": 7, "mutationId": "door-frame", "sceneId": "S", "objectId": "door-ui", "kind": "iframe",
 "fields": {"x": 0, "y": 0, "width": 8, "height": 4, "interactive": true,
  "content": {"iframe": {"url": "https://chat.example/#/internal/iform/F?channel=C&world=W", "scale": 1}},
  "actions": [{"id": "open-door", "type": "object.toggle", "payload": {"objectId": "door"}}],
  "embedEventBindings": [{"topic": "door.open", "actionIds": ["open-door"]}]}}
```

4. 用户在已授权“AI 协作”的浏览器中点击嵌入按钮（或嵌入代码自身调用 `events.publish`）；正常 Host 流程只由发起 publish 的该 iframe 实例上抛并触发一次。
5. `theater_read events/object` 确认 revision 与显隐变化，`theater_capture` 截图验证。也可以用 `theater_control trigger_action`（`objectId=door-ui, actionId=open-door`）单独验证已绑定动作，这不经过嵌入事件。

### 验证与限制

Go：`go test ./service -run 'TestTheaterEmbedEvent|TestTheaterMCPObjectMetadata|TestMCPTheater'`、`go test ./utils -run TestMCP`，API 包的 `TestMCPEmbed*`（`api` 测试包现有部分文件无法编译，需排除后运行）。前端：`node scripts/theater-embed-event-bindings.spec.cjs`，覆盖 publish 成功一次、拒绝/上下文变化不触发、Gateway 广播不触发、同 form 多实例不串线、payload 无法选择动作、删除/复制动作时绑定裁剪与重映射、HostBridge 与点击语义。

成功的 Theater iframe `events.publish` 才生成 5 分钟 execution receipt；每个执行单元成功后续期 5 分钟，足以跨越合法 sequence delay，并允许较长的多动作执行继续推进。仅 Theater iframe 宿主链传入 `theaterContext` 定位已有对象；服务端重新校验 actor 的 `stage.action.trigger`、room/object、iframe 可交互、保存的内部 iForm URL（`config.domain` 的 `;` 多域名及 `webUrl` base path）、world/channel/form 与精确 topic 绑定。receipt 从创建起绑定 actor/world/channel/form/topic/room/object；同 form 的另一个 iframe 不能使用它，事件 payload 不能指定动作、对象或序列。

一个 event execution 可以完成该 topic binding 的多个 saved actions，包括 sequence 的多个 step、clue 的多个 entry，以及多动作 batch。服务端根据 actionId/stepId/entryId 生成 useKey（不包含 actionRequestId）；每个执行单元只能成功一次，inflight reservation 阻止并发重复执行，batch 原子预留每个 actionId。业务动作成功后才 commit；revision conflict、validation error 或执行失败会释放 reservation，同一 event/action/step/entry 可以正常重试。原有 mutation/chat/clue/effect 权限链继续生效。

普通 Gateway 广播和非 Theater embed publish 不生成 Theater execution receipt。MCP explicit `theater_control trigger_action` 是独立控制路径，不要求 receipt，仍只允许显式执行 iframe 绑定引用的 saved action；iframe 不能作为普通 click、`object.trigger` 或场景 sequence 的点击 target。没有可由 JSON 传入的 trusted/source 授权标志。

receipt 是有容量限制、成功执行后滑动续期、惰性 TTL 清理的单实例进程内状态，不做分布式持久化；容量压力下旧 receipt 仍可能被提前淘汰。receipt 绑定 actor/scope/object，但不额外绑定 renderer/browser session；正常 Host 不会向其他实例广播 eventId。嵌入代码若自动循环 publish，会在每个打开该舞台并运行它的客户端各自触发（受 publish 限流）。`surfaceEmbeds` 事件联动、条件匹配、多域名下自动选择用户访问域名均留给后续阶段。真实浏览器双客户端同步与截图仍需人工验收。

## 第三期：Design Plan / AI 布景闭环

Theater 固定为 **8 个顶层工具**：`theater_read`、`theater_catalog`、`theater_design`、`theater_scene`、`theater_object`、`theater_control`、`theater_view`、`theater_capture`。Tool Search 下，完整设计、搭建、布置或重构小剧场优先发现 `theater_design`；单对象或小批对象修改使用 `theater_object`，单场景修改使用 `theater_scene`。catalog 将对象能力、特效、叠层、图片/video、音频及限制保留为 operation，不拆新的 audio/effects/resources 顶层工具。

推荐流程：

```text
theater_read
→ theater_catalog
→ identity_read / embed_read / embed_save（必要时）
→ theater_design validate
→ theater_design apply
→ theater_view fit_scene
→ theater_capture（模型查看真实截图）
→ theater_object / theater_scene 小修，或下一次 theater_design
→ theater_control 验证互动
```

角色仍通过 `identity_read/identity_save` 管理；网页组件通过 `embed_read/embed_catalog/embed_save/embed_delete` 管理。Design Plan 只引用已有、可访问的资源和 iForm URL，不接收 HTML/JS、上传文件或修改 embedCode。`theater:write` 不包含 `embed:write`、`theater:control`、`theater:capture`，不增加 `theater:design` scope。Plan 要求完整 `stage.object.edit`，不接受 delegated edit；角色绑定另检查原生角色权限和身份归属。Plan 拒绝锁定场景/对象及锁定父组，不通过管理员身份自动解锁。

### Plan 格式与原子提交

顶层仅为 TheaterScope、`operation: validate | apply`、必填 `expectedRevision`、apply 必填 `mutationId`、`steps[]`。create 必须明确提供稳定 `sceneId/objectId`，后续步骤直接引用这些 ID。没有动态 alias 或表达式语言。

| step kind | 类型化参数 |
| --- | --- |
| scene.create / scene.update | sceneId、fields（与单场景 create/update 相同） |
| scene.delete | sceneId、可选 fallbackSceneId |
| scene.reorder / scene.folders | sceneIds / folders |
| scene.field | sceneId、fields（下述 8 个基础设计字段） |
| scene.surface | sceneId、target、image 或 clear、可选 style |
| scene.surface_embed | sceneId、target、url、可选 scale/interactive，或 clear |
| scene.overlay | sceneId、overlays |
| scene.music | sceneId、music/switchAudio，或 clear |
| scene.transition / scene.sequence | sceneId、transition / sequences |
| object.create | objectId、可选 sceneId、objectType、fields |
| object.update / object.delete | objectId、fields / 可选 cascade |
| character.bind | object.create 参数，objectType=character、identityId、ownerUserId |
| layout.align / layout.distribute | objectIds、mode |
| layout.grid | objectIds、columns、gapX、gapY、可选 originX/originY |

scene 的 target 为 background/foreground。content/actions/embedEventBindings 的替换语义沿用单对象工具。scene.surface_embed 可以配置 Plan 内的新场景或其他场景；单场景工具的 surface_embed_set/clear 仍仅支持当前场景，且 surfaceEmbeds 不获得 StageAction。

```json
{
  "worldId": "WORLD", "scopeType": "world",
  "operation": "validate", "expectedRevision": 7,
  "steps": [
    {"kind": "scene.create", "sceneId": "forest", "fields": {"name": "森林"}},
    {"kind": "scene.field", "sceneId": "forest", "fields": {"fieldWidth": 48, "fieldHeight": 28, "backgroundColor": "#101d18", "displayGrid": false}},
    {"kind": "object.create", "objectId": "sign-a", "sceneId": "forest", "objectType": "text", "fields": {"width": 6, "height": 2, "content": {"text": "入口"}}},
    {"kind": "object.create", "objectId": "sign-b", "sceneId": "forest", "objectType": "text", "fields": {"width": 6, "height": 2, "content": {"text": "深处"}}},
    {"kind": "layout.grid", "objectIds": ["sign-a", "sign-b"], "columns": 2, "gapX": 2, "gapY": 1, "originX": -8, "originY": 0}
  ]
}
```

确认摘要后，使用完全相同的 steps 和 expectedRevision，改为 `operation=apply` 并增加新的 mutationId。一个 Plan 最多 64 steps，整个 native payload 不超过 **128 KiB**；不因 MCP HTTP 上限更大而放宽。`theater_catalog limits` 返回 designPlanSteps/designPayloadBytes/layoutModes/duplicateLimits。大型舞台按 scene 拆多个 Plan。

validate 与 apply 共用严格 typed normalization、原生 schema 和内存 document planner，包括权限、锁、层级、跨引用、资源、音频与 iForm 校验。validate 只读取预期 revision 的副本，返回 valid/revision/stepCount、scene/object 创建/修改/删除 ID、layout 摘要及 warnings；不创建房间、mutation、audit、资源引用或 Gateway 事件。错误附带 stepIndex（从 0 开始）、stepKind 和 nativeCode/nativeMessage。**validate 不是锁**。

apply 在原生 mutation 的一个数据库事务中重新规划并检查 expectedRevision CAS，再执行规划出的窄原生设计操作；任何失败全部 rollback。无论 steps 数量，只产生 `N → N+1`，资源引用在最终状态刷新一次，checksum 计算一次，记录一条 mutation/audit。返回 mutationId/revisionBefore/revision/checksum、affected、sceneIds/objectIds、warnings。Gateway 和 event history 只发送 bounded summary，客户端按更高 revision 走现有 reload/snapshot 收敛。

成功的同凭证、同 mutationId、同 normalized Plan 重试返回首次结果；JSON object key 顺序不影响 fingerprint，不同 Plan 或不同 revision 复用 ID 返回 conflict/reused。stale revision 无设计写入，不自动 rebase。revision 冲突后重新读取并使用新 mutationId。

### 确定性布局与单场景字段

layout 是确定性几何原语，不做 AI 自动审美或 constraint solver。align 支持 left/center_x/right/top/center_y/bottom，按参与对象总体 AABB 边缘或中心对齐。width/height、scaleX/scaleY（兼容 scale）和 rotation 用简单三角计算形成 transformed AABB。distribute 支持 horizontal/vertical，保持首尾位置并均分边缘间距；按当前中心 x/y 排序，相同中心以 objectId 打破平局。重叠范围可产生负间距。

grid 使用传入 objectIds 顺序，列宽/行高取该列/行最大 AABB 尺寸，对象在单元格居中；origin 是布局区域左上角，省略时使用参与对象当前 AABB 的左上角。columns 为正且不超过对象数，gaps 为非负有限数。align/grid 接受 1..200 个对象，distribute 至少两个对象。

参与对象必须同 scene/persistent scope、同 parentId，使用该父组的局部坐标；只产出 x/y transform 更新，不 reparent 或更改 scope。锁定对象/父组/场景、missing 对象、effect 均拒绝。effect 的 1920×1080 坐标不进入普通 layout，没有持久化 layout schema。

`theater_scene field_update` 的顶层类型化参数：backgroundColor、fieldWidth、fieldHeight、fieldObjectFit（cover/contain/fill）、displayGrid、gridOnTop、gridSize、alignWithGrid。Plan 的 scene.field 将这些字段放在 fields 中。尺寸范围为 1..1000000，gridSize 为 0.01..1000000。它们合并到现有 scene state/grid，不接收任意 state JSON，也不丢失其他 surface/overlay/music/transition/sequence 配置。

### 复制和音频发现

`theater_scene duplicate(sceneId)` 复制完整 scene 与 scene local objects、层级和 state；返回新 sceneId、objectIdMap、actionIdMap。`theater_object duplicate(objectId, offsetX?, offsetY?)` 复制一个 root 与 child subtree，保持原 scene/persistent scope 和 root 的外部 parent，仅 root 应用 offset，返回新 objectId 与同样的映射。每次最多一个 scene/root，subtree 最多 2000 个对象，并继续受房间/场景总量限制。

复制重新生成 scene/object/action、sequence/step/trigger ID，按原生结构重映射 parentId、scene.apply、object.toggle/object.trigger、effect.play、nested action.sequence、scene theaterSequences 和 iframe embedEventBindings.actionIds。场景副本沿用源对象的 transitionKey（缺省为源 objectId），支持跨场景转场对应；同场景对象副本使用新 objectId 作为 transitionKey。新场景默认未发布。subtree 外部合法引用保留，叠层 registry 的 effectId 不当作 Theater 对象 ID。resourceId、image/video/audio 文件、角色 identity 与 iForm URL 继续引用原数据，不物理复制。duplicate 不进入 Design Plan，不增加动态 ID alias 系统。

`theater_catalog audio_resources` 复用现有 Theater 音频的权限与 feature/channel tag 语义，但会把 Theater room scope 与素材筛选频道分开处理：channel Theater 使用自身 channelId；world Theater 的 room channelId 仍为空，素材发现使用 inputChannelId，缺失时明确报错。返回仅投影 id/name/duration/transcodeStatus/ready 并分页；普通 audio_assets 继续排除 TheaterFeatureAudioTag。world Theater 已进入共享舞台状态的同 world、ready 音频引用不会因后续从其他 inputChannelId 编辑而失效；channel Theater 仍要求匹配当前频道 tag。音频不增加顶层工具或上传 MCP。

renderer/capture 与设计持久化解耦：apply 不自动截图、不等待 renderer；没有协作浏览器也能保存设计。成功设计是正常 revision，用户需要调整时提交修正 Plan，或先 duplicate scene 保存副本。admin snapshot restore/replace/package import 不属于正常设计能力，也不由 theater:write 放行。
