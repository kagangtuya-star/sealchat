# SealChat

<div align="center">

面向 TRPG、文字跑团与角色协作场景的自托管实时聊天平台

[在线体验](https://kagangtuya-sc.sealdice.com/) · [服务端 Releases](https://github.com/kagangtuya-star/sealchat/releases) · [APP Releases](https://github.com/kagangtuya-star/sealchat-app/releases/) · [文档](doc/README.md) · [QQ 群](https://qm.qq.com/q/wL4lD8saIM)

[综合使用说明](https://bv1ofo8afz3.feishu.cn/wiki/DVPgwwwbBi4CVpk5JY3cw3Ijnoh) · [常见问题答疑](https://my.feishu.cn/wiki/FTrOwMTY8itexxkNs6lcPw3Ens8)

[![Latest Release](https://img.shields.io/github/v/release/kagangtuya-star/sealchat?style=flat-square&label=Latest%20Release)](https://github.com/kagangtuya-star/sealchat/releases/latest)
[![Downloads](https://img.shields.io/github/downloads/kagangtuya-star/sealchat/total?style=flat-square&label=Downloads)](https://github.com/kagangtuya-star/sealchat/releases)
[![Backend Go](https://img.shields.io/badge/Backend-Go-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![Frontend Vue](https://img.shields.io/badge/Frontend-Vue-4FC08D?style=flat-square&logo=vuedotjs&logoColor=white)](https://vuejs.org/)

</div>

## 项目介绍

SealChat 以“世界 → 频道 → 消息”组织长期跑团、文字演绎和角色协作。参与者可以在不同频道使用独立身份与角色外观，并通过世界和频道权限控制主持人、玩家、观众及 Bot 的访问边界。

平台覆盖实时聊天、IC/OOC、骰子、角色卡、素材管理、搜索和导出，也提供嵌入页、Bridge 与 Webhook 等扩展入口。它同样适合同人社区及其他需要多角色协作的自托管场景。

服务端使用 Go，前端使用 Vue 3 与 Vite。前端产物可嵌入服务端发行包；项目提供 Docker 镜像和单体发行包，默认使用 SQLite，也支持 PostgreSQL 与 MySQL。

## 截图

![SealChat 主界面](https://github.com/user-attachments/assets/e307c10f-b057-459a-a174-7892f86a9a97)


![SealChat 界面截图 1](https://github.com/user-attachments/assets/2530ed53-9e95-43eb-b3ef-ed6ed659f1e0)

![SealChat 界面截图 2](https://github.com/user-attachments/assets/47534f2c-6c39-4ce1-8c5d-f0fbeff4591f)

## 主要能力

### 世界、频道与角色

- 世界与多级频道，支持公开、私有、成员权限与旁观协作
- 频道身份、角色切换、共享身份与独立角色外观
- 人物卡模板、快照、角色徽标与角色状态展示
- 主持、玩家、观众和 Bot 等不同协作边界

### 跑团与聊天

- WebSocket 实时聊天、IC/OOC、悄悄话、消息历史与跨频道提醒
- 骰子指令、骰子宏、富文本、文字演出效果与角色化发言
- 全文搜索与消息筛选，以及转发、多选、复制为图片等消息工具
- 附件、图库、表情、世界术语、便签和音频工作台
- 聊天归档与图文导出，支持 HTML、DOCX 等跑团记录方式

### 线索与调查

- 世界级线索箱，支持分类管理、定向揭示、隐藏、重放和批量操作
- 世界协作与个人线索板，提供无限画布、关系连线、绘图与撤销能力
- 线索状态可实时同步，并可通过独立浮窗、小剧场组件等入口跨场景使用

### 小剧场与演出

- 可视化场景幕布，支持图片、形状、网页组件、网格、素材库和场景文件夹
- 场景叠加、预设、CCFOLIA 导入，以及可发布给玩家的场景浏览能力
- 角色立绘、对话演出、多人驻场、全局对话框与人物数据浮层
- 组件点击动作、随机表、线索联动与序列器编排，可组合自动演出流程
- 图片与网页组件可应用轻量视觉效果；支持标准、画中画和移动端小剧场

### AI 与语音

- 语病修正、战报总结与相关模型配置能力
- TTS 语音朗读支持阿里云、腾讯云等多服务商，支持个人音色、音色复刻、多语言朗读以及翻译朗读，自动播放与播放状态同步
- 平台可按世界控制 TTS 白名单、额度与用量，并将语音结果保存到本地或 S3

### 扩展、分屏与集成

- Channel Embed API：为频道开发具备受控能力的 iForm / iframe 应用
- Internal Surface：让人物卡、便签、线索和频道组件以浮窗、分屏或独立窗口运行，并可与小剧场流转
- SealChat Bridge 与 Webhook API：嵌入完整频道页，或与外部系统同步消息
- MCP：通过个人 API Key 和细粒度 scopes 向外部 Agent 提供受控数据读取、写入与综合搜索能力
- Bot、自动化以及频道摘要主动推送或拉取

### 自托管

- Docker Compose、Docker 或二进制发行包
- SQLite，以及 PostgreSQL、MySQL 数据库
- 本地文件或 S3 兼容对象存储，支持对象存储快速配置与状态检查
- SQLite 自动备份、S3 备份、聊天导出与存储迁移工具
- 版本检测与自动更新流程，并在更新前执行数据库备份

## 快速开始

在仓库目录中准备 Docker 配置并启动：

```bash
cp config.docker.yaml.example config.yaml
docker compose up -d
```

访问 [http://localhost:3212/](http://localhost:3212/)。全新数据库中的第一个注册用户会成为平台管理员，并创建默认世界。

> 生产部署不要只依赖这里的启动片段。持久化、安全、反向代理、升级和备份要求请以[部署指南](doc/deployment.md)与[配置指南](doc/configuration.md)为准。

## 文档

| 我想…… | 文档 |
| --- | --- |
| 部署或升级 SealChat | [部署指南](doc/deployment.md) |
| 修改配置、存储、SMTP 或 S3 | [配置指南](doc/configuration.md) |
| 备份、恢复或执行管理员操作 | [运维指南](doc/administration.md) |
| 从源码构建或参与开发 | [开发指南](doc/development.md) |
| 开发频道嵌入应用 | [Channel Embed API](doc/channel-embed-api-developer-guide.md) |
| 将 SealChat 嵌入其他页面 | [Bridge API](doc/sealchat-bridge-api.md) |
| 与外部系统同步消息 | [Webhook API](doc/sealchat-webhook-api.md) |
| 开发角色卡模板 | [角色卡模板开发](doc/character-sheet-template-development.md) |
| 配置频道摘要 | [频道未读提醒](doc/channel-digest-push.md) |
| 通过 MCP 接入 SealChat 数据与工具 | [平台 MCP 接入](doc/mcp.md) |
| 了解 TTS、模型与音色能力 | [TTS 能力目录](doc/tts-catalog.md) |
| 开发或排查隐式 BOT 调用 | [BOT Interaction](doc/bot-interaction.md) |
| 了解原生模块独立运行与浮窗架构 | [Internal Surface 架构](doc/Internal_surface_architecture.md) |

[查看完整文档索引](doc/README.md)

## 技术架构

```text
Browser
  ↓
Vue 3 / Vite
  ↓ HTTP + WebSocket
Go / Fiber
  ↓
Database + Local or S3-compatible Storage
```

前端构建产物和部分运行时文档会通过 `go:embed` 进入服务端发行包，从而以单个主程序提供 Web 界面和 API。详细构建方式见[开发指南](doc/development.md)。

## 项目状态

SealChat 仍在持续开发。升级实例前，请查看对应版本的 Release Notes，并备份数据库、配置和本地资源。

## 参与项目

欢迎通过 [Issues](https://github.com/kagangtuya-star/sealchat/issues) 报告问题或提出建议，也欢迎提交 Pull Request。开发环境、构建和检查命令见[开发指南](doc/development.md)。

## 致谢

感谢所有贡献者、测试者和社区用户。

爱发电赞助：

- tanis
- 爱发电用户_NTdp（不愿透露姓名的 xnn）

友情链接：[linux.do](https://linux.do/)
