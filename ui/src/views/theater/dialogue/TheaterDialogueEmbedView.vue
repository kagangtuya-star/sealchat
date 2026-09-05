<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { NButton, NCheckbox, NForm, NFormItem, NIcon, NInput, NInputNumber, NSelect } from 'naive-ui'
import { Settings } from '@vicons/tabler'
import { SealChatEmbed, SealChatEmbedError, type ChannelEmbedClient } from '@/bridge/channelEmbedSdk'
import { listPlatformFonts } from '@/services/font/platformFontApi'
import { resolvePlatformFontFamily } from '@/services/font/platformFontRegistry'
import { createPlatformFontSelectPreviewController } from '@/services/font/platformFontSelectPreview'
import type { PlatformFontAsset } from '@/services/font/platformFontTypes'
import type { ChatCharactersSnapshotPayload } from '../bridge/theater-bridge-protocol'
import { shouldEnqueueTheaterDialogue } from '../bridge/theater-dialogue-queue'
import { TheaterDialogueRuntime } from './theater-dialogue-runtime'
import TheaterDialogueOverlay from './TheaterDialogueOverlay.vue'
import { normalizeTheaterDialogueEmbedSettings, THEATER_DIALOGUE_EMBED_SETTINGS_KEY } from './theater-dialogue-embed-settings'
import type { TheaterDialogueEmbedSettings } from './theater-dialogue-embed-settings'
import { replaceTheaterDialogueEmbedIdentity } from './theater-dialogue-embed-binding'

const route = useRoute()
const documentClass = 'theater-dialogue-embed-document'
document.documentElement.classList.add(documentClass)
const runtime = new TheaterDialogueRuntime()
const snapshot: ChatCharactersSnapshotPayload = { revision: 0, updatedAt: 0, activeIdentityId: null, characters: [] }
const context = ref({ worldId: '', channelId: '' })
const settings = ref(normalizeTheaterDialogueEmbedSettings(null))
const draft = ref({ ...settings.value })
const characters = ref<Array<{ label: string; value: string }>>([])
const fonts = ref<PlatformFontAsset[]>([])
const selectedFontId = computed({ get: () => draft.value.fontAssetId || null, set: value => { draft.value.fontAssetId = value || '' } })
const fontPreview = createPlatformFontSelectPreviewController({ fonts, selectedId: selectedFontId, menuClass: 'dialogue-embed-font-menu', immediateSelectedPreview: false })
const panelOpen = ref(false)
const saving = ref(false)
const ready = ref(false)
const canWrite = ref(false)
const notice = ref('')
let revision: number | undefined
let client: ChannelEmbedClient | null = null
const disposers: Array<() => void> = []
let reconnectTimer: ReturnType<typeof setTimeout> | undefined
let generation = 0
let disposed = false
let fontsRequested = false
let boundIdentityId = ''
let draftRevision: number | undefined
type StorageChangeEnqueue = (task: () => Promise<void>, onError?: (error: unknown) => void) => Promise<void>
let storageChangeEnqueue: StorageChangeEnqueue | null = null
const retryableEmbedErrorCodes = new Set([
  'HANDSHAKE_FAILED',
  'SESSION_EXPIRED',
  'CONTEXT_CHANGED',
  'TIMEOUT',
  'WS_OFFLINE',
  'INTERNAL_ERROR',
])
const MAX_RECONNECT_DELAY = 30_000
let reconnectDelay = 1000
type IdentityBindingResult = 'bound' | 'unbound' | 'missing'

