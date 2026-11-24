<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { useMessage } from 'naive-ui'
import { nanoid } from 'nanoid'
import { chatEvent, useChatStore } from '@/stores/chat'
import { resolveTheaterChatEventChannelId, serializeTheaterDialogueMessage } from '@/bridge/sealchatBridgeSerializer'
import type { TheaterDialogueMessage } from '@/views/theater/bridge/theater-dialogue-queue'
import { theaterEffectConfigFromObject } from '@/views/theater/effects/theater-effect-types'
import { normalizeWorldPresentationConfig } from './world-presentation-config'
import { readWorldPresentation } from './world-presentation-api'
import { canPlayWorldSequence, createWorldPresentationAuthority, type WorldPresentationLiveControl, type WorldPresentationTarget } from './world-presentation-runtime'
import { useWorldPresentationPlayback } from './useWorldPresentationPlayback'
import WorldPresentationOverlay from './WorldPresentationOverlay.vue'
import WorldPresentationConsoleModal from './WorldPresentationConsoleModal.vue'

// The chat page keys this host by user, world and channel.
const props = defineProps<{ worldId: string, channelId: string, canManage: boolean }>()
const chat = useChatStore()
const message = useMessage()
const scope = { worldId: props.worldId, channelId: props.channelId }
const lifetime = new AbortController()
const settings = shallowRef<Awaited<ReturnType<typeof readWorldPresentation>>>()
const config = computed(() => settings.value?.config ?? normalizeWorldPresentationConfig(null))
const { playbacks, play } = useWorldPresentationPlayback(() => config.value)
const showConsole = ref(false)
const failed = ref(false)
const effects = computed(() => config.value.effects.filter((effect) => effect.visible))
const sequences = computed(() => config.value.sequences.filter((sequence) => canPlayWorldSequence(sequence, config.value)))
const hasAutomaticTriggers = computed(() => effects.value.some((effect) => theaterEffectConfigFromObject(effect).keywords.length)
  || sequences.value.some((sequence) => sequence.triggers.some((trigger) => trigger.type === 'message' && trigger.keywords.length)))
const readyToControl = computed(() => props.canManage && hasAutomaticTriggers.value)

let loading: Promise<void> | undefined
let notifiedRevision = ''
let authority: ReturnType<typeof createWorldPresentationAuthority> | undefined
let authorityScope = ''
const pendingAutomaticMessages: Array<{ controller: string, source: TheaterDialogueMessage }> = []
const stopAuthority = () => {
  authority?.dispose()
  authority = undefined
  authorityScope = ''
}

const refresh = () => {
  if (loading) return loading
  loading = (async () => {
    do {
      const requestedRevision = notifiedRevision
      try {
        const result = await readWorldPresentation(scope.worldId, lifetime.signal)
        if (lifetime.signal.aborted) return
        if (requestedRevision !== notifiedRevision && result.revision !== notifiedRevision) continue
        notifiedRevision = ''
        settings.value = result
        failed.value = false
        flushPendingAutomaticMessages()
        if (readyToControl.value) registerControl(true)
      } catch {
        if (!lifetime.signal.aborted) {
          failed.value = true
          pendingAutomaticMessages.length = 0
          stopAuthority()
          registerControl(false)
        }
      }
      return
    } while (!lifetime.signal.aborted)
  })().finally(() => { loading = undefined })
  return loading
}

const registerControl = (enabled: boolean) => {
  if (chat.connectState !== 'connected') return
  void chat.sendAPI('world.presentation.control', { ...scope, enabled }).catch(() => {})
}
watch(readyToControl, (enabled) => {
  if (!enabled) stopAuthority()
  registerControl(enabled)
})
watch(() => chat.connectState, (state) => {
  if (state !== 'connected') stopAuthority()
})

const onChannelEnter = async (event?: { argv?: { channelId?: string } }) => {
  if (event?.argv?.channelId !== scope.channelId) return
  // Reconnect can miss a settings update while the socket was offline.
  await refresh()
  if (!lifetime.signal.aborted && !failed.value && readyToControl.value) registerControl(true)
}

const sendPlay = (kind: WorldPresentationTarget, targetId: string, source: { messageId: string, controller: string } | { requestId: string }, revision: string) => (
  chat.sendAPI('world.presentation.play', { ...scope, kind, targetId, revision, ...source })
)

