<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import RichTextContent from '@/components/rich-text/RichTextContent.vue'
import { WORLD_UNIT_PX, type StageEntrancePlayback, type StageObject } from '../shared/stage-types'

defineOptions({ name: 'StageTextVisualObject' })

const props = defineProps<{
  object: StageObject
  objects: Record<string, StageObject>
  entrancePlaybacks: Record<string, StageEntrancePlayback>
  hiddenObjectIds: Set<string>
}>()

const contentRef = ref<HTMLElement | null>(null)
const plainTextFontSize = ref(28)
let contentResizeObserver: ResizeObserver | null = null
let fitFrame = 0

const isPlainText = computed(() => props.object.type === 'text'
  && props.object.metadata?.textEditorMode !== 'rich')

const children = computed(() => Object.values(props.objects)
  .filter((object) => (
    object.parentId === props.object.id
    && (object.visible || props.entrancePlaybacks[object.id]?.direction === 'exit')
  ))
  .sort((a, b) => a.transform.z - b.transform.z || a.transform.order - b.transform.order))

const entrancePlayback = computed(() => props.entrancePlaybacks[props.object.id])
const entranceClass = computed(() => {
  const playback = entrancePlayback.value
  if (!playback || playback.preset === 'none') return undefined
  return [`entrance-${playback.preset}`, playback.direction === 'enter' ? 'is-entering' : 'is-exiting']
})

const style = computed(() => {
  const transform = props.object.transform
  return {
    left: `${transform.x * WORLD_UNIT_PX}px`,
    top: `${transform.y * WORLD_UNIT_PX}px`,
    width: `${Math.max(0.5, transform.width) * WORLD_UNIT_PX}px`,
    height: `${Math.max(0.5, transform.height) * WORLD_UNIT_PX}px`,
    transform: `translate(-50%, -50%) rotate(${transform.rotation}deg) scale(${transform.scaleX}, ${transform.scaleY})`,
    visibility: props.hiddenObjectIds.has(props.object.id) ? 'hidden' : undefined,
    '--theater-text-entrance-duration': entrancePlayback.value ? `${entrancePlayback.value.durationMs}ms` : undefined,
  }
})

const contentStyle = computed(() => isPlainText.value
  ? { '--theater-text-font-size': `${plainTextFontSize.value}px` }
  : undefined)

const schedulePlainTextFit = () => {
  if (!isPlainText.value || fitFrame) return
  fitFrame = window.requestAnimationFrame(() => {
    fitFrame = 0
    const element = contentRef.value
    if (!element) return

    const width = element.clientWidth
    const height = element.clientHeight
    if (!width || !height) return

    // Keep default size at default bounds, then scale with resized area.
    // Reduce further only when wrapped content would overflow its frame.
    const defaultContentWidth = 7 * WORLD_UNIT_PX - 20
    const defaultContentHeight = 4.5 * WORLD_UNIT_PX - 20
    const contentWidth = Math.max(1, width - 20)
    const contentHeight = Math.max(1, height - 20)
    let size = Math.min(192, Math.max(6, 28 * Math.sqrt(
      (contentWidth * contentHeight) / (defaultContentWidth * defaultContentHeight),
    )))
    element.style.setProperty('--theater-text-font-size', `${size}px`)
    if (element.scrollHeight > height + 1) {
      let minimum = 6
      let maximum = size
      for (let attempt = 0; attempt < 8; attempt += 1) {
        const candidate = (minimum + maximum) / 2
        element.style.setProperty('--theater-text-font-size', `${candidate}px`)
        if (element.scrollHeight <= height + 1) minimum = candidate
        else maximum = candidate
      }
      size = minimum
      element.style.setProperty('--theater-text-font-size', `${size}px`)
    }
    plainTextFontSize.value = size
  })
}

watch(
  () => [
    isPlainText.value,
    props.object.text,
    props.object.transform.width,
    props.object.transform.height,
  ],
  () => nextTick(schedulePlainTextFit),
  { flush: 'post' },
)

