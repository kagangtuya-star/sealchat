<script setup lang="ts">
import { computed, onBeforeMount, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useWindowSize } from '@vueuse/core'
import { NButton, NIcon, useMessage } from 'naive-ui'
import { ArrowsMaximize, MessageOff } from '@vicons/tabler'
import { useChatStore } from '@/stores/chat'
import { useAudioStudioStore } from '@/stores/audioStudio'
import { useUserStore } from '@/stores/user'
import StageApp from '../stage/StageApp.vue'
import { createTheaterStageStore } from '../stage/StageStore'
import { mergeTheaterBridgePermissions, TheaterHostBridge } from '../bridge/TheaterHostBridge'
import { createTheaterBridgeId } from '../bridge/theater-bridge-protocol'
import type { ChatCharactersSnapshotPayload } from '../bridge/theater-bridge-protocol'
import { TheaterSyncClient } from '../sync/TheaterSyncClient'
import type { StagePointerTraceInput } from '../shared/stage-types'
import { TheaterDialogueRuntime } from '../dialogue/theater-dialogue-runtime'
import { theaterPresentationSchema, type TheaterPresentation } from '@/types/theaterPresentation'
import type { TheaterEditorCommand, TheaterSection, TheaterSelection } from '@/components/theater-presentation/theaterPresentationEditorState'
import DiceOverlayLoader from '@/features/dice3d/components/DiceOverlayLoader.vue'
import { dice3dRuntime, isDice3DTheaterMessage } from '@/features/dice3d/runtime'
import { useDisplayStore } from '@/stores/display'
import {
  installTheaterBridgeDebugConsoleCommand,
  isTheaterBridgeDebugEnabled,
} from '../bridge/theater-bridge-debug'

const route = useRoute()
const router = useRouter()
const message = useMessage()
const chat = useChatStore()
const user = useUserStore()
const display = useDisplayStore()
const audioStudio = useAudioStudioStore()
const { width } = useWindowSize()

const routeWorldId = computed(() => typeof route.query.worldId === 'string' ? route.query.worldId.trim() : '')
const routeChannelId = computed(() => typeof route.query.channelId === 'string' ? route.query.channelId.trim() : '')
const worldId = ref(routeWorldId.value)
const channelId = ref(routeChannelId.value)
const stageStore = createTheaterStageStore()
const sessionId = createTheaterBridgeId('session')
const dialogueRuntime = new TheaterDialogueRuntime()

installTheaterBridgeDebugConsoleCommand()

const layoutRef = ref<HTMLDivElement | null>(null)
const iframeRef = ref<HTMLIFrameElement | null>(null)
const stageAppRef = ref<InstanceType<typeof StageApp> | null>(null)
const stageSurfaceRef = ref<HTMLElement | null>(null)
const splitRatio = ref(0.7)
const splitDragging = ref(false)
const chatHidden = ref(false)
const mobileTab = ref<'stage' | 'chat'>('stage')
const isNarrow = computed(() => width.value < 840)
const chatVisible = computed(() => isNarrow.value ? mobileTab.value === 'chat' : !chatHidden.value)
const theaterDividerWidth = 7
const chatBridgeOnline = ref(false)
const theaterSyncing = ref(false)
const theaterSyncReady = ref(false)
const theaterPermissions = ref<string[]>([])
const sceneDialogueStorageKey = 'sealchat.theater.scene-switch-text.enabled.v1'
const readSceneDialogueEnabled = () => {
  try {
    return window.localStorage.getItem(sceneDialogueStorageKey) === '1'
  } catch {
    return false
  }
}
const sceneDialogueEnabled = ref(typeof window !== 'undefined' && readSceneDialogueEnabled())
type AppearancePreviewState = {
  previewId: string
  draft: TheaterPresentation
  selection: TheaterSelection
  activeSection: TheaterSection
  previewName: string
  previewText: string
}
const appearancePreview = ref<AppearancePreviewState | null>(null)
const characterSnapshot = ref<ChatCharactersSnapshotPayload>({
  revision: 0,
  updatedAt: 0,
  activeIdentityId: null,
  characters: [],
})
let theaterBridge: TheaterHostBridge | null = null
let theaterSync: TheaterSyncClient | null = null
let theaterSyncGeneration = 0

