<template>
  <n-modal
    :show="show"
    preset="card"
    class="sticky-background-modal"
    :style="modalStyle"
    :content-style="{ maxHeight: 'min(70vh, 680px)', overflowY: 'auto' }"
    :bordered="false"
    @update:show="handleShow"
  >
    <template #header>
      <div class="sticky-background-modal__drag-handle" @pointerdown="startModalDrag">便签背景</div>
    </template>
    <div class="sticky-background-modal__layout">
      <section class="sticky-background-modal__controls">
        <div class="sticky-background-modal__section">
          <div class="sticky-background-modal__label">应用范围</div>
          <n-select
            :value="targetNoteId"
            clearable
            :options="noteOptions"
            placeholder="仅编辑世界默认"
            @update:value="$emit('update:targetNoteId', $event || '')"
          />
        </div>
        <div class="sticky-background-modal__section">
          <div class="sticky-background-modal__label">背景图片</div>
          <input ref="fileInput" class="sticky-background-modal__file" type="file" accept="image/png,image/jpeg,image/webp,image/avif" @change="handleFile" />
          <div class="sticky-background-modal__file-row">
            <button class="sticky-background-modal__choose" type="button" @click="fileInput?.click()">选择图片</button>
            <button v-if="draft.background || selectedFile" class="sticky-background-modal__clear" type="button" @click="clearImage">移除</button>
          </div>
          <div class="sticky-background-modal__hint">建议 16:10 以上图片，保存后在世界内复用。</div>
        </div>
        <div class="sticky-background-modal__section">
          <div class="sticky-background-modal__label">填充方式</div>
          <n-radio-group v-model:value="fit" size="small">
            <n-radio-button value="cover">裁切填充</n-radio-button>
            <n-radio-button value="contain">完整显示</n-radio-button>
            <n-radio-button value="stretch">拉伸</n-radio-button>
            <n-radio-button value="tile">平铺</n-radio-button>
          </n-radio-group>
        </div>
        <div class="sticky-background-modal__section">
          <div class="sticky-background-modal__label">图片透明度 <span>{{ Math.round(opacity * 100) }}%</span></div>
          <n-slider v-model:value="opacity" :min="0" :max="1" :step="0.01" />
        </div>
        <div class="sticky-background-modal__section">
          <div class="sticky-background-modal__label">内容衬底 <span>{{ Math.round(wash * 100) }}%</span></div>
          <n-slider v-model:value="wash" :min="0" :max="0.85" :step="0.01" />
        </div>
        <div class="sticky-background-modal__section" v-if="fit !== 'tile'">
          <div class="sticky-background-modal__label">图片焦点</div>
          <div class="sticky-background-modal__position">
            <n-slider v-model:value="positionX" :min="0" :max="100" :step="1" />
            <n-slider v-model:value="positionY" :min="0" :max="100" :step="1" />
          </div>
        </div>
      </section>
      <section class="sticky-background-modal__preview-wrap">
        <div class="sticky-background-modal__preview-caption">实时预览</div>
        <div class="sticky-background-modal__preview" :style="previewStyle">
          <div class="sticky-background-modal__preview-header">便签标题 <span>⋯</span></div>
          <div class="sticky-background-modal__preview-body">一段示例内容，用于检查图片透明度与文字可读性。</div>
          <div class="sticky-background-modal__preview-footer">编辑者 · 刚刚</div>
        </div>
      </section>
    </div>
    <template #footer>
      <div class="sticky-background-modal__footer">
        <button type="button" class="sticky-background-modal__secondary" @click="restoreDefault">恢复世界默认</button>
        <div class="sticky-background-modal__actions">
          <button type="button" class="sticky-background-modal__secondary" @click="handleShow(false)">取消</button>
          <button v-if="targetNoteId" type="button" class="sticky-background-modal__primary" :disabled="saving" @click="save('note')">{{ saving ? '保存中…' : '应用到便签' }}</button>
          <button type="button" class="sticky-background-modal__primary" :disabled="saving || !canSetWorldDefault" :title="canSetWorldDefault ? '设为世界默认' : '仅世界管理员可设置'" @click="save('world')">{{ saving ? '保存中…' : '设为世界默认' }}</button>
        </div>
      </div>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { uploadImageAttachment } from '@/views/chat/composables/useAttachmentUploader'
import { normalizeAttachmentId } from '@/composables/useAttachmentResolver'
import type { StickyNote, StickyNoteAppearance, StickyNoteAppearanceBackground } from '@/stores/stickyNote'

