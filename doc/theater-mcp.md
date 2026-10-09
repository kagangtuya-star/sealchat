# Theater MCP 第一期

复用现有场景、普通对象、StageAction、序列器、频道 iForm 和 `surfaceEmbeds`。不增加 widget、代码执行环境、嵌入编辑器、代码 CRUD、通用 Bridge 注入、Playwright 或布局事务。全部 durable 写入经 `ApplyTheaterMutation`，沿用权限、schema、CAS、checksum、审计和房间事件。

## 作用域和坐标

所有工具均传 `worldId`、`scopeType: world | channel`。主小剧场为 `scopeType=world, channelId=""`（可省略 channelId）；频道 Theater 必须传该世界内的 channelId。`inputChannelId` 只表示可读的聊天上下文，绝不替代 Theater 房间 channelId。响应 `scope` / 任务 `scope` 给出实际房间。

普通对象的 x/y 是中心锚点世界坐标，`WORLD_UNIT_PX=24`；子对象还受父组变换影响。相机 x/y 是视口平移像素，zoom 为缩放。特效使用独立 1920×1080 设计坐标，不可直接把截图像素当对象或特效坐标。

`theater_read(summary)` 返回投影后的 revision/checksum、当前场景、场景目录、指定或当前场景加常驻对象摘要、权限和同账号 renderer。目录与对象分别分页；`page/limit` 默认 1/50，最多 200。`scene` 返回一个场景的完整 state 和分页对象，`object` 返回一个完整对象，`events` 按 afterRevision 读取原生权限过滤事件，`renderers` 读取协作端。隐藏对象、隐藏父组与未发布场景沿用快照权限投影，不由 MCP 放行。

`theater_catalog` 的 `object_types/effects/overlays/resources/limits` 复用原生类型/特效 allowlist、场景叠层 preset 服务与房间资源。浏览器注册时提供现有叠层注册表及特效默认配置；未授权在线 renderer 时，这部分 runtimeCatalog 为空。房间 resources 不是公共素材全库搜索。

## 工具操作

| 工具 | operation |
| --- | --- |
| `theater_read` | summary、scene、object、events、renderers |
| `theater_catalog` | object_types、effects、overlays、resources、limits |
| `theater_scene` | create、update、reorder、delete、apply、folders_update、surface_update、surface_embed_set、surface_embed_clear、overlay_update、music_update、transition_update、sequence_update |
| `theater_object` | create、update、batch_update、delete、toggle、bind_character |
| `theater_control` | apply_scene、trigger_action、trigger_sequence、execution_status、cancel_execution |
| `theater_view` | get、camera_set、fit_scene、focus_object、select_objects、clear_selection |
| `theater_capture` | capture、status、cancel |

各工具 JSON Schema 列出类型化字段及 operation 枚举。禁止任意 mutation type/payload、管理员恢复及 JavaScript。对象类型维持 `group/drawing/text/image/button/character/video/effect/iframe`，原生 schema 是最终限制；未知类型/字段拒绝。

场景 create 用 `sceneId, fields.name`；update 仅接受 name/switchText/order/folderId/locked/published。场景局部配置分别使用对应 operation，不能提交整份 state：surface_update 修改单个前景/背景图片、样式及场地尺寸；overlay_update/music_update/transition_update/sequence_update 分别接受 overlays/music（及 switchAudio）/transition/sequences。music_update 的 clear 清除音乐和切换音效。

对象 create 用 `objectId, sceneId, kind, fields`（sceneId 省略/null 为常驻）；update 用 objectId/fields。fields 支持名称、父组、位置、大小、旋转、缩放、z/orderKey、显隐、锁定、交互、可编辑、content/actions。content/actions 遵循原生整体替换语义，修改前先读取完整字段。`batch_update` 用 `updates[{objectId,fields}]` 复用原子 `object.batchUpdate`，只能更新已有对象；不支持混合创建、更新、删除事务。toggle 要求显式 visible 目标，重试不反向翻转。bind_character 用 identityId/ownerUserId，世界 Theater 的角色频道由 inputChannelId 指定；原生 character.bind 新增可选 inputChannelId，省略时保持原频道房间语义。

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

平台 Theater 模块默认 off；read/write 不自动包含额外控制、截图或聊天。Key/OAuth 需分别明确授权 `theater:read/write/control/capture/chat`，并受实时业务权限限制。`theater:write` 不授予频道嵌入编辑权限。scene apply 另检查原生场景切换权限；read 无法切换场景。

control 只执行已保存的动作（objectId，可选 actionId）或当前场景序列（sequenceId），必须传当前 sceneId、expectedRevision、rendererId。服务端展开最多 256 节点、8 层；原生动作和序列 timing/schedule 调度在指定浏览器执行。每个业务 leaf 经服务器以发起 MCP Actor 和当前有效凭证重新鉴权，浏览器不能供应动作 payload 或借用自己的登录权限。chat.send/insert/random-table 另需 theater:chat；clue.execute 另需 clue:write/publish 和线索权限；带音频的 effect.play 另需 audio:write 及共享音频业务权限。

`object.trigger` 属于现有场景序列器步骤（触发目标对象已保存动作），不是普通对象可保存的原子点击动作；catalog 分别列出 actions 和 sceneSequenceActions，不扩展原生动作 schema。

返回任务 `requestId`（作为 executionId/captureId）、rendererId、scope、sceneId、revision 和 pending/running/stopping/completed/failed/cancelled。查询/取消须保留同一 scope，包括 inputChannelId。服务端 descriptor 不是完成状态；只有浏览器回应完成且各 leaf 已确认才 completed。视觉 effect 仅发给指定 renderer，不广播重复播放。MCP scene.apply 只提交原生切换，不调用手动切场景附带的聊天/共享音频副作用。取消停止后续步骤，不能回滚已发生的业务副作用；超时、断线、revision/场景变化、未提交编辑均拒绝或失败。

