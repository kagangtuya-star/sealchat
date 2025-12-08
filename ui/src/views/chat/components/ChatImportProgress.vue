<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, watch } from 'vue'
import { api } from '@/stores/_config'
import { chatEvent } from '@/stores/chat'

interface Props {
  visible: boolean
  channelId: string
  jobId: string
}

interface Emits {
  (e: 'update:visible', visible: boolean): void
  (e: 'complete'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

interface JobProgress {
  jobId: string
  status: string
  totalLines: number
  processedLines: number
  importedCount: number
  skippedCount: number
  errorMessage?: string
  percentage: number
}

const progress = ref<JobProgress | null>(null)
const polling = ref<number | null>(null)
const wsListenerRegistered = ref(false)

const statusText = computed(() => {
  if (!progress.value) return '加载中...'
  switch (progress.value.status) {
    case 'pending': return '等待中...'
    case 'running': return '导入中...'
    case 'done': return '导入完成'
    case 'failed': return '导入失败'
    default: return progress.value.status
  }
})

const statusType = computed(() => {
  if (!progress.value) return 'default'
  switch (progress.value.status) {
    case 'done': return 'success'
    case 'failed': return 'error'
    default: return 'info'
  }
})

const isComplete = computed(() =>
  progress.value?.status === 'done' || progress.value?.status === 'failed'
)

// WebSocket 事件处理
const handleWsProgress = (data: any) => {
  if (data?.type === 'chat-import-progress' &&
      data?.progress?.jobId === props.jobId) {
    progress.value = data.progress
    if (isComplete.value) {
      stopPolling()
    }
  }
}

// 注册 WebSocket 监听
const registerWsListener = () => {
  if (wsListenerRegistered.value) return
  chatEvent.on('chat-import-progress', handleWsProgress)
  wsListenerRegistered.value = true
}

const unregisterWsListener = () => {
  if (!wsListenerRegistered.value) return
  chatEvent.off('chat-import-progress', handleWsProgress)
  wsListenerRegistered.value = false
}

const fetchProgress = async () => {
  try {
    const res = await api.get<JobProgress>(
      `/api/v1/channels/${props.channelId}/import/jobs/${props.jobId}`
    )
    progress.value = res.data

    if (isComplete.value) {
      stopPolling()
    }
  } catch (e) {
    console.error('获取进度失败:', e)
  }
}

const startPolling = () => {
  fetchProgress()
  // 使用较长的轮询间隔，因为有 WebSocket 实时更新
  polling.value = window.setInterval(fetchProgress, 3000)
}

const stopPolling = () => {
  if (polling.value) {
    clearInterval(polling.value)
    polling.value = null
  }
}

const handleClose = () => {
  emit('update:visible', false)
  if (isComplete.value) {
    emit('complete')
  }
}

watch(() => props.visible, (visible) => {
  if (visible && props.jobId) {
    registerWsListener()
    startPolling()
  } else {
    stopPolling()
    unregisterWsListener()
  }
})

onMounted(() => {
  if (props.visible && props.jobId) {
    registerWsListener()
    startPolling()
  }
})

onUnmounted(() => {
  stopPolling()
  unregisterWsListener()
})
</script>

<template>
  <n-modal
    :show="visible"
    @update:show="emit('update:visible', $event)"
    preset="card"
    title="导入进度"
    class="import-progress-dialog"
    :auto-focus="false"
    style="width: 400px; max-width: 90vw;"
  >
    <div class="progress-content">
      <n-result
        v-if="isComplete"
        :status="statusType as 'success' | 'error'"
        :title="statusText"
        :description="progress?.errorMessage || ''"
      >
        <template #footer>
          <div class="result-stats" v-if="progress">
            <n-statistic label="已导入" :value="progress.importedCount" />
            <n-statistic label="已跳过" :value="progress.skippedCount" />
          </div>
        </template>
      </n-result>

      <div v-else class="progress-running">
        <n-progress
          type="line"
          :percentage="progress?.percentage || 0"
          :indicator-placement="'inside'"
          processing
        />
        <div class="progress-stats">
          <span>{{ statusText }}</span>
          <span v-if="progress">
            {{ progress.processedLines }} / {{ progress.totalLines }} 行
          </span>
        </div>
        <div class="progress-details" v-if="progress">
          <n-space>
            <n-tag type="success" size="small">
              已导入: {{ progress.importedCount }}
            </n-tag>
            <n-tag type="warning" size="small">
              已跳过: {{ progress.skippedCount }}
            </n-tag>
          </n-space>
        </div>
      </div>
    </div>

    <template #footer>
      <n-space justify="end">
        <n-button @click="handleClose">
          {{ isComplete ? '关闭' : '后台运行' }}
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<style lang="scss" scoped>
.progress-content {
  min-height: 150px;
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.progress-running {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.progress-stats {
  display: flex;
  justify-content: space-between;
  font-size: 0.9rem;
  color: var(--sc-text-secondary);
}

.progress-details {
  display: flex;
  justify-content: center;
}

.result-stats {
  display: flex;
  gap: 2rem;
  justify-content: center;
}
</style>
