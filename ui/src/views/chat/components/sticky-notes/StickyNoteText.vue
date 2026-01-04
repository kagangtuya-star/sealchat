<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import type { StickyNote } from '@/stores/stickyNote'
import { useStickyNoteStore } from '@/stores/stickyNote'
import StickyNoteEditor from '../StickyNoteEditor.vue'
import { isTipTapJson, tiptapJsonToHtml } from '@/utils/tiptap-render'

const props = defineProps<{
  note: StickyNote
  isEditing: boolean
}>()

const stickyNoteStore = useStickyNoteStore()

const localContent = ref('')
const richMode = ref(false)
const editorRef = ref<InstanceType<typeof StickyNoteEditor> | null>(null)

watch(() => props.note?.content, (newContent) => {
  if (!props.isEditing && newContent !== undefined) {
    localContent.value = newContent
  }
}, { immediate: true })

watch(() => props.isEditing, (editing) => {
  if (editing) {
    localContent.value = props.note?.content || ''
    richMode.value = isTipTapJson(localContent.value)
  }
})

const sanitizedContent = computed(() => {
  const content = props.note?.content || ''
  if (isTipTapJson(content)) {
    try {
      return tiptapJsonToHtml(content, { imageClass: 'sticky-note__image' })
    } catch {
      // fallback
    }
  }
  const imgPlaceholders: string[] = []
  let processed = content.replace(/<img\s+[^>]*>/gi, (match) => {
    imgPlaceholders.push(match)
    return `__IMG_PLACEHOLDER_${imgPlaceholders.length - 1}__`
  })
  processed = processed
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/\n/g, '<br>')
  imgPlaceholders.forEach((img, i) => {
    processed = processed.replace(`__IMG_PLACEHOLDER_${i}__`, img)
  })
  return processed
})

let saveTimeout: ReturnType<typeof setTimeout> | null = null

function debouncedSaveContent() {
  if (saveTimeout) clearTimeout(saveTimeout)
  saveTimeout = setTimeout(() => {
    saveContentNow()
  }, 500)
}

function saveContentNow() {
  if (saveTimeout) {
    clearTimeout(saveTimeout)
    saveTimeout = null
  }
  if (props.note && localContent.value !== props.note.content) {
    stickyNoteStore.updateNote(props.note.id, {
      content: localContent.value,
      contentText: localContent.value.replace(/<[^>]*>/g, '')
    })
  }
}

defineExpose({
  saveContentNow
})
</script>

<template>
  <div class="sticky-note-text">
    <div v-if="isEditing" class="sticky-note-text__editor">
      <StickyNoteEditor
        v-if="richMode"
        ref="editorRef"
        v-model="localContent"
        :channel-id="note?.channelId"
        @update:model-value="debouncedSaveContent"
      />
      <div v-else class="sticky-note-text__simple-editor">
        <div class="sticky-note-text__simple-toolbar">
          <button
            class="sticky-note-text__toolbar-btn"
            @click="richMode = true"
            title="切换到富文本模式"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor">
              <path d="M5 4v3h5.5v12h3V7H19V4H5z"/>
            </svg>
          </button>
        </div>
        <textarea
          v-model="localContent"
          class="sticky-note-text__textarea"
          placeholder="在此输入内容..."
          @input="debouncedSaveContent"
        ></textarea>
      </div>
    </div>
    <div
      v-else
      class="sticky-note-text__content"
      v-html="sanitizedContent"
    ></div>
  </div>
</template>

<style scoped>
.sticky-note-text {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.sticky-note-text__editor {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.sticky-note-text__simple-editor {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.sticky-note-text__simple-toolbar {
  display: flex;
  padding: 4px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.1);
}

.sticky-note-text__toolbar-btn {
  padding: 4px;
  border: none;
  background: transparent;
  cursor: pointer;
  border-radius: 4px;
  color: rgba(0, 0, 0, 0.6);
}

.sticky-note-text__toolbar-btn:hover {
  background: rgba(0, 0, 0, 0.1);
}

.sticky-note-text__textarea {
  flex: 1;
  width: 100%;
  padding: 8px;
  border: none;
  resize: none;
  background: transparent;
  font-size: 14px;
  line-height: 1.5;
  color: rgba(0, 0, 0, 0.85);
}

.sticky-note-text__textarea:focus {
  outline: none;
}

.sticky-note-text__content {
  flex: 1;
  padding: 8px;
  font-size: 14px;
  line-height: 1.5;
  overflow-y: auto;
  word-break: break-word;
}

.sticky-note-text__content :deep(img) {
  max-width: 100%;
  height: auto;
  border-radius: 4px;
  margin: 4px 0;
}

.sticky-note-text__content :deep(p) {
  margin: 0 0 0.5em;
}

.sticky-note-text__content :deep(p:last-child) {
  margin-bottom: 0;
}
</style>