audioStudio.setPlaybackAuthority(false)

const iframeSrc = computed(() => {
  if (typeof window === 'undefined') return ''
  const url = new URL(window.location.href)
  const params = new URLSearchParams({
    mode: 'theater',
    viewport: 'mobile',
    paneId: 'theater-chat',
    worldId: worldId.value,
    channelId: channelId.value,
    sessionId,
    audioOwner: '1',
  })
  url.hash = `/embed?${params.toString()}`
  return url.toString()
})

const normalizeRatio = (value: number) => Math.min(1, Math.max(0, value))

const splitPaneWidth = (ratio: number) => {
  const normalized = normalizeRatio(ratio)
  return `calc(${normalized * 100}% - ${normalized * theaterDividerWidth}px)`
}

const updateRatio = (clientX: number) => {
  const rect = layoutRef.value?.getBoundingClientRect()
  if (!rect?.width) return
  const availableWidth = Math.max(0, rect.width - theaterDividerWidth)
  if (!availableWidth) return
  const stageWidth = Math.min(availableWidth, Math.max(0, clientX - rect.left))
  splitRatio.value = stageWidth / availableWidth
}

const handleDividerDown = (event: PointerEvent) => {
  if (event.button !== 0) return
  splitDragging.value = true
  ;(event.currentTarget as HTMLElement | null)?.setPointerCapture?.(event.pointerId)
  updateRatio(event.clientX)
}

const handleDividerMove = (event: PointerEvent) => {
  if (splitDragging.value) updateRatio(event.clientX)
}

const stopDivider = (event: PointerEvent) => {
  if (!splitDragging.value) return
  splitDragging.value = false
  ;(event.currentTarget as HTMLElement | null)?.releasePointerCapture?.(event.pointerId)
}

const resetLayout = () => {
  splitRatio.value = 0.7
  chatHidden.value = false
  mobileTab.value = 'stage'
}

const toggleChat = () => {
  if (isNarrow.value) {
    mobileTab.value = mobileTab.value === 'chat' ? 'stage' : 'chat'
    return
  }
  chatHidden.value = !chatHidden.value
}

const exitTheater = async () => {
  await router.push({
    name: 'world-channel',
    params: { worldId: worldId.value, channelId: channelId.value },
  })
}

const selectChatCharacter = async (identityId: string) => {
  try {
    await theaterBridge?.selectChatCharacter(identityId)
  } catch (error) {
    message.warning(error instanceof Error ? error.message : '切换角色失败')
  }
}

const selectChatCharacterVariant = async (payload: { identityId: string, variantId: string | null }) => {
  try {
    await theaterBridge?.selectChatCharacterVariant(payload.identityId, payload.variantId)
  } catch (error) {
    message.warning(error instanceof Error ? error.message : '切换差分失败')
  }
}

const requestTheaterPreload = async (sceneIds: string[]) => {
  try {
    await theaterSync?.requestPreload(sceneIds)
  } catch (error) {
    message.warning(error instanceof Error ? error.message : '场景预加载请求失败')
  }
}

const publishTheaterPointerTrace = (trace: StagePointerTraceInput) => {
  void theaterSync?.publishPointerTrace(trace).catch((error) => {
    message.warning(error instanceof Error ? error.message : '临时轨迹同步失败')
  })
}

