# SealChat 配置指南

SealChat 的主配置文件是仓库或发行目录中的 `config.yaml`。本文按配置领域解释推荐方式；完整字段和默认形态请直接查看仓库根目录的 [`config.yaml.example`](../config.yaml.example) 与 [`config.docker.yaml.example`](../config.docker.yaml.example)，不要从本文复制一份长期不更新的完整配置。

## 配置文件

- `config.yaml`：程序实际读取和写回的配置。
- [`config.yaml.example`](../config.yaml.example)：二进制和本地运行的完整示例。
- [`config.docker.yaml.example`](../config.docker.yaml.example)：与仓库 Compose 挂载目录对齐的示例，默认关闭容器内自更新。

配置文件存在时，启动过程读取它并将变化保存到配置历史。配置文件不存在时，程序先尝试从数据库中的当前配置恢复并重建文件；数据库也没有记录时才生成默认配置。配置 YAML 无法解析时程序会退出，而不是静默采用默认值。

修改前先备份配置。生产环境建议通过管理界面或一次明确的文件变更完成修改，重启后检查日志和实际生效值。

## 环境变量与优先级

SealChat 只对少数敏感值提供环境变量覆盖，不支持把任意 YAML 键自动映射为环境变量：

| 环境变量 | 用途 |
| --- | --- |
| `SEALCHAT_S3_ACCESS_KEY` | 覆盖 S3 Access Key |
| `SEALCHAT_S3_SECRET_KEY` | 覆盖 S3 Secret Key |
| `SEALCHAT_S3_SESSION_TOKEN` | 覆盖可选的 S3 Session Token |
| `SEALCHAT_SMTP_PASSWORD` | 覆盖 SMTP 密码 |
| `SEALCHAT_GITHUB_TOKEN` | 覆盖版本检测使用的 GitHub Token |
| `SEALCHAT_DSN` | 配置恢复引导与运维 CLI 的数据库连接 |

正常启动且 `config.yaml` 存在时，业务数据库由文件中的 `dbUrl` 决定。配置文件缺失时，`SEALCHAT_DSN` 用于定位可能保存了配置历史的数据库，否则回退到 `./data/chat.db`。运维 CLI 的连接优先级是 `SEALCHAT_DSN`、`config.yaml` 的 `dbUrl`、默认 SQLite。

## 数据库配置

默认配置使用：

```yaml
dbUrl: ./data/chat.db
```

代码当前支持三类 DSN：

- SQLite：以 `.db` 结尾、`file:` 开头或内存 DSN。
- PostgreSQL：以 `postgres://` 或 `postgresql://` 开头。
- MySQL / MariaDB：以 `mysql://` 开头或包含 `@tcp(...)`。

SQLite 的 WAL、连接数、超时和整理策略位于 `sqlite` 段。外部数据库的账号应只拥有 SealChat 所需数据库权限，不要使用数据库超级用户。更换数据库不是简单改写 DSN 的数据迁移机制；迁移既有实例前应另行制定导出、验证和回滚方案。

## 本地文件存储

默认 `storage.mode` 为 `local`。常用目录包括：

- `storage.local.uploadDir`：附件和图片。
- `storage.local.audioDir`：存储管理器使用的音频目录。
- `storage.local.fontDir`：平台字体原文件、分片和 manifest。
- `storage.local.tempDir`：上传临时目录。
- `audio.storageDir`、`audio.tempDir`、`audio.importDir`：音频素材、转码和目录导入。
- `export.storageDir`：聊天及小剧场导出产物。

