<script setup lang="ts">
import { computed, onBeforeUnmount, ref, toRaw, watch, type CSSProperties } from 'vue'
import IFormEmbedFrame from '@/components/iform/IFormEmbedFrame.vue'
import type { ChannelEmbedTheaterCharacterSource, ChannelEmbedTheaterEventSink, ChannelEmbedTheaterContext } from '@/bridge/channelEmbedHost'
import { useChatStore } from '@/stores/chat'
import { useIFormStore } from '@/stores/iform'
import { useUtilsStore } from '@/stores/utils'
import { vMediaFx, type MediaFxDirectiveValue } from '@/features/media-fx/media-fx-dom'
import { parseInternalSurfaceLink } from '@/utils/internalSurfaceLink'
import { normalizeStageIframeContent, resolveSafeStageIframeUrl, type StageIframeContent } from '../shared/stage-types'
import type { ChatCharactersSnapshotPayload } from '../bridge/theater-bridge-protocol'

const props = defineProps<{
  iframe: StageIframeContent
  interactive: boolean
  title?: string
  objectId?: string
  blockFocus?: boolean
  mediaFx?: MediaFxDirectiveValue
  worldId?: string
  channelId?: string
  scopeType?: 'world' | 'channel'
  characterSnapshot: ChatCharactersSnapshotPayload
  // Only plain iframe objects pass a sink; surface embeds keep no event linkage.
  embedEventSink?: ChannelEmbedTheaterEventSink
}>()

const chat = useChatStore()
const iformStore = useIFormStore()
const utilsStore = useUtilsStore()
iformStore.bootstrap()
const iframeContent = computed(() => normalizeStageIframeContent(props.iframe))
const configuredUrl = computed(() => iframeContent.value.url)
const iframeSrc = computed(() => resolveSafeStageIframeUrl(configuredUrl.value))
const normalizeBasePath = (value: string) => {
  const normalized = `/${value}`.replace(/\/+/g, '/').replace(/\/+$/, '')
  return normalized === '/' ? '' : normalized
}
const pathMatchesBase = (url: URL, basePath: string) => (
  !basePath || url.pathname === basePath || url.pathname.startsWith(`${basePath}/`)
)
const matchesConfiguredInternalDomain = (url: URL, domain: string, basePath: string) => {
  try {
    const hasExplicitProtocol = /^[a-z][a-z0-9+.-]*:\/\//i.test(domain)
    const canonicalUrl = new URL(hasExplicitProtocol ? domain : `http://${domain}`)
    if (!['http:', 'https:'].includes(canonicalUrl.protocol) || canonicalUrl.username || canonicalUrl.password) return false
    const hostMatches = hasExplicitProtocol
      ? url.origin === canonicalUrl.origin
      : url.hostname === canonicalUrl.hostname
        && (canonicalUrl.port ? (url.port || (url.protocol === 'https:' ? '443' : '80')) === canonicalUrl.port : !url.port)
    return hostMatches && pathMatchesBase(url, basePath)
  } catch {
    return false
  }
}
const isTrustedInternalSurfaceUrl = (url: URL) => {
  if (!['http:', 'https:'].includes(url.protocol)) return false
  try {
    const documentUrl = new URL(window.location.href.split('#', 1)[0])
    if (
      url.origin === documentUrl.origin
      && pathMatchesBase(url, normalizeBasePath(documentUrl.pathname))
    ) return true

    const canonicalBasePath = normalizeBasePath(utilsStore.config?.webUrl?.trim() || '')
    return (utilsStore.config?.domain || '').split(';').map(domain => domain.trim()).filter(Boolean)
      .some(domain => matchesConfiguredInternalDomain(url, domain, canonicalBasePath))
  } catch {
    return false
  }
}
const internalIFormTarget = computed(() => {
  if (!iframeSrc.value || typeof window === 'undefined') return null
  try {
    const url = new URL(iframeSrc.value)
    if (!isTrustedInternalSurfaceUrl(url)) return null
    const parsed = parseInternalSurfaceLink(url.href)
    return parsed?.type === 'iform' ? parsed : null
  } catch {
    return null
  }
})
const internalIFormContextMatches = computed(() => {
  const target = internalIFormTarget.value
  if (!target) return false
  return (
    String(chat.currentWorldId || '') === target.worldId
    && String(chat.curChannel?.id || '') === target.channelId
    && (props.worldId === undefined || props.worldId === target.worldId)
    && (props.channelId === undefined || props.channelId === target.channelId)
  )
})
const directIForm = computed(() => {
  const target = internalIFormTarget.value
  if (!target || !internalIFormContextMatches.value) return null
  return (iformStore.formsByChannel[target.channelId] || [])
    .find(item => item.id === target.id) || null
})
const directIFormState = ref<'idle' | 'loading' | 'loaded' | 'error'>('idle')
let loadEpoch = 0
const theaterCharacterSourceStops = new Set<() => void>()
const cloneCharacterSnapshot = (snapshot: ChatCharactersSnapshotPayload) => structuredClone(toRaw(snapshot))
const theaterCharacterSource: ChannelEmbedTheaterCharacterSource = {
  getSnapshot: () => cloneCharacterSnapshot(props.characterSnapshot),
  subscribe: (listener) => {
    const stopWatch = watch(
      () => props.characterSnapshot,
      snapshot => listener(cloneCharacterSnapshot(snapshot)),
      { flush: 'sync' },
    )
    const stop = () => {
      stopWatch()
      theaterCharacterSourceStops.delete(stop)
    }
    theaterCharacterSourceStops.add(stop)
    return stop
  },
}

