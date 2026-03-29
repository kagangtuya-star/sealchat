<script setup lang="ts">
import { computed, ref, watch, useAttrs, onMounted, onBeforeUnmount, nextTick, reactive } from 'vue'
import type { CSSProperties } from 'vue'
import Avatar from '@/components/avatar.vue'
import {
  resolveAttachmentUrl,
  fetchAttachmentMetaById,
  normalizeAttachmentId,
  type AttachmentMeta,
} from '@/composables/useAttachmentResolver'
import { useDisplayStore } from '@/stores/display'
import type { AvatarDecoration } from '@/types'
import { normalizeAvatarDecorations } from '@/utils/avatarDecorations'

defineOptions({
  inheritAttrs: false,
})

const props = withDefaults(defineProps<{
  src?: string
  size?: number
  border?: boolean
  fallbackText?: string
  useTextFallback?: boolean
  decoration?: AvatarDecoration | null
  decorations?: AvatarDecoration[] | null
  decorationEnabled?: boolean
  pauseWhenOutOfView?: boolean
  activeDecorationId?: string
  highlightActiveDecoration?: boolean
}>(), {
  src: '',
  size: 0,
  border: true,
  fallbackText: '',
  useTextFallback: false,
  decoration: null,
  decorations: null,
  decorationEnabled: true,
  pauseWhenOutOfView: true,
  activeDecorationId: '',
  highlightActiveDecoration: false,
})

type DecorationLayerEntry = {
  id: string
  decoration: AvatarDecoration
  kind: 'image' | 'video'
  src: string
  isBackground: boolean
  isActive: boolean
  style: CSSProperties
}

let transparentWebMSupportPromise: Promise<boolean> | null = null

const detectTransparentWebMSupport = async (): Promise<boolean> => {
  if (typeof window === 'undefined' || typeof document === 'undefined') {
    return false
  }
  if (transparentWebMSupportPromise) {
    return transparentWebMSupportPromise
  }
  transparentWebMSupportPromise = Promise.resolve().then(() => {
    const ua = navigator.userAgent || ''
    const platform = navigator.platform || ''
    const maxTouchPoints = Number(navigator.maxTouchPoints || 0)
    const isIOS = /iPad|iPhone|iPod/i.test(ua) || (platform === 'MacIntel' && maxTouchPoints > 1)
    const isSafari = /^((?!chrome|android|crios|fxios|edgios).)*safari/i.test(ua)
    if (isIOS || isSafari) {
      return false
    }
    const video = document.createElement('video')
    return [
      'video/webm; codecs="vp9"',
      'video/webm; codecs="vp8"',
      'video/webm',
    ].some((type) => video.canPlayType(type) !== '')
  })
  return transparentWebMSupportPromise
}

const attrs = useAttrs()
const display = useDisplayStore()
const rootRef = ref<HTMLElement | null>(null)
const transparentWebMSupported = ref(false)
const isInViewport = ref(true)
const resourceMetaMap = reactive<Record<string, AttachmentMeta | null | undefined>>({})
const pendingMetaIds = new Set<string>()
const metaRetryCountMap = reactive<Record<string, number>>({})
const metaRetryTimerMap = new Map<string, number>()
const resourceLoadFailedMap = reactive<Record<string, boolean>>({})
const fallbackLoadFailedMap = reactive<Record<string, boolean>>({})

let viewportObserver: IntersectionObserver | null = null

const normalizedDecorations = computed(() => normalizeAvatarDecorations(props.decorations, props.decoration))

const ensureAttachmentMetaLoaded = async (attachmentId: string) => {
  const normalized = normalizeAttachmentId(attachmentId)
  if (!normalized || pendingMetaIds.has(normalized) || resourceMetaMap[normalized] !== undefined) {
    return
  }
  pendingMetaIds.add(normalized)
  try {
    const meta = await fetchAttachmentMetaById(normalized)
    if (meta) {
      resourceMetaMap[normalized] = meta
      metaRetryCountMap[normalized] = 0
      const timer = metaRetryTimerMap.get(normalized)
      if (timer) {
        window.clearTimeout(timer)
        metaRetryTimerMap.delete(normalized)
      }
      return
    }
    const retryCount = Number(metaRetryCountMap[normalized] || 0)
    resourceMetaMap[normalized] = undefined
    if (typeof window !== 'undefined' && retryCount < 6) {
      metaRetryCountMap[normalized] = retryCount + 1
      const delay = Math.min(1200, 180 * (retryCount + 1))
      const previousTimer = metaRetryTimerMap.get(normalized)
      if (previousTimer) {
        window.clearTimeout(previousTimer)
      }
      const timerId = window.setTimeout(() => {
        metaRetryTimerMap.delete(normalized)
        void ensureAttachmentMetaLoaded(attachmentId)
      }, delay)
      metaRetryTimerMap.set(normalized, timerId)
    }
  } finally {
    pendingMetaIds.delete(normalized)
  }
}

