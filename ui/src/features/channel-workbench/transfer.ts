import { cloneWorkbenchLayout, sanitizeWorkbenchLayout, WorkbenchLayoutError } from './layout'
import { WORKBENCH_LAYOUT_VERSION, type WorkbenchLayout, type WorkbenchPanel } from './types'

// 导入导出协议：
//   { kind: 'sealchat.workbench', version: 1, exportType: 'template' | 'design', exportedAt, source?, layout }
// - template：只保留面板骨架（类型、位置、标题、外观），去掉世界/频道内资源绑定，可在任何世界使用。
// - design：完整设计，保留资源描述（resourceId / channelId）；导入到其他世界时缺失资源进入“重新绑定”。
// 两者都只保存资源描述，从不保存生成后的 Internal Surface / Embed URL。

export const WORKBENCH_TRANSFER_KIND = 'sealchat.workbench' as const
export const WORKBENCH_TRANSFER_VERSION = 1 as const

export type WorkbenchExportType = 'template' | 'design'

export interface WorkbenchTransferDocument {
  kind: typeof WORKBENCH_TRANSFER_KIND
  version: typeof WORKBENCH_TRANSFER_VERSION
  exportType: WorkbenchExportType
  exportedAt: string
  name?: string
  source?: { worldId?: string; channelId?: string; channelName?: string }
  layout: WorkbenchLayout
}

export interface WorkbenchImportResult {
  exportType: WorkbenchExportType
  name?: string
  layout: WorkbenchLayout
  warnings: string[]
  /** 设计来自其他世界时，频道/资源绑定大概率需要重新绑定。 */
  foreignWorld: boolean
}

/** 模板去掉资源绑定；线索画板只有一个 main 资源，保留。外部网页地址不是世界资源，保留。 */
export const stripWorkbenchBindings = (layout: WorkbenchLayout): WorkbenchLayout => {
  const next = cloneWorkbenchLayout(layout)
  next.panels = next.panels.map((panel): WorkbenchPanel => {
    const source = { ...panel.source }
    delete source.channelId
    delete source.label
    if (source.type === 'surface' && source.surface !== 'clue-board') delete source.resourceId
    return { ...panel, source }
  })
  return next
}

export const buildWorkbenchExport = (
  layout: WorkbenchLayout,
  exportType: WorkbenchExportType,
  meta: { name?: string; worldId?: string; channelId?: string; channelName?: string; now?: Date } = {},
): WorkbenchTransferDocument => {
  const document: WorkbenchTransferDocument = {
    kind: WORKBENCH_TRANSFER_KIND,
    version: WORKBENCH_TRANSFER_VERSION,
    exportType,
    exportedAt: (meta.now || new Date()).toISOString(),
    layout: exportType === 'template' ? stripWorkbenchBindings(layout) : cloneWorkbenchLayout(layout),
  }
  if (meta.name) document.name = meta.name
  if (exportType === 'design' && (meta.worldId || meta.channelId)) {
    document.source = { worldId: meta.worldId, channelId: meta.channelId, channelName: meta.channelName }
  }
  return document
}

