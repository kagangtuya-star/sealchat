interface TooltipContent {
  title: string
  description: string
}

type ContentResolver = (keywordId: string) => TooltipContent | null | undefined

let sharedTooltip: HTMLDivElement | null = null
let sharedTooltipRefCount = 0

function acquireTooltip() {
  if (!sharedTooltip) {
    sharedTooltip = document.createElement('div')
    sharedTooltip.className = 'keyword-tooltip'
    sharedTooltip.style.display = 'none'
    document.body.appendChild(sharedTooltip)
  }
  sharedTooltipRefCount += 1
  return sharedTooltip
}

function releaseTooltip() {
  sharedTooltipRefCount = Math.max(0, sharedTooltipRefCount - 1)
  if (sharedTooltipRefCount === 0 && sharedTooltip) {
    sharedTooltip.remove()
    sharedTooltip = null
  }
}

export function createKeywordTooltip(resolver: ContentResolver) {
  if (typeof document === 'undefined') {
    return {
      show() {},
      hide() {},
      destroy() {},
    }
  }
  const tooltip = acquireTooltip()

  const hide = () => {
    tooltip.style.display = 'none'
  }

  const show = (target: HTMLElement, keywordId: string) => {
    const data = resolver(keywordId)
    if (!data) {
      hide()
      return
    }
    const title = data.title || '术语'
    const description = data.description || ''
    tooltip.innerHTML = ''
    const header = document.createElement('div')
    header.className = 'keyword-tooltip__header'
    header.textContent = title
    tooltip.appendChild(header)
    if (description) {
      const body = document.createElement('div')
      body.className = 'keyword-tooltip__body'
      body.textContent = description
      tooltip.appendChild(body)
    }
    tooltip.style.visibility = 'hidden'
    tooltip.style.display = 'block'
    tooltip.style.top = '0'
    tooltip.style.left = '0'
    const rect = target.getBoundingClientRect()
    const { offsetWidth, offsetHeight } = tooltip
    const gap = 12
    const top = Math.max(8, rect.top + window.scrollY - offsetHeight - gap)
    const maxLeft = window.innerWidth - offsetWidth - 8
    const centered = rect.left + rect.width / 2 - offsetWidth / 2
    const left = Math.min(maxLeft, Math.max(8, centered)) + window.scrollX
    tooltip.style.visibility = 'visible'
    tooltip.style.top = `${top}px`
    tooltip.style.left = `${left}px`
  }

  const destroy = () => {
    hide()
    releaseTooltip()
  }

  return { show, hide, destroy }
}
