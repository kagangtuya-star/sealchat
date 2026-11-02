import { computed, ref, shallowRef, watch } from 'vue'
import type { Pinia } from 'pinia'
import type { Router } from 'vue-router'
import { createDiscreteApi, darkTheme } from 'naive-ui'
import { chatEvent, useChatStore } from '@/stores/chat'
import { useUserStore } from '@/stores/user'
import { useDisplayStore } from '@/stores/display'
import { useUtilsStore } from '@/stores/utils'
import { useCharacterCardStore } from '@/stores/characterCard'
import { useChannelCharacterSnapshotStore } from '@/stores/channelCharacterSnapshot'
import { createChannelExtensionClient } from '@/bridge/channelExtensionClient'
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver'
import { buildBridgeMessagePayload } from '@/bridge/sealchatBridgeSerializer'
import { loadUICustomizations } from './api'
import { createUICustomizationRuntime } from './cssRuntime'
import { createBlobModuleImporter, createScriptRuntime, type NotifyType, type UICustomizationEvent } from './jsRuntime'
import { hasSafeUIParameter, resolveCustomizationContext } from './resolver'
import { createScriptController, type PendingWorldScript } from './scripts'
import { createTrustStore, UI_CUSTOMIZATION_TRUST_KEY, type TrustStore } from './trust'

export const UI_CUSTOMIZATION_REFRESH_EVENT = 'sealchat:ui-customization-refresh'
export const UI_CUSTOMIZATION_SAFE_SESSION_KEY = 'sealchat.uiCustomization.safeSession.v1'

const safeSessionStorage = () => {
  try { return typeof window === 'undefined' ? null : window.sessionStorage } catch { return null }
}

export function isUICustomizationSafeSession() {
  try { return safeSessionStorage()?.getItem(UI_CUSTOMIZATION_SAFE_SESSION_KEY) === '1' } catch { return false }
}

export function setUICustomizationSafeSession(disabled: boolean) {
  try { safeSessionStorage()?.setItem(UI_CUSTOMIZATION_SAFE_SESSION_KEY, disabled ? '1' : '0') } catch { /* private mode */ }
  requestUICustomizationRefresh()
}

/** Lets management pages apply a successful save in the current tab. */
export function requestUICustomizationRefresh() {
  if (typeof window !== 'undefined') window.dispatchEvent(new Event(UI_CUSTOMIZATION_REFRESH_EVENT))
}

const pendingWorldScripts = shallowRef<PendingWorldScript[]>([])
let activeController: ReturnType<typeof createScriptController> | null = null
let trustStore: TrustStore | null = null

const safeLocalStorage = () => {
  try { return window.localStorage } catch { return null }
}

export function getUICustomizationTrustStore(): TrustStore {
  trustStore ||= createTrustStore(typeof window === 'undefined' ? null : safeLocalStorage())
  return trustStore
}

/** State and actions for the world script trust prompt (App-level host). */
export function useUICustomizationTrustPrompt() {
  return {
    pending: pendingWorldScripts,
    trust: (ids: string[]) => activeController?.trust(ids),
    deny: (ids: string[]) => activeController?.deny(ids),
    postpone: (ids: string[]) => activeController?.postpone(ids),
    /** Re-evaluate after decisions were cleared elsewhere (e.g. world settings). */
    reevaluate: () => activeController?.reevaluate(),
  }
}

// In-app paths only: scripts may not use navigate() to leave SealChat.
const isInAppPath = (target: unknown): target is string =>
  typeof target === 'string' && target.startsWith('/') && !target.startsWith('//') && !/[\u0000-\u001f\\]/.test(target)

