import { INTERNAL_SURFACE_TYPES } from '@/utils/internalSurfaceLink'
import {
  WORKBENCH_LAYOUT_VERSION,
  type WorkbenchGrid,
  type WorkbenchLayout,
  type WorkbenchPanel,
  type WorkbenchPresentation,
  type WorkbenchRect,
  type WorkbenchSource,
  type WorkbenchSurfaceType,
} from './types'

// 纯函数：布局校验 / 迁移 / 网格操作。规则镜像服务端 normalizeChannelWorkbenchLayout，
// 服务端仍是最终校验方；这里负责导入时的宽松修复与编辑器的不重叠约束。

export const WORKBENCH_LIMITS = {
  maxPanels: 48,
  minRows: 4,
  maxRows: 60,
  maxGap: 32,
  maxTitle: 80,
  maxLabel: 120,
  maxResourceId: 200,
  maxChannelId: 100,
  maxUrl: 2048,
} as const

const PANEL_ID_PATTERN = /^[A-Za-z0-9_-]{1,40}$/
const ACCENT_PATTERN = /^#[0-9a-fA-F]{6}$/
const MOBILE_MODES = ['', 'tabs', 'stack']
const PANEL_STYLES = ['', 'flat', 'outlined', 'glass']
const MINIMAL_ACTIONS_PLACEMENTS = ['', 'inside', 'outside']
const WEB_DISPLAYS = ['', 'embed', 'link']

export class WorkbenchLayoutError extends Error {}

export const createEmptyWorkbenchLayout = (): WorkbenchLayout => ({
  version: WORKBENCH_LAYOUT_VERSION,
  grid: { columns: 24, rows: 12, gap: 8 },
  settings: {},
  panels: [],
})

export const cloneWorkbenchLayout = (layout: WorkbenchLayout): WorkbenchLayout => JSON.parse(JSON.stringify(layout))

const isRecord = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value)
const text = (value: unknown) => (typeof value === 'string' ? value.trim() : '')
const integer = (value: unknown, fallback: number) => (
  typeof value === 'number' && Number.isFinite(value) ? Math.round(value) : fallback
)
const clamp = (value: number, min: number, max: number) => Math.min(Math.max(value, min), Math.max(min, max))
const runes = (value: string) => Array.from(value)
const truncate = (value: string, max: number) => {
  const chars = runes(value)
  return chars.length > max ? chars.slice(0, max).join('') : value
}

/** 外部网页只接受 http(s) 绝对地址，且不能带账号信息。 */
export const normalizeWorkbenchWebUrl = (raw: unknown): string => {
  const value = text(raw)
  if (!value || value.length > WORKBENCH_LIMITS.maxUrl) return ''
  try {
    const url = new URL(value)
    if ((url.protocol !== 'http:' && url.protocol !== 'https:') || !url.host || url.username || url.password) return ''
    return value
  } catch {
    return ''
  }
}

const isSurfaceType = (value: string): value is WorkbenchSurfaceType => (INTERNAL_SURFACE_TYPES as readonly string[]).includes(value)

