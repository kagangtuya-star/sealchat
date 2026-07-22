import type { TheaterStageStore } from '../stage/StageStore'
import { TheaterBridgeClient, TheaterBridgeRequestError } from './TheaterBridgeClient'
import {
  THEATER_BRIDGE_VERSION,
  THEATER_CHAT_MESSAGE_EVENT_NAMES,
  THEATER_STAGE_CAPABILITIES,
  createTheaterBridgeMessage,
  isNewerCharacterSnapshot,
  type ApplyScenePayload,
  type BridgeErrorResult,
  type ChatCharacterReadResult,
  type ChatCharactersSnapshotPayload,
  type ChatComposerInsertPayload,
  type ChatComposerInsertResult,
  type ChatMessageSendPayload,
  type ChatMessageSendResult,
  type InitializedPayload,
  type ReadyPayload,
  type SelectCharacterPayload,
  type SelectCharacterResult,
  type SelectCharacterVariantPayload,
  type StageAction,
  type StageActionTriggeredPayload,
  type TheaterBridgeContext,
  type TheaterDialogueMessagePayload,
  type TheaterDialogueMessageRemovedPayload,
  type TheaterBridgeMessage,
} from './theater-bridge-protocol'
import { MemoryTransport, PostMessageTransport } from './theater-bridge-transport'

interface TheaterHostBridgeOptions {
  context: TheaterBridgeContext
  stageStore: TheaterStageStore
  getChatWindow: () => Window | null
  origin: string
  userId: string
  permissions: readonly string[]
  stageCapabilities?: readonly string[]
  now?: () => number
  chatQueueLimit?: number
  chatQueueTtlMs?: number
  debug?: boolean | (() => boolean)
  onChatOnlineChange?: (online: boolean) => void
  onCharacterSnapshotChange?: (snapshot: ChatCharactersSnapshotPayload) => void
  onChatMessageCreated?: (payload: TheaterDialogueMessagePayload) => void
  onChatMessageUpdated?: (payload: TheaterDialogueMessagePayload) => void
  onChatMessageRemoved?: (payload: TheaterDialogueMessageRemovedPayload) => void
  triggerStageAction?: (payload: StageActionTriggeredPayload) => Promise<boolean | StageAction>
  onSceneApplied?: (sceneId: string) => void
}

const CHAT_BRIDGE_PERMISSIONS = [
  'chat.message.send',
  'chat.composer.insert',
  'chat.character.read',
  'chat.character.select',
  'chat.character.variant.select',
] as const

const sameStageAction = (left: StageAction, right: StageAction) => {
  if (left.id !== right.id || left.type !== right.type) return false
  switch (left.type) {
    case 'chat.send':
      return right.type === 'chat.send'
        && left.payload.content === right.payload.content
        && left.payload.channelId === right.payload.channelId
        && left.payload.characterId === right.payload.characterId
    case 'chat.insert':
      return right.type === 'chat.insert' && left.payload.content === right.payload.content
    case 'scene.apply':
      return right.type === 'scene.apply' && left.payload.sceneId === right.payload.sceneId
    case 'object.toggle':
      return right.type === 'object.toggle' && left.payload.objectId === right.payload.objectId
  }
}

export const mergeTheaterBridgePermissions = (
  stagePermissions: readonly string[],
  canControlStage = false,
): string[] => [...new Set([
  ...(canControlStage ? ['stage.control'] : []),
  ...stagePermissions,
  ...CHAT_BRIDGE_PERMISSIONS,
])]

export class TheaterHostBridge {
  private static readonly CHAT_QUEUE_LIMIT = 32
  private static readonly CHAT_QUEUE_TTL_MS = 6_000
  private readonly chatTransport: PostMessageTransport
  private readonly hostStageTransport: MemoryTransport
  private readonly stageTransport: MemoryTransport
  private readonly hostClient: TheaterBridgeClient
  private readonly stageClient: TheaterBridgeClient
  private readonly stageCapabilities: readonly string[]
  private readonly stageCapabilitySet: ReadonlySet<string>
  private routerUnsubscribers: Array<() => void> = []
  private chatOnline = false
  private started = false
  private pendingChatMessages: Array<{ message: TheaterBridgeMessage, expiresAt: number }> = []
  private characterSnapshot: ChatCharactersSnapshotPayload = {
    revision: 0,
    updatedAt: 0,
    activeIdentityId: null,
    characters: [],
  }

