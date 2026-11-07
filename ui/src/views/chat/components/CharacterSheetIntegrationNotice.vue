<script setup lang="ts">
import { computed, watch } from 'vue';
import { NButton } from 'naive-ui';
import { useCharacterSheetIntegrationStore } from '@/stores/characterSheetIntegration';
import {
  getCharacterSheetTemplateIntegration,
  getCharacterSheetTypeMeta,
  parseCharacterSheetTemplateManifest,
} from '@/utils/characterSheetType';

const props = withDefaults(defineProps<{
  sheetType?: string;
  templateHtml?: string;
  channelId?: string;
  autoCheck?: boolean;
}>(), {
  autoCheck: false,
});

const integrationStore = useCharacterSheetIntegrationStore();
const manifest = computed(() => parseCharacterSheetTemplateManifest(props.templateHtml));
const meta = computed(() => getCharacterSheetTypeMeta(props.sheetType));
const integration = computed(() => getCharacterSheetTemplateIntegration(props.templateHtml, props.sheetType));
const status = computed(() => integrationStore.getStatus(
  props.sheetType || '',
  props.templateHtml || '',
  props.channelId,
));
const cacheKey = computed(() => integrationStore.getCacheKey(
  props.sheetType || '',
  props.templateHtml || '',
  props.channelId,
));
const displayLabel = computed(() => manifest.value?.label || meta.value?.label || '此');
const acquireUrl = computed(() => {
  const url = integration.value?.plugin.acquireUrl;
  if (!url) return undefined;
  try {
    const parsed = new URL(url);
    return ['https:', 'http:'].includes(parsed.protocol) ? parsed.href : undefined;
  } catch {
    return undefined;
  }
});
const showManualPrompt = computed(() => (
  !!integration.value
  && !props.autoCheck
  && status.value !== 'available'
  && status.value !== 'missing'
));

const check = (force = false) => integrationStore.check(
  props.sheetType || '',
  props.templateHtml || '',
  props.channelId,
  force,
);
watch(cacheKey, () => {
  if (integration.value && props.autoCheck) void check();
}, { immediate: true });
</script>

<template>
  <div v-if="integration && status === 'missing'" class="sheet-integration-notice sheet-integration-notice--error" role="alert" aria-live="polite">
    <div>未检测到「{{ integration.plugin.name }}」，部分人物卡功能可能无法正常使用。</div>
    <div v-if="integration.plugin.description">{{ integration.plugin.description }}</div>
    <div v-if="integration.plugin.acquireText">获取方式：{{ integration.plugin.acquireText }}。</div>
    <a v-if="acquireUrl" :href="acquireUrl" target="_blank" rel="noopener noreferrer">获取插件</a>
    <n-button text size="tiny" @click="check(true)">重新检测</n-button>
  </div>
  <div v-else-if="showManualPrompt" class="sheet-integration-notice" role="status" aria-live="polite">
    <div v-if="status === 'checking'">正在检测「{{ integration?.plugin.name }}」……</div>
    <template v-else>
      <div>{{ displayLabel }}人物卡模板声明需要 SealDice「{{ integration?.plugin.name }}」。</div>
      <div v-if="status === 'unavailable'">暂时无法确认插件状态。</div>
      <div v-if="integration?.plugin.acquireText">获取方式：{{ integration.plugin.acquireText }}。</div>
      <a v-if="acquireUrl" :href="acquireUrl" target="_blank" rel="noopener noreferrer">获取插件</a>
      <n-button text size="tiny" @click="check(true)">{{ status === 'unknown' ? '检测插件' : '重新检测' }}</n-button>
    </template>
  </div>
</template>

<style scoped>
.sheet-integration-notice {
  flex-shrink: 0;
  margin: 8px 12px;
  padding-left: 8px;
  border-left: 2px solid var(--sc-border-color);
  color: var(--sc-text-secondary);
  font-size: 12px;
  line-height: 1.6;
  overflow-wrap: anywhere;
}
.sheet-integration-notice--error {
  color: var(--sc-text-secondary);
}
.sheet-integration-notice a {
  color: inherit;
  margin-right: 8px;
}
</style>