const props = defineProps<{
  show: boolean
  worldId: string
  channelId: string
  targetNoteId: string
  notes: StickyNote[]
  initialAppearance?: StickyNoteAppearance
  worldAppearance?: StickyNoteAppearance
  canSetWorldDefault: boolean
}>()
const emit = defineEmits<{
  (event: 'update:show', value: boolean): void
  (event: 'update:targetNoteId', value: string): void
  (event: 'apply-note', value: StickyNoteAppearance): void
  (event: 'apply-world', value: StickyNoteAppearance): void
}>()
const message = useMessage()

const fileInput = ref<HTMLInputElement | null>(null)
const selectedFile = ref<File | null>(null)
const localPreviewUrl = ref('')
const saving = ref(false)
const modalOffset = ref({ x: 0, y: 0 })
const dragStart = ref({ x: 0, y: 0, offsetX: 0, offsetY: 0 })
const dragging = ref(false)
const draft = ref<StickyNoteAppearance>({ version: 1 })
const fit = ref<StickyNoteAppearanceBackground['fit']>('cover')
const opacity = ref(0.72)
const wash = ref(0)
const positionX = ref(50)
const positionY = ref(50)

const noteOptions = computed(() => props.notes.map(note => ({ label: note.title || '无标题便签', value: note.id })))
const modalStyle = computed(() => ({
  width: 'min(760px, calc(100vw - 28px))',
  maxHeight: 'calc(100vh - 28px)',
  position: 'relative' as const,
  left: `${modalOffset.value.x}px`,
  top: `${modalOffset.value.y}px`,
}))
const imageId = computed(() => draft.value.background?.attachmentId || '')
const imageUrl = computed(() => {
  if (localPreviewUrl.value) return localPreviewUrl.value
  const id = normalizeAttachmentId(imageId.value)
  return id ? `/api/v1/attachment/${id}` : ''
})
const previewStyle = computed(() => ({
  '--bg-image': imageUrl.value ? `url("${imageUrl.value}")` : 'none',
  '--bg-opacity': String(opacity.value),
  '--bg-wash': String(wash.value),
  '--bg-size': fit.value === 'stretch' ? '100% 100%' : fit.value === 'tile' ? 'auto' : fit.value,
  '--bg-repeat': fit.value === 'tile' ? 'repeat' : 'no-repeat',
  '--bg-position': `${positionX.value}% ${positionY.value}%`,
}))

function loadAppearance(value?: StickyNoteAppearance) {
  if (localPreviewUrl.value) URL.revokeObjectURL(localPreviewUrl.value)
  const background = value?.background
  draft.value = background ? { version: 1, background: { ...background } } : { version: 1 }
  fit.value = background?.fit || 'cover'
  opacity.value = background?.opacity ?? 0.72
  wash.value = background?.contentWashOpacity ?? 0
  positionX.value = background?.positionX ?? 50
  positionY.value = background?.positionY ?? 50
  selectedFile.value = null
  if (fileInput.value) fileInput.value.value = ''
  localPreviewUrl.value = ''
}

watch(() => props.show, show => {
  if (show) {
    modalOffset.value = { x: 0, y: 0 }
    loadAppearance(props.initialAppearance)
  } else {
    stopModalDrag()
  }
})
watch(() => props.initialAppearance, value => { if (props.show) loadAppearance(value) })

function handleShow(value: boolean) {
  if (!value) stopModalDrag()
  emit('update:show', value)
}
function startModalDrag(event: PointerEvent) {
  if (event.button !== 0) return
  dragging.value = true
  dragStart.value = { x: event.clientX, y: event.clientY, offsetX: modalOffset.value.x, offsetY: modalOffset.value.y }
  document.addEventListener('pointermove', handleModalDrag)
  document.addEventListener('pointerup', stopModalDrag)
  event.preventDefault()
}
function handleModalDrag(event: PointerEvent) {
  if (!dragging.value) return
  const maxX = Math.max(0, window.innerWidth / 2 - 48)
  const maxY = Math.max(0, window.innerHeight / 2 - 48)
  modalOffset.value = {
    x: Math.min(maxX, Math.max(-maxX, dragStart.value.offsetX + event.clientX - dragStart.value.x)),
    y: Math.min(maxY, Math.max(-maxY, dragStart.value.offsetY + event.clientY - dragStart.value.y)),
  }
}
function stopModalDrag() {
  dragging.value = false
  document.removeEventListener('pointermove', handleModalDrag)
  document.removeEventListener('pointerup', stopModalDrag)
}
function handleFile(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (!file) return
  selectedFile.value = file
  if (localPreviewUrl.value) URL.revokeObjectURL(localPreviewUrl.value)
  localPreviewUrl.value = URL.createObjectURL(file)
}
function clearImage() {
  if (localPreviewUrl.value) URL.revokeObjectURL(localPreviewUrl.value)
  draft.value = { version: 1 }; selectedFile.value = null; localPreviewUrl.value = ''
  if (fileInput.value) fileInput.value.value = ''
}
function restoreDefault() { loadAppearance(props.worldAppearance) }

