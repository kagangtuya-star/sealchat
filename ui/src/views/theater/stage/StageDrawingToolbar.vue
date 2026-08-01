<script setup lang="ts">
import { computed, ref, type Component } from 'vue'
import { NButton, NCheckbox, NColorPicker, NIcon, NInputNumber, NPopover, NSelect, NSlider } from 'naive-ui'
import {
  ArrowNarrowRight,
  Brush,
  ChevronDown,
  Circle,
  Eraser,
  Highlight,
  Line,
  Pencil,
  Polygon,
  Square,
  Triangle,
} from '@vicons/tabler'
import type { StageDrawingDash, StageDrawingStyle, StageDrawingTool } from '../shared/stage-types'

export type StageCanvasTool = StageDrawingTool | 'eraser'

const props = defineProps<{
  tool: StageCanvasTool | null
  style: StageDrawingStyle
  smoothing: number
  sides: number
  disabled?: boolean
}>()

const emit = defineEmits<{
  select: [tool: StageCanvasTool]
  'update:style': [style: StageDrawingStyle]
  'update:smoothing': [value: number]
  'update:sides': [value: number]
}>()

interface ToolOption {
  value: StageCanvasTool
  label: string
  icon: Component
}

const brushTools: ToolOption[] = [
  { value: 'pen', label: '画笔', icon: Pencil },
  { value: 'highlighter', label: '荧光笔', icon: Highlight },
  { value: 'eraser', label: '橡皮擦', icon: Eraser },
]

const shapeTools: ToolOption[] = [
  { value: 'line', label: '直线', icon: Line },
  { value: 'arrow', label: '箭头', icon: ArrowNarrowRight },
  { value: 'rectangle', label: '矩形', icon: Square },
  { value: 'ellipse', label: '椭圆', icon: Circle },
  { value: 'triangle', label: '三角形', icon: Triangle },
  { value: 'polygon', label: '多边形', icon: Polygon },
]

const activeOption = computed(() => [...brushTools, ...shapeTools].find((option) => option.value === props.tool))
const activeIcon = computed(() => activeOption.value?.icon || Brush)
const isFreehand = computed(() => props.tool === 'pen' || props.tool === 'highlighter')
const supportsFill = computed(() => ['rectangle', 'ellipse', 'triangle', 'polygon'].includes(props.tool || ''))
const dashOptions = [
  { label: '实线', value: 'solid' },
  { label: '虚线', value: 'dashed' },
  { label: '点线', value: 'dotted' },
]
const colorPresets = ['#f8fafc', '#ef4444', '#f59e0b', '#22c55e', '#38bdf8', '#8b5cf6', '#ec4899', '#111827']
const paletteOpen = ref(false)
const theaterRawPopoverThemeOverrides = {
  color: 'transparent',
  padding: '0',
  boxShadow: 'none',
}
const theaterSecondaryMenuProps = { class: 'theater-secondary-surface' }

const updateStyle = (patch: Partial<StageDrawingStyle>) => emit('update:style', { ...props.style, ...patch })
const toggleFill = (checked: boolean) => updateStyle({ fill: checked ? props.style.fill || props.style.stroke : null })
const handlePrimaryClick = () => {
  if (props.tool) {
    emit('select', props.tool)
    paletteOpen.value = false
    return
  }
  paletteOpen.value = true
}
</script>

