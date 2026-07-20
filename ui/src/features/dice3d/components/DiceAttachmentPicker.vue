<script setup lang="ts">
import { computed, ref } from 'vue'
import { useMessage } from 'naive-ui'

import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver'
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader'

const props = withDefaults(defineProps<{ modelValue?: string; worldId?: string; platform?: boolean; accept?: string; kind?: 'audio' | 'image' }>(), { accept: '*/*', kind: 'audio' })
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const message = useMessage()
const inputRef = ref<HTMLInputElement | null>(null)
const uploading = ref(false)
const url = computed(() => resolveAttachmentUrl(props.modelValue || ''))

const upload = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  uploading.value = true
  try {
    const result = await uploadImageAttachment(file, {
      channelId: props.platform ? 'platform-dice3d-asset' : 'dice3d-asset',
      rootId: props.platform ? 'platform' : props.worldId,
      rootIdType: props.platform ? 'platform_dice3d_asset' : 'dice3d_asset',
      confirm: true,
      skipCompression: true,
    })
    emit('update:modelValue', result.attachmentId)
    message.success('附件已上传；保存后生效')
  } catch (error: any) {
    message.error(error?.message || '附件上传失败')
  } finally {
    uploading.value = false
  }
}
</script>

<template>
  <div class="asset-picker">
    <input ref="inputRef" type="file" :accept="accept" hidden @change="upload">
    <audio v-if="kind === 'audio' && url" :src="url" controls preload="metadata" />
    <img v-else-if="kind === 'image' && url" :src="url" alt="附件预览">
    <span v-else class="asset-picker__empty">未上传附件</span>
    <n-button size="small" secondary :loading="uploading" @click="inputRef?.click()">{{ modelValue ? '更换文件' : '点击上传' }}</n-button>
    <n-button v-if="modelValue" size="small" quaternary type="error" @click="emit('update:modelValue', '')">清除</n-button>
  </div>
</template>

<style scoped>
.asset-picker { width: 100%; display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }.asset-picker audio { min-width: 220px; max-width: 100%; height: 34px; }.asset-picker img { width: 64px; height: 64px; object-fit: cover; border-radius: 8px; }.asset-picker__empty { flex: 1; min-width: 140px; color: var(--sc-text-secondary); font-size: 12px; }
</style>
