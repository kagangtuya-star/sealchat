import type { ChannelIForm, ChannelIFormBridgePolicy } from '@/types/iform'
import { chatEvent } from '@/stores/chat'
import type { CharacterCardApiStatus, CharacterCardAttrsPatchResult, CharacterCardData } from '@/stores/characterCard'
import type { ChannelCharacterSnapshotItem } from '@/stores/channelCharacterSnapshot'
import { resolveAttachmentUrl } from '@/composables/useAttachmentResolver'
import { createChannelEmbedTheaterDialogue } from './channelEmbedTheaterDialogue'
import {
  CHANNEL_EMBED_EVENT,
  CHANNEL_EMBED_HANDSHAKE_ACK,
  CHANNEL_EMBED_RESPONSE,
  isEmbedHandshakeRequest,
  isEmbedRequest,
  randomEmbedId,
  type EmbedEvent,
  type EmbedRequest,
} from './channelEmbedProtocol'

const defaultCapabilities = ['context.read', 'user.read', 'members.read', 'world.admins.read', 'characters.read', 'characterCard.read', 'characterCard.write', 'permissions.read', 'storage.read', 'storage.write', 'events.subscribe', 'events.publish', 'messages.send']
const publicErrorCodes = new Set(['ORIGIN_DENIED', 'HANDSHAKE_FAILED', 'SESSION_EXPIRED', 'CONTEXT_CHANGED', 'CAPABILITY_DENIED', 'PERMISSION_DENIED', 'INVALID_PARAMS', 'NOT_FOUND', 'REVISION_CONFLICT', 'QUOTA_EXCEEDED', 'PAYLOAD_TOO_LARGE', 'RATE_LIMITED', 'WS_OFFLINE', 'TIMEOUT', 'INTERNAL_ERROR'])

const dialogueSource = createChannelEmbedTheaterDialogue(chatEvent, resolveAttachmentUrl)
type EmbedSession = { port: MessagePort; source: WindowProxy; origin: string; disposeDialogue?: () => void }

type HostDeps = {
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
  form: ChannelIForm
  iframe: HTMLIFrameElement
  worldId: string
  channelId: string
}

const safeString = (value: unknown, max = 512) => typeof value === 'string' ? value.slice(0, max) : ''
const jsonSize = (value: unknown) => { try { return new TextEncoder().encode(JSON.stringify(value)).byteLength } catch { return Number.POSITIVE_INFINITY } }
const safeUser = (user: any) => user?.id ? { id: String(user.id), displayName: safeString(user.nick || user.nickname || user.username) || String(user.id), avatar: safeString(user.avatar) } : null
const safeUserList = (users: any[]) => users.map(safeUser).filter((user): user is NonNullable<ReturnType<typeof safeUser>> => !!user)
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
const safeAttrs = (value: unknown): Record<string, any> => value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, any> : {}
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

const publicError = (error: unknown) => {
  const code = errorCode(error)
  if (code === 'INTERNAL_ERROR' || code === 'WS_OFFLINE') return { code, message: code === 'WS_OFFLINE' ? 'WebSocket unavailable' : 'Embed request failed' }
  const message = String((error as any)?.message || error || code)
  const details = code === 'REVISION_CONFLICT'
    ? (() => { const match = message.match(/currentRevision=(\d+)/); return match ? { currentRevision: Number(match[1]) } : undefined })()
    : undefined
  return { code, message: message.replace(new RegExp(`^${code}:\\s*`), '').slice(0, 512), details }
}

