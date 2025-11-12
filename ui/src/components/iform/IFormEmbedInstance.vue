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
  formId: string;
}>();

const iform = useIFormStore();
iform.bootstrap();

const host = computed<HTMLElement | null>(() => iform.resolveEmbedHost(props.formId));
const form = computed<ChannelIForm | undefined>(() => iform.getForm(iform.currentChannelId, props.formId));
</script>
