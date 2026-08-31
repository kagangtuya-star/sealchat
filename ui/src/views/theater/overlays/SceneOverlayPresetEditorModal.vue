<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NButton, NInput, NModal, NSpace } from 'naive-ui'
import type { StageSceneOverlayBinding } from '../shared/stage-types'
import type { TheaterSceneOverlayPreset } from './scene-overlay-preset-api'

const props = defineProps<{
  show: boolean
  overlays: StageSceneOverlayBinding[]
  preset?: TheaterSceneOverlayPreset | null
  saving?: boolean
}>()

const emit = defineEmits<{
  close: []
  save: [payload: { name: string, description: string, tags: string[], overlays: StageSceneOverlayBinding[] }]
}>()

const name = ref('')
const description = ref('')
const tagsText = ref('')
const error = ref('')
const title = computed(() => props.preset ? '编辑场景预设' : '保存场景预设')

watch(() => props.show, (show) => {
  if (!show) return
  name.value = props.preset?.name || ''
  description.value = props.preset?.description || ''
  tagsText.value = props.preset?.tags?.join(', ') || ''
  error.value = ''
}, { immediate: true })

const submit = () => {
  const normalizedName = name.value.trim()
  if (!normalizedName) {
    error.value = '请输入预设名称'
    return
  }
  const tags = tagsText.value.split(/[,，]/).map((tag) => tag.trim()).filter(Boolean).slice(0, 16)
  emit('save', { name: normalizedName, description: description.value.trim(), tags, overlays: props.overlays })
}
</script>

<template>
  <n-modal :show="show" preset="card" style="width: min(440px, calc(100vw - 32px))" :title="title" @update:show="value => !value && emit('close')">
    <div class="scene-overlay-preset-editor">
      <n-input v-model:value="name" maxlength="128" show-count placeholder="预设名称" />
      <n-input v-model:value="description" type="textarea" maxlength="512" show-count :autosize="{ minRows: 3, maxRows: 6 }" placeholder="描述（可选）" />
      <n-input v-model:value="tagsText" placeholder="标签，用逗号分隔（可选）" />
      <small v-if="error" class="scene-overlay-preset-editor__error">{{ error }}</small>
      <n-space justify="end">
        <n-button @click="emit('close')">取消</n-button>
        <n-button type="primary" :loading="saving" :disabled="!preset && !overlays.length" @click="submit">保存</n-button>
      </n-space>
    </div>
  </n-modal>
</template>

<style scoped>
.scene-overlay-preset-editor { display: grid; gap: 12px; }
.scene-overlay-preset-editor__error { color: var(--n-error-color); }
</style>
