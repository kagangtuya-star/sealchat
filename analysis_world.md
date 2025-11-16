# 世界体系问题分析

## 1. world_members 数据缺失导致权限校验失败
- **症状**：加入邀请后频道报 “用户不是该世界成员”。
- **根因**：`EnsureWorldMemberActive` 依赖 `world_members` 表，若该记录缺失（旧数据、事务失败等），世界校验就失败。
- **影响**：频道列表 API 也依赖该检查，会导致整个世界看上去“空”或“频道不存在”。

## 2. WebSocket/前端 worldId 同步不一致
- **症状**：切换世界或邀请加入后，WS 仍在旧世界，`channel.enter` 请求中的 `world_id` 为空。
- **根因**：Identify/`channel.enter` 时没有携带最新 worldId；或前端 store 尚未更新就发请求。
- **影响**：服务端强制检查 `world_id`，因此返回“频道不属于当前世界”，client 端随即断线重连。

## 3. 新世界缺省频道/角色
- **症状**：创建世界后没频道，进入世界看不到任何内容。
- **根因**：`CreateWorld` 不自动建频道；权限体系也未默认给 world owner 建立相应角色。
- **影响**：用户误以为世界“不可用”，不得不手动创建频道。

## 4. 世界大厅未显示私密世界
- **症状**：自己创建/加入的私密世界在大厅看不到。
- **根因**：`WorldListPublic` 默认仅查询 `visibility=public`，直到最近才加入 owner/member 异或条件；此前邀请加入的世界不会出现在列表。
- **影响**：从大厅无法返回刚加入的世界，只能依赖邀请链接。

## 5. 邀请完成后未刷新本地数据
- **症状**：接受邀请成功，但 `worldStore` 仍停留在旧数据，导致当前世界/频道仍是之前的。
- **根因**：前端邀请页接受后仅跳转，不触发 `worldStore.fetchWorlds`、`chat.ensureWorldSession`。
- **影响**：用户加入后第一次切换世界会失败（worldId 为空）。

---

# 修复方案（概要）

1. **WorldMember 修复与兜底**
   - 接受邀请/加入世界时确保 `world_member` 记录存在（若缺失则补写）。
   - `EnsureWorldMemberActive` 除了读 `world_members`，也可 fallback 到 `members` 表（已有兜底，但需确认写入是否落地）。

2. **前端世界上下文同步**
   - Identify/`channel.enter` 统一使用 `chat.connectedWorldId || worldStore.currentWorldId`，并在切换世界后立即更新。
   - 邀请页成功后刷新 `worldStore` 并调用 `chat.ensureWorldSession`。

3. **创建默认频道与角色**
   - 创建世界时自动创建一个文本频道并赋予 owner 对应频道角色。
   - 或在 UI 中提示“世界暂无频道，点击创建”。

4. **世界大厅查询补齐**
   - `WorldList` 在登录状态下带上 owner/member 条件，确保私密世界在大厅中可见。
   - 若必要，可提供“仅显示我加入”的筛选。

5. **邀请页体验**
   - 生成/复制邀请链接时使用 `/#/invite/{code}` 以兼容 hash router。
   - 邀请页 `POST /accept` 成功后跳转世界详情并刷新世界/频道数据。

请在新的对话中按优先级处理这些点，优先保证 world 成员校验和前端 worldId 同步，随后再处理体验细节。
