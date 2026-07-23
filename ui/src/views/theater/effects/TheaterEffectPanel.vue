<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import {
  NButton,
  NButtonGroup,
  NCheckbox,
  NColorPicker,
  NIcon,
  NInput,
  NInputNumber,
  NRadioButton,
  NRadioGroup,
  NSelect,
  NSlider,
  NDropdown,
  useDialog,
} from 'naive-ui'
import { ArrowDown, ArrowUp, ChevronDown, ChevronRight, Edit, Eye, EyeOff, Filter, Folder, GripVertical, Photo, PlayerPlay, Search, Stars, Trash, Upload } from '@vicons/tabler'

import type { StageObject } from '../shared/stage-types'
import type { AudioAsset } from '@/types/audio'
import type { TheaterStageStore } from '../stage/StageStore'
import type { TheaterEffectRuntime } from './theater-effect-runtime'
import type { TheaterPanelFolder, TheaterPanelItem } from './theater-panel-organizer'
import { useTheaterPointerSort, type TheaterPointerDrag, type TheaterPointerTarget } from './useTheaterPointerSort'
import {
  createDefaultTheaterEffectConfig,
  isTheaterEffectObject,
  setTheaterEffectConfig,
  theaterBuiltinEffectThemes,
  theaterEffectConfigFromObject,
  type TheaterEffectConfig,
  type TheaterEffectKind,
} from './theater-effect-types'

const props = defineProps<{
  store: TheaterStageStore
  runtime: TheaterEffectRuntime
  canEdit: boolean
  canUpload: boolean
  editingTarget: 'frame' | 'media'
  audioAssets: AudioAsset[]
  audioLoading: boolean
  audioUploading: boolean
  audioError: string
  organizerFolders: TheaterPanelFolder[]
  organizerItems: TheaterPanelItem[]
}>()

const emit = defineEmits<{
  upload: [objectId: string]
  uploadAudio: [objectId: string, file: File]
  createFolder: [done: (folder: TheaterPanelFolder | null) => void]
  renameFolder: [folderId: string, name: string]
  deleteFolder: [folderId: string]
  collapseFolder: [folderId: string, collapsed: boolean]
  reorderFolders: [folderIds: string[]]
  reorderItems: [folderId: string, targetIds: string[]]
  'update:editingTarget': [value: 'frame' | 'media']
}>()

const effects = computed(() => Object.values(props.store.activeObjects.value)
  .filter(isTheaterEffectObject)
  .sort((left, right) => right.transform.z - left.transform.z || right.transform.order - left.transform.order))
const filteredEffects = computed(() => effects.value.filter(matchesEffectFilter))
const checkedEffectIds = ref<string[]>([])
const editingFolderId = ref('')
const folderNameDraft = ref('')
const folderNameInputRef = ref<HTMLInputElement | null>(null)
const effectFolders = computed(() => props.organizerFolders
  .filter((folder) => folder.domain === 'effect')
  .sort((left, right) => left.sortOrder - right.sortOrder || left.id.localeCompare(right.id)))
const effectItemMap = computed(() => new Map(props.organizerItems
  .filter((item) => item.domain === 'effect')
  .map((item) => [item.targetId, item])))
const orderedFolderEffects = (folderId: string) => effects.value
  .filter((object) => (effectItemMap.value.get(object.id)?.folderId || '') === folderId)
  .sort((left, right) => {
    const leftOrder = effectItemMap.value.get(left.id)?.sortOrder ?? Number.MAX_SAFE_INTEGER
    const rightOrder = effectItemMap.value.get(right.id)?.sortOrder ?? Number.MAX_SAFE_INTEGER
    return leftOrder - rightOrder || left.name.localeCompare(right.name)
  })
const folderEffects = (folderId: string) => orderedFolderEffects(folderId).filter(matchesEffectFilter)
const moveOptions = computed(() => [
  { label: '未分类', key: '' },
  ...effectFolders.value.map((folder) => ({ label: folder.name, key: folder.id })),
])
const selectedEffect = computed(() => {
  const id = props.store.state.selectedObjectId
  const object = id ? props.store.activeObjects.value[id] : null
  return isTheaterEffectObject(object) ? object : null
})
const config = computed(() => selectedEffect.value ? theaterEffectConfigFromObject(selectedEffect.value) : null)
const hasMedia = computed(() => Boolean(config.value?.media || selectedEffect.value?.image))
const keywordDraft = ref('')
const targetActorNameDraft = ref('')
const audioInputRef = ref<HTMLInputElement | null>(null)
const pendingAudioEffectId = ref('')
const dialog = useDialog()
const effectSearch = ref('')
const effectKindFilter = ref<'all' | TheaterEffectKind>('all')
const effectVisibilityFilter = ref<'all' | 'visible' | 'hidden'>('all')
const effectListHeight = ref(190)
let effectListResizeStartY = 0
let effectListResizeStartHeight = 0

