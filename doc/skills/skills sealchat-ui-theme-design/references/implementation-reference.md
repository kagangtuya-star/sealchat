# 实现速查

本文件是原有开发指南的**索引和补充**，不是替代。接口细节以 `original/ui-customization-package/ui-customization-package-development-guide.md` 和 `ui/src/services/ui-customization/` 源码为准。下面的内容已在 2026-11-12 对照源码核实；SealChat 升级后先用文末命令复核。

## 1. 包格式要点

```json
{
  "format": "sealchat.ui-package",
  "formatVersion": 1,
  "runtimeApiVersion": 2,
  "manifest": { "name": "1～100 字", "description": "≤500 字" },
  "targets": ["global", "observer", "embed", "log-public"],
  "channelCapabilities": [],
  "css": "...",
  "js": ""
}
```

- 服务端校验（`service/ui_customization*.go`）：`formatVersion` 必须是 1；`runtimeApiVersion` 在 1～2 之间或省略；名称 1～100 个字符；说明 ≤500 个字符；`targets` 1～32 项且都是合法 Surface；CSS + JS 字节数默认 ≤1024 KiB（`uiCustomization.maxCodeSizeKB`，64～16384）。
- 多包可用 `sealchat.ui-bundle` 格式：`{ "format": "sealchat.ui-bundle", "formatVersion": 1, "packages": [ ... ] }`，每个包最多 50 个。
- 导入后是停用状态，需要手动启用。

## 2. Surface 与根属性

| Surface | 页面 | 被 `global` 覆盖 |
| --- | --- | --- |
| `auth` | 登录、注册、找回/重置密码 | 是 |
| `lobby` | 世界大厅、关于页 | 是 |
| `world-detail` | 世界详情、世界控制台 | 是 |
| `chat` | 根 `/` 与频道聊天 | 是 |
| `split` | 分屏 | 是 |
| `theater` | World Theater 小剧场 | 是 |
| `invite` | 邀请入口 | 是 |
| `log-layout` | LOG 排版编辑 | 是 |
| `status` | 状态页 | 是 |
| `observer` | OB 旁观（公共） | **否**，需显式写入 |
| `embed` | 频道嵌入（公共） | **否**，需显式写入 |
| `log-public` | LOG 公开页（公共） | **否**，需显式写入 |
| `internal` | 内部页面 | 永不加载 |

根节点属性：`data-sc-surface`、`data-sc-palette`（`day` / `night`）、`data-sc-layout`（如 `compact`）、`data-sc-observer`（`true`）、`data-sc-ui-customized`（有包 CSS 生效时为 `true`）。

`targets` 只选需要维护的页面。只做聊天皮肤时用 `["chat", "split"]` 甚至 `["chat"]`，不要为了“完整”覆盖所有页面。

## 3. 变量全集

令牌块里按组覆盖。**粗体**是 21 个传统颜色字段对应的变量，必须覆盖。

**页面与组件**
**`--sc-bg-surface`**、**`--sc-bg-elevated`**、**`--sc-bg-input`**、**`--sc-bg-header`**、`--sc-bg-page`、`--sc-bg-sidebar`、`--sc-bg-layer`、`--sc-bg-layer-strong`、`--sc-bg-hover`、`--sc-bg-active`

**文字与边框**
**`--sc-text-primary`**、**`--sc-text-secondary`**、`--sc-fg-primary`、`--sc-fg-muted`、**`--sc-border-mute`**、**`--sc-border-strong`**

**强调**
**`--primary-color`**、**`--primary-color-hover`**、**`--sc-action-ribbon-hover-text`**、`--sc-badge-bg`、`--sc-badge-text`、`--sc-sidebar-hover`、`--sc-sidebar-active`、`--sc-chip-bg`

**滚动条与字体**
`--sc-scrollbar-size`、`--sc-scrollbar-thumb`、`--sc-scrollbar-thumb-hover`、`--sc-font-family`、`--sc-code-font`

**浮层**（`.sc-overlay` 基线，参考主题已使用）
`--sc-overlay-background`、`--sc-overlay-border-color`、`--sc-overlay-radius`、`--sc-overlay-shadow`

