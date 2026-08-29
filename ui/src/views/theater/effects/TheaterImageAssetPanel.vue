<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { NButton, NCheckbox, NDropdown, NIcon, NTooltip } from 'naive-ui'
import { Dots, Edit, Photo, Plus, Refresh, Trash, Upload } from '@vicons/tabler'

import type { TheaterPanelFolder, TheaterPanelItem } from './theater-panel-organizer'
import { THEATER_IMAGE_ASSET_DRAG_TYPE, type TheaterImageAsset } from './theater-image-assets'

type ImageDensity = 'small' | 'medium' | 'large'

const props = defineProps<{
  assets: TheaterImageAsset[]
  loading: boolean
  uploading: boolean
  error: string
  canUpload: boolean
  canEdit: boolean
  canDelete: boolean
  organizerFolders: TheaterPanelFolder[]
  organizerItems: TheaterPanelItem[]
}>()

const emit = defineEmits<{
  refresh: []
  upload: [files: File[], folderId: string]
  rename: [assetId: string, name: string]
  delete: [asset: TheaterImageAsset]
  deleteBatch: [assets: TheaterImageAsset[]]
  createFolder: [done: (folder: TheaterPanelFolder | null) => void]
  renameFolder: [folderId: string, name: string]
  deleteFolder: [folderId: string]
  reorderFolders: [folderIds: string[]]
  reorderItems: [folderId: string, targetIds: string[]]
}>()

const FOLDER_DRAG_TYPE = 'application/x-sealchat-theater-image-folder'
const inputRef = ref<HTMLInputElement | null>(null)
const tabStripRef = ref<HTMLElement | null>(null)
const activeFolderId = ref('')
const checkedIds = ref<string[]>([])
const lastSelectedIndex = ref(-1)
const density = ref<ImageDensity>('medium')
const editingFolderId = ref('')
const folderNameDraft = ref('')
const folderNameInputRef = ref<HTMLInputElement | null>(null)
const editingAssetId = ref('')
const assetNameDraft = ref('')
const assetNameInputRef = ref<HTMLInputElement | null>(null)
const draggingAssetIds = ref<string[]>([])
const draggingFolderId = ref('')
const dragOverFolderId = ref<string | null>(null)
const dragOverAssetId = ref<string | null>(null)

const imageFolders = computed(() => props.organizerFolders
  .filter((folder) => folder.domain === 'image')
  .sort((left, right) => left.sortOrder - right.sortOrder || left.id.localeCompare(right.id)))
const imageItemMap = computed(() => new Map(props.organizerItems
  .filter((item) => item.domain === 'image')
  .map((item) => [item.targetId, item])))
const folderAssets = (folderId: string) => props.assets
  .filter((asset) => (imageItemMap.value.get(asset.id)?.folderId || '') === folderId)
  .sort((left, right) => {
    const leftOrder = imageItemMap.value.get(left.id)?.sortOrder ?? Number.MAX_SAFE_INTEGER
    const rightOrder = imageItemMap.value.get(right.id)?.sortOrder ?? Number.MAX_SAFE_INTEGER
    return leftOrder - rightOrder || left.name.localeCompare(right.name)
  })
const assetPlaybackMimeType = (asset: TheaterImageAsset) => asset.resource.playbackMimeType || asset.resource.mimeType
const currentAssets = computed(() => folderAssets(activeFolderId.value))
const checkedAssets = computed(() => props.assets.filter((asset) => checkedIds.value.includes(asset.id)))
const allCurrentChecked = computed(() => currentAssets.value.length > 0 && currentAssets.value.every((asset) => checkedIds.value.includes(asset.id)))
const moveOptions = computed(() => [
  { label: '未分类', key: '' },
  ...imageFolders.value.map((folder) => ({ label: folder.name, key: folder.id })),
].map((option) => ({ ...option, disabled: option.key === activeFolderId.value })))
const activeFolder = computed(() => imageFolders.value.find((folder) => folder.id === activeFolderId.value) || null)
const folderMenuOptions = [
  { label: '重命名', key: 'rename' },
  { label: '删除', key: 'delete' },
]
const densityOptions: Array<{ value: ImageDensity, label: string }> = [
  { value: 'small', label: '小' },
  { value: 'medium', label: '中' },
  { value: 'large', label: '大' },
]

watch(imageFolders, (folders) => {
  if (activeFolderId.value && !folders.some((folder) => folder.id === activeFolderId.value)) activeFolderId.value = ''
})
watch(() => props.assets, (assets) => {
  const validIds = new Set(assets.map((asset) => asset.id))
  checkedIds.value = checkedIds.value.filter((id) => validIds.has(id))
})