程序会创建必要目录。容器或系统服务用户必须拥有读写权限；在 NAS 和多挂载环境中，临时目录与最终目录应尽量放在同一挂载点，详见[部署指南](deployment.md#8-nas-与-docker-挂载注意事项)。

## S3 兼容对象存储

S3 配置位于 `storage.s3`。当前分类开关包括：

- `attachmentsEnabled`：附件与图片。
- `audioEnabled`：音频。
- `fontsEnabled`：平台字体原文件、分片与 manifest。
- `theaterEnabled`：小剧场视觉与音频资源；省略时分别继承附件和音频开关。

关键连接字段是 `enabled`、`endpoint`、`region`、`bucket`、`pathStyle`、`useSSL`、`publicBaseUrl`，以及凭据。`storage.mode` 可取 `local`、`s3` 或 `auto`；只有 S3 初始化成功且对应分类开关启用时，该分类才使用 S3。

凭据建议留空并使用 `SEALCHAT_S3_ACCESS_KEY`、`SEALCHAT_S3_SECRET_KEY` 和可选的 `SEALCHAT_S3_SESSION_TOKEN`。启用 S3 后，启动过程会在 `sealchat/_healthcheck/` 下执行一次小文件的 `put/get/read/delete` 自检。初始化或单次上传失败时会记录日志并回退本地，因此应监控日志和本地磁盘，不能把“服务已启动”等同于“S3 正常”。

MinIO 等服务可能需要 `pathStyle: true`。腾讯 COS 通常要求 virtual-host style，此时使用区域根 endpoint、包含 APPID 后缀的完整桶名并设置 `pathStyle: false`。`publicBaseUrl` 应与真实访问方式一致：自定义域名需要桶路径时将桶路径包含在该值中。

管理端提供按图片附件、音频和小剧场资源迁移到 S3 的能力。先使用模拟运行（`dryRun`）检查范围和错误，再决定是否删除源文件；迁移数据不会自动替你修改分类开关。执行前同时备份数据库和本地资源。

## 邮件

邮箱注册验证、邮箱绑定和邮件密码重置由 `emailAuth.enabled` 控制，并复用 `emailNotification.smtp`：

```yaml
emailNotification:
  smtp:
    host: smtp.example.com
    port: 587
    username: your@example.com
    password: ""
    fromAddress: noreply@example.com
    fromName: SealChat
    useTLS: true
    skipVerify: false
emailAuth:
  enabled: false
```

即使只启用邮箱认证，也必须配置 SMTP 的 `host`、端口和 `fromAddress`；需要认证时再提供用户名和密码。生产环境使用 `SEALCHAT_SMTP_PASSWORD`，保持证书校验开启。

`emailNotification.enabled` 属于旧未读邮件提醒配置。当前启动流程不再默认启动旧提醒 worker，而是启动频道摘要链路；摘要的主动推送和被动拉取方式见[频道未读提醒](channel-digest-push.md)。不要把旧开关当作频道摘要开关。

## 音频、FFmpeg 与 FFprobe

`audio.enableTranscode` 控制转码能力，`audio.ffmpegPath` 可指定 FFmpeg；留空时程序按运行环境探测。FFprobe 与 FFmpeg 同目录时会用于时长探测，缺失时程序尝试通过 FFmpeg 回退解析。没有 FFmpeg 时服务仍可启动，但音频工作台的转码不可用。

Docker 镜像已安装 FFmpeg。二进制部署若需要转码，应自行准备与平台匹配的 FFmpeg/FFprobe，并在启动日志中确认检测结果。`audio.tempDir` 应有足够空间，且最好与最终音频目录位于同一挂载点。

## 字体资源

平台字体由 `storage.local.fontDir` 或 S3 的 `fontsEnabled` 分类保存。普通字体访问和全局 UI 字体不要求安装字体分割运行时；只有管理员执行“分割并发布”时才需要 `bin/cn-font-split/`。运行时准备和版本对应方式见[开发指南](development.md#平台字体分割器)。

## 配置安全

- 不要提交 `config.yaml`、数据库密码、SMTP 密码或云存储 AK/SK。
- 优先使用上表支持的环境变量保存敏感值，并限制配置、备份和容器环境文件的读取权限。
- 不要在问题报告、日志粘贴或配置历史导出中泄露 token；CLI 的查看命令会遮罩常见敏感字段，但导出文件仍应按敏感配置处理。
- 生产变更前备份配置与数据库。对象存储和外部数据库还应启用服务商侧版本、快照或备份策略。
