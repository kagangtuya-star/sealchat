import assert from 'node:assert/strict'

import Konva from 'konva'

import type { StageObject } from '../src/views/theater/shared/stage-types'
import { syncStageObjectHierarchy } from '../src/views/theater/stage/stage-layering.js'
import { createTheaterStageStore } from '../src/views/theater/stage/StageStore.js'

const makeObject = (id: string, z: number, parentId: string | null = null): StageObject => ({
  id,
  parentId,
  type: parentId === null && id === 'group' ? 'group' : 'drawing',
  name: id,
  transform: { x: 0, y: 0, width: 1, height: 1, rotation: 0, scaleX: 1, scaleY: 1, z, order: z },
  visible: true,
  locked: false,
  aspectRatioLocked: true,
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

const store = createTheaterStageStore()
const initialGroup = store.addObject('group')
const initialChild = store.addObject('text')
assert.equal(store.setParent(initialChild.id, initialGroup.id), true)
const rootDrawing = store.addDrawing({
  tool: 'rectangle',
  style: { stroke: '#ffffff', strokeWidth: 2, opacity: 1, fill: null, dash: 'solid' },
}, { x: 0, y: 0, width: 2, height: 2, rotation: 0 })
assert.equal(initialChild.aspectRatioLocked, true)
assert.equal(store.setParent(initialChild.id, rootDrawing.id), false)

const nestedGroup = store.addObject('group')
assert.equal(store.setParent(nestedGroup.id, initialGroup.id), true)
assert.equal(store.setParent(initialGroup.id, nestedGroup.id), false)
assert.equal(store.reparentObject(initialChild.id, null, { x: 3, y: 4, rotation: 25, scaleX: 1.5, scaleY: 0.75 }), true)
assert.equal(initialChild.parentId, null)
assert.deepEqual(
  {
    x: initialChild.transform.x,
    y: initialChild.transform.y,
    rotation: initialChild.transform.rotation,
    scaleX: initialChild.transform.scaleX,
    scaleY: initialChild.transform.scaleY,
  },
  { x: 3, y: 4, rotation: 25, scaleX: 1.5, scaleY: 0.75 },
)

const legacySnapshot = store.getSnapshot() as any
delete legacySnapshot.liveState.sceneObjects[initialChild.id].aspectRatioLocked
store.replaceState(legacySnapshot)
assert.equal(store.activeObjects.value[initialChild.id].aspectRatioLocked, true)

const placementStore = createTheaterStageStore()
const baseObject = placementStore.addObject('text')
placementStore.selectObject(baseObject.id)
const insertedObject = placementStore.addObject('image')
assert.equal(insertedObject.parentId, null)
assert.ok(insertedObject.transform.z > baseObject.transform.z)

placementStore.selectObject(null)
const topObject = placementStore.addObject('button')
assert.ok(topObject.transform.z > insertedObject.transform.z)

const groupObject = placementStore.addObject('group')
placementStore.selectObject(groupObject.id)
const groupedObject = placementStore.addObject('text')
assert.equal(groupedObject.parentId, groupObject.id)

placementStore.selectObject(baseObject.id)
assert.equal(placementStore.copySelectedObject(), true)
const copiedObject = placementStore.pasteObject()!
assert.equal(copiedObject.parentId, baseObject.parentId)
assert.ok(copiedObject.transform.z > baseObject.transform.z)

placementStore.selectObject(groupObject.id)
assert.equal(placementStore.copySelectedObject(), true)
const copiedGroup = placementStore.pasteObject()!
const copiedGroupChild = Object.values(placementStore.activeObjects.value)
  .find((object) => object.parentId === copiedGroup.id)
assert.equal(copiedGroup.parentId, groupObject.parentId)
assert.ok(copiedGroup.transform.z > groupObject.transform.z)
assert.equal(copiedGroupChild?.transform.z, groupedObject.transform.z)

console.log('theater stage layering runtime tests passed')
