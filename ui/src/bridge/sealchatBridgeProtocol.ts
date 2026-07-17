import type { TheaterPresentation, TheaterPresentationPatch } from '../types/theaterPresentation'

export interface SealChatBridgeHandshakeRequest {
  type: 'sealchat.bridge.handshake'
  version: 1
  nonce: string
  want: Array<'roles' | 'messages'>
  currentChannelOnly: true
}

export interface SealChatBridgeUnsubscribeRequest {
  type: 'sealchat.bridge.unsubscribe'
  nonce?: string
}

export type SealChatBridgeRequest =
  | SealChatBridgeHandshakeRequest
  | SealChatBridgeUnsubscribeRequest

export interface SealChatBridgeHandshakeAck {
  type: 'sealchat.bridge.handshake.ack'
  version: 1
  nonce: string
  ok: true
  worldId: string
  channelId: string
}

export interface BridgeRoleSnapshot {
  identityId: string
  displayName: string
  color: string
  avatarUrl: string
  isTemporary: boolean
  icOocOnActivate?: '' | 'ic' | 'ooc'
  activeVariantId: string | null
  activeVariantDisplayName?: string
  activeVariantColor?: string
  activeVariantAvatarUrl?: string
  isActive: boolean
  revision: number
  updatedAt: number
  baseAppearance: BridgeCharacterAppearance
  variants: BridgeCharacterVariant[]
  resolvedAppearance: BridgeCharacterAppearance
  extensions: Record<string, unknown>
}

export interface BridgeImageRef {
  resourceId: string
  url: string
  alt?: string
}

export interface BridgeCharacterDecoration {
  id: string
  resource: BridgeImageRef
  enabled: boolean
  zIndex: number
  settings: Record<string, unknown>
  extensions: Record<string, unknown>
}

export interface BridgeCharacterAppearance {
  displayName: string
  color: string
  avatar: BridgeImageRef | null
  decorations: BridgeCharacterDecoration[]
  theaterPresentation?: TheaterPresentation | null
  extensions: Record<string, unknown>
}

export interface BridgeCharacterVariant {
  variantId: string
  keyword: string
  selectorEmoji: string
  note: string
  enabled: boolean
  appearancePatch: Omit<Partial<BridgeCharacterAppearance>, 'theaterPresentation'> & {
    theaterPresentation?: TheaterPresentationPatch | null
  }
  extensions: Record<string, unknown>
}

export interface SealChatBridgeRolesSnapshot {
  type: 'sealchat.bridge.roles.snapshot'
  worldId: string
  channelId: string
  generatedAt: number
  roles: BridgeRoleSnapshot[]
}

export type SealChatBridgeMessageEvent =
  | 'message-created'
  | 'message-updated'
  | 'message-deleted'

export interface SealChatBridgeMessagePayload {
  type: 'sealchat.bridge.message'
  event: SealChatBridgeMessageEvent
  worldId: string
  channelId: string
  messageId: string
  createdAt?: number
  icMode: 'ic' | 'ooc'
  isWhisper: boolean
  identityId: string | null
  displayName: string
  color: string
  avatarUrl: string
  contentRaw: string
  contentText: string
}

export type SealChatBridgeResponse =
  | SealChatBridgeHandshakeAck
  | SealChatBridgeRolesSnapshot
  | SealChatBridgeMessagePayload
