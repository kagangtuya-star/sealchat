# SealChat 文档索引

这里是 SealChat 的统一文档入口。根目录 `README.md` 负责产品概览和常用入口；部署、配置、运维、开发、扩展协议、内部架构和专题资料统一在这里索引。

文档按用途分类，而不是按文件目录机械罗列。稳定接口和开发指南可作为实现参考；内部设计文档记录特定阶段的架构与决策，使用时应同时核对当前源码和 Release Notes。

## 部署、配置与运维

- [部署指南](deployment.md)：选择 Docker Compose、Docker 或二进制发行包，完成启动、反向代理、升级和生产部署。
- [配置指南](configuration.md)：配置数据库、本地与 S3 存储、邮件、音频、PDF、网络和敏感信息。
- [运维指南](administration.md)：检查服务状态、管理配置历史、重置密码、备份恢复和处理常见故障。

## SealChat 本体开发与 UI 规范

- [开发指南](development.md)：搭建开发环境，运行前后端、本地联调、构建、测试并了解发行结构。
- [Fluid Modal 弹窗模板](ui/fluid-modal-template.md)：后台及管理类宽弹窗的布局约定、滚动边界和验证清单。
- [功能总览与视频提纲](feature-overview.md)：按当前代码和历史整理的产品能力地图，适合快速了解模块边界；它不是稳定协议文档。

## 扩展与二次开发

- [角色卡模板开发](character-sheet-template-development.md)：人物卡 iframe 的数据流、模板 API、事件约定和调试方式；参考模板位于 [template/](template/)。
- [Channel Embed API 开发指南](channel-embed-api-developer-guide.md)：为频道 iForm 或第三方 iframe 开发受控嵌入应用。
- [UI 自定义包开发指南](ui-customization-package/ui-customization-package-development-guide.md)：开发主题、世界皮肤和高级 CSS/JavaScript UI 包；同目录附带可导入的赛博朋克 Demo 和 CSS 源码。
- [LOG 排版样式开发指南](log-layout-style-authoring.md)：开发 LOG 自定义样式包、模板绑定、CSS、资源与兼容规则。
- [SealChat Bridge API](sealchat-bridge-api.md)：从外部宿主页面嵌入完整频道页并读取当前角色与消息流。
- [Webhook 外部数据操作 API](sealchat-webhook-api.md)：通过 Change Feed 与写入接口和外部系统进行消息同步。
- [隐式 BOT Interaction](bot-interaction.md)：从前端向频道 Primary BOT 发起不落库的临时调用并等待结果。

## Agent、MCP 与自动化

- [平台 MCP 接入](mcp.md)：平台级 MCP、Personal API Key、OAuth、scopes、鉴权和工具访问约束。
- [Theater MCP](theater-mcp.md)：通过 MCP 读取、设计和控制 World Theater，以及频道嵌入和相关自动化能力。
- [SealChat Agent 爬取协议](SEALCHAT_AGENT_CRAWL_GUIDE_COMPACT.md)：只读 Agent 访问链接的 manifest、消息、计数、游标和安全约束。
- [频道未读提醒](channel-digest-push.md)：生成频道或世界摘要，并通过主动推送或带令牌接口交付给 Bot 或外部系统。

## 架构与内部设计

- [Internal Surface 独立运行环境](Internal_surface_architecture.md)：人物卡、便签、线索和频道组件脱离聊天主界面运行时的宿主、路由和复用架构。
- [小剧场对话框与角色演出资源设计方案](theater-dialog-module-design.md)：小剧场对话、立绘、演出资源和运行时设计；文档保留阶段性实现状态，应结合当前代码阅读。

这类文档主要用于理解内部结构和历史设计决策，不承诺形成长期稳定的公开 API。

## 功能参考、兼容性与来源资料

- [TTS Provider / Model / Voice 能力目录](tts-catalog.md)：TTS Provider、模型、音色、实例路由和创建能力的工程参考。
- [TTS 模型目录与定价](tts-pricing.md)：TTS 模型能力、计费模式、价格来源与最终结算规则。
- [CCFOLIA 导入功能来源说明](CCFOLIA_IMPORT_PROVENANCE.md)：说明 CCFOLIA 房间导入的独立实现来源、测试数据和内容权利边界。

仓库中还包含与具体功能配套的脚本、模板和示例资源，例如 `log-style-validate.py`、`template/` 和 `ui-customization-package/`。这些资源应从对应开发文档进入，而不是作为独立文档逐项维护索引。

## 文档维护

新增面向开发者、部署者或集成方的正式文档时，应放在 `doc/` 或其明确子目录，并在本索引的对应分类中增加一句简介。不要新建并行的 `docs/` 文档根目录，也不要把大段部署、配置、运维或协议细节复制回项目首页。

当实现与文档冲突时，以当前代码为准，并同步修正文档。阶段性设计稿应明确状态或基线，避免被误认为稳定接口。
