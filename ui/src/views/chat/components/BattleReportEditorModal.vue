<script setup lang="ts">
import { computed, reactive, watch } from 'vue'
import type { BattleReport } from '@/types'

interface Props {
  visible: boolean
  report?: BattleReport | null
  mode?: 'view' | 'edit'
}

interface Emits {
  (e: 'update:visible', value: boolean): void
  (e: 'save', payload: { title: string; content: string }): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const form = reactive({
  title: '',
  content: '',
})
let loadedReportId = ''
let loadedSignature = ''

const isFailed = computed(() => props.report?.status === 'failed')
const isGenerating = computed(() => props.report?.status === 'generating')
const isReadonly = computed(() => props.mode !== 'edit')
const periodText = computed(() => {
  if (!props.report?.periodStart || !props.report?.periodEnd) return '未设置周期'
  const format = (value: number) => new Date(value).toLocaleString('zh-CN', {
    year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
  })
  return `${format(props.report.periodStart)} - ${format(props.report.periodEnd)}`
})

watch(
  () => [
    props.visible,
    props.report?.id,
    props.report?.title,
    props.report?.content,
    props.report?.updatedAt,
  ] as const,
  () => {
    if (!props.visible) return
    const reportId = props.report?.id || ''
    const signature = [
      reportId,
      props.report?.title || '',
      props.report?.content || '',
      props.report?.updatedAt || 0,
    ].join('\u0001')
    if (reportId === loadedReportId && signature === loadedSignature) return
    form.title = props.report?.title || ''
    form.content = props.report?.content || ''
    loadedReportId = reportId
    loadedSignature = signature
  },
  { immediate: true },
)

const close = () => emit('update:visible', false)
const save = () => emit('save', {
  title: form.title.trim() || '未命名战报',
  content: form.content.trim(),
})
</script>

<template>
  <n-modal
    :show="visible"
    preset="card"
    class="battle-report-editor-modal"
    :class="{ 'battle-report-editor-modal--view': isReadonly, 'battle-report-editor-modal--edit': !isReadonly }"
    :title="isReadonly ? '战报总结' : '编辑战报'"
    :bordered="false"
    :style="{ width: 'min(1100px, calc(100vw - 24px))', maxWidth: 'none' }"
    :content-style="{ maxHeight: 'none', overflow: 'hidden' }"
    :auto-focus="false"
    @update:show="emit('update:visible', $event)"
  >
    <div class="battle-report-content-modal__header">
      <div class="battle-report-content-modal__heading">
        <n-input
          v-if="!isReadonly"
          v-model:value="form.title"
          class="battle-report-content-modal__title-input"
          maxlength="120"
          show-count
          placeholder="战报标题"
        />
        <div v-else class="battle-report-content-modal__title">{{ report?.title || '未命名战报' }}</div>
        <div class="battle-report-content-modal__meta">
          <span>{{ periodText }}</span>
          <span class="battle-report-content-modal__mode">{{ isReadonly ? '查看模式' : '编辑模式' }}</span>
        </div>
      </div>
    </div>
    <n-alert v-if="isFailed" type="error" :show-icon="false" class="battle-report-editor-modal__alert">
      {{ report?.errorMessage || (isReadonly ? '生成失败。' : '生成失败，可编辑后保存，或重新新建总结。') }}
    </n-alert>
    <n-alert v-else-if="isGenerating" type="info" :show-icon="false" class="battle-report-editor-modal__alert">
      AI 总结仍在生成中，完成后会自动填充内容。
    </n-alert>
    <n-form v-if="!isReadonly" label-placement="top">
      <n-form-item label="内容">
        <n-input
          class="battle-report-content-modal__content-input"
          v-model:value="form.content"
          type="textarea"
          :autosize="false"
          :rows="18"
          placeholder="纯文本战报内容"
        />
      </n-form-item>
    </n-form>
    <div v-else class="battle-report-content-modal__body">{{ report?.content || report?.contentPreview || '暂无内容' }}</div>
    <template #footer>
      <n-space v-if="!isReadonly" class="battle-report-content-modal__footer-actions" justify="end">
        <n-button @click="close">取消</n-button>
        <n-button type="primary" :disabled="isGenerating" @click="save">保存</n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<style scoped>
.battle-report-editor-modal {
  width: min(1100px, calc(100vw - 24px));
  max-width: none;
  border: 0;
  box-shadow: 0 24px 80px rgba(15, 23, 42, 0.24);
}

.battle-report-editor-modal :deep(.n-card) {
  width: 100%;
  max-width: none;
  border: 0;
}

.battle-report-editor-modal--view {
  --battle-report-modal-accent: #2563eb;
}

.battle-report-editor-modal--edit {
  --battle-report-modal-accent: #0f766e;
}

.battle-report-content-modal__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
}

.battle-report-content-modal__heading {
  min-width: 0;
}

.battle-report-content-modal__title {
  overflow-wrap: anywhere;
  font-size: 22px;
  font-weight: 800;
  line-height: 1.3;
}

.battle-report-content-modal__title-input {
  width: min(720px, 100%);
  font-size: 22px;
  font-weight: 800;
}

.battle-report-editor-modal--edit :deep(.battle-report-content-modal__content-input .n-input__textarea-el) {
  height: min(64vh, 620px) !important;
  min-height: min(64vh, 620px) !important;
  max-height: min(64vh, 620px) !important;
  overflow-y: auto;
  line-height: 1.7;
}

.battle-report-content-modal__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 6px;
  color: var(--text-color-3);
  font-size: 13px;
}

.battle-report-content-modal__mode {
  padding: 1px 8px;
  border-radius: 999px;
  background: rgba(15, 118, 110, 0.12);
  color: #0f766e;
}

.battle-report-content-modal__body {
  max-height: min(70vh, 720px);
  overflow: auto;
  padding: 16px;
  border: 1px solid rgba(148, 163, 184, 0.25);
  border-radius: 12px;
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.75;
}

.battle-report-editor-modal__alert {
  margin-bottom: 12px;
}

.battle-report-content-modal__footer-actions {
  padding-top: 18px;
}

@media (max-width: 720px) {
  .battle-report-editor-modal {
    width: calc(100vw - 12px);
  }

  .battle-report-content-modal__body {
    max-height: 70vh;
    padding: 12px;
  }

  .battle-report-editor-modal--edit :deep(.battle-report-content-modal__content-input .n-input__textarea-el) {
    height: 58vh !important;
    min-height: 58vh !important;
    max-height: 58vh !important;
  }

  :deep(.n-card-header),
  :deep(.n-card__content),
  :deep(.n-card__footer) {
    padding-left: 14px;
    padding-right: 14px;
  }
}
</style>
