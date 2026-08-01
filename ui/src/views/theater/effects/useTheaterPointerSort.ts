import { onBeforeUnmount } from 'vue'

export type TheaterPointerDrag = {
  kind: 'folder' | 'item'
  ids: string[]
}

export type TheaterPointerTarget = {
  kind: 'folder' | 'item' | 'bucket'
  id: string
  folderId: string
}

export const useTheaterPointerSort = (
  onDrop: (drag: TheaterPointerDrag, target: TheaterPointerTarget) => void,
) => {
  let active: { pointerId: number, drag: TheaterPointerDrag, startX: number, startY: number, moved: boolean } | null = null
  let targetElement: HTMLElement | null = null
  let frame = 0
  let point = { x: 0, y: 0 }

  const clearTarget = () => {
    targetElement?.classList.remove('is-pointer-sort-target')
    targetElement = null
  }

  const resolveTarget = () => {
    frame = 0
    const hovered = document.elementFromPoint(point.x, point.y)
    const element = hovered?.closest<HTMLElement>('[data-theater-sort-kind]') || null
    if (element !== targetElement) {
      clearTarget()
      targetElement = element
      targetElement?.classList.add('is-pointer-sort-target')
    }
    const scroll = hovered?.closest<HTMLElement>('[data-theater-sort-scroll]') || null
    if (scroll) {
      const rect = scroll.getBoundingClientRect()
      const edge = Math.min(42, rect.height / 4)
      const speed = point.y < rect.top + edge ? -10 : point.y > rect.bottom - edge ? 10 : 0
      if (speed) {
        scroll.scrollTop += speed
        frame = requestAnimationFrame(resolveTarget)
      }
    }
  }

  const begin = (event: PointerEvent, drag: TheaterPointerDrag) => {
    if (event.button !== 0 || !drag.ids.length) return
    event.preventDefault()
    const target = event.currentTarget as HTMLElement
    target.setPointerCapture(event.pointerId)
    active = { pointerId: event.pointerId, drag, startX: event.clientX, startY: event.clientY, moved: false }
    point = { x: event.clientX, y: event.clientY }
    target.classList.add('is-pointer-sorting')
  }

  const move = (event: PointerEvent) => {
    if (!active || active.pointerId !== event.pointerId) return
    event.preventDefault()
    if (!active.moved) {
      const deltaX = event.clientX - active.startX
      const deltaY = event.clientY - active.startY
      if (deltaX * deltaX + deltaY * deltaY < 16) return
      active.moved = true
    }
    point = { x: event.clientX, y: event.clientY }
    if (!frame) frame = requestAnimationFrame(resolveTarget)
  }

  const finish = (event: PointerEvent, cancelled = false) => {
    if (!active || active.pointerId !== event.pointerId) return
    if (frame) cancelAnimationFrame(frame)
    frame = 0
    point = { x: event.clientX, y: event.clientY }
    resolveTarget()
    if (frame) cancelAnimationFrame(frame)
    frame = 0
    const drag = active.drag
    const moved = active.moved
    active = null
    const source = event.currentTarget as HTMLElement
    source.classList.remove('is-pointer-sorting')
    const element = targetElement
    clearTarget()
    if (cancelled || !moved || !element) return
    const kind = element.dataset.theaterSortKind as TheaterPointerTarget['kind'] | undefined
    if (!kind) return
    onDrop(drag, {
      kind,
      id: element.dataset.targetId || element.dataset.folderId || '',
      folderId: element.dataset.folderId || '',
    })
  }

  const end = (event: PointerEvent) => finish(event)
  const cancel = (event: PointerEvent) => finish(event, true)

  onBeforeUnmount(() => {
    if (frame) cancelAnimationFrame(frame)
    clearTarget()
    active = null
  })

  return { begin, move, end, cancel }
}
