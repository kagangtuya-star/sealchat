<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NButton, NIcon, NSwitch, useDialog, useMessage } from 'naive-ui'
import { ArrowDown, ArrowUp, Copy, Edit, Plus, Stack2, Trash } from '@vicons/tabler'

import type { TheaterImageAsset } from '../effects/theater-image-assets'
import type { StageSceneOverlayBinding } from '../shared/stage-types'
import type { TheaterStageStore } from '../stage/StageStore'
import { cloneStageData } from '../stage/stage-editing'
import { registerBuiltInSceneOverlayEffects } from './effects'
import { createSceneOverlayBinding, getSceneOverlayEffect } from './scene-overlay-registry'
import SceneOverlayCustomMediaPicker from './SceneOverlayCustomMediaPicker.vue'
import SceneOverlayEffectCatalog from './SceneOverlayEffectCatalog.vue'
import SceneOverlayInspector from './SceneOverlayInspector.vue'
import SceneOverlayPresetCatalog from './SceneOverlayPresetCatalog.vue'
import SceneOverlayPresetEditorModal from './SceneOverlayPresetEditorModal.vue'
import {
  createTheaterSceneOverlayPreset,
  deleteTheaterSceneOverlayPreset,
  listTheaterSceneOverlayPresets,
  updateTheaterSceneOverlayPreset,
  type TheaterSceneOverlayPreset,
} from './scene-overlay-preset-api'
import {
  getSceneOverlayPreset,
  instantiateSceneOverlayPreset,
  instantiateSceneOverlayPresetDefinition,
  registerBuiltInSceneOverlayPresets,
  type SceneOverlayPresetApplyMode,
} from './presets'

const props = defineProps<{
  store: TheaterStageStore
  canEdit: boolean
  imageAssets: TheaterImageAsset[]
  imageLoading: boolean
  imageUploading: boolean
  imageError: string
  canUploadMedia: boolean
  canEditMedia: boolean
  canDeleteMedia: boolean
  worldId: string
  channelId: string
  scopeType?: 'channel' | 'world'
  presetRefreshToken?: number
}>()

const emit = defineEmits<{
  uploadMedia: [files: File[]]
  renameMedia: [assetId: string, name: string]
  deleteMedia: [asset: TheaterImageAsset]
}>()

registerBuiltInSceneOverlayEffects()
registerBuiltInSceneOverlayPresets()

const dialog = useDialog()
const message = useMessage()
const catalogOpen = ref(false)
const customPickerOpen = ref(false)
const presetCatalogOpen = ref(false)
const replacingMediaOverlayId = ref<string | null>(null)
const selectedOverlayId = ref<string | null>(null)
const customPresets = ref<TheaterSceneOverlayPreset[]>([])
const presetEditorOpen = ref(false)
const editingPreset = ref<TheaterSceneOverlayPreset | null>(null)
let customPresetRequestVersion = 0
const activeSceneId = computed(() => props.store.state.activeSceneId)
const activeScene = computed(() => props.store.state.scenes[activeSceneId.value])
const overlays = computed(() => props.store.state.liveState.sceneOverlays)
const displayOverlays = computed(() => [...overlays.value].reverse())
const selectedOverlay = computed(() => overlays.value.find((binding) => binding.id === selectedOverlayId.value) || null)
const selectedMediaAsset = computed(() => {
  const resourceId = selectedOverlay.value?.media?.resourceId
  return resourceId ? props.imageAssets.find((asset) => asset.resourceId === resourceId) : undefined
})

watch([activeSceneId, overlays], () => {
  if (selectedOverlayId.value && !overlays.value.some((binding) => binding.id === selectedOverlayId.value)) {
    selectedOverlayId.value = null
  }
})
watch(() => props.presetRefreshToken, () => { if (presetCatalogOpen.value) void loadCustomPresets() })
watch(() => [props.worldId, props.channelId, props.scopeType], () => {
  customPresetRequestVersion += 1
  customPresets.value = []
  if (presetCatalogOpen.value) void loadCustomPresets()
})

