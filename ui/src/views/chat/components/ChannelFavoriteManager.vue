<script setup lang="ts">
import { computed, ref, watch, onBeforeUnmount } from 'vue'
import { useChatStore } from '@/stores/chat'
import { useDisplayStore, FAVORITE_CHANNEL_LIMIT } from '@/stores/display'
import type { FavoriteHotkey } from '@/stores/display'
import type { SChannel } from '@/types'
import { useMessage } from 'naive-ui'
import { Plus as PlusIcon, Trash as TrashIcon, Star as StarIcon } from '@vicons/tabler'
import { useEventListener } from '@vueuse/core'
import { buildHotkeyDescriptor, formatHotkeyCombo } from '@/utils/hotkey'

interface Props {
  show: boolean
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:show', value: boolean): void
}>()

const chat = useChatStore()
const display = useDisplayStore()
const message = useMessage()

const selectedChannelId = ref<string | null>(null)
const newFavoriteHotkey = ref<FavoriteHotkey | null>(null)
const shortcutRecordingTarget = ref<{ type: 'existing' | 'new'; channelId?: string } | null>(null)
const favoriteCandidates = ref<SChannel[]>([])
const favoriteCandidatesReady = ref(false)
let stopShortcutListener: (() => void) | null = null

const flattenChannels = (channels?: SChannel[]): SChannel[] => {
  if (!channels || channels.length === 0) return []
  const result: SChannel[] = []
  const traverse = (nodes: SChannel[]) => {
    nodes.forEach((node) => {
      result.push(node)
      if (node.children && node.children.length) {
        traverse(node.children)
      }
    })
  }
  traverse(channels)
  return result
}

const allChannels = computed<SChannel[]>(() => {
  const worldId = chat.currentWorldId
  const tree = (worldId && chat.channelTreeByWorld?.[worldId]) || []
  if (!Array.isArray(tree) || tree.length === 0) return []
  const publicChannels = flattenChannels(tree)
  const privateChannels = flattenChannels(chat.channelTreePrivate)
  return [...publicChannels, ...privateChannels]
})

const currentWorldId = computed(() => chat.currentWorldId || undefined)
const favoriteIds = computed(() => display.getFavoriteChannelIds(currentWorldId.value))
const favoriteHotkeyMap = computed<Record<string, FavoriteHotkey>>(
  () => display.getFavoriteHotkeyMap(currentWorldId.value),
)

const favoriteDataReady = computed(() => {
  const worldId = currentWorldId.value
  const worldReady = worldId ? !!chat.channelTreeReady?.[worldId] : chat.channelTree.length > 0
  return Boolean(worldReady && chat.channelTreePrivateReady)
})

const channelOptions = computed(() =>
  favoriteCandidates.value.map((channel) => ({
    label: channel.name,
    value: channel.id,
    disabled: favoriteIds.value.includes(channel.id),
  })),
)

const favoriteDetails = computed(() =>
  favoriteIds.value.map((id) => ({
    id,
    channel: allChannels.value.find((channel) => channel.id === id) ?? null,
    hotkey: favoriteHotkeyMap.value[id] || null,
  })),
)

const remainingSlots = computed(() => Math.max(FAVORITE_CHANNEL_LIMIT - favoriteIds.value.length, 0))
const canAddMore = computed(() => favoriteIds.value.length < FAVORITE_CHANNEL_LIMIT)
const hasChannelsAvailable = computed(() =>
  channelOptions.value.some((option) => !option.disabled),
)

const loadFavoriteCandidates = async (force = false) => {
  const worldId = currentWorldId.value
  favoriteCandidatesReady.value = false
  if (!worldId) {
    favoriteCandidates.value = []
    favoriteCandidatesReady.value = true
    return
  }
  try {
    const list = await chat.channelFavoriteCandidateList(worldId, force)
    favoriteCandidates.value = Array.isArray(list) ? list : []
  } catch (error) {
    console.warn('获取收藏候选频道失败', error)
    favoriteCandidates.value = []
  } finally {
    favoriteCandidatesReady.value = true
  }
}

const handleAddFavorite = () => {
  const channelId = selectedChannelId.value
  if (!channelId) {
    message.warning('请选择要收藏的频道')
    return
  }
  if (!canAddMore.value) {
    message.error('已达到收藏上限')
    return
  }
  display.addFavoriteChannel(channelId, currentWorldId.value)
  if (newFavoriteHotkey.value) {
    const result = display.setFavoriteHotkey(channelId, newFavoriteHotkey.value, currentWorldId.value)
    if (!result.success) {
      if (result.reason === 'conflict') {
        message.error('该快捷键已被其他频道使用')
      } else {
        message.error('快捷键记录失败，请重试')
      }
    } else {
      message.success('快捷键已记录')
      newFavoriteHotkey.value = null
    }
  }
  selectedChannelId.value = null
  message.success('已添加到收藏')
}

const handleRemoveFavorite = (id: string) => {
  display.removeFavoriteChannel(id, currentWorldId.value)
  message.success('已从收藏中移除')
}

