import { isTipTapJson, tiptapJsonToPlainText } from '@/utils/tiptap-render'

const DEFAULT_BOT_COMMAND_PREFIXES = ['.', '。', '．', '｡', '/']
const MENTION_TOKEN_REGEX = /<at\s+id=(['"])([^'"]*)\1(?:\s+name=(['"])(.*?)\3)?\s*\/?\s*>/gi

export const normalizeBotCommandPrefixes = (raw?: unknown): string[] => {
  if (!Array.isArray(raw)) {
    return [...DEFAULT_BOT_COMMAND_PREFIXES]
  }
  const seen = new Set<string>()
  const normalized = raw
    .map((item) => String(item ?? '').trim())
    .filter((item) => {
      if (!item || seen.has(item)) {
        return false
      }
      seen.add(item)
      return true
    })
  return normalized.length ? normalized : [...DEFAULT_BOT_COMMAND_PREFIXES]
}

const resolveBotCommandSource = (content: string): string => {
  const trimmed = String(content || '').trim()
  if (!trimmed) {
    return ''
  }
  if (!isTipTapJson(trimmed)) {
    return trimmed
  }
  try {
    return tiptapJsonToPlainText(trimmed).trim()
  } catch {
    return trimmed
  }
}

const escapeHtml = (value: string) => value
  .replace(/&/g, '&amp;')
  .replace(/</g, '&lt;')
  .replace(/>/g, '&gt;')
  .replace(/"/g, '&quot;')

const ensureTrailingNewline = (parts: string[]) => {
  if (!parts.length || parts[parts.length - 1].endsWith('\n')) {
    return
  }
  parts.push('\n')
}

const decodeMentionAttr = (value: string) => String(value || '')
  .replace(/&quot;/g, '"')
  .replace(/&#039;/g, "'")
  .replace(/&apos;/g, "'")
  .replace(/&gt;/g, '>')
  .replace(/&lt;/g, '<')
  .replace(/&amp;/g, '&')

const renderMentionCapsuleHtml = (id: string, name: string) => {
  const mentionId = decodeMentionAttr(id || '').trim()
  const mentionName = decodeMentionAttr(name || '').trim()
  const display = mentionName || mentionId
  if (!display) {
    return ''
  }
  const className = mentionId === 'all' ? 'mention-capsule mention-capsule--all' : 'mention-capsule'
  return `<span class="${className}">@${escapeHtml(display)}</span>`
}

const replaceMentionTokensWithDisplayText = (value: string, options?: { html?: boolean }) => {
  MENTION_TOKEN_REGEX.lastIndex = 0
  return String(value || '').replace(MENTION_TOKEN_REGEX, (_full, _idQuote, id, _nameQuote, name) => {
    if (options?.html) {
      return renderMentionCapsuleHtml(id || '', name || '')
    }
    const display = decodeMentionAttr(name || id || '').trim()
    return display ? `@${display}` : ''
  })
}

const resolveDiceHtmlSource = (el: HTMLElement): string => {
  const className = el.getAttribute('class') || ''
  const source = el.getAttribute('data-dice-source') || ''
  if (!source) {
    return ''
  }
  if (className.includes('dice-roll-group') || className.includes('dice-chip')) {
    return source
  }
  return ''
}

const applyMarks = (text: string, marks?: Array<{ type: string; attrs?: Record<string, any> }>) => {
  if (!marks?.length) {
    return text
  }
  return marks.reduce((result, mark) => {
    const type = String(mark?.type || '').trim().toLowerCase()
    switch (type) {
      case 'bold':
      case 'strong':
        return `**${result}**`
      case 'italic':
      case 'em':
        return `*${result}*`
      case 'strike':
      case 's':
        return `~~${result}~~`
      case 'code':
        return `\`${result}\``
      case 'link': {
        const href = String(mark?.attrs?.href || '').trim()
        return href ? `[${result}](${href})` : result
      }
      default:
        return result
    }
  }, text)
}

const isMentionNodeType = (value: unknown) => {
  const normalized = String(value || '').trim().toLowerCase()
  return normalized === 'mention' || normalized === 'satorimention'
}

const serializeTipTapNode = (node: any, parts: string[]) => {
  if (!node) return
  const type = String(node.type || '').trim().toLowerCase()
  switch (type) {
    case 'doc':
    case 'bulletlist':
    case 'orderedlist':
      ;(node.content || []).forEach((child: any) => serializeTipTapNode(child, parts))
      return
    case 'paragraph':
    case 'heading':
    case 'blockquote':
      ;(node.content || []).forEach((child: any) => serializeTipTapNode(child, parts))
      ensureTrailingNewline(parts)
      return
    case 'listitem':
      parts.push('- ')
      ;(node.content || []).forEach((child: any) => serializeTipTapNode(child, parts))
      ensureTrailingNewline(parts)
      return
    case 'text':
      parts.push(applyMarks(replaceMentionTokensWithDisplayText(String(node.text || '')), node.marks))
      return
    case 'hardbreak':
      parts.push('\n')
      return
    case 'mention':
    case 'satorimention': {
      const attrs = node.attrs || {}
      const label = String(attrs.label || attrs.name || attrs.id || node.text || '').trim()
      if (label) parts.push(`@${label}`)
      return
    }
    default:
      ;(node.content || []).forEach((child: any) => serializeTipTapNode(child, parts))
  }
}

const serializeHtmlNode = (node: Node, parts: string[], inCodeBlock = false) => {
  if (node.nodeType === Node.TEXT_NODE) {
    parts.push(replaceMentionTokensWithDisplayText(node.textContent || ''))
    return
  }
  if (node.nodeType !== Node.ELEMENT_NODE) {
    return
  }
  const el = node as HTMLElement
  const diceSource = resolveDiceHtmlSource(el)
  if (diceSource) {
    parts.push(diceSource)
    return
  }
  const tag = el.tagName.toLowerCase()
  switch (tag) {
    case 'br':
      parts.push('\n')
      return
    case 'p':
    case 'div':
    case 'blockquote':
      Array.from(el.childNodes).forEach((child) => serializeHtmlNode(child, parts, inCodeBlock))
      ensureTrailingNewline(parts)
      return
    case 'ul':
    case 'ol':
      Array.from(el.childNodes).forEach((child) => serializeHtmlNode(child, parts, inCodeBlock))
      return
    case 'li':
      parts.push('- ')
      Array.from(el.childNodes).forEach((child) => serializeHtmlNode(child, parts, inCodeBlock))
      ensureTrailingNewline(parts)
      return
    case 'strong':
    case 'b':
      parts.push('**')
      Array.from(el.childNodes).forEach((child) => serializeHtmlNode(child, parts, inCodeBlock))
      parts.push('**')
      return
    case 'em':
    case 'i':
      parts.push('*')
      Array.from(el.childNodes).forEach((child) => serializeHtmlNode(child, parts, inCodeBlock))
      parts.push('*')
      return
    case 'code':
      if (!inCodeBlock) parts.push('`')
      Array.from(el.childNodes).forEach((child) => serializeHtmlNode(child, parts, inCodeBlock))
      if (!inCodeBlock) parts.push('`')
      return
    case 'pre':
      parts.push('```\n')
      Array.from(el.childNodes).forEach((child) => serializeHtmlNode(child, parts, true))
      ensureTrailingNewline(parts)
      parts.push('```')
      ensureTrailingNewline(parts)
      return
    case 'a': {
      const href = String(el.getAttribute('href') || '').trim()
      if (href) parts.push('[')
      Array.from(el.childNodes).forEach((child) => serializeHtmlNode(child, parts, inCodeBlock))
      if (href) parts.push(`](${href})`)
      return
    }
    case 'at': {
      const id = String(el.getAttribute('id') || '').trim()
      const name = String(el.getAttribute('name') || '').trim()
      const label = name || id
      if (label) {
        parts.push(`@${label}`)
      }
      return
    }
    default:
      Array.from(el.childNodes).forEach((child) => serializeHtmlNode(child, parts, inCodeBlock))
  }
}

export const serializeBotCommandContent = (content: string): string => {
  const raw = String(content || '')
  const trimmed = raw.trim()
  if (!trimmed) {
    return ''
  }
  if (isTipTapJson(trimmed)) {
    try {
      const doc = JSON.parse(trimmed)
      const parts: string[] = []
      serializeTipTapNode(doc, parts)
      return parts.join('').replace(/\n{3,}/g, '\n\n').trim()
    } catch {
      return resolveBotCommandSource(raw)
    }
  }
  if (/[<>]/.test(trimmed)) {
    const container = document.createElement('div')
    container.innerHTML = trimmed
    const parts: string[] = []
    Array.from(container.childNodes).forEach((child) => serializeHtmlNode(child, parts, false))
    const normalized = parts.join('').replace(/\n{3,}/g, '\n\n').trim()
    if (normalized) {
      return normalized
    }
  }
  return raw
}

export const renderBotCommandTextAsHtml = (content: string): string => {
  const serialized = serializeBotCommandContent(content)
  const withMentions = replaceMentionTokensWithDisplayText(serialized, { html: true })
  const html = withMentions
    .split(/\r\n|\r|\n/g)
    .map((line) => line
      .split(/(<span class="mention-capsule(?: mention-capsule--all)?">.*?<\/span>)/g)
      .map((segment) => segment.startsWith('<span class="mention-capsule')
        ? segment
        : escapeHtml(segment))
      .join(''))
    .join('<br />')
  return html
}

export const isBotCommandLikeContent = (content: string, prefixes?: unknown): boolean => {
  const source = serializeBotCommandContent(content)
  if (!source) {
    return false
  }
  const leading = source.trimStart()
  if (!leading) {
    return false
  }
  return normalizeBotCommandPrefixes(prefixes).some((prefix) => leading.startsWith(prefix))
}
