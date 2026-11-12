import { watch } from 'vue'
import type { ChannelIForm, ChannelIFormBridgePolicy } from '@/types/iform'
import { chatEvent } from '@/stores/chat'
import type { CharacterCardApiStatus, CharacterCardAttrsPatchResult, CharacterCardData } from '@/stores/characterCard'
import type { ChannelCharacterSnapshotItem } from '@/stores/channelCharacterSnapshot'
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver'
import { uploadChannelEmbedImage } from '@/services/embed/channelEmbedImageUpload'
import type { createChannelEmbedTheaterDialogue } from './channelEmbedTheaterDialogue'
import type { ChatCharactersSnapshotPayload } from '@/views/theater/bridge/theater-bridge-protocol'
import { randomEmbedId } from './channelEmbedProtocol'

import { CHANNEL_EXTENSION_CAPABILITIES, isChannelExtensionWriteCapability, requiresChannelTextSendPermission } from './channelExtensionCapabilities'
const publicErrorCodes = new Set(['ORIGIN_DENIED', 'HANDSHAKE_FAILED', 'SESSION_EXPIRED', 'CONTEXT_CHANGED', 'CAPABILITY_DENIED', 'PERMISSION_DENIED', 'INVALID_PARAMS', 'NOT_FOUND', 'REVISION_CONFLICT', 'QUOTA_EXCEEDED', 'PAYLOAD_TOO_LARGE', 'RATE_LIMITED', 'WS_OFFLINE', 'TIMEOUT', 'INTERNAL_ERROR'])

export type ChannelExtensionScope = { type: 'iform' | 'ui-customization'; id: string }
export type ChannelExtensionEvent = { topic: string; payload: unknown; seq?: number; contextVersion: number; at: number }
type ExtensionRequest = { sessionId: string; method: string; params?: unknown; contextVersion: number }
type ExtensionSession = { emit: (event: ChannelExtensionEvent) => void; disposeDialogue?: () => void; theaterCharacterIdentityId?: string; topics: Set<string> }

export interface ChannelEmbedTheaterCharacterSource {
  getSnapshot(): ChatCharactersSnapshotPayload
  subscribe(listener: (snapshot: ChatCharactersSnapshotPayload) => void): () => void
}

// Only metadata of an events.publish that this host sent and the server accepted.
// The payload is deliberately absent: it never selects Theater actions.
export interface ChannelEmbedPublishedEvent {
  eventId: string
  formId: string
  channelId: string
  topic: string
  at: number
}

export type ChannelEmbedTheaterEventSink = (event: ChannelEmbedPublishedEvent) => void

export interface ChannelEmbedTheaterContext {
  worldId: string
  scopeType: 'world' | 'channel'
  channelId: string
  objectId: string
}

export type ChannelExtensionDeps = {
  chat: any
  user: any
  characterCard: {
    getCharacterApiStatus: (channelId: string) => CharacterCardApiStatus
    getActiveCard: (channelId: string, options?: { throwOnError?: boolean }) => Promise<CharacterCardData | null>
    patchActiveCardAttrs: (channelId: string, attrsPatch: Record<string, any>) => Promise<CharacterCardAttrsPatchResult>
  }
  characterSnapshot: {
    refreshChannel: (channelId: string) => Promise<void>
    getChannelItems: (channelId: string) => ChannelCharacterSnapshotItem[]
  }
  form?: ChannelIForm
  scope: ChannelExtensionScope
  capabilities: string[]
  readOnly?: () => boolean
  isCurrent?: () => boolean
  onDispose?: () => void
  theaterDialogueSource?: ReturnType<typeof createChannelEmbedTheaterDialogue>
  worldId: string
  channelId: string
  theaterCharacterSource?: ChannelEmbedTheaterCharacterSource
  // Set only by a plain Theater iframe object; other embed hosts never report.
  theaterEventSink?: ChannelEmbedTheaterEventSink
  theaterContext?: ChannelEmbedTheaterContext
}

