import type { CompiledKeywordSpan } from '@/stores/worldGlossary'
import { createImageTokenRegex, isValidAttachmentToken } from '@/utils/attachmentMarkdown'
import { tiptapJsonToHtml, isTipTapJson } from '@/utils/tiptap-render'

interface TooltipContent {
  title: string
  description: string
  descriptionFormat?: 'plain' | 'rich'
  matchedVia?: string
}

type ContentResolver = (keywordId: string) => TooltipContent | null | undefined

interface TooltipInstance {
  element: HTMLDivElement
  level: number
  keywordId: string
  isPinned: boolean
}

const MAX_NESTING_DEPTH = 4
const TOOLTIP_GAP = 12
const TOOLTIP_PADDING = 8
const TOOLTIP_MAX_WIDTH = 360
const TOOLTIP_MIN_WIDTH = 180
const TOOLTIP_MAX_HEIGHT_RATIO = 0.6 // 最大高度为视口高度的60%

// 确保滚动条样式已注入到页面
let tooltipStylesInjected = false
function ensureTooltipStyles() {
  if (tooltipStylesInjected || typeof document === 'undefined') return
  tooltipStylesInjected = true

  const styleId = 'keyword-tooltip-scrollbar-styles'
  if (document.getElementById(styleId)) return

  const style = document.createElement('style')
  style.id = styleId
  style.textContent = `
    /* Keyword Tooltip Scrollbar - Minimal/Invisible Design */
    .keyword-tooltip {
      scrollbar-width: thin;
      scrollbar-color: transparent transparent;
    }
    .keyword-tooltip:hover {
      scrollbar-color: rgba(128, 128, 128, 0.2) transparent;
    }
    .keyword-tooltip::-webkit-scrollbar {
      width: 4px !important;
      height: 4px !important;
    }
    .keyword-tooltip::-webkit-scrollbar-track {
      background: transparent !important;
    }
    .keyword-tooltip::-webkit-scrollbar-thumb {
      background: transparent !important;
      border-radius: 2px !important;
    }
    .keyword-tooltip:hover::-webkit-scrollbar-thumb {
      background: rgba(128, 128, 128, 0.2) !important;
    }
    /* Night mode */
    [data-display-palette='night'] .keyword-tooltip:hover,
    :root[data-display-palette='night'] .keyword-tooltip:hover {
      scrollbar-color: rgba(255, 255, 255, 0.2) transparent;
    }
    [data-display-palette='night'] .keyword-tooltip:hover::-webkit-scrollbar-thumb,
    :root[data-display-palette='night'] .keyword-tooltip:hover::-webkit-scrollbar-thumb {
      background: rgba(255, 255, 255, 0.2) !important;
    }
    /* Custom theme */
    :root[data-custom-theme='true'] .keyword-tooltip:hover {
      scrollbar-color: rgba(128, 128, 128, 0.25) transparent;
    }
    :root[data-custom-theme='true'] .keyword-tooltip:hover::-webkit-scrollbar-thumb {
      background: rgba(128, 128, 128, 0.25) !important;
    }
  `
  document.head.appendChild(style)
}

let tooltipStack: TooltipInstance[] = []
let globalClickHandler: ((e: MouseEvent | TouchEvent) => void) | null = null
let globalKeyHandler: ((e: KeyboardEvent) => void) | null = null
let pendingHideTimer: ReturnType<typeof setTimeout> | null = null

function clearPendingHide() {
  if (pendingHideTimer) {
    clearTimeout(pendingHideTimer)
    pendingHideTimer = null
  }
}

// Clean up any orphaned tooltip elements that aren't tracked in the stack
function cleanupOrphanedTooltips(includeHoverTooltips = false) {
  const allTooltips = document.querySelectorAll('.keyword-tooltip')
  const trackedElements = new Set(tooltipStack.map(t => t.element))

  allTooltips.forEach(tooltip => {
    if (!trackedElements.has(tooltip as HTMLDivElement)) {
      const isHover = tooltip.classList.contains('keyword-tooltip--hover')
      if (isHover) {
        // Only hide hover tooltips (don't remove - controllers manage their lifecycle)
        if (includeHoverTooltips) {
          ; (tooltip as HTMLElement).style.display = 'none'
        }
      } else {
        // Remove non-hover orphaned tooltips
        ; (tooltip as HTMLElement).style.display = 'none'
        tooltip.remove()
      }
    }
  })
}