const cleanupSession = () => {
  ready.value = false
  boundIdentityId = ''
  storageChangeEnqueue = null
  disposers.splice(0).forEach(dispose => dispose())
  client?.close()
  client = null
  runtime.reset()
}
const clearReconnectTimer = () => {
  if (reconnectTimer === undefined) return
  clearTimeout(reconnectTimer)
  reconnectTimer = undefined
}
const scheduleReconnect = () => {
  if (disposed || reconnectTimer !== undefined) return
  const delay = reconnectDelay
  reconnectDelay = Math.min(reconnectDelay * 2, MAX_RECONNECT_DELAY)
  reconnectTimer = setTimeout(() => { reconnectTimer = undefined; void connect() }, delay)
}
const updateCharacters = (input: unknown) => {
  characters.value = Array.isArray(input) ? input.flatMap(item => {
    if (!item || typeof item !== 'object' || !('id' in item) || typeof item.id !== 'string') return []
    return [{ value: item.id, label: 'displayName' in item && typeof item.displayName === 'string' ? item.displayName : item.id }]
  }) : []
}
const normalizeRevision = (input: unknown) => typeof input === 'number' && Number.isInteger(input) && input >= 0 ? input : undefined
const bindIdentity = async (active: ChannelEmbedClient, current: () => boolean): Promise<IdentityBindingResult> => {
  boundIdentityId = ''
  const identityId = settings.value.identityId
  const exists = characters.value.some(item => item.value === identityId)
  await replaceTheaterDialogueEmbedIdentity(runtime, active.theater.dialogue, exists ? identityId : '', current)
  if (!current()) return 'unbound'
  if (identityId && !exists) {
    notice.value = '已配置角色不在当前频道，请重新选择。'
    return 'missing'
  }
  boundIdentityId = identityId
  return identityId ? 'bound' : 'unbound'
}
const applyRemoteSettings = async (
  nextSettings: TheaterDialogueEmbedSettings,
  nextRevision: number | undefined,
  active: ChannelEmbedClient,
  current: () => boolean,
  options: { forceBind?: boolean; syncDraft?: boolean } = {},
): Promise<IdentityBindingResult | undefined> => {
  if (!current()) return
  const identityChanged = settings.value.identityId !== nextSettings.identityId
  revision = nextRevision
  settings.value = nextSettings
  if (options.syncDraft ?? !panelOpen.value) {
    draft.value = { ...nextSettings }
    draftRevision = nextRevision
  }
  let bindingResult: IdentityBindingResult | undefined
  if (options.forceBind || identityChanged || boundIdentityId !== nextSettings.identityId) {
    bindingResult = await bindIdentity(active, current)
  }
  if (current() && bindingResult !== 'missing') notice.value = ''
  return bindingResult
}
const readSettings = async (
  active: ChannelEmbedClient,
  current: () => boolean,
  options: { syncDraft?: boolean } = {},
): Promise<IdentityBindingResult | undefined> => {
  const result = await active.storage.get(THEATER_DIALOGUE_EMBED_SETTINGS_KEY)
  if (!current()) return
  const record = result && typeof result === 'object' ? result as Record<string, unknown> : {}
  const nextRevision = normalizeRevision(record.revision)
  const nextSettings = normalizeTheaterDialogueEmbedSettings(record.value)
  if (nextSettings.fontAssetId) void resolvePlatformFontFamily(nextSettings.fontAssetId).catch(() => undefined)
  return applyRemoteSettings(nextSettings, nextRevision, active, current, {
    syncDraft: options.syncDraft ?? !panelOpen.value,
  })
}
const applyRemoteStorageChange = async (input: unknown, active: ChannelEmbedClient, current: () => boolean) => {
  if (!current() || !input || typeof input !== 'object') return
  const change = input as Record<string, unknown>
  if (change.kind === 'resynced') {
    await readSettings(active, current)
    return
  }
  if (change.key !== THEATER_DIALOGUE_EMBED_SETTINGS_KEY) return
  const nextRevision = normalizeRevision(change.revision)
  if (change.kind === 'set') {
    if (nextRevision === undefined || (revision !== undefined && nextRevision <= revision)) return
    await applyRemoteSettings(normalizeTheaterDialogueEmbedSettings(change.value), nextRevision, active, current)
    return
  }
  if (change.kind === 'delete') {
    await applyRemoteSettings(normalizeTheaterDialogueEmbedSettings(null), nextRevision, active, current, { forceBind: true })
  }
}
const connect = async () => {
  clearReconnectTimer()
  const attempt = ++generation
  cleanupSession()
  const current = () => !disposed && generation === attempt
  try {
    const hostOrigin = typeof route.query.hostOrigin === 'string' ? route.query.hostOrigin : window.location.origin
    const active = await SealChatEmbed.connect({ targetOrigin: hostOrigin })
    if (!current()) { active.close(); return }
    client = active
    disposers.push(active.session.onClosed(() => {
      if (!current()) return
      ++generation
      cleanupSession()
      scheduleReconnect()
    }))
    disposers.push(active.characters.onChanged(value => {
      if (!current()) return
      updateCharacters(value)
      window.dispatchEvent(new CustomEvent('sealchat:theater-appearance-invalidated', { detail: { channelId: context.value.channelId } }))
      if (boundIdentityId && !characters.value.some(item => item.value === boundIdentityId)) {
        boundIdentityId = ''
        runtime.reset()
        void active.theater.dialogue.unsubscribe().catch(() => undefined)
        notice.value = '已配置角色不在当前频道，请重新选择。'
      }
    }))
    disposers.push(active.permissions.onChanged(() => {
      void active.permissions.getCurrent().then(value => { if (current()) canWrite.value = value.canWriteStorage }).catch(() => undefined)
    }))
    disposers.push(active.theater.dialogue.onCreated(payload => {
      if (current() && payload.actor.identityId === boundIdentityId) runtime.created(payload)
    }))
    disposers.push(active.theater.dialogue.onUpdated(payload => {
      if (!current() || payload.actor.identityId !== boundIdentityId) return
      if (shouldEnqueueTheaterDialogue(payload)) runtime.updated(payload)
      else runtime.removed(payload.messageId)
    }))
    disposers.push(active.theater.dialogue.onRemoved(payload => { if (current()) runtime.removed(payload.messageId) }))
    let storageChangeChain: Promise<void> = Promise.resolve()
    const enqueueStorageChange: StorageChangeEnqueue = (task, onError) => {
      const pending = storageChangeChain.then(task)
      storageChangeChain = pending.catch(error => {
        onError?.(error)
      })
      return pending
    }
    storageChangeEnqueue = enqueueStorageChange
    disposers.push(active.storage.onChanged(value => {
      void enqueueStorageChange(() => applyRemoteStorageChange(value, active, current), error => {
        if (current()) notice.value = error instanceof Error ? error.message : '设置同步失败'
      })
    }))
    const initialStorageTask = enqueueStorageChange(async () => {
      const [channel, identities, permissions] = await Promise.all([
        active.request<{ worldId: string; id: string }>('channel.getState'),
        active.characters.list(), active.permissions.getCurrent(),
      ])
      if (!current()) return
      context.value = { worldId: channel.worldId, channelId: channel.id }
      updateCharacters(identities)
      canWrite.value = permissions.canWriteStorage
      const bindingResult = await readSettings(active, current, { syncDraft: true })
      if (current()) {
        ready.value = true
        reconnectDelay = 1000
        if (bindingResult !== 'missing') notice.value = ''
      }
    })
    await initialStorageTask
  } catch (error) {
    if (!current()) return
    notice.value = error instanceof Error ? error.message : '连接失败'
    ++generation
    cleanupSession()
    if (error instanceof SealChatEmbedError && retryableEmbedErrorCodes.has(error.code)) scheduleReconnect()
  }
}
const openPanel = async () => {
  draft.value = { ...settings.value }
  draftRevision = revision
  panelOpen.value = true
  if (fontsRequested) return
  fontsRequested = true
  try { const items = await listPlatformFonts(); if (!disposed) fonts.value = items }
  catch { if (!disposed) { fontsRequested = false; notice.value = '平台字体列表加载失败，可重新打开面板重试。' } }
}
const save = async () => {
  const active = client
  const enqueueStorageChange = storageChangeEnqueue
  if (!active || !enqueueStorageChange || !ready.value || !canWrite.value || saving.value) return
  const attempt = generation
  const current = () => !disposed && generation === attempt && client === active
  const next = normalizeTheaterDialogueEmbedSettings(draft.value)
  if (next.identityId && !characters.value.some(item => item.value === next.identityId)) { notice.value = '请选择当前频道角色。'; return }
  const expectedRevision = draftRevision
  saving.value = true
  try {
    await enqueueStorageChange(async () => {
      if (!current()) return
      try {
        const result = await active.storage.set(THEATER_DIALOGUE_EMBED_SETTINGS_KEY, next, { ifRevision: expectedRevision })
        if (!current()) return
        const record = result && typeof result === 'object' ? result as Record<string, unknown> : {}
        const returnedRevision = normalizeRevision(record.revision)
        const bindingResult = await applyRemoteSettings(next, returnedRevision, active, current, { syncDraft: true })
        if (current()) {
          panelOpen.value = false
          if (bindingResult !== 'missing') notice.value = ''
        }
      } catch (error) {
        if (!current()) return
        if (error instanceof SealChatEmbedError && error.code === 'REVISION_CONFLICT') {
          try {
            const bindingResult = await readSettings(active, current, { syncDraft: true })
            if (current() && bindingResult !== 'missing') notice.value = '设置已被其他窗口修改，已重新读取，请检查后再保存。'
          } catch {
            if (current()) notice.value = '设置冲突后重新读取失败，请重新连接后再保存。'
          }
          return
        }
        throw error
      }
    })
  } catch (error) {
    if (!current()) return
    notice.value = error instanceof Error ? error.message : '保存失败'
  } finally { saving.value = false }
}
const closeOnPageHide = () => {
  ++generation
  clearReconnectTimer()
  cleanupSession()
}
const reconnectOnPageShow = (event: PageTransitionEvent) => { if (event.persisted) void connect() }
onMounted(() => {
  void connect()
  window.addEventListener('pagehide', closeOnPageHide)
  window.addEventListener('pageshow', reconnectOnPageShow)
})
onBeforeUnmount(() => {
  disposed = true
  ++generation
  clearReconnectTimer()
  cleanupSession()
  runtime.dispose()
  window.removeEventListener('pagehide', closeOnPageHide)
  window.removeEventListener('pageshow', reconnectOnPageShow)
  document.documentElement.classList.remove(documentClass)
})
</script>

