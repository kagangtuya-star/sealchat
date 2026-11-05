<script setup lang="ts">
// “更多跑团功能”布局的所见即所得编辑板：按聊天中「更多」面板的两列形态展示入口，
// 支持指针拖拽（鼠标/触屏共用 Pointer Events，拖动柄禁用触摸滚动）、方向键微调与显隐切换。
// 只编辑 order/hidden 两个列表，不涉及入口的业务可用性。
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import { NIcon, useThemeVars } from 'naive-ui'
import { Eye, EyeOff, GripVertical } from '@vicons/tabler'
import { getActionRibbonCatalogItem, type ActionRibbonCatalogItem } from '../actionRibbonCatalog'

const props = withDefaults(defineProps<{
  /** 已补全的已知入口顺序（包含隐藏项）。 */
  order: string[]
  hidden: string[]
  readonly?: boolean
  /** 提供时，显隐与之不同的入口标记为“已覆写”。 */
  baseHidden?: string[] | null
}>(), { readonly: false, baseHidden: null })

const emit = defineEmits<{
  'update:order': [value: string[]]
  'update:hidden': [value: string[]]
}>()

const themeVars = useThemeVars()
const boardStyle = computed(() => ({ '--arl-accent': themeVars.value.primaryColor }))

const gridRef = ref<HTMLElement | null>(null)
const draggingKey = ref<string | null>(null)
const previewOrder = ref<string[] | null>(null)

const effectiveOrder = computed(() => previewOrder.value ?? props.order)
const hiddenSet = computed(() => new Set(props.hidden))
const baseHiddenSet = computed(() => (props.baseHidden ? new Set(props.baseHidden) : null))

const toItems = (keys: string[]) => keys
  .map(key => getActionRibbonCatalogItem(key))
  .filter((item): item is ActionRibbonCatalogItem => !!item)

const visibleItems = computed(() => toItems(effectiveOrder.value.filter(key => !hiddenSet.value.has(key))))
const hiddenItems = computed(() => toItems(effectiveOrder.value.filter(key => hiddenSet.value.has(key))))

const isOverridden = (key: string) => !!baseHiddenSet.value && baseHiddenSet.value.has(key) !== hiddenSet.value.has(key)

const moveKeyTo = (order: string[], key: string, targetKey: string) => {
  const from = order.indexOf(key)
  const to = order.indexOf(targetKey)
  if (from < 0 || to < 0 || from === to) return order
  const next = order.slice()
  next.splice(from, 1)
  next.splice(to, 0, key)
  return next
}

const setHidden = (key: string, hidden: boolean) => {
  if (props.readonly) return
  const next = props.hidden.filter(item => item !== key)
  if (hidden) next.push(key)
  emit('update:hidden', next)
}

// ---- 指针拖拽 ----
const START_DISTANCE = 5
const EDGE = 48
let pending: { key: string; x: number; y: number; pointerId: number } | null = null
let lastTargetKey = ''
let scrollParent: HTMLElement | null = null

const findScrollParent = (element: HTMLElement | null) => {
  let node = element?.parentElement || null
  while (node && node !== document.body) {
    const overflowY = getComputedStyle(node).overflowY
    if ((overflowY === 'auto' || overflowY === 'scroll') && node.scrollHeight > node.clientHeight) return node
    node = node.parentElement
  }
  return null
}

const detachWindowListeners = () => {
  window.removeEventListener('pointermove', handlePointerMove)
  window.removeEventListener('pointerup', handlePointerUp)
  window.removeEventListener('pointercancel', resetDrag)
}

function resetDrag() {
  detachWindowListeners()
  pending = null
  lastTargetKey = ''
  scrollParent = null
  draggingKey.value = null
  previewOrder.value = null
}

// 移动/抬起监听挂在 window 上，指针离开拖动柄或捕获失效时拖拽也能正常结束。
const handlePointerDown = (event: PointerEvent, key: string) => {
  if (props.readonly || (event.pointerType === 'mouse' && event.button !== 0)) return
  resetDrag()
  pending = { key, x: event.clientX, y: event.clientY, pointerId: event.pointerId }
  window.addEventListener('pointermove', handlePointerMove, { passive: false })
  window.addEventListener('pointerup', handlePointerUp)
  window.addEventListener('pointercancel', resetDrag)
}

const hitTest = (clientX: number, clientY: number) => {
  const dragged = draggingKey.value
  if (!gridRef.value || !dragged || !previewOrder.value) return
  const tiles = gridRef.value.querySelectorAll<HTMLElement>('[data-arl-key]')
  for (const tile of Array.from(tiles)) {
    const key = tile.dataset.arlKey || ''
    if (key === dragged) continue
    const rect = tile.getBoundingClientRect()
    if (clientX < rect.left || clientX > rect.right || clientY < rect.top || clientY > rect.bottom) continue
    // 位置交换后目标块会滑走，只有指针进入另一个块时才再次移动，避免来回抖动。
    if (key !== lastTargetKey) {
      lastTargetKey = key
      previewOrder.value = moveKeyTo(previewOrder.value, dragged, key)
    }
    return
  }
  lastTargetKey = ''
}

