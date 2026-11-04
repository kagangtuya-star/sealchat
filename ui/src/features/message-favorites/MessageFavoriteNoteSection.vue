<script setup lang="ts">
import { computed } from 'vue'
import { NButton, NIcon, NInput, useDialog, useMessage, useThemeVars } from 'naive-ui'
import { Notes } from '@vicons/tabler'
import { MESSAGE_FAVORITE_NOTE_MAX, type MessageFavoriteItem, type MessageFavoriteNote } from './api'
import type { useFavoriteNotes } from './useFavoriteNotes'
import MessageFavoriteNoteItem from './MessageFavoriteNoteItem.vue'

const props = defineProps<{
  item: MessageFavoriteItem
  editor: ReturnType<typeof useFavoriteNotes>
  currentUserId: string
  canEdit: boolean
}>()
const dialog = useDialog()
const notice = useMessage()
const themeVars = useThemeVars()
const rootStyle = computed(() => ({ '--mf-note-error': `var(--sc-color-error, ${themeVars.value.errorColor})` }))

const publish = async () => {
  if (props.canEdit && await props.editor.publish()) notice.success('备注已发布')
}
const remove = (note: MessageFavoriteNote) => {
  dialog.warning({
    title: '删除这条备注？', content: '删除后无法恢复。', positiveText: '删除', negativeText: '保留',
    onPositiveClick: async () => { if (await props.editor.remove(note)) notice.success('备注已删除') },
  })
}
const handleKeydown = (event: KeyboardEvent) => {
  if (!event.isComposing && event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
    event.preventDefault()
    void publish()
  }
}
</script>

<template>
  <section class="mf-note" data-sc-favorite-note="panel" data-sc-favorite-part="notes" :style="rootStyle" aria-label="收藏备注">
    <header class="mf-note__head">
      <NIcon :size="15" class="mf-note__icon" aria-hidden="true"><Notes /></NIcon>
      <span class="mf-note__title">备注</span>
      <span v-if="editor.notes.value.length" class="mf-note__meta">{{ editor.notes.value.length }}</span>
    </header>
    <p v-if="editor.loading.value && !editor.notes.value.length" class="mf-note__meta" role="status">正在加载备注…</p>
    <div data-sc-favorite-note="list">
      <MessageFavoriteNoteItem
        v-for="note in editor.notes.value"
        :key="note.id"
        :item="note"
        :editor="editor.existingEditorFor(note.id)"
        :ensure-editor="() => editor.editorFor(note)"
        :current-user-id="currentUserId"
        :can-edit="canEdit && note.authorUserId === currentUserId"
        @remove="remove(note)"
      />
    </div>
    <p v-if="!editor.loading.value && !editor.notes.value.length && !editor.error.value" class="mf-note__meta">暂无备注</p>
    <div v-if="editor.error.value" class="mf-note__error" role="alert">
      {{ editor.error.value }}
      <button type="button" class="mf-note__link" @click="editor.refresh">重试</button>
    </div>
    <div v-if="canEdit" class="mf-note__composer" data-sc-favorite-note="composer">
      <NInput
        v-model:value="editor.composer.draft"
        type="textarea"
        :autosize="{ minRows: 2, maxRows: 10 }"
        :disabled="editor.composer.saving"
        :status="editor.composerOverLimit.value ? 'error' : undefined"
        placeholder="添加备注（纯文本）"
        aria-label="添加备注"
        @keydown="handleKeydown"
      />
      <p v-if="editor.composer.error" class="mf-note__error" role="alert">{{ editor.composer.error }}</p>
      <footer class="mf-note__actions">
        <span class="mf-note__meta" :class="{ 'is-over': editor.composerOverLimit.value }">{{ editor.composerLength.value }} / {{ MESSAGE_FAVORITE_NOTE_MAX }}</span>
        <NButton
          size="small" type="primary" data-sc-favorite-note="add"
          :loading="editor.composer.saving"
          :disabled="!editor.composerLength.value || editor.composerOverLimit.value"
          title="发布（Ctrl+Enter）" @click="publish"
        >发布</NButton>
      </footer>
    </div>
  </section>
</template>

<style scoped>
.mf-note {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px;
  border: 1px solid var(--sc-border-mute);
  border-radius: 10px;
  background: var(--sc-bg-layer);
}
.mf-note__head, .mf-note__actions {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 24px;
}
.mf-note__icon { color: var(--mf-accent); }
.mf-note__title { color: var(--sc-text-primary); font-size: 13px; font-weight: 600; }
.mf-note__meta { margin: 0; color: var(--sc-text-secondary); font-size: 12px; }
.mf-note__composer {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding-top: 8px;
  border-top: 1px solid var(--sc-border-mute);
}
.mf-note__actions { justify-content: space-between; }
.mf-note__error { margin: 0; color: var(--mf-note-error); font-size: 12px; }
.mf-note__meta.is-over { color: var(--mf-note-error); }
.mf-note__link {
  padding: 0;
  border: 0;
  background: none;
  color: var(--mf-accent);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}
</style>