onMounted(() => {
  contentResizeObserver = new ResizeObserver(schedulePlainTextFit)
  if (contentRef.value) contentResizeObserver.observe(contentRef.value)
  schedulePlainTextFit()
})

onBeforeUnmount(() => {
  contentResizeObserver?.disconnect()
  if (fitFrame) window.cancelAnimationFrame(fitFrame)
})
</script>

<template>
  <div class="theater-text-visual-object" :class="entranceClass" :style="style">
    <div
      v-if="props.object.type === 'text'"
      ref="contentRef"
      class="theater-text-visual-object__content"
      :class="{ 'is-plain-text': isPlainText }"
      :style="contentStyle"
    >
      <RichTextContent
        class="theater-text-visual-object__rich-text"
        :content="props.object.text || props.object.name"
        autoplay
      />
    </div>
    <StageTextVisualObject
      v-for="child in children"
      :key="`${child.id}:${props.entrancePlaybacks[child.id]?.token || 0}`"
      :object="child"
      :objects="props.objects"
      :entrance-playbacks="props.entrancePlaybacks"
      :hidden-object-ids="props.hiddenObjectIds"
    />
  </div>
</template>

<style scoped>
.theater-text-visual-object {
  position: absolute;
  transform-origin: center;
  pointer-events: none;
}

.theater-text-visual-object.is-entering.entrance-fade { animation: theater-text-fade-in var(--theater-text-entrance-duration) ease-out both; }
.theater-text-visual-object.is-exiting.entrance-fade { animation: theater-text-fade-out var(--theater-text-entrance-duration) ease-out both; }
.theater-text-visual-object.is-entering.entrance-slide { animation: theater-text-slide-in var(--theater-text-entrance-duration) ease-out both; }
.theater-text-visual-object.is-exiting.entrance-slide { animation: theater-text-slide-out var(--theater-text-entrance-duration) ease-out both; }
.theater-text-visual-object.is-entering.entrance-zoom { animation: theater-text-zoom-in var(--theater-text-entrance-duration) ease-out both; }
.theater-text-visual-object.is-exiting.entrance-zoom { animation: theater-text-zoom-out var(--theater-text-entrance-duration) ease-out both; }
.theater-text-visual-object.is-entering.entrance-mask { animation: theater-text-mask-in var(--theater-text-entrance-duration) ease-out both; }
.theater-text-visual-object.is-exiting.entrance-mask { animation: theater-text-mask-out var(--theater-text-entrance-duration) ease-out both; }

@keyframes theater-text-fade-in { from { opacity: 0; } to { opacity: 1; } }
@keyframes theater-text-fade-out { from { opacity: 1; } to { opacity: 0; } }
@keyframes theater-text-slide-in { from { opacity: 0; translate: 0 18px; } to { opacity: 1; translate: 0 0; } }
@keyframes theater-text-slide-out { from { opacity: 1; translate: 0 0; } to { opacity: 0; translate: 0 18px; } }
@keyframes theater-text-zoom-in { from { opacity: 0; scale: .92; } to { opacity: 1; scale: 1; } }
@keyframes theater-text-zoom-out { from { opacity: 1; scale: 1; } to { opacity: 0; scale: .92; } }
@keyframes theater-text-mask-in { from { clip-path: inset(0 100% 0 0); } to { clip-path: inset(0); } }
@keyframes theater-text-mask-out { from { clip-path: inset(0); } to { clip-path: inset(0 100% 0 0); } }

.theater-text-visual-object__content {
  width: 100%;
  height: 100%;
  box-sizing: border-box;
  padding: 10px;
  overflow: hidden;
  color: #fff;
  font-size: 28px;
  font-weight: 700;
  line-height: 1.3;
}

.theater-text-visual-object__content.is-plain-text {
  font-size: var(--theater-text-font-size, 28px);
}

.theater-text-visual-object__rich-text {
  min-width: 0;
}

.theater-text-visual-object__content :deep(a),
.theater-text-visual-object__content :deep(.tiptap-spoiler),
.theater-text-visual-object__content :deep(.tiptap-ruby[data-ruby-spoiler='true']) {
  pointer-events: auto;
}
</style>
