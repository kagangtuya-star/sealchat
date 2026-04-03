import type { CompiledKeywordSpan } from '@/stores/worldGlossary'
import type { KeywordTooltipController } from './keywordTooltip'
import { applyKeywordTooltipSessionToElement } from './worldKeywordTooltipCandidates'

interface HighlightOptions {
  underlineOnly: boolean
  deduplicate?: boolean
  onKeywordDoubleInvoke?: (keywordId: string) => void
}

const HIGHLIGHT_CLASS = 'keyword-highlight'
const UNDERLINE_ONLY_CLASS = 'keyword-highlight--underline'

interface MatchedKeywordCandidate {
  keyword: CompiledKeywordSpan
  matchedVia: string
}

interface HighlightRangeGroup {
  start: number
  end: number
  candidates: MatchedKeywordCandidate[]
}

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

function compareKeywordPriority(left: CompiledKeywordSpan, right: CompiledKeywordSpan) {
  const leftCategoryPriority = left.categoryPriority || 0
  const rightCategoryPriority = right.categoryPriority || 0
  if (leftCategoryPriority !== rightCategoryPriority) return rightCategoryPriority - leftCategoryPriority

  const leftTier = left.sourceType === 'world' ? 0 : 1
  const rightTier = right.sourceType === 'world' ? 0 : 1
  if (leftTier !== rightTier) return leftTier - rightTier

  const leftSourceSort = left.sourceSortOrder || 0
  const rightSourceSort = right.sourceSortOrder || 0
  if (leftSourceSort !== rightSourceSort) return rightSourceSort - leftSourceSort

  const leftSortOrder = left.sortOrder || 0
  const rightSortOrder = right.sortOrder || 0
  if (leftSortOrder !== rightSortOrder) return rightSortOrder - leftSortOrder

  if ((left.updatedAt || '') !== (right.updatedAt || '')) return (right.updatedAt || '').localeCompare(left.updatedAt || '')
  return (left.id || '').localeCompare(right.id || '')
}

function compareRangePriority(
  left: { start: number; end: number; keyword: CompiledKeywordSpan },
  right: { start: number; end: number; keyword: CompiledKeywordSpan },
) {
  const keywordPriority = compareKeywordPriority(left.keyword, right.keyword)
  if (keywordPriority !== 0) return keywordPriority

  const leftLength = left.end - left.start
  const rightLength = right.end - right.start
  if (leftLength !== rightLength) return rightLength - leftLength
  if (left.start !== right.start) return left.start - right.start
  if (left.end !== right.end) return left.end - right.end
  return 0
}

function rangesOverlap(
  left: { start: number; end: number },
  right: { start: number; end: number },
) {
  return left.start < right.end && right.start < left.end
}

function buildRanges(text: string, compiled: CompiledKeywordSpan[]): HighlightRangeGroup[] {
  const rawRanges: Array<{ start: number; end: number; keyword: CompiledKeywordSpan }> = []

  if (!compiled.length || !text) {
    return []
  }

  compiled.forEach((entry) => {
    const regex = new RegExp(entry.regex.source, entry.regex.flags.includes('g') ? entry.regex.flags : `${entry.regex.flags}g`)
    let match: RegExpExecArray | null
    while ((match = regex.exec(text)) !== null) {
      if (!match[0]) {
        regex.lastIndex += 1
        continue
      }
      rawRanges.push({ start: match.index, end: match.index + match[0].length, keyword: entry })
      if (match.index === regex.lastIndex) {
        regex.lastIndex += 1
      }
    }
  })

  rawRanges.sort(compareRangePriority)

  // Overlapping candidates now prefer configured classification priority first.
  const selected: HighlightRangeGroup[] = []
  rawRanges.forEach((range) => {
    const existingExactRange = selected.find((existing) => existing.start === range.start && existing.end === range.end)
    if (existingExactRange) {
      if (!existingExactRange.candidates.some((candidate) => candidate.keyword.id === range.keyword.id)) {
        existingExactRange.candidates.push({
          keyword: range.keyword,
          matchedVia: range.keyword.source,
        })
        existingExactRange.candidates.sort((left, right) => compareKeywordPriority(left.keyword, right.keyword))
      }
      return
    }

    if (selected.some((existing) => rangesOverlap(existing, range))) {
      return
    }
    selected.push({
      start: range.start,
      end: range.end,
      candidates: [{
        keyword: range.keyword,
        matchedVia: range.keyword.source,
      }],
    })
  })

  selected.sort((a, b) => {
    if (a.start !== b.start) return a.start - b.start
    if (a.end !== b.end) return a.end - b.end
    return compareKeywordPriority(a.candidates[0].keyword, b.candidates[0].keyword)
  })
  return selected
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
  highlightedKeywords: Set<string>,
) {
  if (!ranges.length) return
  const text = node.textContent || ''
  const fragment = document.createDocumentFragment()
  let lastIndex = 0

  ranges.forEach((range) => {
    const primaryCandidate = range.candidates[0]
    if (!primaryCandidate) {
      return
    }

    // Skip if deduplication is enabled and this keyword was already highlighted
    if (options.deduplicate && highlightedKeywords.has(primaryCandidate.keyword.id)) {
      return
    }

    if (range.start > lastIndex) {
      fragment.appendChild(document.createTextNode(text.slice(lastIndex, range.start)))
    }
    const span = document.createElement('span')
    span.className = HIGHLIGHT_CLASS
    const shouldUnderline =
      primaryCandidate.keyword.display === 'minimal' ||
      (options.underlineOnly && primaryCandidate.keyword.display === 'inherit')
    if (shouldUnderline) {
      span.classList.add(UNDERLINE_ONLY_CLASS)
    }
    applyKeywordTooltipSessionToElement(span, {
      candidates: range.candidates.map((candidate) => ({
        keywordId: candidate.keyword.id,
        matchedVia: candidate.matchedVia,
      })),
      activeIndex: 0,
    })
    span.textContent = text.slice(range.start, range.end)

    // No individual event listeners - using event delegation instead

    fragment.appendChild(span)

    // Track this keyword as highlighted if deduplication is enabled
    if (options.deduplicate) {
      highlightedKeywords.add(primaryCandidate.keyword.id)
    }

    lastIndex = range.end
  })

  if (lastIndex < text.length) {
    fragment.appendChild(document.createTextNode(text.slice(lastIndex)))
  }
  node.replaceWith(fragment)
}

