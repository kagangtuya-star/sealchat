# SealChat 部署指南

本文说明如何安装、运行和升级 SealChat。配置字段与敏感信息管理见[配置指南](configuration.md)，备份与日常维护见[运维指南](administration.md)。

## 1. 部署方式选择

- **Docker Compose（推荐）**：使用仓库现有 `docker-compose.yml`，配置、数据和资源目录均可持久化。
- **Docker run**：适合不使用 Compose 的单容器环境，需要自行保持挂载和启动参数一致。
- **二进制发行包**：适合直接运行程序或注册系统服务。必须保留发行包内随附的运行时文件。
- **源码运行**：用于开发和自定义构建，不是本文的生产主路径，参见[开发指南](development.md)。

## 2. 系统和运行环境

Docker 路径需要 Docker Engine 与 Docker Compose，并开放默认端口 `3212`。当前镜像以 Alpine 为运行环境，内含 CA 证书、时区数据、`wget` 和 FFmpeg。

二进制路径应选择与操作系统和 CPU 架构匹配的 Release 资产。不要沿用旧部署文档中针对早期 Go 版本推导的老旧操作系统兼容范围；实际支持范围以对应 Release 说明和构建目标为准。

## 3. Docker Compose

### 准备并启动

在包含 `docker-compose.yml` 的仓库或发行目录中执行：

```bash
cp config.docker.yaml.example config.yaml
docker compose up -d
```

访问 `http://localhost:3212/`。全新数据库中的第一个注册用户会获得平台管理员角色，并创建默认世界。

### 状态、日志与停止

```bash
docker compose ps
docker compose logs -f sealchat
docker compose restart sealchat
docker compose down
```

Compose 配置包含 HTTP 健康检查，每 30 秒请求一次容器内的 `http://localhost:3212/`。

### 持久化目录

| 宿主机路径 | 容器路径 | 内容 |
| --- | --- | --- |
| `./data` | `/app/data` | 默认 SQLite 数据库、导出及其他运行数据 |
| `./sealchat-data` | `/app/sealchat-data` | 附件、音频等本地资源 |
| `./static` | `/app/static` | 静态资源和音频目录 |
| `./config.yaml` | `/app/config.yaml` | 主配置文件 |

当前 Compose 以 `0:0` 运行容器，主要用于避免宿主机挂载目录的写入权限问题。若改为非 root 用户，应先确保上述目录及配置文件对目标 UID/GID 可读写。

### 更新

更新前先按[运维指南](administration.md)备份，然后查看目标版本的 Release Notes：

```bash
docker compose pull
docker compose up -d
docker compose logs -f sealchat
```

## 4. Docker run

先将 `config.docker.yaml.example` 复制为 `config.yaml`，再运行：

```bash
docker run -d --name sealchat --restart unless-stopped \
  -u 0:0 \
  -p 3212:3212 \
  -v "$(pwd)/data:/app/data" \
  -v "$(pwd)/sealchat-data:/app/sealchat-data" \
  -v "$(pwd)/static:/app/static" \
  -v "$(pwd)/config.yaml:/app/config.yaml" \
  -e TZ=Asia/Shanghai \
  ghcr.io/kagangtuya-star/sealchat:latest
```

更新时停止并删除旧容器、拉取目标镜像，再用完全相同的挂载和端口参数重建。不要删除宿主机持久化目录。

## 5. 二进制部署

1. 从 [GitHub Releases](https://github.com/kagangtuya-star/sealchat/releases) 下载对应平台的发行包并完整解压。
2. 保留主程序旁的 `bin/`、`builtin/` 和示例配置等发行内容。启动时会检查 WebP 工具；发行目录不完整会导致启动失败。
3. 复制 `config.yaml.example` 为 `config.yaml` 并按需修改。若不提供，程序会尝试从数据库恢复配置；全新安装则生成默认配置。
4. 在发行目录运行 `./sealchat-server`；Windows 使用 `sealchat-server.exe`。
5. 访问 `http://localhost:3212/`。默认端口由 `serveAt: :3212` 决定。

程序需要对当前工作目录、数据目录和资源目录有写权限。Windows 可使用 `sealchat-server.exe --install` 注册系统服务，使用 `--uninstall` 停止并卸载；服务工作目录是执行安装命令时的当前目录。

## 6. 数据库

`dbUrl` 的格式决定数据库驱动：

- SQLite：默认 `./data/chat.db`；也接受 `file:` DSN。
- PostgreSQL：`postgres://...` 或 `postgresql://...`。
- MySQL / MariaDB：`mysql://...` 或包含 `@tcp(...)` 的 MySQL DSN。

SQLite 是默认且部署最简单的选择。使用外部数据库时，应在首次启动前创建数据库和权限受限的业务用户，并独立建立数据库备份方案。MySQL DSN 通常需要 `charset=utf8mb4&parseTime=True&loc=Local`。配置详情见[数据库配置](configuration.md#数据库配置)。

## 7. 反向代理与 HTTPS

反向代理必须转发普通 HTTP 请求和 WebSocket 升级头。根路径部署保持 `webUrl: /`；部署到 `/chat/` 一类子路径时，将 `webUrl` 配置为不带尾斜杠的 `/chat`，并确保代理的保留或剥离前缀策略与之匹配。

若要让日志、限流和快捷登录风控获得真实客户端 IP，请设置 `proxy.proxyHeader` 和 `proxy.trustedProxies`。只信任实际反向代理的地址或网段，不要使用 `0.0.0.0/0`。TLS 可在反向代理终止；示例配置中也提供面向公网 IP 的内置证书选项，启用前应确认挑战端口和发行版本说明。

## 8. NAS 与 Docker 挂载注意事项

群晖 Container Manager、Btrfs 子卷或多个独立宿主机挂载可能使临时目录和最终目录位于不同文件系统。此时 Linux `rename` 会返回 `invalid cross-device link`；SealChat 会降级为复制，但会增加 I/O。

建议让临时目录和目标目录位于同一挂载点，例如：

```yaml
storage:
  local:
    tempDir: ./sealchat-data/temp
audio:
  tempDir: ./static/audio-temp
```

同时确认容器用户对源目录、临时目录和目标目录都有读写权限。

## 9. 升级

1. 备份数据库、`config.yaml` 和所有本地资源；外部数据库与 S3 另按服务商方式备份。
2. 阅读目标版本 Release Notes，确认迁移或兼容要求。
3. Docker 部署拉取并重建容器；二进制部署完整替换发行包程序和随附运行时文件。
4. 启动后检查日志、健康状态、登录、消息发送及资源访问。
5. 确认新版本正常后再执行清理或存储迁移。

## 10. 下一步

- [配置数据库、存储、邮件和音频](configuration.md)
- [备份、恢复与管理员 CLI](administration.md)