const stopShortcutRecording = () => {
  stopShortcutListener?.()
  stopShortcutListener = null
  shortcutRecordingTarget.value = null
}

const applyRecordedHotkey = (channelId: string, hotkey: FavoriteHotkey) => {
  const result = display.setFavoriteHotkey(channelId, hotkey, currentWorldId.value)
  if (!result.success) {
    if (result.reason === 'conflict') {
      message.error('该快捷键已被其他频道使用')
    } else {
      message.error('快捷键设置失败')
    }
    return false
  }
  message.success('快捷键已更新')
  return true
}

const handleShortcutCapture = (event: KeyboardEvent) => {
  if (!shortcutRecordingTarget.value) return
  event.preventDefault()
  event.stopPropagation()
  if (!event.ctrlKey && !event.metaKey && !event.altKey && event.key === 'Escape' && !event.shiftKey) {
    stopShortcutRecording()
    message.info('已取消快捷键录制')
    return
  }
  const descriptor = buildHotkeyDescriptor(event)
  if (!descriptor) {
    message.warning('请按下包含 Ctrl/Cmd/Alt 的组合键')
    return
  }
  const target = shortcutRecordingTarget.value
  if (target.type === 'existing' && target.channelId) {
    const success = applyRecordedHotkey(target.channelId, descriptor)
    if (success) {
      stopShortcutRecording()
    }
    return
  }
  if (target.type === 'new') {
    newFavoriteHotkey.value = descriptor
    message.success(`已记录 ${descriptor.combo}`)
    stopShortcutRecording()
  }
}

const beginShortcutRecording = (target: { type: 'existing' | 'new'; channelId?: string }) => {
  stopShortcutRecording()
  shortcutRecordingTarget.value = target
  stopShortcutListener = useEventListener(window, 'keydown', handleShortcutCapture, {
    capture: true,
    passive: false,
  })
  message.info('请按下要绑定的组合键，按 ESC 退出')
}

const clearFavoriteHotkey = (channelId: string) => {
  display.clearFavoriteHotkey(channelId, currentWorldId.value)
  message.success('已清除快捷键')
}

const clearPendingHotkey = () => {
  if (newFavoriteHotkey.value) {
    newFavoriteHotkey.value = null
    message.success('已清除待添加快捷键')
  }
}

const handleToggleBar = (value: boolean) => {
  display.setFavoriteBarEnabled(value)
}

const handleClose = () => emit('update:show', false)

watch(
  () => ({
    ready: favoriteDataReady.value,
    worldId: currentWorldId.value,
    ids: allChannels.value.map((c) => c.id).filter(Boolean),
  }),
  ({ ready, worldId, ids }) => {
    if (!ready || !ids.length) return
    display.syncFavoritesWithChannels(ids, worldId)
  },
  { immediate: true },
)

watch(
  () => [props.show, currentWorldId.value] as const,
  ([visible, worldId], [prevVisible, prevWorldId]) => {
    if (!visible) return
    const worldChanged = worldId && worldId !== prevWorldId
    if (worldChanged) {
      favoriteCandidates.value = []
    }
    if (worldId && (worldChanged || !favoriteCandidatesReady.value)) {
      loadFavoriteCandidates(worldChanged)
    }
  },
  { immediate: true },
)

watch(
  () => props.show,
  (visible) => {
    if (!visible) {
      selectedChannelId.value = null
      newFavoriteHotkey.value = null
      stopShortcutRecording()
    }
  },
)

onBeforeUnmount(() => {
  stopShortcutRecording()
})
</script>

