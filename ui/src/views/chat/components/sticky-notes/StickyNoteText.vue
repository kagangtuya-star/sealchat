<script setup lang="ts">
import { ref, computed, watch, h } from 'vue'
import type { StickyNote } from '@/stores/stickyNote'
import { useStickyNoteStore } from '@/stores/stickyNote'
import StickyNoteEditor from '../StickyNoteEditor.vue'
import { isTipTapJson, tiptapJsonToHtml, tiptapJsonToPlainText } from '@/utils/tiptap-render'
import { parseSingleIFormEmbedLinkText, generateIFormEmbedLink } from '@/utils/iformEmbedLink'
import IFormEmbedFrame from '@/components/iform/IFormEmbedFrame.vue'
import type { ChannelIForm } from '@/types/iform'
import { useIFormStore } from '@/stores/iform'
import { useChatStore } from '@/stores/chat'
import { useUtilsStore } from '@/stores/utils'
import { copyTextWithFallback } from '@/utils/clipboard'
import { useMessage } from 'naive-ui'

const props = defineProps<{
  note: StickyNote
  isEditing: boolean
}>()

const stickyNoteStore = useStickyNoteStore()
const message = useMessage()
const iFormStore = useIFormStore()
const chat = useChatStore()
const utils = useUtilsStore()
iFormStore.bootstrap()

const localContent = ref('')
const richMode = ref(false)
const editorRef = ref<InstanceType<typeof StickyNoteEditor> | null>(null)

const resolveIFormEmbedLinkBase = () => {
  const domain = utils.config?.domain?.trim() || ''
  if (!domain) {
    return undefined
  }
  const webUrl = utils.config?.webUrl?.trim() || ''
  let base = domain
  if (!/^(https?:)?\/\//i.test(base)) {
    base = `${window.location.protocol}//${base}`
  }
  if (webUrl) {
    base = `${base}${webUrl.startsWith('/') ? '' : '/'}${webUrl}`
  }
  return base
}

const defaultIFormEmbedLink = computed(() => {
  const channelId = (props.note?.channelId || '').trim()
  const worldId = (chat.currentWorldId || '').trim()
  if (!channelId || !worldId) {
    return ''
  }
  const firstForm = iFormStore.formsByChannel[channelId]?.[0]
  if (!firstForm?.id) {
    return ''
  }
  return generateIFormEmbedLink(
    {
      worldId,
      channelId: firstForm.sourceChannelId || firstForm.channelId,
      formId: firstForm.id,
      width: firstForm.defaultWidth,
      height: firstForm.defaultHeight,
    },
    { base: resolveIFormEmbedLinkBase() },
  )
})

const copyIFormEmbedLink = async () => {
  const link = defaultIFormEmbedLink.value
  if (!link) {
    message.warning('当前频道暂无可复制 iForm')
    return
  }
  const copied = await copyTextWithFallback(link)
  if (copied) {
    message.success('iForm 嵌入链接已复制')
  } else {
    message.error('复制失败')
  }
}

const insertIFormEmbedLinkToSimple = () => {
  const link = defaultIFormEmbedLink.value
  if (!link) {
    return
  }
  localContent.value = localContent.value
    ? `${localContent.value}\n${link}`
    : link
  debouncedSaveContent()
}

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

const singleIFormLink = computed(() => {
  const rawContent = props.note?.content || ''
  const direct = parseSingleIFormEmbedLinkText(rawContent)
  if (direct) {
    return direct
  }
  if (isTipTapJson(rawContent)) {
    const plainText = tiptapJsonToPlainText(rawContent)
    return parseSingleIFormEmbedLinkText(plainText)
  }
  return null
})

const resolveMatchedIForm = () => {
  if (!singleIFormLink.value) {
    return undefined
  }
  const directMatch = (iFormStore.formsByChannel[singleIFormLink.value.channelId] || [])
    .find((item) => item.id === singleIFormLink.value?.formId)
  if (directMatch) {
    return directMatch
  }
  const currentChannelId = String(props.note?.channelId || '').trim()
  if (!currentChannelId) {
    return undefined
  }
  return (iFormStore.formsByChannel[currentChannelId] || []).find((item) => (
    item.id === singleIFormLink.value?.formId
    && (item.sourceChannelId || item.channelId) === singleIFormLink.value.channelId
  ))
}

const stickyIFormNode = computed(() => {
  if (!singleIFormLink.value) {
    return null
  }
  const matchedForm = resolveMatchedIForm()
  const width = Math.max(120, Math.min(1920, Math.round(singleIFormLink.value.width || 640)))
  const height = Math.max(72, Math.min(1200, Math.round(singleIFormLink.value.height || 360)))
  const runtimeForm: ChannelIForm = {
    id: singleIFormLink.value.formId,
    channelId: matchedForm?.channelId || singleIFormLink.value.channelId,
    sourceChannelId: matchedForm?.sourceChannelId,
    name: matchedForm?.name || '便签嵌入窗',
    url: matchedForm?.url,
    embedCode: matchedForm?.embedCode,
    defaultWidth: width,
    defaultHeight: height,
    defaultCollapsed: false,
    defaultFloating: false,
    allowPopout: false,
    orderIndex: 0,
    worldShared: matchedForm?.worldShared,
    sharedRef: matchedForm?.sharedRef,
    sharedWorldId: matchedForm?.sharedWorldId,
    readonly: matchedForm?.readonly,
    mediaOptions: matchedForm?.mediaOptions,
  }
  return h(
    'div',
    {
      class: 'sticky-note-text__iform',
      style: {
        width: `${width}px`,
        maxWidth: '100%',
        height: `${height}px`,
        minWidth: '120px',
        minHeight: '72px',
      },
    },
    [h(IFormEmbedFrame, { form: runtimeForm })],
  )
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
          <button
            class="sticky-note-text__toolbar-btn"
            :disabled="!defaultIFormEmbedLink"
            @click="copyIFormEmbedLink"
            :title="defaultIFormEmbedLink ? '复制首个 iForm 嵌入链接' : '当前频道暂无 iForm'"
          >⧉</button>
          <button
            class="sticky-note-text__toolbar-btn"
            :disabled="!defaultIFormEmbedLink"
            @click="insertIFormEmbedLinkToSimple"
            :title="defaultIFormEmbedLink ? '插入首个 iForm 嵌入链接' : '当前频道暂无 iForm'"
          >↘</button>
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
      v-else-if="!singleIFormLink"
      class="sticky-note-text__content"
      v-html="sanitizedContent"
    ></div>
    <component v-else :is="stickyIFormNode" />
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

.sticky-note-text__iform {
  position: relative;
  overflow: hidden;
  border-radius: 10px;
  border: 1px solid rgba(15, 23, 42, 0.12);
  background: rgba(255, 255, 255, 0.45);
  resize: both;
}

.sticky-note-text__iform :deep(.iform-frame) {
  border: none;
  border-radius: 10px;
  background: transparent;
  box-shadow: none;
}

.sticky-note-text__iform :deep(.iform-frame__iframe),
.sticky-note-text__iform :deep(.iform-frame__html) {
  border-radius: 10px;
}
</style>