watch(
  [
    () => selectedEffect.value?.id,
    () => config.value?.keywords.join('\n') || '',
    () => config.value?.targetActorName || '',
  ],
  ([, keywords, targetActorName]) => {
    keywordDraft.value = keywords
    targetActorNameDraft.value = targetActorName
  },
  { immediate: true },
)

const themeOptions = theaterBuiltinEffectThemes.map((theme) => ({ label: theme, value: theme }))
const kindOptions = [
  { label: '内置特效', value: 'builtin' },
  { label: '媒体', value: 'media' },
]
const audioOptions = computed(() => {
  const options = props.audioAssets
  .filter((asset) => !asset.transcodeStatus || asset.transcodeStatus === 'ready')
    .map((asset) => ({ label: asset.name, value: asset.id }))
  const selected = config.value?.audio
  if (selected && !options.some((option) => option.value === selected.assetId)) {
    options.unshift({ label: selected.name || selected.assetId, value: selected.assetId })
  }
  return options
})

const matchesEffectFilter = (object: StageObject) => {
  const normalizedSearch = effectSearch.value.trim().toLocaleLowerCase()
  const effectConfig = theaterEffectConfigFromObject(object)
  if (normalizedSearch && ![object.name, ...effectConfig.keywords].some((value) => value.toLocaleLowerCase().includes(normalizedSearch))) return false
  if (effectKindFilter.value !== 'all' && effectConfig.kind !== effectKindFilter.value) return false
  if (effectVisibilityFilter.value === 'visible' && !object.visible) return false
  if (effectVisibilityFilter.value === 'hidden' && object.visible) return false
  return true
}

const effectFilterCount = computed(() => Number(Boolean(effectSearch.value.trim()))
  + Number(effectKindFilter.value !== 'all')
  + Number(effectVisibilityFilter.value !== 'all'))
const filteredEffectFolders = computed(() => effectFolders.value.filter((folder) => (
  effectFilterCount.value === 0 || folderEffects(folder.id).length > 0
)))
const showUncategorizedEffects = computed(() => (
  effectFilterCount.value === 0 || folderEffects('').length > 0
))
const cycleEffectKindFilter = () => {
  effectKindFilter.value = effectKindFilter.value === 'all'
    ? 'builtin'
    : effectKindFilter.value === 'builtin'
      ? 'media'
      : 'all'
}
const cycleEffectVisibilityFilter = () => {
  effectVisibilityFilter.value = effectVisibilityFilter.value === 'all'
    ? 'visible'
    : effectVisibilityFilter.value === 'visible'
      ? 'hidden'
      : 'all'
}
const resetEffectFilters = () => {
  effectSearch.value = ''
  effectKindFilter.value = 'all'
  effectVisibilityFilter.value = 'all'
}

const startEffectListResize = (event: PointerEvent) => {
  const handle = event.currentTarget as HTMLElement
  handle.setPointerCapture(event.pointerId)
  effectListResizeStartY = event.clientY
  effectListResizeStartHeight = effectListHeight.value
  event.preventDefault()
}

const resizeEffectList = (event: PointerEvent) => {
  if (!event.currentTarget || !event.currentTarget.hasPointerCapture(event.pointerId)) return
  const panel = (event.currentTarget as HTMLElement).closest<HTMLElement>('.theater-effect-panel-content')
  const maxHeight = Math.max(120, (panel?.clientHeight || 420) - 180)
  effectListHeight.value = Math.min(maxHeight, Math.max(120, effectListResizeStartHeight + event.clientY - effectListResizeStartY))
}

const finishEffectListResize = (event: PointerEvent) => {
  const handle = event.currentTarget as HTMLElement
  if (handle.hasPointerCapture(event.pointerId)) handle.releasePointerCapture(event.pointerId)
}

