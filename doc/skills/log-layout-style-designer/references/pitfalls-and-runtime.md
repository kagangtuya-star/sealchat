# SealChat LOG 样式开发踩坑与运行时参考

本文沉淀自 SealChat 真实源码与实战调试，供 `log-layout-style-designer` 技能在设计与实现阶段查阅。

---

## 1. 运行时默认配置与页眉页脚陷阱

源码依据：`service/log_project.go`（`defaultLogDraft`）、`ui/src/services/log/logRenderer.ts`（`buildFrameDocument`）、`ui/src/services/log/pagination/logFrameRuntime.js`（`newPage`）。

- **新建工程默认页设置**：
  - 纸张：`A5`（`148mm × 210mm`），四周边距默认 `15mm`（约 `56.7px`）。
  - `headerTemplateId: null` → **`.log-page-header` 根本不会创建在 DOM 中**。
  - `footerTemplateId: null` 且 `showPageNumbers: true` → `.log-page-footer` 会创建，但里面只有一个裸节点：`<span class="log-page-number" data-log-fill="pageNumber">1</span>`。
- **正确设计策略**：
  1. **页面级边角装饰（角花、出血立条等）放进 `.log-page::before` 和 `.log-page::after`**，设 `position: absolute; pointer-events: none;`，这样零配置导入即可见；
  2. **不要覆写 `.log-page` 自身的 `background-size` / `background-position`**，否则用户上传自定义页面背景图时（运行时通过内联 `page.style.backgroundImage` 注入）会被裁坏；
  3. **`.log-page::before` / `::after` 不要画横穿页眉页脚中轴的通栏线**，否则用户开启 `pageHeader` / `pageFooter` 时线条会直接划过文字；
  4. **务必同时美化 `.log-page-number`**，保证未选 `pageFooter` 时默认页码也与主题统一；
  5. **`pageHeader` 内部高度控制在 34–42px 以内**，确保在 `15mm`（~`56px`）默认上边距且 `.log-page-header` 带有 `overflow: hidden` 时绝不被裁切。

---

## 2. 字体继承与富文本保护陷阱

源码依据：`ui/src/services/log/pagination/logFrame.css`、`ui/src/services/log/logTemplateCompiler.ts`。

- `logFrame.css` 明确写了 `.log-body { font-family: var(--log-font-family); }`。
  - 如果只在消息外层容器写 `font-family: var(--log-face)`，正文区 `.log-body` 仍会回退成页面设置里的无衬线字体；
  - 应在需要主题字体的正文槽位上使用 `.ms-body.log-body { font-family: var(--log-face); }`（骰子用等宽、场外用无衬线同理）。
- **保护正文内联富文本**：
  - TipTap 富文本保留显式字色、高亮、字体、下划线和 Ruby 注音；
  - **禁止**在 `.log-body *` 上强设颜色/字体，**禁止**使用 `!important`，以免冲掉作者在正文里特意标出的局部格式。

---

## 3. CSS 校验白名单速查

源码依据：`service/log_style_validation.go`（`logValidateCSS`）。