const safeString = (value: unknown, max = 512) => typeof value === 'string' ? value.slice(0, max) : ''
const jsonSize = (value: unknown) => { try { return new TextEncoder().encode(JSON.stringify(value)).byteLength } catch { return Number.POSITIVE_INFINITY } }
const safeUser = (user: any) => user?.id ? { id: String(user.id), displayName: safeString(user.nick || user.nickname || user.username) || String(user.id), avatar: safeString(user.avatar) } : null
const safeMember = (member: any) => member ? {
  userId: safeString(member.user?.id || member.userId || member.user_id || member.id),
  displayName: safeString(member.nick || member.nickname || member.user?.nick || member.user?.nickname),
  avatar: safeString(member.avatar || member.user?.avatar),
} : null
const safeMemberList = (users: any[]) => users.map((user) => {
  const safe = safeUser(user)
  return safe ? { userId: safe.id, displayName: safe.displayName, avatar: safe.avatar } : null
}).filter((member): member is NonNullable<ReturnType<typeof safeMember>> => !!member)
const safeWorldAdminList = (admins: any[]) => admins.map((admin) => {
  const userId = safeString(admin?.userId || admin?.user_id || admin?.id, 100)
  if (!userId) return null
  const role = admin?.role === 'owner' ? 'owner' : 'admin'
  return {
    userId,
    displayName: safeString(admin?.displayName || admin?.nickname || admin?.username) || userId,
    avatar: safeString(admin?.avatar),
    role,
  }
}).filter((admin): admin is { userId: string; displayName: string; avatar: string; role: 'owner' | 'admin' } => !!admin)
const safeAttrs = (value: unknown): Record<string, any> => value && typeof value === 'object' && !Array.isArray(value) ? JSON.parse(JSON.stringify(value)) as Record<string, any> : {}
const safeCardAvatar = (value?: string) => safeString(resolveAttachmentUrl(value) || value)
const safeCurrentCharacterCard = (card: CharacterCardData) => {
  const avatar = safeCardAvatar(card.avatarUrl)
  return {
    name: safeString(card.name),
    sheetType: safeString(card.type),
    attrs: safeAttrs(card.attrs),
    ...(avatar ? { avatar } : {}),
  }
}
const safeCharacterCardSnapshot = (item: ChannelCharacterSnapshotItem) => {
  const identity = item.data.identity
  const identityAvatar = safeCardAvatar(identity.avatarAttachmentId)
  const card = item.data.card
  const cardAvatar = safeCardAvatar(card?.avatarAttachmentId)
  const updatedAt = Number(item.sourceUpdatedAt || item.lastSeenAt || 0)
  return {
    identityId: safeString(item.identityId, 100),
    userId: safeString(item.userId, 100),
    identity: {
      id: safeString(identity.id, 100),
      userId: safeString(identity.userId, 100),
      ...(identity.displayName ? { displayName: safeString(identity.displayName) } : {}),
      ...(identity.color ? { color: safeString(identity.color, 32) } : {}),
      ...(identityAvatar ? { avatar: identityAvatar } : {}),
    },
    card: card ? {
      name: safeString(card.name),
      sheetType: safeString(card.sheetType),
      attrs: safeAttrs(card.attrs),
      ...(cardAvatar ? { avatar: cardAvatar } : {}),
    } : null,
    revision: Number.isFinite(item.serverRevision) ? item.serverRevision : 0,
    ...(Number.isFinite(updatedAt) && updatedAt > 0 ? { updatedAt } : {}),
  }
}

const normalizedPolicy = (policy?: ChannelIFormBridgePolicy) => ({
  enabled: !!policy?.enabled,
  allowedOrigins: Array.isArray(policy?.allowedOrigins) ? policy!.allowedOrigins!.map((origin) => String(origin).trim()).filter(Boolean) : [],
  capabilities: Array.isArray(policy?.capabilities) ? [...new Set(policy!.capabilities!.map((item) => String(item).trim()).filter(Boolean))] : [],
})

const errorCode = (error: unknown) => {
  const message = String((error as any)?.message || error || 'INTERNAL_ERROR')
  const candidate = message.includes(':') ? message.slice(0, message.indexOf(':')) : message
  if (publicErrorCodes.has(candidate)) return candidate
  if (/timeout|请求超时/i.test(message)) return 'TIMEOUT'
  if (/ws not connected|websocket/i.test(message)) return 'WS_OFFLINE'
  return 'INTERNAL_ERROR'
}

export const channelExtensionPublicError = (error: unknown) => {
  const code = errorCode(error)
  if (code === 'INTERNAL_ERROR' || code === 'WS_OFFLINE') return { code, message: code === 'WS_OFFLINE' ? 'WebSocket unavailable' : 'Embed request failed' }
  const message = String((error as any)?.message || error || code)
  const details = code === 'REVISION_CONFLICT'
    ? (() => { const match = message.match(/currentRevision=(\d+)/); return match ? { currentRevision: Number(match[1]) } : undefined })()
    : undefined
  return { code, message: message.replace(new RegExp(`^${code}:\\s*`), '').slice(0, 512), details }
}

