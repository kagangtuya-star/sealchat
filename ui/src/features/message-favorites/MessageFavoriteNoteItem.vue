<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { NButton, NIcon, NInput, useMessage, useThemeVars } from 'naive-ui'
import { Lock } from '@vicons/tabler'
import dayjs from 'dayjs'
import { MESSAGE_FAVORITE_NOTE_MAX, favoriteNoteAuthorName, type MessageFavoriteNote } from './api'
import type { useFavoriteNoteEditor } from './useFavoriteNoteEditor'

// 单条备注沿用原备注区的文本、锁提示和编辑器样式。

const props = defineProps<{
  item: MessageFavoriteNote
  editor: ReturnType<typeof useFavoriteNoteEditor> | null
  ensureEditor: () => ReturnType<typeof useFavoriteNoteEditor>
  currentUserId: string
  // 后端仍是最终授权；这里只决定是否展示编辑入口（例如世界旁观者只读）。
  canEdit: boolean
}>()

const emit = defineEmits<{ (e: 'remove'): void }>()

const notice = useMessage()
const themeVars = useThemeVars()
// 错误色跟随 Naive 主题，主题包可通过 --sc-color-error 覆盖。
const rootStyle = computed(() => ({ '--mf-note-error': `var(--sc-color-error, ${themeVars.value.errorColor})` }))
const inputRef = ref<InstanceType<typeof NInput> | null>(null)

const session = computed(() => (props.editor?.state.noteId === props.item.id ? props.editor.state : null))
const status = computed(() => session.value?.status || 'idle')
const editorOpen = computed(() => status.value === 'acquiring' || status.value === 'editing' || status.value === 'saving' || status.value === 'lost')
const editorLength = computed(() => props.editor?.length.value || 0)
const editorOverLimit = computed(() => props.editor?.overLimit.value === true)
const editorDirty = computed(() => props.editor?.dirty.value === true)
const nearLimit = computed(() => editorLength.value > MESSAGE_FAVORITE_NOTE_MAX * 0.9)

// 列表快照中的锁只在过期前显示；自己的会话优先。
const snapshotLock = computed(() => {
  const lock = props.item.noteLock
  return lock && lock.expiresAt > Date.now() ? lock : null
})
const lockHolder = computed(() => (status.value === 'locked' ? session.value?.holder || null : (status.value === 'idle' ? snapshotLock.value : null)))
const lockText = computed(() => {
  const holder = lockHolder.value
  if (!holder) return ''
  return holder.userId && holder.userId === props.currentUserId ? '你正在其他窗口编辑此备注' : '其他成员正在编辑此备注'
})

const updatedLabel = computed(() => {
  const time = props.item.createdAt ? dayjs(props.item.createdAt) : null
  return time?.isValid() ? time.format(time.year() === dayjs().year() ? 'MM-DD HH:mm' : 'YYYY-MM-DD HH:mm') : ''
})

const stateAttr = computed(() => (status.value === 'idle' && lockHolder.value ? 'locked' : status.value))

const focusEditor = async () => {
  await nextTick()
  inputRef.value?.focus()
}

watch(status, (next, previous) => {
  if (next === 'editing' && previous === 'acquiring') void focusEditor()
})

const startEdit = () => {
  if (!props.canEdit) return
  const editor = props.editor || props.ensureEditor()
  void editor.begin(props.item)
}

const save = async () => {
  if (props.editor && await props.editor.save()) notice.success('备注已保存')
}

const cancelEdit = () => props.editor?.cancel()
const adoptRemote = () => props.editor?.adoptRemote()

const updateDraft = (value: string) => {
  if (session.value) session.value.draft = value
}

const handleKeydown = (event: KeyboardEvent) => {
  if (event.isComposing) return
  if (event.key === 'Enter' && (event.ctrlKey || event.metaKey)) {
    event.preventDefault()
    void save()
  } else if (event.key === 'Escape' && !editorDirty.value && status.value !== 'saving') {
    event.preventDefault()
    event.stopPropagation()
    cancelEdit()
  }
}
</script>

