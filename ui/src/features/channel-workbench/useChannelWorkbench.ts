import { onBeforeUnmount, ref, shallowRef, watch, type Ref } from 'vue'
import { chatEvent } from '@/stores/chat'
import { applyWorkbenchPresentation, fetchWorkbench, saveWorkbenchLayout, workbenchErrorMessage } from './api'
import type { WorkbenchLayout, WorkbenchPresentationInput, WorkbenchUpdatedEventPayload, WorkbenchView } from './types'

// 布局频道的服务端状态：布局定义（layoutRevision）与 GM 展示状态（presentationRevision）。
// WS 事件只做失效通知，客户端重新拉取，隐藏面板因此不会通过广播泄露给玩家。

const REFRESH_DEBOUNCE = 120

export function useChannelWorkbench(channelId: Ref<string>, enabled: Ref<boolean>) {
  const view = shallowRef<WorkbenchView | null>(null)
  const loading = ref(false)
  const error = ref('')

  let epoch = 0
  let controller: AbortController | null = null
  let refreshTimer: number | null = null

  const cancelPending = () => {
    controller?.abort()
    controller = null
    if (refreshTimer !== null) {
      window.clearTimeout(refreshTimer)
      refreshTimer = null
    }
  }

  const reload = async () => {
    const targetChannelId = channelId.value
    cancelPending()
    const requestEpoch = ++epoch
    if (!enabled.value || !targetChannelId) {
      view.value = null
      loading.value = false
      error.value = ''
      return null
    }
    const request = new AbortController()
    controller = request
    loading.value = !view.value || view.value.channelId !== targetChannelId
    try {
      const next = await fetchWorkbench(targetChannelId, request.signal)
      if (requestEpoch !== epoch || channelId.value !== targetChannelId) return null
      view.value = next
      error.value = ''
      return next
    } catch (err) {
      if (requestEpoch !== epoch || request.signal.aborted) return null
      if (view.value?.channelId !== targetChannelId) view.value = null
      error.value = workbenchErrorMessage(err, '布局加载失败')
      return null
    } finally {
      if (requestEpoch === epoch) {
        loading.value = false
        controller = null
      }
    }
  }

  const scheduleReload = () => {
    if (refreshTimer !== null) window.clearTimeout(refreshTimer)
    refreshTimer = window.setTimeout(() => {
      refreshTimer = null
      void reload()
    }, REFRESH_DEBOUNCE)
  }

  const saveLayout = async (layout: WorkbenchLayout, baseRevision: number) => {
    const targetChannelId = channelId.value
    const saved = await saveWorkbenchLayout(targetChannelId, layout, baseRevision)
    if (channelId.value === targetChannelId) {
      // 保存结果直接成为新视图；随后到达的自身广播只会再拉取一次相同版本。
      epoch++
      cancelPending()
      view.value = saved
      error.value = ''
    }
    return saved
  }

  const applyPresentation = async (input: WorkbenchPresentationInput) => {
    const targetChannelId = channelId.value
    await applyWorkbenchPresentation(targetChannelId, input)
    if (channelId.value === targetChannelId) await reload()
  }

  const handleUpdated = (event?: { channelWorkbench?: WorkbenchUpdatedEventPayload; channel?: { id?: string } }) => {
    const target = event?.channelWorkbench?.channelId || event?.channel?.id || ''
    if (!enabled.value || !target || target !== channelId.value) return
    const current = view.value
    const payload = event?.channelWorkbench
    if (current && payload) {
      const layoutCurrent = !payload.layoutRevision || payload.layoutRevision <= current.layoutRevision
      const presentationCurrent = !payload.presentationRevision || payload.presentationRevision <= current.presentationRevision
      if (layoutCurrent && presentationCurrent) return
    }
    scheduleReload()
  }

  const handleConnected = () => {
    if (enabled.value && channelId.value) scheduleReload()
  }

  watch([channelId, enabled], () => {
    view.value = view.value?.channelId === channelId.value ? view.value : null
    void reload()
  }, { immediate: true })

  chatEvent.on('channel-workbench-updated' as any, handleUpdated as any)
  chatEvent.on('connected', handleConnected)

  onBeforeUnmount(() => {
    epoch++
    cancelPending()
    chatEvent.off('channel-workbench-updated' as any, handleUpdated as any)
    chatEvent.off('connected', handleConnected)
  })

  return { view, loading, error, reload, saveLayout, applyPresentation }
}
