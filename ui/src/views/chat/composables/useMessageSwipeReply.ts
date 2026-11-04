import { computed, onScopeDispose, ref, watch, type Ref } from 'vue'

export const MESSAGE_SWIPE_REPLY = {
  deadZone: 16,
  directionRatio: 1.6,
  activationDistance: 64,
  verticalLimit: 44,
  edgeGuard: 24,
} as const

// Kept independent of DOM events so release and movement use exactly the same rules.
export function createMessageSwipeReplyGesture() {
  let phase: 'idle' | 'pending' | 'locked' = 'idle'
  let startX = 0
  let startY = 0
  let lastY = 0
  let verticalTravel = 0
  let distance = 0
  const cancel = () => { phase = 'idle'; distance = 0 }
  const move = (x: number, y: number) => {
    if (phase === 'idle') return
    verticalTravel += Math.abs(y - lastY)
    lastY = y
    const dx = startX - x
    const dy = Math.abs(y - startY)
    if (phase === 'pending') {
      if (Math.hypot(dx, dy) < MESSAGE_SWIPE_REPLY.deadZone) return
      // Do not reject a swipe because the first meaningful sample is slightly diagonal.
      // Only give the gesture to vertical scrolling once vertical intent is clearly dominant.
      if (dx <= -MESSAGE_SWIPE_REPLY.deadZone) return cancel()
      if (dy >= MESSAGE_SWIPE_REPLY.deadZone && dy > Math.max(dx, 0)) return cancel()
      if (dx < MESSAGE_SWIPE_REPLY.deadZone || dx < dy * MESSAGE_SWIPE_REPLY.directionRatio) return
      phase = 'locked'
    }
    if (dx <= 0 || verticalTravel > MESSAGE_SWIPE_REPLY.verticalLimit) return cancel()
    distance = dx
  }
  return {
    start(x: number, y: number) {
      startX = x; startY = y; lastY = y
      verticalTravel = 0; distance = 0; phase = 'pending'
    },
    move,
    cancel,
    get ready() { return phase === 'locked' && distance >= MESSAGE_SWIPE_REPLY.activationDistance },
    get locked() { return phase === 'locked' },
    get active() { return phase !== 'idle' },
    get distance() { return distance },
  }
}

const EXCLUDED_TARGETS = [
  'a', 'button', 'input', 'textarea', 'select', 'label', 'summary',
  'audio', 'video', 'iframe', 'object', 'embed', 'canvas',
  '[contenteditable]:not([contenteditable="false"])', '[tabindex]', '[draggable="true"]',
  '[role="button"]', '[role="link"]', '[role="slider"]', '[role="checkbox"]',
  '[data-swipe-reply-ignore]', '.message-row__handle', '.message-action-bar',
  '.message-iform-embed', '.message-sticky-note-embed', '.battle-report-embed-card',
  '.clue-message-card',
].join(',')

export function isMessageSwipeReplyTarget(target: EventTarget | null, host: HTMLElement) {
  if (!(target instanceof Element) || !host.contains(target) || target.closest(EXCLUDED_TARGETS)) return false
  // Include ancestors: a theme may put the content host inside a horizontal scroller.
  for (let node: Element | null = target; node; node = node.parentElement) {
    const style = getComputedStyle(node)
    if (/(auto|scroll)/.test(style.overflowX) && node.scrollWidth > node.clientWidth) return false
  }
  return true
}

interface MessageSwipeReplyOptions {
  enabled: () => boolean
  // The caller supplies a stable reactive context, invalidated on message/channel changes.
  context: () => unknown
  reply: () => void
}

