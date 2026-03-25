<script setup lang="tsx">
import imgAvatar from '@/assets/head3.png'
import { computed, ref, watch } from 'vue';
import { onLongPress } from '@vueuse/core'
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';
import { useDisplayStore } from '@/stores/display';
import { buildGeneratedAvatarDataUrl } from '@/utils/generatedAvatarImage';

const props = withDefaults(defineProps<{
  src?: string;
  size?: number;
  border?: boolean;
  fallbackText?: string;
  useTextFallback?: boolean;
}>(), {
  src: '',
  size: 0,
  border: true,
  fallbackText: '',
  useTextFallback: false,
});

const resolvedSrc = computed(() => {
  return resolveAttachmentUrl(props.src);
});
const display = useDisplayStore();
const imageLoadFailed = ref(false);
watch(() => props.src, () => {
  imageLoadFailed.value = false;
});
const showImage = computed(() => Boolean(resolvedSrc.value) && !imageLoadFailed.value);
const normalizedFallbackText = computed(() => {
  const collapsed = String(props.fallbackText || '').replace(/\s+/g, '').trim();
  if (!collapsed) {
    return '匿';
  }
  return Array.from(collapsed).slice(0, 2).join('');
});
const showTextFallback = computed(() => !showImage.value && props.useTextFallback);
const generatedFallbackSrc = computed(() => {
  const themePalette = display.settings.palette;
  const customThemeEnabled = display.settings.customThemeEnabled;
  const activeCustomThemeId = display.settings.activeCustomThemeId;
  if (!props.useTextFallback || showImage.value) {
    return '';
  }
  return buildGeneratedAvatarDataUrl({
    displayName: props.fallbackText,
    size: props.size > 0 ? Math.max(props.size * 2, 96) : 128,
    themeSeed: {
      palette: themePalette,
      customThemeEnabled,
      activeCustomThemeId,
    },
  });
});
const showGeneratedFallback = computed(() => !showImage.value && Boolean(generatedFallbackSrc.value));

// Size style: use props.size if specified, otherwise inherit from CSS variable
const sizeStyle = computed(() => {
  if (props.size > 0) {
    return {
      width: `${props.size}px`,
      height: `${props.size}px`,
      minWidth: `${props.size}px`,
      minHeight: `${props.size}px`,
    }
  }
  // Inherit from CSS variable
  return {
    width: 'var(--chat-avatar-size, 48px)',
    height: 'var(--chat-avatar-size, 48px)',
    minWidth: 'var(--chat-avatar-size, 48px)',
    minHeight: 'var(--chat-avatar-size, 48px)',
  }
})
const handleImageError = () => {
  imageLoadFailed.value = true;
};

const emit = defineEmits(['longpress']);

const htmlRefHook = ref<HTMLElement | null>(null)
const onLongPressCallbackHook = (e: PointerEvent) => {
  emit('longpress', e)
}

onLongPress(
  htmlRefHook,
  onLongPressCallbackHook,
  { modifiers: { prevent: true } }
)
</script>

<template>
  <div
    ref="htmlRefHook"
    class="avatar-shell"
    :class="border ? 'avatar-shell--bordered' : 'avatar-shell--plain'"
    :style="sizeStyle"
    @contextmenu.prevent
    @dragstart.prevent
  >
    <img v-if="showImage" class="avatar-img" :src="resolvedSrc" draggable="false" @error="handleImageError" />
    <img v-else-if="showGeneratedFallback" class="avatar-img avatar-img--generated" :src="generatedFallbackSrc" draggable="false" />
    <div v-else-if="showTextFallback" class="avatar-text-fallback">{{ normalizedFallbackText }}</div>
    <img v-else class="avatar-img avatar-img--fallback" :src="imgAvatar" draggable="false" />
  </div>
</template>

<style scoped>
.avatar-shell {
  position: relative;
  overflow: hidden;
  border-radius: var(--chat-avatar-radius, 0.85rem);
  -webkit-touch-callout: none;
  -webkit-user-select: none;
  user-select: none;
  touch-action: manipulation;
}

.avatar-img {
  width: 100%;
  height: 100%;
  pointer-events: none;
  -webkit-touch-callout: none;
  -webkit-user-drag: none;
  user-select: none;
}

.avatar-img--fallback {
  display: block;
}

.avatar-shell--bordered {
  border: 1px solid rgba(148, 163, 184, 0.6);
  background-color: #ffffff;
}

:root[data-display-palette='night'] .avatar-shell--bordered {
  background-color: rgba(30, 41, 59, 0.95);
  border-color: rgba(148, 163, 184, 0.35);
}

.avatar-shell--plain {
  border: none;
  background: transparent;
}

.avatar-text-fallback {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background:
    radial-gradient(circle at top, rgba(255, 255, 255, 0.75), transparent 58%),
    linear-gradient(135deg, rgba(15, 23, 42, 0.9), rgba(71, 85, 105, 0.92));
  color: #f8fafc;
  font-weight: 700;
  letter-spacing: 0.04em;
  font-size: clamp(0.72rem, 0.38rem + 0.8vw, 1rem);
}

:root[data-display-palette='night'] .avatar-text-fallback {
  background:
    radial-gradient(circle at top, rgba(255, 255, 255, 0.12), transparent 58%),
    linear-gradient(135deg, rgba(148, 163, 184, 0.28), rgba(30, 41, 59, 0.96));
}
</style>
