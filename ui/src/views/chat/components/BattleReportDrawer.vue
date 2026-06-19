<script setup lang="ts">
import dayjs from 'dayjs'
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useDialog, useMessage } from 'naive-ui'
import { useBattleReportStore } from '@/stores/battleReport'
import { useAIStore } from '@/stores/ai'
import { chatEvent, useChatStore } from '@/stores/chat'
import type { BattleReport, SChannel } from '@/types'
import { copyTextWithFallback } from '@/utils/clipboard'
import { generateBattleReportEmbedLink } from '@/utils/battleReportEmbedLink'
import ActiveDayDateRangePicker from './export/ActiveDayDateRangePicker.vue'
import BattleReportEditorModal from './BattleReportEditorModal.vue'

interface Props {
  visible: boolean
  channelId?: string
  worldId?: string
}

interface Emits {
  (e: 'update:visible', value: boolean): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const message = useMessage()
const dialog = useDialog()
const store = useBattleReportStore()
const aiStore = useAIStore()
const chat = useChatStore()

const createVisible = ref(false)
const displayVisible = ref(false)
const editorVisible = ref(false)
const editingReportId = ref('')
const draggedId = ref('')
const createMode = ref<'manual' | 'ai'>('ai')
const localSummaryRunning = ref(false)
const localSummaryStatus = ref('')
const createForm = reactive({
  title: '',
  content: '',
  period: null as [number, number] | null,
  contextReportCount: 3,
  sourceChannelIds: [] as string[],
})
const displayForm = reactive({
  name: '战报时间线',
})
let pollTimer: number | null = null

const displayChannel = computed(() => props.channelId ? (store.displayByChannel[props.channelId] || null) : null)
const sourceChannelId = computed(() => displayChannel.value?.sourceChannelId || props.channelId || '')
const displayChannelId = computed(() => displayChannel.value?.displayChannelId || '')
const sourceReports = computed(() => sourceChannelId.value ? (store.itemsByChannel[sourceChannelId.value] || []) : [])
const editingReport = computed(() => editingReportId.value ? store.detailById[editingReportId.value] : null)
const hasGenerating = computed(() => sourceReports.value.some((item) => item.status === 'generating'))
const createSubmitting = computed(() => store.saving || localSummaryRunning.value)

const flattenChannels = (items: SChannel[] = []): SChannel[] => {
  const out: SChannel[] = []
  items.forEach((item) => {
    if (!item?.id) return
    out.push(item)
    if (Array.isArray(item.children) && item.children.length) {
      out.push(...flattenChannels(item.children))
    }
  })
  return out
}

const createChannelOptions = computed(() => flattenChannels(chat.currentWorldChannels)
  .filter((item) => !item.isPrivate)
  .map((item) => ({
    label: item.name || `频道 ${item.id.slice(0, 6)}`,
    value: item.id,
  })))

const normalizeCreateSourceChannelIds = () => {
  const validIds = new Set(createChannelOptions.value.map((item) => item.value))
  const rawSelected = Array.isArray(createForm.sourceChannelIds) ? createForm.sourceChannelIds : []
  const selected = rawSelected
    .map((item) => String(item || '').trim())
    .filter((item, index, list) => item && list.indexOf(item) === index && (!validIds.size || validIds.has(item)))
  if (selected.length) return selected
  return sourceChannelId.value ? [sourceChannelId.value] : []
}

const createPrimaryChannelId = computed(() => normalizeCreateSourceChannelIds()[0] || sourceChannelId.value)

const formatPeriod = (item: BattleReport) => {
  if (!item.periodStart || !item.periodEnd) return '未设置周期'
  return `${dayjs(item.periodStart).format('YYYY-MM-DD HH:mm')} - ${dayjs(item.periodEnd).format('YYYY-MM-DD HH:mm')}`
}

const previewText = (item: BattleReport) => (item.contentPreview || item.content || '暂无内容').slice(0, 200)

const resetCreateForm = () => {
  createMode.value = 'ai'
  createForm.title = ''
  createForm.content = ''
  createForm.period = null
  createForm.contextReportCount = 3
  createForm.sourceChannelIds = sourceChannelId.value ? [sourceChannelId.value] : []
}

const refresh = async () => {
  if (!props.channelId) return
  try {
    const display = await store.getDisplayChannel(props.channelId)
    const targetChannelId = display?.sourceChannelId || props.channelId
    await store.list(targetChannelId)
    if (display?.displayChannelId) {
      store.displayByChannel[display.displayChannelId] = display
    }
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '加载战报失败')
  }
}