/** 解析导入文本；结构问题降级为警告，只有不可识别的文档或更高版本才抛错。 */
export const parseWorkbenchImport = (input: string | unknown, currentWorldId = ''): WorkbenchImportResult => {
  let value: unknown = input
  if (typeof input === 'string') {
    try {
      value = JSON.parse(input)
    } catch {
      throw new WorkbenchLayoutError('文件不是合法的 JSON')
    }
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new WorkbenchLayoutError('文件内容不是布局文档')
  const doc = value as Record<string, unknown>
  if (doc.kind !== WORKBENCH_TRANSFER_KIND) throw new WorkbenchLayoutError('不是 SealChat 布局频道导出文件')
  if (typeof doc.version !== 'number' || !Number.isInteger(doc.version) || doc.version < 1) throw new WorkbenchLayoutError('导出文件版本无效')
  if (doc.version > WORKBENCH_TRANSFER_VERSION) throw new WorkbenchLayoutError(`导出文件版本 ${doc.version} 高于当前支持的版本 ${WORKBENCH_TRANSFER_VERSION}`)
  const exportType: WorkbenchExportType = doc.exportType === 'design' ? 'design' : 'template'
  const { layout, warnings } = sanitizeWorkbenchLayout(doc.layout)
  const finalLayout = exportType === 'template' ? stripWorkbenchBindings(layout) : layout
  const source = doc.source && typeof doc.source === 'object' ? doc.source as Record<string, unknown> : {}
  const sourceWorldId = typeof source.worldId === 'string' ? source.worldId : ''
  return {
    exportType,
    name: typeof doc.name === 'string' ? doc.name.slice(0, 80) : undefined,
    layout: finalLayout,
    warnings,
    foreignWorld: exportType === 'design' && !!sourceWorldId && !!currentWorldId && sourceWorldId !== currentWorldId,
  }
}

export const workbenchExportFilename = (name: string, exportType: WorkbenchExportType) => {
  const safe = (name || 'workbench').replace(/[\\/:*?"<>|\u0000-\u001f]+/g, '_').slice(0, 60) || 'workbench'
  return `${safe}.${exportType === 'template' ? 'workbench-template' : 'workbench'}.json`
}

// ---------------------------------------------------------------------------
// 预设：与导出模板同一格式，导入路径完全相同。

const panel = (id: string, title: string, rect: WorkbenchPanel['rect'], source: WorkbenchPanel['source'], extra: Partial<WorkbenchPanel> = {}): WorkbenchPanel => ({
  id, title, rect, source, chrome: 'minimal', ...extra,
})

const presetLayout = (rows: number, panels: WorkbenchPanel[], settings: WorkbenchLayout['settings'] = {}): WorkbenchLayout => ({
  version: WORKBENCH_LAYOUT_VERSION,
  grid: { columns: 24, rows, gap: 8 },
  settings,
  panels,
})

export interface WorkbenchPreset {
  id: string
  name: string
  description: string
  document: WorkbenchTransferDocument
}

const presetDocument = (name: string, layout: WorkbenchLayout): WorkbenchTransferDocument => ({
  kind: WORKBENCH_TRANSFER_KIND,
  version: WORKBENCH_TRANSFER_VERSION,
  exportType: 'template',
  exportedAt: '2026-11-15T00:00:00.000Z',
  name,
  layout,
})

export const WORKBENCH_PRESETS: WorkbenchPreset[] = [
  {
    id: 'character-wall',
    name: '角色卡墙',
    description: '四张人物卡并排 + 底部频道聊天，适合 DM 屏幕或队伍总览。',
    document: presetDocument('角色卡墙', presetLayout(12, [
      panel('pc-1', '角色 1', { x: 0, y: 0, w: 6, h: 8 }, { type: 'surface', surface: 'character' }),
      panel('pc-2', '角色 2', { x: 6, y: 0, w: 6, h: 8 }, { type: 'surface', surface: 'character' }),
      panel('pc-3', '角色 3', { x: 12, y: 0, w: 6, h: 8 }, { type: 'surface', surface: 'character' }),
      panel('pc-4', '角色 4', { x: 18, y: 0, w: 6, h: 8 }, { type: 'surface', surface: 'character' }),
      panel('chat', '频道聊天', { x: 0, y: 8, w: 24, h: 4 }, { type: 'chat' }),
    ], { mobileMode: 'tabs' })),
  },
  {
    id: 'map-chat',
    name: '地图 + 聊天',
    description: '大幅线索画板或小剧场作为地图，右侧聊天与便签。',
    document: presetDocument('地图 + 聊天', presetLayout(12, [
      panel('map', '地图', { x: 0, y: 0, w: 17, h: 12 }, { type: 'surface', surface: 'clue-board', resourceId: 'main' }),
      panel('chat', '频道聊天', { x: 17, y: 0, w: 7, h: 8 }, { type: 'chat' }),
      panel('note', '便签', { x: 17, y: 8, w: 7, h: 4 }, { type: 'surface', surface: 'note' }),
    ], { mobileMode: 'tabs' })),
  },
  {
    id: 'blank',
    name: '空白工作台',
    description: '从空白网格开始自由添加面板。',
    document: presetDocument('空白工作台', presetLayout(12, [])),
  },
]
