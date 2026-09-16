<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, type CSSProperties } from 'vue'
import { useRoute } from 'vue-router'
import { NButton, NForm, NFormItem, NIcon, NSelect } from 'naive-ui'
import { Settings } from '@vicons/tabler'
import {
  SealChatEmbed,
  SealChatEmbedError,
  type ChannelEmbedClient,
  type EmbedTheaterCharacterSnapshot,
} from '@/bridge/channelEmbedSdk'
import TheaterPresentationMedia from '@/components/theater-presentation/TheaterPresentationMedia.vue'
import { resolveTheaterTransformStyle, type TheaterVisualLayer } from '@/types/theaterPresentation'
import '@/components/theater-presentation/theaterComposition.css'
import {
  normalizeTheaterCharacterPortraitEmbedSettings,
  THEATER_CHARACTER_PORTRAIT_EMBED_SETTINGS_KEY,
  type TheaterCharacterPortraitEmbedSettings,
} from './theater-character-portrait-embed-settings'

const route = useRoute()
const documentClass = 'theater-character-portrait-embed-document'
document.documentElement.classList.add(documentClass)

const settings = ref(normalizeTheaterCharacterPortraitEmbedSettings(null))
const draft = ref({ ...settings.value })
const characters = ref<Array<{ label: string; value: string }>>([])
const snapshot = ref<EmbedTheaterCharacterSnapshot | null>(null)
const panelOpen = ref(false)
const saving = ref(false)
const ready = ref(false)
const canWrite = ref(false)
const notice = ref('')
let revision: number | undefined
let draftRevision: number | undefined
let client: ChannelEmbedClient | null = null
let reconnectTimer: ReturnType<typeof setTimeout> | undefined
let reconnectDelay = 1000
let generation = 0
let bindingEpoch = 0
let boundIdentityId = ''
let disposed = false
const disposers: Array<() => void> = []
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
type IdentityBindingResult = 'bound' | 'unbound' | 'missing'

const presentation = computed(() => snapshot.value?.character?.resolvedAppearance.theaterPresentation || null)
const portraitLayer = computed(() => presentation.value?.portrait || null)
const portrait = computed(() => portraitLayer.value?.enabled ? portraitLayer.value : null)
const decorations = computed(() => {
  if (!portrait.value) return []
  return [...(presentation.value?.portraitDecorations || [])]
    .filter(layer => layer.enabled)
    .sort((left, right) => left.transform.zIndex - right.transform.zIndex)
})
const portraitStyle = computed<CSSProperties>(() => {
  const layer = portrait.value
  if (!layer) return {}
  return {
    mixBlendMode: layer.blendMode,
  }
})
const portraitRootStyle = computed<CSSProperties>(() => {
  const layer = portrait.value
  if (!layer) return {}
  return {
    opacity: String(layer.transform.opacity),
    transform: `rotate(${layer.transform.rotation}deg)`,
    transformOrigin: 'center center',
  }
})
const decorationStyle = (layer: TheaterVisualLayer): CSSProperties => ({
  ...resolveTheaterTransformStyle(layer.transform),
  mixBlendMode: layer.blendMode,
})