const refreshDisplayChannelContent = async () => {
  const targetId = displayChannelId.value
  if (!targetId) return
  try {
    await store.resyncDisplayChannel(targetId)
    chatEvent.emit('battle-report-display-refresh' as any, { channelId: targetId })
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '刷新展示频道失败')
  }
}

const stopPolling = () => {
  if (pollTimer === null) return
  window.clearInterval(pollTimer)
  pollTimer = null
}

const syncPolling = () => {
  stopPolling()
  if (!props.visible || !props.channelId || !hasGenerating.value) return
  pollTimer = window.setInterval(() => {
    void refresh()
  }, 2500)
}

watch(
  () => [props.visible, props.channelId] as const,
  ([visible, channelId]) => {
    if (visible && channelId) {
      void refresh()
    } else {
      stopPolling()
    }
  },
  { immediate: true },
)

watch(hasGenerating, syncPolling)

const openCreate = () => {
  resetCreateForm()
  createVisible.value = true
}

const openDisplaySetup = () => {
  displayForm.name = displayChannel.value?.displayName || '战报时间线'
  displayVisible.value = true
}

const ensureDisplayChannel = async () => {
  const targetSourceChannelId = sourceChannelId.value
  if (!targetSourceChannelId) {
    message.error('未选择频道')
    return
  }
  const name = displayForm.name.trim() || '战报时间线'
  try {
    const item = await store.ensureDisplayChannel(targetSourceChannelId, name)
    if (!item?.displayChannelId) {
      message.error('展示频道创建失败')
      return
    }
    displayVisible.value = false
    if (props.worldId || chat.currentWorldId) {
      await chat.channelList(props.worldId || chat.currentWorldId, true, { autoSwitch: false })
    }
    await chat.channelSwitchTo(item.displayChannelId)
    message.success('已打开战报展示频道')
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '创建展示频道失败')
  }
}

const openDisplayChannel = async () => {
  const targetId = displayChannelId.value
  if (!targetId) {
    openDisplaySetup()
    return
  }
  try {
    await chat.channelSwitchTo(targetId)
  } catch (error: any) {
    message.error(error?.message || '打开展示频道失败')
  }
}

const disableDisplayChannel = async () => {
  const targetId = displayChannelId.value || props.channelId || ''
  if (!targetId) return
  dialog.warning({
    title: '关闭战报展示频道',
    content: '旧展示频道会归档，战报本身不会删除。',
    positiveText: '关闭展示频道',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await store.disableDisplayChannel(targetId)
        if (props.worldId || chat.currentWorldId) {
          await chat.channelList(props.worldId || chat.currentWorldId, true, { autoSwitch: false })
        }
        if (sourceChannelId.value && chat.curChannel?.id === targetId) {
          await chat.channelSwitchTo(sourceChannelId.value)
        }
        message.success('战报展示频道已关闭')
        await refresh()
      } catch (error: any) {
        message.error(error?.response?.data?.message || error?.message || '关闭展示频道失败')
      }
    }
  })
}

const openEditor = async (item: BattleReport) => {
  editingReportId.value = item.id
  try {
    await store.get(item.id)
  } catch (error) {
    console.warn('加载战报详情失败', error)
  }
  editorVisible.value = true
}

const openEditorById = async (reportId: string) => {
  const normalized = String(reportId || '').trim()
  if (!normalized) return
  emit('update:visible', true)
  editingReportId.value = normalized
  try {
    await store.get(normalized)
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '加载战报详情失败')
    return
  }
  editorVisible.value = true
}

