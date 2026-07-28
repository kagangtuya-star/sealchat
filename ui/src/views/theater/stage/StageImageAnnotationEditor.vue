<script setup lang="ts">
import { ref, watch } from 'vue'
import { NButton, NCard, NCheckbox, NColorPicker, NInput, NInputNumber, NModal, NSelect, NSlider } from 'naive-ui'
import {
  createDefaultStageImageAnnotation,
  normalizeStageImageAnnotation,
  type StageImageAnnotation,
} from '../shared/stage-types'

const props = defineProps<{
  show: boolean
  value?: StageImageAnnotation
  objectName: string
}>()

const emit = defineEmits<{
  close: []
  save: [value: StageImageAnnotation]
}>()

const draft = ref<StageImageAnnotation>(createDefaultStageImageAnnotation())
const styleOptions = [
  { label: '卡片式', value: 'card' },
  { label: '气泡式', value: 'bubble' },
  { label: '标签式', value: 'tag' },
  { label: '简约浮层', value: 'floating' },
  { label: '底部提示', value: 'footer' },
]
const placementOptions = [
  { label: '自动避让', value: 'auto' },
  { label: '上方', value: 'top' },
  { label: '右侧', value: 'right' },
  { label: '下方', value: 'bottom' },
  { label: '左侧', value: 'left' },
]

watch(() => [props.show, props.value] as const, ([show]) => {
  if (!show) return
  draft.value = normalizeStageImageAnnotation(props.value) || createDefaultStageImageAnnotation()
}, { immediate: true, deep: true })

const reset = () => { draft.value = createDefaultStageImageAnnotation() }
const save = () => emit('save', normalizeStageImageAnnotation(draft.value) || createDefaultStageImageAnnotation())
</script>

<template>
  <n-modal :show="show" :mask-closable="false" @update:show="value => { if (!value) emit('close') }">
    <n-card
      class="theater-image-annotation-editor"
      :title="`图片标注 · ${objectName}`"
      closable
      style="width: min(520px, calc(100vw - 24px))"
      @close="emit('close')"
    >
      <div class="theater-image-annotation-editor__body">
        <section class="theater-image-annotation-editor__form">
          <n-checkbox v-model:checked="draft.enabled">启用悬停标注</n-checkbox>

          <label>标注内容</label>
          <n-input v-model:value="draft.text" type="textarea" :autosize="{ minRows: 4, maxRows: 10 }" maxlength="2000" show-count />

          <div class="theater-image-annotation-editor__grid">
            <label>样式</label>
            <n-select v-model:value="draft.style" :options="styleOptions" />
            <label>位置</label>
            <n-select v-model:value="draft.placement" :options="placementOptions" />
            <label>文字颜色</label>
            <n-color-picker v-model:value="draft.textColor" :show-alpha="false" :modes="['hex']" />
            <label>背景颜色</label>
            <n-color-picker v-model:value="draft.backgroundColor" :show-alpha="false" :modes="['hex']" />
          </div>

          <label>字号 · {{ draft.fontSize }} px</label>
          <n-slider v-model:value="draft.fontSize" :min="10" :max="36" :step="1" />

          <label>背景不透明度 · {{ Math.round(draft.backgroundOpacity * 100) }}%</label>
          <n-slider v-model:value="draft.backgroundOpacity" :min="0" :max="1" :step="0.05" />

          <div class="theater-image-annotation-editor__grid">
            <label>最大宽度</label>
            <n-input-number v-model:value="draft.maxWidth" :min="120" :max="480" :step="10"><template #suffix>px</template></n-input-number>
            <label>显示延迟</label>
            <n-input-number v-model:value="draft.delayMs" :min="0" :max="1000" :step="50"><template #suffix>ms</template></n-input-number>
          </div>
        </section>

      </div>

      <template #footer>
        <div class="theater-image-annotation-editor__actions">
          <n-button quaternary @click="reset">恢复默认</n-button>
          <span />
          <n-button @click="emit('close')">取消</n-button>
          <n-button type="primary" @click="save">保存</n-button>
        </div>
      </template>
    </n-card>
  </n-modal>
</template>

<style scoped>
.theater-image-annotation-editor { max-height: calc(100vh - 24px); overflow: auto; }
.theater-image-annotation-editor__body { display: grid; grid-template-columns: minmax(0, 1fr); }
.theater-image-annotation-editor__form { min-width: 0; display: grid; gap: 9px; }
.theater-image-annotation-editor__form > label, .theater-image-annotation-editor__grid > label { color: var(--n-text-color-2); font-size: 12px; }
.theater-image-annotation-editor__grid { display: grid; grid-template-columns: 82px minmax(0, 1fr); align-items: center; gap: 8px 10px; }
.theater-image-annotation-editor__actions { display: grid; grid-template-columns: auto 1fr auto auto; gap: 8px; }
</style>
