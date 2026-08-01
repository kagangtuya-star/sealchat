<script setup lang="ts">
import { computed } from 'vue'
import { NButton, NButtonGroup, NInput, useDialog, useMessage } from 'naive-ui'
import RichTextEditor from '@/components/rich-text/RichTextEditor.vue'
import { isTipTapJson, plainTextToTiptapJson, tiptapJsonToPlainText } from '@/utils/tiptap-render'

export type StageTextEditorMode = 'plain' | 'rich'

const props = withDefaults(defineProps<{
  modelValue: string
  mode?: StageTextEditorMode
  canUploadImages?: boolean
}>(), {
  modelValue: '',
  mode: 'plain',
  canUploadImages: false,
})

const emit = defineEmits<{
  (event: 'update:modelValue', value: string): void
  (event: 'update:mode', value: StageTextEditorMode): void
}>()

const dialog = useDialog()
const message = useMessage()
const maximumContentLength = 100_000
const plainValue = computed(() => isTipTapJson(props.modelValue)
  ? tiptapJsonToPlainText(props.modelValue)
  : props.modelValue)

const containsFormatting = (value: string) => {
  if (!isTipTapJson(value)) return false
  try {
    const visit = (node: any): boolean => {
      if (Array.isArray(node?.marks) && node.marks.length > 0) return true
      if (!['doc', 'paragraph', 'text', 'hardBreak'].includes(String(node?.type || ''))) return true
      return Array.isArray(node?.content) && node.content.some(visit)
    }
    return visit(JSON.parse(value))
  } catch {
    return false
  }
}

const updateValue = (value: string) => {
  if (value.length > maximumContentLength) {
    message.warning('文字内容不能超过 100000 个字符')
    return false
  }
  emit('update:modelValue', value)
  return true
}

const applyMode = (mode: StageTextEditorMode) => {
  if (mode === props.mode) return
  if (mode === 'rich') {
    const content = isTipTapJson(props.modelValue)
      ? props.modelValue
      : JSON.stringify(plainTextToTiptapJson(props.modelValue))
    if (!updateValue(content)) return
    emit('update:mode', mode)
    return
  }

  const convert = () => {
    if (!updateValue(plainValue.value)) return
    emit('update:mode', mode)
  }
  if (!containsFormatting(props.modelValue)) {
    convert()
    return
  }
  dialog.warning({
    title: '切换为纯文本',
    content: '现有富文本格式会被移除，只保留文字内容。',
    positiveText: '继续切换',
    negativeText: '取消',
    onPositiveClick: convert,
  })
}
</script>

<template>
  <div class="theater-text-editor">
    <NButtonGroup size="tiny" class="theater-text-editor__mode">
      <NButton :type="props.mode === 'plain' ? 'primary' : 'default'" @click="applyMode('plain')">纯文本</NButton>
      <NButton :type="props.mode === 'rich' ? 'primary' : 'default'" @click="applyMode('rich')">富文本</NButton>
    </NButtonGroup>
    <NInput
      v-if="props.mode === 'plain'"
      :value="plainValue"
      type="textarea"
      :maxlength="maximumContentLength"
      show-count
      :autosize="{ minRows: 2, maxRows: 6 }"
      @update:value="updateValue"
    />
    <RichTextEditor
      v-else
      :model-value="props.modelValue"
      placeholder="输入舞台文字"
      :image-upload-mode="props.canUploadImages ? 'self' : 'none'"
      min-height="180px"
      input-class="theater-text-editor__rich-input"
      performance-popover-placement="left-start"
      performance-popover-content-class="tiptap-toolbar-popover--theater"
      @update:model-value="updateValue"
    />
  </div>
</template>

<style scoped>
.theater-text-editor {
  grid-column: 1 / -1;
  display: grid;
  gap: 7px;
  min-width: 0;
}

.theater-text-editor__mode {
  justify-self: start;
}

:deep(.theater-text-editor__rich-input .tiptap-wrapper) {
  min-height: 180px;
}

:deep(.theater-text-editor__rich-input .tiptap-content) {
  min-height: 116px;
  color: #fff;
}

:global(.tiptap-toolbar-popover--theater .tiptap-performance-panel) {
  max-height: calc(100vh - 76px);
  overflow-y: auto;
  overscroll-behavior: contain;
}
</style>
