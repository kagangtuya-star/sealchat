<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch, type CSSProperties } from 'vue'
import { NButton, NIcon, NTooltip } from 'naive-ui'
import { PlayerSkipForward, X } from '@vicons/tabler'
import RichTextContent from '@/components/rich-text/RichTextContent.vue'
import TheaterPresentationMedia from '@/components/theater-presentation/TheaterPresentationMedia.vue'
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver'
import { createDefaultTheaterPresentation, resolveTheaterBackdropColor, resolveTheaterTransformStyle, type TheaterVisualLayer } from '@/types/theaterPresentation'
import { isTipTapJson } from '@/utils/tiptap-render'
import { hasPerformanceContent } from '@/utils/tiptap-performance-parser'
import type { ChatCharactersSnapshotPayload } from '../bridge/theater-bridge-protocol'
import {
  resolveTheaterDialoguePresentation,
  type TheaterDialogueRuntime,
  type TheaterDialogueRuntimeSnapshot,
} from './theater-dialogue-runtime'
import '@/components/theater-presentation/theaterComposition.css'
import { useTheaterAppearanceCache } from '@/composables/useTheaterAppearanceCache'

const props = defineProps<{
  runtime: TheaterDialogueRuntime
  characterSnapshot: ChatCharactersSnapshotPayload
  worldId: string
  channelId: string
}>()

const rootRef = ref<HTMLElement | null>(null)
const richTextRef = ref<InstanceType<typeof RichTextContent> | null>(null)
const visibleInViewport = ref(true)
const snapshot = ref<TheaterDialogueRuntimeSnapshot>(props.runtime.getSnapshot())
const livePresentation = ref<ReturnType<typeof resolveTheaterDialoguePresentation> | null>(null)
const appearanceCache = useTheaterAppearanceCache()
let unsubscribe: (() => void) | null = null
let intersectionObserver: IntersectionObserver | null = null
let motionQuery: MediaQueryList | null = null
let invalidateAppearance: ((event: Event) => void) | null = null

const current = computed(() => snapshot.value.queue.current)
const message = computed(() => current.value?.message || null)
const presentation = computed(() => livePresentation.value || resolveTheaterDialoguePresentation(message.value, props.characterSnapshot))
const dialogueStyle = computed<CSSProperties>(() => ({
  ...resolveTheaterTransformStyle(presentation.value.dialogue.transform),
}))
const dialogueControlsStyle = computed<CSSProperties>(() => ({ ...dialogueStyle.value, zIndex: '1000' }))
const portrait = computed(() => presentation.value.portrait?.enabled ? presentation.value.portrait : null)
const portraitStyle = computed<CSSProperties | undefined>(() => portrait.value
  ? { ...resolveTheaterTransformStyle(portrait.value.transform) }
  : undefined)
const portraitDecorations = computed(() => presentation.value.portraitDecorations
  .filter((layer) => layer.enabled)
  .sort((left, right) => left.transform.zIndex - right.transform.zIndex))
const frame = computed(() => presentation.value.dialogue.frame?.enabled ? presentation.value.dialogue.frame : null)
const narration = computed(() => presentation.value.narration)
const narrationStyle = computed<CSSProperties>(() => ({
  backgroundColor: resolveTheaterBackdropColor(
    narration.value.backdropColor,
    narration.value.backdropOpacity,
  ),
}))
const revealedText = computed(() => Array.from(message.value?.contentText || '')
  .slice(0, current.value?.revealedCharacters || 0)
  .join(''))
const typing = computed(() => snapshot.value.phase === 'typing')
const richContent = computed(() => {
  const content = message.value?.contentRichText || ''
  return content && isTipTapJson(content) ? content : ''
})
const useRichPlayback = computed(() => {
  if (!richContent.value) return false
  // Bridge payloads from older clients may omit optional flag; derive from document.
  return hasPerformanceContent(richContent.value)
})
const showRichContent = computed(() => Boolean(richContent.value && (!typing.value || useRichPlayback.value)))
const mediaActive = computed(() => Boolean(current.value && visibleInViewport.value))
const speakerColor = computed(() => {
  const color = String(message.value?.actor.color || '').trim()
  return typeof CSS !== 'undefined' && CSS.supports('color', color) ? color : 'var(--sc-text-primary, #f4f4f5)'
})
const textLayerStyle = (kind: 'speaker' | 'content'): CSSProperties => ({
  ...resolveTheaterTransformStyle(presentation.value.dialogue[kind].transform),
  display: presentation.value.dialogue[kind].enabled ? (kind === 'speaker' ? 'grid' : 'block') : 'none',
  textAlign: presentation.value.dialogue.textAlign,
  '--theater-font-scale': String(presentation.value.dialogue[kind].fontScale),
})
const contentStyle = computed<CSSProperties>(() => ({
  ...textLayerStyle('content'),
  color: presentation.value.dialogue.contentColor,
}))