const handleBattleReportOpenEditor = (payload: any) => {
  const reportId = String(payload?.reportId || '').trim()
  const channelId = String(payload?.channelId || '').trim()
  const allowedChannelIds = new Set([
    String(props.channelId || '').trim(),
    String(sourceChannelId.value || '').trim(),
    String(displayChannelId.value || '').trim(),
  ].filter(Boolean))
  if (!payload?.deferToDrawer && channelId && allowedChannelIds.size && !allowedChannelIds.has(channelId)) {
    return
  }
  void openEditorById(reportId)
}

const handleDisplayMessageReordered = (payload: any) => {
  const channelId = String(payload?.channelId || '').trim()
  if (!channelId || channelId !== displayChannelId.value) return
  void refresh()
}

onMounted(() => {
  chatEvent.on('battle-report-open-editor' as any, handleBattleReportOpenEditor)
  chatEvent.on('battle-report-display-message-reordered' as any, handleDisplayMessageReordered)
})

onBeforeUnmount(() => {
  stopPolling()
  chatEvent.off('battle-report-open-editor' as any, handleBattleReportOpenEditor)
  chatEvent.off('battle-report-display-message-reordered' as any, handleDisplayMessageReordered)
})

const copyReportLink = async (item: BattleReport) => {
  const worldId = props.worldId || item.worldId
  if (!worldId || !item.channelId || !item.id) {
    message.error('缺少战报链接参数')
    return
  }
  const link = generateBattleReportEmbedLink({ worldId, channelId: item.channelId, reportId: item.id })
  await copyTextWithFallback(link)
  message.success('战报嵌入链接已复制')
}

const createLocalAISummaryReport = async (primaryChannelId: string, payload: {
  title: string
  content: string
  periodStart: number
  periodEnd: number
  contextReportCount: number
  source: string
  sourceChannelIds: string[]
}) => {
  localSummaryRunning.value = true
  try {
    localSummaryStatus.value = '正在整理战报上下文'
    const input = await store.buildSummaryInput(primaryChannelId, payload)
    if (!input.trim()) {
      throw new Error('战报总结输入为空')
    }
    localSummaryStatus.value = '正在调用本地 AI 总结'
    const resp = await aiStore.runTask('battle_summary', {
      worldId: props.worldId,
      channelId: primaryChannelId,
      input,
      source: 'user',
    })
    const result = String(resp?.data?.result || '').trim()
    if (!result) {
      throw new Error('AI 返回空战报')
    }
    localSummaryStatus.value = '正在提交战报内容'
    return store.create(primaryChannelId, {
      ...payload,
      content: result,
      source: 'user',
      aiProviderId: String(resp?.data?.providerId || '').trim(),
      aiModel: String(resp?.data?.model || '').trim(),
      aiFeatureKey: 'battle_summary',
    })
  } finally {
    localSummaryRunning.value = false
    localSummaryStatus.value = ''
  }
}

const createReport = async () => {
  const sourceChannelIds = normalizeCreateSourceChannelIds()
  const primaryChannelId = sourceChannelIds[0] || sourceChannelId.value
  if (!primaryChannelId) {
    message.error('未选择频道')
    return
  }
  if (!createForm.period) {
    message.error('请选择战报时间周期')
    return
  }
  const payload = {
    title: createForm.title.trim() || '新战报',
    content: createForm.content.trim(),
    periodStart: createForm.period[0],
    periodEnd: createForm.period[1],
    contextReportCount: createForm.contextReportCount,
    source: aiStore.currentSource,
    sourceChannelIds,
  }
  try {
    if (createMode.value === 'ai') {
      if (aiStore.currentSource === 'user') {
        await createLocalAISummaryReport(primaryChannelId, payload)
        message.success('本地 AI 战报已创建')
      } else {
        await store.summarize(primaryChannelId, payload)
        message.success('AI 总结已开始')
      }
    } else {
      await store.create(primaryChannelId, payload)
      message.success('战报已创建')
    }
    createVisible.value = false
    await refresh()
    await refreshDisplayChannelContent()
  } catch (error: any) {
    const detail = error?.response?.data?.error || error?.response?.data?.message || error?.message || '创建战报失败'
    if (String(detail).includes('战报总结输入过长')) {
      dialog.warning({
        title: '战报内容过长',
        content: detail,
        positiveText: '重新选择',
      })
      return
    }
    message.error(detail)
  }
}

