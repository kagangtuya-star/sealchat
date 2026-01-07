<template>
  <IFormEmbedInstance v-for="windowId in activeWindowIds" :key="windowId" :window-id="windowId" />
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useIFormStore } from '@/stores/iform';
import IFormEmbedInstance from './IFormEmbedInstance.vue';

const iform = useIFormStore();
iform.bootstrap();

const activeWindowIds = computed(() => iform.activeEmbedWindowIds);
</script>
