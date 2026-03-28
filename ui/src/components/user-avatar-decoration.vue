<script setup lang="ts">
import { computed, ref, watch, useAttrs } from 'vue'
import type { CSSProperties } from 'vue'
import Avatar from '@/components/avatar.vue'
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver'
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
}>(), {
  src: '',
  size: 0,
  border: true,
  fallbackText: '',
  useTextFallback: false,
  decoration: null,
  decorationEnabled: true,
})

const attrs = useAttrs()
const display = useDisplayStore()
const resourceLoadFailed = ref(false)

watch(() => props.decoration?.resourceAttachmentId, () => {
  resourceLoadFailed.value = false
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

const resourceSrc = computed(() => resolveAttachmentUrl(props.decoration?.resourceAttachmentId || ''))
const fallbackSrc = computed(() => resolveAttachmentUrl(props.decoration?.fallbackAttachmentId || ''))
const effectiveDecorationSrc = computed(() => {
  if (display.settings.preferStaticAvatarDecoration && fallbackSrc.value) {
    return fallbackSrc.value
  }
  if (!resourceLoadFailed.value && resourceSrc.value) {
    return resourceSrc.value
  }
  return fallbackSrc.value
})

const shouldRenderDecoration = computed(() => (
  props.decorationEnabled
  && props.decoration?.enabled === true
  && Boolean(effectiveDecorationSrc.value)
))

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

const handleDecorationError = () => {
  if (!resourceLoadFailed.value && resourceSrc.value) {
    resourceLoadFailed.value = true
  }
}
</script>

<template>
  <div
    class="user-avatar-decoration"
    :style="shellStyle"
    v-bind="attrs"
  >
    <img
      v-if="shouldRenderDecoration && isBackgroundDecoration"
      class="user-avatar-decoration__layer user-avatar-decoration__layer--background"
      :src="effectiveDecorationSrc"
      :style="decorationLayerStyle"
      draggable="false"
      @error="handleDecorationError"
    />
    <Avatar
      :src="src"
      :size="size"
      :border="border"
      :fallback-text="fallbackText"
      :use-text-fallback="useTextFallback"
    />
    <img
      v-if="shouldRenderDecoration && !isBackgroundDecoration"
      class="user-avatar-decoration__layer user-avatar-decoration__layer--foreground"
      :src="effectiveDecorationSrc"
      :style="decorationLayerStyle"
      draggable="false"
      @error="handleDecorationError"
    />
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
