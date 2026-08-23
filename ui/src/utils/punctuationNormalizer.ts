const HALF_TO_FULL = Object.freeze<Record<string, string>>({ ',': '，', '.': '。', ';': '；', ':': '：', '?': '？', '!': '！' })
const SAFE_NORMALIZATION = Object.freeze<Record<string, string>>({ '⁇': '？？', '‼': '！！', '⁈': '？！', '⁉': '！？' })
const ASCII_PAUSE = new Set(Array.from(',.;:?!'))
const FULLWIDTH_PAUSE = new Set(Array.from('，。、；：？！'))
const LEFT_MARK = new Set(Array.from('“‘「『（［｛〔【〖《〈'))
const RIGHT_MARK = new Set(Array.from('”’」』）］｝〕】〗》〉'))
const NON_WESTERN = new Set(Array.from('，。、；：？！⁈⁇‼⁉“”‘’（）〔〕［］｛｝《》〈〉「」『』【】〖〗—⸺…'))

// Single scanner for machine syntax and chat visuals. Keep aligned with Go.
const PROTECTED_RE = /```[\s\S]*?(?:```|$)|`[^`\r\n]*`|!?\[[^\]\r\n]*\]\([^\s\r\n]+\)|https?:\/\/[A-Z0-9:\/?#\[\]@!$&'()*+,;=%._~\-]+|\bwww\.[A-Z0-9:\/?#\[\]@!$&'()*+,;=%._~\-]+|\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b|\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+:\/[^\s，。！？；]+|\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}(?::[0-9]+)?\b|\b(?:[0-9A-F]{1,4}:){2,7}[0-9A-F]{1,4}\b|\b(?:[0-9A-F]{1,4}:){1,6}:[0-9A-F]{1,4}\b|\b[A-Z]:\\[^\s，。！？；]+|(?:^|[\s（(])(?:\.{1,2}\/|\/)[A-Z0-9._~%+\-]+(?:\/[A-Z0-9._~%+\-]+)+|\b[A-Z0-9._~%+\-]+(?:\/[A-Z0-9._~%+\-]+)+\b|\bv?[0-9]+(?:\.[0-9]+)+(?:[-+][A-Z0-9.-]+)?\b|\b[0-9]{4}[-/][0-9]{1,2}[-/][0-9]{1,2}\b|\b[0-9]{1,2}:[0-9]{2}(?::[0-9]{2})?\b|\b[0-9]+:[0-9]+\b|\b[0-9]+x[0-9]+\b|\b[0-9]{1,3}(?:,[0-9]{3})+(?:\.[0-9]+)?\b|\b[0-9]+\.[0-9]+\b|\b[A-Z_][A-Z0-9_]*(?:::[A-Z_][A-Z0-9_]*)+\b|\b[A-Z_][A-Z0-9_]*(?:\.[A-Z_][A-Z0-9_]*)+\b|\b[A-Z]+(?:'[A-Z]+)+\b|\b[A-Z]+'\b|(?:^|\s)'[0-9]{2}s\b|\brock\s+'n'\s+roll\b|\.{3,}|-{2,}|_{3,}|(?:(?:[:;=8xX][-^']?[)(\/DPp])|(?:[:;][-^']?\()|XD)|\([^()\r\n]*(?:・|；|∀|ω|□|°|´|｀|╯|┻|━)[^()\r\n]*\)(?:[^\r\n]*┻━┻)?|[～〜~]{2,}|[●○]{2,}|\\["'?!:.,;]|<\/?[A-Z][^>\r\n]*>|\[\/?[A-Z][^\]\r\n]*\]|\[\[(?:IMG|IMAGE|ATTACHMENT|EMOJI):[^\]\r\n]+\]\]/giu

const isHan = (char: string) => !!char && /\p{Script=Han}/u.test(char)
const isWestern = (char: string) => !!char && /[\p{Script=Latin}\p{Script=Greek}\p{N}_]/u.test(char)
const isContext = (char: string) => isHan(char) || FULLWIDTH_PAUSE.has(char) || LEFT_MARK.has(char) || RIGHT_MARK.has(char)
const hasNonWestern = (text: string) => Array.from(text).some(char => isHan(char) || NON_WESTERN.has(char))

const isMachineDocument = (text: string) => {
  const value = text.trim()
  if (!value || !['{', '['].includes(value[0])) return false
  try { return typeof JSON.parse(value) === 'object' } catch { return false }
}

interface Span { start: number; end: number }
const trimUrlBoundary = (value: string) => {
  if (!/^(?:https?:\/\/|www\.)/iu.test(value)) return value.length
  let end = value.length
  const openParen = (value.match(/\(/g) || []).length
  let closeParen = (value.match(/\)/g) || []).length
  const openBracket = (value.match(/\[/g) || []).length
  let closeBracket = (value.match(/\]/g) || []).length
  while (end > 0) {
    const prefix = value.slice(0, end)
    const char = Array.from(prefix).at(-1) || ''
    if (/[.,;:!?]/u.test(char)) { end -= char.length; continue }
    if (char === ')' && closeParen > openParen) { end--; closeParen--; continue }
    if (char === ']' && closeBracket > openBracket) { end--; closeBracket--; continue }
    break
  }
  return end
}
const scanProtectedSpans = (text: string): Span[] => {
  const spans: Span[] = []
  PROTECTED_RE.lastIndex = 0
  for (const match of text.matchAll(PROTECTED_RE)) {
    const start = match.index
    const end = start + trimUrlBoundary(match[0])
    if (end <= start) continue
    const previous = spans.at(-1)
    if (previous && start <= previous.end) previous.end = Math.max(previous.end, end)
    else spans.push({ start, end })
  }
  return spans
}

interface QuoteFamily { left: string; right: string; kind: string; neutral: '"' | "'" }
const QUOTE_FAMILIES: readonly QuoteFamily[] = Object.freeze([
  { left: '“', right: '”', kind: 'double-curly', neutral: '"' }, { left: '‘', right: '’', kind: 'single-curly', neutral: "'" },
  { left: '「', right: '」', kind: 'corner', neutral: '"' }, { left: '『', right: '』', kind: 'double-corner', neutral: '"' },
  { left: '《', right: '》', kind: 'book', neutral: '"' }, { left: '〈', right: '〉', kind: 'single-book', neutral: "'" },
  { left: '【', right: '】', kind: 'square', neutral: '"' }, { left: '〖', right: '〗', kind: 'double-square', neutral: '"' },
  { left: '〔', right: '〕', kind: 'tortoise-shell', neutral: '"' },
])
const LEFT_FAMILY = new Map(QUOTE_FAMILIES.map(family => [family.left, family]))
const RIGHT_FAMILY = new Map(QUOTE_FAMILIES.map(family => [family.right, family]))
const DEFAULT_FAMILY: Record<'"' | "'", QuoteFamily> = { '"': QUOTE_FAMILIES[0], "'": QUOTE_FAMILIES[1] }
interface QuoteGroup { start: number; neutral: '"' | "'"; family?: QuoteFamily }

const isApostrophe = (chars: string[], index: number) => {
  const previous = chars[index - 1] || ''; const next = chars[index + 1] || ''
  if (isWestern(previous) && isWestern(next)) return true
  if (isWestern(previous) && (!next || /[\s\p{P}]/u.test(next))) return true
  return /\p{N}/u.test(next) && (!previous || /[\s\p{P}]/u.test(previous))
}
const neutralDirection = (chars: string[], index: number) => {
  const previous = chars[index - 1] || ''; const next = chars[index + 1] || ''
  if (!previous || LEFT_MARK.has(previous) || '([{：:，,；;！？!?\n\r'.includes(previous)) return 1
  if (!next || RIGHT_MARK.has(next) || ')]，,。.;；:：！？!?\n\r'.includes(next)) return -1
  return 0
}
const normalizeQuotes = (text: string) => {
  const chars = Array.from(text); const stack: QuoteGroup[] = []
  for (let index = 0; index < chars.length; index++) {
    const quote = chars[index]; const leftFamily = LEFT_FAMILY.get(quote); const rightFamily = RIGHT_FAMILY.get(quote)
    const neutral = quote === '"' || quote === "'"
    if (!neutral && !leftFamily && !rightFamily) continue
    if (quote === "'" && isApostrophe(chars, index)) continue
    if (neutral) {
      let direction = neutralDirection(chars, index); const active = stack.at(-1)
      if (!direction && active?.neutral === quote) direction = -1
      if (direction < 0) {
        if (!active || active.neutral !== quote) continue
        stack.pop(); const family = active.family || DEFAULT_FAMILY[quote]
        if (!active.family) chars[active.start] = family.left
        chars[index] = family.right; continue
      }
      stack.push({ start: index, neutral: quote }); continue
    }
    if (leftFamily) { stack.push({ start: index, neutral: leftFamily.neutral, family: leftFamily }); continue }
    const active = stack.at(-1); if (!active || !rightFamily) continue
    if (active.family === rightFamily) stack.pop()
    else if (!active.family && active.neutral === rightFamily.neutral) { chars[active.start] = rightFamily.left; stack.pop() }
  }
  return chars.join('')
}

const normalizeSpaces = (text: string) => {
  const chars = Array.from(text); const output: string[] = []
  for (let index = 0; index < chars.length; index++) {
    if (chars[index] !== ' ') { output.push(chars[index]); continue }
    let end = index; while (chars[end + 1] === ' ') end++
    const previous = output.at(-1) || ''; const next = chars[end + 1] || ''
    if (FULLWIDTH_PAUSE.has(next) || RIGHT_MARK.has(next) || LEFT_MARK.has(previous) || FULLWIDTH_PAUSE.has(previous)) { index = end; continue }
    output.push(...chars.slice(index, end + 1)); index = end
  }
  return output.join('')
}
const normalizeUnprotected = (text: string) => {
  if (!text || !hasNonWestern(text)) return text
  const chars = Array.from(normalizeQuotes(Array.from(text, char => SAFE_NORMALIZATION[char] ?? char).join('')))
  const context = new Array<boolean>(chars.length).fill(false); let start = 0; let found = false
  for (let index = 0; index < chars.length; index++) {
    if (isContext(chars[index])) found = true
    if (chars[index] === '\n' || chars[index] === '\r') { for (let i = start; i < index; i++) context[i] = found; start = index + 1; found = false }
  }
  for (let i = start; i < chars.length; i++) context[i] = found
  for (let index = 0; index < chars.length; index++) {
    if (!ASCII_PAUSE.has(chars[index]) || !context[index]) continue
    if (!(isWestern(chars[index - 1] || '') && isWestern(chars[index + 1] || ''))) chars[index] = HALF_TO_FULL[chars[index]]
  }
  return normalizeSpaces(chars.join(''))
}

export function normalizeChinesePunctuation(input: string): string {
  const source = String(input ?? '')
  if (!source || isMachineDocument(source)) return source
  const spans = scanProtectedSpans(source)
  if (!spans.length) return normalizeUnprotected(source)
  let output = ''; let position = 0
  for (const span of spans) { output += normalizeUnprotected(source.slice(position, span.start)); output += source.slice(span.start, span.end); position = span.end }
  return output + normalizeUnprotected(source.slice(position))
}

export type MessageInputMode = 'plain' | 'rich'
export const normalizePunctuationForMessageSend = (text: string, enabled: boolean, mode: MessageInputMode, isCommandLike = false, isHistoricalResend = false): string => (
  enabled && mode === 'plain' && !isCommandLike && !isHistoricalResend ? normalizeChinesePunctuation(text) : text
)
export default normalizeChinesePunctuation
