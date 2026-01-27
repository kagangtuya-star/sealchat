
# SealChat 部署指南

## 0. 系统兼容性

SealChat 推荐使用以下操作系统：

- Windows 10 及以上版本（64位）
- Windows Server 2016 及以上版本（64位）
- Linux（64位，推荐使用 Ubuntu 20.04 或更高版本）
- macOS 10.15 及以上版本

注意：由于使用 Go 1.22 进行开发，因此无法在 Windows Server 2012 / Windows 8.1 上运行。

未来可能会将 Windows 的最低支持版本降低至 Windows Server 2012。这意味着 SealChat 可能会在以下额外的 Windows 版本上运行：

- Windows 8.1（64位）
- Windows Server 2012 R2（64位）


此外，SealChat 在主流 Linux 环境上的兼容性如下：

- Ubuntu 9.04 及更高版本(经过完全测试，9.04到24.04)
- Debian 6 及更高版本(7.0实测可用)
- CentOS 6.0 及更高版本(7.9实测可用)
- Rocky Linux 8 及更高版本(Rocky 8实测可用)
- openSUSE 11.2 及更高版本(未测试)
- Arch Linux (未测试，理论2009年1月以后的版本都可用)
- Linux Mint 7 及更高版本 (未测试)
- OpenWRT 8.09.1 及更高版本(23.05 amd64实测可用)

经过群友 洛拉娜·奥蕾莉娅 闲着没事测了一整晚的结果，确认最低Ubuntu 9.04，也就是至少需要内核版本为2.6.28的Linux，才能运行。

如果使用魔改版的Linux，理论低于2.6.28几个版本的内核可能也能够正常运行，只需要该内核拥有完整实现的epoll支持，和accept4等accept调用的扩展。

虽然SealChat能够兼容很旧的操作系统，但还是建议使用较新的操作系统版本以确保最佳兼容性和性能。

## 0.5 Docker 部署（推荐）

使用 Docker 部署是最简单快捷的方式，无需安装任何依赖。

### 前置条件

