import { api } from '@/stores/_config'
import type { AdminAIUsageLogListResult } from '@/types'
import { SpeechRequestKeys } from './runtime'
import type { MessageSpeech, ResolvedSpeechModel, ResolvedSpeechProvider, RoleSpeechConfig, SpeechConfig, SpeechJob, SpeechPolicy, SpeechProviderResolveRequest, SpeechQuota, SpeechRequest, SpeechVoice, UnknownSpeechUsage, VoiceCreationProvider, VoiceDirectory, VoiceDirectoryQuery } from './types'

const root = 'api/v1/tts'
const id = encodeURIComponent
const submissionKeys = new SpeechRequestKeys()
export function clearSpeechSubmissionKeys() { submissionKeys.clear() }
export const speechAPI = {
  async queue(channelId: string) { return (await api.get<{ items: { id: string; messageId: string; status: string }[]; canControl: boolean }>(`${root}/channels/${id(channelId)}/queue`)).data },
  async control(channelId: string, action: 'skip' | 'stop' | 'clear') { await api.post(`${root}/channels/${id(channelId)}/control`, { action }) },
  async logs(params: Record<string, string | number | undefined>) { return api.get<AdminAIUsageLogListResult>('api/v1/admin/ai/usage-logs', { params: { ...params, quotaKind: 'speech' } }) },
  async cleanupLogs(retentionDays?: number) { return api.post<{ affectedRows: number }>('api/v1/admin/ai/usage-logs/cleanup', { retentionDays, quotaKind: 'speech' }) },
  async unknown() { return (await api.get<UnknownSpeechUsage[]>(`${root}/admin/unknown`)).data },
  async resolveUnknown(jobId: string, action: 'settle' | 'release', note: string, units: number, inputTokens?: number | null, outputTokens?: number | null) { await api.post(`${root}/admin/unknown/${id(jobId)}`, { action, note, units, inputTokens, outputTokens }) },
  async policy(userId: string) { return (await api.get<{ quota: SpeechQuota; policy: SpeechPolicy }>(`${root}/admin/users/${id(userId)}`)).data },
  async savePolicy(userId: string, policy: SpeechPolicy) { await api.put(`${root}/admin/users/${id(userId)}`, policy) },
  async wsTicket(channelId: string) { return (await api.post<{ ticket: string; path: string }>(`${root}/channels/${id(channelId)}/ws-ticket`)).data },
  async states(channelId: string) { return (await api.get<{ id: string; tts: MessageSpeech | null }[]>(`${root}/channels/${id(channelId)}/states`)).data },
  async adminConfig() { return (await api.get<{ config: SpeechConfig | null }>(`${root}/admin/config`)).data.config },
  async models() { return (await api.get<ResolvedSpeechModel[]>(`${root}/admin/models`)).data },
  async saveAdminConfig(config: SpeechConfig) { return (await api.patch<{ config: SpeechConfig }>(`${root}/admin/config`, config)).data.config },
  async resolveProvider(request: SpeechProviderResolveRequest) {
    return (await api.post<ResolvedSpeechProvider>(`${root}/admin/provider/resolve`, request)).data
  },
  async me() { return (await api.get<SpeechQuota>(`${root}/me`)).data },
  async voiceTargets() { return (await api.get<VoiceCreationProvider[]>(`${root}/voice-targets`)).data },
  async settings(autoSynthesis: boolean) { await api.patch(`${root}/settings`, { autoSynthesis }) },
  async source(file: File) {
    const form = new FormData()
    form.append('file', file)
    form.append('authorized', 'true')
    return (await api.post<{ id: string }>(`${root}/sources`, form)).data.id
  },
  async voices(params: VoiceDirectoryQuery) {
    return (await api.get<VoiceDirectory>(`${root}/voices`, { params })).data
  },
  async voice(voiceId: string) { return (await api.get<SpeechVoice>(`${root}/voices/${id(voiceId)}`)).data },
  async submit(operation: 'audition' | 'design' | 'clone', request: SpeechRequest) {
    const requestKey = submissionKeys.resolve(operation, { ...request })
    return (await api.post<SpeechJob>(`${root}/jobs/${operation}`, { ...request, requestKey })).data
  },
  async job(jobId: string) { return (await api.get<SpeechJob>(`${root}/jobs/${id(jobId)}`)).data },
  async save(voiceId: string, replaceId = '') { await api.post(`${root}/voices/${id(voiceId)}/save`, { replaceId }) },
  async update(voice: SpeechVoice) { await api.patch(`${root}/voices/${id(voice.id)}`, voice) },
  async remove(voiceId: string) { await api.delete(`${root}/voices/${id(voiceId)}`) },
  async role(identityId: string) { return (await api.get<RoleSpeechConfig>(`${root}/roles/${id(identityId)}`)).data },
  async saveRole(identityId: string, value: RoleSpeechConfig) { await api.put(`${root}/roles/${id(identityId)}`, value) },
  async message(messageId: string) { return (await api.get<{ tts: MessageSpeech | null }>(`${root}/messages/${id(messageId)}`)).data.tts },
  async ticket(kind: 'messages' | 'resources', resourceId: string) {
    return (await api.post<{ url: string }>(`${root}/${kind}/${id(resourceId)}/ticket`)).data.url
  },
  async audio(url: string, signal: AbortSignal) {
    return (await api.get<Blob>(url, { responseType: 'blob', signal })).data
  },
}

export function speechError(error: unknown): string {
  if (typeof error === 'object' && error !== null && 'response' in error) {
    const response = error.response as { data?: unknown; status?: unknown }
    if (typeof response.data === 'object' && response.data !== null && 'message' in response.data) {
      const message = (response.data as { message?: unknown }).message
      if (typeof message === 'string') return message
    }
    if (typeof response.data === 'string' && response.data.trim()) return response.data
    if (typeof response.status === 'number') return `请求失败（HTTP ${response.status}）`
  }
  return error instanceof Error ? error.message : '语音操作失败，请重试查询状态。'
}