const sendSceneDialogue = async (sceneId: string) => {
  const scene = stageStore.state.scenes[sceneId]
  if (!sceneDialogueEnabled.value || !scene?.switchText) return
  try {
    const result = await theaterBridge?.sendChatMessage({
      content: scene.switchText,
      channelId: channelId.value,
      preserveComposer: true,
      ...(characterSnapshot.value.activeIdentityId ? { characterId: characterSnapshot.value.activeIdentityId } : {}),
    })
    if (result && !result.ok) message.warning(`场景已切换，台词发送失败：${result.error.message}`)
  } catch (error) {
    message.warning(`场景已切换，台词发送失败：${error instanceof Error ? error.message : '未知错误'}`)
  }
}

const requestSceneSwitch = (sceneId: string) => {
  if (!stageStore.applyScene(sceneId)) return
  void sendSceneDialogue(sceneId)
}

watch(width, () => { splitRatio.value = normalizeRatio(splitRatio.value) })
watch(sceneDialogueEnabled, (enabled) => {
  try {
    window.localStorage.setItem(sceneDialogueStorageKey, enabled ? '1' : '0')
  } catch {
    // The setting remains active for this page when storage is unavailable.
  }
})

const emptyCharacterSnapshot = (): ChatCharactersSnapshotPayload => ({
  revision: 0,
  updatedAt: 0,
  activeIdentityId: null,
  characters: [],
})

const resolveBridgePermissions = (stagePermissions: readonly string[]) => {
  const memberRole = chat.worldDetailMap[worldId.value]?.memberRole
  return mergeTheaterBridgePermissions(stagePermissions, memberRole === 'owner' || memberRole === 'admin')
}

const startTheaterBridge = () => {
  if (!worldId.value || !channelId.value || typeof window === 'undefined') return
  dialogueRuntime.reset()
  theaterBridge?.stop()
  theaterBridge = null
  chatBridgeOnline.value = false
  characterSnapshot.value = emptyCharacterSnapshot()
  const memberRole = chat.worldDetailMap[worldId.value]?.memberRole
  const stagePermissions = theaterPermissions.value.length
    ? theaterPermissions.value
    : memberRole === 'owner' || memberRole === 'admin'
      ? ['stage.view', 'stage.scene.switch', 'stage.object.edit', 'stage.action.trigger']
      : ['stage.view', 'stage.object.edit.delegated', 'stage.action.trigger']
  const permissions = resolveBridgePermissions(stagePermissions)
  theaterBridge = new TheaterHostBridge({
    context: { worldId: worldId.value, channelId: channelId.value, sessionId },
    stageStore,
    getChatWindow: () => iframeRef.value?.contentWindow || null,
    origin: window.location.origin,
    userId: user.info?.id ? String(user.info.id) : '',
    permissions,
    debug: () => import.meta.env.DEV || route.query.bridgeDebug === '1' || isTheaterBridgeDebugEnabled(),
    onChatOnlineChange: (online) => { chatBridgeOnline.value = online },
    onCharacterSnapshotChange: (snapshot) => { characterSnapshot.value = snapshot },
    onChatMessageCreated: dialogueRuntime.created,
    onChatMessageUpdated: dialogueRuntime.updated,
    onChatMessageRemoved: ({ messageId }) => dialogueRuntime.removed(messageId),
    onSceneApplied: (sceneId) => { void sendSceneDialogue(sceneId) },
    triggerStageAction: async (payload) => {
      if (!theaterSync) return false
      try {
        const handled = await theaterSync.triggerAction(payload)
        if (handled === true && payload.action.type === 'scene.apply') {
          await sendSceneDialogue(stageStore.state.activeSceneId)
        }
        return handled
      } catch (error) {
        message.warning(error instanceof Error ? error.message : '舞台动作执行失败')
        return true
      }
    },
  })
  void theaterBridge.start().catch((error) => {
    console.warn('[theater-bridge] host startup failed', error)
  })
}

