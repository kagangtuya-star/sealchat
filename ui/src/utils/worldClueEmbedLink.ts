export interface WorldClueEmbedLinkParams {
  worldId: string
  channelId: string
  clueId: string
}

export interface ParsedSingleWorldClueEmbedLink extends WorldClueEmbedLinkParams {
  rawLink: string
}

const CLUE_LINK_EXACT_REGEX = /^https?:\/\/[^\s<>"']*#\/([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_-]+)\?([^\s#]+)$/

const normalizeInput = (value: string) => value.replace(/&amp;/gi, '&').trim()

export function generateWorldClueEmbedLink(params: WorldClueEmbedLinkParams, options?: { base?: string }): string {
  const base = (options?.base || (typeof window === 'undefined' ? '' : window.location.origin)).trim().replace(/\/+$/, '')
  const search = new URLSearchParams({ clue: params.clueId })
  return `${base}/#/${params.worldId}/${params.channelId}?${search.toString()}`
}

export function parseWorldClueEmbedLink(value: string): WorldClueEmbedLinkParams | null {
  const match = normalizeInput(value || '').match(CLUE_LINK_EXACT_REGEX)
  if (!match) return null
  const [, worldId, channelId, query] = match
  const clueId = new URLSearchParams(query).get('clue')?.trim() || ''
  return worldId && channelId && clueId ? { worldId, channelId, clueId } : null
}

export function parseSingleWorldClueEmbedLinkText(text: string): ParsedSingleWorldClueEmbedLink | null {
  const normalized = normalizeInput(text || '').replace(/\u00a0/g, ' ').trim()
  if (!normalized || /\s/.test(normalized)) return null
  const parsed = parseWorldClueEmbedLink(normalized)
  return parsed ? { ...parsed, rawLink: normalized } : null
}
