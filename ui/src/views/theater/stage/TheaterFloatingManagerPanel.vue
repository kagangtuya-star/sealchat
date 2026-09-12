<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { NButton, NIcon, NInput, NInputNumber, NSelect, useMessage } from 'naive-ui'
import { AppWindow, ArrowUp, ArrowsMaximize, Eye, EyeOff, FileText, Id, LayoutBoard, Message, Minus, Note, Pencil, Plus, Search, Trash, World, X } from '@vicons/tabler'
import type { TheaterFloatingWindowAction, TheaterFloatingWindowSummary } from '../host/theater-floating-window'
import { resolveSafeStageIframeUrl } from '../shared/stage-types'

const props = defineProps<{
  windows: TheaterFloatingWindowSummary[]
  channelOptions: { value: string; label: string }[]
}>()
const emit = defineEmits<{ action: [action: TheaterFloatingWindowAction] }>()
const message = useMessage()
const inspectorMode = ref<'add' | 'edit' | null>(null)
const editingWindowId = ref<string | null>(null)
const editingWindow = computed(() => (
  editingWindowId.value
    ? props.windows.find(item => item.id === editingWindowId.value) || null
    : null
))
const channelId = ref<string | null>(null)
const title = ref('')
const url = ref('')
const width = ref<number | null>(null)
const height = ref<number | null>(null)
const menuProps = { class: 'theater-floating-channel-select-menu' }
const displayWindows = computed(() => [...props.windows].sort((a, b) => b.zIndex - a.zIndex))
const types = {
  character: { label: '人物卡', icon: Id },
  note: { label: '便签', icon: Note },
  clue: { label: '线索', icon: Search },
  'clue-board': { label: '线索板', icon: LayoutBoard },
  iform: { label: 'IForm', icon: FileText },
  web: { label: '网页', icon: World },
  chat: { label: '聊天', icon: Message },
}
const windowType = (item: TheaterFloatingWindowSummary) => (
  item.source === 'internal'
    ? (item.resourceType ? types[item.resourceType] : { label: '内部窗口', icon: AppWindow })
    : types[item.source]
)

const clearDraft = () => {
  channelId.value = null
  title.value = ''
  url.value = ''
  width.value = null
  height.value = null
}

const closeInspector = () => {
  inspectorMode.value = null
  editingWindowId.value = null
  clearDraft()
}

watch(
  () => editingWindow.value,
  (item) => {
    if (inspectorMode.value === 'edit' && !item) closeInspector()
  },
)

const enterAddMode = () => {
  if (inspectorMode.value === 'add') {
    closeInspector()
    return
  }
  closeInspector()
  inspectorMode.value = 'add'
}

const editWindow = (item: TheaterFloatingWindowSummary) => {
  if (item.source !== 'web' && item.source !== 'chat') return
  inspectorMode.value = 'edit'
  editingWindowId.value = item.id
  title.value = item.source === 'web' ? item.title : ''
  url.value = item.source === 'web' ? (item.url || '') : ''
  width.value = item.source === 'web' ? item.width : null
  height.value = item.source === 'web' ? item.height : null
  channelId.value = item.source === 'chat' ? (item.targetChannelId || null) : null
}

const addChat = () => {
  if (!channelId.value) return
  emit('action', { type: 'add-chat', channelId: channelId.value })
  closeInspector()
}

const addWeb = () => {
  const normalized = resolveSafeStageIframeUrl(url.value)
  if (!normalized) {
    message.warning('请输入有效的 http / https URL')
    return
  }
  emit('action', { type: 'add-web', title: title.value, url: normalized, width: width.value ?? undefined, height: height.value ?? undefined })
  closeInspector()
}

const updateWeb = () => {
  if (!editingWindowId.value || !editingWindow.value) return
  const normalized = resolveSafeStageIframeUrl(url.value)
  if (!normalized) {
    message.warning('请输入有效的 http / https URL')
    return
  }
  emit('action', {
    type: 'update-web',
    id: editingWindowId.value,
    title: title.value,
    url: normalized,
    width: width.value ?? undefined,
    height: height.value ?? undefined,
  })
  closeInspector()
}

const updateChat = () => {
  if (!editingWindowId.value || !channelId.value || !editingWindow.value) return
  emit('action', { type: 'update-chat', id: editingWindowId.value, channelId: channelId.value })
  closeInspector()
}
</script>

