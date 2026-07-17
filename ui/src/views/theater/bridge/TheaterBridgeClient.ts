import {
  createTheaterBridgeMessage,
  parseTheaterBridgeMessage,
  type BridgeEndpoint,
  type BridgeErrorResult,
  type BridgeKind,
  type TheaterBridgeContext,
  type TheaterBridgeMessage,
} from './theater-bridge-protocol'
import type { BridgeTransport } from './theater-bridge-transport'

type MessageHandler<T = unknown, R = unknown> = (
  payload: T,
  message: TheaterBridgeMessage<T>,
) => R | Promise<R>

interface PendingRequest {
  resolve: (payload: unknown) => void
  reject: (error: Error) => void
  timeout: ReturnType<typeof setTimeout>
  target: BridgeEndpoint
  resultName: string
}

export interface BridgeDebugEntry {
  timestamp: number
  direction: 'send' | 'receive' | 'reject' | 'lifecycle'
  endpoint: BridgeEndpoint
  name: string
  detail?: unknown
}

export class TheaterBridgeRequestError extends Error {
  constructor(
    public readonly code: string,
    message: string,
    public readonly details?: unknown,
  ) {
    super(message)
    this.name = 'TheaterBridgeRequestError'
  }
}

interface TheaterBridgeClientOptions {
  endpoint: BridgeEndpoint
  context: TheaterBridgeContext
  transport: BridgeTransport
  capabilities?: readonly string[]
  requestTimeoutMs?: number
  debug?: boolean | (() => boolean)
  maxDebugEntries?: number
}

export class TheaterBridgeClient {
  private connected = false
  private unsubscribeTransport: (() => void) | null = null
  private pendingRequests = new Map<string, PendingRequest>()
  private commandHandlers = new Map<string, MessageHandler>()
  private systemHandlers = new Map<string, MessageHandler>()
  private eventHandlers = new Map<string, Set<MessageHandler>>()
  private remoteCapabilities = new Map<BridgeEndpoint, Set<string>>()
  private debugEntries: BridgeDebugEntry[] = []
  private readonly requestTimeoutMs: number
  private readonly maxDebugEntries: number
  readonly capabilities: readonly string[]

  constructor(private readonly options: TheaterBridgeClientOptions) {
    this.requestTimeoutMs = options.requestTimeoutMs || 8_000
    this.maxDebugEntries = options.maxDebugEntries || 200
    this.capabilities = options.capabilities || []
  }

  async connect() {
    if (this.connected) return
    this.unsubscribeTransport = this.options.transport.subscribe((message) => {
      void this.handleMessage(message)
    })
    await this.options.transport.connect()
    this.connected = true
    this.log('lifecycle', 'connected')
  }

  disconnect() {
    if (!this.connected && !this.unsubscribeTransport) return
    this.connected = false
    this.unsubscribeTransport?.()
    this.unsubscribeTransport = null
    for (const pending of this.pendingRequests.values()) {
      clearTimeout(pending.timeout)
      pending.reject(new TheaterBridgeRequestError('BRIDGE_DISCONNECTED', 'Theater Bridge disconnected'))
    }
    this.pendingRequests.clear()
    this.options.transport.disconnect()
    this.log('lifecycle', 'disconnected')
  }

  getDebugEntries() {
    return [...this.debugEntries]
  }

  setRemoteCapabilities(endpoint: BridgeEndpoint, capabilities: readonly string[]) {
    this.remoteCapabilities.set(endpoint, new Set(capabilities))
  }

  supports(endpoint: BridgeEndpoint, capability: string) {
    return this.remoteCapabilities.get(endpoint)?.has(capability) === true
  }

  onCommand<T = unknown, R = unknown>(name: string, handler: MessageHandler<T, R>) {
    this.commandHandlers.set(name, handler as MessageHandler)
    return () => this.commandHandlers.delete(name)
  }

  onSystem<T = unknown>(name: string, handler: MessageHandler<T, void>) {
    this.systemHandlers.set(name, handler as MessageHandler)
    return () => this.systemHandlers.delete(name)
  }

  onEvent<T = unknown>(name: string, handler: MessageHandler<T, void>) {
    const handlers = this.eventHandlers.get(name) || new Set<MessageHandler>()
    handlers.add(handler as MessageHandler)
    this.eventHandlers.set(name, handlers)
    return () => handlers.delete(handler as MessageHandler)
  }

  sendSystem<T>(target: BridgeEndpoint, name: string, payload: T) {
    this.send('system', target, name, payload)
  }

  emit<T>(target: BridgeEndpoint, name: string, payload: T) {
    this.send('event', target, name, payload)
  }

