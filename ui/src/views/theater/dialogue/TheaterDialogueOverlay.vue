<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch, type CSSProperties } from 'vue'
import { NButton, NIcon, NTooltip } from 'naive-ui'
import { PlayerSkipForward, X } from '@vicons/tabler'
import RichTextContent from '@/components/rich-text/RichTextContent.vue'
import TheaterPresentationMedia from '@/components/theater-presentation/TheaterPresentationMedia.vue'
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver'
import { createDefaultTheaterPresentation, DEFAULT_THEATER_PORTRAIT_FADE_DURATION_MS, resolveTheaterBackdropColor, resolveTheaterTextTransformStyle, resolveTheaterTransformStyle, type TheaterVisualLayer } from '@/types/theaterPresentation'
import { resolvePlatformFontFamily } from '@/services/font/platformFontRegistry'
import { isTipTapJson } from '@/utils/tiptap-render'
import type { ChatCharactersSnapshotPayload } from '../bridge/theater-bridge-protocol'
import {
  hasTheaterDialoguePerformanceContent,
  resolveTheaterDialoguePresentation,
  type TheaterDialogueRuntimeController,
  type TheaterDialogueRuntimeSnapshot,
} from './theater-dialogue-runtime'
import '@/components/theater-presentation/theaterComposition.css'
import { useTheaterAppearanceCache } from '@/composables/useTheaterAppearanceCache'
import { resolveTheaterReducedMotion } from '../shared/theater-reduced-motion'
import { isTheaterBridgeDebugEnabled, logTheaterDialogueDebug } from '../bridge/theater-bridge-debug'
import type { TheaterDialogueEmbedSettings } from './theater-dialogue-embed-settings'
import TheaterDialogueResidents from './TheaterDialogueResidents.vue'
import type { DialogueController, DialogueControllerTemplate } from './theater-dialogue-controller'
import type { DialoguePosition } from './theater-dialogue-layout'

const props = defineProps<{
  runtime: TheaterDialogueRuntimeController
  characterSnapshot: ChatCharactersSnapshotPayload
  worldId: string
  channelId: string
  fillContainer?: boolean
  textOnly?: boolean
  textOverrides?: TheaterDialogueEmbedSettings
  hideDialoguePerformance?: boolean
  hidePortraitPerformance?: boolean
  controller?: DialogueController
  controllerTemplate?: DialogueControllerTemplate | null
  canDragPortraits?: boolean
  savePortraitPosition?: (key: string, position: DialoguePosition) => Promise<void>
}>()

const rootRef = ref<HTMLElement | null>(null)
const bodyRef = ref<HTMLElement | null>(null)
const bodyContentRef = ref<HTMLElement | null>(null)
const richTextRef = ref<InstanceType<typeof RichTextContent> | null>(null)
const visibleInViewport = ref(true)
const snapshot = ref<TheaterDialogueRuntimeSnapshot>(props.runtime.getSnapshot())
const livePresentation = ref<ReturnType<typeof resolveTheaterDialoguePresentation> | null>(null)
const appearanceCache = useTheaterAppearanceCache()
let unsubscribe: (() => void) | null = null
let intersectionObserver: IntersectionObserver | null = null
let bodyContentObserver: { disconnect: () => void } | null = null
let motionQuery: MediaQueryList | null = null
let invalidateAppearance: ((event: Event) => void) | null = null
let appearanceRequestGeneration = 0
let livePresentationContextKey = ''
const speakerFontFamily = ref('')
const contentFontFamily = ref('')
let speakerFontLoadGeneration = 0
let contentFontLoadGeneration = 0

/** Keep the latest revealed line visible while dialogue text grows. */
const stickDialogueBodyToBottom = () => {
  const body = bodyRef.value
  const content = bodyContentRef.value
  if (!body || !content) return

  // Rich performance: base layer already has full height; follow the overlay frontier only.
  const overlay = content.querySelector('.twin-layer-message__overlay-text')
  if (overlay) {
    const frontier = overlay.lastElementChild
    if (!(frontier instanceof HTMLElement)) return
    const bodyRect = body.getBoundingClientRect()
    const frontierRect = frontier.getBoundingClientRect()
    if (frontierRect.bottom > bodyRect.bottom) {
      body.scrollTop += frontierRect.bottom - bodyRect.bottom
    }
    return
  }

  // Plain / non-performance: content height tracks revealed text.
  body.scrollTop = body.scrollHeight
}

