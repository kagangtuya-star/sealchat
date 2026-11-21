<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import ChatItem from '@/views/chat/components/chat-item.vue'
import { useDisplayStore } from '@/stores/display'
import { CHAT_SKIN_ORNAMENTS } from '../chatSkinOrnaments'
import type { ChatSkin, ChatSkinLayout } from '../chatSkinSchema'

// 实时预览：直接渲染真实的 chat-item（只读），继承当前主题与自定义样式；
// 示例覆盖场内、引用、长文、图片、场外与自己发言。主示例上的装饰图层可直接拖动定位。
const props = defineProps<{
  skin: ChatSkin | null
  layout: ChatSkinLayout
  name: string
  color: string
  avatarSrc: string
  avatarAttachmentId?: string
  selectedLayerId: string
  draggable: boolean
}>()

const emit = defineEmits<{
  (e: 'select-layer', id: string): void
  (e: 'drag-start', id: string): void
  (e: 'drag-move', id: string, dx: number, dy: number): void
  (e: 'drag-end', id: string): void
}>()

const display = useDisplayStore()
const rootRef = ref<HTMLElement | null>(null)

const nowIso = new Date().toISOString()
const sampleImage = computed(() => (
  props.avatarAttachmentId ? `id:${props.avatarAttachmentId.replace(/^id:/, '')}` : CHAT_SKIN_ORNAMENTS.find(item => item.id === 'flower')?.src || ''
))

const buildMessage = (id: string, content: string, options: { self?: boolean; ooc?: boolean; quote?: any } = {}) => ({
  id: `chat-skin-preview-${id}`,
  content,
  createdAt: nowIso,
  updatedAt: nowIso,
  icMode: options.ooc ? 'ooc' : 'ic',
  user: { id: options.self ? 'chat-skin-preview-self' : 'chat-skin-preview-user', nick: props.name, avatar: props.avatarSrc },
  member: { nick: props.name },
  quote: options.quote,
  identity: {
    id: 'chat-skin-preview-identity',
    displayName: props.name,
    color: props.color,
    avatarAttachment: '',
    chatSkin: props.skin,
  },
})

const samples = computed(() => {
  const first = buildMessage('main', '今晚的月色真美，要不要一起去河边走走？')
  return [
    { key: 'main', label: '场内', tone: 'ic' as const, self: false, item: first },
    { key: 'quote', label: '引用', tone: 'ic' as const, self: false, item: buildMessage('quote', '好呀，我带上灯笼。', { quote: first }) },
    {
      key: 'long',
      label: '长文',
      tone: 'ic' as const,
      self: false,
      item: buildMessage('long', '她推开窗，夜风把烛火吹得摇摇晃晃。远处传来钟声，一下、两下、三下……数到第十二下时，街角的灯忽然全灭了。\n「别出声。」她压低嗓音，「有人在看着我们。」'),
    },
    { key: 'image', label: '图片', tone: 'ic' as const, self: false, item: buildMessage('image', sampleImage.value ? `<img src="${sampleImage.value}"/>看，这是今天捡到的。` : '看，这是今天捡到的。') },
    { key: 'ooc', label: '场外', tone: 'ooc' as const, self: false, item: buildMessage('ooc', '（场外）我先去倒杯水，马上回来', { ooc: true }) },
    { key: 'self', label: '自己发言', tone: 'ic' as const, self: true, item: buildMessage('self', '那就出发吧！', { self: true }) },
  ]
})

interface Handle { id: string; left: number; top: number; width: number; height: number }
const handles = ref<Handle[]>([])

const measure = () => {
  const root = rootRef.value
  if (!root || !props.draggable) {
    handles.value = []
    return
  }
  const rootRect = root.getBoundingClientRect()
  const main = root.querySelector('[data-csk-sample="main"]')
  const nodes = main ? Array.from(main.querySelectorAll<HTMLElement>('.chat-skin-layer[data-layer-id]')) : []
  handles.value = nodes.map((node) => {
    const rect = node.getBoundingClientRect()
    return {
      id: node.dataset.layerId || '',
      left: rect.left - rootRect.left + root.scrollLeft,
      top: rect.top - rootRect.top + root.scrollTop,
      width: Math.max(14, rect.width),
      height: Math.max(14, rect.height),
    }
  }).filter(handle => handle.id)
}