watch(
  () => [
    internalIFormTarget.value?.id || '',
    internalIFormTarget.value?.worldId || '',
    internalIFormTarget.value?.channelId || '',
    internalIFormContextMatches.value,
  ] as const,
  async ([formId, , channelId, contextMatches]) => {
    const epoch = ++loadEpoch
    directIFormState.value = 'idle'
    if (!formId || !channelId || !contextMatches) return
    directIFormState.value = 'loading'
    try {
      const hadCachedForms = iformStore.hasLoadedForms(channelId)
      await iformStore.ensureForms(channelId)
      if (epoch !== loadEpoch) return
      const hasTargetForm = () => (
        (iformStore.formsByChannel[channelId] || [])
          .some(item => item.id === formId)
      )
      if (hadCachedForms && !hasTargetForm()) {
        await iformStore.ensureForms(channelId, true)
        if (epoch !== loadEpoch) return
      }
      directIFormState.value = 'loaded'
    } catch {
      if (epoch !== loadEpoch) return
      directIFormState.value = 'error'
    }
  },
  { immediate: true },
)

// Forward only events from the form this frame currently renders.
const theaterContext = computed<ChannelEmbedTheaterContext | undefined>(() => {
  const target = internalIFormTarget.value
  if (!props.embedEventSink || !props.objectId || !props.scopeType || !target || !internalIFormContextMatches.value) return undefined
  return {
    worldId: target.worldId,
    scopeType: props.scopeType,
    channelId: props.scopeType === 'world' ? '' : target.channelId,
    objectId: props.objectId,
  }
})

const forwardEmbedEvent: ChannelEmbedTheaterEventSink = (event) => {
  const target = internalIFormTarget.value
  if (
    !props.embedEventSink
    || !target
    || !internalIFormContextMatches.value
    || event.formId !== target.id
    || event.channelId !== target.channelId
  ) return
  props.embedEventSink(event)
}

onBeforeUnmount(() => {
  loadEpoch += 1
  Array.from(theaterCharacterSourceStops).forEach(stop => stop())
})

const frameRoot = ref<HTMLElement | null>(null)
const inputDisabled = computed(() => !props.interactive || props.blockFocus === true)
watch(inputDisabled, disabled => {
  if (!disabled || typeof document === 'undefined') return
  const activeElement = document.activeElement
  if (activeElement instanceof HTMLElement && frameRoot.value?.contains(activeElement)) {
    activeElement.blur()
  }
}, { flush: 'post' })

const pointerEvents = computed<'auto' | 'none'>(() => (
  inputDisabled.value ? 'none' : 'auto'
))
const frameStyle = computed<CSSProperties>(() => ({
  width: `${100 / iframeContent.value.scale}%`,
  height: `${100 / iframeContent.value.scale}%`,
  transform: `scale(${iframeContent.value.scale})`,
  transformOrigin: 'top left',
  pointerEvents: pointerEvents.value,
}))
// Media FX applies to the whole frame from its own wrapper, so WAAPI motion never
// replaces frameStyle's content scale and nothing reaches into the embedded document.
const showsFrame = computed(() => (
  internalIFormTarget.value
    ? Boolean(internalIFormContextMatches.value && directIForm.value)
    : Boolean(iframeSrc.value)
))
</script>

