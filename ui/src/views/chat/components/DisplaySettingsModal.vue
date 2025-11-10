<script setup lang="ts">
import { reactive, watch, computed } from 'vue'
import type { DisplaySettings } from '@/stores/display'

interface Props {
  visible: boolean
  settings: DisplaySettings
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'save', value: DisplaySettings): void
}>()

const draft = reactive<DisplaySettings>({
  layout: 'bubble',
  palette: 'day',
  showAvatar: true,
  mergeNeighbors: true,
  maxExportMessages: 5000,
  maxExportConcurrency: 2,
  fontSize: 15,
  lineHeight: 1.6,
  letterSpacing: 0,
  bubbleGap: 12,
  paragraphSpacing: 8,
  messagePaddingX: 18,
  messagePaddingY: 14,
})

watch(
  () => props.settings,
  (value) => {
    if (!value) return
    draft.layout = value.layout
    draft.palette = value.palette
    draft.showAvatar = value.showAvatar
    draft.mergeNeighbors = value.mergeNeighbors
    draft.maxExportMessages = value.maxExportMessages
    draft.maxExportConcurrency = value.maxExportConcurrency
    draft.fontSize = value.fontSize
    draft.lineHeight = value.lineHeight
    draft.letterSpacing = value.letterSpacing
    draft.bubbleGap = value.bubbleGap
    draft.paragraphSpacing = value.paragraphSpacing
    draft.messagePaddingX = value.messagePaddingX
    draft.messagePaddingY = value.messagePaddingY
  },
  { deep: true, immediate: true },
)

const previewClasses = computed(() => [
  'display-preview',
  `display-preview--${draft.palette}`,
  `display-preview--${draft.layout}`,
])

const previewStyleVars = computed(() => ({
  '--chat-font-size': `${draft.fontSize / 16}rem`,
  '--chat-line-height': `${draft.lineHeight}`,
  '--chat-letter-spacing': `${draft.letterSpacing}px`,
  '--chat-bubble-gap': `${draft.bubbleGap}px`,
  '--chat-paragraph-spacing': `${draft.paragraphSpacing}px`,
  '--chat-message-padding-x': `${draft.messagePaddingX}px`,
  '--chat-message-padding-y': `${draft.messagePaddingY}px`,
}))

const formatPxTooltip = (value: number) => `${Math.round(value)}px`
const formatLetterSpacingTooltip = (value: number) => `${value.toFixed(1)}px`
const formatLineHeightTooltip = (value: number) => value.toFixed(2)
type NumericSettingKey =
  | 'fontSize'
  | 'lineHeight'
  | 'letterSpacing'
  | 'bubbleGap'
  | 'paragraphSpacing'
  | 'messagePaddingX'
  | 'messagePaddingY'
const handleNumericInput = (key: NumericSettingKey) => (value: number | null) => {
  if (value === null) return
  draft[key] = value as DisplaySettings[NumericSettingKey]
}

const handleClose = () => emit('update:visible', false)
const handleConfirm = () => emit('save', { ...draft })
</script>

