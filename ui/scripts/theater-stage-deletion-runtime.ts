import assert from 'node:assert/strict'

import { createTheaterStageStore } from '../src/views/theater/stage/StageStore'

const store = createTheaterStageStore()
const group = store.addObject('group')
const child = store.addObject('text')
const sibling = store.addObject('image', true)

assert.equal(store.setParent(child.id, group.id), true)
store.setBulkSelectionMode(true)
store.setSelectedObjectIds([group.id, child.id, sibling.id], sibling.id)

assert.equal(store.removeSelectedObjects(), 3)
assert.equal(store.activeObjects.value[group.id], undefined)
assert.equal(store.activeObjects.value[child.id], undefined)
assert.equal(store.activeObjects.value[sibling.id], undefined)
assert.deepEqual(store.selection.selectedIds, [])
assert.equal(store.selection.bulkMode, true)

assert.equal(store.undo(), true)
assert.ok(store.activeObjects.value[group.id])
assert.ok(store.activeObjects.value[child.id])
assert.ok(store.activeObjects.value[sibling.id])
assert.deepEqual(store.selection.selectedIds, [group.id, child.id, sibling.id])
assert.equal(store.state.selectedObjectId, sibling.id)

const unrelated = store.addObject('button')
store.setBulkSelectionMode(true)
store.setSelectedObjectIds([unrelated.id], unrelated.id)
assert.equal(store.removeObjects([group.id]), 2)
assert.ok(store.activeObjects.value[unrelated.id])
assert.deepEqual(store.selection.selectedIds, [unrelated.id])

console.log('theater stage deletion runtime tests passed')
