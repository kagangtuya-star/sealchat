<script setup lang="ts">
import { computed } from 'vue'
import { ArrowDown, ArrowUp, Refresh, Trash } from '@vicons/tabler'
import type { TheaterPresentation, TheaterTransform, TheaterVisualLayer } from '@/types/theaterPresentation'
import type { TheaterEditorCommand, TheaterSection, TheaterSectionMode, TheaterSelection } from './theaterPresentationEditorState'

const props = defineProps<{
  draft: TheaterPresentation
  selection: TheaterSelection
  mode: 'base' | 'variant'
  sectionModes: Record<TheaterSection, TheaterSectionMode>
}>()
const emit = defineEmits<{
  dispatch: [command: TheaterEditorCommand, options?: { transient?: boolean }]
  transactionStart: []
  transactionEnd: []
}>()
const paddingKeys = ['top', 'right', 'bottom', 'left'] as const

const section = computed<TheaterSection>(() => {
  if (props.selection.kind === 'portrait') return 'portrait'
  if (props.selection.kind === 'speaker') return 'speaker'
  if (props.selection.kind === 'content') return 'content'
  if (props.selection.kind === 'decoration') return 'decorations'
  return 'dialogue'
})
const layer = computed<TheaterVisualLayer | null>(() => {
  const selection = props.selection
  if (selection.kind === 'portrait') return props.draft.portrait
  if (selection.kind === 'decoration') return props.draft.portraitDecorations.find((item) => item.id === selection.id) || null
  if (selection.kind === 'dialogue-frame') return props.draft.dialogue.frame
  return null
})
const transform = computed(() => {
  if (props.selection.kind === 'dialogue') return props.draft.dialogue.transform
  if (props.selection.kind === 'speaker') return props.draft.dialogue.speaker.transform
  if (props.selection.kind === 'content') return props.draft.dialogue.content.transform
  return layer.value?.transform || null
})
const decorationIndex = computed(() => props.selection.kind === 'decoration'
  ? props.draft.portraitDecorations.findIndex((item) => item.id === (props.selection as { kind: 'decoration'; id: string }).id)
  : -1)

const setTransform = (key: keyof TheaterTransform, value: number | null) => {
  if (value === null || !Number.isFinite(value)) return
  emit('dispatch', { type: 'set-transform', target: props.selection, transform: { [key]: value } }, { transient: true })
}
const textLayer = computed(() => props.selection.kind === 'speaker' || props.selection.kind === 'content'
  ? props.draft.dialogue[props.selection.kind]
  : null)
const setRotation = (value: number) => setTransform('rotation', value)
const setOpacity = (value: number) => setTransform('opacity', value)
const setPlaybackRate = (value: number | null) => {
  if (value === null) return
  emit('dispatch', { type: 'set-layer-property', target: props.selection, property: 'playbackRate', value }, { transient: true })
}
const setFontScale = (value: number) => {
  emit('dispatch', { type: 'set-layer-property', target: props.selection, property: 'fontScale', value }, { transient: true })
}
const setContentColor = (value: string) => {
  emit('dispatch', { type: 'set-dialogue-property', property: 'contentColor', value })
}
const setNarration = (property: 'enabled' | 'backdropColor' | 'backdropOpacity', value: boolean | string | number) => {
  emit('dispatch', { type: 'set-narration-property', property, value }, { transient: property === 'backdropOpacity' })
}
const setPadding = (key: typeof paddingKeys[number], value: number | null) => {
  if (value === null) return
  emit('dispatch', { type: 'set-dialogue-padding', padding: { [key]: value } }, { transient: true })
}
const setNameGap = (value: number | null) => {
  if (value === null) return
  emit('dispatch', { type: 'set-dialogue-property', property: 'nameGap', value }, { transient: true })
}
const reorder = (direction: -1 | 1) => {
  if (props.selection.kind !== 'decoration') return
  const index = decorationIndex.value
  const nextIndex = index + direction
  if (index < 0 || nextIndex < 0 || nextIndex >= props.draft.portraitDecorations.length) return
  const beforeId = direction < 0
    ? props.draft.portraitDecorations[nextIndex].id
    : props.draft.portraitDecorations[nextIndex + 1]?.id || null
  emit('dispatch', { type: 'reorder-decoration', id: props.selection.id, beforeId })
}
</script>