let tooltipIdCounter = 0

function createTooltipElement(level: number): HTMLDivElement {
  // 确保滚动条样式已注入到页面
  ensureTooltipStyles()

  tooltipIdCounter++
  const tooltip = document.createElement('div')
  tooltip.id = `keyword-tooltip-${tooltipIdCounter}`
  tooltip.className = 'keyword-tooltip'
  tooltip.dataset.level = String(level)
  tooltip.style.cssText = `
    display: none;
    position: fixed;
    z-index: ${999999 + level};
    pointer-events: auto;
  `
  document.body.appendChild(tooltip)
  return tooltip
}

/**
 * 获取当前页面的缩放比例
 * 支持浏览器 Ctrl+/- 缩放和移动端捏合缩放
 */
function getPageZoomLevel(): number {
  // 方法1: 使用 outerWidth/innerWidth 检测桌面浏览器缩放
  // 这是检测 Ctrl+/- 缩放的可靠方法
  if (window.outerWidth && window.innerWidth) {
    const zoomRatio = window.outerWidth / window.innerWidth
    // 只有当比例明显不同于1时才使用（避免浮点误差）
    if (Math.abs(zoomRatio - 1) > 0.05) {
      return zoomRatio
    }
  }

  // 方法2: 移动端捏合缩放
  if (window.visualViewport && window.visualViewport.scale !== 1) {
    return window.visualViewport.scale
  }

  // 默认无缩放
  return 1
}

/**
 * 调整tooltip尺寸以适应视口
 * 如果高度过高，则扩展宽度以减少高度
 */
function adjustTooltipSize(tooltip: HTMLDivElement, viewportWidth: number, viewportHeight: number): void {
  const zoomLevel = getPageZoomLevel()
  const effectiveViewportHeight = viewportHeight / zoomLevel
  const effectiveViewportWidth = viewportWidth / zoomLevel
  const maxHeight = effectiveViewportHeight * TOOLTIP_MAX_HEIGHT_RATIO

  // 重置样式以获取自然尺寸
  tooltip.style.maxWidth = `${TOOLTIP_MAX_WIDTH}px`
  tooltip.style.maxHeight = ''
  tooltip.style.overflowY = ''

  // 获取当前高度
  const currentHeight = tooltip.offsetHeight

  // 如果高度超过最大允许高度，尝试扩展宽度
  if (currentHeight > maxHeight) {
    // 计算需要的宽度扩展比例（基于内容面积估算）
    const areaRatio = currentHeight / maxHeight
    let newMaxWidth = Math.min(
      TOOLTIP_MAX_WIDTH * Math.sqrt(areaRatio) * 1.1, // 增加10%余量
      effectiveViewportWidth - TOOLTIP_PADDING * 2 // 不超过视口宽度
    )
    newMaxWidth = Math.max(newMaxWidth, TOOLTIP_MIN_WIDTH)

    tooltip.style.maxWidth = `${newMaxWidth}px`

    // 重新检查高度，如果仍然过高则启用滚动
    const newHeight = tooltip.offsetHeight
    if (newHeight > maxHeight) {
      tooltip.style.maxHeight = `${maxHeight}px`
      tooltip.style.overflowY = 'auto'
    }
  }
}

