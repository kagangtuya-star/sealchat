<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { NButton, NCheckbox, NDropdown, NIcon, NProgress, NSlider, NTabPane, NTabs, NTooltip } from 'naive-ui'
import { Box, ChevronDown, ChevronRight, Edit, Folder, GripVertical, Music, PlayerPlay, Refresh, Trash, Upload } from '@vicons/tabler'

import type { AudioAsset, AudioQuotaSummary } from '@/types/audio'
import type { TheaterPanelFolder, TheaterPanelItem } from './theater-panel-organizer'
import { useTheaterPointerSort, type TheaterPointerDrag, type TheaterPointerTarget } from './useTheaterPointerSort'

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
  organizerFolders: TheaterPanelFolder[]
  organizerItems: TheaterPanelItem[]
}>()

const emit = defineEmits<{
  refresh: []
  upload: [file: File]
  preview: [asset: AudioAsset]
  delete: [asset: AudioAsset]
  deleteBatch: [assets: AudioAsset[]]
  createFolder: [done: (folder: TheaterPanelFolder | null) => void]
  renameFolder: [folderId: string, name: string]
  deleteFolder: [folderId: string]
  collapseFolder: [folderId: string, collapsed: boolean]
  reorderFolders: [folderIds: string[]]
  reorderItems: [folderId: string, targetIds: string[]]
  'update:masterVolume': [value: number]
}>()

const inputRef = ref<HTMLInputElement | null>(null)
const referenced = computed(() => new Set(props.referencedAssetIds))
const checkedIds = ref<string[]>([])
const editingFolderId = ref('')
const folderNameDraft = ref('')
const folderNameInputRef = ref<HTMLInputElement | null>(null)
const audioFolders = computed(() => props.organizerFolders
  .filter((folder) => folder.domain === 'audio')
  .sort((left, right) => left.sortOrder - right.sortOrder || left.id.localeCompare(right.id)))
const audioItemMap = computed(() => new Map(props.organizerItems
  .filter((item) => item.domain === 'audio')
  .map((item) => [item.targetId, item])))
const folderAssets = (folderId: string) => props.assets
  .filter((asset) => (audioItemMap.value.get(asset.id)?.folderId || '') === folderId)
  .sort((left, right) => {
    const leftOrder = audioItemMap.value.get(left.id)?.sortOrder ?? Number.MAX_SAFE_INTEGER
    const rightOrder = audioItemMap.value.get(right.id)?.sortOrder ?? Number.MAX_SAFE_INTEGER
    return leftOrder - rightOrder || left.name.localeCompare(right.name)
  })
const checkedAssets = computed(() => props.assets.filter((asset) => checkedIds.value.includes(asset.id)))
const moveOptions = computed(() => [
  { label: '未分类', key: '' },
  ...audioFolders.value.map((folder) => ({ label: folder.name, key: folder.id })),
])
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

const createFolder = () => {
  emit('createFolder', (folder) => {
    if (folder) startFolderRename(folder)
  })
}

const startFolderRename = (folder: TheaterPanelFolder) => {
  editingFolderId.value = folder.id
  folderNameDraft.value = folder.name
  void nextTick(() => {
    folderNameInputRef.value?.focus()
    folderNameInputRef.value?.select()
  })
}

const finishFolderRename = (folder: TheaterPanelFolder) => {
  if (editingFolderId.value !== folder.id) return
  const name = folderNameDraft.value.trim()
  editingFolderId.value = ''
  if (name && name !== folder.name) emit('renameFolder', folder.id, name)
}

const deleteFolder = (folder: TheaterPanelFolder) => {
  if (window.confirm(`删除文件夹“${folder.name}”？其中素材将移到未分类。`)) emit('deleteFolder', folder.id)
}

const moveTargets = (folderId: string, targetIds: string[]) => {
  const existing = folderAssets(folderId).map((asset) => asset.id).filter((id) => !targetIds.includes(id))
  emit('reorderItems', folderId, [...existing, ...targetIds])
}

const moveChecked = (folderId: string | number) => {
  if (!checkedIds.value.length) return
  moveTargets(String(folderId), checkedIds.value)
  checkedIds.value = []
}

const handlePointerDrop = (drag: TheaterPointerDrag, target: TheaterPointerTarget) => {
  if (drag.kind === 'folder') {
    if (target.kind !== 'folder' || drag.ids[0] === target.id) return
    const ids = audioFolders.value.map((folder) => folder.id).filter((id) => id !== drag.ids[0])
    const targetIndex = ids.indexOf(target.id)
    ids.splice(targetIndex < 0 ? ids.length : targetIndex, 0, drag.ids[0])
    emit('reorderFolders', ids)
    return
  }
  const moved = drag.ids
  const folderId = target.folderId
  if (target.kind !== 'item') {
    moveTargets(folderId, moved)
    return
  }
  const ids = folderAssets(folderId).map((asset) => asset.id).filter((id) => !moved.includes(id))
  const targetIndex = ids.indexOf(target.id)
  ids.splice(targetIndex < 0 ? ids.length : targetIndex, 0, ...moved)
  emit('reorderItems', folderId, ids)
}