<template>
  <n-modal
    preset="card"
    :show="props.visible"
    title="显示模式"
    :style="{ width: '520px' }"
    @update:show="emit('update:visible', $event)"
  >
    <div class="display-settings">
      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">版式</p>
            <p class="section-desc">气泡模式强调对话气泡，紧凑模式更接近论坛流</p>
          </div>
        </header>
        <n-radio-group v-model:value="draft.layout" size="large">
          <n-radio-button value="bubble">气泡模式</n-radio-button>
          <n-radio-button value="compact">紧凑模式</n-radio-button>
        </n-radio-group>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">主题</p>
            <p class="section-desc">在日间/夜间之间切换沉浸背景</p>
          </div>
        </header>
        <n-radio-group v-model:value="draft.palette" size="large">
          <n-radio-button value="day">日间模式</n-radio-button>
          <n-radio-button value="night">夜间模式</n-radio-button>
        </n-radio-group>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">头像显示</p>
            <p class="section-desc">隐藏头像可获得更紧凑的布局</p>
          </div>
        </header>
        <n-switch v-model:value="draft.showAvatar">
          <template #checked>显示头像</template>
          <template #unchecked>隐藏头像</template>
        </n-switch>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">合并连续消息</p>
            <p class="section-desc">相邻同角色消息视作一段，拖动可拆分</p>
          </div>
        </header>
        <n-switch v-model:value="draft.mergeNeighbors">
          <template #checked>已启用</template>
          <template #unchecked>已关闭</template>
        </n-switch>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">排版（字号 / 行距 / 字距）</p>
            <p class="section-desc">控制阅读密度，满足不同屏幕与视力偏好</p>
          </div>
        </header>
        <div class="display-settings__controls">
          <div class="control-field">
            <div>
              <p class="control-title">字号</p>
              <p class="control-desc">影响聊天内容与预览文本大小</p>
            </div>
            <div class="control-input">
              <n-slider v-model:value="draft.fontSize" :min="12" :max="22" :step="1" :format-tooltip="formatPxTooltip" />
              <n-input-number
                :value="draft.fontSize"
                size="small"
                :min="12"
                :max="22"
                @update:value="handleNumericInput('fontSize')"
              />
            </div>
          </div>
          <div class="control-field">
            <div>
              <p class="control-title">行距</p>
              <p class="control-desc">控制段落纵向密度</p>
            </div>
            <div class="control-input">
              <n-slider
                v-model:value="draft.lineHeight"
                :min="1.2"
                :max="2"
                :step="0.05"
                :format-tooltip="formatLineHeightTooltip"
              />
              <n-input-number
                :value="draft.lineHeight"
                size="small"
                :min="1.2"
                :max="2"
                :step="0.05"
                @update:value="handleNumericInput('lineHeight')"
              />
            </div>
          </div>
          <div class="control-field">
            <div>
              <p class="control-title">字距</p>
              <p class="control-desc">微调字符间隔，提升可读性</p>
            </div>
            <div class="control-input">
              <n-slider
                v-model:value="draft.letterSpacing"
                :min="-1"
                :max="2"
                :step="0.1"
                :format-tooltip="formatLetterSpacingTooltip"
              />
              <n-input-number
                :value="draft.letterSpacing"
                size="small"
                :min="-1"
                :max="2"
                :step="0.1"
                @update:value="handleNumericInput('letterSpacing')"
              />
            </div>
          </div>
        </div>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">气泡与段落间距</p>
            <p class="section-desc">调节消息块之间、段落之间的空白</p>
          </div>
        </header>
        <div class="display-settings__controls">
          <div class="control-field">
            <div>
              <p class="control-title">气泡间距</p>
              <p class="control-desc">作用于消息行之间的 gap</p>
            </div>
            <div class="control-input">
              <n-slider v-model:value="draft.bubbleGap" :min="4" :max="48" :step="2" :format-tooltip="formatPxTooltip" />
              <n-input-number
                :value="draft.bubbleGap"
                size="small"
                :min="4"
                :max="48"
                :step="2"
                @update:value="handleNumericInput('bubbleGap')"
              />
            </div>
          </div>
          <div class="control-field">
            <div>
              <p class="control-title">段落间距</p>
              <p class="control-desc">连续段落之间的外边距</p>
            </div>
            <div class="control-input">
              <n-slider
                v-model:value="draft.paragraphSpacing"
                :min="0"
                :max="24"
                :step="1"
                :format-tooltip="formatPxTooltip"
              />
              <n-input-number
                :value="draft.paragraphSpacing"
                size="small"
                :min="0"
                :max="24"
                @update:value="handleNumericInput('paragraphSpacing')"
              />
            </div>
          </div>
        </div>
      </section>

      <section class="display-settings__section">
        <header>
          <div>
            <p class="section-title">气泡内边距</p>
            <p class="section-desc">对齐不同设备的左右/上下空白</p>
          </div>
        </header>
        <div class="display-settings__controls">
          <div class="control-field">
            <div>
              <p class="control-title">左右内边距</p>
              <p class="control-desc">默认 18px，可适配窄屏</p>
            </div>
            <div class="control-input">
              <n-slider
                v-model:value="draft.messagePaddingX"
                :min="8"
                :max="48"
                :step="1"
                :format-tooltip="formatPxTooltip"
              />
              <n-input-number
                :value="draft.messagePaddingX"
                size="small"
                :min="8"
                :max="48"
                @update:value="handleNumericInput('messagePaddingX')"
              />
            </div>
          </div>
          <div class="control-field">
            <div>
              <p class="control-title">上下内边距</p>
              <p class="control-desc">默认 14px，影响气泡高度</p>
            </div>
            <div class="control-input">
              <n-slider
                v-model:value="draft.messagePaddingY"
                :min="4"
                :max="32"
                :step="1"
                :format-tooltip="formatPxTooltip"
              />
              <n-input-number
                :value="draft.messagePaddingY"
                size="small"
                :min="4"
                :max="32"
                @update:value="handleNumericInput('messagePaddingY')"
              />
            </div>
          </div>
        </div>
      </section>

      <section class="display-settings__section">
        <header class="preview-header">
          <div>
            <p class="section-title">实时预览</p>
            <p class="section-desc">排版参数实时映射至聊天气泡</p>
          </div>
        </header>
        <div :class="previewClasses" :style="previewStyleVars">
          <div class="preview-card">
            <div class="preview-avatar" />
            <div>
              <p class="preview-name">晨星角色 · 场内</p>
              <p class="preview-body">采用 {{ draft.layout === 'bubble' ? '气泡' : '紧凑' }} 模式展示。</p>
            </div>
          </div>
          <div class="preview-card preview-card--ooc">
            <div class="preview-avatar" />
            <div>
              <p class="preview-name">旁白 · 场外</p>
              <p class="preview-body">日夜模式在此同步变化。</p>
            </div>
          </div>
          <div class="preview-card preview-card--preview">
            <div>
              <p class="preview-name">实时预览</p>
              <p class="preview-body">无气泡，使用密排圆点背景。</p>
            </div>
          </div>
        </div>
      </section>

      <n-space justify="space-between" align="center" class="display-settings__footer">
        <n-button quaternary size="small" @click="handleClose">取消</n-button>
        <n-button type="primary" size="small" @click="handleConfirm">应用设置</n-button>
      </n-space>
    </div>
  </n-modal>
