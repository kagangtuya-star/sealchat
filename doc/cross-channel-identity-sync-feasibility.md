# 跨频道角色静默同步可行性方案

## 结论

方案可行，且比“同一频道角色记录跨频道复用”更兼容现有架构。

采用“共享角色模板 + 每频道真实副本”：每个频道角色继续拥有独立 `ChannelIdentityModel.ID` 和真实 `ChannelID`，现有消息、权限、BOT、小剧场、人物卡及缓存逻辑大体不变；额外使用共享组 ID 表达跨频道等价关系。

## 数据模型

新增共享模板：

```go
type SharedChannelIdentityModel struct {
	StringPKBaseModel
	UserID              string
	DisplayName         string
	Color               string
	AvatarAttachmentID  string
	AvatarDecorations   protocol.AvatarDecorationList
	TheaterPresentation *protocol.TheaterPresentation
	Revision            int64
}
```

`ChannelIdentityModel` 新增：

```go
SharedIdentityID string `json:"sharedIdentityId,omitempty" gorm:"size:100;index"`
SharedRevision   int64  `json:"sharedRevision,omitempty"`
```

约束：同一共享模板在同一频道最多一个副本，建议唯一索引 `(shared_identity_id, channel_id)`。

## 同步范围

同步字段：

- 昵称、颜色、头像、头像装饰。
- 同一世界内的小剧场演出配置。

频道独立字段：

- 副本 ID、`ChannelID`、默认身份、排序、文件夹、IC/OOC 映射。
- 人物卡绑定、人物卡数据、角色卡快照 active 状态。
- 临时 NPC、隐藏身份、BOT 频道外观。

首版排除 BOT、Webhook 外部身份、临时 NPC，避免现有频道级生命周期与全局同步冲突。

## 写入流程

所有共享角色修改必须经过统一服务，不允许副本互相触发同步：

1. 用户编辑任一频道副本。
2. 后端确认操作者为身份本人。
3. 更新共享模板并增加 `Revision`。
4. 按模板覆写全部频道副本的同步字段和 `SharedRevision`。
5. 局部修补各频道角色卡快照的身份展示字段。
6. 向受影响频道广播角色与快照刷新事件。

数据库字段同步应在事务内完成。小剧场资源映射、广播失败等事务外操作使用 outbox/retry 补偿，避免部分频道永久落后。

## 副本创建与修复

副本仅覆盖用户实际加入的频道：

- 创建共享角色时，为用户现有频道批量创建副本。
- 用户加入频道或创建新频道时，创建其共享角色副本。
- 加载频道角色列表时补建缺失副本。
- 定期按 `Revision` 修复缺失或落后的副本。

用户退出频道后可删除该频道副本；历史消息使用发送时快照，不受影响。

## 小剧场资源

世界房间资源仍按现有世界级小剧场资源链处理。频道角色自行上传的演出外观使用 `TheaterAppearanceAssetModel`，带频道范围。

同一世界同步演出配置时：

- 世界模板资源直接复用。
- 角色外观资源为目标频道创建轻量资源映射。
- 复用底层附件和转码结果，仅重写目标副本演出 JSON 中的 asset ID。

跨世界首版不自动同步小剧场演出配置。

## 频道角色卡快照

`ChannelCharacterSnapshotModel` 保持 `(ChannelID, IdentityID)` 频道级快照，不跨频道复制人物卡内容。

共享角色外观修改时，仅修补快照中的：

- `data.identity.displayName`
- `data.identity.color`
- `data.identity.avatarAttachmentId`
- `data.identity.avatarDecorations`

保留 `data.card`、`SourceCardID`、属性数据和 active 状态；随后重算 `ContentHash`、增加 `ServerRevision` 并广播刷新。修补失败时可将快照设为 inactive，等待客户端重建。

## 权限与删除

- 仅身份本人可创建、编辑或删除共享角色。
- 频道管理员不得通过代管身份修改共享模板。
- 删除共享角色应显式确认，并删除模板及全部频道副本。
- 不允许单独删除副本；否则补偿逻辑会再次创建。
- 可选“拆分为独立角色”必须显式设计；首版建议不提供。

## 兼容性

- 消息继续记录频道副本 ID，并冻结发送时外观；无需迁移历史消息。
- 频道内筛选、引用、Webhook、OneBot 和小剧场绑定继续使用真实副本 ID。
- 人物卡及快照继续按频道工作。
- 跨频道统计同一角色时，通过 `SharedIdentityID` 聚合。
- 频道复制应跳过共享模板复制，由目标频道自动生成新副本。

## 建议实施顺序

1. 新增共享模板、关联字段、唯一索引与统一同步服务。
2. 接入角色创建/编辑入口、频道加入与新频道副本创建。
3. 接入广播、revision 修复与角色卡快照局部修补。
4. 接入同世界小剧场外观资源映射。
5. 评估头像差分、跨世界同步及 BOT 支持。

总体风险可控。首版限制同步字段和支持对象后，核心消息链与频道级角色校验无需重构。
