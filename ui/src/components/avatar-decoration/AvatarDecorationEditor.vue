<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { nanoid } from 'nanoid'
import { api } from '@/stores/_config'
import { useUserStore } from '@/stores/user'
import { useUtilsStore } from '@/stores/utils'
import { fetchAttachmentMetaById } from '@/composables/useAttachmentResolver'
import type { AttachmentMeta } from '@/composables/useAttachmentResolver'
import type { AvatarDecoration } from '@/types'
import UserAvatarDecoration from '@/components/user-avatar-decoration.vue'
import { normalizeAvatarDecorations } from '@/utils/avatarDecorations'

const props = defineProps<{
  modelValue?: AvatarDecoration[] | null
  avatarSrc?: string
  fallbackText?: string
  previewName?: string
  uploadChannelId?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: AvatarDecoration[] | null): void
}>()

const user = useUserStore()
const utils = useUtilsStore()
const message = useMessage()
const resourceInputRef = ref<HTMLInputElement | null>(null)
const fallbackInputRef = ref<HTMLInputElement | null>(null)
const uploadingTarget = ref<'resource' | 'fallback' | null>(null)
const dragging = ref(false)
const selectedDecorationId = ref('')
const selectedResourceMeta = ref<AttachmentMeta | null>(null)
const selectedFallbackMeta = ref<AttachmentMeta | null>(null)

let dragPointerId: number | null = null
let dragStartX = 0
let dragStartY = 0
let dragStartOffsetX = 0
let dragStartOffsetY = 0

const defaultSettings = () => ({
  scale: 1,
  offsetX: 0,
  offsetY: 0,
  rotation: 0,
  zIndex: 1,
  opacity: 1,
  blendMode: 'normal',
})

const createDecoration = (): AvatarDecoration => ({
  id: nanoid(),
  enabled: true,
  decorationId: '',
  resourceAttachmentId: '',
  fallbackAttachmentId: '',
  settings: defaultSettings(),
})

const cloneDecoration = (item: AvatarDecoration): AvatarDecoration => ({
  ...item,
  settings: item.settings ? { ...item.settings } : undefined,
})

const normalizedDecorations = computed<AvatarDecoration[]>(() => (
  normalizeAvatarDecorations(props.modelValue).map(cloneDecoration)
))

const updateDecorations = (next: AvatarDecoration[]) => {
  emit('update:modelValue', next.length ? next : [])
}

watch(normalizedDecorations, (list) => {
  if (!list.length) {
    selectedDecorationId.value = ''
    selectedResourceMeta.value = null
    selectedFallbackMeta.value = null
    stopDragging()
    return
  }
  if (!selectedDecorationId.value || !list.some(item => item.id === selectedDecorationId.value)) {
    selectedDecorationId.value = list[0].id || ''
  }
}, { immediate: true, deep: true })

const selectedDecoration = computed<AvatarDecoration | null>(() => (
  normalizedDecorations.value.find(item => item.id === selectedDecorationId.value) || null
))

const selectedIndex = computed(() => normalizedDecorations.value.findIndex(item => item.id === selectedDecorationId.value))

const selectedDecorationLabel = computed(() => (
  selectedIndex.value >= 0 ? `装饰 ${selectedIndex.value + 1}` : '未选择装饰'
))

const selectedDecorationKind = computed(() => {
  const mime = String(selectedResourceMeta.value?.mimeType || '').toLowerCase()
  if (mime === 'video/webm') {
    return 'WEBM'
  }
  if (mime === 'image/png' || mime === 'image/webp') {
    return 'IMG'
  }
  return ''
})

watch(
  () => [selectedDecoration.value?.resourceAttachmentId, selectedDecoration.value?.fallbackAttachmentId],
  async ([resourceAttachmentId, fallbackAttachmentId]) => {
    selectedResourceMeta.value = resourceAttachmentId ? await fetchAttachmentMetaById(resourceAttachmentId) : null
    selectedFallbackMeta.value = fallbackAttachmentId ? await fetchAttachmentMetaById(fallbackAttachmentId) : null
  },
  { immediate: true },
)

