<template>
  <div class="iform-frame" :class="{ 'has-embed': hasEmbed, 'has-url': !hasEmbed && !!form?.url }">
    <div v-if="hasEmbed" class="iform-frame__html" v-html="sanitizedEmbed"></div>
    <iframe
      v-else-if="form?.url"
      class="iform-frame__iframe"
      :src="form.url"
      allow="autoplay; fullscreen; microphone; camera; clipboard-read; clipboard-write"
      sandbox="allow-same-origin allow-scripts allow-forms allow-pointer-lock allow-popups"
      referrerpolicy="no-referrer"
    ></iframe>
    <div v-else class="iform-frame__empty">
      <n-empty description="未配置 URL 或嵌入代码" size="small" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import DOMPurify from 'dompurify';
import type { ChannelIForm } from '@/types/iform';

const props = defineProps<{ form?: ChannelIForm | null }>();

const hasEmbed = computed(() => Boolean(props.form?.embedCode));

const sanitizedEmbed = computed(() => {
  if (!props.form?.embedCode) {
    return '';
  }
  return DOMPurify.sanitize(props.form.embedCode, {
    ADD_ATTR: ['allow', 'allowfullscreen', 'frameborder', 'referrerpolicy'],
    ADD_TAGS: ['iframe'],
  });
});
</script>

<style scoped>
.iform-frame {
  position: relative;
  width: 100%;
  height: 100%;
  background-color: var(--sc-bg-panel, rgba(15, 23, 42, 0.03));
  border-radius: 12px;
  overflow: hidden;
  border: 1px solid var(--sc-border-mute, rgba(15, 23, 42, 0.06));
  display: flex;
  align-items: stretch;
  justify-content: stretch;
}

.iform-frame__iframe,
.iform-frame__html {
  width: 100%;
  height: 100%;
  border: none;
  display: block;
}

.iform-frame__empty {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
}

.iform-frame.has-embed {
  align-items: flex-start;
  justify-content: flex-start;
  overflow: visible;
}

.iform-frame.has-embed .iform-frame__html {
  width: auto;
  height: auto;
  overflow: visible;
}
</style>
