<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NButton, NIcon, NInput, NRadio, NRadioGroup, NTag } from 'naive-ui'
import { Search, Stack2, X } from '@vicons/tabler'

import { getSceneOverlayEffect } from './scene-overlay-registry'
import {
  listSceneOverlayPresets,
  registerBuiltInSceneOverlayPresets,
  sceneOverlayPresetCategories,
  sceneOverlayPresetCategoryLabels,
  type SceneOverlayPresetCategory,
  type SceneOverlayPresetApplyMode,
} from './presets'
import type { TheaterSceneOverlayPreset } from './scene-overlay-preset-api'

const props = withDefaults(defineProps<{
  currentOverlayCount: number
  maximumOverlayCount?: number
  customPresets?: TheaterSceneOverlayPreset[]
}>(), {
  maximumOverlayCount: 32,
  customPresets: () => [],
})

const emit = defineEmits<{
  apply: [presetId: string, mode: SceneOverlayPresetApplyMode]
  edit: [preset: TheaterSceneOverlayPreset]
  delete: [preset: TheaterSceneOverlayPreset]
  close: []
}>()

registerBuiltInSceneOverlayPresets()

const selectedCategory = ref<'custom' | 'all' | SceneOverlayPresetCategory>('custom')
const keyword = ref('')
const applyMode = ref<SceneOverlayPresetApplyMode>('append')
const visibleCount = ref(24)
const categoryScroller = ref<HTMLElement | null>(null)
const categoryDrag = ref<{ pointerId: number, startX: number, startScrollLeft: number, moved: boolean } | null>(null)
let suppressCategoryClick = false
const presets = listSceneOverlayPresets()
const allPresets = computed(() => [
  ...props.customPresets.map((preset) => ({ ...preset, category: 'custom' as const })),
  ...presets,
])
const normalizedKeyword = computed(() => keyword.value.trim().toLocaleLowerCase())
const filteredPresets = computed(() => allPresets.value.filter((preset) => {
  if (selectedCategory.value !== 'all' && preset.category !== selectedCategory.value) return false
  if (!normalizedKeyword.value) return true
  return [preset.name, preset.description, ...(preset.tags || [])]
    .some((value) => value.toLocaleLowerCase().includes(normalizedKeyword.value))
}))
const visiblePresets = computed(() => filteredPresets.value.slice(0, visibleCount.value))

watch([selectedCategory, keyword], () => {
  visibleCount.value = 24
})

const beginCategoryDrag = (event: PointerEvent) => {
  const scroller = categoryScroller.value
  if (!scroller || event.button !== 0) return
  categoryDrag.value = {
    pointerId: event.pointerId,
    startX: event.clientX,
    startScrollLeft: scroller.scrollLeft,
    moved: false,
  }
  scroller.setPointerCapture(event.pointerId)
}

const moveCategoryDrag = (event: PointerEvent) => {
  const drag = categoryDrag.value
  const scroller = categoryScroller.value
  if (!drag || !scroller || drag.pointerId !== event.pointerId) return
  const delta = event.clientX - drag.startX
  if (!drag.moved && Math.abs(delta) < 4) return
  drag.moved = true
  scroller.scrollLeft = drag.startScrollLeft - delta
  event.preventDefault()
}

const endCategoryDrag = (event: PointerEvent) => {
  const drag = categoryDrag.value
  const scroller = categoryScroller.value
  if (!drag || !scroller || drag.pointerId !== event.pointerId) return
  suppressCategoryClick = drag.moved
  if (drag.moved) window.setTimeout(() => { suppressCategoryClick = false }, 0)
  categoryDrag.value = null
  if (scroller.hasPointerCapture(event.pointerId)) scroller.releasePointerCapture(event.pointerId)
}

const cancelCategoryDrag = (event: PointerEvent) => {
  const drag = categoryDrag.value
  const scroller = categoryScroller.value
  if (!drag || !scroller || drag.pointerId !== event.pointerId) return
  suppressCategoryClick = false
  categoryDrag.value = null
  if (scroller.hasPointerCapture(event.pointerId)) scroller.releasePointerCapture(event.pointerId)
}

