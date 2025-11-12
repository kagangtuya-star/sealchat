<template>
  <div ref="hostEl" class="iform-embed-portal"></div>
</template>

<script setup lang="ts">
import { ref, watchEffect } from 'vue';
import { useIFormStore } from '@/stores/iform';

const props = defineProps<{
  formId: string;
  surface: 'panel' | 'floating' | 'drawer';
}>();

const hostEl = ref<HTMLElement | null>(null);
const iform = useIFormStore();
iform.bootstrap();

watchEffect((onCleanup) => {
  const host = hostEl.value;
  if (!host || !props.formId) {
    return;
  }
  iform.registerEmbedHost(props.formId, host, props.surface);
  onCleanup(() => {
    iform.unregisterEmbedHost(props.formId, props.surface, host);
  });
});
</script>

<style scoped>
.iform-embed-portal {
  width: 100%;
  height: 100%;
  position: relative;
}
</style>
