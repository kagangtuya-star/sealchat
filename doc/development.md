# SealChat 开发指南

本文面向从源码构建、调试或贡献 SealChat 的开发者。生产安装请使用[部署指南](deployment.md)。

## 技术栈

- 后端：Go、Fiber、GORM、HTTP 与 WebSocket。
- 前端：Vue 3、Vite、TypeScript、Naive UI、Pinia 和 Tiptap。
- 数据库：SQLite、PostgreSQL、MySQL / MariaDB。
- 存储：本地文件与 S3 兼容对象存储。

依赖的精确版本由 `go.mod`、`go.sum`、`ui/package.json` 和 `ui/package-lock.json` 管理，不在本文重复维护。

## 环境要求

- `go.mod` 当前声明 `go 1.24.0`，本地构建应使用 Go 1.24 工具链或更高的兼容版本。
- 前端没有在 `package.json` 中声明 `engines` 最低版本。仓库 Docker 构建固定使用 Node.js 20，因此本地开发推荐 Node.js 20；不要从 `@types/node` 的版本推断运行时最低版本。
- 仓库提交了 `ui/package-lock.json`，以下命令以 npm 为准。
- SQLite 本地驱动为纯 Go 实现，常规构建不依赖 CGO。音频转码开发需要 FFmpeg，建议同时提供 FFprobe。

## 获取代码

```bash
git clone https://github.com/kagangtuya-star/sealchat.git
cd sealchat
```

切换到你要开发的分支，并先阅读仓库根目录及目标子目录的 `AGENTS.md`（若存在）。不要提交本地 `config.yaml`、数据库、上传资源或密钥。

## 后端开发

首次准备依赖：

```bash
go mod download
```

前端产物已经存在于 `ui/dist` 时，可从仓库根目录运行：

```bash
go run .
```

服务端生产/常规运行默认监听 `:3212`。使用前端 Vite 开发模式联调时，`13211` 保留给前端开发服务器，建议将本地后端 `serveAt` 改为 `:13212`；这只是开发约定，不改变服务端默认配置。后端入口由同一 `main` 包中的多个文件共同组成，因此不要使用只编译单个文件的 `go run main.go` 代替 `go run .`。

权限代码需要重新生成时使用仓库声明的生成入口：

```bash
go generate ./...
```

仅在改动确实涉及生成输入时执行，并检查生成 diff。

## 前端开发

```bash
cd ui
npm ci
npm run dev
```

`npm run dev` 启动 Vite 开发服务并固定监听 `13211`，用于保留开发浏览器的同一 origin 与本地设置。开发环境下前端 API、主 WebSocket 以及 OneBot 路径分别通过 `/api`、`/ws`、`/onebot` 代理到后端，默认目标为 `http://127.0.0.1:13212`；前端业务代码仍只访问当前 Vite origin，不直接绑定后端端口。

默认联调时，将本地后端 `serveAt` 配置为 `:13212`，然后在另一个终端从仓库根目录运行 `go run .`。如果后端运行在其他地址或端口，可通过环境变量 `SEALCHAT_DEV_BACKEND` 覆盖代理目标，例如设置为 `http://127.0.0.1:3212`；无需修改前端源码。

常用前端命令：

```bash
npm run type-check
npm run build
npm run build-export-viewer
```

## 完整构建

先生成 `ui/dist`，再构建 Go 主程序：

```bash
go mod download
cd ui
npm ci
npm run build
cd ..
go build -o sealchat-server .
```

发行和容器构建还需要相应平台的 `bin/<platform>/cwebp`、`gif2webp` 及其他随包运行时资源。正式发行应使用仓库现有构建或发布流程，不能只分发裸主程序。

仓库 `Dockerfile` 使用 Node.js 20 构建前端、Go 1.24 构建无 CGO 服务端，并在最终 Alpine 镜像中加入 WebP 工具和 FFmpeg。

## 开发模式

推荐使用前后端分离的本地联调方式。先将本地后端监听地址设为 `:13212`，终端 1 在仓库根目录运行：

```bash
go run .
```

终端 2 运行：

```bash
cd ui && npm run dev
```

浏览器继续访问原有开发地址 `http://<开发主机>:13211/`。Vite 负责 Vue、TypeScript 与样式的 HMR，并将 `/api`、`/ws`、`/onebot` 转发到开发后端，因此前端修改不再需要反复执行 `npm run build`。如果需要保留已有 LocalStorage、IndexedDB 等浏览器数据，除了端口保持 `13211` 外，也应继续使用原来的 hostname，例如不要在 `127.0.0.1` 与 `localhost` 之间随意切换。