  constructor(private readonly options: TheaterHostBridgeOptions) {
    this.stageCapabilities = options.stageCapabilities || THEATER_STAGE_CAPABILITIES
    this.stageCapabilitySet = new Set(this.stageCapabilities)
    ;[this.hostStageTransport, this.stageTransport] = MemoryTransport.createPair()
    this.chatTransport = new PostMessageTransport({
      receiveWindow: window,
      targetWindow: options.getChatWindow,
      expectedSource: options.getChatWindow,
      targetOrigin: options.origin,
      expectedOrigin: options.origin,
      onRejected: (reason, error) => this.debug('postMessage rejected', { reason, error }),
    })
    this.hostClient = new TheaterBridgeClient({
      endpoint: 'host',
      context: options.context,
      transport: this.chatTransport,
      debug: options.debug,
    })
    this.stageClient = new TheaterBridgeClient({
      endpoint: 'stage',
      context: options.context,
      transport: this.stageTransport,
      capabilities: this.stageCapabilities,
      debug: options.debug,
    })
    this.registerHandlers()
  }

  async start() {
    if (this.started) return
    this.started = true
    await this.hostStageTransport.connect()
    this.routerUnsubscribers = [
      this.chatTransport.subscribe((message) => this.routeFromChat(message)),
      this.hostStageTransport.subscribe((message) => this.routeFromStage(message)),
    ]
    await Promise.all([this.hostClient.connect(), this.stageClient.connect()])
  }

  setPermissions(permissions: readonly string[]) {
    const target = this.options.permissions as string[]
    target.splice(0, target.length, ...new Set(permissions))
  }

  stop() {
    if (!this.started) return
    this.started = false
    this.setChatOnline(false)
    this.pendingChatMessages = []
    this.routerUnsubscribers.forEach((unsubscribe) => unsubscribe())
    this.routerUnsubscribers = []
    this.hostClient.disconnect()
    this.stageClient.disconnect()
    this.hostStageTransport.disconnect()
  }

  handleChatFrameLoad() {
    this.setChatOnline(false)
  }

  triggerStageAction(payload: StageActionTriggeredPayload) {
    if (!this.started) return
    try {
      this.stageClient.emit('host', 'stage.action.triggered', payload)
    } catch (error) {
      this.debug('invalid stage action rejected', error)
    }
  }

  async sendChatMessage(payload: ChatMessageSendPayload) {
    if (!this.started) {
      throw new TheaterBridgeRequestError('BRIDGE_NOT_READY', 'Theater bridge is not ready')
    }
    return this.stageClient.request<ChatMessageSendPayload, ChatMessageSendResult>(
      'chat',
      'chat.message.send',
      payload,
    )
  }

  selectChatCharacter(identityId: string) {
    return this.stageClient.request<SelectCharacterPayload, SelectCharacterResult>(
      'chat',
      'chat.character.select',
      { identityId },
    ).then((result) => {
      if (result.ok) this.applyCharacterSnapshot(result.snapshot)
      return result
    })
  }

  selectChatCharacterVariant(identityId: string, variantId: string | null) {
    return this.stageClient.request<SelectCharacterVariantPayload, SelectCharacterResult>(
      'chat',
      'chat.character.variant.select',
      { identityId, variantId },
    ).then((result) => {
      if (result.ok) this.applyCharacterSnapshot(result.snapshot)
      return result
    })
  }

