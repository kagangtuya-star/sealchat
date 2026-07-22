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