function findBestPosition(
  target: HTMLElement,
  tooltip: HTMLDivElement,
  existingTooltips: TooltipInstance[]
): { top: number; left: number } {
  const zoomLevel = getPageZoomLevel()
  const targetRect = target.getBoundingClientRect()

  // 调整tooltip尺寸（考虑视口和高度限制）
  const viewportWidth = window.innerWidth
  const viewportHeight = window.innerHeight
  adjustTooltipSize(tooltip, viewportWidth, viewportHeight)

  const tooltipWidth = tooltip.offsetWidth
  const tooltipHeight = tooltip.offsetHeight

  // 考虑缩放比例调整padding和gap
  const effectivePadding = TOOLTIP_PADDING / zoomLevel
  const effectiveGap = TOOLTIP_GAP / zoomLevel

  const occupiedRects = existingTooltips.map(t => t.element.getBoundingClientRect())
  const candidates: Array<{ top: number; left: number; score: number }> = []

  // Above target
  const aboveTop = targetRect.top - tooltipHeight - effectiveGap
  const aboveLeft = Math.max(effectivePadding, Math.min(
    viewportWidth - tooltipWidth - effectivePadding,
    targetRect.left + targetRect.width / 2 - tooltipWidth / 2
  ))
  candidates.push({
    top: aboveTop,
    left: aboveLeft,
    score: calculatePositionScore(aboveTop, aboveLeft, tooltipWidth, tooltipHeight, viewportWidth, viewportHeight, occupiedRects)
  })

  // Below target
  const belowTop = targetRect.bottom + effectiveGap
  candidates.push({
    top: belowTop,
    left: aboveLeft,
    score: calculatePositionScore(belowTop, aboveLeft, tooltipWidth, tooltipHeight, viewportWidth, viewportHeight, occupiedRects)
  })

  // Right of target
  const rightTop = Math.max(effectivePadding, Math.min(
    viewportHeight - tooltipHeight - effectivePadding,
    targetRect.top + targetRect.height / 2 - tooltipHeight / 2
  ))
  const rightLeft = targetRect.right + effectiveGap
  candidates.push({
    top: rightTop,
    left: rightLeft,
    score: calculatePositionScore(rightTop, rightLeft, tooltipWidth, tooltipHeight, viewportWidth, viewportHeight, occupiedRects)
  })

  // Left of target
  const leftLeft = targetRect.left - tooltipWidth - effectiveGap
  candidates.push({
    top: rightTop,
    left: leftLeft,
    score: calculatePositionScore(rightTop, leftLeft, tooltipWidth, tooltipHeight, viewportWidth, viewportHeight, occupiedRects)
  })

  candidates.sort((a, b) => b.score - a.score)
  const best = candidates[0]

  return {
    top: Math.max(effectivePadding, Math.min(viewportHeight - tooltipHeight - effectivePadding, best.top)),
    left: Math.max(effectivePadding, Math.min(viewportWidth - tooltipWidth - effectivePadding, best.left))
  }
}

function calculatePositionScore(
  top: number,
  left: number,
  width: number,
  height: number,
  viewportWidth: number,
  viewportHeight: number,
  occupiedRects: DOMRect[]
): number {
  let score = 100
  if (top < TOOLTIP_PADDING) score -= 50
  if (left < TOOLTIP_PADDING) score -= 50
  if (top + height > viewportHeight - TOOLTIP_PADDING) score -= 50
  if (left + width > viewportWidth - TOOLTIP_PADDING) score -= 50

  const rect = new DOMRect(left, top, width, height)
  for (const occupied of occupiedRects) {
    if (rectsOverlap(rect, occupied)) {
      score -= 30
    }
  }
  return score
}

function rectsOverlap(a: DOMRect, b: DOMRect): boolean {
  return !(a.right < b.left || a.left > b.right || a.bottom < b.top || a.top > b.bottom)
}

function setupGlobalClickHandler(onClickOutside: () => void) {
  if (globalClickHandler) return

  globalClickHandler = (e: MouseEvent | TouchEvent) => {
    const target = e.target as HTMLElement
    if (!target) return

    const isInsideTooltip = tooltipStack.some(t => t.element.contains(target))
    const isOnKeyword = target.closest('.keyword-highlight')

    if (!isInsideTooltip && !isOnKeyword) {
      // Use requestAnimationFrame to ensure DOM state is stable
      requestAnimationFrame(() => {
        onClickOutside()
        // Also clean up any orphaned tooltips
        cleanupOrphanedTooltips()
      })
    }
  }

  // Use mousedown for faster response and to catch clicks before other handlers
  document.addEventListener('mousedown', globalClickHandler, true)
  document.addEventListener('touchstart', globalClickHandler, true)

  // Also add Escape key handler
  if (!globalKeyHandler) {
    globalKeyHandler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClickOutside()
        cleanupOrphanedTooltips()
      }
    }
    document.addEventListener('keydown', globalKeyHandler, true)
  }
}

