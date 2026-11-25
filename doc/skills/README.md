# SealChat Agent Skills 目录

本目录收录面向 AI Coding Agent（支持 `SKILL.md` 规范的工具，如 Claude Code、Codex、Cursor、Antigravity 等）的 SealChat 官方技能包（Skills）。

这些技能包将 SealChat 的扩展协议、运行时约束、审美与原创性原则、以及打包校验脚本整理为可执行的工作流，帮助 Agent 端到端地设计、实现、校验并交付原创扩展产物。

> 接口、校验规则与运行时行为始终以当前源码和 `doc/` 下的正式开发文档为准。

## 技能总览

| Skill 名称 | 目录入口 | 适用场景 | 主要产出 | 配套脚本 | 对应正式文档 |
| --- | --- | --- | --- | --- | --- |
| `log-layout-style-designer` | [log-layout-style-designer/SKILL.md](log-layout-style-designer/SKILL.md) | 为 TRPG 战役、规则体系或模组设计原创 LOG 排版样式主题包 | StylePackage JSON、标准可导入 `.zip`、交互式 HTML 预览 Demo | `scripts/pack_style_zip.py`（配合 `doc/log-style-validate.py`） | [LOG 排版样式开发指南](../log-layout-style-authoring.md) |
| `sealchat-theater-room` | [skills sealchat-theater-room/SKILL.md](skills%20sealchat-theater-room/SKILL.md) | 为 TRPG 模组设计小剧场幕布/跑团房间、iForm 网页组件、GM 同步特效与对话框皮肤 | 单文件 HTML iForm 组件、对话框透明 PNG（默认 `1920×336`）、预览台与使用文档 | `scripts/sync_runtime.py`、`scripts/check_embed.py`、`scripts/export_png.py` | [Channel Embed API 开发指南](../channel-embed-api-developer-guide.md)、[小剧场对话框与角色演出资源设计方案](../theater-dialog-module-design.md) |
| `sealchat-ui-theme-design` | [skills sealchat-ui-theme-design/SKILL.md](skills%20sealchat-ui-theme-design/SKILL.md) | 为世界观或跑团场景设计原创界面自定义包（世界皮肤 / 平台换肤，CSS + 可选 JS） | `sealchat.ui-package` JSON、`theme-<slug>.css`、（可选）`theme-<slug>.js` 与设计说明 | `scripts/pack_theme.mjs`、`scripts/check_theme.mjs`、`scripts/contract.mjs` | [UI 自定义包开发指南](../ui-customization-package/ui-customization-package-development-guide.md)、[Channel Embed API 开发指南](../channel-embed-api-developer-guide.md) |

---

## 技能详情

### 1. `log-layout-style-designer` — LOG 排版样式主题设计与开发

入口：[log-layout-style-designer/SKILL.md](log-layout-style-designer/SKILL.md)

- **工作流**：情报搜集与合约核对 → 创意设计方案（先确认再动手）→ 交互式 HTML 预览 Demo → 正式 JSON 样式包与 ZIP 打包 → 服务端校验与启用指引。
- **核心约束**：
  - 严格遵守 `ValidateLogStyle` 的 CSS / HTML / `settingsSchema` 白名单与无障碍对比度要求；
  - 默认消息以长文阅读舒适为优先，兼容悄悄话、引用、线索卡片与打印模式退化；
  - 再次迭代已有包 ID 时递增 SemVer `version`，使用专用脚本生成 POSIX 路径与 `0644` 权限的 ZIP 包。
- **目录结构**：
  - [SKILL.md](log-layout-style-designer/SKILL.md)：5 阶段工作流与高频易错问题速查
  - [references/pitfalls-and-runtime.md](log-layout-style-designer/references/pitfalls-and-runtime.md)：默认页眉页脚、字体覆盖、白名单雷区与 ZIP 规范
  - [scripts/pack_style_zip.py](log-layout-style-designer/scripts/pack_style_zip.py)：按服务端 `ImportLogStyle` 要求打包标准 `.zip`

### 2. `sealchat-theater-room` — 小剧场幕布房间与对话框设计

入口：[skills sealchat-theater-room/SKILL.md](skills%20sealchat-theater-room/SKILL.md)

- **工作流**：情报分级（公开 / GM 专用 / 秘密）→ 素材清单 → 风格立项（发散 3 个差异方向并择优）→ 规划场景与事件 → 基于工程骨架重写组件 → 注入运行时 → 绘制并导出同源对话框 PNG → 约束检查、浏览器预览与交付。
- **核心约束**：
  - 单个组件 ≤ 88192 字节（UTF-8），不使用 `alert/confirm/prompt` 与外部 `<script src>`；
  - 需要联动的界面放进同一个 iForm（Storage / Events 按 iForm 隔离）；
  - 防剧透原则：秘密、真实身份、解除条件与暗号永不写入房间组件或对话框底图。