watch(normalizedDecorations, (list) => {
  list.forEach((item) => {
    if (item.resourceAttachmentId) {
      void ensureAttachmentMetaLoaded(item.resourceAttachmentId)
    }
    if (item.fallbackAttachmentId) {
      void ensureAttachmentMetaLoaded(item.fallbackAttachmentId)
    }
  })
}, { immediate: true })

const buildLayerStyle = (decoration: AvatarDecoration): CSSProperties => {
  const settings = decoration.settings || {}
  return {
    transform: `translate(${settings.offsetX ?? 0}px, ${settings.offsetY ?? 0}px) scale(${settings.scale ?? 1}) rotate(${settings.rotation ?? 0}deg)`,
    opacity: `${settings.opacity ?? 1}`,
    mixBlendMode: (settings.blendMode || 'normal') as CSSProperties['mixBlendMode'],
  }
}

const resolveDecorationLayer = (decoration: AvatarDecoration): DecorationLayerEntry | null => {
  const id = String(decoration.id || '').trim()
  if (!props.decorationEnabled || decoration.enabled !== true || !id) {
    return null
  }
  const resourceAttachmentId = decoration.resourceAttachmentId || ''
  const resourceKey = normalizeAttachmentId(resourceAttachmentId)
  const fallbackAttachmentId = decoration.fallbackAttachmentId || ''
  const fallbackKey = normalizeAttachmentId(fallbackAttachmentId)
  const resourceMeta = resourceKey ? resourceMetaMap[resourceKey] : null
  const resourceMetaResolved = resourceKey ? resourceMetaMap[resourceKey] !== undefined : true
  const resourceMime = String(resourceMeta?.mimeType || '').trim().toLowerCase()
  const resourceSrc = resolveAttachmentUrl(resourceAttachmentId)
  const fallbackSrc = fallbackLoadFailedMap[id] ? '' : resolveAttachmentUrl(fallbackAttachmentId)
  const resourceFailed = Boolean(resourceLoadFailedMap[id])

  let kind: 'image' | 'video' | null = null
  let src = ''

  if (!resourceFailed && resourceSrc) {
    if (resourceKey && !resourceMetaResolved) {
      return null
    }
    if (resourceMime === 'video/webm') {
      if (display.settings.preferStaticAvatarDecoration) {
        if (fallbackSrc) {
          kind = 'image'
          src = fallbackSrc
        }
      } else if (transparentWebMSupported.value) {
        kind = 'video'
        src = resourceSrc
      } else if (fallbackSrc) {
        kind = 'image'
        src = fallbackSrc
      }
    } else if (resourceMime === 'image/png' || resourceMime === 'image/webp' || resourceMime === '') {
      kind = 'image'
      src = resourceSrc
    }
  }

  if (!kind && fallbackSrc) {
    kind = 'image'
    src = fallbackSrc
  }
  if (!kind || !src) {
    return null
  }

  return {
    id,
    decoration,
    kind,
    src,
    isBackground: (decoration.settings?.zIndex ?? 1) < 0,
    isActive: props.highlightActiveDecoration && id === props.activeDecorationId,
    style: buildLayerStyle(decoration),
  }
}

const resolvedLayers = computed(() => normalizedDecorations.value
  .map(resolveDecorationLayer)
  .filter((item): item is DecorationLayerEntry => Boolean(item)))

const backgroundLayers = computed(() => resolvedLayers.value.filter((item) => item.isBackground))
const foregroundLayers = computed(() => resolvedLayers.value.filter((item) => !item.isBackground))

const shellStyle = computed<CSSProperties>(() => {
  if (props.size > 0) {
    return {
      width: `${props.size}px`,
      height: `${props.size}px`,
      minWidth: `${props.size}px`,
      minHeight: `${props.size}px`,
    }
  }
  return {
    width: 'var(--chat-avatar-size, 48px)',
    height: 'var(--chat-avatar-size, 48px)',
    minWidth: 'var(--chat-avatar-size, 48px)',
    minHeight: 'var(--chat-avatar-size, 48px)',
  }
})

const shouldPlayVideo = computed(() => !props.pauseWhenOutOfView || isInViewport.value)

const updateVideoPlayback = async () => {
  await nextTick()
  const root = rootRef.value
  if (!root) {
    return
  }
  const videos = Array.from(root.querySelectorAll<HTMLVideoElement>('[data-decoration-video="1"]'))
  await Promise.all(videos.map(async (video) => {
    const layerId = String(video.dataset.decorationId || '').trim()
    const decoration = normalizedDecorations.value.find((item) => item.id === layerId)
    const playbackRate = decoration?.settings?.playbackRate ?? 1
    if (video.playbackRate !== playbackRate) {
      video.playbackRate = playbackRate
      video.defaultPlaybackRate = playbackRate
    }
    if (!shouldPlayVideo.value) {
      video.pause()
      return
    }
    try {
      await video.play()
    } catch {
      // Ignore autoplay failures.
    }
  }))
}

