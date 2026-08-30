export const CHANNEL_EMBED_VERSION = 1 as const
export const CHANNEL_EMBED_HANDSHAKE = 'sealchat.embed.handshake' as const
export const CHANNEL_EMBED_HANDSHAKE_ACK = 'sealchat.embed.handshake.ack' as const
export const CHANNEL_EMBED_REQUEST = 'sealchat.embed.request' as const
export const CHANNEL_EMBED_RESPONSE = 'sealchat.embed.response' as const
export const CHANNEL_EMBED_EVENT = 'sealchat.embed.event' as const

export type EmbedErrorCode =
  | 'ORIGIN_DENIED' | 'HANDSHAKE_FAILED' | 'SESSION_EXPIRED' | 'CONTEXT_CHANGED'
  | 'CAPABILITY_DENIED' | 'PERMISSION_DENIED' | 'INVALID_PARAMS' | 'NOT_FOUND'
  | 'REVISION_CONFLICT' | 'QUOTA_EXCEEDED' | 'PAYLOAD_TOO_LARGE' | 'RATE_LIMITED'
  | 'WS_OFFLINE' | 'TIMEOUT' | 'INTERNAL_ERROR'

export interface EmbedRequest {
  type: typeof CHANNEL_EMBED_REQUEST
  version: 1
  sessionId: string
  requestId: string
  method: string
  contextVersion: number
  params?: unknown
}

export interface EmbedResponse {
  type: typeof CHANNEL_EMBED_RESPONSE
  version: 1
  sessionId: string
  requestId: string
  ok: boolean
  result?: unknown
  error?: { code: EmbedErrorCode | string; message: string; details?: unknown }
}

export interface EmbedEvent {
  type: typeof CHANNEL_EMBED_EVENT
  version: 1
  sessionId: string
  eventId: string
  topic: string
  seq?: number
  contextVersion?: number
  payload: unknown
  at: number
}

export interface EmbedHandshakeRequest {
  type: typeof CHANNEL_EMBED_HANDSHAKE
  version: 1
  nonce: string
}

export interface EmbedHandshakeAck {
  type: typeof CHANNEL_EMBED_HANDSHAKE_ACK
  version: 1
  nonce: string
  ok: boolean
  sessionId?: string
  contextVersion?: number
  capabilities?: string[]
  error?: { code: EmbedErrorCode | string; message: string }
}

export const isEmbedHandshakeRequest = (value: unknown): value is EmbedHandshakeRequest => {
  if (!value || typeof value !== 'object') return false
  const data = value as Record<string, unknown>
  return data.type === CHANNEL_EMBED_HANDSHAKE && data.version === 1
    && typeof data.nonce === 'string' && data.nonce.length >= 16 && data.nonce.length <= 256
}

export const isEmbedRequest = (value: unknown): value is EmbedRequest => {
  if (!value || typeof value !== 'object') return false
  const data = value as Record<string, unknown>
  return data.type === CHANNEL_EMBED_REQUEST && data.version === 1
    && typeof data.sessionId === 'string' && data.sessionId.length >= 1 && data.sessionId.length <= 256
    && typeof data.requestId === 'string' && data.requestId.length >= 1 && data.requestId.length <= 128
    && typeof data.method === 'string' && data.method.length > 0 && data.method.length <= 80
    && Number.isInteger(data.contextVersion) && (data.contextVersion as number) >= 0
}

export const randomEmbedId = (prefix: string) => {
  const cryptoObj = typeof globalThis !== 'undefined' ? globalThis.crypto : undefined
  if (cryptoObj?.randomUUID) return `${prefix}_${cryptoObj.randomUUID()}`
  return `${prefix}_${Date.now().toString(36)}_${Math.random().toString(36).slice(2)}`
}
