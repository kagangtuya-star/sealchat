<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import DOMPurify from 'dompurify'
import type { WorldClueDetail } from '@/stores/worldClue'
import { tiptapJsonToHtml } from '@/utils/tiptap-render'
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver'
import { urlBase } from '@/stores/_config'
import SandboxedUrlFrame from './SandboxedUrlFrame.vue'

const props = withDefaults(defineProps<{ clue: WorldClueDetail; interactive?: boolean; privateLabel?: string }>(), { interactive: true, privateLabel: '仅你可见' })
const sharedHtml = computed(() => props.clue.contentFormat === 'tiptap'
  ? DOMPurify.sanitize(tiptapJsonToHtml(props.clue.content, { attachmentResolver: resolveAttachmentUrl }))
  : '')
const privateHtml = computed(() => props.clue.privateContentFormat === 'tiptap'
  ? DOMPurify.sanitize(tiptapJsonToHtml(props.clue.privateContent || '', { attachmentResolver: resolveAttachmentUrl }))
  : '')
const mediaUrl = computed(() => props.clue.imageAttachmentId
	? `${urlBase}/api/v1/worlds/${encodeURIComponent(props.clue.worldId)}/clues/${encodeURIComponent(props.clue.id)}/image?v=${encodeURIComponent(props.clue.imageAttachmentId)}`
  : props.clue.imageUrl || '')
const videoFallbackUrl = ref('')
const mediaIsVideo = computed(() => {
  if (props.clue.presentation?.mediaType === 'video' || videoFallbackUrl.value === mediaUrl.value) return true
  try {
    return /\.(webm)$/i.test(new URL(mediaUrl.value).pathname)
  } catch {
    return false
  }
})
watch(mediaUrl, () => { videoFallbackUrl.value = '' })
function useVideoMedia() {
  videoFallbackUrl.value = mediaUrl.value
}
const layoutClass = computed(() => `world-clue-content--${props.clue.presentation?.mediaPlacement || 'left'}`)
</script>

<template>
  <article class="world-clue-content" :class="layoutClass" :style="{ '--clue-media-ratio': String(clue.presentation?.mediaRatio || .42) }">
    <div v-if="clue.kind === 'image' && mediaUrl" class="world-clue-content__media">
      <video v-if="mediaIsVideo" :src="mediaUrl" :style="{ objectFit: clue.presentation?.objectFit || 'contain' }" controls autoplay muted loop playsinline />
      <img v-else :src="mediaUrl" :alt="clue.title" :style="{ objectFit: clue.presentation?.objectFit || 'contain' }" loading="lazy" referrerpolicy="no-referrer" @error="useVideoMedia" />
    </div>
    <div v-else-if="clue.kind === 'iframe'" class="world-clue-content__media">
      <SandboxedUrlFrame v-if="interactive && clue.embedUrl" :url="clue.embedUrl" :title="clue.title" />
      <div v-else class="world-clue-content__iframe-placeholder">交互内容仅在打开线索后加载</div>
    </div>
    <div class="world-clue-content__body">
      <div v-if="clue.contentFormat === 'tiptap'" class="world-clue-content__rich tiptap-content" v-html="sharedHtml" />
      <p v-else class="world-clue-content__plain">{{ clue.content }}</p>
      <section v-if="clue.privateContent" class="world-clue-content__private">
        <div class="world-clue-content__private-label">{{ privateLabel }}</div>
        <div v-if="clue.privateContentFormat === 'tiptap'" class="world-clue-content__rich tiptap-content" v-html="privateHtml" />
        <p v-else class="world-clue-content__plain">{{ clue.privateContent }}</p>
      </section>
    </div>
  </article>
</template>

<style scoped>
.world-clue-content { display: grid; min-height: 0; gap: 22px; color: var(--sc-text, inherit); }
.world-clue-content--left { grid-template-columns: minmax(0, calc(var(--clue-media-ratio) * 100%)) minmax(0, 1fr); }
.world-clue-content--right { grid-template-columns: minmax(0, 1fr) minmax(0, calc(var(--clue-media-ratio) * 100%)); }
.world-clue-content--right .world-clue-content__media { order: 2; }
.world-clue-content--top, .world-clue-content--bottom { grid-template-columns: 1fr; }
.world-clue-content--bottom .world-clue-content__media { order: 2; }
.world-clue-content__media { min-width: 0; min-height: 260px; }
.world-clue-content__media img, .world-clue-content__media video { display: block; width: 100%; height: 100%; max-height: 70dvh; }
.world-clue-content__body { min-width: 0; overflow-wrap: anywhere; }
.world-clue-content__plain { margin: 0; white-space: pre-wrap; line-height: 1.7; }
.world-clue-content__rich :deep(p:first-child) { margin-top: 0; }
.world-clue-content__private { margin-top: 22px; padding: 14px 16px; border: 1px solid color-mix(in srgb, var(--primary-color, #3388de) 20%, transparent); border-left: 2px solid var(--primary-color, #3388de); border-radius: 7px; background: color-mix(in srgb, var(--primary-color, #3388de) 5%, transparent); }
.world-clue-content__private-label { display: inline-block; margin-bottom: 10px; padding: 2px 6px; border-radius: 4px; background: color-mix(in srgb, var(--primary-color, #3388de) 8%, transparent); font-size: 11px; font-weight: 500; color: var(--primary-color, #3388de); }
.world-clue-content__iframe-placeholder { display: grid; min-height: 260px; place-items: center; border: 1px dashed var(--sc-border, #8885); color: var(--sc-text-muted, #888); }
@media (max-width: 720px) {
  .world-clue-content--left, .world-clue-content--right { grid-template-columns: 1fr; }
  .world-clue-content--right .world-clue-content__media { order: 0; }
  .world-clue-content__media { min-height: 210px; }
}
</style>
