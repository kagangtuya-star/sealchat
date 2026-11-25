---
name: sealchat-ui-theme-design
description: 为 SealChat 设计并制作原创的界面自定义包（UI Package / 世界皮肤 / 跑团主题 / 平台换肤，CSS + 可选 JS，导出为 sealchat.ui-package JSON）。从用户的世界观、团风格、规则与素材中收集线索，提出有依据的原创设计方向，再落成日夜双配色、适配各 Surface、可导入可校验的主题包。用户提到 SealChat 主题、界面自定义包、世界皮肤、换肤、UI package、theme-*.css、data-sc-ui、sc.on 脚本，或想为某个团/世界/模组做专属聊天界面时使用。不用于只改 21 个传统颜色字段的场景（那用颜色主题即可）。
---

# SealChat 原创主题设计与制作

这个 skill 的产出是一套**为某个具体用户、具体世界量身定做**的 SealChat UI 自定义包：可直接导入的 `sealchat.ui-package` JSON，以及可读的 `theme-<slug>.css` / `theme-<slug>.js` 源码和一段设计说明。

它不是换皮模板。每个主题都要从用户给出的线索里长出来：世界里的物件、团里的习惯、规则里的机制、玩家在意和讨厌的东西。仓库自带的 4 个主题只用来学习**接口用法**，不能当视觉起点。

## 硬规则

1. **接口以原文档和源码为准。** 动手前读 `references/original/ui-customization-package/ui-customization-package-development-guide.md`（原有开发指南，逐字保留）。Surface、hook、变量、包格式、`sc` API 不得凭印象编造；本 skill 的速查与原文档冲突时，以原文档和 `ui/src/services/ui-customization/` 源码为准。
2. **不仿造。** 不复刻参考主题的标志性手法，不复刻任何现有游戏、应用、品牌的界面、Logo、官方美术或商标视觉。用户点名“做成像 X 一样”时，提炼 X 带给他们的**感受与功能**，再用该世界自己的材料重新表达，并向用户说明这样做的原因。详见 `references/originality-and-aesthetics.md`。
3. **线索先于风格。** 每一个视觉决定（颜色、字体、材质、装饰、动效、文案）都要能指回线索板上的一条线索。说不出来源的装饰就删掉。
4. **正文可读性高于氛围。** 消息正文保持高对比、默认字号和用户的字号/行距/头像个人设置。装饰放在边框、标签、背景、空白处。
5. **保护系统功能样式。** 不改 `.tiptap-performance`、`.twin-layer-message__char(-glyph)` 的 `animation*`、`display`、`transform*`、`filter`、`opacity`；不声明 `sc-performance-*` 关键帧；不隐藏错误提示、访问控制和登录提示；保留 `:focus-visible`。
6. **日夜都要成立。** 两个配色是同一概念的两种情境（例如“白昼的值班室 / 深夜的值班室”），不是简单反色。
7. **不加载远程资源。** 不用 `@import` 远程字体或 `url(http…)` 图片；字体只用本机字体栈加完整 fallback；图形用 CSS 渐变、SVG data URI 或纯 CSS 构造。
8. **脚本可选且克制。** 只有当它表达了世界或规则的意义时才写 JS。纯视觉包 `js` 留空。脚本不发网络请求、不改写消息文本、不移动 SealChat DOM，所有资源经 `sc.listen / sc.timeout / sc.interval / sc.observe / sc.cleanup` 注册。
9. **仓库规则照常适用。** 不改依赖、不 commit、不 push；不修改 `doc/ui-customization-package/` 里的原有主题，除非用户明确要求。新主题默认写到用户指定位置，未指定时写到会话 scratchpad 或 `doc/ui-customization-package/` 旁的新文件（先问用户是否要进仓库）。

## 工作流程

### 阶段 1：线索收集

目标是拿到足够的“具体物”，而不是“风格形容词”。

1. 先从用户已经给的内容里提取线索：世界名、题材、频道名、角色名、规则体系、骰子格式、附带图片或设定文本、他们说过喜欢和讨厌的东西。
2. 信息不足时，**只问一轮**，3～6 个问题，挑对设计影响最大的。问题库和追问技巧见 `references/design-discovery.md`。优先问：
   - 这个世界里，角色会在**什么东西上**读写文字？（信纸、终端、祷文、账本、无线电日志……）
   - 团的气质：紧张、轻松、悲伤、荒诞？跑团时大家多用日间还是夜间、手机还是电脑？
   - 规则与骰子：用什么骰制？有没有值得被界面“看见”的机制（成功等级、资源池、疯狂、压力）？
   - 范围与权限：平台包还是世界包？平台是否开启世界 JS？要覆盖哪些页面（只聊天，还是连登录、大厅、OB、公开 LOG）？
   - 有没有明确不要的东西（某种颜色、动效、恐怖元素、闪烁）？
