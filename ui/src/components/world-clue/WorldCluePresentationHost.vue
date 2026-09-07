<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { chatEvent } from '@/stores/chat'
import { useWorldClueStore, type WorldClueDetail, type WorldCluePresentationRequest } from '@/stores/worldClue'
import WorldCluePresentationOverlay from './WorldCluePresentationOverlay.vue'

const route = useRoute()
const store = useWorldClueStore()
const active = ref<{ request: WorldCluePresentationRequest; clue: WorldClueDetail } | null>(null)
const loading = ref(false)
let loadEpoch = 0
const currentWorldId = computed(() => String(route.params.worldId || route.query.worldId || route.query.scopeWorldId || '').trim())

function normalizePayload(event: any) {
  const direct = event?.worldClue
  const options = event?.argv?.options || event?.argv?.Options
  return (direct || options || {}) as { worldId?: string; clueId?: string; publishSeq?: number; action?: string }
}

const handlePublished = (event: any) => {
  const payload = normalizePayload(event)
  const request = { worldId: String(payload.worldId || ''), clueId: String(payload.clueId || ''), publishSeq: Number(payload.publishSeq || 0) }
  if (window.parent !== window) {
    window.parent.postMessage({ type: 'sealchat.world-clue.presentation', ...request }, window.location.origin)
    return
  }
  store.enqueuePresentation(request)
}

const handleChanged = async (event: any) => {
  const payload = normalizePayload(event)
  await store.handleChanged(payload).catch(() => undefined)
  if (payload.action === 'edit-lock') return
  if (!active.value || active.value.request.worldId !== payload.worldId || active.value.request.clueId !== payload.clueId) return
  const current = active.value
  try {
    const clue = await store.fetchDetail(current.request.worldId, current.request.clueId, false)
    if (active.value !== current || current.request.worldId !== currentWorldId.value) return
    if (!current.request.manual && clue.status !== 'published') {
      active.value = null
      void showNext()
      return
    }
    active.value.clue = clue
  } catch {
    if (active.value === current) {
      active.value = null
      void showNext()
    }
  }
}
const handleOpen = async (payload: any) => {
  const worldId = String(payload?.worldId || '')
  const clueId = String(payload?.clueId || '')
  if (!worldId || !clueId || worldId !== currentWorldId.value) return
  try {
    const clue = await store.fetchDetail(worldId, clueId)
    if (worldId !== currentWorldId.value) return
    active.value = { request: { worldId, clueId, publishSeq: Math.max(1, clue.publishSeq), manual: true }, clue }
  } catch {
    // The live reference became inaccessible.
  }
}

const handleMessage = (event: MessageEvent) => {
  if (event.origin !== window.location.origin || event.data?.type !== 'sealchat.world-clue.presentation') return
  const worldId = typeof event.data.worldId === 'string' ? event.data.worldId : ''
  const clueId = typeof event.data.clueId === 'string' ? event.data.clueId : ''
  const publishSeq = Number(event.data.publishSeq)
  if (!worldId || !clueId || !Number.isSafeInteger(publishSeq) || publishSeq <= 0) return
  store.enqueuePresentation({ worldId, clueId, publishSeq })
}

async function showNext(): Promise<void> {
  if (active.value || loading.value || !store.presentationQueue.length) return
  const request = store.presentationQueue.shift()!
  if (request.worldId !== currentWorldId.value) return void showNext()
  const epoch = ++loadEpoch
  loading.value = true
  try {
    const clue = await store.fetchDetail(request.worldId, request.clueId, false)
    if (request.worldId !== currentWorldId.value) return
    if (clue.status === 'published' && clue.publishSeq >= request.publishSeq) active.value = { request, clue }
  } catch {
    // Permission was revoked or the clue no longer exists.
  } finally {
    if (epoch === loadEpoch) {
      loading.value = false
      if (!active.value) void showNext()
    }
  }
}

async function closeActive() {
  const current = active.value
  active.value = null
  if (current && !current.request.manual) {
    await store.acknowledgePresented(current.request).catch(() => undefined)
  }
  void showNext()
}

watch(() => store.presentationQueue.length, () => void showNext())
watch(currentWorldId, worldId => {
  loadEpoch++
  active.value = null
  loading.value = false
  store.setWorld(worldId)
  if (!worldId) return
}, { immediate: true })

onMounted(() => {
  chatEvent.on('world-clue-published' as any, handlePublished as any)
  chatEvent.on('world-clue-changed' as any, handleChanged as any)
  chatEvent.on('world-clue-open' as any, handleOpen as any)
  window.addEventListener('message', handleMessage)
})
onBeforeUnmount(() => {
  chatEvent.off('world-clue-published' as any, handlePublished as any)
  chatEvent.off('world-clue-changed' as any, handleChanged as any)
  chatEvent.off('world-clue-open' as any, handleOpen as any)
  window.removeEventListener('message', handleMessage)
})
</script>

<template><WorldCluePresentationOverlay v-if="active" :clue="active.clue" @close="closeActive" /></template>