const startTheaterSync = async () => {
  const generation = ++theaterSyncGeneration
  const targetWorldId = worldId.value
  const targetChannelId = channelId.value
  const previousClient = theaterSync
  theaterSync = null
  theaterSyncReady.value = false
  theaterSyncing.value = false
  theaterPermissions.value = []
  await previousClient?.stop()
  const isCurrent = () => generation === theaterSyncGeneration
  if (!isCurrent() || !targetWorldId || !targetChannelId) return
  if (chat.currentWorldId !== targetWorldId) chat.setCurrentWorld(targetWorldId)
  await chat.tryInit()
  if (!isCurrent()) return
  if (chat.curChannel?.id !== targetChannelId) {
    const switched = await chat.channelSwitchTo(targetChannelId)
    if (!isCurrent()) return
    if (!switched) throw new Error('无法进入小剧场频道')
  }
  const client = new TheaterSyncClient({
    worldId: targetWorldId,
    channelId: '',
    inputChannelId: targetChannelId,
    scopeType: 'world',
    store: stageStore,
    sendGatewayAPI: (apiName, data) => chat.sendAPI(apiName, data),
    onPermissionsChange: (permissions) => {
      if (!isCurrent() || theaterSync !== client) return
      theaterPermissions.value = permissions
      theaterBridge?.setPermissions(resolveBridgePermissions(permissions))
    },
    onSyncingChange: (syncing) => {
      if (isCurrent() && theaterSync === client) theaterSyncing.value = syncing
    },
    onPreloadRequested: (sceneIds, requestId) => {
      if (isCurrent() && theaterSync === client) void stageAppRef.value?.preloadScenes(sceneIds, requestId)
    },
    onPointerTrace: (trace) => {
      if (isCurrent() && theaterSync === client) stageAppRef.value?.appendPointerTrace(trace)
    },
    onError: (error) => {
      if (isCurrent() && theaterSync === client) message.warning(error)
    },
  })
  if (!isCurrent()) return
  theaterSync = client
  try {
    await client.start()
    if (!isCurrent() || theaterSync !== client) {
      await client.stop()
      return
    }
    theaterSyncReady.value = true
  } catch (error) {
    await client.stop()
    if (theaterSync === client) theaterSync = null
    if (isCurrent()) throw error
  }
}

const handleTheaterContext = (event: MessageEvent) => {
  if (event.origin !== window.location.origin || event.source !== iframeRef.value?.contentWindow) return
  const data = event.data as Record<string, unknown> | null
  if (!data) return
  if (data.type === 'sealchat.theater.appearance-preview.stop') {
    appearancePreview.value = null
    return
  }
  if (data.type === 'sealchat.theater.appearance.invalidated') {
    if (data.sessionId !== sessionId || typeof data.channelId !== 'string') return
    window.dispatchEvent(new CustomEvent('sealchat:theater-appearance-invalidated', {
      detail: { channelId: data.channelId, targetUserId: data.targetUserId },
    }))
    return
  }
  if (data.type === 'sealchat.theater.appearance-preview.start' || data.type === 'sealchat.theater.appearance-preview.update') {
    const parsed = theaterPresentationSchema.safeParse(data.draft)
    if (!parsed.success || typeof data.previewId !== 'string' || !data.selection || typeof data.selection !== 'object' || typeof data.activeSection !== 'string') return
    appearancePreview.value = {
      previewId: data.previewId,
      draft: parsed.data,
      selection: data.selection as TheaterSelection,
      activeSection: data.activeSection as TheaterSection,
      previewName: typeof data.previewName === 'string' ? data.previewName : '角色名',
      previewText: typeof data.previewText === 'string' ? data.previewText : '夜色正好，我们该出发了。',
    }
    return
  }
  if (
    data.type !== 'sealchat.theater.context'
    || data.sessionId !== sessionId
    || typeof data.worldId !== 'string'
    || typeof data.channelId !== 'string'
  ) return
  const nextWorldId = data.worldId.trim()
  const nextChannelId = data.channelId.trim()
  if (!nextWorldId || !nextChannelId || (nextWorldId === worldId.value && nextChannelId === channelId.value)) return
  worldId.value = nextWorldId
  channelId.value = nextChannelId
  void router.replace({
    name: 'theater',
    query: { ...route.query, worldId: nextWorldId, channelId: nextChannelId },
  })
  startTheaterBridge()
  void startTheaterSync().catch((error) => {
    message.error(error instanceof Error ? error.message : '小剧场同步启动失败')
  })
}

