<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver'
import { useDisplayStore } from '@/stores/display'
import type { TheaterMediaRef } from '@/types/theaterPresentation'
import { resolveTheaterMediaCandidates } from './theaterPresentationMedia'

const props = withDefaults(defineProps<{
  media: TheaterMediaRef
  playbackRate?: number
  active?: boolean
}>(), { playbackRate: 1, active: true })

const display = useDisplayStore()
const failedIndex = ref(-1)
const supportsVideo = ref(true)
const videoRef = ref<HTMLVideoElement | null>(null)

watch(() => [props.media.resourceAttachmentId, props.media.fallbackAttachmentId], () => {
  failedIndex.value = -1
})
watch(() => [display.settings.preferStaticAvatarDecoration, supportsVideo.value], () => {
  failedIndex.value = -1
})
watch(() => props.playbackRate, (value) => {
  if (!videoRef.value) return
  videoRef.value.playbackRate = value
  videoRef.value.defaultPlaybackRate = value
})
watch(() => props.active, (active) => {
  if (!videoRef.value) return
  if (active) void videoRef.value.play().catch(() => undefined)
  else videoRef.value.pause()
})

onMounted(() => {
  const video = document.createElement('video')
  supportsVideo.value = video.canPlayType('video/webm; codecs="vp9"') !== '' || video.canPlayType('video/webm') !== ''
})

const candidates = computed(() => resolveTheaterMediaCandidates(props.media, {
  preferStatic: display.settings.preferStaticAvatarDecoration,
  supportsVideo: supportsVideo.value,
}))
const candidate = computed(() => candidates.value[failedIndex.value + 1] || null)
const src = computed(() => resolveAttachmentUrl(candidate.value?.attachmentId || ''))

const handleError = () => { failedIndex.value += 1 }
const handleVideoLoaded = () => {
  if (!videoRef.value) return
  videoRef.value.playbackRate = props.playbackRate
  videoRef.value.defaultPlaybackRate = props.playbackRate
  if (props.active) void videoRef.value.play().catch(() => undefined)
  else videoRef.value.pause()
}
</script>

<template>
  <video
    v-if="candidate?.kind === 'video' && src"
    ref="videoRef"
    class="theater-media"
    :src="src"
    style="object-fit: cover"
    muted
    autoplay
    loop
    playsinline
    @loadeddata="handleVideoLoaded"
    @error="handleError"
  />
  <img
    v-else-if="candidate?.kind === 'image' && src"
    class="theater-media"
    :src="src"
    alt=""
    draggable="false"
    style="object-fit: cover"
    @error="handleError"
  >
</template>

<style scoped>
.theater-media {
  width: 100%;
  height: 100%;
  display: block;
  pointer-events: none;
  user-select: none;
}
</style>