let frame = 0
const scheduleMeasure = () => {
  if (frame) cancelAnimationFrame(frame)
  frame = requestAnimationFrame(() => {
    frame = 0
    measure()
  })
}

watch(() => [props.skin, props.layout, props.draggable], () => nextTick(scheduleMeasure), { deep: true })

let observer: ResizeObserver | null = null
onMounted(() => {
  nextTick(scheduleMeasure)
  if (typeof ResizeObserver !== 'undefined' && rootRef.value) {
    observer = new ResizeObserver(scheduleMeasure)
    observer.observe(rootRef.value)
  }
})
onBeforeUnmount(() => {
  observer?.disconnect()
  if (frame) cancelAnimationFrame(frame)
})

let drag: { id: string; x: number; y: number; pointerId: number } | null = null
const onHandleDown = (event: PointerEvent, id: string) => {
  emit('select-layer', id)
  drag = { id, x: event.clientX, y: event.clientY, pointerId: event.pointerId }
  ;(event.currentTarget as HTMLElement).setPointerCapture?.(event.pointerId)
  emit('drag-start', id)
  event.preventDefault()
}
const onHandleMove = (event: PointerEvent) => {
  if (!drag || event.pointerId !== drag.pointerId) return
  emit('drag-move', drag.id, Math.round(event.clientX - drag.x), Math.round(event.clientY - drag.y))
}
const onHandleUp = (event: PointerEvent) => {
  if (!drag || event.pointerId !== drag.pointerId) return
  emit('drag-end', drag.id)
  drag = null
}
</script>

<template>
  <div ref="rootRef" class="csk-preview" @scroll.passive="scheduleMeasure">
    <div
      class="chat csk-preview__chat"
      :class="[`chat--layout-${layout}`, `chat--palette-${display.palette}`]"
    >
      <div
        v-for="sample in samples"
        :key="sample.key"
        class="csk-preview__sample"
        :class="{ 'is-self': sample.self }"
        :data-csk-sample="sample.key"
      >
        <span class="csk-preview__tag">{{ sample.label }}</span>
        <ChatItem
          :avatar="avatarSrc"
          :username="name"
          :identity-color="color"
          :content="sample.item.content"
          :item="sample.item"
          :tone="sample.tone"
          :layout="layout"
          :is-self="sample.self"
          :show-avatar="true"
          :show-header="true"
          :readonly="true"
          :swipe-reply-disabled="true"
        />
      </div>
    </div>
    <div v-if="draggable" class="csk-preview__handles">
      <button
        v-for="handle in handles"
        :key="handle.id"
        type="button"
        class="csk-preview__handle"
        :class="{ 'is-selected': handle.id === selectedLayerId }"
        :style="{ left: `${handle.left}px`, top: `${handle.top}px`, width: `${handle.width}px`, height: `${handle.height}px` }"
        :aria-label="`拖动图层`"
        @pointerdown="onHandleDown($event, handle.id)"
        @pointermove="onHandleMove"
        @pointerup="onHandleUp"
        @pointercancel="onHandleUp"
      ></button>
    </div>
  </div>
</template>

<style scoped>
.csk-preview { position: relative; min-height: 0; height: 100%; overflow-x: hidden; overflow-y: auto; padding: 12px 12px 20px; background: var(--sc-bg-chat, var(--sc-bg-surface, transparent)); }
.csk-preview__chat { display: flex; flex-direction: column; gap: 6px; overflow: visible; height: auto; padding: 0; }
.csk-preview__sample { position: relative; padding-top: 14px; }
.csk-preview__tag { position: absolute; top: 0; left: 0; font-size: 10px; line-height: 12px; color: var(--sc-text-secondary, #94a3b8); pointer-events: none; }
.csk-preview__sample.is-self .csk-preview__tag { left: auto; right: 0; }
.csk-preview__sample :deep(.chat-item) { width: 100%; }
.csk-preview__handles { position: absolute; inset: 0; pointer-events: none; }
.csk-preview__handle { position: absolute; padding: 0; border: 1px dashed rgba(99, 102, 241, .55); border-radius: 3px; background: transparent; cursor: move; pointer-events: auto; touch-action: none; }
.csk-preview__handle:hover { border-color: rgba(99, 102, 241, .9); background: rgba(99, 102, 241, .06); }
.csk-preview__handle.is-selected { border: 1.5px solid var(--primary-color, #6366f1); box-shadow: 0 0 0 2px rgba(99, 102, 241, .18); }
</style>
