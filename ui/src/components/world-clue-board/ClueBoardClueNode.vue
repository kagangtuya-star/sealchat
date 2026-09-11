<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NButton, NIcon } from 'naive-ui'
import { Edit, ExternalLink, Pin, Pinned, Photo, World } from '@vicons/tabler'
import type { BoardNodeLayout } from './boardTypes'
import { urlBase } from '@/stores/_config'

const props = defineProps<{
  node: BoardNodeLayout
  worldId: string
  selected?: boolean
  dimmed?: boolean
  filteredOut?: boolean
  interactionLocked?: boolean
}>()

const emit = defineEmits<{
  pointerdown: [event: PointerEvent]
  select: [value: { additive: boolean }]
  open: []
  edit: []
  togglePin: []
}>()

const summary = computed(() => props.node.summary)
const mediaState = ref<'image' | 'video' | 'failed'>('image')
const mediaURL = computed(() => {
  const item = summary.value
  if (item.imageAttachmentId) {
    return `${urlBase}/api/v1/worlds/${encodeURIComponent(props.worldId)}/clues/${encodeURIComponent(item.id)}/image?v=${encodeURIComponent(item.imageAttachmentId)}`
  }
  return item.imageUrl || ''
})
const showImage = computed(() => summary.value.kind === 'image' && !!mediaURL.value)
const iconLabel = computed(() => summary.value.kind === 'iframe' ? (summary.value.embedDomain || '网页线索') : summary.value.kind === 'image' ? '图片线索' : '文字线索')

watch(mediaURL, () => { mediaState.value = 'image' })

function onPointerdown(event: PointerEvent) {
  if (event.button !== 0) return
  emit('pointerdown', event)
}
</script>

<template>
  <article
    class="clue-board-node"
    :class="{ 'is-selected': selected, 'is-dimmed': dimmed, 'is-filtered': filteredOut, 'is-unread': summary.unread, 'is-locked': interactionLocked }"
    :style="{ width: `${node.width}px`, height: `${node.height}px` }"
    :data-clue-id="node.id"
    @pointerdown="onPointerdown"
    @click.stop="emit('select', { additive: $event.shiftKey })"
  >
    <div v-if="showImage && mediaState !== 'failed'" class="clue-board-node__media">
      <img v-if="mediaState === 'image'" :src="mediaURL" :alt="summary.title" loading="lazy" :referrerpolicy="summary.imageAttachmentId ? undefined : 'no-referrer'" @error="mediaState = 'video'" />
      <video v-else :src="mediaURL" muted playsinline preload="metadata" @error="mediaState = 'failed'" />
    </div>
    <div v-else class="clue-board-node__media clue-board-node__media--placeholder">
      <NIcon><component :is="summary.kind === 'iframe' ? World : Photo" /></NIcon>
      <span>{{ iconLabel }}</span>
    </div>
    <header class="clue-board-node__header">
      <span class="clue-board-node__unread" aria-label="未读" v-if="summary.unread" />
      <strong :title="summary.title">{{ summary.title || '未命名线索' }}</strong>
      <span class="clue-board-node__status" v-if="node.temporary">待整理</span>
    </header>
    <p>{{ summary.kind === 'iframe' ? (summary.embedDomain || '网页线索') : (summary.contentText || '暂无摘要') }}</p>
    <footer class="clue-board-node__actions" @pointerdown.stop>
      <NButton text size="tiny" @click.stop="emit('open')"><template #icon><NIcon><ExternalLink /></NIcon></template>打开</NButton>
      <NButton v-if="summary.effectiveAccess === 'edit'" text size="tiny" @click.stop="emit('edit')"><template #icon><NIcon><Edit /></NIcon></template>编辑</NButton>
      <NButton text size="tiny" :disabled="interactionLocked" :title="node.pinned ? '取消固定' : '固定'" @click.stop="emit('togglePin')"><template #icon><NIcon><component :is="node.pinned ? Pinned : Pin" /></NIcon></template></NButton>
    </footer>
  </article>
</template>

<style scoped>
.clue-board-node { position: relative; display: flex; min-width: 0; flex-direction: column; overflow: hidden; color: var(--sc-text-primary); border: 1px solid var(--sc-border-mute); border-radius: 10px; background: color-mix(in srgb, var(--sc-bg-elevated) 96%, #3388de 3%); box-shadow: 0 8px 22px #0002; cursor: grab; user-select: none; transition: opacity .16s ease, border-color .16s ease, box-shadow .16s ease; }
.clue-board-node:active { cursor: grabbing; }
.clue-board-node.is-locked, .clue-board-node.is-locked:active { cursor: default; }
.clue-board-node.is-selected { border-color: var(--primary-color, #3388de); box-shadow: 0 0 0 2px color-mix(in srgb, var(--primary-color, #3388de) 30%, transparent), 0 8px 24px #0003; }
.clue-board-node.is-dimmed { opacity: .38; }
.clue-board-node.is-filtered { display: none; }
.clue-board-node.is-unread { border-left: 3px solid var(--primary-color, #3388de); }
.clue-board-node__media { display: flex; width: 100%; height: 84px; flex: 0 0 84px; align-items: center; justify-content: center; overflow: hidden; color: var(--sc-text-secondary); background: color-mix(in srgb, var(--sc-bg-surface) 86%, #000 14%); font-size: 11px; gap: 5px; }
.clue-board-node__media img, .clue-board-node__media video { width: 100%; height: 100%; object-fit: contain; }
.clue-board-node__header { display: flex; min-width: 0; align-items: center; gap: 5px; padding: 8px 10px 2px; }
.clue-board-node__header strong { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.clue-board-node__unread { width: 7px; height: 7px; flex: 0 0 7px; border-radius: 50%; background: var(--primary-color, #3388de); }
.clue-board-node__status { margin-left: auto; color: var(--sc-text-secondary); font-size: 10px; }
.clue-board-node p { display: -webkit-box; min-height: 30px; margin: 2px 10px 0; overflow: hidden; color: var(--sc-text-secondary); font-size: 11px; line-height: 1.4; -webkit-box-orient: vertical; -webkit-line-clamp: 2; }
.clue-board-node__actions { display: flex; align-items: center; gap: 2px; margin-top: auto; padding: 3px 6px; border-top: 1px solid var(--sc-border-mute); }
</style>