const normalizedSelectedDecoration = computed<AvatarDecoration | null>(() => {
  if (!selectedDecoration.value) {
    return null
  }
  return {
    id: selectedDecoration.value.id,
    enabled: selectedDecoration.value.enabled === true,
    decorationId: selectedDecoration.value.decorationId || '',
    resourceAttachmentId: selectedDecoration.value.resourceAttachmentId || '',
    fallbackAttachmentId: selectedDecoration.value.fallbackAttachmentId || '',
    settings: {
      scale: selectedDecoration.value.settings?.scale ?? 1,
      offsetX: selectedDecoration.value.settings?.offsetX ?? 0,
      offsetY: selectedDecoration.value.settings?.offsetY ?? 0,
      rotation: selectedDecoration.value.settings?.rotation ?? 0,
      zIndex: selectedDecoration.value.settings?.zIndex ?? 1,
      opacity: selectedDecoration.value.settings?.opacity ?? 1,
      blendMode: selectedDecoration.value.settings?.blendMode ?? 'normal',
    },
  }
})

const clamp = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value))
const getStepPrecision = (step: number) => {
  const text = String(step)
  const index = text.indexOf('.')
  return index >= 0 ? text.length - index - 1 : 0
}
const clampStepValue = (value: number, min: number, max: number, step: number) => {
  const precision = getStepPrecision(step)
  const normalized = Math.round(value / step) * step
  return Number(clamp(normalized, min, max).toFixed(precision))
}

const patchDecoration = (decorationId: string, patch: Partial<AvatarDecoration>) => {
  const next = normalizedDecorations.value.map((item) => {
    if (item.id !== decorationId) {
      return item
    }
    const base = {
      ...item,
      settings: {
        ...defaultSettings(),
        ...(item.settings || {}),
      },
    }
    return {
      ...base,
      ...patch,
      settings: {
        ...base.settings,
        ...(patch.settings || {}),
      },
    }
  })
  updateDecorations(next)
}

const patchSelectedDecoration = (patch: Partial<AvatarDecoration>) => {
  const decorationId = selectedDecorationId.value
  if (!decorationId) {
    return
  }
  patchDecoration(decorationId, patch)
}

const addDecoration = () => {
  const nextDecoration = createDecoration()
  updateDecorations([...normalizedDecorations.value, nextDecoration])
  selectedDecorationId.value = nextDecoration.id || ''
}

const removeDecoration = (decorationId: string) => {
  const list = normalizedDecorations.value
  const targetIndex = list.findIndex(item => item.id === decorationId)
  if (targetIndex < 0) {
    return
  }
  const next = list.filter(item => item.id !== decorationId)
  updateDecorations(next)
  if (selectedDecorationId.value === decorationId) {
    selectedDecorationId.value = next[targetIndex]?.id || next[targetIndex - 1]?.id || ''
  }
}

const resetInput = (target: 'resource' | 'fallback') => {
  const input = target === 'resource' ? resourceInputRef.value : fallbackInputRef.value
  if (input) {
    input.value = ''
  }
}

const ensureSelectedDecoration = () => {
  if (selectedDecoration.value?.id) {
    return selectedDecoration.value.id
  }
  const nextDecoration = createDecoration()
  updateDecorations([...normalizedDecorations.value, nextDecoration])
  selectedDecorationId.value = nextDecoration.id || ''
  return nextDecoration.id || ''
}

const selectResourceFile = () => {
  ensureSelectedDecoration()
  resetInput('resource')
  resourceInputRef.value?.click()
}

const selectFallbackFile = () => {
  ensureSelectedDecoration()
  resetInput('fallback')
  fallbackInputRef.value?.click()
}