<template>
  <main class="dialogue-embed">
    <TheaterDialogueOverlay :runtime="runtime" :character-snapshot="snapshot" :world-id="context.worldId" :channel-id="context.channelId" fill-container text-only :text-overrides="settings" />
    <n-button class="dialogue-embed__gear" quaternary circle aria-label="对话框设置" @click="openPanel"><template #icon><n-icon><Settings /></n-icon></template></n-button>
    <section v-if="panelOpen" class="dialogue-embed__settings" aria-label="对话框设置" @keydown.esc="panelOpen = false">
      <n-form label-placement="left" label-width="100" size="small" :disabled="saving || !ready || !canWrite">
        <n-form-item label="频道角色"><n-select v-model:value="draft.identityId" :options="characters" placeholder="选择角色" /></n-form-item>
        <n-form-item label="字号"><n-input-number :value="draft.fontSize" :min="12" :max="72" @update:value="draft.fontSize = $event ?? 24" /></n-form-item>
        <n-form-item label="角色名颜色"><n-input v-model:value="draft.speakerColor" placeholder="留空沿用角色颜色" /></n-form-item>
        <n-form-item label="正文颜色"><n-input v-model:value="draft.contentColor" placeholder="留空沿用角色设置" /></n-form-item>
        <n-form-item label="平台字体"><n-select v-model:value="selectedFontId" clearable :options="fontPreview.platformFontOptions.value" :render-label="fontPreview.renderPlatformFontLabel" :render-option="fontPreview.renderPlatformFontOption" menu-class="dialogue-embed-font-menu" placeholder="沿用角色设置" @update:show="fontPreview.handleDropdownVisible" /></n-form-item>
        <n-form-item label="角色名"><n-checkbox v-model:checked="draft.showSpeaker">显示角色名</n-checkbox></n-form-item>
        <n-form-item label="播放速度"><n-input-number v-model:value="draft.charactersPerSecond" clearable :min="1" :max="60" placeholder="沿用角色默认" /></n-form-item>
      </n-form>
      <p v-if="notice" role="status">{{ notice }}</p>
      <p v-if="!canWrite && ready">当前会话只读。</p>
      <div class="dialogue-embed__actions"><n-button size="small" @click="panelOpen = false">取消</n-button><n-button size="small" :loading="saving" :disabled="!ready || !canWrite" @click="save">保存</n-button></div>
    </section>
  </main>
