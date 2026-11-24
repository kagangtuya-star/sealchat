# LOG 排版落点

设计态预览以 `sectionStart → (before 元素、消息、after 元素)* → sectionEnd` 排列节点。未加载完整的片段没有可选的 `sectionEnd`。消息排名仍属于频道；元素没有全局排名。批量上移、下移按消息计数；重定位以所点击的视觉间隙为准。

`message.reorder` / `message.reorder.log` 的 `after_id` 指左邻，`before_id` 指右邻。客户端可只给用户指向的一侧，服务端排除移动消息，以频道的 `(display_order, created_at, id)` 查出另一侧。两侧都指定时必须是同一真实间隙。

`message.relocate.batch` 保持原有 `message_ids`、`target_message_id`、`placement` 和 `groups` 语义。需要同时调整元素时，使用单组、`expected_versions`、`client_op_id`，并增加与 `message.reorder.log` 相同的可选 `log_draft`：

```text
log_draft: { projectId, sectionId, expectedVersion, placements: [{ elementId, anchor, order }] }
```

新锚点只能指向本次移动集合中的消息。若消息的相对顺序不变、仅需跨越元素，可省去 `target_message_id`。消息版本检查、排名更新、草稿 CAS 和幂等结果同事务提交；成功回包和幂等重放都包含 `draft_version`。无 `log_draft` 的请求不更新工程版本。

单条或批量混合移动提交后，服务端另发 `log-project-draft-changed`（`projectId`、`draftVersion`）给有设计访问权的其他普通连接。它不包含草稿、消息或元素信息；发起连接从命令回包采纳版本。频道 `message-reordered` 和 `sourceVersion` 不携带或替代工程草稿版本。

直接移动元素只保存工程草稿的 `anchor` / `order`，不写消息排名。显式落到章节首尾会改为章节端点锚点，即使视觉顺序未变。分页预览、打印与 PDF 按视觉顺序将消息及其前后元素同页排版。整组超出空页容量时回退到逐块分页（`ANCHOR_GROUP_OVERSIZE`），仍放不下且不可继续切分的块按栏宽和可用高度缩小；组内有显式分页时尊重模板（`ANCHOR_GROUP_BREAK`）。流式排版不使用此规则。

后台 PDF（含分段 PDF 压缩包）仍可下载。若成品发生锚定组拆页，完成态的 `GET /api/v1/chat/export/:taskId` 通过可选的 `log_pdf_warnings` 返回按上述原因统计的组数；目录回填后的重新渲染替换首轮统计。导出对话框会在完成态显示这些提示。

新发布快照的消息锚定元素包含可选 `anchorNodeId`，只引用本次发布生成的公开消息节点 ID，不包含聊天消息 ID。章节端点不带该字段；旧快照没有可靠的锚点信息，保留原分页，重新发布后可使用同页规则。
