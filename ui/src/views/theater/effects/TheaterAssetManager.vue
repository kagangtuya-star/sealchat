<script setup lang="ts">
import { computed, ref } from 'vue'
import { NButton, NIcon, NProgress, NSlider, NTabPane, NTabs, NTooltip } from 'naive-ui'
import { Box, Music, PlayerPlay, Refresh, Trash, Upload } from '@vicons/tabler'

import type { AudioAsset, AudioQuotaSummary } from '@/types/audio'

const props = defineProps<{
  assets: AudioAsset[]
  quota: AudioQuotaSummary | null
  loading: boolean
  uploading: boolean
  error: string
  canUpload: boolean
  canDelete: boolean
  referencedAssetIds: string[]
  masterVolume: number
}>()

const emit = defineEmits<{
  refresh: []
  upload: [file: File]
  preview: [asset: AudioAsset]
  delete: [asset: AudioAsset]
  'update:masterVolume': [value: number]
}>()

const inputRef = ref<HTMLInputElement | null>(null)
const referenced = computed(() => new Set(props.referencedAssetIds))
const quotaPercentage = computed(() => {
  if (!props.quota?.limited) return 0
  return Math.max(0, Math.min(100, props.quota.usagePercent ?? 0))
})
const quotaLabel = computed(() => {
  if (!props.quota) return ''
  if (!props.quota.limited) return '储存配额：无限制'
  return `储存配额：${formatBytes(props.quota.usedBytes)} / ${formatBytes(props.quota.quotaBytes || 0)}`
})

const pickFile = () => inputRef.value?.click()
const handleFile = (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (file) emit('upload', file)
}

const formatBytes = (value: number) => {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1024 / 1024).toFixed(1)} MB`
}

const formatDuration = (value: number) => {
  const seconds = Math.max(0, Math.round(value || 0))
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`
}

const assetStatus = (asset: AudioAsset) => {
  if (asset.transcodeStatus === 'pending') return '处理中'
  if (asset.transcodeStatus === 'failed') return '处理失败'
  return ''
}
</script>

<template>
  <div class="theater-asset-manager">
    <n-tabs type="line" size="small" animated>
      <n-tab-pane name="audio" tab="音频">
        <div class="theater-asset-manager__toolbar">
          <div class="theater-asset-manager__quota">
            <span>{{ quotaLabel || '储存配额加载中' }}</span>
            <n-progress v-if="quota?.limited" type="line" :percentage="quotaPercentage" :show-indicator="false" :height="3" />
          </div>
          <div class="theater-asset-manager__actions">
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-button quaternary size="tiny" :loading="loading" aria-label="刷新音频素材" @click="emit('refresh')">
                  <template #icon><n-icon><Refresh /></n-icon></template>
                </n-button>
              </template>
              刷新
            </n-tooltip>
            <n-button v-if="canUpload" size="tiny" secondary :loading="uploading" @click="pickFile">
              <template #icon><n-icon><Upload /></n-icon></template>
              上传
            </n-button>
            <input ref="inputRef" class="theater-asset-manager__input" type="file" accept="audio/ogg,audio/mpeg,audio/wav,.ogg,.mp3,.wav" @change="handleFile">
          </div>
        </div>

        <div class="theater-asset-manager__volume">
          <span>声音大小</span>
          <n-slider :value="masterVolume" :min="0" :max="1" :step="0.05" @update:value="emit('update:masterVolume', $event)" />
          <output>{{ Math.round(masterVolume * 100) }}%</output>
        </div>

        <p v-if="error" class="theater-asset-manager__error">{{ error }}</p>
        <div v-if="assets.length" class="theater-asset-manager__list">
          <div v-for="asset in assets" :key="asset.id" class="theater-asset-manager__row">
            <n-icon class="theater-asset-manager__kind"><Music /></n-icon>
            <div class="theater-asset-manager__meta">
              <strong :title="asset.name">{{ asset.name }}</strong>
              <span>{{ formatDuration(asset.duration) }} · {{ formatBytes(asset.size) }}<template v-if="assetStatus(asset)"> · {{ assetStatus(asset) }}</template></span>
            </div>
            <n-button quaternary circle size="tiny" :disabled="asset.transcodeStatus === 'pending' || asset.transcodeStatus === 'failed'" aria-label="试听音频" @click="emit('preview', asset)">
              <template #icon><n-icon><PlayerPlay /></n-icon></template>
            </n-button>
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-button
                  v-if="canDelete"
                  quaternary
                  circle
                  size="tiny"
                  type="error"
                  :disabled="referenced.has(asset.id)"
                  aria-label="删除音频素材"
                  @click="emit('delete', asset)"
                >
                  <template #icon><n-icon><Trash /></n-icon></template>
                </n-button>
              </template>
              {{ referenced.has(asset.id) ? '素材正被特效引用' : '删除' }}
            </n-tooltip>
          </div>
        </div>
        <div v-else-if="!loading" class="theater-asset-manager__empty">当前频道暂无特性音频</div>
      </n-tab-pane>

      <n-tab-pane name="reserved" tab="其他素材" disabled>
        <div class="theater-asset-manager__reserved">
          <n-icon><Box /></n-icon>
          <span>预留素材管理入口</span>
        </div>
      </n-tab-pane>
    </n-tabs>
  </div>
</template>

<style scoped>
.theater-asset-manager { min-height: 0; flex: 1; overflow: hidden; padding: 0 9px 9px; }
.theater-asset-manager :deep(.n-tabs), .theater-asset-manager :deep(.n-tabs-pane-wrapper), .theater-asset-manager :deep(.n-tab-pane) { height: 100%; min-height: 0; }
.theater-asset-manager :deep(.n-tab-pane) { display: flex; flex-direction: column; }
.theater-asset-manager__toolbar { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding: 5px 0 8px; }
.theater-asset-manager__quota { min-width: 0; flex: 1; display: grid; gap: 4px; color: var(--sc-text-secondary); font-size: 10px; }
.theater-asset-manager__actions { display: flex; align-items: center; gap: 4px; }
.theater-asset-manager__volume { display: grid; grid-template-columns: auto minmax(90px, 1fr) 34px; align-items: center; gap: 8px; padding: 0 0 8px; color: var(--sc-text-secondary); font-size: 10px; }
.theater-asset-manager__volume output { color: var(--sc-text-primary); text-align: right; }
.theater-asset-manager__input { display: none; }
.theater-asset-manager__list { min-height: 0; overflow: auto; border-top: 1px solid var(--theater-border); }
.theater-asset-manager__row { min-height: 48px; display: flex; align-items: center; gap: 8px; border-bottom: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08)); }
.theater-asset-manager__kind { flex: 0 0 auto; color: var(--theater-accent); font-size: 18px; }
.theater-asset-manager__meta { min-width: 0; flex: 1; display: grid; gap: 2px; }
.theater-asset-manager__meta strong { overflow: hidden; color: var(--sc-text-primary); font-size: 11px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
.theater-asset-manager__meta span { color: var(--sc-text-secondary); font-size: 9px; }
.theater-asset-manager__empty, .theater-asset-manager__reserved { min-height: 160px; display: grid; place-content: center; justify-items: center; gap: 7px; color: var(--sc-text-secondary); font-size: 11px; }
.theater-asset-manager__reserved .n-icon { font-size: 24px; }
.theater-asset-manager__error { margin: 0 0 7px; color: #f87171; font-size: 10px; }
</style>