const commit = (next: StageSceneOverlayBinding[]) => {
  if (!props.canEdit) return false
  return props.store.updateSceneOverlays(activeSceneId.value, next)
}

const loadCustomPresets = async () => {
  const requestVersion = ++customPresetRequestVersion
  const scope = { worldId: props.worldId, channelId: props.channelId, scopeType: props.scopeType }
  try {
    const presets = await listTheaterSceneOverlayPresets(scope)
    if (requestVersion !== customPresetRequestVersion) return
    customPresets.value = presets
  } catch {
    if (requestVersion !== customPresetRequestVersion) return
    message.error('加载自制场景预设失败')
  }
}

const openPresetEditor = (preset: TheaterSceneOverlayPreset | null = null) => {
  if (!props.canEdit || (!preset && !overlays.value.length)) return
  editingPreset.value = preset
  presetEditorOpen.value = true
}

const presetOverlaysFromBindings = (bindings: StageSceneOverlayBinding[]) => bindings.map(({ id: _id, version: _version, ...binding }) => binding)

const savePreset = async (payload: { name: string, description: string, tags: string[], overlays: StageSceneOverlayBinding[] }) => {
  const scope = { worldId: props.worldId, channelId: props.channelId, scopeType: props.scopeType }
  try {
    if (editingPreset.value) {
      const updated = await updateTheaterSceneOverlayPreset(scope, editingPreset.value.id, {
        name: payload.name, description: payload.description, tags: payload.tags,
        revision: editingPreset.value.revision,
      })
      customPresets.value = customPresets.value.map((item) => item.id === updated.id ? updated : item)
    } else {
      const created = await createTheaterSceneOverlayPreset(scope, {
        name: payload.name, description: payload.description, tags: payload.tags,
        overlays: presetOverlaysFromBindings(payload.overlays),
      })
      customPresets.value = [created, ...customPresets.value]
    }
    presetEditorOpen.value = false
    editingPreset.value = null
    message.success('场景预设已保存')
  } catch {
    message.error('保存场景预设失败')
  }
}

const editPreset = (preset: TheaterSceneOverlayPreset) => openPresetEditor(preset)

const removePreset = (preset: TheaterSceneOverlayPreset) => {
  dialog.warning({
    title: '删除场景预设', content: `确定删除“${preset.name}”？`, positiveText: '确认删除', negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await deleteTheaterSceneOverlayPreset({ worldId: props.worldId, channelId: props.channelId, scopeType: props.scopeType }, preset.id)
        customPresets.value = customPresets.value.filter((item) => item.id !== preset.id)
      } catch {
        message.error('删除场景预设失败')
      }
    },
  })
}

const addOverlay = (binding: StageSceneOverlayBinding) => {
  if (overlays.value.length >= 32) return
  commit([...cloneStageData(overlays.value), binding])
  selectedOverlayId.value = binding.id
  catalogOpen.value = false
  customPickerOpen.value = false
  replacingMediaOverlayId.value = null
}

const mediaRefFromAsset = (asset: TheaterImageAsset) => ({
  resourceId: asset.resourceId,
  variant: asset.resource.playbackVariant || 'original',
  mimeType: asset.resource.playbackMimeType || asset.resource.mimeType,
  animated: asset.resource.animated === true,
  ...(Number.isInteger(asset.resource.loopCount) && (asset.resource.loopCount || 0) > 0
    ? { loopCount: asset.resource.loopCount! }
    : {}),
})

const selectCustomMedia = (asset: TheaterImageAsset) => {
  const replacement = replacingMediaOverlayId.value
    ? overlays.value.find((binding) => binding.id === replacingMediaOverlayId.value)
    : undefined
  if (replacement) {
    updateOverlay({ ...cloneStageData(replacement), media: mediaRefFromAsset(asset) })
    selectedOverlayId.value = replacement.id
    customPickerOpen.value = false
    replacingMediaOverlayId.value = null
    return
  }
  const binding = createSceneOverlayBinding('custom.media')
  binding.name = asset.name.slice(0, 128)
  binding.media = mediaRefFromAsset(asset)
  addOverlay(binding)
}