const uploadDecorationFile = async (file: File, target: 'resource' | 'fallback') => {
  const decorationId = ensureSelectedDecoration()
  if (!decorationId) {
    return
  }
  if (file.size > utils.fileSizeLimit) {
    const limitMB = (utils.fileSizeLimit / 1024 / 1024).toFixed(1)
    message.error(`文件大小超过限制（最大 ${limitMB} MB）`)
    return
  }
  const allowedMimeTypes = target === 'resource'
    ? ['image/png', 'image/webp', 'video/webm']
    : ['image/png', 'image/webp']
  if (!allowedMimeTypes.includes(file.type)) {
    message.error(target === 'resource'
      ? '头像装饰资源仅支持 PNG、WEBP 或 WEBM'
      : '静态兜底图仅支持 PNG 或 WEBP')
    return
  }

  uploadingTarget.value = target
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
    if (target === 'resource') {
      patchDecoration(decorationId, {
        enabled: true,
        resourceAttachmentId: `id:${attachmentId}`,
      })
      message.success('装饰资源上传成功')
      return
    }
    patchDecoration(decorationId, {
      fallbackAttachmentId: `id:${attachmentId}`,
    })
    message.success('静态兜底图上传成功')
  } catch (error) {
    message.error((target === 'resource' ? '装饰资源上传失败: ' : '静态兜底图上传失败: ') + String(error))
  } finally {
    uploadingTarget.value = null
  }
}

const handleResourceFileChange = async (event: Event) => {
  const target = event.target as HTMLInputElement | null
  const file = target?.files?.[0]
  if (!file) {
    return
  }
  await uploadDecorationFile(file, 'resource')
}

const handleFallbackFileChange = async (event: Event) => {
  const target = event.target as HTMLInputElement | null
  const file = target?.files?.[0]
  if (!file) {
    return
  }
  await uploadDecorationFile(file, 'fallback')
}

const resetSelectedDecoration = () => {
  if (!selectedDecoration.value?.id) {
    return
  }
  patchDecoration(selectedDecoration.value.id, {
    enabled: true,
    settings: defaultSettings(),
  })
}

const removeSelectedDecoration = () => {
  if (!selectedDecoration.value?.id) {
    return
  }
  removeDecoration(selectedDecoration.value.id)
}

const updateEnabled = (value: boolean) => {
  patchSelectedDecoration({
    enabled: value,
  })
}

const updateResourceAttachmentId = (value: string) => {
  patchSelectedDecoration({
    enabled: true,
    resourceAttachmentId: value.trim(),
  })
}

const updateFallbackAttachmentId = (value: string) => {
  patchSelectedDecoration({
    fallbackAttachmentId: value.trim(),
  })
}

const clearFallbackAttachment = () => {
  patchSelectedDecoration({
    fallbackAttachmentId: '',
  })
}

const updateScale = (value: number | null) => {
  patchSelectedDecoration({
    enabled: true,
    settings: { scale: value ?? 1 },
  })
}

const updateOffsetX = (value: number | null) => {
  patchSelectedDecoration({
    enabled: true,
    settings: { offsetX: value ?? 0 },
  })
}

const updateOffsetY = (value: number | null) => {
  patchSelectedDecoration({
    enabled: true,
    settings: { offsetY: value ?? 0 },
  })
}

const updateRotation = (value: number | null) => {
  patchSelectedDecoration({
    enabled: true,
    settings: { rotation: value ?? 0 },
  })
}

const updateOpacity = (value: number | null) => {
  patchSelectedDecoration({
    enabled: true,
    settings: { opacity: value ?? 1 },
  })
}

const updateZIndex = (value: number) => {
  patchSelectedDecoration({
    enabled: true,
    settings: { zIndex: value },
  })
}

