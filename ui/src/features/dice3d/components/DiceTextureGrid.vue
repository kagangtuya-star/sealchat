<script setup lang="ts">
import { ref } from 'vue'
import { useMessage } from 'naive-ui'

import type { Dice3DSkin } from '@/types'
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver'
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader'

const props = defineProps<{ modelValue: Dice3DSkin; worldId?: string; platform?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: Dice3DSkin] }>()
const message = useMessage()
const inputRef = ref<HTMLInputElement | null>(null)
const activeType = ref<string | null>(null)
const uploadingType = ref<string | null>(null)
const diceTypes = ['d2', 'd4', 'd6', 'd8', 'd10', 'd12', 'd20', 'd100']

const trigger = (type: string) => {
  activeType.value = type
  inputRef.value?.click()
}
const clear = (type: string) => {
  const textures = { ...(props.modelValue.textures || {}) }
  delete textures[type]
  emit('update:modelValue', { ...props.modelValue, textures })
}
const upload = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  const type = activeType.value
  input.value = ''
  if (!file || !type) return
  uploadingType.value = type
  try {
    const result = await uploadImageAttachment(file, {
      rootId: props.platform ? 'platform' : props.worldId,
      rootIdType: props.platform ? 'platform_dice3d_texture' : 'dice3d_texture',
      confirm: true,
      skipCompression: true,
    })
    emit('update:modelValue', { ...props.modelValue, textures: { ...(props.modelValue.textures || {}), [type]: result.attachmentId } })
    message.success(`${type} 图集已上传；保存后生效`)
  } catch (error: any) {
    message.error(error?.message || `${type} 图集上传失败`)
  } finally {
    uploadingType.value = null
    activeType.value = null
  }
}
</script>

<template>
  <div class="texture-grid">
    <input ref="inputRef" type="file" accept="image/png,image/jpeg,image/webp" hidden @change="upload">
    <section v-for="type in diceTypes" :key="type" class="texture-item">
      <button type="button" class="texture-item__preview" @click="trigger(type)">
        <img v-if="modelValue.textures?.[type]" :src="resolveAttachmentUrl(modelValue.textures[type])" :alt="`${type} 图集`">
        <span v-else>{{ type }}<small>点击上传</small></span>
        <i v-if="uploadingType === type">上传中</i>
      </button>
      <div class="texture-item__actions">
        <strong>{{ type }}</strong>
        <n-button v-if="modelValue.textures?.[type]" text size="tiny" type="error" @click="clear(type)">清除</n-button>
      </div>
    </section>
  </div>
</template>

<style scoped>
.texture-grid { width: 100%; display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; }
.texture-item { min-width: 0; }.texture-item__preview { position: relative; width: 100%; aspect-ratio: 1; display: grid; place-items: center; overflow: hidden; padding: 0; border: 1px dashed var(--sc-border-muted, rgba(148,163,184,.32)); border-radius: 9px; color: var(--sc-text-secondary); background: color-mix(in srgb, var(--sc-bg-input, #111827) 92%, transparent); cursor: pointer; }.texture-item__preview:hover { border-color: #36ad92; }.texture-item__preview img { width: 100%; height: 100%; object-fit: cover; }.texture-item__preview span { display: flex; flex-direction: column; gap: 4px; font-weight: 700; }.texture-item__preview small { font-size: 9px; font-weight: 400; }.texture-item__preview i { position: absolute; inset: 0; display: grid; place-items: center; color: #fff; background: rgba(15,23,42,.72); font-size: 10px; font-style: normal; }
.texture-item__actions { display: flex; align-items: center; justify-content: space-between; padding: 4px 2px 0; font-size: 11px; }
@media (max-width: 520px) { .texture-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
</style>
