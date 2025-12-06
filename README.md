# SealChat

SealChat 是一款自托管的轻量即时通讯与角色协作平台，服务端使用 Go 1.22 开发，前端基于 Vue 3 + Vite。通过“世界 → 频道 → 消息”的结构以及细粒度权限控制，它既能满足跑团/同人/社区的沉浸式聊天场景，也能覆盖小型团队的内部沟通需求。
![PixPin_2025-12-02_01-38-21](https://github.com/user-attachments/assets/a7ad086a-12c7-4e87-b8c6-20e93247ac42)

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
- **存储**：附件可存储在本地或 S3/兼容对象存储 (`service/storage`)，音频依赖可选 `ffmpeg`/`ffprobe`，导出与音频的缓存位置均由 `config.yaml` 配置。

## 快速开始

### Docker 部署（推荐）

```bash
# 1. 拉取最新镜像
docker pull ghcr.io/kagangtuya-star/sealchat:latest

# 2. 创建配置文件 (可选，首次运行会自动生成)
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
  -e TZ=Asia/Shanghai \
  ghcr.io/kagangtuya-star/sealchat:latest
```

> 详细的 Docker 部署说明请参考 [`deploy_zh.md`](deploy_zh.md) 中的 Docker 部署章节。

### 二进制部署

1. 从发行页下载或 `go build ./...` 编译，运行 `./sealchat_server`（Windows 下为 `.exe`）。
2. 首次启动会生成 `config.yaml` 与 `data/` 目录，按照示例修改域名、端口、数据库、附件/音频/导出目录。
3. 浏览器访问 `http://<domain>:3212/`，注册首个账号（自动成为管理员并创建默认世界）。
4. 参考 [`docs/product-introduction.md`](docs/product-introduction.md) 或 `deploy_zh.md` 完成世界、频道、权限与资产配置。

### 从源码构建
- **先决条件**：Go >= 1.22，Node.js >= 18（建议搭配 pnpm 或 npm），`ffmpeg/ffprobe` 可选。
- **步骤**：
  1. `go mod download`
  2. `cd ui && npm install && npm run build`（或 `pnpm i && pnpm build`）
  3. 回到仓库根目录执行 `go build -o sealchat_server ./`
- **开发模式**：可运行 `npm run dev` 启动前端热更新，同时在根目录 `go run main.go`。

### 常用命令
- `go run main.go`：启动服务端并自动托管静态资源。
- `go test ./...`：执行后端单元测试（导出/骰子等模块含示例测试）。
- `./sealchat_server -i` / `./sealchat_server --uninstall`：在 Windows 上注册/卸载系统服务。

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