const editConfig = (label: string, mutate: (value: TheaterEffectConfig) => void) => {
  const object = selectedEffect.value
  if (!object || !props.canEdit) return
  props.store.beginObjectEdit(label)
  const next = theaterEffectConfigFromObject(object)
  mutate(next)
  setTheaterEffectConfig(object, next)
  props.store.commitObjectEdit()
}

const addEffect = (kind: TheaterEffectKind) => {
  if (!props.canEdit) return
  const object = props.store.addObject('effect')
  object.name = kind === 'media' ? '新建媒体特效' : '新建内置特效'
  setTheaterEffectConfig(object, createDefaultTheaterEffectConfig(kind))
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
  if (window.confirm(`删除文件夹“${folder.name}”？其中特效将移到未分类。`)) emit('deleteFolder', folder.id)
}

const setEffectsVisible = (ids: string[], visible: boolean) => {
  const targets = ids.map((id) => props.store.activeObjects.value[id]).filter(isTheaterEffectObject)
  if (!props.canEdit || !targets.length) return
  props.store.beginObjectEdit(visible ? '批量启用特效' : '批量停用特效')
  targets.forEach((object) => { object.visible = visible })
  props.store.commitObjectEdit()
}

const folderVisibility = (folderId: string) => {
  const values = folderEffects(folderId).map((object) => object.visible)
  if (!values.length) return true
  if (values.every(Boolean)) return true
  if (values.every((value) => !value)) return false
  return null
}

const deleteCheckedEffects = () => {
  if (!checkedEffectIds.value.length || !window.confirm(`删除选中的 ${checkedEffectIds.value.length} 个特效？`)) return
  props.store.removeObjects(checkedEffectIds.value)
  checkedEffectIds.value = []
}

const moveTargets = (folderId: string, targetIds: string[]) => {
  const existing = orderedFolderEffects(folderId).map((object) => object.id).filter((id) => !targetIds.includes(id))
  emit('reorderItems', folderId, [...existing, ...targetIds])
}

const moveChecked = (folderId: string | number) => {
  if (!checkedEffectIds.value.length) return
  moveTargets(String(folderId), checkedEffectIds.value)
  checkedEffectIds.value = []
}

