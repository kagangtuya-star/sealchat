# 小剧场舞台 UI 开发指引

## 适用范围

本文件适用于 `ui/src/views/theater/stage/`，并叠加父级小剧场指引和前端指引。

## Teleport 与层级

- `Teleport to="body"` 的内容不会继承编辑器 CSS 变量。每个 Teleport 根节点都要定义所需变量（例如两个 modal 根节点），并复用同一套主题值。
- Naive UI 的 select/dropdown 应沿用 `TheaterActionSequenceEditor.vue` 的 `menu-props.class` 写法，并保留专用 class（例如 `theater-clue-select-menu`）。针对实际菜单类（如 `.n-select-menu.n-base-select-menu.<feature-class>`）设置样式，只提升该功能的 follower；不要修改全局 select。
- 应在 modal/follower 层修复 stacking context。禁止使用 `top`、`margin`、`translate` 或 `transform` 人工伪造弹层位置。

## 异步 UI 与生命周期

- 异步 dialog/picker 读取需要轻量的 request epoch 或 context-id 保护。旧响应不能覆盖当前选择；关闭或卸载时要使未完成响应失效。
- 先读取并校验数据，再修改由 `StageStore` 同步的字段。读取为空或失败时，持久化对象状态必须保持不变。
- Vue listener、timer、watcher 和 Konva 资源必须成对清理。重建画布内容时同步更新 node/layer/tween/animation 映射并销毁过期对象。

## 动作与编辑器契约

- 动作 payload 必须通过共享 types/actions 模块保持规范化，不要创建组件私有变体。
- `STAGE_ACTION_CANCELLED` 是控制流结果，不是错误。取消后序列必须停止且不显示错误 toast；真实请求失败仍沿用原有错误链。
- 修改线索或序列编辑器时，保持普通舞台动作行为不变。

## 验证

在 `ui/` 运行 `npm run type-check` 和 `npm run build`；交付前在仓库根目录运行 `git diff --check`。
