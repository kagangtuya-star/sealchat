# 聊天装扮（Chat Skin）数据与渲染约定

聊天装扮是挂在**频道角色**上的消息外观：气泡 / 紧凑行的底色、渐变、底图、描边、圆角、阴影、内外边距、尺寸、名字与正文样式，以及最多 8 个图片装饰图层（猫耳、蝴蝶结、角花等）。它与头像装饰、小剧场演出是三套独立能力，互不覆盖。

本文面向维护者与主题 / UI 自定义包作者，说明数据结构、继承关系、快照规则、资源引用、DOM 约定和各输出端的接入状态。

## 1. 数据结构

唯一权威定义：

- 前端：`ui/src/features/chat-skin/chatSkinSchema.ts`（`normalizeChatSkin` 同时是安全边界）
- 后端：`protocol/chat_skin.go`（`SanitizeChatSkin` / `SanitizeChatSkinPatch`，保存时强制执行）

```jsonc
{
  "version": 1,
  "scope": "ic",                 // "ic"：仅场内消息；"all"：场内外都显示
  "bubble": {                    // 气泡布局表面，null / 缺省 = 该布局保持主题原样
    "enabled": true,
    "style": {                   // 任何缺省的分组 = 继承当前主题
      "background": { "mode": "color|gradient|none", "color": "#rrggbb[aa]", "color2": "…", "angle": 135,
                      "imageRef": "id:<附件ID>", "imageFit": "cover|contain|tile|stretch", "imagePosition": "center|top|…" },
      "border": { "width": 0-8, "style": "solid|dashed|dotted|double", "color": "…" },
      "radius": { "tl": 0-48, "tr": …, "br": …, "bl": … },
      "shadow": { "mode": "none|custom", "color": "…", "blur": 0-48, "x": -24-24, "y": …, "spread": -12-12 },
      "padding": { "x": 0-48, "y": 0-40 },
      "size": { "minWidth": 0-480, "minHeight": 0-240, "maxWidth": 0-960 },   // 0 = 不限制 / 沿用主题
      "spacing": { "top": 0-64, "bottom": 0-64 },
      "accent": { "color": "…", "width": 1-12, "side": "left|right|top|bottom" },
      "blur": 0-20,
      "text": { "color": "…", "font": "sans|serif|kai|mono|rounded", "scale": 0.8-1.4, "shadow": "…" },
      "name": { "color": "…", "font": "…", "weight": "normal|bold|black", "italic": true, "chip": "…", "shadow": "…" }
    },
    "night": { /* 与 style 同结构；按分组整体覆盖 style，仅夜间主题生效 */ },
    "layers": [
      { "id": "ear", "label": "", "enabled": true, "ref": "builtin:cat-ear | id:<附件ID>", "tint": "#…",
        "anchor": "top-left|top|top-right|left|center|right|bottom-left|bottom|bottom-right",
        "x": -240-240, "y": -240-240, "size": 8-480, "aspect": 0.05-20, "rotate": -180-180, "opacity": 0-1,
        "z": "front|back", "flipX": false, "flipY": false, "mirror": true, "blend": "normal|multiply|screen|overlay|soft-light" }
    ],
    "mirrorOnSelf": true,        // 自己发言（气泡在右侧）时镜像左右方向
    "reserveSpace": true,        // 伸出上下边缘的图层自动预留外边距
    "advanced": { "vars": { "--chat-…": "…" }, "surfaceCss": "…", "textCss": "…", "nameCss": "…" }
  },
  "compact": { /* 紧凑布局表面，结构同 bubble，与气泡互不回退 */ },
  "meta": { "name": "", "presetId": "builtin:paper" }
}
```

要点：

- **未配置即继承主题**。不存在固定的白底默认值；没有任何表面配置时整个装扮为 `null`。布局或图层的 `enabled=false` 仅影响显示，配置仍完整保存；清空操作才删除配置。
- 气泡与紧凑是两份独立设计：气泡的外形不会被套到紧凑行上，反之亦然。编辑器提供「从气泡设计派生」作为起点。
- 限制：图层 ≤ 8，主题变量 ≤ 24，高级 CSS 每项 ≤ 600 字符，序列化后 ≤ 16 KB（`ErrChatSkinTooLarge`，HTTP 413）。
- 图层保持 `aspect=height/width` 的表达方式，联合缩小 `size`，使最终宽高各不超过 480px。

## 2. 存储与接口（无新增表）

| 位置 | 列 / 字段 | 内容 |
| --- | --- | --- |
| `channel_identities.chat_skin` | JSON | 角色基础装扮 |
| `channel_identity_variants.appearance_json` 的 `chatSkin` 键 | JSON | 差分覆写 `{ "mode": "merge|replace|none", "skin": {...} }` |
| `messages.sender_chat_skin` | JSON | 发送时按差分解析后的装扮快照 |
| 共享角色 `shared_data_json.chatSkin` | JSON | 跨频道角色模板；同步到每个副本的 `chat_skin` |

