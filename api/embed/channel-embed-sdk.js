/* SealChat Channel Embed SDK v1. Served by SealChat; do not vendor this file. */
(function (global) {
  'use strict'

  const CHANNEL_EMBED_VERSION = 1
  const CHANNEL_EMBED_HANDSHAKE = 'sealchat.embed.handshake'
  const CHANNEL_EMBED_HANDSHAKE_ACK = 'sealchat.embed.handshake.ack'
  const CHANNEL_EMBED_REQUEST = 'sealchat.embed.request'
  const CHANNEL_EMBED_RESPONSE = 'sealchat.embed.response'
  const CHANNEL_EMBED_EVENT = 'sealchat.embed.event'

  const randomEmbedId = prefix => {
    const cryptoObject = global.crypto
    if (cryptoObject && cryptoObject.randomUUID) return `${prefix}_${cryptoObject.randomUUID()}`
    return `${prefix}_${Date.now().toString(36)}_${Math.random().toString(36).slice(2)}`
  }

  class SealChatEmbedError extends Error {
    constructor(error) {
      super(error.message)
      this.name = 'SealChatEmbedError'
      this.code = error.code
      this.details = error.details
    }
  }

  class ChannelEmbedClient {
    constructor(port, sessionId, contextVersion) {
      this.port = port
      this.sessionId = sessionId
      this.contextVersion = contextVersion
      this.requestCounter = 0
      this.pending = new Map()
      this.handlers = new Map()
      this.closedHandlers = new Set()
      this.storageSeq = 0
      this.storageResyncing = false
      this.closed = false
      this.context = {
        get: () => this.request('context.get'),
        onChanged: handler => this.on('context.changed', handler)
      }
      this.user = { getCurrent: () => this.request('user.getCurrent') }
      this.member = { getCurrent: () => this.request('member.getCurrent') }
      this.members = {
        // scope: online | guild | world-admins
        list: params => this.request('members.list', params),
        onChanged: handler => this.on('members.changed', handler)
      }
      this.characters = {
        list: () => this.request('characters.list'),
        getCurrent: () => this.request('characters.getCurrent'),
        onChanged: handler => this.on('characters.changed', handler)
      }
      this.characterCard = {
        getStatus: () => this.request('characterCard.getStatus'),
        getCurrent: () => this.request('characterCard.getCurrent'),
        listSnapshots: () => this.request('characterCard.listSnapshots'),
        getSnapshot: params => this.request('characterCard.getSnapshot', params),
        updateAttrs: attrsPatch => this.request('characterCard.updateAttrs', { attrs: attrsPatch })
      }
      this.permissions = {
        getCurrent: () => this.request('permissions.getCurrent'),
        onChanged: handler => this.on('permissions.changed', handler)
      }
      this.channel = { getState: () => this.request('channel.getState') }
      this.connection = {
        getState: () => this.request('connection.getState'),
        onChanged: handler => this.on('connection.changed', handler)
      }
      this.session = { onClosed: handler => this.onClosed(handler) }
      this.storage = {
        get: key => this.request('storage.get', { key }),
        set: (key, value, options) => this.request('storage.set', { key, value, ifRevision: options && options.ifRevision }),
        delete: (key, options) => this.request('storage.delete', { key, ifRevision: options && options.ifRevision }),
        list: options => this.request('storage.list', options || {}),
        snapshot: () => this.request('storage.snapshot'),
        onChanged: handler => this.on('storage.changed', handler)
      }
      this.events = {
        publish: (topic, payload) => this.request('events.publish', { topic, payload }),
        on: (topic, handler) => {
          void this.request('events.subscribe', { topic }).catch(() => undefined)
          return this.on(`event:${topic}`, handler)
        },
        off: (topic, handler) => this.off(`event:${topic}`, handler)
      }
      this.messages = { send: params => this.request('messages.send', params) }
      port.onmessage = message => this.handleMessage(message.data)
      if (port.start) port.start()
    }

    handleMessage(value) {
      if (!value || typeof value !== 'object' || value.version !== CHANNEL_EMBED_VERSION) return
      if (value.type === CHANNEL_EMBED_RESPONSE && value.sessionId === this.sessionId && typeof value.requestId === 'string') {
        const pending = this.pending.get(value.requestId)
        if (!pending) return
        this.pending.delete(value.requestId)
        clearTimeout(pending.timer)
        if (value.ok) pending.resolve(value.result)
        else pending.reject(new SealChatEmbedError(value.error || { code: 'INTERNAL_ERROR', message: 'Embed request failed' }))
        return
      }
      if (value.type !== CHANNEL_EMBED_EVENT || value.sessionId !== this.sessionId || typeof value.topic !== 'string') return
      if (typeof value.contextVersion === 'number') this.contextVersion = value.contextVersion
      if (value.topic === 'session.closed') {
        this.close(value)
        return
      }
      if (value.topic === 'storage.changed' && Number.isInteger(value.seq)) {
        const seq = Number(value.seq)
        const payload = value.payload || {}
        if (seq <= this.storageSeq) return
        if (payload.snapshot) {
          this.storageSeq = seq
          this.emit('storage.changed', { kind: 'resynced', seq, snapshot: payload.snapshot }, value)
          return
        }
        if (seq > this.storageSeq + 1) {
          if (!this.storageResyncing) {
            this.storageResyncing = true
            void this.request('storage.snapshot').then(snapshot => {
              this.storageSeq = Number.isFinite(Number(snapshot && snapshot.seq)) ? Number(snapshot.seq) : seq
              this.emit('storage.changed', { kind: 'resynced', seq: this.storageSeq, snapshot }, value)
            }).catch(() => undefined).finally(() => { this.storageResyncing = false })
          }
          return
        }
        this.storageSeq = seq
        this.emit('storage.changed', { kind: payload.op === 'delete' ? 'delete' : 'set', seq, ...payload }, value)
        return
      }
      if (typeof global.CustomEvent === 'function') {
        global.dispatchEvent(new CustomEvent('sealchat:embed:event', { detail: value }))
      }
      this.emit(value.topic, value.payload, value)
    }

    emit(topic, payload, event) {
      const handlers = this.handlers.get(topic)
      if (handlers) handlers.forEach(handler => handler(payload, event))
      const allHandlers = this.handlers.get('*')
      if (allHandlers) allHandlers.forEach(handler => handler(payload, event))
    }

    request(method, params, timeoutMs = 10_000) {
      if (this.closed) return Promise.reject(new SealChatEmbedError({ code: 'SESSION_EXPIRED', message: 'Embed session closed' }))
      const requestId = randomEmbedId(`r${++this.requestCounter}`)
      return new Promise((resolve, reject) => {
        const timer = setTimeout(() => {
          this.pending.delete(requestId)
          reject(new SealChatEmbedError({ code: 'TIMEOUT', message: `${method} timeout` }))
        }, timeoutMs)
        this.pending.set(requestId, { resolve, reject, timer })
        try {
          this.port.postMessage({
            type: CHANNEL_EMBED_REQUEST,
            version: CHANNEL_EMBED_VERSION,
            sessionId: this.sessionId,
            requestId,
            method,
            contextVersion: this.contextVersion,
            params
          })
        } catch {
          clearTimeout(timer)
          this.pending.delete(requestId)
          reject(new SealChatEmbedError({ code: 'INVALID_PARAMS', message: 'Request cannot be cloned' }))
        }
      })
    }

    on(topic, handler) {
      const handlers = this.handlers.get(topic) || new Set()
      handlers.add(handler)
      this.handlers.set(topic, handlers)
      return () => this.off(topic, handler)
    }

    off(topic, handler) {
      const handlers = this.handlers.get(topic)
      if (handlers) handlers.delete(handler)
    }

    onClosed(handler) {
      if (this.closed) {
        queueMicrotask(() => handler())
        return () => undefined
      }
      this.closedHandlers.add(handler)
      return () => this.closedHandlers.delete(handler)
    }

    close(event) {
      if (this.closed) return
      this.closed = true
      this.pending.forEach(pending => {
        clearTimeout(pending.timer)
        pending.reject(new SealChatEmbedError({ code: 'SESSION_EXPIRED', message: 'Embed session closed' }))
      })
      this.pending.clear()
      this.closedHandlers.forEach(handler => {
        try { handler(event) } catch (_) { /* subscriber errors must not break cleanup */ }
      })
      this.closedHandlers.clear()
      this.port.close()
    }
  }

  const SealChatEmbed = {
    connect(options) {
      if (global.parent === global) return Promise.reject(new SealChatEmbedError({ code: 'HANDSHAKE_FAILED', message: 'Embed must run inside iframe' }))
      const opts = options || {}
      const nonce = randomEmbedId('nonce')
      const targetOrigin = opts.targetOrigin || '*'
      const timeoutMs = Math.max(1, opts.timeoutMs || 10_000)
      return new Promise((resolve, reject) => {
        let settled = false
        const cleanup = () => {
          global.removeEventListener('message', onMessage)
          clearTimeout(timer)
          clearInterval(retryTimer)
        }
        const timer = setTimeout(() => {
          if (settled) return
          settled = true
          cleanup()
          reject(new SealChatEmbedError({ code: 'HANDSHAKE_FAILED', message: 'Embed handshake timeout' }))
        }, timeoutMs)
        const retryTimer = setInterval(() => {
          if (!settled) global.parent.postMessage({ type: CHANNEL_EMBED_HANDSHAKE, version: CHANNEL_EMBED_VERSION, nonce }, targetOrigin)
        }, 500)
        const onMessage = event => {
          if (event.source !== global.parent ||
              (targetOrigin !== '*' && event.origin !== targetOrigin) ||
              !event.data || event.data.type !== CHANNEL_EMBED_HANDSHAKE_ACK ||
              event.data.version !== CHANNEL_EMBED_VERSION || event.data.nonce !== nonce) return
          if (settled) return
          settled = true
          cleanup()
          if (!event.data.ok || !event.ports[0] || !event.data.sessionId) {
            reject(new SealChatEmbedError(event.data.error || { code: 'HANDSHAKE_FAILED', message: 'Embed handshake rejected' }))
            return
          }
          resolve(new ChannelEmbedClient(event.ports[0], event.data.sessionId, event.data.contextVersion || 0))
        }
        global.addEventListener('message', onMessage)
        global.parent.postMessage({ type: CHANNEL_EMBED_HANDSHAKE, version: CHANNEL_EMBED_VERSION, nonce }, targetOrigin)
      })
    }
  }

  global.SealChatEmbed = SealChatEmbed
  global.SealChatEmbedError = SealChatEmbedError
})(window)