<template>
  <div
    ref="frameRoot"
    class="theater-iframe-frame"
    :data-stage-object-id="props.objectId || undefined"
    :class="{ 'is-input-disabled': inputDisabled }"
    :style="{ pointerEvents }"
    :inert="inputDisabled || undefined"
  >
    <div v-if="showsFrame" v-media-fx="mediaFx" class="theater-iframe-visual-object__media-fx">
      <IFormEmbedFrame
        v-if="internalIFormTarget && directIForm"
        class="theater-iframe-visual-object__iform"
        :form="directIForm"
        :channel-id="internalIFormTarget.channelId"
        :enable-channel-embed="true"
        :theater-character-source="theaterCharacterSource"
        :theater-event-sink="props.embedEventSink ? forwardEmbedEvent : undefined"
        :theater-context="theaterContext"
        :style="frameStyle"
      />
      <iframe
        v-else
        class="theater-iframe-visual-object__frame"
        :src="iframeSrc || undefined"
        :title="props.title || '网页内容'"
        :data-stage-object-id="props.objectId"
        :style="frameStyle"
        allow="autoplay; fullscreen; microphone; camera; clipboard-read; clipboard-write"
        sandbox="allow-same-origin allow-scripts allow-forms allow-pointer-lock allow-popups"
        referrerpolicy="no-referrer"
        loading="lazy"
      ></iframe>
    </div>
    <span
      v-else-if="internalIFormTarget"
      class="theater-iframe-visual-object__placeholder"
    >
      {{
        !internalIFormContextMatches
          ? 'IForm 链接与当前频道不匹配'
          : directIFormState === 'loading'
            ? '正在加载 IForm'
            : directIFormState === 'error'
              ? 'IForm 加载失败'
              : 'IForm 不存在或当前用户不可见'
      }}
    </span>
    <span v-else class="theater-iframe-visual-object__placeholder">
      {{ configuredUrl ? '仅支持 HTTP/HTTPS URL' : '请配置 URL' }}
    </span>
  </div>
</template>

<style scoped>
.theater-iframe-frame {
  width: 100%;
  height: 100%;
  box-sizing: border-box;
  margin: 0;
  padding: 0;
  overflow: visible;
  border: 0;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
}

/* Keep the entire host subtree out of hit-testing, including inline IForm embeds.
   These rules affect only host DOM; native input passes through to the canvas. */
.theater-iframe-frame.is-input-disabled,
.theater-iframe-frame.is-input-disabled :deep(*) {
  pointer-events: none !important;
  user-select: none !important;
  -webkit-user-select: none !important;
  -webkit-user-drag: none;
}

/* Clips the scaled frame exactly as the root did; the root stays unclipped so
   Media FX motion moves the whole clipped frame instead of panning inside it. */
.theater-iframe-visual-object__media-fx {
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.theater-iframe-visual-object__frame {
  display: block;
  width: 100%;
  height: 100%;
  margin: 0;
  padding: 0;
  border: 0;
  background: transparent;
}

.theater-iframe-visual-object__iform {
  border: 0 !important;
  border-radius: 0 !important;
  background: transparent !important;
  background-color: transparent !important;
  box-shadow: none !important;
  overflow: hidden !important;
}

.theater-iframe-visual-object__iform :deep(.iform-frame__iframe),
.theater-iframe-visual-object__iform :deep(.iform-frame__html),
.theater-iframe-visual-object__iform :deep(.iform-frame__html > iframe) {
  display: block;
  width: 100% !important;
  height: 100% !important;
  min-width: 0;
  min-height: 0;
  margin: 0;
  padding: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  background-color: transparent;
}

.theater-iframe-visual-object__placeholder {
  display: flex;
  width: 100%;
  height: 100%;
  align-items: center;
  justify-content: center;
  color: rgba(226, 232, 240, 0.72);
  font-size: 14px;
  pointer-events: none;
}
</style>
