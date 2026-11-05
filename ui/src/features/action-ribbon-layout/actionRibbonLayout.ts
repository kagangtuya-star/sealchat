// “更多跑团功能”入口布局：平台多方案 + 世界“引用 + 差异覆写”。
// 布局只决定入口是否显示及先后顺序；入口是否存在、是否可用仍由 ChatActionRibbon 的业务条件决定，
// 因此 applyActionRibbonLayout 只会过滤和重排已有按钮，绝不会凭配置新增按钮。
// 未知 key（来自更新版本）在规范化时保留、在应用时忽略；不在 order 中的新入口按默认顺序追加到末尾。

export const ACTION_RIBBON_LAYOUT_VERSION = 1

const MAX_KEYS = 200
const MAX_KEY_LENGTH = 64
const MAX_PRESET_ID_LENGTH = 64
export const MAX_ACTION_RIBBON_PRESETS = 50
export const MAX_ACTION_RIBBON_PRESET_NAME = 32
const KEY_PATTERN = /^[A-Za-z0-9_:.-]+$/

export interface ActionRibbonLayoutPreset {
  id: string
  name: string
  order: string[]
  hidden: string[]
  updatedAt: number
}

export interface ActionRibbonLayoutConfig {
  version: number
  /** 为空表示平台默认使用系统内置顺序。 */
  defaultPresetId: string
  presets: ActionRibbonLayoutPreset[]
}

export type WorldActionRibbonLayoutMode = 'inherit' | 'preset' | 'custom'

export interface WorldActionRibbonLayout {
  version: number
  mode: WorldActionRibbonLayoutMode
  /** preset：引用的平台方案；custom：基础方案（空为平台默认）。 */
  presetId?: string
  /** custom：完整排序覆写；空表示沿用基础方案顺序。 */
  order?: string[]
  /** custom：相对基础方案额外隐藏的入口。 */
  hide?: string[]
  /** custom：相对基础方案重新显示的入口。 */
  show?: string[]
}

/** 布局的有效形态：order 为空表示保持默认顺序。 */
export interface ActionRibbonLayoutShape {
  order: string[]
  hidden: string[]
}

export interface ResolvedActionRibbonLayout extends ActionRibbonLayoutShape {
  mode: WorldActionRibbonLayoutMode
  /** 实际作为基础的平台方案；null 表示系统内置顺序。 */
  basePreset: ActionRibbonLayoutPreset | null
  /** 平台默认方案（可能为 null）。 */
  platformDefault: ActionRibbonLayoutPreset | null
  /** 世界引用的方案已不存在，已回退到平台默认。 */
  missingPresetId: string
}

export const SYSTEM_DEFAULT_LAYOUT: Readonly<ActionRibbonLayoutShape> = Object.freeze({ order: [], hidden: [] })

const isRecord = (value: unknown): value is Record<string, unknown> =>
  !!value && typeof value === 'object' && !Array.isArray(value)

export const normalizeActionRibbonKeys = (raw: unknown): string[] => {
  if (!Array.isArray(raw)) return []
  const out: string[] = []
  const seen = new Set<string>()
  for (const item of raw) {
    if (typeof item !== 'string') continue
    const key = item.trim()
    if (!key || key.length > MAX_KEY_LENGTH || !KEY_PATTERN.test(key) || seen.has(key)) continue
    seen.add(key)
    out.push(key)
    if (out.length >= MAX_KEYS) break
  }
  return out
}

const normalizePresetId = (raw: unknown) => {
  const id = typeof raw === 'string' ? raw.trim() : ''
  return id.length > MAX_PRESET_ID_LENGTH ? '' : id
}

const normalizePresetName = (raw: unknown) => {
  const name = typeof raw === 'string' ? raw.trim() : ''
  return Array.from(name).slice(0, MAX_ACTION_RIBBON_PRESET_NAME).join('') || '未命名方案'
}