  request<TPayload, TResult>(
    target: BridgeEndpoint,
    name: string,
    payload: TPayload,
    timeoutMs = this.requestTimeoutMs,
  ): Promise<TResult> {
    if (!this.supports(target, name)) {
      return Promise.reject(new TheaterBridgeRequestError(
        'CAPABILITY_UNAVAILABLE',
        `${target} does not declare capability ${name}`,
      ))
    }
    const message = createTheaterBridgeMessage(this.options.context, {
      kind: 'command',
      source: this.options.endpoint,
      target,
      name,
      payload,
    })
    return new Promise<TResult>((resolve, reject) => {
      const timeout = setTimeout(() => {
        this.pendingRequests.delete(message.id)
        reject(new TheaterBridgeRequestError('REQUEST_TIMEOUT', `Theater Bridge request timed out: ${name}`))
      }, timeoutMs)
      this.pendingRequests.set(message.id, {
        resolve: resolve as (payload: unknown) => void,
        reject,
        timeout,
        target,
        resultName: `${name}.result`,
      })
      try {
        this.sendMessage(message)
      } catch (error) {
        clearTimeout(timeout)
        this.pendingRequests.delete(message.id)
        reject(error instanceof Error ? error : new Error(String(error)))
      }
    })
  }

  sendResult<T>(request: TheaterBridgeMessage, name: string, payload: T) {
    this.sendMessage(createTheaterBridgeMessage(this.options.context, {
      kind: 'result',
      source: this.options.endpoint,
      target: request.source,
      correlationId: request.id,
      name,
      payload,
    }))
  }

  private send<T>(kind: BridgeKind, target: BridgeEndpoint, name: string, payload: T) {
    this.sendMessage(createTheaterBridgeMessage(this.options.context, {
      kind,
      source: this.options.endpoint,
      target,
      name,
      payload,
    }))
  }

  private sendMessage(message: TheaterBridgeMessage) {
    if (!this.connected) throw new Error('TheaterBridgeClient is not connected')
    const parsed = parseTheaterBridgeMessage(message)
    this.options.transport.send(parsed)
    this.log('send', parsed.name, parsed)
  }

  private async handleMessage(input: TheaterBridgeMessage) {
    let message: TheaterBridgeMessage
    try {
      message = parseTheaterBridgeMessage(input)
      if (
        message.worldId !== this.options.context.worldId
        || message.channelId !== this.options.context.channelId
        || message.sessionId !== this.options.context.sessionId
      ) {
        throw new Error('Theater Bridge context mismatch')
      }
      if (message.target !== this.options.endpoint && message.target !== 'broadcast') return
    } catch (error) {
      this.log('reject', 'invalid-message', error)
      return
    }

    this.log('receive', message.name, message)

    if (message.kind === 'result') {
      const pending = message.correlationId ? this.pendingRequests.get(message.correlationId) : null
      if (!pending || !message.correlationId) return
      if (
        message.name !== pending.resultName
        || (message.source !== pending.target && message.source !== 'host')
      ) {
        this.log('reject', 'unexpected-result', message)
        return
      }
      clearTimeout(pending.timeout)
      this.pendingRequests.delete(message.correlationId)
      const payload = message.payload as Partial<BridgeErrorResult>
      if (payload?.ok === false && payload.error) {
        pending.reject(new TheaterBridgeRequestError(
          payload.error.code,
          payload.error.message,
          payload.error.details,
        ))
      } else {
        pending.resolve(message.payload)
      }
      return
    }

    if (message.kind === 'system') {
      await this.systemHandlers.get(message.name)?.(message.payload, message)
      return
    }

    if (message.kind === 'event') {
      const handlers = this.eventHandlers.get(message.name)
      if (!handlers) return
      await Promise.all([...handlers].map((handler) => handler(message.payload, message)))
      return
    }

    const handler = this.commandHandlers.get(message.name)
    if (!handler) {
      this.sendResult(message, `${message.name}.result`, {
        ok: false,
        error: { code: 'UNSUPPORTED_COMMAND', message: `Unsupported command: ${message.name}` },
      })
      return
    }
    try {
      const result = await handler(message.payload, message)
      this.sendResult(message, `${message.name}.result`, result)
    } catch (error) {
      const requestError = error instanceof TheaterBridgeRequestError
        ? error
        : new TheaterBridgeRequestError(
          'COMMAND_FAILED',
          error instanceof Error ? error.message : String(error),
        )
      this.sendResult(message, `${message.name}.result`, {
        ok: false,
        error: {
          code: requestError.code,
          message: requestError.message,
          details: requestError.details,
        },
      })
    }
  }

  private log(direction: BridgeDebugEntry['direction'], name: string, detail?: unknown) {
    const entry: BridgeDebugEntry = {
      timestamp: Date.now(),
      direction,
      endpoint: this.options.endpoint,
      name,
      detail,
    }
    this.debugEntries.push(entry)
    if (this.debugEntries.length > this.maxDebugEntries) this.debugEntries.shift()
    const debug = typeof this.options.debug === 'function' ? this.options.debug() : this.options.debug
    if (debug) {
      const method = direction === 'reject' ? console.warn : console.info
      method(`[theater-bridge:${this.options.endpoint}] ${direction} ${name}`, detail || '')
    }
  }
}
