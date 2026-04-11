<script setup lang="ts">
import { computed } from 'vue'
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

const onlineMembers = computed(() => {
  const now = Date.now()
  return props.members.filter(member => {
    const presence = props.presenceMap[member.id]
    return presence && (now - presence.lastPing) < 120000 // 2分钟内算在线
  })
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
      <div class="presence-heading">
        <span class="presence-title">在线成员</span>
        <span class="presence-count">{{ onlineMembers.length }}</span>
      </div>
      <n-button size="tiny" secondary class="presence-refresh" @click="handleRefresh">
        刷新状态
      </n-button>
    </div>

    <div class="presence-list">
      <div
        v-for="member in onlineMembers"
        :key="member.id"
        class="presence-item"
      >
        <Avatar
          :src="member.avatar"
          :size="32"
          :border="false"
        />
        <div class="presence-info">
          <div class="presence-name">
            <span
              :style="getMemberColor(member) ? { color: getMemberColor(member) } : undefined"
            >
              {{ getMemberDisplayName(member) }}
            </span>
          </div>
          <div class="presence-meta">
            <span class="latency">
              {{ getLatency(member.id) }}ms
            </span>
            <n-icon
              :component="isFocused(member.id) ? EyeOutline : EyeOffOutline"
              size="14"
              :class="{ 'focused': isFocused(member.id), 'unfocused': !isFocused(member.id) }"
            />
          </div>
        </div>
      </div>

      <div v-if="onlineMembers.length === 0" class="presence-empty">
        当前暂无在线成员
      </div>
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
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}

.presence-heading {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  min-width: 0;
}

.presence-title {
  font-size: 0.9rem;
  font-weight: 600;
  color: var(--sc-text-primary, #1f2937);
}

.presence-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 1.5rem;
  height: 1.5rem;
  padding: 0 0.4rem;
  border-radius: 999px;
  background: rgba(59, 130, 246, 0.14);
  color: #2563eb;
  font-size: 0.75rem;
  font-weight: 600;
}

.presence-refresh {
  flex-shrink: 0;
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

:global([data-display-palette='night']) .presence-popover .presence-title {
  color: #fff;
}

:global([data-display-palette='night']) .presence-popover .presence-count {
  background: rgba(96, 165, 250, 0.2);
  color: #93c5fd;
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
  padding: 1.5rem 1rem;
}
</style>
