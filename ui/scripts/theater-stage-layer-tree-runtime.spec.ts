import assert from 'node:assert/strict'

import type { StageObject } from '../src/views/theater/shared/stage-types'
import { buildStageLayerRows, stageLayerSelectionExpansionIds } from '../src/views/theater/stage/stage-layer-tree'

const object = (id: string, type: StageObject['type'], parentId: string | null, order: number): StageObject => ({
  id,
  parentId,
  type,
  name: id,
  transform: { x: 0, y: 0, width: 1, height: 1, rotation: 0, scaleX: 1, scaleY: 1, z: order, order },
  visible: true,
  locked: false,
  aspectRatioLocked: true,
  interactive: type !== 'group',
  editable: false,
  fill: '#fff',
  actions: [],
  metadata: {},
})

const group = object('group', 'group', null, 3)
const nested = object('nested', 'group', group.id, 2)
const child = object('child', 'image', nested.id, 1)
const root = object('root', 'text', null, 0)
const objects = [group, nested, child, root]

assert.deepEqual(
  buildStageLayerRows(objects, new Set([group.id]), new Set()).map((row) => row.object.id),
  [group.id, root.id],
)
assert.deepEqual(
  buildStageLayerRows(objects, new Set([group.id, nested.id]), new Set([group.id])).map((row) => row.object.id),
  [group.id, nested.id, root.id],
)
assert.deepEqual(
  buildStageLayerRows(objects, new Set([group.id, nested.id]), new Set([group.id, nested.id])).map((row) => row.object.id),
  [group.id, nested.id, child.id, root.id],
)

const byId = Object.fromEntries(objects.map((item) => [item.id, item]))
assert.deepEqual([...stageLayerSelectionExpansionIds(byId, group.id)].sort(), [group.id])
assert.deepEqual([...stageLayerSelectionExpansionIds(byId, child.id)].sort(), [group.id, nested.id])

console.log('theater stage layer tree runtime tests passed')