export const createChannelExtensionContext = (deps: ChannelExtensionDeps) => {
  const policy = normalizedPolicy(deps.form?.bridgePolicy || { enabled: true, capabilities: deps.capabilities })
  const sessions = new Map<string, ExtensionSession>()
  const listeners: Array<() => void> = []
  const seenGatewayEvents = new Set<string>()
  const reportedPublishedEvents = new Set<string>()
  const authenticatedUserId = safeString(deps.user?.info?.id, 100)
  const authenticatedToken = deps.user?.token
  let contextVersion = 1
  let lastContextKey = ''
  let storageSeq = 0
  let storageEventChain: Promise<void> = Promise.resolve()
  let closed = false

  let canWrite = false
  const scopeParams = () => ({ channel_id: deps.channelId, scope: deps.scope })
  const refreshWritePermission = async () => {
    try {
      const permitted = await Promise.all(['func_channel_text_send', 'func_channel_text_send_all'].map(permission => deps.chat.hasChannelPermission?.(deps.channelId, permission, authenticatedUserId) ?? false))
      const rootId = deps.chat.curChannel?.rootId
      const inherited = rootId ? await deps.chat.hasChannelPermission?.(rootId, 'func_channel_text_send_all', authenticatedUserId) : false
      canWrite = permitted.some(Boolean) || !!inherited
    } catch { canWrite = false }
  }
  const getIdentities = () => {
    const identities = deps.chat.getScopedChannelIdentities?.(deps.channelId) || []
    return identities.map((identity: any) => {
      const variants = deps.chat.getIdentityVariants?.(deps.channelId, identity.id) || []
      const activeVariant = deps.chat.getActiveIdentityVariant?.(deps.channelId, identity.id)
      const activeId = safeString(deps.chat.activeChannelIdentity?.[deps.channelId], 100)
      return {
        id: safeString(identity.id, 100), displayName: safeString(identity.displayName || identity.name), name: safeString(identity.displayName || identity.name),
        avatar: safeString(resolveAttachmentUrl(identity.avatarAttachmentId) || identity.avatar), color: safeString(identity.color, 32),
        isActive: safeString(identity.id, 100) === activeId,
        activeVariant: activeVariant ? { id: safeString(activeVariant.id || activeVariant.variantId), displayName: safeString(activeVariant.displayName || activeVariant.name || activeVariant.keyword) } : null,
        variants: variants.map((variant: any) => ({ id: safeString(variant.id || variant.variantId), displayName: safeString(variant.displayName || variant.name || variant.keyword) })),
      }
    })
  }
  const currentCharacter = () => {
    const activeId = safeString(deps.chat.activeChannelIdentity?.[deps.channelId], 100)
    return getIdentities().find((identity: any) => identity.id === activeId) || null
  }
  const currentActiveIdentityId = () => safeString(deps.chat.activeChannelIdentity?.[deps.channelId], 100)
  const effectiveCapabilities = () => policy.capabilities.filter((capability) => {
    if (!CHANNEL_EXTENSION_CAPABILITIES.includes(capability)) return false
    if (capability === 'theater.dialogue.subscribe' && !deps.theaterDialogueSource) return false
    if (capability === 'theater.character.read' && !deps.theaterCharacterSource) return false
    if (capability === 'members.read' && (!deps.chat.curMember || deps.chat.observerMode)) return false
    if (isChannelExtensionWriteCapability(capability) && (!deps.chat.curMember || deps.chat.observerMode || deps.readOnly?.())) return false
    if (requiresChannelTextSendPermission(capability) && !canWrite) return false
    return true
  })
  const has = (capability: string) => effectiveCapabilities().includes(capability)
  const connectionState = () => ({
    state: deps.chat.connectState === 'connected'
      ? 'connected'
      : deps.chat.connectState === 'connecting' || deps.chat.connectState === 'reconnecting'
        ? 'reconnecting'
        : 'offline',
    latencyMs: Number.isFinite(deps.chat.lastLatencyMs) ? deps.chat.lastLatencyMs : undefined,
  })
  const worldAuthorization = () => {
    const detail = deps.chat.worldDetailMap?.[deps.worldId] || {}
    const world = detail.world || deps.chat.worldMap?.[deps.worldId] || {}
    const userID = safeString(deps.user?.info?.id, 100)
    const ownerID = safeString(world.ownerId || world.owner_id, 100)
    const memberRole = safeString(detail.memberRole || detail.role, 24)
    const isWorldOwner = memberRole === 'owner' || (!!ownerID && ownerID === userID)
    const isWorldAdmin = isWorldOwner || memberRole === 'admin'
    const isSystemAdmin = Boolean(deps.user?.checkPerm?.('mod_admin'))
    return {
      worldRole: memberRole || (isWorldOwner ? 'owner' : null),
      isWorldOwner,
      isWorldAdmin,
      isSystemAdmin,
      canManageWorld: isWorldAdmin || isSystemAdmin,
    }
  }
  const getContext = () => {
    const channel = deps.chat.curChannel || {}
    const world = deps.chat.worldMap?.[deps.worldId] || deps.chat.worldDetailMap?.[deps.worldId]?.world || {}
    const guild = channel.guild || deps.chat.curGuild || deps.chat.guildMap?.[channel.guildId || channel.guild_id]
    const member = safeMember(deps.chat.curMember)
    const channelType = typeof channel.type === 'number' ? channel.type : safeString(channel.type) || undefined
    const authorization = worldAuthorization()
    return {
      world: { id: deps.worldId, name: safeString(world.name) || undefined },
      ...(guild?.id ? { guild: { id: safeString(guild.id, 100), name: safeString(guild.name) || undefined } } : {}),
      channel: { id: deps.channelId, name: safeString(channel.name) || undefined, type: channelType },
      currentUser: safeUser(deps.user.info), currentMember: member, currentCharacter: has('characters.read') ? currentCharacter() : null,
      connection: connectionState(),
      permissions: { canSendMessage: has('messages.send') && deps.chat.connectState === 'connected', canReadMembers: has('members.read'), canReadCharacters: has('characters.read'), canReadCharacterCard: has('characterCard.read'), canWriteCharacterCard: has('characterCard.write'), canReadStorage: has('storage.read'), canWriteStorage: has('storage.write'), canPublishEvents: has('events.publish'), ...authorization },
      capabilities: effectiveCapabilities(), contextVersion,
    }
  }
  const contextKey = () => JSON.stringify({ capabilities: effectiveCapabilities(), activeChannelId: safeString(deps.chat.curChannel?.id), channel: { id: deps.channelId, name: safeString(deps.chat.curChannel?.name), type: safeString(deps.chat.curChannel?.type) }, worldId: deps.worldId, member: safeMember(deps.chat.curMember), authorization: worldAuthorization(), character: currentCharacter(), state: deps.chat.connectState, users: deps.chat.curChannelUsers?.map((user: any) => safeString(user?.id, 100)) })
  const postEventToSession = (_id: string, session: ExtensionSession, topic: string, payload: unknown, seq?: number) => {
    if (closed && topic !== 'session.closed') return
    try { session.emit({ topic, payload, seq, contextVersion, at: Date.now() }) } catch { /* subscriber owns its transport */ }
  }
  const postEvent = (topic: string, payload: unknown, seq?: number) => {
    sessions.forEach((session, id) => postEventToSession(id, session, topic, payload, seq))
  }
  const theaterCharacterSnapshot = (snapshot: ChatCharactersSnapshotPayload, identityId: string) => ({
    revision: snapshot.revision,
    updatedAt: snapshot.updatedAt,
    activeIdentityId: snapshot.activeIdentityId,
    identityId,
    character: snapshot.characters.find(character => character.identityId === identityId) || null,
  })
  const disposeTheaterCharacterSource = deps.theaterCharacterSource?.subscribe((snapshot) => {
    if (closed || !has('theater.character.read')) return
    sessions.forEach((session, targetSessionId) => {
      const identityId = session.theaterCharacterIdentityId
      if (!identityId) return
      postEventToSession(targetSessionId, session, 'theater.character.changed', theaterCharacterSnapshot(snapshot, identityId))
    })
  })
  const publishContextChanges = () => {
    if (closed) return
    const next = contextKey()
    if (next === lastContextKey) return
    lastContextKey = next
    contextVersion++
    if (has('context.read')) {
      postEvent('context.changed', getContext())
      postEvent('connection.changed', connectionState())
    } else postEvent('capabilities.changed', effectiveCapabilities())
    if (has('characters.read')) postEvent('characters.changed', getIdentities())
    if (has('members.read')) postEvent('members.changed', safeMemberList(deps.chat.curChannelUsers || []))
    if (has('permissions.read')) postEvent('permissions.changed', getContext().permissions)
  }
  const refreshOnlineMembers = async () => {
    if (!has('members.read')) return
    try {
      const result = await deps.chat.sendAPI('channel.member.list.online', { channel_id: deps.channelId })
      const items = result?.data?.data || result?.data || result || []
      ensureSessionContext()
      if (Array.isArray(items) && safeString(deps.chat.curChannel?.id, 100) === deps.channelId) {
        deps.chat.curChannelUsers = items
        publishContextChanges()
      }
    } catch { /* next gateway event or reconnect retries */ }
  }
  const closeActiveSessions = () => {
    sessions.forEach((session, id) => { session.disposeDialogue?.(); session.disposeDialogue = undefined; session.topics.clear(); postEventToSession(id, session, 'session.closed', null) })
    sessions.clear()
  }
  const sendSnapshotToSession = async (session: ExtensionSession) => {
    try {
      const result = await deps.chat.sendAPI('channel.extension.storage.snapshot', scopeParams())
      ensureSessionContext()
      const snapshotSeq = Number(result?.data?.seq || 0)
      if (!Number.isFinite(snapshotSeq) || snapshotSeq < storageSeq || ![...sessions.values()].includes(session)) return
      storageSeq = snapshotSeq
      postEventToSession('', session, 'storage.changed', { snapshot: result?.data }, snapshotSeq)
    } catch { /* request path reports WS_OFFLINE */ }
  }
  const onGatewayEvent = (event: any) => {
    if (closed) return
    const embed = event?.channelIFormEmbed
    if (!embed || embed.channelId !== deps.channelId || (embed.scopeType || 'iform') !== deps.scope.type || (embed.scopeId || embed.formId) !== deps.scope.id) return
    const eventId = safeString(embed.eventId, 128)
    if (eventId) {
      if (seenGatewayEvents.has(eventId)) return
      seenGatewayEvents.add(eventId)
      if (seenGatewayEvents.size > 2048) {
        const oldest = seenGatewayEvents.values().next().value
        if (oldest) seenGatewayEvents.delete(oldest)
      }
    }
    if (embed.op === 'event') { if (has('events.subscribe')) postEvent(`event:${embed.topic}`, embed.payload); return }
    if (!has('storage.read')) return
    const seq = Number(embed.seq || 0)
    if (seq <= storageSeq) return
    if (seq > storageSeq + 1) { sessions.forEach((session) => { void sendSnapshotToSession(session) }); return }
    storageSeq = seq
    storageEventChain = storageEventChain.then(async () => {
      let value = embed.value
      if (embed.op === 'set' && value === undefined) {
        try {
          const result = await deps.chat.sendAPI('channel.extension.storage.get', { ...scopeParams(), key: embed.key })
          value = result?.data?.value
        } catch {
          sessions.forEach((session) => { void sendSnapshotToSession(session) })
          return
        }
      }
      ensureSessionContext()
      postEvent('storage.changed', { key: embed.key, op: embed.op, revision: embed.revision, value }, seq)
    }).catch(() => undefined)
  }
  const eventNames = ['channel-iform-embed', 'channel-iform-updated', 'channel-role-updated', 'channel-identity-updated', 'channel-identities-updated', 'channel-member-updated', 'channel-presence-updated', 'channel-updated', 'world-updated', 'channel-switch-to', 'channel-context-cleared', 'connected', 'connection.changed']
  eventNames.forEach((name) => {
    const handler = (event: any) => {
      if (name === 'channel-iform-embed') {
        onGatewayEvent(event)
        return
      }
      const eventChannelId = safeString(event?.channel?.id, 100)
      if (deps.scope.type === 'iform' && name === 'channel-iform-updated' && (!eventChannelId || eventChannelId === deps.channelId)) {
        const forms = event?.iform?.forms
        const changedForm = Array.isArray(forms) ? forms.find((item: any) => item.id === deps.scope.id) : event?.iform?.form?.id === deps.scope.id ? event.iform.form : undefined
        if ((Array.isArray(forms) && !changedForm) || (changedForm && JSON.stringify(normalizedPolicy(changedForm.bridgePolicy)) !== JSON.stringify(policy))) { stop(); return }
      }
      const activeChannelId = safeString(deps.chat.curChannel?.id, 100)
      if (!activeChannelId || activeChannelId !== deps.channelId) {
        stop()
        return
      }
      void refreshWritePermission().then(() => { if (!closed) publishContextChanges() }).catch(() => { canWrite = false })
      if (name === 'connected') {
        sessions.forEach(session => session.topics.forEach(topic => { void deps.chat.sendAPI('channel.extension.events.subscribe', { ...scopeParams(), topic }).catch(() => undefined) }))
      }
      if (name === 'connected' && has('storage.read')) {
        sessions.forEach((session) => { void sendSnapshotToSession(session) })
      }
      if (name === 'channel-presence-updated') void refreshOnlineMembers()
      publishContextChanges()
    }
    chatEvent.on(name as any, handler as any)
    listeners.push(() => chatEvent.off(name as any, handler as any))
  })
  lastContextKey = contextKey()
  if (typeof deps.chat.worldDetail === 'function') {
    void deps.chat.worldDetail(deps.worldId, { force: true }).then(() => publishContextChanges()).catch(() => undefined)
  }

  // Runs once per server-accepted publish from this host instance. Gateway
  // channel-iform-embed broadcasts never reach here, so other clients and other
  // instances of the same form cannot start this object's Theater actions.
  const reportPublishedEvent = (topic: string, result: unknown) => {
    const sink = deps.theaterEventSink
    if (!sink || closed) return
    const eventId = safeString((result as { eventId?: unknown } | null)?.eventId, 128)
    if (!eventId || reportedPublishedEvents.has(eventId)) return
    try {
      ensureSessionContext()
    } catch {
      return
    }
    reportedPublishedEvents.add(eventId)
    if (reportedPublishedEvents.size > 256) {
      const oldest = reportedPublishedEvents.values().next().value
      if (oldest) reportedPublishedEvents.delete(oldest)
    }
    try {
      sink({ eventId, formId: deps.scope.id, channelId: deps.channelId, topic, at: Date.now() })
    } catch { /* Theater handling never changes the embed's publish result */ }
  }
  const requestParams = (request: ExtensionRequest) => {
    if (request.params === undefined) return {}
    if (!request.params || typeof request.params !== 'object' || Array.isArray(request.params)) throw new Error('INVALID_PARAMS')
    return request.params as Record<string, any>
  }
  const isPlainObject = (value: unknown): value is Record<string, any> => {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return false
    const prototype = Object.getPrototypeOf(value)
    return prototype === Object.prototype || prototype === null
  }
  const hasOnlyKeys = (value: Record<string, any>, keys: string[]) => Object.keys(value).every((key) => keys.includes(key))
  const boundedString = (value: unknown, max: number, required = false) => {
    if (typeof value !== 'string' || value.length > max) throw new Error('INVALID_PARAMS')
    const normalized = value.trim()
    if (required && !normalized) throw new Error('INVALID_PARAMS')
    return normalized
  }
  const ensureSessionContext = () => {
    if (closed || deps.isCurrent?.() === false) throw new Error('CONTEXT_CHANGED')
    if (!authenticatedUserId || safeString(deps.user?.info?.id, 100) !== authenticatedUserId || deps.user?.token !== authenticatedToken) throw new Error('SESSION_EXPIRED')
    const activeChannelId = safeString(deps.chat.curChannel?.id, 100)
    if (!activeChannelId || activeChannelId !== deps.channelId || ('currentWorldId' in deps.chat && safeString(deps.chat.currentWorldId, 100) !== deps.worldId)) throw new Error('CONTEXT_CHANGED')
  }
  const requireContext = (request: ExtensionRequest, mutating: boolean) => {
    ensureSessionContext()
    if (mutating && request.contextVersion !== contextVersion) throw new Error('CONTEXT_CHANGED')
  }
  const dispatchRequest = async (request: ExtensionRequest, session: ExtensionSession) => {
    if (request.method === 'session.close') {
      session.disposeDialogue?.()
      session.disposeDialogue = undefined
      sessions.delete(request.sessionId)
      return null
    }
    ensureSessionContext()
    await refreshWritePermission()
    ensureSessionContext()
    const params = requestParams(request)
    const method = request.method
    if (method.startsWith('storage.') && !has(method === 'storage.get' || method === 'storage.list' || method === 'storage.snapshot' ? 'storage.read' : 'storage.write')) throw new Error('CAPABILITY_DENIED')
    if (method === 'events.publish' && !has('events.publish')) throw new Error('CAPABILITY_DENIED')
    if (method === 'events.subscribe' && !has('events.subscribe')) throw new Error('CAPABILITY_DENIED')
    if (method === 'messages.send' && !has('messages.send')) throw new Error('CAPABILITY_DENIED')
    if (method === 'attachments.uploadImage' && !has('attachments.upload')) throw new Error('CAPABILITY_DENIED')
    if (method.startsWith('characterCard.') && !has(method === 'characterCard.updateAttrs' ? 'characterCard.write' : 'characterCard.read')) throw new Error('CAPABILITY_DENIED')
    if (method === 'members.list' && params.scope === 'world-admins' && !has('world.admins.read')) throw new Error('CAPABILITY_DENIED')
    const contextSensitive = method.startsWith('storage.') || method === 'members.list' || method === 'member.getCurrent' || method.startsWith('characters.') || method.startsWith('characterCard.') || method.startsWith('theater.character.') || method === 'permissions.getCurrent' || method === 'events.publish' || method === 'events.subscribe' || method === 'messages.send' || method === 'attachments.uploadImage'
    requireContext(request, contextSensitive)
    switch (method) {
      case 'theater.dialogue.subscribe': {
        if (!has('theater.dialogue.subscribe')) throw new Error('CAPABILITY_DENIED')
        if (!hasOnlyKeys(params, ['identityId'])) throw new Error('INVALID_PARAMS')
        const identityId = boundedString(params.identityId, 100, true)
        session.disposeDialogue?.()
        session.disposeDialogue = deps.theaterDialogueSource!.subscribe(deps.channelId, identityId, ({ topic, payload }) => {
          if (closed || sessions.get(request.sessionId) !== session || !has('theater.dialogue.subscribe')) return
          try {
            ensureSessionContext()
            postEventToSession(request.sessionId, session, topic, payload)
          } catch { session.disposeDialogue?.(); session.disposeDialogue = undefined }
        })
        return { identityId }
      }
      case 'theater.dialogue.unsubscribe':
        if (!has('theater.dialogue.subscribe')) throw new Error('CAPABILITY_DENIED')
        session.disposeDialogue?.()
        session.disposeDialogue = undefined
        return null
      case 'theater.character.get': {
        if (!has('theater.character.read') || !deps.theaterCharacterSource) throw new Error('CAPABILITY_DENIED')
        if (!hasOnlyKeys(params, ['identityId'])) throw new Error('INVALID_PARAMS')
        const identityId = boundedString(params.identityId, 100, true)
        return theaterCharacterSnapshot(deps.theaterCharacterSource.getSnapshot(), identityId)
      }
      case 'theater.character.subscribe': {
        if (!has('theater.character.read') || !deps.theaterCharacterSource) throw new Error('CAPABILITY_DENIED')
        if (!hasOnlyKeys(params, ['identityId'])) throw new Error('INVALID_PARAMS')
        const identityId = boundedString(params.identityId, 100, true)
        session.theaterCharacterIdentityId = identityId
        return theaterCharacterSnapshot(deps.theaterCharacterSource.getSnapshot(), identityId)
      }
      case 'theater.character.unsubscribe':
        if (!has('theater.character.read')) throw new Error('CAPABILITY_DENIED')
        session.theaterCharacterIdentityId = undefined
        return null
      case 'context.get': if (!has('context.read')) throw new Error('CAPABILITY_DENIED'); return getContext()
      case 'user.getCurrent': if (!has('user.read')) throw new Error('CAPABILITY_DENIED'); return safeUser(deps.user.info)
      case 'member.getCurrent': if (!has('context.read')) throw new Error('CAPABILITY_DENIED'); return safeMember(deps.chat.curMember)
      case 'members.list': {
        if (!has('members.read')) throw new Error('CAPABILITY_DENIED')
        if (params.scope !== 'online' && params.scope !== 'guild' && params.scope !== 'world-admins') throw new Error('INVALID_PARAMS')
        const scope = params.scope
        if (scope === 'online') return safeMemberList(deps.chat.curChannelUsers || [])
        if (scope === 'world-admins') {
          if (typeof deps.chat.worldDetail !== 'function') return []
          const detail = await deps.chat.worldDetail(deps.worldId, { force: true })
          publishContextChanges()
          return safeWorldAdminList(Array.isArray(detail?.admins) ? detail.admins : [])
        }
        const guildId = safeString(deps.chat.curChannel?.guildId || deps.chat.curChannel?.guild_id)
        if (!guildId) return []
        const cursor = params.cursor === undefined ? '' : boundedString(params.cursor, 128)
        const result = await deps.chat.guildMemberListRaw(guildId, cursor)
        const items = result?.data?.data || result?.data || result || []
        return Array.isArray(items) ? safeMemberList(items) : []
      }
      case 'characters.list': if (!has('characters.read')) throw new Error('CAPABILITY_DENIED'); return getIdentities()
      case 'characters.getCurrent': if (!has('characters.read')) throw new Error('CAPABILITY_DENIED'); return currentCharacter()
      case 'characterCard.getStatus': return deps.characterCard.getCharacterApiStatus(deps.channelId)
      case 'characterCard.getCurrent': {
        let status = deps.characterCard.getCharacterApiStatus(deps.channelId)
        if (!status.available) return { status, card: null }
        const current = await deps.characterCard.getActiveCard(deps.channelId, { throwOnError: true })
        ensureSessionContext()
        status = deps.characterCard.getCharacterApiStatus(deps.channelId)
        return { status, card: status.available && current ? safeCurrentCharacterCard(current) : null }
      }
      case 'characterCard.listSnapshots': {
        await deps.characterSnapshot.refreshChannel(deps.channelId)
        ensureSessionContext()
        return deps.characterSnapshot.getChannelItems(deps.channelId).map(safeCharacterCardSnapshot)
      }
      case 'characterCard.getSnapshot': {
        if (!hasOnlyKeys(params, ['identityId'])) throw new Error('INVALID_PARAMS')
        const identityId = boundedString(params.identityId, 100, true)
        await deps.characterSnapshot.refreshChannel(deps.channelId)
        ensureSessionContext()
        const item = deps.characterSnapshot.getChannelItems(deps.channelId).find((snapshot) => snapshot.identityId === identityId)
        return item ? safeCharacterCardSnapshot(item) : null
      }
      case 'characterCard.updateAttrs': {
        if (!hasOnlyKeys(params, ['attrs']) || !isPlainObject(params.attrs)) throw new Error('INVALID_PARAMS')
        if (jsonSize(params.attrs) > 1024 * 1024) throw new Error('PAYLOAD_TOO_LARGE')
        const result = await deps.characterCard.patchActiveCardAttrs(deps.channelId, params.attrs)
        ensureSessionContext()
        return result
      }
      case 'permissions.getCurrent': if (!has('permissions.read')) throw new Error('CAPABILITY_DENIED'); return getContext().permissions
      case 'channel.getState': if (!has('context.read')) throw new Error('CAPABILITY_DENIED'); return { id: deps.channelId, worldId: deps.worldId, ...(deps.scope.type === 'iform' ? { formId: deps.scope.id } : { scopeType: deps.scope.type, scopeId: deps.scope.id }) }
      case 'connection.getState': if (!has('context.read')) throw new Error('CAPABILITY_DENIED'); return connectionState()
      case 'storage.get': { const key = boundedString(params.key, 128, true); return (await deps.chat.sendAPI('channel.extension.storage.get', { ...scopeParams(), key }))?.data || null }
      case 'storage.set': { const key = boundedString(params.key, 128, true); if (params.value === undefined) throw new Error('INVALID_PARAMS'); if (params.ifRevision !== undefined && (!Number.isInteger(params.ifRevision) || params.ifRevision < 0)) throw new Error('INVALID_PARAMS'); if (jsonSize(params.value) > 64 * 1024) throw new Error('PAYLOAD_TOO_LARGE'); const result = (await deps.chat.sendAPI('channel.extension.storage.set', { ...scopeParams(), key, value: params.value, if_revision: params.ifRevision }))?.data; if (Number.isFinite(result?.seq)) storageSeq = Math.max(storageSeq, Number(result.seq)); return result }
      case 'storage.delete': { const key = boundedString(params.key, 128, true); if (params.ifRevision !== undefined && (!Number.isInteger(params.ifRevision) || params.ifRevision < 0)) throw new Error('INVALID_PARAMS'); const result = (await deps.chat.sendAPI('channel.extension.storage.delete', { ...scopeParams(), key, if_revision: params.ifRevision }))?.data; if (Number.isFinite(result?.seq)) storageSeq = Math.max(storageSeq, Number(result.seq)); return result }
      case 'storage.list': { const limit = params.limit === undefined ? 256 : params.limit; if (!Number.isInteger(limit) || limit < 1 || limit > 256) throw new Error('INVALID_PARAMS'); const prefix = params.prefix === undefined ? '' : boundedString(params.prefix, 128); const cursor = params.cursor === undefined ? '' : boundedString(params.cursor, 128); return (await deps.chat.sendAPI('channel.extension.storage.list', { ...scopeParams(), prefix, cursor, limit }))?.data }
      case 'storage.snapshot': { const result = (await deps.chat.sendAPI('channel.extension.storage.snapshot', { ...scopeParams() }))?.data; if (Number.isFinite(result?.seq)) storageSeq = Number(result.seq); return result }
      case 'events.publish': {
        const topic = boundedString(params.topic, 64, true)
        if (params.payload === undefined) throw new Error('INVALID_PARAMS')
        if (jsonSize(params.payload) > 16 * 1024) throw new Error('PAYLOAD_TOO_LARGE')
        const result = (await deps.chat.sendAPI('channel.extension.events.publish', {
          ...scopeParams(), topic, payload: params.payload,
          ...(deps.theaterContext ? { theaterContext: deps.theaterContext } : {}),
        }))?.data
        reportPublishedEvent(topic, result)
        return result
      }
      case 'events.subscribe': { const topic = boundedString(params.topic, 64, true); session.topics.add(topic); return (await deps.chat.sendAPI('channel.extension.events.subscribe', { ...scopeParams(), topic }))?.data }
      case 'messages.send': {
        const text = boundedString(params.text, 64 * 1024, true)
        const identityId = params.identityId === undefined ? currentActiveIdentityId() : boundedString(params.identityId, 100, true)
        const identity = identityId ? getIdentities().find((item: any) => item.id === identityId) : undefined
        if (identityId && !identity) throw new Error('PERMISSION_DENIED')
        const variantId = params.identityVariantId === undefined ? undefined : boundedString(params.identityVariantId, 100, true)
        if (variantId && (!identity || !identity.variants.some((variant: any) => variant.id === variantId))) throw new Error('PERMISSION_DENIED')
        const replyTo = params.replyTo === undefined ? undefined : boundedString(params.replyTo, 100, true)
        if (params.icMode !== undefined && params.icMode !== 'ic' && params.icMode !== 'ooc') throw new Error('INVALID_PARAMS')
        const clientId = deps.scope.type === 'iform' ? `iform_embed:${deps.scope.id}:${randomEmbedId('message')}` : `channel_extension:${deps.scope.type}:${deps.scope.id}:${randomEmbedId('message')}`
        return deps.chat.messageCreate(text, replyTo, undefined, clientId, identityId || null, undefined, undefined, undefined, undefined, variantId, params.icMode, deps.channelId)
      }
      case 'attachments.uploadImage': {
        if (!hasOnlyKeys(params, ['file', 'filename'])) throw new Error('INVALID_PARAMS')
        if (typeof Blob === 'undefined' || !(params.file instanceof Blob)) throw new Error('INVALID_PARAMS')
        const mimeType = safeString(params.file.type, 128).toLowerCase()
        if (!mimeType.startsWith('image/')) throw new Error('INVALID_PARAMS')
        const filename = params.filename === undefined ? undefined : boundedString(params.filename, 255)
        const result = await uploadChannelEmbedImage(params.file, filename || undefined)
        requireContext(request, true)
        return result
      }
      default: throw new Error('NOT_FOUND: method')
    }
  }
  const stop = () => {
    if (closed) return
    closed = true
    listeners.forEach(dispose => dispose())
    disposeTheaterCharacterSource?.()
    closeActiveSessions()
    seenGatewayEvents.clear()
    reportedPublishedEvents.clear()
    deps.onDispose?.()
  }
  listeners.push(watch(() => [deps.user?.info?.id, deps.user?.token, deps.chat.curChannel?.id, deps.chat.currentWorldId, deps.isCurrent?.()], () => {
    try { ensureSessionContext(); if (deps.user?.token !== authenticatedToken) stop() } catch { stop() }
  }, { flush: 'sync' }))
  const ready = refreshWritePermission().catch(() => { canWrite = false })
  return {
    ready, stop, dispose: stop, getContext, isActive: () => !closed,
    openSession(emit: ExtensionSession['emit']) {
      ensureSessionContext()
      const id = randomEmbedId('extension')
      const session: ExtensionSession = { emit, topics: new Set() }
      sessions.set(id, session)
      return {
        initialize: async () => { await ready; if (!closed && has('storage.read')) await sendSnapshotToSession(session) },
        request: async (method: string, params?: unknown, version = contextVersion) => {
          if (sessions.get(id) !== session) throw new Error('SESSION_EXPIRED')
          const result = await dispatchRequest({ sessionId: id, method, params, contextVersion: version }, session)
          if (method !== 'session.close') ensureSessionContext()
          return result
        },
        dispose: () => { session.disposeDialogue?.(); session.disposeDialogue = undefined; sessions.delete(id); session.topics.clear() },
      }
    },
  }
}
