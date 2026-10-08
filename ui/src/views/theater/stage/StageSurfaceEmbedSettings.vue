<script setup lang="ts">
import { ref, watch } from 'vue'
import { NButton, NInput, NInputNumber, NSwitch } from 'naive-ui'
import { resolveSafeStageIframeUrl, STAGE_IFRAME_MIN_SCALE, STAGE_IFRAME_MAX_SCALE, type StageSurfaceEmbed, type StageSurfaceEmbedPatch } from '../shared/stage-types'

const props = defineProps<{ embed: StageSurfaceEmbed | null; testing: boolean }>()
const emit = defineEmits<{
  set: [embed: StageSurfaceEmbed]
  patch: [patch: StageSurfaceEmbedPatch]
  remove: []
  test: []
}>()
const urlDraft = ref(props.embed?.iframe.url || '')
const error = ref('')
watch(() => props.embed?.iframe.url || '', url => { urlDraft.value = url; error.value = '' })
const submitUrl = () => {
  const url = urlDraft.value.trim()
  if (!resolveSafeStageIframeUrl(url) || url.length > 8192) {
    error.value = '请输入有效的 HTTP/HTTPS URL 或当前频道 IForm 链接'
    return
  }
  error.value = ''
  if (props.embed) emit('patch', { iframe: { url } })
  else emit('set', { type: 'iframe', iframe: { url, scale: 1 }, interactive: false })
}
const updateScale = (value: number | null) => {
  if (value !== null && Number.isFinite(value)) emit('patch', { iframe: { scale: value / 100 } })
}
</script>

<template>
  <div class="theater-surface-embed-settings">
    <strong>网页</strong>
    <n-input v-model:value="urlDraft" size="small" placeholder="HTTP/HTTPS URL 或频道 IForm 链接" :status="error ? 'error' : undefined" @keydown.enter.prevent="submitUrl" />
    <small v-if="error" class="is-error">{{ error }}</small>
    <small v-else>点击应用或按 Enter 加载；图片与网页可同时显示。</small>
    <n-button size="tiny" @click="submitUrl">应用 URL</n-button>
    <template v-if="embed">
      <label>内容缩放
        <n-input-number :value="Math.round(embed.iframe.scale * 100)" :min="STAGE_IFRAME_MIN_SCALE * 100" :max="STAGE_IFRAME_MAX_SCALE * 100" size="small" @update:value="updateScale"><template #suffix>%</template></n-input-number>
      </label>
      <label>阻止交互<n-switch :value="!embed.interactive" size="small" @update:value="emit('patch', { interactive: !$event })" /></label>
      <small>编辑时默认穿透网页。测试仅影响本地；阻止交互不停止网页脚本或媒体。</small>
      <div class="theater-surface-embed-settings__actions">
        <n-button size="tiny" :type="testing ? 'warning' : 'default'" @click="emit('test')">{{ testing ? '退出网页测试' : '测试网页交互' }}</n-button>
        <n-button size="tiny" quaternary type="error" @click="emit('remove')">移除网页</n-button>
      </div>
    </template>
  </div>
</template>

<style scoped>
.theater-surface-embed-settings { display: grid; gap: 8px; min-width: 0; padding-top: 8px; border-top: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08)); }
.theater-surface-embed-settings strong { font-size: 12px; }
.theater-surface-embed-settings small { color: var(--sc-text-secondary, #b5b5c5); font-size: 11px; }
.theater-surface-embed-settings .is-error { color: #f87171; }
.theater-surface-embed-settings label { display: grid; grid-template-columns: 86px minmax(0, 1fr); align-items: center; gap: 8px; font-size: 12px; }
.theater-surface-embed-settings__actions { display: flex; gap: 8px; flex-wrap: wrap; }
</style>