- **目录结构**：
  - [SKILL.md](skills%20sealchat-theater-room/SKILL.md)：9 步流程、模板清单、脚本命令与硬约束
  - `references/`：[设计核心](skills%20sealchat-theater-room/references/design-essence.md)、[平台硬约束](skills%20sealchat-theater-room/references/sealchat-stage.md)、[8 步实现流程](skills%20sealchat-theater-room/references/room-workflow.md)、[组件与运行时目录](skills%20sealchat-theater-room/references/component-catalog.md)、[对话框底图规范](skills%20sealchat-theater-room/references/dialogue-box.md)
  - `templates/`：主背景（`frame.html`）、开场揭幕（`opening.html`）、氛围粒子（`ambient.html`）、资料夹（`dossier.html`）、演出核心（`stage-core.html`）、对话框设计源（`dialogue-box.html`）与 `runtime.js`
  - `scripts/`：`sync_runtime.py`（注入/更新运行时）、`check_embed.py`（嵌入约束静态检查）、`export_png.py`（无头 Chrome 导出透明 PNG 并校验）
  - [examples/paradise/README.md](skills%20sealchat-theater-room/examples/paradise/README.md)：忍神模组《乐园》孔版会志版完整实例与本地预览台

### 3. `sealchat-ui-theme-design` — 原创 UI 自定义包（世界皮肤 / 平台换肤）设计

入口：[skills sealchat-ui-theme-design/SKILL.md](skills%20sealchat-ui-theme-design/SKILL.md)

- **工作流**：线索收集 → 概念发散与选择 → 设计系统（私有令牌与语义变量映射）→ 分区实现（日夜双配色与多 Surface 适配）→ 打包 `sealchat.ui-package` JSON → 静态校验与对比度检查 → 交付。
- **核心约束**：
  - 强调原创性与“线索先于风格”，不仿造参考主题或其他产品界面，正文可读性高于氛围；
  - 保护文字演出节点与保留关键帧、保留 `:focus-visible`、不加载远程字体或外链图片；
  - 脚本可选且克制，所有监听器、定时器与观察器统一经 `sc.*` 注册并在卸载时清理。
- **目录结构**：
  - [SKILL.md](skills%20sealchat-ui-theme-design/SKILL.md)：9 条硬规则、7 阶段流程与参考索引
  - `references/`：[线索收集](skills%20sealchat-ui-theme-design/references/design-discovery.md)、[原创性与审美自检](skills%20sealchat-ui-theme-design/references/originality-and-aesthetics.md)、[实现参考与 API 速查](skills%20sealchat-ui-theme-design/references/implementation-reference.md)、[校验矩阵](skills%20sealchat-ui-theme-design/references/validation.md)，以及 `references/original/`（随 skill 附带的开发指南与参考主题副本）
  - [assets/design-brief.md](skills%20sealchat-ui-theme-design/assets/design-brief.md)：线索板与设计决定记录表单
  - `scripts/`：`pack_theme.mjs`（打包 JSON）、`check_theme.mjs`（包结构、资源、对比度与规则静态检查）、`contract.mjs`

---

## 如何使用

1. **安装到你的 AI Coding Agent**：
   将需要的技能目录复制到对应工具的 skills 发现目录（例如项目级 `.agents/skills/`、`.claude/skills/`，或对应工具的用户全局 skills 目录）。
   - 建议按各 `SKILL.md` 顶部 frontmatter 的 `name` 命名目标文件夹：
     - `log-layout-style-designer`
     - `sealchat-theater-room`（仓库内当前目录名为 `skills sealchat-theater-room`）
     - `sealchat-ui-theme-design`（仓库内当前目录名为 `skills sealchat-ui-theme-design`）
   - 复制后，如 `SKILL.md` 中的脚本示例路径与你的安装目录不同，按实际路径调用即可。
2. **在对话中直接引用**：
   如果不想复制，也可以直接在提示词中让 Agent 先阅读对应目录的 `SKILL.md` 及 `references/`，再按工作流执行。
3. **本地脚本运行环境**：
   - `log-layout-style-designer` 与 `sealchat-theater-room` 的打包/校验脚本需要 **Python 3**（导出对话框 PNG 需本机可用 Chrome/Chromium 或 Playwright 等无头浏览器环境，详见脚本说明）。
   - `sealchat-ui-theme-design` 的打包/校验脚本需要 **Node.js**。

## 维护约定

- 新增或更新技能时，请同步更新本索引表及 [SealChat 文档索引](../README.md)。
- 技能内附带的协议副本（如 `sealchat-ui-theme-design/references/original/`）与仓库主文档不一致时，以仓库 `doc/` 和当前源码为准并及时同步。
- 技能文档、示例与产出包中不得包含真实聊天记录、私人头像、账号凭据或模组剧透秘密。
