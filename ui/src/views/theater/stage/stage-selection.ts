import type { StageObject, StageObjectScope } from '../shared/stage-types'

export interface StageSelectionGroup {
  memberIds: string[]
  rootIds: string[]
  primaryId: string | null
  members: StageObject[]
  roots: StageObject[]
  scopes: StageObjectScope[]
  lockedIds: string[]
}

export const stageSelectionRootIds = (
  objects: Record<string, StageObject>,
  objectIds: string[],
) => {
  const candidates = new Set(objectIds.filter((id) => Boolean(objects[id])))
  return [...candidates].filter((id) => {
    const visited = new Set<string>()
    let parentId = objects[id]?.parentId || null
    while (parentId && !visited.has(parentId)) {
      if (candidates.has(parentId)) return false
      visited.add(parentId)
      parentId = objects[parentId]?.parentId || null
    }
    return true
  })
}

export const createStageSelectionGroup = (
  objects: Record<string, StageObject>,
  selectedIds: string[],
  primaryId: string | null,
  scopeForObject: (objectId: string) => StageObjectScope,
): StageSelectionGroup => {
  const memberIds = [...new Set(selectedIds)].filter((id) => Boolean(objects[id]))
  const rootIds = stageSelectionRootIds(objects, memberIds)
  const members = memberIds.map((id) => objects[id])
  const roots = rootIds.map((id) => objects[id])
  const validPrimaryId = primaryId && memberIds.includes(primaryId)
    ? primaryId
    : memberIds[memberIds.length - 1] || null

  return {
    memberIds,
    rootIds,
    primaryId: validPrimaryId,
    members,
    roots,
    scopes: [...new Set(memberIds.map(scopeForObject))],
    lockedIds: members.filter((object) => object.locked).map((object) => object.id),
  }
}
