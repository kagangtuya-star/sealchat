<script setup lang="tsx">
import imgAvatar from '@/assets/head3.png'
import { computed, onMounted, ref } from 'vue';
import { onLongPress } from '@vueuse/core'
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver';

const props = defineProps({
  src: String,
  size: {
    type: Number,
    default: 48,
  },
  border: {
    type: Boolean,
    default: true,
  },
})

const opacity = ref(0)
const onload = function () {
  opacity.value = 0;
}

onMounted(() => {
})

const resolvedSrc = computed(() => {
  const url = resolveAttachmentUrl(props.src);
  if (!url) {
    opacity.value = 1;
  }
  return url;
})

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
    :style="{ width: `${size}px`, height: `${size}px`, 'min-width': `${size}px`, 'min-height': `${size}px` }"
    @contextmenu.prevent
    @dragstart.prevent
  >
    <img class="avatar-img" :src="resolvedSrc" v-if="resolvedSrc" :onload="onload" draggable="false" />
    <img class="avatar-img avatar-img--fallback" :src="imgAvatar" :style="{ opacity: opacity }" draggable="false" />
  </div>
</template>

<style scoped>
.avatar-shell {
  position: relative;
  overflow: hidden;
  border-radius: 0.85rem;
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
  position: absolute;
  top: 0;
  left: 0;
}

.avatar-shell--bordered {
  border: 1px solid rgba(148, 163, 184, 0.6);
  background-color: #ffffff;
}

.avatar-shell--plain {
  border: none;
  background: transparent;
}
</style>