**聊天：传统字段的 custom 变量**
**`--custom-chat-ic-bg`**、**`--custom-chat-ooc-bg`**、**`--custom-chat-stage-bg`**、**`--custom-chat-preview-bg`**、**`--custom-chat-preview-dot`**、**`--custom-keyword-bg`**、**`--custom-keyword-border`**，以及 `--keyword-bg`、`--keyword-border-color`

**聊天：语义变量**
`--chat-stage-bg`、`--chat-ic-bg`、`--chat-ooc-bg`、`--chat-preview-bg`、`--chat-preview-dot`、`--chat-preview-dot-ic`、`--chat-preview-dot-ooc`、`--chat-text-primary`、`--chat-text-secondary`、`--chat-border-mute`、`--chat-bubble-border`、`--chat-ooc-border`、`--chat-message-shadow`、`--chat-dice-result-shadow`、`--chat-archived-bg`、`--chat-archived-border`、`--chat-whisper-accent`、`--chat-whisper-surface`、`--chat-whisper-bg`、`--chat-whisper-border`、`--chat-whisper-tag-bg`、`--chat-whisper-tag-fg`、**`--chat-inline-code-bg`**、**`--chat-inline-code-fg`**、**`--chat-inline-code-border`**、`--chat-editing-preview-bg`、`--chat-editing-preview-ic-bg`、`--chat-editing-preview-ooc-bg`、`--chat-live-preview-ic-bg`、`--chat-live-preview-ooc-bg`

**音频**
`--audio-panel-surface`、`--audio-panel-border`、`--audio-panel-shadow`、`--audio-card-surface`、`--audio-card-border`、`--audio-progress-track`、`--audio-progress-buffer`

**玻璃**
`--sc-glass-bg`、`--sc-glass-solid`、`--sc-glass-border`、`--sc-glass-shadow`、`--sc-glass-blur`、`--sc-glass-saturate`、`--sc-glass-contrast`

**收藏备注帖**（只在需要时）：`--mf-accent`、`--sc-color-error`

不要覆盖用户个人设置写入的字号、行高、气泡间距、头像尺寸和消息 padding。

### 令牌结构建议

```css
:root[data-sc-surface] {
  color-scheme: light;            /* 与默认配色一致 */
  --xx-…: …;                      /* 私有原料：颜色、纹理、字体栈 */
  --sc-…: var(--xx-…);            /* 语义映射，只写一次 */
}
:root[data-sc-palette='night'] {
  color-scheme: dark;
  --xx-…: …;                      /* 只改原料，映射自动跟随 */
}
```

把语义映射写在共同块里、日夜块只改私有原料，能保证两套配色覆盖一致，`check_theme.mjs` 的对比度计算也能解析出来。

## 4. 稳定 hook（`data-sc-ui`）

`app`、`header`、`sidebar`、`channel-list`、`message-list`、`message`、`composer`、`world-console`、`admin-settings`、`user-profile`、`ui-customization-trust`、`selection-toolbar`、`keyword-tooltip`、`mention-popup`、`candidate-popup`、`image-popup`、`identity-popup`、`font-popup`、`color-popup`、`floating-panel`、`message-favorite-inspector`。

收藏备注帖：`[data-sc-favorite-note='panel'|'list'|'item'|'composer'|'editor'|'lock'|'summary'|'add']`，`item` 带 `data-sc-state`。

公开 class：`.sc-overlay`、`.sc-message-context-menu`、`.sc-avatar-context-menu`。小剧场 `.theater-secondary-surface` 有自己的 palette。

## 5. 兼容性敏感选择器清单

以下 class 在参考主题中实际使用，2026-11-12 在 `ui/src` 中确认存在。它们不是稳定接口，使用时在 CSS 注释中标“兼容性敏感”，并尽量挂在稳定 hook 之下。

