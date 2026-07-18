import type {
  BridgeCharacterAppearance,
  BridgeCharacterDecoration,
  BridgeCharacterVariant,
  BridgeImageRef,
  BridgeRoleSnapshot,
  SealChatBridgeMessageEvent,
  SealChatBridgeMessagePayload,
} from './sealchatBridgeProtocol'
import {
  theaterPresentationPatchSchema,
  theaterPresentationSchema,
  type TheaterPresentation,
  type TheaterPresentationPatch,
} from '../types/theaterPresentation'
import type {
  TheaterDialogueMessagePayload,
  TheaterDialogueMessageRemovedPayload,
} from '../views/theater/bridge/theater-bridge-protocol'
import { isSafeStageImageUrl } from '../views/theater/shared/stage-types'
import { hasPerformanceContent } from '../utils/tiptap-performance-parser'

type AvatarDecorationLike = {
  id?: string
  enabled: boolean
  decorationId?: string
  resourceAttachmentId?: string
  fallbackAttachmentId?: string
  settings?: object
}

const inlineImageTokenPattern = /\[\[(?:图片:[^\]]+|img:[^\]]+)\]\]/gi
const botStateWidgetPrefix = '[[STATE_WIDGET]]'

type TipTapNode = {
  type?: string
  text?: string
  attrs?: Record<string, unknown>
  content?: TipTapNode[]
}

const isMentionNodeType = (value: unknown): boolean => {
  const normalized = String(value || '').trim().toLowerCase()
  return normalized === 'mention' || normalized === 'satorimention'
}

type IdentityLike = {
  id?: string
  displayName?: string
  color?: string
  avatarAttachmentId?: string
  avatarAttachment?: string
  isTemporary?: boolean
  icOocOnActivate?: '' | 'ic' | 'ooc'
  avatarDecoration?: AvatarDecorationLike | null
  avatarDecorations?: AvatarDecorationLike[] | null
  theaterPresentation?: TheaterPresentation | null
}

type VariantLike = {
  id?: string
  keyword?: string
  selectorEmoji?: string
  note?: string
  enabled?: boolean
  displayName?: string
  color?: string
  avatarAttachmentId?: string
  appearance?: Record<string, unknown>
  theaterPresentation?: TheaterPresentationPatch | null
  updatedAt?: string
}

type ResolvedAppearanceLike = {
  displayName?: string
  color?: string
  avatarAttachmentId?: string
  avatarDecorations?: AvatarDecorationLike[] | null
  theaterPresentation?: TheaterPresentation | null
}

type BridgeMessageLike = {
  id?: string
  content?: unknown
  contentRichText?: unknown
  createdAt?: number
  timestamp?: number
  displayOrder?: number
  icMode?: string
  ic_mode?: string
  isWhisper?: boolean
  is_whisper?: boolean
  isArchived?: boolean
  is_archived?: boolean
  isDeleted?: boolean
  is_deleted?: boolean
  isRevoked?: boolean
  is_revoked?: boolean
  identity?: IdentityLike | null
  senderRoleId?: string
  sender_role_id?: string
  sender_identity_name?: string
  sender_identity_color?: string
  sender_identity_avatar_id?: string
  sender_identity_variant_id?: string
  sender_theater_presentation?: unknown
}

const isTipTapJson = (content: string): boolean => {
  if (!content || typeof content !== 'string') {
    return false
  }
  try {
    let parsed = JSON.parse(content)
    if (typeof parsed === 'string') {
      try {
        parsed = JSON.parse(parsed)
      } catch {
        return false
      }
    }
    return Boolean(parsed && typeof parsed === 'object' && parsed.type === 'doc')
  } catch {
    return false
  }
}

const normalizeMessageContent = (value: unknown): string => {
  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (trimmed.startsWith('"{') || trimmed.startsWith("'{")) {
      try {
        const decoded = JSON.parse(trimmed)
        if (typeof decoded === 'string' && decoded.trim().startsWith('{')) return decoded
      } catch {
        // Keep original string for non-JSON message content.
      }
    }
    return value
  }
  if (value && typeof value === 'object') {
    try {
      return JSON.stringify(value)
    } catch {
      return ''
    }
  }
  return String(value || '')
}

