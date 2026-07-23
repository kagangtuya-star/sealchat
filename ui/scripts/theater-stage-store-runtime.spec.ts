import assert from 'node:assert/strict'

import { createTheaterStageStore } from '../src/views/theater/stage/StageStore'
import { setTheaterEffectConfig, theaterEffectConfigFromObject } from '../src/views/theater/effects/theater-effect-types'

const store = createTheaterStageStore()
assert.equal(store.state.camera.zoom, 0.5)
store.state.camera.zoom = 1
store.resetCamera()
assert.deepEqual(store.state.camera, { x: 0, y: 0, zoom: 0.5 })

const menuObjectStore = createTheaterStageStore()
const expectedMenuOffsets = [
  { x: 2, y: -1.5 },
  { x: -2, y: 1.5 },
  { x: -2, y: -1.5 },
  { x: 2, y: 1.5 },
]
const menuObjectTypes = ['text', 'image', 'button', 'group'] as const
for (const [index, type] of menuObjectTypes.entries()) {
  const object = menuObjectStore.addObject(type)
  assert.deepEqual({ x: object.transform.x, y: object.transform.y }, expectedMenuOffsets[index])
}
const fixedMenuImage = menuObjectStore.addObject('image', 'scene-fixed')
assert.deepEqual({ x: fixedMenuImage.transform.x, y: fixedMenuImage.transform.y }, { x: 0, y: 0 })

const activeScene = store.activeScene.value
assert.equal(activeScene.switchText, '')
assert.equal(store.updateSceneDetails(activeScene.id, '新场景名', '切换台词'), true)
assert.equal(store.activeScene.value.name, '新场景名')
assert.equal(store.activeScene.value.switchText, '切换台词')
assert.equal(store.updateSceneDetails(activeScene.id, '新场景名', '切换台词'), false)

const sceneText = store.addObject('text')
const fixedText = store.addObject('text', 'scene-fixed')
const fixedImage = store.addObject('image', 'scene-fixed')
assert.equal(store.state.liveState.sceneObjects[sceneText.id]?.id, sceneText.id)
assert.equal(store.state.persistentObjects[fixedText.id]?.id, fixedText.id)
assert.equal(store.state.persistentObjects[fixedImage.id]?.id, fixedImage.id)
assert.equal(store.isSceneFixedObject(sceneText.id), false)
assert.equal(store.isSceneFixedObject(fixedText.id), true)
assert.equal(store.setParent(sceneText.id, fixedText.id), false)

const drawing = store.addDrawing({
  tool: 'line',
  style: { stroke: '#ffffff', strokeWidth: 2, opacity: 1, fill: null, dash: 'solid' },
}, { x: 0, y: 0, width: 2, height: 2, rotation: 0 })
assert.equal(drawing.interactive, false)
assert.equal(store.addObjectAction(drawing.id, {
  id: 'drawing-send',
  type: 'chat.send',
  payload: { content: '绘制动作' },
}), true)
assert.equal(store.activeObjects.value[drawing.id].interactive, true)
assert.equal(store.activeObjects.value[drawing.id].actions.length, 1)

const nextScene = store.scenes.value.find((scene) => scene.id !== activeScene.id)
assert.ok(nextScene)
store.selectScene(nextScene.id)
assert.equal(store.activeObjects.value[sceneText.id], undefined)
assert.equal(store.activeObjects.value[fixedText.id]?.id, fixedText.id)
assert.equal(store.activeObjects.value[fixedImage.id]?.id, fixedImage.id)
store.selectObject(fixedText.id)
assert.equal(store.copySelectedObject(), true)
const pastedFixedText = store.pasteObject()
assert.ok(pastedFixedText)
assert.equal(store.isSceneFixedObject(pastedFixedText.id), true)

const created = store.addObject('effect')
assert.deepEqual({ x: created.transform.x, y: created.transform.y }, { x: 960, y: 540 })
const mediaUrl = 'https://example.com/effect.webp'

assert.equal(store.setObjectImage(created.id, mediaUrl, 'resource-1', 'image/webp'), true)

const reactiveEffect = store.activeObjects.value[created.id]
setTheaterEffectConfig(reactiveEffect, theaterEffectConfigFromObject(reactiveEffect))

assert.doesNotThrow(() => store.removeSelectedObject())
assert.equal(store.activeObjects.value[created.id], undefined)
assert.equal(store.undo(), true)
assert.equal(store.activeObjects.value[created.id]?.image?.url, mediaUrl)

const group = store.addObject('group')
const groupChild = store.addObject('image')
assert.equal(group.interactive, false)
assert.equal(group.editable, false)
assert.deepEqual(group.actions, [])
assert.equal(store.setParent(groupChild.id, group.id), true)
store.setBulkSelectionMode(true)
store.setSelectedObjectIds([group.id, groupChild.id], groupChild.id)
assert.equal(store.patchSelectedObjects({ interactive: true, editable: true }), 1)
assert.equal(store.activeObjects.value[group.id].interactive, false)
assert.equal(store.activeObjects.value[group.id].editable, false)
assert.equal(store.activeObjects.value[groupChild.id].interactive, true)
assert.equal(store.activeObjects.value[groupChild.id].editable, true)

store.clearSelection()
const adaptiveGroup = store.addObject('group')
const fixedMemberA = store.addObject('image', 'scene-fixed')
assert.equal(store.setParent(fixedMemberA.id, adaptiveGroup.id), true)
assert.equal(store.isSceneFixedObject(adaptiveGroup.id), true)
assert.equal(store.isSceneFixedObject(fixedMemberA.id), true)

