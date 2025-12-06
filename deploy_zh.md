
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

# 创建配置文件 (可选)
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

主程序文件名为 `sealchat_server`。根据您的操作系统，可能会有不同的扩展名：
- Windows: sealchat_server.exe
- Linux/macOS: sealchat_server


## 3. 运行程序

根据您的操作系统，按照以下步骤运行程序：

### Windows

直接双击 `sealchat_server.exe` 文件来运行程序。

打开浏览器，访问 http://localhost:3212/ 即可使用，第一个注册的帐号会成为管理员账号。

### Linux

1. 打开终端
2. 使用 `cd` 命令导航到解压缩的目录，例如：
   ```
   cd /path/to/sealchat
   ```
3. 给予执行权限（如果尚未授予）：
   ```
   chmod +x sealchat_server
   ```
4. 运行以下命令：
   ```
   ./sealchat_server
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

## 其他说明

由于开发资源有限，且处于早期版本，应用场景最为广泛的SQLite是我们的第一优先级支持数据库。

PostgreSQL因为开发者比较常用，是第二优先级支持的数据库。

MySQL的支持可能不如前两者完善。

如果在使用过程中遇到任何问题，请及时向我们反馈，我们会尽快解决。
