import { api } from '@/stores/_config'

export type TypingHeroesPeriodKind = 'week' | 'month'
export type TypingHeroesMetric = 'chars' | 'messages' | 'speed' | 'dailyAvg'
export type TypingHeroesSubject = 'user' | 'identity'
export type TypingHeroesSelfStatus = 'ranked' | 'no_data' | 'not_eligible'
export type TypingHeroesEntryKind = 'self' | 'anonymous' | 'public' | 'identity'

export const TYPING_HEROES_METRICS: TypingHeroesMetric[] = ['chars', 'messages', 'speed', 'dailyAvg']
export const TYPING_HEROES_PUBLIC_PREF_KEY = 'typing_heroes_public'
export const TYPING_HEROES_PREFS_KEY = 'sc-typing-heroes-prefs'

export interface TypingHeroesStats {
  totalChars: number
  totalMessages: number
  typingSpeed: number
  dailyAvg: number
  activeDays: number
  activeMinutes: number
}

export interface TypingHeroesEntry {
  key: string
  rank: number
  value: number
  kind: TypingHeroesEntryKind
  name: string
  username?: string
  color?: string
}

export interface TypingHeroesIdentityInfo {
  id: string
  displayName: string
  color?: string
  channelId: string
  channelName: string
  worldId: string
  worldName: string
}

export interface TypingHeroesBoard {
  period: { kind: TypingHeroesPeriodKind; offset: number; start: number; end: number; tzOffset: number; current: boolean }
  metric: TypingHeroesMetric
  subject: TypingHeroesSubject
  participants: number
  me: {
    status: TypingHeroesSelfStatus
    rank?: number
    value: number
    beatPercent?: number
    gapToPrev?: number
    stats: TypingHeroesStats
  }
  entries: TypingHeroesEntry[]
  identity?: TypingHeroesIdentityInfo
  isPublic: boolean
  speedMinActiveMinutes: number
  generatedAt: number
}

export interface TypingHeroesQuery {
  period: TypingHeroesPeriodKind
  offset: number
  metric: TypingHeroesMetric
  subject: TypingHeroesSubject
  identityId?: string
}

export interface TypingHeroesPrefs {
  period: TypingHeroesPeriodKind
  metric: TypingHeroesMetric
  subject: TypingHeroesSubject
  identityId: string
}

export const DEFAULT_TYPING_HEROES_PREFS: TypingHeroesPrefs = {
  period: 'week',
  metric: 'chars',
  subject: 'user',
  identityId: '',
}

export function normalizeTypingHeroesPrefs(raw: unknown): TypingHeroesPrefs {
  const prefs = { ...DEFAULT_TYPING_HEROES_PREFS }
  if (!raw || typeof raw !== 'object') return prefs
  const value = raw as Record<string, unknown>
  if (value.period === 'week' || value.period === 'month') prefs.period = value.period
  if (TYPING_HEROES_METRICS.includes(value.metric as TypingHeroesMetric)) prefs.metric = value.metric as TypingHeroesMetric
  if (value.subject === 'user' || value.subject === 'identity') prefs.subject = value.subject
  if (typeof value.identityId === 'string') prefs.identityId = value.identityId.slice(0, 100)
  return prefs
}

/** 当前浏览器时区相对 UTC 的分钟数，用于服务端划分周/月与自然日。 */
export function currentTZOffsetMinutes(date = new Date()): number {
  return -date.getTimezoneOffset()
}

export function currentTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || ''
  } catch {
    return ''
  }
}

export async function fetchTypingHeroesBoard(query: TypingHeroesQuery): Promise<TypingHeroesBoard> {
  const params: Record<string, string | number> = {
    period: query.period,
    offset: query.offset,
    metric: query.metric,
    subject: query.subject,
    tzOffset: currentTZOffsetMinutes(),
  }
  const timezone = currentTimezone()
  if (timezone) params.timezone = timezone
  if (query.subject === 'identity' && query.identityId) params.identityId = query.identityId
  const res = await api.get<TypingHeroesBoard>('api/v1/user/input-stats/heroes', { params })
  return res.data
}

export async function fetchTypingHeroesIdentities(): Promise<TypingHeroesIdentityInfo[]> {
  const res = await api.get<{ items?: TypingHeroesIdentityInfo[] }>('api/v1/user/input-stats/heroes/identities')
  return Array.isArray(res.data?.items) ? res.data.items : []
}

