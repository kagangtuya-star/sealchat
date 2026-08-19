# SealChat

<div align="center">

面向 TRPG、文字跑团与角色协作场景的自托管实时聊天平台

[在线体验](https://kagangtuya-sc.sealdice.com/) · [Releases](https://github.com/kagangtuya-star/sealchat/releases) · [文档](doc/README.md) · [QQ 群](https://qm.qq.com/q/wL4lD8saIM)

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

- 世界与多级频道，支持公开、私有和成员权限管理
- 频道身份、角色切换、共享身份与角色外观
- 主持、玩家、观众和 Bot 等协作边界
- 角色卡、世界资料与频道级嵌入应用

### 跑团与聊天

- WebSocket 实时聊天、IC/OOC、悄悄话和消息历史
- 骰子指令、骰子宏与角色化发言
- 全文搜索、附件、图库、音频素材库和表情
- 聊天导出、归档与跑团素材沉淀

### 扩展与集成

- Channel Embed API：为频道开发 iForm / iframe 应用
- SealChat Bridge：将完整频道页嵌入外部页面
- Webhook API：与外部系统双向同步消息
- Bot、自动化以及频道摘要主动推送或拉取

### 自托管

- Docker Compose、Docker 或二进制发行包
- SQLite，以及 PostgreSQL、MySQL 数据库
- 本地文件或 S3 兼容对象存储
- SQLite 自动备份、聊天导出与存储迁移工具

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