const selectFolder = (folderId: string) => {
  if (activeFolderId.value === folderId) return
  activeFolderId.value = folderId
  checkedIds.value = []
  lastSelectedIndex.value = -1
  editingAssetId.value = ''
}

const pickFiles = () => inputRef.value?.click()
const uploadFiles = (files: File[]) => {
  if (files.length) emit('upload', files, activeFolderId.value)
}
const handleFiles = (event: Event) => {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files || [])
  input.value = ''
  uploadFiles(files)
}
const handleExternalDrop = (event: DragEvent) => {
  if (draggingAssetIds.value.length || draggingFolderId.value) return
  uploadFiles(Array.from(event.dataTransfer?.files || []))
}

const createFolder = () => emit('createFolder', (folder) => {
  if (!folder) return
  activeFolderId.value = folder.id
  startFolderRename(folder)
})
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
const handleFolderMenu = (key: string | number) => {
  const folder = activeFolder.value
  if (!folder) return
  if (key === 'rename') startFolderRename(folder)
  if (key === 'delete') deleteFolder(folder)
}

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

const updateChecked = (assetId: string, checked: boolean) => {
  checkedIds.value = checked
    ? [...new Set([...checkedIds.value, assetId])]
    : checkedIds.value.filter((id) => id !== assetId)
  lastSelectedIndex.value = currentAssets.value.findIndex((asset) => asset.id === assetId)
}
const handleCardClick = (event: MouseEvent, assetId: string) => {
  if ((event.target as HTMLElement).closest('button, input, .n-checkbox')) return
  const index = currentAssets.value.findIndex((asset) => asset.id === assetId)
  if (event.shiftKey && lastSelectedIndex.value >= 0) {
    const [start, end] = [lastSelectedIndex.value, index].sort((left, right) => left - right)
    checkedIds.value = [...new Set([...checkedIds.value, ...currentAssets.value.slice(start, end + 1).map((asset) => asset.id)])]
  } else if (event.ctrlKey || event.metaKey) {
    updateChecked(assetId, !checkedIds.value.includes(assetId))
  } else {
    checkedIds.value = checkedIds.value.length === 1 && checkedIds.value[0] === assetId ? [] : [assetId]
  }
  lastSelectedIndex.value = index
}
const toggleAllCurrent = () => {
  checkedIds.value = allCurrentChecked.value ? [] : currentAssets.value.map((asset) => asset.id)
  lastSelectedIndex.value = -1
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

const beginAssetDrag = (event: DragEvent, asset: TheaterImageAsset) => {
  draggingAssetIds.value = checkedIds.value.includes(asset.id) ? checkedIds.value : [asset.id]
  event.dataTransfer?.setData(THEATER_IMAGE_ASSET_DRAG_TYPE, asset.id)
  if (event.dataTransfer) event.dataTransfer.effectAllowed = 'copyMove'
}
const beginFolderDrag = (event: DragEvent, folderId: string) => {
  draggingFolderId.value = folderId
  event.dataTransfer?.setData(FOLDER_DRAG_TYPE, folderId)
  if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move'
}
const autoScrollTabs = (event: DragEvent) => {
  const strip = tabStripRef.value
  if (!strip) return
  const rect = strip.getBoundingClientRect()
  if (event.clientX < rect.left + 32) strip.scrollLeft -= 12
  if (event.clientX > rect.right - 32) strip.scrollLeft += 12
}
const handleTabDragOver = (event: DragEvent, folderId: string) => {
  if (!draggingAssetIds.value.length && !draggingFolderId.value) return
  event.preventDefault()
  dragOverFolderId.value = folderId
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
  autoScrollTabs(event)
}
const handleTabDrop = (event: DragEvent, folderId: string) => {
  event.preventDefault()
  if (draggingAssetIds.value.length) {
    if (folderId !== activeFolderId.value) moveTargets(folderId, draggingAssetIds.value)
    checkedIds.value = []
  } else if (draggingFolderId.value && folderId && draggingFolderId.value !== folderId) {
    const ids = imageFolders.value.map((folder) => folder.id).filter((id) => id !== draggingFolderId.value)
    const targetIndex = ids.indexOf(folderId)
    ids.splice(targetIndex < 0 ? ids.length : targetIndex, 0, draggingFolderId.value)
    emit('reorderFolders', ids)
  } else {
    uploadFiles(Array.from(event.dataTransfer?.files || []))
  }
  clearDragState()
}
const handleCardDragOver = (event: DragEvent, assetId: string) => {
  if (!draggingAssetIds.value.length || draggingAssetIds.value.includes(assetId)) return
  event.preventDefault()
  event.stopPropagation()
  dragOverAssetId.value = assetId
  if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
}
const handleCardDrop = (event: DragEvent, assetId: string) => {
  event.preventDefault()
  event.stopPropagation()
  const files = Array.from(event.dataTransfer?.files || [])
  if (files.length) {
    uploadFiles(files)
    clearDragState()
    return
  }
  if (!draggingAssetIds.value.length || draggingAssetIds.value.includes(assetId)) {
    clearDragState()
    return
  }
  const ids = currentAssets.value.map((asset) => asset.id).filter((id) => !draggingAssetIds.value.includes(id))
  const targetIndex = ids.indexOf(assetId)
  ids.splice(targetIndex < 0 ? ids.length : targetIndex, 0, ...draggingAssetIds.value)
  emit('reorderItems', activeFolderId.value, ids)
  checkedIds.value = []
  clearDragState()
}
const handleGridDrop = (event: DragEvent) => {
  if (!draggingAssetIds.value.length) return
  event.preventDefault()
  event.stopPropagation()
  moveTargets(activeFolderId.value, draggingAssetIds.value)
  checkedIds.value = []
  clearDragState()
}
const clearDragState = () => {
  draggingAssetIds.value = []
  draggingFolderId.value = ''
  dragOverFolderId.value = null
  dragOverAssetId.value = null
}
</script>

<template>
  <div class="theater-image-assets" @dragover.prevent="autoScrollTabs" @drop.prevent="handleExternalDrop">
    <div class="theater-image-assets__tabs-shell">
      <div ref="tabStripRef" class="theater-image-assets__tabs" role="tablist" aria-label="图片素材文件夹">
        <button type="button" role="tab" class="theater-image-assets__tab" :class="{ 'is-active': activeFolderId === '', 'is-drop-target': dragOverFolderId === '' }" :aria-selected="activeFolderId === ''" @click="selectFolder('')" @dragover="handleTabDragOver($event, '')" @dragleave="dragOverFolderId = null" @drop.stop="handleTabDrop($event, '')">
          未分类 <small>{{ folderAssets('').length }}</small>
        </button>
        <div v-for="folder in imageFolders" :key="folder.id" role="tab" tabindex="0" draggable="true" class="theater-image-assets__tab" :class="{ 'is-active': activeFolderId === folder.id, 'is-drop-target': dragOverFolderId === folder.id }" :aria-selected="activeFolderId === folder.id" @click="selectFolder(folder.id)" @keydown.enter.prevent="selectFolder(folder.id)" @dblclick="startFolderRename(folder)" @dragstart="beginFolderDrag($event, folder.id)" @dragend="clearDragState" @dragover="handleTabDragOver($event, folder.id)" @dragleave="dragOverFolderId = null" @drop.stop="handleTabDrop($event, folder.id)">
          <input v-if="editingFolderId === folder.id" ref="folderNameInputRef" v-model="folderNameDraft" class="theater-name-input" maxlength="128" @click.stop @blur="finishFolderRename(folder)" @keydown.enter.prevent="finishFolderRename(folder)" @keydown.esc.prevent="editingFolderId = ''">
          <template v-else>{{ folder.name }} <small>{{ folderAssets(folder.id).length }}</small></template>
        </div>
      </div>
      <n-button v-if="canEdit" quaternary circle size="tiny" aria-label="新建文件夹" @click="createFolder"><template #icon><n-icon><Plus /></n-icon></template></n-button>
      <n-dropdown v-if="canEdit && activeFolder" trigger="click" :options="folderMenuOptions" @select="handleFolderMenu">
        <n-button quaternary circle size="tiny" aria-label="当前文件夹操作"><template #icon><n-icon><Dots /></n-icon></template></n-button>
      </n-dropdown>
    </div>

    <div class="theater-image-assets__toolbar">
      <span>拖入图片，或点击上传</span>
      <div class="theater-image-assets__toolbar-actions">
        <n-tooltip trigger="hover"><template #trigger><n-button quaternary circle size="tiny" :loading="loading" aria-label="刷新图片素材" @click="emit('refresh')"><template #icon><n-icon><Refresh /></n-icon></template></n-button></template>刷新</n-tooltip>
        <n-button v-if="canUpload" size="tiny" secondary :loading="uploading" @click="pickFiles"><template #icon><n-icon><Upload /></n-icon></template>上传</n-button>
        <div class="theater-image-assets__density" aria-label="缩略图大小">
          <button v-for="item in densityOptions" :key="item.value" type="button" :class="{ 'is-active': density === item.value }" :aria-label="`${item.label}图`" @click="density = item.value">{{ item.label }}</button>
        </div>
        <input ref="inputRef" type="file" accept="image/jpeg,image/png,image/apng,image/webp,image/gif,video/webm,.jpg,.jpeg,.png,.apng,.webp,.gif,.webm" multiple @change="handleFiles">
      </div>
    </div>

    <p v-if="error" class="theater-image-assets__error">{{ error }}</p>
    <div v-if="checkedIds.length" class="theater-image-assets__batch">
      <n-checkbox :checked="allCurrentChecked" :indeterminate="checkedIds.length > 0 && !allCurrentChecked" @update:checked="toggleAllCurrent" />
      <span>已选 {{ checkedIds.length }}</span>
      <n-dropdown trigger="click" :options="moveOptions" @select="moveChecked"><n-button size="tiny" secondary>移动</n-button></n-dropdown>
      <n-button v-if="canDelete" size="tiny" type="error" secondary @click="emit('deleteBatch', checkedAssets)">删除</n-button>
      <n-button size="tiny" quaternary @click="checkedIds = []">取消</n-button>
    </div>

    <div v-if="currentAssets.length" class="theater-image-assets__viewport">
      <div class="theater-image-assets__grid" :class="`is-${density}`" @dragover.prevent @drop="handleGridDrop">
        <article v-for="asset in currentAssets" :key="asset.id" class="theater-image-assets__card" :class="{ 'is-selected': checkedIds.includes(asset.id), 'is-drop-target': dragOverAssetId === asset.id }" draggable="true" @click="handleCardClick($event, asset.id)" @dragstart="beginAssetDrag($event, asset)" @dragend="clearDragState" @dragover="handleCardDragOver($event, asset.id)" @dragleave="dragOverAssetId = null" @drop="handleCardDrop($event, asset.id)">
          <div class="theater-image-assets__thumb" title="拖到文件夹迁移；拖到舞台创建图片组件">
            <video v-if="assetPlaybackMimeType(asset) === 'video/webm'" :src="asset.url" muted loop autoplay playsinline />
            <img v-else :src="asset.url" :alt="asset.name" draggable="false">
          </div>
          <n-checkbox class="theater-image-assets__check" :checked="checkedIds.includes(asset.id)" @click.stop @update:checked="updateChecked(asset.id, $event)" />
          <div class="theater-image-assets__card-actions">
            <n-button v-if="canEdit" quaternary circle size="tiny" aria-label="重命名素材" @click.stop="startAssetRename(asset)"><template #icon><n-icon><Edit /></n-icon></template></n-button>
            <n-button v-if="canDelete" quaternary circle size="tiny" type="error" aria-label="删除素材" @click.stop="emit('delete', asset)"><template #icon><n-icon><Trash /></n-icon></template></n-button>
          </div>
          <input v-if="editingAssetId === asset.id" ref="assetNameInputRef" v-model="assetNameDraft" class="theater-name-input theater-image-assets__name-input" maxlength="255" @click.stop @blur="finishAssetRename(asset)" @keydown.enter.prevent="finishAssetRename(asset)" @keydown.esc.prevent="editingAssetId = ''">
          <strong v-else :title="asset.name">{{ asset.name }}</strong>
        </article>
      </div>
    </div>
    <div v-else-if="!loading" class="theater-image-assets__empty"><n-icon><Photo /></n-icon><span>当前文件夹暂无图片</span></div>
  </div>
</template>

<style scoped>
.theater-image-assets { min-height: 0; flex: 1; display: flex; flex-direction: column; }
.theater-image-assets__tabs-shell { display: flex; align-items: center; gap: 2px; border-bottom: 1px solid var(--theater-border); }
.theater-image-assets__tabs { min-width: 0; flex: 1; display: flex; gap: 2px; overflow-x: auto; scrollbar-width: none; }
.theater-image-assets__tabs::-webkit-scrollbar { display: none; }
.theater-image-assets__tab { min-width: max-content; height: 32px; display: flex; align-items: center; gap: 4px; border: 0; border-bottom: 2px solid transparent; padding: 0 9px; color: var(--sc-text-secondary); background: transparent; font: inherit; font-size: 11px; cursor: pointer; }
.theater-image-assets__tab:hover, .theater-image-assets__tab.is-active { color: var(--sc-text-primary); }
.theater-image-assets__tab.is-active { border-bottom-color: var(--theater-accent); }
.theater-image-assets__tab.is-drop-target { color: var(--theater-accent); background: color-mix(in srgb, var(--theater-accent) 12%, transparent); }
.theater-image-assets__tab small { color: var(--sc-text-secondary); font-size: 9px; }
.theater-image-assets__toolbar { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 7px 0; color: var(--sc-text-secondary); font-size: 10px; }
.theater-image-assets__toolbar-actions { display: flex; align-items: center; gap: 4px; }
.theater-image-assets__toolbar input { display: none; }
.theater-image-assets__density { display: flex; overflow: hidden; border: 1px solid var(--theater-border); border-radius: 4px; }
.theater-image-assets__density button { width: 23px; height: 22px; border: 0; padding: 0; color: var(--sc-text-secondary); background: transparent; font-size: 9px; cursor: pointer; }
.theater-image-assets__density button:hover, .theater-image-assets__density button.is-active { color: var(--sc-text-primary); background: color-mix(in srgb, var(--theater-accent) 18%, transparent); }
.theater-image-assets__error { margin: 0 0 7px; color: #f87171; font-size: 10px; }
.theater-image-assets__batch { display: flex; align-items: center; gap: 5px; padding: 5px 0; color: var(--sc-text-secondary); font-size: 10px; }
.theater-image-assets__batch span { min-width: 0; flex: 1; }
.theater-image-assets__viewport { min-height: 0; flex: 1; overflow: auto; }
.theater-image-assets__grid { display: grid; align-content: start; gap: 7px; padding: 2px 1px 8px; }
.theater-image-assets__grid.is-small { grid-template-columns: repeat(auto-fill, minmax(64px, 1fr)); }
.theater-image-assets__grid.is-medium { grid-template-columns: repeat(auto-fill, minmax(92px, 1fr)); }
.theater-image-assets__grid.is-large { grid-template-columns: repeat(auto-fill, minmax(132px, 1fr)); }
.theater-image-assets__card { position: relative; min-width: 0; overflow: hidden; border: 1px solid transparent; border-radius: 5px; background: transparent; cursor: grab; }
.theater-image-assets__card:hover, .theater-image-assets__card.is-selected { border-color: color-mix(in srgb, var(--theater-accent) 64%, var(--theater-border)); }
.theater-image-assets__card.is-drop-target { box-shadow: inset 3px 0 0 var(--theater-accent); }
.theater-image-assets__card:active { cursor: grabbing; }
.theater-image-assets__thumb { aspect-ratio: 1; overflow: hidden; background: transparent; }
.theater-image-assets__thumb img, .theater-image-assets__thumb video { width: 100%; height: 100%; display: block; object-fit: contain; pointer-events: none; }
.theater-image-assets__card > strong { display: block; overflow: hidden; padding: 5px 6px 6px; color: var(--sc-text-primary); background: color-mix(in srgb, var(--sc-bg-elevated) 54%, transparent); font-size: 10px; font-weight: 500; text-overflow: ellipsis; white-space: nowrap; }
.theater-image-assets__check { position: absolute; top: 4px; left: 4px; opacity: 0; filter: drop-shadow(0 1px 2px rgba(0, 0, 0, .7)); transition: opacity .12s ease; }
.theater-image-assets__card:hover .theater-image-assets__check, .theater-image-assets__card.is-selected .theater-image-assets__check { opacity: 1; }
.theater-image-assets__card-actions { position: absolute; top: 2px; right: 2px; display: flex; opacity: 0; border-radius: 4px; background: color-mix(in srgb, var(--theater-panel) 86%, transparent); transition: opacity .12s ease; }
.theater-image-assets__card:hover .theater-image-assets__card-actions, .theater-image-assets__card:focus-within .theater-image-assets__card-actions { opacity: 1; }
.theater-name-input { min-width: 0; border: 1px solid var(--theater-accent); border-radius: 4px; padding: 2px 5px; color: var(--sc-text-primary); background: var(--sc-bg-surface); font: inherit; outline: none; }
.theater-image-assets__tab .theater-name-input { width: 100px; }
.theater-image-assets__name-input { box-sizing: border-box; width: calc(100% - 8px); margin: 3px 4px 4px; font-size: 10px; }
.theater-image-assets__empty { min-height: 160px; flex: 1; display: grid; place-content: center; justify-items: center; gap: 7px; color: var(--sc-text-secondary); font-size: 11px; }
.theater-image-assets__empty .n-icon { font-size: 24px; }
@media (hover: none) { .theater-image-assets__check, .theater-image-assets__card-actions { opacity: 1; } }
@media (prefers-reduced-motion: reduce) { .theater-image-assets__check, .theater-image-assets__card-actions { transition: none; } }
</style>