const sendAppearancePreviewCommand = (command: TheaterEditorCommand, transient = false) => {
  const preview = appearancePreview.value
  const target = iframeRef.value?.contentWindow
  if (!preview || !target) return
  target.postMessage({
    type: 'sealchat.theater.appearance-preview.command',
    previewId: preview.previewId,
    command,
    transient,
  }, window.location.origin)
}

const sendAppearancePreviewPhase = (phase: 'start' | 'end') => {
  const preview = appearancePreview.value
  const target = iframeRef.value?.contentWindow
  if (!preview || !target) return
  target.postMessage({
    type: 'sealchat.theater.appearance-preview.command',
    previewId: preview.previewId,
    phase,
  }, window.location.origin)
}

onBeforeMount(startTheaterBridge)

onMounted(async () => {
  window.addEventListener('message', handleTheaterContext)
	window.addEventListener('message', handleDice3DMessage)
  if (!worldId.value || !channelId.value) {
    message.warning('请先进入频道')
    await router.replace({ name: 'home' })
    return
  }
  try {
    await startTheaterSync()
  } catch (error) {
    message.error(error instanceof Error ? error.message : '小剧场同步启动失败')
  }
})

onBeforeUnmount(() => {
  theaterSyncGeneration += 1
  window.removeEventListener('message', handleTheaterContext)
	window.removeEventListener('message', handleDice3DMessage)
  appearancePreview.value = null
  theaterBridge?.stop()
  theaterBridge = null
  dialogueRuntime.dispose()
  void theaterSync?.stop()
  theaterSync = null
  audioStudio.setPlaybackAuthority(true)
})

function handleDice3DMessage(event: MessageEvent) {
	if (!isDice3DTheaterMessage(event)) return
	dice3dRuntime.play(event.data.payload)
}
</script>

<template>
  <main class="theater-host">
    <div
      ref="layoutRef"
      class="theater-host-layout"
      :class="{ 'is-dragging': splitDragging, 'is-narrow': isNarrow, 'is-chat-hidden': chatHidden }"
    >
      <section
		ref="stageSurfaceRef"
        v-show="!isNarrow || mobileTab === 'stage'"
        class="theater-host-stage"
        :class="{ 'is-sync-pending': !theaterSyncReady }"
        :style="!isNarrow && !chatHidden ? { width: splitPaneWidth(splitRatio) } : undefined"
      >
        <StageApp
          ref="stageAppRef"
          :store="stageStore"
          :world-id="worldId"
          :channel-id="channelId"
          scope-type="world"
          :character-snapshot="characterSnapshot"
          :chat-bridge-online="chatBridgeOnline"
          :chat-visible="chatVisible"
          :sync-ready="theaterSyncReady"
          :syncing="theaterSyncing"
          :permissions="theaterPermissions"
          :dialogue-runtime="dialogueRuntime"
          :appearance-preview="appearancePreview"
          :scene-dialogue-enabled="sceneDialogueEnabled"
          @action-triggered="theaterBridge?.triggerStageAction($event)"
          @pointer-trace="publishTheaterPointerTrace($event)"
          @preload-requested="requestTheaterPreload"
          @scene-switch-requested="requestSceneSwitch"
          @update-scene-dialogue-enabled="sceneDialogueEnabled = $event"
          @select-character="selectChatCharacter"
          @select-character-variant="selectChatCharacterVariant"
          @toggle-chat="toggleChat"
          @reset-layout="resetLayout"
          @exit-theater="exitTheater"
          @appearance-preview-command="sendAppearancePreviewCommand"
          @appearance-preview-phase="sendAppearancePreviewPhase"
        />
        <div v-if="!theaterSyncReady" class="theater-sync-loading">正在加载后端舞台……</div>
		<DiceOverlayLoader
          v-if="display.settings.dice3dEnabled"
          :surface-element="stageSurfaceRef"
          :chat-surface-element="iframeRef"
        />
      </section>

      <div
        v-if="!isNarrow && !chatHidden"
        class="theater-host-divider"
        role="separator"
        aria-label="调整幕布与聊天宽度"
        @pointerdown="handleDividerDown"
        @pointermove="handleDividerMove"
        @pointerup="stopDivider"
        @pointercancel="stopDivider"
      ><n-icon><ArrowsMaximize /></n-icon></div>

      <section
        v-show="!chatHidden && (!isNarrow || mobileTab === 'chat')"
        class="theater-host-chat"
        :style="!isNarrow ? { width: splitPaneWidth(1 - splitRatio) } : undefined"
      >
        <n-button
          v-if="isNarrow"
          class="theater-host-chat-close"
          quaternary
          circle
          aria-label="隐藏聊天"
          @click="toggleChat"
        ><template #icon><n-icon><MessageOff /></n-icon></template></n-button>
        <iframe
          ref="iframeRef"
          class="theater-host-chat-frame"
          title="SealChat 小剧场聊天"
          :src="iframeSrc"
          frameborder="0"
          allow="autoplay; clipboard-read; clipboard-write"
          @load="theaterBridge?.handleChatFrameLoad()"
        />
      </section>
    </div>
  </main>