export const createChannelEmbedHost = (deps: HostDeps) => {
  let policy = normalizedPolicy(deps.form.bridgePolicy)
  let sessionId = randomEmbedId('session')
  const nonceSeen = new Set<string>()
  const sessions = new Map<string, EmbedSession>()
  const listeners: Array<() => void> = []
  const seenGatewayEvents = new Set<string>()
  const authenticatedUserId = safeString(deps.user?.info?.id, 100)
  let contextVersion = 1
  let lastContextKey = ''
  let storageSeq = 0
  let storageEventChain: Promise<void> = Promise.resolve()
  let closed = false

  const expectedOrigin = (() => {
    try {
      const src = deps.iframe.src || deps.form.url
      return src ? new URL(src, window.location.href).origin : ''
    } catch { return '' }
  })()
  const allowedOrigin = (origin: string) => {
    const allowed = policy.allowedOrigins
    if (allowed.length > 0) return origin === 'null' ? true : allowed.includes(origin)
    if (origin === 'null') return true
    return !!expectedOrigin && expectedOrigin !== 'null' && expectedOrigin === origin
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
    if (!defaultCapabilities.includes(capability) && capability !== 'theater.dialogue.subscribe') return false
    if (['members.read', 'characterCard.write', 'storage.write', 'events.publish', 'messages.send'].includes(capability) && (!deps.chat.curMember || deps.chat.observerMode)) return false
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
  const contextKey = () => JSON.stringify({ activeChannelId: safeString(deps.chat.curChannel?.id), channel: { id: deps.channelId, name: safeString(deps.chat.curChannel?.name), type: safeString(deps.chat.curChannel?.type) }, worldId: deps.worldId, member: safeMember(deps.chat.curMember), authorization: worldAuthorization(), character: currentCharacter(), state: deps.chat.connectState, users: deps.chat.curChannelUsers?.map((user: any) => safeString(user?.id, 100)) })
  const postEvent = (topic: string, payload: unknown, seq?: number) => {
    sessions.forEach((session) => {
      const event: EmbedEvent = { type: CHANNEL_EMBED_EVENT, version: 1, sessionId, eventId: randomEmbedId('event'), topic, seq, contextVersion, payload, at: Date.now() }
      try { session.port.postMessage(event) } catch { /* session cleaned on next request */ }
    })
  }
  const publishContextChanges = () => {
    const next = contextKey()
    if (next === lastContextKey) return
    lastContextKey = next
    contextVersion++
    if (has('context.read')) {
      postEvent('context.changed', getContext())
      postEvent('connection.changed', connectionState())
    }
    if (has('characters.read')) postEvent('characters.changed', getIdentities())
    if (has('members.read')) postEvent('members.changed', safeMemberList(deps.chat.curChannelUsers || []))
    if (has('permissions.read')) postEvent('permissions.changed', getContext().permissions)
  }
  const refreshOnlineMembers = async () => {
    if (!has('members.read')) return
    try {
      const result = await deps.chat.sendAPI('channel.member.list.online', { channel_id: deps.channelId })
      const items = result?.data?.data || result?.data || result || []
      if (Array.isArray(items) && safeString(deps.chat.curChannel?.id, 100) === deps.channelId) {
        deps.chat.curChannelUsers = items
        publishContextChanges()
      }
    } catch { /* next gateway event or reconnect retries */ }
  }
  const closeActiveSessions = () => {
    sessions.forEach((session) => {
      session.disposeDialogue?.()
      try {
        session.port.postMessage({ type: CHANNEL_EMBED_EVENT, version: 1, sessionId, eventId: randomEmbedId('event'), topic: 'session.closed', contextVersion, payload: null, at: Date.now() })
      } catch { /* ignore closed ports */ }
      session.port.close()
    })
    sessions.clear()
  }
  const sendSnapshotToSession = async (session: { port: MessagePort }) => {
    try {
      const result = await deps.chat.sendAPI('iform.storage.snapshot', { channel_id: deps.channelId, form_id: deps.form.id })
      const snapshotSeq = Number(result?.data?.seq || 0)
      if (!Number.isFinite(snapshotSeq) || snapshotSeq < storageSeq) return
      storageSeq = snapshotSeq
      session.port.postMessage({ type: CHANNEL_EMBED_EVENT, version: 1, sessionId, eventId: randomEmbedId('event'), topic: 'storage.changed', seq: snapshotSeq, contextVersion, payload: { snapshot: result?.data }, at: Date.now() })
    } catch { /* request path reports WS_OFFLINE */ }
  }
  const onGatewayEvent = (event: any) => {
    const embed = event?.channelIFormEmbed
    if (!embed || embed.channelId !== deps.channelId || embed.formId !== deps.form.id) return
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
          const result = await deps.chat.sendAPI('iform.storage.get', { channel_id: deps.channelId, form_id: deps.form.id, key: embed.key })
          value = result?.data?.value
        } catch {
          sessions.forEach((session) => { void sendSnapshotToSession(session) })
          return
        }
      }
      postEvent('storage.changed', { key: embed.key, op: embed.op, revision: embed.revision, value }, seq)
    }).catch(() => undefined)
  }
  const eventNames = ['channel-iform-embed', 'channel-iform-updated', 'channel-identity-updated', 'channel-identities-updated', 'channel-member-updated', 'channel-presence-updated', 'channel-updated', 'world-updated', 'channel-switch-to', 'channel-context-cleared', 'connected', 'connection.changed']
  eventNames.forEach((name) => {
    const handler = (event: any) => {
      if (name === 'channel-iform-embed') {
        onGatewayEvent(event)
        return
      }
      const eventChannelId = safeString(event?.channel?.id, 100)
      if (name === 'channel-iform-updated' && (!eventChannelId || eventChannelId === deps.channelId)) {
        const forms = event?.iform?.forms
        const changedForm = Array.isArray(forms)
          ? forms.find((item: any) => safeString(item?.id, 100) === deps.form.id)
          : event?.iform?.form?.id === deps.form.id ? event.iform.form : undefined
        if (Array.isArray(forms) && !changedForm) {
          stop()
          return
        }
        const previousPolicy = JSON.stringify(policy)
        policy = normalizedPolicy(changedForm?.bridgePolicy || deps.form.bridgePolicy)
        if (!policy.enabled) {
          stop()
          return
        }
        if (JSON.stringify(policy) !== previousPolicy) {
          if ([...sessions.values()].some((session) => !allowedOrigin(session.origin))) {
            stop()
            return
          }
          closeActiveSessions()
          contextVersion++
        }
      }
      const activeChannelId = safeString(deps.chat.curChannel?.id, 100)
      if (!activeChannelId || activeChannelId !== deps.channelId) {
        stop()
        return
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

  const requestParams = (request: EmbedRequest) => {
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
    if (!authenticatedUserId || safeString(deps.user?.info?.id, 100) !== authenticatedUserId) throw new Error('SESSION_EXPIRED')
    const activeChannelId = safeString(deps.chat.curChannel?.id, 100)
    if (!activeChannelId || activeChannelId !== deps.channelId) throw new Error('CONTEXT_CHANGED')
  }
  const requireContext = (request: EmbedRequest, mutating: boolean) => {
    ensureSessionContext()
    if (mutating && request.contextVersion !== contextVersion) throw new Error('CONTEXT_CHANGED')
  }
  const dispatchRequest = async (request: EmbedRequest, session: EmbedSession) => {
    if (request.method === 'session.close') {
      session.disposeDialogue?.()
      session.disposeDialogue = undefined
      sessions.delete(request.sessionId)
      return null
    }
    ensureSessionContext()
    const params = requestParams(request)
    const method = request.method
    if (method.startsWith('storage.') && !has(method === 'storage.get' || method === 'storage.list' || method === 'storage.snapshot' ? 'storage.read' : 'storage.write')) throw new Error('CAPABILITY_DENIED')
    if (method === 'events.publish' && !has('events.publish')) throw new Error('CAPABILITY_DENIED')
    if (method === 'events.subscribe' && !has('events.subscribe')) throw new Error('CAPABILITY_DENIED')
    if (method === 'messages.send' && !has('messages.send')) throw new Error('CAPABILITY_DENIED')
    if (method.startsWith('characterCard.') && !has(method === 'characterCard.updateAttrs' ? 'characterCard.write' : 'characterCard.read')) throw new Error('CAPABILITY_DENIED')
    if (method === 'members.list' && params.scope === 'world-admins' && !has('world.admins.read')) throw new Error('CAPABILITY_DENIED')
    const contextSensitive = method.startsWith('storage.') || method === 'members.list' || method === 'member.getCurrent' || method.startsWith('characters.') || method.startsWith('characterCard.') || method === 'permissions.getCurrent' || method === 'events.publish' || method === 'events.subscribe' || method === 'messages.send'
    requireContext(request, contextSensitive)
    switch (method) {
      case 'theater.dialogue.subscribe': {
        if (!has('theater.dialogue.subscribe')) throw new Error('CAPABILITY_DENIED')
        if (!hasOnlyKeys(params, ['identityId'])) throw new Error('INVALID_PARAMS')
        const identityId = boundedString(params.identityId, 100, true)
        if (!getIdentities().some((identity: { id: string }) => identity.id === identityId)) throw new Error('NOT_FOUND: identity')
        session.disposeDialogue?.()
        session.disposeDialogue = dialogueSource.subscribe(deps.channelId, identityId, ({ topic, payload }) => {
          if (closed || sessions.get(request.sessionId) !== session || !has('theater.dialogue.subscribe')) return
          try {
            ensureSessionContext()
            session.port.postMessage({ type: CHANNEL_EMBED_EVENT, version: 1, sessionId: request.sessionId, eventId: randomEmbedId('event'), topic, contextVersion, payload, at: Date.now() } satisfies EmbedEvent)
          } catch { session.disposeDialogue?.(); session.disposeDialogue = undefined }
        })
        return { identityId }
      }
      case 'theater.dialogue.unsubscribe':
        if (!has('theater.dialogue.subscribe')) throw new Error('CAPABILITY_DENIED')
        session.disposeDialogue?.()
        session.disposeDialogue = undefined
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
      case 'channel.getState': if (!has('context.read')) throw new Error('CAPABILITY_DENIED'); return { id: deps.channelId, worldId: deps.worldId, formId: deps.form.id }
      case 'connection.getState': if (!has('context.read')) throw new Error('CAPABILITY_DENIED'); return connectionState()
      case 'storage.get': { const key = boundedString(params.key, 128, true); return (await deps.chat.sendAPI('iform.storage.get', { channel_id: deps.channelId, form_id: deps.form.id, key }))?.data || null }
      case 'storage.set': { const key = boundedString(params.key, 128, true); if (params.value === undefined) throw new Error('INVALID_PARAMS'); if (params.ifRevision !== undefined && (!Number.isInteger(params.ifRevision) || params.ifRevision < 0)) throw new Error('INVALID_PARAMS'); if (jsonSize(params.value) > 64 * 1024) throw new Error('PAYLOAD_TOO_LARGE'); const result = (await deps.chat.sendAPI('iform.storage.set', { channel_id: deps.channelId, form_id: deps.form.id, key, value: params.value, if_revision: params.ifRevision }))?.data; if (Number.isFinite(result?.seq)) storageSeq = Math.max(storageSeq, Number(result.seq)); return result }
      case 'storage.delete': { const key = boundedString(params.key, 128, true); if (params.ifRevision !== undefined && (!Number.isInteger(params.ifRevision) || params.ifRevision < 0)) throw new Error('INVALID_PARAMS'); const result = (await deps.chat.sendAPI('iform.storage.delete', { channel_id: deps.channelId, form_id: deps.form.id, key, if_revision: params.ifRevision }))?.data; if (Number.isFinite(result?.seq)) storageSeq = Math.max(storageSeq, Number(result.seq)); return result }
      case 'storage.list': { const limit = params.limit === undefined ? 256 : params.limit; if (!Number.isInteger(limit) || limit < 1 || limit > 256) throw new Error('INVALID_PARAMS'); const prefix = params.prefix === undefined ? '' : boundedString(params.prefix, 128); const cursor = params.cursor === undefined ? '' : boundedString(params.cursor, 128); return (await deps.chat.sendAPI('iform.storage.list', { channel_id: deps.channelId, form_id: deps.form.id, prefix, cursor, limit }))?.data }
      case 'storage.snapshot': { const result = (await deps.chat.sendAPI('iform.storage.snapshot', { channel_id: deps.channelId, form_id: deps.form.id }))?.data; if (Number.isFinite(result?.seq)) storageSeq = Number(result.seq); return result }
      case 'events.publish': { const topic = boundedString(params.topic, 64, true); if (params.payload === undefined) throw new Error('INVALID_PARAMS'); if (jsonSize(params.payload) > 16 * 1024) throw new Error('PAYLOAD_TOO_LARGE'); return (await deps.chat.sendAPI('iform.event.publish', { channel_id: deps.channelId, form_id: deps.form.id, topic, payload: params.payload }))?.data }
      case 'events.subscribe': { const topic = boundedString(params.topic, 64, true); return (await deps.chat.sendAPI('iform.event.subscribe', { channel_id: deps.channelId, form_id: deps.form.id, topic }))?.data }
      case 'messages.send': {
        const text = boundedString(params.text, 64 * 1024, true)
        const identityId = params.identityId === undefined ? currentActiveIdentityId() : boundedString(params.identityId, 100, true)
        const identity = identityId ? getIdentities().find((item: any) => item.id === identityId) : undefined
        if (identityId && !identity) throw new Error('PERMISSION_DENIED')
        const variantId = params.identityVariantId === undefined ? undefined : boundedString(params.identityVariantId, 100, true)
        if (variantId && (!identity || !identity.variants.some((variant: any) => variant.id === variantId))) throw new Error('PERMISSION_DENIED')
        const replyTo = params.replyTo === undefined ? undefined : boundedString(params.replyTo, 100, true)
        if (params.icMode !== undefined && params.icMode !== 'ic' && params.icMode !== 'ooc') throw new Error('INVALID_PARAMS')
        return deps.chat.messageCreate(text, replyTo, undefined, `iform_embed:${deps.form.id}:${randomEmbedId('message')}`, identityId || null, undefined, undefined, undefined, undefined, variantId, params.icMode, deps.channelId)
      }
      default: throw new Error('NOT_FOUND: method')
    }
  }
  const handlePortMessage = (session: EmbedSession, value: unknown) => {
    if (closed || !isEmbedRequest(value) || value.sessionId !== sessionId || sessions.get(sessionId) !== session) return
    void dispatchRequest(value, session).then((result) => session.port.postMessage({ type: CHANNEL_EMBED_RESPONSE, version: 1, sessionId, requestId: value.requestId, ok: true, result }), (error) => session.port.postMessage({ type: CHANNEL_EMBED_RESPONSE, version: 1, sessionId, requestId: value.requestId, ok: false, error: publicError(error) }))
      .catch(() => { session.disposeDialogue?.(); session.disposeDialogue = undefined })
      .finally(() => { if (value.method === 'session.close') session.port.close() })
  }
  const handleHandshake = (event: MessageEvent) => {
    if (closed || !authenticatedUserId || event.source !== deps.iframe.contentWindow || !isEmbedHandshakeRequest(event.data) || nonceSeen.has(event.data.nonce)) return
    if (!policy.enabled || !allowedOrigin(event.origin)) { event.source?.postMessage({ type: CHANNEL_EMBED_HANDSHAKE_ACK, version: 1, nonce: event.data.nonce, ok: false, error: { code: 'ORIGIN_DENIED', message: 'Embed origin denied' } }, event.origin === 'null' ? '*' : event.origin); return }
    nonceSeen.add(event.data.nonce)
    const previousSessionId = sessionId
    sessions.forEach((previousSession) => {
      previousSession.disposeDialogue?.()
      try {
        previousSession.port.postMessage({ type: CHANNEL_EMBED_EVENT, version: 1, sessionId: previousSessionId, eventId: randomEmbedId('event'), topic: 'session.closed', contextVersion, payload: null, at: Date.now() })
      } catch { /* ignore closed ports */ }
      previousSession.port.close()
    })
    sessions.clear()
    sessionId = randomEmbedId('session')
    const channel = new MessageChannel()
    const session = { port: channel.port1, source: event.source as WindowProxy, origin: event.origin }
    sessions.set(sessionId, session)
    channel.port1.onmessage = (message) => handlePortMessage(session, message.data)
    channel.port1.start?.()
    const targetOrigin = event.origin === 'null' ? '*' : event.origin
    event.source?.postMessage({ type: CHANNEL_EMBED_HANDSHAKE_ACK, version: 1, nonce: event.data.nonce, ok: true, sessionId, contextVersion, capabilities: effectiveCapabilities() }, targetOrigin, [channel.port2])
    if (has('storage.read')) void sendSnapshotToSession(session)
  }
  window.addEventListener('message', handleHandshake)
  const stop = () => {
    if (closed) return
    closed = true
    window.removeEventListener('message', handleHandshake)
    listeners.forEach((dispose) => dispose())
    closeActiveSessions()
  }
  return { get sessionId() { return sessionId }, stop, getContext, isActive: () => !closed }
}
