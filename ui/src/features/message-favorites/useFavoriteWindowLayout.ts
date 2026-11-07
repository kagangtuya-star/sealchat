import { computed, onBeforeUnmount, reactive, ref, watch, type Ref } from 'vue'
import { useEventListener } from '@vueuse/core'
import {
  createFloatingBadgeGesture,
  finishFloatingBadgeGesture,
  updateFloatingBadgeGesture,
  type FloatingBadgeGestureState,
} from '@/components/iform/floatingBadgeGesture'

// 收藏悬浮窗的纯 UI 几何状态：位置、尺寸、最小化与图标位置，只保存在浏览器本地。
// 手势统一使用 Pointer Events + pointer capture，事件绑定在手柄元素自身，
// 因此拖动结束或组件卸载后不会残留 window 级 pointer 监听。

export type FavoriteWindowMode = 'hidden' | 'expanded' | 'minimized'
export type FavoriteResizeDirection = 'n' | 's' | 'e' | 'w' | 'ne' | 'nw' | 'se' | 'sw'

interface PersistedLayout {
  version: 1
  mode: FavoriteWindowMode
  x: number
  y: number
  width: number
  height: number
  badgeX: number
  badgeY: number
}

type Gesture =
  | { kind: 'move', pointerId: number, target: HTMLElement, startX: number, startY: number, originX: number, originY: number }
  | { kind: 'resize', pointerId: number, target: HTMLElement, startX: number, startY: number, origin: Rect, direction: FavoriteResizeDirection }
  | { kind: 'badge', pointerId: number, target: HTMLElement, state: FloatingBadgeGestureState }

interface Rect { x: number, y: number, width: number, height: number }

const STORAGE_PREFIX = 'sealchat:message-favorites:window:v1'
const PADDING = 8
const MIN_WIDTH = 300
const MIN_HEIGHT = 340
const COMPACT_BREAKPOINT = 640
export const FAVORITE_BADGE_SIZE = 48

const finite = (value: unknown, fallback: number) => (typeof value === 'number' && Number.isFinite(value) ? value : fallback)