export const normalizeActionRibbonLayoutConfig = (raw: unknown): ActionRibbonLayoutConfig => {
  const source = isRecord(raw) ? raw : {}
  const presets: ActionRibbonLayoutPreset[] = []
  const seen = new Set<string>()
  for (const item of Array.isArray(source.presets) ? source.presets : []) {
    if (!isRecord(item)) continue
    const id = normalizePresetId(item.id)
    if (!id || seen.has(id)) continue
    seen.add(id)
    presets.push({
      id,
      name: normalizePresetName(item.name),
      order: normalizeActionRibbonKeys(item.order),
      hidden: normalizeActionRibbonKeys(item.hidden),
      updatedAt: Number(item.updatedAt) || 0,
    })
    if (presets.length >= MAX_ACTION_RIBBON_PRESETS) break
  }
  const defaultPresetId = normalizePresetId(source.defaultPresetId)
  return {
    version: ACTION_RIBBON_LAYOUT_VERSION,
    defaultPresetId: seen.has(defaultPresetId) ? defaultPresetId : '',
    presets,
  }
}

export const normalizeWorldActionRibbonLayout = (raw: unknown): WorldActionRibbonLayout => {
  const source = isRecord(raw) ? raw : {}
  const presetId = normalizePresetId(source.presetId)
  if (source.mode === 'preset' && presetId) {
    return { version: ACTION_RIBBON_LAYOUT_VERSION, mode: 'preset', presetId }
  }
  if (source.mode === 'custom') {
    const hide = normalizeActionRibbonKeys(source.hide)
    const hideSet = new Set(hide)
    return {
      version: ACTION_RIBBON_LAYOUT_VERSION,
      mode: 'custom',
      presetId,
      order: normalizeActionRibbonKeys(source.order),
      hide,
      show: normalizeActionRibbonKeys(source.show).filter(key => !hideSet.has(key)),
    }
  }
  return { version: ACTION_RIBBON_LAYOUT_VERSION, mode: 'inherit' }
}

export const findActionRibbonPreset = (config: ActionRibbonLayoutConfig, presetId?: string) =>
  (presetId && config.presets.find(item => item.id === presetId)) || null

const shapeOf = (preset: ActionRibbonLayoutPreset | null): ActionRibbonLayoutShape =>
  preset ? { order: preset.order, hidden: preset.hidden } : SYSTEM_DEFAULT_LAYOUT

/**
 * 解析最终布局。平台方案按 id 引用，因此平台修改方案后引用它的世界立即同步；
 * 被引用的方案删除后回退到平台默认（再缺失则回到系统内置顺序）。
 */
export const resolveActionRibbonLayout = (platformRaw: unknown, worldRaw: unknown): ResolvedActionRibbonLayout => {
  const platform = normalizeActionRibbonLayoutConfig(platformRaw)
  const world = normalizeWorldActionRibbonLayout(worldRaw)
  const platformDefault = findActionRibbonPreset(platform, platform.defaultPresetId)
  let basePreset = platformDefault
  let missingPresetId = ''
  if (world.mode !== 'inherit' && world.presetId) {
    const referenced = findActionRibbonPreset(platform, world.presetId)
    if (referenced) {
      basePreset = referenced
    } else {
      missingPresetId = world.presetId
    }
  }
  const base = shapeOf(basePreset)
  const result: ResolvedActionRibbonLayout = {
    mode: world.mode,
    basePreset,
    platformDefault,
    missingPresetId,
    order: base.order,
    hidden: base.hidden,
  }
  if (world.mode === 'custom') {
    const hidden = new Set(base.hidden)
    for (const key of world.hide || []) hidden.add(key)
    for (const key of world.show || []) hidden.delete(key)
    result.order = world.order?.length ? world.order : base.order
    result.hidden = Array.from(hidden)
  }
  return result
}

/**
 * 在按钮数组最终形成阶段应用布局：先去掉隐藏项，再按 order 稳定排序；
 * order 未提及的入口保持默认相对顺序追加在末尾。布局为空时原样返回同一数组。
 */
export const applyActionRibbonLayout = <T extends { key: string }>(buttons: T[], layout: ActionRibbonLayoutShape | null | undefined): T[] => {
  if (!layout || (!layout.order.length && !layout.hidden.length)) return buttons
  const hidden = new Set(layout.hidden)
  const visible = hidden.size ? buttons.filter(button => !hidden.has(button.key)) : buttons.slice()
  if (!layout.order.length) return visible
  const rank = new Map(layout.order.map((key, index) => [key, index]))
  const fallback = layout.order.length
  return visible
    .map((button, index) => ({ button, index, rank: rank.get(button.key) ?? fallback }))
    .sort((a, b) => a.rank - b.rank || a.index - b.index)
    .map(item => item.button)
}

