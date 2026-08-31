<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { NButton, NIcon, NSpin } from 'naive-ui'
import { Edit, Photo, Trash, Upload, X } from '@vicons/tabler'

import type { TheaterImageAsset } from '../effects/theater-image-assets'

const props = defineProps<{
  assets: TheaterImageAsset[]
  loading: boolean
  uploading: boolean
  canUpload: boolean
  canEdit: boolean
  canDelete: boolean
  error: string
}>()

const emit = defineEmits<{
  select: [asset: TheaterImageAsset]
  upload: [files: File[]]
  rename: [assetId: string, name: string]
  delete: [asset: TheaterImageAsset]
  close: []
}>()

const inputRef = ref<HTMLInputElement | null>(null)
const editingAssetId = ref('')
const assetNameDraft = ref('')
const assetNameInputRef = ref<HTMLInputElement | null>(null)
const readyAssets = computed(() => props.assets.filter((asset) => asset.resource.status === 'ready'))
const autoPlayPreviews = computed(() => readyAssets.value.length <= 12)
const playbackMimeType = (asset: TheaterImageAsset) => asset.resource.playbackMimeType || asset.resource.mimeType
const isDynamic = (asset: TheaterImageAsset) => asset.resource.animated === true || playbackMimeType(asset) === 'video/webm'

const startAssetRename = (asset: TheaterImageAsset) => {
  editingAssetId.value = asset.id
  assetNameDraft.value = asset.name
  void nextTick(() => {
    assetNameInputRef.value?.focus()
    assetNameInputRef.value?.select()
  })
}

const finishAssetRename = (asset: TheaterImageAsset) => {
  if (editingAssetId.value !== asset.id) return
  const name = assetNameDraft.value.trim()
  editingAssetId.value = ''
  if (name && name !== asset.name) emit('rename', asset.id, name)
}

const handleFiles = (event: Event) => {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  if (files.length) emit('upload', files)
}

const playPreview = (event: MouseEvent | FocusEvent) => {
  const video = (event.currentTarget as HTMLElement).querySelector('video')
  if (video) void video.play().catch(() => undefined)
}

const pausePreview = (event: MouseEvent | FocusEvent) => {
  if (autoPlayPreviews.value) return
  const video = (event.currentTarget as HTMLElement).querySelector('video')
  video?.pause()
}
</script>

<template>
  <section class="scene-overlay-media-picker">
    <header>
      <strong>自定义效果</strong>
      <div>
        <n-button v-if="canUpload" size="tiny" secondary :loading="uploading" @click="inputRef?.click()">
          <template #icon><n-icon><Upload /></n-icon></template>
          上传素材
        </n-button>
        <n-button quaternary circle size="tiny" aria-label="关闭自定义效果选择器" @click="emit('close')">
          <template #icon><n-icon><X /></n-icon></template>
        </n-button>
        <input
          ref="inputRef"
          type="file"
          accept="image/jpeg,image/png,image/apng,image/webp,image/gif,video/webm,.jpg,.jpeg,.png,.apng,.webp,.gif,.webm"
          multiple
          @change="handleFiles"
        >
      </div>
    </header>

    <p v-if="error" class="scene-overlay-media-picker__error">{{ error }}</p>
    <div v-if="loading && !readyAssets.length" class="scene-overlay-media-picker__state"><n-spin size="small" /></div>
    <div v-else-if="readyAssets.length" class="scene-overlay-media-picker__grid">
      <article
        v-for="asset in readyAssets"
        :key="asset.id"
        class="scene-overlay-media-picker__asset"
        :title="asset.name"
        @mouseenter="playPreview"
        @mouseleave="pausePreview"
        @focusin="playPreview"
        @focusout="pausePreview"
      >
        <button
          type="button"
          class="scene-overlay-media-picker__select"
          @click="emit('select', asset)"
        >
          <span class="scene-overlay-media-picker__thumb">
            <video
              v-if="playbackMimeType(asset) === 'video/webm'"
              :src="asset.url"
              muted
              loop
              playsinline
              :autoplay="autoPlayPreviews"
              :preload="autoPlayPreviews ? 'auto' : 'metadata'"
            />
            <img v-else :src="asset.url" :alt="asset.name" draggable="false">
            <small v-if="isDynamic(asset)">动态</small>
          </span>
          <strong v-if="editingAssetId !== asset.id">{{ asset.name }}</strong>
        </button>
        <div class="scene-overlay-media-picker__actions">
          <n-button v-if="canEdit" quaternary circle size="tiny" aria-label="重命名素材" title="重命名素材" @click.stop="startAssetRename(asset)">
            <template #icon><n-icon><Edit /></n-icon></template>
          </n-button>
          <n-button v-if="canDelete" quaternary circle size="tiny" type="error" aria-label="删除素材" title="删除素材" @click.stop="emit('delete', asset)">
            <template #icon><n-icon><Trash /></n-icon></template>
          </n-button>
        </div>
        <input
          v-if="editingAssetId === asset.id"
          ref="assetNameInputRef"
          v-model="assetNameDraft"
          class="scene-overlay-media-picker__name-input"
          maxlength="255"
          @click.stop
          @blur="finishAssetRename(asset)"
          @keydown.enter.prevent="finishAssetRename(asset)"
          @keydown.esc.prevent="editingAssetId = ''"
        >
      </article>
    </div>
    <div v-else class="scene-overlay-media-picker__state">
      <n-icon><Photo /></n-icon>
      <span>暂无可用视觉素材</span>
    </div>
  </section>