export function installUICustomizationRuntime({ pinia, router }: { pinia: Pinia; router: Router }) {
  if (typeof document === 'undefined') return null
  const chat = useChatStore(pinia)
  const user = useUserStore(pinia)
  const display = useDisplayStore(pinia)
  const utils = useUtilsStore(pinia)

  // Some routes (notably LOG layout) are authorized by a slug and only learn
  // their world from the runtime response. Keep that server-resolved context
  // for scripts/trust without feeding it back into request construction.
  let resolvedRuntimeContext: { requestKey: string; worldId: string; channelId: string } | null = null
  const runtimeContextKey = (ctx: { surface: string; worldId: string; channelId: string; slug: string }) =>
    `${ctx.surface}\u0000${ctx.worldId}\u0000${ctx.channelId}\u0000${ctx.slug}`
  const effectiveContext = () => {
    const ctx = resolve()
    return resolvedRuntimeContext?.requestKey === runtimeContextKey(ctx)
      ? { ...ctx, worldId: resolvedRuntimeContext.worldId, channelId: resolvedRuntimeContext.channelId }
      : ctx
  }

  const listeners = new Map<UICustomizationEvent, Set<(payload: unknown) => void>>()
  const emit = (event: UICustomizationEvent, payload: () => unknown) => {
    const handlers = listeners.get(event)
    if (!handlers?.size) return
    const value = payload()
    for (const handler of [...handlers]) handler(value)
  }
  let notice: ReturnType<typeof createDiscreteApi>['message'] | null = null
  const notify = (type: NotifyType, text: string) => {
    notice ||= createDiscreteApi(['message'], {
      configProviderProps: computed(() => ({ theme: display.palette === 'night' ? darkTheme : null })),
    }).message
    const method = (['info', 'success', 'warning', 'error'] as const).includes(type) ? type : 'info'
    notice[method](text)
  }
  const scriptRuntime = createScriptRuntime({
    channel(pkg) {
      const ctx = effectiveContext()
      const userId = user.info.id
      const token = user.token
      const key = runtimeContextKey(ctx)
      if (!resolvedRuntimeContext || !ctx.channelId || !ctx.worldId || !userId || String(chat.curChannel?.id || '') !== ctx.channelId || chat.currentWorldId !== ctx.worldId || document.documentElement.dataset.scSafeUi === 'true') return null
      return createChannelExtensionClient({
        chat, user, characterCard: useCharacterCardStore(pinia), characterSnapshot: useChannelCharacterSnapshotStore(pinia),
        scope: { type: 'ui-customization', id: pkg.id }, capabilities: pkg.channelCapabilities || [],
        worldId: ctx.worldId, channelId: ctx.channelId, readOnly: () => effectiveContext().observer,
        isCurrent: () => user.info.id === userId && user.token === token && runtimeContextKey(effectiveContext()) === key && document.documentElement.dataset.scSafeUi !== 'true',
      })
    },
    context: () => {
      const ctx = effectiveContext()
      return { surface: ctx.surface, worldId: ctx.worldId, channelId: ctx.channelId, observer: ctx.observer, palette: display.palette }
    },
    subscribe(event, handler) {
      let set = listeners.get(event)
      if (!set) listeners.set(event, set = new Set())
      set.add(handler)
      return () => { set!.delete(handler) }
    },
    async navigate(target) {
      if (!isInAppPath(target)) throw new TypeError('sc.navigate only accepts SealChat paths starting with "/"')
      await router.push(target)
    },
    notify,
    importModule: createBlobModuleImporter(window),
    reportError(pkg, phase, error) {
      console.error(`[SealChat UI customization] ${pkg.scopeType} package "${pkg.name}" (${pkg.id}) ${phase} failed:`, error)
      if (phase === 'activate') notify('error', `自定义脚本加载失败：${pkg.name}`)
    },
    window,
  })
  const controller = createScriptController({
    runtime: scriptRuntime,
    trust: getUICustomizationTrustStore(),
    getWorldId: () => effectiveContext().worldId,
    onPending: pending => { pendingWorldScripts.value = pending },
  })
  activeController = controller
  const runtime = createUICustomizationRuntime({
    document,
    loadPackages: loadUICustomizations,
    onPackages: (packages, interim, result) => {
      if (interim || !result) {
        resolvedRuntimeContext = null
      } else {
        const requestContext = resolve()
        resolvedRuntimeContext = {
          requestKey: runtimeContextKey(requestContext),
          worldId: result.worldId || '',
          channelId: result.channelId || '',
        }
      }
      void controller.apply(packages, interim)
    },
  })
  // Latch the URL escape hatch before the first request. Navigation guards may
  // redirect and drop query parameters; that must never undo a safe boot.
  let urlSafe = hasSafeUIParameter(window.location)
  try {
    if (window.parent !== window) {
      urlSafe ||= hasSafeUIParameter(window.parent.location)
        || window.parent.document.documentElement.dataset.scSafeUi === 'true'
    }
  } catch { /* A cross-origin embed cannot inspect its parent. */ }
  document.documentElement.dataset.scSafeUi = String(urlSafe || isUICustomizationSafeSession())
  const embedRoute = ref('')
  let ready = false
  let queued = false
  let disposed = false
  const resolve = () => resolveCustomizationContext(router.currentRoute.value, {
    currentWorldId: chat.currentWorldId || '', channelId: String(chat.curChannel?.id || ''),
    observerMode: chat.observerMode, observerWorldId: chat.observerWorldId,
    observerChannelId: chat.observerChannelId, observerSlug: chat.observerSlug, invite: chat.lastConsumedWorldInvite || undefined,
    embedReady: embedRoute.value === router.currentRoute.value.fullPath,
  })
  const refresh = () => {
    if (!ready || disposed) return Promise.resolve()
    const context = resolve()
    urlSafe ||= hasSafeUIParameter(window.location) || router.currentRoute.value.query['safe-ui'] === '1'
    const safe = urlSafe || isUICustomizationSafeSession()
    document.documentElement.dataset.scSurface = context.surface
    document.documentElement.dataset.scObserver = String(context.observer)
    document.documentElement.dataset.scSafeUi = String(safe)
    if (safe) { resolvedRuntimeContext = null; scriptRuntime.invalidateChannelContexts() }
    return runtime.reconcile(context, safe, `${user.info.id || ''}:${user.token || ''}`)
  }
  const scheduleRefresh = () => {
    if (queued || !ready) return
    queued = true
    queueMicrotask(() => { queued = false; void refresh() })
  }
  const stopContext = watch(() => JSON.stringify({
    context: resolve(), user: user.info.id, token: user.token, policy: utils.config?.uiCustomization,
  }), () => { resolvedRuntimeContext = null; scriptRuntime.invalidateChannelContexts(); scheduleRefresh() }, { flush: 'sync' })
  const stopTheme = watch(() => [display.palette, display.layout], () => {
    document.documentElement.dataset.scPalette = display.palette
    document.documentElement.dataset.scLayout = display.layout
  }, { immediate: true, flush: 'post' })
  const stopEmbed = watch(() => [router.currentRoute.value.fullPath, chat.currentWorldId], () => {
    const route = router.currentRoute.value
    if (route.name === 'embed' && chat.currentWorldId && (!route.query.worldId || chat.currentWorldId === route.query.worldId)) {
      embedRoute.value = route.fullPath
    }
  }, { immediate: true, flush: 'sync' })
  // Lifecycle events for running scripts. Packages that no longer match are
  // disposed by reconcile; scripts that stay active observe the change here.
  const stopEvents = watch(() => {
    const ctx = resolve()
    return [ctx.surface, ctx.worldId, ctx.channelId, display.palette] as const
  }, ([surface, worldId, channelId, palette], previous) => {
    if (!previous) return
    if (surface !== previous[0]) emit('surface-change', () => ({ surface, previous: previous[0] }))
    if (worldId !== previous[1]) emit('world-change', () => ({ worldId, previous: previous[1] }))
    if (channelId !== previous[2]) emit('channel-change', () => ({ channelId, previous: previous[2] }))
    if (palette !== previous[3]) emit('theme-change', () => ({ palette, previous: previous[3] }))
  }, { immediate: true })
  // Message events reuse the gateway chatEvent stream and the stable Bridge
  // serializer; scripts never see raw store objects.
  const messageHandlers = (['message-created', 'message-updated', 'message-deleted'] as const).map(name => {
    const handler = (event?: any) => emit(name, () => buildBridgeMessagePayload({
      event: name,
      worldId: String(chat.currentWorldId || ''),
      channelId: String(event?.channel?.id || chat.curChannel?.id || ''),
      message: event?.message || {},
      resolveAttachmentUrl,
    }))
    chatEvent.on(name, handler)
    return [name, handler] as const
  })
  const stopNavigation = router.afterEach((_to, _from, failure) => { if (!failure) scheduleRefresh() })
  // Management clients can refresh immediately after a successful API write.
  const handleRefresh = () => { void refresh() }
  // Trust decisions made in another tab apply here without a reload. A
  // session-safe change also reaches same-origin child frames in this tab via
  // the sessionStorage `storage` event, so split/theater embeds stop at once.
  const handleStorage = (event: StorageEvent) => {
    if (event.key === UI_CUSTOMIZATION_TRUST_KEY) void controller.reevaluate()
    if (event.key === UI_CUSTOMIZATION_SAFE_SESSION_KEY) void refresh()
  }
  window.addEventListener(UI_CUSTOMIZATION_REFRESH_EVENT, handleRefresh)
  window.addEventListener('storage', handleStorage)
  void router.isReady().then(() => { if (!disposed) { ready = true; return refresh() } })
  return {
    refresh,
    dispose() {
      ready = false
      disposed = true
      stopContext(); stopTheme(); stopEmbed(); stopEvents(); stopNavigation()
      for (const [name, handler] of messageHandlers) chatEvent.off(name, handler)
      listeners.clear()
      window.removeEventListener(UI_CUSTOMIZATION_REFRESH_EVENT, handleRefresh)
      window.removeEventListener('storage', handleStorage)
      runtime.dispose()
      void controller.dispose()
      if (activeController === controller) activeController = null
    },
  }
}