function removeGlobalClickHandler() {
  if (globalClickHandler) {
    document.removeEventListener('mousedown', globalClickHandler, true)
    document.removeEventListener('touchstart', globalClickHandler, true)
    globalClickHandler = null
  }
  if (globalKeyHandler) {
    document.removeEventListener('keydown', globalKeyHandler, true)
    globalKeyHandler = null
  }
}

function hideTooltipsFromLevel(level: number) {
  while (tooltipStack.length > 0 && tooltipStack[tooltipStack.length - 1].level >= level) {
    const instance = tooltipStack.pop()
    if (instance) {
      instance.element.style.display = 'none'
      instance.element.remove()
    }
  }

  // Update has-child class on remaining tooltips
  updateParentHasChildClass()

  if (tooltipStack.length === 0) {
    removeGlobalClickHandler()
  }
}

function updateParentHasChildClass() {
  // Remove has-child class from all tooltips first
  tooltipStack.forEach(t => t.element.classList.remove('keyword-tooltip--has-child'))

  // Add has-child class to tooltips that have children at higher levels
  const maxLevel = tooltipStack.length > 0
    ? Math.max(...tooltipStack.map(t => t.level))
    : -1

  tooltipStack.forEach(t => {
    if (t.level < maxLevel) {
      t.element.classList.add('keyword-tooltip--has-child')
    }
  })
}

function hideAllTooltips() {
  clearPendingHide()
  hideTooltipsFromLevel(0)

  // Also hide all hover tooltips
  const hoverTooltips = document.querySelectorAll('.keyword-tooltip--hover')
  hoverTooltips.forEach(tooltip => {
    ; (tooltip as HTMLElement).style.display = 'none'
  })

  // Also clean up any orphaned tooltips (including hover)
  cleanupOrphanedTooltips(true)
}

export interface KeywordTooltipOptions {
  level?: number
  compiledKeywords?: CompiledKeywordSpan[]
  onKeywordDoubleInvoke?: (keywordId: string) => void
  underlineOnly?: boolean
  textIndent?: number  // 多段首行缩进值（em），0 或未定义则不缩进
}

export interface KeywordTooltipController {
  show: (target: HTMLElement, keywordId: string) => void
  hide: (target?: HTMLElement) => void
  pin: (target: HTMLElement, keywordId: string) => void
  unpin: () => void
  destroy: () => void
  hideAll: () => void
  getCurrentLevel: () => number
}

