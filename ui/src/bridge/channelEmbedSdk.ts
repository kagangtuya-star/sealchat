import {
  CHANNEL_EMBED_EVENT,
  CHANNEL_EMBED_HANDSHAKE,
  CHANNEL_EMBED_HANDSHAKE_ACK,
  CHANNEL_EMBED_REQUEST,
  CHANNEL_EMBED_RESPONSE,
  type EmbedErrorCode,
  type EmbedEvent,
  type EmbedHandshakeAck,
  randomEmbedId,
} from './channelEmbedProtocol'

export class SealChatEmbedError extends Error {
  code: EmbedErrorCode | string
  details?: unknown
  constructor(error: { code: EmbedErrorCode | string; message: string; details?: unknown }) {
    super(error.message)
    this.name = 'SealChatEmbedError'
    this.code = error.code
    this.details = error.details
  }
}

type EventHandler = (payload: any, event?: EmbedEvent) => void
type ClosedHandler = (event?: EmbedEvent) => void

export type EmbedMemberListScope = 'online' | 'guild' | 'world-admins'
export type EmbedWorldRole = 'owner' | 'admin' | 'member' | 'spectator'
export interface EmbedSafeMember {
  userId: string
  displayName: string
  avatar?: string
}
export interface EmbedWorldAdmin {
  userId: string
  displayName: string
  avatar?: string
  role: 'owner' | 'admin'
}
export interface EmbedPermissionSummary {
  canSendMessage: boolean
  canReadMembers: boolean
  canReadCharacters: boolean
  canReadStorage: boolean
  canWriteStorage: boolean
  canPublishEvents: boolean
  worldRole: EmbedWorldRole | null
  isWorldOwner: boolean
  isWorldAdmin: boolean
  isSystemAdmin: boolean
  canManageWorld: boolean
}

export class ChannelEmbedClient {
  private readonly port: MessagePort
  private readonly sessionId: string
  private contextVersion: number
  private requestCounter = 0
  private pending = new Map<string, { resolve: (value: any) => void; reject: (reason: unknown) => void; timer: ReturnType<typeof setTimeout> }>()
  private handlers = new Map<string, Set<EventHandler>>()
  private closedHandlers = new Set<ClosedHandler>()
  private storageSeq = 0
  private storageResyncing = false
  private closed = false

  constructor(port: MessagePort, sessionId: string, contextVersion: number) {
    this.port = port
    this.sessionId = sessionId
    this.contextVersion = contextVersion
    port.onmessage = (message) => this.handleMessage(message.data)
    port.start?.()
  }

  private handleMessage(value: unknown) {
    if (!value || typeof value !== 'object') return
    const data = value as Record<string, any>
    if (data.version !== 1) return
    if (data.type === CHANNEL_EMBED_RESPONSE && data.sessionId === this.sessionId && typeof data.requestId === 'string') {
      const pending = this.pending.get(data.requestId)
      if (!pending) return
      this.pending.delete(data.requestId)
      clearTimeout(pending.timer)
      if (data.ok) pending.resolve(data.result)
      else pending.reject(new SealChatEmbedError(data.error || { code: 'INTERNAL_ERROR', message: 'Embed request failed' }))
      return
    }
    if (data.type === CHANNEL_EMBED_EVENT && data.sessionId === this.sessionId && typeof data.topic === 'string') {
      if (typeof (data as any).contextVersion === 'number') this.contextVersion = (data as any).contextVersion
      if (data.topic === 'session.closed') {
        this.close(data as EmbedEvent)
        return
      }
      if (data.topic === 'storage.changed' && Number.isInteger(data.seq)) {
        const seq = Number(data.seq)
        const payload = data.payload as Record<string, any> | undefined
        if (seq <= this.storageSeq) return
        if (payload?.snapshot) {
          this.storageSeq = seq
          this.emit('storage.changed', { kind: 'resynced', seq, snapshot: payload.snapshot }, data as EmbedEvent)
          return
        }
        if (seq > this.storageSeq + 1) {
          if (!this.storageResyncing) {
            this.storageResyncing = true
            void this.request('storage.snapshot').then((snapshot: any) => {
              const snapshotSeq = Number(snapshot?.seq || seq)
              this.storageSeq = Number.isFinite(snapshotSeq) ? snapshotSeq : seq
              this.emit('storage.changed', { kind: 'resynced', seq: this.storageSeq, snapshot }, data as EmbedEvent)
            }).catch(() => {
              // Host may also deliver an authoritative snapshot event.
            }).finally(() => { this.storageResyncing = false })
          }
          return
        }
        this.storageSeq = seq
        const kind = payload?.op === 'delete' ? 'delete' : 'set'
        this.emit('storage.changed', { kind, seq, ...payload }, data as EmbedEvent)
        return
      }
      if (typeof window !== 'undefined' && typeof CustomEvent !== 'undefined') window.dispatchEvent(new CustomEvent('sealchat:embed:event', { detail: data }))
      this.emit(data.topic, (data as EmbedEvent).payload, data as EmbedEvent)
    }
  }

