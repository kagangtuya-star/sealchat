import type { UIRuntimeSurface } from './surfaces'
import type { UICustomizationScopeType } from './types'
import type { ChannelEmbedClient } from '@/bridge/channelEmbedSdk'
import { runtimeFingerprint } from './fingerprint'

// Custom scripts run in the same origin as SealChat. Blob modules only give
// each package its own module scope and lifecycle; they are NOT a sandbox.

export const UI_CUSTOMIZATION_API_VERSION = 2 as const

export const UI_CUSTOMIZATION_EVENTS = [
  'surface-change', 'world-change', 'channel-change', 'theme-change',
  'message-created', 'message-updated', 'message-deleted',
] as const
export type UICustomizationEvent = typeof UI_CUSTOMIZATION_EVENTS[number]

export type NotifyType = 'info' | 'success' | 'warning' | 'error'
type Cleanup = () => void | Promise<void>

export interface UICustomizationScriptContext {
  surface: UIRuntimeSurface
  worldId: string
  channelId: string
  observer: boolean
  palette: 'day' | 'night'
}

/** The stable, versioned API handed to package scripts as `sc`. */
export interface SealChatCustomizationAPI {
  readonly apiVersion: typeof UI_CUSTOMIZATION_API_VERSION
  readonly packageId: string
  context(): UICustomizationScriptContext
  channel(): ChannelEmbedClient | null
  cleanup(fn: Cleanup): void
  on(event: UICustomizationEvent, handler: (payload: unknown) => void): () => void
  navigate(target: string): Promise<void>
  notify(type: NotifyType, message: string): void
  // Auto-cleaned conveniences for resources scripts commonly forget.
  listen(target: EventTarget, type: string, handler: EventListenerOrEventListenerObject, options?: boolean | AddEventListenerOptions): () => void
  timeout(handler: () => void, ms?: number): () => void
  interval(handler: () => void, ms: number): () => void
  observe(target: Node, options: MutationObserverInit, callback: MutationCallback): () => void
}

export interface ScriptPackage {
  id: string
  scopeType: UICustomizationScopeType
  name: string
  js: string
  jsHash: string
  runtimeFingerprint?: string
  channelCapabilities?: string[]
}

export interface ScriptHost {
  context(): UICustomizationScriptContext
  channel?(pkg: ScriptPackage): ChannelEmbedClient | null
  subscribe(event: UICustomizationEvent, handler: (payload: unknown) => void): () => void
  navigate(target: string): Promise<void>
  notify(type: NotifyType, message: string): void
  importModule(source: string): Promise<{ default?: unknown }>
  reportError(pkg: ScriptPackage, phase: 'activate' | 'cleanup' | 'event', error: unknown): void
  window?: Pick<Window, 'setTimeout' | 'clearTimeout' | 'setInterval' | 'clearInterval'> & { MutationObserver?: typeof MutationObserver }
}

interface ActiveScript {
  pkg: ScriptPackage
  cleanups: Cleanup[]
  disposed: boolean
  superseded?: boolean
  channel?: ChannelEmbedClient | null
}

// A body that already uses module syntax is loaded as-is; otherwise it is
// wrapped so authors may write a plain activate body that receives `sc`.
export function wrapScriptModule(js: string): string {
  if (/^\s*(?:export|import)\s/m.test(js)) return js
  return `export default async function activate(sc) {\n${js}\n}\n`
}

export function createBlobModuleImporter(win: { URL: typeof URL } = window) {
  return async (source: string) => {
    const url = win.URL.createObjectURL(new Blob([source], { type: 'text/javascript' }))
    try {
      return await import(/* @vite-ignore */ url)
    } finally {
      win.URL.revokeObjectURL(url)
    }
  }
}

