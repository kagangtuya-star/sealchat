import type {
  BridgeRoleSnapshot,
  SealChatBridgeMessageEvent,
  SealChatBridgeMessagePayload,
} from './sealchatBridgeProtocol'

const inlineImageTokenPattern = /\[\[(?:图片:[^\]]+|img:[^\]]+)\]\]/gi
const botStateWidgetPrefix = '[[STATE_WIDGET]]'

type TipTapNode = {
  type?: string
  text?: string
  attrs?: Record<string, unknown>
  content?: TipTapNode[]
}

type IdentityLike = {
  id?: string
  displayName?: string
  color?: string
  avatarAttachmentId?: string
  avatarAttachment?: string
  isTemporary?: boolean
  icOocOnActivate?: '' | 'ic' | 'ooc'
}

type VariantLike = {
  id?: string
  displayName?: string
  color?: string
  avatarAttachmentId?: string
}

type BridgeMessageLike = {
  id?: string
  content?: string
  createdAt?: number
  icMode?: string
  ic_mode?: string
  isWhisper?: boolean
  is_whisper?: boolean
  identity?: IdentityLike | null
  senderRoleId?: string
  sender_role_id?: string
  sender_identity_name?: string
  sender_identity_color?: string
  sender_identity_avatar_id?: string
}

const isTipTapJson = (content: string): boolean => {
  if (!content || typeof content !== 'string') {
    return false
  }
  try {
    const parsed = JSON.parse(content)
    return Boolean(parsed && typeof parsed === 'object' && parsed.type === 'doc')
  } catch {
    return false
  }
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
  if (node.type === 'mention') {
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

export const normalizeBridgePlainText = (raw: string): string => {
  const content = String(raw || '')
  if (!content) {
    return ''
  }

  const plainText = isTipTapJson(content) ? tiptapJsonToPlainText(content) : content
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
      return normalizeAbsoluteUrl(resolveAttachmentUrl(token))
    }
  }
  return ''
}

export const buildRoleSnapshot = ({
  identity,
  variant,
  resolveAttachmentUrl,
}: {
  identity: IdentityLike
  variant?: VariantLike | null
  resolveAttachmentUrl: (token?: string) => string
}): BridgeRoleSnapshot => ({
  identityId: String(identity.id || ''),
  displayName: variant?.displayName || identity.displayName || '',
  color: variant?.color || identity.color || '',
  avatarUrl: resolveAvatarUrl(
    resolveAttachmentUrl,
    variant?.avatarAttachmentId,
    identity.avatarAttachmentId,
    identity.avatarAttachment,
  ),
  isTemporary: Boolean(identity.isTemporary),
  icOocOnActivate: identity.icOocOnActivate || '',
  activeVariantId: variant?.id || null,
  activeVariantDisplayName: variant?.displayName || '',
  activeVariantColor: variant?.color || '',
  activeVariantAvatarUrl: resolveAvatarUrl(resolveAttachmentUrl, variant?.avatarAttachmentId),
})

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
  const rawContent = String(message.content || '')
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
