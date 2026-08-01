import {
  THEATER_BRIDGE_PROTOCOL,
  THEATER_BRIDGE_MAX_MESSAGE_BYTES,
  parseTheaterBridgeMessage,
  type TheaterBridgeMessage,
} from './theater-bridge-protocol'

export interface BridgeTransport {
  connect(): Promise<void>
  disconnect(): void
  send(message: TheaterBridgeMessage): void
  subscribe(handler: (message: TheaterBridgeMessage) => void): () => void
}

type MessageHandler = (message: TheaterBridgeMessage) => void

const assertMessageSize = (message: unknown) => {
  const serialized = JSON.stringify(message)
  const bytes = new TextEncoder().encode(serialized).byteLength
  if (bytes > THEATER_BRIDGE_MAX_MESSAGE_BYTES) {
    throw new Error(`Theater bridge message exceeds ${THEATER_BRIDGE_MAX_MESSAGE_BYTES} bytes`)
  }
}

export class MemoryTransport implements BridgeTransport {
  private connected = false
  private peer: MemoryTransport | null = null
  private handlers = new Set<MessageHandler>()

  static createPair(): [MemoryTransport, MemoryTransport] {
    const first = new MemoryTransport()
    const second = new MemoryTransport()
    first.peer = second
    second.peer = first
    return [first, second]
  }

  async connect() {
    this.connected = true
  }

  disconnect() {
    this.connected = false
    this.handlers.clear()
  }

  send(message: TheaterBridgeMessage) {
    if (!this.connected || !this.peer?.connected) {
      throw new Error('MemoryTransport is not connected')
    }
    assertMessageSize(message)
    const parsed = parseTheaterBridgeMessage(message)
    queueMicrotask(() => this.peer?.dispatch(parsed))
  }

  subscribe(handler: MessageHandler) {
    this.handlers.add(handler)
    return () => this.handlers.delete(handler)
  }

  private dispatch(message: TheaterBridgeMessage) {
    if (!this.connected) return
    this.handlers.forEach((handler) => handler(message))
  }
}

interface PostMessageTransportOptions {
  receiveWindow: Window
  targetWindow: () => Window | null
  expectedSource: () => MessageEventSource | null
  targetOrigin: string
  expectedOrigin: string
  maxMessagesPerSecond?: number
  onRejected?: (reason: string, error?: unknown) => void
}

export class PostMessageTransport implements BridgeTransport {
  private connected = false
  private handlers = new Set<MessageHandler>()
  private rateWindowStartedAt = 0
  private rateWindowCount = 0
  private readonly maxMessagesPerSecond: number

  constructor(private readonly options: PostMessageTransportOptions) {
    this.maxMessagesPerSecond = options.maxMessagesPerSecond || 120
  }

  async connect() {
    if (this.connected) return
    this.connected = true
    this.options.receiveWindow.addEventListener('message', this.handleMessage)
  }

  disconnect() {
    if (!this.connected) return
    this.connected = false
    this.options.receiveWindow.removeEventListener('message', this.handleMessage)
    this.handlers.clear()
  }

  send(message: TheaterBridgeMessage) {
    if (!this.connected) throw new Error('PostMessageTransport is not connected')
    const target = this.options.targetWindow()
    if (!target) throw new Error('PostMessageTransport target window is unavailable')
    assertMessageSize(message)
    target.postMessage(message, this.options.targetOrigin)
  }

  subscribe(handler: MessageHandler) {
    this.handlers.add(handler)
    return () => this.handlers.delete(handler)
  }

  private handleMessage = (event: MessageEvent) => {
    if (!this.connected) return
    if (event.origin !== this.options.expectedOrigin) return
    const expectedSource = this.options.expectedSource()
    if (!expectedSource || event.source !== expectedSource) return
    if (!event.data || typeof event.data !== 'object' || event.data.protocol !== THEATER_BRIDGE_PROTOCOL) return
    if (!this.takeRateToken()) {
      this.options.onRejected?.('rate-limit')
      return
    }
    try {
      assertMessageSize(event.data)
      const message = parseTheaterBridgeMessage(event.data)
      this.handlers.forEach((handler) => handler(message))
    } catch (error) {
      this.options.onRejected?.('invalid-message', error)
    }
  }

  private takeRateToken() {
    const now = Date.now()
    if (now - this.rateWindowStartedAt >= 1000) {
      this.rateWindowStartedAt = now
      this.rateWindowCount = 0
    }
    this.rateWindowCount += 1
    return this.rateWindowCount <= this.maxMessagesPerSecond
  }
}
