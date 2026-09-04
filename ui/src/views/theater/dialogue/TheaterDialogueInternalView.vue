<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import type { ChatCharactersSnapshotPayload } from '../bridge/theater-bridge-protocol'
import { createTheaterDialogueQueueState } from '../bridge/theater-dialogue-queue'
import TheaterDialogueOverlay from './TheaterDialogueOverlay.vue'
import type {
  TheaterDialogueRuntimeController,
  TheaterDialogueRuntimeSnapshot,
} from './theater-dialogue-runtime'
import {
  THEATER_DIALOGUE_SURFACE_MESSAGE_TYPES,
  isTheaterDialogueSurfaceCharactersMessage,
  isTheaterDialogueSurfaceRuntimeMessage,
  type TheaterDialogueSurfaceCommand,
  type TheaterDialogueSurfaceContext,
} from './theater-dialogue-surface'

const normalizeRouteValue = (value: unknown) => typeof value === 'string' ? value.trim() : ''
const route = useRoute()
const context = reactive<TheaterDialogueSurfaceContext>({
  identityId: normalizeRouteValue(route.params.identityId),
  worldId: normalizeRouteValue(route.query.world),
  channelId: normalizeRouteValue(route.query.channel),
})
const hasContext = computed(() => Boolean(context.identityId && context.worldId && context.channelId))
const documentClass = 'theater-dialogue-surface-document'
document.documentElement.classList.add(documentClass)

const characterSnapshot = ref<ChatCharactersSnapshotPayload>({
  revision: 0,
  updatedAt: 0,
  activeIdentityId: null,
  characters: [],
})
let sessionId = ''
let mounted = false

const initialRuntimeSnapshot = (): TheaterDialogueRuntimeSnapshot => ({
  queue: createTheaterDialogueQueueState(),
  phase: 'idle',
  reducedMotion: false,
})

class DialogueSurfaceRuntimeAdapter implements TheaterDialogueRuntimeController {
  private snapshot = initialRuntimeSnapshot()
  private reducedMotionValue: boolean | null = null
  private readonly listeners = new Set<(snapshot: TheaterDialogueRuntimeSnapshot) => void>()

  getSnapshot = () => this.filteredSnapshot()

  subscribe = (listener: (snapshot: TheaterDialogueRuntimeSnapshot) => void) => {
    this.listeners.add(listener)
    listener(this.getSnapshot())
    return () => { this.listeners.delete(listener) }
  }

  update = (snapshot: TheaterDialogueRuntimeSnapshot) => {
    const ownedBefore = this.ownsCurrent()
    this.snapshot = snapshot
    const ownedNow = this.ownsCurrent()
    if (!ownedBefore && ownedNow && this.reducedMotionValue !== null) {
      sendCommand({ name: 'set-reduced-motion', value: this.reducedMotionValue })
    }
    const filtered = this.filteredSnapshot()
    this.listeners.forEach(listener => listener(filtered))
  }

  completeCurrent = (messageId?: string) => {
    if (this.ownsCurrent()) sendCommand({ name: 'complete-current', ...(messageId ? { messageId } : {}) })
  }

  skip = () => {
    if (this.ownsCurrent()) sendCommand({ name: 'skip' })
  }

  close = () => {
    if (this.ownsCurrent()) sendCommand({ name: 'close' })
  }

  setReducedMotion = (value: boolean) => {
    this.reducedMotionValue = value
    if (this.ownsCurrent()) sendCommand({ name: 'set-reduced-motion', value })
  }

  setCharactersPerSecond = (value: number) => {
    if (this.ownsCurrent()) sendCommand({ name: 'set-characters-per-second', value })
  }

  private ownsCurrent() {
    return this.snapshot.queue.current?.message.actor.identityId === context.identityId
  }

  private filteredSnapshot(): TheaterDialogueRuntimeSnapshot {
    if (this.ownsCurrent()) return this.snapshot
    return {
      ...this.snapshot,
      queue: { ...this.snapshot.queue, current: null },
      phase: 'idle',
    }
  }
}