const adjustScaleByWheel = (event: WheelEvent) => {
  event.preventDefault()
  const current = normalizedSelectedDecoration.value?.settings?.scale ?? 1
  updateScale(clampStepValue(current + (event.deltaY < 0 ? 0.05 : -0.05), 0.5, 1.5, 0.05))
}

const adjustOffsetXByWheel = (event: WheelEvent) => {
  event.preventDefault()
  const current = normalizedSelectedDecoration.value?.settings?.offsetX ?? 0
  updateOffsetX(clampStepValue(current + (event.deltaY < 0 ? 1 : -1), -128, 128, 1))
}

const adjustOffsetYByWheel = (event: WheelEvent) => {
  event.preventDefault()
  const current = normalizedSelectedDecoration.value?.settings?.offsetY ?? 0
  updateOffsetY(clampStepValue(current + (event.deltaY < 0 ? 1 : -1), -128, 128, 1))
}

const adjustRotationByWheel = (event: WheelEvent) => {
  event.preventDefault()
  const current = normalizedSelectedDecoration.value?.settings?.rotation ?? 0
  updateRotation(clampStepValue(current + (event.deltaY < 0 ? 1 : -1), 0, 360, 1))
}

const adjustOpacityByWheel = (event: WheelEvent) => {
  event.preventDefault()
  const current = normalizedSelectedDecoration.value?.settings?.opacity ?? 1
  updateOpacity(clampStepValue(current + (event.deltaY < 0 ? 0.05 : -0.05), 0, 1, 0.05))
}

const stopDragging = () => {
  dragging.value = false
  dragPointerId = null
  window.removeEventListener('pointermove', handlePointerMove)
  window.removeEventListener('pointerup', handlePointerUp)
  window.removeEventListener('pointercancel', handlePointerUp)
}

