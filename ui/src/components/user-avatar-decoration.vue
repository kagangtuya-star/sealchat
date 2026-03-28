<script setup lang="ts">
import { computed, ref, watch, useAttrs, onMounted, onBeforeUnmount, nextTick } from 'vue'
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
  decorationEnabled?: boolean
  pauseWhenOutOfView?: boolean
}>(), {
  src: '',
  size: 0,
  border: true,
  fallbackText: '',
  useTextFallback: false,
  decoration: null,
  decorationEnabled: true,
  pauseWhenOutOfView: true,
})

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
const videoRef = ref<HTMLVideoElement | null>(null)
const resourceMeta = ref<AttachmentMeta | null>(null)
const resourceMetaResolved = ref(false)
const transparentWebMSupported = ref(false)
const resourceLoadFailed = ref(false)
const fallbackLoadFailed = ref(false)
const isInViewport = ref(true)

let resourceMetaRequestId = 0
let viewportObserver: IntersectionObserver | null = null

const resetResourceState = () => {
  resourceLoadFailed.value = false
  resourceMeta.value = null
  resourceMetaResolved.value = false
}

const resetFallbackState = () => {
  fallbackLoadFailed.value = false
}

watch(() => props.decoration?.resourceAttachmentId, async (value) => {
  resetResourceState()
  const requestId = ++resourceMetaRequestId
  const normalized = normalizeAttachmentId(value || '')
  if (!normalized) {
    resourceMetaResolved.value = true
    return
  }
  const meta = await fetchAttachmentMetaById(normalized)
  if (requestId !== resourceMetaRequestId) {
    return
  }
  resourceMeta.value = meta
  resourceMetaResolved.value = true
}, { immediate: true })

watch(() => props.decoration?.fallbackAttachmentId, () => {
  resetFallbackState()
})

const decorationSettings = computed(() => ({
  scale: props.decoration?.settings?.scale ?? 1,
  offsetX: props.decoration?.settings?.offsetX ?? 0,
  offsetY: props.decoration?.settings?.offsetY ?? 0,
  rotation: props.decoration?.settings?.rotation ?? 0,
  zIndex: props.decoration?.settings?.zIndex ?? 1,
  opacity: props.decoration?.settings?.opacity ?? 1,
  blendMode: props.decoration?.settings?.blendMode ?? 'normal',
}))

const resourceMime = computed(() => String(resourceMeta.value?.mimeType || '').trim().toLowerCase())
const resourceSrc = computed(() => resolveAttachmentUrl(props.decoration?.resourceAttachmentId || ''))
const fallbackSrc = computed(() => resolveAttachmentUrl(props.decoration?.fallbackAttachmentId || ''))
const availableFallbackSrc = computed(() => (fallbackLoadFailed.value ? '' : fallbackSrc.value))
const shouldPreferStaticFallback = computed(() => (
  display.settings.preferStaticAvatarDecoration
  && resourceMime.value === 'video/webm'
))

const effectiveDecorationKind = computed<'none' | 'image' | 'video'>(() => {
  if (!props.decorationEnabled || props.decoration?.enabled !== true) {
    return 'none'
  }
  if (!resourceSrc.value || resourceLoadFailed.value) {
    return availableFallbackSrc.value ? 'image' : 'none'
  }
  if (!resourceMetaResolved.value) {
    return 'none'
  }
  if (resourceMime.value === 'video/webm') {
    if (shouldPreferStaticFallback.value) {
      return availableFallbackSrc.value ? 'image' : 'none'
    }
    if (!transparentWebMSupported.value) {
      return availableFallbackSrc.value ? 'image' : 'none'
    }
    return 'video'
  }
  if (resourceMime.value === 'image/png' || resourceMime.value === 'image/webp') {
    return 'image'
  }
  return availableFallbackSrc.value ? 'image' : 'none'
})