const toggleCustomPicker = () => {
  replacingMediaOverlayId.value = null
  catalogOpen.value = false
  presetCatalogOpen.value = false
  customPickerOpen.value = !customPickerOpen.value
}

const toggleCatalog = () => {
  customPickerOpen.value = false
  presetCatalogOpen.value = false
  replacingMediaOverlayId.value = null
  catalogOpen.value = !catalogOpen.value
}

const togglePresetCatalog = () => {
  customPickerOpen.value = false
  catalogOpen.value = false
  replacingMediaOverlayId.value = null
  presetCatalogOpen.value = !presetCatalogOpen.value
  if (presetCatalogOpen.value) void loadCustomPresets()
}

const applyPreset = (presetId: string, mode: SceneOverlayPresetApplyMode) => {
  const preset = getSceneOverlayPreset(presetId) || customPresets.value.find((item) => item.id === presetId)
  if (!preset) {
    message.error('场景预设不可用')
    return
  }
  const bindings = getSceneOverlayPreset(presetId)
    ? instantiateSceneOverlayPreset(presetId)
    : instantiateSceneOverlayPresetDefinition({ ...preset, category: 'city' }, { skipUnknownEffects: true })
  if (!bindings.length) {
    message.warning('预设中的效果已不可用')
    return
  }
  const next = mode === 'replace'
    ? bindings
    : [...cloneStageData(overlays.value), ...bindings]
  if (next.length > 32) {
    message.warning('场景叠加效果最多 32 个')
    return
  }
  if (!commit(next)) {
    message.error('无法更新当前场景叠加效果')
    return
  }
  selectedOverlayId.value = bindings.at(-1)?.id || null
  presetCatalogOpen.value = false
  message.success(mode === 'replace'
    ? `已应用场景预设：${preset.name}`
    : `已添加场景预设：${preset.name}`)
}

const replaceSelectedMedia = () => {
  if (!selectedOverlay.value?.media) return
  replacingMediaOverlayId.value = selectedOverlay.value.id
  catalogOpen.value = false
  presetCatalogOpen.value = false
  customPickerOpen.value = true
}

const updateOverlay = (binding: StageSceneOverlayBinding) => {
  const next = cloneStageData(overlays.value)
  const index = next.findIndex((item) => item.id === binding.id)
  if (index < 0) return
  next[index] = binding
  commit(next)
}

const setEnabled = (binding: StageSceneOverlayBinding, enabled: boolean) => updateOverlay({ ...cloneStageData(binding), enabled })

const createCopyId = () => {
  const value = typeof crypto !== 'undefined' && crypto.randomUUID
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return `scene-overlay-${value}`
}

const duplicateOverlay = (binding: StageSceneOverlayBinding) => {
  if (overlays.value.length >= 32) return
  const index = overlays.value.findIndex((item) => item.id === binding.id)
  if (index < 0) return
  const copy = cloneStageData(binding)
  copy.id = createCopyId()
  copy.name = `${binding.name || getSceneOverlayEffect(binding.effectId)?.name || '效果'} 副本`.slice(0, 128)
  const next = cloneStageData(overlays.value)
  next.splice(index + 1, 0, copy)
  commit(next)
  selectedOverlayId.value = copy.id
}

const moveOverlay = (binding: StageSceneOverlayBinding, direction: -1 | 1) => {
  const next = cloneStageData(overlays.value)
  const index = next.findIndex((item) => item.id === binding.id)
  const target = index + direction
  if (index < 0 || target < 0 || target >= next.length) return
  ;[next[index], next[target]] = [next[target], next[index]]
  commit(next)
}

const removeOverlay = (binding: StageSceneOverlayBinding) => {
  dialog.warning({
    title: '删除场景叠加',
    content: `确定删除“${binding.name || getSceneOverlayEffect(binding.effectId)?.name || '未知效果'}”？`,
    positiveText: '确认删除',
    negativeText: '取消',
    onPositiveClick: () => {
      commit(cloneStageData(overlays.value).filter((item) => item.id !== binding.id))
      if (selectedOverlayId.value === binding.id) selectedOverlayId.value = null
    },
  })
}

