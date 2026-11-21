import { reactive, readonly } from 'vue'

// 供界面自定义脚本读取的只读快照（sc.context().workbench 与 workbench-change 事件）。
// 只依赖 vue，界面自定义运行时可以直接引用而不牵连布局频道的其他模块。

export interface WorkbenchRuntimeSnapshot {
  active: boolean
  channelId: string
  mode: 'view' | 'design' | 'preview'
  immersive: boolean
  focusedPanelId: string
  spotlightPanelId: string
}

const runtimeState = reactive<WorkbenchRuntimeSnapshot>({
  active: false,
  channelId: '',
  mode: 'view',
  immersive: false,
  focusedPanelId: '',
  spotlightPanelId: '',
})

export const workbenchRuntimeState = readonly(runtimeState)

export const publishWorkbenchRuntimeState = (patch: Partial<WorkbenchRuntimeSnapshot>) => {
  Object.assign(runtimeState, patch)
}

export const resetWorkbenchRuntimeState = () => {
  Object.assign(runtimeState, { active: false, channelId: '', mode: 'view', immersive: false, focusedPanelId: '', spotlightPanelId: '' })
}