</template>

<style>
html.theater-dialogue-embed-document,
html.theater-dialogue-embed-document body,
html.theater-dialogue-embed-document #app,
html.theater-dialogue-embed-document[data-custom-theme='true'],
html.theater-dialogue-embed-document[data-custom-theme='true'] body,
html.theater-dialogue-embed-document[data-custom-theme='true'] #app {
  background: transparent !important;
  background-color: transparent !important;
  margin: 0 !important;
}
html.theater-dialogue-embed-document .n-config-provider,
html.theater-dialogue-embed-document[data-custom-theme='true'] .n-config-provider {
  background: transparent !important;
  background-color: transparent !important;
}
</style>
<style scoped>
.dialogue-embed { position: fixed; inset: 0; background: transparent !important; background-color: transparent !important; overflow: hidden; }
.dialogue-embed__gear { position: absolute; top: 6px; right: 6px; z-index: 10000; opacity: .5; background: rgba(0, 0, 0, .16); transition: opacity 150ms, background-color 150ms; }
.dialogue-embed__gear:hover, .dialogue-embed__gear:focus-visible { opacity: 1; background: rgba(0, 0, 0, .28); }
.dialogue-embed__settings { position: absolute; z-index: 10001; top: 8px; right: 8px; width: min(360px, calc(100vw - 16px)); max-width: calc(100vw - 16px); max-height: calc(100vh - 16px); box-sizing: border-box; overflow: auto; padding: 16px; border-radius: 10px; background: var(--sc-bg-primary, #202024); opacity: .94; box-shadow: 0 4px 24px #0004; }
.dialogue-embed__actions { display: flex; justify-content: flex-end; gap: 8px; }
@media (prefers-reduced-motion: reduce) { .dialogue-embed__gear { transition: none; } }
</style>
