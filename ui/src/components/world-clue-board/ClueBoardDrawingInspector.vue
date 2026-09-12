<script setup lang="ts">
import { computed } from 'vue'
import { NIcon, NTooltip } from 'naive-ui'
import {
  AlignCenter, AlignLeft, AlignRight, ArrowBackUp, ArrowBarToDown, ArrowBarToUp,
  ArrowForwardUp, ChevronDown, ChevronUp, Circle, Clipboard, Copy, Diamond, Focus2,
  Cut, GridDots, GripHorizontal, Hexagon, Minus, Send, Square, Star, Trash, TrashX,
  Triangle, X,
} from '@vicons/tabler'
import {
  COLOR_IDS, DASH_IDS, FILL_IDS, GEO_IDS, SIZE_IDS, THEMES,
  type ColorId, type DashId, type FillId, type FontId, type GeoId, type GridId,
  type ShapeType, type SizeId, type ThemeId, type ToolId,
} from '@quickdrawjs/core'

const props = defineProps<{
  theme: ThemeId
  activeTool: ToolId
  color: ColorId
  size: SizeId
  dash: DashId
  fill: FillId
  font: FontId
  geoKind: GeoId
  grid: GridId
  penMode: boolean
  hasSelection: boolean
  selectedKinds: ShapeType[]
  selectedAlign: 'start' | 'middle' | 'end'
  selectedBend: number
  canUndo: boolean
  canRedo: boolean
  collapsed: boolean
  maxHeight: number
  disabled?: boolean
}>()

const emit = defineEmits<{
  'drag-start': [event: PointerEvent]
  'toggle-collapse': []
  close: []
  color: [value: ColorId]
  size: [value: SizeId]
  dash: [value: DashId]
  fill: [value: FillId]
  font: [value: FontId]
  geo: [value: GeoId]
  grid: [value: GridId]
  'pen-mode': [value: boolean]
  align: [value: 'start' | 'middle' | 'end']
  bend: [value: number]
  copy: []
  cut: []
  delete: []
  front: []
  back: []
  undo: []
  redo: []
  paste: []
  fit: []
  clear: []
}>()

const COLOR_LABELS: Record<ColorId, string> = {
  black: '黑色', grey: '灰色', 'light-violet': '浅紫', violet: '紫色', blue: '蓝色', 'light-blue': '浅蓝',
  yellow: '黄色', orange: '橙色', green: '绿色', 'light-green': '浅绿', 'light-red': '浅红', red: '红色',
}
const SIZE_LABELS: Record<SizeId, string> = { s: '细', m: '中', l: '粗', xl: '特粗' }
const DASH_LABELS: Record<DashId, string> = { draw: '手绘', solid: '实线', dashed: '虚线', dotted: '点线' }
const FILL_LABELS: Record<FillId, string> = { none: '无填充', semi: '半透明', solid: '实心', pattern: '图案' }
const FONT_LABELS: Record<FontId, string> = { draw: '手写', sans: '无衬线', serif: '衬线', mono: '等宽' }
const GEO_LABELS: Record<GeoId, string> = { rectangle: '矩形', ellipse: '椭圆', triangle: '三角形', diamond: '菱形', hexagon: '六边形', star: '星形' }
const GRID_LABELS: Record<GridId, string> = { none: '无网格', lines: '线条网格', dots: '点阵网格' }
const FONT_IDS: FontId[] = ['draw', 'sans', 'serif', 'mono']
const GRID_IDS: GridId[] = ['none', 'lines', 'dots']
const geoIcons = { rectangle: Square, ellipse: Circle, triangle: Triangle, diamond: Diamond, hexagon: Hexagon, star: Star }