  private registerHandlers() {
    this.hostClient.onSystem<ReadyPayload>('system.ready', (payload, message) => {
      if (message.source !== 'chat' || payload.endpoint !== 'chat') return
      if (!payload.supportedVersions.includes(THEATER_BRIDGE_VERSION)) {
        this.debug('chat version rejected', payload.supportedVersions)
        return
      }
      this.setChatOnline(false)
      this.hostClient.setRemoteCapabilities('chat', payload.capabilities)
      this.stageClient.setRemoteCapabilities('chat', payload.capabilities)
      this.hostClient.sendSystem('chat', 'system.initialize', {
        selectedVersion: THEATER_BRIDGE_VERSION,
        worldId: this.options.context.worldId,
        channelId: this.options.context.channelId,
        userId: this.options.userId,
        permissions: [...this.options.permissions],
        capabilities: [...this.stageCapabilities],
        initialContext: {
          activeSceneId: this.options.stageStore.state.activeSceneId,
          activeCharacterId: null,
        },
      })
    })

    this.hostClient.onSystem<InitializedPayload>('system.initialized', (payload, message) => {
      if (
        message.source !== 'chat'
        || payload.endpoint !== 'chat'
        || payload.selectedVersion !== THEATER_BRIDGE_VERSION
      ) return
      this.hostClient.setRemoteCapabilities('chat', payload.capabilities)
      this.stageClient.setRemoteCapabilities('chat', payload.capabilities)
      this.setChatOnline(true)
    })

    this.stageClient.onCommand('stage.scene.read', () => ({
      ok: true as const,
      state: this.options.stageStore.getSnapshot(),
    }))

    this.stageClient.onCommand<ApplyScenePayload>('stage.scene.apply', (payload) => this.applyScene(payload))

    const applyCharacterEvent = (payload: ChatCharactersSnapshotPayload) => {
      this.applyCharacterSnapshot(payload)
    }
    this.stageClient.onEvent<ChatCharactersSnapshotPayload>('chat.character.updated', applyCharacterEvent)
    this.stageClient.onEvent<ChatCharactersSnapshotPayload>('chat.character.selected', applyCharacterEvent)
    this.stageClient.onEvent<ChatCharactersSnapshotPayload>('chat.character.appearance.updated', applyCharacterEvent)
    this.stageClient.onEvent<ChatCharactersSnapshotPayload>('chat.character.variant.selected', applyCharacterEvent)
    this.stageClient.onEvent<TheaterDialogueMessagePayload>('chat.message.created', (payload) => {
      this.options.onChatMessageCreated?.(structuredClone(payload))
    })
    this.stageClient.onEvent<TheaterDialogueMessagePayload>('chat.message.updated', (payload) => {
      this.options.onChatMessageUpdated?.(structuredClone(payload))
    })
    this.stageClient.onEvent<TheaterDialogueMessageRemovedPayload>('chat.message.removed', (payload) => {
      this.options.onChatMessageRemoved?.(structuredClone(payload))
    })
  }

  private routeFromChat(message: TheaterBridgeMessage) {
    if (message.source !== 'chat' || message.target !== 'stage') return
    if (!this.matchesContext(message)) return
    if (!this.chatOnline) {
      this.rejectCommand(message, 'ENDPOINT_OFFLINE', 'Theater chat endpoint is not initialized')
      return
    }
    if (
      message.kind === 'event'
      && THEATER_CHAT_MESSAGE_EVENT_NAMES.includes(message.name as typeof THEATER_CHAT_MESSAGE_EVENT_NAMES[number])
    ) {
      if (!this.hostClient.supports('chat', message.name)) {
        this.debug('chat message source capability unavailable', message.name)
        return
      }
      if (!this.stageCapabilitySet.has(message.name)) {
        this.debug('stage message capability unavailable', message.name)
        return
      }
    }
    if (message.kind === 'command'
      && !this.stageCapabilitySet.has(message.name)) {
      this.rejectCommand(message, 'CAPABILITY_UNAVAILABLE', `Stage capability unavailable: ${message.name}`)
      return
    }
    if (message.kind === 'command'
      && message.name === 'stage.scene.apply'
      && !this.options.permissions.includes('stage.scene.switch')) {
      this.rejectCommand(message, 'PERMISSION_DENIED', 'Missing permission: stage.scene.switch')
      return
    }
    try {
      this.hostStageTransport.send(message)
    } catch (error) {
      this.rejectCommand(message, 'ROUTE_FAILED', error instanceof Error ? error.message : String(error))
    }
  }

