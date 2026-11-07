import { onBeforeUnmount, ref, type Ref } from 'vue'

// 只能从排序把手开始的指针排序。鼠标、触控笔和触摸共用 Pointer Events；
// 把手设置 touch-action: none 并捕获指针，列表其余区域保持原生滚动和文字选择。

const ITEM_SELECTOR = '[data-favorite-sort-id]'
const EDGE = 44
const MAX_SCROLL_STEP = 14
const MOVE_THRESHOLD = 4

export const useFavoritePointerSort = (options: {
  scroller: Ref<HTMLElement | null>
  enabled: () => boolean
  onCommit: (id: string, toIndex: number) => void
}) => {
  const sortingId = ref('')
  // 插入位置：0..n，表示落在第 n 项之前；n 等于项数时落在末尾。
  const dropIndex = ref(-1)
  let active: { pointerId: number, target: HTMLElement, fromIndex: number, startY: number, moved: boolean } | null = null
  let pointerY = 0
  let frame = 0

  const items = () => Array.from(options.scroller.value?.querySelectorAll<HTMLElement>(ITEM_SELECTOR) || [])

  const resolve = () => {
    frame = 0
    if (!active) return
    const elements = items()
    let index = elements.length
    for (let i = 0; i < elements.length; i += 1) {
      const box = elements[i].getBoundingClientRect()
      if (pointerY < box.top + box.height / 2) {
        index = i
        break
      }
    }
    dropIndex.value = index
    const scroller = options.scroller.value
    if (!scroller) return
    const box = scroller.getBoundingClientRect()
    const step = pointerY < box.top + EDGE
      ? -Math.ceil(((box.top + EDGE - pointerY) / EDGE) * MAX_SCROLL_STEP)
      : pointerY > box.bottom - EDGE
        ? Math.ceil(((pointerY - box.bottom + EDGE) / EDGE) * MAX_SCROLL_STEP)
        : 0
    if (step) {
      const before = scroller.scrollTop
      scroller.scrollTop += step
      if (scroller.scrollTop !== before) frame = requestAnimationFrame(resolve)
    }
  }

  const reset = () => {
    if (frame) cancelAnimationFrame(frame)
    frame = 0
    if (active) {
      try {
        if (active.target.hasPointerCapture?.(active.pointerId)) active.target.releasePointerCapture(active.pointerId)
      } catch {
        // ignore
      }
    }
    active = null
    sortingId.value = ''
    dropIndex.value = -1
  }

  const begin = (event: PointerEvent, id: string) => {
    if (!options.enabled() || active) return
    if (event.pointerType === 'mouse' && event.button !== 0) return
    const fromIndex = items().findIndex((element) => element.dataset.favoriteSortId === id)
    if (fromIndex < 0) return
    event.preventDefault()
    const target = event.currentTarget as HTMLElement
    try {
      target.setPointerCapture(event.pointerId)
    } catch {
      return
    }
    active = { pointerId: event.pointerId, target, fromIndex, startY: event.clientY, moved: false }
    pointerY = event.clientY
  }

  const move = (event: PointerEvent) => {
    if (!active || active.pointerId !== event.pointerId) return
    event.preventDefault()
    pointerY = event.clientY
    if (!active.moved) {
      if (Math.abs(event.clientY - active.startY) < MOVE_THRESHOLD) return
      active.moved = true
      sortingId.value = active.target.closest<HTMLElement>(ITEM_SELECTOR)?.dataset.favoriteSortId || ''
    }
    if (!frame) frame = requestAnimationFrame(resolve)
  }

  const end = (event: PointerEvent) => {
    if (!active || active.pointerId !== event.pointerId) return
    const current = active
    pointerY = event.clientY
    if (frame) cancelAnimationFrame(frame)
    resolve()
    const insertAt = dropIndex.value
    const id = sortingId.value
    reset()
    if (event.type !== 'pointerup' || !current.moved || !id || insertAt < 0) return
    const toIndex = insertAt > current.fromIndex ? insertAt - 1 : insertAt
    if (toIndex !== current.fromIndex) options.onCommit(id, toIndex)
  }

  onBeforeUnmount(reset)

  return { sortingId, dropIndex, begin, move, end, cancel: reset }
}