  private emit(topic: string, payload: any, event?: EmbedEvent) {
    this.handlers.get(topic)?.forEach((handler) => handler(payload, event))
    this.handlers.get('*')?.forEach((handler) => handler(payload, event))
  }

  request<T = unknown>(method: string, params?: unknown, timeoutMs = 10_000): Promise<T> {
    if (this.closed) return Promise.reject(new SealChatEmbedError({ code: 'SESSION_EXPIRED', message: 'Embed session closed' }))
    const requestId = randomEmbedId(`r${++this.requestCounter}`)
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(requestId)
        reject(new SealChatEmbedError({ code: 'TIMEOUT', message: `${method} timeout` }))
      }, timeoutMs)
      this.pending.set(requestId, { resolve, reject, timer })
      try {
        this.port.postMessage({ type: CHANNEL_EMBED_REQUEST, version: 1, sessionId: this.sessionId, requestId, method, contextVersion: this.contextVersion, params })
      } catch {
        clearTimeout(timer)
        this.pending.delete(requestId)
        reject(new SealChatEmbedError({ code: 'INVALID_PARAMS', message: 'Request cannot be cloned' }))
      }
    })
  }

  on(topic: string, handler: EventHandler) { const set = this.handlers.get(topic) || new Set<EventHandler>(); set.add(handler); this.handlers.set(topic, set); return () => this.off(topic, handler) }
  off(topic: string, handler: EventHandler) { this.handlers.get(topic)?.delete(handler) }
  onClosed(handler: ClosedHandler) {
    if (this.closed) {
      queueMicrotask(() => handler())
      return () => undefined
    }
    this.closedHandlers.add(handler)
    return () => this.closedHandlers.delete(handler)
  }
  close(event?: EmbedEvent) {
    if (this.closed) return
    this.closed = true
    this.pending.forEach((pending) => { clearTimeout(pending.timer); pending.reject(new SealChatEmbedError({ code: 'SESSION_EXPIRED', message: 'Embed session closed' })) })
    this.pending.clear()
    this.closedHandlers.forEach((handler) => {
      try { handler(event) } catch { /* subscriber errors must not break cleanup */ }
    })
    this.closedHandlers.clear()
    this.port.close()
  }

  readonly context = { get: () => this.request('context.get'), onChanged: (handler: EventHandler) => this.on('context.changed', handler) }
  readonly user = { getCurrent: () => this.request('user.getCurrent') }
  readonly member = { getCurrent: () => this.request('member.getCurrent') }
  readonly members = { list: (params: { scope: EmbedMemberListScope; cursor?: string }) => this.request<EmbedSafeMember[] | EmbedWorldAdmin[]>('members.list', params), onChanged: (handler: EventHandler) => this.on('members.changed', handler) }
  readonly characters = { list: () => this.request('characters.list'), getCurrent: () => this.request('characters.getCurrent'), onChanged: (handler: EventHandler) => this.on('characters.changed', handler) }
  readonly permissions = { getCurrent: () => this.request<EmbedPermissionSummary>('permissions.getCurrent'), onChanged: (handler: EventHandler) => this.on('permissions.changed', handler) }
  readonly channel = { getState: () => this.request('channel.getState') }
  readonly connection = { getState: () => this.request('connection.getState'), onChanged: (handler: EventHandler) => this.on('connection.changed', handler) }
  readonly session = { onClosed: (handler: ClosedHandler) => this.onClosed(handler) }
  readonly storage = {
    get: (key: string) => this.request('storage.get', { key }),
    set: (key: string, value: unknown, options?: { ifRevision?: number }) => this.request('storage.set', { key, value, ifRevision: options?.ifRevision }),
    delete: (key: string, options?: { ifRevision?: number }) => this.request('storage.delete', { key, ifRevision: options?.ifRevision }),
    list: (options?: { prefix?: string; cursor?: string }) => this.request('storage.list', options || {}),
    snapshot: () => this.request('storage.snapshot'),
    onChanged: (handler: EventHandler) => this.on('storage.changed', handler),
  }
  readonly events = {
    publish: (topic: string, payload: unknown) => this.request('events.publish', { topic, payload }),
    on: (topic: string, handler: EventHandler) => {
      void this.request('events.subscribe', { topic }).catch(() => undefined)
      return this.on(`event:${topic}`, handler)
    },
    off: (topic: string, handler: EventHandler) => this.off(`event:${topic}`, handler),
  }
  readonly messages = { send: (params: { text: string; replyTo?: string; identityId?: string; identityVariantId?: string; icMode?: 'ic' | 'ooc' }) => this.request('messages.send', params) }
}

