<script setup lang="ts">
// Route entry for #/worlds/:worldId: only picks the modern or legacy world detail UI.
import { computed } from 'vue';
import { useRoute } from 'vue-router';
import { useUiVariant } from '@/composables/useUiVariant';
import WorldDetailLegacy from './WorldDetailLegacy.vue';
import WorldDetailModern from './detail/WorldDetailModern.vue';

const route = useRoute();
const worldId = computed(() => String(route.params.worldId || ''));
const managementTarget = computed(() => {
  const target = typeof route.query.open === 'string' ? route.query.open : '';
  return target === 'log' || target === 'observer' || target === 'ui' ? target : '';
});
const { isLegacy, setVariant } = useUiVariant('world-detail');
</script>

<template>
  <!-- Legacy loads once on mount; keying by worldId keeps /worlds/A -> /worlds/B correct without rewriting it. -->
  <WorldDetailLegacy v-if="isLegacy && !managementTarget" :key="worldId" @switch-variant="setVariant('modern')" />
  <WorldDetailModern v-else :world-id="worldId" @switch-variant="setVariant('legacy')" />
</template>