3. 用户说“你看着办”时，不再追问，按保守假设推进，并在交付时列出假设。
4. 产出**线索板**（写在回复里或 `assets/design-brief.md` 的副本里）：每条线索标注来源（用户原话 / 附件 / 世界数据 / 合理推断），并标出 1～2 条“核心线索”。

### 阶段 2：概念发散与选择

1. 基于线索板提出 **2～3 个明显不同**的方向。每个方向写清：
   - **一句话隐喻**：“这个界面是 ___”（必须来自世界内部，例如“灯塔守夜人的值班簿”，而不是“暗色科技风”）。
   - **依据**：指回哪几条线索。
   - **材质与光**：纸、金属、布、玻璃、石、光源从哪来。
   - **字体性格**：衬线 / 无衬线 / 等宽 / 手写感，用本机字体栈实现。
   - **色彩逻辑**：主色从哪件物品来，强调色代表什么含义（危险、神圣、金钱……），用量比例。
   - **一个签名元素**：整个主题最让人记住的那一处设计，以及它放在哪个 hook 上。
   - **机制钩子（可选）**：规则机制如何被看见（CSS 状态或 JS）。
   - **日 / 夜两种情境**。
2. 用 `references/originality-and-aesthetics.md` 的原创性自检筛一遍，删掉或改造与参考主题、现有产品撞车的部分。
3. 让用户选择或混合。用户授权自主决定时，选线索支撑最强的方向，并说明理由。

### 阶段 3：设计系统

在写选择器之前，先把选定方向落成一组令牌：

1. **私有令牌命名空间**：为主题取 2～3 字母前缀（避开 `nr` `rl` `ta` `sc` `n`），例如 `--lh-*`。原料色、材质纹理、字体栈都放在私有令牌里。
2. **映射到语义变量**：把私有令牌映射到 `--sc-*`、`--chat-*`、`--primary-color*`、`--custom-*`、`--audio-*`、`--sc-glass-*`、`--sc-overlay-*`。完整清单见 `references/implementation-reference.md` §3，不要漏掉 21 个传统颜色字段对应的变量。
3. **日夜两组值**：`:root[data-sc-surface]` 放共同结构和一个默认配色，`:root[data-sc-palette='day'|'night']` 放差异。
4. **装饰清单**：列出签名元素和 2～3 个辅助母题，以及各自落在哪个 hook 上。超过这个数量就是堆砌。
5. **文案声音**：如果用 `content:` 或脚本加入世界内文案（标签、空频道提示、HUD 字样），先定语气，文案要短，并且不能伪装成系统提示或遮挡真实信息。

### 阶段 4：实现

按 `references/implementation-reference.md` 编写。推荐分区顺序（可按需删减，不必照抄）：

1. 头部注释：主题名、隐喻、线索摘要、日夜名称、脚本能力、兼容性说明。
2. 设计令牌（默认配色）→ 另一配色。
3. 画布（body 背景、暗角、纹理）。
4. 稳定 hook：`header`、`sidebar`、`channel-list`、`message-list`、`message`、`composer`。
5. 浮层与其他 hook：`world-console`、`admin-settings`、`user-profile`、各类 popup、`.sc-overlay`。
6. 其他 Surface：只写 `targets` 里真正需要的页面。
7. 兼容性敏感区：`.chat-item*`、`.n-*` 等，每组用注释标出“兼容性敏感”。
8. 脚本增强样式：只作用于主题私有 class 和 `data-<prefix>-*` 标记。
9. 紧凑布局、640 px 以下窄屏、`prefers-reduced-motion`（排除富文本演出节点）。

实现时：

- 先改变量，变量不够再用稳定 hook，最后才用兼容性敏感 class；`!important` 只用于压过组件内联变量。
- Teleport 浮层从 `:root[data-sc-surface] body .n-*` 或 `.sc-overlay` 限定，不要挂在组件内部选择器下。
- 公共 Surface（`observer`、`embed`、`log-public`）不会被 `global` 命中，需要时显式写入 `targets`；公共页面不依赖登录用户和脚本。
- 宽度用 `clamp()` / `min()` / 百分比，不写死侧栏、输入区、弹窗像素宽度。

需要脚本时，先读 `references/implementation-reference.md` §6 的 `sc` API 速查（已对照源码核实），遵守：