<template>
  <div class="theater-inspector">
    <div v-if="mode === 'variant'" class="theater-inspector__mode">
      <div class="theater-inspector__label">当前部分</div>
      <n-radio-group
        :value="sectionModes[section]"
        @update:value="emit('dispatch', { type: 'set-section-mode', section, mode: $event as TheaterSectionMode })"
      >
        <n-radio-button value="inherit">继承</n-radio-button>
        <n-radio-button value="custom">自定义</n-radio-button>
        <n-radio-button value="clear">清除</n-radio-button>
      </n-radio-group>
    </div>

    <template v-if="transform">
      <div class="theater-inspector__label">变换</div>
      <div v-if="selection.kind !== 'decoration'" class="theater-inspector__number-grid">
        <n-input-number :value="transform.x" :step="0.01" :min="-1" :max="2" @focus="emit('transactionStart')" @blur="emit('transactionEnd')" @update:value="setTransform('x', $event)" ><template #prefix>X</template></n-input-number>
        <n-input-number :value="transform.y" :step="0.01" :min="-1" :max="2" @focus="emit('transactionStart')" @blur="emit('transactionEnd')" @update:value="setTransform('y', $event)" ><template #prefix>Y</template></n-input-number>
        <n-input-number :value="transform.width" :step="0.01" :min="0.01" :max="3" @focus="emit('transactionStart')" @blur="emit('transactionEnd')" @update:value="setTransform('width', $event)" ><template #prefix>W</template></n-input-number>
        <n-input-number :value="transform.height" :step="0.01" :min="0.01" :max="3" @focus="emit('transactionStart')" @blur="emit('transactionEnd')" @update:value="setTransform('height', $event)" ><template #prefix>H</template></n-input-number>
      </div>
      <div v-else class="theater-inspector__sync-values">
        X {{ transform.x.toFixed(3) }} · Y {{ transform.y.toFixed(3) }} · W {{ transform.width.toFixed(3) }} · H {{ transform.height.toFixed(3) }}
      </div>
      <div class="theater-inspector__slider-field">
        <span>旋转</span>
        <n-slider :value="transform.rotation" :min="-180" :max="180" :step="1" :tooltip="false" @pointerdown="emit('transactionStart')" @pointerup="emit('transactionEnd')" @focus="emit('transactionStart')" @blur="emit('transactionEnd')" @update:value="setRotation" />
        <span>{{ Math.round(transform.rotation) }}°</span>
      </div>
      <div class="theater-inspector__slider-field">
        <span>透明度</span>
        <n-slider :value="transform.opacity" :min="0" :max="1" :step="0.01" :tooltip="false" @pointerdown="emit('transactionStart')" @pointerup="emit('transactionEnd')" @focus="emit('transactionStart')" @blur="emit('transactionEnd')" @update:value="setOpacity" />
        <span>{{ Math.round(transform.opacity * 100) }}%</span>
      </div>
    </template>

    <template v-if="section === 'decorations'">
      <div class="theater-inspector__label">配饰列表</div>
      <div class="theater-inspector__decoration-list">
        <n-button
          v-for="(item, index) in draft.portraitDecorations"
          :key="item.id"
          size="small"
          :type="selection.kind === 'decoration' && selection.id === item.id ? 'primary' : 'default'"
          :secondary="selection.kind === 'decoration' && selection.id === item.id"
          @click="emit('dispatch', { type: 'select', target: { kind: 'decoration', id: item.id } })"
        >配饰 {{ index + 1 }}{{ item.enabled ? '' : '（停用）' }}</n-button>
      </div>
    </template>

    <template v-if="textLayer">
      <div class="theater-inspector__label">显示</div>
      <n-switch
        :value="textLayer.enabled"
        @update:value="emit('dispatch', { type: 'set-layer-property', target: selection, property: 'enabled', value: $event })"
      />
      <div class="theater-inspector__label">字体缩放</div>
      <div class="theater-inspector__slider-field">
        <span>比例</span>
        <n-slider
          :value="textLayer.fontScale"
          :min="0.25"
          :max="4"
          :step="0.05"
          :tooltip="false"
          @pointerdown="emit('transactionStart')"
          @pointerup="emit('transactionEnd')"
          @focus="emit('transactionStart')"
          @blur="emit('transactionEnd')"
          @update:value="setFontScale"
        />
        <span>{{ Math.round(textLayer.fontScale * 100) }}%</span>
      </div>
      <template v-if="selection.kind === 'content'">
        <div class="theater-inspector__label">默认文本颜色</div>
        <n-color-picker
          :value="draft.dialogue.contentColor"
          :show-alpha="false"
          :modes="['hex']"
          @update:value="setContentColor"
        />
      </template>
    </template>

    <template v-if="layer">
      <div class="theater-inspector__label">图层</div>
      <n-checkbox
        :checked="layer.enabled"
        @update:checked="emit('dispatch', { type: 'set-layer-property', target: selection, property: 'enabled', value: $event })"
      >启用</n-checkbox>
      <n-select
        :value="layer.blendMode"
        :options="[{ label: 'Normal', value: 'normal' }, { label: 'Multiply', value: 'multiply' }, { label: 'Screen', value: 'screen' }, { label: 'Overlay', value: 'overlay' }]"
        @update:value="emit('dispatch', { type: 'set-layer-property', target: selection, property: 'blendMode', value: $event })"
      />
      <n-input-number
        v-if="layer.media.kind === 'video'"
        :value="layer.playbackRate"
        :step="0.25"
        :min="0.25"
        :max="4"
        @focus="emit('transactionStart')"
        @blur="emit('transactionEnd')"
        @update:value="setPlaybackRate"
      >
        <template #prefix>速度</template>
      </n-input-number>
    </template>

    <template v-if="selection.kind === 'dialogue'">
      <div class="theater-inspector__label">对话框内容</div>
      <div class="theater-inspector__number-grid">
        <n-input-number v-for="key in paddingKeys" :key="key" :value="draft.dialogue.padding[key]" :step="0.01" :min="0" :max="1" @focus="emit('transactionStart')" @blur="emit('transactionEnd')" @update:value="setPadding(key, $event)"><template #prefix>{{ key.slice(0, 1).toUpperCase() }}</template></n-input-number>
      </div>
      <n-input-number :value="draft.dialogue.nameGap" :step="0.01" :min="0" :max="1" @focus="emit('transactionStart')" @blur="emit('transactionEnd')" @update:value="setNameGap"><template #prefix>nameGap</template></n-input-number>
      <n-radio-group :value="draft.dialogue.textAlign" @update:value="emit('dispatch', { type: 'set-dialogue-property', property: 'textAlign', value: $event })">
        <n-radio-button value="left">左</n-radio-button>
        <n-radio-button value="center">中</n-radio-button>
        <n-radio-button value="right">右</n-radio-button>
      </n-radio-group>
    </template>

    <section v-if="section === 'portrait'" class="theater-inspector__narration">
      <div v-if="mode === 'variant'" class="theater-inspector__mode">
        <div class="theater-inspector__label">旁白模式配置</div>
        <n-radio-group
          :value="sectionModes.narration"
          @update:value="emit('dispatch', { type: 'set-section-mode', section: 'narration', mode: $event as TheaterSectionMode })"
        >
          <n-radio-button value="inherit">继承</n-radio-button>
          <n-radio-button value="custom">自定义</n-radio-button>
          <n-radio-button value="clear">清除</n-radio-button>
        </n-radio-group>
      </div>
      <div class="theater-inspector__narration-switch">
        <span>旁白模式</span>
        <n-switch size="large" :value="draft.narration.enabled" @update:value="setNarration('enabled', $event)" />
      </div>
      <template v-if="draft.narration.enabled">
        <div class="theater-inspector__label">幕布颜色</div>
        <n-color-picker
          :value="draft.narration.backdropColor"
          :show-alpha="false"
          :modes="['hex']"
          @update:value="setNarration('backdropColor', $event)"
        />
        <div class="theater-inspector__slider-field">
          <span>透明度</span>
          <n-slider
            :value="draft.narration.backdropOpacity"
            :min="0"
            :max="1"
            :step="0.01"
            :tooltip="false"
            @pointerdown="emit('transactionStart')"
            @pointerup="emit('transactionEnd')"
            @focus="emit('transactionStart')"
            @blur="emit('transactionEnd')"
            @update:value="setNarration('backdropOpacity', $event)"
          />
          <span>{{ Math.round(draft.narration.backdropOpacity * 100) }}%</span>
        </div>
      </template>
    </section>

    <div class="theater-inspector__actions">
      <n-tooltip><template #trigger><n-button quaternary circle @click="emit('dispatch', { type: 'reset-section', section })"><template #icon><n-icon><Refresh /></n-icon></template></n-button></template>重置当前部分</n-tooltip>
      <template v-if="selection.kind === 'decoration'">
        <n-tooltip><template #trigger><n-button quaternary circle :disabled="decorationIndex <= 0" @click="reorder(-1)"><template #icon><n-icon><ArrowUp /></n-icon></template></n-button></template>上移</n-tooltip>
        <n-tooltip><template #trigger><n-button quaternary circle :disabled="decorationIndex < 0 || decorationIndex >= draft.portraitDecorations.length - 1" @click="reorder(1)"><template #icon><n-icon><ArrowDown /></n-icon></template></n-button></template>下移</n-tooltip>
        <n-tooltip><template #trigger><n-button quaternary circle type="error" @click="emit('dispatch', { type: 'remove-decoration', id: selection.id })"><template #icon><n-icon><Trash /></n-icon></template></n-button></template>删除图层</n-tooltip>
      </template>
    </div>
  </div>