export function createKeywordTooltip(
  resolver: ContentResolver,
  options?: KeywordTooltipOptions
): KeywordTooltipController {
  if (typeof document === 'undefined') {
    return {
      show() { },
      hide() { },
      pin() { },
      unpin() { },
      destroy() { },
      hideAll() { },
      getCurrentLevel: () => 0
    }
  }

  const level = options?.level ?? 0
  const underlineOnly = options?.underlineOnly ?? false
  const textIndent = options?.textIndent ?? 0
  let currentTooltip: TooltipInstance | null = null
  let hoverTooltipElement: HTMLDivElement | null = null
  let nestedTooltipControllers: Map<string, KeywordTooltipController> = new Map()
  let currentHoveredKeywordId: string | null = null

  const getHoverTooltip = () => {
    if (!hoverTooltipElement) {
      hoverTooltipElement = createTooltipElement(level)
      hoverTooltipElement.classList.add('keyword-tooltip--hover')
    }
    return hoverTooltipElement
  }

  const cleanupNestedControllers = () => {
    nestedTooltipControllers.forEach(ctrl => ctrl.destroy())
    nestedTooltipControllers.clear()
    currentHoveredKeywordId = null
  }

  const getOrCreateNestedController = (keywordId: string): KeywordTooltipController => {
    let controller = nestedTooltipControllers.get(keywordId)
    if (!controller) {
      controller = createKeywordTooltip(resolver, {
        level: level + 1,
        compiledKeywords: options?.compiledKeywords,
        onKeywordDoubleInvoke: options?.onKeywordDoubleInvoke,
        underlineOnly: underlineOnly,
        textIndent: textIndent,
      })
      nestedTooltipControllers.set(keywordId, controller)
    }
    return controller
  }

  const setupTooltipEventDelegation = (tooltip: HTMLDivElement, currentKeywordId: string) => {
    // Use event delegation on the tooltip element for nested keyword interactions
    tooltip.addEventListener('mouseover', (e) => {
      const target = e.target as HTMLElement
      const keywordSpan = target.closest('.keyword-highlight') as HTMLElement
      if (!keywordSpan) {
        return
      }

      const nestedKeywordId = keywordSpan.dataset.keywordId

      if (!nestedKeywordId) {
        return
      }

      if (nestedKeywordId === currentKeywordId) {
        return
      }

      // Avoid re-showing if already hovered
      if (currentHoveredKeywordId === nestedKeywordId) {
        return
      }
      currentHoveredKeywordId = nestedKeywordId

      const nestedController = getOrCreateNestedController(nestedKeywordId)
      nestedController.show(keywordSpan, nestedKeywordId)
    })

    tooltip.addEventListener('mouseout', (e) => {
      const target = e.target as HTMLElement
      const keywordSpan = target.closest('.keyword-highlight') as HTMLElement
      if (!keywordSpan) return

      const nestedKeywordId = keywordSpan.dataset.keywordId
      if (!nestedKeywordId || nestedKeywordId === currentKeywordId) return

      // Check if we're moving to a child element (still inside the keyword)
      const relatedTarget = (e as MouseEvent).relatedTarget as HTMLElement
      if (relatedTarget && keywordSpan.contains(relatedTarget)) return

      currentHoveredKeywordId = null
      const nestedController = nestedTooltipControllers.get(nestedKeywordId)
      if (nestedController) {
        nestedController.hide()
      }
    })

    tooltip.addEventListener('click', (e) => {
      const target = e.target as HTMLElement
      const keywordSpan = target.closest('.keyword-highlight') as HTMLElement

      // If clicking on a nested keyword, handle it
      if (keywordSpan) {
        const nestedKeywordId = keywordSpan.dataset.keywordId
        if (!nestedKeywordId || nestedKeywordId === currentKeywordId) return

        e.stopPropagation()
        e.preventDefault()

        const nestedController = getOrCreateNestedController(nestedKeywordId)
        nestedController.pin(keywordSpan, nestedKeywordId)
      } else {
        // Clicking on non-keyword area of this tooltip - close all child tooltips
        hideTooltipsFromLevel(level + 1)
      }
    })

    tooltip.addEventListener('dblclick', (e) => {
      if (!options?.onKeywordDoubleInvoke) return

      const target = e.target as HTMLElement
      const keywordSpan = target.closest('.keyword-highlight') as HTMLElement
      if (!keywordSpan) return

      const nestedKeywordId = keywordSpan.dataset.keywordId
      if (!nestedKeywordId || nestedKeywordId === currentKeywordId) return
      e.stopPropagation()
      e.preventDefault()
      options.onKeywordDoubleInvoke(nestedKeywordId)
    })
  }

  const renderContent = (tooltip: HTMLDivElement, data: TooltipContent, keywordId: string, isPinned: boolean) => {
    cleanupNestedControllers()

    tooltip.innerHTML = ''
    tooltip.classList.toggle('keyword-tooltip--pinned', isPinned)

    const header = document.createElement('div')
    header.className = 'keyword-tooltip__header'
    header.textContent = data.title || '术语'
    tooltip.appendChild(header)

    // Show redirect info if matched via alias
    if (data.matchedVia && data.matchedVia.toLowerCase() !== data.title.toLowerCase()) {
      const redirect = document.createElement('div')
      redirect.className = 'keyword-tooltip__redirect'
      redirect.textContent = `重定向自: ${data.matchedVia}`
      tooltip.appendChild(redirect)
    }

    if (data.description) {
      const body = document.createElement('div')
      body.className = 'keyword-tooltip__body'
      body.addEventListener('click', (event) => {
        const target = event.target as HTMLElement | null
        if (!target) return
        if (target.closest('a')) return
        const spoiler = target.closest('.tiptap-spoiler') as HTMLElement | null
        if (!spoiler || !body.contains(spoiler)) return
        event.preventDefault()
        event.stopPropagation()
        spoiler.classList.toggle('is-revealed')
      })

      // 根据格式选择渲染方式
      const isRich = data.descriptionFormat === 'rich' && isTipTapJson(data.description)

      if (isRich) {
        // 富文本模式：使用 TipTap 渲染器
        const textRenderer = (text: string) =>
          applyHighlightsToText(text, options?.compiledKeywords || [], underlineOnly)
        body.innerHTML = tiptapJsonToHtml(data.description, { textRenderer })
        body.classList.add('keyword-tooltip__body--rich')
      } else {
        // 纯文本模式：按段落渲染
        // 检测是否包含换行，如果有则按段落渲染
        // 支持 \n, \r\n, \r 等各种换行符
        const paragraphs = data.description.split(/\r?\n|\r/)
        const shouldIndent = paragraphs.length > 1 && textIndent > 0

        if (shouldIndent) {
          body.classList.add('keyword-tooltip__body--indented')
          body.style.setProperty('--keyword-tooltip-text-indent', `${textIndent}em`)

          // 将每个段落包装在 p 标签中
          paragraphs.forEach((para, index) => {
            if (para.trim() === '' && index !== paragraphs.length - 1) {
              // 空行用 br 表示
              body.appendChild(document.createElement('br'))
              return
            }
            if (para.trim() === '') return

            const p = document.createElement('p')
            p.className = 'keyword-tooltip__paragraph'

            // Use renderTextWithImages for image support
            p.innerHTML = renderTextWithImages(para, options?.compiledKeywords, underlineOnly, level)
            body.appendChild(p)
          })
        } else {
          // 单段或禁用缩进时，使用原有逻辑 with image support
          body.innerHTML = renderTextWithImages(data.description, options?.compiledKeywords, underlineOnly, level)
        }
      }

      tooltip.appendChild(body)
    }

    // Setup event delegation for nested keywords
    setupTooltipEventDelegation(tooltip, keywordId)

    // Setup image click handler for ViewerJS
    setupImageViewer(tooltip)
  }

  const positionTooltip = (tooltip: HTMLDivElement, target: HTMLElement) => {
    tooltip.style.visibility = 'hidden'
    tooltip.style.display = 'block'
    tooltip.style.top = '0'
    tooltip.style.left = '0'

    const lowerTooltips = tooltipStack.filter(t => t.level < level && t.isPinned)
    const { top, left } = findBestPosition(target, tooltip, lowerTooltips)

    tooltip.style.visibility = 'visible'
    tooltip.style.top = `${top}px`
    tooltip.style.left = `${left}px`
  }

  const show = (target: HTMLElement, keywordId: string) => {
    clearPendingHide()

    if (currentTooltip?.isPinned) return

    const data = resolver(keywordId)
    if (!data) {
      hide()
      return
    }

    // Get matched source from target element
    const matchedVia = target.dataset.keywordSource || undefined
    const enrichedData = { ...data, matchedVia }

    const tooltip = getHoverTooltip()
    renderContent(tooltip, enrichedData, keywordId, false)
    positionTooltip(tooltip, target)
  }

  const hide = () => {
    clearPendingHide()
    pendingHideTimer = setTimeout(() => {
      if (hoverTooltipElement && !currentTooltip?.isPinned) {
        hoverTooltipElement.style.display = 'none'
      }
      pendingHideTimer = null
    }, 100)
  }

  const pin = (target: HTMLElement, keywordId: string) => {
    clearPendingHide()

    const data = resolver(keywordId)
    if (!data) return

    // Get matched source from target element
    const matchedVia = target.dataset.keywordSource || undefined
    const enrichedData = { ...data, matchedVia }

    hideTooltipsFromLevel(level)

    if (hoverTooltipElement) {
      hoverTooltipElement.style.display = 'none'
    }

    const tooltip = createTooltipElement(level)
    tooltip.classList.add('keyword-tooltip--pinned')
    renderContent(tooltip, enrichedData, keywordId, true)
    positionTooltip(tooltip, target)

    currentTooltip = {
      element: tooltip,
      level,
      keywordId,
      isPinned: true
    }
    tooltipStack.push(currentTooltip)

    // Update parent dim effect
    updateParentHasChildClass()

    setupGlobalClickHandler(() => {
      hideAllTooltips()
    })
  }

  const unpin = () => {
    if (currentTooltip) {
      hideTooltipsFromLevel(level)
      currentTooltip = null
    }
  }

  const destroy = () => {
    clearPendingHide()
    cleanupNestedControllers()
    if (hoverTooltipElement) {
      hoverTooltipElement.remove()
      hoverTooltipElement = null
    }
    if (currentTooltip) {
      const idx = tooltipStack.indexOf(currentTooltip)
      if (idx >= 0) {
        tooltipStack.splice(idx, 1)
      }
      currentTooltip.element.remove()
      currentTooltip = null
    }
    if (tooltipStack.length === 0) {
      removeGlobalClickHandler()
    }
  }

  return {
    show,
    hide,
    pin,
    unpin,
    destroy,
    hideAll: hideAllTooltips,
    getCurrentLevel: () => level
  }
}