const handlePointerMove = (event: PointerEvent) => {
  if (!dragging.value || dragPointerId !== event.pointerId || !selectedDecoration.value) {
    return
  }
  const nextOffsetX = clamp(Math.round(dragStartOffsetX + (event.clientX - dragStartX)), -128, 128)
  const nextOffsetY = clamp(Math.round(dragStartOffsetY + (event.clientY - dragStartY)), -128, 128)
  patchSelectedDecoration({
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
  if (!normalizedSelectedDecoration.value?.resourceAttachmentId) {
    return
  }
  if (event.pointerType === 'mouse' && event.button !== 0) {
    return
  }
  dragPointerId = event.pointerId
  dragStartX = event.clientX
  dragStartY = event.clientY
  dragStartOffsetX = normalizedSelectedDecoration.value.settings?.offsetX ?? 0
  dragStartOffsetY = normalizedSelectedDecoration.value.settings?.offsetY ?? 0
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
          仅作用于当前频道角色，并且只在频道消息头像中显示。支持 PNG、WEBP 与透明 WEBM；全部装饰会一起预览，当前选中层可直接拖拽调整。
        </div>
      </div>
      <div class="avatar-decoration-editor__message">
        <div
          class="avatar-decoration-editor__avatar-stage"
          :class="{ 'is-draggable': !!normalizedSelectedDecoration?.resourceAttachmentId, 'is-dragging': dragging }"
          @pointerdown="handlePreviewPointerDown"
        >
          <UserAvatarDecoration
            :src="avatarSrc"
            :size="68"
            :border="true"
            :fallback-text="fallbackText || previewName || '频道角色'"
            :use-text-fallback="!avatarSrc"
            :decorations="normalizedDecorations"
            :active-decoration-id="selectedDecorationId"
            :highlight-active-decoration="true"
            :pause-when-out-of-view="false"
          />
          <div v-if="normalizedSelectedDecoration?.resourceAttachmentId" class="avatar-decoration-editor__drag-badge">
            {{ dragging ? '拖拽中' : `拖拽调整 ${selectedDecorationLabel}` }}
          </div>
        </div>
        <div class="avatar-decoration-editor__bubble">
          <div class="avatar-decoration-editor__bubble-name">{{ previewName || fallbackText || '频道角色预览' }}</div>
          <div class="avatar-decoration-editor__bubble-text">这条示例频道消息会实时反映全部头像装饰；当前选中的装饰层会在预览里高亮。</div>
        </div>
      </div>
    </div>

    <div class="avatar-decoration-editor__asset-panel">
      <div class="avatar-decoration-editor__asset-head">
        <div>
          <div class="avatar-decoration-editor__title">装饰预览区</div>
          <div class="avatar-decoration-editor__asset-hint">点击某个装饰进行编辑；悬浮小图会显示红色删除按钮。</div>
        </div>
        <n-button size="small" type="primary" @click="addDecoration">新增装饰</n-button>
      </div>
      <div v-if="normalizedDecorations.length" class="avatar-decoration-editor__asset-list">
        <div
          v-for="item in normalizedDecorations"
          :key="item.id"
          class="avatar-decoration-editor__asset-item"
          :class="{ 'is-active': item.id === selectedDecorationId }"
          role="button"
          tabindex="0"
          @click="selectedDecorationId = item.id || ''"
        >
          <button
            type="button"
            class="avatar-decoration-editor__asset-delete"
            aria-label="删除装饰"
            @click.stop="removeDecoration(item.id || '')"
          >
            ×
          </button>
          <UserAvatarDecoration
            :src="avatarSrc"
            :size="42"
            :border="false"
            :fallback-text="fallbackText || previewName || '频道角色'"
            :use-text-fallback="!avatarSrc"
            :decorations="[item]"
            :active-decoration-id="item.id || ''"
            :highlight-active-decoration="item.id === selectedDecorationId"
            :pause-when-out-of-view="false"
          />
          <div class="avatar-decoration-editor__asset-meta">
            <span>{{ item.settings?.zIndex === -1 ? '背景' : '前景' }}</span>
          </div>
        </div>
      </div>
      <n-empty v-else description="还没有头像装饰">
        <template #extra>
          <n-button size="small" type="primary" @click="addDecoration">创建第一个装饰</n-button>
        </template>
      </n-empty>
      <div v-if="selectedDecoration" class="avatar-decoration-editor__asset-status">
        <n-tag size="small" type="info">{{ selectedDecorationLabel }}</n-tag>
        <n-tag v-if="selectedDecorationKind" size="small" :type="selectedDecorationKind === 'WEBM' ? 'warning' : 'default'">
          {{ selectedDecorationKind }}
        </n-tag>
        <n-tag v-if="selectedFallbackMeta" size="small" type="success">有静态兜底</n-tag>
        <n-tag v-if="selectedDecoration.enabled" size="small" type="success">已启用</n-tag>
        <n-tag v-else size="small">已停用</n-tag>
      </div>
    </div>

    <div class="avatar-decoration-editor__toolbar">
      <input
        ref="resourceInputRef"
        class="avatar-decoration-editor__file"
        type="file"
        accept="image/png,image/webp,video/webm"
        @change="handleResourceFileChange"
      />
      <input
        ref="fallbackInputRef"
        class="avatar-decoration-editor__file"
        type="file"
        accept="image/png,image/webp"
        @change="handleFallbackFileChange"
      />
      <n-button size="small" :loading="uploadingTarget === 'resource'" @click="selectResourceFile">上传装饰资源</n-button>
      <n-button size="small" tertiary :loading="uploadingTarget === 'fallback'" :disabled="!selectedDecoration" @click="selectFallbackFile">上传静态兜底</n-button>
      <n-button size="small" quaternary :disabled="!selectedDecoration" @click="resetSelectedDecoration">重置当前参数</n-button>
      <n-button size="small" quaternary type="error" :disabled="!selectedDecoration" @click="removeSelectedDecoration">删除当前装饰</n-button>
    </div>

    <template v-if="normalizedSelectedDecoration">
      <n-form label-placement="top" size="small">
        <div class="avatar-decoration-editor__meta-grid">
          <n-form-item label="启用状态">
            <n-switch
              :value="normalizedSelectedDecoration.enabled"
              @update:value="updateEnabled"
            />
          </n-form-item>
          <n-form-item label="资源附件 ID">
            <n-input
              :value="normalizedSelectedDecoration.resourceAttachmentId"
              placeholder="id:attachment_id"
              @update:value="updateResourceAttachmentId"
            />
          </n-form-item>
          <n-form-item label="静态兜底附件 ID">
            <div class="avatar-decoration-editor__fallback-field">
              <n-input
                :value="normalizedSelectedDecoration.fallbackAttachmentId"
                placeholder="可选，id:attachment_id"
                @update:value="updateFallbackAttachmentId"
              />
              <n-button size="small" quaternary :disabled="!normalizedSelectedDecoration.fallbackAttachmentId" @click="clearFallbackAttachment">清除</n-button>
            </div>
          </n-form-item>
        </div>

        <div class="avatar-decoration-editor__grid">
          <n-form-item label="缩放">
            <div class="avatar-decoration-editor__wheel-field" @wheel.prevent="adjustScaleByWheel">
              <n-input-number
                :value="normalizedSelectedDecoration.settings?.scale ?? 1"
                :min="0.5"
                :max="1.5"
                :step="0.05"
                @update:value="updateScale"
              />
            </div>
          </n-form-item>
          <n-form-item label="X 偏移">
            <div class="avatar-decoration-editor__wheel-field" @wheel.prevent="adjustOffsetXByWheel">
              <n-input-number
                :value="normalizedSelectedDecoration.settings?.offsetX ?? 0"
                :min="-128"
                :max="128"
                :step="1"
                @update:value="updateOffsetX"
              />
            </div>
          </n-form-item>
          <n-form-item label="Y 偏移">
            <div class="avatar-decoration-editor__wheel-field" @wheel.prevent="adjustOffsetYByWheel">
              <n-input-number
                :value="normalizedSelectedDecoration.settings?.offsetY ?? 0"
                :min="-128"
                :max="128"
                :step="1"
                @update:value="updateOffsetY"
              />
            </div>
          </n-form-item>
          <n-form-item label="旋转">
            <div class="avatar-decoration-editor__slider-field" @wheel.prevent="adjustRotationByWheel">
              <n-slider
                :value="normalizedSelectedDecoration.settings?.rotation ?? 0"
                :min="0"
                :max="360"
                :step="1"
                :tooltip="false"
                @update:value="updateRotation"
              />
              <span class="avatar-decoration-editor__slider-value">{{ Math.round(normalizedSelectedDecoration.settings?.rotation ?? 0) }}°</span>
            </div>
          </n-form-item>
          <n-form-item label="透明度">
            <div class="avatar-decoration-editor__slider-field" @wheel.prevent="adjustOpacityByWheel">
              <n-slider
                :value="normalizedSelectedDecoration.settings?.opacity ?? 1"
                :min="0"
                :max="1"
                :step="0.05"
                :tooltip="false"
                @update:value="updateOpacity"
              />
              <span class="avatar-decoration-editor__slider-value">{{ Math.round((normalizedSelectedDecoration.settings?.opacity ?? 1) * 100) }}%</span>
            </div>
          </n-form-item>
          <n-form-item label="层级">
            <n-radio-group
              :value="normalizedSelectedDecoration.settings?.zIndex ?? 1"
              size="small"
              @update:value="updateZIndex"
            >
              <n-radio-button :value="-1">头像下方</n-radio-button>
              <n-radio-button :value="1">头像上方</n-radio-button>
            </n-radio-group>
          </n-form-item>
        </div>
      </n-form>
    </template>
    <n-empty v-else description="请先从上方新增或选择一个装饰后再编辑参数" />
  </div>
</template>

<style scoped lang="scss">
.avatar-decoration-editor {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  width: 100%;
}

.avatar-decoration-editor__preview-card,
.avatar-decoration-editor__asset-panel {
  display: flex;
  flex-direction: column;
  gap: 0.9rem;
  padding: 0.85rem;
  border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.25));
  border-radius: 0.85rem;
  background: color-mix(in srgb, var(--sc-bg-elevated, rgba(248, 250, 252, 0.92)) 94%, transparent);
}

.avatar-decoration-editor__preview-head,
.avatar-decoration-editor__asset-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
}