const normalizeRevision = (input: unknown) => typeof input === 'number' && Number.isInteger(input) && input >= 0 ? input : undefined
const updateCharacters = (input: unknown) => {
  characters.value = Array.isArray(input) ? input.flatMap(item => {
    if (!item || typeof item !== 'object' || !('id' in item) || typeof item.id !== 'string') return []
    return [{
      value: item.id,
      label: 'displayName' in item && typeof item.displayName === 'string' ? item.displayName : item.id,
    }]
  }) : []
}
const currentPortraitNotice = (identityId = settings.value.identityId) => {
  if (!identityId) return ''
  if (!snapshot.value) return '正在同步角色立绘……'
  if (!snapshot.value.character) return '已配置角色当前没有可用的小剧场角色快照。'
  return portrait.value ? '' : '当前角色未配置小剧场立绘。'
}
const cleanupSession = () => {
  ready.value = false
  boundIdentityId = ''
  bindingEpoch += 1
  snapshot.value = null
  storageChangeEnqueue = null
  disposers.splice(0).forEach(dispose => dispose())
  client?.close()
  client = null
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
  reconnectTimer = setTimeout(() => {
    reconnectTimer = undefined
    void connect()
  }, delay)
}
const bindIdentity = async (active: ChannelEmbedClient, current: () => boolean): Promise<IdentityBindingResult> => {
  const attempt = ++bindingEpoch
  const identityId = settings.value.identityId
  const previousIdentityId = boundIdentityId
  boundIdentityId = ''
  snapshot.value = null
  if (previousIdentityId) await active.theater.character.unsubscribe().catch(() => undefined)
  if (!current() || bindingEpoch !== attempt) return 'unbound'
  if (!identityId) return 'unbound'
  const nextSnapshot = await active.theater.character.subscribe({ identityId })
  if (!current() || bindingEpoch !== attempt) return 'unbound'
  boundIdentityId = identityId
  snapshot.value = nextSnapshot
  return 'bound'
}
const applyRemoteSettings = async (
  nextSettings: TheaterCharacterPortraitEmbedSettings,
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
  if (current() && bindingResult !== 'missing') {
    notice.value = bindingResult === 'bound' ? currentPortraitNotice(nextSettings.identityId) : ''
  }
  return bindingResult
}
const readSettings = async (
  active: ChannelEmbedClient,
  current: () => boolean,
  options: { syncDraft?: boolean } = {},
) => {
  const result = await active.storage.get(THEATER_CHARACTER_PORTRAIT_EMBED_SETTINGS_KEY)
  if (!current()) return
  const record = result && typeof result === 'object' ? result as Record<string, unknown> : {}
  return applyRemoteSettings(
    normalizeTheaterCharacterPortraitEmbedSettings(record.value),
    normalizeRevision(record.revision),
    active,
    current,
    { syncDraft: options.syncDraft ?? !panelOpen.value },
  )
}
const applyRemoteStorageChange = async (input: unknown, active: ChannelEmbedClient, current: () => boolean) => {
  if (!current() || !input || typeof input !== 'object') return
  const change = input as Record<string, unknown>
  if (change.kind === 'resynced') {
    await readSettings(active, current)
    return
  }
  if (change.key !== THEATER_CHARACTER_PORTRAIT_EMBED_SETTINGS_KEY) return
  const nextRevision = normalizeRevision(change.revision)
  if (change.kind === 'set') {
    if (nextRevision === undefined || (revision !== undefined && nextRevision <= revision)) return
    await applyRemoteSettings(normalizeTheaterCharacterPortraitEmbedSettings(change.value), nextRevision, active, current)
  } else if (change.kind === 'delete') {
    await applyRemoteSettings(normalizeTheaterCharacterPortraitEmbedSettings(null), nextRevision, active, current, { forceBind: true })
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
    if (!active.capabilities.includes('theater.character.read')) {
      active.close()
      throw new SealChatEmbedError({ code: 'CAPABILITY_DENIED', message: '当前运行上下文不提供小剧场角色快照。' })
    }
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
    }))
    disposers.push(active.permissions.onChanged(() => {
      void active.permissions.getCurrent().then(value => {
        if (current()) canWrite.value = value.canWriteStorage
      }).catch(() => undefined)
    }))
    disposers.push(active.theater.character.onChanged(value => {
      if (!current() || value.identityId !== boundIdentityId) return
      snapshot.value = value
      notice.value = currentPortraitNotice(value.identityId)
    }))
    let storageChangeChain: Promise<void> = Promise.resolve()
    const enqueueStorageChange: StorageChangeEnqueue = (task, onError) => {
      const pending = storageChangeChain.then(task)
      storageChangeChain = pending.catch(error => { onError?.(error) })
      return pending
    }
    storageChangeEnqueue = enqueueStorageChange
    disposers.push(active.storage.onChanged(value => {
      void enqueueStorageChange(() => applyRemoteStorageChange(value, active, current), error => {
        if (current()) notice.value = error instanceof Error ? error.message : '设置同步失败'
      })
    }))
    await enqueueStorageChange(async () => {
      const [identities, permissions] = await Promise.all([
        active.characters.list(),
        active.permissions.getCurrent(),
      ])
      if (!current()) return
      updateCharacters(identities)
      canWrite.value = permissions.canWriteStorage
      const bindingResult = await readSettings(active, current, { syncDraft: true })
      if (!current()) return
      ready.value = true
      reconnectDelay = 1000
      if (bindingResult !== 'missing' && settings.value.identityId && !portrait.value) {
        notice.value = currentPortraitNotice()
      }
    })
  } catch (error) {
    if (!current()) return
    notice.value = error instanceof Error ? error.message : '连接失败'
    ++generation
    cleanupSession()
    if (error instanceof SealChatEmbedError && retryableEmbedErrorCodes.has(error.code)) scheduleReconnect()
  }
}
const openPanel = () => {
  draft.value = { ...settings.value }
  draftRevision = revision
  panelOpen.value = true
  if (settings.value.identityId) notice.value = currentPortraitNotice()
}
const save = async () => {
  const active = client
  const enqueueStorageChange = storageChangeEnqueue
  if (!active || !enqueueStorageChange || !ready.value || !canWrite.value || saving.value) return
  const attempt = generation
  const current = () => !disposed && generation === attempt && client === active
  const next = normalizeTheaterCharacterPortraitEmbedSettings(draft.value)
  const identityChanged = next.identityId !== settings.value.identityId
  if (identityChanged && next.identityId && !characters.value.some(item => item.value === next.identityId)) {
    notice.value = '请选择当前可用的频道角色。'
    return
  }
  const expectedRevision = draftRevision
  saving.value = true
  try {
    await enqueueStorageChange(async () => {
      if (!current()) return
      try {
        const result = await active.storage.set(THEATER_CHARACTER_PORTRAIT_EMBED_SETTINGS_KEY, next, { ifRevision: expectedRevision })
        if (!current()) return
        const record = result && typeof result === 'object' ? result as Record<string, unknown> : {}
        const bindingResult = await applyRemoteSettings(next, normalizeRevision(record.revision), active, current, { syncDraft: true })
        if (current()) {
          panelOpen.value = false
          if (bindingResult !== 'missing' && next.identityId && !portrait.value) {
            notice.value = currentPortraitNotice(next.identityId)
          }
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
    if (current()) notice.value = error instanceof Error ? error.message : '保存失败'
  } finally {
    saving.value = false
  }
}
const closeOnPageHide = () => {
  ++generation
  clearReconnectTimer()
  cleanupSession()
}
const reconnectOnPageShow = (event: PageTransitionEvent) => {
  if (event.persisted) void connect()
}

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
  window.removeEventListener('pagehide', closeOnPageHide)
  window.removeEventListener('pageshow', reconnectOnPageShow)
  document.documentElement.classList.remove(documentClass)
})
</script>

<template>
  <main class="portrait-embed" :class="{ 'is-settings-open': panelOpen }" @keydown.esc="panelOpen = false">
    <div class="portrait-embed__composition theater-composition-host">
      <div class="portrait-embed__portrait-root" :style="portraitRootStyle">
        <div v-if="portrait" class="portrait-embed__portrait" :style="portraitStyle">
          <TheaterPresentationMedia :media="portrait.media" :playback-rate="portrait.playbackRate" />
        </div>
        <div
          v-for="layer in decorations"
          :key="layer.id"
          class="portrait-embed__decoration"
          :style="decorationStyle(layer)"
        >
          <TheaterPresentationMedia :media="layer.media" :playback-rate="layer.playbackRate" />
        </div>
      </div>
    </div>
    <n-button class="portrait-embed__gear" quaternary circle aria-label="立绘设置" @click="panelOpen ? panelOpen = false : openPanel()">
      <template #icon><n-icon><Settings /></n-icon></template>
    </n-button>
    <section v-if="panelOpen" class="portrait-embed__settings" aria-label="立绘设置">
      <n-form label-placement="left" label-width="100" size="small" :disabled="saving || !ready || !canWrite">
        <n-form-item label="频道角色">
          <n-select v-model:value="draft.identityId" clearable :options="characters" placeholder="选择角色" />
        </n-form-item>
      </n-form>
      <p v-if="notice" role="status">{{ notice }}</p>
      <p v-if="!canWrite && ready">当前会话只读。</p>
      <div class="portrait-embed__actions">
        <n-button size="small" @click="panelOpen = false">取消</n-button>
        <n-button size="small" :loading="saving" :disabled="!ready || !canWrite" @click="save">保存</n-button>
      </div>
    </section>
  </main>
</template>

<style>
html.theater-character-portrait-embed-document,
html.theater-character-portrait-embed-document body,
html.theater-character-portrait-embed-document #app,
html.theater-character-portrait-embed-document[data-custom-theme='true'],
html.theater-character-portrait-embed-document[data-custom-theme='true'] body,
html.theater-character-portrait-embed-document[data-custom-theme='true'] #app {
  background: transparent !important;
  background-color: transparent !important;
  margin: 0 !important;
}
html.theater-character-portrait-embed-document .n-config-provider,
html.theater-character-portrait-embed-document[data-custom-theme='true'] .n-config-provider {
  background: transparent !important;
  background-color: transparent !important;
}
</style>

<style scoped>
.portrait-embed { position: fixed; inset: 0; overflow: hidden; background: transparent !important; }
.portrait-embed__composition { position: absolute; inset: 0; overflow: hidden; isolation: isolate; }
.portrait-embed__portrait-root { position: absolute; inset: 0; }
.portrait-embed__portrait { position: absolute; inset: 0; }
.portrait-embed__decoration { pointer-events: none; }
.portrait-embed__gear { position: absolute; top: 6px; right: 6px; z-index: 10002; opacity: 0; pointer-events: none; background: rgba(0, 0, 0, 0); transition: opacity 150ms, background-color 150ms; }
.portrait-embed:hover .portrait-embed__gear,
.portrait-embed:focus-within .portrait-embed__gear,
.portrait-embed.is-settings-open .portrait-embed__gear { opacity: 1; pointer-events: auto; background: rgba(0, 0, 0, .2); }
.portrait-embed__gear:hover, .portrait-embed__gear:focus-visible { background: rgba(0, 0, 0, .32); }
.portrait-embed__settings { position: absolute; z-index: 10001; top: 8px; right: 8px; width: min(360px, calc(100vw - 16px)); max-width: calc(100vw - 16px); max-height: calc(100vh - 16px); box-sizing: border-box; overflow: auto; padding: 16px; border-radius: 10px; background: var(--sc-bg-primary, #202024); opacity: .94; box-shadow: 0 4px 24px #0004; }
.portrait-embed__actions { display: flex; justify-content: flex-end; gap: 8px; }
@media (hover: none) { .portrait-embed__gear { opacity: .3; pointer-events: auto; background: rgba(0, 0, 0, .12); } }
@media (prefers-reduced-motion: reduce) { .portrait-embed__gear { transition: none; } }
</style>