/** 编辑器视角的完整顺序：已知 key 按 order 排列，其余已知 key 按默认顺序追加；未知 key 不出现。 */
export const completeActionRibbonOrder = (order: string[], knownKeys: string[]) => {
  const known = new Set(knownKeys)
  const head = order.filter(key => known.has(key))
  const headSet = new Set(head)
  return [...head, ...knownKeys.filter(key => !headSet.has(key))]
}

const unknownKeys = (keys: string[] | undefined, known: Set<string>) => (keys || []).filter(key => !known.has(key))

/** 已知入口重排时，把旧配置中的未知入口留在原有槽位，避免旧客户端破坏新版入口顺序。 */
const mergeKnownOrderWithPreviousUnknown = (previous: string[], editedKnown: string[], known: Set<string>) => {
  const remaining = [...editedKnown]
  const merged: string[] = []
  for (const key of previous) {
    if (known.has(key)) {
      const next = remaining.shift()
      if (next) merged.push(next)
    } else {
      merged.push(key)
    }
  }
  return [...merged, ...remaining]
}

/** 写回编辑结果时保留未知 key（来自更新版本），避免旧前端编辑后把新版入口的设置抹掉。 */
export const mergeEditedActionRibbonShape = (
  edited: ActionRibbonLayoutShape,
  previous: ActionRibbonLayoutShape,
  knownKeys: string[],
): ActionRibbonLayoutShape => {
  const known = new Set(knownKeys)
  return {
    order: [...edited.order.filter(key => known.has(key)), ...unknownKeys(previous.order, known)],
    hidden: [...edited.hidden.filter(key => known.has(key)), ...unknownKeys(previous.hidden, known)],
  }
}

const sameList = (a: string[], b: string[]) => a.length === b.length && a.every((key, index) => key === b[index])

/**
 * 把世界编辑结果转换为相对基础方案的差异覆写。
 * 只有排序与基础方案不同才保存 order；显隐只保存差异。
 */
export const buildWorldCustomLayout = (
  presetId: string,
  base: ActionRibbonLayoutShape,
  edited: ActionRibbonLayoutShape,
  knownKeys: string[],
  previous?: WorldActionRibbonLayout | null,
): WorldActionRibbonLayout => {
  const known = new Set(knownKeys)
  const baseOrder = completeActionRibbonOrder(base.order, knownKeys)
  const editedOrder = completeActionRibbonOrder(edited.order, knownKeys)
  const baseHidden = new Set(base.hidden.filter(key => known.has(key)))
  const editedHidden = new Set(edited.hidden.filter(key => known.has(key)))
  const keep = previous?.mode === 'custom' ? previous : null
  const previousOrder = keep?.order || []
  const hasUnknownOrder = unknownKeys(previousOrder, known).length > 0
  const sameKnownOrder = sameList(baseOrder, editedOrder)
  let order = sameKnownOrder ? [] : [...editedOrder]
  if (hasUnknownOrder) {
    order = sameKnownOrder && keep?.presetId === presetId
      ? [...previousOrder]
      : mergeKnownOrderWithPreviousUnknown(previousOrder, editedOrder, known)
  }
  return {
    version: ACTION_RIBBON_LAYOUT_VERSION,
    mode: 'custom',
    presetId,
    order,
    hide: [...knownKeys.filter(key => editedHidden.has(key) && !baseHidden.has(key)), ...unknownKeys(keep?.hide, known)],
    show: [...knownKeys.filter(key => baseHidden.has(key) && !editedHidden.has(key)), ...unknownKeys(keep?.show, known)],
  }
}

/** 世界自定义相对基础方案的覆写统计，用于界面提示。 */
export const countWorldOverrides = (world: WorldActionRibbonLayout) => ({
  reordered: world.mode === 'custom' && !!world.order?.length,
  visibility: world.mode === 'custom' ? (world.hide?.length || 0) + (world.show?.length || 0) : 0,
})

// ---- 导入导出 ----

