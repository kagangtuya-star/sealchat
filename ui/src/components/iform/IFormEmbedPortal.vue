<template>
  <div ref="hostEl" class="iform-embed-portal"></div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { useIFormStore } from '@/stores/iform';

const props = defineProps<{
  windowId: string;
  formId: string;
  surface: 'panel' | 'floating' | 'drawer';
}>();

const hostEl = ref<HTMLElement | null>(null);
const iform = useIFormStore();
iform.bootstrap();

watch(
  () => [hostEl.value, props.windowId, props.formId, props.surface] as const,
  ([host, windowId, formId, surface], _prev, onCleanup) => {
    if (!host || !formId || !windowId) {
      return;
    }
    iform.registerEmbedHost(windowId, formId, host, surface);
    onCleanup(() => {
      iform.unregisterEmbedHost(windowId, surface, host);
    });
  },
  { immediate: true },
);
</script>

<style scoped>
.iform-embed-portal {
  width: 100%;
  height: 100%;
  position: relative;
}
</style>