.avatar-decoration-editor__title {
  font-size: 0.85rem;
  font-weight: 600;
}

.avatar-decoration-editor__hint,
.avatar-decoration-editor__asset-hint {
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

.avatar-decoration-editor__asset-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(84px, 1fr));
  gap: 0.75rem;
}

.avatar-decoration-editor__asset-item {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.35rem;
  padding: 0.55rem 0.45rem 0.45rem;
  border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.24));
  border-radius: 0.85rem;
  background: color-mix(in srgb, var(--sc-bg-input, #fff) 94%, transparent);
  cursor: pointer;
  transition: border-color 0.2s ease, transform 0.2s ease, box-shadow 0.2s ease;
}

.avatar-decoration-editor__asset-item:hover {
  transform: translateY(-1px);
  border-color: color-mix(in srgb, var(--primary-color, #3388de) 42%, var(--sc-border-mute, rgba(148, 163, 184, 0.24)));
}

.avatar-decoration-editor__asset-item.is-active {
  border-color: color-mix(in srgb, var(--primary-color, #3388de) 72%, white);
  box-shadow: 0 0 0 1px color-mix(in srgb, var(--primary-color, #3388de) 22%, transparent);
}

.avatar-decoration-editor__asset-meta {
  font-size: 0.72rem;
  color: var(--sc-text-secondary, #64748b);
}

.avatar-decoration-editor__asset-delete {
  position: absolute;
  top: 0.3rem;
  right: 0.3rem;
  width: 1.2rem;
  height: 1.2rem;
  border: none;
  border-radius: 999px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 0.9rem;
  line-height: 1;
  color: #fff;
  background: rgba(239, 68, 68, 0.92);
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.2s ease;
}

.avatar-decoration-editor__asset-item:hover .avatar-decoration-editor__asset-delete,
.avatar-decoration-editor__asset-item.is-active .avatar-decoration-editor__asset-delete {
  opacity: 1;
  pointer-events: auto;
}

.avatar-decoration-editor__asset-status {
  display: flex;
  flex-wrap: wrap;
  gap: 0.4rem;
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

.avatar-decoration-editor__meta-grid {
  display: grid;
  grid-template-columns: 180px minmax(0, 1fr) minmax(0, 1fr);
  gap: 0.75rem;
}

.avatar-decoration-editor__fallback-field {
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.avatar-decoration-editor__fallback-field :deep(.n-input) {
  flex: 1;
}

.avatar-decoration-editor__wheel-field,
.avatar-decoration-editor__wheel-field :deep(.n-input-number) {
  width: 100%;
}

.avatar-decoration-editor__slider-field {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  width: 100%;
}

.avatar-decoration-editor__slider-field :deep(.n-slider) {
  flex: 1;
}

.avatar-decoration-editor__slider-value {
  min-width: 3rem;
  text-align: right;
  font-size: 0.78rem;
  color: var(--sc-text-secondary, #94a3b8);
}

.avatar-decoration-editor__file {
  display: none;
}

@media (max-width: 640px) {
  .avatar-decoration-editor__preview-head,
  .avatar-decoration-editor__asset-head,
  .avatar-decoration-editor__message {
    flex-direction: column;
  }

  .avatar-decoration-editor__meta-grid,
  .avatar-decoration-editor__grid {
    grid-template-columns: 1fr;
  }
}
</style>
