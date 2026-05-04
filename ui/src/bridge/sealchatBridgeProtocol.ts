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
