<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'

import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver'
import { useUtilsStore } from '@/stores/utils'
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader'
import { diceAudio } from '../diceAudio'

const props = withDefaults(defineProps<{
  modelValue?: string
  worldId?: string
  platform?: boolean
  accept?: string
  kind?: 'audio' | 'image'
  disabled?: boolean
  disabledReason?: string
}>(), {
  accept: '*/*',
  kind: 'audio',
  disabled: false,
})
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()
const message = useMessage()
const utils = useUtilsStore()
const inputRef = ref<HTMLInputElement | null>(null)
const uploading = ref(false)
const url = computed(() => resolveAttachmentUrl(props.modelValue || ''))

const allowedAudioMimes = ['audio/mpeg', 'audio/mp3', 'audio/ogg', 'audio/wav', 'audio/x-wav', 'audio/webm', 'audio/aac', 'audio/flac', 'audio/mp4']
const allowedAudioExt = /\.(mp3|ogg|wav|webm|aac|flac|m4a)$/i

const validateAudioFile = (file: File): string | null => {
  const mime = (file.type || '').toLowerCase()
  const mimeOk = !mime || allowedAudioMimes.some(item => mime === item || mime.startsWith(item))
  const extOk = allowedAudioExt.test(file.name)
  if (!mimeOk && !extOk) return '仅支持 mp3 / ogg / wav / webm / aac / flac 音效文件'
  const sizeLimit = utils.fileSizeLimit
  if (file.size > sizeLimit) {
    return `文件大小超过限制（最大 ${(sizeLimit / 1024 / 1024).toFixed(1)} MB）`
  }
  return null
}

watch(() => props.modelValue, (value, previous) => {
  if (previous) diceAudio.invalidate(previous)
  if (value) void diceAudio.ensureLoaded(value)
}, { immediate: true })

// 打开设置即尝试解锁，便于后续各端投掷广播可播
void diceAudio.unlock()

const upload = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file || props.disabled) return
  if (props.kind === 'audio') {
    const invalid = validateAudioFile(file)
    if (invalid) {
      message.error(invalid)
      return
    }
  }
  uploading.value = true
  try {
    // 不传伪造 ChannelId：后端会按频道解析，假 ID 会 404「频道不存在」
    const result = await uploadImageAttachment(file, {
      rootId: props.platform ? 'platform' : props.worldId,
      rootIdType: props.platform ? 'platform_dice3d_asset' : 'dice3d_asset',
      confirm: true,
      skipCompression: true,
    })
    if (props.modelValue) diceAudio.invalidate(props.modelValue)
    emit('update:modelValue', result.attachmentId)
    const loaded = await diceAudio.ensureLoaded(result.attachmentId)
    if (props.kind === 'audio' && !loaded) {
      message.warning('附件已上传，但浏览器无法预加载该音效，请试听确认')
    } else {
      message.success('附件已上传；保存后生效')
    }
  } catch (error: any) {
    message.error(error?.message || '附件上传失败')
  } finally {
    uploading.value = false
  }
}

const clear = () => {
  if (props.disabled) return
  if (props.modelValue) diceAudio.invalidate(props.modelValue)
  emit('update:modelValue', '')
}
</script>

<template>
  <div class="asset-picker" :class="{ 'is-disabled': disabled }">
    <input ref="inputRef" type="file" :accept="accept" hidden :disabled="disabled" @change="upload">
    <audio v-if="kind === 'audio' && url" :src="url" controls preload="metadata" />
    <img v-else-if="kind === 'image' && url" :src="url" alt="附件预览">
    <span v-else class="asset-picker__empty">{{ kind === 'audio' ? '未上传自定义音效' : '未上传附件' }}</span>
    <n-button size="small" secondary :loading="uploading" :disabled="disabled" @click="inputRef?.click()">
      {{ modelValue ? '更换文件' : '点击上传' }}
    </n-button>
    <n-button v-if="modelValue" size="small" quaternary type="error" :disabled="disabled" @click="clear">清除</n-button>
    <p v-if="disabled && disabledReason" class="asset-picker__hint">{{ disabledReason }}</p>
    <p v-else-if="kind === 'audio' && !modelValue" class="asset-picker__hint">无默认音效；上传后才会在投掷时播放</p>
  </div>
</template>

<style scoped>
.asset-picker { width: 100%; display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.asset-picker audio { min-width: 220px; max-width: 100%; height: 34px; }
.asset-picker img { width: 64px; height: 64px; object-fit: cover; border-radius: 8px; }
.asset-picker__empty { flex: 1; min-width: 140px; color: var(--sc-text-secondary); font-size: 12px; }
.asset-picker__hint { flex: 1 1 100%; margin: 0; color: var(--sc-text-secondary, #71717a); font-size: 12px; line-height: 1.4; }
.asset-picker.is-disabled { opacity: 0.72; }
</style>