// WeakMap to track delegated event listeners per container
const delegatedContainers = new WeakMap<HTMLElement, boolean>()

function setupEventDelegation(
  root: HTMLElement,
  options: HighlightOptions,
  tooltip?: KeywordTooltipController,
) {
  // Skip if already delegated
  if (delegatedContainers.has(root)) {
    return
  }
  delegatedContainers.set(root, true)

  // Hover events (using capture for mouseenter/leave)
  if (tooltip) {
    let currentHoveredSpan: HTMLElement | null = null

    root.addEventListener('mouseover', (e) => {
      const span = (e.target as HTMLElement).closest<HTMLElement>(`span.${HIGHLIGHT_CLASS}`)
      if (span && root.contains(span)) {
        currentHoveredSpan = span
        const keywordId = span.dataset.keywordId
        if (keywordId) {
          tooltip.show(span, keywordId)
        }
      }
    })

    root.addEventListener('mouseout', (e) => {
      const span = (e.target as HTMLElement).closest<HTMLElement>(`span.${HIGHLIGHT_CLASS}`)
      if (!span || !root.contains(span)) return

      // Check if mouse moved to another highlight or outside
      const relatedTarget = (e as MouseEvent).relatedTarget as HTMLElement | null
      const movedToHighlight = relatedTarget?.closest<HTMLElement>(`span.${HIGHLIGHT_CLASS}`)

      // Only hide if not moving to another highlight
      if (!movedToHighlight || !root.contains(movedToHighlight)) {
        tooltip.hide(span)
        currentHoveredSpan = null
      }
    })

    // Fallback: hide tooltip when mouse leaves root entirely
    root.addEventListener('mouseleave', () => {
      if (currentHoveredSpan) {
        tooltip.hide(currentHoveredSpan)
        currentHoveredSpan = null
      }
    })

    // Click for pin (on highlight) or hide all (on non-highlight)
    root.addEventListener('click', (e) => {
      const span = (e.target as HTMLElement).closest<HTMLElement>(`span.${HIGHLIGHT_CLASS}`)

      // If clicking outside highlight area, hide all tooltips as fallback
      if (!span || !root.contains(span)) {
        tooltip.hideAll()
        return
      }

      const keywordId = span.dataset.keywordId
      if (!keywordId) return

      e.preventDefault()
      e.stopPropagation()

      // Handle double-click detection
      if (clickState.timer && clickState.target === span && clickState.keywordId === keywordId) {
        clearTimeout(clickState.timer)
        clickState.timer = null
        clickState.target = null
        clickState.keywordId = null
        if (options.onKeywordDoubleInvoke) {
          options.onKeywordDoubleInvoke(keywordId)
        }
        return
      }

      if (clickState.timer) {
        clearTimeout(clickState.timer)
      }

      clickState.target = span
      clickState.keywordId = keywordId
      clickState.timer = setTimeout(() => {
        tooltip.pin(span, keywordId)
        clickState.timer = null
        clickState.target = null
        clickState.keywordId = null
      }, DOUBLE_CLICK_DELAY)
    })
  }

  // Double click for edit
  if (options.onKeywordDoubleInvoke) {
    root.addEventListener('dblclick', (e) => {
      const span = (e.target as HTMLElement).closest<HTMLElement>(`span.${HIGHLIGHT_CLASS}`)
      if (!span || !root.contains(span)) return

      const keywordId = span.dataset.keywordId
      if (!keywordId) return

      e.preventDefault()
      e.stopPropagation()

      if (clickState.timer) {
        clearTimeout(clickState.timer)
        clickState.timer = null
        clickState.target = null
        clickState.keywordId = null
      }

      options.onKeywordDoubleInvoke!(keywordId)
    })
  }
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

  // Setup event delegation once per container
  setupEventDelegation(root, options, tooltip)

  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT)
  const nodes: Text[] = []
  let current = walker.nextNode()
  while (current) {
    if (canProcessNode(current)) {
      nodes.push(current as Text)
    }
    current = walker.nextNode()
  }

  // Create shared Set for deduplication across all text nodes in this message
  const highlightedKeywords = new Set<string>()

  nodes.forEach((node) => {
    const ranges = buildRanges(node.textContent || '', compiled)
    wrapRanges(node, ranges, options, highlightedKeywords)
  })
}

