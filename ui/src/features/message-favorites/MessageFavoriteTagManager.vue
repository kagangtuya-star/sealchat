<script setup lang="ts">
import { ref } from 'vue'
import { NButton, NEmpty, NIcon, NInput, NPopconfirm, NPopover, NScrollbar, useMessage } from 'naive-ui'
import { Check, Pencil, Plus, Tags, Trash, X } from '@vicons/tabler'
import { describeMessageFavoriteError, type MessageFavoriteScope, type MessageFavoriteTag } from './api'
import { useMessageFavoriteStore } from './store'

// 标签维护（新建、重命名、删除）。筛选状态由窗口持有，这里只改标签数据。

const props = defineProps<{
  scope: MessageFavoriteScope
  worldId: string
  tags: MessageFavoriteTag[]
}>()

const store = useMessageFavoriteStore()
const notice = useMessage()
const newName = ref('')
const editingId = ref('')
const editingName = ref('')
const pending = ref(false)

const run = async (task: () => Promise<unknown>, fallback: string) => {
  pending.value = true
  try {
    await task()
    return true
  } catch (error) {
    notice.error(describeMessageFavoriteError(error, fallback))
    return false
  } finally {
    pending.value = false
  }
}

const create = async () => {
  const name = newName.value.trim()
  if (!name || pending.value) return
  if (await run(() => store.createTag(props.scope, props.scope === 'world' ? props.worldId : '', name), '新建标签失败')) newName.value = ''
}

const startRename = (tag: MessageFavoriteTag) => {
  editingId.value = tag.id
  editingName.value = tag.name
}

const commitRename = async () => {
  const id = editingId.value
  const name = editingName.value.trim()
  if (!id || pending.value) return
  const current = props.tags.find((tag) => tag.id === id)
  if (!name || name === current?.name) {
    editingId.value = ''
    return
  }
  if (await run(() => store.renameTag(id, name), '重命名标签失败')) editingId.value = ''
}

const remove = (tag: MessageFavoriteTag) => run(() => store.removeTag(tag.id), '删除标签失败')
</script>

<template>
  <NPopover trigger="click" placement="bottom-end" :show-arrow="false" style="padding: 0" @update:show="(value: boolean) => { if (!value) editingId = '' }">
    <template #trigger>
      <NButton size="small" quaternary aria-label="管理标签" title="管理标签">
        <template #icon><NIcon><Tags /></NIcon></template>
      </NButton>
    </template>
    <div class="mf-tag-manager">
      <div class="mf-tag-manager__title">{{ scope === 'world' ? '世界标签' : '个人标签' }}</div>
      <NScrollbar style="max-height: 260px">
        <ul class="mf-tag-manager__list">
          <li v-for="tag in tags" :key="tag.id" class="mf-tag-manager__row">
            <template v-if="editingId === tag.id">
              <NInput
                v-model:value="editingName"
                size="small"
                :maxlength="32"
                autofocus
                @keydown.enter.prevent="commitRename"
                @keydown.esc.prevent="editingId = ''"
              />
              <NButton size="tiny" quaternary :disabled="pending" aria-label="保存名称" @click="commitRename">
                <template #icon><NIcon><Check /></NIcon></template>
              </NButton>
              <NButton size="tiny" quaternary aria-label="取消重命名" @click="editingId = ''">
                <template #icon><NIcon><X /></NIcon></template>
              </NButton>
            </template>
            <template v-else>
              <span class="mf-tag-manager__name" :title="tag.name">{{ tag.name }}</span>
              <NButton size="tiny" quaternary :disabled="pending" :aria-label="`重命名标签 ${tag.name}`" @click="startRename(tag)">
                <template #icon><NIcon><Pencil /></NIcon></template>
              </NButton>
              <NPopconfirm positive-text="删除" negative-text="取消" @positive-click="remove(tag)">
                <template #trigger>
                  <NButton size="tiny" quaternary :disabled="pending" :aria-label="`删除标签 ${tag.name}`">
                    <template #icon><NIcon><Trash /></NIcon></template>
                  </NButton>
                </template>
                删除标签「{{ tag.name }}」？收藏本身不会被删除。
              </NPopconfirm>
            </template>
          </li>
        </ul>
        <NEmpty v-if="!tags.length" size="small" description="还没有标签" />
      </NScrollbar>
      <form class="mf-tag-manager__create" @submit.prevent="create">
        <NInput v-model:value="newName" size="small" :maxlength="32" placeholder="新标签名（1～32 字）" :disabled="pending" />
        <NButton size="small" attr-type="submit" :disabled="!newName.trim() || pending" aria-label="新建标签">
          <template #icon><NIcon><Plus /></NIcon></template>
        </NButton>
      </form>
    </div>
  </NPopover>
</template>

<style scoped>
.mf-tag-manager {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 240px;
  padding: 10px;
  box-sizing: border-box;
}

.mf-tag-manager__title {
  color: var(--sc-text-secondary);
  font-size: 12px;
}

.mf-tag-manager__list {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: 0 6px 0 0;
  list-style: none;
}

.mf-tag-manager__row {
  display: flex;
  align-items: center;
  gap: 2px;
  min-height: 30px;
}

.mf-tag-manager__name {
  overflow: hidden;
  flex: 1;
  min-width: 0;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mf-tag-manager__create {
  display: flex;
  gap: 6px;
  padding-top: 8px;
  border-top: 1px solid var(--sc-border-mute);
}
</style>
