import { toRaw } from 'vue'
import type { StageObject } from '../shared/stage-types'

export interface StageClipboardBundle {
  version: 1
  sourceSceneId: string
  persistent: boolean
  rootId: string
  objects: StageObject[]
}

export interface StageObjectCollectionsSnapshot {
  sceneId: string
  sceneObjects: Record<string, StageObject>
  persistentObjects: Record<string, StageObject>
}

export interface StageObjectPatch {
  target: 'scene' | 'persistent'
  path: string[]
  beforeExists: boolean
  before?: unknown
  afterExists: boolean
  after?: unknown
}

export interface StageSelectionSnapshot {
  selectedIds: string[]
  primaryId: string | null
}

export interface StageObjectHistoryEntry {
  label: string
  sceneId: string
  patches: StageObjectPatch[]
  selectionBefore: StageSelectionSnapshot
  selectionAfter: StageSelectionSnapshot
}

const clone = <T>(value: T): T => structuredClone(toRaw(value))
const own = (value: object, key: string) => Object.prototype.hasOwnProperty.call(value, key)
const same = (left: unknown, right: unknown) => JSON.stringify(left) === JSON.stringify(right)

const diffRecord = (
  target: StageObjectPatch['target'],
  before: Record<string, unknown>,
  after: Record<string, unknown>,
  path: string[],
  result: StageObjectPatch[],
) => {
  const keys = new Set([...Object.keys(before), ...Object.keys(after)])
  keys.forEach((key) => {
    const beforeExists = own(before, key)
    const afterExists = own(after, key)
    const beforeValue = before[key]
    const afterValue = after[key]
    const nextPath = [...path, key]
    if (!beforeExists || !afterExists) {
      result.push({
        target,
        path: nextPath,
        beforeExists,
        ...(beforeExists ? { before: clone(beforeValue) } : {}),
        afterExists,
        ...(afterExists ? { after: clone(afterValue) } : {}),
      })
      return
    }
    if (same(beforeValue, afterValue)) return
    if (
      beforeValue && afterValue
      && typeof beforeValue === 'object' && typeof afterValue === 'object'
      && !Array.isArray(beforeValue) && !Array.isArray(afterValue)
    ) {
      diffRecord(
        target,
        beforeValue as Record<string, unknown>,
        afterValue as Record<string, unknown>,
        nextPath,
        result,
      )
      return
    }
    result.push({
      target,
      path: nextPath,
      beforeExists: true,
      before: clone(beforeValue),
      afterExists: true,
      after: clone(afterValue),
    })
  })
}

export const createObjectHistoryEntry = (
  label: string,
  before: StageObjectCollectionsSnapshot,
  after: StageObjectCollectionsSnapshot,
  selectionBefore: StageSelectionSnapshot,
  selectionAfter: StageSelectionSnapshot,
): StageObjectHistoryEntry | null => {
  if (before.sceneId !== after.sceneId) return null
  const patches: StageObjectPatch[] = []
  diffRecord('scene', before.sceneObjects, after.sceneObjects, [], patches)
  diffRecord('persistent', before.persistentObjects, after.persistentObjects, [], patches)
  return patches.length ? {
    label,
    sceneId: before.sceneId,
    patches,
    selectionBefore,
    selectionAfter,
  } : null
}

const resolveParent = (root: Record<string, unknown>, path: string[], create: boolean) => {
  let current = root
  for (const key of path.slice(0, -1)) {
    const next = current[key]
    if (!next || typeof next !== 'object' || Array.isArray(next)) {
      if (!create) return null
      current[key] = {}
    }
    current = current[key] as Record<string, unknown>
  }
  return current
}

export const applyObjectHistoryEntry = (
  entry: StageObjectHistoryEntry,
  direction: 'undo' | 'redo',
  sceneObjects: Record<string, StageObject>,
  persistentObjects: Record<string, StageObject>,
) => {
  const patches = direction === 'undo' ? [...entry.patches].reverse() : entry.patches
  patches.forEach((patch) => {
    const root = (patch.target === 'scene' ? sceneObjects : persistentObjects) as Record<string, unknown>
    const exists = direction === 'undo' ? patch.beforeExists : patch.afterExists
    const value = direction === 'undo' ? patch.before : patch.after
    const parent = resolveParent(root, patch.path, exists)
    if (!parent) return
    const key = patch.path[patch.path.length - 1]
    if (exists) parent[key] = clone(value)
    else delete parent[key]
  })
}

export const collectObjectSubtree = (
  collection: Record<string, StageObject>,
  rootId: string,
): StageObject[] => {
  const result: StageObject[] = []
  const visit = (id: string) => {
    const object = collection[id]
    if (!object) return
    result.push(clone(object))
    Object.values(collection)
      .filter((candidate) => candidate.parentId === id)
      .forEach((candidate) => visit(candidate.id))
  }
  visit(rootId)
  return result
}

export const instantiateClipboardBundle = (
  bundle: StageClipboardBundle,
  makeId: (prefix: string) => string,
  offset: number,
  rootParentId: string | null,
) => {
  const idMap = new Map(bundle.objects.map((object) => [object.id, makeId('object')]))
  const objects = bundle.objects.map((source) => {
    const object = clone(source)
    object.id = idMap.get(source.id)!
    object.parentId = source.id === bundle.rootId
      ? rootParentId
      : source.parentId ? idMap.get(source.parentId) || null : null
    object.actions = object.actions.map((action) => {
      const copiedAction = clone(action)
      copiedAction.id = makeId('action')
      if (copiedAction.type === 'object.toggle' && idMap.has(copiedAction.payload.objectId)) {
        copiedAction.payload.objectId = idMap.get(copiedAction.payload.objectId)!
      }
      return copiedAction
    })
    if (source.id === bundle.rootId) {
      object.name = `${object.name} 副本`
      object.transform.x += offset
      object.transform.y += offset
    }
    return object
  })
  return { rootId: idMap.get(bundle.rootId)!, objects }
}