- 读取 `prefers-reduced-motion`，减少动画时不播放装饰动效。
- 只向 `body` 追加 `aria-hidden` 装饰元素，或给消息行写 `data-<prefix>-*` 标记；在 `embed`、`log-public`、`theater` 等安静 Surface 隐藏装饰层。
- 本地状态用带主题前缀的 `localStorage` / `sessionStorage` key，并用 try/catch 包裹；需要跨成员共享时才申请 `channelCapabilities`，按最小权限声明。
- 解析消息时读 `.dice-chip` 的 `data-dice-result-detail` / `data-dice-formula`，并考虑骰子文本嵌在 `.content` 里的情况；用 `sc.observe` + 节流扫描，用签名避免重复处理。

### 阶段 5：打包

用 `scripts/pack_theme.mjs` 生成可导入 JSON（不依赖仓库里写死主题列表的 `build-theme-packages.mjs`）：

```bash
node .claude/skills/sealchat-ui-theme-design/scripts/pack_theme.mjs \
  --css theme-<slug>.css [--js theme-<slug>.js] \
  --name "<主题名>" --description "<≤500 字说明>" \
  --targets global,observer,embed,log-public \
  [--capabilities context.read,storage.read] \
  --out sealchat-ui-package-<slug>.json
```

如果用户要求把主题并入仓库并随 `build-theme-packages.mjs` 构建，再修改那个脚本的 `THEMES` 列表，并同步更新开发指南中的主题表。

### 阶段 6：校验与自评

1. 静态检查（必须运行）：

   ```bash
   node .claude/skills/sealchat-ui-theme-design/scripts/check_theme.mjs sealchat-ui-package-<slug>.json
   ```

   它会检查包格式、名称与说明长度、Surface 与能力名、代码体积、远程资源、受保护节点与保留关键帧、焦点样式、减少动画、日夜配色块、脚本资源泄漏、网络请求，以及日夜两套关键文字/背景对的对比度，并提示与参考主题私有命名撞车的地方。`ERROR` 必须修复；`WARN` 逐条判断并在交付说明中解释保留理由。
2. 实际渲染检查：条件允许时在真实 SealChat 实例里导入并启用，按 `references/validation.md` 的矩阵截图检查（Surface × 日/夜 × 紧凑/气泡 × 桌面/窄屏）。没有实际运行就如实说明“未做视觉验证”。
3. 用 `references/originality-and-aesthetics.md` 的审美评分表自评；任何一项低于及格线就回到阶段 3 或 4 修改，不要带着问题交付。

### 阶段 7：交付

交付内容：

- `theme-<slug>.css`、（可选）`theme-<slug>.js`、`sealchat-ui-package-<slug>.json`。
- 设计说明：隐喻、线索 → 设计决定的对应表、签名元素、日夜情境、脚本做了什么、申请了哪些能力及原因。
- 使用说明：导入位置（平台 / 世界）、世界脚本需要 `allowWorldJs` 与浏览器信任、`safe-ui=1` 应急方式。
- 校验结果：实际运行的命令及输出摘要；未验证的项目和已知限制（例如依赖了哪些兼容性敏感 class）。

## 参考文件

| 文件 | 何时读 |
| --- | --- |
| `references/original/ui-customization-package/ui-customization-package-development-guide.md` | **每次必读**。原有开发指南，接口事实来源。 |
| `references/original/channel-embed-api-developer-guide.md` | 脚本需要 `sc.channel()`、Storage、Events、消息发送等频道能力时。 |
| `references/original/ui-customization-package/theme-*.css/js` | 想看某个接口的真实用法时按需检索（`rg`），**不要通读后模仿视觉**。 |
| `references/design-discovery.md` | 阶段 1～2：问题库、线索提取、线索到设计的转化方法。 |
| `references/originality-and-aesthetics.md` | 阶段 2、6：原创性边界、自检清单、审美评分表。 |
| `references/implementation-reference.md` | 阶段 3～4：变量全集、hook 与兼容性敏感选择器清单、`sc` API 速查、常见坑。 |
| `references/validation.md` | 阶段 6：静态检查说明、渲染检查矩阵、本地实例验证方法。 |
| `assets/design-brief.md` | 记录线索板与设计决定的空白表单（不是视觉模板）。 |

`references/original/` 是 `doc/ui-customization-package/` 与 `doc/channel-embed-api-developer-guide.md` 的副本，目录结构与 `doc/` 一致，文内相对链接可用。仓库原文档更新后，以仓库为准，并用下面的命令重新同步：

```bash
cp doc/channel-embed-api-developer-guide.md .claude/skills/sealchat-ui-theme-design/references/original/
cp doc/ui-customization-package/*.{md,css,js,json,mjs} .claude/skills/sealchat-ui-theme-design/references/original/ui-customization-package/
```
