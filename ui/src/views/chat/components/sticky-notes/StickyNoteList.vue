<script setup lang="ts">
import { ref, computed, watch, nextTick } from 'vue'
import type { StickyNote, ListTypeData, ListItem } from '@/stores/stickyNote'
import { useStickyNoteStore } from '@/stores/stickyNote'

const props = defineProps<{
  note: StickyNote
  isEditing: boolean
}>()

const stickyNoteStore = useStickyNoteStore()

const typeData = computed<ListTypeData>(() => {
  const parsed = stickyNoteStore.parseTypeData<ListTypeData>(props.note)
  return parsed || { items: [] }
})

const newItemContent = ref('')
const editingItemId = ref<string | null>(null)
const editingContent = ref('')

function generateId(): string {
  return Math.random().toString(36).substring(2, 9)
}

function addItem() {
  if (!newItemContent.value.trim()) return
  const newItem: ListItem = {
    id: generateId(),
    content: newItemContent.value.trim(),
    checked: false,
    indent: 0
  }
  const newItems = [...typeData.value.items, newItem]
  stickyNoteStore.updateTypeData(props.note.id, { items: newItems })
  newItemContent.value = ''
}

function toggleItem(itemId: string) {
  const items = typeData.value.items.map(item =>
    item.id === itemId ? { ...item, checked: !item.checked } : item
  )
  stickyNoteStore.updateTypeData(props.note.id, { items })
}

function removeItem(itemId: string) {
  const items = typeData.value.items.filter(item => item.id !== itemId)
  stickyNoteStore.updateTypeData(props.note.id, { items })
}

function startEdit(item: ListItem) {
  editingItemId.value = item.id
  editingContent.value = item.content
}

function saveEdit() {
  if (!editingItemId.value) return
  const items = typeData.value.items.map(item =>
    item.id === editingItemId.value ? { ...item, content: editingContent.value } : item
  )
  stickyNoteStore.updateTypeData(props.note.id, { items })
  editingItemId.value = null
  editingContent.value = ''
}

function cancelEdit() {
  editingItemId.value = null
  editingContent.value = ''
}

function handleKeydown(e: KeyboardEvent, item: ListItem) {
  if (e.key === 'Tab') {
    e.preventDefault()
    const delta = e.shiftKey ? -1 : 1
    const newIndent = Math.max(0, Math.min(4, item.indent + delta))
    const items = typeData.value.items.map(i =>
      i.id === item.id ? { ...i, indent: newIndent } : i
    )
    stickyNoteStore.updateTypeData(props.note.id, { items })
  }
}

function setAllChecked(checked: boolean) {
  const items = typeData.value.items.map(item => ({ ...item, checked }))
  stickyNoteStore.updateTypeData(props.note.id, { items })
}

function moveItem(index: number, direction: 'up' | 'down') {
  const items = [...typeData.value.items]
  const targetIndex = direction === 'up' ? index - 1 : index + 1
  if (targetIndex < 0 || targetIndex >= items.length) return
  ;[items[index], items[targetIndex]] = [items[targetIndex], items[index]]
  stickyNoteStore.updateTypeData(props.note.id, { items })
}
</script>

