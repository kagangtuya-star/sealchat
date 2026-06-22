<script setup lang="ts">
import { ref, watch, onBeforeUnmount } from 'vue'
import { useDisplayStore } from '@/stores/display'
import type { ToolbarHotkeyKey, ToolbarHotkeyConfig } from '@/stores/display'
import { useMessage } from 'naive-ui'
import { useEventListener } from '@vueuse/core'
import { buildHotkeyDescriptor, formatHotkeyCombo } from '@/utils/hotkey'

interface Props {
  show: boolean
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
}>()

const display = useDisplayStore()
const message = useMessage()

// 工具栏快捷键配置项
interface HotkeyItem {
  key: ToolbarHotkeyKey
  label: string
  description: string
}

const hotkeyItems: HotkeyItem[] = [
  { key: 'icToggle', label: '场内/场外切换', description: '快速切换 IC/OOC 模式' },
  { key: 'interject', label: '插话', description: '发送当前消息后进入插话流程' },
  { key: 'whisper', label: '悄悄话面板', description: '打开/关闭悄悄话选择面板' },
  { key: 'upload', label: '上传图片', description: '快速触发图片上传' },
  { key: 'richMode', label: '富文本模式', description: '切换富文本/纯文本编辑模式' },
  { key: 'broadcast', label: '实时广播', description: '切换实时预览广播状态' },
  { key: 'emoji', label: '表情面板', description: '打开/关闭表情选择面板' },
  { key: 'wideInput', label: '广域输入模式', description: '切换广域输入面板' },
  { key: 'history', label: '输入历史', description: '打开输入历史面板' },
  { key: 'diceTray', label: '掷骰面板', description: '打开/关闭掷骰托盘' },
]

// 本地草稿，避免直接修改 store
const draft = ref<Record<ToolbarHotkeyKey, ToolbarHotkeyConfig>>({} as any)

// 录制状态
const recordingTarget = ref<ToolbarHotkeyKey | null>(null)
let stopListener: (() => void) | null = null

// 初始化草稿
const initDraft = () => {
  const config = display.settings.toolbarHotkeys
  const copy: Record<string, ToolbarHotkeyConfig> = {}
  Object.entries(config).forEach(([key, value]) => {
    copy[key] = {
      enabled: value.enabled,
      hotkey: value.hotkey ? { ...value.hotkey } : null,
    }
  })
  draft.value = copy as Record<ToolbarHotkeyKey, ToolbarHotkeyConfig>
}

// 关闭录制
const stopRecording = () => {
  stopListener?.()
  stopListener = null
  recordingTarget.value = null
}

// 处理快捷键捕获
const handleKeyCapture = (event: KeyboardEvent) => {
  if (!recordingTarget.value) return
  event.preventDefault()
  event.stopPropagation()

  // ESC 取消录制
  if (!event.ctrlKey && !event.metaKey && !event.altKey && event.key === 'Escape' && !event.shiftKey) {
    stopRecording()
    message.info('已取消快捷键录制')
    return
  }

  const descriptor = buildHotkeyDescriptor(event)
  if (!descriptor) {
    message.warning('请按下包含 Ctrl/Cmd/Alt 的组合键')
    return
  }

  // 检查冲突
  const conflict = Object.entries(draft.value).find(
    ([key, config]) =>
      key !== recordingTarget.value && config.hotkey?.combo === descriptor.combo
  )

  if (conflict) {
    message.error(`该快捷键已被「${hotkeyItems.find((item) => item.key === conflict[0])?.label}」使用`)
    return
  }

  // 应用新快捷键
  const target = recordingTarget.value
  draft.value[target].hotkey = descriptor
  message.success(`已更新为 ${descriptor.combo}`)
  stopRecording()
}

// 开始录制
const beginRecording = (key: ToolbarHotkeyKey) => {
  stopRecording()
  recordingTarget.value = key
  stopListener = useEventListener(window, 'keydown', handleKeyCapture, {
    capture: true,
    passive: false,
  })
  message.info('请按下要绑定的组合键，按 ESC 取消')
}

// 清除快捷键
const clearHotkey = (key: ToolbarHotkeyKey) => {
  draft.value[key].hotkey = null
  message.success('已清除快捷键')
}

// 切换启用状态
const toggleEnabled = (key: ToolbarHotkeyKey, value: boolean) => {
  draft.value[key].enabled = value
}

// 保存设置
const handleSave = () => {
  display.updateSettings({ toolbarHotkeys: { ...draft.value } })
  message.success('快捷键设置已保存')
  handleClose()
}

