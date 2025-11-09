<script setup lang="tsx">
import imgAvatar from '@/assets/head3.png'
import { urlBase } from '@/stores/_config';
import { useUserStore } from '@/stores/user';
import { computed, onMounted, ref } from 'vue';
import { onLongPress } from '@vueuse/core'

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

const opacity = ref(1)
const onload = function () {
  opacity.value = 0;
}

onMounted(() => {
})

const resolvedSrc = computed(() => {
  const source = (props.src || '').trim();
  if (!source) {
    opacity.value = 1;
    return '';
  }
  if (/^(https?:|blob:|data:|\/\/)/i.test(source)) {
    return source;
  }
  const normalized = source.startsWith('id:') ? source.slice(3) : source;
  if (!normalized) {
    opacity.value = 1;
    return '';
  }
  return `${urlBase}/api/v1/attachment/${normalized}`;
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
  >
    <img class="w-full h-full" :src="resolvedSrc" v-if="resolvedSrc" :onload="onload" />
    <img class="absolute w-full h-full" :class="{ 'pointer-events-none': opacity === 0 }" :src="imgAvatar" style="top:0" :style="{ opacity: opacity }" />
  </div>
</template>

<style scoped>
.avatar-shell {
  position: relative;
  overflow: hidden;
  border-radius: 0.85rem;
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
