<script setup lang="ts">
import { computed } from 'vue'
import {
  NAlert,
  NButton,
  NDescriptions,
  NDescriptionsItem,
  NSpace,
  NTag,
  NText,
} from 'naive-ui'
import { sealChatBridgeStatusState } from '@/bridge/sealchatBridgeStatus'

interface Props {
  channelId: string
  refreshing: boolean
  resultText: string
}

interface Emits {
  (e: 'refresh-avatars'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const bridgeStatus = sealChatBridgeStatusState

const formatTimestamp = (value: number) => {
  if (!Number.isFinite(value) || value <= 0) {
    return '未记录'
  }
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

const activeLabel = computed(() => bridgeStatus.active ? '已握手' : '未订阅')
const activeTagType = computed(() => bridgeStatus.active ? 'success' : 'default')
const connectStateText = computed(() => bridgeStatus.connectState || 'unknown')
const isCurrentChannelReady = computed(() => String(props.channelId || '').trim().length > 0)
const resultAlertType = computed(() => props.resultText.startsWith('刷新失败：') ? 'error' : 'success')
const isContextMatched = computed(() => {
  const currentChannelId = String(props.channelId || '').trim()
  if (!currentChannelId) {
    return false
  }
  return currentChannelId === bridgeStatus.channelId
})

const diagnostics = computed(() => [
  { key: 'active', label: '桥接状态', value: activeLabel.value },
  { key: 'connectState', label: '连接状态', value: connectStateText.value },
  { key: 'targetOrigin', label: '目标 Origin', value: bridgeStatus.targetOrigin || '未记录' },
  { key: 'worldId', label: '世界 ID', value: bridgeStatus.worldId || '未记录' },
  { key: 'channelId', label: '频道 ID', value: bridgeStatus.channelId || '未记录' },
  { key: 'lastHandshakeAt', label: '最近握手', value: formatTimestamp(bridgeStatus.lastHandshakeAt) },
  { key: 'lastRolesSnapshotAt', label: '最近角色快照', value: formatTimestamp(bridgeStatus.lastRolesSnapshotAt) },
  { key: 'lastMessageAt', label: '最近消息推送', value: formatTimestamp(bridgeStatus.lastMessageAt) },
  { key: 'lastInboundType', label: '最近入站类型', value: bridgeStatus.lastInboundType || '未记录' },
  { key: 'lastOutboundType', label: '最近出站类型', value: bridgeStatus.lastOutboundType || '未记录' },
  { key: 'lastError', label: '最近错误', value: bridgeStatus.lastError || '无' },
])
</script>

<template>
  <n-space vertical :size="16" class="bridge-status-panel">
    <n-alert type="info" :show-icon="false">
      这里显示 [doc/sealchat-bridge-api.md] 定义的桥接运行状态，并提供当前频道头像强制重签发入口。
    </n-alert>

    <div class="bridge-status-panel__header">
      <div class="bridge-status-panel__status-group">
        <span class="bridge-status-panel__title">当前状态</span>
        <n-tag :type="activeTagType" size="small">{{ activeLabel }}</n-tag>
        <n-tag size="small" :type="isContextMatched ? 'success' : 'warning'">
          {{ isContextMatched ? '频道匹配' : '频道未对齐' }}
        </n-tag>
      </div>
      <n-button
        class="bridge-status-panel__action"
        type="warning"
        :loading="refreshing"
        :disabled="!isCurrentChannelReady"
        @click="emit('refresh-avatars')"
      >
        刷新频道角色头像
      </n-button>
    </div>

    <n-text depth="3">
      作用范围：当前频道内你可管理用户的频道角色头像与头像差分。该操作会重建附件 ID 与存储文件名，但文件内容保持不变。
    </n-text>

    <n-alert v-if="resultText" :type="resultAlertType" :show-icon="false">
      {{ resultText }}
    </n-alert>

    <n-descriptions label-placement="left" bordered :column="1">
      <n-descriptions-item
        v-for="item in diagnostics"
        :key="item.key"
        :label="item.label"
      >
        {{ item.value }}
      </n-descriptions-item>
    </n-descriptions>
  </n-space>
</template>

<style scoped>
.bridge-status-panel {
  min-width: 0;
}

.bridge-status-panel__title {
  font-size: 14px;
  font-weight: 600;
}

.bridge-status-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.bridge-status-panel__status-group {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.bridge-status-panel__action {
  flex-shrink: 0;
}

@media (max-width: 640px) {
  .bridge-status-panel__header {
    align-items: stretch;
    flex-direction: column;
  }

  .bridge-status-panel__action {
    width: 100%;
  }
}
</style>
