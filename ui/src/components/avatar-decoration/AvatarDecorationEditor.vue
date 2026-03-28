<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { api } from '@/stores/_config'
import { useUserStore } from '@/stores/user'
import { useUtilsStore } from '@/stores/utils'
import type { AvatarDecoration } from '@/types'
import UserAvatarDecoration from '@/components/user-avatar-decoration.vue'

const props = defineProps<{
  modelValue?: AvatarDecoration | null
  avatarSrc?: string
  fallbackText?: string
  previewName?: string
  uploadChannelId?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: AvatarDecoration | null): void
}>()

const user = useUserStore()
const utils = useUtilsStore()
const message = useMessage()
const inputRef = ref<HTMLInputElement | null>(null)
const uploading = ref(false)
const dragging = ref(false)

let dragPointerId: number | null = null
let dragStartX = 0
let dragStartY = 0
let dragStartOffsetX = 0
let dragStartOffsetY = 0

const decoration = computed<AvatarDecoration | null>({
  get: () => props.modelValue || null,
  set: (value) => emit('update:modelValue', value),
})

const normalizedDecoration = computed<AvatarDecoration>(() => ({
  enabled: decoration.value?.enabled === true,
  decorationId: decoration.value?.decorationId || '',
  resourceAttachmentId: decoration.value?.resourceAttachmentId || '',
  fallbackAttachmentId: decoration.value?.fallbackAttachmentId || '',
  settings: {
    scale: decoration.value?.settings?.scale ?? 1,
    offsetX: decoration.value?.settings?.offsetX ?? 0,
    offsetY: decoration.value?.settings?.offsetY ?? 0,
    rotation: decoration.value?.settings?.rotation ?? 0,
    zIndex: decoration.value?.settings?.zIndex ?? 1,
    opacity: decoration.value?.settings?.opacity ?? 1,
    blendMode: decoration.value?.settings?.blendMode ?? 'normal',
  },
}))

const applyPatch = (patch: Partial<AvatarDecoration>) => {
  decoration.value = {
    ...normalizedDecoration.value,
    ...patch,
    settings: {
      ...normalizedDecoration.value.settings,
      ...(patch.settings || {}),
    },
  }
}

const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value))

const selectFile = () => {
  if (inputRef.value) {
    inputRef.value.value = ''
  }
  inputRef.value?.click()
}

const handleFileChange = async (event: Event) => {
  const target = event.target as HTMLInputElement | null
  const file = target?.files?.[0]
  if (!file) {
    return
  }
  if (file.size > utils.fileSizeLimit) {
    const limitMB = (utils.fileSizeLimit / 1024 / 1024).toFixed(1)
    message.error(`文件大小超过限制（最大 ${limitMB} MB）`)
    return
  }
  if (!['image/png', 'image/webp'].includes(file.type)) {
    message.error('头像装饰仅支持 PNG 或 WEBP')
    return
  }

  uploading.value = true
  try {
    const formData = new FormData()
    formData.append('file', file, file.name)
    const resp = await api.post('/api/v1/upload', formData, {
      headers: {
        Authorization: `${user.token}`,
        ChannelId: props.uploadChannelId || 'user-avatar-decoration',
      },
    })
    const attachmentId = resp.data?.ids?.[0]
    if (!attachmentId) {
      message.error('上传失败，未返回附件ID')
      return
    }
    applyPatch({
      enabled: true,
      resourceAttachmentId: `id:${attachmentId}`,
    })
    message.success('装饰资源上传成功')
  } catch (error) {
    message.error('装饰资源上传失败: ' + String(error))
  } finally {
    uploading.value = false
  }
}

const clearDecoration = () => {
  decoration.value = null
}

const resetDecoration = () => {
  if (!normalizedDecoration.value.resourceAttachmentId) {
    decoration.value = null
    return
  }
  decoration.value = {
    ...normalizedDecoration.value,
    enabled: true,
    settings: {
      scale: 1,
      offsetX: 0,
      offsetY: 0,
      rotation: 0,
      zIndex: 1,
      opacity: 1,
      blendMode: 'normal',
    },
  }
}

const updateResourceAttachmentId = (value: string) => {
  applyPatch({
    enabled: true,
    resourceAttachmentId: value.trim(),
  })
}

const updateScale = (value: number | null) => {
  applyPatch({
    enabled: true,
    settings: { scale: value ?? 1 },
  })
}

const updateOffsetX = (value: number | null) => {
  applyPatch({
    enabled: true,
    settings: { offsetX: value ?? 0 },
  })
}

const updateOffsetY = (value: number | null) => {
  applyPatch({
    enabled: true,
    settings: { offsetY: value ?? 0 },
  })
}

const updateRotation = (value: number | null) => {
  applyPatch({
    enabled: true,
    settings: { rotation: value ?? 0 },
  })
}