  private routeFromStage(message: TheaterBridgeMessage) {
    if (message.source !== 'stage') return
    if (!this.matchesContext(message)) return
    if (message.target === 'host') {
      if (message.kind === 'event' && message.name === 'stage.action.triggered') {
        void this.handleStageActionTriggered(message.payload as StageActionTriggeredPayload)
      }
      return
    }
    if (message.target !== 'chat') return
    if (message.kind === 'command') {
      if (!this.hostClient.supports('chat', message.name)) {
        this.rejectStageCommand(message, 'CAPABILITY_UNAVAILABLE', `Chat capability unavailable: ${message.name}`)
        return
      }
      const requiredPermission = message.name === 'chat.message.send'
        ? 'chat.message.send'
        : message.name === 'chat.composer.insert'
          ? 'chat.composer.insert'
          : message.name === 'chat.character.read'
            ? 'chat.character.read'
            : message.name === 'chat.character.select'
              ? 'chat.character.select'
              : message.name === 'chat.character.variant.select'
                ? 'chat.character.variant.select'
                : ''
      if (!requiredPermission || !this.options.permissions.includes(requiredPermission)) {
        this.rejectStageCommand(message, 'PERMISSION_DENIED', `Missing permission: ${requiredPermission || message.name}`)
        return
      }
    }
    if (!this.chatOnline) {
      this.queueChatMessage(message)
      return
    }
    if (
      message.kind === 'event'
      && !this.hostClient.supports('chat', message.name)
    ) {
      this.debug('chat capability unavailable', message.name)
      return
    }
    try {
      this.chatTransport.send(message)
    } catch (error) {
      this.debug('stage route failed', error)
    }
  }

  private async handleStageActionTriggered(payload: StageActionTriggeredPayload) {
    if (!this.options.permissions.includes('stage.action.trigger')) {
      this.debug('stage action permission denied', payload.actionId)
      return
    }
    const object = this.options.stageStore.activeObjects.value[payload.objectId]
    if (!object || !object.visible || !object.interactive || !['text', 'image', 'button'].includes(object.type)) {
      this.debug('stage action object rejected', payload.objectId)
      return
    }
    const action = object.actions.find((item) => item.id === payload.actionId)
    if (!action || !sameStageAction(action, payload.action)) {
      this.debug('stage action payload rejected', payload.actionId)
      return
    }
    try {
      if (this.options.triggerStageAction) {
        const handled = await this.options.triggerStageAction(payload)
        if (handled === true) return
        if (handled) {
          await this.executeStageAction(handled)
          return
        }
      }
      await this.executeStageAction(action)
    } catch (error) {
      this.debug('stage action failed', error)
    }
  }

  private async executeStageAction(action: StageAction) {
    if (action.type === 'scene.apply') {
      if (!this.options.permissions.includes('stage.scene.switch')) {
        throw new TheaterBridgeRequestError('PERMISSION_DENIED', 'Missing permission: stage.scene.switch')
      }
      this.applyScene({ sceneId: action.payload.sceneId })
      return
    }
    if (action.type === 'object.toggle') {
      if (!this.options.permissions.includes('stage.object.edit')) {
        throw new TheaterBridgeRequestError('PERMISSION_DENIED', 'Missing permission: stage.object.edit')
      }
      if (!this.options.stageStore.toggleObject(action.payload.objectId)) {
        throw new TheaterBridgeRequestError('OBJECT_NOT_FOUND', `Object not found: ${action.payload.objectId}`)
      }
      return
    }
    if (action.type === 'chat.send') {
      await this.stageClient.request<ChatMessageSendPayload, ChatMessageSendResult>(
        'chat',
        'chat.message.send',
        action.payload,
      )
      return
    }
    await this.stageClient.request<ChatComposerInsertPayload, ChatComposerInsertResult>(
      'chat',
      'chat.composer.insert',
      action.payload,
    )
  }

  private applyScene(payload: ApplyScenePayload) {
    const previousSceneId = this.options.stageStore.state.activeSceneId || null
    if (!this.options.stageStore.applyScene(payload.sceneId)) {
      throw new TheaterBridgeRequestError('SCENE_NOT_FOUND', `Scene not found: ${payload.sceneId}`)
    }
    this.stageClient.emit('chat', 'stage.scene.applied', {
      sceneId: payload.sceneId,
      previousSceneId,
      transition: payload.transition,
    })
    this.options.onSceneApplied?.(payload.sceneId)
    return { ok: true as const, sceneId: payload.sceneId }
  }