<template>
  <div class="floating-manager">
    <div class="floating-manager__summary">
      <small>{{ windows.length }} 个窗口 · {{ windows.filter(item => !item.hidden).length }} 可见</small>
      <div class="floating-manager__actions">
        <n-button size="small" quaternary :type="inspectorMode === 'add' ? 'primary' : 'default'" :title="inspectorMode === 'add' ? '关闭添加' : '添加'" :aria-label="inspectorMode === 'add' ? '关闭添加' : '添加'" @click="enterAddMode"><n-icon><Plus /></n-icon></n-button>
        <n-button size="small" quaternary :disabled="!windows.length" title="全部关闭" aria-label="全部关闭" @click="emit('action', { type: 'close-all' })"><n-icon><Trash /></n-icon></n-button>
      </div>
    </div>
    <div class="floating-manager__body" :class="{ 'has-inspector': inspectorMode !== null }">
      <div class="floating-manager__list">
        <div v-if="!windows.length" class="floating-manager__empty">暂无悬浮窗，点击添加打开网页或聊天频道</div>
        <article v-for="item in displayWindows" :key="item.id" class="floating-manager__row" :class="{ 'is-hidden': item.hidden }">
          <button type="button" class="floating-manager__select" :title="item.title" @click="emit('action', { type: 'focus', id: item.id })">
            <n-icon><component :is="windowType(item).icon" /></n-icon>
            <span><strong>{{ item.title }}</strong><small>{{ windowType(item).label }} · {{ item.hidden ? '已隐藏' : '可见' }}{{ item.minimized ? ' · 已最小化' : '' }}</small></span>
          </button>
          <div class="floating-manager__actions">
            <n-button text size="tiny" :title="item.hidden ? '显示' : '隐藏'" :aria-label="item.hidden ? '显示' : '隐藏'" @click="emit('action', { type: 'toggle-hidden', id: item.id })"><n-icon><Eye v-if="item.hidden" /><EyeOff v-else /></n-icon></n-button>
            <n-button text size="tiny" :title="item.minimized ? '恢复' : '最小化'" :aria-label="item.minimized ? '恢复' : '最小化'" @click="emit('action', { type: 'toggle-minimized', id: item.id })"><n-icon><ArrowsMaximize v-if="item.minimized" /><Minus v-else /></n-icon></n-button>
            <n-button text size="tiny" title="置顶" aria-label="置顶" @click="emit('action', { type: 'focus', id: item.id })"><n-icon><ArrowUp /></n-icon></n-button>
            <n-button v-if="item.source === 'web' || item.source === 'chat'" text size="tiny" title="编辑配置" aria-label="编辑配置" @click="editWindow(item)"><n-icon><Pencil /></n-icon></n-button>
            <n-button text size="tiny" title="关闭" aria-label="关闭" @click="emit('action', { type: 'close', id: item.id })"><n-icon><X /></n-icon></n-button>
          </div>
        </article>
      </div>
      <div v-if="inspectorMode !== null" class="floating-manager__inspector">
        <template v-if="inspectorMode === 'add'">
          <section>
            <strong>快捷添加聊天频道</strong>
            <n-select v-model:value="channelId" :options="channelOptions" :menu-props="menuProps" filterable clearable size="small" placeholder="选择当前世界频道" />
            <n-button size="small" secondary :disabled="!channelOptions.some(item => item.value === channelId)" @click="addChat"><template #icon><n-icon><Message /></n-icon></template>打开聊天浮窗</n-button>
          </section>
          <section>
            <strong>自定义网页</strong>
            <label>名称<n-input v-model:value="title" size="small" placeholder="网页" /></label>
            <label>URL<n-input v-model:value="url" size="small" placeholder="https://" @keydown.enter="addWeb" /></label>
            <div class="floating-manager__dimensions">
              <label>初始宽度<n-input-number v-model:value="width" size="small" :min="320" :precision="0" placeholder="520" /></label>
              <label>初始高度<n-input-number v-model:value="height" size="small" :min="220" :precision="0" placeholder="440" /></label>
            </div>
            <n-button size="small" secondary @click="addWeb"><template #icon><n-icon><Plus /></n-icon></template>打开网页浮窗</n-button>
          </section>
        </template>
        <template v-else-if="editingWindow?.source === 'web'">
          <section>
            <strong>编辑网页浮窗</strong>
            <label>名称<n-input v-model:value="title" size="small" placeholder="网页" /></label>
            <label>URL<n-input v-model:value="url" size="small" placeholder="https://" @keydown.enter="updateWeb" /></label>
            <div class="floating-manager__dimensions">
              <label>宽度<n-input-number v-model:value="width" size="small" :min="320" :precision="0" /></label>
              <label>高度<n-input-number v-model:value="height" size="small" :min="220" :precision="0" /></label>
            </div>
            <n-button size="small" type="primary" secondary @click="updateWeb">保存修改</n-button>
          </section>
        </template>
        <template v-else-if="editingWindow?.source === 'chat'">
          <section>
            <strong>编辑聊天浮窗</strong>
            <n-select v-model:value="channelId" :options="channelOptions" :menu-props="menuProps" filterable clearable size="small" placeholder="选择当前世界频道" />
            <n-button size="small" type="primary" secondary :disabled="!channelOptions.some(item => item.value === channelId)" @click="updateChat">保存修改</n-button>
          </section>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
