# SealChat 小剧场嵌入：平台约束与接入

> 依据 SealChat 源码核对（`ui/src/views/theater/stage/StageIframeVisualObject.vue`、`ui/src/components/iform/IFormEmbedFrame.vue`、`api/channel_iform*.go`、`ui/src/bridge/channelEmbed*.ts`）。版本变化时以源码为准。

## 1. 两种挂载方式

| 方式 | 舞台对象 URL | 是否有 Embed Session | 适用 |
| --- | --- | --- | --- |
| **频道 iForm（推荐）** | 舞台「添加网页」→「快速添加内置工具」→「频道嵌入」选 iForm（生成 `#/internal/iform/<id>?world=..&channel=..` 内部链接） | 有（iForm 启用 Embed API 时） | 一切组件 |
| 裸外链 | 直接填 `https://...` | **没有** | 纯装饰、无需同步的页面 |

iForm 内容可以是「嵌入代码」（srcdoc）或「URL」。本 skill 默认产出单文件 HTML 粘贴到嵌入代码。

## 2. 硬约束

| 约束 | 数值 / 行为 | 应对 |
| --- | --- | --- |
| 嵌入代码长度 | **≤ 88192 字节**（服务端 `sanitizeEmbedCode`，UTF-8 字节；中文 3 字节） | 组件拆分；位图用舞台图片对象或外链 URL iForm；`check_embed.py` 检查 |
| srcdoc 沙箱 | `allow-scripts allow-forms allow-pointer-lock allow-popups allow-modals allow-downloads`，**无 `allow-same-origin`** | `localStorage` / `sessionStorage` / `document.cookie` 访问会抛错 → 全部 try；持久状态用 Embed Storage |
| URL iForm / 舞台外链沙箱 | `allow-same-origin allow-scripts allow-forms allow-pointer-lock allow-popups` | — |
| 对话框 | 文档要求不用 `alert/confirm/prompt` | 页面内 DOM；危险操作"点两次确认" |
| 背景 | 舞台对象容器透明 | `html, body { background: transparent }` |
| 配色方案 | 父子 `color-scheme` 不一致时浏览器会在 iframe 后垫不透明底 | `<meta name="color-scheme" content="light dark">` |
| 交互 | 对象 `interactive=false` 时整个 iframe `pointer-events:none` | 全屏层关交互；可点组件做成小尺寸 |
| 外部字体 | Google Fonts 在部分网络环境不可达，阻塞式 `<link>` 会让组件长时间空白 | 非阻塞加载 + 系统字体回退 |
| 缩放 | 对象可设「网页缩放」0.25–5，iframe 内 `innerWidth = 对象宽 / 缩放` | 组件自身按 1920×1080 设计并等比覆盖缩放 |
| Media FX | 宿主可对整个 iframe 外层施加动效 | 组件不要依赖父层 transform |

## 3. Embed API 要点（SealStage 运行时已封装）

- 握手：`SealChatEmbed.connect({ targetOrigin: __SEALCHAT_EMBED_CONFIG__.hostOrigin })`；SDK 由宿主注入的 `sdkUrl` 动态加载，**不要** `<script src>` 写死。
- **Storage / Events 作用域 = (频道, iForm)**。不同 iForm 之间不能通信；同一 iForm 在舞台上放多份、或同时作为聊天浮窗打开，则共享状态。
  → 需要联动的界面（控制台 + 特效层）必须做进**同一个 iForm**，用自身尺寸区分角色。
- Storage：JSON KV，key ≤ 128 字符，值 ≤ 64 KiB，写入有限流（约 30 次/窗口）。用于"迟到者也要看到"的状态。
- Events：不持久化，topic `^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,63}$`，payload ≤ 16 KiB，有限流（约 60 次/窗口）。用于"此刻一起播放"的特效。发布者也可能收到回声 → 用 `_id` 去重。
- GM 判断：`permissions.getCurrent()` 的 `canManageWorld / isWorldAdmin / isWorldOwner`。不要用 `storage.write` 反推身份。
- 新建 iForm 默认能力已含 `storage.*`、`events.*`、`permissions.read`；旧 iForm 不会自动补，需要手动添加。
- 断线：`session.onClosed` 后重新 `connect()`；`HANDSHAKE_FAILED / SESSION_EXPIRED / CONTEXT_CHANGED / TIMEOUT / WS_OFFLINE / INTERNAL_ERROR` 退避重试，其余错误不重试。

## 4. 角色按尺寸切换（一个 iForm，多种身份）

```js
const role = innerWidth < 800 ? 'console' : 'overlay'   // 舞台铺满 = 1920 宽；浮窗 ≈ 420 宽
addEventListener('resize', pickRole)                      // 拖动缩放时自动切换
```

- 控制台：GM 在聊天侧把该 iForm **以浮窗打开**（玩家看不到），或小尺寸放在舞台上公开显示。
- 特效层：铺满舞台、置顶、关闭交互。
- 同时提供 `CONFIG.role` 与 `?role=` 覆盖，便于预览与特殊布局。

## 5. 场景与生命周期

- 小剧场场景切换会卸载 / 挂载 iframe → **"挂载即播放"** 是最可靠的入场动画触发方式（序幕、切幕标题）。
- 同一组件需要跨场景常驻时：在每个场景各放一份（引用同一 iForm 即可共享状态），或使用小剧场的场景固定对象。
- 迟到者：Events 不重放；特效结束后应停留的结果（结局卡、计数）必须写进 Storage，由组件在加载时渲染静态版本。

## 6. 推荐层级（自下而上）

1. 主背景 `frame`（铺满，不可交互）
2. 场景图对象（主窗 960×540 @ (480,140)）
3. 立绘 / 对话框（SealChat 内置；对话框底图在「小剧场外观」上传，见 `dialogue-box.md`）
4. 氛围层 `ambient`（铺满，不可交互）
5. 资料夹 `dossier`（一翼，可交互）
6. 演出核心特效层 `stage-core` / 模组专用核心（铺满，不可交互，最上）