若开发后端不是默认的 `http://127.0.0.1:13212`，启动 Vite 前设置 `SEALCHAT_DEV_BACKEND` 即可覆盖代理目标。代理只覆盖前端实际依赖的后端入口，不使用全路径代理，以免干扰 Vite 自身的页面、模块和 HMR 请求。

若只验证服务端内嵌页面，先运行 `npm run build` 更新 `ui/dist`，再重启 `go run .`。Go 的嵌入内容在编译时确定，不会随磁盘上的 `ui/dist` 热更新。

## 测试与检查

按改动范围从小到大执行：

```bash
go test ./...

cd ui
npm run type-check
npm run build
```

文档修改至少运行 `git diff --check`，并检查 Markdown 相对链接。仓库未声明独立的通用 Markdown lint 脚本。

## 单体发行结构

`main.go` 通过 `//go:embed ui/dist` 将前端产物嵌入主程序。Channel Embed SDK、频道内置工具、导出 Viewer 资源和 `doc/SEALCHAT_AGENT_CRAWL_GUIDE_COMPACT.md` 也分别由对应 Go 文件嵌入。

单体主程序并不意味着所有外部工具都已内嵌：WebP 编码器、可选字体分割 WASM、FFmpeg/FFprobe 和配置仍可能需要作为发行目录的一部分保留。`builtin/channel-embed-tools/` 可作为开发或定制部署时的外部覆盖，不再是内置频道工具运行前提。

## 平台字体分割器

平台字体分割器只用于管理员在“平台管理 → 主题与样式管理”执行“分割并发布”。普通访问、富文本字体和全局 UI 字体不依赖它；不开发或发布字体分片时无需安装。

运行时目录：

```text
sealchat-server(.exe)
bin/
  cn-font-split/
    libffi-wasm32-wasip1.wasm
    version
```

准备方式：

1. 从 `cn-font-split` 上游 Release 获取 `libffi-wasm32-wasip1.wasm`。
2. 将文件放入主程序工作目录或可执行文件目录下的 `bin/cn-font-split/`。
3. 创建文本文件 `version`，内容为 `wasm32-wasip1@<前端实际安装版本>`。
4. 在管理页面点击“检测分割器”。前端会检查版本文件和 WASM，再按需创建 Web Worker。

当前 `ui/package.json` 声明 `cn-font-split` 为 `^7.4.1`，锁文件当前解析为 `7.4.1`，因此与当前锁文件构建对应的内容是：

```text
wasm32-wasip1@7.4.1
```

依赖更新后应重新从 `ui/package-lock.json` 确认实际版本，不能长期照抄本文数值。`version` 用于能力探测和展示；JS 包与 WASM 仍应采用同一上游版本，避免不兼容。

## 系统服务

主程序实现了 `--install`（短参数 `-i`）和 `--uninstall`。Windows 二进制部署方式见[部署指南](deployment.md#5-二进制部署)。开发时调用安装命令会修改系统服务状态，普通调试应直接运行进程。

## 项目目录

| 路径 | 作用 |
| --- | --- |
| `api/` | Fiber HTTP、WebSocket 路由与处理器 |
| `service/` | 世界、频道、消息、存储、音频、导出等业务逻辑 |
| `model/` | GORM 模型、数据库初始化和数据访问 |
| `pm/` | 权限模型与生成器 |
| `ui/` | Vue 3 前端、导出 Viewer 与前端脚本 |
| `cmd/` | 可独立运行的辅助程序 |
| `utils/` | 配置及跨模块基础工具 |
| `bin/` | 随发行包使用的平台工具与资源 |
| `builtin/` | 内置模板和运行时内容源码（频道工具会嵌入主程序） |
| `doc/` | 用户指南、外部 API 和专题设计文档 |
| `specs/` | 需求、协议和实现规格记录 |
| `plans/` | 阶段性实施计划 |
| `scripts/` | 构建、校验或维护脚本 |

文档根目录是 `doc/`；不要创建并行的 `docs/` 入口。

## 文档维护规则

- `README.md`：项目首页，只保留定位、能力、截图、最短启动和导航。
- `doc/deployment.md`：安装、运行、反向代理和升级。
- `doc/configuration.md`：配置领域、存储、邮件和安全。
- `doc/administration.md`：管理员 CLI、备份恢复和排障。
- `doc/development.md`：开发环境、构建、测试和发行结构。
- `doc/*.md`：特定功能、公开协议或带状态说明的设计专题。

功能变化时修改唯一权威文档，并从其他入口链接过去；不要把易失真的字段说明同时复制到 README、部署和配置文档。
