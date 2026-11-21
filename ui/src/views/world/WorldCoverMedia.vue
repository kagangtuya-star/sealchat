<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { fetchAttachmentMetaById, normalizeAttachmentId, resolveAttachmentUrl } from '@/composables/useAttachmentResolver';

const props = defineProps<{ attachment?: string; alt?: string }>();
const url = computed(() => resolveAttachmentUrl(props.attachment, { fetchMeta: false }));
const mimeType = ref<string>();
const failed = ref(false);

watch(() => props.attachment, async (attachment, _previous, onCleanup) => {
  mimeType.value = undefined;
  failed.value = false;
  const id = normalizeAttachmentId(attachment || '');
  if (!id) return;
  if (id.includes('/')) {
    mimeType.value = '';
    return;
  }

  let stale = false;
  onCleanup(() => { stale = true; });
  const meta = await fetchAttachmentMetaById(id);
  if (!stale) mimeType.value = meta?.mimeType?.trim().toLowerCase() || '';
}, { immediate: true });
</script>

<template>
  <div v-if="url && !failed" :key="url">
    <video
      v-if="mimeType === 'video/webm'"
      class="world-cover-media__asset"
      :src="url"
      :aria-label="alt || undefined"
      :aria-hidden="alt ? undefined : true"
      muted
      autoplay
      loop
      playsinline
      preload="metadata"
      @error="failed = true"
    />
    <img
      v-else-if="mimeType !== undefined"
      class="world-cover-media__asset"
      :src="url"
      :alt="alt || ''"
      decoding="async"
      @error="failed = true"
    />
  </div>
</template>

<style scoped>
.world-cover-media__asset {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}
</style>