若取消、超时或断线发生在服务端步骤执行期间，任务先进入 `stopping`，待该步骤返回后再变为终态，期间拒绝新步骤。`inFlightStepId` 表示正在运行的步骤，`completedStepIds` 记录已确认的服务端步骤，`partialSideEffectsPossible` 表示失败/取消后可能仍有部分业务效果（即使服务端操作曾返回错误也不能保证零副作用）。视觉渲染或本地动作失败不清除已确认步骤；不能直接重放整个 execution，应先检查当前房间状态，再明确决定后续动作。

## 浏览器协作和本地视图

用户在小剧场点击顶部闪电图标打开“聊天桥接”弹层，在“舞台同步”下方手动开启“AI 协作”开关并确认，随后浏览器会请求共享当前标签页以提供完整截图；应选择当前 SealChat 标签页。完成后才登记本页面 renderer；关闭开关、停止共享、关闭页面、断线或切世界/聊天上下文都会撤销授权并停止捕获流。默认没有授权，普通玩家不会自动登记。MCP Actor 必须与浏览器同账号、同 Theater scope 和聊天上下文，每次只向一个 renderer 定向分发；没有授权端返回 renderer_unavailable。

登记包含 userId/worldId/scopeType/channelId/inputChannelId、activeSceneId、revision、viewport/camera、capabilities 和注册表。heartbeat 15 秒、登记 TTL 60 秒，过期懒清理；运行请求另有有界超时。仅支持当前单实例进程内 broker，无 Redis、无跨实例路由；服务重启丢失协作登记及截图。任务/结果保留约 2 分钟（懒清理），每 renderer 同时 1 个、每用户 2 个、全局 8 个任务，最多保留 128 个任务/renderer 登记。

view get 返回实际相机/视口/选择（任务 result）及场景/revision；camera_set 的 camera 为 x/y/zoom，fit_scene 按场地，focus_object 接一个 objectIds，select_objects 最多 200，clear_selection 清除选择。这些只修改现有 Store 的本地相机/选择，不发 Theater durable mutation，不影响其他客户端。非 get 操作额外需要 theater:control。view 超时 8 秒。

## 截图

capture 请求传 rendererId、当前 sceneId、expectedRevision、mode(viewport/object)、可选 objectId、maxEdge(默认1600，64..1600)、maxBytes(默认2097152，1024..2097152)。object 模式裁剪该对象在当前视口内的边界，不自动改相机；完全在视口外时失败。字体、图片、可见视频 readiness、入场/场景媒体/切换状态采用事件或帧检查与有界超时，不用固定 sleep。renderer 不提交或覆盖用户的未提交编辑；截图期间相机、视口、scene/revision 或草稿变化拒绝。

截图优先使用浏览器共享的当前标签页合成画面：隐藏舞台编辑辅助层后等待下一帧，按 Stage 根节点在浏览器视口中的实际位置裁剪，因此 Konva、DOM、内部频道 iForm、surfaceEmbeds、普通跨域 iframe 和视频都按浏览器最终合成像素进入结果；object 模式继续在此基础上二次裁剪对象区域。该路径返回 includedLayers=[browser-composite]，正常情况下为 complete。

若浏览器不支持标签页捕获、共享流失效或画面尺寸无法可靠映射，则回退原有 DOM 合成：Konva canvas 使用原生导出，DOM 克隆通过成对锚点维持层序，iframe/iForm 仍使用灰色占位并返回 partial。回退路径不放宽 iframe sandbox，也不尝试跨域读取。complete 表示浏览器合成路径成功或回退路径未检测到缺失；真实像素仍需浏览器验收。

capture 启动返回 captureId/task；用 status 查询。完成的 status 返回真正 MCP ImageContent（PNG/JPEG）和 structuredContent/TextContent 元数据：captureId、rendererId、sceneId、revision、status(complete/partial)、width/height/mimeType、coordinateMapping、includedLayers、missingLayers、unsupportedObjects、warnings。图片只在进程内短期保存，不上传图库/CDN，仅发起用户与同一 Key/OAuth 凭证可读取/取消，capture 工具不能被 control status 绕过。

coordinateMapping 中 `worldOriginPx` 表示世界原点在输出图上的像素，`pixelsPerWorldUnit` 为普通世界单位对应像素：`worldX=(imageX-worldOriginPx.x)/pixelsPerWorldUnit`，Y 同理。另含 camera、crop、worldUnitPx=24、anchor=center 和 effectDesignSize；这是舞台世界坐标，不是父组局部坐标或特效设计坐标。截图任务超时 15 秒；取消/超时不保证底层不可取消 DOM 克隆立即结束，但其响应不再被接受，临时标记会在 finally 恢复。

## 验证边界

新增 Go 用例覆盖 scope/projection、state merge/CAS/幂等、对象 iframe 与 surfaceEmbeds、内部链接、scopes、renderer 生命周期和任务/图片结果；前端脚本覆盖定向消息、dirty/replay/断线、序列取消、真实克隆回调语义和 partial 坐标映射。运行 `node scripts/theater-mcp-renderer.spec.cjs` 以及现有 `node scripts/run-theater-dialogue-tests.cjs`。

单元测试不等同真实浏览器合成验证。真实双客户端同步、混合图层光栅、iForm 交互、字体/媒体以及普通用户完整手动流程仍需浏览器验收；不声称这些已经验证。