const sceneMember = store.addObject('image')
assert.equal(store.canSetParent(sceneMember.id, adaptiveGroup.id), false)
assert.equal(store.setParent(sceneMember.id, adaptiveGroup.id), false)
assert.equal(store.isSceneFixedObject(sceneMember.id), false)

const fixedMemberB = store.addObject('image', 'scene-fixed')
assert.equal(store.setParent(fixedMemberB.id, adaptiveGroup.id), true)
assert.equal(store.setParent(fixedMemberA.id, null), true)
assert.equal(store.isSceneFixedObject(adaptiveGroup.id), true)
assert.equal(store.setParent(fixedMemberB.id, null), true)
assert.equal(store.isSceneFixedObject(adaptiveGroup.id), false)
assert.equal(store.isSceneFixedObject(fixedMemberA.id), true)
assert.equal(store.isSceneFixedObject(fixedMemberB.id), true)
assert.equal(store.setParent(sceneMember.id, adaptiveGroup.id), true)
assert.equal(store.isSceneFixedObject(adaptiveGroup.id), false)

store.clearSelection()
const moveTargetGroup = store.addObject('group')
const moveTargetChild = store.addObject('image')
assert.equal(store.setParent(moveTargetChild.id, moveTargetGroup.id), true)
store.clearSelection()
const movedObject = store.addObject('image')
const movedObjectOriginal = structuredClone(movedObject.transform)
const moveTargetChildOriginal = structuredClone(moveTargetChild.transform)
assert.equal(store.moveObject(movedObject.id, moveTargetGroup.id, {
  x: 12,
  y: 18,
  rotation: 7,
  scaleX: 1.2,
  scaleY: 0.8,
}, moveTargetChild.id, 'before'), true)
assert.equal(store.activeObjects.value[movedObject.id].parentId, moveTargetGroup.id)
assert.equal(store.activeObjects.value[movedObject.id].transform.x, 12)
assert.ok(store.activeObjects.value[movedObject.id].transform.z > store.activeObjects.value[moveTargetChild.id].transform.z)
assert.deepEqual(store.activeObjects.value[moveTargetChild.id].transform, moveTargetChildOriginal)
assert.deepEqual(store.selection.selectedIds, [movedObject.id])
assert.equal(store.undo(), true)
assert.equal(store.activeObjects.value[movedObject.id].parentId, null)
assert.deepEqual(store.activeObjects.value[movedObject.id].transform, movedObjectOriginal)
assert.deepEqual(store.selection.selectedIds, [movedObject.id])

const batchClipboardStore = createTheaterStageStore()
const batchSceneA = batchClipboardStore.addObject('image')
const batchSceneB = batchClipboardStore.addObject('text')
const batchFixed = batchClipboardStore.addObject('image', 'scene-fixed')
assert.equal(batchClipboardStore.addObjectAction(batchSceneA.id, {
  id: 'batch-toggle-peer',
  type: 'object.toggle',
  payload: { objectId: batchSceneB.id },
}), true)
batchClipboardStore.setBulkSelectionMode(true)
batchClipboardStore.setSelectedObjectIds([batchSceneA.id, batchSceneB.id, batchFixed.id], batchFixed.id)
assert.deepEqual(batchClipboardStore.selectionGroup.value.memberIds, [batchSceneA.id, batchSceneB.id, batchFixed.id])
assert.deepEqual(batchClipboardStore.selectionGroup.value.rootIds, [batchSceneA.id, batchSceneB.id, batchFixed.id])
assert.deepEqual(new Set(batchClipboardStore.selectionGroup.value.scopes), new Set(['scene', 'scene-fixed']))
assert.equal(batchClipboardStore.copySelectedObjects(), true)
assert.ok(batchClipboardStore.pasteObject())
const pastedBatchIds = [...batchClipboardStore.selection.selectedIds]
assert.equal(pastedBatchIds.length, 3)
assert.equal(pastedBatchIds.every((id) => ![batchSceneA.id, batchSceneB.id, batchFixed.id].includes(id)), true)
assert.equal(pastedBatchIds.filter((id) => batchClipboardStore.isSceneFixedObject(id)).length, 1)
const pastedPeerAction = batchClipboardStore.activeObjects.value[pastedBatchIds[0]].actions[0]
assert.equal(pastedPeerAction.type, 'object.toggle')
if (pastedPeerAction.type === 'object.toggle') {
  assert.equal(pastedPeerAction.payload.objectId, pastedBatchIds[1])
}
assert.equal(batchClipboardStore.patchSelectedObjects({ locked: true }), 3)
assert.deepEqual(new Set(batchClipboardStore.selectionGroup.value.lockedIds), new Set(pastedBatchIds))
assert.equal(batchClipboardStore.undo(), true)
assert.deepEqual(batchClipboardStore.selectionGroup.value.lockedIds, [])

const selectionParent = batchClipboardStore.addObject('group')
const selectionChild = batchClipboardStore.addObject('image')
assert.equal(batchClipboardStore.setParent(selectionChild.id, selectionParent.id), true)
batchClipboardStore.setSelectedObjectIds([selectionParent.id, selectionChild.id], selectionChild.id)
assert.deepEqual(batchClipboardStore.selectionGroup.value.memberIds, [selectionParent.id, selectionChild.id])
assert.deepEqual(batchClipboardStore.selectionGroup.value.rootIds, [selectionParent.id])

console.log('theater stage store runtime tests passed')