const effectiveImageSrc = computed(() => {
  if (effectiveDecorationKind.value !== 'image') {
    return ''
  }
  if (availableFallbackSrc.value && (resourceLoadFailed.value || shouldPreferStaticFallback.value || resourceMime.value === 'video/webm')) {
    return availableFallbackSrc.value
  }
  return resourceSrc.value || availableFallbackSrc.value
})

const effectiveVideoSrc = computed(() => (
  effectiveDecorationKind.value === 'video' ? resourceSrc.value : ''
))

const shouldRenderImageDecoration = computed(() => Boolean(effectiveImageSrc.value))
const shouldRenderVideoDecoration = computed(() => Boolean(effectiveVideoSrc.value))

const decorationLayerStyle = computed<CSSProperties>(() => {
  const settings = decorationSettings.value
  return {
    transform: `translate(${settings.offsetX}px, ${settings.offsetY}px) scale(${settings.scale}) rotate(${settings.rotation}deg)`,
    opacity: `${settings.opacity}`,
    mixBlendMode: settings.blendMode as CSSProperties['mixBlendMode'],
  }
})

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

const isBackgroundDecoration = computed(() => decorationSettings.value.zIndex < 0)
const shouldPlayVideo = computed(() => (
  shouldRenderVideoDecoration.value
  && (!props.pauseWhenOutOfView || isInViewport.value)
))

const handleImageError = () => {
  if (effectiveImageSrc.value === resourceSrc.value) {
    resourceLoadFailed.value = true
    return
  }
  fallbackLoadFailed.value = true
}

const handleVideoError = () => {
  resourceLoadFailed.value = true
}

const updateVideoPlayback = async () => {
  await nextTick()
  const video = videoRef.value
  if (!video) {
    return
  }
  if (!shouldPlayVideo.value) {
    video.pause()
    return
  }
  try {
    await video.play()
  } catch {
    // Ignore autoplay failures. In unsupported environments we already degrade conservatively.
  }
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
  }, {
    threshold: 0.05,
  })
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
  videoRef.value?.pause()
})

watch(() => props.pauseWhenOutOfView, () => {
  setupViewportObserver()
  void updateVideoPlayback()
})

watch(() => rootRef.value, () => {
  setupViewportObserver()
})

watch([
  shouldPlayVideo,
  effectiveVideoSrc,
], () => {
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
    <img
      v-if="shouldRenderImageDecoration && isBackgroundDecoration"
      class="user-avatar-decoration__layer user-avatar-decoration__layer--background"
      :src="effectiveImageSrc"
      :style="decorationLayerStyle"
      draggable="false"
      @error="handleImageError"
    />
    <video
      v-else-if="shouldRenderVideoDecoration && isBackgroundDecoration"
      ref="videoRef"
      class="user-avatar-decoration__layer user-avatar-decoration__layer--background"
      :src="effectiveVideoSrc"
      :style="decorationLayerStyle"
      muted
      loop
      playsinline
      preload="metadata"
      disablepictureinpicture
      disableremoteplayback
      @error="handleVideoError"
    ></video>
    <Avatar
      :src="src"
      :size="size"
      :border="border"
      :fallback-text="fallbackText"
      :use-text-fallback="useTextFallback"
    />
    <img
      v-if="shouldRenderImageDecoration && !isBackgroundDecoration"
      class="user-avatar-decoration__layer user-avatar-decoration__layer--foreground"
      :src="effectiveImageSrc"
      :style="decorationLayerStyle"
      draggable="false"
      @error="handleImageError"
    />
    <video
      v-else-if="shouldRenderVideoDecoration && !isBackgroundDecoration"
      ref="videoRef"
      class="user-avatar-decoration__layer user-avatar-decoration__layer--foreground"
      :src="effectiveVideoSrc"
      :style="decorationLayerStyle"
      muted
      loop
      playsinline
      preload="metadata"
      disablepictureinpicture
      disableremoteplayback
      @error="handleVideoError"
    ></video>
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
</style>
