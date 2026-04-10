<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Speakerphone } from '@vicons/tabler'
import { useAnnouncementStore } from '@/stores/announcement'
import { useUserStore } from '@/stores/user'
import { chatEvent } from '@/stores/chat'
import { isTipTapJson, tiptapJsonToPlainText } from '@/utils/tiptap-render'
import type { AnnouncementItem } from '@/models/announcement'

const emit = defineEmits<{
  (event: 'open-announcement', item: AnnouncementItem): void
}>()

const announcementStore = useAnnouncementStore()
const user = useUserStore()

const items = ref<AnnouncementItem[]>([])
const loading = ref(false)
const activeIndex = ref(0)

let rotateTimer: ReturnType<typeof setInterval> | null = null

const canLoad = computed(() => !!user.info.id)
const activeItem = computed(() => {
  if (!items.value.length) {
    return null
  }
  const safeIndex = ((activeIndex.value % items.value.length) + items.value.length) % items.value.length
  return items.value[safeIndex] || null
})

const normalizeSummary = (item: AnnouncementItem) => {
  const raw = item.contentFormat === 'rich' && isTipTapJson(item.content)
    ? tiptapJsonToPlainText(item.content)
    : item.content || ''
  return raw.replace(/\s+/g, ' ').trim().slice(0, 120)
}

const activeSummary = computed(() => {
  const item = activeItem.value
  if (!item) {
    return ''
  }
  return normalizeSummary(item)
})

const clearRotateTimer = () => {
  if (rotateTimer !== null) {
    clearInterval(rotateTimer)
    rotateTimer = null
  }
}

const syncRotateTimer = () => {
  clearRotateTimer()
  if (typeof window === 'undefined' || items.value.length <= 1) {
    return
  }
  rotateTimer = setInterval(() => {
    activeIndex.value = (activeIndex.value + 1) % items.value.length
  }, 5000)
}

const load = async () => {
  if (!canLoad.value) {
    items.value = []
    activeIndex.value = 0
    clearRotateTimer()
    return
  }
  loading.value = true
  try {
    const data = await announcementStore.fetchLobbyList({
      page: 1,
      pageSize: 20,
      showInTicker: true,
    })
    items.value = data.items || []
    activeIndex.value = 0
    syncRotateTimer()
  } catch (error) {
    console.warn('load lobby ticker announcements failed', error)
    items.value = []
    activeIndex.value = 0
    clearRotateTimer()
  } finally {
    loading.value = false
  }
}

const handleLobbyAnnouncementUpdated = () => {
  void load()
}

const openAnnouncement = () => {
  const item = activeItem.value
  if (!item) {
    return
  }
  emit('open-announcement', item)
}

watch(canLoad, (value) => {
  if (value) {
    void load()
    return
  }
  items.value = []
  activeIndex.value = 0
  clearRotateTimer()
}, { immediate: true })

watch(() => items.value.length, () => {
  if (activeIndex.value >= items.value.length) {
    activeIndex.value = 0
  }
  syncRotateTimer()
})

onMounted(() => {
  chatEvent.on('lobby-announcement-updated', handleLobbyAnnouncementUpdated as any)
})

onBeforeUnmount(() => {
  chatEvent.off('lobby-announcement-updated', handleLobbyAnnouncementUpdated as any)
  clearRotateTimer()
})
</script>

<template>
  <button
    v-if="activeItem"
    type="button"
    class="world-lobby-ticker"
    :disabled="loading"
    @click="openAnnouncement"
  >
    <span class="world-lobby-ticker__icon" aria-hidden="true">
      <n-icon size="14">
        <Speakerphone />
      </n-icon>
    </span>
    <div class="world-lobby-ticker__content">
      <div class="world-lobby-ticker__line">
        <span class="world-lobby-ticker__title">{{ activeItem.title }}</span>
        <span v-if="activeSummary" class="world-lobby-ticker__separator">-</span>
        <span v-if="activeSummary" class="world-lobby-ticker__summary">{{ activeSummary }}</span>
      </div>
    </div>
    <div v-if="items.length > 1" class="world-lobby-ticker__dots" aria-hidden="true">
      <span
        v-for="(item, index) in items"
        :key="item.id"
        class="world-lobby-ticker__dot"
        :class="{ 'world-lobby-ticker__dot--active': index === activeIndex }"
      />
    </div>
  </button>
</template>

<style scoped>
.world-lobby-ticker {
  width: 100%;
  border: none;
  border-radius: 10px;
  padding: 4px 2px;
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 8px;
  background: transparent;
  box-shadow: none;
  cursor: pointer;
  transition: color 0.2s ease, opacity 0.2s ease;
  text-align: left;
}

.world-lobby-ticker:hover .world-lobby-ticker__icon,
.world-lobby-ticker:hover .world-lobby-ticker__title {
  color: color-mix(in srgb, #3388de 78%, var(--sc-text-primary));
}

.world-lobby-ticker:focus-visible {
  outline: 2px solid color-mix(in srgb, #3388de 28%, transparent);
  outline-offset: 3px;
}

.world-lobby-ticker:disabled {
  cursor: default;
  opacity: 0.72;
}

.world-lobby-ticker__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: color-mix(in srgb, #3388de 66%, var(--sc-text-secondary));
  opacity: 0.9;
}

.world-lobby-ticker__content {
  min-width: 0;
}

.world-lobby-ticker__line {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 1.4;
}

.world-lobby-ticker__title {
  font-size: 13px;
  font-weight: 600;
  color: var(--sc-text-primary);
}

.world-lobby-ticker__separator {
  margin: 0 6px;
  font-size: 12px;
  color: color-mix(in srgb, var(--sc-text-secondary) 56%, transparent);
}

.world-lobby-ticker__summary {
  font-size: 12px;
  color: var(--sc-text-secondary);
}

.world-lobby-ticker__dots {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  justify-self: end;
}

.world-lobby-ticker__dot {
  width: 4px;
  height: 4px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--sc-text-secondary) 22%, transparent);
  transition: transform 0.2s ease, background-color 0.2s ease;
}

.world-lobby-ticker__dot--active {
  background: color-mix(in srgb, #3388de 78%, var(--sc-text-primary));
  transform: scale(1.1);
}
</style>
