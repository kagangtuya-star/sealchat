export interface MessageSpeech {
  status: string
  audioResourceId?: string
  durationMs?: number
  format?: string
  messageRevision: number
}
export interface VoiceContext {
  providerKind: string
  providerId: string
  modelId: string
}
export interface SpeechQuota {
  enabled: boolean
  autoSynthesis: boolean
  policy: { dailyLimit: number | null; monthlyLimit: number | null; lifetimeLimit: number | null }
  usage: { DailySettled: number; MonthlySettled: number; LifetimeSettled: number; ActiveReserved: number }
  saved: number
  slots: number
  format: string
  characterPrice: number | null
  pricingMode?: 'character' | 'token'
  inputTokenPrice?: number | null
  outputTokenPrice?: number | null
  designPrice: number | null
  clonePrice: number | null
  defaultModel: string
  defaultVoice: string
  voiceContext: VoiceContext
}
export interface SpeechVoice {
  id: string
  ownerUserId: string
  providerId?: string
  name: string
  tags: string
  description: string
  parameters: string
  targetModel: string
  kind: string
  isPublic: boolean
  lifecycle: string
  providerStatus: string
  supported?: boolean
  revision: number
  previewExpiresAt?: string
  previewResourceId?: string
}
export interface SystemVoice { id: string; name: string; providerKind: string; models: string[]; targetModel?: string; languages: string[]; kind: string; tags?: string }
export interface VoiceDirectory { items: SpeechVoice[]; system: SystemVoice[]; total: number; catalogVersion: string }
// `scope` selects the saved-voice picker catalog; without it `mine` keeps the
// workbench listing (previews and creating voices included).
export interface VoiceDirectoryQuery {
  scope?: 'all' | 'mine' | 'public'
  mine?: boolean
  page?: number
  search?: string
  model?: string
  providerId?: string
  kind?: string
  tag?: string
}
export interface SpeechRequest {
  requestKey: string
  text?: string
  voiceId?: string
  systemVoice?: string
  systemVoiceProvider?: string
  systemVoiceModel?: string
  instruction?: string
  rate?: number
  pitch?: number
  volume?: number
  name?: string
  description?: string
  sourceResourceId?: string
}
export interface SpeechJob {
  id: string
  status: string
  operation: string
  model?: string
  voiceId?: string
  audioResourceId?: string
  usageStatus: string
  errorCode?: string
  message?: string
  estimatedUnits: number
  actualUnits?: number | null
  actualCost?: number
  pricingMode?: 'character' | 'token' | 'request'
  inputTokens?: number | null
  outputTokens?: number | null
  media?: { codec: string; container: string; sampleRate: number; channelCount: number; durationMs: number }
}
export interface RoleSpeechConfig {
  identityId: string
  voiceId: string
  systemVoice: string
  systemVoiceProvider: string
  systemVoiceModel: string
  instruction: string
  rate: number
  pitch: number
  volume: number
  revision: number
}
export interface SpeechProvider {
  id: string; providerKind: string; enabled: boolean; credentialScope: string; region: string; workspace: string
  apiKey: string; hasApiKey?: boolean; synthesisEndpoint: string; voiceEndpoint: string; model: string
  characterPrice: number | null; designPrice: number | null; clonePrice: number | null
  pricingMode?: 'character' | 'token'; inputTokenPrice?: number | null; outputTokenPrice?: number | null
  accountVoiceLimit: number | null; revision: number
}
export interface SpeechConfig {
  enabled: boolean; providers: SpeechProvider[]; defaultProvider: string; defaultVoice: string
  format: string; quotaDefault: SpeechQuota['policy']; defaultSlots: number
  previewTTLMinutes: number; previewLimit: number; requestTimeoutSeconds: number
  maxConcurrent: number; channelQueueLimit: number
}
export interface ResolvedSpeechModel {
  id: string
  providerKind: string
  defaultVoice: string
  capabilities: { httpStreaming: boolean; webSocketStreaming: boolean; voiceDesign: boolean; voiceClone: boolean }
  name: string
  pricingMode: 'character' | 'token'
  characterPrice: number | null
  inputTokenPrice: number | null
  outputTokenPrice: number | null
  displayPrice: string
  pricingSource: 'online' | 'modelsdev' | 'builtin' | 'mixed' | 'unknown'
  designPrice: number | null
  clonePrice: number
}
export interface ResolvedSpeechProvider {
  providerId: string
  providerKind: string
  workspace: string
  region: string
  credentialScope: string
  synthesisEndpoint: string
  voiceEndpoint: string
  models: ResolvedSpeechModel[]
}
export interface SpeechPolicy {
  overrideEnabled: boolean; dailyLimit: number | null; monthlyLimit: number | null
  lifetimeLimit: number | null; slots: number | null
}
export interface UnknownSpeechUsage {
  id: string; operation: string; payerUserId: string; providerRequestId: string
  estimatedUnits: number; errorCode: string
  pricingMode?: 'character' | 'token' | 'request'
}
