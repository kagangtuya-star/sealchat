<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, onBeforeUnmount, watch } from 'vue'
import { useUserStore } from '@/stores/user'
import { useChatStore } from '@/stores/chat'
import { useSpeechStore } from './store'
import { speechPlayer } from './player'
import { speechAPI } from './api'
import { api } from '@/stores/_config'
import { SpeechQueue, type SpeechEnd } from './runtime'

const SpeechPanel = defineAsyncComponent(() => import('./SpeechPanel.vue'))
const speech = useSpeechStore()
const user = useUserStore()
const chat = useChatStore()
const mainWindow = window.parent === window
const currentChannel = computed(() => mainWindow ? speech.scopeChannel || chat.curChannel?.id : chat.curChannel?.id)
const waitingStatuses = new Set(['pending', 'queued', 'running', 'storage_pending', 'archiving'])
const reconnectingError = '语音订阅中断，正在重新连接。'
const playbackIdle = () => speechPlayer.state.automatic && !speechPlayer.state.loading && !speechPlayer.state.playing
let subscription: { drain(): void; claim(key: string): void } | undefined
// One subscription per (preference, channel, account). It owns the only socket,
// reconnect timer and playback queue; cleanup makes every late callback inert.
// Immediate: a page that mounts with all three ready must subscribe at once.
watch([() => speechPlayer.state.preferred, currentChannel, () => user.info.id], ([preferred, channelId, userId], _, cleanup) => {
  if (!mainWindow || !preferred || !channelId || !userId) return
  const channel = channelId
  let active = true
  let socket: WebSocket | undefined
  let reconnectTimer: ReturnType<typeof setTimeout> | undefined
  let recheckTimer: ReturnType<typeof setTimeout> | undefined
  let failures = 0
  let draining = false
  const queue = new SpeechQueue()
  const idle = playbackIdle

  function begin(messageId: string, live?: Parameters<typeof speechPlayer.startPCM>[0]) {
    queue.begin(messageId)
    const onEnd = (result: SpeechEnd) => {
      if (!active) return
      queue.end(messageId, result, !!live)
      void drain()
    }
    if (live) void speechPlayer.startPCM(live, onEnd)
    else void speechPlayer.play('messages', messageId, true, onEnd)
  }
  // Plays owed messages strictly in order from their archived files.
  async function drain() {
    if (!active || draining) return
    draining = true
    try {
      while (active && idle() && !queue.current && queue.pending.length > 0) {
        const messageId = queue.pending[0]
        let state
        try {
          state = await speechAPI.message(messageId)
        } catch {
          recheck()
          return
        }
        if (!active) return
        speech.messageStates[messageId] = state
        if (queue.pending[0] !== messageId) continue
        if (state?.status === 'ready') {
          if (idle() && !queue.current) begin(messageId)
          return
        }
        // Normally the archive's start frame arrives first; the slow recheck
        // only covers states that are never broadcast (e.g. storage recovery).
        if (state && waitingStatuses.has(state.status)) {
          recheck()
          return
        }
        queue.dismiss(messageId)
      }
    } finally {
      draining = false
    }
  }
  function recheck() {
    if (!active || recheckTimer) return
    recheckTimer = setTimeout(() => { recheckTimer = undefined; void drain() }, 3000)
  }
  subscription = {
    drain() { void drain() },
    // A message the user replays by hand while it is still owed counts as heard.
    claim(key) {
      const messageId = key.startsWith('messages:') ? key.slice('messages:'.length) : ''
      if (messageId && messageId !== queue.current) queue.dismiss(messageId)
    },
  }

  function onFrame(value: any) {
    if (!Number.isSafeInteger(value.epoch) || value.epoch < 0) return
    if (value.type === 'start' && typeof value.messageId === 'string') {
      const action = queue.start(value.epoch, value.messageId, idle())
      if (action === 'play') begin(value.messageId, value.mode === 'pcm' && value.media ? value : undefined)
      else if (action === 'queue') void drain()
      return
    }
    if (value.type === 'end') { speechPlayer.endPCM(value.epoch); return }
    const messageId = queue.message(value.epoch)
    if (!messageId || !['cancel', 'desynced'].includes(value.type)) return
    const playing = speechPlayer.state.key === `messages:${messageId}`
    // Cancel means skipped/stopped/invalidated: never play it here. A server
    // desync only breaks this realtime attempt; the archive stays owed.
    if (value.type === 'cancel') {
      queue.dismiss(messageId)
      if (playing) speechPlayer.stop()
    } else if (playing) speechPlayer.stop('failed')
  }
  // Recovery after (re)connect: messages still in the server queue, in server
  // order. It never replays archives this page already settled.
  async function recover() {
    try {
      const result = await speechAPI.queue(channel)
      if (!active) return
      for (const item of result.items) queue.offer(item.messageId)
      void drain()
    } catch { /* The next reconnect retries recovery; live frames still arrive. */ }
  }
  function reconnect() {
    if (!active || reconnectTimer) return
    speechPlayer.state.error = reconnectingError
    reconnectTimer = setTimeout(connect, Math.min(1000 * 2 ** failures++, 15000))
  }
  function connect() {
    reconnectTimer = undefined
    // Tickets are single-use and short-lived: every attempt requests a new one.
    speechAPI.wsTicket(channel).then(ticket => {
      if (!active) return
      const base = new URL(api.defaults.baseURL || '/', window.location.href)
      const url = new URL(ticket.path, base)
      url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
      url.searchParams.set('ticket', ticket.ticket)
      url.searchParams.set('mode', 'pcm')
      const ws = new WebSocket(url)
      let openedAt = 0
      socket = ws
      ws.binaryType = 'arraybuffer'
      ws.onopen = () => {
        if (!active || socket !== ws) return
        openedAt = Date.now()
        queue.resetEpochs()
        if (speechPlayer.state.error === reconnectingError) speechPlayer.state.error = ''
        void recover()
      }
      ws.onmessage = event => {
        if (!active || socket !== ws) return
        if (event.data instanceof ArrayBuffer) { speechPlayer.pcmPacket(event.data); return }
        if (typeof event.data !== 'string') return
        try { onFrame(JSON.parse(event.data)) } catch { /* Invalid control frames never trigger a charged operation. */ }
      }
      // An error is always followed by close; close alone schedules the retry.
      ws.onclose = () => {
        if (!active || socket !== ws) return
        socket = undefined
        // A server that accepts and closes at once (e.g. connection limit) keeps backing off.
        if (openedAt && Date.now() - openedAt > 10000) failures = 0
        reconnect()
      }
    }).catch(() => reconnect())
  }
  cleanup(() => {
    active = false
    subscription = undefined
    clearTimeout(reconnectTimer)
    clearTimeout(recheckTimer)
    const ws = socket
    socket = undefined
    ws?.close()
    speechPlayer.stop()
  })
  connect()
}, { flush: 'sync', immediate: true })
// Unlock or the end of any playback (including manual replay) resumes owed messages.
watch(playbackIdle, value => { if (value) subscription?.drain() })
watch(() => speechPlayer.state.key, key => { if (key) subscription?.claim(key) }, { flush: 'sync' })
function onIntent(event: MessageEvent) {
  if (event.origin !== window.location.origin) return
  const value = event.data
  if (!value || value.userId !== user.info.id || !user.info.id) return
  if (!mainWindow) {
    if (event.source !== window.parent || value.type !== 'sealchat:tts-state') return
    if (value.temporary && typeof value.temporary === 'object') {
      const entries = Object.entries(value.temporary).filter(([key, enabled]) => /^[A-Za-z0-9_-]{1,100}$/.test(key) && typeof enabled === 'boolean')
      speech.temporary = Object.fromEntries(entries.slice(0, 512)) as Record<string, boolean>
    }
    if (speech.quota && typeof value.autoSynthesis === 'boolean') {
      speech.quota.autoSynthesis = value.autoSynthesis
    }
    if (typeof value.playingKey === 'string' && typeof value.loading === 'boolean' && typeof value.playing === 'boolean') {
      speechPlayer.state.key = value.playingKey
      speechPlayer.state.loading = value.loading
      speechPlayer.state.playing = value.playing
    }
    return
  }
  if (!Array.from(document.querySelectorAll('iframe')).some(frame => frame.contentWindow === event.source)) return
  if (value.type !== 'sealchat:tts-intent') return
  // Embedded chats forward their first gesture; never flip the stored preference from here.
  if (value.action === 'resume') {
    if (speechPlayer.state.preferred && !speechPlayer.state.automatic) void speechPlayer.resumePreferred()
    return
  }
  if (value.action === 'open') speech.open(typeof value.channelId === 'string' ? value.channelId : '')
  if (value.action === 'state') broadcastState()
  if (value.action === 'temporary' && typeof value.channelId === 'string' && /^[A-Za-z0-9_-]{1,100}$/.test(value.channelId) && typeof value.enabled === 'boolean') speech.setTemporary(value.channelId, value.enabled)
  if (value.action === 'play' && (value.kind === 'messages' || value.kind === 'resources') && typeof value.id === 'string' && value.id.length <= 100) {
    void speechPlayer.play(value.kind, value.id)
  }
}
function broadcastState() {
  if (!mainWindow) return
  const state = { type: 'sealchat:tts-state', userId: user.info.id, temporary: { ...speech.temporary }, enabled: speech.quota?.enabled, autoSynthesis: speech.quota?.autoSynthesis, playingKey: speechPlayer.state.key, loading: speechPlayer.state.loading, playing: speechPlayer.state.playing }
  for (const frame of document.querySelectorAll('iframe')) frame.contentWindow?.postMessage(state, window.location.origin)
}
watch(() => [JSON.stringify(speech.temporary), speech.quota?.enabled, speech.quota?.autoSynthesis, speechPlayer.state.key, speechPlayer.state.loading, speechPlayer.state.playing], broadcastState)
// The first page gesture satisfies autoplay policy; resume the remembered preference silently.
function resumePreferredPlayback() {
  if (!user.info.id) return
  if (mainWindow) {
    if (!speechPlayer.state.preferred) return
    if (speechPlayer.state.automatic) return
    void speechPlayer.resumePreferred()
    return
  }
  // Iframe gestures never reach the parent DOM; ask the main-window player to resume instead.
  window.parent.postMessage({ type: 'sealchat:tts-intent', action: 'resume', userId: user.info.id }, window.location.origin)
}
onMounted(() => {
  window.addEventListener('message', onIntent)
  window.addEventListener('pointerdown', resumePreferredPlayback, true)
  window.addEventListener('keydown', resumePreferredPlayback, true)
})
onBeforeUnmount(() => {
  window.removeEventListener('message', onIntent)
  window.removeEventListener('pointerdown', resumePreferredPlayback, true)
  window.removeEventListener('keydown', resumePreferredPlayback, true)
  if (mainWindow) speechPlayer.stop()
})
watch(() => user.info.id, (id) => {
  speech.reset()
  speechPlayer.state.automatic = false
  if (!mainWindow && id === user.info.id) window.parent.postMessage({ type: 'sealchat:tts-intent', action: 'state', userId: id }, window.location.origin)
}, { immediate: true, flush: 'sync' })
watch(() => chat.curChannel?.id, channelId => { if (mainWindow && channelId) speech.scopeChannel = channelId }, { immediate: true })
watch([currentChannel, () => user.info.id], ([channelId, userId]) => {
  if (userId) void speech.refresh(channelId || '').catch(() => { /* Failed capabilities stay unavailable. */ })
}, { immediate: true, flush: 'sync' })
watch([currentChannel, () => user.info.id], ([channelId, userId], _, cleanup) => {
  speechPlayer.stop()
  speech.messageStates = {}
  let active = true
  let timer: ReturnType<typeof setTimeout> | undefined
  async function poll() {
    if (!channelId || !userId || !active) return
    try {
      const items = await speechAPI.states(channelId)
      if (!active) return
      for (const item of items) speech.messageStates[item.id] = item.tts
    } catch { /* Silent metadata refresh; explicit playback reports authorization failures. */ }
    if (active) timer = setTimeout(poll, 3000)
  }
  void poll()
  cleanup(() => { active = false; clearTimeout(timer) })
}, { immediate: true })
</script>

<template>
  <SpeechPanel v-if="mainWindow && speech.visible && speech.quota?.enabled" />
</template>