const sanitizeSource = (raw: unknown): { source: WorkbenchSource | null; warning?: string } => {
  if (!isRecord(raw)) return { source: null, warning: '缺少资源描述' }
  const type = text(raw.type)
  const channelId = text(raw.channelId)
  const label = truncate(text(raw.label), WORKBENCH_LIMITS.maxLabel)
  const validChannelId = channelId.length <= WORKBENCH_LIMITS.maxChannelId ? channelId : ''
  switch (type) {
    case 'surface': {
      const surface = text(raw.surface)
      if (!isSurfaceType(surface)) return { source: null, warning: `不支持的内部面板类型 ${surface || '(空)'}` }
      let resourceId = text(raw.resourceId)
      let warning: string | undefined
      if (resourceId.length > WORKBENCH_LIMITS.maxResourceId || /[\u0000/?#]/.test(resourceId)) {
        resourceId = ''
        warning = '资源 ID 无效，已清空绑定'
      }
      if (surface === 'clue-board' && resourceId && resourceId !== 'main') resourceId = 'main'
      const source: WorkbenchSource = { type, surface }
      if (resourceId) source.resourceId = resourceId
      if (validChannelId) source.channelId = validChannelId
      if (label) source.label = label
      return { source, warning }
    }
    case 'chat':
    case 'theater': {
      const source: WorkbenchSource = { type }
      if (validChannelId) source.channelId = validChannelId
      if (label) source.label = label
      return { source }
    }
    case 'web': {
      const url = normalizeWorkbenchWebUrl(raw.url)
      const display = text(raw.display)
      const source: WorkbenchSource = { type }
      if (url) source.url = url
      if (WEB_DISPLAYS.includes(display) && display) source.display = display as WorkbenchSource['display']
      return { source, warning: text(raw.url) && !url ? '网页地址无效，已清空' : undefined }
    }
    default:
      return { source: null, warning: `不支持的面板来源 ${type || '(空)'}` }
  }
}

const sanitizeGrid = (raw: unknown): WorkbenchGrid => {
  const grid = isRecord(raw) ? raw : {}
  const columns = grid.columns === 12 ? 12 : 24
  return {
    columns,
    rows: clamp(integer(grid.rows, 12), WORKBENCH_LIMITS.minRows, WORKBENCH_LIMITS.maxRows),
    gap: clamp(integer(grid.gap, 8), 0, WORKBENCH_LIMITS.maxGap),
  }
}

const clampRect = (raw: unknown, grid: WorkbenchGrid): WorkbenchRect => {
  const rect = isRecord(raw) ? raw : {}
  const w = clamp(integer(rect.w, Math.min(6, grid.columns)), 1, grid.columns)
  const h = clamp(integer(rect.h, Math.min(4, grid.rows)), 1, grid.rows)
  return {
    x: clamp(integer(rect.x, 0), 0, grid.columns - w),
    y: clamp(integer(rect.y, 0), 0, grid.rows - h),
    w,
    h,
  }
}

export interface WorkbenchSanitizeResult {
  layout: WorkbenchLayout
  warnings: string[]
}

/**
 * 迁移并宽松修复任意来源的布局文档（导入、草稿恢复）。
 * 只有“不是对象”或“版本高于当前支持”时抛错；单个面板的问题降级为警告。
 */
export const sanitizeWorkbenchLayout = (raw: unknown): WorkbenchSanitizeResult => {
  if (!isRecord(raw)) throw new WorkbenchLayoutError('布局不是对象')
  const version = raw.version === undefined ? WORKBENCH_LAYOUT_VERSION : raw.version
  if (typeof version !== 'number' || !Number.isInteger(version) || version < 1) throw new WorkbenchLayoutError('布局版本无效')
  if (version > WORKBENCH_LAYOUT_VERSION) throw new WorkbenchLayoutError(`布局版本 ${version} 高于当前支持的版本 ${WORKBENCH_LAYOUT_VERSION}`)
  const warnings: string[] = []
  const grid = sanitizeGrid(raw.grid)
  const rawSettings = isRecord(raw.settings) ? raw.settings : {}
  const settings: WorkbenchLayout['settings'] = {}
  const mobileMode = text(rawSettings.mobileMode)
  if (mobileMode && MOBILE_MODES.includes(mobileMode)) settings.mobileMode = mobileMode as WorkbenchLayout['settings']['mobileMode']
  const panelStyle = text(rawSettings.panelStyle)
  if (panelStyle && PANEL_STYLES.includes(panelStyle)) settings.panelStyle = panelStyle as WorkbenchLayout['settings']['panelStyle']
  if (text(rawSettings.minimalActionsPlacement) === 'outside') settings.minimalActionsPlacement = 'outside'
  const accent = text(rawSettings.accent)
  if (accent && ACCENT_PATTERN.test(accent)) settings.accent = accent.toLowerCase()

  const rawPanels = Array.isArray(raw.panels) ? raw.panels : []
  if (rawPanels.length > WORKBENCH_LIMITS.maxPanels) warnings.push(`面板超过 ${WORKBENCH_LIMITS.maxPanels} 个，多余部分已丢弃`)
  const seen = new Set<string>()
  const panels: WorkbenchPanel[] = []
  rawPanels.slice(0, WORKBENCH_LIMITS.maxPanels).forEach((entry, index) => {
    if (!isRecord(entry)) {
      warnings.push(`第 ${index + 1} 个面板结构无效，已跳过`)
      return
    }
    let id = text(entry.id)
    if (!PANEL_ID_PATTERN.test(id) || seen.has(id)) id = nextWorkbenchPanelId(seen)
    seen.add(id)
    const title = truncate(text(entry.title), WORKBENCH_LIMITS.maxTitle)
    const { source, warning } = sanitizeSource(entry.source)
    if (!source) {
      warnings.push(`面板「${title || id}」：${warning || '资源无效'}，已跳过`)
      return
    }
    if (warning) warnings.push(`面板「${title || id}」：${warning}`)
    const panel: WorkbenchPanel = { id, rect: clampRect(entry.rect, grid), source }
    if (title) panel.title = title
    if (entry.hiddenByDefault === true) panel.hiddenByDefault = true
    if (entry.chrome === 'minimal') panel.chrome = 'minimal'
    panels.push(panel)
  })
  const layout: WorkbenchLayout = { version: WORKBENCH_LAYOUT_VERSION, grid, settings, panels }
  if (panels.some((panel, index) => panels.slice(index + 1).some(other => rectsOverlap(panel.rect, other.rect)))) {
    const arranged = autoArrangeWorkbench(layout)
    if (!validateWorkbenchLayout(arranged)) {
      warnings.push('检测到重叠面板，已自动重新排列')
      return { layout: arranged, warnings }
    }
    warnings.push('检测到重叠面板，网格空间不足，请手动删除或调整面板后保存')
  }
  return { layout, warnings }
}

/** 保存前的严格检查（与服务端一致）；返回首个错误。 */
export const validateWorkbenchLayout = (layout: WorkbenchLayout): string | null => {
  if (layout.version !== WORKBENCH_LAYOUT_VERSION) return '布局版本无效'
  const { columns, rows, gap } = layout.grid
  if (columns !== 12 && columns !== 24) return '网格列数只能是 12 或 24'
  if (rows < WORKBENCH_LIMITS.minRows || rows > WORKBENCH_LIMITS.maxRows) return `网格行数需在 ${WORKBENCH_LIMITS.minRows}-${WORKBENCH_LIMITS.maxRows} 之间`
  if (gap < 0 || gap > WORKBENCH_LIMITS.maxGap) return '面板间距无效'
  if (!MINIMAL_ACTIONS_PLACEMENTS.includes(layout.settings.minimalActionsPlacement || '')) return '极简操作区位置无效'
  if (layout.panels.length > WORKBENCH_LIMITS.maxPanels) return `面板数量不能超过 ${WORKBENCH_LIMITS.maxPanels}`
  const seen = new Set<string>()
  for (const panel of layout.panels) {
    if (!PANEL_ID_PATTERN.test(panel.id) || seen.has(panel.id)) return '面板 ID 无效或重复'
    seen.add(panel.id)
    const { x, y, w, h } = panel.rect
    if (![x, y, w, h].every(Number.isInteger) || x < 0 || y < 0 || w < 1 || h < 1 || x + w > columns || y + h > rows) {
      return `面板「${panel.title || panel.id}」超出网格范围`
    }
    if (runes(panel.title || '').length > WORKBENCH_LIMITS.maxTitle) return '面板标题过长'
    if (panel.source.type === 'web' && panel.source.url && !normalizeWorkbenchWebUrl(panel.source.url)) return '网页地址必须是 http(s) 绝对地址'
  }
  for (let index = 0; index < layout.panels.length; index++) {
    const panel = layout.panels[index]
    for (const other of layout.panels.slice(index + 1)) {
      if (rectsOverlap(panel.rect, other.rect)) return `面板「${panel.title || panel.id}」与「${other.title || other.id}」发生重叠`
    }
  }
  return null
}

let idCounter = 0
export const nextWorkbenchPanelId = (taken: Iterable<string>, random: () => string = () => Math.random().toString(36).slice(2, 8)) => {
  const used = new Set(taken)
  for (let attempt = 0; attempt < 64; attempt++) {
    const id = `p-${random() || (++idCounter).toString(36)}`
    if (PANEL_ID_PATTERN.test(id) && !used.has(id)) return id
  }
  let fallback = used.size
  while (used.has(`p-${fallback}`)) fallback++
  return `p-${fallback}`
}

// ---------------------------------------------------------------------------
// 网格操作：全部返回新布局；无法完成时返回 null（编辑器据此提示“放不下”）。

export const rectsOverlap = (a: WorkbenchRect, b: WorkbenchRect) => (
  a.x < b.x + b.w && b.x < a.x + a.w && a.y < b.y + b.h && b.y < a.y + a.h
)

const fitsGrid = (rect: WorkbenchRect, grid: WorkbenchGrid) => (
  rect.x >= 0 && rect.y >= 0 && rect.w >= 1 && rect.h >= 1 && rect.x + rect.w <= grid.columns && rect.y + rect.h <= grid.rows
)

const collides = (rect: WorkbenchRect, panels: WorkbenchPanel[], ignore: Set<string>) => (
  panels.some(panel => !ignore.has(panel.id) && rectsOverlap(rect, panel.rect))
)

export const findFreeRect = (layout: WorkbenchLayout, w: number, h: number, ignoreId?: string): WorkbenchRect | null => {
  const { columns, rows } = layout.grid
  const width = clamp(w, 1, columns)
  const height = clamp(h, 1, rows)
  const ignore = new Set(ignoreId ? [ignoreId] : [])
  for (let y = 0; y + height <= rows; y++) {
    for (let x = 0; x + width <= columns; x++) {
      const rect = { x, y, w: width, h: height }
      if (!collides(rect, layout.panels, ignore)) return rect
    }
  }
  return null
}

export const defaultPanelSize = (layout: WorkbenchLayout, type: WorkbenchSource['type']) => {
  const unit = layout.grid.columns / 24
  const width = type === 'chat' ? 6 : type === 'theater' ? 12 : 8
  return { w: Math.max(1, Math.round(width * unit)), h: Math.min(layout.grid.rows, type === 'theater' ? 7 : 6) }
}

/** 放入新面板：先找空位，再尝试缩小，最后扩展行数。 */
export const addWorkbenchPanel = (layout: WorkbenchLayout, panel: Omit<WorkbenchPanel, 'rect'> & { rect?: Partial<WorkbenchRect> }): WorkbenchLayout | null => {
  if (layout.panels.length >= WORKBENCH_LIMITS.maxPanels) return null
  const next = cloneWorkbenchLayout(layout)
  const size = defaultPanelSize(next, panel.source.type)
  const w = panel.rect?.w ?? size.w
  const h = panel.rect?.h ?? size.h
  let rect = findFreeRect(next, w, h)
    || findFreeRect(next, Math.max(1, Math.ceil(w / 2)), Math.max(1, Math.ceil(h / 2)))
  while (!rect && next.grid.rows < WORKBENCH_LIMITS.maxRows) {
    next.grid.rows = Math.min(WORKBENCH_LIMITS.maxRows, next.grid.rows + h)
    rect = findFreeRect(next, w, h)
  }
  if (!rect) return null
  next.panels.push({ ...panel, rect } as WorkbenchPanel)
  return next
}

/** 移动面板；落点与恰好一个面板冲突且两者能互换时交换位置。 */
export const moveWorkbenchPanel = (layout: WorkbenchLayout, id: string, x: number, y: number): WorkbenchLayout | null => {
  const source = layout.panels.find(panel => panel.id === id)
  if (!source) return null
  const target: WorkbenchRect = {
    ...source.rect,
    x: clamp(Math.round(x), 0, layout.grid.columns - source.rect.w),
    y: clamp(Math.round(y), 0, layout.grid.rows - source.rect.h),
  }
  if (target.x === source.rect.x && target.y === source.rect.y) return layout
  const hits = layout.panels.filter(panel => panel.id !== id && rectsOverlap(target, panel.rect))
  const next = cloneWorkbenchLayout(layout)
  const moved = next.panels.find(panel => panel.id === id)!
  if (!hits.length) {
    moved.rect = target
    return next
  }
  if (hits.length !== 1) return null
  const other = next.panels.find(panel => panel.id === hits[0].id)!
  const swapped: WorkbenchRect = { ...other.rect, x: source.rect.x, y: source.rect.y }
  if (!fitsGrid(swapped, next.grid)) return null
  const ignore = new Set([id, other.id])
  if (collides(target, next.panels, ignore) || collides(swapped, next.panels, ignore) || rectsOverlap(target, swapped)) return null
  moved.rect = target
  other.rect = swapped
  return next
}

export const resizeWorkbenchPanel = (layout: WorkbenchLayout, id: string, w: number, h: number): WorkbenchLayout | null => {
  const panel = layout.panels.find(item => item.id === id)
  if (!panel) return null
  const rect: WorkbenchRect = {
    ...panel.rect,
    w: clamp(Math.round(w), 1, layout.grid.columns - panel.rect.x),
    h: clamp(Math.round(h), 1, layout.grid.rows - panel.rect.y),
  }
  if (rect.w === panel.rect.w && rect.h === panel.rect.h) return layout
  if (collides(rect, layout.panels, new Set([id]))) return null
  const next = cloneWorkbenchLayout(layout)
  next.panels.find(item => item.id === id)!.rect = rect
  return next
}

/** 直接设置矩形（检查器数值输入），超界或重叠时返回 null。 */
export const setWorkbenchPanelRect = (layout: WorkbenchLayout, id: string, rect: WorkbenchRect): WorkbenchLayout | null => {
  const normalized = { x: Math.round(rect.x), y: Math.round(rect.y), w: Math.round(rect.w), h: Math.round(rect.h) }
  if (!fitsGrid(normalized, layout.grid) || collides(normalized, layout.panels, new Set([id]))) return null
  const next = cloneWorkbenchLayout(layout)
  const panel = next.panels.find(item => item.id === id)
  if (!panel) return null
  panel.rect = normalized
  return next
}

/** 按当前阅读顺序（上→下、左→右）重新首次适配排布，尺寸保持不变。 */
export const autoArrangeWorkbench = (layout: WorkbenchLayout): WorkbenchLayout => {
  const next = cloneWorkbenchLayout(layout)
  const ordered = [...next.panels].sort((a, b) => a.rect.y - b.rect.y || a.rect.x - b.rect.x)
  const placed: WorkbenchPanel[] = []
  for (const panel of ordered) {
    const probe: WorkbenchLayout = { ...next, panels: placed }
    let rect = findFreeRect(probe, panel.rect.w, panel.rect.h)
    while (!rect && probe.grid.rows < WORKBENCH_LIMITS.maxRows) {
      probe.grid = { ...probe.grid, rows: probe.grid.rows + 1 }
      next.grid = probe.grid
      rect = findFreeRect(probe, panel.rect.w, panel.rect.h)
    }
    panel.rect = rect || { ...panel.rect, x: 0, y: 0 }
    placed.push(panel)
  }
  // 保留数组顺序（移动端标签页顺序），只改坐标。
  return next
}

export const changeWorkbenchColumns = (layout: WorkbenchLayout, columns: 12 | 24): WorkbenchLayout => {
  if (layout.grid.columns === columns) return layout
  const ratio = columns / layout.grid.columns
  const next = cloneWorkbenchLayout(layout)
  next.grid.columns = columns
  next.panels.forEach((panel) => {
    const w = clamp(Math.round(panel.rect.w * ratio), 1, columns)
    panel.rect = { ...panel.rect, w, x: clamp(Math.round(panel.rect.x * ratio), 0, columns - w) }
  })
  const hasOverlap = next.panels.some((panel, index) => next.panels.slice(index + 1).some(other => rectsOverlap(panel.rect, other.rect)))
  return hasOverlap ? autoArrangeWorkbench(next) : next
}

/** 行数不能小于现有面板占用的最低行。 */
export const minimumWorkbenchRows = (layout: WorkbenchLayout) => Math.max(
  WORKBENCH_LIMITS.minRows,
  ...layout.panels.map(panel => panel.rect.y + panel.rect.h),
)

export const changeWorkbenchRows = (layout: WorkbenchLayout, rows: number): WorkbenchLayout | null => {
  const value = Math.round(rows)
  if (value < minimumWorkbenchRows(layout) || value > WORKBENCH_LIMITS.maxRows) return null
  const next = cloneWorkbenchLayout(layout)
  next.grid.rows = value
  return next
}

export const duplicateWorkbenchPanel = (layout: WorkbenchLayout, id: string): { layout: WorkbenchLayout; panelId: string } | null => {
  const panel = layout.panels.find(item => item.id === id)
  if (!panel) return null
  const panelId = nextWorkbenchPanelId(layout.panels.map(item => item.id))
  const copy = JSON.parse(JSON.stringify(panel)) as WorkbenchPanel
  copy.id = panelId
  copy.title = truncate(`${panel.title || '面板'} 副本`, WORKBENCH_LIMITS.maxTitle)
  const next = addWorkbenchPanel(layout, { ...copy, rect: { w: panel.rect.w, h: panel.rect.h } })
  return next ? { layout: next, panelId } : null
}

export const removeWorkbenchPanel = (layout: WorkbenchLayout, id: string): WorkbenchLayout => {
  const next = cloneWorkbenchLayout(layout)
  next.panels = next.panels.filter(panel => panel.id !== id)
  return next
}

/** 调整数组顺序（移动端标签/堆叠顺序与键盘焦点顺序）。 */
export const reorderWorkbenchPanel = (layout: WorkbenchLayout, id: string, delta: number): WorkbenchLayout => {
  const index = layout.panels.findIndex(panel => panel.id === id)
  const target = clamp(index + delta, 0, layout.panels.length - 1)
  if (index < 0 || target === index) return layout
  const next = cloneWorkbenchLayout(layout)
  const [panel] = next.panels.splice(index, 1)
  next.panels.splice(target, 0, panel)
  return next
}

export const updateWorkbenchPanel = (layout: WorkbenchLayout, id: string, patch: Partial<Omit<WorkbenchPanel, 'id' | 'rect'>>): WorkbenchLayout => {
  const next = cloneWorkbenchLayout(layout)
  const panel = next.panels.find(item => item.id === id)
  if (!panel) return layout
  Object.assign(panel, JSON.parse(JSON.stringify(patch)))
  if (!panel.title) delete panel.title
  if (!panel.hiddenByDefault) delete panel.hiddenByDefault
  if (!panel.chrome) delete panel.chrome
  return next
}

// ---------------------------------------------------------------------------
// 展示状态

export const isWorkbenchPanelHidden = (panel: WorkbenchPanel, presentation?: WorkbenchPresentation | null) => {
  const override = presentation?.visibility?.[panel.id]
  if (override === 'shown') return false
  if (override === 'hidden') return true
  return panel.hiddenByDefault === true
}

/** 管理者“玩家视角预览”：与服务端 filterChannelWorkbenchForViewer 相同的过滤。 */
export const filterWorkbenchForPlayer = (layout: WorkbenchLayout, presentation?: WorkbenchPresentation | null): WorkbenchLayout => ({
  ...layout,
  panels: layout.panels.filter(panel => !isWorkbenchPanelHidden(panel, presentation)),
})

export const layoutsEqual = (a: WorkbenchLayout | null | undefined, b: WorkbenchLayout | null | undefined) => (
  JSON.stringify(a ?? null) === JSON.stringify(b ?? null)
)