<template>
  <section
    class="mf-note"
    data-sc-favorite-note="item"
    :data-sc-state="stateAttr"
    :style="rootStyle"
    :aria-label="`${favoriteNoteAuthorName(item)}的备注`"
  >
    <header class="mf-note__head">
      <span class="mf-note__title">{{ favoriteNoteAuthorName(item) }}</span>
      <span v-if="updatedLabel" class="mf-note__meta" :title="item.createdAt">· {{ updatedLabel }}</span>
      <span class="mf-note__spacer" />
      <NButton
        v-if="canEdit && !editorOpen"
        size="tiny"
        quaternary
        class="mf-note__edit"
        @click="startEdit"
      >
        编辑
      </NButton>
      <NButton v-if="canEdit && !editorOpen" size="tiny" quaternary @click="emit('remove')">删除</NButton>
    </header>

    <div v-if="lockHolder" class="mf-note__lock" data-sc-favorite-note="lock" role="status">
      <NIcon :size="13" aria-hidden="true"><Lock /></NIcon>
      <span>{{ lockText }}</span>
      <button v-if="canEdit" type="button" class="mf-note__link" @click="startEdit">重试</button>
    </div>

    <template v-if="!editorOpen">
      <p class="mf-note__text">{{ item.content }}</p>
      <p v-if="session?.error" class="mf-note__error" role="alert">{{ session.error }}</p>
    </template>

    <div v-else class="mf-note__editor" data-sc-favorite-note="editor">
      <NInput
        ref="inputRef"
        type="textarea"
        class="mf-note__input"
        :value="session?.draft || ''"
        :autosize="{ minRows: 4, maxRows: 14 }"
        :disabled="status === 'acquiring' || status === 'saving'"
        :placeholder="status === 'acquiring' ? '正在获取编辑权…' : '为这条收藏写下备注（纯文本）'"
        :status="editorOverLimit ? 'error' : undefined"
        aria-label="备注内容"
        @update:value="updateDraft"
        @keydown="handleKeydown"
      />
      <div v-if="session?.remoteChanged" class="mf-note__hint" role="status">
        <span>备注已在其他窗口更新，保存将以你的版本覆盖。</span>
        <button type="button" class="mf-note__link" @click="adoptRemote">使用最新版本</button>
      </div>
      <p v-if="session?.error" class="mf-note__error" role="alert">{{ session.error }}</p>
      <footer class="mf-note__actions">
        <span
          class="mf-note__count"
          :class="{ 'is-near': nearLimit, 'is-over': editorOverLimit }"
          aria-live="polite"
        >{{ editorLength }} / {{ MESSAGE_FAVORITE_NOTE_MAX }}</span>
        <span class="mf-note__spacer" />
        <NButton size="small" quaternary :disabled="status === 'saving'" @click="cancelEdit">
          {{ status === 'lost' ? '放弃' : '取消' }}
        </NButton>
        <NButton
          v-if="status === 'lost'"
          size="small"
          secondary
          type="primary"
          @click="startEdit"
        >
          重新获取编辑权
        </NButton>
        <NButton
          v-else
          size="small"
          type="primary"
          :loading="status === 'saving'"
          :disabled="status !== 'editing' || editorOverLimit || !editorLength || !editorDirty"
          title="保存（Ctrl+Enter）"
          @click="save"
        >
          保存
        </NButton>
      </footer>
    </div>
  </section>
</template>

<style scoped>
.mf-note {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px 0;
  border-top: 1px solid var(--sc-border-mute);
  background: var(--sc-bg-layer);
}

.mf-note[data-sc-state='editing'],
.mf-note[data-sc-state='saving'],
.mf-note[data-sc-state='acquiring'] {
  border-color: var(--mf-accent);
}

.mf-note__head {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 24px;
}

.mf-note__title {
  color: var(--sc-text-primary);
  font-size: 13px;
  font-weight: 600;
}

.mf-note__meta {
  overflow: hidden;
  min-width: 0;
  color: var(--sc-text-secondary);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mf-note__spacer {
  flex: 1;
}

.mf-note__lock {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px 6px;
  padding: 4px 8px;
  border-radius: 6px;
  background: var(--sc-bg-hover);
  color: var(--sc-text-secondary);
  font-size: 12px;
}

.mf-note__text {
  margin: 0;
  color: var(--sc-text-primary);
  font-size: 13px;
  line-height: 1.6;
  overflow-wrap: anywhere;
  white-space: pre-wrap;
}

.mf-note__editor {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.mf-note__hint,
.mf-note__error {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 8px;
  margin: 0;
  font-size: 12px;
}

.mf-note__hint {
  color: var(--sc-text-secondary);
}

.mf-note__error {
  color: var(--mf-note-error);
}

.mf-note__link {
  padding: 0;
  border: 0;
  background: none;
  color: var(--mf-accent);
  font: inherit;
  font-size: 12px;
  cursor: pointer;
}

.mf-note__actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.mf-note__count {
  color: var(--sc-text-secondary);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}

.mf-note__count.is-near {
  color: var(--sc-text-primary);
  font-weight: 600;
}

.mf-note__count.is-over {
  color: var(--mf-note-error);
  font-weight: 600;
}
</style>