const updateOpacity = (value: number | null) => {
  applyPatch({
    enabled: true,
    settings: { opacity: value ?? 1 },
  })
}

const updateZIndex = (value: number) => {
  applyPatch({
    enabled: true,
    settings: { zIndex: value },
  })
}

const stopDragging = () => {
  dragging.value = false
  dragPointerId = null
  window.removeEventListener('pointermove', handlePointerMove)
  window.removeEventListener('pointerup', handlePointerUp)
  window.removeEventListener('pointercancel', handlePointerUp)
}

const handlePointerMove = (event: PointerEvent) => {
  if (!dragging.value || dragPointerId !== event.pointerId) {
    return
  }
  const nextOffsetX = clamp(Math.round(dragStartOffsetX + (event.clientX - dragStartX)), -128, 128)
  const nextOffsetY = clamp(Math.round(dragStartOffsetY + (event.clientY - dragStartY)), -128, 128)
  applyPatch({
    enabled: true,
    settings: {
      offsetX: nextOffsetX,
      offsetY: nextOffsetY,
    },
  })
}

const handlePointerUp = (event: PointerEvent) => {
  if (dragPointerId !== event.pointerId) {
    return
  }
  stopDragging()
}

const handlePreviewPointerDown = (event: PointerEvent) => {
  if (!normalizedDecoration.value.resourceAttachmentId) {
    return
  }
  if (event.pointerType === 'mouse' && event.button !== 0) {
    return
  }
  dragPointerId = event.pointerId
  dragStartX = event.clientX
  dragStartY = event.clientY
  dragStartOffsetX = normalizedDecoration.value.settings?.offsetX ?? 0
  dragStartOffsetY = normalizedDecoration.value.settings?.offsetY ?? 0
  dragging.value = true
  window.addEventListener('pointermove', handlePointerMove)
  window.addEventListener('pointerup', handlePointerUp)
  window.addEventListener('pointercancel', handlePointerUp)
}

onBeforeUnmount(() => {
  stopDragging()
})
</script>

<template>
  <div class="avatar-decoration-editor">
    <div class="avatar-decoration-editor__preview-card">
      <div class="avatar-decoration-editor__preview-head">
        <div class="avatar-decoration-editor__title">频道消息预览</div>
        <div class="avatar-decoration-editor__hint">
          仅作用于当前频道角色，并且只在频道消息头像中显示。可直接拖拽头像上的装饰层调整位置。
        </div>
      </div>
      <div class="avatar-decoration-editor__message">
        <div
          class="avatar-decoration-editor__avatar-stage"
          :class="{ 'is-draggable': !!normalizedDecoration.resourceAttachmentId, 'is-dragging': dragging }"
          @pointerdown="handlePreviewPointerDown"
        >
          <UserAvatarDecoration
            :src="avatarSrc"
            :size="68"
            :border="true"
            :fallback-text="fallbackText || previewName || '频道角色'"
            :use-text-fallback="!avatarSrc"
            :decoration="normalizedDecoration"
            :decoration-enabled="normalizedDecoration.enabled && !!normalizedDecoration.resourceAttachmentId"
          />
          <div v-if="normalizedDecoration.resourceAttachmentId" class="avatar-decoration-editor__drag-badge">
            {{ dragging ? '拖拽中' : '拖拽调整' }}
          </div>
        </div>
        <div class="avatar-decoration-editor__bubble">
          <div class="avatar-decoration-editor__bubble-name">{{ previewName || fallbackText || '频道角色预览' }}</div>
          <div class="avatar-decoration-editor__bubble-text">这条示例频道消息会实时反映你的头像装饰调整。</div>
        </div>
      </div>
    </div>

    <div class="avatar-decoration-editor__toolbar">
      <input
        ref="inputRef"
        class="avatar-decoration-editor__file"
        type="file"
        accept="image/png,image/webp"
        @change="handleFileChange"
      />
      <n-button size="small" :loading="uploading" @click="selectFile">上传装饰图</n-button>
      <n-button size="small" quaternary @click="resetDecoration">重置参数</n-button>
      <n-button size="small" quaternary type="warning" @click="clearDecoration">取消佩戴</n-button>
    </div>

    <n-form label-placement="top" size="small">
      <n-form-item label="资源附件 ID">
        <n-input
          :value="normalizedDecoration.resourceAttachmentId"
          placeholder="id:attachment_id"
          @update:value="updateResourceAttachmentId"
        />
      </n-form-item>

      <div class="avatar-decoration-editor__grid">
        <n-form-item label="缩放">
          <n-input-number
            :value="normalizedDecoration.settings?.scale ?? 1"
            :min="0.5"
            :max="1.5"
            :step="0.05"
            @update:value="updateScale"
          />
        </n-form-item>
        <n-form-item label="X 偏移">
          <n-input-number
            :value="normalizedDecoration.settings?.offsetX ?? 0"
            :min="-128"
            :max="128"
            :step="1"
            @update:value="updateOffsetX"
          />
        </n-form-item>
        <n-form-item label="Y 偏移">
          <n-input-number
            :value="normalizedDecoration.settings?.offsetY ?? 0"
            :min="-128"
            :max="128"
            :step="1"
            @update:value="updateOffsetY"
          />
        </n-form-item>
        <n-form-item label="旋转">
          <n-input-number
            :value="normalizedDecoration.settings?.rotation ?? 0"
            :min="0"
            :max="360"
            :step="1"
            @update:value="updateRotation"
          />
        </n-form-item>
        <n-form-item label="透明度">
          <n-input-number
            :value="normalizedDecoration.settings?.opacity ?? 1"
            :min="0"
            :max="1"
            :step="0.05"
            @update:value="updateOpacity"
          />
        </n-form-item>
        <n-form-item label="层级">
          <n-radio-group
            :value="normalizedDecoration.settings?.zIndex ?? 1"
            size="small"
            @update:value="updateZIndex"
          >
            <n-radio-button :value="-1">头像下方</n-radio-button>
            <n-radio-button :value="1">头像上方</n-radio-button>
          </n-radio-group>
        </n-form-item>
      </div>
    </n-form>
  </div>