const saveEditor = async (payload: { title: string; content: string }) => {
  const item = editingReport.value
  if (!item) return
  try {
    await store.update(item.id, {
      ...payload,
      periodStart: item.periodStart,
      periodEnd: item.periodEnd,
      contextReportCount: item.contextReportCount,
    })
    editorVisible.value = false
    message.success('战报已保存')
    await refresh()
    await refreshDisplayChannelContent()
  } catch (error: any) {
    message.error(error?.response?.data?.message || error?.message || '保存战报失败')
  }
}

const deleteReport = async (item: BattleReport) => {
  dialog.warning({
    title: '删除战报',
    content: `删除战报“${item.title || '未命名战报'}”？此操作不会删除聊天记录。`,
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await store.delete(item.id)
        message.success('战报已删除')
        await refresh()
        await refreshDisplayChannelContent()
      } catch (error: any) {
        message.error(error?.response?.data?.message || error?.message || '删除战报失败')
      }
    }
  })
}

const handleDragStart = (item: BattleReport, event: DragEvent) => {
  draggedId.value = item.id
  event.dataTransfer?.setData('text/plain', item.id)
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
  }
}

const handleDrop = async (target: BattleReport, event: DragEvent) => {
  event.preventDefault()
  const sourceId = draggedId.value || event.dataTransfer?.getData('text/plain') || ''
  draggedId.value = ''
  if (!sourceChannelId.value || !sourceId || sourceId === target.id) return
  const current = sourceReports.value.slice()
  const sourceIndex = current.findIndex((item) => item.id === sourceId)
  const targetIndex = current.findIndex((item) => item.id === target.id)
  if (sourceIndex < 0 || targetIndex < 0) return
  const next = current.slice()
  const [moved] = next.splice(sourceIndex, 1)
  next.splice(targetIndex, 0, moved)
  store.setChannelItems(sourceChannelId.value, next)
  try {
    await store.reorder(sourceChannelId.value, next.map((item) => item.id))
    await refresh()
    if (displayChannelId.value) {
      chatEvent.emit('battle-report-display-refresh' as any, { channelId: displayChannelId.value })
    }
  } catch (error: any) {
    store.setChannelItems(sourceChannelId.value, current)
    message.error(error?.response?.data?.message || error?.message || '战报排序失败')
  }
}
</script>