const current = computed(() => snapshot.value.queue.current)
const message = computed(() => current.value?.message || null)
const presentation = computed(() => {
  const base = livePresentation.value || resolveTheaterDialoguePresentation(message.value, props.characterSnapshot)
  if (props.controller?.enabled && props.controllerTemplate) return { ...props.controllerTemplate.presentation, portrait: null, narration: base.narration }
  const overrides = props.textOverrides
  if (!overrides) return base
  return { ...base, dialogue: {
    ...base.dialogue,
    contentColor: overrides.contentColor || base.dialogue.contentColor,
    charactersPerSecond: overrides.charactersPerSecond ?? base.dialogue.charactersPerSecond,
    speaker: { ...base.dialogue.speaker, enabled: overrides.showSpeaker, fontAssetId: overrides.fontAssetId || base.dialogue.speaker.fontAssetId },
    content: { ...base.dialogue.content, fontAssetId: overrides.fontAssetId || base.dialogue.content.fontAssetId },
  } }
})
const effectiveDialogueTransform = computed(() => (
  props.fillContainer
    ? {
        ...presentation.value.dialogue.transform,
        x: 0,
        y: 0,
        width: 1,
        height: 1,
        rotation: 0,
      }
    : presentation.value.dialogue.transform
))
const dialogueStyle = computed<CSSProperties>(() => ({
  ...resolveTheaterTransformStyle(effectiveDialogueTransform.value),
}))
const dialogueControlsStyle = computed<CSSProperties>(() => ({ ...dialogueStyle.value, zIndex: '1000' }))
const portrait = computed(() => presentation.value.portrait?.enabled ? presentation.value.portrait : null)
const portraitFadeDurationMs = computed(() => (
  portrait.value?.fadeDurationMs ?? DEFAULT_THEATER_PORTRAIT_FADE_DURATION_MS
))
const portraitKey = computed(() => {
  const layer = portrait.value
  const actor = current.value?.message.actor
  if (!layer || !actor) return ''
  return JSON.stringify([
    actor.identityId,
    actor.variantId || null,
    layer.media.assetId,
    layer.media.resourceAttachmentId,
  ])
})
const portraitStyle = computed<CSSProperties | undefined>(() => {
  const layer = portrait.value
  if (!layer) return undefined
  const { opacity, ...layoutStyle } = resolveTheaterTransformStyle(layer.transform)
  const fadeDurationMs = layer.fadeDurationMs ?? DEFAULT_THEATER_PORTRAIT_FADE_DURATION_MS
  return {
    ...layoutStyle,
    '--theater-portrait-opacity': opacity,
    '--theater-portrait-fade-duration': `${fadeDurationMs}ms`,
  }
})
const portraitDecorations = computed(() => presentation.value.portraitDecorations
  .filter((layer) => layer.enabled)
  .sort((left, right) => left.transform.zIndex - right.transform.zIndex))
const frame = computed(() => presentation.value.dialogue.frame?.enabled ? presentation.value.dialogue.frame : null)
const narration = computed(() => presentation.value.narration)
const canGatePlaybackForPortrait = computed(() => Boolean(
  current.value
  && portrait.value
  && !props.textOnly
  && !narration.value.enabled
  && !snapshot.value.reducedMotion
  && !props.hidePortraitPerformance
  && !props.hideDialoguePerformance
  && portraitFadeDurationMs.value > 0
))
const portraitPlaybackGate = ref(false)
let trackedPortraitMessageId = ''
let portraitGateMessageId = ''
let pendingPortraitKey = ''
let settledPortraitKey = ''

const releasePortraitPlaybackGate = (settledKey?: string, settledMessageId?: string) => {
  portraitPlaybackGate.value = false
  portraitGateMessageId = ''
  pendingPortraitKey = ''
  props.runtime.setPlaybackPaused(false, settledKey, settledMessageId)
}

