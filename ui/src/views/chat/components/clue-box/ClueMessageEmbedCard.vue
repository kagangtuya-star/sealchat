<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NButton, NIcon, NSpin, useMessage } from 'naive-ui'
import { Edit, ExternalLink, FileText, Photo, PictureInPicture, World } from '@vicons/tabler'
import { useWorldClueStore, type WorldClueResolveItem } from '@/stores/worldClue'
import { chatEvent } from '@/stores/chat'
import { urlBase } from '@/stores/_config'
import { useUtilsStore } from '@/stores/utils'
import {
  buildInternalSurfaceResourceKey,
  generateInternalSurfaceLink,
  resolveInternalSurfaceLinkBase,
} from '@/utils/internalSurfaceLink'
import { isTheaterChatFrame, requestTheaterFloatingOpen } from '@/utils/theaterFloatingBridge'
import { parseWorldClueEmbedLink } from '@/utils/worldClueEmbedLink'

const props = defineProps<{ worldId: string; clueId: string; rawLink: string }>()
const store = useWorldClueStore()
const utilsStore = useUtilsStore()
const message = useMessage()
const item = ref<WorldClueResolveItem | null>(null)
const loading = ref(true)
const icon = computed(() => item.value?.kind === 'image' ? Photo : item.value?.kind === 'iframe' ? World : FileText)
const thumbnail = computed(() => item.value?.thumbnailAttachmentId
	? `${urlBase}/api/v1/worlds/${encodeURIComponent(props.worldId)}/clues/${encodeURIComponent(props.clueId)}/image?v=${encodeURIComponent(item.value.thumbnailAttachmentId)}`
  : item.value?.thumbnailUrl || '')
const updatedLabel = computed(() => item.value?.updatedAt
  ? new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(item.value.updatedAt))
  : '')
const updatedISO = computed(() => item.value?.updatedAt ? new Date(item.value.updatedAt).toISOString() : '')

async function resolveItem() {
  loading.value = true
  try { item.value = await store.requestResolve(props.worldId, props.clueId) }
  catch { item.value = { accessible: false } }
  finally { loading.value = false }
}

watch(() => [props.worldId, props.clueId] as const, () => void resolveItem(), { immediate: true })

function handleChanged(event: any) {
  const payload = event?.worldClue || event?.argv?.options || event?.argv?.Options || {}
  if (String(payload.worldId || '') !== props.worldId) return
  if (payload.action === 'edit-lock') return
  const clueId = String(payload.clueId || '')
  if (clueId && clueId !== props.clueId) return
  store.invalidate(props.worldId, clueId || undefined)
  void resolveItem()
}

onMounted(() => chatEvent.on('world-clue-changed' as any, handleChanged as any))
onBeforeUnmount(() => chatEvent.off('world-clue-changed' as any, handleChanged as any))

function getResource() {
	const channelId = parseWorldClueEmbedLink(props.rawLink)?.channelId || ''
	if (!channelId) return null
	const params = { type: 'clue' as const, id: props.clueId, worldId: props.worldId, channelId }
	return {
		key: buildInternalSurfaceResourceKey(params),
		url: generateInternalSurfaceLink(params, { base: resolveInternalSurfaceLinkBase(utilsStore.config) }),
		title: item.value?.title || '线索',
		presentation: { chrome: 'minimal' as const, width: 560, height: 460 },
	}
}

async function open(event?: MouseEvent) {
	if (!item.value?.accessible) return
	const resource = getResource()
	if (resource && event) {
		const accepted = await requestTheaterFloatingOpen(resource, event)
		if (accepted) {
			void store.markSeen(props.worldId, props.clueId).catch(() => undefined)
			return
		}
	}
	chatEvent.emit('world-clue-open' as any, { worldId: props.worldId, clueId: props.clueId })
}
async function openFloating(event: MouseEvent) {
	if (!item.value?.accessible) return
	const resource = getResource()
	if (!resource) return
	if (isTheaterChatFrame()) {
		if (await requestTheaterFloatingOpen(resource, event)) {
			void store.markSeen(props.worldId, props.clueId).catch(() => undefined)
		} else message.warning('小剧场浮窗打开失败')
		return
	}
	chatEvent.emit('internal-surface-floating-open' as any, {
		resource,
		clientX: event.clientX,
		clientY: event.clientY,
		onOpened: (accepted: boolean) => {
			if (accepted) void store.markSeen(props.worldId, props.clueId).catch(() => undefined)
		},
	})
}
function edit() {
  if (!item.value?.accessible || item.value.effectiveAccess !== 'edit') return
  chatEvent.emit('world-clue-edit' as any, { worldId: props.worldId, clueId: props.clueId })
}
</script>