const overlayIndex = (binding: StageSceneOverlayBinding) => overlays.value.findIndex((item) => item.id === binding.id)
</script>

<template>
  <div class="scene-overlay-manager">
    <div class="scene-overlay-manager__summary">
      <div class="scene-overlay-manager__summary-copy">
        <strong>{{ activeScene?.name || '当前场景' }}</strong>
        <small>{{ overlays.length }} 个叠加 · {{ overlays.filter(item => item.enabled).length }} 已启用</small>
      </div>
      <div class="scene-overlay-manager__summary-actions">
        <n-button size="small" secondary :disabled="!canEdit" @click="togglePresetCatalog">
          <template #icon><n-icon><Stack2 /></n-icon></template>
          场景预设
        </n-button>
        <n-button size="small" secondary :disabled="!canEdit || overlays.length >= 32" @click="toggleCustomPicker">自定义效果</n-button>
        <n-button size="small" type="primary" secondary :disabled="!canEdit || overlays.length >= 32" @click="toggleCatalog">
          <template #icon><n-icon><Plus /></n-icon></template>
          添加效果
        </n-button>
        <n-button size="small" secondary :disabled="!canEdit || !overlays.length" @click="openPresetEditor()">保存场景预设</n-button>
      </div>
    </div>

    <SceneOverlayPresetCatalog
      v-if="presetCatalogOpen"
      :current-overlay-count="overlays.length"
      :custom-presets="customPresets"
      @apply="applyPreset"
      @edit="editPreset"
      @delete="removePreset"
      @close="presetCatalogOpen = false"
    />
    <SceneOverlayPresetEditorModal
      :show="presetEditorOpen"
      :preset="editingPreset"
      :overlays="overlays"
      @close="presetEditorOpen = false; editingPreset = null"
      @save="savePreset"
    />
    <SceneOverlayCustomMediaPicker
      v-if="customPickerOpen"
      :assets="imageAssets"
      :loading="imageLoading"
      :uploading="imageUploading"
      :error="imageError"
      :can-upload="canUploadMedia"
      :can-edit="canEditMedia"
      :can-delete="canDeleteMedia"
      @select="selectCustomMedia"
      @upload="emit('uploadMedia', $event)"
      @rename="(assetId, name) => emit('renameMedia', assetId, name)"
      @delete="emit('deleteMedia', $event)"
      @close="customPickerOpen = false; replacingMediaOverlayId = null"
    />
    <SceneOverlayEffectCatalog v-if="catalogOpen" @add="addOverlay" />

    <div class="scene-overlay-manager__body" :class="{ 'has-inspector': selectedOverlay }">
      <div class="scene-overlay-manager__list">
        <div v-if="!overlays.length" class="scene-overlay-manager__empty">当前场景没有叠加效果</div>
        <article
          v-for="binding in displayOverlays"
          v-else
          :key="binding.id"
          class="scene-overlay-manager__row"
          :class="{ 'is-selected': selectedOverlayId === binding.id, 'is-disabled': !binding.enabled }"
        >
          <n-switch :value="binding.enabled" size="small" :disabled="!canEdit" @update:value="setEnabled(binding, $event)" />
          <button class="scene-overlay-manager__select" type="button" @click="selectedOverlayId = binding.id">
            <strong>{{ binding.name || getSceneOverlayEffect(binding.effectId)?.name || '未命名效果' }}</strong>
            <span>{{ getSceneOverlayEffect(binding.effectId)?.name || '未知效果' }} · {{ binding.layer === 'aboveCharacters' ? '覆盖角色' : '不覆盖角色' }}</span>
          </button>
          <div class="scene-overlay-manager__actions">
            <n-button text size="tiny" aria-label="配置效果" title="配置效果" @click="selectedOverlayId = binding.id"><n-icon><Edit /></n-icon></n-button>
            <n-button text size="tiny" aria-label="复制效果" title="复制效果" :disabled="!canEdit || overlays.length >= 32" @click="duplicateOverlay(binding)"><n-icon><Copy /></n-icon></n-button>
            <n-button text size="tiny" aria-label="向上层移动" title="向上层移动" :disabled="!canEdit || overlayIndex(binding) >= overlays.length - 1" @click="moveOverlay(binding, 1)"><n-icon><ArrowUp /></n-icon></n-button>
            <n-button text size="tiny" aria-label="向下层移动" title="向下层移动" :disabled="!canEdit || overlayIndex(binding) <= 0" @click="moveOverlay(binding, -1)"><n-icon><ArrowDown /></n-icon></n-button>
            <n-button text size="tiny" type="error" aria-label="删除效果" title="删除效果" :disabled="!canEdit" @click="removeOverlay(binding)"><n-icon><Trash /></n-icon></n-button>
          </div>
        </article>
      </div>

      <SceneOverlayInspector
        v-if="selectedOverlay"
        :binding="selectedOverlay"
        :definition="getSceneOverlayEffect(selectedOverlay.effectId)"
        :media-asset="selectedMediaAsset"
        :can-edit="canEdit"
        @update="updateOverlay"
        @remove="removeOverlay(selectedOverlay)"
        @replace-media="replaceSelectedMedia"
      />
    </div>
  </div>
