import type { CompiledKeywordSet } from '@/stores/worldKeywords'

interface HighlightOptions {
  tooltipEnabled: boolean
}

const DEFAULT_OPTIONS: HighlightOptions = {
  tooltipEnabled: true,
}

let tooltipEl: HTMLDivElement | null = null
let tooltipPinnedTarget: HTMLElement | null = null

const ensureTooltipEl = () => {
  if (typeof document === 'undefined') return null
  if (!tooltipEl) {
    tooltipEl = document.createElement('div')
    tooltipEl.className = 'keyword-tooltip'
    tooltipEl.dataset.visible = 'false'
    document.body.appendChild(tooltipEl)
    const hide = () => hideTooltip()
    window.addEventListener('scroll', hide, true)
    window.addEventListener('resize', hide)
    document.addEventListener('click', (event) => {
      if (!tooltipEl) return
      if (tooltipPinnedTarget && tooltipPinnedTarget.contains(event.target as Node)) {
        return
      }
      hideTooltip()
    })
  }
  return tooltipEl
}

const showTooltip = (target: HTMLElement, description: string, pin = false) => {
  const el = ensureTooltipEl()
  if (!el) return
  el.textContent = description
  const rect = target.getBoundingClientRect()
  const top = rect.top + window.scrollY - el.offsetHeight - 8
  const left = rect.left + window.scrollX + rect.width / 2
  el.style.top = `${Math.max(top, 0)}px`
  el.style.left = `${left}px`
  el.dataset.visible = 'true'
  tooltipPinnedTarget = pin ? target : null
}

const hideTooltip = () => {
  if (!tooltipEl) return
  tooltipEl.dataset.visible = 'false'
  tooltipPinnedTarget = null
}

const unwrap = (node: Element) => {
  const parent = node.parentNode
  if (!parent) return
  while (node.firstChild) {
    parent.insertBefore(node.firstChild, node)
  }
  parent.removeChild(node)
}

const clearExistingHighlights = (root: HTMLElement) => {
  const existing = root.querySelectorAll('.keyword-highlight')
  existing.forEach((node) => unwrap(node))
}

const buildMatchList = (text: string, compiled: CompiledKeywordSet) => {
  if (!compiled.pattern) return []
  const regex = new RegExp(compiled.pattern.source, compiled.pattern.flags)
  const matches: Array<{ start: number; end: number; text: string; definition: { keyword: string; description: string; id?: string } }> = []
  let exec: RegExpExecArray | null
  while ((exec = regex.exec(text)) !== null) {
    const hit = exec[0]
    const def = compiled.map.get(hit.toLowerCase())
    if (!def) continue
    matches.push({
      start: exec.index,
      end: exec.index + hit.length,
      text: hit,
      definition: { keyword: def.keyword, description: def.description, id: def.id },
    })
  }
  return matches
}

export const applyKeywordHighlights = (
  root: HTMLElement | null,
  compiled?: CompiledKeywordSet,
  options?: Partial<HighlightOptions>,
) => {
  const mergedOptions = { ...DEFAULT_OPTIONS, ...options }
  if (!root) {
    return { matchCount: 0 }
  }
  clearExistingHighlights(root)
  if (!compiled || !compiled.pattern || !compiled.entries.length) {
    return { matchCount: 0 }
  }
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode(node) {
      if (!node.parentElement) {
        return NodeFilter.FILTER_REJECT
      }
      if (node.parentElement.closest('.keyword-highlight')) {
        return NodeFilter.FILTER_REJECT
      }
      return NodeFilter.FILTER_ACCEPT
    },
  })
  let matchTotal = 0
  const nodes: Text[] = []
  while (walker.nextNode()) {
    const current = walker.currentNode as Text
    if (current.nodeValue?.trim()) {
      nodes.push(current)
    }
  }
  nodes.forEach((textNode) => {
    const text = textNode.nodeValue || ''
    const matches = buildMatchList(text, compiled)
    if (!matches.length) {
      return
    }
    matchTotal += matches.length
    const fragment = document.createDocumentFragment()
    let cursor = 0
    matches.forEach((match) => {
      if (match.start > cursor) {
        fragment.appendChild(document.createTextNode(text.slice(cursor, match.start)))
      }
      const span = document.createElement('span')
      span.className = 'keyword-highlight'
      span.textContent = match.text
      span.dataset.keyword = match.definition.keyword
      span.dataset.description = match.definition.description
      if (match.definition.id) {
        span.dataset.keywordId = match.definition.id
      }
      if (mergedOptions.tooltipEnabled) {
        span.addEventListener('mouseenter', () => showTooltip(span, match.definition.description))
        span.addEventListener('mouseleave', () => {
          if (tooltipPinnedTarget === span) return
          hideTooltip()
        })
        span.addEventListener('click', (event) => {
          event.stopPropagation()
          const nextPinned = tooltipPinnedTarget === span ? null : span
          if (nextPinned) {
            showTooltip(span, match.definition.description, true)
          } else {
            hideTooltip()
          }
        })
      } else {
        span.title = match.definition.description
      }
      fragment.appendChild(span)
      cursor = match.end
    })
    if (cursor < text.length) {
      fragment.appendChild(document.createTextNode(text.slice(cursor)))
    }
    textNode.replaceWith(fragment)
  })
  if (!matchTotal) {
    hideTooltip()
  }
  return { matchCount: matchTotal }
}

export const clearKeywordHighlights = (root: HTMLElement | null) => {
  if (!root) return
  clearExistingHighlights(root)
  hideTooltip()
}