const autoScroll = (clientY: number) => {
  if (!scrollParent) return
  const rect = scrollParent.getBoundingClientRect()
  if (clientY < rect.top + EDGE) scrollParent.scrollTop -= 10
  else if (clientY > rect.bottom - EDGE) scrollParent.scrollTop += 10
}

function handlePointerMove(event: PointerEvent) {
  if (!pending || event.pointerId !== pending.pointerId) return
  if (!draggingKey.value) {
    if (Math.hypot(event.clientX - pending.x, event.clientY - pending.y) < START_DISTANCE) return
    draggingKey.value = pending.key
    previewOrder.value = props.order.slice()
    scrollParent = findScrollParent(gridRef.value)
  }
  event.preventDefault()
  hitTest(event.clientX, event.clientY)
  autoScroll(event.clientY)
}

function handlePointerUp(event: PointerEvent) {
  if (!pending || event.pointerId !== pending.pointerId) return
  const next = previewOrder.value
  const changed = !!next && next.some((key, index) => key !== props.order[index])
  resetDrag()
  if (next && changed) emit('update:order', next)
}

onBeforeUnmount(resetDrag)

// ---- 键盘微调：在可见入口之间移动 ----
const focusHandle = (key: string) => {
  nextTick(() => {
    gridRef.value?.querySelector<HTMLElement>(`[data-arl-key="${key}"] .arl-tile__handle`)?.focus()
  })
}

const handleKeydown = (event: KeyboardEvent, key: string) => {
  if (props.readonly) return
  const visible = visibleItems.value.map(item => item.key)
  const index = visible.indexOf(key)
  let target: string | undefined
  if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') target = visible[index - 1]
  else if (event.key === 'ArrowRight' || event.key === 'ArrowDown') target = visible[index + 1]
  else if (event.key === 'Home') target = visible[0]
  else if (event.key === 'End') target = visible[visible.length - 1]
  else return
  event.preventDefault()
  if (!target || target === key) return
  emit('update:order', moveKeyTo(props.order, key, target))
  focusHandle(key)
}
</script>

<template>
  <div class="arl-board" :class="{ 'is-readonly': readonly, 'is-dragging': !!draggingKey }" :style="boardStyle">
    <div class="arl-board__meta">
      <span>显示 {{ visibleItems.length }} 项<template v-if="hiddenItems.length"> · 隐藏 {{ hiddenItems.length }} 项</template></span>
      <span v-if="!readonly" class="arl-board__tip">拖动 <n-icon :component="GripVertical" /> 调整顺序，点 <n-icon :component="Eye" /> 隐藏</span>
    </div>

    <div v-if="visibleItems.length" ref="gridRef" class="arl-board__grid">
      <TransitionGroup name="arl-tile">
        <div
          v-for="(item, index) in visibleItems"
          :key="item.key"
          class="arl-tile"
          :class="{ 'is-dragging': draggingKey === item.key, 'is-overridden': isOverridden(item.key) }"
          :data-arl-key="item.key"
        >
          <button
            v-if="!readonly"
            type="button"
            class="arl-tile__handle"
            :aria-label="`调整「${item.label}」的位置，可用方向键移动`"
            @pointerdown="handlePointerDown($event, item.key)"
            @keydown="handleKeydown($event, item.key)"
          >
            <n-icon :component="GripVertical" />
          </button>
          <span class="arl-tile__index">{{ index + 1 }}</span>
          <n-icon class="arl-tile__icon" :component="item.icon" />
          <span class="arl-tile__text">
            <span class="arl-tile__label">{{ item.label }}</span>
            <span v-if="item.condition" class="arl-tile__condition">{{ item.condition }}</span>
          </span>
          <span v-if="isOverridden(item.key)" class="arl-tile__mark" title="世界已覆写此入口的显隐">已覆写</span>
          <button
            v-if="!readonly"
            type="button"
            class="arl-tile__toggle"
            :aria-label="`隐藏「${item.label}」`"
            title="隐藏"
            @click="setHidden(item.key, true)"
          >
            <n-icon :component="Eye" />
          </button>
        </div>
      </TransitionGroup>
    </div>
    <div v-else class="arl-board__empty">所有入口都已隐藏，聊天中将不显示任何跑团功能入口。</div>

    <div v-if="hiddenItems.length" class="arl-board__hidden">
      <div class="arl-board__hidden-title">已隐藏{{ readonly ? '' : '（点击恢复显示）' }}</div>
      <div class="arl-board__hidden-list">
        <button
          v-for="item in hiddenItems"
          :key="item.key"
          type="button"
          class="arl-chip"
          :class="{ 'is-overridden': isOverridden(item.key) }"
          :disabled="readonly"
          :title="readonly ? undefined : `恢复显示「${item.label}」`"
          @click="setHidden(item.key, false)"
        >
          <n-icon :component="EyeOff" />
          <span>{{ item.label }}</span>
          <span v-if="isOverridden(item.key)" class="arl-chip__mark">已覆写</span>
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.arl-board {
  --arl-accent-soft: color-mix(in srgb, var(--arl-accent) 14%, transparent);
  display: flex;
  flex-direction: column;
  gap: 10px;
  min-width: 0;
  container-type: inline-size;
  color: var(--sc-text-primary);
}