const contextKinds = computed(() => props.hasSelection ? props.selectedKinds : [props.activeTool as ShapeType])
const hasKind = (...kinds: string[]) => contextKinds.value.some(kind => kinds.includes(kind))
const showStroke = computed(() => hasKind('draw', 'highlight', 'geo', 'arrow', 'line'))
const showDash = computed(() => hasKind('draw', 'geo', 'arrow', 'line'))
const showFill = computed(() => hasKind('geo'))
const showFont = computed(() => hasKind('text', 'note'))
const showGeo = computed(() => (!props.hasSelection && props.activeTool === 'geo') || (props.selectedKinds.length === 1 && props.selectedKinds[0] === 'geo'))
const showAlign = computed(() => props.selectedKinds.length === 1 && props.selectedKinds[0] === 'text')
const showBend = computed(() => props.selectedKinds.length === 1 && (props.selectedKinds[0] === 'line' || props.selectedKinds[0] === 'arrow'))
const showPenMode = computed(() => !props.hasSelection && (props.activeTool === 'draw' || props.activeTool === 'highlight'))

function colorStyle(id: ColorId) {
  return { background: THEMES[props.theme].colors[id].stroke }
}
</script>

<template>
  <section class="clue-board-drawing-inspector" :class="{ 'is-collapsed': collapsed }" :style="{ maxHeight: `${maxHeight}px` }">
    <header class="clue-board-drawing-inspector__header" @pointerdown="emit('drag-start', $event)">
      <NIcon :size="16"><GripHorizontal /></NIcon>
      <strong>绘图属性</strong>
      <NTooltip>
        <template #trigger><button type="button" class="inspector-icon-button" :aria-label="collapsed ? '展开属性面板' : '折叠属性面板'" @pointerdown.stop @click.stop="emit('toggle-collapse')"><NIcon :size="16"><component :is="collapsed ? ChevronDown : ChevronUp" /></NIcon></button></template>
        {{ collapsed ? '展开属性面板' : '折叠属性面板' }}
      </NTooltip>
      <NTooltip>
        <template #trigger><button type="button" class="inspector-icon-button" aria-label="关闭属性面板" @pointerdown.stop @click.stop="emit('close')"><NIcon :size="16"><X /></NIcon></button></template>
        关闭属性面板
      </NTooltip>
    </header>

    <div v-if="!collapsed" class="clue-board-drawing-inspector__body" :style="{ maxHeight: `${Math.max(85, maxHeight - 35)}px` }">
      <template v-if="showStroke || showFont">
        <section class="inspector-section">
          <h4>{{ showFont && !showStroke ? '文字颜色' : '描边' }}</h4>
          <div class="inspector-grid inspector-grid--colors">
            <NTooltip v-for="id in COLOR_IDS" :key="id">
              <template #trigger><button type="button" class="inspector-color" :class="{ 'is-active': color === id }" :style="colorStyle(id)" :disabled="disabled" :aria-label="COLOR_LABELS[id]" @click="emit('color', id)" /></template>
              {{ COLOR_LABELS[id] }}
            </NTooltip>
          </div>
        </section>

        <section class="inspector-section">
          <h4>{{ showFont && !showStroke ? '字号' : '粗细' }}</h4>
          <div class="inspector-grid inspector-grid--four">
            <NTooltip v-for="(id, index) in SIZE_IDS" :key="id">
              <template #trigger><button type="button" class="inspector-tile" :class="{ 'is-active': size === id }" :disabled="disabled" :aria-label="SIZE_LABELS[id]" @click="emit('size', id)"><span class="inspector-weight" :style="{ height: `${index + 1}px` }" /></button></template>
              {{ showFont && !showStroke ? `字号：${SIZE_LABELS[id]}` : `粗细：${SIZE_LABELS[id]}` }}
            </NTooltip>
          </div>
        </section>
      </template>

      <section v-if="showDash" class="inspector-section">
        <h4>线型</h4>
        <div class="inspector-grid inspector-grid--four">
          <NTooltip v-for="id in DASH_IDS" :key="id">
            <template #trigger><button type="button" class="inspector-tile" :class="{ 'is-active': dash === id }" :disabled="disabled" :aria-label="DASH_LABELS[id]" @click="emit('dash', id)"><span class="inspector-dash" :class="`is-${id}`" /></button></template>
            {{ DASH_LABELS[id] }}
          </NTooltip>
        </div>
      </section>

      <section v-if="showBend" class="inspector-section">
        <h4>曲率</h4>
        <div class="inspector-grid inspector-grid--three">
          <NTooltip v-for="preset in [{ value: -40, label: '左弯' }, { value: 0, label: '直线' }, { value: 40, label: '右弯' }]" :key="preset.value">
            <template #trigger><button type="button" class="inspector-tile" :class="{ 'is-active': Math.sign(selectedBend) === Math.sign(preset.value) }" :disabled="disabled" :aria-label="preset.label" @click="emit('bend', preset.value)"><span class="inspector-bend" :class="preset.value < 0 ? 'is-left' : preset.value > 0 ? 'is-right' : 'is-straight'" /></button></template>
            {{ preset.label }}
          </NTooltip>
        </div>
      </section>

      <section v-if="showGeo" class="inspector-section">
        <h4>图形</h4>
        <div class="inspector-grid inspector-grid--three">
          <NTooltip v-for="id in GEO_IDS" :key="id">
            <template #trigger><button type="button" class="inspector-tile" :class="{ 'is-active': geoKind === id }" :disabled="disabled" :aria-label="GEO_LABELS[id]" @click="emit('geo', id)"><NIcon :size="18"><component :is="geoIcons[id]" /></NIcon></button></template>
            {{ GEO_LABELS[id] }}
          </NTooltip>
        </div>
      </section>

      <section v-if="showFill" class="inspector-section">
        <h4>填充</h4>
        <div class="inspector-grid inspector-grid--four">
          <NTooltip v-for="id in FILL_IDS" :key="id">
            <template #trigger><button type="button" class="inspector-tile" :class="{ 'is-active': fill === id }" :disabled="disabled" :aria-label="FILL_LABELS[id]" @click="emit('fill', id)"><span class="inspector-fill" :class="`is-${id}`" /></button></template>
            {{ FILL_LABELS[id] }}
          </NTooltip>
        </div>
      </section>

      <section v-if="showFont" class="inspector-section">
        <h4>字体</h4>
        <div class="inspector-grid inspector-grid--four">
          <NTooltip v-for="id in FONT_IDS" :key="id">
            <template #trigger><button type="button" class="inspector-tile inspector-font" :class="[`is-${id}`, { 'is-active': font === id }]" :disabled="disabled" :aria-label="FONT_LABELS[id]" @click="emit('font', id)">A</button></template>
            {{ FONT_LABELS[id] }}
          </NTooltip>
        </div>
      </section>

      <section v-if="showAlign" class="inspector-section">
        <h4>对齐</h4>
        <div class="inspector-grid inspector-grid--three">
          <NTooltip v-for="option in [{ id: 'start', label: '左对齐', icon: AlignLeft }, { id: 'middle', label: '居中', icon: AlignCenter }, { id: 'end', label: '右对齐', icon: AlignRight }]" :key="option.id">
            <template #trigger><button type="button" class="inspector-tile" :class="{ 'is-active': selectedAlign === option.id }" :disabled="disabled" :aria-label="option.label" @click="emit('align', option.id as 'start' | 'middle' | 'end')"><NIcon :size="18"><component :is="option.icon" /></NIcon></button></template>
            {{ option.label }}
          </NTooltip>
        </div>
      </section>

      <section v-if="showPenMode" class="inspector-section">
        <h4>输入</h4>
        <NTooltip>
          <template #trigger><button type="button" class="inspector-wide-button" :class="{ 'is-active': penMode }" :disabled="disabled" :aria-pressed="penMode" @click="emit('pen-mode', !penMode)"><NIcon :size="17"><Send /></NIcon><span>手写笔模式</span></button></template>
          开启后忽略手掌触摸，压感由支持的手写笔自动提供
        </NTooltip>
      </section>

      <section class="inspector-section">
        <h4>画布</h4>
        <div class="inspector-grid inspector-grid--three">
          <NTooltip v-for="id in GRID_IDS" :key="id">
            <template #trigger><button type="button" class="inspector-tile" :class="{ 'is-active': grid === id }" :disabled="disabled" :aria-label="GRID_LABELS[id]" @click="emit('grid', id)"><NIcon v-if="id === 'dots'" :size="18"><GridDots /></NIcon><Minus v-else-if="id === 'lines'" style="width: 18px" /><span v-else class="inspector-none" /></button></template>
            {{ GRID_LABELS[id] }}
          </NTooltip>
        </div>
      </section>

      <section class="inspector-section inspector-section--actions">
        <h4>对象与历史</h4>
        <div class="inspector-grid inspector-grid--six">
          <NTooltip><template #trigger><button type="button" class="inspector-tile" :disabled="disabled || !hasSelection" aria-label="复制" @click="emit('copy')"><NIcon :size="18"><Copy /></NIcon></button></template>复制所选（Ctrl/Cmd+C）</NTooltip>
          <NTooltip><template #trigger><button type="button" class="inspector-tile" :disabled="disabled || !hasSelection" aria-label="剪切" @click="emit('cut')"><NIcon :size="18"><Cut /></NIcon></button></template>剪切所选（Ctrl/Cmd+X）</NTooltip>
          <NTooltip><template #trigger><button type="button" class="inspector-tile" :disabled="disabled" aria-label="粘贴" @click="emit('paste')"><NIcon :size="18"><Clipboard /></NIcon></button></template>粘贴绘图或图片（Ctrl/Cmd+V）</NTooltip>
          <NTooltip><template #trigger><button type="button" class="inspector-tile" :disabled="disabled || !hasSelection" aria-label="删除" @click="emit('delete')"><NIcon :size="18"><Trash /></NIcon></button></template>删除所选（Del）</NTooltip>
          <NTooltip><template #trigger><button type="button" class="inspector-tile" :disabled="disabled || !hasSelection" aria-label="置于顶层" @click="emit('front')"><NIcon :size="18"><ArrowBarToUp /></NIcon></button></template>置于顶层</NTooltip>
          <NTooltip><template #trigger><button type="button" class="inspector-tile" :disabled="disabled || !hasSelection" aria-label="置于底层" @click="emit('back')"><NIcon :size="18"><ArrowBarToDown /></NIcon></button></template>置于底层</NTooltip>
          <NTooltip><template #trigger><button type="button" class="inspector-tile" :disabled="disabled || !canUndo" aria-label="撤销" @click="emit('undo')"><NIcon :size="18"><ArrowBackUp /></NIcon></button></template>撤销（Ctrl/Cmd+Z）</NTooltip>
          <NTooltip><template #trigger><button type="button" class="inspector-tile" :disabled="disabled || !canRedo" aria-label="重做" @click="emit('redo')"><NIcon :size="18"><ArrowForwardUp /></NIcon></button></template>重做</NTooltip>
          <NTooltip><template #trigger><button type="button" class="inspector-tile" aria-label="适配内容" @click="emit('fit')"><NIcon :size="18"><Focus2 /></NIcon></button></template>适配绘图内容</NTooltip>
          <NTooltip><template #trigger><button type="button" class="inspector-tile is-danger" :disabled="disabled" aria-label="清空绘图" @click="emit('clear')"><NIcon :size="18"><TrashX /></NIcon></button></template>清空绘图，不影响线索卡片、位置和关系</NTooltip>
        </div>
      </section>
    </div>
  </section>