const handleLayerImageError = (id: string, src: string) => {
  const decoration = normalizedDecorations.value.find((item) => item.id === id)
  if (!decoration) {
    return
  }
  const resourceSrc = resolveAttachmentUrl(decoration.resourceAttachmentId || '')
  if (src === resourceSrc) {
    resourceLoadFailedMap[id] = true
    return
  }
  fallbackLoadFailedMap[id] = true
}

const handleLayerVideoError = (id: string) => {
  resourceLoadFailedMap[id] = true
}

const setupViewportObserver = () => {
  viewportObserver?.disconnect()
  viewportObserver = null
  if (!props.pauseWhenOutOfView || !rootRef.value || typeof IntersectionObserver === 'undefined') {
    isInViewport.value = true
    return
  }
  viewportObserver = new IntersectionObserver((entries) => {
    const [entry] = entries
    isInViewport.value = Boolean(entry?.isIntersecting)
  }, { threshold: 0.05 })
  viewportObserver.observe(rootRef.value)
}

onMounted(async () => {
  transparentWebMSupported.value = await detectTransparentWebMSupport()
  setupViewportObserver()
  void updateVideoPlayback()
})

onBeforeUnmount(() => {
  viewportObserver?.disconnect()
  viewportObserver = null
  metaRetryTimerMap.forEach((timerId) => window.clearTimeout(timerId))
  metaRetryTimerMap.clear()
  rootRef.value?.querySelectorAll<HTMLVideoElement>('[data-decoration-video="1"]').forEach((video) => video.pause())
})

watch(() => props.pauseWhenOutOfView, () => {
  setupViewportObserver()
  void updateVideoPlayback()
})

watch(() => rootRef.value, () => {
  setupViewportObserver()
})

watch([resolvedLayers, shouldPlayVideo], () => {
  void updateVideoPlayback()
}, { flush: 'post' })
</script>

<template>
  <div
    ref="rootRef"
    class="user-avatar-decoration"
    :style="shellStyle"
    v-bind="attrs"
  >
    <template v-for="layer in backgroundLayers" :key="`bg-${layer.id}`">
      <img
        v-if="layer.kind === 'image'"
        class="user-avatar-decoration__layer user-avatar-decoration__layer--background"
        :class="{ 'user-avatar-decoration__layer--active': layer.isActive }"
        :src="layer.src"
        :style="layer.style"
        draggable="false"
        @error="handleLayerImageError(layer.id, layer.src)"
      />
      <video
        v-else
        class="user-avatar-decoration__layer user-avatar-decoration__layer--background"
        :class="{ 'user-avatar-decoration__layer--active': layer.isActive }"
        :src="layer.src"
        :style="layer.style"
        data-decoration-video="1"
        :data-decoration-id="layer.id"
        muted
        loop
        playsinline
        preload="metadata"
        disablepictureinpicture
        disableremoteplayback
        @error="handleLayerVideoError(layer.id)"
      ></video>
    </template>

    <Avatar
      :src="src"
      :size="size"
      :border="border"
      :fallback-text="fallbackText"
      :use-text-fallback="useTextFallback"
    />

    <template v-for="layer in foregroundLayers" :key="`fg-${layer.id}`">
      <img
        v-if="layer.kind === 'image'"
        class="user-avatar-decoration__layer user-avatar-decoration__layer--foreground"
        :class="{ 'user-avatar-decoration__layer--active': layer.isActive }"
        :src="layer.src"
        :style="layer.style"
        draggable="false"
        @error="handleLayerImageError(layer.id, layer.src)"
      />
      <video
        v-else
        class="user-avatar-decoration__layer user-avatar-decoration__layer--foreground"
        :class="{ 'user-avatar-decoration__layer--active': layer.isActive }"
        :src="layer.src"
        :style="layer.style"
        data-decoration-video="1"
        :data-decoration-id="layer.id"
        muted
        loop
        playsinline
        preload="metadata"
        disablepictureinpicture
        disableremoteplayback
        @error="handleLayerVideoError(layer.id)"
      ></video>
    </template>
  </div>
</template>

<style scoped>
.user-avatar-decoration {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  overflow: visible;
  flex-shrink: 0;
}

.user-avatar-decoration__layer {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  pointer-events: none;
  user-select: none;
  -webkit-user-drag: none;
  transform-origin: center;
  object-fit: contain;
  background: transparent;
}

.user-avatar-decoration__layer--background {
  z-index: 0;
}

.user-avatar-decoration :deep(.avatar-shell) {
  z-index: 1;
}

.user-avatar-decoration__layer--foreground {
  z-index: 2;
}

.user-avatar-decoration__layer--active {
  filter: drop-shadow(0 0 10px rgba(59, 130, 246, 0.6));
}
</style>