export const SealChatEmbed = {
  connect(options?: { timeoutMs?: number; targetOrigin?: string }): Promise<ChannelEmbedClient> {
    if (typeof window === 'undefined' || window.parent === window) return Promise.reject(new SealChatEmbedError({ code: 'HANDSHAKE_FAILED', message: 'Embed must run inside iframe' }))
    const nonce = randomEmbedId('nonce')
    const targetOrigin = options?.targetOrigin || '*'
    const timeoutMs = Math.max(1, options?.timeoutMs || 10_000)
    return new Promise((resolve, reject) => {
      let settled = false
      const cleanup = () => { window.removeEventListener('message', onMessage); clearTimeout(timer); clearInterval(retryTimer) }
      const timer = setTimeout(() => { if (settled) return; settled = true; cleanup(); reject(new SealChatEmbedError({ code: 'HANDSHAKE_FAILED', message: 'Embed handshake timeout' })) }, timeoutMs)
      const retryTimer = setInterval(() => {
        if (!settled) window.parent.postMessage({ type: CHANNEL_EMBED_HANDSHAKE, version: 1, nonce }, targetOrigin)
      }, 500)
      const onMessage = (event: MessageEvent) => {
        if (event.source !== window.parent || (targetOrigin !== '*' && event.origin !== targetOrigin) || !event.data || event.data.type !== CHANNEL_EMBED_HANDSHAKE_ACK || event.data.version !== 1 || event.data.nonce !== nonce) return
        const ack = event.data as EmbedHandshakeAck
        if (settled) return
        settled = true
        cleanup()
        if (!ack.ok || !event.ports[0] || !ack.sessionId) { reject(new SealChatEmbedError(ack.error || { code: 'HANDSHAKE_FAILED', message: 'Embed handshake rejected' })); return }
        resolve(new ChannelEmbedClient(event.ports[0], ack.sessionId, ack.contextVersion || 0))
      }
      window.addEventListener('message', onMessage)
      window.parent.postMessage({ type: CHANNEL_EMBED_HANDSHAKE, version: 1, nonce }, targetOrigin)
    })
  },
}