<template>
  <n-drawer
    :show="visible"
    :width="620"
    placement="right"
    @update:show="emit('update:visible', $event)"
  >
    <n-drawer-content title="战报总结" closable>
      <div class="battle-report-toolbar">
        <div>
          <div class="battle-report-toolbar__title">世界战报</div>
          <div class="battle-report-toolbar__hint">手动或 AI 总结当前世界的战报。</div>
        </div>
        <div class="battle-report-toolbar__actions">
          <n-button v-if="displayChannel" size="small" secondary @click="openDisplayChannel">打开展示频道</n-button>
          <n-button v-if="displayChannel" size="small" tertiary @click="disableDisplayChannel">关闭展示频道</n-button>
          <n-button v-else size="small" secondary @click="openDisplaySetup">开启展示频道</n-button>
          <n-button size="small" type="primary" @click="openCreate">新建战报</n-button>
        </div>
      </div>
      <n-spin :show="store.loading">
        <div v-if="!sourceReports.length" class="battle-report-empty">
          暂无战报。点击上方新建，手写或交给 AI 总结。
        </div>
        <div v-else class="battle-report-timeline">
          <div
            v-for="item in sourceReports"
            :key="item.id"
            class="battle-report-item"
            :class="`battle-report-item--${item.status}`"
            draggable="true"
            @dragstart="handleDragStart(item, $event)"
            @dragover.prevent
            @drop="handleDrop(item, $event)"
          >
            <n-tooltip trigger="hover">
              <template #trigger>
                <div class="battle-report-node">
                  <span v-if="item.status === 'generating'" class="battle-report-node__spinner"></span>
                </div>
              </template>
              {{ formatPeriod(item) }}
            </n-tooltip>
            <div class="battle-report-card" @dblclick="openEditor(item)">
              <div class="battle-report-card__main">
                <n-popover trigger="hover" placement="left" :width="280">
                  <template #trigger>
                    <button class="battle-report-title" type="button" @click="openEditor(item)">
                      {{ item.title || '未命名战报' }}
                    </button>
                  </template>
                  <div class="battle-report-preview">{{ previewText(item) }}</div>
                </n-popover>
                <span class="battle-report-meta">{{ formatPeriod(item) }}</span>
                <span v-if="item.status === 'failed'" class="battle-report-error">
                  {{ item.errorMessage || '生成失败' }}
                </span>
              </div>
              <div class="battle-report-actions" @click.stop @dblclick.stop>
                <n-button quaternary circle size="tiny" title="编辑战报" @click="openEditor(item)">✎</n-button>
                <n-button quaternary circle size="tiny" title="复制嵌入链接" @click="copyReportLink(item)">⧉</n-button>
                <n-button quaternary circle size="tiny" title="删除" @click="deleteReport(item)">×</n-button>
              </div>
            </div>
          </div>
        </div>
      </n-spin>
    </n-drawer-content>
  </n-drawer>

  <n-modal
    v-model:show="createVisible"
    preset="card"
    title="新建战报"
    class="battle-report-create-modal"
    :auto-focus="false"
    :mask-closable="!createSubmitting"
    :close-on-esc="!createSubmitting"
  >
    <n-spin :show="createSubmitting">
      <template #description>
        {{ localSummaryStatus || '正在处理战报' }}
      </template>
      <n-form label-placement="top">
        <n-form-item label="生成方式">
          <n-radio-group v-model:value="createMode" :disabled="createSubmitting">
            <n-radio-button value="ai">AI 总结</n-radio-button>
            <n-radio-button value="manual">手动创建</n-radio-button>
          </n-radio-group>
        </n-form-item>
        <n-form-item label="来源频道">
          <n-select
            v-model:value="createForm.sourceChannelIds"
            :options="createChannelOptions"
            :disabled="createSubmitting"
            multiple
            filterable
            clearable
            placeholder="选择要纳入战报的频道"
          />
          <template #feedback>默认当前频道。多选时会按频道分别拼接同一时间段内的内容。</template>
        </n-form-item>
        <n-form-item label="时间周期">
          <ActiveDayDateRangePicker
            v-model="createForm.period"
            :channel-id="createPrimaryChannelId"
            placeholder="选择需要总结的活跃消息周期"
          />
        </n-form-item>
        <n-form-item label="前情提要">
          <n-input-number v-model:value="createForm.contextReportCount" :min="0" :max="20" :disabled="createSubmitting" />
          <template #feedback>AI 总结时引用多少篇之前的已完成战报。</template>
        </n-form-item>
        <n-form-item label="标题">
          <n-input v-model:value="createForm.title" maxlength="120" show-count placeholder="留空则使用默认标题" :disabled="createSubmitting" />
        </n-form-item>
        <n-form-item v-if="createMode === 'manual'" label="内容">
          <n-input
            v-model:value="createForm.content"
            type="textarea"
            :autosize="{ minRows: 8, maxRows: 18 }"
            placeholder="纯文本战报内容"
            :disabled="createSubmitting"
          />
        </n-form-item>
      </n-form>
    </n-spin>
    <template #footer>
      <n-space justify="end">
        <n-button :disabled="createSubmitting" @click="createVisible = false">取消</n-button>
        <n-button type="primary" :loading="createSubmitting" :disabled="createSubmitting" @click="createReport">
          {{ createMode === 'ai' ? '开始总结' : '创建战报' }}
        </n-button>
      </n-space>
    </template>
  </n-modal>

  <n-modal
    v-model:show="displayVisible"
    preset="dialog"
    title="战报展示频道"
    positive-text="开启并打开"
    negative-text="取消"
    :positive-button-props="{ loading: store.saving }"
    @positive-click="ensureDisplayChannel"
  >
    <n-form label-placement="top">
      <n-form-item label="频道名称">
        <n-input v-model:value="displayForm.name" maxlength="80" show-count placeholder="战报时间线" />
      </n-form-item>
    </n-form>
  </n-modal>

  <BattleReportEditorModal
    v-model:visible="editorVisible"
    :report="editingReport"
    @save="saveEditor"
  />
