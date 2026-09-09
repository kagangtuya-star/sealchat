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

## 边界与约定

- 舞台状态和历史 mutation 由 `StageStore` 负责。mutation 可能触发自动同步，因此异步前置读取成功前不要修改持久化状态。
- 复用 `shared/stage-types.ts` 和 `shared/stage-actions.ts`。旧数据或不可信 payload 应在边界处 normalize，不要在视图组件内复制动作结构。
- Bridge capability 名称、事件名、payload 和动作结果语义都属于兼容性接口。修改前先检查所有生产者和消费者。
- 小剧场通过 iframe 嵌入聊天。保留现有 URL 规范化、`postMessage` 的 origin/source 校验和卸载清理。
- 动作确认取消属于正常控制流：通过 bridge/runtime 传递专用 cancellation sentinel，不要转换成面向用户的失败提示。

## 验证

在 `ui/` 目录运行：

```bash
npm run type-check
npm run build
```

对于非小型修改，还应在仓库根目录运行 `git diff --check`，并检查最终 diff 是否包含无关文件。
