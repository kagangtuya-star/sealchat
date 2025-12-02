export interface WorldKeyword {
  id: string
  worldId: string
  keyword: string
  description: string
  createdBy: string
  updatedBy: string
  createdByName: string
  updatedByName: string
  createdAt: string
  updatedAt: string
}

export interface WorldKeywordEventPayload {
  worldId: string
  updatedAt: number
  keywords: Array<{
    id: string
    keyword: string
    description: string
    updatedAt: number
  }>
}
