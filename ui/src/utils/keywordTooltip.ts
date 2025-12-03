interface TooltipContent {
  title: string
  description: string
}

type ContentResolver = (keywordId: string) => TooltipContent | null | undefined

export function createKeywordTooltip(resolver: ContentResolver) {
  if (typeof document === 'undefined') {
    return {
      show() {},
      hide() {},
    }
  }
  const tooltip = document.createElement('div')
  tooltip.className = 'keyword-tooltip'
  tooltip.style.display = 'none'
  document.body.appendChild(tooltip)

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

  return { show, hide }
}