.arl-board__meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 4px 12px;
  color: var(--sc-text-secondary);
  font-size: 12px;
}

.arl-board__tip {
  display: inline-flex;
  align-items: center;
  gap: 3px;
}

.arl-board__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  padding: 8px;
  border: 1px solid var(--sc-border-strong);
  border-radius: 12px;
  background: var(--sc-bg-elevated);
}

.arl-tile {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  min-height: 44px;
  padding: 4px 6px;
  border: 1px solid var(--sc-border-strong);
  border-radius: 8px;
  background: var(--sc-bg-elevated);
  color: var(--sc-text-primary);
  user-select: none;
}

.arl-board:not(.is-dragging) .arl-tile:hover {
  background: var(--sc-chip-bg);
}

.arl-tile.is-dragging {
  position: relative;
  z-index: 1;
  border-color: var(--arl-accent);
  background: var(--arl-accent-soft);
  box-shadow: 0 6px 18px color-mix(in srgb, var(--sc-text-primary) 16%, transparent);
}

.arl-tile.is-overridden {
  border-style: dashed;
  border-color: var(--arl-accent);
}

.arl-tile-move {
  transition: transform 0.16s ease;
}

.arl-tile__handle,
.arl-tile__toggle {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 32px;
  padding: 0;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--sc-text-secondary);
  font-size: 16px;
  cursor: var(--sc-cursor-pointer, pointer);
}

.arl-tile__handle {
  cursor: var(--sc-cursor-grab, grab);
  touch-action: none;
}

.arl-board.is-dragging,
.arl-board.is-dragging .arl-tile__handle {
  cursor: var(--sc-cursor-grabbing, grabbing);
}

.arl-tile__handle:hover,
.arl-tile__toggle:hover,
.arl-tile__handle:focus-visible,
.arl-tile__toggle:focus-visible {
  background: var(--sc-chip-bg);
  color: var(--sc-text-primary);
}

.arl-tile__handle:focus-visible,
.arl-tile__toggle:focus-visible,
.arl-chip:focus-visible {
  outline: 2px solid var(--arl-accent);
  outline-offset: 1px;
}

.arl-tile__index {
  flex: none;
  min-width: 18px;
  color: var(--sc-text-secondary);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
  text-align: center;
}

.arl-board.is-readonly .arl-tile {
  padding-left: 10px;
}

.arl-tile__icon {
  flex: none;
  font-size: 16px;
}

.arl-tile__text {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-width: 0;
  line-height: 1.3;
}

.arl-tile__label,
.arl-tile__condition {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.arl-tile__label {
  font-size: 13px;
}

.arl-tile__condition {
  color: var(--sc-text-secondary);
  font-size: 11px;
}

.arl-tile__mark,
.arl-chip__mark {
  flex: none;
  padding: 0 5px;
  border-radius: 999px;
  background: var(--arl-accent-soft);
  color: var(--arl-accent);
  font-size: 11px;
  line-height: 18px;
}

.arl-board__empty {
  padding: 18px 12px;
  border: 1px dashed var(--sc-border-strong);
  border-radius: 12px;
  color: var(--sc-text-secondary);
  font-size: 13px;
  text-align: center;
}

.arl-board__hidden {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.arl-board__hidden-title {
  color: var(--sc-text-secondary);
  font-size: 12px;
}

.arl-board__hidden-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.arl-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  max-width: 100%;
  padding: 3px 10px;
  border: 1px dashed var(--sc-border-strong);
  border-radius: 999px;
  background: transparent;
  color: var(--sc-text-secondary);
  font-size: 12px;
  cursor: var(--sc-cursor-pointer, pointer);
}

.arl-chip:hover:not(:disabled) {
  background: var(--sc-chip-bg);
  color: var(--sc-text-primary);
}

.arl-chip:disabled {
  cursor: default;
}

.arl-chip.is-overridden {
  border-color: var(--arl-accent);
}

@container (max-width: 340px) {
  .arl-board__grid {
    grid-template-columns: minmax(0, 1fr);
  }
}

@media (prefers-reduced-motion: reduce) {
  .arl-tile-move {
    transition: none;
  }
}
</style>
