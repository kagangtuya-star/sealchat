<script setup lang="ts">
import { computed, type CSSProperties } from 'vue'
import { normalizeStageIframeContent, resolveSafeStageIframeUrl, type StageObject } from '../shared/stage-types'

const props = defineProps<{
  object: StageObject
  editing: boolean
}>()

const iframeContent = computed(() => normalizeStageIframeContent(props.object.content?.iframe))
const configuredUrl = computed(() => iframeContent.value.url)
const iframeSrc = computed(() => resolveSafeStageIframeUrl(configuredUrl.value))
const pointerEvents = computed<'auto' | 'none'>(() => (
  props.editing || !props.object.interactive ? 'none' : 'auto'
))
const frameStyle = computed<CSSProperties>(() => ({
  width: `${100 / iframeContent.value.scale}%`,
  height: `${100 / iframeContent.value.scale}%`,
  transform: `scale(${iframeContent.value.scale})`,
  transformOrigin: 'top left',
  pointerEvents: pointerEvents.value,
}))
</script>

<template>
  <div class="theater-iframe-visual-object" :style="{ pointerEvents }">
    <iframe
      v-if="iframeSrc"
      class="theater-iframe-visual-object__frame"
      :src="iframeSrc"
      :title="props.object.name || '网页内容'"
      :style="frameStyle"
      allow="autoplay; fullscreen; microphone; camera; clipboard-read; clipboard-write"
      sandbox="allow-same-origin allow-scripts allow-forms allow-pointer-lock allow-popups"
      referrerpolicy="no-referrer"
      loading="lazy"
    ></iframe>
    <span v-else-if="props.editing" class="theater-iframe-visual-object__placeholder">
      {{ configuredUrl ? '仅支持 HTTP/HTTPS URL' : '请配置 URL' }}
    </span>
  </div>
</template>

<style scoped>
.theater-iframe-visual-object {
  width: 100%;
  height: 100%;
  box-sizing: border-box;
  margin: 0;
  padding: 0;
  overflow: hidden;
  border: 0;
  border-radius: 0;
  background: transparent;
  box-shadow: none;
}

.theater-iframe-visual-object__frame {
  display: block;
  width: 100%;
  height: 100%;
  margin: 0;
  padding: 0;
  border: 0;
  background: transparent;
}

.theater-iframe-visual-object__placeholder {
  display: flex;
  width: 100%;
  height: 100%;
  align-items: center;
  justify-content: center;
  color: rgba(226, 232, 240, 0.72);
  font-size: 14px;
  pointer-events: none;
}
</style>