export function createScriptRuntime(host: ScriptHost) {
  const active = new Map<string, ActiveScript>()
  let queue: Promise<void> = Promise.resolve()
  let latest: ScriptPackage[] | null = null
  let disposed = false
  let invalidatingChannels = false
  const timers = host.window || globalThis

  const runCleanups = async (entry: ActiveScript) => {
    entry.disposed = true
    entry.channel?.close()
    entry.channel = null
    // LIFO, and one failing cleanup never blocks the rest.
    while (entry.cleanups.length) {
      const fn = entry.cleanups.pop()!
      try { await fn() } catch (error) { host.reportError(entry.pkg, 'cleanup', error) }
    }
  }

  const createAPI = (entry: ActiveScript): SealChatCustomizationAPI => {
    const register = (fn: Cleanup) => {
      if (entry.disposed) {
        // Late registrations (e.g. after an awaited activate) run immediately.
        void Promise.resolve().then(fn).catch(error => host.reportError(entry.pkg, 'cleanup', error))
        return () => undefined
      }
      entry.cleanups.push(fn)
      return () => {
        const index = entry.cleanups.indexOf(fn)
        if (index >= 0) entry.cleanups.splice(index, 1)
      }
    }
    const own = (release: () => void) => {
      let done = false
      const once = () => { if (!done) { done = true; release() } }
      const unregister = register(once)
      return () => { unregister(); once() }
    }
    return Object.freeze({
      apiVersion: UI_CUSTOMIZATION_API_VERSION,
      packageId: entry.pkg.id,
      context: () => ({ ...host.context() }),
      channel: () => {
        if (entry.disposed || invalidatingChannels) return null
        if (!entry.channel) {
          entry.channel = host.channel?.(entry.pkg) || null
          const client = entry.channel
          client?.onClosed(() => { if (entry.channel === client) entry.channel = null })
        }
        return entry.channel
      },
      cleanup(fn: Cleanup) {
        if (typeof fn !== 'function') throw new TypeError('sc.cleanup expects a function')
        register(fn)
      },
      on(event: UICustomizationEvent, handler: (payload: unknown) => void) {
        if (!UI_CUSTOMIZATION_EVENTS.includes(event)) throw new TypeError(`Unknown SealChat event: ${String(event)}`)
        if (entry.disposed) return () => undefined
        return own(host.subscribe(event, payload => {
          if (entry.disposed) return
          try { handler(payload) } catch (error) { host.reportError(entry.pkg, 'event', error) }
        }))
      },
      navigate: (target: string) => host.navigate(target),
      notify: (type: NotifyType, message: string) => host.notify(type, String(message)),
      listen(target, type, handler, options) {
        target.addEventListener(type, handler, options)
        return own(() => target.removeEventListener(type, handler, options))
      },
      timeout(handler, ms) {
        const id = timers.setTimeout(handler, ms)
        return own(() => timers.clearTimeout(id))
      },
      interval(handler, ms) {
        const id = timers.setInterval(handler, ms)
        return own(() => timers.clearInterval(id))
      },
      observe(target, options, callback) {
        const Observer = host.window?.MutationObserver || globalThis.MutationObserver
        const observer = new Observer(callback)
        observer.observe(target, options)
        return own(() => observer.disconnect())
      },
    } satisfies SealChatCustomizationAPI)
  }

  const activate = async (pkg: ScriptPackage) => {
    const entry: ActiveScript = { pkg, cleanups: [], disposed: false }
    active.set(pkg.id, entry)
    try {
      const mod = await host.importModule(wrapScriptModule(pkg.js))
      if (entry.disposed) return
      if (typeof mod?.default === 'function') {
        const dispose = await (mod.default as (sc: SealChatCustomizationAPI) => unknown)(createAPI(entry))
        if (typeof dispose === 'function') {
          if (entry.disposed) await Promise.resolve().then(dispose as Cleanup).catch(error => host.reportError(pkg, 'cleanup', error))
          else entry.cleanups.unshift(dispose as Cleanup) // returned dispose runs last
        }
      }
    } catch (error) {
      // Keep the failed entry so it is not retried until its hash changes,
      // but release whatever it registered before throwing.
      host.reportError(pkg, 'activate', error)
      await runCleanups(entry)
    }
  }

  const sync = async (desired: ScriptPackage[]) => {
    const wanted = new Map(desired.map(p => [p.id, p]))
    // Unload in reverse installation order (world before platform).
    for (const [id, entry] of [...active].reverse()) {
      const next = wanted.get(id)
      if (entry.superseded || !next || runtimeFingerprint(next) !== runtimeFingerprint(entry.pkg) || next.js !== entry.pkg.js) {
        active.delete(id)
        await runCleanups(entry)
      }
    }
    for (const pkg of desired) {
      if (disposed || latest !== desired) return
      if (!active.has(pkg.id)) await activate(pkg)
    }
  }

  return {
    /** Desired executable scripts, already trust-filtered and ordered. */
    reconcile(packages: ScriptPackage[]): Promise<void> {
      if (disposed) return queue
      const desired = packages.filter(p => p.js)
      const wanted = new Map(desired.map(p => [p.id, p]))
      for (const [id, entry] of active) {
        const next = wanted.get(id)
        if (!next || runtimeFingerprint(next) !== runtimeFingerprint(entry.pkg) || next.js !== entry.pkg.js) {
          entry.superseded = true
          entry.disposed = true
          entry.channel?.close()
          entry.channel = null
        }
      }
      latest = desired
      // Serialized: each run disposes before activating; superseded runs stop early.
      queue = queue.then(() => (latest === desired && !disposed ? sync(desired) : undefined))
      return queue
    },
    activeIds: () => [...active].filter(([, e]) => !e.disposed).map(([id]) => id),
    invalidateChannelContexts() {
      invalidatingChannels = true
      try {
        for (const entry of active.values()) {
          const client = entry.channel
          entry.channel = null
          client?.close()
        }
      } finally { invalidatingChannels = false }
    },
    dispose(): Promise<void> {
      if (disposed) return queue
      for (const entry of active.values()) { entry.disposed = true; entry.channel?.close(); entry.channel = null }
      latest = []
      const done = queue.then(() => sync([]))
      disposed = true
      queue = done
      return done
    },
  }
}
