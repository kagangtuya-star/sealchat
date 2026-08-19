# SealChat 运维指南

本文覆盖已部署实例的检查、管理员 CLI、备份恢复与日常排障。首次安装和升级流程见[部署指南](deployment.md)，配置字段见[配置指南](configuration.md)。下列 CLI 应在主程序工作目录执行；Docker 部署可通过 `docker exec sealchat /app/sealchat-server ...` 调用，并先确认容器名称。

## 服务运行检查

Docker 实例可使用：

```bash
docker compose ps
docker compose logs -f sealchat
```

Compose 健康检查访问容器内首页。登录后的状态接口还提供当前指标与历史数据。排障时重点确认：数据库连接成功、存储初始化结果、FFmpeg 检测、HTTP 监听地址、后台 worker 启动，以及是否出现持续回退或权限错误。

## 配置历史管理

SealChat 会将配置变化保存到数据库，并提供以下已实现参数：

```bash
# 列出历史版本
./sealchat-server --config-list

# 查看版本；常见敏感字段会被遮罩
./sealchat-server --config-show 3

# 交互确认后回滚，并写回 config.yaml
./sealchat-server --config-rollback 2

# 导出指定版本；不传 --output 时使用 config.v1.yaml 一类名称
./sealchat-server --config-export 1 --output config.backup.yaml
```

回滚会保存一条新的 `rollback` 来源版本。配置文件存在时启动会将其同步入历史；文件缺失时程序尝试从数据库当前配置恢复。CLI 数据库连接优先级为 `SEALCHAT_DSN`、`config.yaml` 中的 `dbUrl`、`./data/chat.db`。

配置导出文件可能含凭据，必须按生产密钥处理。查看命令的遮罩不代表导出内容已经脱敏。

## 数据库维护命令

```bash
# 整理 SQLite 空间；非 SQLite 数据库会拒绝执行
./sealchat-server --sqlite-vacuum

# 全量重建 SQLite 全文检索索引
./sealchat-server --sqlite-fts-rebuild

# 清理没有 active 引用的 webhook/digest 系统 Bot 历史数据
./sealchat-server --cleanup-webhook-bot-friends
```

运行前停止普通服务进程，确保 CLI 指向正确数据库并完成可恢复备份。`--cleanup-webhook-bot-friends` 会物理删除符合条件的系统 Bot 及其好友、私聊、成员和 token 等历史数据，当前命令没有交互确认；只有在核对输出范围和备份后才应使用。

## 用户密码重置

```bash
# 列出平台管理员
./sealchat-server --user-secret list

# 重置一个或多个指定用户
./sealchat-server --user-secret reset --username alice --username bob

# 仅允许目标为平台管理员
./sealchat-server --user-secret reset --admin-only --username alice

# 跳过交互确认
./sealchat-server --user-secret reset --username alice --yes
```

`reset` 至少需要一个可重复的 `--username`。当前实现会把目标密码重置为固定值 `123456`；这是临时恢复手段，执行后应立即通知用户登录并改成唯一的强密码。`--yes` 只适用于已核对目标的自动化维护。

## 数据备份

### SQLite

`backup` 配置可启动 SQLite 自动备份。内置备份 ZIP 包含 SQLite 主文件、存在时的 WAL/SHM 文件和 `config.yaml`，但**不包含**附件、音频、字体、静态资源或 S3 对象。备份路径和保留数量由配置决定；该功能不支持 PostgreSQL 或 MySQL。

完整备份至少应覆盖：

- SQLite 数据库，或外部 PostgreSQL/MySQL 的一致性备份。
- `config.yaml`。
- `storage.local.*`、`audio.*Dir`、`export.storageDir` 指向的本地数据。
- Compose 中持久化的 `data/`、`sealchat-data/` 和 `static/`。
- S3 桶中 SealChat 使用的对象；按服务商能力启用版本控制或快照。

备份应存放在实例目录之外，并定期验证可读取、可解压和可恢复。

## 数据恢复

1. 停止 SealChat，避免恢复过程中继续写入。
2. 保留当前故障现场的副本。
3. 恢复与同一时间点匹配的数据库、`config.yaml` 和本地资源；外部数据库及 S3 按各自工具恢复。
4. 检查目录所有者、权限、DSN 和对象存储凭据。
5. 启动服务并检查迁移日志、登录、频道历史、附件、音频和导出。

只恢复数据库而不恢复本地资源会留下失效的附件引用；只恢复资源而不恢复数据库则无法重建资源元数据。跨版本恢复前应查看 Release Notes。

## Docker 运维

```bash
docker compose logs -f sealchat
docker compose restart sealchat
docker compose pull
docker compose up -d
```

镜像更新前必须备份并阅读 Release Notes。容器是可替换的，数据应全部落在明确的宿主机挂载或外部服务中。不要在容器可写层保存唯一副本，也不要在 Docker 容器内使用进程内二进制更新。

## 存储迁移

管理端可将本地图片附件、音频和小剧场资源迁移到 S3。先执行 `dryRun`，核对对象数量、失败原因和目标 URL，再执行实际迁移。分类开关、回退策略和 COS/MinIO 注意事项见[配置指南](configuration.md#s3-兼容对象存储)。

## 常见排障

### 容器不健康或页面不可达

确认 `docker compose ps`、端口 `3212`、`serveAt` 和日志。反向代理场景还要检查 WebSocket 升级头、`webUrl` 子路径和受信代理配置。

### `permission denied`

确认运行用户对数据库、配置文件、上传、临时、音频、导出和备份目录均有权限。更改容器 UID/GID 后，需要同步调整宿主机目录所有权。

### `invalid cross-device link`

临时目录和目标目录位于不同挂载点。程序会尝试复制回退，但应把 `storage.local.tempDir`、`audio.tempDir` 调整到对应目标目录所在挂载点，以减少 I/O。

### S3 启动自检失败

检查 endpoint、桶名、region、path-style、TLS、凭据，以及对 `sealchat/_healthcheck/` 的 Put/Get/Delete 权限。服务可能已回退本地；先修复并确认日志，再迁移期间产生的本地文件。

### 音频不能转码

检查启动日志中的 FFmpeg/FFprobe 探测结果和 `audio.ffmpegPath`。Docker 镜像自带 FFmpeg；二进制部署需要自行提供。