const pointerSort = useTheaterPointerSort(handlePointerDrop)
const beginAssetSort = (event: PointerEvent, assetId: string) => pointerSort.begin(event, {
  kind: 'item',
  ids: checkedIds.value.includes(assetId) ? checkedIds.value : [assetId],
})
const beginFolderSort = (event: PointerEvent, folderId: string) => pointerSort.begin(event, { kind: 'folder', ids: [folderId] })

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
            <n-button v-if="canUpload || canDelete" size="tiny" secondary @click="createFolder">
              <template #icon><n-icon><Folder /></n-icon></template>
              文件夹
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
        <div v-if="checkedIds.length" class="theater-asset-manager__batch">
          <span>已选 {{ checkedIds.length }}</span>
          <n-dropdown trigger="click" :options="moveOptions" @select="moveChecked">
            <n-button size="tiny" secondary>移动</n-button>
          </n-dropdown>
          <n-button v-if="canDelete" size="tiny" type="error" secondary @click="emit('deleteBatch', checkedAssets)">删除</n-button>
          <n-button size="tiny" quaternary @click="checkedIds = []">取消</n-button>
        </div>
        <div v-if="assets.length || audioFolders.length" class="theater-asset-manager__list" data-theater-sort-scroll>
          <section
            v-for="folder in audioFolders"
            :key="folder.id"
            class="theater-asset-manager__folder"
            data-theater-sort-kind="folder"
            :data-folder-id="folder.id"
          >
            <div class="theater-asset-manager__folder-row">
              <button class="theater-pointer-sort-handle" type="button" aria-label="拖拽文件夹排序" @pointerdown="beginFolderSort($event, folder.id)" @pointermove="pointerSort.move" @pointerup="pointerSort.end" @pointercancel="pointerSort.cancel"><n-icon><GripVertical /></n-icon></button>
              <div class="theater-asset-manager__folder-main">
                <button type="button" class="theater-asset-manager__folder-collapse" :aria-label="folder.collapsed ? '展开文件夹' : '折叠文件夹'" @click="emit('collapseFolder', folder.id, !folder.collapsed)"><n-icon :component="folder.collapsed ? ChevronRight : ChevronDown" /></button>
                <n-icon><Folder /></n-icon>
                <input
                  v-if="editingFolderId === folder.id"
                  ref="folderNameInputRef"
                  v-model="folderNameDraft"
                  class="theater-folder-name-input"
                  maxlength="128"
                  @click.stop
                  @blur="finishFolderRename(folder)"
                  @keydown.enter.prevent="finishFolderRename(folder)"
                  @keydown.esc.prevent="editingFolderId = ''"
                >
                <strong v-else title="双击重命名" @dblclick="startFolderRename(folder)">{{ folder.name }}</strong>
                <small>{{ folderAssets(folder.id).length }}</small>
              </div>
              <n-button quaternary circle size="tiny" aria-label="重命名文件夹" @click="startFolderRename(folder)"><template #icon><n-icon><Edit /></n-icon></template></n-button>
              <n-button quaternary circle size="tiny" type="error" aria-label="删除文件夹" @click="deleteFolder(folder)"><template #icon><n-icon><Trash /></n-icon></template></n-button>
            </div>
            <div v-if="!folder.collapsed">
              <div
                v-for="asset in folderAssets(folder.id)"
                :key="asset.id"
                class="theater-asset-manager__row is-nested"
                data-theater-sort-kind="item"
                :data-target-id="asset.id"
                :data-folder-id="folder.id"
              >
                <button class="theater-pointer-sort-handle" type="button" aria-label="拖拽素材排序" @pointerdown="beginAssetSort($event, asset.id)" @pointermove="pointerSort.move" @pointerup="pointerSort.end" @pointercancel="pointerSort.cancel"><n-icon><GripVertical /></n-icon></button>
                <n-checkbox :checked="checkedIds.includes(asset.id)" @update:checked="$event ? checkedIds.push(asset.id) : checkedIds = checkedIds.filter(id => id !== asset.id)" />
                <n-icon class="theater-asset-manager__kind"><Music /></n-icon>
                <div class="theater-asset-manager__meta"><strong :title="asset.name">{{ asset.name }}</strong><span>{{ formatDuration(asset.duration) }} · {{ formatBytes(asset.size) }}</span></div>
                <n-button quaternary circle size="tiny" :disabled="asset.transcodeStatus === 'pending' || asset.transcodeStatus === 'failed'" @click="emit('preview', asset)"><template #icon><n-icon><PlayerPlay /></n-icon></template></n-button>
                <n-button v-if="canDelete" quaternary circle size="tiny" type="error" :disabled="referenced.has(asset.id)" @click="emit('delete', asset)"><template #icon><n-icon><Trash /></n-icon></template></n-button>
              </div>
            </div>
          </section>

          <section class="theater-asset-manager__folder" data-theater-sort-kind="bucket" data-folder-id="">
            <div class="theater-asset-manager__folder-row is-virtual">
              <div class="theater-asset-manager__folder-main"><n-icon><Folder /></n-icon><strong>未分类</strong><small>{{ folderAssets('').length }}</small></div>
            </div>
            <div
              v-for="asset in folderAssets('')"
              :key="asset.id"
              class="theater-asset-manager__row is-nested"
              data-theater-sort-kind="item"
              :data-target-id="asset.id"
              data-folder-id=""
            >
              <button class="theater-pointer-sort-handle" type="button" aria-label="拖拽素材排序" @pointerdown="beginAssetSort($event, asset.id)" @pointermove="pointerSort.move" @pointerup="pointerSort.end" @pointercancel="pointerSort.cancel"><n-icon><GripVertical /></n-icon></button>
              <n-checkbox :checked="checkedIds.includes(asset.id)" @update:checked="$event ? checkedIds.push(asset.id) : checkedIds = checkedIds.filter(id => id !== asset.id)" />
              <n-icon class="theater-asset-manager__kind"><Music /></n-icon>
              <div class="theater-asset-manager__meta"><strong :title="asset.name">{{ asset.name }}</strong><span>{{ formatDuration(asset.duration) }} · {{ formatBytes(asset.size) }}<template v-if="assetStatus(asset)"> · {{ assetStatus(asset) }}</template></span></div>
              <n-button quaternary circle size="tiny" :disabled="asset.transcodeStatus === 'pending' || asset.transcodeStatus === 'failed'" @click="emit('preview', asset)"><template #icon><n-icon><PlayerPlay /></n-icon></template></n-button>
              <n-button v-if="canDelete" quaternary circle size="tiny" type="error" :disabled="referenced.has(asset.id)" @click="emit('delete', asset)"><template #icon><n-icon><Trash /></n-icon></template></n-button>
            </div>
          </section>
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
.theater-asset-manager__batch { display: flex; align-items: center; gap: 5px; padding: 5px 0; color: var(--sc-text-secondary); font-size: 10px; }
.theater-asset-manager__batch span { flex: 1; }
.theater-asset-manager__folder { border-bottom: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08)); }
.theater-asset-manager__folder-row { min-height: 36px; display: flex; align-items: center; background: color-mix(in srgb, var(--sc-bg-elevated) 72%, transparent); }
.theater-asset-manager__folder-row.is-virtual { opacity: .8; }
.theater-asset-manager__folder-main { min-width: 0; flex: 1; display: flex; align-items: center; gap: 6px; border: 0; padding: 6px 5px; color: inherit; background: transparent; text-align: left; }
.theater-asset-manager__folder-collapse { width: 20px; height: 24px; display: grid; place-items: center; border: 0; padding: 0; color: inherit; background: transparent; cursor: pointer; }
.theater-asset-manager__folder-main strong { min-width: 0; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 11px; }
.theater-asset-manager__folder-main small { color: var(--sc-text-secondary); }
.theater-folder-name-input { min-width: 0; flex: 1; border: 1px solid var(--theater-accent); border-radius: 4px; padding: 2px 5px; color: var(--sc-text-primary); background: var(--sc-bg-surface); font: inherit; outline: none; }
.theater-pointer-sort-handle { width: 25px; height: 32px; display: grid; flex: 0 0 auto; place-items: center; border: 0; padding: 0; color: var(--sc-text-secondary); background: transparent; cursor: grab; touch-action: none; }
.theater-pointer-sort-handle.is-pointer-sorting { cursor: grabbing; color: var(--theater-accent); }
.is-pointer-sort-target > .theater-asset-manager__folder-row, .theater-asset-manager__row.is-pointer-sort-target { box-shadow: inset 0 2px 0 var(--theater-accent); }
.theater-asset-manager__row { min-height: 48px; display: flex; align-items: center; gap: 8px; border-bottom: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08)); }
.theater-asset-manager__row.is-nested { padding-left: 13px; }
.theater-asset-manager__kind { flex: 0 0 auto; color: var(--theater-accent); font-size: 18px; }
.theater-asset-manager__meta { min-width: 0; flex: 1; display: grid; gap: 2px; }
.theater-asset-manager__meta strong { overflow: hidden; color: var(--sc-text-primary); font-size: 11px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
.theater-asset-manager__meta span { color: var(--sc-text-secondary); font-size: 9px; }
.theater-asset-manager__empty, .theater-asset-manager__reserved { min-height: 160px; display: grid; place-content: center; justify-items: center; gap: 7px; color: var(--sc-text-secondary); font-size: 11px; }
.theater-asset-manager__reserved .n-icon { font-size: 24px; }
.theater-asset-manager__error { margin: 0 0 7px; color: #f87171; font-size: 10px; }
</style>