watch(
  () => [
    current.value?.message.messageId || '',
    portraitKey.value,
    canGatePlaybackForPortrait.value,
  ] as const,
  ([messageId, nextPortraitKey, canGate]) => {
    if (!messageId) {
      trackedPortraitMessageId = ''
      settledPortraitKey = ''
      releasePortraitPlaybackGate()
      return
    }

    if (messageId !== trackedPortraitMessageId) {
      trackedPortraitMessageId = messageId
      if (canGate && nextPortraitKey && nextPortraitKey !== settledPortraitKey) {
        portraitPlaybackGate.value = true
        portraitGateMessageId = messageId
        pendingPortraitKey = nextPortraitKey
        props.runtime.setPlaybackPaused(true)
        return
      }
      if (!canGate) {
        settledPortraitKey = nextPortraitKey
        releasePortraitPlaybackGate(nextPortraitKey, messageId)
        return
      }
      // Same settled portrait: no new Transition is required.
      // Still release a possible parent-side pre-pause for this exact message.
      releasePortraitPlaybackGate(undefined, messageId)
      return
    }

    if (portraitPlaybackGate.value) {
      if (canGate && nextPortraitKey) {
        pendingPortraitKey = nextPortraitKey
        return
      }
      settledPortraitKey = nextPortraitKey
      releasePortraitPlaybackGate(nextPortraitKey, messageId)
      return
    }

    // Once this message has begun, asynchronous appearance changes may animate
    // the portrait but must never pause or restart its subtitle playback.
    settledPortraitKey = nextPortraitKey
    if (!canGate) releasePortraitPlaybackGate(nextPortraitKey, messageId)
  },
  { immediate: true, flush: 'sync' },
)

const resolveEnteredPortraitState = (element: Element) => {
  if (!(element instanceof HTMLElement)) return { portraitKey: '', messageId: '' }
  return {
    portraitKey: element.dataset.portraitKey ?? '',
    messageId: element.dataset.messageId ?? '',
  }
}

const handlePortraitAfterEnter = (element: Element) => {
  const { portraitKey: enteredPortraitKey, messageId: enteredMessageId } = resolveEnteredPortraitState(element)
  if (!enteredPortraitKey || !enteredMessageId) return
  if (portraitPlaybackGate.value) {
    if (enteredMessageId !== portraitGateMessageId || enteredPortraitKey !== pendingPortraitKey) return
    settledPortraitKey = enteredPortraitKey
    releasePortraitPlaybackGate(enteredPortraitKey, enteredMessageId)
    return
  }
  settledPortraitKey = enteredPortraitKey
  props.runtime.setPlaybackPaused(false, enteredPortraitKey, enteredMessageId)
}

const handlePortraitEnterCancelled = () => {
  const messageId = current.value?.message.messageId || ''
  if (
    canGatePlaybackForPortrait.value
    && portraitPlaybackGate.value
    && messageId === portraitGateMessageId
    && portraitKey.value
  ) return
  settledPortraitKey = portraitKey.value
  releasePortraitPlaybackGate(portraitKey.value, messageId)
}
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
  return hasTheaterDialoguePerformanceContent(message.value)
})
const showRichContent = computed(() => Boolean(richContent.value && (!typing.value || useRichPlayback.value)))
const mediaActive = computed(() => Boolean(current.value && typing.value && visibleInViewport.value))
const speakerColor = computed(() => {
  if (props.controller?.enabled) return presentation.value.dialogue.contentColor
  if (props.textOverrides?.speakerColor) return props.textOverrides.speakerColor
  const color = String(message.value?.actor.color || '').trim()
  return typeof CSS !== 'undefined' && CSS.supports('color', color) ? color : 'var(--sc-text-primary, #f4f4f5)'
})
const textLayerStyle = (kind: 'speaker' | 'content'): CSSProperties => ({
  ...(props.textOnly ? {} : resolveTheaterTextTransformStyle(presentation.value.dialogue[kind].transform)),
  display: presentation.value.dialogue[kind].enabled ? (kind === 'speaker' ? 'grid' : 'block') : 'none',
  textAlign: presentation.value.dialogue.textAlign,
  '--theater-font-scale': String(presentation.value.dialogue[kind].fontScale),
  ...(props.textOnly && props.textOverrides ? { fontSize: `${props.textOverrides.fontSize}px`, '--theater-font-scale': '1' } : {}),
})
const speakerStyle = computed<CSSProperties>(() => ({
  ...textLayerStyle('speaker'),
  color: speakerColor.value,
  ...(speakerFontFamily.value ? { fontFamily: speakerFontFamily.value } : {}),
}))
const contentStyle = computed<CSSProperties>(() => ({
  ...textLayerStyle('content'),
  color: presentation.value.dialogue.contentColor,
  ...(contentFontFamily.value ? { fontFamily: contentFontFamily.value } : {}),
}))

