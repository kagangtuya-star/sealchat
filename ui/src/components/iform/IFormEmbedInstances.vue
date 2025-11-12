<template>
  <IFormEmbedInstance v-for="formId in activeFormIds" :key="formId" :form-id="formId" />
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useIFormStore } from '@/stores/iform';
import IFormEmbedInstance from './IFormEmbedInstance.vue';

const iform = useIFormStore();
iform.bootstrap();

const activeFormIds = computed(() => iform.activeEmbedFormIds);
</script>
