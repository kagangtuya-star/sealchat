# SealChat
<div align="center">

[![Live Demo](https://img.shields.io/badge/Live%20Demo-点击%E7%AB%8B%E5%8D%B3%E4%BD%93%E9%AA%8C-blueviolet?style=flat-square&logo=google-chrome&logoColor=white)](https://kagangtuya-sc.sealdice.com/)
[![QQ Group](https://img.shields.io/badge/QQ%E7%BE%A4-%E7%82%B9%E5%87%BB%E5%8A%A0%E5%85%A5-12B7F5?style=flat-square&logo=tencentqq&logoColor=white)](https://qm.qq.com/q/wL4lD8saIM)
<br/>
[![Total Downloads](https://img.shields.io/github/downloads/kagangtuya-star/sealchat/total?style=flat-square&color=brightgreen&label=Total%20Downloads)](https://github.com/kagangtuya-star/sealchat/releases)
[![Latest Release Downloads](https://img.shields.io/github/downloads/kagangtuya-star/sealchat/latest/total?style=flat-square&color=2ea44f&label=Latest%20Release)](https://github.com/kagangtuya-star/sealchat/releases/latest)
[![GitHub release (latest by date)](https://img.shields.io/github/v/release/kagangtuya-star/sealchat?style=flat-square&color=orange&label=Version)](https://github.com/kagangtuya-star/sealchat/releases)
[![License](https://img.shields.io/github/license/kagangtuya-star/sealchat?style=flat-square&color=blue)](https://github.com/kagangtuya-star/sealchat/blob/main/LICENSE)
[![Go](https://img.shields.io/badge/-Go-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![Vue.js 3](https://img.shields.io/badge/-Vue.js%203-4FC08D?style=flat-square&logo=vue.js&logoColor=white)](https://vuejs.org/)
[![Vite](https://img.shields.io/badge/-Vite-646CFF?style=flat-square&logo=vite&logoColor=white)](https://vitejs.dev/)
<br/>
[![GitHub Stars](https://img.shields.io/github/stars/kagangtuya-star/sealchat?style=social)](https://github.com/kagangtuya-star/sealchat/stargazers)
[![GitHub Forks](https://img.shields.io/github/forks/kagangtuya-star/sealchat?style=social)](https://github.com/kagangtuya-star/sealchat/network/members)
</div>

SealChat 是一款自托管的轻量即时通讯与角色协作平台，服务端使用 Go 1.22 开发，前端基于 Vue 3 + Vite。通过“世界 → 频道 → 消息”的结构以及细粒度权限控制，它既能满足跑团/同人/社区的沉浸式聊天场景，也能覆盖小型团队的内部沟通需求。
![PixPin_2025-12-07_00-01-43](https://github.com/user-attachments/assets/2530ed53-9e95-43eb-b3ef-ed6ed659f1e0)
![PixPin_2025-12-07_00-02-57](https://github.com/user-attachments/assets/47534f2c-6c39-4ce1-8c5d-f0fbeff4591f)


## 功能亮点
- **多层组织模型**：`service/world.go` 定义公开/私有世界、默认大厅、收藏夹；频道支持子层级、身份卡、嵌入窗 (iForm)。
- **灵活权限与身份**：`pm` 权限树结合频道身份 (`service/channel_identity.go`) 提供多角色扮演、主持/观众模式、Bot 权限。
- **丰富消息形态**：文本、附件、图库、音频素材库、骰子宏、悄悄话、OOC/IC 标记、全文检索、导出任务等功能均在 `api/*` 文件中实现。
- **资产与归档**：附件/图库 (`api/attachment.go`, `api/gallery.go`)、音频库 (`api/audio.go`)、导出 worker (`service/export_*.go`) 让素材沉淀、聊天备份和合规审计更加简单。
- **监控与自动化**：`service/metrics` + `/status` 页面输出运行指标，`api/admin_*` 管理用户与 Bot，兼容 Satori 协议扩展。

## 功能与操作指南
- **账号与访问控制**：参考 `docs/product-introduction.md` 第 4.1 节，覆盖注册/登录、系统角色、好友、Presence 的 UI 与 API 操作。
- **世界与频道治理**：同文档第 4.2-4.3 节描述如何创建世界、维护频道层级、身份/权限、iForm 及骰子宏设置。
- **消息与资产**：第 4.4-4.5 节梳理 WebSocket 流程、消息撤回、附件/图库/音频库上传与复用。
- **检索与归档**：第 4.6 节提供全文搜索、历史锚点、导出任务的详细流程与注意事项。
- **监控与自动化**：第 4.7-4.8 节总结 `/status` 看板、Presence/时间线，以及 Bot token、命令注册和自动化范式。

## 架构一览
- **服务端**：Go + Fiber + WebSocket，单一可执行文件内嵌 `ui/dist`，默认 SQLite (WAL) 也支持 PostgreSQL/MySQL。
- **前端**：`ui/` 目录使用 Vue 3、Naive UI、Tiptap、RxJS，开发期可独立运行 Vite 服务，构建后通过 `go:embed` 打包。
- **存储**：附件可存储在本地或 S3/兼容对象存储 (`service/storage`)，音频依赖可选 `ffmpeg`（转码）与 `ffprobe`（时长探测，缺失时回退 `ffmpeg -i` 解析），导出与音频的缓存位置均由 `config.yaml` 配置。

## 对象存储（S3 兼容）

SealChat 支持将**附件/图片**与**音频**分别存入 S3（或兼容协议的对象存储，如 MinIO、腾讯 COS 等），并提供“迁移到 S3”的管理端工具。

- 配置入口：`config.yaml` 的 `storage` 段（参考 `config.yaml.example` / `config.docker.yaml.example`）。
- 分类开关：`storage.s3.attachmentsEnabled`（附件/图片）、`storage.s3.audioEnabled`（音频）。
- 安全建议：建议通过环境变量配置 AK/SK（`SEALCHAT_S3_ACCESS_KEY`、`SEALCHAT_S3_SECRET_KEY`、`SEALCHAT_S3_SESSION_TOKEN`），避免把密钥写入配置文件。
- 启动自检：启用 S3 时会进行一次小文件 `put/get/delete` 自检，自检失败会回退本地并输出原因日志。
- 迁移工具：管理端“迁移到 S3”支持图片/音频分别迁移，建议先“模拟运行（dryRun）”。

更完整的 S3/COS 配置示例与常见问题请参考 `deploy_zh.md` 的“对象存储（S3 兼容）”章节。

## 未读信息邮件通知与邮箱登录认证

SealChat 的**未读信息邮件通知**与**邮箱认证（邮件注册验证/邮件密码重置）**共用 `config.yaml` 的 SMTP 配置。

- 配置入口：`emailNotification.smtp`（即使只启用邮箱认证，也需要完整 SMTP 配置）。
- 功能开关：`emailNotification.enabled` 控制未读信息提醒；`emailAuth.enabled` 控制邮件注册验证/密码重置。

配置示例与常见问题请参考 `deploy_zh.md` 的“未读信息邮件通知与邮箱登录认证”章节。
示例（节选自 `config.yaml.example`）：

## 快速开始

### Docker 部署（推荐）

```bash
# 1. 拉取最新镜像
docker pull ghcr.io/kagangtuya-star/sealchat:latest

# 2. 创建配置文件（推荐，便于持久化）
cp config.docker.yaml.example config.yaml

# 3. 使用 Docker Compose 启动
docker compose up -d

# 4. 访问 http://localhost:3212/ ，首个注册账号将成为管理员

# 更新到最新版本
docker compose pull && docker compose up -d
```

**或使用 docker run 一键启动：**

```bash
docker run -d --name sealchat --restart unless-stopped \
  -u 0:0 \
  -p 3212:3212 \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/sealchat-data:/app/sealchat-data \
  -v $(pwd)/static:/app/static \
  -v $(pwd)/config.yaml:/app/config.yaml \
  -e TZ=Asia/Shanghai \
  ghcr.io/kagangtuya-star/sealchat:latest
```

> 详细的 Docker 部署说明请参考 [`deploy_zh.md`](deploy_zh.md) 中的 Docker 部署章节。

### 二进制部署

1. 从发行页下载或 `go build ./...` 编译，运行 `./sealchat_server`（Windows 下为 `.exe`）。发行包内附 `config.yaml.example` 与 `config.docker.yaml.example`，可按需复制为 `config.yaml`。
2. 首次启动会生成 `config.yaml` 与 `data/` 目录，按照示例修改域名、端口、数据库、附件/音频/导出目录。
3. 浏览器访问 `http://<domain>:3212/`，注册首个账号（自动成为管理员并创建默认世界）。
4. 参考 [`docs/product-introduction.md`](docs/product-introduction.md) 或 `deploy_zh.md` 完成世界、频道、权限与资产配置。

### 从源码构建
- **先决条件**：Go >= 1.22，Node.js >= 18（建议搭配 pnpm 或 npm），`ffmpeg` 可选（转码），建议同时提供 `ffprobe` 用于更稳定的时长探测。
- **步骤**：
  1. `go mod download`
  2. `cd ui && npm install && npm run build`（或 `pnpm i && pnpm build`）
  3. 回到仓库根目录执行 `go build -o sealchat_server ./`
- **开发模式**：可运行 `npm run dev` 启动前端热更新，同时在根目录 `go run main.go`。

### 常用命令
- `go run main.go`：启动服务端并自动托管静态资源。
- `go test ./...`：执行后端单元测试（导出/骰子等模块含示例测试）。
- `./sealchat_server -i` / `./sealchat_server --uninstall`：在 Windows 上注册/卸载系统服务。

### 配置版本管理
SealChat 支持配置文件数据库持久化与版本历史管理，最多保留 10 个历史版本。

```bash
# 列出配置历史版本
./sealchat_server --config-list

# 查看指定版本配置详情（敏感字段已遮罩）
./sealchat_server --config-show 3

# 回滚到指定版本
./sealchat_server --config-rollback 2

# 导出指定版本为 YAML 文件
./sealchat_server --config-export 1 --output config.backup.yaml
```

配置同步逻辑：
- 配置文件存在时：读取并同步到数据库
- 配置文件丢失时：从数据库恢复并重建配置文件

### 用户密码重置（CLI）
可通过命令行直接读取当前配置对应数据库，列出平台管理员，或按用户名重置密码为 `123456`。

```bash
# 列出平台管理员用户
./sealchat_server --user-secret list

# 重置指定用户密码（支持平台管理员和普通用户）
./sealchat_server --user-secret reset --username alice --username bob

# 仅允许重置平台管理员，并跳过确认
./sealchat_server --user-secret reset --admin-only --username alice --yes
```

数据库连接优先级：
- `SEALCHAT_DSN` 环境变量
- `config.yaml` 中的 `dbUrl`
- 默认 `./data/chat.db`

## 目录导览
| 目录 | 说明 |
| --- | --- |
| `api/` | Fiber HTTP/WebSocket 接口、业务 RPC 封装 |
| `service/` | 世界、频道、附件、音频、导出、指标等业务逻辑 |
| `model/` | GORM 模型与数据访问层 |
| `pm/` | 权限模型与代码生成器 (`go generate ./pm/generator`) |
| `ui/` | Vue 3 前端工程与导出 Viewer 构建脚本 |
| `specs/` & `plans/` | 需求与实现规划文档 |
| `docs/` | 产品/部署等补充文档，新增的《产品介绍》位于 `docs/product-introduction.md` |
| `deploy_zh.md` | 官方部署指南（含数据库切换、系统兼容性） |

> 本项目仍处于持续迭代阶段（WIP），欢迎根据实际场景扩展世界/频道权限、Bot 能力与前端组件。
