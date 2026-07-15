import assert from 'node:assert/strict'

import Konva from 'konva'

import type { StageObject } from '../src/views/theater/shared/stage-types'
import { syncStageObjectHierarchy } from '../src/views/theater/stage/stage-layering.js'

const makeObject = (id: string, z: number, parentId: string | null = null): StageObject => ({
  id,
  parentId,
  type: parentId === null && id === 'group' ? 'group' : 'shape',
  name: id,
  transform: { x: 0, y: 0, width: 1, height: 1, rotation: 0, z, order: z },
  visible: true,
  locked: false,
  sizeLocked: false,
  interactive: true,
  editable: false,
  fill: '#000000',
  metadata: {},
  actions: [],
})

const root = new Konva.Group()
const objects = {
  scene: makeObject('scene', 3),
  persistent: makeObject('persistent', 1),
  group: makeObject('group', 2),
  child: makeObject('child', 1, 'group'),
}
const nodes = new Map<string, Konva.Group>()
Object.values(objects).forEach((object) => {
  const node = new Konva.Group()
  node.setAttr('stageObjectId', object.id)
  nodes.set(object.id, node)
})
nodes.get('group')!.add(new Konva.Rect())

syncStageObjectHierarchy(objects, nodes, root)

assert.deepEqual(
  root.getChildren().map((node) => (node as Konva.Node).getAttr('stageObjectId')),
  ['persistent', 'group', 'scene'],
)
assert.equal(nodes.get('persistent')!.getParent(), root)
assert.equal(nodes.get('scene')!.getParent(), root)
assert.equal(nodes.get('child')!.getParent(), nodes.get('group'))
assert.equal(nodes.get('child')!.zIndex(), 1)

objects.persistent.transform.z = 4
objects.persistent.transform.order = 4
syncStageObjectHierarchy(objects, nodes, root)
assert.deepEqual(
  root.getChildren().map((node) => (node as Konva.Node).getAttr('stageObjectId')),
  ['group', 'scene', 'persistent'],
)

console.log('theater stage layering runtime tests passed')
