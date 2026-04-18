export const MANAGER_UNCATEGORIZED_KEY = '__uncategorized__'

export interface WorldKeywordManagerWorldSourceItem {
  id: string
  worldId: string
  keyword: string
  category: string
  aliases: string[]
  matchMode: 'plain' | 'regex'
  description: string
  descriptionFormat?: 'plain' | 'rich'
  display: 'standard' | 'minimal' | 'inherit'
  sortOrder: number
  isEnabled: boolean
  createdAt: string
  updatedAt: string
  createdBy?: string
  updatedBy?: string
  matchedVia?: string
}

export interface WorldKeywordManagerEffectiveSourceItem extends WorldKeywordManagerWorldSourceItem {
  sourceType: 'world' | 'external_library'
  sourceId: string
  sourceName: string
  canQuickEdit?: boolean
}

export interface KeywordManagerListItem {
  id: string
  keyword: string
  category: string
  aliases: string[]
  matchMode: 'plain' | 'regex'
  description: string
  descriptionFormat?: 'plain' | 'rich'
  display: 'standard' | 'minimal' | 'inherit'
  sortOrder: number
  isEnabled: boolean
  createdAt: string
  updatedAt: string
  createdBy?: string
  updatedBy?: string
  matchedVia?: string
  sourceType: 'world' | 'external_library'
  sourceId: string
  sourceName: string
  isReadonly: boolean
}

export function filterReadonlyExternalItems(items: WorldKeywordManagerEffectiveSourceItem[]): WorldKeywordManagerEffectiveSourceItem[] {
  return items.filter((item) => item.sourceType === 'external_library')
}

export function toKeywordManagerListItem(
  item: WorldKeywordManagerWorldSourceItem | WorldKeywordManagerEffectiveSourceItem,
): KeywordManagerListItem {
  if ((item as WorldKeywordManagerEffectiveSourceItem).sourceType === 'external_library') {
    const effectiveItem = item as WorldKeywordManagerEffectiveSourceItem
    return {
      ...effectiveItem,
      sourceType: effectiveItem.sourceType,
      sourceId: effectiveItem.sourceId,
      sourceName: effectiveItem.sourceName,
      isReadonly: true,
    }
  }
  const worldItem = item as WorldKeywordManagerWorldSourceItem
  return {
    ...worldItem,
    sourceType: 'world',
    sourceId: worldItem.worldId,
    sourceName: '当前世界',
    isReadonly: false,
  }
}

export function buildKeywordManagerItems(
  worldItems: WorldKeywordManagerWorldSourceItem[],
  externalItems: WorldKeywordManagerEffectiveSourceItem[],
): KeywordManagerListItem[] {
  return [
    ...worldItems.map((item) => toKeywordManagerListItem(item)),
    ...filterReadonlyExternalItems(externalItems).map((item) => toKeywordManagerListItem(item)),
  ]
}

export function buildDisplayCategoryKey(item: Pick<KeywordManagerListItem, 'sourceType' | 'sourceId' | 'category'>): string {
  const category = String(item.category || '').trim() || MANAGER_UNCATEGORIZED_KEY
  if (item.sourceType === 'external_library') {
    return `ext:${item.sourceId}:${category}`
  }
  return `world:${category}`
}

export function buildDisplayCategoryLabel(
  item: Pick<KeywordManagerListItem, 'sourceType' | 'sourceName' | 'category'>,
): string {
  const category = String(item.category || '').trim() || '(未分类)'
  if (item.sourceType === 'external_library') {
    return `${item.sourceName}-${category}`
  }
  return category
}
