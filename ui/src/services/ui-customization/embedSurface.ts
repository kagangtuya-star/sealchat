import type { UIRuntimeSurface } from './surfaces'

export function resolveEffectiveCustomizationSurface(baseSurface: UIRuntimeSurface, isChatEmbed: boolean): UIRuntimeSurface {
  return baseSurface === 'embed' && isChatEmbed ? 'chat' : baseSurface
}

// Match both the actual frame and its built-in chat host, including Teleport roots.
const CHAT_EMBED_FRAMES = [
  '.inline-chat-split .inline-chat-split__iframe',
  '.sc-split-panes .sc-split-iframe.is-chat',
  '.theater-host .theater-host-chat .theater-host-chat-frame',
  '.theater-floating-host .theater-floating-window.is-chat .theater-floating-window__frame',
].join(', ')

/** Only actual same-origin, built-in chat frames change the embed surface. */
export function detectChatEmbedHost(
  routeName: unknown,
  win: Pick<Window, 'parent' | 'frameElement'> | undefined = typeof window === 'undefined' ? undefined : window,
): boolean {
  if (routeName !== 'embed' || !win || win.parent === win) return false
  try {
    const parentDocument = win.parent.document
    // frameElement belongs to the parent realm; instanceof checks are unsafe here.
    const frame = win.frameElement
    if (!frame) return false
    if (frame.closest('[data-sc-ui="workbench-panel"][data-sc-panel-type="chat"]')
      && parentDocument.querySelector('[data-sc-ui="workbench"], html[data-sc-workbench]')) return true
    return frame.matches(CHAT_EMBED_FRAMES)
  } catch {
    // Cross-origin parents (and unavailable/detached frames) remain ordinary embeds.
    return false
  }
}
