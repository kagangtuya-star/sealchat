<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NButton, NCard, NInput, NModal } from 'naive-ui'

import { THEATER_EFFECT_WEB_HTML_MAX_BYTES, theaterEffectWebHtmlBytes } from './theater-effect-types'

const props = defineProps<{
  show: boolean
  value: string
  objectName: string
  readonly: boolean
}>()

const emit = defineEmits<{
  close: []
  save: [html: string]
}>()

const draft = ref('')
const htmlFileInputRef = ref<HTMLInputElement | null>(null)
const helpVisible = ref(false)
const importError = ref('')

watch(() => props.show, (show) => {
  if (show) {
    draft.value = props.value
    helpVisible.value = false
    importError.value = ''
  }
}, { immediate: true })

const bytes = computed(() => theaterEffectWebHtmlBytes(draft.value))
const overLimit = computed(() => bytes.value > THEATER_EFFECT_WEB_HTML_MAX_BYTES)
const sizeLabel = computed(() => `${(bytes.value / 1024).toFixed(1)} / ${THEATER_EFFECT_WEB_HTML_MAX_BYTES / 1024} KiB`)

const triggerHtmlImport = () => {
  if (!props.readonly) htmlFileInputRef.value?.click()
}

const handleHtmlImport = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file || props.readonly) return
  importError.value = ''
  try {
    draft.value = await file.text()
  } catch {
    importError.value = 'HTML 文件读取失败'
  }
}

const save = () => {
  if (props.readonly || overLimit.value) return
  emit('save', draft.value)
}
</script>

<template>
  <n-modal :show="show" :mask-closable="false" @update:show="value => { if (!value) emit('close') }">
    <n-card
      class="theater-effect-web-code-editor"
      :title="`网页代码 · ${objectName}`"
      closable
      style="width: min(860px, calc(100vw - 24px))"
      @close="emit('close')"
    >
      <input
        ref="htmlFileInputRef"
        type="file"
        accept=".html,.htm,text/html"
        hidden
        @change="handleHtmlImport"
      >
      <div class="theater-effect-web-code-editor__intro">
        <span>支持完整 HTML/CSS/SVG/Canvas/WebGL，单文件上限 128 KiB。</span>
        <div class="theater-effect-web-code-editor__intro-actions">
          <n-button v-if="!readonly" size="small" secondary class="theater-effect-web-code-editor__upload" @click="triggerHtmlImport">
            上传 HTML
          </n-button>
          <n-button
            size="small"
            quaternary
            circle
            class="theater-effect-web-code-editor__help-button"
            :type="helpVisible ? 'primary' : 'default'"
            aria-label="查看网页代码运行说明"
            @click="helpVisible = !helpVisible"
          >?</n-button>
        </div>
      </div>
      <p v-if="helpVisible" class="theater-effect-web-code-editor__help">
        代码在隔离沙箱中运行，默认透明且不可交互，不能访问 SealChat 页面、Cookie 或同源存储，也没有弹窗、表单和顶层导航权限；可加载外部 HTTPS 资源。每次播放都会重新执行。
      </p>
      <p v-if="importError" class="theater-effect-web-code-editor__error">{{ importError }}</p>
      <n-input
        v-model:value="draft"
        class="theater-effect-web-code-editor__input"
        type="textarea"
        :readonly="readonly"
        :rows="22"
        placeholder="粘贴或编写完整 HTML 文档"
        :input-props="{ spellcheck: false, autocomplete: 'off', autocapitalize: 'off' }"
      />
      <template #footer>
        <div class="theater-effect-web-code-editor__actions">
          <span :class="{ 'is-error': overLimit }">{{ overLimit ? '代码超出大小上限：' : '' }}{{ sizeLabel }}</span>
          <n-button @click="emit('close')">{{ readonly ? '关闭' : '取消' }}</n-button>
          <n-button v-if="!readonly" type="primary" :disabled="overLimit" @click="save">保存</n-button>
        </div>
      </template>
    </n-card>
  </n-modal>
</template>

<style scoped>
.theater-effect-web-code-editor { max-height: calc(100vh - 24px); overflow: auto; }
.theater-effect-web-code-editor__intro { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin: 0 0 10px; color: var(--n-text-color-2); font-size: 12px; line-height: 1.4; }
.theater-effect-web-code-editor__intro-actions { display: flex; flex: 0 0 auto; align-items: center; gap: 5px; }
.theater-effect-web-code-editor__upload { opacity: .78; }
.theater-effect-web-code-editor__upload:hover { opacity: 1; }
.theater-effect-web-code-editor__help-button { font-weight: 700; }
.theater-effect-web-code-editor__help { margin: -2px 0 10px; padding: 8px 10px; border-radius: 6px; color: var(--n-text-color-2); background: rgba(127, 127, 127, .08); font-size: 12px; line-height: 1.5; }
.theater-effect-web-code-editor__error { margin: -2px 0 8px; color: #f87171; font-size: 12px; }
.theater-effect-web-code-editor__input { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 12px; }
.theater-effect-web-code-editor__actions { display: grid; grid-template-columns: 1fr auto auto; align-items: center; gap: 8px; }
.theater-effect-web-code-editor__actions > span { color: var(--n-text-color-2); font-size: 12px; }
.theater-effect-web-code-editor__actions > span.is-error { color: #f87171; }
@media (max-width: 560px) {
  .theater-effect-web-code-editor__intro { align-items: flex-start; }
  .theater-effect-web-code-editor__intro > span { min-width: 0; }
}
</style>
