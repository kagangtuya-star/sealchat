<script setup lang="ts">
import { computed, ref } from 'vue'
import { NButton, NCheckbox, NEmpty, NIcon, NInput, NPopover, NScrollbar, useMessage } from 'naive-ui'
import { Plus, Tag as TagIcon } from '@vicons/tabler'
import { describeMessageFavoriteError, type MessageFavoriteItem, type MessageFavoriteTag } from './api'
import { useMessageFavoriteStore } from './store'

// 单条收藏的标签绑定：提交完整 tagIds 集合，由后端事务替换。

const props = defineProps<{
  item: MessageFavoriteItem
  tags: MessageFavoriteTag[]
  canMaintain: boolean
}>()

const store = useMessageFavoriteStore()
const notice = useMessage()
const show = ref(false)
const pending = ref(false)
const newName = ref('')
const keyword = ref('')

const selected = computed(() => new Set(props.item.tags.map((tag) => tag.id)))
const visibleTags = computed(() => {
  const text = keyword.value.trim().toLowerCase()
  return text ? props.tags.filter((tag) => tag.name.toLowerCase().includes(text)) : props.tags
})

const submit = async (tagIds: string[]) => {
  pending.value = true
  try {
    await store.setFavoriteTags(props.item.id, tagIds)
  } catch (error) {
    notice.error(describeMessageFavoriteError(error, '设置标签失败'))
  } finally {
    pending.value = false
  }
}

const toggle = (tagId: string, checked: boolean) => {
  if (pending.value) return
  const next = new Set(selected.value)
  if (checked) next.add(tagId)
  else next.delete(tagId)
  void submit(Array.from(next))
}

const createAndBind = async () => {
  const name = newName.value.trim()
  if (!name || pending.value) return
  const existing = props.tags.find((tag) => tag.name.toLowerCase() === name.toLowerCase())
  pending.value = true
  try {
    const tag = existing || await store.createTag(props.item.scope, props.item.scope === 'world' ? props.item.worldId : '', name)
    newName.value = ''
    pending.value = false
    if (!selected.value.has(tag.id)) await submit([...selected.value, tag.id])
  } catch (error) {
    notice.error(describeMessageFavoriteError(error, '新建标签失败'))
  } finally {
    pending.value = false
  }
}
</script>

<template>
  <NPopover v-model:show="show" trigger="click" placement="bottom-start" :show-arrow="false" style="padding: 0">
    <template #trigger>
      <button type="button" class="mf-tag-picker__trigger" :aria-label="`设置标签（已选 ${item.tags.length} 个）`" title="设置标签">
        <NIcon :size="13"><TagIcon /></NIcon>
        <span v-if="!item.tags.length">标签</span>
      </button>
    </template>
    <div class="mf-tag-picker">
      <NInput
        v-if="tags.length > 8"
        v-model:value="keyword"
        size="small"
        clearable
        placeholder="查找标签"
      />
      <NScrollbar style="max-height: 220px">
        <div class="mf-tag-picker__list">
          <NCheckbox
            v-for="tag in visibleTags"
            :key="tag.id"
            :checked="selected.has(tag.id)"
            :disabled="pending"
            @update:checked="toggle(tag.id, $event)"
          >
            {{ tag.name }}
          </NCheckbox>
          <NEmpty v-if="!visibleTags.length" size="small" :description="tags.length ? '没有匹配的标签' : '还没有标签'" />
        </div>
      </NScrollbar>
      <form v-if="canMaintain" class="mf-tag-picker__create" @submit.prevent="createAndBind">
        <NInput v-model:value="newName" size="small" :maxlength="32" placeholder="新标签名" :disabled="pending" />
        <NButton size="small" attr-type="submit" :disabled="!newName.trim() || pending" :loading="pending" aria-label="新建并添加标签">
          <template #icon><NIcon><Plus /></NIcon></template>
        </NButton>
      </form>
    </div>
  </NPopover>
</template>

<style scoped>
.mf-tag-picker__trigger {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  min-height: 22px;
  padding: 0 6px;
  border: 1px dashed var(--sc-border-strong);
  border-radius: 999px;
  background: transparent;
  color: var(--sc-text-secondary);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}

.mf-tag-picker__trigger:hover {
  background: var(--sc-bg-hover);
  color: var(--sc-text-primary);
}

.mf-tag-picker {
  display: flex;
  flex-direction: column;
  gap: 8px;
  width: 220px;
  padding: 10px;
  box-sizing: border-box;
}

.mf-tag-picker__list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-right: 6px;
}

.mf-tag-picker__create {
  display: flex;
  gap: 6px;
  padding-top: 8px;
  border-top: 1px solid var(--sc-border-mute);
}
</style>
