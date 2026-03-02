const FONT_SURFACE_ATTR = 'data-sc-font-surface'
const FONT_SURFACE_IGNORE_ATTR = 'data-sc-font-surface-ignore'

const FLOATING_SURFACE_SELECTOR = [
  '.v-binder-follower-container',
  '.v-binder-follower-content',
  '.n-message-container',
  '.n-message-wrapper',
  '.n-message',
  '.n-notification-container',
  '.n-notification-wrapper',
  '.n-notification',
  '.n-dialog-container',
  '.n-dialog',
  '.n-modal',
  '.n-modal-container',
  '.n-drawer',
  '.n-popover',
  '.n-dropdown-menu',
  '.n-base-select-menu',
  '.n-select-menu',
  '.n-auto-complete-menu',
  '.n-cascader-menu',
  '.n-date-panel',
  '.n-time-picker-panel',
  '.n-color-picker-panel',
  '.n-image-preview-container',
  '.mx-context-menu',
  '.context-menu',
  '[role="dialog"]',
  '[role="menu"]',
  '[role="listbox"]',
].join(',')

let observerStarted = false

const isElement = (value: unknown): value is HTMLElement => value instanceof HTMLElement

const shouldMarkSurface = (el: HTMLElement): boolean => {
  if (el.getAttribute(FONT_SURFACE_IGNORE_ATTR) === 'true') return false
  if (el.closest(`[${FONT_SURFACE_IGNORE_ATTR}="true"]`)) return false
  if (el.closest('#app')) return false
  return true
}

const markSurface = (el: HTMLElement) => {
  if (!shouldMarkSurface(el)) return
  if (el.getAttribute(FONT_SURFACE_ATTR) === 'true') return
  el.setAttribute(FONT_SURFACE_ATTR, 'true')
}

const markNodeAndDescendants = (root: HTMLElement) => {
  if (root.matches(FLOATING_SURFACE_SELECTOR)) {
    markSurface(root)
  }
  root.querySelectorAll<HTMLElement>(FLOATING_SURFACE_SELECTOR).forEach(markSurface)
}

const scanExistingFloatingSurfaces = () => {
  if (typeof document === 'undefined') return
  document.querySelectorAll<HTMLElement>(FLOATING_SURFACE_SELECTOR).forEach(markSurface)
}

export const startFontSurfaceAutoMarking = () => {
  if (observerStarted) return
  if (typeof window === 'undefined' || typeof document === 'undefined') return
  if (typeof MutationObserver === 'undefined') return

  const start = () => {
    if (observerStarted) return
    const body = document.body
    if (!body) return
    observerStarted = true
    scanExistingFloatingSurfaces()
    const observer = new MutationObserver((mutations) => {
      for (const mutation of mutations) {
        mutation.addedNodes.forEach(node => {
          if (!isElement(node)) return
          markNodeAndDescendants(node)
        })
      }
    })
    observer.observe(body, { childList: true, subtree: true })
  }

  if (document.readyState === 'loading') {
    window.addEventListener('DOMContentLoaded', start, { once: true })
    return
  }
  start()
}
