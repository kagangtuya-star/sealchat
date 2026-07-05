/**
 * 消息链接工具函数
 * 用于生成和解析消息跳转链接
 */

const CHAT_LINK_PATH_EXACT_REGEX = /^(?:https?:\/\/[^\s<>"']*)?\/?#\/([a-zA-Z0-9_-]+)\/([a-zA-Z0-9_-]+)(?:\?([^\s#]+))?$/
const CHAT_LINK_REGEX_SOURCE = '(?:https?:\\/\\/[^\\s<>"]*)?\\/?#\\/[a-zA-Z0-9_-]+\\/[a-zA-Z0-9_-]+(?:\\?[^\\s<>\"]+)?'
const RESERVED_CHAT_ROUTE_SEGMENTS = new Set([
  'about',
  'embed',
  'invite',
  'ob',
  'split',
  'status',
  'user',
  'worlds',
])

export interface MessageLinkParams {
  worldId: string
  channelId: string
  messageId: string
}

export interface ChannelLinkParams {
  worldId: string
  channelId: string
}

export interface ChatLinkParams extends ChannelLinkParams {
  messageId?: string
}

export interface ParsedSingleChatLink extends ChatLinkParams {
  rawLink: string
}

/**
 * 生成消息的完整链接
 */
export function generateMessageLink(
  params: MessageLinkParams,
  options?: { base?: string }
): string {
  const { worldId, channelId, messageId } = params
  const base = resolveMessageLinkBase(options?.base)
  return `${base}/#/${worldId}/${channelId}?msg=${messageId}`
}

export function generateChannelLink(
  params: ChannelLinkParams,
  options?: { base?: string }
): string {
  const { worldId, channelId } = params
  const base = resolveMessageLinkBase(options?.base)
  return `${base}/#/${worldId}/${channelId}`
}

function resolveMessageLinkBase(base?: string): string {
  const trimmed = (base || '').trim()
  if (trimmed) {
    return trimmed.replace(/\/+$/, '')
  }
  if (typeof window === 'undefined') {
    return ''
  }
  return window.location.origin
}

const normalizeChatLinkInput = (value: string): string => value.replace(/&amp;/gi, '&').trim()

export function parseChatLink(url: string): ChatLinkParams | null {
  if (!url || typeof url !== 'string') return null
  const normalized = normalizeChatLinkInput(url)
  const match = normalized.match(CHAT_LINK_PATH_EXACT_REGEX)
  if (!match) return null

  const [, worldId, channelId, queryString] = match
  if (!worldId || !channelId) return null
  if (RESERVED_CHAT_ROUTE_SEGMENTS.has(worldId.toLowerCase())) return null

  const search = new URLSearchParams(queryString || '')
  const messageId = (search.get('msg') || '').trim()

  return messageId
    ? { worldId, channelId, messageId }
    : { worldId, channelId }
}

export function parseSingleChatLinkText(text: string): ParsedSingleChatLink | null {
  if (!text || typeof text !== 'string') return null
  const normalized = normalizeChatLinkInput(text).replace(/\u00a0/g, ' ').trim()
  if (!normalized || /\s/.test(normalized)) return null
  const parsed = parseChatLink(normalized)
  if (!parsed) return null
  return { ...parsed, rawLink: normalized }
}

export function getRelativeChannelLinkTitle(input: {
  currentPath: string[]
  targetPath: string[]
}): string {
  const current = (input.currentPath || []).map(part => String(part || '').trim()).filter(Boolean)
  const target = (input.targetPath || []).map(part => String(part || '').trim()).filter(Boolean)
  if (target.length === 0) {
    return ''
  }
  let index = 0
  while (index < current.length && index < target.length && current[index] === target[index]) {
    index += 1
  }
  const remaining = target.slice(index)
  return (remaining.length > 0 ? remaining : target.slice(-1)).join(' / ')
}

/**
 * 解析消息链接，返回 worldId, channelId, messageId
 * 仅匹配路径格式，忽略域名
 */
export function parseMessageLink(url: string): MessageLinkParams | null {
  const parsed = parseChatLink(url)
  if (!parsed?.messageId) return null
  return {
    worldId: parsed.worldId,
    channelId: parsed.channelId,
    messageId: parsed.messageId,
  }
}

/**
 * 检查 URL 是否为消息链接格式
 * 仅检查路径格式，不检查域名
 */
export function isLocalMessageLink(url: string): boolean {
  return parseMessageLink(url) !== null
}

export function isLocalChatLink(url: string): boolean {
  return parseChatLink(url) !== null
}

/**
 * 消息链接的正则表达式（用于在纯文本中匹配链接）
 * 匹配格式: http(s)://domain/#/{worldId}/{channelId}?msg={messageId}
 */
export const MESSAGE_LINK_REGEX =
  /(?:https?:\/\/[^\s<>"]*)?\/?#\/[a-zA-Z0-9_-]+\/[a-zA-Z0-9_-]+(?:\?msg=[^\s<>"]+)?/g

/**
 * 带自定义标题的消息链接正则表达式
 * 匹配格式: [自定义标题](http(s)://domain/#/{worldId}/{channelId}?msg={messageId})
 */
export const TITLED_MESSAGE_LINK_REGEX =
  new RegExp(`\\[([^\\]]+)\\]\\((${CHAT_LINK_REGEX_SOURCE})\\)`, 'g')

export interface TitledMessageLink {
  title: string
  url: string
  params: ChatLinkParams
}

/**
 * 解析带标题的消息链接
 */
export function parseTitledMessageLink(text: string): TitledMessageLink | null {
  TITLED_MESSAGE_LINK_REGEX.lastIndex = 0
  const match = TITLED_MESSAGE_LINK_REGEX.exec(text)
  if (!match) return null

  const [, title, url] = match
  const params = parseChatLink(url)
  if (!params) return null

  return { title, url, params }
}
