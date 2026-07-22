import assert from 'node:assert/strict'

import { createTheaterStageStore } from '../src/views/theater/stage/StageStore'
import { setTheaterEffectConfig, theaterEffectConfigFromObject } from '../src/views/theater/effects/theater-effect-types'

const store = createTheaterStageStore()
assert.equal(store.state.camera.zoom, 0.5)
store.state.camera.zoom = 1
store.resetCamera()
assert.deepEqual(store.state.camera, { x: 0, y: 0, zoom: 0.5 })

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
const mediaUrl = 'https://example.com/effect.webp'

assert.equal(store.setObjectImage(created.id, mediaUrl, 'resource-1', 'image/webp'), true)

const reactiveEffect = store.activeObjects.value[created.id]
setTheaterEffectConfig(reactiveEffect, theaterEffectConfigFromObject(reactiveEffect))

assert.doesNotThrow(() => store.removeSelectedObject())
assert.equal(store.activeObjects.value[created.id], undefined)
assert.equal(store.undo(), true)
assert.equal(store.activeObjects.value[created.id]?.image?.url, mediaUrl)

console.log('theater stage store runtime tests passed')
