import type { StageObject } from '../shared/stage-types'

export interface StageLayerRow {
  object: StageObject
  depth: number
}

export const buildStageLayerRows = (
  objects: StageObject[],
  persistedCollapsedIds: ReadonlySet<string>,
  temporaryExpandedIds: ReadonlySet<string>,
): StageLayerRow[] => {
  const rows: StageLayerRow[] = []
  const visited = new Set<string>()
  const append = (parentId: string | null, depth: number) => {
    objects
      .filter((object) => object.parentId === parentId)
      .sort((left, right) => right.transform.z - left.transform.z || right.transform.order - left.transform.order)
      .forEach((object) => {
        if (visited.has(object.id)) return
        visited.add(object.id)
        rows.push({ object, depth })
        const collapsed = object.type === 'group'
          && persistedCollapsedIds.has(object.id)
          && !temporaryExpandedIds.has(object.id)
        if (!collapsed) append(object.id, depth + 1)
      })
  }
  append(null, 0)
  return rows
}

export const stageLayerSelectionExpansionIds = (
  objects: Record<string, StageObject>,
  selectedObjectId: string | null,
) => {
  const ids = new Set<string>()
  const visited = new Set<string>()
  let object = selectedObjectId ? objects[selectedObjectId] : undefined
  if (object?.type === 'group') ids.add(object.id)
  while (object?.parentId && !visited.has(object.parentId)) {
    visited.add(object.parentId)
    const parent = objects[object.parentId]
    if (!parent) break
    if (parent.type === 'group') ids.add(parent.id)
    object = parent
  }
  return ids
}