<template>
  <n-modal
    preset="card"
    :show="props.show"
    title="频道收藏"
    class="favorite-manager"
    :style="{ width: '520px' }"
    @update:show="emit('update:show', $event)"
  >
    <section class="favorite-manager__section">
      <header>
        <div class="section-title">
          <n-icon :component="StarIcon" size="16" />
          <span>收藏栏开关</span>
        </div>
        <p class="section-desc">开启后将常驻显示在频道标题下方，便于快速切换频道</p>
      </header>
      <n-switch :value="display.favoriteBarEnabled" @update:value="handleToggleBar">
        <template #checked>已开启</template>
        <template #unchecked>已关闭</template>
      </n-switch>
    </section>

    <n-alert
      v-if="shortcutRecordingTarget"
      type="info"
      size="small"
      :bordered="false"
      class="favorite-manager__section"
    >
      正在录制快捷键，按 ESC 取消。
    </n-alert>

    <section class="favorite-manager__section">
      <header>
        <div class="section-title">
          <span>已收藏频道</span>
          <n-tag size="small" type="info">{{ favoriteDetails.length }}/{{ FAVORITE_CHANNEL_LIMIT }}</n-tag>
        </div>
        <p class="section-desc">最多可收藏 {{ FAVORITE_CHANNEL_LIMIT }} 个频道，顺序即为显示顺序</p>
      </header>

      <template v-if="favoriteDetails.length">
        <div class="favorite-manager__list">
          <div v-for="item in favoriteDetails" :key="item.id" class="favorite-manager__item">
            <div class="favorite-manager__item-meta">
              <p class="favorite-manager__item-name">
                {{ item.channel?.name || '频道不可用' }}
              </p>
              <p class="favorite-manager__item-desc">
                {{ item.channel ? `ID：${item.channel.id}` : '该频道可能已删除或不可访问' }}
              </p>
              <div class="favorite-manager__shortcut">
                <span class="favorite-manager__shortcut-label">
                  快捷键：{{ formatHotkeyCombo(item.hotkey) || '未设置' }}
                </span>
                <div class="favorite-manager__shortcut-actions">
                  <n-button
                    text
                    size="tiny"
                    @click="beginShortcutRecording({ type: 'existing', channelId: item.id })"
                  >
                    录制
                  </n-button>
                  <n-button
                    text
                    size="tiny"
                    :disabled="!item.hotkey"
                    @click="clearFavoriteHotkey(item.id)"
                  >
                    清除
                  </n-button>
                </div>
              </div>
            </div>
            <n-button text size="small" type="error" @click="handleRemoveFavorite(item.id)">
              <template #icon>
                <n-icon :component="TrashIcon" size="16" />
              </template>
              移除
            </n-button>
          </div>
        </div>
      </template>
      <n-empty v-else description="尚未收藏任何频道" />
    </section>

    <section class="favorite-manager__section">
      <header>
        <div class="section-title">
          <span>添加新频道</span>
        </div>
        <p class="section-desc">
          仅展示当前可访问的频道，已收藏的频道会自动过滤
        </p>
      </header>
      <div class="favorite-manager__add">
        <n-select
          v-model:value="selectedChannelId"
          :options="channelOptions"
          placeholder="选择要收藏的频道"
          size="small"
          filterable
          clearable
        />
        <n-button type="primary" size="small" :disabled="!selectedChannelId || !canAddMore" @click="handleAddFavorite">
          <template #icon>
            <n-icon :component="PlusIcon" size="16" />
          </template>
          添加
        </n-button>
      </div>
      <div class="favorite-manager__shortcut-new">
        <div>
          <span class="favorite-manager__shortcut-label">
            预设快捷键：{{ formatHotkeyCombo(newFavoriteHotkey) || '未设置' }}
          </span>
          <p class="favorite-manager__shortcut-desc">快捷键需包含 Ctrl/Cmd/Alt 中至少一个按键</p>
        </div>
        <div class="favorite-manager__shortcut-actions">
          <n-button
            text
            size="small"
            @click="beginShortcutRecording({ type: 'new' })"
          >
            录制
          </n-button>
          <n-button text size="small" :disabled="!newFavoriteHotkey" @click="clearPendingHotkey">
            清除
          </n-button>
        </div>
      </div>
      <n-alert
        v-if="!hasChannelsAvailable"
        type="warning"
        size="small"
        :bordered="false"
      >
        所有可访问频道都已收藏或不可用。
      </n-alert>
      <n-alert
        v-else
        type="info"
        size="small"
        :bordered="false"
      >
        还可以添加 {{ remainingSlots }} 个收藏频道。
      </n-alert>
    </section>

    <template #footer>
      <div class="favorite-manager__footer">
        <n-button @click="handleClose">完成</n-button>
      </div>
    </template>
  </n-modal>
</template>

<style scoped lang="scss">
.favorite-manager__section + .favorite-manager__section {
  margin-top: 1rem;
}

.section-title {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  font-weight: 600;
  color: var(--sc-text-primary);
}

.section-desc {
  margin: 0.25rem 0 0;
  font-size: 0.85rem;
  color: var(--sc-text-secondary);
}

.favorite-manager__list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  margin-top: 0.75rem;
}

.favorite-manager__item {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
  padding: 0.6rem 0.4rem;
  border-bottom: 1px solid var(--sc-border-soft, rgba(148, 163, 184, 0.2));
}

.favorite-manager__item:last-child {
  border-bottom: none;
}

.favorite-manager__item-meta {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}

.favorite-manager__item-name {
  font-weight: 600;
  margin: 0;
}

.favorite-manager__item-desc {
  margin: 0.2rem 0 0;
  font-size: 0.8rem;
  color: var(--sc-text-secondary);
}

.favorite-manager__shortcut {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  flex-wrap: wrap;
}

.favorite-manager__shortcut-label {
  font-size: 0.85rem;
  color: var(--sc-text-primary);
}

.favorite-manager__shortcut-actions {
  display: inline-flex;
  gap: 0.25rem;
  flex-shrink: 0;
}

.favorite-manager__shortcut-new {
  margin-top: 0.75rem;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  flex-wrap: wrap;
  padding: 0.5rem;
  border: 1px dashed var(--sc-border-soft, rgba(148, 163, 184, 0.5));
  border-radius: 6px;
}

.favorite-manager__shortcut-desc {
  margin: 0.15rem 0 0;
  font-size: 0.75rem;
  color: var(--sc-text-secondary);
}

.favorite-manager__add {
  margin-top: 0.75rem;
  display: flex;
  gap: 0.5rem;
  align-items: center;
}

.favorite-manager__footer {
  display: flex;
  justify-content: flex-end;
}
</style>