// 关闭面板
const handleClose = () => {
  stopRecording()
  emit('update:show', false)
}

// 重置为默认
const handleReset = () => {
  initDraft()
  message.success('已重置为默认配置')
}

watch(
  () => props.show,
  (visible) => {
    if (visible) {
      initDraft()
    } else {
      stopRecording()
    }
  },
  { immediate: true }
)

onBeforeUnmount(() => {
  stopRecording()
})
</script>

<template>
  <n-modal
    preset="card"
    :show="props.show"
    title="快捷键管理"
    class="shortcut-settings-panel"
    :style="{ width: '620px' }"
    @update:show="emit('update:show', $event)"
  >
    <n-alert
      v-if="recordingTarget"
      type="info"
      size="small"
      :bordered="false"
      class="shortcut-settings-panel__alert"
    >
      正在录制快捷键，按 ESC 取消。
    </n-alert>

    <section class="shortcut-settings-panel__section">
      <header>
        <p class="section-title">工具栏快捷键</p>
        <p class="section-desc">自定义各功能的快捷键绑定，快捷键需包含 Ctrl/Cmd/Alt 中至少一个按键</p>
      </header>

      <div class="shortcut-list">
        <div v-for="item in hotkeyItems" :key="item.key" class="shortcut-item">
          <div class="shortcut-item__meta">
            <div class="shortcut-item__header">
              <span class="shortcut-item__label">{{ item.label }}</span>
              <n-switch
                size="small"
                :value="draft[item.key]?.enabled"
                @update:value="(val) => toggleEnabled(item.key, val)"
              >
                <template #checked>启用</template>
                <template #unchecked>禁用</template>
              </n-switch>
            </div>
            <p class="shortcut-item__desc">{{ item.description }}</p>
            <div class="shortcut-item__hotkey">
              <span class="shortcut-item__hotkey-label">
                快捷键：{{ formatHotkeyCombo(draft[item.key]?.hotkey) || '未设置' }}
              </span>
              <div class="shortcut-item__actions">
                <n-button text size="tiny" @click="beginRecording(item.key)">
                  录制
                </n-button>
                <n-button
                  text
                  size="tiny"
                  :disabled="!draft[item.key]?.hotkey"
                  @click="clearHotkey(item.key)"
                >
                  清除
                </n-button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>

    <template #footer>
      <div class="shortcut-settings-panel__footer">
        <n-space size="small">
          <n-button size="small" @click="handleClose">取消</n-button>
          <n-button size="small" tertiary @click="handleReset">重置默认</n-button>
        </n-space>
        <n-button type="primary" size="small" @click="handleSave">保存设置</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped lang="scss">
.shortcut-settings-panel :global(.n-card) {
  background-color: var(--sc-bg-elevated);
  border: 1px solid var(--sc-border-strong);
  color: var(--sc-text-primary);
}

.shortcut-settings-panel__alert {
  margin-bottom: 1rem;
}

.shortcut-settings-panel__section + .shortcut-settings-panel__section {
  margin-top: 1rem;
}

.section-title {
  font-size: 0.95rem;
  font-weight: 600;
  color: var(--sc-text-primary);
  margin: 0;
}

.section-desc {
  margin: 0.25rem 0 0;
  font-size: 0.85rem;
  color: var(--sc-text-secondary);
}

.shortcut-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  margin-top: 0.75rem;
}

.shortcut-item {
  padding: 0.75rem;
  border: 1px solid var(--sc-border-soft, rgba(148, 163, 184, 0.2));
  border-radius: 8px;
  background: var(--sc-bg-surface);
  transition: border-color 0.2s ease;
}

.shortcut-item:hover {
  border-color: var(--sc-border-medium, rgba(148, 163, 184, 0.35));
}

.shortcut-item__meta {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.shortcut-item__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.shortcut-item__label {
  font-weight: 600;
  font-size: 0.9rem;
  color: var(--sc-text-primary);
}

.shortcut-item__desc {
  margin: 0;
  font-size: 0.8rem;
  color: var(--sc-text-secondary);
}

.shortcut-item__hotkey {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding-top: 0.25rem;
  border-top: 1px dashed var(--sc-border-soft, rgba(148, 163, 184, 0.2));
}

.shortcut-item__hotkey-label {
  font-size: 0.85rem;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  color: var(--sc-text-primary);
}

.shortcut-item__actions {
  display: inline-flex;
  gap: 0.25rem;
  flex-shrink: 0;
}

.shortcut-settings-panel__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
}
</style>