const extractTipTapText = (node: TipTapNode | null | undefined): string => {
  if (!node) {
    return ''
  }
  if (typeof node.text === 'string') {
    return node.text
  }
  if (node.type === 'hardBreak') {
    return '\n'
  }
  if (isMentionNodeType(node.type)) {
    const mentionId = String(node.attrs?.id || '').trim()
    const mentionName = String(node.attrs?.name || '').trim()
    return `@${mentionName || mentionId || '用户'}`
  }
  if (Array.isArray(node.content) && node.content.length > 0) {
    const joined = node.content.map((child) => extractTipTapText(child)).join('')
    if (node.type === 'paragraph' || node.type === 'heading' || node.type === 'listItem') {
      return `${joined}\n`
    }
    return joined
  }
  return ''
}

const tiptapJsonToPlainText = (content: string): string => {
  try {
    const parsed = JSON.parse(content) as TipTapNode
    return extractTipTapText(parsed).replace(/\n+$/, '')
  } catch {
    return ''
  }
}

const decodeHtmlEntities = (content: string): string => content.replace(
  /&(#x[0-9a-f]+|#\d+|amp|lt|gt|quot|apos|nbsp);/gi,
  (match, entity: string) => {
    const normalized = entity.toLowerCase()
    if (normalized === 'amp') return '&'
    if (normalized === 'lt') return '<'
    if (normalized === 'gt') return '>'
    if (normalized === 'quot') return '"'
    if (normalized === 'apos') return "'"
    if (normalized === 'nbsp') return ' '
    const radix = normalized.startsWith('#x') ? 16 : 10
    const codePoint = Number.parseInt(normalized.slice(radix === 16 ? 2 : 1), radix)
    if (!Number.isInteger(codePoint) || codePoint < 0 || codePoint > 0x10ffff) return match
    try {
      return String.fromCodePoint(codePoint)
    } catch {
      return match
    }
  },
)

const htmlToPlainText = (content: string): string => decodeHtmlEntities(content
  .replace(/<br\s*\/?>/gi, '\n')
  .replace(/<\/(?:p|div|li|blockquote|h[1-6]|pre)>/gi, '\n')
  .replace(/<[^>]+>/g, '')
  .replace(/\n{3,}/g, '\n\n')
  .replace(/\n+$/, ''))

export const normalizeBridgePlainText = (raw: string): string => {
  const content = String(raw || '')
  if (!content) {
    return ''
  }

  const plainText = isTipTapJson(content)
    ? tiptapJsonToPlainText(content)
    : /<\/?[a-z][\s\S]*>/i.test(content)
      ? htmlToPlainText(content)
      : decodeHtmlEntities(content)
  const trimmedStart = plainText.trimStart()
  const withoutStateWidget = trimmedStart.startsWith(botStateWidgetPrefix)
    ? `${plainText.slice(0, plainText.length - trimmedStart.length)}${trimmedStart.slice(botStateWidgetPrefix.length).replace(/^\s*/, '')}`
    : plainText

  return withoutStateWidget.replace(inlineImageTokenPattern, '[图片]')
}

const resolveAvatarUrl = (
  resolveAttachmentUrl: (token?: string) => string,
  ...tokens: Array<string | undefined>
): string => {
  const normalizeAbsoluteUrl = (value: string): string => {
    if (!value.startsWith('//')) {
      return value
    }
    const protocol = typeof globalThis.location?.protocol === 'string' && globalThis.location.protocol
      ? globalThis.location.protocol
      : 'https:'
    return `${protocol}${value}`
  }

  for (const token of tokens) {
    if (typeof token === 'string' && token.trim()) {
      const resolved = normalizeAbsoluteUrl(resolveAttachmentUrl(token))
      if (resolved && isSafeStageImageUrl(resolved)) return resolved
    }
  }
  return ''
}

const buildImageRef = (
  token: string | undefined,
  resolveAttachmentUrl: (token?: string) => string,
  alt?: string,
): BridgeImageRef | null => {
  const resourceId = String(token || '').trim()
  if (!resourceId) return null
  const url = resolveAvatarUrl(resolveAttachmentUrl, resourceId)
  if (!url) return null
  return { resourceId, url, ...(alt ? { alt } : {}) }
}

const buildFirstImageRef = (
  tokens: Array<string | undefined>,
  resolveAttachmentUrl: (token?: string) => string,
  alt?: string,
): BridgeImageRef | null => {
  for (const token of tokens) {
    const image = buildImageRef(token, resolveAttachmentUrl, alt)
    if (image) return image
  }
  return null
}

const buildDecorations = (
  decorations: AvatarDecorationLike[] | null | undefined,
  resolveAttachmentUrl: (token?: string) => string,
): BridgeCharacterDecoration[] => (Array.isArray(decorations) ? decorations : [])
  .map((decoration, index) => {
    const primaryToken = String(decoration.resourceAttachmentId || '').trim()
    const fallbackToken = String(decoration.fallbackAttachmentId || '').trim()
    const resource = buildFirstImageRef([primaryToken, fallbackToken], resolveAttachmentUrl)
    if (!resource) return null
    const fallbackResource = fallbackToken && fallbackToken !== resource.resourceId
      ? buildImageRef(fallbackToken, resolveAttachmentUrl)
      : null
    return {
      id: String(decoration.id || decoration.decorationId || `decoration-${index}`).trim(),
      resource,
      enabled: decoration.enabled === true,
      zIndex: Number.isFinite((decoration.settings as { zIndex?: number } | undefined)?.zIndex)
        ? Number((decoration.settings as { zIndex?: number }).zIndex)
        : 1,
      settings: { ...(decoration.settings || {}) },
      extensions: {
        ...(decoration.decorationId ? { decorationId: decoration.decorationId } : {}),
        ...(fallbackResource ? { fallbackResource } : {}),
      },
    } satisfies BridgeCharacterDecoration
  })
  .filter((item): item is BridgeCharacterDecoration => Boolean(item))

const buildAppearance = ({
  displayName,
  color,
  avatarAttachmentId,
  avatarFallbackAttachmentIds,
  decorations,
  theaterPresentation,
  resolveAttachmentUrl,
}: {
  displayName?: string
  color?: string
  avatarAttachmentId?: string
  avatarFallbackAttachmentIds?: Array<string | undefined>
  decorations?: AvatarDecorationLike[] | null
  theaterPresentation?: TheaterPresentation | null
  resolveAttachmentUrl: (token?: string) => string
}): BridgeCharacterAppearance => {
  const parsedTheaterPresentation = theaterPresentation === null
    ? { success: true as const, data: null }
    : theaterPresentationSchema.safeParse(theaterPresentation)
  return {
    displayName: String(displayName || ''),
    color: String(color || ''),
    avatar: buildFirstImageRef(
      [avatarAttachmentId, ...(avatarFallbackAttachmentIds || [])],
      resolveAttachmentUrl,
      displayName,
    ),
    decorations: buildDecorations(decorations, resolveAttachmentUrl),
    ...(theaterPresentation !== undefined && parsedTheaterPresentation.success
      ? { theaterPresentation: parsedTheaterPresentation.data }
      : {}),
    extensions: {},
  }
}

const clonePublicRecord = (value: Record<string, unknown>): Record<string, unknown> => {
  try {
    return JSON.parse(JSON.stringify(value)) as Record<string, unknown>
  } catch {
    return {}
  }
}

const buildVariantSnapshot = (
  variant: VariantLike,
  resolveAttachmentUrl: (token?: string) => string,
): BridgeCharacterVariant => {
  const avatar = buildImageRef(variant.avatarAttachmentId, resolveAttachmentUrl, variant.displayName)
  const rawTheaterPresentation = variant.theaterPresentation !== undefined
    ? variant.theaterPresentation
    : variant.appearance?.theaterPresentation
  const parsedTheaterPresentation = rawTheaterPresentation === null
    ? { success: true as const, data: null }
    : theaterPresentationPatchSchema.safeParse(rawTheaterPresentation)
  return {
    variantId: String(variant.id || ''),
    keyword: String(variant.keyword || ''),
    selectorEmoji: String(variant.selectorEmoji || ''),
    note: String(variant.note || ''),
    enabled: variant.enabled !== false,
    appearancePatch: {
      ...(variant.displayName ? { displayName: variant.displayName } : {}),
      ...(variant.color ? { color: variant.color } : {}),
      ...(avatar ? { avatar } : {}),
      ...(rawTheaterPresentation !== undefined && parsedTheaterPresentation.success
        ? { theaterPresentation: structuredClone(parsedTheaterPresentation.data) }
        : {}),
    },
    extensions: {
      ...(variant.appearance ? { appearance: clonePublicRecord(variant.appearance) } : {}),
      ...(variant.updatedAt ? { updatedAt: variant.updatedAt } : {}),
    },
  }
}

export const buildRoleSnapshot = ({
  identity,
  variant,
  variants = [],
  resolvedAppearance,
  isActive = false,
  revision = 0,
  updatedAt = Date.now(),
  resolveAttachmentUrl,
}: {
  identity: IdentityLike
  variant?: VariantLike | null
  variants?: VariantLike[]
  resolvedAppearance?: ResolvedAppearanceLike | null
  isActive?: boolean
  revision?: number
  updatedAt?: number
  resolveAttachmentUrl: (token?: string) => string
}): BridgeRoleSnapshot => {
  const identityDecorations = identity.avatarDecorations
    || (identity.avatarDecoration ? [identity.avatarDecoration] : [])
  const resolvedDisplayName = resolvedAppearance?.displayName || variant?.displayName || identity.displayName || ''
  const resolvedColor = resolvedAppearance?.color || variant?.color || identity.color || ''
  const resolvedAvatarAttachmentId = resolvedAppearance?.avatarAttachmentId
    || variant?.avatarAttachmentId
    || identity.avatarAttachmentId
    || identity.avatarAttachment
  const resolvedDecorations = resolvedAppearance?.avatarDecorations || identityDecorations
  const resolvedTheaterPresentation = resolvedAppearance && 'theaterPresentation' in resolvedAppearance
    ? resolvedAppearance.theaterPresentation
    : identity.theaterPresentation
  const baseAppearance = buildAppearance({
    displayName: identity.displayName,
    color: identity.color,
    avatarAttachmentId: identity.avatarAttachmentId || identity.avatarAttachment,
    decorations: identityDecorations,
    theaterPresentation: identity.theaterPresentation,
    resolveAttachmentUrl,
  })
  const finalAppearance = buildAppearance({
    displayName: resolvedDisplayName,
    color: resolvedColor,
    avatarAttachmentId: resolvedAvatarAttachmentId,
    avatarFallbackAttachmentIds: [
      variant?.avatarAttachmentId,
      identity.avatarAttachmentId,
      identity.avatarAttachment,
    ],
    decorations: resolvedDecorations,
    theaterPresentation: resolvedTheaterPresentation,
    resolveAttachmentUrl,
  })
  return {
    identityId: String(identity.id || ''),
    displayName: resolvedDisplayName,
    color: resolvedColor,
    avatarUrl: finalAppearance.avatar?.url || '',
    isTemporary: Boolean(identity.isTemporary),
    icOocOnActivate: identity.icOocOnActivate || '',
    activeVariantId: variant?.id || null,
    activeVariantDisplayName: variant?.displayName || '',
    activeVariantColor: variant?.color || '',
    activeVariantAvatarUrl: buildImageRef(variant?.avatarAttachmentId, resolveAttachmentUrl)?.url || '',
    isActive,
    revision,
    updatedAt,
    baseAppearance,
    variants: variants.map((item) => buildVariantSnapshot(item, resolveAttachmentUrl)),
    resolvedAppearance: finalAppearance,
    extensions: {},
  }
}

export const buildBridgeMessagePayload = ({
  event,
  worldId,
  channelId,
  message,
  liveIdentity,
  liveVariant,
  resolveAttachmentUrl,
}: {
  event: SealChatBridgeMessageEvent
  worldId: string
  channelId: string
  message: BridgeMessageLike
  liveIdentity?: IdentityLike | null
  liveVariant?: VariantLike | null
  resolveAttachmentUrl: (token?: string) => string
}): SealChatBridgeMessagePayload => {
  const rawContent = normalizeMessageContent(message.contentRichText ?? message.content)
  const identity = message.identity || null
  const displayIdentity = liveIdentity || identity
  const normalizedMode = String(message.icMode ?? message.ic_mode ?? 'ic').toLowerCase() === 'ooc' ? 'ooc' : 'ic'

  return {
    type: 'sealchat.bridge.message',
    event,
    worldId,
    channelId,
    messageId: String(message.id || ''),
    createdAt: typeof message.createdAt === 'number' ? message.createdAt : undefined,
    icMode: normalizedMode,
    isWhisper: Boolean(message.isWhisper ?? message.is_whisper),
    identityId: displayIdentity?.id || message.senderRoleId || message.sender_role_id || null,
    displayName: liveVariant?.displayName || displayIdentity?.displayName || message.sender_identity_name || '',
    color: liveVariant?.color || displayIdentity?.color || message.sender_identity_color || '',
    avatarUrl: resolveAvatarUrl(
      resolveAttachmentUrl,
      liveVariant?.avatarAttachmentId,
      displayIdentity?.avatarAttachment,
      displayIdentity?.avatarAttachmentId,
      message.sender_identity_avatar_id,
    ),
    contentRaw: rawContent,
    contentText: normalizeBridgePlainText(rawContent),
  }
}

const asRecord = (value: unknown): Record<string, unknown> => (
  value && typeof value === 'object' ? value as Record<string, unknown> : {}
)

const normalizeOptionalId = (value: unknown): string | null => {
  const normalized = typeof value === 'string' ? value.trim() : ''
  return normalized || null
}

const normalizeTimestamp = (value: unknown): number => {
  if (typeof value === 'number' && Number.isFinite(value) && value >= 0) return Math.floor(value)
  if (typeof value === 'string' && value.trim()) {
    const numeric = Number(value)
    if (Number.isFinite(numeric) && numeric >= 0) return Math.floor(numeric)
    const parsed = Date.parse(value)
    if (Number.isFinite(parsed) && parsed >= 0) return parsed
  }
  if (value instanceof Date && Number.isFinite(value.getTime())) return value.getTime()
  return 0
}

const normalizeFrozenTheaterPresentation = (value: unknown): TheaterPresentation | null => {
  const parsed = theaterPresentationSchema.safeParse(value)
  return parsed.success ? structuredClone(parsed.data) : null
}

const normalizeFrozenIdentity = (
  message: Record<string, unknown>,
  resolveAttachmentUrl: (token?: string) => string,
) => {
  const identity = asRecord(message.identity)
  const displayName = String(identity.displayName || message.sender_identity_name || '')
  const color = String(identity.color || message.sender_identity_color || '')
  const user = asRecord(message.user)
  const member = asRecord(message.member)
  const userId = normalizeOptionalId(
    user.id
    || member.userId
    || member.user_id
    || message.userId
    || message.user_id
    || message.senderUserId
    || message.sender_user_id,
  )
  const identityId = normalizeOptionalId(identity.id || message.senderRoleId || message.sender_role_id)
  const variantId = normalizeOptionalId(identity.variantId || message.sender_identity_variant_id)
  const avatarAttachmentId = String(identity.avatarAttachment || message.sender_identity_avatar_id || '').trim()
  const decorations = Array.isArray(identity.avatarDecorations)
    ? identity.avatarDecorations as AvatarDecorationLike[]
    : identity.avatarDecoration && typeof identity.avatarDecoration === 'object'
      ? [identity.avatarDecoration as AvatarDecorationLike]
      : []
  const theaterPresentation = normalizeFrozenTheaterPresentation(
    identity.theaterPresentation ?? message.senderTheaterPresentation ?? message.sender_theater_presentation,
  )
  return {
    userId,
    identityId,
    variantId,
    displayName,
    color,
    appearance: {
      ...buildAppearance({
        displayName,
        color,
        avatarAttachmentId,
        decorations,
        theaterPresentation,
        resolveAttachmentUrl,
      }),
      theaterPresentation,
    },
  }
}

export const serializeTheaterDialogueMessage = (
  input: unknown,
  resolveAttachmentUrl: (token?: string) => string = () => '',
): TheaterDialogueMessagePayload | null => {
  const message = asRecord(input)
  const messageId = normalizeOptionalId(message.id || message.messageId)
  if (!messageId) return null
  const rawContent = normalizeMessageContent(message.contentRichText ?? message.content)
  const richContent = isTipTapJson(rawContent) ? rawContent : ''
  const actor = normalizeFrozenIdentity(message, resolveAttachmentUrl)
  const displayOrder = typeof message.displayOrder === 'number' && Number.isFinite(message.displayOrder)
    ? message.displayOrder
    : typeof message.display_order === 'number' && Number.isFinite(message.display_order)
      ? message.display_order
      : undefined
  return {
    messageId,
    createdAt: normalizeTimestamp(message.createdAt ?? message.timestamp ?? message.created_at),
    ...(displayOrder !== undefined ? { displayOrder } : {}),
    icMode: String(message.icMode ?? message.ic_mode ?? 'ic').toLowerCase() === 'ooc' ? 'ooc' : 'ic',
    isWhisper: Boolean(message.isWhisper || message.is_whisper),
    isArchived: Boolean(message.isArchived || message.is_archived),
    isDeleted: Boolean(message.isDeleted || message.is_deleted || message.isRevoked || message.is_revoked),
    contentText: normalizeBridgePlainText(rawContent),
    ...(richContent
      ? {
          contentRichText: richContent,
          hasPerformanceContent: hasPerformanceContent(richContent),
        }
      : {}),
    actor,
  }
}

export const serializeTheaterDialogueRemoved = (input: unknown): TheaterDialogueMessageRemovedPayload | null => {
  const message = asRecord(input)
  const messageId = normalizeOptionalId(message.id || message.messageId)
  return messageId ? { messageId } : null
}

export const resolveTheaterChatEventChannelId = (input: unknown): string => {
  const event = asRecord(input)
  const message = asRecord(event.message)
  const channel = asRecord(event.channel)
  const messageChannel = asRecord(message.channel)
  return String(
    channel.id
    || messageChannel.id
    || message.channelId
    || message.channel_id
    || '',
  ).trim()
}