</template>

<style scoped>
.clue-board-drawing-inspector { width: 264px; max-width: calc(100vw - 16px); overflow: hidden; color: var(--sc-text-primary); border: 1px solid var(--sc-border-mute); border-radius: 12px; background: color-mix(in srgb, var(--sc-bg-elevated) 96%, transparent); box-shadow: 0 12px 32px #0003; backdrop-filter: blur(12px); }
.clue-board-drawing-inspector.is-collapsed { width: 168px; }
.clue-board-drawing-inspector__header { display: flex; height: 34px; align-items: center; gap: 7px; padding: 0 5px 0 10px; border-bottom: 1px solid var(--sc-border-mute); cursor: grab; touch-action: none; user-select: none; }
.clue-board-drawing-inspector__header:active { cursor: grabbing; }
.clue-board-drawing-inspector__header strong { flex: 1; font-size: 11px; letter-spacing: .04em; }
.is-collapsed .clue-board-drawing-inspector__header { border-bottom: 0; }
.clue-board-drawing-inspector__body { overflow-y: auto; padding: 7px 9px 10px; scrollbar-width: thin; }
.inspector-icon-button, .inspector-tile, .inspector-wide-button { display: grid; place-items: center; color: var(--sc-text-primary); border: 1px solid transparent; background: transparent; cursor: pointer; }
.inspector-icon-button { width: 26px; height: 26px; border-radius: 6px; }
.inspector-icon-button:hover, .inspector-tile:hover:not(:disabled), .inspector-wide-button:hover:not(:disabled) { border-color: var(--sc-border-mute); background: color-mix(in srgb, var(--primary-color, #3388de) 10%, transparent); }
.inspector-section { padding: 7px 1px; border-bottom: 1px solid color-mix(in srgb, var(--sc-border-mute) 70%, transparent); }
.inspector-section:last-child { border-bottom: 0; }
.inspector-section h4 { margin: 0 0 6px; color: var(--sc-text-secondary); font-size: 10px; font-weight: 600; letter-spacing: .08em; }
.inspector-grid { display: grid; gap: 5px; }
.inspector-grid--colors { grid-template-columns: repeat(6, 1fr); }
.inspector-grid--three { grid-template-columns: repeat(3, 1fr); }
.inspector-grid--four { grid-template-columns: repeat(4, 1fr); }
.inspector-grid--six { grid-template-columns: repeat(6, 1fr); }
.inspector-color { width: 27px; height: 27px; justify-self: center; padding: 0; border: 3px solid var(--sc-bg-elevated); border-radius: 8px; outline: 1px solid color-mix(in srgb, var(--sc-border-mute) 80%, transparent); cursor: pointer; }
.inspector-color.is-active { outline: 2px solid var(--primary-color, #3388de); outline-offset: 1px; }
.inspector-tile { width: 100%; height: 31px; border-radius: 7px; }
.inspector-tile.is-active, .inspector-wide-button.is-active { color: var(--primary-color, #3388de); border-color: color-mix(in srgb, var(--primary-color, #3388de) 52%, transparent); background: color-mix(in srgb, var(--primary-color, #3388de) 14%, transparent); }
.inspector-tile:disabled, .inspector-color:disabled, .inspector-wide-button:disabled { cursor: not-allowed; opacity: .42; }
.inspector-weight { width: 20px; min-height: 1px; border-radius: 9px; background: currentColor; }
.inspector-dash { width: 22px; height: 2px; background: currentColor; }
.inspector-dash.is-draw { transform: rotate(-3deg); border-radius: 50%; clip-path: polygon(0 30%, 18% 0, 36% 70%, 55% 10%, 75% 80%, 100% 25%, 100% 100%, 0 100%); height: 5px; }
.inspector-dash.is-dashed { background: repeating-linear-gradient(90deg, currentColor 0 6px, transparent 6px 10px); }
.inspector-dash.is-dotted { background: repeating-linear-gradient(90deg, currentColor 0 2px, transparent 2px 6px); }
.inspector-bend { width: 25px; height: 13px; border-top: 2px solid currentColor; }
.inspector-bend.is-left { border-radius: 70% 0 0; transform: rotate(-12deg); }
.inspector-bend.is-right { border-radius: 0 70% 0 0; transform: rotate(12deg); }
.inspector-bend.is-straight { height: 2px; }
.inspector-fill { width: 18px; height: 18px; border: 1px solid currentColor; border-radius: 4px; }
.inspector-fill.is-semi { background: color-mix(in srgb, currentColor 28%, transparent); }
.inspector-fill.is-solid { background: currentColor; }
.inspector-fill.is-pattern { background: repeating-linear-gradient(135deg, currentColor 0 1px, transparent 1px 4px); }
.inspector-font { font-size: 15px; }
.inspector-font.is-draw { font-family: cursive; }
.inspector-font.is-serif { font-family: serif; }
.inspector-font.is-mono { font-family: monospace; }
.inspector-none { width: 17px; height: 17px; border: 1px solid var(--sc-border-mute); border-radius: 4px; }
.inspector-wide-button { width: 100%; height: 32px; grid-auto-flow: column; justify-content: center; gap: 7px; border-radius: 7px; font-size: 11px; }
.inspector-tile.is-danger:hover:not(:disabled) { color: var(--error-color, #d03050); }
</style>
