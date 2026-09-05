<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch, type CSSProperties } from 'vue'
import IFormEmbedFrame from '@/components/iform/IFormEmbedFrame.vue'
import { useChatStore } from '@/stores/chat'
import { useIFormStore } from '@/stores/iform'
import { parseInternalSurfaceLink } from '@/utils/internalSurfaceLink'
import { normalizeStageIframeContent, resolveSafeStageIframeUrl, type StageObject } from '../shared/stage-types'

const props = defineProps<{
  object: StageObject
}>()

const chat = useChatStore()
const iformStore = useIFormStore()
iformStore.bootstrap()
const iframeContent = computed(() => normalizeStageIframeContent(props.object.content?.iframe))
const configuredUrl = computed(() => iframeContent.value.url)
const iframeSrc = computed(() => resolveSafeStageIframeUrl(configuredUrl.value))
const internalIFormTarget = computed(() => {
  if (!iframeSrc.value || typeof window === 'undefined') return null
  try {
    const url = new URL(iframeSrc.value)
    if (url.origin !== window.location.origin) return null
    const parsed = parseInternalSurfaceLink(url.href)
    return parsed?.type === 'iform' ? parsed : null
  } catch {
    return null
  }
})
const internalIFormContextMatches = computed(() => {
  const target = internalIFormTarget.value
  if (!target) return false
  return (
    String(chat.currentWorldId || '') === target.worldId
    && String(chat.curChannel?.id || '') === target.channelId
  )
})
const directIForm = computed(() => {
  const target = internalIFormTarget.value
  if (!target || !internalIFormContextMatches.value) return null
  return (iformStore.formsByChannel[target.channelId] || [])
    .find(item => item.id === target.id) || null
})
const directIFormState = ref<'idle' | 'loading' | 'loaded' | 'error'>('idle')
let loadEpoch = 0

watch(
  () => [
    internalIFormTarget.value?.id || '',
    internalIFormTarget.value?.worldId || '',
    internalIFormTarget.value?.channelId || '',
    internalIFormContextMatches.value,
  ] as const,
  async ([formId, , channelId, contextMatches]) => {
    const epoch = ++loadEpoch
    directIFormState.value = 'idle'
    if (!formId || !channelId || !contextMatches) return
    directIFormState.value = 'loading'
    try {
      const hadCachedForms = iformStore.hasLoadedForms(channelId)
      await iformStore.ensureForms(channelId)
      if (epoch !== loadEpoch) return
      const hasTargetForm = () => (
        (iformStore.formsByChannel[channelId] || [])
          .some(item => item.id === formId)
      )
      if (hadCachedForms && !hasTargetForm()) {
        await iformStore.ensureForms(channelId, true)
        if (epoch !== loadEpoch) return
      }
      directIFormState.value = 'loaded'
    } catch {
      if (epoch !== loadEpoch) return
      directIFormState.value = 'error'
    }
  },
  { immediate: true },
)

onBeforeUnmount(() => { loadEpoch += 1 })

const pointerEvents = computed<'auto' | 'none'>(() => (
  props.object.interactive ? 'auto' : 'none'
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
    <IFormEmbedFrame
      v-if="internalIFormTarget && internalIFormContextMatches && directIForm"
      class="theater-iframe-visual-object__iform"
      :form="directIForm"
      :channel-id="internalIFormTarget.channelId"
      :enable-channel-embed="true"
      :style="frameStyle"
    />
    <span
      v-else-if="internalIFormTarget"
      class="theater-iframe-visual-object__placeholder"
    >
      {{
        !internalIFormContextMatches
          ? 'IForm 链接与当前频道不匹配'
          : directIFormState === 'loading'
            ? '正在加载 IForm'
            : directIFormState === 'error'
              ? 'IForm 加载失败'
              : 'IForm 不存在或当前用户不可见'
      }}
    </span>
    <iframe
      v-else-if="iframeSrc"
      class="theater-iframe-visual-object__frame"
      :src="iframeSrc"
      :title="props.object.name || '网页内容'"
      :data-stage-object-id="props.object.id"
      :style="frameStyle"
      allow="autoplay; fullscreen; microphone; camera; clipboard-read; clipboard-write"
      sandbox="allow-same-origin allow-scripts allow-forms allow-pointer-lock allow-popups"
      referrerpolicy="no-referrer"
      loading="lazy"
    ></iframe>
    <span v-else class="theater-iframe-visual-object__placeholder">
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

.theater-iframe-visual-object__iform {
  border: 0 !important;
  border-radius: 0 !important;
  background: transparent !important;
  background-color: transparent !important;
  box-shadow: none !important;
  overflow: hidden !important;
}

.theater-iframe-visual-object__iform :deep(.iform-frame__iframe),
.theater-iframe-visual-object__iform :deep(.iform-frame__html),
.theater-iframe-visual-object__iform :deep(.iform-frame__html > iframe) {
  display: block;
  width: 100% !important;
  height: 100% !important;
  min-width: 0;
  min-height: 0;
  margin: 0;
  padding: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  background-color: transparent;
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
