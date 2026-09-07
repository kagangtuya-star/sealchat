<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{ url: string; title?: string }>()
const safeUrl = computed(() => {
  try {
    const parsed = new URL(props.url)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? parsed.toString() : ''
  } catch {
    return ''
  }
})
</script>

<template>
  <div class="clue-frame">
    <iframe
      v-if="safeUrl"
      :src="safeUrl"
      :title="title || '线索交互内容'"
      sandbox="allow-scripts allow-forms allow-popups"
      referrerpolicy="no-referrer"
      loading="lazy"
    />
    <div v-else class="clue-frame__fallback">无效的网页地址</div>
    <div v-if="safeUrl" class="clue-frame__footer">
      <span>网页可能禁止嵌入。</span>
      <a :href="safeUrl" target="_blank" rel="noopener noreferrer">在新窗口打开</a>
    </div>
  </div>
</template>

<style scoped>
.clue-frame { display: flex; min-height: 280px; flex-direction: column; overflow: hidden; border: 1px solid var(--sc-border, rgba(127, 127, 127, .28)); background: var(--sc-surface, rgba(20, 20, 24, .92)); }
.clue-frame iframe { min-height: 280px; flex: 1; border: 0; background: white; }
.clue-frame__footer { display: flex; justify-content: space-between; gap: 12px; padding: 8px 10px; font-size: 12px; color: var(--sc-text-muted, #888); }
.clue-frame__footer a { color: var(--sc-primary, #3388de); }
.clue-frame__fallback { display: grid; min-height: 280px; place-items: center; color: var(--sc-text-muted, #888); }
</style>