.floating-manager { min-height: 0; display: flex; flex: 1; flex-direction: column; overflow: hidden; container-type: inline-size; }
.floating-manager__summary { box-sizing: border-box; min-height: 40px; display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 5px 8px; border-bottom: 1px solid var(--theater-border); }
.floating-manager small { color: var(--sc-text-secondary); font-size: 10px; }
.floating-manager__body { min-height: 0; display: grid; flex: 1; grid-template-columns: minmax(0, 1fr); overflow: hidden; }
.floating-manager__body.has-inspector { grid-template-columns: minmax(260px, 1fr) minmax(260px, 1fr); }
.floating-manager__list { min-width: 0; overflow: auto; }
.floating-manager__empty { padding: 36px 14px; color: var(--sc-text-secondary); font-size: 11px; text-align: center; }
.floating-manager__row { box-sizing: border-box; min-height: 42px; display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: 6px; padding: 4px 6px; border-bottom: 1px solid color-mix(in srgb, var(--theater-border) 68%, transparent); }
.floating-manager__row:hover { background: color-mix(in srgb, var(--theater-accent) 14%, transparent); }
.floating-manager__row.is-hidden .floating-manager__select { opacity: .55; }
.floating-manager__select { min-width: 0; display: flex; align-items: center; gap: 7px; border: 0; padding: 3px 0; color: inherit; background: transparent; text-align: left; cursor: pointer; }
.floating-manager__select > span { min-width: 0; display: grid; gap: 2px; }
.floating-manager__select strong, .floating-manager__select small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.floating-manager strong { font-size: 11px; }
.floating-manager__actions { display: flex; align-items: center; gap: 1px; }
.floating-manager__actions :deep(.n-button) { width: 24px; height: 24px; padding: 0; }
.floating-manager__inspector { min-width: 0; overflow: auto; padding: 9px; border-left: 1px solid var(--theater-border); background: var(--theater-panel); }
.floating-manager__inspector section { display: grid; gap: 8px; margin-bottom: 14px; }
.floating-manager__inspector label { min-width: 0; display: grid; gap: 4px; color: var(--sc-text-secondary); font-size: 11px; }
.floating-manager__dimensions { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 6px; }
@container (max-width: 560px) {
  .floating-manager__body.has-inspector { grid-template-columns: minmax(0, 1fr); grid-template-rows: minmax(0, .8fr) minmax(0, 1.2fr); }
  .floating-manager__inspector { border-top: 1px solid var(--theater-border); border-left: 0; }
}
:global(.n-select-menu.n-base-select-menu.theater-floating-channel-select-menu) {
  --n-color: var(--sc-bg-surface, #1b1b20) !important;
  --n-option-text-color: var(--sc-text-primary, #f4f4f5) !important;
  --n-option-text-color-active: var(--sc-text-primary, #f4f4f5) !important;
  --n-option-color-pending: var(--sc-bg-hover, rgba(255, 255, 255, .08)) !important;
  border: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .12));
}
:global(body:has(.theater-stage-app) .v-binder-follower-container:has(.theater-floating-channel-select-menu)) { z-index: 10003 !important; }
</style>