<template>
	<article class="clue-message-card" :data-world-clue-link="rawLink">
    <button class="clue-message-card__main" type="button" :disabled="!item?.accessible" @click="open">
    <NSpin v-if="loading" size="small" />
    <template v-else-if="item?.accessible">
      <template v-if="item.kind === 'image' && thumbnail">
        <video v-if="item.mediaType === 'video'" class="clue-message-card__thumb" :src="thumbnail" autoplay muted loop playsinline preload="metadata" />
        <img v-else class="clue-message-card__thumb" :src="thumbnail" alt="" loading="lazy" referrerpolicy="no-referrer" />
      </template>
      <div class="clue-message-card__body">
        <div class="clue-message-card__eyebrow"><NIcon><component :is="icon" /></NIcon><span>{{ item.kind === 'iframe' ? (item.embedDomain || '交互内容') : '线索' }}</span></div>
        <strong>{{ item.title }}</strong>
        <p v-if="item.kind !== 'iframe' && item.excerpt">{{ item.excerpt }}</p>
        <span v-if="item.kind === 'iframe'" class="clue-message-card__action">打开交互内容</span>
        <span v-if="item.hasPrivateContent" class="clue-message-card__private">含仅你可见信息</span>
        <time v-if="updatedLabel" class="clue-message-card__time" :datetime="updatedISO">{{ updatedLabel }}</time>
      </div>
    </template>
    <span v-else class="clue-message-card__unavailable">线索不可用</span>
    </button>
    <div v-if="item?.accessible" class="clue-message-card__actions">
      <NButton circle quaternary size="small" title="悬浮打开" aria-label="悬浮打开" @click.stop="openFloating"><template #icon><NIcon><PictureInPicture /></NIcon></template></NButton>
      <NButton circle quaternary size="small" title="打开线索" aria-label="打开线索" @click.stop="open"><template #icon><NIcon><ExternalLink /></NIcon></template></NButton>
      <NButton v-if="item.effectiveAccess === 'edit'" circle quaternary size="small" title="编辑线索" aria-label="编辑线索" @click.stop="edit"><template #icon><NIcon><Edit /></NIcon></template></NButton>
    </div>
  </article>
</template>

<style scoped>
.clue-message-card { position: relative; width: min(440px, 100%); overflow: hidden; color: var(--sc-text-primary); border: 1px solid var(--sc-border-mute); border-radius: 6px; background: color-mix(in srgb, var(--sc-bg-elevated) 94%, transparent); }
.clue-message-card__main { display: grid; width: 100%; min-height: 92px; grid-template-columns: auto minmax(0, 1fr); padding: 0; border: 0; background: transparent; color: inherit; font: inherit; text-align: left; cursor: pointer; }
.clue-message-card__main:disabled { grid-template-columns: 1fr; cursor: default; }
.clue-message-card__main:not(:has(.clue-message-card__thumb)) { grid-template-columns: 1fr; }
.clue-message-card__actions { position: absolute; top: 5px; right: 5px; z-index: 2; display: flex; gap: 1px; padding: 2px; border: 1px solid color-mix(in srgb, var(--sc-border-mute) 74%, transparent); border-radius: 6px; background: color-mix(in srgb, var(--sc-bg-elevated) 78%, transparent); opacity: .35; transition: opacity .15s; backdrop-filter: blur(8px); }
.clue-message-card:hover .clue-message-card__actions, .clue-message-card:focus-within .clue-message-card__actions { opacity: 1; }
@media (hover: none), (max-width: 760px) { .clue-message-card__actions { opacity: 1; } }
.clue-message-card__actions :deep(.n-button) { width: 24px; height: 24px; }
.clue-message-card__thumb { width: 112px; height: 100%; min-height: 92px; object-fit: cover; }
.clue-message-card__body { min-width: 0; padding: 38px 14px 12px; }
.clue-message-card:has(.clue-message-card__thumb) .clue-message-card__body { padding-top: 12px; }
.clue-message-card__body strong { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.clue-message-card__body p { display: -webkit-box; margin: 6px 0 0; overflow: hidden; color: var(--sc-text-secondary); font-size: 13px; line-height: 1.45; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
.clue-message-card__eyebrow { display: flex; align-items: center; gap: 5px; margin-bottom: 4px; color: var(--primary-color, #3388de); font-size: 11px; }
.clue-message-card__action, .clue-message-card__private { display: inline-block; margin-top: 7px; color: var(--primary-color, #3388de); font-size: 12px; }
.clue-message-card__private { margin-left: 10px; color: var(--sc-text-secondary); }
.clue-message-card__time { display: block; margin-top: 7px; color: var(--sc-text-secondary); font-size: 11px; }
.clue-message-card__unavailable { padding: 18px; color: var(--sc-text-secondary); }
</style>