- 安装 [Docker](https://docs.docker.com/get-docker/) 和 Docker Compose
- 确保 3212 端口可用

### 快速开始

```bash
# 拉取最新镜像
docker pull ghcr.io/kagangtuya-star/sealchat:latest

# 创建配置文件（推荐，便于持久化）
cp config.docker.yaml.example config.yaml

# 启动服务
docker compose up -d

# 查看日志
docker compose logs -f sealchat
```

访问 `http://localhost:3212/`，第一个注册的账号会成为管理员。

### 使用 docker run 一键启动

如果不使用 Docker Compose，可以直接使用以下命令启动：

> **提示**：程序会自动创建所需的数据目录，无需手动创建。

**Linux / macOS:**

```bash
docker run -d --name sealchat --restart unless-stopped \
  -u 0:0 \
  -p 3212:3212 \
  -v $(pwd)/sealchat/data:/app/data \
  -v $(pwd)/sealchat/sealchat-data:/app/sealchat-data \
  -v $(pwd)/sealchat/static:/app/static \
  -v $(pwd)/sealchat/config.yaml:/app/config.yaml \
  -e TZ=Asia/Shanghai \
  ghcr.io/kagangtuya-star/sealchat:latest
```

参数说明：
- `-d` 后台运行
- `--name sealchat` 容器名称
- `--restart unless-stopped` 自动重启
- `-u 0:0` 以 root 用户运行（解决目录权限问题）
- `-p 3212:3212` 端口映射
- `-v` 数据持久化挂载
- `-e TZ=Asia/Shanghai` 时区设置

更新镜像时需要先停止并删除旧容器：

```bash
docker stop sealchat && docker rm sealchat
docker pull ghcr.io/kagangtuya-star/sealchat:latest
# 然后重新执行上面的 docker run 命令
```

### 更新镜像

```bash
# 拉取最新镜像并重启
docker compose pull && docker compose up -d
```

### 数据持久化

Docker Compose 配置默认挂载以下目录以实现数据持久化：

| 容器路径 | 宿主机路径 | 说明 |
| --- | --- | --- |
| `/app/data` | `./data` | 数据库文件、临时文件、导出任务 |
| `/app/sealchat-data` | `./sealchat-data` | 上传的附件和音频文件 |
| `/app/static` | `./static` | 静态资源 |
| `/app/config.yaml` | `./config.yaml` | 配置文件 |

### 使用 PostgreSQL (生产环境推荐)

对于生产环境，推荐使用 PostgreSQL 数据库：

```bash
# 1. 创建 .env 文件设置数据库密码
echo "POSTGRES_PASSWORD=your_secure_password" > .env

# 2. 修改 config.yaml 中的数据库连接
# dbUrl: postgresql://sealchat:your_secure_password@postgres:5432/sealchat

# 3. 使用生产配置启动
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

### PostgreSQL 数据库备份

```bash
# 备份
docker exec sealchat-postgres pg_dump -U sealchat sealchat > backup_$(date +%Y%m%d).sql

# 恢复
cat backup.sql | docker exec -i sealchat-postgres psql -U sealchat sealchat
```

### 常用命令

```bash
# 启动服务
docker compose up -d

# 停止服务
docker compose down

# 重启服务
docker compose restart

# 查看日志
docker compose logs -f sealchat

# 查看服务状态
docker compose ps

# 进入容器
docker exec -it sealchat sh
```

## 1. 下载最新开发版本

1. 访问 SealChat 的 GitHub 发布页面：https://github.com/sealdice/sealchat/releases/tag/dev-release
2. 下载最新的开发版本压缩包

## 2. 解压文件

将下载的压缩包解压到您选择的目录中。

Linux下压缩包为.tar.gz格式，使用 `tar -xzvf xxx.tar.gz` 命令进行解压。

Windows下为zip格式。

### 主程序

主程序文件名通常为 `sealchat-server`（自行编译时也可命名为 `sealchat_server`）。根据您的操作系统，可能会有不同的扩展名：
- Windows: sealchat-server.exe
- Linux/macOS: sealchat-server

同时发行包会包含 `bin/<平台目录>/cwebp` 与 `bin/<平台目录>/gif2webp`（以及 `LICENSE`），用于图片压缩/转换；请保持它们与主程序同目录，不要删掉或改名。
发行包还会包含 `config.yaml.example` 与 `config.docker.yaml.example`，按需复制为 `config.yaml` 后再修改。


## 3. 运行程序

根据您的操作系统，按照以下步骤运行程序：

### Windows

直接双击 `sealchat-server.exe` 文件来运行程序。

打开浏览器，访问 http://localhost:3212/ 即可使用，第一个注册的帐号会成为管理员账号。

### Linux

1. 打开终端
2. 使用 `cd` 命令导航到解压缩的目录，例如：
   ```
   cd /path/to/sealchat
   ```
3. 给予执行权限（如果尚未授予）：
   ```
   chmod +x sealchat-server
   ```
4. 运行以下命令：
   ```
   ./sealchat-server
   ```

注意：首次运行时，程序会自动创建配置文件并初始化数据库。请确保程序有足够的权限在当前目录下创建文件。

如果您看到类似"Server listening at :xxx"的消息，说明程序已成功启动。

打开浏览器，访问 http://localhost:3212/ 即可使用，第一个注册的帐号会成为管理员账号。


## 进阶：使用 PostgreSQL 或 MySQL 作为数据库

SealChat 默认使用 SQLite 作为数据库，这使得它可以双击部署，一键运行。

数据库 SQLite 非常稳定、迁移方便且性能优秀，能够满足绝大部分场景的需求。

不过，如果你想使用其他数据库，我们也对 postgresql 和 mysql 提供了支持

### 配置文件

主程序首次运行时会自动生成 config.yaml 配置文件，我们主要关心dbUrl这一项：

```yaml
dbUrl: ./data/chat.db
```

这就是默认的数据库路径。


### PostgreSQL 配置

对于PostgreSQL环境，请按以下步骤配置：

1. 确保您已安装并启动PostgreSQL服务。

2. 使用PostgreSQL客户端或管理工具，执行以下SQL命令来创建数据库和用户：

   这里创建了数据库 sealchat，用户 seal 密码为 123，请注意在正式使用前，务必修改此密码。

   ```sql
   CREATE DATABASE sealchat;
   CREATE USER seal WITH PASSWORD '123';
   GRANT ALL PRIVILEGES ON DATABASE sealchat TO seal;
   \c sealchat
   GRANT CREATE ON SCHEMA public TO seal;
   ```

3. 在`config.yaml`文件中，设置`dbUrl`如下：

   ```yaml
   dbUrl: postgresql://seal:123@localhost:5432/sealchat
   ```

   请根据实际情况调整用户名、密码和主机地址。

4. 保存`config.yaml`文件，重新启动主程序。

注意：请确保PostgreSQL服务器已启动，并且配置的用户有足够的权限访问和操作sealchat数据库。


### MySQL / MariaDB 配置

对于MySQL/MariaDB环境，请按以下步骤配置：

1. 确保您已安装并启动MySQL服务。

2. 使用MySQL客户端或管理工具，执行以下SQL命令来创建数据库和用户：

这里创建了数据库 sealchat，用户 seal 密码为 123，请注意在正式使用前，务必修改此密码。

  ```sql
  CREATE DATABASE sealchat;
  CREATE USER 'seal'@'localhost' IDENTIFIED BY '123';
  GRANT ALL PRIVILEGES ON sealchat.* TO 'seal'@'localhost';
  FLUSH PRIVILEGES;
  ```

3. 在`config.yaml`文件中，设置`dbUrl`如下：

   ```yaml
   dbUrl: seal:123@tcp(localhost:3306)/sealchat?charset=utf8mb4&parseTime=True&loc=Local
   ```

   请根据实际情况调整用户名、密码和主机地址。

   这里的 charset parseTime loc 参数较为关键，不可省略。

4. 保存`config.yaml`文件，重新启动主程序

注意：请确保MySQL服务器已启动，并且配置的用户有足够的权限访问和操作sealchat数据库。

## 一份配置文件示例

```yaml
# 主页
domain: 127.0.0.1:3212
# 是否压缩图片
imageCompress: true
# 压缩质量(1-100，越低压缩越狠)
imageCompressQuality: 85
# 图片上传大小限制
imageSizeLimit: 99999999
# 注册是否开放
registerOpen: true
# 提供服务端口
serveAt: :3212
# 前端子路径
webUrl: /
# 启用小海豹
builtInSealBotEnable: true
# 历史保留时限，用户能看到多少天前的聊天记录，默认为-1(永久)，未实装
chatHistoryPersistentDays: -1
# 数据库地址，默认为 ./data/chat.db
dbUrl: postgresql://seal:123@localhost:5432/sealchat
```

### IPv6 配置示例

```yaml
serveAt: "[::]:3212"
domain: "[2001:db8::1]:3212"
```

注意：IPv6 地址必须使用中括号，否则会解析失败；服务地址保存时会自动补全中括号。

### 邮箱相关配置（邮件通知 / 邮箱认证）

SealChat 的邮件通知与邮箱认证共用同一套 SMTP 配置，位置在 `emailNotification.smtp`。即使只启用邮箱认证，也需要完整填写 SMTP 字段。

关键字段与开关（节选自 `config.yaml.example`）：

```yaml
emailNotification:
  enabled: false              # 未读邮件提醒开关
  smtp:
    host: smtp.example.com
    port: 587
    username: your@email.com
    password: ""              # 留空，改用环境变量 SEALCHAT_SMTP_PASSWORD
    fromAddress: noreply@example.com
    fromName: SealChat
    useTLS: true
    skipVerify: false

emailAuth:
  enabled: true               # 注册验证 / 密码重置开关
  codeLength: 6
  codeTTLSeconds: 300
  maxAttempts: 5
  rateLimitPerIP: 5
```

说明：
- 只需要邮箱认证时，可保持 `emailNotification.enabled: false`，但 SMTP 仍需配置。
- 邮件通知与邮箱认证的验证码发送频率、有效期等由 `emailAuth` 段控制。

## 4. 对象存储（S3 兼容）

SealChat 支持将附件/图片与音频存入 S3（或兼容协议的对象存储，如 MinIO、腾讯 COS 等）。相关配置在 `config.yaml` 的 `storage` 段，建议直接参考并复制 `config.yaml.example` / `config.docker.yaml.example` 中的示例，再按实际替换。

### 4.1 存储模式与目录

- `storage.mode`
  - `local`：全部存本地（默认）
  - `s3`：优先写入 S3（若 S3 初始化失败会回退本地）
  - `auto`：有 S3 就用 S3，否则本地
- 本地目录（当对应类型走本地时生效）
  - `storage.local.uploadDir`：附件/图片目录
  - `storage.local.audioDir`：音频目录
  - `storage.local.tempDir`：临时目录
- 音频导入目录（用于素材库“读取数据目录”功能）
  - `audio.importDir`：扫描导入目录，默认 `${audio.storageDir}/import`
  - 手工验证：放入音频文件 → 管理端“读取数据目录”预览 → 选择导入并确认原文件清理

### 4.2 S3 通用配置项（含分类开关）

以下为关键字段说明（省略的字段请看示例文件）：

```yaml
storage:
  mode: s3
  local:
    uploadDir: ./sealchat-data/upload
    audioDir: ./sealchat-data/static/audio
    tempDir: ./data/temp
  s3:
    enabled: true
    attachmentsEnabled: true
    audioEnabled: true
    endpoint: https://s3.example.com
    region: ""
    bucket: your-bucket-name
    accessKey: ""       # 建议留空，改用环境变量
    secret: ""          # 建议留空，改用环境变量
    sessionToken: ""
    pathStyle: false
    publicBaseUrl: https://cdn.example.com
    useSSL: true
```

- `storage.s3.attachmentsEnabled`：是否将**附件/图片**写入 S3。
- `storage.s3.audioEnabled`：是否将**音频**写入 S3。
- `storage.s3.endpoint`：对象存储服务地址（通常为区域根域名或 MinIO 地址）。
- `storage.s3.bucket`：桶名（部分服务商要求包含 appid/租户后缀）。
- `storage.s3.pathStyle`：是否强制 path-style（`/bucket/key`）。不同服务商要求不同（见下文 COS）。
- `storage.s3.publicBaseUrl`：对外访问前缀，服务端会用它拼接为 `publicBaseUrl/<objectKey>`。
  - 如果你的自定义域名访问需要带桶路径（例如 `https://域名/桶名/...`），这里就必须填 `https://域名/桶名`。
  - 如果你的域名已经 CNAME 到桶的 virtual-host 域名（例如 `https://桶名.xxx/...`），这里填 `https://域名` 即可。

### 4.3 密钥与权限（强烈建议用环境变量）

为避免密钥写入配置文件，建议使用环境变量：

- `SEALCHAT_S3_ACCESS_KEY`
- `SEALCHAT_S3_SECRET_KEY`
- `SEALCHAT_S3_SESSION_TOKEN`（可选）

权限建议（至少满足以下能力）：

- 启动自检：对前缀 `sealchat/_healthcheck/*` 具备 `PutObject/GetObject/DeleteObject`（只用于验证连通性）。
- 业务读写：对前缀 `attachments/*`、`audio/*` 具备 `PutObject/GetObject/DeleteObject`（删除用于资源清理/迁移）。

### 4.4 启动自检与回退策略

当 `storage.s3.enabled=true` 时，服务启动会执行一次小文件 `put/get/delete` 自检：

- 成功：S3 初始化成功并按 `storage.mode` + 分类开关写入。
- 失败：打印类似日志并回退本地（示例）：
  - `[storage] 初始化 S3 失败，回退到本地：S3 自检失败: put/get/read/delete ...`

### 4.5 腾讯 COS 特别说明（常见坑）

COS 常见报错：`The bucket you are attempting to access must be addressed using COS virtual-styled domain.`

处理方式：

- 使用 virtual-host style（不要 path-style）：
  - `storage.s3.pathStyle: false`
- `storage.s3.endpoint` 使用区域根域名（不要带桶名），例如：
  - `https://cos.ap-shanghai.myqcloud.com`
- `storage.s3.bucket` 填 COS 的完整桶名（通常带 `-APPID` 后缀），例如：
  - `mybucket-125xxxxxxx`

并确认 `publicBaseUrl` 与你实际的对外访问方式一致：

- 若访问形式为 `https://域名/桶名/<objectKey>`，则 `publicBaseUrl` 必须包含桶名路径：`https://域名/桶名`
- 若访问形式为 `https://桶名.cos.<region>.myqcloud.com/<objectKey>` 或 CDN 域名无需桶路径，则填对应域名即可

### 4.6 管理端迁移到 S3

管理端提供“迁移到 S3”：

- 支持选择迁移类型：**图片附件** / **音频**
- 建议先执行“模拟运行（dryRun）”观察待迁移数量与错误原因
- “删除源文件”建议谨慎开启：图片迁移会在确认上传成功且 URL 可访问后才删除本地源文件
- 注意：迁移只迁移数据，不会自动修改 `storage.s3.attachmentsEnabled/audioEnabled` 开关

## 其他说明

由于开发资源有限，且处于早期版本，应用场景最为广泛的SQLite是我们的第一优先级支持数据库。

PostgreSQL因为开发者比较常用，是第二优先级支持的数据库。

MySQL的支持可能不如前两者完善。

如果在使用过程中遇到任何问题，请及时向我们反馈，我们会尽快解决。

## 5. 配置版本管理

SealChat 支持配置文件数据库持久化与版本历史管理，便于配置恢复和变更追溯。

### 5.1 功能特性

- **自动同步**：每次启动时自动将配置文件同步到数据库
- **版本历史**：保留最近 10 个配置版本，超出自动清理
- **灾难恢复**：配置文件丢失时自动从数据库恢复
- **敏感字段保护**：CLI 查看时自动遮罩密码、密钥等敏感信息

### 5.2 CLI 命令

```bash
# 列出所有配置历史版本
./sealchat-server --config-list

# 查看指定版本的配置详情（敏感字段显示为 ******）
./sealchat-server --config-show 3

# 回滚到指定版本（会创建新版本记录）
./sealchat-server --config-rollback 2

# 导出指定版本为 YAML 文件
./sealchat-server --config-export 1 --output config.backup.yaml
```

### 5.3 配置同步逻辑

启动时的双路径逻辑：

1. **配置文件存在**：读取 `config.yaml` → 同步到数据库（若内容有变化则创建新版本）
2. **配置文件丢失**：从数据库读取最新配置 → 写入 `config.yaml` 恢复

### 5.4 版本来源标识

每个版本记录包含来源标识：

| 来源 | 说明 |
| --- | --- |
| `file` | 从配置文件同步 |
| `init` | 初始安装 |
| `api` | 通过管理 API 修改 |
| `rollback` | 回滚操作 |

### 5.5 环境变量

CLI 命令支持通过环境变量指定数据库连接（优先级高于配置文件）：

```bash
export SEALCHAT_DSN="postgresql://user:pass@localhost:5432/sealchat"
./sealchat-server --config-list
```