专用接口（通用角色 / 差分更新接口不会读写装扮，避免旧客户端误清空）：

- `PUT /api/v1/channel-identities/:id/chat-skin`，body `{ channelId, targetUserId?, chatSkin: ChatSkin | null }`
- `PUT /api/v1/channel-identity-variants/:id/chat-skin`，body `{ channelId, targetUserId?, chatSkin: { mode, skin } | null }`

保存时服务端会：白名单清洗；校验每个 `id:` 附件对操作者可访问且为 `image/*`；共享角色只允许所有者修改基础装扮；成功后广播角色刷新事件。

协议字段：`ChannelIdentity.chatSkin`、`MessageIdentity.chatSkin`（消息事件里的 `identity`）、消息模型 `sender_chat_skin`。字段为空时一律省略，旧客户端看到的载荷不变。

## 3. 继承与覆写

```
当前主题（含 UI 自定义包 / 自定义 CSS）
  └─ 角色基础装扮 chat_skin（只覆盖其中出现的分组）
       └─ 差分覆写 appearance.chatSkin
            merge   ：深合并（对象递归合并、数组整体替换、null 删除字段）
            replace ：整套替换
            none    ：此差分不显示装扮
            （缺省）：跟随基础装扮
       └─ 夜间覆写 surface.night（按分组覆盖 style，仅夜间主题）
  └─ 本地开关「显示聊天装扮」关闭时：只跳过渲染，不改数据
```

前后端合并语义一致：`deepMergeChatSkinDocument`（TS）与 `MergeChatSkinDocument`（Go）。差分自动切换（关键词 / 前缀匹配）选中的差分会按上面的规则解析后写入快照。

合并前在 ChatSkin 领域层规范化圆角：完整文档（含历史数据）的 scalar radius 展开为四角；partial object 只覆盖出现的角，scalar patch 覆盖全部四角，`null` 仍删除字段。

## 4. 历史一致性（快照规则）

消息只读取自身快照，**不回退到角色的实时配置**：修改或删除角色装扮不会改变任何历史消息。

| 路径 | 行为 |
| --- | --- |
| 发送（API / WebSocket / Webhook / 转发） | 解析「基础 + 差分」写入 `sender_chat_skin`，并设置 `SenderChatSkinResolved` 跳过模型层兜底 |
| 其他创建路径 | `MessageModel.BeforeCreate` 按身份与差分兜底解析（与小剧场演出快照同一次查询） |
| 乐观消息（前端 tmpMsg） | 用发送瞬间的解析结果填 `identity.chatSkin`，回执后被服务端快照替换 |
| 编辑消息 | 身份与差分都未改变时保留原快照；切换身份或差分时重新解析；切换为无身份时清除 |
| 自己的编辑预览 / 插话恢复 | 与上一行同一规则（`resolveIdentityPreviewInfo` 按 identityId + variantId 判断） |
| 他人的输入 / 编辑预览 | 服务端在 typing 事件的 `member.identity.chatSkin` 中下发；编辑时按同一规则返回原快照 |
| 历史加载 / 导出 | 读 `identity.chatSkin`（存在即优先，即使为 null）否则读 `sender_chat_skin` |

## 5. 资源引用

- `id:<附件ID>`：普通附件（上传走现有附件系统），快照与配置中只保存引用，不复制图片数据。
- `builtin:<名称>`：随前端代码分发的单色 SVG 纹样（`ui/src/features/chat-skin/ornaments/`），以遮罩方式渲染并按图层 `tint` 着色。
- 任何 URL、`data:` 地址都会在规范化时被丢弃。
- 频道复制会克隆装扮引用的附件并改写引用；角色导出 / 装扮文件导出把附件写成 `asset:<素材键>`（与角色导出的素材载荷同结构），导入时在目标频道重建附件后再改写回 `id:`。无法访问的附件对应图层会被丢弃，不会导致整个装扮失效。

## 6. 渲染与 DOM 约定

渲染模型：`buildChatSkinRender(skin, ctx)`（`chatSkinRender.ts`）把装扮转换为内联样式和图层描述，`chat-item.vue` 只做绑定，不存在第二套消息渲染系统，也不生成按角色 ID 的选择器。

稳定约定（主题与 UI 自定义包可以依赖）：

| 选择器 / 属性 | 含义 |
| --- | --- |
| `.chat-item[data-chat-skin="bubble|compact"]` | 当前消息应用了装扮 |
| `.chat-item[data-chat-skin-preset="…"]` | 来源预设（若有） |
| `.chat-item--chat-skin`、`.chat-item--chat-skin-bubble`、`.chat-item--chat-skin-compact` | 同上，按布局区分 |
| `.chat-item--chat-skin-layered` | 含装饰图层 |
| `.chat-item--chat-skin-surface` | 紧凑行带有底色 / 描边 / 色条（补最小内边距与圆角） |
| `.chat-skin-layers > .chat-skin-layer[data-layer-id]` | 装饰图层容器与图层；`--front` / `--back`，`--image` / `--mask` |
| `--sc-chat-skin-bg` / `-border` / `-accent` / `-text` / `-name` | 只在配置了对应分组时出现，可用 `var(--sc-chat-skin-bg, fallback)` 引用 |