export const ACTION_RIBBON_PRESET_FILE_TYPE = 'sealchat.action-ribbon-layout.platform-preset'
export const ACTION_RIBBON_PRESETS_FILE_TYPE = 'sealchat.action-ribbon-layout.platform-presets'
export const ACTION_RIBBON_WORLD_FILE_TYPE = 'sealchat.action-ribbon-layout.world'

const FILE_TYPE_LABELS: Record<string, string> = {
  [ACTION_RIBBON_PRESET_FILE_TYPE]: '平台布局方案',
  [ACTION_RIBBON_PRESETS_FILE_TYPE]: '平台布局方案集合',
  [ACTION_RIBBON_WORLD_FILE_TYPE]: '世界布局配置',
}

export const exportActionRibbonPreset = (preset: ActionRibbonLayoutPreset) => ({
  type: ACTION_RIBBON_PRESET_FILE_TYPE,
  version: ACTION_RIBBON_LAYOUT_VERSION,
  preset: { name: preset.name, order: [...preset.order], hidden: [...preset.hidden] },
})

export const exportActionRibbonPresets = (config: ActionRibbonLayoutConfig) => ({
  type: ACTION_RIBBON_PRESETS_FILE_TYPE,
  version: ACTION_RIBBON_LAYOUT_VERSION,
  defaultPresetId: config.defaultPresetId,
  presets: config.presets.map(item => ({ id: item.id, name: item.name, order: [...item.order], hidden: [...item.hidden] })),
})

export const exportWorldActionRibbonLayout = (layout: WorldActionRibbonLayout, presetName?: string) => ({
  type: ACTION_RIBBON_WORLD_FILE_TYPE,
  version: ACTION_RIBBON_LAYOUT_VERSION,
  // 方案名称仅供人工识别；导入时仍以 presetId 引用目标平台的方案。
  presetName: presetName || undefined,
  layout: normalizeWorldActionRibbonLayout(layout),
})

const parseLayoutFile = (text: string, expected: string[]) => {
  let data: unknown
  try {
    data = JSON.parse(text)
  } catch {
    throw new Error('文件不是有效的 JSON')
  }
  if (!isRecord(data) || typeof data.type !== 'string') {
    throw new Error('无法识别的布局文件：缺少 type 标识')
  }
  if (!expected.includes(data.type)) {
    const label = FILE_TYPE_LABELS[data.type]
    throw new Error(label ? `这是「${label}」文件，不能在此导入` : '无法识别的布局文件类型')
  }
  const version = Number(data.version)
  if (!Number.isInteger(version) || version < 1) {
    throw new Error('布局文件缺少有效的版本号')
  }
  // 更高版本按当前结构兼容读取：未知 key 保留，未知字段忽略。
  return data
}

export type ImportedPlatformLayout =
  | { kind: 'preset'; preset: Omit<ActionRibbonLayoutPreset, 'id' | 'updatedAt'> }
  | { kind: 'presets'; config: ActionRibbonLayoutConfig }

export const parsePlatformActionRibbonLayoutFile = (text: string): ImportedPlatformLayout => {
  const data = parseLayoutFile(text, [ACTION_RIBBON_PRESET_FILE_TYPE, ACTION_RIBBON_PRESETS_FILE_TYPE])
  if (data.type === ACTION_RIBBON_PRESET_FILE_TYPE) {
    if (!isRecord(data.preset)) throw new Error('布局方案文件缺少 preset 内容')
    return {
      kind: 'preset',
      preset: {
        name: normalizePresetName(data.preset.name),
        order: normalizeActionRibbonKeys(data.preset.order),
        hidden: normalizeActionRibbonKeys(data.preset.hidden),
      },
    }
  }
  if (!Array.isArray(data.presets)) throw new Error('方案集合文件缺少 presets 列表')
  return { kind: 'presets', config: normalizeActionRibbonLayoutConfig(data) }
}

export const parseWorldActionRibbonLayoutFile = (text: string) => {
  const data = parseLayoutFile(text, [ACTION_RIBBON_WORLD_FILE_TYPE])
  if (!isRecord(data.layout)) throw new Error('世界布局文件缺少 layout 内容')
  return {
    layout: normalizeWorldActionRibbonLayout(data.layout),
    presetName: typeof data.presetName === 'string' ? data.presetName : '',
  }
}

export const createActionRibbonPresetId = () =>
  `arl_${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}`