// embedded：分屏 pane 等同源 iframe 宿主。它们共享 localStorage，因此使用独立键，
// 并且不自动恢复打开状态，只在显式入口打开，避免多个 pane 同时弹出窗口。
export const useFavoriteWindowLayout = (userId: Ref<string>, options: { embedded?: boolean } = {}) => {
  const viewport = reactive({
    width: typeof window === 'undefined' ? 1280 : Math.max(1, window.innerWidth),
    height: typeof window === 'undefined' ? 800 : Math.max(1, window.innerHeight),
  })
  const compact = computed(() => viewport.width < COMPACT_BREAKPOINT)
  const storageKey = computed(() => `${STORAGE_PREFIX}:${userId.value || 'anonymous'}:${options.embedded ? 'embed:' : ''}${compact.value ? 'compact' : 'regular'}`)

  const defaults = (): PersistedLayout => {
    const width = compact.value ? viewport.width - PADDING * 2 : 400
    const height = compact.value ? Math.round(viewport.height * 0.72) : Math.min(640, viewport.height - 120)
    return {
      version: 1,
      mode: 'hidden',
      x: compact.value ? PADDING : viewport.width - width - 24,
      y: compact.value ? Math.max(PADDING, viewport.height - height - 72) : 72,
      width,
      height,
      badgeX: viewport.width - FAVORITE_BADGE_SIZE - 16,
      badgeY: Math.round(viewport.height * 0.55),
    }
  }

  // 用户选择的几何；渲染值另行按视口约束，不把临时缩小的视口写回偏好。
  const layout = reactive<PersistedLayout>(defaults())
  const gesture = ref<Gesture | null>(null)

  const load = () => {
    const fallback = defaults()
    let next = fallback
    try {
      const raw = localStorage.getItem(storageKey.value)
      const parsed = raw ? JSON.parse(raw) as Partial<PersistedLayout> : null
      if (parsed?.version === 1) {
        next = {
          version: 1,
          mode: parsed.mode === 'expanded' || parsed.mode === 'minimized' ? parsed.mode : 'hidden',
          x: finite(parsed.x, fallback.x),
          y: finite(parsed.y, fallback.y),
          width: finite(parsed.width, fallback.width),
          height: finite(parsed.height, fallback.height),
          badgeX: finite(parsed.badgeX, fallback.badgeX),
          badgeY: finite(parsed.badgeY, fallback.badgeY),
        }
      }
    } catch {
      next = fallback
    }
    // 切换账号或断点时保持当前打开状态，只替换几何。
    releaseGesture()
    const restoredMode = options.embedded ? 'hidden' : next.mode
    Object.assign(layout, next, { mode: layout.mode !== 'hidden' ? layout.mode : restoredMode })
  }

  const persist = () => {
    try {
      localStorage.setItem(storageKey.value, JSON.stringify(layout))
    } catch {
      // 存储不可用时只失去记忆能力。
    }
  }

  const minSize = computed(() => ({
    width: Math.min(MIN_WIDTH, viewport.width - PADDING * 2),
    height: Math.min(MIN_HEIGHT, viewport.height - PADDING * 2),
  }))

  const clampRect = (rect: Rect): Rect => {
    const width = Math.round(Math.min(Math.max(rect.width, minSize.value.width), viewport.width - PADDING * 2))
    const height = Math.round(Math.min(Math.max(rect.height, minSize.value.height), viewport.height - PADDING * 2))
    return {
      width,
      height,
      x: Math.round(Math.min(Math.max(rect.x, PADDING), viewport.width - width - PADDING)),
      y: Math.round(Math.min(Math.max(rect.y, PADDING), viewport.height - height - PADDING)),
    }
  }

  const clampBadge = (x: number, y: number) => ({
    x: Math.round(Math.min(Math.max(x, PADDING), viewport.width - FAVORITE_BADGE_SIZE - PADDING)),
    y: Math.round(Math.min(Math.max(y, PADDING), viewport.height - FAVORITE_BADGE_SIZE - PADDING)),
  })

  const rect = computed(() => clampRect(layout))
  const badge = computed(() => clampBadge(layout.badgeX, layout.badgeY))

  const setMode = (mode: FavoriteWindowMode) => {
    if (layout.mode === mode) return
    releaseGesture()
    layout.mode = mode
    persist()
  }

  // 拖动结束时把约束后的渲染值写回，避免下次打开时出现跳动。
  const commitRect = (next: Rect) => {
    const clamped = clampRect(next)
    layout.x = clamped.x
    layout.y = clamped.y
    layout.width = clamped.width
    layout.height = clamped.height
  }

  // 展开到至少 width（例如为收藏详情分栏腾出空间），右缘保持不动，左移超出视口时由约束回收。
  const widenTo = (width: number) => {
    const current = rect.value
    if (current.width >= width || gesture.value) return
    commitRect({ ...current, x: current.x - (width - current.width), width })
    persist()
  }

  const accepts = (event: PointerEvent) => !(event.pointerType === 'mouse' && event.button !== 0)

  const capture = (event: PointerEvent) => {
    const target = event.currentTarget as HTMLElement
    try {
      target.setPointerCapture(event.pointerId)
    } catch {
      // 指针已结束时 capture 会抛错，忽略即可。
    }
    return target
  }

  const releaseGesture = () => {
    const current = gesture.value
    if (!current) return
    gesture.value = null
    try {
      if (current.target.hasPointerCapture?.(current.pointerId)) current.target.releasePointerCapture(current.pointerId)
    } catch {
      // ignore
    }
  }

  const startMove = (event: PointerEvent) => {
    if (!accepts(event) || gesture.value) return
    const origin = event.target as HTMLElement | null
    if (origin?.closest('button, input, textarea, select, a, [role="tab"], [data-no-window-drag]')) return
    event.preventDefault()
    gesture.value = {
      kind: 'move',
      pointerId: event.pointerId,
      target: capture(event),
      startX: event.clientX,
      startY: event.clientY,
      originX: rect.value.x,
      originY: rect.value.y,
    }
  }

  const startResize = (direction: FavoriteResizeDirection, event: PointerEvent) => {
    if (!accepts(event) || gesture.value) return
    event.preventDefault()
    event.stopPropagation()
    gesture.value = {
      kind: 'resize',
      pointerId: event.pointerId,
      target: capture(event),
      startX: event.clientX,
      startY: event.clientY,
      origin: { ...rect.value },
      direction,
    }
  }

  const startBadge = (event: PointerEvent) => {
    if (!accepts(event) || gesture.value) return
    event.preventDefault()
    gesture.value = {
      kind: 'badge',
      pointerId: event.pointerId,
      target: capture(event),
      state: createFloatingBadgeGesture({
        pointerId: event.pointerId,
        startX: event.clientX,
        startY: event.clientY,
        windowX: badge.value.x,
        windowY: badge.value.y,
      }),
    }
  }

  const onPointerMove = (event: PointerEvent) => {
    const current = gesture.value
    if (!current || current.pointerId !== event.pointerId) return
    if (event.pointerType === 'mouse' && event.buttons === 0) {
      onPointerEnd(event)
      return
    }
    event.preventDefault()
    if (current.kind === 'badge') {
      const result = updateFloatingBadgeGesture(current.state, event.clientX, event.clientY)
      if (result.dragActivated) {
        layout.badgeX = result.position.x
        layout.badgeY = result.position.y
      }
      return
    }
    const dx = event.clientX - current.startX
    const dy = event.clientY - current.startY
    if (current.kind === 'move') {
      const next = clampRect({ ...rect.value, x: current.originX + dx, y: current.originY + dy })
      layout.x = next.x
      layout.y = next.y
      return
    }
    const { origin, direction } = current
    const min = minSize.value
    let { x, y, width, height } = origin
    if (direction.includes('e')) width = Math.min(origin.width + dx, viewport.width - PADDING - origin.x)
    if (direction.includes('s')) height = Math.min(origin.height + dy, viewport.height - PADDING - origin.y)
    if (direction.includes('w')) {
      width = Math.max(min.width, Math.min(origin.width - dx, origin.x + origin.width - PADDING))
      x = origin.x + origin.width - width
    }
    if (direction.includes('n')) {
      height = Math.max(min.height, Math.min(origin.height - dy, origin.y + origin.height - PADDING))
      y = origin.y + origin.height - height
    }
    commitRect({ x, y, width, height })
  }

  // 返回 true 表示这次是对最小化图标的轻点（应恢复窗口）。
  const onPointerEnd = (event: PointerEvent) => {
    const current = gesture.value
    if (!current || current.pointerId !== event.pointerId) return false
    releaseGesture()
    if (current.kind === 'badge') {
      const tap = finishFloatingBadgeGesture(current.state).action === 'toggle'
      if (!tap) {
        const next = clampBadge(layout.badgeX, layout.badgeY)
        layout.badgeX = next.x
        layout.badgeY = next.y
        persist()
      }
      return tap && event.type === 'pointerup'
    }
    commitRect(rect.value)
    persist()
    return false
  }

  useEventListener(typeof window === 'undefined' ? undefined : window, 'resize', () => {
    viewport.width = Math.max(1, window.innerWidth)
    viewport.height = Math.max(1, window.innerHeight)
  })
  // 失焦时中断手势（例如 Android 切后台），防止残留拖动状态。
  useEventListener(typeof window === 'undefined' ? undefined : window, 'blur', releaseGesture)

  watch(storageKey, load, { immediate: true })
  onBeforeUnmount(releaseGesture)

  return {
    layout,
    viewport,
    rect,
    badge,
    compact,
    gesture,
    setMode,
    widenTo,
    startMove,
    startResize,
    startBadge,
    onPointerMove,
    onPointerEnd,
  }
}