const runtime = new DialogueSurfaceRuntimeAdapter()

const messageMatchesContext = (message: TheaterDialogueSurfaceContext) => (
  message.identityId === context.identityId
  && message.worldId === context.worldId
  && message.channelId === context.channelId
)

const sendSurfaceMessage = (message: Record<string, unknown>) => {
  if (!hasContext.value || window.parent === window) return
  window.parent.postMessage({ ...context, ...message }, window.location.origin)
}

const sendSurfaceMessageFor = (targetContext: TheaterDialogueSurfaceContext, message: Record<string, unknown>) => {
  if (!targetContext.identityId || !targetContext.worldId || !targetContext.channelId || window.parent === window) return
  window.parent.postMessage({ ...targetContext, ...message }, window.location.origin)
}

const sendCommand = (command: TheaterDialogueSurfaceCommand) => {
  if (!sessionId) return
  sendSurfaceMessage({
    type: THEATER_DIALOGUE_SURFACE_MESSAGE_TYPES.command,
    sessionId,
    command,
  })
}

const handleParentMessage = (event: MessageEvent) => {
  if (event.origin !== window.location.origin || event.source !== window.parent) return
  if (isTheaterDialogueSurfaceRuntimeMessage(event.data)) {
    if (!messageMatchesContext(event.data) || (sessionId && event.data.sessionId !== sessionId)) return
    sessionId = event.data.sessionId
    runtime.update(event.data.snapshot)
    return
  }
  if (!isTheaterDialogueSurfaceCharactersMessage(event.data)) return
  if (!messageMatchesContext(event.data) || (sessionId && event.data.sessionId !== sessionId)) return
  sessionId = event.data.sessionId
  characterSnapshot.value = event.data.snapshot
}

const sendDispose = () => sendSurfaceMessage({ type: THEATER_DIALOGUE_SURFACE_MESSAGE_TYPES.dispose })

watch(() => route.fullPath, () => {
  const previousContext = { ...context }
  if (mounted) sendSurfaceMessageFor(previousContext, { type: THEATER_DIALOGUE_SURFACE_MESSAGE_TYPES.dispose })
  context.identityId = normalizeRouteValue(route.params.identityId)
  context.worldId = normalizeRouteValue(route.query.world)
  context.channelId = normalizeRouteValue(route.query.channel)
  sessionId = ''
  characterSnapshot.value = {
    revision: 0,
    updatedAt: 0,
    activeIdentityId: null,
    characters: [],
  }
  runtime.update(initialRuntimeSnapshot())
  if (mounted) void nextTick(() => sendSurfaceMessage({ type: THEATER_DIALOGUE_SURFACE_MESSAGE_TYPES.ready }))
})

onMounted(() => {
  mounted = true
  window.addEventListener('message', handleParentMessage)
  window.addEventListener('pagehide', sendDispose)
  sendSurfaceMessage({ type: THEATER_DIALOGUE_SURFACE_MESSAGE_TYPES.ready })
})

onBeforeUnmount(() => {
  mounted = false
  sendDispose()
  window.removeEventListener('message', handleParentMessage)
  window.removeEventListener('pagehide', sendDispose)
  document.documentElement.classList.remove(documentClass)
})
</script>

<template>
  <main
    class="theater-dialogue-internal-view"
    :data-rich-message-world-id="context.worldId || undefined"
    :data-rich-message-channel-id="context.channelId || undefined"
  >
    <TheaterDialogueOverlay
      v-if="hasContext"
      :runtime="runtime"
      :character-snapshot="characterSnapshot"
      :world-id="context.worldId"
      :channel-id="context.channelId"
      fill-container
    />
  </main>
</template>

<style>
html.theater-dialogue-surface-document,
html.theater-dialogue-surface-document body,
html.theater-dialogue-surface-document #app {
  margin: 0;
  background: transparent !important;
}
</style>

<style scoped>
.theater-dialogue-internal-view {
  position: relative;
  width: 100vw;
  height: 100vh;
  overflow: hidden;
  background: transparent;
}
</style>
