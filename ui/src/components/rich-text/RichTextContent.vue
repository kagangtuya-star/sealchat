<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import DOMPurify from 'dompurify'
import TwinLayerMessage from '@/components/chat/TwinLayerMessage.vue'
import { preloadPlatformFontsFromDom } from '@/services/font/platformFontRegistry'
import { urlBase } from '@/stores/_config'
import { hasPerformanceContent } from '@/utils/tiptap-performance-parser'
import { isTipTapJson, tiptapJsonToHtml } from '@/utils/tiptap-render'

const props = withDefaults(defineProps<{
  content: string
  autoplay?: boolean
  baseUrl?: string
  imageClass?: string
  linkClass?: string
  attachmentResolver?: (src: string) => string
}>(), {
  content: '',
  autoplay: false,
  baseUrl: '',
  imageClass: 'rich-text-content__image',
  linkClass: 'rich-text-content__link',
})

const rootRef = ref<HTMLElement | null>(null)
const twinLayerRef = ref<InstanceType<typeof TwinLayerMessage> | null>(null)
const emit = defineEmits<{
  (event: 'state-change', value: { waiting: boolean; playing: boolean; completed: boolean }): void
}>()
const rich = computed(() => isTipTapJson(props.content))
const performance = computed(() => {
  if (!rich.value) return false
  try {
    return hasPerformanceContent(JSON.parse(props.content))
  } catch {
    return false
  }
})
const resolvedBaseUrl = computed(() => props.baseUrl || urlBase)
const html = computed(() => rich.value
  ? DOMPurify.sanitize(tiptapJsonToHtml(props.content, {
      baseUrl: resolvedBaseUrl.value,
      imageClass: props.imageClass,
      linkClass: props.linkClass,
    }))
  : '')

const preloadFonts = async () => {
  await nextTick()
  await preloadPlatformFontsFromDom(rootRef.value)
}

const handleClick = (event: MouseEvent) => {
  const target = event.target instanceof Element ? event.target : null
  const spoiler = target?.closest<HTMLElement>('.tiptap-spoiler, .tiptap-ruby[data-ruby-spoiler="true"]')
  if (!spoiler) return
  spoiler.classList.toggle('is-revealed')
}

watch(() => props.content, () => { void preloadFonts() })
onMounted(() => { void preloadFonts() })

defineExpose({
  skip: () => twinLayerRef.value?.skip(),
})
</script>

<template>
  <div ref="rootRef" class="rich-text-content" @click="handleClick">
    <TwinLayerMessage
      v-if="performance"
      ref="twinLayerRef"
      :content="props.content"
      :autoplay="props.autoplay"
      :base-url="resolvedBaseUrl"
      :image-class="props.imageClass"
      :link-class="props.linkClass"
      :attachment-resolver="props.attachmentResolver"
      @state-change="emit('state-change', $event)"
    />
    <div v-else-if="rich" class="rich-text-content__body" v-html="html"></div>
    <div v-else class="rich-text-content__plain">{{ props.content }}</div>
  </div>
</template>

<style scoped>
.rich-text-content {
  width: 100%;
  min-width: 0;
  color: inherit;
  font: inherit;
  line-height: 1.45;
  overflow-wrap: anywhere;
}

.rich-text-content__plain {
  white-space: pre-wrap;
}

.rich-text-content :deep(p),
.rich-text-content :deep(h1),
.rich-text-content :deep(h2),
.rich-text-content :deep(h3),
.rich-text-content :deep(blockquote),
.rich-text-content :deep(pre),
.rich-text-content :deep(ul),
.rich-text-content :deep(ol) {
  margin: 0 0 0.55em;
}

.rich-text-content :deep(p:last-child),
.rich-text-content :deep(h1:last-child),
.rich-text-content :deep(h2:last-child),
.rich-text-content :deep(h3:last-child),
.rich-text-content :deep(blockquote:last-child),
.rich-text-content :deep(pre:last-child),
.rich-text-content :deep(ul:last-child),
.rich-text-content :deep(ol:last-child) {
  margin-bottom: 0;
}

.rich-text-content :deep(h1) { font-size: 1.75em; line-height: 1.2; }
.rich-text-content :deep(h2) { font-size: 1.5em; line-height: 1.25; }
.rich-text-content :deep(h3) { font-size: 1.25em; line-height: 1.3; }

.rich-text-content :deep(ul),
.rich-text-content :deep(ol) {
  padding-left: 1.5em;
}

.rich-text-content :deep(blockquote) {
  padding-left: 0.8em;
  border-left: 3px solid currentColor;
  opacity: 0.82;
}

.rich-text-content :deep(code) {
  padding: 0.08em 0.28em;
  border-radius: 3px;
  background: rgba(15, 23, 42, 0.46);
  font-family: var(--sc-code-font, ui-monospace, SFMono-Regular, Consolas, monospace);
  font-size: 0.9em;
}

.rich-text-content :deep(pre) {
  padding: 0.65em 0.8em;
  overflow: auto;
  border-radius: 5px;
  background: rgba(15, 23, 42, 0.68);
  white-space: pre-wrap;
}

.rich-text-content :deep(pre code) {
  padding: 0;
  background: transparent;
}

.rich-text-content :deep(img) {
  display: inline-block;
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
  vertical-align: middle;
}

.rich-text-content :deep(a) {
  color: #93c5fd;
  text-decoration: underline;
  text-underline-offset: 0.15em;
}

.rich-text-content :deep(.mention-capsule) {
  display: inline-flex;
  padding: 0 0.35em;
  border-radius: 4px;
  background: rgba(59, 130, 246, 0.18);
  color: #bfdbfe;
}

.rich-text-content :deep(.tiptap-spoiler),
.rich-text-content :deep(.tiptap-ruby[data-ruby-spoiler='true']) {
  border-radius: 3px;
  background: currentColor;
  cursor: pointer;
}

.rich-text-content :deep(.tiptap-spoiler:not(.is-revealed)),
.rich-text-content :deep(.tiptap-ruby[data-ruby-spoiler='true']:not(.is-revealed)) {
  color: transparent !important;
}

.rich-text-content :deep(.tiptap-spoiler.is-revealed),
.rich-text-content :deep(.tiptap-ruby[data-ruby-spoiler='true'].is-revealed) {
  background: rgba(255, 255, 255, 0.16);
}

.rich-text-content :deep(.tiptap-ruby) {
  ruby-align: center;
  font-family: var(--ruby-base-font-family, var(--ruby-font-family, inherit));
  font-size: var(--ruby-base-font-size, var(--ruby-font-size, inherit));
  color: var(--ruby-color, inherit);
  font-weight: var(--ruby-font-weight, inherit);
  font-style: var(--ruby-font-style, inherit);
  background-color: var(--ruby-background-color, transparent);
}

.rich-text-content :deep(.tiptap-ruby rt) {
  font-family: var(--ruby-rt-font-family, var(--ruby-font-family, inherit));
  font-size: var(--ruby-rt-font-size, 0.58em);
  color: var(--ruby-color, inherit);
}
</style>
