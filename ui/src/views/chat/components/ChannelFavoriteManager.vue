<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useChatStore } from '@/stores/chat'
import { useDisplayStore, FAVORITE_CHANNEL_LIMIT } from '@/stores/display'
import type { SChannel } from '@/types'
import { useMessage } from 'naive-ui'
import { Plus as PlusIcon, Trash as TrashIcon, Star as StarIcon } from '@vicons/tabler'

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
  const publicChannels = flattenChannels(chat.channelTree)
  const privateChannels = flattenChannels(chat.channelTreePrivate)
  return [...publicChannels, ...privateChannels]
})

const channelOptions = computed(() =>
  allChannels.value.map((channel) => ({
    label: channel.name,
    value: channel.id,
    disabled: display.favoriteChannelIds.includes(channel.id),
  })),
)

const favoriteDetails = computed(() =>
  display.favoriteChannelIds.map((id) => ({
    id,
    channel: allChannels.value.find((channel) => channel.id === id) ?? null,
  })),
)

const remainingSlots = computed(() => Math.max(FAVORITE_CHANNEL_LIMIT - display.favoriteChannelIds.length, 0))
const canAddMore = computed(() => display.favoriteChannelIds.length < FAVORITE_CHANNEL_LIMIT)
const hasChannelsAvailable = computed(() =>
  channelOptions.value.some((option) => !option.disabled),
)

const handleAddFavorite = () => {
  if (!selectedChannelId.value) {
    message.warning('请选择要收藏的频道')
    return
  }
  if (!canAddMore.value) {
    message.error('已达到收藏上限')
    return
  }
  display.addFavoriteChannel(selectedChannelId.value)
  selectedChannelId.value = null
  message.success('已添加到收藏')
}

const handleRemoveFavorite = (id: string) => {
  display.removeFavoriteChannel(id)
  message.success('已从收藏中移除')
}

const handleToggleBar = (value: boolean) => {
  display.setFavoriteBarEnabled(value)
}

const handleClose = () => emit('update:show', false)

watch(
  () => props.show,
  (visible) => {
    if (!visible) {
      selectedChannelId.value = null
    }
  },
)
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
  align-items: center;
  justify-content: space-between;
  padding: 0.6rem 0.4rem;
  border-bottom: 1px solid var(--sc-border-soft, rgba(148, 163, 184, 0.2));
}

.favorite-manager__item:last-child {
  border-bottom: none;
}

.favorite-manager__item-meta {
  display: flex;
  flex-direction: column;
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
