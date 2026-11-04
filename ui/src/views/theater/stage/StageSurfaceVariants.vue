<script setup lang="ts">
import { computed, ref } from 'vue'
import { NButton, NDropdown, NInput } from 'naive-ui'
import { uuid } from '@/utils/uuid'
import type { StageImageRef, StageSurfaceVariants } from '../shared/stage-types'
import type { SurfaceVariantOperation } from '../shared/stage-surface-variants'
import type { TheaterStageMediaLocation } from './stage-media'
import StageSceneThumbnail from './StageSceneThumbnail.vue'

const props = defineProps<{
  group: StageSurfaceVariants
  foreground: boolean
  uploading: boolean
  canUpload: boolean
  thumbnail: (image: StageImageRef | null) => TheaterStageMediaLocation | null
}>()
const emit = defineEmits<{
  operate: [operation: SurfaceVariantOperation]
  upload: [id: string | null]
}>()
const menu = computed(() => [
  { key: 'rename', label: '重命名' },
  { key: 'replace', label: '替换', disabled: !props.canUpload },
  { key: 'copy', label: '复制' },
  { key: 'delete', label: '删除' },
])
const editingId = ref<string | null>(null)
const name = ref('')
const menuProps = () => ({ class: 'theater-secondary-surface theater-surface-variants-menu' })
const contextMenu = ref<{ id: string, x: number, y: number } | null>(null)
const openContextMenu = (event: MouseEvent, id: string) => { contextMenu.value = { id, x: event.clientX, y: event.clientY } }
const selectContextMenu = (key: string | number) => {
  if (contextMenu.value) selectMenu(key, contextMenu.value.id)
  contextMenu.value = null
}
const selectMenu = (key: string | number, id: string) => {
  if (key === 'rename') {
    editingId.value = id
    name.value = props.group.slots.find(slot => slot.id === id)?.name ?? ''
  } else if (key === 'replace') emit('upload', id)
  else if (key === 'copy') emit('operate', { type: 'copy', id, newId: uuid() })
  else if (key === 'delete') emit('operate', { type: 'delete', id })
}
const rename = () => {
  if (editingId.value && name.value.trim()) emit('operate', { type: 'rename', id: editingId.value, name: name.value })
  editingId.value = null
}
</script>

<template>
  <div class="surface-variants">
    <div class="surface-variants__list" :aria-label="foreground ? '前景槽位' : '背景槽位'">
      <button v-if="foreground" type="button" class="surface-variants__empty" :class="{ 'is-active': group.activeId === null }" :aria-pressed="group.activeId === null" @click="emit('operate', { type: 'select', id: null })">无前景</button>
      <div v-for="slot in group.slots" :key="slot.id" class="surface-variants__card" :class="{ 'is-active': group.activeId === slot.id }" :title="slot.name">
        <button type="button" class="surface-variants__select" :aria-label="slot.name" :aria-pressed="group.activeId === slot.id" @click="emit('operate', { type: 'select', id: slot.id })" @contextmenu.prevent="openContextMenu($event, slot.id)">
          <StageSceneThumbnail :location="thumbnail(slot.image)" background-color="transparent" />
          <span>{{ slot.name }}</span>
        </button>
        <n-dropdown trigger="click" :options="menu" :menu-props="menuProps" @select="selectMenu($event, slot.id)">
          <button type="button" class="surface-variants__more" :aria-label="`${slot.name}操作`">⋯</button>
        </n-dropdown>
      </div>
      <n-button class="surface-variants__add" size="tiny" dashed :disabled="!canUpload || uploading" @click="emit('upload', null)">+ 添加</n-button>
    </div>
    <n-dropdown trigger="manual" :show="Boolean(contextMenu)" :x="contextMenu?.x" :y="contextMenu?.y" :options="menu" :menu-props="menuProps" @select="selectContextMenu" @clickoutside="contextMenu = null" />
    <div v-if="editingId" class="surface-variants__rename">
      <n-input v-model:value="name" size="small" :maxlength="128" placeholder="槽位名称" @keydown.enter.prevent="rename" @keydown.esc="editingId = null" />
      <n-button size="tiny" @click="rename">保存</n-button>
      <n-button size="tiny" quaternary @click="editingId = null">取消</n-button>
    </div>
  </div>
</template>

<style scoped>
.surface-variants { min-width: 0; width: 100%; margin: 2px 0 7px; }
.surface-variants__list { display: flex; gap: 6px; overflow-x: auto; padding: 2px; overscroll-behavior-x: contain; touch-action: pan-x pan-y; }
.surface-variants__card { position: relative; flex: 0 0 78px; overflow: hidden; border: 1px solid var(--sc-border-mute, #6666); border-radius: 5px; }
.surface-variants__select { display: block; width: 100%; padding: 3px; border: 0; background: transparent; color: var(--sc-text-primary, inherit); cursor: pointer; text-align: left; }
.surface-variants__select > span:last-child { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 10px; padding: 3px 18px 0 1px; }
.surface-variants__more { position: absolute; right: 2px; bottom: 2px; border: 0; border-radius: 3px; background: transparent; color: inherit; cursor: pointer; }
.surface-variants__empty { flex: 0 0 60px; background: transparent; border: 1px solid var(--sc-border-mute, #6666); border-radius: 5px; color: inherit; cursor: pointer; font-size: 11px; }
.surface-variants__card.is-active, .surface-variants__empty.is-active { border-color: var(--sc-accent, var(--theater-accent)); box-shadow: 0 0 0 1px var(--sc-accent, var(--theater-accent)); }
.surface-variants__add { flex: 0 0 auto; align-self: stretch; height: auto; min-height: 44px; }
.surface-variants__rename { display: flex; align-items: center; gap: 4px; margin-top: 5px; }
</style>
