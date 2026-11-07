// These strings form the public CSS targeting contract. global is a package
// target only; it never represents a route. It covers ordinary surfaces only:
// public surfaces need an explicit target and internal is never customizable.
export const UI_SURFACES = [
  'auth', 'lobby', 'world-detail', 'chat', 'split', 'embed', 'theater',
  'observer', 'invite', 'log-layout', 'log-public', 'status', 'internal', 'global',
] as const

export type UISurface = typeof UI_SURFACES[number]
export type UIRuntimeSurface = Exclude<UISurface, 'global'>

export const UI_PUBLIC_SURFACES: readonly UISurface[] = ['observer', 'embed', 'log-public']

export const UI_SURFACE_LABELS: Record<UISurface, string> = {
  global: '全部普通页面',
  auth: '登录注册',
  lobby: '世界大厅与关于页',
  'world-detail': '世界详情',
  chat: '聊天',
  split: '分屏',
  theater: '小剧场',
  invite: '邀请',
  'log-layout': 'LOG 排版',
  status: '状态页',
  observer: 'OB 旁观',
  embed: '嵌入',
  'log-public': 'LOG 公开页',
  internal: '内部页面',
}

export function surfaceCoveredByGlobal(surface: UISurface): boolean {
  return surface !== 'global' && surface !== 'internal' && !UI_PUBLIC_SURFACES.includes(surface)
}