样式落点：气泡外形写在 `.content`，紧凑外形写在 `.chat-item` 根节点，名字写在 `.name`，正文样式写在 `.content`。气泡内边距通过 `--chat-message-padding-x/y` 变量传递，编辑状态的操作槽位补白继续生效。

不阻断交互：图层容器与每个图层都是 `pointer-events: none`、不可选中、`aria-hidden`；前后景分别承载，前景 `z-index: 1`，消息操作栏、编辑操作、多选框等控件在 `z-index: 2`。预留空间基于最终尺寸、旋转后的包围盒、纵向锚点与 y 偏移；未知表面高度按 0 保守估算，上下各最多 96px。超出部分只在装饰容器裁切，正文不裁切；关闭 reserveSpace 时装饰不得越出表面上下边界。

主题兼容：没有 `!important`、没有全局选择器；所有值都是单条消息上的内联样式，未配置的分组完全由主题决定。

高级模式只接受**声明**（`属性: 值;`），属性在白名单内，禁止 `url()`、定位、变换、`@import`、花括号等；主题变量只能覆写 `--chat-*`、`--quote-*`、`--message-*`、`--sc-skin-*`，作用域是该条消息。不支持 HTML 或脚本。

Go、Vue 与静态 Viewer 使用同一声明过滤规则并去掉 `!important`。背景、渐变、阴影、outline、filter、字体等视觉能力保留；渲染时约束 min-width/min-height、padding、border 宽度及行高，避免极端长度传播到消息列表布局。编辑预览的 `chatSkin` 为 undefined 时沿用快照，为 null 时明确无装扮，为 object 时使用预览；紧凑外置名字与正文共用此规则。

## 7. 预设、导入导出

- 内置预设：`chatSkinPresets.ts` 的 `BUILTIN_CHAT_SKIN_PRESETS`，与用户装扮同结构。世界 / 平台预设以后只需提供同样的 `ChatSkinPreset` 条目（`source` 区分来源）即可接入编辑器。
- 「我的预设」保存在浏览器本地（`sealchat_chat_skin_presets_v1`），只存引用不存图片。
- 装扮文件：`sealchat.chat-skin/v1`，含 `skin`、可选 `patch` 与 `assets`；也接受直接粘贴的裸装扮 JSON。
- 频道角色导出（`IdentityExportItem.chatSkin`、`IdentityExportVariantItem.chatSkin`）随角色一起导出 / 导入装扮；旧版文件没有这些字段时导入不会改动目标角色已有的装扮。

## 8. 各输出端

| 输出端 | 状态 |
| --- | --- |
| 频道聊天（气泡 / 紧凑 / 紧凑网格名字列）、置顶区、分屏、嵌入页（`/embed`） | 复用 `chat-item.vue`，按快照渲染 |
| 输入中 / 编辑中预览 | 名字样式 + 气泡底面（不渲染图层，避免预览区跳动） |
| 小剧场 | 小剧场内的聊天面板复用频道聊天，按快照显示装扮；舞台对白继续使用独立的小剧场演出外观，不受装扮影响 |
| HTML 导出 / 查看器 | 导出载荷携带 `sender_chat_skin` 与 `sender_chat_skin_assets`（附件内联为 `scasset:` 资源），`service/embed/export_viewer.js` 按同一语义渲染，切换版式 / 日夜时重新应用 |
| 文本 / JSON 等其他导出格式 | 不携带装扮（纯文本格式不需要外观） |
| LOG | 不消费聊天装扮。LOG 是独立排版的出版物，有自己的按角色样式规则；把聊天外观自动套进 LOG 会破坏其独立设计。消息快照已持久化，将来如需「从聊天装扮生成 LOG 角色样式」可直接读取 `sender_chat_skin` |
| 归档、悄悄话 | 保持主题原样（悄悄话与归档有专门的视觉语义） |

## 9. 测试

- `go test ./protocol`：清洗、限制、合并语义、引用映射。
- `go test ./model`：快照持久化、身份修改后历史不变、旧消息形状不变、`MessageUpdate` 序列化。
- `go test ./service -run 'ChatSkin|CopyChannelIdentities'`：保存校验、差分覆写在通用更新后保留、频道复制改写附件。
- `go test ./api -run 'ChatSkin|TypingPreviewChatSkin'`：编辑时快照保留 / 切换差分重算、编辑预览规则（该包部分既有测试文件无法编译时用 `-overlay` 排除）。
- `node --test ui/src/features/chat-skin/chat-skin.test.mjs`：前端契约、渲染模型、导入导出、预设。