watch(
  () => presentation.value.dialogue.charactersPerSecond,
  (speed) => props.runtime.setCharactersPerSecond(speed),
  { immediate: true },
)

const layerStyle = (layer: TheaterVisualLayer): CSSProperties => ({
  ...resolveTheaterTransformStyle(layer.transform),
  mixBlendMode: layer.blendMode,
})
const frameStyle = computed<CSSProperties | undefined>(() => frame.value
  ? { ...layerStyle(frame.value), zIndex: '1' }
  : undefined)

const completeCurrent = () => {
  if (!typing.value) return
  richTextRef.value?.skip()
  props.runtime.completeCurrent()
}

const skip = () => {
  richTextRef.value?.skip()
  props.runtime.skip()
}

const updateReducedMotion = () => props.runtime.setReducedMotion(Boolean(motionQuery?.matches))

onMounted(() => {
  unsubscribe = props.runtime.subscribe((value) => { snapshot.value = value })
  motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
  motionQuery.addEventListener('change', updateReducedMotion)
  updateReducedMotion()
  invalidateAppearance = (event: Event) => {
    const detail = (event as CustomEvent<{ channelId?: string }>).detail
    if (String(detail?.channelId || '').trim() !== String(props.channelId).trim()) return
    appearanceCache.invalidate(props.worldId, props.channelId)
    const actor = message.value?.actor
    if (!actor?.identityId) return
    livePresentation.value = null
    void appearanceCache.resolve(props.worldId, props.channelId, {
      identityId: actor.identityId,
      variantId: actor.variantId,
    }).then((resolved) => {
      livePresentation.value = resolved?.presentation || createDefaultTheaterPresentation()
    }).catch(() => undefined)
  }
  window.addEventListener('sealchat:theater-appearance-invalidated', invalidateAppearance)
  void nextTick(() => {
    if (!rootRef.value) return
    intersectionObserver = new IntersectionObserver(([entry]) => { visibleInViewport.value = entry.isIntersecting })
    intersectionObserver.observe(rootRef.value)
  })
})

watch(
  () => [props.worldId, props.channelId, message.value?.actor.identityId, message.value?.actor.variantId, message.value?.messageId] as const,
  async () => {
    livePresentation.value = null
    const actor = message.value?.actor
    if (!actor?.identityId) return
    try {
      const resolved = await appearanceCache.resolve(props.worldId, props.channelId, {
        identityId: actor.identityId,
        variantId: actor.variantId,
      })
      if (resolved) livePresentation.value = resolved.presentation || createDefaultTheaterPresentation()
    } catch {
      // Message snapshot remains usable when API unavailable.
    }
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  unsubscribe?.()
  intersectionObserver?.disconnect()
  motionQuery?.removeEventListener('change', updateReducedMotion)
  if (invalidateAppearance) window.removeEventListener('sealchat:theater-appearance-invalidated', invalidateAppearance)
})
</script>

<template>
  <div
    ref="rootRef"
    class="theater-dialogue-overlay theater-composition-host"
    :class="{ 'is-open': current, 'is-reduced-motion': snapshot.reducedMotion }"
    aria-live="polite"
  >
    <div v-if="current && narration.enabled" class="theater-dialogue-narration" :style="narrationStyle" />
    <div v-if="current" class="theater-composition">
      <div v-if="portrait && !narration.enabled" class="theater-dialogue-portrait" :style="portraitStyle">
        <TheaterPresentationMedia
          class="theater-dialogue-portrait__base"
          :media="portrait.media"
          :playback-rate="portrait.playbackRate"
          :active="mediaActive"
        />
        <div
          v-for="decoration in portraitDecorations"
          :key="decoration.id"
          class="theater-dialogue-portrait__decoration"
          :style="layerStyle(decoration)"
        >
          <TheaterPresentationMedia
            :media="decoration.media"
            :playback-rate="decoration.playbackRate"
            :active="mediaActive"
          />
        </div>
      </div>

      <section class="theater-dialogue-shell" :style="dialogueStyle">
        <div v-if="!frame && !narration.enabled" class="theater-dialogue-shell__default" />
        <div v-if="frame && !narration.enabled" class="theater-dialogue-frame" :style="frameStyle">
          <TheaterPresentationMedia
            :media="frame.media"
            :playback-rate="frame.playbackRate"
            :active="mediaActive"
          />
        </div>
        <div class="theater-dialogue-content" @click="completeCurrent">
          <div v-if="!narration.enabled" class="theater-dialogue-speaker" :style="{ ...textLayerStyle('speaker'), color: speakerColor }">
            <span class="theater-dialogue-speaker__value">{{ message?.actor.displayName || '角色' }}</span>
          </div>
          <div class="theater-dialogue-body" :style="contentStyle">
            <RichTextContent
              v-if="showRichContent"
              ref="richTextRef"
              :key="message?.messageId"
              class="theater-dialogue-rich-text"
              :content="richContent"
              :autoplay="useRichPlayback && typing && !snapshot.reducedMotion"
              :characters-per-second="presentation.dialogue.charactersPerSecond"
              :attachment-resolver="resolveAttachmentUrl"
              @state-change="state => { if (state.completed && typing) props.runtime.completeCurrent() }"
            />
            <span v-else>{{ revealedText }}</span>
          </div>
        </div>
      </section>
      <div class="theater-dialogue-controls" :style="dialogueControlsStyle">
        <div class="theater-dialogue-actions">
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-button quaternary circle aria-label="跳过当前对话" @click.stop="skip">
                <template #icon><n-icon><PlayerSkipForward /></n-icon></template>
              </n-button>
            </template>
            跳过
          </n-tooltip>
          <n-tooltip trigger="hover">
            <template #trigger>
              <n-button quaternary circle aria-label="关闭对话框" @click.stop="props.runtime.close()">
                <template #icon><n-icon><X /></n-icon></template>
              </n-button>
            </template>
            关闭
          </n-tooltip>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.theater-dialogue-overlay {
  position: absolute;
  z-index: 5;
  inset: 0;
  overflow: hidden;
  pointer-events: none;
  color: #f4f4f5;
  font-size: 16px;
  line-height: 1.5;
}