function applyHighlightsToText(text: string, compiled: CompiledKeywordSpan[], underlineOnly: boolean): string {
  if (!text || !compiled.length) return escapeHtml(text)

  type Range = { start: number; end: number; keyword: CompiledKeywordSpan }
  const ranges: Range[] = []

  compiled.forEach((entry) => {
    const regex = new RegExp(
      entry.regex.source,
      entry.regex.flags.includes('g') ? entry.regex.flags : `${entry.regex.flags}g`
    )
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
  const filtered: Range[] = []
  let cursor = -1
  ranges.forEach((range) => {
    if (range.start < cursor) return
    filtered.push(range)
    cursor = range.end
  })

  let result = ''
  let lastIndex = 0
  filtered.forEach((range) => {
    if (range.start > lastIndex) {
      result += escapeHtml(text.slice(lastIndex, range.start))
    }
    const classes = ['keyword-highlight']
    const shouldUnderline =
      range.keyword.display === 'minimal' ||
      (underlineOnly && range.keyword.display === 'inherit')
    if (shouldUnderline) {
      classes.push('keyword-highlight--underline')
    }
    const matchedText = text.slice(range.start, range.end)
    result += `<span class="${classes.join(' ')}" data-keyword-id="${escapeHtml(range.keyword.id)}" data-keyword-source="${escapeHtml(range.keyword.source)}">${escapeHtml(matchedText)}</span>`
    lastIndex = range.end
  })
  if (lastIndex < text.length) {
    result += escapeHtml(text.slice(lastIndex))
  }

  return result
}

function escapeHtml(text: string): string {
  const div = document.createElement('div')
  div.textContent = text
  return div.innerHTML
}

/**
 * Parse markdown image syntax ![alt](url) and convert to HTML img tags.
 * Supports id:xxx format for attachment IDs.
 */
function parseMarkdownImages(text: string): string {
  if (!text) return ''
  const imageRegex = createImageTokenRegex()
  let result = ''
  let lastIndex = 0
  let match: RegExpExecArray | null

  while ((match = imageRegex.exec(text)) !== null) {
    if (match.index > lastIndex) {
      result += escapeHtml(text.slice(lastIndex, match.index))
    }

    const [full, alt, token] = match
    if (!isValidAttachmentToken(token)) {
      result += escapeHtml(full)
    } else {
      const url = `/api/v1/attachment/${token}`
      const thumbUrl = `/api/v1/attachment/${token}/thumb?size=150`
      const escapedAlt = escapeHtml(alt || '')
      result += `<img class="keyword-tooltip__image" src="${thumbUrl}" data-original="${url}" alt="${escapedAlt}" loading="lazy" />`
    }

    lastIndex = match.index + full.length
  }

  if (lastIndex < text.length) {
    result += escapeHtml(text.slice(lastIndex))
  }

  return result
}

/**
 * Apply image parsing to text content, combining with optional keyword highlights.
 */
function renderTextWithImages(text: string, compiled?: CompiledKeywordSpan[], underlineOnly = false, level = 0): string {
  if (!text) return ''

  const imageRegex = createImageTokenRegex()
  const hasImages = imageRegex.test(text)
  imageRegex.lastIndex = 0

  if (!hasImages) {
    if (compiled && compiled.length > 0 && level < MAX_NESTING_DEPTH - 1) {
      return applyHighlightsToText(text, compiled, underlineOnly)
    }
    return escapeHtml(text)
  }

  const parts: string[] = []
  let lastIndex = 0
  let match: RegExpExecArray | null

  while ((match = imageRegex.exec(text)) !== null) {
    if (match.index > lastIndex) {
      const textPart = text.slice(lastIndex, match.index)
      if (compiled && compiled.length > 0 && level < MAX_NESTING_DEPTH - 1) {
        parts.push(applyHighlightsToText(textPart, compiled, underlineOnly))
      } else {
        parts.push(escapeHtml(textPart))
      }
    }

    const [full, alt, token] = match
    if (!isValidAttachmentToken(token)) {
      if (compiled && compiled.length > 0 && level < MAX_NESTING_DEPTH - 1) {
        parts.push(applyHighlightsToText(full, compiled, underlineOnly))
      } else {
        parts.push(escapeHtml(full))
      }
    } else {
      const url = `/api/v1/attachment/${token}`
      const thumbUrl = `/api/v1/attachment/${token}/thumb?size=150`
      parts.push(`<img class="keyword-tooltip__image" src="${thumbUrl}" data-original="${url}" alt="${escapeHtml(alt || '')}" loading="lazy" />`)
    }

    lastIndex = match.index + full.length
  }

  if (lastIndex < text.length) {
    const textPart = text.slice(lastIndex)
    if (compiled && compiled.length > 0 && level < MAX_NESTING_DEPTH - 1) {
      parts.push(applyHighlightsToText(textPart, compiled, underlineOnly))
    } else {
      parts.push(escapeHtml(textPart))
    }
  }

  return parts.join('')
}

/**
 * Setup image click handler with ViewerJS (no navbar/preview panel).
 * Dynamically imports ViewerJS to avoid bundling if unused.
 */
function setupImageViewer(tooltip: HTMLDivElement) {
  if (tooltip.dataset.imageViewerBound === '1') return
  tooltip.dataset.imageViewerBound = '1'

  tooltip.addEventListener('click', async (e) => {
    const target = e.target as HTMLElement
    if (target.classList.contains('keyword-tooltip__image')) {
      e.stopPropagation()
      e.preventDefault()

      const img = target as HTMLImageElement
      const originalUrl = img.dataset.original || img.src

      // Dynamically import ViewerJS
      try {
        const { default: Viewer } = await import('viewerjs')

        // Create a temporary container with the full image
        const container = document.createElement('div')
        container.style.display = 'none'
        const fullImg = document.createElement('img')
        fullImg.src = originalUrl
        container.appendChild(fullImg)
        document.body.appendChild(container)

        const viewer = new Viewer(container, {
          navbar: false,  // No preview panel
          title: false,
          toolbar: {
            zoomIn: true,
            zoomOut: true,
            oneToOne: true,
            reset: true,
            prev: false,
            next: false,
            play: false,
            rotateLeft: true,
            rotateRight: true,
            flipHorizontal: false,
            flipVertical: false,
          },
          zIndex: 1000000,
          hidden() {
            viewer.destroy()
            container.remove()
          },
        })

        viewer.show()
      } catch (error) {
        console.warn('[KeywordTooltip] Failed to load ViewerJS:', error)
        // Fallback: open image in new tab
        window.open(originalUrl, '_blank')
      }
    }
  })
}

export { applyHighlightsToText, MAX_NESTING_DEPTH, parseMarkdownImages, renderTextWithImages }

