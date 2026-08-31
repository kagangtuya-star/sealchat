<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NButton, NIcon, NSwitch, useDialog } from 'naive-ui'
import { ArrowDown, ArrowUp, Copy, Edit, Plus, Trash } from '@vicons/tabler'

import type { StageSceneOverlayBinding } from '../shared/stage-types'
import type { TheaterStageStore } from '../stage/StageStore'
import { cloneStageData } from '../stage/stage-editing'
import { registerBuiltInSceneOverlayEffects } from './effects'
import { getSceneOverlayEffect } from './scene-overlay-registry'
import SceneOverlayEffectCatalog from './SceneOverlayEffectCatalog.vue'
import SceneOverlayInspector from './SceneOverlayInspector.vue'

const props = defineProps<{
  store: TheaterStageStore
  canEdit: boolean
}>()

registerBuiltInSceneOverlayEffects()

const dialog = useDialog()
const catalogOpen = ref(false)
const selectedOverlayId = ref<string | null>(null)
const activeSceneId = computed(() => props.store.state.activeSceneId)
const activeScene = computed(() => props.store.state.scenes[activeSceneId.value])
const overlays = computed(() => props.store.state.liveState.sceneOverlays)
const displayOverlays = computed(() => [...overlays.value].reverse())
const selectedOverlay = computed(() => overlays.value.find((binding) => binding.id === selectedOverlayId.value) || null)

watch([activeSceneId, overlays], () => {
  if (selectedOverlayId.value && !overlays.value.some((binding) => binding.id === selectedOverlayId.value)) {
    selectedOverlayId.value = null
  }
})

const commit = (next: StageSceneOverlayBinding[]) => {
  if (!props.canEdit) return false
  return props.store.updateSceneOverlays(activeSceneId.value, next)
}

const addOverlay = (binding: StageSceneOverlayBinding) => {
  if (overlays.value.length >= 32) return
  commit([...cloneStageData(overlays.value), binding])
  selectedOverlayId.value = binding.id
  catalogOpen.value = false
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
      <div>
        <strong>{{ activeScene?.name || '当前场景' }}</strong>
        <small>{{ overlays.length }} 个叠加 · {{ overlays.filter(item => item.enabled).length }} 已启用</small>
      </div>
      <n-button size="small" type="primary" secondary :disabled="!canEdit || overlays.length >= 32" @click="catalogOpen = !catalogOpen">
        <template #icon><n-icon><Plus /></n-icon></template>
        添加效果
      </n-button>
    </div>

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
        :can-edit="canEdit"
        @update="updateOverlay"
        @remove="removeOverlay(selectedOverlay)"
      />
    </div>
  </div>
</template>

<style scoped>
.scene-overlay-manager { min-height: 0; display: flex; flex: 1; flex-direction: column; overflow: hidden; }
.scene-overlay-manager__summary { min-height: 48px; display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 7px 9px; border-bottom: 1px solid var(--theater-border); }
.scene-overlay-manager__summary > div { min-width: 0; display: grid; gap: 1px; }
.scene-overlay-manager__summary strong { overflow: hidden; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.scene-overlay-manager__summary small { color: var(--sc-text-secondary); font-size: 10px; }
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