.theater-dialogue-narration {
  position: absolute;
  z-index: 0;
  inset: 0;
  pointer-events: none;
}

.theater-dialogue-overlay > .theater-composition {
  z-index: 1;
}

.theater-dialogue-portrait,
.theater-dialogue-portrait__decoration,
.theater-dialogue-frame {
  pointer-events: none;
}

.theater-dialogue-portrait__base,
.theater-dialogue-portrait__decoration,
.theater-dialogue-frame {
  transition: opacity 180ms ease, transform 180ms ease;
}

.theater-dialogue-shell {
  min-width: 0;
  min-height: 0;
  box-sizing: border-box;
  pointer-events: auto;
}

.theater-dialogue-shell__default {
  position: absolute;
  z-index: 0;
  inset: 0;
  border: 1px solid rgba(255, 255, 255, 0.22);
  border-radius: 4px;
  background: rgba(12, 12, 14, 0.94);
  box-shadow: 0 12px 34px rgba(0, 0, 0, 0.42);
}

.theater-dialogue-content {
  position: absolute;
  inset: 0;
  z-index: 10;
  min-width: 0;
  min-height: 0;
  pointer-events: none;
}

.theater-dialogue-controls {
  box-sizing: border-box;
  pointer-events: none;
}

.theater-dialogue-speaker {
  container-type: size;
  box-sizing: border-box;
  padding: 0;
  font-weight: 600;
  letter-spacing: 0;
  place-items: center start;
  overflow: hidden;
  cursor: pointer;
  pointer-events: auto;
}

.theater-dialogue-speaker__value {
  max-width: 100%;
  overflow: hidden;
  font-size: calc(100cqh * var(--theater-font-scale, 1));
  line-height: 1;
  white-space: nowrap;
}

.theater-dialogue-body {
  box-sizing: border-box;
  min-width: 0;
  min-height: 0;
  padding: 0;
  font-size: calc(1em * var(--theater-font-scale, 1));
  overflow: auto;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  scrollbar-width: thin;
  cursor: pointer;
  pointer-events: auto;
}

.theater-dialogue-actions {
  position: absolute;
  z-index: 1000;
  top: max(8px, env(safe-area-inset-top));
  right: max(8px, env(safe-area-inset-right));
  display: flex;
  gap: 4px;
  pointer-events: auto;
}

.theater-dialogue-actions :deep(.n-button) {
  width: 44px;
  height: 44px;
  min-width: 44px;
  color: #fff;
  background: rgba(0, 0, 0, 0.34);
}

.theater-dialogue-rich-text :deep(.rich-text-content) {
  line-height: inherit;
}

.theater-dialogue-overlay.is-reduced-motion *,
.theater-dialogue-overlay.is-reduced-motion *::before,
.theater-dialogue-overlay.is-reduced-motion *::after {
  transition: none !important;
}

/* Keep persistent text effects visible; reduced motion only removes entrance motion. */
.theater-dialogue-overlay.is-reduced-motion .theater-dialogue-rich-text :deep(.enter-blur),
.theater-dialogue-overlay.is-reduced-motion .theater-dialogue-rich-text :deep(.enter-typewriter) {
  animation: none !important;
}
</style>