</template>

<style scoped>
.theater-host { height: 100vh; width: 100vw; overflow: hidden; color: var(--sc-text-primary, #f4f4f5); background: var(--sc-bg-page, #141418); }
.theater-host-layout { width: 100%; height: 100%; display: flex; overflow: hidden; }
.theater-host-stage, .theater-host-chat { min-width: 0; height: 100%; overflow: hidden; }
.theater-host-stage { position: relative; }
.theater-host-stage.is-sync-pending :deep(.theater-stage-app) { pointer-events: none; opacity: .55; }
.theater-sync-loading { position: absolute; z-index: 20; inset: 0; display: grid; place-items: center; color: var(--sc-text-primary, #f4f4f5); background: color-mix(in srgb, var(--sc-bg-page, #141418) 76%, transparent); font-size: 13px; }
.theater-host-layout.is-chat-hidden .theater-host-stage { width: 100% !important; }
.theater-host-divider { position: relative; z-index: 3; width: 7px; flex: 0 0 7px; display: grid; place-items: center; overflow: visible; color: var(--sc-fg-muted, #71717a); background: var(--sc-bg-header, #262626); cursor: col-resize; touch-action: none; user-select: none; }
.theater-host-divider::before { content: ''; position: absolute; inset: 0 2px; background: var(--sc-border-mute, rgba(255, 255, 255, .08)); }
.theater-host-divider :deep(svg) { position: relative; width: 12px; padding: 2px 0; border-radius: 4px; background: var(--sc-bg-header, #262626); }
.theater-host-divider:hover::before, .is-dragging .theater-host-divider::before { background: #3b82f6; }
.theater-host-chat { position: relative; border-left: 1px solid var(--sc-border-mute, rgba(255, 255, 255, .08)); background: var(--sc-bg-surface, #1b1b20); }
.theater-host-chat-frame { width: 100%; height: 100%; display: block; box-sizing: border-box; margin: 0; border: 0; outline: 0; background: var(--sc-bg-surface, #1b1b20); }
.is-dragging .theater-host-chat-frame { pointer-events: none; }
.theater-host-chat-close { position: absolute; z-index: 4; top: 8px; left: 8px; width: 34px; height: 34px; background: color-mix(in srgb, var(--sc-bg-elevated, #26262c) 92%, transparent); box-shadow: 0 6px 18px rgba(0, 0, 0, .2); }
.theater-host-layout.is-narrow { display: block; }
.theater-host-layout.is-narrow .theater-host-stage, .theater-host-layout.is-narrow .theater-host-chat { width: 100%; }
</style>