const handlePointerDrop = (drag: TheaterPointerDrag, target: TheaterPointerTarget) => {
  if (drag.kind === 'folder') {
    if (target.kind !== 'folder' || drag.ids[0] === target.id) return
    const ids = effectFolders.value.map((folder) => folder.id).filter((id) => id !== drag.ids[0])
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
  const ids = orderedFolderEffects(folderId).map((object) => object.id).filter((id) => !moved.includes(id))
  const targetIndex = ids.indexOf(target.id)
  ids.splice(targetIndex < 0 ? ids.length : targetIndex, 0, ...moved)
  emit('reorderItems', folderId, ids)
}

const pointerSort = useTheaterPointerSort(handlePointerDrop)
const beginEffectSort = (event: PointerEvent, effectId: string) => pointerSort.begin(event, {
  kind: 'item',
  ids: checkedEffectIds.value.includes(effectId) ? checkedEffectIds.value : [effectId],
})
const beginFolderSort = (event: PointerEvent, folderId: string) => pointerSort.begin(event, { kind: 'folder', ids: [folderId] })

const selectEffect = (object: StageObject) => props.store.selectObject(object.id)

const removeSelectedEffect = () => {
  const object = selectedEffect.value
  if (!object || !props.canEdit) return
  dialog.warning({
    title: '删除特效',
    content: `确定删除特效“${object.name}”？`,
    positiveText: '确认删除',
    negativeText: '取消',
    onPositiveClick: () => props.store.removeObjects([object.id]),
  })
}

const updateKeywords = (value: string) => editConfig('修改特效关键词', (next) => {
  next.keywords = value.split(/[\n,，]/).map((item) => item.trim()).filter(Boolean)
})

const updateTargetActorName = () => editConfig('修改触发角色', (next) => {
  next.targetActorName = targetActorNameDraft.value.trim() || null
})

const updateAudioAsset = (assetId: string | null) => editConfig('修改特效音效', (next) => {
  const asset = props.audioAssets.find((item) => item.id === assetId)
  next.audio = asset ? { assetId: asset.id, name: asset.name, volume: next.audio?.volume ?? 1 } : null
})

const requestAudioUpload = (objectId: string) => {
  pendingAudioEffectId.value = objectId
  audioInputRef.value?.click()
}

const handleAudioInput = (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  const objectId = pendingAudioEffectId.value
  input.value = ''
  pendingAudioEffectId.value = ''
  if (file && objectId) emit('uploadAudio', objectId, file)
}

</script>

<template>
  <div class="theater-effect-panel-content">
    <input ref="audioInputRef" class="theater-effect-audio-input" type="file" accept="audio/ogg,audio/mpeg,audio/wav,.ogg,.mp3,.wav" @change="handleAudioInput">
    <div class="theater-effect-filter">
      <n-input v-model:value="effectSearch" size="small" clearable placeholder="搜索特效名称或关键词">
        <template #prefix><n-icon><Search /></n-icon></template>
      </n-input>
      <n-button
        quaternary
        size="small"
        :type="effectKindFilter !== 'all' ? 'primary' : 'default'"
        :aria-label="effectKindFilter === 'all' ? '筛选全部类型' : effectKindFilter === 'builtin' ? '筛选内置特效' : '筛选媒体特效'"
        :title="effectKindFilter === 'all' ? '类型：全部' : effectKindFilter === 'builtin' ? '类型：内置特效' : '类型：媒体特效'"
        @click="cycleEffectKindFilter"
      >
        <template #icon><n-icon :component="effectKindFilter === 'all' ? Filter : effectKindFilter === 'builtin' ? Stars : Photo" /></template>
      </n-button>
      <n-button
        quaternary
        size="small"
        :type="effectVisibilityFilter !== 'all' ? 'primary' : 'default'"
        :aria-label="effectVisibilityFilter === 'all' ? '筛选全部状态' : effectVisibilityFilter === 'visible' ? '筛选启用特效' : '筛选停用特效'"
        :title="effectVisibilityFilter === 'all' ? '状态：全部' : effectVisibilityFilter === 'visible' ? '状态：启用' : '状态：停用'"
        @click="cycleEffectVisibilityFilter"
      >
        <template #icon><n-icon :component="effectVisibilityFilter === 'hidden' ? EyeOff : Eye" /></template>
      </n-button>
      <n-button v-if="effectFilterCount" quaternary size="small" aria-label="清除筛选" title="清除筛选" @click="resetEffectFilters">
        <template #icon><n-icon><X /></n-icon></template>
      </n-button>
    </div>
    <div v-if="canEdit" class="theater-effect-add-row">
      <n-button size="tiny" secondary @click="addEffect('builtin')"><template #icon><n-icon><Stars /></n-icon></template>内置</n-button>
      <n-button size="tiny" secondary @click="addEffect('media')"><template #icon><n-icon><Photo /></n-icon></template>媒体</n-button>
      <n-button size="tiny" secondary @click="createFolder"><template #icon><n-icon><Folder /></n-icon></template>文件夹</n-button>
    </div>

    <div v-if="checkedEffectIds.length" class="theater-effect-batch">
      <span>已选 {{ checkedEffectIds.length }}</span>
      <n-button size="tiny" secondary @click="setEffectsVisible(checkedEffectIds, true)"><template #icon><n-icon><Eye /></n-icon></template>启用</n-button>
      <n-button size="tiny" secondary @click="setEffectsVisible(checkedEffectIds, false)"><template #icon><n-icon><EyeOff /></n-icon></template>停用</n-button>
      <n-dropdown trigger="click" :options="moveOptions" @select="moveChecked"><n-button size="tiny" secondary>移动</n-button></n-dropdown>
      <n-button size="tiny" type="error" secondary @click="deleteCheckedEffects"><template #icon><n-icon><Trash /></n-icon></template></n-button>
    </div>

    <div class="theater-effect-list" :style="{ height: `${effectListHeight}px` }" data-theater-sort-scroll>
      <section
        v-for="folder in filteredEffectFolders"
        :key="folder.id"
        class="theater-effect-folder"
        data-theater-sort-kind="folder"
        :data-folder-id="folder.id"
      >
        <div class="theater-effect-folder__row">
          <button class="theater-pointer-sort-handle" type="button" aria-label="拖拽文件夹排序" @pointerdown="beginFolderSort($event, folder.id)" @pointermove="pointerSort.move" @pointerup="pointerSort.end" @pointercancel="pointerSort.cancel"><n-icon><GripVertical /></n-icon></button>
          <div class="theater-effect-folder__main">
            <button type="button" class="theater-effect-folder__collapse" :aria-label="folder.collapsed ? '展开文件夹' : '折叠文件夹'" @click="emit('collapseFolder', folder.id, !folder.collapsed)"><n-icon :component="folder.collapsed ? ChevronRight : ChevronDown" /></button>
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
            <small>{{ folderEffects(folder.id).length }}</small>
          </div>
          <button v-if="canEdit" type="button" class="theater-effect-row__icon" :class="{ 'is-mixed': folderVisibility(folder.id) === null }" @click="setEffectsVisible(folderEffects(folder.id).map(object => object.id), folderVisibility(folder.id) !== true)"><n-icon :component="folderVisibility(folder.id) === false ? EyeOff : Eye" /></button>
          <button type="button" class="theater-effect-row__icon" aria-label="重命名文件夹" @click="startFolderRename(folder)"><n-icon><Edit /></n-icon></button>
          <button type="button" class="theater-effect-row__icon is-danger" aria-label="删除文件夹" @click="deleteFolder(folder)"><n-icon><Trash /></n-icon></button>
        </div>
        <div v-if="!folder.collapsed">
          <div
            v-for="object in folderEffects(folder.id)"
            :key="object.id"
            class="theater-effect-row is-nested"
            :class="{ 'is-active': object.id === selectedEffect?.id, 'is-hidden': !object.visible }"
            data-theater-sort-kind="item"
            :data-target-id="object.id"
            :data-folder-id="folder.id"
          >
            <button class="theater-pointer-sort-handle" type="button" aria-label="拖拽特效排序" @pointerdown="beginEffectSort($event, object.id)" @pointermove="pointerSort.move" @pointerup="pointerSort.end" @pointercancel="pointerSort.cancel"><n-icon><GripVertical /></n-icon></button>
            <n-checkbox :checked="checkedEffectIds.includes(object.id)" @update:checked="$event ? checkedEffectIds.push(object.id) : checkedEffectIds = checkedEffectIds.filter(id => id !== object.id)" />
            <button type="button" class="theater-effect-row__select" @click="selectEffect(object)"><n-icon :component="theaterEffectConfigFromObject(object).kind === 'builtin' ? Stars : Photo" /><span>{{ object.name }}</span><small>{{ theaterEffectConfigFromObject(object).keywords.length }}</small></button>
            <button v-if="canEdit" type="button" class="theater-effect-row__icon" @click="store.setObjectFlag(object.id, 'visible', !object.visible)"><n-icon :component="object.visible ? Eye : EyeOff" /></button>
          </div>
        </div>
      </section>

      <section v-if="showUncategorizedEffects" class="theater-effect-folder" data-theater-sort-kind="bucket" data-folder-id="">
        <div class="theater-effect-folder__row is-virtual">
          <div class="theater-effect-folder__main"><n-icon><Folder /></n-icon><strong>未分类</strong><small>{{ folderEffects('').length }}</small></div>
          <button v-if="canEdit" type="button" class="theater-effect-row__icon" :class="{ 'is-mixed': folderVisibility('') === null }" @click="setEffectsVisible(folderEffects('').map(object => object.id), folderVisibility('') !== true)"><n-icon :component="folderVisibility('') === false ? EyeOff : Eye" /></button>
        </div>
        <div
          v-for="object in folderEffects('')"
          :key="object.id"
          class="theater-effect-row is-nested"
          :class="{ 'is-active': object.id === selectedEffect?.id, 'is-hidden': !object.visible }"
          data-theater-sort-kind="item"
          :data-target-id="object.id"
          data-folder-id=""
        >
          <button class="theater-pointer-sort-handle" type="button" aria-label="拖拽特效排序" @pointerdown="beginEffectSort($event, object.id)" @pointermove="pointerSort.move" @pointerup="pointerSort.end" @pointercancel="pointerSort.cancel"><n-icon><GripVertical /></n-icon></button>
          <n-checkbox :checked="checkedEffectIds.includes(object.id)" @update:checked="$event ? checkedEffectIds.push(object.id) : checkedEffectIds = checkedEffectIds.filter(id => id !== object.id)" />
          <button type="button" class="theater-effect-row__select" @click="selectEffect(object)"><n-icon :component="theaterEffectConfigFromObject(object).kind === 'builtin' ? Stars : Photo" /><span>{{ object.name }}</span><small>{{ theaterEffectConfigFromObject(object).keywords.length }}</small></button>
          <button v-if="canEdit" type="button" class="theater-effect-row__icon" @click="store.setObjectFlag(object.id, 'visible', !object.visible)"><n-icon :component="object.visible ? Eye : EyeOff" /></button>
        </div>
      </section>
      <div v-if="!effects.length" class="theater-effect-empty">暂无特效</div>
      <div v-else-if="!filteredEffects.length" class="theater-effect-empty">没有匹配的特效</div>
    </div>
    <div
      class="theater-effect-editor-resize"
      role="separator"
      aria-label="拖拽调整特效列表高度"
      aria-orientation="horizontal"
      @pointerdown="startEffectListResize"
      @pointermove="resizeEffectList"
      @pointerup="finishEffectListResize"
      @pointercancel="finishEffectListResize"
    />

    <div v-if="selectedEffect && config" class="theater-effect-editor">
      <label>名称</label>
      <n-input :value="selectedEffect.name" size="small" maxlength="512" @update:value="value => { store.beginObjectEdit('修改特效名称'); selectedEffect!.name = value; store.commitObjectEdit() }" />

      <label>类型</label>
      <n-select :value="config.kind" :options="kindOptions" size="small" @update:value="value => editConfig('修改特效类型', next => { next.kind = value as TheaterEffectKind })" />

      <label>关键词</label>
      <n-input
        :value="keywordDraft"
        type="textarea"
        :autosize="{ minRows: 2, maxRows: 4 }"
        placeholder="每行一个；命中任意关键词触发"
        @update:value="keywordDraft = $event"
        @change="updateKeywords(keywordDraft)"
      />

      <label>指定频道角色名</label>
      <n-input
        :value="targetActorNameDraft"
        size="small"
        clearable
        placeholder="按角色名匹配；留空表示全部"
        @update:value="targetActorNameDraft = $event"
        @change="updateTargetActorName"
      />

      <label>持续时间</label>
      <n-input-number :value="config.durationMs" :min="300" :max="30000" :step="100" @update:value="value => value !== null && editConfig('修改特效时长', next => { next.durationMs = value })" />

      <label>冷却时间</label>
      <n-input-number :value="config.cooldownMs" :min="0" :max="300000" :step="500" @update:value="value => value !== null && editConfig('修改特效冷却', next => { next.cooldownMs = value })" />

      <label>媒体</label>
      <div class="theater-effect-media-row">
        <n-button size="small" :type="hasMedia ? 'primary' : 'default'" secondary :disabled="!canUpload" @click="emit('upload', selectedEffect.id)">
          <template #icon><n-icon><Photo /></n-icon></template>
          {{ hasMedia ? '图片' : '上传' }}
        </n-button>
      </div>

      <label>音效</label>
      <div class="theater-effect-audio-row">
        <n-select
          :value="config.audio?.assetId || null"
          :options="audioOptions"
          :loading="audioLoading"
          size="small"
          clearable
          filterable
          placeholder="从频道素材选择"
          @update:value="updateAudioAsset"
        />
        <n-button size="small" secondary :disabled="!canUpload" :loading="audioUploading" @click="requestAudioUpload(selectedEffect.id)">
          <template #icon><n-icon><Upload /></n-icon></template>
          上传
        </n-button>
      </div>

      <template v-if="config.audio">
        <label>声音大小 {{ Math.round(config.audio.volume * 100) }}%</label>
        <n-slider :value="config.audio.volume" :min="0" :max="1" :step="0.05" @update:value="value => editConfig('修改特效音量', next => { if (next.audio) next.audio.volume = value })" />
      </template>

      <p v-if="audioError" class="theater-effect-audio-error">{{ audioError }}</p>

      <template v-if="config.kind === 'builtin'">
        <label>主题</label>
        <n-select :value="config.builtin.theme" :options="themeOptions" size="small" @update:value="value => editConfig('修改特效主题', next => { next.builtin.theme = value })" />

        <label>格式</label>
        <n-radio-group :value="config.builtin.format" size="small" @update:value="value => editConfig('修改特效格式', next => { next.builtin.format = value })">
          <n-radio-button value="popout">弹出</n-radio-button>
          <n-radio-button value="boxed">框内</n-radio-button>
        </n-radio-group>

        <label>主文案</label>
        <n-input :value="config.builtin.text" size="small" maxlength="512" @update:value="value => editConfig('修改特效文案', next => { next.builtin.text = value })" />

        <label>副文案</label>
        <n-input :value="config.builtin.subText" size="small" maxlength="512" @update:value="value => editConfig('修改特效文案', next => { next.builtin.subText = value })" />

        <div class="theater-effect-colors">
          <label><span>强调</span><n-color-picker :value="config.builtin.accentColor" :show-alpha="false" :modes="['hex']" @update:value="value => editConfig('修改特效颜色', next => { next.builtin.accentColor = value })" /></label>
          <label><span>主字</span><n-color-picker :value="config.builtin.mainTextColor" :show-alpha="false" :modes="['hex']" @update:value="value => editConfig('修改特效颜色', next => { next.builtin.mainTextColor = value })" /></label>
          <label><span>副字</span><n-color-picker :value="config.builtin.subTextColor" :show-alpha="false" :modes="['hex']" @update:value="value => editConfig('修改特效颜色', next => { next.builtin.subTextColor = value })" /></label>
        </div>

        <label>背景压暗 {{ config.builtin.dimIntensity }}%</label>
        <n-slider :value="config.builtin.dimIntensity" :min="0" :max="100" :step="1" @update:value="value => editConfig('修改特效背景', next => { next.builtin.dimIntensity = value })" />

        <label>震动 {{ config.builtin.shakeIntensity.toFixed(1) }}</label>
        <n-slider :value="config.builtin.shakeIntensity" :min="0" :max="10" :step="0.5" @update:value="value => editConfig('修改特效震动', next => { next.builtin.shakeIntensity = value })" />

        <label>媒体缩放 {{ config.builtin.mediaTransform.scale.toFixed(2) }}</label>
        <n-slider :value="config.builtin.mediaTransform.scale" :min="0.1" :max="5" :step="0.05" @update:value="value => editConfig('修改特效媒体', next => { next.builtin.mediaTransform.scale = value })" />

        <label>媒体旋转</label>
        <n-input-number :value="config.builtin.mediaTransform.rotation" :min="-360" :max="360" @update:value="value => value !== null && editConfig('修改特效媒体', next => { next.builtin.mediaTransform.rotation = value })" />
        <n-checkbox :checked="config.builtin.mediaTransform.mirror" @update:checked="value => editConfig('修改特效媒体', next => { next.builtin.mediaTransform.mirror = value })">镜像媒体</n-checkbox>
      </template>

      <template v-if="config.kind === 'media'">
        <label>媒体缩放 {{ config.builtin.mediaTransform.scale.toFixed(2) }}</label>
        <n-slider :value="config.builtin.mediaTransform.scale" :min="0.1" :max="5" :step="0.05" @update:value="value => editConfig('修改特效媒体', next => { next.builtin.mediaTransform.scale = value })" />
        <label>媒体旋转</label>
        <n-input-number :value="config.builtin.mediaTransform.rotation" :min="-360" :max="360" @update:value="value => value !== null && editConfig('修改特效媒体', next => { next.builtin.mediaTransform.rotation = value })" />
        <n-checkbox :checked="config.builtin.mediaTransform.mirror" @update:checked="value => editConfig('修改特效媒体', next => { next.builtin.mediaTransform.mirror = value })">镜像媒体</n-checkbox>
      </template>

      <label>画布控制</label>
      <n-button-group size="small">
        <n-button :type="editingTarget === 'frame' ? 'primary' : 'default'" @click="emit('update:editingTarget', 'frame')">特效框</n-button>
        <n-button :type="editingTarget === 'media' ? 'primary' : 'default'" @click="emit('update:editingTarget', 'media')">媒体</n-button>
      </n-button-group>

      <div class="theater-effect-actions">
        <n-button size="small" secondary @click="runtime.preview(selectedEffect)"><template #icon><n-icon><PlayerPlay /></n-icon></template>测试</n-button>
        <n-button size="small" quaternary @click="store.moveOrder(selectedEffect.id, 1)"><template #icon><n-icon><ArrowUp /></n-icon></template></n-button>
        <n-button size="small" quaternary @click="store.moveOrder(selectedEffect.id, -1)"><template #icon><n-icon><ArrowDown /></n-icon></template></n-button>
        <n-button v-if="canEdit" size="small" type="error" secondary @click="removeSelectedEffect"><template #icon><n-icon><Trash /></n-icon></template>删除</n-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.theater-effect-panel-content { min-height: 0; display: flex; flex: 1; flex-direction: column; overflow: hidden; }
.theater-effect-filter { display: flex; align-items: center; gap: 5px; padding: 8px 8px 0; }
.theater-effect-filter > :first-child { min-width: 0; flex: 1; }
.theater-effect-add-row { display: flex; gap: 6px; padding: 8px; border-bottom: 1px solid var(--theater-border); }
.theater-effect-batch { display: flex; align-items: center; gap: 4px; overflow-x: auto; padding: 6px 8px; border-bottom: 1px solid var(--theater-border); }
.theater-effect-batch span { flex: 1; white-space: nowrap; color: var(--sc-text-secondary); font-size: 10px; }
.theater-effect-list { min-height: 120px; overflow: auto; border-bottom: 1px solid var(--theater-border); }
.theater-effect-editor-resize { height: 8px; flex: 0 0 auto; cursor: row-resize; touch-action: none; }
.theater-effect-editor-resize::after { width: 32px; height: 2px; display: block; margin: 3px auto; border-radius: 99px; background: var(--theater-border); content: ''; }
.theater-effect-editor-resize:hover::after { background: var(--theater-accent); }
.theater-effect-folder { border-bottom: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08)); }
.theater-effect-folder__row { min-height: 36px; display: flex; align-items: center; background: color-mix(in srgb, var(--sc-bg-elevated) 72%, transparent); }
.theater-effect-folder__row.is-virtual { opacity: .8; }
.theater-effect-folder__main { min-width: 0; flex: 1; display: flex; align-items: center; gap: 6px; border: 0; padding: 6px 8px; color: inherit; background: transparent; text-align: left; }
.theater-effect-folder__collapse { width: 20px; height: 24px; display: grid; place-items: center; border: 0; padding: 0; color: inherit; background: transparent; cursor: pointer; }
.theater-effect-folder__main strong { min-width: 0; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 11px; }
.theater-effect-folder__main small { color: var(--sc-text-secondary); }
.theater-folder-name-input { min-width: 0; flex: 1; border: 1px solid var(--theater-accent); border-radius: 4px; padding: 2px 5px; color: var(--sc-text-primary); background: var(--sc-bg-surface); font: inherit; outline: none; }
.theater-pointer-sort-handle { width: 25px; height: 32px; display: grid; flex: 0 0 auto; place-items: center; border: 0; padding: 0; color: var(--sc-text-secondary); background: transparent; cursor: grab; touch-action: none; }
.theater-pointer-sort-handle.is-pointer-sorting { cursor: grabbing; color: var(--theater-accent); }
.is-pointer-sort-target > .theater-effect-folder__row, .theater-effect-row.is-pointer-sort-target { box-shadow: inset 0 2px 0 var(--theater-accent); }
.theater-effect-row { min-height: 38px; display: flex; align-items: center; }
.theater-effect-row.is-nested { padding-left: 10px; }
.theater-effect-row:hover, .theater-effect-row.is-active { background: color-mix(in srgb, var(--theater-accent) 16%, transparent); }
.theater-effect-row.is-hidden { opacity: .55; }
.theater-effect-row__select { min-width: 0; flex: 1; display: flex; align-items: center; gap: 7px; padding: 7px 9px; border: 0; color: inherit; background: transparent; text-align: left; cursor: pointer; }
.theater-effect-row__select span { min-width: 0; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.theater-effect-row__select small { color: var(--sc-text-secondary); }
.theater-effect-row__icon { width: 32px; height: 32px; border: 0; color: inherit; background: transparent; cursor: pointer; }
.theater-effect-row__icon.is-mixed { opacity: .55; }
.theater-effect-row__icon.is-danger { color: #f87171; }
.theater-effect-empty { padding: 22px; color: var(--sc-text-secondary); text-align: center; }
.theater-effect-editor { min-height: 0; display: grid; grid-template-columns: 92px minmax(0, 1fr); align-items: center; gap: 8px; overflow: auto; padding: 10px; }
.theater-effect-editor > label { color: var(--sc-text-secondary); font-size: 12px; }
.theater-effect-media-row, .theater-effect-actions { display: flex; gap: 5px; }
.theater-effect-audio-row { min-width: 0; display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 5px; }
.theater-effect-audio-input { display: none; }
.theater-effect-audio-error { grid-column: 1 / -1; margin: 0; color: #f87171; font-size: 11px; }
.theater-effect-colors { grid-column: 1 / -1; display: grid; grid-template-columns: repeat(3, 1fr); gap: 7px; }
.theater-effect-colors label { display: grid; gap: 4px; color: var(--sc-text-secondary); font-size: 11px; }
.theater-effect-actions { grid-column: 1 / -1; justify-content: flex-end; padding-top: 4px; }
</style>
