import {
  buildInternalSurfaceResourceKey,
  generateInternalSurfaceLink,
  type InternalSurfaceLinkParams,
} from '@/utils/internalSurfaceLink'
import { buildEmbedChatWindowUrl } from '@/views/theater/host/theater-floating-window'
import { normalizeWorkbenchWebUrl } from './layout'
import type { WorkbenchPanel, WorkbenchSource } from './types'

// 资源描述 → 运行时地址。所有内部地址都由既有构造器生成：
// Internal Surface 走 generateInternalSurfaceLink（保留嵌套 iframe 的 __sc_surface 标记），
// 聊天走 Embed Chat，小剧场走 /theater 路由；外部网页只做 http(s) 校验并强制沙箱。

/** 与 IForm 外链 iframe 相同的沙箱；同源地址在解析时已被拒绝，因此 allow-same-origin 只作用于对方站点自身。 */
export const WORKBENCH_WEB_SANDBOX = 'allow-same-origin allow-scripts allow-forms allow-pointer-lock allow-popups'

export interface WorkbenchSourceContext {
  worldId: string
  channelId: string
  origin?: string
}

export type WorkbenchResolvedSource =
  | { kind: 'surface'; url: string; floatingKey: string; params: InternalSurfaceLinkParams }
  | { kind: 'chat'; url: string; externalUrl: string; floatingKey: string; targetChannelId: string }
  | { kind: 'theater'; url: string; externalUrl: string; targetChannelId: string }
  | { kind: 'web'; url: string; display: 'embed' | 'link'; floatingKey: string }
  | { kind: 'unbound'; reason: string }
  | { kind: 'invalid'; reason: string }

export const workbenchSourceChannelId = (source: WorkbenchSource, context: WorkbenchSourceContext) => (
  (source.channelId || '').trim() || context.channelId
)

export const workbenchSurfaceResourceId = (source: WorkbenchSource) => (
  source.surface === 'clue-board' ? (source.resourceId || 'main') : (source.resourceId || '').trim()
)

const currentOrigin = (context: WorkbenchSourceContext) => (
  context.origin ?? (typeof window !== 'undefined' ? window.location.origin : '')
)

/** 外部网页：只接受跨源 http(s)，同源地址应改用聊天 / 内部面板。 */
export const resolveWorkbenchWebUrl = (raw: string | undefined, origin: string) => {
  const url = normalizeWorkbenchWebUrl(raw)
  if (!url) return { url: '', reason: '网页地址必须是 http(s) 绝对地址' }
  try {
    if (origin && new URL(url).origin === origin) return { url: '', reason: '不能嵌入 SealChat 自身页面，请改用聊天或内部面板' }
  } catch {
    return { url: '', reason: '网页地址无效' }
  }
  return { url, reason: '' }
}

const buildTheaterUrl = (worldId: string, channelId: string, embedded: boolean) => {
  const url = new URL(window.location.href.split('#', 1)[0])
  const params = new URLSearchParams({ worldId, channelId })
  if (embedded) {
    // 面板内默认只展示舞台；不启用移动端画中画，音频由布局频道主窗口负责。
    params.set('chat', '0')
    params.set('pip', '0')
    params.set('audioOwner', '0')
  }
  url.hash = `/theater?${params.toString()}`
  return url.toString()
}

export const resolveWorkbenchSource = (panel: WorkbenchPanel, context: WorkbenchSourceContext): WorkbenchResolvedSource => {
  const source = panel.source
  if (!context.worldId || !context.channelId) return { kind: 'invalid', reason: '布局频道上下文缺失' }
  switch (source.type) {
    case 'surface': {
      if (!source.surface) return { kind: 'invalid', reason: '内部面板类型缺失' }
      const id = workbenchSurfaceResourceId(source)
      if (!id) return { kind: 'unbound', reason: '尚未绑定资源' }
      const params: InternalSurfaceLinkParams = {
        type: source.surface,
        id,
        worldId: context.worldId,
        channelId: workbenchSourceChannelId(source, context),
      }
      return { kind: 'surface', url: generateInternalSurfaceLink(params), floatingKey: buildInternalSurfaceResourceKey(params), params }
    }
    case 'chat': {
      const targetChannelId = workbenchSourceChannelId(source, context)
      return {
        kind: 'chat',
        url: buildEmbedChatWindowUrl(context.worldId, targetChannelId),
        externalUrl: buildEmbedChatWindowUrl(context.worldId, targetChannelId, false),
        floatingKey: `chat-window:workbench:${panel.id}`,
        targetChannelId,
      }
    }
    case 'theater': {
      const targetChannelId = workbenchSourceChannelId(source, context)
      return {
        kind: 'theater',
        url: buildTheaterUrl(context.worldId, targetChannelId, true),
        externalUrl: buildTheaterUrl(context.worldId, targetChannelId, false),
        targetChannelId,
      }
    }
    case 'web': {
      if (!source.url) return { kind: 'unbound', reason: '尚未填写网页地址' }
      const { url, reason } = resolveWorkbenchWebUrl(source.url, currentOrigin(context))
      if (!url) return { kind: 'invalid', reason }
      return { kind: 'web', url, display: source.display === 'link' ? 'link' : 'embed', floatingKey: `custom:workbench:${panel.id}` }
    }
    default:
      return { kind: 'invalid', reason: '不支持的面板来源' }
  }
}

export const WORKBENCH_SURFACE_LABELS: Record<string, string> = {
  character: '人物卡',
  iform: 'IForm',
  note: '便签',
  clue: '线索',
  'clue-board': '线索画板',
}

export const workbenchSourceTypeLabel = (source: WorkbenchSource) => {
  switch (source.type) {
    case 'surface': return WORKBENCH_SURFACE_LABELS[source.surface || ''] || '内部面板'
    case 'chat': return '聊天'
    case 'theater': return '小剧场'
    case 'web': return '外部网页'
    default: return '面板'
  }
}

export const workbenchPanelTitle = (panel: WorkbenchPanel) => (
  panel.title || panel.source.label || workbenchSourceTypeLabel(panel.source)
)