</template>

<style scoped>
.theater-inspector { display: flex; flex-direction: column; gap: 10px; min-width: 0; }
.theater-inspector__label { margin-top: 4px; color: var(--sc-text-secondary, #64748b); font-size: 12px; font-weight: 600; }
.theater-inspector__mode { display: flex; flex-direction: column; gap: 8px; }
.theater-inspector__number-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
.theater-inspector__slider-field { display: grid; grid-template-columns: 52px minmax(0, 1fr) 42px; align-items: center; gap: 8px; font-size: 12px; }
.theater-inspector__slider-field > :last-child { text-align: right; font-variant-numeric: tabular-nums; }
.theater-inspector__sync-values { color: var(--sc-text-secondary, #64748b); font-size: 12px; font-variant-numeric: tabular-nums; }
.theater-inspector__decoration-list { display: flex; flex-direction: column; gap: 6px; }
.theater-inspector__actions { display: flex; align-items: center; gap: 4px; border-top: 1px solid var(--sc-border-mute, rgba(148,163,184,.24)); padding-top: 8px; }
.theater-inspector__narration { display: flex; flex-direction: column; gap: 10px; padding-top: 12px; border-top: 1px solid var(--sc-border-mute, rgba(148,163,184,.24)); }
.theater-inspector__narration-switch { display: flex; align-items: center; justify-content: space-between; gap: 12px; font-size: 15px; font-weight: 700; }
@media (max-width: 720px) { .theater-inspector__number-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); } }
</style>