async function save(target: 'note' | 'world') {
  saving.value = true
  try {
    let attachmentId = imageId.value
    if (selectedFile.value) {
      const result = await uploadImageAttachment(selectedFile.value, {
        channelId: props.channelId,
        rootId: props.worldId,
        rootIdType: 'sticky_note_background',
        confirm: true,
      })
      attachmentId = result.attachmentId
    }
    const appearance: StickyNoteAppearance = { version: 1 }
    if (attachmentId) appearance.background = { kind: 'image', attachmentId, opacity: opacity.value, fit: fit.value, positionX: positionX.value, positionY: positionY.value, contentWashOpacity: wash.value }
    if (target === 'note') emit('apply-note', appearance)
    else emit('apply-world', appearance)
    emit('update:show', false)
  } catch (error: any) {
    message.error(error?.message || '背景图片保存失败')
  } finally { saving.value = false }
}

onUnmounted(() => {
  stopModalDrag()
  if (localPreviewUrl.value) URL.revokeObjectURL(localPreviewUrl.value)
})
</script>

<style scoped>
.sticky-background-modal__drag-handle { width: 100%; cursor: move; user-select: none; touch-action: none; }
.sticky-background-modal__layout { display: flex; flex-wrap: wrap; align-items: flex-start; gap: 24px; min-width: 0; }
.sticky-background-modal__controls { display: grid; flex: 1 1 340px; min-width: 0; gap: 16px; }
.sticky-background-modal__section { display: grid; gap: 8px; }
.sticky-background-modal__label { display: flex; justify-content: space-between; color: var(--sc-text-primary, #e2e8f0); font-size: 13px; font-weight: 600; }
.sticky-background-modal__hint { color: var(--sc-text-secondary, #94a3b8); font-size: 12px; line-height: 1.4; }
.sticky-background-modal__file { display: none; }
.sticky-background-modal__file-row, .sticky-background-modal__footer, .sticky-background-modal__actions { display: flex; align-items: center; gap: 8px; }
.sticky-background-modal__footer { justify-content: space-between; }
.sticky-background-modal__actions { justify-content: flex-end; }
.sticky-background-modal__choose, .sticky-background-modal__clear, .sticky-background-modal__primary, .sticky-background-modal__secondary { border: 1px solid rgba(148,163,184,.24); border-radius: 6px; min-height: 34px; padding: 0 12px; cursor: pointer; font-size: 12px; }
.sticky-background-modal__choose, .sticky-background-modal__primary { background: var(--sc-primary-color, #3b82f6); color: white; border-color: transparent; }
.sticky-background-modal__secondary, .sticky-background-modal__clear { background: transparent; color: var(--sc-text-secondary, #94a3b8); }
.sticky-background-modal__preview-wrap { display: grid; flex: 1 1 260px; min-width: 0; align-content: start; justify-items: center; gap: 8px; }
.sticky-background-modal__preview-caption { color: var(--sc-text-secondary, #94a3b8); font-size: 12px; }
.sticky-background-modal__preview { position: relative; isolation: isolate; overflow: hidden; display: flex; flex-direction: column; width: min(100%, 320px); height: 210px; border-radius: 8px; background: linear-gradient(135deg, #fff9c4, #fff59d); color: rgba(0,0,0,.76); box-shadow: 0 12px 28px rgba(0,0,0,.22); }
.sticky-background-modal__preview::before, .sticky-background-modal__preview::after { content: ''; position: absolute; inset: 0; pointer-events: none; }
.sticky-background-modal__preview::before { z-index: -2; background-image: var(--bg-image); background-size: var(--bg-size); background-repeat: var(--bg-repeat); background-position: var(--bg-position); opacity: var(--bg-opacity); }
.sticky-background-modal__preview::after { z-index: -1; background: rgba(255,255,255,var(--bg-wash)); }
.sticky-background-modal__preview-header, .sticky-background-modal__preview-body, .sticky-background-modal__preview-footer { position: relative; z-index: 1; }
.sticky-background-modal__preview-header { padding: 10px 12px; border-bottom: 1px solid rgba(0,0,0,.1); font-weight: 700; font-size: 13px; display: flex; justify-content: space-between; }
.sticky-background-modal__preview-body { flex: 1; padding: 14px; font-size: 13px; line-height: 1.6; }
.sticky-background-modal__preview-footer { padding: 8px 12px; border-top: 1px solid rgba(0,0,0,.08); font-size: 11px; opacity: .7; }
.sticky-background-modal__position { display: grid; gap: 8px; }
@media (max-width: 640px) { .sticky-background-modal__footer { align-items: stretch; flex-direction: column; } .sticky-background-modal__actions { flex-wrap: wrap; } }
</style>
