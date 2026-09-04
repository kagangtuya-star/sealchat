import type { ChatCharactersSnapshotPayload } from '../bridge/theater-bridge-protocol'
import type { TheaterDialogueRuntimeSnapshot } from './theater-dialogue-runtime'

export const THEATER_DIALOGUE_SURFACE_MESSAGE_TYPES = {
  ready: 'sealchat.theater.dialogue-surface.ready',
  dispose: 'sealchat.theater.dialogue-surface.dispose',
  runtime: 'sealchat.theater.dialogue-surface.runtime',
  characters: 'sealchat.theater.dialogue-surface.characters',
  command: 'sealchat.theater.dialogue-surface.command',
} as const

export interface TheaterDialogueSurfaceContext {
  identityId: string
  worldId: string
  channelId: string
}

export type TheaterDialogueSurfaceCommand =
  | { name: 'complete-current'; messageId?: string }
  | { name: 'skip' }
  | { name: 'close' }
  | { name: 'set-reduced-motion'; value: boolean }
  | { name: 'set-characters-per-second'; value: number }

export type TheaterDialogueSurfaceReadyMessage = TheaterDialogueSurfaceContext & {
  type: typeof THEATER_DIALOGUE_SURFACE_MESSAGE_TYPES.ready
}

export type TheaterDialogueSurfaceDisposeMessage = TheaterDialogueSurfaceContext & {
  type: typeof THEATER_DIALOGUE_SURFACE_MESSAGE_TYPES.dispose
}

export type TheaterDialogueSurfaceRuntimeMessage = TheaterDialogueSurfaceContext & {
  type: typeof THEATER_DIALOGUE_SURFACE_MESSAGE_TYPES.runtime
  sessionId: string
  snapshot: TheaterDialogueRuntimeSnapshot
}

export type TheaterDialogueSurfaceCharactersMessage = TheaterDialogueSurfaceContext & {
  type: typeof THEATER_DIALOGUE_SURFACE_MESSAGE_TYPES.characters
  sessionId: string
  snapshot: ChatCharactersSnapshotPayload
}

export type TheaterDialogueSurfaceCommandMessage = TheaterDialogueSurfaceContext & {
  type: typeof THEATER_DIALOGUE_SURFACE_MESSAGE_TYPES.command
  sessionId: string
  command: TheaterDialogueSurfaceCommand
}

const isRecord = (value: unknown): value is Record<string, unknown> => (
  Boolean(value) && typeof value === 'object' && !Array.isArray(value)
)

const hasSurfaceContext = (value: Record<string, unknown>): value is Record<string, unknown> & TheaterDialogueSurfaceContext => (
  typeof value.identityId === 'string' && Boolean(value.identityId.trim())
  && typeof value.worldId === 'string' && Boolean(value.worldId.trim())
  && typeof value.channelId === 'string' && Boolean(value.channelId.trim())
)

export const isTheaterDialogueSurfaceReadyMessage = (value: unknown): value is TheaterDialogueSurfaceReadyMessage => (
  isRecord(value)
  && value.type === THEATER_DIALOGUE_SURFACE_MESSAGE_TYPES.ready
  && hasSurfaceContext(value)
)

export const isTheaterDialogueSurfaceDisposeMessage = (value: unknown): value is TheaterDialogueSurfaceDisposeMessage => (
  isRecord(value)
  && value.type === THEATER_DIALOGUE_SURFACE_MESSAGE_TYPES.dispose
  && hasSurfaceContext(value)
)

const isTheaterDialogueSurfaceCommand = (value: unknown): value is TheaterDialogueSurfaceCommand => {
  if (!isRecord(value) || typeof value.name !== 'string') return false
  if (value.name === 'complete-current') return value.messageId === undefined || typeof value.messageId === 'string'
  if (value.name === 'skip' || value.name === 'close') return true
  if (value.name === 'set-reduced-motion') return typeof value.value === 'boolean'
  return value.name === 'set-characters-per-second'
    && typeof value.value === 'number'
    && Number.isFinite(value.value)
}

export const isTheaterDialogueSurfaceCommandMessage = (value: unknown): value is TheaterDialogueSurfaceCommandMessage => (
  isRecord(value)
  && value.type === THEATER_DIALOGUE_SURFACE_MESSAGE_TYPES.command
  && hasSurfaceContext(value)
  && typeof value.sessionId === 'string'
  && Boolean(value.sessionId)
  && isTheaterDialogueSurfaceCommand(value.command)
)

const isTheaterDialogueRuntimeSnapshot = (value: unknown): value is TheaterDialogueRuntimeSnapshot => (
  isRecord(value)
  && isRecord(value.queue)
  && (value.phase === 'idle' || value.phase === 'typing' || value.phase === 'hold')
  && typeof value.reducedMotion === 'boolean'
)

export const isTheaterDialogueSurfaceRuntimeMessage = (value: unknown): value is TheaterDialogueSurfaceRuntimeMessage => (
  isRecord(value)
  && value.type === THEATER_DIALOGUE_SURFACE_MESSAGE_TYPES.runtime
  && hasSurfaceContext(value)
  && typeof value.sessionId === 'string'
  && Boolean(value.sessionId)
  && isTheaterDialogueRuntimeSnapshot(value.snapshot)
)

const isChatCharactersSnapshot = (value: unknown): value is ChatCharactersSnapshotPayload => (
  isRecord(value)
  && typeof value.revision === 'number'
  && typeof value.updatedAt === 'number'
  && Array.isArray(value.characters)
)

export const isTheaterDialogueSurfaceCharactersMessage = (value: unknown): value is TheaterDialogueSurfaceCharactersMessage => (
  isRecord(value)
  && value.type === THEATER_DIALOGUE_SURFACE_MESSAGE_TYPES.characters
  && hasSurfaceContext(value)
  && typeof value.sessionId === 'string'
  && Boolean(value.sessionId)
  && isChatCharactersSnapshot(value.snapshot)
)

export const buildTheaterDialogueSurfaceUrl = (context: TheaterDialogueSurfaceContext): string => {
  if (typeof window === 'undefined') return ''
  const base = window.location.href.split('#', 1)[0].replace(/\/+$/, '')
  const search = new URLSearchParams({ world: context.worldId, channel: context.channelId })
  return `${base}/#/internal/theater-dialogue/${encodeURIComponent(context.identityId)}?${search.toString()}`
}

export const parseTheaterDialogueSurfaceUrl = (
  value: string,
  expectedOrigin = typeof window === 'undefined' ? '' : window.location.origin,
): TheaterDialogueSurfaceContext | null => {
  if (!value || typeof value !== 'string') return null
  try {
    const url = new URL(value.replace(/&amp;/gi, '&').trim(), expectedOrigin || 'http://localhost')
    if (expectedOrigin && url.origin !== expectedOrigin) return null
    const path = url.hash.startsWith('#') ? url.hash.slice(1) : url.pathname
    const match = path.match(/^\/internal\/theater-dialogue\/([^/?#]+)\?([^#]+)$/)
    if (!match) return null
    const identityId = decodeURIComponent(match[1] || '').trim()
    const search = new URLSearchParams(match[2])
    const worldId = (search.get('world') || '').trim()
    const channelId = (search.get('channel') || '').trim()
    if (!identityId || !worldId || !channelId) return null
    return { identityId, worldId, channelId }
  } catch {
    return null
  }
}