const refreshSpeakerFontFamily = (fontAssetId: string | undefined) => {
  const generation = ++speakerFontLoadGeneration
  const normalizedId = String(fontAssetId || '').trim()
  if (!normalizedId) {
    speakerFontFamily.value = ''
    return
  }
  speakerFontFamily.value = ''
  void resolvePlatformFontFamily(normalizedId).then((family) => {
    if (generation === speakerFontLoadGeneration) speakerFontFamily.value = family
  }).catch(() => {
    if (generation === speakerFontLoadGeneration) speakerFontFamily.value = ''
  })
}

const refreshContentFontFamily = (fontAssetId: string | undefined) => {
  const generation = ++contentFontLoadGeneration
  const normalizedId = String(fontAssetId || '').trim()
  if (!normalizedId) {
    contentFontFamily.value = ''
    return
  }
  contentFontFamily.value = ''
  void resolvePlatformFontFamily(normalizedId).then((family) => {
    if (generation === contentFontLoadGeneration) contentFontFamily.value = family
  }).catch(() => {
    if (generation === contentFontLoadGeneration) contentFontFamily.value = ''
  })
}

watch(
  () => presentation.value.dialogue.speaker.fontAssetId,
  refreshSpeakerFontFamily,
  { immediate: true },
)

watch(
  () => presentation.value.dialogue.content.fontAssetId,
  refreshContentFontFamily,
  { immediate: true },
)

watch(
  () => presentation.value.dialogue.charactersPerSecond,
  (speed) => props.runtime.setCharactersPerSecond(speed),
  { immediate: true },
)

watch(bodyContentRef, (el) => {
  bodyContentObserver?.disconnect()
  bodyContentObserver = null
  if (!el) return
  // Height growth (plain reveal / reflow) + DOM growth (rich overlay chars).
  const resizeObserver = new ResizeObserver(() => { stickDialogueBodyToBottom() })
  const mutationObserver = new MutationObserver(() => { stickDialogueBodyToBottom() })
  resizeObserver.observe(el)
  mutationObserver.observe(el, { childList: true, subtree: true, characterData: true })
  bodyContentObserver = {
    disconnect: () => {
      resizeObserver.disconnect()
      mutationObserver.disconnect()
    },
  }
  void nextTick(stickDialogueBodyToBottom)
}, { flush: 'post' })

// Plain-text reveal updates text without always resizing in the same tick; keep in sync.
watch(
  () => current.value?.revealedCharacters,
  () => { void nextTick(stickDialogueBodyToBottom) },
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
  const messageId = message.value?.messageId
  if (!messageId) return
  richTextRef.value?.skip()
  props.runtime.completeCurrent(messageId)
}

const skip = () => {
  richTextRef.value?.skip()
  props.runtime.skip()
}

const handleRichPlaybackState = (state: { completed: boolean }) => {
  const messageId = message.value?.messageId
  logTheaterDialogueDebug('overlay.player-state', { messageId: messageId || null, typing: typing.value, ...state })
  if (!state.completed || !typing.value || !messageId) return
  props.runtime.completeCurrent(messageId)
}

const handleRichPlaybackCompleted = () => {
  const messageId = message.value?.messageId
  logTheaterDialogueDebug('overlay.player-completed', { messageId: messageId || null, typing: typing.value })
  if (!typing.value || !messageId) return
  props.runtime.completeCurrent(messageId)
}

const updateReducedMotion = () => props.runtime.setReducedMotion(resolveTheaterReducedMotion().effectiveReducedMotion)

const resolveLivePresentationContextKey = (identityId: string, variantId: string | null) => JSON.stringify([
  props.worldId,
  props.channelId,
  props.characterSnapshot.revision,
  props.characterSnapshot.updatedAt,
  identityId,
  variantId,
])