<template>
  <div class="theater-drawing-trigger-group">
    <n-button
      class="theater-drawing-trigger theater-drawing-trigger--primary"
      :class="{ 'is-active': tool }"
      :disabled="disabled"
      size="small"
      :aria-label="tool ? '退出画笔与形状工具' : '打开画笔与形状工具'"
      @click="handlePrimaryClick"
    >
      <n-icon><component :is="activeIcon" /></n-icon>
    </n-button>

    <n-popover
      trigger="click"
      placement="bottom-start"
      :show="paletteOpen"
      :show-arrow="false"
      :disabled="disabled"
      raw
      :theme-overrides="theaterRawPopoverThemeOverrides"
      @update:show="paletteOpen = $event"
    >
      <template #trigger>
      <n-button
        class="theater-drawing-trigger theater-drawing-trigger--menu"
        :class="{ 'is-active': tool }"
        :disabled="disabled"
        size="small"
        aria-label="选择画笔与形状工具"
      >
        <n-icon class="theater-drawing-trigger__chevron"><ChevronDown /></n-icon>
      </n-button>
      </template>

      <div class="theater-drawing-palette theater-secondary-surface">
      <section>
        <div class="theater-drawing-palette__heading">画笔</div>
        <div class="theater-drawing-tool-grid is-brush-grid">
          <button
            v-for="option in brushTools"
            :key="option.value"
            type="button"
            :class="{ 'is-active': tool === option.value }"
            @click="emit('select', option.value)"
          >
            <n-icon><component :is="option.icon" /></n-icon>
            <span>{{ option.label }}</span>
          </button>
        </div>
      </section>

      <section>
        <div class="theater-drawing-palette__heading">形状</div>
        <div class="theater-drawing-tool-grid">
          <button
            v-for="option in shapeTools"
            :key="option.value"
            type="button"
            :class="{ 'is-active': tool === option.value }"
            @click="emit('select', option.value)"
          >
            <n-icon><component :is="option.icon" /></n-icon>
            <span>{{ option.label }}</span>
          </button>
        </div>
      </section>

      <section v-if="tool && tool !== 'eraser'" class="theater-drawing-settings">
        <div class="theater-drawing-palette__heading">样式</div>
        <label class="theater-drawing-setting-row">
          <span>描边</span>
          <n-color-picker
            :value="style.stroke"
            :show-alpha="false"
            :modes="['hex']"
            size="small"
            @update:value="updateStyle({ stroke: $event })"
          />
        </label>
        <div class="theater-drawing-swatches" aria-label="描边预设颜色">
          <button
            v-for="color in colorPresets"
            :key="color"
            type="button"
            :class="{ 'is-active': style.stroke.toLowerCase() === color.toLowerCase() }"
            :style="{ backgroundColor: color }"
            :aria-label="`使用颜色 ${color}`"
            @click="updateStyle({ stroke: color })"
          />
        </div>
        <label class="theater-drawing-setting-row">
          <span>粗细</span>
          <n-slider
            :value="style.strokeWidth"
            :min="1"
            :max="32"
            :step="1"
            @update:value="updateStyle({ strokeWidth: $event })"
          />
          <span class="theater-drawing-setting-value">{{ style.strokeWidth }} px</span>
        </label>
        <label class="theater-drawing-setting-row">
          <span>透明</span>
          <n-slider
            :value="Math.round(style.opacity * 100)"
            :min="5"
            :max="100"
            :step="5"
            @update:value="updateStyle({ opacity: $event / 100 })"
          />
          <span class="theater-drawing-setting-value">{{ Math.round(style.opacity * 100) }}%</span>
        </label>
        <label v-if="isFreehand" class="theater-drawing-setting-row">
          <span>平滑</span>
          <n-slider
            :value="Math.round(smoothing * 100)"
            :min="0"
            :max="80"
            :step="5"
            @update:value="emit('update:smoothing', $event / 100)"
          />
          <span class="theater-drawing-setting-value">{{ Math.round(smoothing * 100) }}%</span>
        </label>
        <label v-else class="theater-drawing-setting-row">
          <span>线型</span>
          <n-select
            :value="style.dash"
            :options="dashOptions"
            size="small"
            :menu-props="theaterSecondaryMenuProps"
            @update:value="updateStyle({ dash: $event as StageDrawingDash })"
          />
        </label>
        <template v-if="supportsFill">
          <label class="theater-drawing-fill-toggle">
            <n-checkbox :checked="style.fill !== null" @update:checked="toggleFill">填充</n-checkbox>
          </label>
          <label v-if="style.fill !== null" class="theater-drawing-setting-row">
            <span>填充色</span>
            <n-color-picker
              :value="style.fill"
              :show-alpha="false"
              :modes="['hex']"
              size="small"
              @update:value="updateStyle({ fill: $event })"
            />
          </label>
        </template>
        <label v-if="tool === 'polygon'" class="theater-drawing-setting-row">
          <span>边数</span>
          <n-input-number
            :value="sides"
            :min="5"
            :max="12"
            size="small"
            @update:value="$event !== null && emit('update:sides', $event)"
          />
        </label>
      </section>
      </div>
    </n-popover>
  </div>