| 选择器 | 含义 |
| --- | --- |
| `.sc-app-shell`（及其下 `.n-layout*`） | 应用外壳；常用于把 Naive 布局层设为透明以露出 body 背景 |
| `[data-sc-ui='message-list'].chat--layout-compact` | 紧凑布局下的消息列表 |
| `.message-row__surface`、`--tone-ic`、`--tone-ooc` | 紧凑布局下消息行底色层 |
| `.chat-item`、`.chat-item--ic`、`.chat-item--ooc`、`.chat-item--self`、`.chat-item--layout-bubble` | 消息主体与状态 |
| `.chat-item > .right > .content` | 消息正文容器（骰子 chip 也在其中） |
| `.chat-item .title`、`.name`、`.time` | 消息头：名字、时间 |
| `.chat-item__avatar`、`.chat-item__avatar--hidden`、`.avatar-shell` | 头像 |
| `.chat-pinned-zone` | 置顶消息区 |
| `.message-action-bar__btn` | 消息操作条按钮 |
| `.typing-preview-bubble` | 正在输入预览 |
| `.dice-chip`（`data-dice-formula`、`data-dice-result-detail`） | 骰子结果 chip |
| `.keyword-highlight`、`.keyword-highlight--underline` | 世界术语高亮 |
| `.message-quote` | 引用 |
| `.tiptap-spoiler`、`.tiptap-ruby` | 剧透、注音 |
| `.sc-channel-title` | 顶栏频道标题 |
| `.sider-item`、`.sider-item.active`、`.channel-item-title` | 频道列表项 |
| `.sc-sidebar__world-action-button`、`.sidebar-footer-actions` | 侧栏按钮 |
| `.hybrid-input`、`.send-action-btn`、`.identity-switcher`、`.chat-input-actions` | 输入区 |
| `.chat-root-container > .text-2xl` | 空频道提示 |
| `.sign-in-content` | 登录卡片 |
| `.world-lobby-header`、`.world-grid-card`、`.world-hero__title` | 世界大厅 |
| `.world-console-panel`、`.world-console-section-title` | 世界控制台 |
| `.n-card`、`.n-input`、`.n-button`、`.n-tabs*`、`.n-dialog`、`.n-drawer`、`.n-popover` 等 | Naive UI，优先改其 `--n-*` 变量 |

不要依赖：`.mx-context-menu-*`、`.v-binder-follower-*`、`.n-dropdown-option-body`、带 hash 的类名、`.n-card > .n-card__content > …` 这类层级。

## 6. 脚本 API 速查（`sc`，apiVersion 2）

### 6.1 执行方式

- 脚本正文不含顶层 `import` / `export` 时，运行时包装成 `export default async function activate(sc) { …正文… }`，可以直接写顶层 `await` 和 `sc.*`。
- 写成 ES module 时，`export default` 一个 `(sc) => dispose?` 函数；返回的函数会在最后执行清理。
- 通过 Blob module 在同源执行。**不是沙箱**，平台包直接运行；世界包需要平台 `allowWorldJs` 和每个浏览器的信任（绑定 package ID + 规范化 JS hash + 能力列表）。
- 激活失败时，已注册的清理会被执行，同一版本不会重试。

### 6.2 方法

| 方法 | 说明 |
| --- | --- |
| `sc.apiVersion` / `sc.packageId` | `2` / 当前包 ID |
| `sc.context()` | `{ surface, worldId, channelId, observer, palette }`，每次返回新副本 |
| `sc.channel()` | 有有效登录频道时返回频道 client，否则 `null`；切换频道后旧 client 失效，需重新获取 |
| `sc.cleanup(fn)` | 注册清理函数，停用、卸载、安全模式时执行 |
| `sc.on(event, handler)` | 订阅事件，返回取消函数，自动清理 |
| `sc.navigate(target)` | 路由跳转 |
| `sc.notify(type, message)` | `info` / `success` / `warning` / `error` 提示 |
| `sc.listen(target, type, handler, options?)` | 自动清理的 `addEventListener` |
| `sc.timeout(fn, ms?)` / `sc.interval(fn, ms)` | 自动清理的定时器，返回取消函数 |
| `sc.observe(node, options, callback)` | 自动断开的 `MutationObserver` |

### 6.3 事件

| 事件 | payload |
| --- | --- |
| `surface-change` | `{ surface, previous }` |
| `world-change` | `{ worldId, previous }` |
| `channel-change` | `{ channelId, previous }` |
| `theme-change` | `{ palette, previous }` |
| `message-created` / `message-updated` / `message-deleted` | `{ type: 'sealchat.bridge.message', event, worldId, channelId, messageId, createdAt?, icMode: 'ic'\|'ooc', isWhisper, identityId, displayName, color, avatarUrl, contentRaw, contentText }` |