const refreshLivePresentation = () => {
  const targetMessage = message.value
  const targetActor = targetMessage?.actor
  const generation = ++appearanceRequestGeneration
  const targetMessageId = targetMessage?.messageId || ''
  const targetIdentityId = targetActor?.identityId || ''
  const targetVariantId = targetActor?.variantId || null
  const targetContextKey = targetActor
    ? resolveLivePresentationContextKey(targetIdentityId, targetVariantId)
    : ''

  if (!targetActor) {
    livePresentation.value = null
    livePresentationContextKey = ''
    return
  }

  const samePresentationContext = livePresentation.value !== null
    && livePresentationContextKey === targetContextKey
  if (!samePresentationContext) {
    // Keep current message's snapshot presentation visible while remote resolution runs.
    livePresentation.value = resolveTheaterDialoguePresentation(targetMessage, props.characterSnapshot)
    livePresentationContextKey = targetContextKey
  }
  if (!targetActor?.identityId) return

  void appearanceCache.resolve(props.worldId, props.channelId, {
    identityId: targetActor.identityId,
    variantId: targetActor.variantId,
  }).then((resolved) => {
    const currentMessage = message.value
    const currentActor = currentMessage?.actor
    if (
      generation !== appearanceRequestGeneration
      || currentMessage?.messageId !== targetMessageId
      || currentActor?.identityId !== targetIdentityId
      || currentActor?.variantId !== targetVariantId
    ) return
    if (resolved) {
      livePresentation.value = resolved.presentation || createDefaultTheaterPresentation()
      livePresentationContextKey = targetContextKey
    }
  }).catch(() => undefined)
}

onMounted(() => {
  unsubscribe = props.runtime.subscribe((value) => { snapshot.value = value })
  motionQuery = window.matchMedia('(prefers-reduced-motion: reduce)')
  motionQuery.addEventListener('change', updateReducedMotion)
  updateReducedMotion()
  invalidateAppearance = (event: Event) => {
    const detail = (event as CustomEvent<{ channelId?: string }>).detail
    const invalidatedChannelId = String(detail?.channelId || '').trim()
    if (!invalidatedChannelId) return
    appearanceCache.invalidateChannel(invalidatedChannelId)
    if (invalidatedChannelId !== String(props.channelId).trim()) return
    refreshLivePresentation()
  }
  window.addEventListener('sealchat:theater-appearance-invalidated', invalidateAppearance)
  void nextTick(() => {
    if (!rootRef.value) return
    intersectionObserver = new IntersectionObserver(([entry]) => { visibleInViewport.value = entry.isIntersecting })
    intersectionObserver.observe(rootRef.value)
  })
})

watch(
  () => [
    props.worldId,
    props.channelId,
    props.characterSnapshot.revision,
    props.characterSnapshot.updatedAt,
    message.value?.actor.identityId,
    message.value?.actor.variantId,
    message.value?.messageId,
  ] as const,
  refreshLivePresentation,
  { immediate: true },
)

onBeforeUnmount(() => {
  speakerFontLoadGeneration += 1
  contentFontLoadGeneration += 1
  appearanceRequestGeneration += 1
  releasePortraitPlaybackGate()
  unsubscribe?.()
  intersectionObserver?.disconnect()
  bodyContentObserver?.disconnect()
  bodyContentObserver = null
  motionQuery?.removeEventListener('change', updateReducedMotion)
  if (invalidateAppearance) window.removeEventListener('sealchat:theater-appearance-invalidated', invalidateAppearance)
})
</script>

