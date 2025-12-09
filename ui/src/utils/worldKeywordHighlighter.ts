import type { CompiledKeywordSpan } from '@/stores/worldGlossary'
import type { KeywordTooltipController } from './keywordTooltip'

interface HighlightOptions {
  underlineOnly: boolean
  onKeywordDoubleInvoke?: (keywordId: string) => void
}

const HIGHLIGHT_CLASS = 'keyword-highlight'
const UNDERLINE_ONLY_CLASS = 'keyword-highlight--underline'

function clearExistingHighlights(root: HTMLElement) {
  const highlights = root.querySelectorAll(`span.${HIGHLIGHT_CLASS}`)
  highlights.forEach((node) => {
    const parent = node.parentNode
    if (!parent) return
    parent.replaceChild(document.createTextNode(node.textContent || ''), node)
    parent.normalize()
  })
}

function canProcessNode(node: Node) {
  if (!node || node.nodeType !== Node.TEXT_NODE) return false
  const parent = node.parentElement
  if (!parent) return false
  if (parent.closest('span.keyword-highlight')) return false
  if (parent.classList.contains('no-keyword-highlight')) return false
  return Boolean(node.textContent && node.textContent.trim().length)
}

function buildRanges(text: string, compiled: CompiledKeywordSpan[]) {
  const ranges: Array<{ start: number; end: number; keyword: CompiledKeywordSpan }> = []
  compiled.forEach((entry) => {
    const regex = new RegExp(entry.regex.source, entry.regex.flags.includes('g') ? entry.regex.flags : `${entry.regex.flags}g`)
    let match: RegExpExecArray | null
    while ((match = regex.exec(text)) !== null) {
      if (!match[0]) {
        regex.lastIndex += 1
        continue
      }
      ranges.push({ start: match.index, end: match.index + match[0].length, keyword: entry })
      if (match.index === regex.lastIndex) {
        regex.lastIndex += 1
      }
    }
  })
  ranges.sort((a, b) => (a.start === b.start ? b.end - a.end : a.start - b.start))
  const filtered: typeof ranges = []
  let cursor = -1
  ranges.forEach((range) => {
    if (range.start < cursor) {
      return
    }
    filtered.push(range)
    cursor = range.end
  })
  return filtered
}

function attachTouchDoubleTap(target: HTMLElement, handler: () => void) {
  let lastTap = 0
  target.addEventListener('touchend', (event) => {
    const now = Date.now()
    if (now - lastTap <= 350) {
      event.preventDefault()
      handler()
    }
    lastTap = now
  })
}

// Track click timing for distinguishing single vs double click
interface ClickState {
  timer: ReturnType<typeof setTimeout> | null
  target: HTMLElement | null
  keywordId: string | null
}

const clickState: ClickState = {
  timer: null,
  target: null,
  keywordId: null
}

const DOUBLE_CLICK_DELAY = 300

function wrapRanges(
  node: Text,
  ranges: ReturnType<typeof buildRanges>,
  options: HighlightOptions,
  tooltip?: KeywordTooltipController
) {
  if (!ranges.length) return
  const text = node.textContent || ''
  const fragment = document.createDocumentFragment()
  let lastIndex = 0

  ranges.forEach((range) => {
    if (range.start > lastIndex) {
      fragment.appendChild(document.createTextNode(text.slice(lastIndex, range.start)))
    }
    const span = document.createElement('span')
    span.className = HIGHLIGHT_CLASS
    if (options.underlineOnly || range.keyword.display === 'minimal') {
      span.classList.add(UNDERLINE_ONLY_CLASS)
    }
    span.dataset.keywordId = range.keyword.id
    span.dataset.keywordSource = range.keyword.source
    span.textContent = text.slice(range.start, range.end)

    if (tooltip) {
      // Hover behavior
      span.addEventListener('mouseenter', () => {
        tooltip.show(span, range.keyword.id)
      })

      span.addEventListener('mouseleave', () => {
        tooltip.hide(span)
      })

      // Click behavior - differentiate single click (pin) from double click (edit)
      span.addEventListener('click', (event) => {
        event.preventDefault()
        event.stopPropagation()

        // If we have a pending click on the same target, this is a double click
        if (clickState.timer && clickState.target === span && clickState.keywordId === range.keyword.id) {
          clearTimeout(clickState.timer)
          clickState.timer = null
          clickState.target = null
          clickState.keywordId = null

          // Double click - invoke edit if available
          if (options.onKeywordDoubleInvoke) {
            options.onKeywordDoubleInvoke(range.keyword.id)
          }
          return
        }

        // Clear any existing timer
        if (clickState.timer) {
          clearTimeout(clickState.timer)
        }

        // Set up for potential double click
        clickState.target = span
        clickState.keywordId = range.keyword.id
        clickState.timer = setTimeout(() => {
          // Single click - pin the tooltip
          tooltip.pin(span, range.keyword.id)
          clickState.timer = null
          clickState.target = null
          clickState.keywordId = null
        }, DOUBLE_CLICK_DELAY)
      })
    }

    // Double click handler for editing (mouse)
    if (options.onKeywordDoubleInvoke) {
      span.addEventListener('mousedown', (event) => {
        if (event.detail === 2) {
          event.preventDefault()
          event.stopPropagation()
        }
      })

      span.addEventListener('dblclick', (event) => {
        event.preventDefault()
        event.stopPropagation()
        // Clear any pending single click
        if (clickState.timer) {
          clearTimeout(clickState.timer)
          clickState.timer = null
          clickState.target = null
          clickState.keywordId = null
        }
        options.onKeywordDoubleInvoke?.(range.keyword.id)
      })

      // Touch double tap for editing
      attachTouchDoubleTap(span, () => {
        options.onKeywordDoubleInvoke?.(range.keyword.id)
      })
    }

    fragment.appendChild(span)
    lastIndex = range.end
  })

  if (lastIndex < text.length) {
    fragment.appendChild(document.createTextNode(text.slice(lastIndex)))
  }
  node.replaceWith(fragment)
}

export function refreshWorldKeywordHighlights(
  root: HTMLElement | null,
  compiled: CompiledKeywordSpan[],
  options: HighlightOptions,
  tooltip?: KeywordTooltipController,
) {
  if (!root) return
  if (!compiled?.length) {
    clearExistingHighlights(root)
    return
  }
  clearExistingHighlights(root)
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  const nodes: Text[] = []
  let current = walker.nextNode()
  while (current) {
    if (canProcessNode(current)) {
      nodes.push(current as Text)
    }
    current = walker.nextNode()
  }
  nodes.forEach((node) => {
    const ranges = buildRanges(node.textContent || '', compiled)
    wrapRanges(node, ranges, options, tooltip)
  })
}
