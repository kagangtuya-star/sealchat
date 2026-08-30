# SealChat 文档

这里是 SealChat 的统一文档入口。项目首页只提供产品概览和最短启动路径；安装、配置、运维、开发以及对外协议分别在下列文档中维护。

## 部署与管理

- [部署指南](deployment.md)：选择 Docker Compose、Docker 或二进制发行包，完成启动、反向代理和升级。
- [配置指南](configuration.md)：配置数据库、本地与 S3 存储、邮件、音频及敏感信息。
- [运维指南](administration.md)：检查服务、管理配置历史、重置密码、备份恢复和处理常见故障。

## 开发

- [开发指南](development.md)：搭建开发环境，构建前后端，运行检查并了解发行结构。
- [角色卡模板开发](character-sheet-template-development.md)：角色卡 iframe 的数据、事件、模板 API 和调试方法。
- [Channel Embed API 开发指南](channel-embed-api-developer-guide.md)：为频道 iForm 或第三方 iframe 开发嵌入应用。
- [Fluid Modal 弹窗模板](ui/fluid-modal-template.md)：后台宽表格弹窗的前端布局约定与验证清单。

## 外部集成

- [SealChat Bridge API](sealchat-bridge-api.md)：从外部宿主页面嵌入完整频道页并读取角色与消息流。
- [Webhook 外部数据操作 API](sealchat-webhook-api.md)：使用轮询与写入接口同外部系统同步消息。
- [频道未读提醒](channel-digest-push.md)：生成频道或世界摘要，并通过主动推送或带令牌接口交付。

## 专题与内部设计

- [小剧场对话框与角色演出资源设计方案](theater-dialog-module-design.md)：小剧场演出资源与运行时方案，文档当前标记为“Phase 5 已实现，待评审”。
- [SealChat Agent 爬取协议](SEALCHAT_AGENT_CRAWL_GUIDE_COMPACT.md)：服务端内嵌的只读 Agent 抓取协议说明。

设计文档记录特定基线下的方案和实现状态，不一定构成稳定公开 API；使用前应同时查看文档状态、当前代码和 Release Notes。对外协议也可能随持续开发调整，集成方应做好版本与能力检测。

## 文档维护

新增文档应放在 `doc/`，并加入本索引。不要新建并行的 `docs/` 文档根目录，也不要把部署、配置或运维细节复制回项目首页。