<template>
  <div class="sticky-note-list">
    <div class="sticky-note-list__items">
      <div
        v-for="(item, index) in typeData.items"
        :key="item.id"
        class="sticky-note-list__item"
        :style="{ paddingLeft: `${item.indent * 20 + 8}px` }"
        @contextmenu.prevent="toggleItem(item.id)"
      >
        <input
          type="checkbox"
          :checked="item.checked"
          class="sticky-note-list__checkbox"
          @change="toggleItem(item.id)"
        />

        <template v-if="editingItemId === item.id">
          <input
            v-model="editingContent"
            class="sticky-note-list__edit-input"
            @blur="saveEdit"
            @keyup.enter="saveEdit"
            @keyup.escape="cancelEdit"
            @keydown="handleKeydown($event, item)"
            autofocus
          />
        </template>
        <template v-else>
          <span
            class="sticky-note-list__content"
            :class="{ 'sticky-note-list__content--checked': item.checked }"
            @dblclick="startEdit(item)"
          >
            {{ item.content }}
          </span>
        </template>

        <div class="sticky-note-list__item-actions">
          <button
            v-if="index > 0"
            class="sticky-note-list__action-btn"
            @click="moveItem(index, 'up')"
            title="上移"
          >↑</button>
          <button
            v-if="index < typeData.items.length - 1"
            class="sticky-note-list__action-btn"
            @click="moveItem(index, 'down')"
            title="下移"
          >↓</button>
          <button
            class="sticky-note-list__action-btn sticky-note-list__action-btn--delete"
            @click="removeItem(item.id)"
            title="删除"
          >×</button>
        </div>
      </div>
    </div>

    <div class="sticky-note-list__add">
      <input
        v-model="newItemContent"
        class="sticky-note-list__add-input"
        placeholder="添加新项目..."
        @keyup.enter="addItem"
      />
      <button
        class="sticky-note-list__add-btn"
        @click="addItem"
        :disabled="!newItemContent.trim()"
      >+</button>
    </div>

    <div class="sticky-note-list__footer">
      <button class="sticky-note-list__footer-btn" @click="setAllChecked(true)">全选</button>
      <button class="sticky-note-list__footer-btn" @click="setAllChecked(false)">全不选</button>
      <span class="sticky-note-list__stats">
        {{ typeData.items.filter(i => i.checked).length }}/{{ typeData.items.length }}
      </span>
    </div>
  </div>
</template>

<style scoped>
.sticky-note-list {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 8px;
}

.sticky-note-list__items {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
}

.sticky-note-list__item {
  display: flex;
  align-items: center;
  padding: 6px 8px;
  border-radius: 4px;
  gap: 8px;
}

.sticky-note-list__item:hover {
  background: rgba(0, 0, 0, 0.05);
}

.sticky-note-list__checkbox {
  width: 16px;
  height: 16px;
  cursor: pointer;
  flex-shrink: 0;
}

.sticky-note-list__content {
  flex: 1;
  font-size: 14px;
  color: rgba(0, 0, 0, 0.85);
  cursor: text;
  word-break: break-word;
}

.sticky-note-list__content--checked {
  text-decoration: line-through;
  color: rgba(0, 0, 0, 0.45);
}

.sticky-note-list__edit-input {
  flex: 1;
  padding: 4px 8px;
  border: 1px solid rgba(0, 0, 0, 0.2);
  border-radius: 4px;
  font-size: 14px;
  background: rgba(255, 255, 255, 0.8);
}

.sticky-note-list__item-actions {
  display: none;
  gap: 4px;
}

.sticky-note-list__item:hover .sticky-note-list__item-actions {
  display: flex;
}

.sticky-note-list__action-btn {
  width: 20px;
  height: 20px;
  border: none;
  background: transparent;
  cursor: pointer;
  font-size: 14px;
  color: rgba(0, 0, 0, 0.5);
  border-radius: 4px;
}

.sticky-note-list__action-btn:hover {
  background: rgba(0, 0, 0, 0.1);
}

.sticky-note-list__action-btn--delete:hover {
  color: #ef4444;
}

.sticky-note-list__add {
  display: flex;
  gap: 8px;
  padding-top: 8px;
  border-top: 1px solid rgba(0, 0, 0, 0.1);
  margin-top: 8px;
}

.sticky-note-list__add-input {
  flex: 1;
  padding: 8px;
  border: 1px solid rgba(0, 0, 0, 0.2);
  border-radius: 4px;
  font-size: 14px;
  background: rgba(255, 255, 255, 0.5);
}

.sticky-note-list__add-btn {
  width: 32px;
  height: 32px;
  border: none;
  background: rgba(0, 0, 0, 0.1);
  border-radius: 4px;
  font-size: 18px;
  cursor: pointer;
  color: rgba(0, 0, 0, 0.7);
}

.sticky-note-list__add-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.sticky-note-list__footer {
  display: flex;
  gap: 8px;
  padding-top: 8px;
  align-items: center;
}

.sticky-note-list__footer-btn {
  padding: 4px 8px;
  border: none;
  background: rgba(0, 0, 0, 0.1);
  border-radius: 4px;
  font-size: 12px;
  cursor: pointer;
  color: rgba(0, 0, 0, 0.7);
}

.sticky-note-list__footer-btn:hover {
  background: rgba(0, 0, 0, 0.15);
}

.sticky-note-list__stats {
  margin-left: auto;
  font-size: 12px;
  color: rgba(0, 0, 0, 0.5);
}
</style>