export function useMessageSwipeReply(host: Ref<HTMLElement | null>, options: MessageSwipeReplyOptions) {
  const distance = ref(0)
  const progress = computed(() => Math.min(distance.value / MESSAGE_SWIPE_REPLY.activationDistance, 1))
  const ready = computed(() => distance.value >= MESSAGE_SWIPE_REPLY.activationDistance)
  const gesture = createMessageSwipeReplyGesture()
  let touchId: number | null = null
  let context: unknown
  let blockLongPress = false
  let suppressClick = false
  let triggered = false
  let detachActiveListeners: (() => void) | undefined
  const hasSelection = () => window.getSelection()?.isCollapsed === false
  const cancel = () => {
    if (touchId !== null) blockLongPress = true
    touchId = null
    gesture.cancel()
    triggered = false
    distance.value = 0
    detachActiveListeners?.()
    detachActiveListeners = undefined
  }
  const valid = () => options.enabled() && options.context() === context && !hasSelection()
  const claimLongPress = () => {
    if (blockLongPress) return false
    cancel()
    blockLongPress = true
    return true
  }

  watch([options.enabled, options.context], cancel, { flush: 'sync' })
  watch(host, (element, _, onCleanup) => {
    if (!element) return
    const onStart = (event: TouchEvent) => {
      cancel()
      blockLongPress = false
      suppressClick = false
      const touch = event.touches[0]
      if (event.touches.length !== 1) { blockLongPress = true; return }
      if (!options.enabled() || hasSelection() || !isMessageSwipeReplyTarget(event.target, element)) return
      if (touch.clientX <= MESSAGE_SWIPE_REPLY.edgeGuard
        || touch.clientX >= window.innerWidth - MESSAGE_SWIPE_REPLY.edgeGuard) return
      touchId = touch.identifier
      context = options.context()
      gesture.start(touch.clientX, touch.clientY)
      const onAdditionalTouch = (next: TouchEvent) => { if (next.touches.length !== 1) cancel() }
      const onSelection = () => { if (hasSelection()) cancel() }
      const onVisibility = () => { if (document.hidden) cancel() }
      // Global listeners exist only during this single touch, and never prevent scrolling.
      document.addEventListener('touchstart', onAdditionalTouch, { capture: true, passive: true })
      document.addEventListener('scroll', cancel, { capture: true, passive: true })
      document.addEventListener('selectionchange', onSelection)
      document.addEventListener('visibilitychange', onVisibility)
      window.addEventListener('blur', cancel)
      detachActiveListeners = () => {
        document.removeEventListener('touchstart', onAdditionalTouch, { capture: true })
        document.removeEventListener('scroll', cancel, { capture: true })
        document.removeEventListener('selectionchange', onSelection)
        document.removeEventListener('visibilitychange', onVisibility)
        window.removeEventListener('blur', cancel)
      }
    }
    const onMove = (event: TouchEvent) => {
      if (touchId === null) return
      if (event.touches.length !== 1 || event.touches[0].identifier !== touchId || !valid()) return cancel()
      if (triggered) {
        if (event.cancelable) event.preventDefault()
        return
      }
      const touch = event.touches[0]
      gesture.move(touch.clientX, touch.clientY)
      if (!gesture.active) return cancel()
      if (gesture.locked) {
        blockLongPress = true
        suppressClick = true
        // A non-cancelable move means the browser already owns the gesture.
        if (!event.cancelable) return cancel()
        event.preventDefault()
      }
      distance.value = gesture.distance
      if (gesture.ready) {
        triggered = true
        options.reply()
      }
    }
    const onEnd = (event: TouchEvent) => {
      if (touchId === null) {
        if (event.touches.length === 0) blockLongPress = false
        return
      }
      cancel()
      if (event.touches.length === 0) blockLongPress = false
    }
    const onCancel = () => {
      cancel()
      blockLongPress = false
      suppressClick = false
    }
    const onPointerDown = (event: PointerEvent) => {
      if (event.pointerType === 'mouse' || event.pointerType === 'pen') {
        blockLongPress = false
        suppressClick = false
      }
    }
    const onClick = (event: MouseEvent) => {
      if (!suppressClick || event.detail === 0) return
      suppressClick = false
      event.preventDefault()
      event.stopImmediatePropagation()
    }
    element.addEventListener('touchstart', onStart, { passive: true })
    element.addEventListener('touchmove', onMove, { passive: false })
    element.addEventListener('touchend', onEnd, { passive: true })
    element.addEventListener('touchcancel', onCancel, { passive: true })
    element.addEventListener('pointercancel', onCancel)
    element.addEventListener('pointerdown', onPointerDown, { capture: true })
    element.addEventListener('click', onClick, { capture: true })
    onCleanup(() => {
      cancel()
      element.removeEventListener('touchstart', onStart)
      element.removeEventListener('touchmove', onMove)
      element.removeEventListener('touchend', onEnd)
      element.removeEventListener('touchcancel', onCancel)
      element.removeEventListener('pointercancel', onCancel)
      element.removeEventListener('pointerdown', onPointerDown, { capture: true })
      element.removeEventListener('click', onClick, { capture: true })
    })
  }, { flush: 'post', immediate: true })
  onScopeDispose(cancel)
  return { progress, ready, claimLongPress, blocksLongPress: () => blockLongPress }
}