`message-created` 触发时 DOM 可能还没渲染。要标记消息行，按 `[data-sc-ui="message"][data-message-id="<id>"]` 查找，找不到时用 `sc.timeout` 短暂重试几次。

### 6.4 频道能力

`context.read`、`user.read`、`members.read`、`world.admins.read`、`characters.read`、`characterCard.read`、`characterCard.write`、`permissions.read`、`storage.read`、`storage.write`、`events.subscribe`、`events.publish`、`messages.send`、`attachments.upload`、`theater.dialogue.subscribe`、`theater.character.read`。

- 写能力：`characterCard.write`、`attachments.upload`、`storage.write`、`events.publish`、`messages.send`；observer / 只读身份不可用。
- `storage.write`、`events.publish`、`messages.send` 还需要频道文本发送权限。
- `theater.*` 需要宿主注入 provider，普通世界脚本通常拿不到。
- 能力变化会让所有浏览器重新确认信任。主题脚本默认不申请能力；确实需要时按最小集合声明，并在说明中写清用途。
- 方法细节见 `original/channel-embed-api-developer-guide.md`。

### 6.5 脚本骨架（结构示意，不含视觉）

```js
const doc = document
const root = doc.documentElement
const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
const QUIET = new Set(['embed', 'log-public', 'theater'])

root.dataset.scTheme = '<slug>'
sc.cleanup(() => { if (root.dataset.scTheme === '<slug>') delete root.dataset.scTheme })

const layer = doc.createElement('div')
layer.className = '<prefix>-layer'
layer.setAttribute('aria-hidden', 'true')
doc.body.appendChild(layer)
sc.cleanup(() => layer.remove())

const sync = () => { layer.hidden = QUIET.has(sc.context().surface) }
sync()
sc.on('surface-change', sync)

let queued = false
const scan = () => {
  for (const row of doc.querySelectorAll('[data-sc-ui="message-list"] [data-sc-ui="message"]')) {
    // 用签名跳过未变化的行；只写 data-<prefix>-* 标记
  }
}
const queue = () => {
  if (queued) return
  queued = true
  sc.timeout(() => { queued = false; scan() }, 200)
}
scan()
sc.observe(doc.body, { childList: true, subtree: true }, queue)
sc.on('message-updated', queue)
sc.cleanup(() => { /* 删除写入的 data-<prefix>-* 标记 */ })
```

## 7. 常见坑

- **颜色不生效**：按原指南 §18 顺序排查（启用 → targets → 包顺序 → 变量 → 内联变量 → 选择器 → `!important`）。
- **body 背景被盖住**：Naive 布局层有自己的背景，需要把 `.sc-app-shell` 下的 `.n-layout*` 设为透明（兼容性敏感）。
- **气泡布局与紧凑布局**：两者结构不同，`--chat-ic-bg` 在紧凑布局里作用在 `.message-row__surface`。两种布局都要看。
- **Teleport 浮层**：下拉、Popover、Dialog、Drawer 挂在 `body` 下，组件内部选择器匹配不到。
- **骰子文本**：`.dice-chip` 在 `.content` 里；用正文文本做正则时要排除 chip，或优先读 `data-dice-result-detail`。不同骰子格式的 detail 不一定包含 `d20=` 这样的前缀，先拿真实消息确认格式。
- **伪元素文案**：`content:` 插入的文字读屏器可能会读出；纯装饰时用符号或给出简短、不误导的文字。
- **全局减少动画规则**：必须排除 `.tiptap-performance`、`.twin-layer-message__char`、`.twin-layer-message__char-glyph`。
- **固定层级**：装饰层用 `pointer-events: none`，z-index 不要盖住 Naive 浮层与对话框。

## 8. 复核命令

SealChat 升级后，用这些命令确认本速查仍然准确：

```bash
rg -n "UI_SURFACES|UI_PUBLIC_SURFACES" ui/src/services/ui-customization/surfaces.ts
rg -n "UI_CUSTOMIZATION_EVENTS|interface SealChatCustomizationAPI" -A14 ui/src/services/ui-customization/jsRuntime.ts
rg -n "ChannelExtensionCapabilities" service/channel_extension.go
rg -o 'data-sc-ui="[a-z-]+"' ui/src | sed 's/.*data-sc-ui=//' | sort -u
```