</template>

<style scoped>
.scene-overlay-manager { min-height: 0; display: flex; flex: 1; flex-direction: column; overflow: hidden; }
.scene-overlay-manager__summary { min-height: 48px; display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 8px; padding: 7px 9px; border-bottom: 1px solid var(--theater-border); }
.scene-overlay-manager__summary-copy { min-width: 0; display: grid; gap: 1px; }
.scene-overlay-manager__summary strong { overflow: hidden; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.scene-overlay-manager__summary small { color: var(--sc-text-secondary); font-size: 10px; }
.scene-overlay-manager__summary-actions { display: flex; flex: 0 0 auto; align-items: center; gap: 5px; margin-left: auto; }
.scene-overlay-manager__body { min-height: 0; display: grid; flex: 1; grid-template-columns: minmax(0, 1fr); overflow: hidden; }
.scene-overlay-manager__body.has-inspector { grid-template-columns: minmax(230px, .9fr) minmax(270px, 1.1fr); }
.scene-overlay-manager__list { min-width: 0; overflow: auto; }
.scene-overlay-manager__empty { padding: 36px 14px; color: var(--sc-text-secondary); font-size: 11px; text-align: center; }
.scene-overlay-manager__row { min-height: 50px; display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 7px; padding: 5px 7px; border-bottom: 1px solid color-mix(in srgb, var(--theater-border) 68%, transparent); }
.scene-overlay-manager__row:hover, .scene-overlay-manager__row.is-selected { background: color-mix(in srgb, var(--theater-accent) 14%, transparent); }
.scene-overlay-manager__row.is-disabled { opacity: .55; }
.scene-overlay-manager__select { min-width: 0; display: grid; gap: 2px; border: 0; padding: 3px 0; color: inherit; background: transparent; text-align: left; cursor: pointer; }
.scene-overlay-manager__select strong, .scene-overlay-manager__select span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.scene-overlay-manager__select strong { font-size: 11px; }
.scene-overlay-manager__select span { color: var(--sc-text-secondary); font-size: 9px; }
.scene-overlay-manager__actions { display: flex; align-items: center; gap: 1px; }
.scene-overlay-manager__actions :deep(.n-button) { width: 24px; height: 24px; padding: 0; }
@media (max-width: 680px) {
  .scene-overlay-manager__body.has-inspector { grid-template-columns: 1fr; grid-template-rows: minmax(120px, .8fr) minmax(220px, 1.2fr); }
  .scene-overlay-manager__body.has-inspector :deep(.scene-overlay-inspector) { border-top: 1px solid var(--theater-border); border-left: 0; }
  .scene-overlay-manager__actions { display: grid; grid-template-columns: repeat(3, 24px); }
}
</style>