  private rejectCommand(request: TheaterBridgeMessage, code: string, message: string) {
    if (request.kind !== 'command') return
    const payload: BridgeErrorResult = { ok: false, error: { code, message } }
    try {
      this.chatTransport.send(createTheaterBridgeMessage(this.options.context, {
        kind: 'result',
        source: 'host',
        target: 'chat',
        correlationId: request.id,
        name: `${request.name}.result`,
        payload,
      }))
    } catch (error) {
      this.debug('command rejection failed', error)
    }
  }

  private rejectStageCommand(request: TheaterBridgeMessage, code: string, message: string) {
    if (request.kind !== 'command') return
    const payload: BridgeErrorResult = { ok: false, error: { code, message } }
    try {
      this.hostStageTransport.send(createTheaterBridgeMessage(this.options.context, {
        kind: 'result',
        source: 'host',
        target: 'stage',
        correlationId: request.id,
        name: `${request.name}.result`,
        payload,
      }))
    } catch (error) {
      this.debug('stage command rejection failed', error)
    }
  }

  private matchesContext(message: TheaterBridgeMessage) {
    return message.worldId === this.options.context.worldId
      && message.channelId === this.options.context.channelId
      && message.sessionId === this.options.context.sessionId
  }

  private setChatOnline(online: boolean) {
    if (this.chatOnline === online) return
    this.chatOnline = online
    this.options.onChatOnlineChange?.(online)
    this.debug(online ? 'chat online' : 'chat offline')
    if (online) {
      this.flushChatQueue()
      void this.refreshCharacterSnapshot()
    }
  }

  private queueChatMessage(message: TheaterBridgeMessage) {
    const now = this.now()
    const expired = this.pendingChatMessages.filter((item) => item.expiresAt <= now)
    this.pendingChatMessages = this.pendingChatMessages.filter((item) => item.expiresAt > now)
    expired.forEach((item) => {
      this.rejectStageCommand(item.message, 'ENDPOINT_OFFLINE', 'Theater chat endpoint did not reconnect in time')
    })
    if (this.pendingChatMessages.length >= this.chatQueueLimit()) {
      const dropped = this.pendingChatMessages.shift()
      if (dropped) {
        this.rejectStageCommand(dropped.message, 'QUEUE_OVERFLOW', 'Theater chat queue is full')
      }
    }
    this.pendingChatMessages.push({
      message,
      expiresAt: now + this.chatQueueTtlMs(),
    })
    this.debug('chat message queued', message.name)
  }

  private flushChatQueue() {
    const now = this.now()
    const queued = this.pendingChatMessages
    this.pendingChatMessages = []
    queued.forEach(({ message, expiresAt }) => {
      if (expiresAt <= now) {
        this.rejectStageCommand(message, 'ENDPOINT_OFFLINE', 'Theater chat endpoint did not reconnect in time')
        return
      }
      this.routeFromStage(message)
    })
  }

  private async refreshCharacterSnapshot() {
    if (!this.stageClient.supports('chat', 'chat.character.read')) return
    try {
      const result = await this.stageClient.request<Record<string, never>, ChatCharacterReadResult>(
        'chat',
        'chat.character.read',
        {},
      )
      if (result.ok) this.applyCharacterSnapshot(result.snapshot)
    } catch (error) {
      this.debug('character snapshot refresh failed', error)
    }
  }

  private applyCharacterSnapshot(snapshot: ChatCharactersSnapshotPayload) {
    if (!isNewerCharacterSnapshot(snapshot, this.characterSnapshot)) return
    this.characterSnapshot = structuredClone(snapshot)
    this.options.onCharacterSnapshotChange?.(structuredClone(snapshot))
  }

  private now() {
    return this.options.now?.() ?? Date.now()
  }

  private chatQueueLimit() {
    return this.options.chatQueueLimit ?? TheaterHostBridge.CHAT_QUEUE_LIMIT
  }

  private chatQueueTtlMs() {
    return this.options.chatQueueTtlMs ?? TheaterHostBridge.CHAT_QUEUE_TTL_MS
  }

  private debug(message: string, detail?: unknown) {
    const enabled = typeof this.options.debug === 'function' ? this.options.debug() : this.options.debug
    if (enabled) console.info(`[theater-bridge:host-router] ${message}`, detail || '')
  }
}