const receiveAutomaticMessage = (controller: string, source: TheaterDialogueMessage) => {
  const revision = settings.value?.revision
  if (!revision) return
  const scopeKey = `${controller}:${revision}`
  if (authorityScope !== scopeKey) {
    stopAuthority()
    authorityScope = scopeKey
    authority = createWorldPresentationAuthority(() => config.value, (kind, targetId, messageId) => {
      void sendPlay(kind, targetId, { messageId, controller }, revision).catch(() => {})
    })
  }
  authority?.receive(source)
}

function flushPendingAutomaticMessages() {
  const pending = pendingAutomaticMessages.splice(0)
  pending.forEach(({ controller, source }) => receiveAutomaticMessage(controller, source))
}

const onMessage = (event: { worldPresentationController?: string, message?: unknown }) => {
  const controller = event.worldPresentationController
  if (!controller || !readyToControl.value || resolveTheaterChatEventChannelId(event) !== scope.channelId) return
  const revision = settings.value?.revision
  if (!revision) return
  const source = serializeTheaterDialogueMessage(event.message)
  if (!source || source.isWhisper || source.isArchived || source.isDeleted) return
  if (notifiedRevision && notifiedRevision !== revision) {
    pendingAutomaticMessages.push({ controller, source })
    if (pendingAutomaticMessages.length > 64) pendingAutomaticMessages.shift()
    return
  }
  receiveAutomaticMessage(controller, source)
}

const onConfigChanged = (event: { argv?: { Options?: { worldId?: string, revision?: string } } }) => {
  const change = event.argv?.Options
  if (change?.worldId !== scope.worldId || !change.revision || change.revision === settings.value?.revision) return
  notifiedRevision = change.revision
  void refresh()
}

type PlaybackEvent = {
  worldPresentationPlayback?: {
    worldId: string
    channelId: string
    kind: WorldPresentationTarget
    targetId: string
    executionId: string
    revision: string
  }
}

const onPlayback = async ({ worldPresentationPlayback: payload }: PlaybackEvent) => {
  if (payload?.worldId !== scope.worldId || payload.channelId !== scope.channelId || !payload.executionId) return
  if (payload.kind !== 'sequence' && payload.kind !== 'effect') return
  if (payload.revision !== settings.value?.revision) await refresh()
  if (!lifetime.signal.aborted && payload.revision === settings.value?.revision) {
    void play(payload.kind, payload.targetId, payload.executionId)
  }
}

const manualPlay = async (kind: WorldPresentationTarget, id: string) => {
  if (!props.canManage || !settings.value) return false
  try {
    await sendPlay(kind, id, { requestId: nanoid() }, settings.value.revision)
    return true
  } catch {
    if (!lifetime.signal.aborted) message.error('播放失败，请确认频道连接和世界演出设置')
    return false
  }
}

const liveControl = computed<WorldPresentationLiveControl>(() => ({
  config: settings.value?.config ?? null,
  failed: failed.value,
  play: manualPlay,
  retry: () => { void refresh() },
}))

// The console's own save is broadcast too; refresh now so live control never lags behind it.
const onConsoleSaved = (revision: string) => {
  if (revision === settings.value?.revision) return
  notifiedRevision = revision
  void refresh()
}

const open = () => {
  if (!props.canManage) return
  showConsole.value = true
  if (!settings.value || failed.value) void refresh()
}
defineExpose({ open, isOpen: showConsole })
chatEvent.on('message-created', onMessage)
chatEvent.on('world-presentation-updated', onConfigChanged)
chatEvent.on('world-presentation-play', onPlayback)
chatEvent.on('channel-switch-to', onChannelEnter)
void refresh()

onBeforeUnmount(() => {
  if (readyToControl.value) registerControl(false)
  lifetime.abort()
  stopAuthority()
  pendingAutomaticMessages.length = 0
  chatEvent.off('message-created', onMessage)
  chatEvent.off('world-presentation-updated', onConfigChanged)
  chatEvent.off('world-presentation-play', onPlayback)
  chatEvent.off('channel-switch-to', onChannelEnter)
})
</script>

<template>
  <!-- Above the open console so the admin sees what they play live. -->
  <WorldPresentationOverlay :world-id="worldId" :playbacks="playbacks" :elevated="showConsole" />
  <WorldPresentationConsoleModal
    v-if="canManage"
    v-model:show="showConsole"
    :world-id="worldId"
    :live="liveControl"
    @saved="onConsoleSaved"
  />
</template>