const selectCategory = (category: 'custom' | 'all' | SceneOverlayPresetCategory) => {
  if (suppressCategoryClick) {
    suppressCategoryClick = false
    return
  }
  selectedCategory.value = category
}

const effectNames = (effectIds: string[]) => effectIds
  .slice(0, 3)
  .map((effectId) => getSceneOverlayEffect(effectId)?.name || effectId)
  .join(' · ')

const capacityAllows = (overlayCount: number) => (
  applyMode.value === 'replace'
    ? overlayCount <= props.maximumOverlayCount
    : props.currentOverlayCount + overlayCount <= props.maximumOverlayCount
)
</script>

<template>
  <section class="scene-overlay-preset-catalog">
    <header class="scene-overlay-preset-catalog__header">
      <strong>场景预设</strong>
      <n-input v-model:value="keyword" size="small" clearable placeholder="搜索名称、描述或标签">
        <template #prefix><n-icon><Search /></n-icon></template>
      </n-input>
      <n-button text aria-label="关闭场景预设" title="关闭" @click="emit('close')"><n-icon><X /></n-icon></n-button>
    </header>

    <nav
      ref="categoryScroller"
      class="scene-overlay-preset-catalog__categories"
      :class="{ 'is-dragging': categoryDrag }"
      aria-label="场景预设分类"
      @pointerdown="beginCategoryDrag"
      @pointermove="moveCategoryDrag"
      @pointerup="endCategoryDrag"
      @pointercancel="cancelCategoryDrag"
      @lostpointercapture="cancelCategoryDrag"
    >
      <n-button size="tiny" :type="selectedCategory === 'custom' ? 'primary' : 'default'" :secondary="selectedCategory === 'custom'" @click="selectCategory('custom')">自制</n-button>
      <n-button size="tiny" :type="selectedCategory === 'all' ? 'primary' : 'default'" :secondary="selectedCategory === 'all'" @click="selectCategory('all')">全部</n-button>
      <n-button
        v-for="category in sceneOverlayPresetCategories"
        :key="category"
        size="tiny"
        :type="selectedCategory === category ? 'primary' : 'default'"
        :secondary="selectedCategory === category"
        @click="selectCategory(category)"
      >{{ sceneOverlayPresetCategoryLabels[category] }}</n-button>
    </nav>

    <div class="scene-overlay-preset-catalog__mode">
      <span>应用方式</span>
      <n-radio-group v-model:value="applyMode" size="small">
        <n-radio value="append">追加到当前效果</n-radio>
        <n-radio value="replace">清空后应用</n-radio>
      </n-radio-group>
      <small v-if="applyMode === 'append' && currentOverlayCount">将追加所选预设效果</small>
    </div>

    <div class="scene-overlay-preset-catalog__list">
      <article v-for="preset in visiblePresets" :key="preset.id" class="scene-overlay-preset-catalog__item">
        <div class="scene-overlay-preset-catalog__copy">
          <div class="scene-overlay-preset-catalog__title">
            <n-icon><Stack2 /></n-icon>
            <strong>{{ preset.name }}</strong>
            <small>{{ preset.category === 'custom' ? '自制' : sceneOverlayPresetCategoryLabels[preset.category] }}</small>
          </div>
          <p>{{ preset.description }}</p>
          <span>{{ effectNames(preset.overlays.map(item => item.effectId)) }}<template v-if="preset.overlays.length > 3"> 等</template></span>
          <div v-if="preset.tags?.length" class="scene-overlay-preset-catalog__tags">
            <n-tag v-for="tag in preset.tags" :key="tag" size="tiny" :bordered="false">{{ tag }}</n-tag>
          </div>
        </div>
        <div class="scene-overlay-preset-catalog__apply">
          <small>{{ preset.overlays.length }} 个效果</small>
          <template v-if="preset.category === 'custom'">
            <n-button size="tiny" secondary @click="emit('edit', preset)">编辑</n-button>
            <n-button size="tiny" tertiary type="error" @click="emit('delete', preset)">删除</n-button>
          </template>
          <n-button
            size="small"
            type="primary"
            secondary
            :disabled="!capacityAllows(preset.overlays.length)"
            @click="emit('apply', preset.id, applyMode)"
          >{{ applyMode === 'append' ? `追加 ${preset.overlays.length} 个效果` : '应用' }}</n-button>
        </div>
      </article>
      <div v-if="!visiblePresets.length" class="scene-overlay-preset-catalog__empty">没有匹配的场景预设</div>
      <n-button v-if="visibleCount < filteredPresets.length" class="scene-overlay-preset-catalog__more" size="small" text @click="visibleCount += 24">显示更多</n-button>
    </div>
  </section>