<template>
  <div
    ref="rootRef"
    class="theater-dialogue-overlay theater-composition-host"
    :class="{
      'is-open': current,
      'is-reduced-motion': snapshot.reducedMotion,
      'is-fill-container': fillContainer,
      'is-text-only': textOnly,
      'is-dialogue-performance-hidden': hideDialoguePerformance,
      'is-portrait-performance-hidden': hidePortraitPerformance,
    }"
    aria-live="polite"
  >
    <div v-if="!textOnly && current && narration.enabled" class="theater-dialogue-narration" :style="narrationStyle" />
    <div class="theater-composition">
      <TheaterDialogueResidents v-if="controller?.enabled && controllerTemplate && savePortraitPosition" :runtime="runtime" :world-id="worldId" :channel-id="channelId" :controller="controller" :template="controllerTemplate" :can-drag="canDragPortraits === true" :save-position="savePortraitPosition" />
      <Transition
        name="theater-portrait-fade"
        appear
        @after-enter="handlePortraitAfterEnter"
        @enter-cancelled="handlePortraitEnterCancelled"
      >
        <div v-if="current && !textOnly && portrait && !narration.enabled" :key="portraitKey" :data-portrait-key="portraitKey" :data-message-id="message?.messageId || ''" class="theater-dialogue-portrait" :style="portraitStyle">
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
      </Transition>

      <section v-if="current" class="theater-dialogue-shell" :style="dialogueStyle">
        <div v-if="!textOnly && !frame && !narration.enabled" class="theater-dialogue-shell__default" />
        <div v-if="!textOnly && frame && !narration.enabled" class="theater-dialogue-frame" :style="frameStyle">
          <TheaterPresentationMedia
            :media="frame.media"
            :playback-rate="frame.playbackRate"
            :active="mediaActive"
          />
        </div>
        <div class="theater-dialogue-content" @click="completeCurrent">
          <div v-if="textOnly || !narration.enabled" class="theater-dialogue-speaker" :style="speakerStyle">
            <span class="theater-dialogue-speaker__value">{{ message?.actor.displayName || '角色' }}</span>
          </div>
          <div ref="bodyRef" class="theater-dialogue-body" :style="contentStyle">
            <div ref="bodyContentRef" class="theater-dialogue-body__content">
              <RichTextContent
                v-if="showRichContent && !portraitPlaybackGate"
                ref="richTextRef"
                :key="message?.messageId"
                class="theater-dialogue-rich-text"
                :content="richContent"
                :autoplay="useRichPlayback && typing"
                :debug-playback="isTheaterBridgeDebugEnabled()"
                :characters-per-second="presentation.dialogue.charactersPerSecond"
                :attachment-resolver="resolveAttachmentUrl"
                @state-change="handleRichPlaybackState"
                @completed="handleRichPlaybackCompleted"
              />
              <span v-else>{{ revealedText }}</span>
            </div>
          </div>
        </div>
      </section>
      <div v-if="current && !textOnly" class="theater-dialogue-controls" :style="dialogueControlsStyle">
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
.is-text-only .theater-dialogue-content {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
}
.is-text-only .theater-dialogue-speaker { container-type: normal; flex: none; }
.is-text-only .theater-dialogue-speaker__value { font-size: inherit; line-height: 1.3; }
.is-text-only .theater-dialogue-body { flex: 1; }

.theater-dialogue-overlay {
  position: absolute;
  z-index: 9500;
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

.theater-dialogue-overlay.is-dialogue-performance-hidden .theater-dialogue-shell,
.theater-dialogue-overlay.is-dialogue-performance-hidden .theater-dialogue-controls,
.theater-dialogue-overlay.is-dialogue-performance-hidden .theater-dialogue-narration,
.theater-dialogue-overlay.is-dialogue-performance-hidden .theater-dialogue-portrait {
  display: none;
}

.theater-dialogue-overlay.is-portrait-performance-hidden .theater-dialogue-portrait {
  display: none;
}

.theater-dialogue-overlay > .theater-composition {
  z-index: 1;
}

.theater-dialogue-overlay.is-fill-container > .theater-composition {
  width: 100cqw;
  height: 100cqh;
  aspect-ratio: auto;
}

.theater-dialogue-portrait,
.theater-dialogue-portrait__decoration,
.theater-dialogue-frame {
  pointer-events: none;
}

.theater-dialogue-portrait { opacity: var(--theater-portrait-opacity, 1); }
.theater-dialogue-portrait__base,
.theater-dialogue-portrait__decoration { transition: transform 180ms ease; }
.theater-dialogue-frame { transition: opacity 180ms ease, transform 180ms ease; }
.theater-portrait-fade-enter-active,
.theater-portrait-fade-leave-active {
  transition: opacity var(--theater-portrait-fade-duration, 90ms) ease;
}
.theater-portrait-fade-enter-to,
.theater-portrait-fade-leave-from { opacity: var(--theater-portrait-opacity, 1); }
.theater-portrait-fade-enter-from,
.theater-portrait-fade-leave-to { opacity: 0; }

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
