# S3 开发约束

## 核心原则

- 优先复用现有 S3 行为，不要为了新功能修改共享 S3 client 的既有初始化、寻址或上传语义。
- 新功能遇到兼容问题时，优先增加局部兼容分支，避免影响附件、字体、小剧场、迁移等既有 S3 业务。
- `ForcePathStyle=false` 不代表所有兼容存储都能正确处理 MinIO `BucketLookupAuto`，尤其注意 COS 等自定义 endpoint。
- 不要假设所有 S3-compatible 服务都完整支持相同的 List Objects 行为。

## 对象列表

- 对兼容性敏感的新增列表能力优先使用 List Objects V1。
- V1 成功但返回空结果时，可以使用 V2 作为补偿。
- V1 返回明确错误时，不要无条件切换 V2。
- `NoSuchKey` / `NoSuchObject` 可能是 endpoint、bucket lookup 或 path-style 寻址问题，不应直接解释为“prefix 不存在”。
- 新增兼容 listing 时，尽量使用独立 sidecar client，不要替换共享业务 client。

## Endpoint 与 Bucket

部分存储可能配置为：

```text
endpoint = <bucket>.<service-endpoint>
bucket   = <bucket>
```

此时错误的 path-style 访问可能导致实际对象路径变成：

```text
<bucket>/<logical-object-key>
```

而不是预期的：

```text
<logical-object-key>
```

兼容此类历史对象时：

- 只在明确的 addressing fallback 中尝试 `<bucket>/<prefix>`。
- 返回上层前必须映射回逻辑 ObjectKey。
- 不要把底层物理路径泄漏给业务层。
- 不要因此修改现有上传、删除、Stat、Presign 的 ObjectKey 语义。

## 删除与写入

- 不要因为列表兼容需求修改现有 `upload`、`exists`、`delete`、`deletePrefix`、`download`、`presign`。
- 如果旧逻辑已稳定工作，新列表能力应保持独立。
- 删除远端对象时应使用业务层原有逻辑 ObjectKey，不要直接使用兼容 listing 返回的物理路径。

## 数据安全

涉及备份或迁移时：

```text
本地生成
→ 远端上传
→ Stat/Exists 校验
→ 列表确认对象可见
→ 才允许删除本地副本
```

- 上传成功不等于远端已经可管理。
- List 报错或列表中找不到刚上传对象时，必须保留本地数据。
- 不要为了“让流程成功”而放宽对象存在性检查。
- 兼容失败优先产生冗余副本，不要冒数据丢失风险。

## 修改范围

修复 S3 问题前先确认实际调用链。

若问题只影响新增能力：

- 不修改共享 client 初始化。
- 不修改已有业务方法。
- 不做全局 S3 重构。
- 不新增无必要配置项。
- 优先最小、可隔离、可回退的实现。

修改完成后检查是否意外影响：

- 附件
- 音频
- 字体
- 小剧场资源
- S3 迁移
- DeletePrefix
- Presign
- PublicURL
- S3 初始化与健康检查