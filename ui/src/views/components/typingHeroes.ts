import { api } from '@/stores/_config'

export type TypingHeroesPeriodKind = 'day' | 'week' | 'month'
export type TypingHeroesMetric = 'chars' | 'messages' | 'speed' | 'dailyAvg'
export type TypingHeroesScope = 'all' | 'ic'
export type TypingHeroesSelfStatus = 'ranked' | 'no_data' | 'not_eligible'
export type TypingHeroesEntryKind = 'self' | 'anonymous' | 'public'

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
}

export interface TypingHeroesBoard {
  period: { kind: TypingHeroesPeriodKind; start: number; end: number; tzOffset: number; current: boolean }
  metric: TypingHeroesMetric
  scope: TypingHeroesScope
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
  isPublic: boolean
  speedMinActiveMinutes: number
  generatedAt: number
}

export interface TypingHeroesQuery {
  period: TypingHeroesPeriodKind
  anchor: string
  metric: TypingHeroesMetric
  scope: TypingHeroesScope
}

export interface TypingHeroesTimezone {
  timezone: string
  tzOffset: number
}

export function typingHeroesTimezone(date = new Date()): TypingHeroesTimezone {
  return { timezone: currentTimezone(), tzOffset: currentTZOffsetMinutes(date) }
}

export type TypingHeroesPrefs = Omit<TypingHeroesQuery, 'anchor'>

export const DEFAULT_TYPING_HEROES_PREFS: TypingHeroesPrefs = {
  period: 'week',
  metric: 'chars',
  scope: 'all',
}

export function normalizeTypingHeroesPrefs(raw: unknown): TypingHeroesPrefs {
  const prefs = { ...DEFAULT_TYPING_HEROES_PREFS }
  if (!raw || typeof raw !== 'object') return prefs
  const value = raw as Record<string, unknown>
  if (value.period === 'day' || value.period === 'week' || value.period === 'month') prefs.period = value.period
  if (TYPING_HEROES_METRICS.includes(value.metric as TypingHeroesMetric)) prefs.metric = value.metric as TypingHeroesMetric
  if (value.scope === 'all' || value.scope === 'ic') prefs.scope = value.scope
  return prefs
}

/** IANA 时区不可用时使用的 UTC 偏移。 */
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

export async function fetchTypingHeroesBoard(query: TypingHeroesQuery, zone = typingHeroesTimezone()): Promise<TypingHeroesBoard> {
  const params: Record<string, string | number> = {
    period: query.period,
    anchor: typingHeroesPeriodStart(query.period, query.anchor),
    metric: query.metric,
    scope: query.scope,
    tzOffset: zone.tzOffset,
  }
  const timezone = zone.timezone
  if (timezone) params.timezone = timezone
  const res = await api.get<TypingHeroesBoard>('api/v1/user/input-stats/heroes', { params })
  return res.data
}

export async function fetchTypingHeroesActiveDays(month: string, scope: TypingHeroesScope, zone = typingHeroesTimezone()): Promise<string[]> {
  const params: Record<string, string | number> = { month, scope, tzOffset: zone.tzOffset }
  if (zone.timezone) params.timezone = zone.timezone
  const res = await api.get<{ month: string; days: string[] }>('api/v1/user/input-stats/heroes/active-days', { params })
  return res.data.days
}

export function isTypingHeroesPeriodDisabled(kind: TypingHeroesPeriodKind, anchor: string, today: string): boolean {
  const start = typingHeroesPeriodStart(kind, anchor)
  return start < '0001-01-01' || start > typingHeroesPeriodStart(kind, today)
}

export async function saveTypingHeroesPublic(value: boolean): Promise<void> {
  await api.post('api/v1/user/preferences', { key: TYPING_HEROES_PUBLIC_PREF_KEY, value: value ? 'true' : 'false' })
}

export function typingHeroesLocalDate(date: Date): string {
  const year = String(date.getFullYear()).padStart(4, '0')
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

/** UTC is only used for civil date arithmetic; the API resolves the actual instants. */
function typingHeroesCivilDate(anchor: string): Date {
  return new Date(`${anchor}T00:00:00Z`)
}

export function typingHeroesPeriodStart(kind: TypingHeroesPeriodKind, anchor: string): string {
  const date = typingHeroesCivilDate(anchor)
  if (kind === 'month') date.setUTCDate(1)
  if (kind === 'week') date.setUTCDate(date.getUTCDate() - (date.getUTCDay() + 6) % 7)
  return date.toISOString().slice(0, 10)
}

export function shiftTypingHeroesAnchor(kind: TypingHeroesPeriodKind, anchor: string, step: number): string {
  const date = typingHeroesCivilDate(anchor)
  if (kind === 'month') {
    const day = date.getUTCDate()
    date.setUTCMonth(date.getUTCMonth() + step, 1)
    const last = new Date(date)
    last.setUTCMonth(last.getUTCMonth() + 1, 0)
    date.setUTCDate(Math.min(day, last.getUTCDate()))
  } else {
    date.setUTCDate(date.getUTCDate() + (kind === 'week' ? 7 : 1) * step)
  }
  return date.toISOString().slice(0, 10)
}

export function formatTypingHeroesPeriod(kind: TypingHeroesPeriodKind, anchor: string, locale: string, today: string): string {
  const start = typingHeroesPeriodStart(kind, anchor)
  const startDate = typingHeroesCivilDate(start)
  if (kind === 'month') {
    return new Intl.DateTimeFormat(locale, { timeZone: 'UTC', year: 'numeric', month: 'long' }).format(startDate)
  }
  const fullDate = { timeZone: 'UTC', year: 'numeric', month: 'numeric', day: 'numeric' } as const
  if (kind === 'day') return new Intl.DateTimeFormat(locale, fullDate).format(startDate)
  const last = typingHeroesCivilDate(shiftTypingHeroesAnchor('day', start, 6))
  const sameYear = startDate.getUTCFullYear() === last.getUTCFullYear() && start.slice(0, 4) === today.slice(0, 4)
  const fmt = new Intl.DateTimeFormat(locale, sameYear ? { ...fullDate, year: undefined } : fullDate)
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
