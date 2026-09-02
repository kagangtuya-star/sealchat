<script setup lang="ts">
import { computed } from 'vue'
import { NIcon } from 'naive-ui'
import {
  NotificationsOutline,
  VolumeHighOutline,
  GlobeOutline,
} from '@vicons/ionicons5'
import { usePushNotificationStore } from '@/stores/pushNotification'
import type { DisplaySettings } from '@/stores/display'
import {
  MESSAGE_SOUND_MODE_LABELS,
  MESSAGE_SOUND_MODE_VALUES,
  type MessageSoundMode,
} from '@/utils/messageSoundMode'

interface Props {
  settings: Pick<DisplaySettings, 'worldMessageToastEnabled' | 'messageSoundMode'>
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (event: 'update', patch: Partial<DisplaySettings>): void
}>()

const pushStore = usePushNotificationStore()

const soundOptions = MESSAGE_SOUND_MODE_VALUES.map((value) => ({
  label: MESSAGE_SOUND_MODE_LABELS[value],
  value,
}))

const permissionLabel = computed(() => {
  if (!pushStore.supported) return '不可用'
  if (pushStore.permission === 'granted') return '已允许'
  if (pushStore.permission === 'denied') return '已拒绝'
  return '未授权'
})

const permissionTone = computed(() => {
  if (!pushStore.supported || pushStore.permission === 'denied') return 'warning'
  if (pushStore.permission === 'granted') return 'success'
  return 'default'
})

const pushToggleDisabled = computed(() => (
  !pushStore.supported
  || pushStore.permission === 'denied'
  || !pushStore.embedNotifyOwnerEnabled
))

const handleWorldToastUpdate = (value: boolean) => {
  emit('update', { worldMessageToastEnabled: value })
}

const handleSoundUpdate = (value: MessageSoundMode) => {
  emit('update', { messageSoundMode: value })
}

const handlePushToggle = () => {
  void pushStore.toggle()
}

const requestBrowserPermission = async () => {
  if (!pushStore.supported || pushStore.permission !== 'default' || !pushStore.embedNotifyOwnerEnabled) {
    return
  }
  await pushStore.requestPermission()
}
</script>

<template>
  <div class="message-notification-settings-panel">
    <section class="message-notification-settings-panel__section">
      <div class="message-notification-settings-panel__header">
        <div>
          <p class="section-title">
            <NIcon :component="GlobeOutline" size="17" />
            <span>世界内消息提醒</span>
          </p>
          <p class="section-desc">在当前世界的其他频道收到消息时，在聊天区域右上角显示消息预览。</p>
        </div>
        <n-switch
          :value="props.settings.worldMessageToastEnabled"
          @update:value="handleWorldToastUpdate"
        >
          <template #checked>开启</template>
          <template #unchecked>关闭</template>
        </n-switch>
      </div>
      <div class="message-notification-settings-panel__preview">
        <span class="preview-dot"></span>
        <span>其他频道新消息提醒</span>
      </div>
    </section>

    <section class="message-notification-settings-panel__section">
      <div class="message-notification-settings-panel__header">
        <div>
          <p class="section-title">
            <NIcon :component="VolumeHighOutline" size="17" />
            <span>消息提示音</span>
          </p>
          <p class="section-desc">控制新消息提示音触发范围，继续沿用现有声音规则。</p>
        </div>
      </div>
      <n-select
        :value="props.settings.messageSoundMode"
        :options="soundOptions"
        size="small"
        @update:value="handleSoundUpdate"
      />
    </section>

    <section class="message-notification-settings-panel__section">
      <div class="message-notification-settings-panel__header">
        <div>
          <p class="section-title">
            <NIcon :component="NotificationsOutline" size="17" />
            <span>浏览器消息推送</span>
          </p>
          <p class="section-desc">切换标签页或最小化时，使用浏览器原生通知提示新消息。</p>
        </div>
        <n-switch
          :value="pushStore.enabled"
          :disabled="pushToggleDisabled"
          @update:value="handlePushToggle"
        >
          <template #checked>已启用</template>
          <template #unchecked>已关闭</template>
        </n-switch>
      </div>

      <div class="message-notification-settings-panel__status" aria-live="polite">
        <div class="status-row">
          <span>浏览器支持</span>
          <strong :class="{ 'status-warning': !pushStore.supported }">
            {{ pushStore.supported ? '支持' : '不支持' }}
          </strong>
        </div>
        <div class="status-row">
          <span>当前权限</span>
          <strong :class="`status-${permissionTone}`">{{ permissionLabel }}</strong>
        </div>
        <div class="status-row">
          <span>当前状态</span>
          <strong>{{ pushStore.enabled ? '已启用' : '已关闭' }}</strong>
        </div>
      </div>

      <n-button
        v-if="pushStore.supported && pushStore.permission === 'default' && pushStore.embedNotifyOwnerEnabled"
        size="small"
        secondary
        @click="requestBrowserPermission"
      >
        授权浏览器通知
      </n-button>
      <n-alert
        v-if="pushStore.permission === 'denied'"
        type="warning"
        size="small"
        :bordered="false"
        class="message-notification-settings-panel__alert"
      >
        通知权限已拒绝。请从浏览器网站权限中恢复后再开启。
      </n-alert>
      <n-alert
        v-else-if="!pushStore.embedNotifyOwnerEnabled"
        type="info"
        size="small"
        :bordered="false"
        class="message-notification-settings-panel__alert"
      >
        当前嵌入容器未允许浏览器通知。
      </n-alert>
    </section>
  </div>
</template>

<style scoped lang="scss">
.message-notification-settings-panel {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.8rem;
}

.message-notification-settings-panel__section {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding: 0.85rem 0.9rem;
  border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.25));
  border-radius: 10px;
  background: color-mix(in srgb, var(--sc-bg-surface, transparent) 56%, transparent);
}

.message-notification-settings-panel__section:last-child {
  grid-column: 1 / -1;
}

.message-notification-settings-panel__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
}

.section-title {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin: 0;
  color: var(--sc-text-primary);
  font-weight: 650;
}

.section-desc {
  margin: 0.28rem 0 0;
  color: var(--sc-text-secondary);
  font-size: 0.82rem;
  line-height: 1.45;
}

.message-notification-settings-panel__preview {
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
  color: var(--sc-text-secondary);
  font-size: 0.78rem;
}

.preview-dot {
  width: 0.42rem;
  height: 0.42rem;
  border-radius: 50%;
  background: var(--primary-color, #3b82f6);
}

.message-notification-settings-panel__status {
  display: grid;
  gap: 0.35rem;
  padding: 0.65rem 0.7rem;
  border-radius: 8px;
  background: color-mix(in srgb, var(--sc-bg-input, var(--sc-bg-surface, transparent)) 62%, transparent);
}

.status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.8rem;
  color: var(--sc-text-secondary);
  font-size: 0.8rem;
}

.status-row strong {
  color: var(--sc-text-primary);
  font-weight: 600;
}

.status-success {
  color: #16a34a !important;
}

.status-warning {
  color: #dc2626 !important;
}

.message-notification-settings-panel__alert {
  margin-top: -0.15rem;
}

@media (max-width: 520px) {
  .message-notification-settings-panel {
    grid-template-columns: minmax(0, 1fr);
  }

  .message-notification-settings-panel__section:last-child {
    grid-column: auto;
  }

  .message-notification-settings-panel__header {
    align-items: stretch;
    flex-direction: column;
    gap: 0.65rem;
  }

  .message-notification-settings-panel__header :deep(.n-switch) {
    align-self: flex-start;
  }
}
</style>
