<template>
  <teleport v-if="host && form" :to="host">
    <IFormEmbedFrame :form="form" />
  </teleport>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useIFormStore } from '@/stores/iform';
import IFormEmbedFrame from './IFormEmbedFrame.vue';
import type { ChannelIForm } from '@/types/iform';

const props = defineProps<{
  windowId: string;
}>();

const iform = useIFormStore();
iform.bootstrap();

const formId = computed(() => iform.getWindowFormId(props.windowId));
const host = computed<HTMLElement | null>(() => iform.resolveEmbedHost(props.windowId));
const form = computed<ChannelIForm | undefined>(() => {
  if (!formId.value) {
    return undefined;
  }
  return iform.getForm(iform.currentChannelId, formId.value);
});
</script>
