
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