</template>

<style scoped>
.scene-overlay-preset-catalog { min-height: 0; max-height: min(520px, 70vh); display: flex; flex-direction: column; border-bottom: 1px solid var(--theater-border); background: color-mix(in srgb, var(--theater-panel) 92%, transparent); }
.scene-overlay-preset-catalog__header { flex: 0 0 auto; display: grid; grid-template-columns: auto minmax(180px, 360px) auto; align-items: center; gap: 9px; padding: 8px 9px; }
.scene-overlay-preset-catalog__header strong { font-size: 12px; }
.scene-overlay-preset-catalog__categories { flex: 0 0 auto; min-height: 29px; display: flex; gap: 4px; overflow-x: auto; overflow-y: hidden; padding: 0 9px 7px; scrollbar-width: none; cursor: grab; touch-action: pan-y; user-select: none; }
.scene-overlay-preset-catalog__categories.is-dragging { cursor: grabbing; }
.scene-overlay-preset-catalog__categories::-webkit-scrollbar { display: none; }
.scene-overlay-preset-catalog__categories :deep(.n-button) { flex: 0 0 auto; }
.scene-overlay-preset-catalog__mode { flex: 0 0 auto; display: flex; flex-wrap: wrap; align-items: center; gap: 8px 14px; padding: 7px 9px; border-block: 1px solid color-mix(in srgb, var(--theater-border) 70%, transparent); font-size: 10px; }
.scene-overlay-preset-catalog__mode > span { color: var(--sc-text-secondary); }
.scene-overlay-preset-catalog__mode > small { margin-left: auto; color: var(--sc-text-secondary); }
.scene-overlay-preset-catalog__list { min-height: 0; flex: 1 1 auto; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 6px; overflow: auto; padding: 8px; }
.scene-overlay-preset-catalog__item { min-width: 0; display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 8px; padding: 8px; border: 1px solid var(--theater-border); border-radius: 6px; background: color-mix(in srgb, var(--theater-panel-muted) 78%, transparent); }
.scene-overlay-preset-catalog__copy { min-width: 0; display: grid; align-content: start; gap: 3px; }
.scene-overlay-preset-catalog__title { min-width: 0; display: flex; align-items: center; gap: 5px; }
.scene-overlay-preset-catalog__title strong { overflow: hidden; font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.scene-overlay-preset-catalog__title small { margin-left: auto; color: var(--sc-text-secondary); font-size: 9px; }
.scene-overlay-preset-catalog__copy p { margin: 0; color: var(--sc-text-secondary); font-size: 9px; line-height: 1.45; }
.scene-overlay-preset-catalog__copy > span { overflow: hidden; font-size: 9px; text-overflow: ellipsis; white-space: nowrap; }
.scene-overlay-preset-catalog__tags { display: flex; flex-wrap: wrap; gap: 3px; margin-top: 2px; }
.scene-overlay-preset-catalog__tags :deep(.n-tag) { height: 18px; font-size: 8px; }
.scene-overlay-preset-catalog__apply { display: grid; align-content: end; justify-items: end; gap: 5px; }
.scene-overlay-preset-catalog__apply small { color: var(--sc-text-secondary); font-size: 9px; }
.scene-overlay-preset-catalog__empty, .scene-overlay-preset-catalog__more { grid-column: 1 / -1; }
.scene-overlay-preset-catalog__empty { padding: 24px; color: var(--sc-text-secondary); font-size: 10px; text-align: center; }
@media (max-width: 720px) {
  .scene-overlay-preset-catalog__list { grid-template-columns: 1fr; }
  .scene-overlay-preset-catalog__header { grid-template-columns: auto minmax(0, 1fr) auto; }
}
</style>
