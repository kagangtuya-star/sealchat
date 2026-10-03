# 小剧场视图开发指引

## 适用范围

本文件适用于 `ui/src/views/theater/` 及其子目录。请同时遵循仓库根目录和 `ui/AGENTS.md`；`stage/AGENTS.md` 补充编辑器 UI 的专属规则。

## 目录结构

- `host/` — 路由级小剧场布局、聊天 iframe，以及 Bridge/Sync 生命周期。
- `stage/` — Konva 舞台编辑器、`StageStore`、对象/动作编辑器和序列运行时。
- `bridge/` — host/stage/chat Bridge 协议、传输层、capability 和事件路由。
- `sync/` — 服务端快照、mutation 持久化、flush 顺序和动作请求。
- `shared/` — 舞台领域类型、动作构造器和 normalizer。
- `dialogue/`、`effects/`、`overlays/` — 相互独立的运行时和渲染子系统。

## 嵌套弹层、焦点与 Teleport

小剧场大量 UI 同时存在浮动面板、Modal、Drawer、Popover、Select/Dropdown follower 和 PIP/嵌入模式。新增或修改这些 UI 时，必须把“焦点归属”和“Teleport 目标”作为组件契约的一部分，而不是出现问题后再给单个输入框打补丁。

- 先确认当前编辑器是否已经位于祖先 `Modal`、`Drawer`、浮动面板或其他 focus scope 中。`sequences/`、`effects/`、`overlays/` 等 sibling 目录同样适用，不能只参考 `stage/AGENTS.md`。
- Naive UI 的 `Modal`/`Drawer` 默认 focus trap 与 Teleport 子层可能互相抢焦点。子弹层或其 Select/Dropdown/Popover follower 如果 Teleport 到祖先 focus scope 之外，必须二选一解决：
  1. 将子层 `to` 到祖先 focus scope 内的稳定宿主；或
  2. 对确实需要跨 focus scope 的子级弹层显式放宽焦点约束，例如现有小剧场嵌套编辑器常用 `:auto-focus="false" :trap-focus="false"`。
- 不要无条件关闭所有 Modal 的 focus trap。只有在确认存在嵌套 overlay / Teleport 竞争时才放宽；普通独立 Modal 保持默认可访问性语义。
- 禁止用单个输入控件的 `mousedown`/`pointerdown.prevent`、手动 `.focus()`、反复 `nextTick().focus()`、修改 `tabindex` 等方式掩盖 focus-scope 冲突。这类补丁只会让输入框暂时可用，却继续破坏 Select/Dropdown/键盘导航。
- `NSelect` 是 Naive UI `NSelect` 的薄封装，默认 follower 行为仍按 Naive UI 处理；不要因为使用了 `@/components/NSelect.vue` 就忽略 Teleport/focus 规则。
- Select/Dropdown/Popover 的菜单显示正常但无法搜索、无法选中、点击后立即失焦，或输入框“能高亮但不能持续输入”，首先按 focus trap / Teleport 冲突排查，不要先怀疑业务 `v-model`。
- 所有 feature-local Select/Dropdown follower 都应使用专用 `menu-props.class`，并只针对该 class 调整 follower 的 stacking context；禁止为了修一个编辑器去改全局 `.n-select-menu` / `.n-dropdown-menu`。
- follower 层级问题应在 Modal/follower 的 stacking context 上解决；禁止用 `top`、`margin`、`translate`、`transform` 等方式伪造弹层位置。
- 新增一个包含输入控件的 Modal/Drawer/Popover 时，提交前至少人工推演并尽量实测：普通 `n-input`、textarea、`n-input-number`、filterable Select 的输入/搜索/选中、Dropdown 点击，以及 Esc/关闭行为。若未做真实浏览器验证，交付说明中必须明确写出。

## 边界与约定

- 舞台状态和历史 mutation 由 `StageStore` 负责。mutation 可能触发自动同步，因此异步前置读取成功前不要修改持久化状态。
- 复用 `shared/stage-types.ts` 和 `shared/stage-actions.ts`。旧数据或不可信 payload 应在边界处 normalize，不要在视图组件内复制动作结构。
- Bridge capability 名称、事件名、payload 和动作结果语义都属于兼容性接口。修改前先检查所有生产者和消费者。
- 小剧场通过 iframe 嵌入聊天。保留现有 URL 规范化、`postMessage` 的 origin/source 校验和卸载清理。
- 动作确认取消属于正常控制流：通过 bridge/runtime 传递专用 cancellation sentinel，不要转换成面向用户的失败提示。
- 接入 `dialogue/theater-dialogue-residency.ts` 时，只从现有 runtime 的 `queue.current` 驱动入场；驻场状态属于当前世界的临时运行时，不放进 `StageStore` 或持久化消息历史。
- `dialogue/theater-dialogue-layout.ts` 使用立绘区域局部坐标，位置偏好必须经过统一布局约束；布局计算本身不写云端。相关纯逻辑用 `ui/scripts/theater-dialogue-residency-layout.spec.ts` 验证。
- 控制器设置和单角色位置通过 `TheaterSyncClient` 的房间提交协调写入；只传字段 patch，不把驻场、拖拽预览或控制器配置放进场景及撤销历史。模式交接沿用原 runtime 的去重和 skip 语义。

## UI 回归检查

涉及弹层、焦点或 follower 的改动，除类型检查外还要检查以下回归面：

- 标准小剧场与 PIP/窄屏模式下，弹层是否仍可操作；
- 浮动面板内打开二级编辑器时，父面板拖拽/置顶逻辑是否不会抢输入焦点；
- Select/Dropdown follower 是否位于正确层级，且不会被父 Modal、遮罩或 Konva 舞台覆盖；
- 打开/关闭子弹层后，父层焦点和键盘快捷键是否恢复正常；
- 不要把“能打开菜单”当成通过，必须确认可搜索、可选择、可键盘操作。

## 验证

在 `ui/` 目录运行：

```bash
npm run type-check
npm run build
node scripts/run-theater-dialogue-tests.cjs
```

对于非小型修改，还应在仓库根目录运行 `git diff --check`，并检查最终 diff 是否包含无关文件。
