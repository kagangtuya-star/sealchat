<script setup lang="ts">
import { computed, ref, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { storeToRefs } from 'pinia'
import { useWindowSize, useInfiniteScroll } from '@vueuse/core'
import { NDrawer, NDrawerContent, NButton, NIcon, NSpin, NEmpty, NButtonGroup, NTooltip } from 'naive-ui'
import { LocationOutline, TimeOutline, GridOutline, AppsOutline, RefreshOutline } from '@vicons/ionicons5'
import dayjs from 'dayjs'
import Viewer from 'viewerjs'
import 'viewerjs/dist/viewer.css'
import { useChannelImagesStore } from '@/stores/channelImages'
import { useChatStore, chatEvent } from '@/stores/chat'
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver'
import Avatar from '@/components/avatar.vue'

interface JumpPayload {
  messageId: string
  displayOrder?: number
  createdAt?: number
}

const emit = defineEmits<{
  (e: 'locate-message', payload: JumpPayload): void
}>()

const channelImages = useChannelImagesStore()
const chat = useChatStore()
const { panelVisible, items, loading, loadingMore, hasMore, total, thumbnailMode } = storeToRefs(channelImages)

const { width: viewportWidth } = useWindowSize()
const isMobileLayout = computed(() => viewportWidth.value > 0 && viewportWidth.value < 768)
const drawerWidth = computed(() => isMobileLayout.value ? '100%' : 520)

const scrollContainerRef = ref<HTMLElement | null>(null)
const imageGridRef = ref<HTMLElement | null>(null)
let galleryViewer: Viewer | null = null

// Grid columns based on thumbnail mode
const gridColumns = computed(() => {
  if (thumbnailMode.value === 'small') {
    return isMobileLayout.value ? 'repeat(4, 1fr)' : 'repeat(5, 1fr)'
  }
  return isMobileLayout.value ? 'repeat(2, 1fr)' : 'repeat(3, 1fr)'
})

// Infinite scroll
useInfiniteScroll(
  scrollContainerRef,
  async () => {
    if (hasMore.value && !loadingMore.value) {
      await channelImages.loadMore()
    }
  },
  { distance: 100 }
)

// Setup viewerjs for the image grid
const destroyGalleryViewer = () => {
  if (galleryViewer) {
    galleryViewer.destroy()
    galleryViewer = null
  }
}

const setupGalleryViewer = async () => {
  await nextTick()
  const grid = imageGridRef.value
  if (!grid) {
    destroyGalleryViewer()
    return
  }

  const images = grid.querySelectorAll<HTMLImageElement>('img.gallery-thumb')
  if (!images.length) {
    destroyGalleryViewer()
    return
  }

  // 总是重新创建以确保图片列表更新
  destroyGalleryViewer()

  const hasMultiple = images.length > 1
  galleryViewer = new Viewer(grid, {
    className: 'channel-gallery-viewer',
    filter: (image: HTMLImageElement) => image.classList.contains('gallery-thumb'),
    url: 'data-original',  // 使用 data-original 属性加载原图
    navbar: hasMultiple,  // 多图时显示缩略图导航
    title: false,
    toolbar: {
      zoomIn: true,
      zoomOut: true,
      oneToOne: true,
      reset: true,
      prev: hasMultiple,
      play: false,
      next: hasMultiple,
      rotateLeft: true,
      rotateRight: true,
      flipHorizontal: false,
      flipVertical: false,
    },
    tooltip: true,
    movable: true,
    zoomable: true,
    scalable: true,
    rotatable: true,
    transition: true,
    fullscreen: true,
    keyboard: true,
    zIndex: 3000,
  })
}

// Watch for items changes to update viewer
watch(() => items.value.length, () => {
  if (panelVisible.value) {
    void setupGalleryViewer()
  }
})

// Reset scroll position when panel opens
watch(panelVisible, (visible) => {
  if (visible) {
    nextTick(() => {
      if (scrollContainerRef.value) {
        scrollContainerRef.value.scrollTop = 0
      }
      void setupGalleryViewer()
    })
  } else {
    destroyGalleryViewer()
  }
})

// Auto-refresh when new messages arrive (only when panel is visible)
const handleNewMessage = (event: any) => {
  if (!panelVisible.value) return
  // Check if message contains an image
  const content = event?.message?.content || ''
  if (content.includes('id:') && channelImages.channelId === event?.message?.channel_id) {
    // Debounced refresh - wait a bit for the message to be saved
    setTimeout(() => {
      channelImages.refresh()
    }, 500)
  }
}

onMounted(() => {
  chatEvent.on('message-created', handleNewMessage)
})

onUnmounted(() => {
  chatEvent.off('message-created', handleNewMessage)
  destroyGalleryViewer()
})

const handleClose = () => {
  channelImages.closePanel()
}

const handleRefresh = () => {
  channelImages.refresh()
}

const handleLocate = (item: { messageId: string; displayOrder: number; createdAt: number }) => {
  emit('locate-message', {
    messageId: item.messageId,
    displayOrder: item.displayOrder,
    createdAt: item.createdAt,
  })
}

const handleImageClick = async (index: number) => {
  if (!galleryViewer) {
    await setupGalleryViewer()
  }
  if (galleryViewer) {
    galleryViewer.view(index)
  }
}

const formatTime = (timestamp: number) => {
  if (!timestamp) return ''
  return dayjs(timestamp).format('MM-DD HH:mm')
}

// Get full original image URL
const getImageUrl = (attachmentId: string) => {
  return resolveAttachmentUrl(attachmentId) || `/api/v1/attachment/${attachmentId}`
}

// Get thumbnail URL with fixed size (200px for all modes to reduce storage)
const getThumbUrl = (item: { thumbUrl?: string; attachmentId: string }) => {
  const size = 200  // 统一使用 200px，减少缓存占用
  if (item.thumbUrl) {
    // Replace size parameter if exists, otherwise append
    return item.thumbUrl.replace(/size=\d+/, `size=${size}`)
  }
  // Fallback to thumb endpoint
  return `/api/v1/attachment/${item.attachmentId}/thumb?size=${size}`
}
</script>

<template>
  <n-drawer
    class="channel-images-drawer"
    :show="panelVisible"
    placement="right"
    :width="drawerWidth"
    :mask-closable="true"
    :close-on-esc="true"
    @update:show="(v) => v ? null : handleClose()"
  >
    <n-drawer-content closable>
      <template #header>
        <div class="images-header">
          <n-button v-if="isMobileLayout" text class="images-header__back" @click="handleClose">
            <template #icon>
              <n-icon size="18">
                <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M15.41 7.41L14 6l-6 6 6 6 1.41-1.41L10.83 12z"/>
                </svg>
              </n-icon>
            </template>
            返回
          </n-button>
          <span class="images-header__title">频道图片</span>
          <span class="images-header__count">{{ total }} 张</span>

          <!-- Toolbar: Mode toggle & Refresh -->
          <div class="images-header__toolbar">
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-button quaternary size="small" @click="handleRefresh" :loading="loading">
                  <template #icon>
                    <n-icon :component="RefreshOutline" />
                  </template>
                </n-button>
              </template>
              刷新
            </n-tooltip>

            <n-button-group size="small">
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-button
                    :type="thumbnailMode === 'large' ? 'primary' : 'default'"
                    quaternary
                    @click="channelImages.setThumbnailMode('large')"
                  >
                    <template #icon>
                      <n-icon :component="GridOutline" />
                    </template>
                  </n-button>
                </template>
                大图模式
              </n-tooltip>
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-button
                    :type="thumbnailMode === 'small' ? 'primary' : 'default'"
                    quaternary
                    @click="channelImages.setThumbnailMode('small')"
                  >
                    <template #icon>
                      <n-icon :component="AppsOutline" />
                    </template>
                  </n-button>
                </template>
                小图模式
              </n-tooltip>
            </n-button-group>
          </div>
        </div>
      </template>

      <div class="images-content" ref="scrollContainerRef">
        <!-- Loading state -->
        <div v-if="loading && items.length === 0" class="images-loading">
          <n-spin size="large" />
          <p>加载中...</p>
        </div>

        <!-- Empty state -->
        <div v-else-if="items.length === 0" class="images-empty">
          <n-empty description="暂无图片" />
        </div>

        <!-- Image grid -->
        <div v-else class="images-grid" :style="{ gridTemplateColumns: gridColumns }" ref="imageGridRef">
          <div
            v-for="(item, index) in items"
            :key="item.id"
            class="image-card"
            :class="{ 'image-card--compact': thumbnailMode === 'small' }"
          >
            <div class="image-card__thumb" @click="handleImageClick(index)">
              <img
                class="gallery-thumb"
                :src="getThumbUrl(item)"
                :data-original="getImageUrl(item.attachmentId)"
                loading="lazy"
                alt=""
              />
            </div>
            <!-- Only show sender info in large mode -->
            <div v-if="thumbnailMode === 'large'" class="image-card__info">
              <div class="image-card__sender">
                <Avatar
                  :src="item.senderAvatar"
                  :size="20"
                  :border="false"
                />
                <span class="sender-name">{{ item.senderName || '未知' }}</span>
              </div>
              <div class="image-card__meta">
                <n-icon :component="TimeOutline" size="12" />
                <span>{{ formatTime(item.createdAt) }}</span>
              </div>
            </div>
            <n-button
              v-if="thumbnailMode === 'large'"
              class="image-card__locate"
              size="tiny"
              type="primary"
              ghost
              @click.stop="handleLocate(item)"
            >
              <template #icon>
                <n-icon :component="LocationOutline" size="14" />
              </template>
              定位
            </n-button>
            <!-- Compact locate button for small mode -->
            <n-tooltip v-else trigger="hover">
              <template #trigger>
                <n-button
                  class="image-card__locate-compact"
                  size="tiny"
                  circle
                  quaternary
                  @click.stop="handleLocate(item)"
                >
                  <template #icon>
                    <n-icon :component="LocationOutline" size="12" />
                  </template>
                </n-button>
              </template>
              定位到消息
            </n-tooltip>
          </div>
        </div>

        <!-- Load more indicator -->
        <div v-if="loadingMore" class="images-loading-more">
          <n-spin size="small" />
          <span>加载更多...</span>
        </div>

        <!-- End of list -->
        <div v-if="!hasMore && items.length > 0" class="images-end">
          已加载全部图片
        </div>
      </div>

      <!-- Keyboard hint -->
      <div class="images-footer">
        <span class="keyboard-hint">点击图片查看大图 · ← → 切换</span>
      </div>
    </n-drawer-content>
  </n-drawer>
</template>

<style scoped>
.channel-images-drawer :deep(.n-drawer),
.channel-images-drawer :deep(.n-drawer-body) {
  background-color: var(--sc-bg-elevated, #ffffff);
  color: var(--sc-text-primary, #0f172a);
  transition: background-color 0.25s ease, color 0.25s ease;
}

.images-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  width: 100%;
}

.images-header__back {
  margin-right: auto;
  font-size: 14px;
}

.images-header__title {
  font-weight: 600;
}

.images-header__count {
  font-size: 0.85rem;
  color: var(--sc-text-secondary, #64748b);
  background: var(--sc-chip-bg, rgba(15, 23, 42, 0.06));
  padding: 0.2rem 0.5rem;
  border-radius: 99px;
}

.images-header__toolbar {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  margin-left: auto;
}

.images-content {
  height: calc(100% - 2rem);
  overflow-y: auto;
  padding: 0 0.25rem;
}

/* Scrollbar */
.images-content::-webkit-scrollbar {
  width: 4px;
}

.images-content::-webkit-scrollbar-track {
  background: transparent;
}

.images-content::-webkit-scrollbar-thumb {
  background: var(--sc-scrollbar-thumb, rgba(148, 163, 184, 0.4));
  border-radius: 2px;
}

.images-content::-webkit-scrollbar-thumb:hover {
  background: var(--sc-scrollbar-thumb-hover, rgba(148, 163, 184, 0.6));
}

.images-loading,
.images-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 200px;
  gap: 1rem;
  color: var(--sc-text-secondary, #64748b);
}

.images-grid {
  display: grid;
  gap: 0.75rem;
  padding-bottom: 1rem;
}

.image-card {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  padding: 0.5rem;
  border-radius: 12px;
  background: var(--sc-bg-surface, #f8fafc);
  border: 1px solid var(--sc-border-mute, rgba(148, 163, 184, 0.15));
  transition: all 0.2s ease;
  position: relative;
}

.image-card--compact {
  padding: 0.25rem;
  gap: 0;
  border-radius: 8px;
}

.image-card:hover {
  border-color: var(--sc-border-strong, rgba(59, 130, 246, 0.4));
  box-shadow: 0 4px 12px rgba(15, 23, 42, 0.08);
  transform: translateY(-1px);
}

.image-card__thumb {
  aspect-ratio: 1;
  border-radius: 8px;
  overflow: hidden;
  background: var(--sc-bg-mute, #e2e8f0);
  cursor: pointer;
}

.image-card--compact .image-card__thumb {
  border-radius: 6px;
}

.image-card__thumb img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  transition: transform 0.2s ease;
}

.image-card:hover .image-card__thumb img {
  transform: scale(1.03);
}

.image-card__info {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.image-card__sender {
  display: flex;
  align-items: center;
  gap: 0.35rem;
}

.sender-name {
  font-size: 0.8rem;
  font-weight: 500;
  color: var(--sc-text-primary, #1e293b);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100px;
}

.image-card__meta {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  font-size: 0.7rem;
  color: var(--sc-text-tertiary, #94a3b8);
}

.image-card__locate {
  margin-top: auto;
}

.image-card__locate-compact {
  position: absolute;
  bottom: 0.25rem;
  right: 0.25rem;
  opacity: 0;
  transition: opacity 0.15s ease;
  background: rgba(0, 0, 0, 0.5);
  color: #fff;
}

.image-card:hover .image-card__locate-compact {
  opacity: 1;
}

.images-loading-more {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  padding: 1rem;
  color: var(--sc-text-secondary, #64748b);
  font-size: 0.85rem;
}

.images-end {
  text-align: center;
  padding: 1rem;
  color: var(--sc-text-tertiary, #94a3b8);
  font-size: 0.8rem;
}

.images-footer {
  padding: 0.5rem 0;
  text-align: center;
}

.keyboard-hint {
  font-size: 0.75rem;
  color: var(--sc-text-tertiary, #94a3b8);
}

@media (max-width: 768px) {
  .images-header__toolbar {
    gap: 0.15rem;
  }

  .image-card {
    padding: 0.35rem;
  }

  .image-card--compact {
    padding: 0.15rem;
  }

  .sender-name {
    max-width: 70px;
  }

  .keyboard-hint {
    display: none;
  }
}
</style>