</template>

<style scoped lang="scss">
:deep(.n-card) {
  background-color: var(--sc-bg-elevated);
  border: 1px solid var(--sc-border-strong);
  color: var(--sc-text-primary);
}

.display-settings {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  color: var(--sc-text-primary);
}

.display-settings__controls {
  display: flex;
  flex-direction: column;
  gap: 0.85rem;
}

.control-field {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  align-items: center;
}

.control-title {
  font-size: 0.85rem;
  font-weight: 600;
}

.control-desc {
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
  margin-top: 0.15rem;
}

.control-input {
  flex: 1;
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 0.6rem;
  align-items: center;
}

.control-input :deep(.n-slider) {
  margin: 0;
}

.display-settings__section header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.45rem;
}

.section-title {
  font-size: 0.95rem;
  font-weight: 600;
  color: var(--sc-text-primary);
}

.section-desc {
  font-size: 0.8rem;
  color: var(--sc-text-secondary);
  margin-top: 0.15rem;
}

.display-preview {
  border-radius: 0.9rem;
  padding: 0.9rem;
  display: flex;
  flex-direction: column;
  gap: var(--chat-bubble-gap, 0.65rem);
  border: 1px solid var(--sc-border-mute);
  background: linear-gradient(135deg, var(--sc-bg-surface), var(--sc-bg-elevated));
}

.display-preview--night {
  background: linear-gradient(135deg, var(--sc-bg-header), var(--sc-bg-elevated));
  border-color: var(--sc-border-strong);
}

.display-preview .preview-card {
  display: flex;
  gap: 0.75rem;
  padding: var(--chat-message-padding-y, 0.65rem) var(--chat-message-padding-x, 0.75rem);
  border-radius: var(--preview-radius, 1rem);
  background-color: var(--sc-bg-surface);
  border: 1px solid var(--sc-border-mute);
}

.display-preview--night .preview-card {
  background-color: var(--sc-bg-input);
  color: var(--sc-text-primary);
}

.display-preview--night .preview-card--ooc {
  background-color: var(--chat-ooc-bg);
}

.display-preview--night .preview-card--preview {
  background-image: radial-gradient(var(--chat-preview-dot) 1px, transparent 1px);
  background-color: var(--chat-preview-bg);
  background-size: 6px 6px;
}

.display-preview--night .preview-name {
  color: var(--sc-text-primary);
}

.display-preview--night .preview-body {
  color: var(--sc-text-secondary);
}

.preview-card--ooc {
  background-color: var(--chat-ooc-bg);
}

.preview-card--preview {
  flex-direction: column;
  background-color: var(--chat-preview-bg);
  background-image: radial-gradient(var(--chat-preview-dot) 1px, transparent 1px);
  background-size: 6px 6px;
}

.preview-avatar {
  width: 2.25rem;
  height: 2.25rem;
  border-radius: 0.75rem;
  background: linear-gradient(135deg, #f87171, #fbbf24);
  border: 1px solid var(--sc-border-mute);
}

.preview-name {
  font-size: calc(var(--chat-font-size, 0.95rem) - 0.05rem);
  font-weight: 600;
  color: var(--sc-text-primary);
}

.preview-body {
  font-size: var(--chat-font-size, 0.95rem);
  line-height: var(--chat-line-height, 1.6);
  letter-spacing: var(--chat-letter-spacing, 0px);
  color: var(--sc-text-secondary);
}

.display-preview--compact {
  --preview-radius: 0.75rem;
}

.display-settings__footer {
  margin-top: 0.5rem;
}
</style>
