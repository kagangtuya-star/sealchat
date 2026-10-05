import { api } from '@/stores/_config'

export type MCPMode = 'off' | 'read' | 'write'
export interface MCPConfig {
  enabled: boolean
  chat: MCPMode
  search: MCPMode
  battleReport: MCPMode
  clue: MCPMode
  glossary: MCPMode
  identity: MCPMode
  audio: MCPMode
  note: MCPMode
  file: MCPMode
  battleReportGenerate: boolean
  cluePublish: boolean
  callsPerMinute: number
  writesPerMinute: number
}
export interface PersonalAPIKey {
  id: string
  name: string
  publicId: string
  tail: string
  scopes: string[]
  createdAt: string
  updatedAt: string
  lastUsedAt: string | null
  expiresAt: string | null
  revokedAt: string | null
}
export interface PersonalAPIKeyCatalog {
  items: PersonalAPIKey[]
  enabled: boolean
  catalog: { id: string; description: string; extra: boolean }[]
  allowedScopes: string[]
  path: string
}
export const personalAPIKeys = {
  list: () => api.get<PersonalAPIKeyCatalog>('api/v1/user/api-keys'),
  create: (input: { name: string; scopes: string[]; neverExpires: boolean }) =>
    api.post<{ key: PersonalAPIKey; token: string }>('api/v1/user/api-keys', input),
  update: (id: string, input: { name: string; scopes: string[] }) =>
    api.patch(`api/v1/user/api-keys/${encodeURIComponent(id)}`, input),
  revoke: (id: string) => api.delete(`api/v1/user/api-keys/${encodeURIComponent(id)}`),
  rotate: (id: string) => api.post<{ key: PersonalAPIKey; token: string }>(`api/v1/user/api-keys/${encodeURIComponent(id)}/rotate`),
}