</template>

<style scoped>
.theater-drawing-trigger-group { display: inline-flex; }
.theater-drawing-trigger {
  padding: 0;
  border-radius: 0;
}
.theater-drawing-trigger--primary {
  width: 30px;
  border-radius: 3px 0 0 3px;
}
.theater-drawing-trigger--menu {
  width: 18px;
  margin-left: -1px;
  border-radius: 0 3px 3px 0;
}
.theater-drawing-trigger.is-active {
  color: #fff;
  background: var(--theater-accent, #3b82f6);
  border-color: var(--theater-accent, #3b82f6);
}
.theater-drawing-trigger__chevron { font-size: 11px; opacity: .72; }
.theater-drawing-palette {
  width: min(304px, calc(100vw - 20px));
  box-sizing: border-box;
  display: grid;
  gap: 13px;
  padding: 12px;
  border-radius: 7px;
}
.theater-drawing-palette section { display: grid; gap: 7px; }
.theater-drawing-palette__heading {
  color: var(--sc-text-secondary, #b5b5c5);
  font-size: 11px;
  font-weight: 700;
}
.theater-drawing-tool-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 5px; }
.theater-drawing-tool-grid button {
  height: 48px;
  min-width: 0;
  display: grid;
  place-items: center;
  align-content: center;
  gap: 3px;
  border: 1px solid transparent;
  border-radius: 6px;
  color: var(--sc-text-secondary, #b5b5c5);
  background: var(--sc-bg-layer, #34343a);
  font: inherit;
  cursor: pointer;
}
.theater-drawing-tool-grid button:hover { color: var(--sc-text-primary, #f4f4f5); background: var(--sc-sidebar-hover, rgba(255, 255, 255, .09)); }
.theater-drawing-tool-grid button.is-active {
  border-color: color-mix(in srgb, var(--theater-accent, #3b82f6) 72%, transparent);
  color: #fff;
  background: color-mix(in srgb, var(--theater-accent, #3b82f6) 22%, var(--sc-bg-layer, #34343a));
}
.theater-drawing-tool-grid .n-icon { font-size: 19px; }
.theater-drawing-tool-grid span { min-width: 0; font-size: 10px; white-space: nowrap; }
.theater-drawing-settings { padding-top: 10px; border-top: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08)); }
.theater-drawing-setting-row {
  min-height: 28px;
  display: grid;
  grid-template-columns: 42px minmax(0, 1fr) auto;
  align-items: center;
  gap: 8px;
  color: var(--sc-text-secondary, #b5b5c5);
  font-size: 11px;
}
.theater-drawing-setting-row :deep(.n-color-picker), .theater-drawing-setting-row :deep(.n-select) { grid-column: 2 / 4; }
.theater-drawing-setting-value { width: 39px; color: var(--sc-fg-muted, #71717a); font-size: 10px; text-align: right; }
.theater-drawing-swatches { display: grid; grid-template-columns: repeat(8, 1fr); gap: 6px; }
.theater-drawing-swatches button {
  aspect-ratio: 1;
  min-width: 0;
  border: 2px solid transparent;
  border-radius: 50%;
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, .24);
  cursor: pointer;
}
.theater-drawing-swatches button.is-active { border-color: #fff; }
.theater-drawing-fill-toggle { color: var(--sc-text-secondary, #b5b5c5); font-size: 11px; }
</style>