export async function saveTypingHeroesPublic(value: boolean): Promise<void> {
  await api.post('api/v1/user/preferences', { key: TYPING_HEROES_PUBLIC_PREF_KEY, value: value ? 'true' : 'false' })
}

export function isTypingHeroesIdentityUnavailable(err: unknown): boolean {
  const resp = (err as { response?: { status?: number; data?: { code?: string } } })?.response
  return resp?.status === 403 && resp?.data?.code === 'identity_unavailable'
}

/** 周期起止（end 不包含）在浏览器本地时间下的显示文本。 */
export function formatTypingHeroesPeriod(kind: TypingHeroesPeriodKind, start: number, end: number, locale: string): string {
  const startDate = new Date(start)
  if (kind === 'month') {
    return new Intl.DateTimeFormat(locale, { year: 'numeric', month: 'long' }).format(startDate)
  }
  const last = new Date(end - 1)
  const sameYear = startDate.getFullYear() === new Date().getFullYear() && last.getFullYear() === startDate.getFullYear()
  const fmt = new Intl.DateTimeFormat(locale, sameYear ? { month: 'numeric', day: 'numeric' } : { year: 'numeric', month: 'numeric', day: 'numeric' })
  return `${fmt.format(startDate)} – ${fmt.format(last)}`
}

export function formatTypingHeroesValue(metric: TypingHeroesMetric, value: number | undefined): string {
  const v = Number(value) || 0
  if (metric === 'speed' || metric === 'dailyAvg') {
    return v >= 10000 ? Math.round(v).toLocaleString() : v.toFixed(1)
  }
  return Math.round(v).toLocaleString()
}

export function typingHeroesStatValue(stats: TypingHeroesStats | undefined, metric: TypingHeroesMetric): number {
  if (!stats) return 0
  switch (metric) {
    case 'messages': return stats.totalMessages
    case 'speed': return stats.typingSpeed
    case 'dailyAvg': return stats.dailyAvg
    default: return stats.totalChars
  }
}

export type TypingHeroesTier = 'champion' | 'blazing' | 'steady' | 'rising' | 'warmup'

export function typingHeroesTier(rank: number | undefined, beatPercent: number | undefined, participants: number): TypingHeroesTier {
  if (rank === 1 && participants > 1) return 'champion'
  const beat = beatPercent ?? 0
  if (beat >= 90) return 'blazing'
  if (beat >= 60) return 'steady'
  if (beat >= 30) return 'rising'
  return 'warmup'
}

/** 附近排名条形图宽度（相对窗口内最大值），保留最小可见宽度。 */
export function typingHeroesBarPercent(value: number, entries: Pick<TypingHeroesEntry, 'value'>[]): number {
  const max = entries.reduce((acc, e) => Math.max(acc, Number(e.value) || 0), 0)
  if (max <= 0) return 0
  return Math.max(4, Math.round(((Number(value) || 0) / max) * 100))
}

export interface TypingHeroesIdentityOptionGroup {
  type: 'group'
  key: string
  label: string
  children: { label: string; value: string; channelName: string; color?: string }[]
}

export function buildTypingHeroesIdentityOptions(items: TypingHeroesIdentityInfo[], unknownWorld: string): TypingHeroesIdentityOptionGroup[] {
  const groups = new Map<string, TypingHeroesIdentityOptionGroup>()
  for (const item of items) {
    let group = groups.get(item.worldId)
    if (!group) {
      group = { type: 'group', key: `world:${item.worldId}`, label: item.worldName || unknownWorld, children: [] }
      groups.set(item.worldId, group)
    }
    group.children.push({ label: item.displayName, value: item.id, channelName: item.channelName, color: item.color })
  }
  return [...groups.values()]
}

/** 优先恢复记住的角色，其次选当前世界的第一个角色，最后选列表第一个。 */
export function pickTypingHeroesIdentity(items: TypingHeroesIdentityInfo[], rememberedId: string, currentWorldId?: string): string {
  if (rememberedId && items.some(item => item.id === rememberedId)) return rememberedId
  if (currentWorldId) {
    const inWorld = items.find(item => item.worldId === currentWorldId)
    if (inWorld) return inWorld.id
  }
  return items[0]?.id || ''
}
