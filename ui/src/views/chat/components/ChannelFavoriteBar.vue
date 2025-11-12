<script setup lang="ts">
import { computed } from 'vue'
import { useChatStore } from '@/stores/chat'
import { useDisplayStore } from '@/stores/display'
import type { SChannel } from '@/types'
import { useMessage } from 'naive-ui'
import { Settings as SettingsIcon } from '@vicons/tabler'

interface FavoriteEntry {
  id: string
  channel: SChannel | null
}

const emit = defineEmits<{
  (e: 'manage'): void
}>()

const chat = useChatStore()
const display = useDisplayStore()
const message = useMessage()

const flattenChannels = (channels?: SChannel[]): SChannel[] => {
  if (!channels || channels.length === 0) return []
  const result: SChannel[] = []
  const stack = [...channels]
  while (stack.length) {
    const current = stack.shift()
    if (!current) continue
    result.push(current)
    if (current.children && current.children.length > 0) {
      stack.unshift(...current.children)
    }
  }
  return result
}

const allChannels = computed<SChannel[]>(() => {
  const publicChannels = flattenChannels(chat.channelTree)
  const privateChannels = flattenChannels(chat.channelTreePrivate)
  return [...publicChannels, ...privateChannels]
})

const channelMap = computed(() => {
  const map = new Map<string, SChannel>()
  allChannels.value.forEach((channel) => {
    if (channel?.id) {
      map.set(channel.id, channel)
    }
  })
  return map
})

const favoriteEntries = computed<FavoriteEntry[]>(() =>
  display.favoriteChannelIds.map((id) => ({
    id,
    channel: channelMap.value.get(id) ?? null,
  })),
)

const activeChannelId = computed(() => chat.curChannel?.id ?? '')
const hasFavorites = computed(() => favoriteEntries.value.length > 0)
const missingCount = computed(() => favoriteEntries.value.filter((entry) => !entry.channel).length)

const handleFavoriteClick = async (entry: FavoriteEntry) => {
  if (!entry.channel) {
    display.removeFavoriteChannel(entry.id)
    message.warning('频道不可用，已自动移除')
    return
  }
  if (entry.id === activeChannelId.value) {
    return
  }
  const success = await chat.channelSwitchTo(entry.id)
  if (!success) {
    message.error('切换频道失败，请检查权限')
  }
}

const handleManageClick = () => emit('manage')
</script>

<template>
<section class="favorite-bar" role="region" aria-label="频道收藏快捷切换">
  <span class="favorite-bar__label">频道收藏</span>

  <div v-if="hasFavorites" class="favorite-bar__list" role="list">
    <button
      v-for="entry in favoriteEntries"
      :key="entry.id"
      class="favorite-bar__pill"
      :class="{
        'is-active': entry.id === activeChannelId,
        'is-disabled': !entry.channel,
      }"
      type="button"
      :title="entry.channel?.name || '频道不可用'"
      :disabled="!entry.channel"
      @click="handleFavoriteClick(entry)"
      role="listitem"
    >
      <span class="favorite-bar__pill-text">{{ entry.channel?.name || '频道不可用' }}</span>
    </button>
  </div>
  <span v-else class="favorite-bar__placeholder">暂无收藏</span>

  <span v-if="missingCount > 0" class="favorite-bar__warning">
    {{ missingCount }} 个失效
  </span>

  <n-button text size="tiny" class="favorite-bar__manage" @click="handleManageClick">
    <template #icon>
      <n-icon :component="SettingsIcon" size="14" />
    </template>
    管理
  </n-button>
</section>
</template>

<style scoped lang="scss">
.favorite-bar {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0;
  margin: 0;
  background: transparent;
  border: none;
}

.favorite-bar__label {
  font-size: 0.82rem;
  font-weight: 600;
  color: var(--sc-text-primary);
  white-space: nowrap;
}

.favorite-bar__list {
  display: flex;
  align-items: center;
  gap: 0.35rem;
  overflow-x: auto;
  padding: 0;
  margin: 0;
  scrollbar-width: none;
}

.favorite-bar__list::-webkit-scrollbar {
  display: none;
}

.favorite-bar__pill {
  border-radius: 999px;
  border: 1px solid transparent;
  background-color: transparent;
  color: var(--sc-text-primary);
  font-size: 0.82rem;
  padding: 0.1rem 0.6rem;
  font-weight: 500;
  cursor: pointer;
  transition: background-color 0.15s ease, color 0.15s ease, border-color 0.15s ease;
  white-space: nowrap;
}

.favorite-bar__pill:hover {
  background-color: rgba(14, 165, 233, 0.15);
}

.favorite-bar__pill.is-active {
  color: #0369a1;
  background-color: rgba(14, 165, 233, 0.22);
  border-color: rgba(14, 165, 233, 0.35);
}

.favorite-bar__pill.is-disabled {
  opacity: 0.6;
  cursor: not-allowed;
  background-color: rgba(148, 163, 184, 0.2);
  color: var(--sc-text-secondary);
}

.favorite-bar__pill-text {
  display: inline-block;
  max-width: 10rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.favorite-bar__placeholder {
  font-size: 0.82rem;
  color: var(--sc-text-secondary);
}

.favorite-bar__warning {
  font-size: 0.75rem;
  color: #f97316;
  white-space: nowrap;
}

.favorite-bar__manage {
  margin-left: auto;
  padding: 0;
}
</style>
