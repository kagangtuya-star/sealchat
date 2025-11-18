<script setup lang="ts">
import { ref, computed } from 'vue'
import { EyeOutline, EyeOffOutline } from '@vicons/ionicons5'
import Avatar from '@/components/avatar.vue'

interface PresenceData {
  lastPing: number
  latencyMs: number
  isFocused: boolean
}

interface Member {
  id: string
  nick?: string
  name?: string
  avatar?: string
  identity?: {
    displayName?: string
    color?: string
  }
}

interface Props {
  members: Member[]
  presenceMap: Record<string, PresenceData>
}

interface Emits {
  (e: 'request-refresh'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const activeTab = ref<'online' | 'offline'>('online')

const onlineMembers = computed(() => {
  const now = Date.now()
  return props.members.filter(member => {
    const presence = props.presenceMap[member.id]
    return presence && (now - presence.lastPing) < 120000 // 2分钟内算在线
  })
})

const offlineMembers = computed(() => {
  const now = Date.now()
  return props.members.filter(member => {
    const presence = props.presenceMap[member.id]
    return !presence || (now - presence.lastPing) >= 120000
  })
})

const currentMembers = computed(() => {
  return activeTab.value === 'online' ? onlineMembers.value : offlineMembers.value
})

const getMemberDisplayName = (member: Member) => {
  return member.identity?.displayName || member.nick || member.name || '未知成员'
}

const getMemberColor = (member: Member) => {
  return member.identity?.color || ''
}

const getLatency = (memberId: string) => {
  const presence = props.presenceMap[memberId]
  return presence?.latencyMs || 0
}

const isFocused = (memberId: string) => {
  const presence = props.presenceMap[memberId]
  return presence?.isFocused || false
}

const handleRefresh = () => {
  emit('request-refresh')
}
</script>

<template>
  <div class="presence-popover">
    <div class="presence-header">
      <n-radio-group
        v-model:value="activeTab"
        size="small"
      >
        <n-radio-button value="online">
          在线 ({{ onlineMembers.length }})
        </n-radio-button>
        <n-radio-button value="offline">
          离线 ({{ offlineMembers.length }})
        </n-radio-button>
      </n-radio-group>
    </div>

    <div class="presence-list">
      <div
        v-for="member in currentMembers"
        :key="member.id"
        class="presence-item"
      >
        <Avatar :src="member.avatar" :size="32" :border="false" />
        <div class="presence-info">
          <div class="presence-name">
            <span
              :style="getMemberColor(member) ? { color: getMemberColor(member) } : undefined"
            >
              {{ getMemberDisplayName(member) }}
            </span>
          </div>
          <div class="presence-meta">
            <span v-if="activeTab === 'online'" class="latency">
              {{ getLatency(member.id) }}ms
            </span>
            <n-icon
              v-if="activeTab === 'online'"
              :component="isFocused(member.id) ? EyeOutline : EyeOffOutline"
              size="14"
              :class="{ 'focused': isFocused(member.id), 'unfocused': !isFocused(member.id) }"
            />
          </div>
        </div>
      </div>

      <div v-if="currentMembers.length === 0" class="presence-empty">
        {{ activeTab === 'online' ? '暂无在线成员' : '暂无离线成员' }}
      </div>
    </div>

    <div class="presence-footer">
      <n-button size="small" @click="handleRefresh">
        刷新状态
      </n-button>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.presence-popover {
  width: 280px;
  max-height: 400px;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding: 0.75rem;
}

.presence-header {
  display: flex;
  justify-content: center;
}

:deep(.n-radio-group) {
  display: inline-flex;
  background: rgba(15, 23, 42, 0.04);
  border-radius: 0.75rem;
  padding: 0.125rem;
}

:deep(.n-radio-button) {
  min-width: 6.5rem;
  justify-content: center;
  border-radius: 0.5rem;
  font-size: 0.75rem;
}

.presence-list {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  max-height: 280px;
}

.presence-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.5rem;
  border-radius: 0.5rem;
  transition: background-color 0.2s ease;
}

.presence-item:hover {
  background-color: rgba(0, 0, 0, 0.04);
}

.presence-info {
  flex: 1;
  min-width: 0;
}

.presence-name {
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--sc-text-primary, #1f2937);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

:global([data-display-palette='night']) .presence-popover .presence-name {
  color: #fff;
}

.presence-meta {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 0.25rem;
}

.latency {
  font-size: 0.75rem;
  color: #6b7280;
  background: rgba(107, 114, 128, 0.1);
  padding: 0.125rem 0.375rem;
  border-radius: 0.25rem;
}

.focused {
  color: #059669;
}

.unfocused {
  color: #9ca3af;
}

.presence-empty {
  text-align: center;
  color: #9ca3af;
  font-size: 0.875rem;
  padding: 1rem;
}

.presence-footer {
  display: flex;
  justify-content: center;
  border-top: 1px solid rgba(0, 0, 0, 0.06);
  padding-top: 0.75rem;
}
</style>