</template>

<style scoped lang="scss">
.avatar-decoration-editor {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  width: 100%;
}

.avatar-decoration-editor__preview-card {
  display: flex;
  flex-direction: column;
  gap: 0.9rem;
  padding: 0.85rem;
  border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.25));
  border-radius: 0.85rem;
  background: color-mix(in srgb, var(--sc-bg-elevated, rgba(248, 250, 252, 0.92)) 94%, transparent);
}

.avatar-decoration-editor__preview-head {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.avatar-decoration-editor__title {
  font-size: 0.85rem;
  font-weight: 600;
}

.avatar-decoration-editor__hint {
  margin-top: 0.25rem;
  font-size: 0.75rem;
  color: var(--sc-text-secondary, #64748b);
  line-height: 1.5;
}

.avatar-decoration-editor__message {
  display: flex;
  align-items: flex-start;
  gap: 0.9rem;
}

.avatar-decoration-editor__avatar-stage {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 96px;
  min-width: 96px;
  height: 96px;
  border-radius: 1rem;
  background:
    linear-gradient(135deg, color-mix(in srgb, var(--sc-bg-input, #fff) 92%, transparent), color-mix(in srgb, var(--sc-bg-layer, #f8fafc) 88%, transparent)),
    radial-gradient(circle at 30% 20%, color-mix(in srgb, var(--primary-color, #3388de) 16%, transparent), transparent 58%);
  border: 1px dashed color-mix(in srgb, var(--sc-border-strong, rgba(148, 163, 184, 0.4)) 90%, transparent);
}

.avatar-decoration-editor__avatar-stage.is-draggable {
  cursor: grab;
}

.avatar-decoration-editor__avatar-stage.is-dragging {
  cursor: grabbing;
}

.avatar-decoration-editor__drag-badge {
  position: absolute;
  left: 50%;
  bottom: 0.35rem;
  transform: translateX(-50%);
  padding: 0.1rem 0.45rem;
  border-radius: 999px;
  font-size: 0.68rem;
  color: var(--sc-text-secondary, #475569);
  background: color-mix(in srgb, var(--sc-bg-elevated, #fff) 92%, transparent);
  border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.18));
  pointer-events: none;
}

.avatar-decoration-editor__bubble {
  flex: 1;
  min-width: 0;
  padding: 0.8rem 0.9rem;
  border-radius: var(--chat-message-radius, 1rem);
  background: var(--chat-preview-bg, rgba(255, 255, 255, 0.9));
  border: 1px solid var(--chat-bubble-border, rgba(148, 163, 184, 0.18));
  color: var(--chat-text-primary, var(--sc-text-primary, #0f172a));
  box-shadow: var(--chat-message-shadow, none);
}

.avatar-decoration-editor__bubble-name {
  font-size: 0.88rem;
  font-weight: 600;
  color: inherit;
}

.avatar-decoration-editor__bubble-text {
  margin-top: 0.3rem;
  font-size: 0.8rem;
  line-height: 1.6;
  color: var(--chat-text-secondary, var(--sc-text-secondary, #64748b));
}

.avatar-decoration-editor__toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
}

.avatar-decoration-editor__grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 0.75rem;
}

.avatar-decoration-editor__file {
  display: none;
}

@media (max-width: 640px) {
  .avatar-decoration-editor__message {
    flex-direction: column;
  }

  .avatar-decoration-editor__grid {
    grid-template-columns: 1fr;
  }
}
</style>
