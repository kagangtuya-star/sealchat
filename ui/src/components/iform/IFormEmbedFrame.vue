<template>
  <div
    class="iform-frame"
    :class="{
      'has-embed': hasEmbed,
      'has-inline-embed': isSingleIframeEmbed,
      'has-srcdoc-embed': isSrcDocEmbed,
      'has-url': !hasEmbed && !!form?.url,
    }"
  >
    <div v-if="isSingleIframeEmbed" class="iform-frame__html" v-html="sanitizedIframeEmbed"></div>
    <iframe
      v-else-if="hasEmbed"
      class="iform-frame__iframe iform-frame__iframe--embed"
      :srcdoc="embedSrcDoc"
      allow="autoplay; fullscreen; microphone; camera; clipboard-read; clipboard-write"
      sandbox="allow-scripts allow-forms allow-pointer-lock allow-popups allow-modals allow-downloads"
      referrerpolicy="no-referrer"
    ></iframe>
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

const embedCode = computed(() => props.form?.embedCode?.trim() || '');
const hasEmbed = computed(() => embedCode.value.length > 0);

const isSingleIframeEmbed = computed(() => {
  if (!hasEmbed.value) {
    return false;
  }
  const normalized = embedCode.value.replace(/<!--[\s\S]*?-->/g, '').trim();
  return /^<iframe\b[\s\S]*<\/iframe>$/i.test(normalized);
});

const sanitizedIframeEmbed = computed(() => {
  if (!isSingleIframeEmbed.value) {
    return '';
  }
  return DOMPurify.sanitize(embedCode.value, {
    ADD_ATTR: ['allow', 'allowfullscreen', 'frameborder', 'referrerpolicy', 'loading'],
    ADD_TAGS: ['iframe'],
  });
});

const isSrcDocEmbed = computed(() => hasEmbed.value && !isSingleIframeEmbed.value);

const embedSrcDoc = computed(() => {
  if (!hasEmbed.value) {
    return '';
  }
  if (isSingleIframeEmbed.value) {
    return '';
  }
  const raw = embedCode.value;
  const hasHtmlShell = /<(?:!doctype|html|head|body)\b/i.test(raw);
  if (hasHtmlShell) {
    return raw;
  }
  return [
    '<!doctype html>',
    '<html><head><meta charset="utf-8">',
    '<meta name="viewport" content="width=device-width, initial-scale=1">',
    '<style>',
    'html,body{margin:0;padding:0;width:100%;height:100%;overflow:auto;background:transparent;border:0;outline:0;scrollbar-width:thin;scrollbar-color:rgba(148,163,184,.42) transparent;}',
    '*{box-sizing:border-box;}',
    '*::-webkit-scrollbar{width:6px;height:6px;}',
    '*::-webkit-scrollbar-track{background:transparent;}',
    '*::-webkit-scrollbar-thumb{background:rgba(148,163,184,.4);border-radius:999px;}',
    '*::-webkit-scrollbar-thumb:hover{background:rgba(148,163,184,.62);}',
    '</style>',
    '</head><body>',
    raw,
    '</body></html>',
  ].join('');
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

.iform-frame__iframe {
  width: 100%;
  height: 100%;
  border: none;
  display: block;
  background: transparent;
}

.iform-frame__html {
  width: 100%;
  height: 100%;
}

.iform-frame__html :deep(iframe) {
  display: block;
  border: none;
}

.iform-frame__empty {
  display: flex;
  align-items: center;
  justify-content: center;
  height: 100%;
}

.iform-frame.has-embed {
  overflow: hidden;
}

.iform-frame.has-srcdoc-embed {
  border: none;
  background: transparent;
}

.iform-frame.has-inline-embed {
  align-items: flex-start;
  justify-content: flex-start;
  overflow: visible;
}

.iform-frame.has-inline-embed .iform-frame__html {
  width: auto;
  height: auto;
  overflow: visible;
}
</style>
