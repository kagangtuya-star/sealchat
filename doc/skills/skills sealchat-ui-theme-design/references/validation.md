# 校验

## 1. 静态检查

```bash
node .claude/skills/sealchat-ui-theme-design/scripts/check_theme.mjs <package.json>
node .claude/skills/sealchat-ui-theme-design/scripts/check_theme.mjs --css theme-x.css [--js theme-x.js]
```

加 `--json` 输出机器可读结果。退出码：有 `ERROR` 时为 1，否则为 0。

| 检查 | 级别 | 说明 |
| --- | --- | --- |
| 包格式、`formatVersion`、`runtimeApiVersion` | ERROR | 与服务端导入校验一致 |
| 名称 1～100 字、说明 ≤500 字 | ERROR | 按 Unicode 字符计 |
| `targets` 合法、1～32 项、不含 `internal` | ERROR | `global` 不覆盖公共 Surface 时给提示 |
| `channelCapabilities` 合法 | ERROR | 写能力和 theater 能力给提示 |
| CSS + JS ≤ 1024 KiB | ERROR | 可用 `--max-kb` 调整 |
| `sc-performance-*` 关键帧 | ERROR | 保留命名空间 |
| 富文本演出节点上的 `animation` / `transform` / `display` / `filter` / `opacity` | ERROR | 受保护属性 |
| 远程 `@import`、`url(http…)`、远程 `@font-face` | ERROR | 不加载远程资源 |
| 脚本中的 `fetch` / `XMLHttpRequest` / `WebSocket` / `sendBeacon` / `EventSource` | ERROR | 主题脚本不发网络请求 |
| `eval` / `new Function` | ERROR | 同上 |
| 缺少日间或夜间配色块 | WARN | 两套都要成立 |
| 缺少 `prefers-reduced-motion` | WARN | 有动画时必须提供 |
| 全局减少动画规则没有排除演出节点 | WARN | 见原指南 §5 |
| `outline: none` 但没有 `:focus-visible` | WARN | 保留焦点 |
| `!important` 数量较多、深层 `.n-*` 层级 | WARN | 兼容性风险 |
| 宽度写死大于 640 px | WARN | 窄屏风险 |
| 引用不推荐的内部 class | WARN | `.mx-context-menu-*` 等 |
| 21 个传统颜色字段变量缺失 | WARN | 列出缺失项 |
| 参考主题私有前缀 | WARN | 原创性提醒 |
| 脚本直接使用 `addEventListener` / `setTimeout` / `setInterval` / `MutationObserver` | WARN | 应该改用 `sc.*` 或确认已在 `sc.cleanup` 中释放 |
| 脚本没有检查减少动画 | WARN | 有动效时需要 |
| 脚本 `innerHTML` 拼接 | WARN | 不要把消息内容拼进 HTML |
| 对比度 | ERROR / WARN | 正文 < 4.5 为 ERROR；次要文字 < 4.5、强调色与徽标 < 3 为 WARN |

对比度只计算能静态解析的颜色（十六进制、`rgb()`、`hsl()`、`transparent` 和变量链）。`color-mix()`、渐变、`currentColor` 会显示“无法解析”，这些要在渲染检查里用开发者工具人工确认。

## 2. 渲染检查矩阵

条件允许时在真实 SealChat 中导入并启用，至少看下面这些组合。截图或描述时写明组合。

| 维度 | 取值 |
| --- | --- |
| Surface | `targets` 中的每一个；至少 `chat` |
| 配色 | 日间、夜间 |
| 布局 | 紧凑、气泡 |
| 宽度 | 桌面（≥1280）、窄屏（≤390） |

每个组合检查：

- 正文、名字、时间、OOC、悄悄话、代码、术语、引用、剧透、注音、骰子结果都可读。
- 自己的消息与别人的消息可区分；置顶区、编辑预览、正在输入预览正常。
- 频道列表悬停、激活、未读徽标。
- 输入区：身份切换、富文本工具、发送按钮、候选浮层（提及、术语、图片）。
- Teleport 浮层：下拉菜单、Popover、Dialog、Drawer、右键菜单、通知。
- 世界控制台、平台管理、用户资料（若 `targets` 覆盖）。
- 键盘 Tab 时焦点清晰可见。
- 开启系统“减少动画”后装饰动效停止，富文本演出仍正常。
- 脚本：停用包后所有装饰元素和 `data-<prefix>-*` 标记消失；切换频道、切换日夜、进入安静 Surface 时表现正确；控制台没有报错。
- `?safe-ui=1` 能恢复默认界面。

## 3. 本地实例验证

在本仓库环境中验证时：

- 管理界面导入：平台管理或世界管理 → 界面自定义 → 导入 JSON → 启用。
- API 导入（平台管理员）：`POST /api/v1/admin/ui-customizations/import` 上传 JSON，`PATCH /api/v1/admin/ui-customizations/:id` 传 `{"enabled": true}` 启用，传 `{"css": "...", "js": "..."}` 迭代。修改后在页面里派发 `sealchat:ui-customization-refresh` 事件即可重新应用。
- 日夜和布局保存在 `localStorage` 的 `sealchat_display_settings`（`{ palette: 'day' | 'night', layout: 'compact' | 'bubble' }`），改完刷新。
- 运行环境的具体做法（后端编译、前端预览、无头浏览器截图）遵循仓库 `AGENTS.md` 与本机记忆中的验证配方；内存有限时不要运行全量类型检查或构建。
- 只用本地测试账号，不要使用或打印真实凭据。

没有实际渲染时，交付说明必须写明“仅做静态检查，未做视觉验证”，并列出需要用户重点看的组合。