</template>

<style scoped>
.scene-overlay-media-picker { max-height: 320px; display: flex; flex-direction: column; border-bottom: 1px solid var(--theater-border); background: var(--theater-panel); }
.scene-overlay-media-picker header { min-height: 38px; display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 6px 9px; }
.scene-overlay-media-picker header > strong { font-size: 11px; }
.scene-overlay-media-picker header > div { display: flex; align-items: center; gap: 4px; }
.scene-overlay-media-picker header input { display: none; }
.scene-overlay-media-picker__error { margin: 0; padding: 0 9px 6px; color: #f87171; font-size: 10px; }
.scene-overlay-media-picker__grid { min-height: 0; display: grid; grid-template-columns: repeat(auto-fill, minmax(84px, 1fr)); gap: 6px; overflow: auto; padding: 1px 8px 9px; }
.scene-overlay-media-picker__asset { position: relative; min-width: 0; overflow: hidden; border: 1px solid var(--theater-border); border-radius: 5px; color: var(--sc-text-primary); background: transparent; }
.scene-overlay-media-picker__asset:hover, .scene-overlay-media-picker__asset:focus-visible { border-color: var(--theater-accent); outline: none; }
.scene-overlay-media-picker__select { display: block; width: 100%; border: 0; padding: 0; color: inherit; background: transparent; text-align: left; cursor: pointer; }
.scene-overlay-media-picker__select:focus-visible { outline: 2px solid var(--theater-accent); outline-offset: -2px; }
.scene-overlay-media-picker__thumb { position: relative; aspect-ratio: 1; display: block; overflow: hidden; }
.scene-overlay-media-picker__thumb img, .scene-overlay-media-picker__thumb video { width: 100%; height: 100%; display: block; object-fit: contain; pointer-events: none; }
.scene-overlay-media-picker__thumb small { position: absolute; right: 4px; bottom: 4px; border-radius: 3px; padding: 1px 4px; color: #fff; background: rgba(0, 0, 0, .68); font-size: 8px; }
.scene-overlay-media-picker__select > strong { display: block; overflow: hidden; padding: 5px 6px; font-size: 10px; font-weight: 500; text-overflow: ellipsis; white-space: nowrap; }
.scene-overlay-media-picker__actions { position: absolute; top: 2px; right: 2px; display: flex; opacity: 0; border-radius: 4px; background: color-mix(in srgb, var(--theater-panel) 86%, transparent); transition: opacity .12s ease; }
.scene-overlay-media-picker__asset:hover .scene-overlay-media-picker__actions, .scene-overlay-media-picker__asset:focus-within .scene-overlay-media-picker__actions { opacity: 1; }
.scene-overlay-media-picker__name-input { box-sizing: border-box; width: calc(100% - 8px); margin: 3px 4px 4px; border: 1px solid var(--theater-border); border-radius: 3px; padding: 2px 4px; color: var(--sc-text-primary); background: var(--sc-bg-elevated); font: inherit; font-size: 10px; }
.scene-overlay-media-picker__state { min-height: 110px; display: grid; place-content: center; justify-items: center; gap: 6px; color: var(--sc-text-secondary); font-size: 10px; }
.scene-overlay-media-picker__state .n-icon { font-size: 22px; }
@media (hover: none) { .scene-overlay-media-picker__actions { opacity: 1; } }
@media (prefers-reduced-motion: reduce) { .scene-overlay-media-picker__actions { transition: none; } }
</style>
