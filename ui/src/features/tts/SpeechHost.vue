<script setup lang="ts">
import { computed, defineAsyncComponent, onMounted, onBeforeUnmount, watch } from 'vue'
import { useUserStore } from '@/stores/user'
import { useChatStore } from '@/stores/chat'
import { useSpeechStore } from './store'
import { speechPlayer } from './player'
import { speechAPI } from './api'
import { api } from '@/stores/_config'
import { SpeechPlaybackOrder } from './runtime'

const SpeechPanel = defineAsyncComponent(() => import('./SpeechPanel.vue'))
const speech = useSpeechStore()
const user = useUserStore()
const chat = useChatStore()
const mainWindow = window.parent === window
const currentChannel = computed(() => mainWindow ? speech.scopeChannel || chat.curChannel?.id : chat.curChannel?.id)
watch([() => speechPlayer.state.automatic, currentChannel, () => user.info.id], ([enabled, channelId, userId], _, cleanup) => {
  let active = true
  let socket: WebSocket | undefined
  const order = new SpeechPlaybackOrder()
  cleanup(() => { active = false; socket?.close(); speechPlayer.stop() })
  if (!mainWindow || !enabled || !channelId || !userId) return
  void speechAPI.wsTicket(channelId).then(ticket => {
    if (!active) return
    const base = new URL(api.defaults.baseURL || '/', window.location.href)
    const url = new URL(ticket.path, base)
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
    url.searchParams.set('ticket', ticket.ticket)
    url.searchParams.set('mode', 'pcm')
    socket = new WebSocket(url)
    socket.binaryType = 'arraybuffer'
    socket.onmessage = event => {
      if (!active) return
      if (event.data instanceof ArrayBuffer) { speechPlayer.pcmPacket(event.data); return }
      if (typeof event.data !== 'string') return
      try {
        const value = JSON.parse(event.data)
        if (!Number.isSafeInteger(value.epoch) || value.epoch < 0) return
        if (value.type === 'start' && typeof value.messageId === 'string') {
          const action = order.start(value.epoch, speechPlayer.state.loading || speechPlayer.state.playing)
          if (action === 'play') {
            if (value.mode === 'pcm' && value.media) void speechPlayer.startPCM(value)
            else void speechPlayer.play('messages', value.messageId, true)
          }
          if (action === 'skip') speechPlayer.state.error = '本地播放尚未结束，已跳过赶不上的语音；可稍后点击消息重听。'
        } else if (order.cancel(value.epoch) && value.type === 'end') speechPlayer.endPCM(value.epoch)
        else if (order.cancel(value.epoch) && ['cancel', 'desynced'].includes(value.type)) speechPlayer.stop()
      } catch { /* Invalid control frames never trigger a charged operation. */ }
    }
    socket.onerror = () => { if (active) speechPlayer.state.error = '语音订阅中断，请重新开启自动播放。' }
  }).catch(() => { if (active) speechPlayer.state.error = '无法订阅当前频道语音。' })
}, { flush: 'sync' })
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
    if (speech.quota && typeof value.enabled === 'boolean' && typeof value.autoSynthesis === 'boolean') {
      speech.quota.enabled = value.enabled
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
watch(() => user.info.id, async (id) => {
  speech.reset()
  speechPlayer.state.automatic = false
  if (id) { try { await speech.refresh() } catch { /* Panel displays request failures locally. */ } }
  if (!mainWindow && id === user.info.id) window.parent.postMessage({ type: 'sealchat:tts-intent', action: 'state', userId: id }, window.location.origin)
}, { immediate: true })
watch(() => chat.curChannel?.id, channelId => { if (mainWindow && channelId) speech.scopeChannel = channelId }, { immediate: true })
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
  <SpeechPanel v-if="mainWindow && speech.visible" />
</template>