</template>

<style scoped>
.battle-report-empty {
  padding: 28px 12px;
  color: var(--text-color-3);
  text-align: center;
}

.battle-report-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
  padding: 12px;
  border: 1px solid rgba(148, 163, 184, 0.22);
  border-radius: 14px;
  background: rgba(148, 163, 184, 0.08);
}

.battle-report-toolbar__title {
  font-weight: 800;
  color: var(--text-color-1);
}

.battle-report-toolbar__hint {
  margin-top: 2px;
  font-size: 12px;
  color: var(--text-color-3);
}

.battle-report-toolbar__actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}

.battle-report-timeline {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px 0 12px;
}

.battle-report-item {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr);
  gap: 10px;
  cursor: grab;
}

.battle-report-item:active {
  cursor: grabbing;
}

.battle-report-node {
  position: relative;
  width: 28px;
  min-height: 58px;
}

.battle-report-node::before {
  content: "";
  position: absolute;
  top: 0;
  bottom: -8px;
  left: 13px;
  width: 2px;
  background: rgba(100, 116, 139, 0.28);
}

.battle-report-node::after {
  content: "";
  position: absolute;
  top: 18px;
  left: 8px;
  width: 12px;
  height: 12px;
  border-radius: 999px;
  background: #2563eb;
  box-shadow: 0 0 0 4px rgba(37, 99, 235, 0.14);
}

.battle-report-item--failed .battle-report-node::after {
  background: #dc2626;
  box-shadow: 0 0 0 4px rgba(220, 38, 38, 0.14);
}

.battle-report-node__spinner {
  position: absolute;
  z-index: 1;
  top: 15px;
  left: 5px;
  width: 18px;
  height: 18px;
  border: 2px solid rgba(37, 99, 235, 0.2);
  border-top-color: #2563eb;
  border-radius: 999px;
  animation: battle-report-spin 0.9s linear infinite;
}

.battle-report-card {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  min-width: 0;
  padding: 12px 12px;
  border: 1px solid rgba(148, 163, 184, 0.25);
  border-radius: 14px;
  background: rgba(148, 163, 184, 0.08);
}

.battle-report-card__main {
  min-width: 0;
}

.battle-report-title {
  display: block;
  max-width: 100%;
  padding: 0;
  border: 0;
  color: var(--text-color-1);
  background: transparent;
  font-weight: 700;
  text-align: left;
  cursor: pointer;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.battle-report-title:hover {
  color: var(--primary-color);
}

.battle-report-meta,
.battle-report-error {
  display: block;
  margin-top: 4px;
  font-size: 12px;
  color: var(--text-color-3);
}

.battle-report-error {
  color: #dc2626;
}

.battle-report-actions {
  display: flex;
  gap: 4px;
  flex-shrink: 0;
}

.battle-report-preview {
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.55;
}

.battle-report-create-modal {
  width: min(780px, calc(100vw - 32px));
}

@keyframes battle-report-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 720px) {
  :deep(.n-drawer) {
    width: 100vw !important;
  }

  .battle-report-toolbar {
    align-items: stretch;
    flex-direction: column;
  }

  .battle-report-toolbar__actions {
    justify-content: flex-start;
  }

  .battle-report-create-modal {
    width: calc(100vw - 16px);
  }
}
</style>