### 明确禁止（触碰即返回 `INVALID_TEMPLATE`）
- 字符：`>`（包括子选择器 `a > b`）、`<`、`\`、`\x00`；
- At-rule：`@import`、`@keyframes`、`@layer` 等（仅允许 `@media`、`@page`、`@font-face`、`@supports` 及 `@top-*`/`@bottom-*` 页边距盒）；
- 函数：`transform`、`clip-path`、`color-mix()`、`conic-gradient()`、`minmax()`、`repeat()`、`:has()`、`attr()`、`expression()`、`image-set()`、带引号或外链的 `url()`；
- 属性：`behavior`、`-moz-binding`、`all`。

### 允许且适合用来做高完成度纯 CSS 视觉的能力
- 渐变家族：`linear-gradient()`、`radial-gradient()`、`repeating-linear-gradient()`、`repeating-radial-gradient()` —— 可用来画希腊几何饰带、多立克列柱、渐隐分隔线、四角色块、纸张横纹等；
- 边框几何：纯 CSS `border` 三角形（山花屋顶、旗帜、角标）、`border-style: double`、`box-shadow` 多重描边环；
- 数值与颜色：`var()`、`calc()`、`min()`、`max()`、`clamp()`、`rgb()`/`rgba()`、`hsl()`/`hsla()`；
- 选择器函数：`:not()`、`:is()`、`:where()`、`:nth-child()`、`:nth-of-type()`；
- 资源引用：仅当 `assets` 中已声明时，允许**不加引号**的 `url(assets/xxx.png)`（前端正则要求无引号）。

---

## 4. HTML 模板、扩展模板与 JSON Schema 陷阱

源码依据：`service/log_style_validation.go`、`ui/src/services/log/logRenderer.ts`。

1. **HTML 白名单**：
   - 允许标签：`div/span/p/section/article/header/footer/main/aside/figure/figcaption/img/h1~h6/blockquote/strong/em/b/i/br/hr/ul/ol/li/table/thead/tbody/tr/td/th`；
   - 允许属性：`class`、`role`、`alt`、`title`、`data-log-text`、`data-log-if`、`data-log-bind-src`、`data-log-slot`、`data-log-part`；
   - **禁止** `aria-*`、内联 `style`、`id`、`<svg>`、自由 `src`/`href`、事件属性。
2. **扩展消息模板与多个自定义组件**：
   - 任何展开后**恰好包含 1 个 `data-log-slot="body"`** 的模板都会被工作台「LOG 显示规则」与单条「对话框格式」识别为可选消息模板，可用来制作战役专属高光模板（如神谕、神话觉醒、电报、内心独白等）；
   - 任何模板 ID 为 `custom` 或以 `custom` 开头、且在 `settingsSchema` 中注册了闭合 Schema 的模板，都会在「插入元素 → 自定义组件」中被自动列出。
3. **`settingsSchema` 易错点**：
   - 每一层 `type: "object"` 必须包含 `"additionalProperties": false`；
   - 凡是在 `settingsSchema` 中声明了条目的模板，其 `properties` 下**必须包含 `style`**（即使为空闭合对象），否则保存元素草稿会被拒；
   - 非 `custom*` 模板不要声明 `props`；
   - 只使用标准 JSON Schema 标量字段关键字（`type`、`title`、`enum`、`minimum`、`maximum`、`maxLength`），**不要写非标准字段（如 `multiline: true`）**，避免被 Go `jsonschema.Schema` 反序列化拒绝。
4. **Token 限制**：
   - 键名匹配 `^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`；
   - 字符串值 ≤ 200 字符，且**绝不能包含 `; { } \ < > @ ( )`**；
   - 布尔值注入为 `:root[data-log-<全小写键>="true|false"]`（注意键名会转小写，例如 `showAvatar` → `data-log-showavatar`）。

---

## 5. 视觉与排版体验陷阱

1. **纯 CSS 徽标防误读**：
   - 多层同心圆 + 中心红/深色圆点很容易在缩小后被看成“眼球”或“靶心”，若非克苏鲁/监视题材应慎用；
   - 优先使用辨识度高、线条利落的建筑剪影或器物轮廓（如用 `border` 三角 + `linear-gradient` 列柱画神庙，用双框/菱星画徽章）。
2. **日常对白克制原则**：
   - 跑团 LOG 80% 以上篇幅是连续对话，日常 `msg` 模板应保持高扫读效率与宽松呼吸感，避免每条都带重阴影、粗边框或冗长角标。
3. **空字段分隔符处理**：
   - 自定义组件里若有两个可选字段并排（如 `oracle` 和 `omen`），不要用 `<span> · </span>` 硬写分隔符（前项为空时会残留多余中点）；应改用相邻兄弟选择器 `.a + .b::before { content: " · "; }`，并对父容器加 `:empty { display: none; }`。
4. **跨页与分页安全**：
   - 不要给长消息或整个 `.log-clue` 设 `break-inside: avoid`（超过一页会被标 `.log-oversize`），只对标题行（`break-after: avoid`）和图片/短卡片（`break-inside: avoid`）防拆；
   - 流式模式没有 `.log-page` 分页壳，正文样式不得依赖 `.log-page` 存在才能正常阅读（背景与变量可挂 `.log-pages`、`.log-page, .log-flow`）。

---

## 6. ZIP 导入与版本管理陷阱

源码依据：`service/log_style.go`（`ImportLogStyle` / `ExportLogStyle`）。

- ZIP 根目录必须直接包含（**不可套一层外层目录**）：
  - `manifest.json`（其中 `tokens: null`、`templates: null`、`css: ""`、`settingsSchema: null`）
  - `tokens.json`
  - `settings.schema.json`
  - `styles/main.css`
  - `templates/<templateId>.html`
  - （可选）`assets/...` 与 `previews/...`
- 每个 ZIP 条目必须满足：
  - 仅使用正斜杠 `/`（禁止 Windows 反斜杠 `\`）；
  - 普通文件且无执行权限（UNIX `0o100644`，满足 `f.Mode().IsRegular()` 且 `f.Mode()&0111 == 0`）；
  - 未加密（`f.Flags&1 == 0`）。
- **重复导入升级**：同一 `id` 再次导入时，`version` 必须严格大于已导入版本，否则服务端返回 `VERSION_CONFLICT`（`样式版本必须递增`）。
